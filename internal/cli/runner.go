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
const campaignPlannerPhase = "campaign-planner"
const conflictResolvePhase = "conflict-resolve"

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

type codexRunner interface {
	StartCodex(context.Context, codexRunSpec) (*tmuxRef, error)
}

type directRunner struct {
	target   string
	commands commandRunner
}

type codexRunSpec struct {
	IssueNumber     int
	Phase           string
	Worktree        string
	WindowName      string
	PromptPath      string
	JSONLogPath     string
	LastMessagePath string
	ResumeSessionID string
	UseDoppler      bool
}

func newCodexRunner(target string, commands commandRunner) codexRunner {
	if target == "windows" {
		return newDirectRunner(target, commands)
	}
	return newTmuxRunner(commands)
}

func newTmuxRunner(commands commandRunner) tmuxRunner {
	if commands == nil {
		commands = execCommandRunner{}
	}
	return tmuxRunner{commands: commands}
}

func newDirectRunner(target string, commands commandRunner) directRunner {
	if commands == nil {
		commands = execCommandRunner{}
	}
	return directRunner{target: target, commands: commands}
}

func (r tmuxRunner) StartCodex(ctx context.Context, spec codexRunSpec) (*tmuxRef, error) {
	ref := tmuxRef{
		Session: tmuxSessionName,
		Window:  "issue-" + strconv.Itoa(spec.IssueNumber),
	}
	if spec.WindowName != "" {
		ref.Window = spec.WindowName
	}
	if err := r.ensureSession(ctx); err != nil {
		return &ref, err
	}
	command := buildCodexTmuxCommand(spec)
	if err := r.commands.Run(ctx, "tmux", "new-window", "-t", ref.Session, "-n", ref.Window, command); err != nil {
		return &ref, err
	}
	return &ref, nil
}

