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

	if spec.CloneDir != filepath.Join(tempDir, "run-weaver", "src") {
		t.Fatalf("clone dir = %q", spec.CloneDir)
	}
	if spec.Path != filepath.Join(tempDir, "run-weaver", "worktrees", "issue-42") {
		t.Fatalf("path = %q", spec.Path)
	}
	if spec.Branch != "codex/issue-42-add-account-export" {
		t.Fatalf("branch = %q", spec.Branch)
	}
}

func TestWritePromptFile(t *testing.T) {
	path := t.TempDir() + "/issues/42/prompt.md"

	if err := writePromptFile(path, githubIssue{Number: 42, Title: "Test", Body: "Add a README.", URL: "https://example.test/42"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "GitHub Issue #42") || !strings.Contains(got, "https://example.test/42") || !strings.Contains(got, "Add a README.") {
		t.Fatalf("prompt = %q", got)
	}
}

func TestWritePromptFileAllowsTitleOnly(t *testing.T) {
	path := t.TempDir() + "/issues/42/prompt.md"

	if err := writePromptFile(path, githubIssue{Number: 42, Title: "Add README", URL: "https://example.test/42"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "If the body is empty but the title clearly describes a concrete change") {
		t.Fatalf("prompt = %q, want title-only guidance", got)
	}
}

func TestWritePromptFileIgnoresRunWeaverRateLimitComments(t *testing.T) {
	path := t.TempDir() + "/issues/42/prompt.md"

	if err := writePromptFile(path, githubIssue{
		Number: 42,
		Title:  "Add README",
		URL:    "https://example.test/42",
		Comments: []githubComment{{
			Body: "<!-- run-weaver-rate-limit:claim:1 -->\nrun-weaver detected a Codex rate limit interruption and started an automatic resume attempt.",
		}, {
			Body: "Please include setup notes.",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "rate limit interruption") {
		t.Fatalf("prompt = %q, should ignore run-weaver rate limit comments", got)
	}
	if !strings.Contains(got, "Please include setup notes.") {
		t.Fatalf("prompt = %q, want human comment", got)
	}
}

func TestPromptFilesIncludeSubagentGuidance(t *testing.T) {
	tempDir := t.TempDir()
	issue := githubIssue{Number: 42, Title: "Add README", Body: "Add a README.", URL: "https://example.test/42"}
	for name, write := range map[string]func(string) error{
		"issue": func(path string) error {
			return writePromptFile(path, issue)
		},
		"campaign-task": func(path string) error {
			return writeCampaignPromptFile(path, issueRef{Number: 7, Title: "Roadmap"}, nil, campaignTask{ID: "task-readme"}, issue, pipelinePhaseReview)
		},
		"campaign-planner": func(path string) error {
			return writeCampaignPlannerPromptFile(path, issue)
		},
	} {
		path := filepath.Join(tempDir, name+".md")
		if err := write(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(data); !strings.Contains(got, "Use Codex built-in subagents") || !strings.Contains(got, "repository AGENTS.md does not prohibit") || !strings.Contains(got, "higher-priority runtime instructions allow") || !strings.Contains(got, "continue with a self-review") {
			t.Fatalf("%s prompt = %q, want subagent guidance", name, got)
		}
	}
}

func TestPromptFilesIncludeDocumentationConflictPolicy(t *testing.T) {
	tempDir := t.TempDir()
	issue := githubIssue{Number: 42, Title: "Add README", Body: "Add a README.", URL: "https://example.test/42"}
	for name, write := range map[string]func(string) error{
		"issue": func(path string) error {
			return writePromptFile(path, issue)
		},
		"campaign-task": func(path string) error {
			return writeCampaignPromptFile(path, issueRef{Number: 7, Title: "Roadmap"}, nil, campaignTask{ID: "task-readme"}, issue, pipelinePhaseReview)
		},
		"campaign-planner": func(path string) error {
			return writeCampaignPlannerPromptFile(path, issue)
		},
	} {
		path := filepath.Join(tempDir, name+".md")
		if err := write(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		for _, want := range []string{
			"Documentation conflict policy:",
			"docs/progress.md",
			"docs/handoff.md",
			"docs/process-log.md",
			"Do not create task dependencies or stacked PRs solely because multiple tasks may update those files.",
			"semantic documentation",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s prompt = %q, want %q", name, got, want)
			}
		}
	}
}

func TestCampaignPlannerPromptAsksForOperationalDocsConsolidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campaign-planner.md")

	err := writeCampaignPlannerPromptFile(path, githubIssue{Number: 7, Title: "Roadmap", Body: "Plan the work."})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "Operational current-state docs alone should not force stacked PRs") {
		t.Fatalf("planner prompt = %q, want non-stacking docs guidance", got)
	}
	if !strings.Contains(got, "add a final documentation consolidation task") {
		t.Fatalf("planner prompt = %q, want docs consolidation guidance", got)
	}
}

func TestCampaignPromptIncludesDecisionAnswers(t *testing.T) {
	path := t.TempDir() + "/campaign-task.md"

	err := writeCampaignPromptFile(path, issueRef{Number: 7, Title: "Roadmap"}, []campaignDecision{{
		ID:             "decision-scope",
		Title:          "Choose scope",
		Status:         "answered",
		Answer:         "approve",
		Recommendation: "approve",
		OptionDetails:  []string{"approve: implement export", "stop: leave export out"},
		BlockedTasks:   []string{"task-export"},
	}}, campaignTask{ID: "task-export"}, githubIssue{Number: 101, Title: "Export"}, pipelinePhaseImplement)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "Campaign decisions:") || !strings.Contains(got, "answer: approve") || !strings.Contains(got, "approve: implement export") {
		t.Fatalf("prompt = %q, want decision answer context", got)
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
	clonePath := filepath.Join(tempDir, "run-weaver", "src")
	if !commands.ran("git", "clone", "https://example.test/repo.git", clonePath) {
		t.Fatalf("commands = %#v, want git clone", commands.calls)
	}
	if !commands.ran("git", "-C", clonePath, "worktree", "add", "-B", spec.Branch, spec.Path, "origin/HEAD") {
		t.Fatalf("commands = %#v, want git worktree add", commands.calls)
	}
}

func TestWorktreePrepareWithBaseUsesDependencyBranch(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir)
	commands := &fakeCommandRunner{}
	manager := newWorktreeManager(commands)

	spec, err := manager.PrepareForRepoWithBase(context.Background(), "wsl", "", githubIssue{Number: 43, Title: "Stacked issue"}, "https://example.test/repo.git", "codex/issue-42-base")

	if err != nil {
		t.Fatal(err)
	}
	clonePath := filepath.Join(tempDir, "run-weaver", "src")
	if !commands.ran("git", "-C", clonePath, "worktree", "add", "-B", spec.Branch, spec.Path, "origin/codex/issue-42-base") {
		t.Fatalf("commands = %#v, want dependency base ref", commands.calls)
	}
}

func TestWorktreePrepareReusesExistingWorktree(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir)
	worktreePath := filepath.Join(tempDir, "run-weaver", "worktrees", "issue-42")
	if err := os.MkdirAll(filepath.Join(worktreePath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	clonePath := filepath.Join(tempDir, "run-weaver", "src")
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
