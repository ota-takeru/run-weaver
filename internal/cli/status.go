package cli

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
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
	return collectStatusForRepo(client, "")
}

func collectStatusForRepo(client githubClient, repository string) statusResult {
	target := defaultStatusTarget()
	statePath := stateFileForRepo(target, repository)
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
	if err != nil && repository != "" && errors.Is(err, os.ErrNotExist) {
		legacyPath := defaultStateFile(target)
		if legacyState, legacyErr := readStateFile(legacyPath); legacyErr == nil {
			if stateBelongsToRepository(legacyState, repository) {
				state = legacyState
				statePath = legacyPath
				output.StateFile = legacyPath
				err = nil
			}
		}
	}
	if err != nil {
		output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "state_file_unavailable")
		return statusResult{Output: output, ExitCode: exitStateUnavailable, Err: err}
	}
	if repository == "" {
		if inferred := inferRepoFromState(state); inferred != "" {
			repository = inferred
			if gh, ok := client.(ghClient); ok && gh.repo == "" {
				client = ghClient{repo: inferred}
			}
		}
	}

	output.Target = state.Target
	output.Daemon = daemonStatus{
		Running: processRunning(state.Daemon.PID),
		PID:     state.Daemon.PID,
		Service: state.Daemon.Service,
	}
	output.Reconciliation.ProcessState = processState(output.Daemon.Running, state.Daemon.PID)

	var githubErr error
	if state.Job != nil {
		output.Job = stateJobToStatusJob(state.Job)
		githubState, err := githubLabelState(client, state.Job.Issue.Number)
		if err != nil {
			githubErr = err
			output.Reconciliation.GitHubLabelState = "unavailable"
			output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "github_unavailable")
		} else {
			output.Reconciliation.GitHubLabelState = githubState
			if githubState != "none" && githubState != state.Job.LabelState {
				output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, "github_label_mismatch")
				output.Job.LabelState = githubState
			}
		}
		output.Reconciliation.TmuxState = tmuxState(state.Target, state.Job.Tmux)
	} else {
		output.Reconciliation.GitHubLabelState = "not_applicable"
		output.Reconciliation.TmuxState = "not_applicable"
	}
	if state.Campaign != nil {
		output.Campaign = stateCampaignToStatusCampaign(state.Campaign)
	}
	output.ReadyQueue = collectReadyQueueStatus(client, state)

	conflicts := detectStatusConflicts(output)
	output.Reconciliation.Conflicts = append(output.Reconciliation.Conflicts, conflicts...)
	if output.Job != nil {
		output.Job.RuntimeState = jobRuntimeState(output)
	}
	if stringSliceContains(output.Reconciliation.Conflicts, "github_unavailable") {
		return statusResult{Output: output, ExitCode: exitStatusAuthRequired, Err: githubErr}
	}
	if len(output.Reconciliation.Conflicts) > 0 {
		return statusResult{Output: output, ExitCode: exitStatusConflict}
	}
	return statusResult{Output: output, ExitCode: exitOK}
}

func inferRepoFromState(state *stateFile) string {
	if state == nil {
		return ""
	}
	if state.Job != nil {
		if state.Job.Issue.Repository != "" {
			return state.Job.Issue.Repository
		}
		if repo := inferGitHubRepoFromIssueURL(state.Job.Issue.URL); repo != "" {
			return repo
		}
	}
	if state.Campaign != nil {
		if state.Campaign.Issue.Repository != "" {
			return state.Campaign.Issue.Repository
		}
		if repo := inferGitHubRepoFromIssueURL(state.Campaign.Issue.URL); repo != "" {
			return repo
		}
	}
	return ""
}

func stateBelongsToRepository(state *stateFile, repository string) bool {
	return repository != "" && inferRepoFromState(state) == repository
}

