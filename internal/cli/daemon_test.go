package cli

import (
	"os"
	"strings"
	"testing"
)

func TestProcessOneIssueStartsCodexAndWritesState(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")

	github := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Add export",
		URL:    "https://github.com/example/repo/issues/42",
		Labels: []githubLabel{{Name: readyLabel}},
	})
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{
		target:  "wsl",
		repoURL: "https://github.com/example/repo.git",
	})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "started issue #42") {
		t.Fatalf("result = %q", result)
	}
	state, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Job == nil || state.Job.Issue.Number != 42 {
		t.Fatalf("state job = %#v", state.Job)
	}
	if state.Job.Tmux == nil || state.Job.Tmux.Window != "issue-42" {
		t.Fatalf("tmux = %#v", state.Job.Tmux)
	}
	if _, err := os.Stat(state.Job.Codex.JSONLogPath); err == nil {
		t.Fatalf("json log should not be created before codex runs")
	}
	if !commands.ranPrefix("tmux", "new-window", "-t", tmuxSessionName, "-n", "issue-42") {
		t.Fatalf("commands = %#v, want tmux new-window", commands.calls)
	}
}

func TestProcessOneIssueNoReadyIssue(t *testing.T) {
	github := newFakeGitHubClient(githubIssue{
		Number: 42,
		Labels: []githubLabel{{Name: runningLabel}},
	})

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if result != "no ready issue" {
		t.Fatalf("result = %q, want no ready issue", result)
	}
}
