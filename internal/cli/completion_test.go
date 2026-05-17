package cli

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCompleteCurrentJobCreatesDraftPR(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		UpdatedAt:     "2026-05-16T09:31:10Z",
		Job: &stateJob{
			Issue: issueRef{
				Number: 42,
				Title:  "Add export",
				URL:    "https://github.com/example/repo/issues/42",
			},
			LabelState: runningLabel,
			Branch:     "codex/issue-42-add-export",
			Worktree:   tempDir + "/worktrees/issue-42",
			Codex: &codexState{
				LastMessagePath: lastMessage,
			},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"})
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + tempDir + "/worktrees/issue-42\x00status\x00--porcelain": []byte(" M index.html\n"),
		},
	}

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected completion")
	}
	if result == "" {
		t.Fatal("expected result")
	}
	if github.draftPR.Head != "codex/issue-42-add-export" {
		t.Fatalf("draft PR = %#v", github.draftPR)
	}
	if !github.added[doneLabel] {
		t.Fatal("done label was not added")
	}
	if !commands.ran("git", "-C", tempDir+"/worktrees/issue-42", "push", "-u", "origin", "codex/issue-42-add-export") {
		t.Fatalf("commands = %#v, want git push", commands.calls)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job.LabelState != doneLabel {
		t.Fatalf("state label = %q, want done", updated.Job.LabelState)
	}
	if len(updated.CompletedIssues) != 1 || updated.CompletedIssues[0].Issue.Number != 42 || updated.CompletedIssues[0].PRURL == "" {
		t.Fatalf("completed issues = %#v", updated.CompletedIssues)
	}
}

func TestCompleteCurrentJobCreatesStackedDraftPR(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:        issueRef{Number: 11, Title: "Stacked"},
			LabelState:   runningLabel,
			Branch:       "codex/issue-11-stacked",
			Worktree:     tempDir + "/worktrees/issue-11",
			Dependencies: []issueDependency{{IssueNumber: 10, Branch: "codex/issue-10-base", PRURL: "https://github.com/example/repo/pull/10"}},
			Codex:        &codexState{LastMessagePath: lastMessage},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 11, Title: "Stacked"})
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + tempDir + "/worktrees/issue-11\x00status\x00--porcelain": []byte(" M index.html\n"),
		},
	}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected completion")
	}
	if github.draftPR.Base != "codex/issue-10-base" {
		t.Fatalf("draft PR = %#v, want stacked base", github.draftPR)
	}
	if !strings.Contains(github.draftPR.Body, "Depends on #10: https://github.com/example/repo/pull/10") {
		t.Fatalf("body = %q", github.draftPR.Body)
	}
}

func TestCompleteCurrentJobBlocksWithoutChanges(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
			Branch:     "codex/issue-42-add-export",
			Worktree:   tempDir + "/worktrees/issue-42",
			Codex:      &codexState{LastMessagePath: lastMessage},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"})

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl"})

	if err == nil {
		t.Fatal("expected no changes error")
	}
	if !completed {
		t.Fatal("expected completed blocked flow")
	}
	if !github.added[blockedLabel] {
		t.Fatal("blocked label was not added")
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job.LabelState != blockedLabel {
		t.Fatalf("state label = %q, want blocked", updated.Job.LabelState)
	}
}

func TestCompleteCurrentJobWaitsForLastMessage(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42},
			LabelState: runningLabel,
			Codex:      &codexState{LastMessagePath: tempDir + "/missing.txt"},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   newFakeGitHubClient(githubIssue{Number: 42}),
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if completed {
		t.Fatal("completion should wait for last message")
	}
}

