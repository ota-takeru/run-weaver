package cli

import (
	"errors"
	"os"
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

func TestWSLServiceFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("PATH", "/home/me/.nvm/versions/node/v22/bin:/usr/bin")

	service := wslServiceFile("/home/me/.local/bin/run-weaver", installOptions{
		Repo:         "example/repo",
		RepoURL:      "https://github.com/example/repo.git",
		PollInterval: time.Minute,
	})

	for _, want := range []string{
		"[Unit]",
		`Environment="PATH=/home/me/.local/bin:` + filepath.Join(tempDir, ".local", "bin") + `:/home/me/.nvm/versions/node/v22/bin:/usr/bin"`,
		"ExecStart=\"/home/me/.local/bin/run-weaver\" daemon --target wsl",
		"--repo-url \"https://github.com/example/repo.git\"",
		"--poll-interval 1m0s",
		"--repo \"example/repo\"",
		"Restart=always",
		"WantedBy=default.target",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("service = %q, missing %q", service, want)
		}
	}
}

func TestInstallWSLCreatesSystemdUserService(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("WSL_INTEROP", "1")
	var calls []string
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		calls = append(calls, commandKey(name, args...))
		return nil, nil
	})
	defer restoreCommands()

	err := installWSL(installOptions{
		Repo:         "example/repo",
		RepoURL:      "https://github.com/example/repo.git",
		Binary:       "/home/me/.local/bin/run-weaver",
		PollInterval: time.Minute,
	})

	if err != nil {
		t.Fatal(err)
	}
	servicePath := filepath.Join(tempDir, ".config", "systemd", "user", "run-weaver.service")
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `--repo "example/repo"`) {
		t.Fatalf("service = %q", data)
	}
	if !containsString(calls, "systemctl\x00--user\x00daemon-reload") || !containsString(calls, "systemctl\x00--user\x00enable\x00--now\x00run-weaver.service") {
		t.Fatalf("calls = %#v", calls)
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

func TestRunInstallWindowsDoesNotRequireRepoURL(t *testing.T) {
	restorePlatform := setTestGOOS(t, "windows")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tempDir)
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return nil, nil
	})
	defer restoreCommands()
	var stdout, stderr strings.Builder

	code := runInstall([]string{"--target", "windows", "--binary", `C:\bin\run-weaver.exe`}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	if strings.Contains(stderr.String(), "requires --repo-url") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunInstallWSLRegistersExplicitRepoURL(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("WSL_INTEROP", "1")
	var calls []string
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		calls = append(calls, commandKey(name, args...))
		if commandKey(name, args...) == "git\x00remote\x00get-url\x00origin" {
			return []byte("https://github.com/example/repo.git\n"), nil
		}
		return nil, nil
	})
	defer restoreCommands()
	var stdout, stderr strings.Builder

	code := runInstall([]string{"--target", "wsl", "--binary", "/home/me/.local/bin/run-weaver", "--repo-url", "https://github.com/example/repo.git"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	servicePath := filepath.Join(tempDir, ".config", "systemd", "user", "run-weaver.service")
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	service := string(data)
	if !strings.Contains(service, `--repo-url "https://github.com/example/repo.git"`) {
		t.Fatalf("service = %q, want inferred repo URL", service)
	}
	if !strings.Contains(service, `--repo "example/repo"`) {
		t.Fatalf("service = %q, want inferred GitHub repo", service)
	}
	config, err := readRepoConfig(defaultRepoConfigFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Repositories) != 1 || config.Repositories[0].Repository != "example/repo" {
		t.Fatalf("config = %#v", config)
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
