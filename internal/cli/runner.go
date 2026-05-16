package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const tmuxSessionName = "run-weaver"

type commandRunner interface {
	Run(context.Context, string, ...string) error
	Output(context.Context, string, ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	out, err := (execCommandRunner{}).Output(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (execCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type tmuxRunner struct {
	commands commandRunner
}

type codexRunSpec struct {
	IssueNumber     int
	Worktree        string
	PromptPath      string
	JSONLogPath     string
	LastMessagePath string
}

func newTmuxRunner(commands commandRunner) tmuxRunner {
	if commands == nil {
		commands = execCommandRunner{}
	}
	return tmuxRunner{commands: commands}
}

func (r tmuxRunner) StartCodex(ctx context.Context, spec codexRunSpec) (tmuxRef, error) {
	ref := tmuxRef{
		Session: tmuxSessionName,
		Window:  "issue-" + strconv.Itoa(spec.IssueNumber),
	}
	if err := r.ensureSession(ctx); err != nil {
		return ref, err
	}
	command := buildCodexTmuxCommand(spec)
	if err := r.commands.Run(ctx, "tmux", "new-window", "-t", ref.Session, "-n", ref.Window, command); err != nil {
		return ref, err
	}
	return ref, nil
}

func (r tmuxRunner) ensureSession(ctx context.Context) error {
	if err := r.commands.Run(ctx, "tmux", "has-session", "-t", tmuxSessionName); err == nil {
		return nil
	}
	return r.commands.Run(ctx, "tmux", "new-session", "-d", "-s", tmuxSessionName, "-n", "control")
}

func buildCodexRunSpec(target string, issueNumber int, worktree string) codexRunSpec {
	issueStateDir := filepath.Join(filepath.Dir(defaultStateFile(target)), "issues", strconv.Itoa(issueNumber))
	return codexRunSpec{
		IssueNumber:     issueNumber,
		Worktree:        worktree,
		PromptPath:      filepath.Join(issueStateDir, "prompt.md"),
		JSONLogPath:     filepath.Join(issueStateDir, "codex.jsonl"),
		LastMessagePath: filepath.Join(issueStateDir, "last-message.txt"),
	}
}

func buildCodexTmuxCommand(spec codexRunSpec) string {
	dir := shellQuote(filepath.Dir(spec.JSONLogPath))
	codexCommand := strings.Join([]string{
		"codex --ask-for-approval never exec --json",
		"--sandbox workspace-write",
		"--cd " + shellQuote(spec.Worktree),
		"--output-last-message " + shellQuote(spec.LastMessagePath),
		"-",
		"< " + shellQuote(spec.PromptPath),
		"> " + shellQuote(spec.JSONLogPath),
		"2>&1",
	}, " ")
	return "mkdir -p " + dir + " && " + codexCommand
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
