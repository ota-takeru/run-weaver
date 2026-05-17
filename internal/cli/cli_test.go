package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRunRequiresCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run(nil, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected usage on stderr")
	}
}

func TestDoctorRequiresTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"doctor"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if got := stderr.String(); got == "" || !bytes.Contains([]byte(got), []byte("--target is required")) {
		t.Fatalf("stderr = %q, want target error", got)
	}
}

func TestDoctorRejectsInvalidTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"doctor", "--target", "linux"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if got := stderr.String(); got == "" || !bytes.Contains([]byte(got), []byte("invalid target")) {
		t.Fatalf("stderr = %q, want invalid target error", got)
	}
}

func TestDoctorJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")

	code := Run([]string{"doctor", "--target", "wsl", "--json"}, &stdout, &stderr)

	if code == exitUsage {
		t.Fatalf("exit code = %d, want doctor result; stderr = %q", code, stderr.String())
	}
	var got doctorOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	if got.Target != "wsl" {
		t.Fatalf("target = %q, want wsl", got.Target)
	}
	if got.Checks == nil {
		t.Fatal("checks is nil")
	}
}

func TestDoctorStopsOnTargetMismatch(t *testing.T) {
	if currentGOOS == "windows" {
		t.Skip("windows target is not a mismatch on Windows")
	}
	var stdout, stderr bytes.Buffer
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)

	code := Run([]string{"doctor", "--target", "windows", "--json"}, &stdout, &stderr)

	if code != exitTargetMismatch {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitTargetMismatch, stderr.String())
	}
	var got doctorOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	if len(got.Checks) != 1 || got.Checks[0].ID != "os_target" {
		t.Fatalf("checks = %#v, want only os_target", got.Checks)
	}
}

func TestWindowsDoctorChecksDependenciesAndTaskScheduler(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	if err := os.MkdirAll(filepath.Join(tempDir, "run-weaver", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	commands := map[string]error{
		"gh\x00auth\x00status":                    nil,
		"codex\x00exec\x00--help":                 nil,
		"codex\x00login\x00status":                nil,
		"doppler\x00configure\x00get\x00project":  nil,
		"schtasks\x00/Query\x00/TN\x00run-weaver": nil,
	}
	restoreCommands := stubCommandEnvironment(t, commands)
	defer restoreCommands()

	result := collectDoctor("windows")

	if result.ExitCode != exitOK {
		t.Fatalf("exit code = %d, want %d; output = %#v", result.ExitCode, exitOK, result.Output)
	}
	if result.Output.StateFile != filepath.Join(tempDir, "run-weaver", "state.json") {
		t.Fatalf("state file = %q", result.Output.StateFile)
	}
	checks := checksByID(result.Output.Checks)
	for _, id := range []string{"os_target", "git", "gh_auth", "codex", "doppler", "codex_clone", "worktree_root", "state_file", "service"} {
		if _, ok := checks[id]; !ok {
			t.Fatalf("missing check %q in %#v", id, result.Output.Checks)
		}
	}
	if checks["service"].Status != "ok" {
		t.Fatalf("service check = %#v, want ok", checks["service"])
	}
	if checks["worktree_root"].Status != "ok" || !strings.Contains(checks["worktree_root"].Message, filepath.Join(tempDir, "run-weaver", "worktrees")) {
		t.Fatalf("worktree root check = %#v", checks["worktree_root"])
	}
}

func TestWindowsDoctorReportsMissingTaskSchedulerTask(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	if err := os.MkdirAll(filepath.Join(tempDir, "run-weaver", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	commands := map[string]error{
		"gh\x00auth\x00status":                    nil,
		"codex\x00exec\x00--help":                 nil,
		"codex\x00login\x00status":                nil,
		"doppler\x00configure\x00get\x00project":  nil,
		"schtasks\x00/Query\x00/TN\x00run-weaver": errors.New("task missing"),
	}
	restoreCommands := stubCommandEnvironment(t, commands)
	defer restoreCommands()

	result := collectDoctor("windows")
	checks := checksByID(result.Output.Checks)

	if checks["service"].Status != "missing" {
		t.Fatalf("service check = %#v, want missing", checks["service"])
	}
}

func TestStatusJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir)

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitStateUnavailable {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitStateUnavailable, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte(`"schemaVersion":1`)) {
		t.Fatalf("stdout = %q, want status schema version", got)
	}
}

func TestStatusReadsStateFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir)

	path := defaultStateFile("wsl")
	lastError := "previous failure"
	err := writeStateFile(path, stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		UpdatedAt:     "2026-05-16T09:31:10Z",
		Daemon: stateDaemon{
			PID:     0,
			Service: "run-weaver.service",
		},
		Job: &stateJob{
			Issue: issueRef{
				Number:     42,
				Title:      "Add account export",
				URL:        "https://github.com/example/repo/issues/42",
				Repository: "example/repo",
			},
			LabelState:          "blocked",
			Branch:              "codex/issue-42-add-account-export",
			Worktree:            tempDir + "/worktrees/issue-42",
			ClaimID:             "run-weaver:wsl:20260516T093000Z",
			ClaimedAt:           "2026-05-16T09:30:00Z",
			LastGitHubCommentAt: "2026-05-16T09:31:10Z",
			LastError:           &lastError,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := collectStatus(newFakeGitHubClient(githubIssue{Number: 42, Labels: []githubLabel{{Name: blockedLabel}}}))
	got := result.Output
	if result.ExitCode != exitOK {
		t.Fatalf("exit code = %d, want %d; err = %v", result.ExitCode, exitOK, result.Err)
	}
	if got.Job == nil || got.Job.Issue.Number != 42 {
		t.Fatalf("job = %#v, want issue 42", got.Job)
	}
	if got.Reconciliation.ProcessState != "not_recorded" {
		t.Fatalf("process state = %q, want not_recorded", got.Reconciliation.ProcessState)
	}
}

func TestStatusDetectsGitHubLabelMismatch(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir)
	err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Add export"},
			LabelState: runningLabel,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := collectStatus(newFakeGitHubClient(githubIssue{
		Number: 42,
		Labels: []githubLabel{{Name: blockedLabel}},
	}))

	if result.ExitCode != exitStatusConflict {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, exitStatusConflict)
	}
	if result.Output.Job == nil || result.Output.Job.LabelState != blockedLabel {
		t.Fatalf("job = %#v, want GitHub label to win", result.Output.Job)
	}
	if !containsString(result.Output.Reconciliation.Conflicts, "github_label_mismatch") {
		t.Fatalf("conflicts = %#v, want github_label_mismatch", result.Output.Reconciliation.Conflicts)
	}
}

