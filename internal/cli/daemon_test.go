package cli

import (
	"os"
	"path/filepath"
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

func TestProcessOneIssueStartsOldestReadyIssue(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")

	github := newFakeGitHubClient(githubIssue{Number: 3, Title: "Third", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 3, Title: "Third", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 1, Title: "First", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 2, Title: "Second", Labels: []githubLabel{{Name: readyLabel}}},
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "started issue #1") {
		t.Fatalf("result = %q, want issue #1", result)
	}
	state, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Job == nil || state.Job.Issue.Number != 1 {
		t.Fatalf("state job = %#v, want issue #1", state.Job)
	}
}

func TestProcessOneIssueDoesNotStartAnotherJobWhileRunning(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 1, Title: "Running"},
			LabelState: runningLabel,
			Codex:      &codexState{LastMessagePath: tempDir + "/missing.txt"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 2, Title: "Next", Labels: []githubLabel{{Name: readyLabel}}})
	commands := &fakeCommandRunner{}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if result != "issue #1 is still running" {
		t.Fatalf("result = %q", result)
	}
	if commands.ranPrefix("tmux", "new-window") {
		t.Fatalf("commands = %#v, should not start another job", commands.calls)
	}
}

func TestProcessOneIssueSkipsWaitingDependencyAndStartsIndependentIssue(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 12, Title: "Independent", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 10, Title: "Dependency", Labels: []githubLabel{{Name: runningLabel}}},
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "started issue #12") {
		t.Fatalf("result = %q, want issue #12", result)
	}
}

func TestProcessOneIssueUsesDependencyBranchForStackedIssue(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		CompletedIssues: []completedIssue{{
			Issue:  issueRef{Number: 10, Title: "Dependency"},
			Branch: "codex/issue-10-dependency",
			PRURL:  "https://github.com/example/repo/pull/10",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 10, Title: "Dependency", Labels: []githubLabel{{Name: doneLabel}}},
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	clonePath := filepath.Join(tempDir, "data", "run-weaver", "src")
	if !commands.ran("git", "-C", clonePath, "worktree", "add", "-B", "codex/issue-11-stacked", filepath.Join(tempDir, "data", "run-weaver", "worktrees", "issue-11"), "origin/codex/issue-10-dependency") {
		t.Fatalf("commands = %#v, want worktree from dependency branch", commands.calls)
	}
	state, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Job == nil || state.Job.BaseBranch != "codex/issue-10-dependency" || len(state.Job.Dependencies) != 1 {
		t.Fatalf("state job = %#v, want stacked dependency", state.Job)
	}
}

func TestProcessOneIssueBlocksDependencyWithoutPRInfoAndContinues(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 11, Title: "Stacked", Body: "depends: #10", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 12, Title: "Independent", Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 10, Title: "Dependency", Labels: []githubLabel{{Name: doneLabel}}},
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !github.added[blockedLabel] {
		t.Fatal("blocked label was not added for unresolved dependency info")
	}
	if !strings.Contains(result, "started issue #12") {
		t.Fatalf("result = %q, want issue #12", result)
	}
}

func TestProcessOneIssueUsesRepoSpecificStateAndTmuxWindow(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))

	repoA := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Repo A task",
		URL:    "https://github.com/example/a/issues/42",
		Labels: []githubLabel{{Name: readyLabel}},
	})
	repoB := newFakeGitHubClient(githubIssue{
		Number: 7,
		Title:  "Repo B task",
		URL:    "https://github.com/example/b/issues/7",
		Labels: []githubLabel{{Name: readyLabel}},
	})
	commands := &fakeCommandRunner{failFirstHasSession: true}

	if _, err := processOneIssue(daemonDeps{
		github:   repoA,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repo: "example/a", repoURL: "https://github.com/example/a.git"}); err != nil {
		t.Fatal(err)
	}
	if _, err := processOneIssue(daemonDeps{
		github:   repoB,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repo: "example/b", repoURL: "https://github.com/example/b.git"}); err != nil {
		t.Fatal(err)
	}

	stateA, err := readStateFile(stateFileForRepo("wsl", "example/a"))
	if err != nil {
		t.Fatal(err)
	}
	stateB, err := readStateFile(stateFileForRepo("wsl", "example/b"))
	if err != nil {
		t.Fatal(err)
	}
	if stateA.Job == nil || stateA.Job.Issue.Repository != "example/a" || stateA.Job.Tmux.Window != "example-a-issue-42" {
		t.Fatalf("state A = %#v", stateA.Job)
	}
	if stateB.Job == nil || stateB.Job.Issue.Repository != "example/b" || stateB.Job.Tmux.Window != "example-b-issue-7" {
		t.Fatalf("state B = %#v", stateB.Job)
	}
	if !strings.Contains(stateA.Job.Worktree, filepath.Join("repos", "example-a", "worktrees", "issue-42")) {
		t.Fatalf("repo A worktree = %q", stateA.Job.Worktree)
	}
	if !strings.Contains(stateB.Job.Worktree, filepath.Join("repos", "example-b", "worktrees", "issue-7")) {
		t.Fatalf("repo B worktree = %q", stateB.Job.Worktree)
	}
}

func TestProcessOneIssueNoReadyIssue(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
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

func TestProcessOneIssueWritesBlockedStateAfterClaimFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Add export",
		Labels: []githubLabel{{Name: readyLabel}},
	})

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl"})

	if err == nil {
		t.Fatal("expected worktree error")
	}
	state, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Job == nil || state.Job.LabelState != blockedLabel {
		t.Fatalf("state job = %#v, want blocked", state.Job)
	}
	if state.Job.LastError == nil || !strings.Contains(*state.Job.LastError, "worktree") {
		t.Fatalf("last error = %#v, want worktree error", state.Job.LastError)
	}
	if !github.added[blockedLabel] {
		t.Fatal("blocked label was not added")
	}
}

func TestProcessOneIssueBlocksBeforeCodexWhenRequiredDopplerMissing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Add export",
		Labels: []githubLabel{{Name: readyLabel}},
	})
	commands := &fakeCommandRunner{}
	originalLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name == "doppler" {
			return "", errFakeCommand
		}
		return originalLookPath(name)
	}
	defer func() { lookPath = originalLookPath }()

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git", dopplerMode: dopplerModeRequired})

	if err == nil {
		t.Fatal("expected doppler error")
	}
	state, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Job == nil || state.Job.LabelState != blockedLabel || state.Job.LastError == nil || !strings.Contains(*state.Job.LastError, "doppler") {
		t.Fatalf("state job = %#v, want doppler blocked", state.Job)
	}
	if commands.ranPrefix("tmux", "new-window") {
		t.Fatalf("commands = %#v, should not start codex", commands.calls)
	}
}
