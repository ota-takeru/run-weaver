package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	if code, handled := maybeAutoUpdateOnStartup(args, stderr); handled {
		return code
	}

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
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRootUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printRootUsage(stderr)
		return exitUsage
	}
}

func runUpdate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("update", stderr)
	checkOnly := fs.Bool("check", false, "check for an update without installing it")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}

	result, err := checkReleaseUpdate(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "update error: %v\n", err)
		return exitConfigMissing
	}
	if !result.Available {
		fmt.Fprintf(stdout, "run-weaver is up to date (%s)\n", Version)
		return exitOK
	}
	if *checkOnly {
		fmt.Fprintf(stdout, "update available: %s -> %s\n", Version, result.Version)
		return exitOK
	}
	if err := installReleaseUpdate(context.Background(), result, argsForRestart(nil)); err != nil {
		if errors.Is(err, errUpdateRestarting) {
			fmt.Fprintf(stdout, "installed run-weaver %s; restarting\n", result.Version)
			return exitOK
		}
		fmt.Fprintf(stderr, "update error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "installed run-weaver %s\n", result.Version)
	return exitOK
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
	repo := fs.String("repo", "", "GitHub repository for gh, in owner/repo form")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}

	result := collectStatus(ghClient{repo: *repo})
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
	repo := fs.String("repo", "", "GitHub repository for gh, in owner/repo form")
	repoURL := fs.String("repo-url", "", "repository URL used to create the Codex clone")
	binary := fs.String("binary", "", "path to the run-weaver binary")
	pollInterval := fs.Duration("poll-interval", time.Minute, "daemon poll interval")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}
	if *pollInterval <= 0 {
		fmt.Fprintln(stderr, "--poll-interval must be greater than zero")
		return exitUsage
	}
	if *repoURL == "" {
		fmt.Fprintf(stderr, "--repo-url is required for install --target %s\n", *target)
		return exitConfigMissing
	}
	repoName := resolveRepoName(*repo, *repoURL)
	if *target == "windows" {
		if err := installWindows(windowsInstallOptions{
			Repo:         repoName,
			RepoURL:      *repoURL,
			Binary:       *binary,
			PollInterval: *pollInterval,
		}); err != nil {
			fmt.Fprintf(stderr, "install error: %v\n", err)
			return exitConfigMissing
		}
		fmt.Fprintf(stdout, "installed Task Scheduler task run-weaver for target windows\n")
		return exitOK
	}

	if err := installWSL(installOptions{
		Repo:         repoName,
		RepoURL:      *repoURL,
		Binary:       *binary,
		PollInterval: *pollInterval,
	}); err != nil {
		fmt.Fprintf(stderr, "install error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "installed systemd user service run-weaver for target wsl\n")
	return exitOK
}

func runDaemon(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("daemon", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	once := fs.Bool("once", false, "process at most one issue and exit")
	repo := fs.String("repo", "", "GitHub repository for gh, in owner/repo form")
	repoURL := fs.String("repo-url", "", "repository URL used to create the Codex clone")
	pollInterval := fs.Duration("poll-interval", time.Minute, "daemon poll interval")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	if *repoURL == "" {
		fmt.Fprintln(stderr, "--repo-url is required for daemon")
		return exitConfigMissing
	}
	if *pollInterval <= 0 {
		fmt.Fprintln(stderr, "--poll-interval must be greater than zero")
		return exitUsage
	}

	deps := daemonDeps{
		github:   ghClient{repo: resolveRepoName(*repo, *repoURL)},
		worktree: newWorktreeManager(nil),
		runner:   newTmuxRunner(nil),
	}
	opts := daemonOptions{
		target:  *target,
		repoURL: *repoURL,
	}
	if !*once {
		return runDaemonLoop(stdout, stderr, deps, opts, *pollInterval)
	}

	result, err := processOneIssue(deps, opts)
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

func resolveRepoName(repo, repoURL string) string {
	if repo != "" {
		return repo
	}
	return inferGitHubRepo(repoURL)
}

func inferGitHubRepo(repoURL string) string {
	value := strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
	switch {
	case strings.HasPrefix(value, "https://github.com/"):
		value = strings.TrimPrefix(value, "https://github.com/")
	case strings.HasPrefix(value, "http://github.com/"):
		value = strings.TrimPrefix(value, "http://github.com/")
	case strings.HasPrefix(value, "git@github.com:"):
		value = strings.TrimPrefix(value, "git@github.com:")
	default:
		return ""
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  doctor   Check dependencies and authentication")
	fmt.Fprintln(w, "  status   Show daemon and job status")
	fmt.Fprintln(w, "  install  Install target-specific daemon configuration")
	fmt.Fprintln(w, "  daemon   Run the issue processing daemon")
	fmt.Fprintln(w, "  update   Update run-weaver from GitHub Releases")
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
	RuntimeState        string      `json:"runtimeState"`
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
