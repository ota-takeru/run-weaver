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
	case "repo":
		return runRepo(args[1:], stdout, stderr)
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

	result, err := checkReleaseUpdateFunc(context.Background())
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
	requestPaths, requestErr := writeDaemonUpdateRequests(result.Version)
	if requestErr != nil {
		fmt.Fprintf(stderr, "update error: %v\n", requestErr)
		return exitConfigMissing
	}
	if err := installReleaseUpdateFunc(context.Background(), result, argsForRestart(nil)); err != nil {
		if errors.Is(err, errUpdateRestarting) {
			fmt.Fprintf(stdout, "installed run-weaver %s; restarting; daemon update requested at %s\n", result.Version, strings.Join(requestPaths, ", "))
			return exitOK
		}
		fmt.Fprintf(stderr, "update error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "installed run-weaver %s; daemon update requested at %s\n", result.Version, strings.Join(requestPaths, ", "))
	return exitOK
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("doctor", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	repo := fs.String("repo", "", "GitHub repository, in owner/repo form")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	result := collectDoctorForRepo(*target, *repo)
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

	if *repo == "" {
		if config, err := readRepoConfig(defaultRepoConfigFile(defaultStatusTarget())); err == nil && len(enabledRepoEntries(config)) > 0 {
			result := collectMultiStatus(enabledRepoEntries(config), defaultStatusTarget())
			if *jsonOutput {
				writeJSON(stdout, result.Output)
				return result.ExitCode
			}
			printStatusHuman(stdout, result.Output)
			return result.ExitCode
		}
	}

	result := collectStatusForRepo(ghClient{repo: *repo}, *repo)
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
	resolvedRepoURL := strings.TrimSpace(*repoURL)
	repoName := strings.TrimSpace(*repo)
	if resolvedRepoURL != "" || repoName != "" {
		var err error
		resolvedRepoURL, err = resolveRepoURL(resolvedRepoURL)
		if err != nil {
			fmt.Fprintf(stderr, "install requires --repo-url or a git origin remote: %v\n", err)
			return exitConfigMissing
		}
		repoName = resolveRepoName(repoName, resolvedRepoURL)
		if repoName == "" {
			fmt.Fprintln(stderr, "install could not infer --repo from --repo-url")
			return exitConfigMissing
		}
		if err := addRepoConfigEntry(defaultRepoConfigFile(*target), repoEntry{Repository: repoName, RepoURL: resolvedRepoURL, AddedFrom: "install", DopplerMode: dopplerModeAuto}); err != nil {
			fmt.Fprintf(stderr, "install repo config error: %v\n", err)
			return exitConfigMissing
		}
	}
	if *target == "windows" {
		if err := installWindows(windowsInstallOptions{
			Repo:         repoName,
			RepoURL:      resolvedRepoURL,
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
		RepoURL:      resolvedRepoURL,
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

	if *pollInterval <= 0 {
		fmt.Fprintln(stderr, "--poll-interval must be greater than zero")
		return exitUsage
	}

	if strings.TrimSpace(*repoURL) == "" && strings.TrimSpace(*repo) == "" {
		if config, err := readRepoConfig(defaultRepoConfigFile(*target)); err == nil && len(enabledRepoEntries(config)) > 0 {
			repos := enabledRepoEntries(config)
			if *once {
				result, err := processRegisteredReposOnce(repos, *target)
				if err != nil {
					fmt.Fprintf(stderr, "daemon error: %v\n", err)
					return exitConfigMissing
				}
				fmt.Fprintln(stdout, result)
				return exitOK
			}
			return runMultiRepoDaemonLoop(stdout, stderr, repos, *target, *pollInterval)
		}
	}

	resolvedRepoURL, err := resolveRepoURL(*repoURL)
	if err != nil {
		fmt.Fprintf(stderr, "daemon requires --repo-url or a git origin remote: %v\n", err)
		return exitConfigMissing
	}
	repoName := resolveRepoName(*repo, resolvedRepoURL)

	deps := daemonDeps{
		github:   ghClient{repo: repoName},
		worktree: newWorktreeManager(nil),
		runner:   newCodexRunner(*target, nil),
	}
	opts := daemonOptions{
		target:      *target,
		repo:        repoName,
		repoURL:     resolvedRepoURL,
		dopplerMode: dopplerModeAuto,
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

func resolveRepoURL(repoURL string) (string, error) {
	if strings.TrimSpace(repoURL) != "" {
		return strings.TrimSpace(repoURL), nil
	}
	out, err := runShortCommandOutput("git", "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git origin remote is empty")
	}
	return value, nil
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

func inferGitHubRepoFromIssueURL(issueURL string) string {
	value := strings.TrimSpace(issueURL)
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimPrefix(value, prefix)
			parts := strings.Split(value, "/")
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				return parts[0] + "/" + parts[1]
			}
		}
	}
	return ""
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  repo     Manage registered repositories")
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
	Campaign       *statusCampaign      `json:"campaign,omitempty"`
	ReadyQueue     []readyQueueItem     `json:"readyQueue,omitempty"`
	Repositories   []statusRepository   `json:"repositories,omitempty"`
	Reconciliation reconciliationStatus `json:"reconciliation"`
}

type statusRepository struct {
	Repository string       `json:"repository"`
	RepoURL    string       `json:"repoUrl,omitempty"`
	Status     statusOutput `json:"status"`
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

type statusCampaign struct {
	Issue          issueRef           `json:"issue"`
	Status         string             `json:"status"`
	CurrentTaskID  string             `json:"currentTaskId,omitempty"`
	Planner        *campaignPlanner   `json:"planner,omitempty"`
	CompletedTasks []string           `json:"completedTasks,omitempty"`
	Tasks          []campaignTask     `json:"tasks"`
	Decisions      []campaignDecision `json:"decisions,omitempty"`
	PRs            []campaignPR       `json:"prs,omitempty"`
}

type reconciliationStatus struct {
	GitHubLabelState string   `json:"githubLabelState"`
	ProcessState     string   `json:"processState"`
	TmuxState        string   `json:"tmuxState,omitempty"`
	Conflicts        []string `json:"conflicts"`
}
