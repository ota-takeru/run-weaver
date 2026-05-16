package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type worktreeManager struct {
	commands commandRunner
}

type worktreeSpec struct {
	CloneDir string
	Path     string
	Branch   string
}

func newWorktreeManager(commands commandRunner) worktreeManager {
	if commands == nil {
		commands = execCommandRunner{}
	}
	return worktreeManager{commands: commands}
}

func (m worktreeManager) Prepare(ctx context.Context, target string, issue githubIssue, repoURL string) (worktreeSpec, error) {
	spec := buildWorktreeSpec(target, issue)
	if err := m.ensureClone(ctx, spec.CloneDir, repoURL); err != nil {
		return spec, err
	}
	if err := os.MkdirAll(filepath.Dir(spec.Path), 0o755); err != nil {
		return spec, err
	}
	if err := m.commands.Run(ctx, "git", "-C", spec.CloneDir, "worktree", "add", "-B", spec.Branch, spec.Path, "origin/HEAD"); err != nil {
		return spec, err
	}
	return spec, nil
}

func (m worktreeManager) ensureClone(ctx context.Context, cloneDir, repoURL string) error {
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
		return m.commands.Run(ctx, "git", "-C", cloneDir, "fetch", "origin")
	}
	if repoURL == "" {
		return fmt.Errorf("repository URL is required to create Codex clone")
	}
	if err := os.MkdirAll(filepath.Dir(cloneDir), 0o755); err != nil {
		return err
	}
	return m.commands.Run(ctx, "git", "clone", repoURL, cloneDir)
}

func buildWorktreeSpec(target string, issue githubIssue) worktreeSpec {
	return worktreeSpec{
		CloneDir: defaultCodexClone(target),
		Path:     filepath.Join(defaultWorktreeRoot(target), "issue-"+strconv.Itoa(issue.Number)),
		Branch:   "codex/issue-" + strconv.Itoa(issue.Number) + "-" + slugify(issue.Title),
	}
}

func writePromptFile(path string, issue githubIssue) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# GitHub Issue #%d

Title: %s
URL: %s

Implement the requested change in this issue. Follow the repository AGENTS.md instructions, run focused verification, and leave a concise final summary.
`, issue.Number, issue.Title, issue.URL)
	return os.WriteFile(path, []byte(body), 0o600)
}

var nonSlugCharRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.ToLower(value)
	slug = nonSlugCharRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "task"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		return "task"
	}
	return slug
}