func TestWindowsProcessRunningParsesTasklistCSV(t *testing.T) {
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		if name != "tasklist" {
			t.Fatalf("command = %q, want tasklist", name)
		}
		return []byte("\"run-weaver.exe\",\"4321\",\"Console\",\"1\",\"12,345 K\"\r\n"), nil
	})
	defer restoreCommands()

	if !windowsProcessRunning(4321) {
		t.Fatal("windowsProcessRunning returned false, want true")
	}
}

func TestWindowsProcessRunningReturnsFalseWhenTasklistHasNoMatch(t *testing.T) {
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return []byte("INFO: No tasks are running which match the specified criteria.\r\n"), nil
	})
	defer restoreCommands()

	if windowsProcessRunning(4321) {
		t.Fatal("windowsProcessRunning returned true, want false")
	}
}

func TestWindowsProcessRunningReturnsFalseOnTasklistError(t *testing.T) {
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("tasklist failed")
	})
	defer restoreCommands()

	if windowsProcessRunning(4321) {
		t.Fatal("windowsProcessRunning returned true, want false")
	}
}

func TestWindowsStatusUsesTasklistForProcessAndNoTmux(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		if name != "tasklist" {
			t.Fatalf("command = %q, want tasklist", name)
		}
		return []byte("\"run-weaver.exe\",\"4321\",\"Console\",\"1\",\"12,345 K\"\r\n"), nil
	})
	defer restoreCommands()
	if err := writeStateFile(defaultStateFile("windows"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "windows",
		Daemon:        stateDaemon{PID: 4321, Service: "run-weaver"},
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Windows status"},
			LabelState: blockedLabel,
		},
	}); err != nil {
		t.Fatal(err)
	}

	result := collectStatus(newFakeGitHubClient(githubIssue{
		Number: 42,
		Labels: []githubLabel{{Name: blockedLabel}},
	}))

	if result.ExitCode != exitOK {
		t.Fatalf("exit code = %d, want %d; output = %#v", result.ExitCode, exitOK, result.Output)
	}
	if result.Output.Reconciliation.ProcessState != "running" {
		t.Fatalf("process state = %q, want running", result.Output.Reconciliation.ProcessState)
	}
	if result.Output.Reconciliation.TmuxState != "not_applicable" {
		t.Fatalf("tmux state = %q, want not_applicable", result.Output.Reconciliation.TmuxState)
	}
}

