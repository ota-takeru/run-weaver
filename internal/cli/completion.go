package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

func completeCurrentJob(ctx context.Context, deps daemonDeps, opts daemonOptions) (string, bool, error) {
	statePath := defaultStateFile(opts.target)
	state, err := readStateFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	if state.Job == nil || state.Job.LabelState != runningLabel || state.Job.Codex == nil {
		return "", false, nil
	}
	if !codexCompletionReady(opts.target, state.Job) {
		return "", false, nil
	}

	committed, err := deps.worktree.CommitAll(ctx, state.Job.Worktree, fmt.Sprintf("Implement issue #%d", state.Job.Issue.Number))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git commit", err)
		_ = writeBlockedState(opts.target, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git commit", err)
		return "", true, err
	}
	if !committed {
		err := fmt.Errorf("codex completed without file changes")
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "codex result", err)
		_ = writeBlockedState(opts.target, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "codex result", err)
		return "", true, err
	}

	if err := deps.worktree.PushBranch(ctx, state.Job.Worktree, state.Job.Branch); err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git push", err)
		_ = writeBlockedState(opts.target, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git push", err)
		return "", true, err
	}

	prURL, err := deps.github.CreateDraftPR(ctx, buildDraftPRSpec(githubIssue{
		Number: state.Job.Issue.Number,
		Title:  state.Job.Issue.Title,
		URL:    state.Job.Issue.URL,
	}, state.Job.Branch))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "draft PR", err)
		_ = writeBlockedState(opts.target, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "draft PR", err)
		return "", true, err
	}
	if err := markDone(ctx, deps.github, state.Job.Issue.Number, prURL, state.Job.Branch, state.Job.Worktree); err != nil {
		return "", true, err
	}

	state.Job.LabelState = doneLabel
	state.Job.LastError = nil
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(statePath, *state); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("completed issue #%d with draft PR %s", state.Job.Issue.Number, prURL), true, nil
}

func codexCompletionReady(target string, job *stateJob) bool {
	if job == nil || job.Codex == nil || job.Codex.LastMessagePath == "" {
		return false
	}
	if _, err := os.Stat(job.Codex.LastMessagePath); err != nil {
		return false
	}
	return tmuxState(target, job.Tmux) != "window_exists"
}

func markDone(ctx context.Context, client githubClient, issueNumber int, prURL, branch, worktree string) error {
	_ = client.RemoveLabel(ctx, issueNumber, runningLabel)
	_ = client.RemoveLabel(ctx, issueNumber, blockedLabel)
	if err := client.EnsureLabel(ctx, doneLabel); err != nil {
		return err
	}
	if err := client.AddLabel(ctx, issueNumber, doneLabel); err != nil {
		return err
	}
	body := fmt.Sprintf(`run-weaver completed this issue.

- draft PR: %s
- branch: %s
- worktree: %s
`, prURL, branch, worktree)
	return client.Comment(ctx, issueNumber, body)
}
