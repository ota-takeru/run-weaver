package cli

import (
	"context"
	"os"
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
	commands := &fakeCommandRunner{}

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
