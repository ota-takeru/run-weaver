package cli

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type windowsInstallOptions struct {
	Repo         string
	RepoURL      string
	Binary       string
	PollInterval time.Duration
}

func installWindows(opts windowsInstallOptions) error {
	if currentGOOS != "windows" {
		return fmt.Errorf("Windows install requires Windows")
	}
	binary := opts.Binary
	if binary == "" {
		current, err := os.Executable()
		if err != nil {
			return err
		}
		binary = current
	}
	for _, dir := range []string{defaultDataRoot("windows"), defaultWorktreeRoot("windows"), defaultLogDir("windows")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	args := []string{
		"/Create",
		"/TN", "run-weaver",
		"/SC", "ONLOGON",
		"/TR", windowsTaskCommand(binary, opts),
		"/F",
		"/RL", "LIMITED",
	}
	if err := runShortCommand("schtasks", args...); err != nil {
		return err
	}
	return nil
}

func windowsTaskCommand(binary string, opts windowsInstallOptions) string {
	parts := []string{
		windowsCmdQuote(binary),
		"daemon",
		"--target", "windows",
		"--repo-url", windowsCmdQuote(opts.RepoURL),
		"--poll-interval", opts.PollInterval.String(),
	}
	if opts.Repo != "" {
		parts = append(parts, "--repo", windowsCmdQuote(opts.Repo))
	}
	parts = append(parts, ">>", windowsCmdQuote(defaultDaemonLogFile("windows")), "2>&1")
	return `cmd /c "` + strings.Join(parts, " ") + `"`
}

func windowsCmdQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
