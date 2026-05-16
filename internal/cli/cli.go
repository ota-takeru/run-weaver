package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	exitOK                 = 0
	exitUsage              = 64
	exitStateUnavailable   = 1
	exitStatusAuthRequired = 2
	exitStatusConflict     = 3
)

var validTargets = map[string]struct{}{
	"wsl":     {},
	"windows": {},
}

// Run executes the run-weaver CLI and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return exitUsage
	}

	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRootUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printRootUsage(stderr)
		return exitUsage
	}
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("doctor", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	result := collectDoctor(*target)
	if *jsonOutput {
		writeJSON(stdout, result.Output)
		return result.ExitCode
	}

	printDoctorHuman(stdout, result.Output)
	return result.ExitCode
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("status", stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}

	result := collectStatus()
	if *jsonOutput {
		writeJSON(stdout, result.Output)
		return result.ExitCode
	}

	printStatusHuman(stdout, result.Output)
	if result.Err != nil && !errors.Is(result.Err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "status error: %v\n", result.Err)
	}
	return result.ExitCode
}

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("install", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	fmt.Fprintf(stdout, "install skeleton for target %s\n", *target)
	return exitOK
}

func runDaemon(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("daemon", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	once := fs.Bool("once", false, "process at most one issue and exit")
	repo := fs.String("repo", "", "GitHub repository for gh, in owner/repo form")
	repoURL := fs.String("repo-url", "", "repository URL used to create the Codex clone")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	if !*once {
		fmt.Fprintln(stderr, "daemon loop is not implemented yet; use --once")
		return exitConfigMissing
	}
	if *repoURL == "" {
		fmt.Fprintln(stderr, "--repo-url is required for daemon --once")
		return exitConfigMissing
	}

	result, err := processOneIssue(daemonDeps{
		github:   ghClient{repo: *repo},
		worktree: newWorktreeManager(nil),
		runner:   newTmuxRunner(nil),
	}, daemonOptions{
		target:  *target,
		repoURL: *repoURL,
	})
	if err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintln(stdout, result)
	return exitOK
}

func newFlagSet(name string, output io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer) bool {
	if err := fs.Parse(args); err != nil {
		return false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument for %s: %s\n", fs.Name(), fs.Arg(0))
		return false
	}
	return true
}

func validateTarget(target string, required bool, stderr io.Writer) bool {
	if target == "" {
		if required {
			fmt.Fprintln(stderr, "--target is required")
			return false
		}
		return true
	}

	if _, ok := validTargets[target]; !ok {
		fmt.Fprintf(stderr, "invalid target %q: expected wsl or windows\n", target)
		return false
	}
	return true
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  doctor   Check dependencies and authentication")
	fmt.Fprintln(w, "  status   Show daemon and job status")
	fmt.Fprintln(w, "  install  Install target-specific daemon configuration")
	fmt.Fprintln(w, "  daemon   Run the issue processing daemon")
}

func writeJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

type doctorOutput struct {
	Target    string        `json:"target"`
	Overall   string        `json:"overall"`
	StateFile string        `json:"stateFile"`
	Checks    []doctorCheck `json:"checks"`
	Next      []string      `json:"next"`
}

type doctorCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type statusOutput struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Target         string               `json:"target"`
	StateFile      string               `json:"stateFile"`
	Daemon         daemonStatus         `json:"daemon"`
	Job            *statusJob           `json:"job"`
	Reconciliation reconciliationStatus `json:"reconciliation"`
}

type daemonStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Service string `json:"service,omitempty"`
}

type statusJob struct {
	Issue               issueRef    `json:"issue"`
	LabelState          string      `json:"labelState"`
	Branch              string      `json:"branch"`
	Worktree            string      `json:"worktree"`
	ClaimID             string      `json:"claimId"`
	ClaimedAt           string      `json:"claimedAt"`
	Tmux                *tmuxRef    `json:"tmux,omitempty"`
	LastGitHubCommentAt string      `json:"lastGitHubCommentAt,omitempty"`
	LastError           *string     `json:"lastError"`
	Codex               *codexState `json:"codex,omitempty"`
}

type issueRef struct {
	Number     int    `json:"number"`
	Title      string `json:"title,omitempty"`
	URL        string `json:"url,omitempty"`
	Repository string `json:"repository,omitempty"`
}

type tmuxRef struct {
	Session string `json:"session"`
	Window  string `json:"window"`
}

type codexState struct {
	SessionID       *string `json:"sessionId"`
	LastMessagePath string  `json:"lastMessagePath,omitempty"`
	JSONLogPath     string  `json:"jsonLogPath,omitempty"`
}

type reconciliationStatus struct {
	GitHubLabelState string   `json:"githubLabelState"`
	ProcessState     string   `json:"processState"`
	TmuxState        string   `json:"tmuxState,omitempty"`
	Conflicts        []string `json:"conflicts"`
}
