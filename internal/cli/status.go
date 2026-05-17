package cli

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var currentGOOS = runtime.GOOS

type statusResult struct {
	Output   statusOutput
	ExitCode int
	Err      error
}

func collectStatus(client githubClient) statusResult {
	target := defaultStatusTarget()
	statePath := defaultStateFile(target)
	output := statusOutput{
		SchemaVersion: stateSchemaVersion,
		Target:        target,
		StateFile:     statePath,
		Daemon: daemonStatus{
			Running: false,
		},
		Job: nil,
		Reconciliation: reconciliationStatus{
			GitHubLabelState: "not_checked",
			ProcessState:     "not_checked",
			TmuxState:        "not_applicable",
			Conflicts:        []string{},
		},
	}

	state, err := readStateFile(statePath)
	if err != nil {
		output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "state_file_unavailable")
		return statusResult{Output: output, ExitCode: exitStateUnavailable, Err: err}
	}

	output.Target = state.Target
	output.Daemon = daemonStatus{
		Running: processRunning(state.Daemon.PID),
		PID:     state.Daemon.PID,
		Service: state.Daemon.Service,
	}
	output.Reconciliation.ProcessState = processState(output.Daemon.Running, state.Daemon.PID)

	if state.Job != nil {
		output.Job = stateJobToStatusJob(state.Job)
		githubState, err := githubLabelState(client, state.Job.Issue.Number)
		if err != nil {
			output.Reconciliation.GitHubLabelState = "unavailable"
			output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "github_unavailable")
			return statusResult{Output: output, ExitCode: exitStatusAuthRequired, Err: err}
		}
		output.Reconciliation.GitHubLabelState = githubState
		if githubState != "none" && githubState != state.Job.LabelState {
			output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "github_label_mismatch")
			output.Job.LabelState = githubState
		}
		output.Reconciliation.TmuxState = tmuxState(state.Target, state.Job.Tmux)
	} else {
		output.Reconciliation.GitHubLabelState = "not_applicable"
		output.Reconciliation.TmuxState = "not_applicable"
	}
	if state.Campaign != nil {
		output.Campaign = stateCampaignToStatusCampaign(state.Campaign)
	}

	conflicts := detectStatusConflicts(output)
	output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, conflicts...)
	if output.Job != nil {
		output.Job.RuntimeState = jobRuntimeState(output)
	}
	if len(output.Reconciliation.Conflicts) > 0 {
		return statusResult{Output: output, ExitCode: exitStatusConflict}
	}
	return statusResult{Output: output, ExitCode: exitOK}
}

