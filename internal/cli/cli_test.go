package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
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
	if runtime.GOOS == "windows" {
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
