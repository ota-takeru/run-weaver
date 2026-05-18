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

func TestCompleteCurrentJobStartsConflictResolutionBeforePush(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := tempDir + "/worktrees/issue-42"
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
			Branch:     "codex/issue-42-add-export",
			Worktree:   worktree,
			Codex:      &codexState{LastMessagePath: lastMessage},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{
		failFirstHasSession: true,
		outputs: map[string][]byte{
			"git\x00-C\x00" + worktree + "\x00status\x00--porcelain":                  []byte(" M index.html\n"),
			"git\x00-C\x00" + worktree + "\x00diff\x00--name-only\x00--diff-filter=U": []byte("internal/app.go\n"),
		},
		runErrors: map[string]error{
			"git\x00-C\x00" + worktree + "\x00merge\x00--no-edit\x00origin/HEAD": errFakeCommand,
		},
	}

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"}),
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed || !strings.Contains(result, "started conflict resolution") {
		t.Fatalf("result = %q completed = %v", result, completed)
	}
	if commands.ranPrefix("git", "-C", worktree, "push") {
		t.Fatalf("commands = %#v, should not push before conflict resolution", commands.calls)
	}
	if !commands.ranPrefix("tmux", "new-window", "-t", tmuxSessionName, "-n", "issue-42") {
		t.Fatalf("commands = %#v, want conflict resolver Codex", commands.calls)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job.PipelinePhase != conflictResolvePhase || updated.Job.ConflictRetryCount != 1 {
		t.Fatalf("job = %#v, want conflict resolution phase", updated.Job)
	}
	if updated.Job.Codex == nil || !strings.Contains(updated.Job.Codex.LastMessagePath, conflictResolvePhase) {
		t.Fatalf("codex state = %#v, want conflict phase paths", updated.Job.Codex)
	}
}

func TestCompleteCurrentJobBlocksHighRiskConflict(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := tempDir + "/worktrees/issue-42"
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add deps"},
			LabelState: runningLabel,
			Branch:     "codex/issue-42-add-deps",
			Worktree:   worktree,
			Codex:      &codexState{LastMessagePath: lastMessage},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add deps"})
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + worktree + "\x00status\x00--porcelain":                  []byte(" M go.mod\n"),
			"git\x00-C\x00" + worktree + "\x00diff\x00--name-only\x00--diff-filter=U": []byte("go.sum\n"),
		},
		runErrors: map[string]error{
			"git\x00-C\x00" + worktree + "\x00merge\x00--no-edit\x00origin/HEAD": errFakeCommand,
		},
	}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err == nil || !strings.Contains(err.Error(), "high-risk conflict") {
		t.Fatalf("err = %v, want high-risk conflict", err)
	}
	if !completed {
		t.Fatal("high-risk conflict should complete this daemon cycle")
	}
	if !github.added[blockedLabel] {
		t.Fatal("blocked label was not added")
	}
	if commands.ranPrefix("tmux", "new-window") {
		t.Fatalf("commands = %#v, should not start Codex for high-risk conflict", commands.calls)
	}
}

func TestCompleteCampaignTaskBlocksHighRiskConflict(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := tempDir + "/worktrees/issue-101"
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:          issueRef{Number: 101, Title: "Add deps"},
			LabelState:     runningLabel,
			Branch:         "codex/issue-101-add-deps",
			Worktree:       worktree,
			CampaignTaskID: "task-add-deps",
			PipelinePhase:  pipelinePhaseVerify,
			Codex:          &codexState{LastMessagePath: lastMessage},
		},
		Campaign: &stateCampaign{
			Issue:         issueRef{Number: 7, Title: "Campaign"},
			Status:        campaignStatusRunning,
			CurrentTaskID: "task-add-deps",
			Tasks: []campaignTask{{
				ID:          "task-add-deps",
				Title:       "Add deps",
				IssueNumber: 101,
				Status:      campaignTaskRunning,
				Phase:       pipelinePhaseVerify,
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 101, Title: "Add deps"})
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + worktree + "\x00status\x00--porcelain":                  []byte(" M go.mod\n"),
			"git\x00-C\x00" + worktree + "\x00diff\x00--name-only\x00--diff-filter=U": []byte("go.sum\n"),
		},
		runErrors: map[string]error{
			"git\x00-C\x00" + worktree + "\x00merge\x00--no-edit\x00origin/HEAD": errFakeCommand,
		},
	}

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err == nil || !strings.Contains(err.Error(), "high-risk conflict") {
		t.Fatalf("err = %v, want high-risk conflict", err)
	}
	if !completed {
		t.Fatal("high-risk conflict should complete this daemon cycle")
	}
	if !github.added[blockedLabel] {
		t.Fatal("blocked label was not added")
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Campaign == nil || updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.CurrentTaskID != "" {
		t.Fatalf("campaign = %#v, want blocked with no current task", updated.Campaign)
	}
	if updated.Job == nil || updated.Job.LabelState != blockedLabel {
		t.Fatalf("job = %#v, want blocked", updated.Job)
	}
}

