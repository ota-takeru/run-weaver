package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const stateSchemaVersion = 3

type stateFile struct {
	SchemaVersion   int              `json:"schemaVersion"`
	Target          string           `json:"target"`
	UpdatedAt       string           `json:"updatedAt"`
	Daemon          stateDaemon      `json:"daemon"`
	Job             *stateJob        `json:"job"`
	Campaign        *stateCampaign   `json:"campaign,omitempty"`
	CompletedIssues []completedIssue `json:"completedIssues,omitempty"`
}

type stateDaemon struct {
	PID     int    `json:"pid"`
	Service string `json:"service"`
}

type stateJob struct {
	Issue               issueRef          `json:"issue"`
	LabelState          string            `json:"labelState"`
	Branch              string            `json:"branch"`
	Worktree            string            `json:"worktree"`
	BaseBranch          string            `json:"baseBranch,omitempty"`
	Dependencies        []issueDependency `json:"dependencies,omitempty"`
	ClaimID             string            `json:"claimId"`
	ClaimedAt           string            `json:"claimedAt"`
	CampaignTaskID      string            `json:"campaignTaskId,omitempty"`
	PipelinePhase       string            `json:"pipelinePhase,omitempty"`
	RetryCount          int               `json:"retryCount,omitempty"`
	ConflictRetryCount  int               `json:"conflictRetryCount,omitempty"`
	LastGitHubCommentAt string            `json:"lastGitHubCommentAt"`
	Tmux                *tmuxRef          `json:"tmux"`
	Codex               *codexState       `json:"codex"`
	LastError           *string           `json:"lastError"`
}

type completedIssue struct {
	Issue  issueRef `json:"issue"`
	Branch string   `json:"branch"`
	PRURL  string   `json:"prUrl"`
}

type issueDependency struct {
	IssueNumber int    `json:"issueNumber"`
	Branch      string `json:"branch,omitempty"`
	PRURL       string `json:"prUrl,omitempty"`
}

type stateCampaign struct {
	Issue          issueRef           `json:"issue"`
	Status         string             `json:"status"`
	CurrentTaskID  string             `json:"currentTaskId,omitempty"`
	Planner        *campaignPlanner   `json:"planner,omitempty"`
	Tasks          []campaignTask     `json:"tasks"`
	Decisions      []campaignDecision `json:"decisions,omitempty"`
	PRs            []campaignPR       `json:"prs,omitempty"`
	CompletedTasks []string           `json:"completedTasks,omitempty"`
}

type campaignPlanner struct {
	Worktree        string      `json:"worktree"`
	Branch          string      `json:"branch"`
	ClaimID         string      `json:"claimId,omitempty"`
	Tmux            *tmuxRef    `json:"tmux,omitempty"`
	LastMessagePath string      `json:"lastMessagePath,omitempty"`
	JSONLogPath     string      `json:"jsonLogPath,omitempty"`
	StartedAt       string      `json:"startedAt"`
	LastError       *string     `json:"lastError,omitempty"`
	Codex           *codexState `json:"codex,omitempty"`
}

type campaignTask struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Body         string   `json:"body,omitempty"`
	IssueNumber  int      `json:"issueNumber,omitempty"`
	IssueURL     string   `json:"issueUrl,omitempty"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies,omitempty"`
	Phase        string   `json:"phase,omitempty"`
	RetryCount   int      `json:"retryCount,omitempty"`
	PRURL        string   `json:"prUrl,omitempty"`
}

type campaignDecision struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Options          []string `json:"options,omitempty"`
	Recommendation   string   `json:"recommendation,omitempty"`
	BlockedTasks     []string `json:"blockedTasks,omitempty"`
	CanContinueTasks []string `json:"canContinueTasks,omitempty"`
	CommentURL       string   `json:"commentUrl,omitempty"`
	Answer           string   `json:"answer,omitempty"`
}

type campaignPR struct {
	TaskID string `json:"taskId"`
	URL    string `json:"url"`
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
	if state.SchemaVersion == 1 || state.SchemaVersion == 2 {
		state.SchemaVersion = stateSchemaVersion
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
