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

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte(`"schemaVersion":1`)) {
		t.Fatalf("stdout = %q, want status schema version", got)
	}
}
