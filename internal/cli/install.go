package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type installOptions struct {
	Repo         string
	RepoURL      string
	Binary       string
	PollInterval time.Duration
}

type windowsInstallOptions struct {
	Repo         string
	RepoURL      string
	Binary       string
	PollInterval time.Duration
}

func installWSL(opts installOptions) error {
	if currentGOOS != "linux" || !isWSL() {
		return fmt.Errorf("WSL install requires WSL")
	}
	binary, err := installBinaryPath(opts.Binary)
	if err != nil {
		return err
	}
	for _, dir := range []string{defaultDataRoot("wsl"), defaultWorktreeRoot("wsl"), defaultLogDir("wsl")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	serviceDir := filepath.Join(homeDir(), ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return err
	}
	servicePath := filepath.Join(serviceDir, "run-weaver.service")
	if err := os.WriteFile(servicePath, []byte(wslServiceFile(binary, opts)), 0o644); err != nil {
		return err
	}
	if err := runShortCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runShortCommand("systemctl", "--user", "enable", "--now", "run-weaver.service"); err != nil {
		return err
	}
	return nil
}

func installWindows(opts windowsInstallOptions) error {
	if currentGOOS != "windows" {
		return fmt.Errorf("Windows install requires Windows")
	}
	binary, err := installBinaryPath(opts.Binary)
	if err != nil {
		return err
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

func installBinaryPath(binary string) (string, error) {
	if binary != "" {
		return binary, nil
	}
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	return current, nil
}

func wslServiceFile(binary string, opts installOptions) string {
	args := []string{
		systemdQuote(binary),
		"daemon",
		"--target", "wsl",
		"--poll-interval", opts.PollInterval.String(),
	}
	if opts.RepoURL != "" {
		args = append(args, "--repo-url", systemdQuote(opts.RepoURL))
	}
	if opts.Repo != "" {
		args = append(args, "--repo", systemdQuote(opts.Repo))
	}
	return strings.Join([]string{
		"[Unit]",
		"Description=run-weaver agent",
		"After=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"ExecStart=" + strings.Join(args, " "),
		"Restart=always",
		"RestartSec=30",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
}

func systemdQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func windowsTaskCommand(binary string, opts windowsInstallOptions) string {
	parts := []string{
		windowsCmdQuote(binary),
		"daemon",
		"--target", "windows",
		"--poll-interval", opts.PollInterval.String(),
	}
	if opts.RepoURL != "" {
		parts = append(parts, "--repo-url", windowsCmdQuote(opts.RepoURL))
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
