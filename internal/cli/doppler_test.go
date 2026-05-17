package cli

import (
	"errors"
	"os"
	"path/filepath"
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
