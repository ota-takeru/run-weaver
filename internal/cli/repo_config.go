package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const repoConfigSchemaVersion = 1

type repoConfigFile struct {
	SchemaVersion int         `json:"schemaVersion"`
	Repositories  []repoEntry `json:"repositories"`
}

type repoEntry struct {
	Repository string `json:"repository"`
	RepoURL    string `json:"repoUrl"`
	AddedFrom  string `json:"addedFrom"`
	Enabled    bool   `json:"enabled"`
	AddedAt    string `json:"addedAt"`
}

func defaultRepoConfigFile(target string) string {
	if target == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "run-weaver", "repos.json")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "run-weaver", "repos.json")
	}
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return filepath.Join(configHome, "run-weaver", "repos.json")
	}
	return filepath.Join(homeDir(), ".config", "run-weaver", "repos.json")
}

func readRepoConfig(path string) (*repoConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config repoConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse repo config %s: %w", path, err)
	}
	if config.SchemaVersion == 0 {
		config.SchemaVersion = repoConfigSchemaVersion
	}
	if config.SchemaVersion != repoConfigSchemaVersion {
		return nil, fmt.Errorf("unsupported repo config schema version %d", config.SchemaVersion)
	}
	return &config, nil
}

func writeRepoConfig(path string, config repoConfigFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	config.SchemaVersion = repoConfigSchemaVersion
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func addRepoConfigEntry(path string, entry repoEntry) error {
	config, err := readRepoConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		config = &repoConfigFile{SchemaVersion: repoConfigSchemaVersion}
	}
	entry.Repository = strings.TrimSpace(entry.Repository)
	entry.RepoURL = strings.TrimSpace(entry.RepoURL)
	if entry.Repository == "" {
		return fmt.Errorf("GitHub repository could not be inferred from origin remote")
	}
	if entry.RepoURL == "" {
		return fmt.Errorf("repository URL is required")
	}
	if entry.AddedAt == "" {
		entry.AddedAt = time.Now().UTC().Format(time.RFC3339)
	}
	entry.Enabled = true
	for i := range config.Repositories {
		if config.Repositories[i].Repository == entry.Repository {
			config.Repositories[i].RepoURL = entry.RepoURL
			config.Repositories[i].AddedFrom = entry.AddedFrom
			config.Repositories[i].Enabled = true
			if config.Repositories[i].AddedAt == "" {
				config.Repositories[i].AddedAt = entry.AddedAt
			}
			return writeRepoConfig(path, *config)
		}
	}
	config.Repositories = append(config.Repositories, entry)
	return writeRepoConfig(path, *config)
}

func removeRepoConfigEntry(path, repository string) (bool, error) {
	config, err := readRepoConfig(path)
	if err != nil {
		return false, err
	}
	next := config.Repositories[:0]
	removed := false
	for _, entry := range config.Repositories {
		if entry.Repository == repository {
			removed = true
			continue
		}
		next = append(next, entry)
	}
	config.Repositories = next
	if !removed {
		return false, nil
	}
	return true, writeRepoConfig(path, *config)
}

func enabledRepoEntries(config *repoConfigFile) []repoEntry {
	if config == nil {
		return nil
	}
	var result []repoEntry
	for _, entry := range config.Repositories {
		if entry.Enabled {
			result = append(result, entry)
		}
	}
	return result
}

func currentRepoEntry() (repoEntry, error) {
	repoURL, err := resolveRepoURL("")
	if err != nil {
		return repoEntry{}, err
	}
	repository := inferGitHubRepo(repoURL)
	if repository == "" {
		return repoEntry{}, fmt.Errorf("GitHub repository could not be inferred from origin remote %q", repoURL)
	}
	addedFrom, _ := os.Getwd()
	return repoEntry{
		Repository: repository,
		RepoURL:    repoURL,
		AddedFrom:  addedFrom,
		Enabled:    true,
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func repoSlug(repository string) string {
	slug := slugify(strings.ReplaceAll(repository, "/", "-"))
	if slug == "" {
		return "repo"
	}
	return slug
}

func stateFileForRepo(target, repository string) string {
	if strings.TrimSpace(repository) == "" {
		return defaultStateFile(target)
	}
	return filepath.Join(defaultStateRoot(target), "repos", repoSlug(repository), "state.json")
}

func dataRootForRepo(target, repository string) string {
	if strings.TrimSpace(repository) == "" {
		return defaultDataRoot(target)
	}
	return filepath.Join(defaultDataRoot(target), "repos", repoSlug(repository))
}
