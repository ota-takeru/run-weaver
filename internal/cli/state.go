package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const stateSchemaVersion = 1

type stateFile struct {
	SchemaVersion int         `json:"schemaVersion"`
	Target        string      `json:"target"`
	UpdatedAt     string      `json:"updatedAt"`
	Daemon        stateDaemon `json:"daemon"`
	Job           *stateJob   `json:"job"`
}

type stateDaemon struct {
	PID     int    `json:"pid"`
	Service string `json:"service"`
}

type stateJob struct {
	Issue               issueRef    `json:"issue"`
	LabelState          string      `json:"labelState"`
	Branch              string      `json:"branch"`
	Worktree            string      `json:"worktree"`
	ClaimID             string      `json:"claimId"`
	ClaimedAt           string      `json:"claimedAt"`
	LastGitHubCommentAt string      `json:"lastGitHubCommentAt"`
	Tmux                *tmuxRef    `json:"tmux"`
	Codex               *codexState `json:"codex"`
	LastError           *string     `json:"lastError"`
}

func readStateFile(path string) (*stateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", path, err)
	}
	if state.SchemaVersion != stateSchemaVersion {
		return nil, fmt.Errorf("unsupported state schema version %d", state.SchemaVersion)
	}
	return &state, nil
}

func writeStateFile(path string, state stateFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
