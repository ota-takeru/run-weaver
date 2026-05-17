package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCodexRunSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir)

	spec := buildCodexRunSpec("wsl", 42, "/tmp/worktree")

	if spec.JSONLogPath != filepath.Join(tempDir, "run-weaver", "issues", "42", "codex.jsonl") {
		t.Fatalf("json log path = %q", spec.JSONLogPath)
	}
	if spec.LastMessagePath != filepath.Join(tempDir, "run-weaver", "issues", "42", "last-message.txt") {
		t.Fatalf("last message path = %q", spec.LastMessagePath)
	}
	if spec.PromptPath != filepath.Join(tempDir, "run-weaver", "issues", "42", "prompt.md") {
		t.Fatalf("prompt path = %q", spec.PromptPath)
	}
}

func TestBuildCodexTmuxCommand(t *testing.T) {
	spec := codexRunSpec{
		IssueNumber:     42,
		Worktree:        "/tmp/work tree",
		PromptPath:      "/tmp/state/prompt.md",
		JSONLogPath:     "/tmp/state/codex.jsonl",
		LastMessagePath: "/tmp/state/last message.txt",
	}

	command := buildCodexTmuxCommand(spec)

	for _, want := range []string{
		"mkdir -p '/tmp/state' && codex --ask-for-approval never exec --json --sandbox workspace-write",
		"codex --ask-for-approval never exec --json",
		"--cd '/tmp/work tree'",
		"--output-last-message '/tmp/state/last message.txt'",
		"- < '/tmp/state/prompt.md'",
		"> '/tmp/state/codex.jsonl'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command = %q, missing %q", command, want)
		}
	}
}

func TestBuildCodexTmuxCommandResume(t *testing.T) {
	spec := codexRunSpec{
		IssueNumber:     42,
		Worktree:        "/tmp/worktree",
		PromptPath:      "/tmp/state/resume.md",
		JSONLogPath:     "/tmp/state/codex.jsonl",
		LastMessagePath: "/tmp/state/last-message.txt",
		ResumeSessionID: "session-123",
	}

	command := buildCodexTmuxCommand(spec)

	for _, want := range []string{
		"cd '/tmp/worktree' && codex --ask-for-approval never exec resume --json",
		"--output-last-message '/tmp/state/last-message.txt'",
		"'session-123' - < '/tmp/state/resume.md'",
		">> '/tmp/state/codex.jsonl'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command = %q, missing %q", command, want)
		}
	}
}

func TestTmuxRunnerStartsWindow(t *testing.T) {
	commands := &fakeCommandRunner{
		failFirstHasSession: true,
	}
	runner := newTmuxRunner(commands)

	ref, err := runner.StartCodex(context.Background(), codexRunSpec{
		IssueNumber:     42,
		Worktree:        "/tmp/worktree",
		PromptPath:      "/tmp/prompt.md",
		JSONLogPath:     "/tmp/codex.jsonl",
		LastMessagePath: "/tmp/last-message.txt",
	})

	if err != nil {
		t.Fatal(err)
	}
	if ref.Session != tmuxSessionName || ref.Window != "issue-42" {
		t.Fatalf("tmux ref = %#v", ref)
	}
	if !commands.ran("tmux", "new-session", "-d", "-s", tmuxSessionName, "-n", "control") {
		t.Fatalf("commands = %#v, want new-session", commands.calls)
	}
	if !commands.ranPrefix("tmux", "new-window", "-t", tmuxSessionName, "-n", "issue-42") {
		t.Fatalf("commands = %#v, want new-window", commands.calls)
	}
}

type fakeCommandRunner struct {
	calls               []commandCall
	outputs             map[string][]byte
	failFirstHasSession bool
}

type commandCall struct {
	name string
	args []string
}

func (r *fakeCommandRunner) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	if r.failFirstHasSession && name == "tmux" && len(args) >= 1 && args[0] == "has-session" {
		r.failFirstHasSession = false
		return errFakeCommand
	}
	return nil
}

func (r *fakeCommandRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	if r.outputs == nil {
		return nil, nil
	}
	return r.outputs[name+"\x00"+strings.Join(args, "\x00")], nil
}

func (r *fakeCommandRunner) ran(name string, args ...string) bool {
	for _, call := range r.calls {
		if call.name == name && strings.Join(call.args, "\x00") == strings.Join(args, "\x00") {
			return true
		}
	}
	return false
}

func (r *fakeCommandRunner) ranPrefix(name string, args ...string) bool {
	for _, call := range r.calls {
		if call.name != name || len(call.args) < len(args) {
			continue
		}
		if strings.Join(call.args[:len(args)], "\x00") == strings.Join(args, "\x00") {
			return true
		}
	}
	return false
}

var errFakeCommand = &fakeCommandError{}

type fakeCommandError struct{}

func (*fakeCommandError) Error() string {
	return "fake command error"
}