func collectMultiStatus(repos []repoEntry, target string) statusResult {
	output := statusOutput{
		SchemaVersion: stateSchemaVersion,
		Target:        target,
		StateFile:     defaultRepoConfigFile(target),
		Daemon:        daemonStatus{Running: false},
		Reconciliation: reconciliationStatus{
			GitHubLabelState: "not_applicable",
			ProcessState:     "not_checked",
			TmuxState:        "not_applicable",
			Conflicts:        []string{},
		},
	}
	exitCode := exitOK
	for _, repo := range repos {
		result := collectStatusForRepo(ghClient{repo: repo.Repository}, repo.Repository)
		output.Repositories = append(output.Repositories, statusRepository{
			Repository: repo.Repository,
			RepoURL:    repo.RepoURL,
			Status:     result.Output,
		})
		if result.ExitCode > exitCode {
			exitCode = result.ExitCode
		}
	}
	return statusResult{Output: output, ExitCode: exitCode}
}

func printStatusHuman(w io.Writer, output statusOutput) {
	if len(output.Repositories) > 0 {
		fmt.Fprintf(w, "Target: %s\n", emptyAsUnknown(output.Target))
		fmt.Fprintf(w, "Repository config: %s\n\n", emptyAsUnknown(output.StateFile))
		for _, repo := range output.Repositories {
			fmt.Fprintf(w, "Repository: %s\n", repo.Repository)
			if repo.Status.Job == nil {
				fmt.Fprintln(w, "  Current job: none")
			} else {
				fmt.Fprintf(w, "  Current job: #%d %s\n", repo.Status.Job.Issue.Number, repo.Status.Job.Issue.Title)
				fmt.Fprintf(w, "  Runtime state: %s\n", emptyAsUnknown(repo.Status.Job.RuntimeState))
				fmt.Fprintf(w, "  Worktree: %s\n", emptyAsUnknown(repo.Status.Job.Worktree))
			}
			fmt.Fprintf(w, "  State file: %s\n", repo.Status.StateFile)
			if len(repo.Status.ReadyQueue) > 0 {
				fmt.Fprintf(w, "  Ready queue: %d\n", len(repo.Status.ReadyQueue))
			}
			if len(repo.Status.Reconciliation.Conflicts) == 0 {
				fmt.Fprintln(w, "  Conflicts: none")
			} else {
				fmt.Fprintf(w, "  Conflicts: %s\n", strings.Join(repo.Status.Reconciliation.Conflicts, ", "))
			}
		}
		return
	}
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
		if output.Campaign.Planner != nil {
			fmt.Fprintf(w, "  Planner: %s\n", plannerRuntimeState(output.Target, output.Campaign.Planner))
			if output.Campaign.Planner.Tmux != nil {
				fmt.Fprintf(w, "  Planner tmux: %s:%s\n", output.Campaign.Planner.Tmux.Session, output.Campaign.Planner.Tmux.Window)
			}
		}
		fmt.Fprintf(w, "  Current task: %s\n", emptyAsNone(output.Campaign.CurrentTaskID))
		fmt.Fprintf(w, "  Completed tasks: %d/%d\n", len(output.Campaign.CompletedTasks), len(output.Campaign.Tasks))
		if len(output.Campaign.PRs) > 0 {
			fmt.Fprintf(w, "  PRs: %d\n", len(output.Campaign.PRs))
		}
		if len(output.Campaign.Decisions) > 0 {
			fmt.Fprintln(w, "  Decisions:")
			for _, decision := range output.Campaign.Decisions {
				fmt.Fprintf(w, "    %s: %s\n", decision.ID, emptyAsUnknown(decision.Status))
				if decision.Answer != "" {
					fmt.Fprintf(w, "      answer: %s\n", decision.Answer)
				}
				if decision.Status == "pending" {
					if len(decision.Options) > 0 {
						fmt.Fprintf(w, "      options: %s\n", strings.Join(decision.Options, ", "))
					}
					fmt.Fprintf(w, "      answer command: %s\n", campaignDecisionAnswerCommand(output.Campaign, decision.ID))
				}
			}
		}
	}
	if len(output.ReadyQueue) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Ready queue:")
		for _, item := range output.ReadyQueue {
			deps := ""
			if len(item.Dependencies) > 0 {
				parts := make([]string, 0, len(item.Dependencies))
				for _, dependency := range item.Dependencies {
					parts = append(parts, "#"+strconv.Itoa(dependency))
				}
				deps = " depends on " + strings.Join(parts, ", ")
			}
			fmt.Fprintf(w, "  #%d %s: %s%s\n", item.Issue.Number, item.Issue.Title, item.State, deps)
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

func campaignDecisionAnswerCommand(campaign *statusCampaign, decisionID string) string {
	if campaign == nil {
		return ""
	}
	repo := campaign.Issue.Repository
	if repo == "" {
		repo = inferGitHubRepoFromIssueURL(campaign.Issue.URL)
	}
	repoArg := ""
	if repo != "" {
		repoArg = " --repo " + repo
	}
	return fmt.Sprintf("run-weaver decision answer%s '#%d' %s <option>", repoArg, campaign.Issue.Number, decisionID)
}

func collectReadyQueueStatus(client githubClient, state *stateFile) []readyQueueItem {
	issues, err := client.ListReadyIssues(context.Background())
	if err != nil {
		return nil
	}
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Number < issues[j].Number
	})
	queue := make([]readyQueueItem, 0, len(issues))
	for _, listed := range issues {
		issue, err := client.ViewIssue(context.Background(), listed.Number)
		if err != nil {
			continue
		}
		dependencies, ambiguous := detectIssueDependencies(issue)
		item := readyQueueItem{
			Issue: issueRef{
				Number: issue.Number,
				Title:  issue.Title,
				URL:    issue.URL,
			},
			State:        readyQueueStateReady,
			Dependencies: dependencies,
		}
		switch {
		case ambiguous:
			item.State = readyQueueStateBlockedDependency
		case len(dependencies) > 0:
			_, blocked, waiting, err := resolveIssueDependencies(context.Background(), client, state, dependencies)
			if err != nil || blocked != "" {
				item.State = readyQueueStateBlockedDependency
			} else if waiting {
				item.State = readyQueueStateWaitingDependency
			}
		}
		queue = append(queue, item)
	}
	return queue
}