func TestWindowsStatusReportsNotRunningFromTasklist(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return []byte("INFO: No tasks are running which match the specified criteria.\r\n"), nil
	})
	defer restoreCommands()
	if err := writeStateFile(defaultStateFile("windows"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "windows",
		Daemon:        stateDaemon{PID: 4321, Service: "run-weaver"},
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Windows status"},
			LabelState: blockedLabel,
		},
	}); err != nil {
		t.Fatal(err)
	}

	result := collectStatus(newFakeGitHubClient(githubIssue{
		Number: 42,
		Labels: []githubLabel{{Name: blockedLabel}},
	}))

	if result.ExitCode != exitOK {
		t.Fatalf("exit code = %d, want %d; output = %#v", result.ExitCode, exitOK, result.Output)
	}
	if result.Output.Reconciliation.ProcessState != "not_running" {
		t.Fatalf("process state = %q, want not_running", result.Output.Reconciliation.ProcessState)
	}
}

func TestWindowsStatusRepoUsesFakeGHFromPath(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		if name == "tasklist" {
			return []byte("\"run-weaver.exe\",\"4321\",\"Console\",\"1\",\"12,345 K\"\r\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", name, args)
	})
	defer restoreCommands()
	writeFakeGH(t, tempDir)
	if err := writeStateFile(defaultStateFile("windows"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "windows",
		Daemon:        stateDaemon{PID: 4321, Service: "run-weaver"},
		Job: &stateJob{
			Issue:      issueRef{Number: 42, Title: "Windows status"},
			LabelState: blockedLabel,
		},
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := runStatus([]string{"--json", "--repo", "example/repo"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q; stdout = %q", code, exitOK, stderr.String(), stdout.String())
	}
	var got statusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	if got.Reconciliation.GitHubLabelState != blockedLabel {
		t.Fatalf("github label state = %q, want %q", got.Reconciliation.GitHubLabelState, blockedLabel)
	}
}

func setTestGOOS(t *testing.T, goos string) func() {
	t.Helper()
	original := currentGOOS
	currentGOOS = goos
	return func() {
		currentGOOS = original
	}
}

func stubCommandEnvironment(t *testing.T, commands map[string]error) func() {
	t.Helper()
	restoreOutput := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		err, ok := commands[commandKey(name, args...)]
		if !ok {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		return nil, err
	})
	originalLookPath := lookPath
	lookPath = func(name string) (string, error) {
		switch name {
		case "git", "gh", "codex", "doppler", "schtasks", "tasklist", "tmux", "systemctl":
			return filepath.Join("fake-bin", name), nil
		default:
			return "", exec.ErrNotFound
		}
	}
	return func() {
		restoreOutput()
		lookPath = originalLookPath
	}
}

func stubCommandOutput(t *testing.T, fn func(string, ...string) ([]byte, error)) func() {
	t.Helper()
	original := runShortCommandOutput
	runShortCommandOutput = fn
	return func() {
		runShortCommandOutput = original
	}
}

func commandKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

func checksByID(checks []doctorCheck) map[string]doctorCheck {
	result := make(map[string]doctorCheck, len(checks))
	for _, check := range checks {
		result[check.ID] = check
	}
	return result
}

func writeFakeGH(t *testing.T, dir string) {
	t.Helper()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		path := filepath.Join(binDir, "gh.cmd")
		script := "@echo off\r\n" +
			"echo {\"number\":42,\"title\":\"Windows status\",\"labels\":[{\"name\":\"blocked\"}],\"comments\":[]}\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		path := filepath.Join(binDir, "gh")
		script := "#!/bin/sh\n" +
			"printf '%s\\n' '{\"number\":42,\"title\":\"Windows status\",\"labels\":[{\"name\":\"blocked\"}],\"comments\":[]}'\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pathValue := binDir
	if existing := os.Getenv("PATH"); existing != "" {
		pathValue += string(os.PathListSeparator) + existing
	}
	t.Setenv("PATH", pathValue)
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}