func printStatusHuman(w io.Writer, output statusOutput) {
	fmt.Fprintf(w, "Target: %s\n", emptyAsUnknown(output.Target))
	if output.Daemon.PID > 0 {
		fmt.Fprintf(w, "Daemon: %s (pid %d)\n", runningWord(output.Daemon.Running), output.Daemon.PID)
	} else {
		fmt.Fprintf(w, "Daemon: %s\n", runningWord(output.Daemon.Running))
	}
	fmt.Fprintf(w, "State file: %s\n\n", emptyAsUnknown(output.StateFile))

	if output.Job == nil {
		fmt.Fprintln(w, "Current job: none")
	} else {
		fmt.Fprintln(w, "Current job:")
		fmt.Fprintf(w, "  Issue: #%d %s\n", output.Job.Issue.Number, output.Job.Issue.Title)
		fmt.Fprintf(w, "  Label state: %s\n", emptyAsUnknown(output.Job.LabelState))
		fmt.Fprintf(w, "  Runtime state: %s\n", emptyAsUnknown(output.Job.RuntimeState))
		fmt.Fprintf(w, "  Branch: %s\n", emptyAsUnknown(output.Job.Branch))
		fmt.Fprintf(w, "  Worktree: %s\n", emptyAsUnknown(output.Job.Worktree))
		fmt.Fprintf(w, "  Claim: %s\n", emptyAsUnknown(output.Job.ClaimID))
		fmt.Fprintf(w, "  Claimed at: %s\n", emptyAsUnknown(output.Job.ClaimedAt))
		if output.Job.Tmux != nil {
			fmt.Fprintf(w, "  tmux: %s:%s\n", output.Job.Tmux.Session, output.Job.Tmux.Window)
		}
	}
	if output.Campaign != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Campaign:")
		fmt.Fprintf(w, "  Issue: #%d %s\n", output.Campaign.Issue.Number, output.Campaign.Issue.Title)
		fmt.Fprintf(w, "  Status: %s\n", emptyAsUnknown(output.Campaign.Status))
		fmt.Fprintf(w, "  Current task: %s\n", emptyAsNone(output.Campaign.CurrentTaskID))
		fmt.Fprintf(w, "  Completed tasks: %d/%d\n", len(output.Campaign.CompletedTasks), len(output.Campaign.Tasks))
		if len(output.Campaign.PRs) > 0 {
			fmt.Fprintf(w, "  PRs: %d\n", len(output.Campaign.PRs))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Reconciliation:")
	fmt.Fprintf(w, "  GitHub: %s\n", output.Reconciliation.GitHubLabelState)
	fmt.Fprintf(w, "  Process: %s\n", output.Reconciliation.ProcessState)
	if output.Reconciliation.TmuxState != "" {
		fmt.Fprintf(w, "  tmux: %s\n", output.Reconciliation.TmuxState)
	}
	if output.Job != nil {
		fmt.Fprintf(w, "  Last GitHub comment: %s\n", emptyAsNone(output.Job.LastGitHubCommentAt))
		fmt.Fprintf(w, "  Last error: %s\n", stringPtrAsNone(output.Job.LastError))
	}
	if len(output.Reconciliation.Conflicts) == 0 {
		fmt.Fprintln(w, "  Conflicts: none")
		return
	}
	fmt.Fprintf(w, "  Conflicts: %s\n", strings.Join(output.Reconciliation.Conflicts, ", "))
}

func stateCampaignToStatusCampaign(campaign *stateCampaign) *statusCampaign {
	if campaign == nil {
		return nil
	}
	return &statusCampaign{
		Issue:          campaign.Issue,
		Status:         campaign.Status,
		CurrentTaskID:  campaign.CurrentTaskID,
		CompletedTasks: append([]string(nil), campaign.CompletedTasks...),
		Tasks:          append([]campaignTask(nil), campaign.Tasks...),
		Decisions:      append([]campaignDecision(nil), campaign.Decisions...),
		PRs:            append([]campaignPR(nil), campaign.PRs...),
	}
}

func stateJobToStatusJob(job *stateJob) *statusJob {
	return &statusJob{
		Issue:               job.Issue,
		LabelState:          job.LabelState,
		RuntimeState:        "unknown",
		Branch:              job.Branch,
		Worktree:            job.Worktree,
		ClaimID:             job.ClaimID,
		ClaimedAt:           job.ClaimedAt,
		Tmux:                job.Tmux,
		LastGitHubCommentAt: job.LastGitHubCommentAt,
		LastError:           job.LastError,
		Codex:               job.Codex,
	}
}

func jobRuntimeState(output statusOutput) string {
	if output.Job == nil {
		return "none"
	}
	if len(output.Reconciliation.Conflicts) > 0 {
		return "needs_attention"
	}
	switch output.Job.LabelState {
	case blockedLabel:
		return "blocked"
	case doneLabel:
		return "done"
	case runningLabel:
		if codexRateLimited(output.Job) && output.Reconciliation.TmuxState != "window_exists" {
			return "rate_limited_waiting"
		}
		if codexLastMessageExists(output.Job) && output.Reconciliation.TmuxState != "window_exists" {
			return "codex_completed"
		}
		if output.Reconciliation.TmuxState == "window_exists" {
			return "codex_running"
		}
		return "running"
	default:
		return "unknown"
	}
}

func codexLastMessageExists(job *statusJob) bool {
	if job == nil || job.Codex == nil || job.Codex.LastMessagePath == "" {
		return false
	}
	_, err := os.Stat(job.Codex.LastMessagePath)
	return err == nil
}

func codexRateLimited(job *statusJob) bool {
	return job != nil && job.Codex != nil && codexLogLooksRateLimited(job.Codex.JSONLogPath)
}

func defaultStatusTarget() string {
	if currentGOOS == "windows" {
		return "windows"
	}
	if currentGOOS == "linux" && isWSL() {
		return "wsl"
	}
	return ""
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	if currentGOOS == "windows" {
		return windowsProcessRunning(pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func windowsProcessRunning(pid int) bool {
	out, err := runShortCommandOutput("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	if err != nil {
		return false
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	reader.FieldsPerRecord = -1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil || len(record) < 2 {
			return false
		}
		if record[1] == strconv.Itoa(pid) {
			return true
		}
	}
}

func processState(running bool, pid int) string {
	if pid <= 0 {
		return "not_recorded"
	}
	if running {
		return "running"
	}
	return "not_running"
}

func tmuxState(target string, tmux *tmuxRef) string {
	if target != "wsl" {
		return "not_applicable"
	}
	if tmux == nil || tmux.Session == "" || tmux.Window == "" {
		return "not_recorded"
	}
	if _, err := lookPath("tmux"); err != nil {
		return "tmux_missing"
	}
	if err := runShortCommand("tmux", "has-session", "-t", tmux.Session+":"+tmux.Window); err != nil {
		return "window_missing"
	}
	return "window_exists"
}

func detectStatusConflicts(output statusOutput) []string {
	conflicts := make([]string, 0)
	if output.Job != nil && output.Job.LabelState == "running" && output.Reconciliation.ProcessState == "not_running" {
		conflicts = append(conflicts, "running_job_without_process")
	}
	if output.Job != nil && output.Job.LabelState == "running" && output.Reconciliation.TmuxState == "window_missing" && !codexLastMessageExists(output.Job) && !codexRateLimited(output.Job) {
		conflicts = append(conflicts, "running_job_without_tmux_window")
	}
	if output.Target == "" {
		conflicts = append(conflicts, "unknown_target")
	}
	return conflicts
}

func githubLabelState(client githubClient, issueNumber int) (string, error) {
	if client == nil || issueNumber == 0 {
		return "not_applicable", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	issue, err := client.ViewIssue(ctx, issueNumber)
	if err != nil {
		return "", err
	}
	for _, state := range []string{runningLabel, doneLabel, blockedLabel} {
		for _, label := range issue.Labels {
			if label.Name == state {
				return state, nil
			}
		}
	}
	return "none", nil
}

func runningWord(running bool) string {
	if running {
		return "running"
	}
	return "not running"
}

func emptyAsUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func emptyAsNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func stringPtrAsNone(value *string) string {
	if value == nil || *value == "" {
		return "none"
	}
	return *value
}

func parsePID(value string) int {
	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return pid
}

func stateFileNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