func TestCompleteCurrentJobFinishesAfterConflictResolution(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	worktree := tempDir + "/worktrees/issue-42"
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worktree+"/index.html", []byte("<h1>resolved</h1>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lastMessage := tempDir + "/conflict-last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("resolved"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:              issueRef{Number: 42, Title: "Add export"},
			LabelState:         runningLabel,
			Branch:             "codex/issue-42-add-export",
			Worktree:           worktree,
			PipelinePhase:      conflictResolvePhase,
			ConflictRetryCount: 1,
			Codex:              &codexState{LastMessagePath: lastMessage},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"})
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + worktree + "\x00diff\x00--name-only\x00--diff-filter=U": []byte(""),
			"git\x00-C\x00" + worktree + "\x00status\x00--porcelain":                  []byte(" M index.html\n"),
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
	if !completed || !strings.Contains(result, "completed issue #42") {
		t.Fatalf("result = %q completed = %v", result, completed)
	}
	if !github.added[doneLabel] {
		t.Fatal("done label was not added")
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

func TestCompleteCurrentJobDoesNotBlockCompletedLogThatMentionsCommandNotFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonLog := tempDir + "/codex.jsonl"
	logLine := `{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"docs mention codex: command not found handling"}}` + "\n"
	if err := os.WriteFile(jsonLog, []byte(logLine), 0o600); err != nil {
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
				LastMessagePath: lastMessage,
			},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00" + tempDir + "/worktrees/issue-42\x00status\x00--porcelain": []byte(" M docs/progress.md\n"),
		},
	}
	github := newFakeGitHubClient(githubIssue{Number: 42, Title: "Add export"})

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed || !strings.Contains(result, "completed issue #42") {
		t.Fatalf("result = %q completed = %v, want completed issue", result, completed)
	}
	if github.added[blockedLabel] {
		t.Fatalf("labels = %#v, should not block completed command output", github.added)
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
	github := newFakeGitHubClient(githubIssue{Number: 42})
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
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
	if len(github.comments) != 1 || !strings.Contains(github.comments[0].Body, "run-weaver detected a Codex rate limit interruption") {
		t.Fatalf("comments = %#v, want rate limit resume comment", github.comments)
	}
	if !strings.Contains(github.comments[0].Body, "json log: "+jsonLog) {
		t.Fatalf("comment = %q, want json log path", github.comments[0].Body)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job.Codex.SessionID == nil || *updated.Job.Codex.SessionID != "session-123" {
		t.Fatalf("session id = %#v", updated.Job.Codex.SessionID)
	}
	if updated.Job.RetryCount != 1 {
		t.Fatalf("retry count = %d, want 1", updated.Job.RetryCount)
	}
	if updated.Job.LastGitHubCommentAt == "" {
		t.Fatal("last GitHub comment timestamp should be recorded")
	}
	if updated.Job.LastError == nil || !strings.Contains(*updated.Job.LastError, "resume attempt 1 started") {
		t.Fatalf("last error = %#v, want rate limit resume message", updated.Job.LastError)
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

func TestCompleteCurrentJobDoesNotResumeCompletedLogThatMentionsRateLimit(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := tempDir + "/last-message.txt"
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonLog := tempDir + "/codex.jsonl"
	logLine := `{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"docs mention rate limit resume behavior"}}` + "\n"
	if err := os.WriteFile(jsonLog, []byte(logLine), 0o600); err != nil {
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
			Codex: &codexState{
				JSONLogPath:     jsonLog,
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
			"git\x00-C\x00" + tempDir + "/worktrees/issue-42\x00status\x00--porcelain": []byte(" M docs/progress.md\n"),
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
	if !completed || !strings.Contains(result, "completed issue #42") {
		t.Fatalf("result = %q completed = %v, want completed issue", result, completed)
	}
	if commands.ranPrefix("tmux", "new-window") {
		t.Fatalf("commands = %#v, should not resume completed log", commands.calls)
	}
	if len(github.comments) != 1 || !strings.Contains(github.comments[0].Body, "run-weaver completed this issue") {
		t.Fatalf("comments = %#v, want completion comment only", github.comments)
	}
}

func TestCodexLogLooksRateLimitedIgnoresCommandOutputMentions(t *testing.T) {
	path := t.TempDir() + "/codex.jsonl"
	log := `{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"docs mention rate limit handling"}}` + "\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	if codexLogLooksRateLimited(path) {
		t.Fatal("command output mentioning rate limit should not be treated as a Codex rate limit")
	}
}

func TestCodexLogLooksRateLimitedDetectsErrorEvents(t *testing.T) {
	path := t.TempDir() + "/codex.jsonl"
	log := `{"type":"turn.failed","error":{"message":"rate limit reached"}}` + "\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	if !codexLogLooksRateLimited(path) {
		t.Fatal("rate limit error event should be detected")
	}
}

func TestCodexLogLooksCommandNotFoundIgnoresCommandOutputMentions(t *testing.T) {
	path := t.TempDir() + "/codex.jsonl"
	log := `{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"docs mention codex: command not found handling"}}` + "\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	if codexLogLooksCommandNotFound(path) {
		t.Fatal("command output mentioning command-not-found should not be treated as Codex startup failure")
	}
}

func TestCodexLogLooksCommandNotFoundDetectsShellStartupFailure(t *testing.T) {
	path := t.TempDir() + "/codex.jsonl"
	if err := os.WriteFile(path, []byte("bash: line 1: codex: command not found\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !codexLogLooksCommandNotFound(path) {
		t.Fatal("shell startup failure should be detected")
	}
}
