package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDopplerRequirementAutoDetectsConfig(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "doppler.yaml"), []byte("project: example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	requirement := resolveDopplerRequirement(dopplerModeAuto, repoRoot)

	if !requirement.Required {
		t.Fatalf("requirement = %#v, want required", requirement)
	}
}

func TestResolveDopplerRequirementOptionalOverridesConfig(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".doppler.yml"), []byte("project: example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	requirement := resolveDopplerRequirement(dopplerModeOptional, repoRoot)

	if requirement.Required {
		t.Fatalf("requirement = %#v, want optional", requirement)
	}
}

func TestCheckDopplerAvailableOptionalDoesNotRequireBinary(t *testing.T) {
	originalLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}
	defer func() { lookPath = originalLookPath }()

	check := checkDopplerAvailable(false)

	if check.Status != "ok" {
		t.Fatalf("check = %#v, want ok", check)
	}
}

func TestCheckDopplerAvailableRequiredMissingBinary(t *testing.T) {
	originalLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}
	defer func() { lookPath = originalLookPath }()

	check := checkDopplerAvailable(true)

	if check.Status != "missing" {
		t.Fatalf("check = %#v, want missing", check)
	}
}

func TestCheckDopplerReadyRequiresWorkingDopplerRunWithRepoConfig(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "doppler.yaml"), []byte("project: example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalLookPath := lookPath
	originalRunInDir := runShortCommandInDir
	lookPath = func(string) (string, error) {
		return "/usr/bin/doppler", nil
	}
	runShortCommandInDir = func(dir, name string, args ...string) ([]byte, error) {
		if dir != repoRoot {
			t.Fatalf("doppler probe dir = %q, want %q", dir, repoRoot)
		}
		return nil, errors.New("not authenticated")
	}
	defer func() {
		lookPath = originalLookPath
		runShortCommandInDir = originalRunInDir
	}()

	check := checkDopplerReady(true, repoRoot)

	if check.Status != "auth_required" {
		t.Fatalf("check = %#v, want auth_required", check)
	}
}

func TestCheckDopplerReadyAcceptsWorkingDopplerRun(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "doppler.yaml"), []byte("project: example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalLookPath := lookPath
	originalRunInDir := runShortCommandInDir
	lookPath = func(string) (string, error) {
		return "/usr/bin/doppler", nil
	}
	runShortCommandInDir = func(dir, name string, args ...string) ([]byte, error) {
		if name != "doppler" {
			t.Fatalf("probe command = %q, want doppler", name)
		}
		return []byte("git version 2.0\n"), nil
	}
	defer func() {
		lookPath = originalLookPath
		runShortCommandInDir = originalRunInDir
	}()

	check := checkDopplerReady(true, repoRoot)

	if check.Status != "ok" || !strings.Contains(check.Message, "works") {
		t.Fatalf("check = %#v, want ok with working probe", check)
	}
}
