package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path"
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
	Phase           string
	Worktree        string
	PromptPath      string
	JSONLogPath     string
	LastMessagePath string
	ResumeSessionID string
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
	return buildCodexRunSpecForPhase(target, issueNumber, worktree, "")
}

func buildCodexRunSpecForPhase(target string, issueNumber int, worktree string, phase string) codexRunSpec {
	issueStateDir := filepath.Join(filepath.Dir(defaultStateFile(target)), "issues", strconv.Itoa(issueNumber))
	if phase != "" {
		issueStateDir = filepath.Join(issueStateDir, phase)
	}
	promptName := "prompt.md"
	if phase != "" {
		promptName = phase + "-prompt.md"
	}
	return codexRunSpec{
		IssueNumber:     issueNumber,
		Phase:           phase,
		Worktree:        worktree,
		PromptPath:      filepath.Join(issueStateDir, promptName),
		JSONLogPath:     filepath.Join(issueStateDir, "codex.jsonl"),
		LastMessagePath: filepath.Join(issueStateDir, "last-message.txt"),
	}
}

func buildCodexTmuxCommand(spec codexRunSpec) string {
	dir := shellQuote(path.Dir(filepath.ToSlash(spec.JSONLogPath)))
	return "mkdir -p " + dir + " && " + buildCodexCommand(spec)
}

func buildCodexCommand(spec codexRunSpec) string {
	if spec.ResumeSessionID != "" {
		sessionArg := shellQuote(spec.ResumeSessionID)
		if spec.ResumeSessionID == "--last" {
			sessionArg = "--last"
		}
		return strings.Join([]string{
			"cd " + shellQuote(filepath.ToSlash(spec.Worktree)) + " && codex --ask-for-approval never exec resume --json",
			"--output-last-message " + shellQuote(filepath.ToSlash(spec.LastMessagePath)),
			sessionArg,
			"-",
			"< " + shellQuote(filepath.ToSlash(spec.PromptPath)),
			">> " + shellQuote(filepath.ToSlash(spec.JSONLogPath)),
			"2>&1",
		}, " ")
	}
	args := []string{
		"codex --ask-for-approval never exec --json",
		"--sandbox workspace-write",
	}
	if config := codexReasoningConfig(spec.Phase); config != "" {
		args = append(args, config)
	}
	args = append(args,
		"--cd "+shellQuote(filepath.ToSlash(spec.Worktree)),
		"--output-last-message "+shellQuote(filepath.ToSlash(spec.LastMessagePath)),
		"-",
		"< "+shellQuote(filepath.ToSlash(spec.PromptPath)),
		"> "+shellQuote(filepath.ToSlash(spec.JSONLogPath)),
		"2>&1",
	)
	return strings.Join(args, " ")
}

func codexReasoningConfig(phase string) string {
	switch phase {
	case pipelinePhasePlan, pipelinePhaseReview:
		return "-c model_reasoning_effort=\"medium\""
	case pipelinePhaseImplement:
		return "-c model_reasoning_effort=\"low\""
	default:
		return ""
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
