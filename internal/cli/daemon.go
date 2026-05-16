package cli

import (
	"context"
	"fmt"
	"io"
	"time"
)

type daemonDeps struct {
	github   githubClient
	worktree worktreeManager
	runner   tmuxRunner
}

type daemonOptions struct {
	target  string
	repoURL string
}

func runDaemonLoop(stdout, stderr io.Writer, deps daemonDeps, opts daemonOptions, interval time.Duration) int {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result, err := processOneIssue(deps, opts)
		if err != nil {
			fmt.Fprintf(stderr, "daemon error: %v\n", err)
		} else {
			fmt.Fprintln(stdout, result)
		}
		<-ticker.C
	}
}

func processOneIssue(deps daemonDeps, opts daemonOptions) (string, error) {
	ctx := context.Background()
	if result, completed, err := completeCurrentJob(ctx, deps, opts); completed || err != nil {
		return result, err
	}

	issues, err := deps.github.ListReadyIssues(ctx)
	if err != nil {
		return "", err
	}
	if len(issues) == 0 {
		return "no ready issue", nil
	}

	issue := issues[0]
	claim, err := claimIssue(ctx, deps.github, issue.Number, opts.target)
	if err != nil {
		return "", err
	}
	switch claim.Outcome {
	case claimSkipped:
		return fmt.Sprintf("skipped issue #%d", issue.Number), nil
	case claimLost:
		return fmt.Sprintf("lost claim for issue #%d", issue.Number), nil
	case claimWon:
	default:
		return "", fmt.Errorf("unknown claim outcome %q", claim.Outcome)
	}

	spec, err := deps.worktree.Prepare(ctx, opts.target, claim.Issue, opts.repoURL)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "worktree", err)
		_ = writeBlockedState(opts.target, claim.Issue, claim.ClaimID, spec, nil, "worktree", err)
		return "", err
	}

	runSpec := buildCodexRunSpec(opts.target, issue.Number, spec.Path)
	if err := writePromptFile(runSpec.PromptPath, claim.Issue); err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "prompt", err)
		_ = writeBlockedState(opts.target, claim.Issue, claim.ClaimID, spec, nil, "prompt", err)
		return "", err
	}

	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "runner", err)
		_ = writeBlockedState(opts.target, claim.Issue, claim.ClaimID, spec, nil, "runner", err)
		return "", err
	}

	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        opts.target,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Daemon: stateDaemon{
			Service: "run-weaver.service",
		},
		Job: &stateJob{
			Issue: issueRef{
				Number: issue.Number,
				Title:  claim.Issue.Title,
				URL:    claim.Issue.URL,
			},
			LabelState: "running",
			Branch:     spec.Branch,
			Worktree:   spec.Path,
			ClaimID:    claim.ClaimID,
			ClaimedAt:  time.Now().UTC().Format(time.RFC3339),
			Tmux:       &tmux,
			Codex: &codexState{
				LastMessagePath: runSpec.LastMessagePath,
				JSONLogPath:     runSpec.JSONLogPath,
			},
		},
	}
	if err := writeStateFile(defaultStateFile(opts.target), state); err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "state file", err)
		return "", err
	}

	return fmt.Sprintf("started issue #%d in tmux %s:%s", issue.Number, tmux.Session, tmux.Window), nil
}

func markBlocked(ctx context.Context, client githubClient, issueNumber int, stage string, err error) error {
	_ = client.RemoveLabel(ctx, issueNumber, runningLabel)
	_ = client.RemoveLabel(ctx, issueNumber, doneLabel)
	if ensureErr := client.EnsureLabel(ctx, blockedLabel); ensureErr != nil {
		return ensureErr
	}
	if labelErr := client.AddLabel(ctx, issueNumber, blockedLabel); labelErr != nil {
		return labelErr
	}
	body := fmt.Sprintf("run-weaver blocked during %s.\n\nError: %s\n", stage, err)
	return client.Comment(ctx, issueNumber, body)
}

func writeBlockedState(target string, issue githubIssue, claimID string, spec worktreeSpec, tmux *tmuxRef, stage string, cause error) error {
	message := fmt.Sprintf("%s: %s", stage, cause)
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        target,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Daemon: stateDaemon{
			Service: "run-weaver.service",
		},
		Job: &stateJob{
			Issue: issueRef{
				Number: issue.Number,
				Title:  issue.Title,
				URL:    issue.URL,
			},
			LabelState: blockedLabel,
			Branch:     spec.Branch,
			Worktree:   spec.Path,
			ClaimID:    claimID,
			ClaimedAt:  time.Now().UTC().Format(time.RFC3339),
			Tmux:       tmux,
			LastError:  &message,
		},
	}
	return writeStateFile(defaultStateFile(target), state)
}
