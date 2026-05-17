package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func runRepo(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRepoUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "add":
		return runRepoAdd(args[1:], stdout, stderr)
	case "list":
		return runRepoList(args[1:], stdout, stderr)
	case "remove":
		return runRepoRemove(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRepoUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown repo command: %s\n\n", args[0])
		printRepoUsage(stderr)
		return exitUsage
	}
}

func runRepoAdd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("repo add", stderr)
	target := fs.String("target", defaultStatusTarget(), "target environment: wsl or windows")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	entry, err := currentRepoEntry()
	if err != nil {
		fmt.Fprintf(stderr, "repo add error: %v\n", err)
		return exitConfigMissing
	}
	if err := addRepoConfigEntry(defaultRepoConfigFile(*target), entry); err != nil {
		fmt.Fprintf(stderr, "repo add error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "registered %s\n", entry.Repository)
	return exitOK
}

func runRepoList(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("repo list", stderr)
	target := fs.String("target", defaultStatusTarget(), "target environment: wsl or windows")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if !parseFlags(fs, args, stderr) {
		return exitUsage
	}
	config, err := readRepoConfig(defaultRepoConfigFile(*target))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			config = &repoConfigFile{SchemaVersion: repoConfigSchemaVersion}
		} else {
			fmt.Fprintf(stderr, "repo list error: %v\n", err)
			return exitConfigMissing
		}
	}
	if *jsonOutput {
		writeJSON(stdout, config)
		return exitOK
	}
	if len(config.Repositories) == 0 {
		fmt.Fprintln(stdout, "Registered repositories: none")
		return exitOK
	}
	fmt.Fprintln(stdout, "Registered repositories:")
	for _, entry := range config.Repositories {
		status := "disabled"
		if entry.Enabled {
			status = "enabled"
		}
		fmt.Fprintf(stdout, "  %s  %s  %s\n", entry.Repository, status, entry.RepoURL)
	}
	return exitOK
}

func runRepoRemove(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("repo remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "repo remove requires owner/repo")
		return exitUsage
	}
	target := defaultStatusTarget()
	removed, err := removeRepoConfigEntry(defaultRepoConfigFile(target), fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "repo remove error: %v\n", err)
		return exitConfigMissing
	}
	if !removed {
		fmt.Fprintf(stderr, "repository not registered: %s\n", fs.Arg(0))
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "removed %s\n", fs.Arg(0))
	return exitOK
}

func printRepoUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver repo <add|list|remove>")
}
