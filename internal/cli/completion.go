package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const rateLimitResumePrompt = `Continue the interrupted work from the previous session. Review the existing worktree, git diff, and prior context before making changes. Preserve previous progress and continue toward the original GitHub Issue goal.`

func completeCurrentJob(ctx context.Context, deps daemonDeps, opts daemonOptions) (string, bool, error) {
	statePath := opts.stateFilePath()
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
	if resumed, err := resumeRateLimitedCodex(ctx, deps, opts, state); resumed || err != nil {
		return "resumed rate-limited Codex session", true, err
	}
	if !codexCompletionReady(opts.target, state.Job) {
		return "", false, nil
	}
	if advanced, result, err := advanceCampaignPipeline(ctx, deps, opts, state); advanced || err != nil {
		return result, true, err
	}

	committed, err := deps.worktree.CommitAll(ctx, state.Job.Worktree, fmt.Sprintf("Implement issue #%d", state.Job.Issue.Number))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git commit", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git commit", err)
		return "", true, err
	}
	if !committed {
		err := fmt.Errorf("codex completed without file changes")
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "codex result", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "codex result", err)
		return "", true, err
	}

	if err := deps.worktree.PushBranch(ctx, state.Job.Worktree, state.Job.Branch); err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git push", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git push", err)
		return "", true, err
	}

	prURL, err := deps.github.CreateDraftPR(ctx, buildDraftPRSpec(githubIssue{
		Number: state.Job.Issue.Number,
		Title:  state.Job.Issue.Title,
		URL:    state.Job.Issue.URL,
	}, state.Job.Branch))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "draft PR", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "draft PR", err)
		return "", true, err
	}
	if err := markDone(ctx, deps.github, state.Job.Issue.Number, prURL, state.Job.Branch, state.Job.Worktree); err != nil {
		return "", true, err
	}

	completedIssueNumber := state.Job.Issue.Number
	wasCampaignTask := state.Job.CampaignTaskID != ""
	markCampaignTaskCompleted(state, prURL)
	if state.Campaign != nil && state.Campaign.Status == campaignStatusDecisionRequired {
		_ = markBlocked(ctx, deps.github, state.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
	}
	if state.Campaign != nil && state.Campaign.Status == campaignStatusDone {
		_ = markCampaignDone(ctx, deps.github, state.Campaign)
	}
	state.Job.LabelState = doneLabel
	state.Job.LastError = nil
	if wasCampaignTask {
		state.Job = nil
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(statePath, *state); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("completed issue #%d with draft PR %s", completedIssueNumber, prURL), true, nil
}

func resumeRateLimitedCodex(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (bool, error) {
	if state == nil || state.Job == nil || state.Job.Codex == nil || state.Job.Codex.JSONLogPath == "" || state.Job.Codex.LastMessagePath == "" {
		return false, nil
	}
	if tmuxState(opts.target, state.Job.Tmux) == "window_exists" {
		return false, nil
	}
	if !codexLogLooksRateLimited(state.Job.Codex.JSONLogPath) {
		return false, nil
	}
	sessionID := stringPtrValue(state.Job.Codex.SessionID)
	if sessionID == "" {
		sessionID = codexSessionIDFromLog(state.Job.Codex.JSONLogPath)
	}
	if sessionID == "" {
		sessionID = "--last"
	}
	resumePromptPath := state.Job.Codex.LastMessagePath + ".resume.md"
	if err := os.WriteFile(resumePromptPath, []byte(rateLimitResumePrompt), 0o600); err != nil {
		return false, err
	}
	spec := codexRunSpec{
		IssueNumber:     state.Job.Issue.Number,
		Worktree:        state.Job.Worktree,
		PromptPath:      resumePromptPath,
		JSONLogPath:     state.Job.Codex.JSONLogPath,
		LastMessagePath: state.Job.Codex.LastMessagePath,
		ResumeSessionID: sessionID,
	}
	tmux, err := deps.runner.StartCodex(ctx, spec)
	if err != nil {
		return false, err
	}
	state.Job.Tmux = &tmux
	if sessionID != "--last" {
		state.Job.Codex.SessionID = &sessionID
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		return false, err
	}
	return true, nil
}

func codexLogLooksRateLimited(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(data))
	return strings.Contains(text, "rate limit") || strings.Contains(text, "rate_limit") || strings.Contains(text, "ratelimit")
}

func codexSessionIDFromLog(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			continue
		}
		if sessionID := findSessionID(value); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func findSessionID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "session_id" || key == "sessionId" {
				if text, ok := nested.(string); ok {
					return text
				}
			}
		}
		for _, nested := range typed {
			if found := findSessionID(nested); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := findSessionID(nested); found != "" {
				return found
			}
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