func TestCompleteCurrentJobBlocksFailedCodexStartup(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	jsonLog := tempDir + "/codex.jsonl"
	if err := os.WriteFile(jsonLog, []byte("bash: line 1: codex: command not found\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
			Branch:     "codex/issue-42-add-export",
			Worktree:   tempDir + "/worktrees/issue-42",
			Tmux:       &tmuxRef{Session: tmuxSessionName, Window: "issue-42"},
			Codex: &codexState{
				JSONLogPath:     jsonLog,
				LastMessagePath: tempDir + "/missing-last-message.txt",
			},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"})

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed || result != "blocked failed Codex startup" {
		t.Fatalf("result = %q completed = %v", result, completed)
	}
	if !github.added[blockedLabel] || !github.removed[runningLabel] {
		t.Fatalf("labels added=%#v removed=%#v", github.added, github.removed)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job == nil || updated.Job.LabelState != blockedLabel || updated.Job.LastError == nil || !strings.Contains(*updated.Job.LastError, "service PATH") {
		t.Fatalf("state job = %#v, want blocked service PATH error", updated.Job)
	}
}

func TestCompleteCurrentJobResumesRateLimitedCodex(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	jsonLog := tempDir + "/codex.jsonl"
	if err := os.WriteFile(jsonLog, []byte(`{"session_id":"session-123"}`+"\n"+`{"error":"rate limit reached"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
			Worktree:   tempDir + "/worktrees/issue-42",
			Codex: &codexState{
				JSONLogPath:     jsonLog,
				LastMessagePath: tempDir + "/last-message.txt",
			},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   newFakeGitHubClient(githubIssue{Number: 42}),
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("rate limit resume should handle this daemon cycle")
	}
	if result != "resumed rate-limited Codex session" {
		t.Fatalf("result = %q", result)
	}
	if !commands.ranPrefix("tmux", "new-window", "-t", tmuxSessionName, "-n", "issue-42") {
		t.Fatalf("commands = %#v, want tmux new-window", commands.calls)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job.Codex.SessionID == nil || *updated.Job.Codex.SessionID != "session-123" {
		t.Fatalf("session id = %#v", updated.Job.Codex.SessionID)
	}
}

func TestCompleteCurrentJobResumesRateLimitedCodexWithRepoWindow(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	jsonLog := tempDir + "/codex.jsonl"
	if err := os.WriteFile(jsonLog, []byte(`{"session_id":"session-123"}`+"\n"+`{"error":"rate limit reached"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export", Repository: "example/repo"},
			LabelState: runningLabel,
			Worktree:   tempDir + "/worktrees/issue-42",
			Codex: &codexState{
				JSONLogPath:     jsonLog,
				LastMessagePath: tempDir + "/last-message.txt",
			},
		},
	}
	if err := writeStateFile(stateFileForRepo("wsl", "example/repo"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   newFakeGitHubClient(githubIssue{Number: 42}),
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repo: "example/repo"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("rate limit resume should handle this daemon cycle")
	}
	if !commands.ranPrefix("tmux", "new-window", "-t", tmuxSessionName, "-n", "example-repo-issue-42") {
		t.Fatalf("commands = %#v, want repo-specific window", commands.calls)
	}
}

func TestCodexSessionIDFromLogFindsNestedSessionID(t *testing.T) {
	path := t.TempDir() + "/codex.jsonl"
	if err := os.WriteFile(path, []byte(`{"event":{"sessionId":"session-abc"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := codexSessionIDFromLog(path); got != "session-abc" {
		t.Fatalf("session id = %q", got)
	}
}

func TestCompleteCurrentJobResumesRateLimitedCodexWithLastFallback(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	jsonLog := tempDir + "/codex.jsonl"
	if err := os.WriteFile(jsonLog, []byte(`{"error":"rate limit reached"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
			Worktree:   tempDir + "/worktrees/issue-42",
			Codex: &codexState{
				JSONLogPath:     jsonLog,
				LastMessagePath: tempDir + "/last-message.txt",
			},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   newFakeGitHubClient(githubIssue{Number: 42}),
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("rate limit resume should handle this daemon cycle")
	}
	var foundResumeLast bool
	for _, call := range commands.calls {
		if len(call.args) >= 5 && strings.Contains(call.args[len(call.args)-1], " --last - ") {
			foundResumeLast = true
		}
	}
	if !foundResumeLast {
		t.Fatalf("commands = %#v, want resume --last", commands.calls)
	}
}
