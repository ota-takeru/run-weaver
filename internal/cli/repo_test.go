package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoAddListRemove(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "config"))
	t.Setenv("WSL_INTEROP", "1")
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		if commandKey(name, args...) == "git\x00remote\x00get-url\x00origin" {
			return []byte("git@github.com:example/repo.git\n"), nil
		}
		return nil, errors.New("unexpected command")
	})
	defer restoreCommands()
	var stdout, stderr bytes.Buffer

	if code := runRepo([]string{"add"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("add exit = %d; stderr = %q", code, stderr.String())
	}
	if code := runRepo([]string{"add"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("second add exit = %d; stderr = %q", code, stderr.String())
	}
	config, err := readRepoConfig(defaultRepoConfigFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Repositories) != 1 || config.Repositories[0].Repository != "example/repo" {
		t.Fatalf("config = %#v", config)
	}

	stdout.Reset()
	if code := runRepo([]string{"list", "--json"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("list exit = %d; stderr = %q", code, stderr.String())
	}
	var listed repoConfigFile
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("list output = %q: %v", stdout.String(), err)
	}
	if len(listed.Repositories) != 1 || !listed.Repositories[0].Enabled {
		t.Fatalf("listed = %#v", listed)
	}

	stdout.Reset()
	if code := runRepo([]string{"remove", "example/repo"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("remove exit = %d; stderr = %q", code, stderr.String())
	}
	config, err = readRepoConfig(defaultRepoConfigFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Repositories) != 0 {
		t.Fatalf("config after remove = %#v", config)
	}
}

func TestRepoAddRejectsNonGitDirectory(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()
	t.Setenv("WSL_INTEROP", "1")
	restoreCommands := stubCommandOutput(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("not a git repository")
	})
	defer restoreCommands()
	var stdout, stderr bytes.Buffer

	code := runRepo([]string{"add"}, &stdout, &stderr)

	if code != exitConfigMissing {
		t.Fatalf("exit = %d, want %d", code, exitConfigMissing)
	}
	if !strings.Contains(stderr.String(), "repo add error") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRepoSpecificPaths(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))

	spec := buildWorktreeSpecForRepo("wsl", "example/repo", githubIssue{Number: 42, Title: "Add Export"})

	if spec.CloneDir != filepath.Join(tempDir, "data", "run-weaver", "repos", "example-repo", "src") {
		t.Fatalf("clone dir = %q", spec.CloneDir)
	}
	if spec.Path != filepath.Join(tempDir, "data", "run-weaver", "repos", "example-repo", "worktrees", "issue-42") {
		t.Fatalf("worktree = %q", spec.Path)
	}
	statePath := stateFileForRepo("wsl", "example/repo")
	if statePath != filepath.Join(tempDir, "state", "run-weaver", "repos", "example-repo", "state.json") {
		t.Fatalf("state path = %q", statePath)
	}
	runSpec := buildCodexRunSpecForRepo("wsl", "example/repo", 42, spec.Path, "")
	if runSpec.WindowName != "example-repo-issue-42" {
		t.Fatalf("window = %q", runSpec.WindowName)
	}
}

func TestCollectMultiStatusIncludesRegisteredRepos(t *testing.T) {
	restorePlatform := setTestGOOS(t, "linux")
	defer restorePlatform()
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))
	t.Setenv("WSL_INTEROP", "1")
	if err := writeStateFile(stateFileForRepo("wsl", "example/a"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job:           nil,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeStateFile(stateFileForRepo("wsl", "example/b"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job:           nil,
	}); err != nil {
		t.Fatal(err)
	}

	result := collectMultiStatus([]repoEntry{
		{Repository: "example/a", RepoURL: "https://github.com/example/a.git", Enabled: true},
		{Repository: "example/b", RepoURL: "https://github.com/example/b.git", Enabled: true},
	}, "wsl")

	if result.ExitCode != exitOK {
		t.Fatalf("exit = %d; output = %#v", result.ExitCode, result.Output)
	}
	if len(result.Output.Repositories) != 2 {
		t.Fatalf("repositories = %#v", result.Output.Repositories)
	}
	if result.Output.Repositories[0].Repository != "example/a" || result.Output.Repositories[0].Status.StateFile != stateFileForRepo("wsl", "example/a") {
		t.Fatalf("repo A status = %#v", result.Output.Repositories[0])
	}
}
