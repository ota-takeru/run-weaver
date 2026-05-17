package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsTaskCommand(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)

	command := windowsTaskCommand(`C:\bin\run-weaver.exe`, windowsInstallOptions{
		Repo:         "example/repo",
		RepoURL:      "https://github.com/example/repo.git",
		PollInterval: time.Minute,
	})

	for _, want := range []string{
		`cmd /c "`,
		`"C:\bin\run-weaver.exe" daemon --target windows`,
		`--repo-url "https://github.com/example/repo.git"`,
		`--poll-interval 1m0s`,
		`--repo "example/repo"`,
		`>> "` + filepath.Join(tempDir, "run-weaver", "logs", "daemon.log") + `" 2>&1`,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command = %q, missing %q", command, want)
		}
	}
}

func TestInstallWindowsCreatesPerUserTask(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	var gotName string
	var gotArgs []string
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil, nil
	})
	defer restoreCommands()

	err := installWindows(windowsInstallOptions{
		Repo:         "example/repo",
		RepoURL:      "https://github.com/example/repo.git",
		Binary:       `C:\bin\run-weaver.exe`,
		PollInterval: time.Minute,
	})

	if err != nil {
		t.Fatal(err)
	}
	if gotName != "schtasks" {
		t.Fatalf("command = %q, want schtasks", gotName)
	}
	if !slicesEqual(gotArgs[:8], []string{"/Create", "/TN", "run-weaver", "/SC", "ONLOGON", "/TR", gotArgs[6], "/F"}) {
		t.Fatalf("args = %#v", gotArgs)
	}
	if !containsString(gotArgs, "/RL") || !containsString(gotArgs, "LIMITED") {
		t.Fatalf("args = %#v, want /RL LIMITED", gotArgs)
	}
	if !strings.Contains(gotArgs[6], filepath.Join(tempDir, "run-weaver", "logs", "daemon.log")) {
		t.Fatalf("task command = %q, want daemon log path", gotArgs[6])
	}
}

func TestInstallWindowsRequiresWindows(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()

	err := installWindows(windowsInstallOptions{RepoURL: "https://github.com/example/repo.git"})

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunInstallWindowsRequiresRepoURL(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	var stdout, stderr strings.Builder

	code := runInstall([]string{"--target", "windows"}, &stdout, &stderr)

	if code != exitConfigMissing {
		t.Fatalf("exit code = %d, want %d", code, exitConfigMissing)
	}
	if !strings.Contains(stderr.String(), "--repo-url is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunInstallWindowsReportsCommandFailure(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("schtasks failed")
	})
	defer restoreCommands()
	var stdout, stderr strings.Builder

	code := runInstall([]string{"--target", "windows", "--repo-url", "https://github.com/example/repo.git", "--binary", `C:\bin\run-weaver.exe`}, &stdout, &stderr)

	if code != exitConfigMissing {
		t.Fatalf("exit code = %d, want %d", code, exitConfigMissing)
	}
	if !strings.Contains(stderr.String(), "install error") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