func stateCampaignToStatusCampaign(campaign *stateCampaign) *statusCampaign {
	if campaign == nil {
		return nil
	}
	return &statusCampaign{
		Issue:          campaign.Issue,
		Status:         campaign.Status,
		CurrentTaskID:  campaign.CurrentTaskID,
		Planner:        campaign.Planner,
		CompletedTasks: append([]string(nil), campaign.CompletedTasks...),
		Tasks:          append([]campaignTask(nil), campaign.Tasks...),
		Decisions:      append([]campaignDecision(nil), campaign.Decisions...),
		PRs:            append([]campaignPR(nil), campaign.PRs...),
	}
}

func plannerRuntimeState(target string, planner *campaignPlanner) string {
	if planner == nil {
		return "none"
	}
	if planner.LastError != nil {
		return "blocked"
	}
	if tmuxState(target, planner.Tmux) == "window_exists" {
		return "codex_running"
	}
	if planner.LastMessagePath != "" {
		if _, err := os.Stat(planner.LastMessagePath); err == nil {
			return "codex_completed"
		}
	}
	return "running"
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
	if hasRuntimeAttentionConflict(output.Reconciliation.Conflicts) {
		return "needs_attention"
	}
	switch output.Job.LabelState {
	case blockedLabel:
		return "blocked"
	case doneLabel:
		return "done"
	case runningLabel:
		if codexLastMessageExists(output.Job) && output.Reconciliation.TmuxState != "window_exists" {
			return "codex_completed"
		}
		if codexRateLimited(output.Job) && output.Reconciliation.TmuxState != "window_exists" {
			return "rate_limited_waiting"
		}
		if output.Reconciliation.TmuxState == "window_exists" {
			return "codex_running"
		}
		return "running"
	default:
		return "unknown"
	}
}

func hasRuntimeAttentionConflict(conflicts []string) bool {
	for _, conflict := range conflicts {
		if conflict != "github_unavailable" {
			return true
		}
	}
	return false
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
