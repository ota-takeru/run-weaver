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
	if os.Getenv("DOPPLER_TOKEN") != "" {
		return newDoctorCheck("doppler", "doppler", "ok", "DOPPLER_TOKEN is set")
	}
	if hasDopplerConfig(repoRoot) {
		return newDoctorCheck("doppler", "doppler", "ok", "repository Doppler config found")
	}
	if err := runShortCommand("doppler", "configure", "get", "project"); err != nil {
		return newDoctorCheck("doppler", "doppler", "auth_required", "doppler is required but DOPPLER_TOKEN is not set and local config was not found")
	}
	return newDoctorCheck("doppler", "doppler", "ok", "local Doppler config found")
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
