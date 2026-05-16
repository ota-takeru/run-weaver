package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

const (
	exitOK    = 0
	exitUsage = 64
)

var validTargets = map[string]struct{}{
	"wsl":     {},
	"windows": {},
}

// Run executes the run-weaver CLI and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return exitUsage
	}

	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRootUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printRootUsage(stderr)
		return exitUsage
	}
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("doctor", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	result := collectDoctor(*target)
	if *jsonOutput {
		writeJSON(stdout, result.Output)
		return result.ExitCode
	}

	printDoctorHuman(stdout, result.Output)
	return result.ExitCode
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("status", stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}

	if *jsonOutput {
		writeJSON(stdout, statusOutput{
			SchemaVersion: 1,
			Target:        "",
			StateFile:     "",
			Daemon: daemonStatus{
				Running: false,
			},
			Job: nil,
			Reconciliation: reconciliationStatus{
				Conflicts: []string{},
			},
		})
		return exitOK
	}

	fmt.Fprintln(stdout, "Target: unknown")
	fmt.Fprintln(stdout, "Daemon: unknown")
	fmt.Fprintln(stdout, "State file: unknown")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Current job: none")
	return exitOK
}

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("install", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	fmt.Fprintf(stdout, "install skeleton for target %s\n", *target)
	return exitOK
}

func runDaemon(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("daemon", stderr)
	target := fs.String("target", "", "target environment: wsl or windows")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}

	fmt.Fprintf(stdout, "daemon skeleton for target %s\n", *target)
	return exitOK
}

func newFlagSet(name string, output io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer) bool {
	if err := fs.Parse(args); err != nil {
		return false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument for %s: %s\n", fs.Name(), fs.Arg(0))
		return false
	}
	return true
}

func validateTarget(target string, required bool, stderr io.Writer) bool {
	if target == "" {
		if required {
			fmt.Fprintln(stderr, "--target is required")
			return false
		}
		return true
	}

	if _, ok := validTargets[target]; !ok {
		fmt.Fprintf(stderr, "invalid target %q: expected wsl or windows\n", target)
		return false
	}
	return true
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  doctor   Check dependencies and authentication")
	fmt.Fprintln(w, "  status   Show daemon and job status")
	fmt.Fprintln(w, "  install  Install target-specific daemon configuration")
	fmt.Fprintln(w, "  daemon   Run the issue processing daemon")
}

func writeJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

type doctorOutput struct {
	Target    string        `json:"target"`
	Overall   string        `json:"overall"`
	StateFile string        `json:"stateFile"`
	Checks    []doctorCheck `json:"checks"`
	Next      []string      `json:"next"`
}

type doctorCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type statusOutput struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Target         string               `json:"target"`
	StateFile      string               `json:"stateFile"`
	Daemon         daemonStatus         `json:"daemon"`
	Job            any                  `json:"job"`
	Reconciliation reconciliationStatus `json:"reconciliation"`
}

type daemonStatus struct {
	Running bool `json:"running"`
}

type reconciliationStatus struct {
	Conflicts []string `json:"conflicts"`
}