func (r directRunner) StartCodex(ctx context.Context, spec codexRunSpec) (*tmuxRef, error) {
	command := buildCodexDirectCommand(r.target, spec)
	if r.target == "windows" {
		ps := "Start-Process -WindowStyle Hidden powershell -ArgumentList @('-NoProfile','-Command'," + powershellQuote(command) + ")"
		if err := r.commands.Run(ctx, "powershell", "-NoProfile", "-Command", ps); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := r.commands.Run(ctx, "sh", "-c", command+" &"); err != nil {
		return nil, err
	}
	return nil, nil
}

func (r tmuxRunner) ensureSession(ctx context.Context) error {
	if err := r.commands.Run(ctx, "tmux", "has-session", "-t", tmuxSessionName); err == nil {
		return nil
	}
	if err := r.commands.Run(ctx, "tmux", "new-session", "-d", "-s", tmuxSessionName, "-n", "control"); err != nil {
		if retryErr := r.commands.Run(ctx, "tmux", "has-session", "-t", tmuxSessionName); retryErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func buildCodexRunSpec(target string, issueNumber int, worktree string) codexRunSpec {
	return buildCodexRunSpecForPhase(target, issueNumber, worktree, "")
}

func buildCodexRunSpecForPhase(target string, issueNumber int, worktree string, phase string) codexRunSpec {
	return buildCodexRunSpecForStatePath(defaultStateFile(target), issueNumber, worktree, phase, "")
}

func buildCodexRunSpecForRepo(target, repository string, issueNumber int, worktree string, phase string) codexRunSpec {
	windowName := ""
	if repository != "" {
		windowName = repoSlug(repository) + "-issue-" + strconv.Itoa(issueNumber)
	}
	return buildCodexRunSpecForStatePath(stateFileForRepo(target, repository), issueNumber, worktree, phase, windowName)
}

func buildCampaignPlannerRunSpec(target, repository string, issueNumber int, worktree string) codexRunSpec {
	spec := buildCodexRunSpecForRepo(target, repository, issueNumber, worktree, campaignPlannerPhase)
	if repository != "" {
		spec.WindowName = repoSlug(repository) + "-campaign-" + strconv.Itoa(issueNumber) + "-planner"
	} else {
		spec.WindowName = "campaign-" + strconv.Itoa(issueNumber) + "-planner"
	}
	return spec
}

func buildCodexRunSpecForStatePath(statePath string, issueNumber int, worktree string, phase string, windowName string) codexRunSpec {
	issueStateDir := filepath.Join(filepath.Dir(statePath), "issues", strconv.Itoa(issueNumber))
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
		WindowName:      windowName,
		PromptPath:      filepath.Join(issueStateDir, promptName),
		JSONLogPath:     filepath.Join(issueStateDir, "codex.jsonl"),
		LastMessagePath: filepath.Join(issueStateDir, "last-message.txt"),
	}
}

func buildCodexTmuxCommand(spec codexRunSpec) string {
	dir := shellQuote(path.Dir(filepath.ToSlash(spec.JSONLogPath)))
	return "mkdir -p " + dir + " && " + buildCodexCommand(spec)
}

func buildCodexDirectCommand(target string, spec codexRunSpec) string {
	if target == "windows" {
		return buildCodexPowerShellCommand(spec)
	}
	return buildCodexTmuxCommand(spec)
}

func buildCodexCommand(spec codexRunSpec) string {
	codex := "codex"
	if spec.UseDoppler {
		codex = "doppler run -- codex"
	}
	if spec.ResumeSessionID != "" {
		sessionArg := shellQuote(spec.ResumeSessionID)
		if spec.ResumeSessionID == "--last" {
			sessionArg = "--last"
		}
		return strings.Join([]string{
			"cd " + shellQuote(filepath.ToSlash(spec.Worktree)) + " && " + codex + " --ask-for-approval never exec resume --json",
			"--output-last-message " + shellQuote(filepath.ToSlash(spec.LastMessagePath)),
			sessionArg,
			"-",
			"< " + shellQuote(filepath.ToSlash(spec.PromptPath)),
			">> " + shellQuote(filepath.ToSlash(spec.JSONLogPath)),
			"2>&1",
		}, " ")
	}
	args := []string{
		codex + " --ask-for-approval never exec --json",
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
	command := strings.Join(args, " ")
	if spec.UseDoppler {
		return "cd " + shellQuote(filepath.ToSlash(spec.Worktree)) + " && " + command
	}
	return command
}

func buildCodexPowerShellCommand(spec codexRunSpec) string {
	dir := powershellQuote(filepath.Dir(spec.JSONLogPath))
	prompt := powershellQuote(spec.PromptPath)
	jsonLog := powershellQuote(spec.JSONLogPath)
	lastMessage := powershellQuote(spec.LastMessagePath)
	worktree := powershellQuote(spec.Worktree)
	codex := "codex"
	if spec.UseDoppler {
		codex = "doppler run -- codex"
	}
	prefix := "$ErrorActionPreference = 'Stop'; New-Item -ItemType Directory -Force -Path " + dir + " | Out-Null; "
	if spec.ResumeSessionID != "" {
		sessionArg := powershellQuote(spec.ResumeSessionID)
		if spec.ResumeSessionID == "--last" {
			sessionArg = "--last"
		}
		return prefix + "Set-Location " + worktree + "; Get-Content -Raw " + prompt + " | " + codex + " --ask-for-approval never exec resume --json --output-last-message " + lastMessage + " " + sessionArg + " - >> " + jsonLog + " 2>&1"
	}
	args := []string{
		codex + " --ask-for-approval never exec --json",
		"--sandbox workspace-write",
	}
	if config := codexReasoningConfig(spec.Phase); config != "" {
		args = append(args, config)
	}
	args = append(args,
		"--cd "+worktree,
		"--output-last-message "+lastMessage,
		"-",
	)
	if spec.UseDoppler {
		prefix += "Set-Location " + worktree + "; "
	}
	return prefix + "Get-Content -Raw " + prompt + " | " + strings.Join(args, " ") + " > " + jsonLog + " 2>&1"
}

func codexReasoningConfig(phase string) string {
	switch phase {
	case pipelinePhasePlan, pipelinePhaseReview, campaignPlannerPhase, conflictResolvePhase:
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

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
