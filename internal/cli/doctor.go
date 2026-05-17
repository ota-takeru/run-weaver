package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	exitMissing        = 1
	exitAuthRequired   = 2
	exitConfigMissing  = 3
	exitTargetMismatch = 4
)

var runShortCommandOutput = defaultRunShortCommandOutput
var lookPath = exec.LookPath

type doctorResult struct {
	Output   doctorOutput
	ExitCode int
}

func collectDoctor(target string) doctorResult {
	stateFile := defaultStateFile(target)
	osCheck := checkOSTarget(target)
	if osCheck.Status == "blocked" {
		return doctorResult{
			Output: doctorOutput{
				Target:    target,
				Overall:   "blocked",
				StateFile: stateFile,
				Checks:    []doctorCheck{osCheck},
				Next:      []string{},
			},
			ExitCode: exitTargetMismatch,
		}
	}
	checks := []doctorCheck{
		osCheck,
		checkCommand("git", "git"),
		checkGHAuth(),
		checkCodex(),
		checkDoppler(),
		checkCodexClone(target),
		checkWritableDir("worktree_root", "Worktree root", defaultWorktreeRoot(target)),
		checkWritableFile("state_file", "State file", stateFile),
		checkService(target),
	}

	if target == "wsl" {
		checks = insertAfter(checks, "doppler", checkCommand("tmux", "tmux"))
		checks = insertAfter(checks, "tmux", checkSystemdUser())
	}
	if target == "windows" {
		checks = insertAfter(checks, "state_file", checkWritableDir("log_dir", "Log dir", defaultLogDir(target)))
	}

	overall, exitCode := summarizeDoctor(checks)
	return doctorResult{
		Output: doctorOutput{
			Target:    target,
			Overall:   overall,
			StateFile: stateFile,
			Checks:    checks,
			Next:      doctorNextSteps(target, checks),
		},
		ExitCode: exitCode,
	}
}

func printDoctorHuman(w io.Writer, output doctorOutput) {
	fmt.Fprintf(w, "Target: %s\n", output.Target)
	fmt.Fprintf(w, "Overall: %s\n\n", output.Overall)
	fmt.Fprintln(w, "Checks:")
	for _, check := range output.Checks {
		fmt.Fprintf(w, "  [%s] %-18s %s\n", check.Status, check.Label, check.Message)
	}
	if len(output.Next) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next:")
	for _, step := range output.Next {
		fmt.Fprintf(w, "  %s\n", step)
	}
}

func checkOSTarget(target string) doctorCheck {
	switch target {
	case "wsl":
		if currentGOOS != "linux" {
			return newDoctorCheck("os_target", "OS target", "blocked", "WSL target must run on Linux under WSL")
		}
		if isWSL() {
			return newDoctorCheck("os_target", "OS target", "ok", "WSL detected")
		}
		return newDoctorCheck("os_target", "OS target", "blocked", "Linux detected, but WSL markers were not found")
	case "windows":
		if currentGOOS == "windows" {
			return newDoctorCheck("os_target", "OS target", "ok", "Windows detected")
		}
		return newDoctorCheck("os_target", "OS target", "blocked", "Windows target must run on Windows")
	default:
		return newDoctorCheck("os_target", "OS target", "blocked", "unknown target")
	}
}

func checkCommand(id, name string) doctorCheck {
	path, err := lookPath(name)
	if err != nil {
		return newDoctorCheck(id, name, "missing", name+" not found in PATH")
	}
	return newDoctorCheck(id, name, "ok", path)
}

func checkGHAuth() doctorCheck {
	if _, err := lookPath("gh"); err != nil {
		return newDoctorCheck("gh_auth", "gh auth", "missing", "gh not found in PATH")
	}
	if err := runShortCommand("gh", "auth", "status"); err != nil {
		return newDoctorCheck("gh_auth", "gh auth", "auth_required", "gh auth status failed")
	}
	return newDoctorCheck("gh_auth", "gh auth", "ok", "authenticated")
}

func checkCodex() doctorCheck {
	if _, err := lookPath("codex"); err != nil {
		return newDoctorCheck("codex", "codex", "missing", "codex not found in PATH")
	}
	if err := runShortCommand("codex", "exec", "--help"); err != nil {
		return newDoctorCheck("codex", "codex", "blocked", "codex exec is not available")
	}
	if err := runShortCommand("codex", "login", "status"); err != nil {
		return newDoctorCheck("codex", "codex", "auth_required", "codex login status failed")
	}
	return newDoctorCheck("codex", "codex", "ok", "codex exec available and logged in")
}

func checkDoppler() doctorCheck {
	if _, err := lookPath("doppler"); err != nil {
		return newDoctorCheck("doppler", "doppler", "missing", "doppler not found in PATH")
	}
	if os.Getenv("DOPPLER_TOKEN") != "" {
		return newDoctorCheck("doppler", "doppler", "ok", "DOPPLER_TOKEN is set")
	}
	if err := runShortCommand("doppler", "configure", "get", "project"); err != nil {
		return newDoctorCheck("doppler", "doppler", "auth_required", "DOPPLER_TOKEN is not set and local doppler config was not found")
	}
	return newDoctorCheck("doppler", "doppler", "warn", "DOPPLER_TOKEN is not set; will use local doppler config")
}

func checkSystemdUser() doctorCheck {
	if _, err := lookPath("systemctl"); err != nil {
		return newDoctorCheck("systemd_user", "systemd user", "missing", "systemctl not found in PATH")
	}
	if err := runShortCommand("systemctl", "--user", "is-system-running"); err != nil {
		return newDoctorCheck("systemd_user", "systemd user", "blocked", "systemctl --user is not running")
	}
	return newDoctorCheck("systemd_user", "systemd user", "ok", "systemctl --user is running")
}

