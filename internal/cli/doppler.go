package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

var dopplerConfigNames = []string{
	"doppler.yaml",
	"doppler.yml",
	".doppler.yaml",
	".doppler.yml",
}

type dopplerRequirement struct {
	Required bool
	Reason   string
}

func resolveDopplerRequirement(mode, repoRoot string) dopplerRequirement {
	if mode == "" {
		mode = dopplerModeAuto
	}
	switch mode {
	case dopplerModeRequired:
		return dopplerRequirement{Required: true, Reason: "configured as required"}
	case dopplerModeOptional:
		return dopplerRequirement{Required: false, Reason: "configured as optional"}
	default:
		if hasDopplerConfig(repoRoot) {
			return dopplerRequirement{Required: true, Reason: "Doppler config file detected"}
		}
		return dopplerRequirement{Required: false, Reason: "Doppler config file not found"}
	}
}

func hasDopplerConfig(repoRoot string) bool {
	if repoRoot == "" {
		return false
	}
	for _, name := range dopplerConfigNames {
		if info, err := os.Stat(filepath.Join(repoRoot, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func checkDopplerAvailable(required bool) doctorCheck {
	return checkDopplerReady(required, "")
}

func checkDopplerReady(required bool, repoRoot string) doctorCheck {
	if !required {
		return newDoctorCheck("doppler", "doppler", "ok", "not required for this repository")
	}
	if _, err := lookPath("doppler"); err != nil {
		return newDoctorCheck("doppler", "doppler", "missing", "doppler is required but not found in PATH")
	}

	probeDir := existingDir(repoRoot)
	if err := probeDopplerRun(probeDir); err == nil {
		if os.Getenv("DOPPLER_TOKEN") != "" {
			return newDoctorCheck("doppler", "doppler", "ok", "DOPPLER_TOKEN is set and doppler run works")
		}
		if hasDopplerConfig(repoRoot) {
			return newDoctorCheck("doppler", "doppler", "ok", "repository Doppler config works")
		}
		return newDoctorCheck("doppler", "doppler", "ok", "local Doppler config works")
	}
	if hasDopplerConfig(repoRoot) {
		return newDoctorCheck("doppler", "doppler", "auth_required", "repository Doppler config was found, but doppler run failed")
	}
	if os.Getenv("DOPPLER_TOKEN") != "" {
		return newDoctorCheck("doppler", "doppler", "auth_required", "DOPPLER_TOKEN is set, but doppler run failed")
	}
	return newDoctorCheck("doppler", "doppler", "auth_required", "doppler is required but DOPPLER_TOKEN is not set and doppler run failed")
}

func requireDopplerForCodex(mode, repoRoot string) error {
	requirement := resolveDopplerRequirement(mode, repoRoot)
	if !requirement.Required {
		return nil
	}
	check := checkDopplerReady(true, repoRoot)
	if check.Status != "ok" {
		return fmt.Errorf("%s: %s", check.Status, check.Message)
	}
	return nil
}

func probeDopplerRun(dir string) error {
	_, err := runShortCommandInDir(dir, "doppler", "run", "--", "git", "--version")
	return err
}

func existingDir(path string) string {
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}
