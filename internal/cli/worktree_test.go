package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWorktreeSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir)

	spec := buildWorktreeSpec("wsl", githubIssue{
		Number: 42,
		Title:  "Add Account Export!",
	})

	if spec.CloneDir != tempDir+"/run-weaver/src" {
		t.Fatalf("clone dir = %q", spec.CloneDir)
	}
	if spec.Path != tempDir+"/run-weaver/worktrees/issue-42" {
		t.Fatalf("path = %q", spec.Path)
	}
	if spec.Branch != "codex/issue-42-add-account-export" {
		t.Fatalf("branch = %q", spec.Branch)
	}
}

func TestWritePromptFile(t *testing.T) {
	path := t.TempDir() + "/issues/42/prompt.md"

	if err := writePromptFile(path, githubIssue{Number: 42, Title: "Test", URL: "https://example.test/42"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "GitHub Issue #42") || !strings.Contains(got, "https://example.test/42") {
		t.Fatalf("prompt = %q", got)
	}
}

func TestWorktreePrepareClonesAndAddsWorktree(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir)
	commands := &fakeCommandRunner{}
	manager := newWorktreeManager(commands)

	spec, err := manager.Prepare(context.Background(), "wsl", githubIssue{Number: 42, Title: "Test issue"}, "https://example.test/repo.git")

	if err != nil {
		t.Fatal(err)
	}
	if spec.Branch != "codex/issue-42-test-issue" {
		t.Fatalf("branch = %q", spec.Branch)
	}
	if !commands.ran("git", "clone", "https://example.test/repo.git", tempDir+"/run-weaver/src") {
		t.Fatalf("commands = %#v, want git clone", commands.calls)
	}
	if !commands.ran("git", "-C", tempDir+"/run-weaver/src", "worktree", "add", "-B", spec.Branch, spec.Path, "origin/HEAD") {
		t.Fatalf("commands = %#v, want git worktree add", commands.calls)
	}
}

func TestWorktreePrepareReusesExistingWorktree(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir)
	worktreePath := tempDir + "/run-weaver/worktrees/issue-42"
	if err := os.MkdirAll(filepath.Join(worktreePath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	clonePath := tempDir + "/run-weaver/src"
	if err := os.MkdirAll(filepath.Join(clonePath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{}
	manager := newWorktreeManager(commands)

	spec, err := manager.Prepare(context.Background(), "wsl", githubIssue{Number: 42, Title: "Test issue"}, "https://example.test/repo.git")

	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != worktreePath {
		t.Fatalf("path = %q", spec.Path)
	}
	if commands.ranPrefix("git", "-C", clonePath, "worktree", "add") {
		t.Fatalf("commands = %#v, should not add existing worktree", commands.calls)
	}
}

func TestBuildDraftPRSpec(t *testing.T) {
	spec := buildDraftPRSpec(githubIssue{Number: 42, Title: "Add export"}, "codex/issue-42-add-export")

	if spec.Title != "Draft: Add export" {
		t.Fatalf("title = %q", spec.Title)
	}
	if spec.Head != "codex/issue-42-add-export" {
		t.Fatalf("head = %q", spec.Head)
	}
	if !strings.Contains(spec.Body, "Closes #42") {
		t.Fatalf("body = %q", spec.Body)
	}
}

func TestWorktreeCommitAllSkipsCleanTree(t *testing.T) {
	commands := &fakeCommandRunner{}
	manager := newWorktreeManager(commands)

	committed, err := manager.CommitAll(context.Background(), "/tmp/worktree", "message")

	if err != nil {
		t.Fatal(err)
	}
	if committed {
		t.Fatal("expected clean tree to skip commit")
	}
	if commands.ranPrefix("git", "-C", "/tmp/worktree", "commit") {
		t.Fatalf("commands = %#v, should not commit", commands.calls)
	}
}

func TestWorktreeCommitAllCommitsDirtyTree(t *testing.T) {
	commands := &fakeCommandRunner{
		outputs: map[string][]byte{
			"git\x00-C\x00/tmp/worktree\x00status\x00--porcelain": []byte(" M index.html\n"),
		},
	}
	manager := newWorktreeManager(commands)

	committed, err := manager.CommitAll(context.Background(), "/tmp/worktree", "message")

	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("expected dirty tree to commit")
	}
	if !commands.ran("git", "-C", "/tmp/worktree", "add", "-A") {
		t.Fatalf("commands = %#v, want git add", commands.calls)
	}
	if !commands.ran("git", "-C", "/tmp/worktree", "commit", "-m", "message") {
		t.Fatalf("commands = %#v, want git commit", commands.calls)
	}
}
