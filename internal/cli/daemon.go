package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type daemonDeps struct {
	github   githubClient
	worktree worktreeManager
	runner   tmuxRunner
}

type daemonOptions struct {
	target  string
	repo    string
	repoURL string
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(data)
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

func runMultiRepoDaemonLoop(stdout, stderr io.Writer, repos []repoEntry, target string, interval time.Duration) int {
	safeStdout := &lockedWriter{w: stdout}
	safeStderr := &lockedWriter{w: stderr}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	configPath := defaultRepoConfigFile(target)
	for {
		current := repos
		if config, err := readRepoConfig(configPath); err == nil {
			current = enabledRepoEntries(config)
		}
		result, err := processRegisteredReposOnce(current, target)
		if err != nil {
			fmt.Fprintf(safeStderr, "daemon error: %v\n", err)
		}
		if result != "" {
			fmt.Fprintln(safeStdout, result)
		}
		<-ticker.C
	}
}

func processRegisteredReposOnce(repos []repoEntry, target string) (string, error) {
	results := make([]string, len(repos))
	var wg sync.WaitGroup
	errs := make(chan error, len(repos))
	for i, repo := range repos {
		index := i
		entry := repo
		wg.Add(1)
		go func() {
			defer wg.Done()
			deps := daemonDeps{
				github:   ghClient{repo: entry.Repository},
				worktree: newWorktreeManager(nil),
				runner:   newTmuxRunner(nil),
			}
			result, err := processOneIssue(deps, daemonOptions{target: target, repo: entry.Repository, repoURL: entry.RepoURL})
			if err != nil {
				errs <- fmt.Errorf("%s: %w", entry.Repository, err)
			}
			results[index] = entry.Repository + ": " + result
		}()
	}
	wg.Wait()
	close(errs)
	var err error
	for candidate := range errs {
		if err == nil {
			err = candidate
		}
	}
	return strings.Join(results, "\n"), err
}

func processOneIssue(deps daemonDeps, opts daemonOptions) (string, error) {
	ctx := context.Background()
	if result, completed, err := completeCurrentJob(ctx, deps, opts); completed || err != nil {
		return result, err
	}
	currentState, err := readStateFile(opts.stateFilePath())
	if err != nil {
		currentState = nil
	}
	if result, handled, err := processCampaign(ctx, deps, opts, currentState); handled || err != nil {
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

	spec, err := deps.worktree.PrepareForRepo(ctx, opts.target, opts.repo, claim.Issue, opts.repoURL)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "worktree", err)
		_ = writeBlockedState(opts, claim.Issue, claim.ClaimID, spec, nil, "worktree", err)
		return "", err
	}

	runSpec := buildCodexRunSpecForRepo(opts.target, opts.repo, issue.Number, spec.Path, "")
	if err := writePromptFile(runSpec.PromptPath, claim.Issue); err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "prompt", err)
		_ = writeBlockedState(opts, claim.Issue, claim.ClaimID, spec, nil, "prompt", err)
		return "", err
	}

	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "runner", err)
		_ = writeBlockedState(opts, claim.Issue, claim.ClaimID, spec, nil, "runner", err)
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
				Number:     issue.Number,
				Title:      claim.Issue.Title,
				URL:        claim.Issue.URL,
				Repository: opts.repo,
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
	if err := writeStateFile(opts.stateFilePath(), state); err != nil {
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

func (opts daemonOptions) stateFilePath() string {
	return stateFileForRepo(opts.target, opts.repo)
}

func writeBlockedState(opts daemonOptions, issue githubIssue, claimID string, spec worktreeSpec, tmux *tmuxRef, stage string, cause error) error {
	message := fmt.Sprintf("%s: %s", stage, cause)
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        opts.target,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Daemon: stateDaemon{
			Service: "run-weaver.service",
		},
		Job: &stateJob{
			Issue: issueRef{
				Number:     issue.Number,
				Title:      issue.Title,
				URL:        issue.URL,
				Repository: opts.repo,
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
	return writeStateFile(opts.stateFilePath(), state)
}