func checkCodexClone(target string) doctorCheck {
	path := defaultCodexClone(target)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return newDoctorCheck("codex_clone", "Codex clone", "ok", path)
	}
	return newDoctorCheck("codex_clone", "Codex clone", "missing", path+" does not exist")
}

func checkWritableDir(id, label, path string) doctorCheck {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return newDoctorCheck(id, label, "blocked", err.Error())
	}
	probe, err := os.CreateTemp(path, ".run-weaver-check-*")
	if err != nil {
		return newDoctorCheck(id, label, "blocked", err.Error())
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return newDoctorCheck(id, label, "ok", "writable: "+path)
}

func checkWritableFile(id, label, path string) doctorCheck {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return newDoctorCheck(id, label, "blocked", err.Error())
	}
	probe, err := os.CreateTemp(dir, ".run-weaver-check-*")
	if err != nil {
		return newDoctorCheck(id, label, "blocked", err.Error())
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return newDoctorCheck(id, label, "ok", "writable: "+path)
}

func checkService(target string) doctorCheck {
	switch target {
	case "wsl":
		userDir := filepath.Join(homeDir(), ".config", "systemd", "user")
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			return newDoctorCheck("service", "service", "blocked", err.Error())
		}
		if _, err := os.Stat(filepath.Join(userDir, "run-weaver.service")); err == nil {
			return newDoctorCheck("service", "service", "ok", "run-weaver.service is installed")
		}
		return newDoctorCheck("service", "service", "missing", "run-weaver.service is not installed")
	case "windows":
		if currentGOOS != "windows" {
			return newDoctorCheck("service", "service", "blocked", "Task Scheduler check requires Windows")
		}
		if err := runShortCommand("schtasks", "/Query", "/TN", "run-weaver"); err != nil {
			return newDoctorCheck("service", "service", "missing", "Task Scheduler task run-weaver is not installed")
		}
		return newDoctorCheck("service", "service", "ok", "Task Scheduler task run-weaver is installed")
	default:
		return newDoctorCheck("service", "service", "blocked", "unknown target")
	}
}

func newDoctorCheck(id, label, status, message string) doctorCheck {
	return doctorCheck{
		ID:      id,
		Label:   label,
		Status:  status,
		Message: message,
	}
}

func summarizeDoctor(checks []doctorCheck) (string, int) {
	exitCode := exitOK
	overall := "ok"
	for _, check := range checks {
		switch check.Status {
		case "blocked":
			if check.ID == "os_target" {
				return "blocked", exitTargetMismatch
			}
			overall = "blocked"
			exitCode = maxExit(exitCode, exitConfigMissing)
		case "auth_required":
			if overall != "blocked" {
				overall = "blocked"
			}
			exitCode = maxExit(exitCode, exitAuthRequired)
		case "missing":
			if overall != "blocked" {
				overall = "blocked"
			}
			exitCode = maxExit(exitCode, exitMissing)
		case "warn":
			if overall == "ok" {
				overall = "warn"
			}
		}
	}
	return overall, exitCode
}

func maxExit(current, next int) int {
	if current == exitOK {
		return next
	}
	if current == exitAuthRequired || next == exitAuthRequired {
		return exitAuthRequired
	}
	if current == exitMissing || next == exitMissing {
		return exitMissing
	}
	return next
}

func doctorNextSteps(target string, checks []doctorCheck) []string {
	steps := make([]string, 0)
	for _, check := range checks {
		switch check.ID {
		case "gh_auth":
			if check.Status == "auth_required" {
				steps = append(steps, "gh auth login")
			}
		case "codex":
			if check.Status == "auth_required" {
				steps = append(steps, "codex login")
			}
		case "doppler":
			if check.Status == "auth_required" || check.Status == "missing" {
				steps = append(steps, "configure Doppler or set DOPPLER_TOKEN")
			}
		case "codex_clone", "service":
			if check.Status == "missing" {
				steps = append(steps, "run-weaver install --target "+target)
			}
		}
	}
	return dedupeStrings(steps)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func insertAfter(checks []doctorCheck, id string, check doctorCheck) []doctorCheck {
	for i, existing := range checks {
		if existing.ID == id {
			out := append(checks[:i+1], append([]doctorCheck{check}, checks[i+1:]...)...)
			return out
		}
	}
	return append(checks, check)
}

func runShortCommand(name string, args ...string) error {
	_, err := runShortCommandOutput(name, args...)
	return err
}

func defaultRunShortCommandOutput(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return out, nil
}

func isWSL() bool {
	if os.Getenv("WSL_INTEROP") != "" || os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

func defaultStateFile(target string) string {
	return filepath.Join(defaultStateRoot(target), "state.json")
}

func defaultStateRoot(target string) string {
	if target == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "run-weaver")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "run-weaver")
	}
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "run-weaver")
	}
	return filepath.Join(homeDir(), ".local", "state", "run-weaver")
}

func defaultDataRoot(target string) string {
	if target == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "run-weaver")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "run-weaver")
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "run-weaver")
	}
	return filepath.Join(homeDir(), ".local", "share", "run-weaver")
}

func defaultCodexClone(target string) string {
	return filepath.Join(defaultDataRoot(target), "src")
}

func defaultWorktreeRoot(target string) string {
	return filepath.Join(defaultDataRoot(target), "worktrees")
}

func defaultLogDir(target string) string {
	return filepath.Join(defaultDataRoot(target), "logs")
}

func defaultDaemonLogFile(target string) string {
	return filepath.Join(defaultLogDir(target), "daemon.log")
}

func homeDir() string {
	if currentGOOS != "windows" {
		if home := os.Getenv("HOME"); home != "" {
			return home
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
