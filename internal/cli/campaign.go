package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	campaignStatusPlanning         = "planning"
	campaignStatusPlanned          = "planned"
	campaignStatusRunning          = "running"
	campaignStatusDecisionRequired = "decision_required"
	campaignStatusBlocked          = "blocked"
	campaignStatusDone             = "done"

	campaignTaskPending   = "pending"
	campaignTaskRunning   = "running"
	campaignTaskCompleted = "completed"
	campaignTaskBlocked   = "blocked"

	pipelinePhasePlan      = "plan"
	pipelinePhaseImplement = "implement"
	pipelinePhaseReview    = "review"
	pipelinePhaseVerify    = "verify"
)

type campaignPlan struct {
	Tasks     []campaignTask
	Decisions []campaignDecision
}

var decisionAnswerRE = regexp.MustCompile(`run-weaver-decision:([a-z0-9-]+):([A-Za-z0-9_-]+)`)

func processCampaign(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (string, bool, error) {
	if state != nil && state.Campaign != nil {
		if state.Job != nil && state.Job.LabelState == runningLabel {
			return "", false, nil
		}
		if state.Campaign.Status == campaignStatusPlanning {
			return completeCampaignPlanning(ctx, deps, opts, state)
		}
		return dispatchCampaignTask(ctx, deps, opts, state)
	}
	campaigns, err := deps.github.ListCampaignIssues(ctx)
	if err != nil {
		return "", false, err
	}
	if len(campaigns) == 0 {
		return "", false, nil
	}
	issue := campaigns[0]
	claim, err := claimIssue(ctx, deps.github, issue.Number, opts.target)
	if err != nil {
		return "", false, err
	}
	if claim.Outcome != claimWon {
		return fmt.Sprintf("campaign issue #%d claim %s", issue.Number, claim.Outcome), true, nil
	}
	completedIssues := currentCompletedIssues(state)
	state, err = startCampaignPlanning(ctx, deps, opts, claim.Issue, claim.ClaimID)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "campaign planner", err)
		return "", true, err
	}
	state.CompletedIssues = append([]completedIssue(nil), completedIssues...)
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "campaign state file", err)
		return "", true, err
	}
	return fmt.Sprintf("started campaign planner for issue #%d", issue.Number), true, nil
}

func startCampaignPlanning(ctx context.Context, deps daemonDeps, opts daemonOptions, issue githubIssue, claimID string) (*stateFile, error) {
	spec, err := deps.worktree.PrepareForRepo(ctx, opts.target, opts.repo, issue, opts.repoURL)
	if err != nil {
		return nil, err
	}
	runSpec := buildCampaignPlannerRunSpec(opts.target, opts.repo, issue.Number, spec.Path)
	if err := opts.prepareCodexRunSpec(&runSpec, spec.Path); err != nil {
		return nil, err
	}
	if err := writeCampaignPlannerPromptFile(runSpec.PromptPath, issue); err != nil {
		return nil, err
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return &stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        opts.target,
		UpdatedAt:     now,
		Daemon:        stateDaemon{Service: "run-weaver.service"},
		Campaign: &stateCampaign{
			Issue: issueRef{
				Number:     issue.Number,
				Title:      issue.Title,
				URL:        issue.URL,
				Repository: opts.repo,
			},
			Status: campaignStatusPlanning,
			Planner: &campaignPlanner{
				Worktree:        spec.Path,
				Branch:          spec.Branch,
				ClaimID:         claimID,
				Tmux:            tmux,
				LastMessagePath: runSpec.LastMessagePath,
				JSONLogPath:     runSpec.JSONLogPath,
				StartedAt:       now,
				Codex: &codexState{
					LastMessagePath: runSpec.LastMessagePath,
					JSONLogPath:     runSpec.JSONLogPath,
				},
			},
		},
		Job:             nil,
		CompletedIssues: []completedIssue{},
	}, nil
}

func completeCampaignPlanning(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (string, bool, error) {
	if state.Campaign.Planner == nil {
		err := fmt.Errorf("campaign planner state is missing")
		return blockCampaignPlanning(ctx, deps.github, opts, state, err)
	}
	plannerTmuxState := tmuxState(opts.target, state.Campaign.Planner.Tmux)
	if plannerTmuxState == "window_exists" {
		return "campaign planning", true, nil
	}
	if _, err := os.Stat(state.Campaign.Planner.LastMessagePath); err != nil {
		if opts.target == "wsl" && plannerTmuxState == "window_missing" {
			return blockCampaignPlanning(ctx, deps.github, opts, state, fmt.Errorf("campaign planner did not produce last-message JSON: %w", err))
		}
		return "campaign planning", true, nil
	}
	data, err := os.ReadFile(state.Campaign.Planner.LastMessagePath)
	if err != nil {
		return blockCampaignPlanning(ctx, deps.github, opts, state, err)
	}
	plan, err := parseCampaignPlannerOutput(data)
	if err != nil {
		return blockCampaignPlanning(ctx, deps.github, opts, state, err)
	}
	updated, err := createCampaignStateFromPlan(ctx, deps.github, opts, state, plan)
	if err != nil {
		return blockCampaignPlanning(ctx, deps.github, opts, state, err)
	}
	updated.CompletedIssues = append([]completedIssue(nil), state.CompletedIssues...)
	if err := writeStateFile(opts.stateFilePath(), *updated); err != nil {
		return "", true, err
	}
	if updated.Campaign.Status == campaignStatusDecisionRequired {
		_ = markBlocked(ctx, deps.github, updated.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
	}
	return fmt.Sprintf("planned campaign issue #%d with %d tasks", updated.Campaign.Issue.Number, len(updated.Campaign.Tasks)), true, nil
}

func blockCampaignPlanning(ctx context.Context, client githubClient, opts daemonOptions, state *stateFile, cause error) (string, bool, error) {
	message := cause.Error()
	state.Campaign.Status = campaignStatusBlocked
	if state.Campaign.Planner != nil {
		state.Campaign.Planner.LastError = &message
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = writeStateFile(opts.stateFilePath(), *state)
	_ = markBlocked(ctx, client, state.Campaign.Issue.Number, "campaign planner", cause)
	return "campaign planner blocked", true, cause
}

func createCampaignStateFromPlan(ctx context.Context, client githubClient, opts daemonOptions, state *stateFile, plan campaignPlan) (*stateFile, error) {
	parent := githubIssue{
		Number: state.Campaign.Issue.Number,
		Title:  state.Campaign.Issue.Title,
		URL:    state.Campaign.Issue.URL,
	}
	if err := client.EnsureLabel(ctx, readyLabel); err != nil {
		return nil, err
	}
	if err := client.EnsureLabel(ctx, campaignTaskLabel); err != nil {
		return nil, err
	}
	tasks := make([]campaignTask, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		child, err := client.CreateIssue(ctx, issueCreateSpec{
			Title:  task.Title,
			Body:   campaignChildIssueBody(parent, task),
			Labels: []string{readyLabel, campaignTaskLabel},
		})
		if err != nil {
			return nil, err
		}
		task.IssueNumber = child.Number
		task.IssueURL = child.URL
		tasks = append(tasks, task)
	}
	decisions := plan.Decisions
	for i := range decisions {
		if decisions[i].Status == "" {
			decisions[i].Status = "pending"
		}
		if err := client.Comment(ctx, parent.Number, decisionRequestBody(state.Campaign.Issue, decisions[i])); err != nil {
			return nil, err
		}
	}
	status := campaignStatusPlanned
	if len(decisions) > 0 && !hasExecutableCampaignTask(tasks, decisions) {
		status = campaignStatusDecisionRequired
	}
	updated := &stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        opts.target,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Daemon:        stateDaemon{Service: "run-weaver.service"},
		Job:           nil,
		Campaign: &stateCampaign{
			Issue:     state.Campaign.Issue,
			Planner:   state.Campaign.Planner,
			Status:    status,
			Tasks:     tasks,
			Decisions: decisions,
		},
	}
	return updated, nil
}

func campaignChildIssueBody(parent githubIssue, task campaignTask) string {
	return fmt.Sprintf(`# Campaign Task

Parent campaign: #%d
Task ID: %s

%s
`, parent.Number, task.ID, emptyAsNone(strings.TrimSpace(task.Body)))
}

func decisionRequestBody(parent issueRef, decision campaignDecision) string {
	return fmt.Sprintf(`<!-- run-weaver-decision-request:%s -->
run-weaver decision request.

## decision
%s

## context
%s

## evidence
%s

## options
%s

## option details
%s

## recommendation
%s

## impact
%s

## reversibility
%s

## blocked tasks
%s

## can continue tasks
%s

To answer, run:

%s

Or, from GitHub mobile, add a new comment containing exactly one of:

%s
`, decision.ID, emptyAsNone(decision.Title), emptyAsNone(decision.Context), markdownList(decision.Evidence), markdownList(decision.Options), markdownList(decision.OptionDetails), emptyAsNone(decision.Recommendation), markdownList(decision.Impact), emptyAsNone(decision.Reversibility), markdownList(decision.BlockedTasks), markdownList(decision.CanContinueTasks), decisionRequestAnswerCommand(parent, decision.ID), decisionQuickReplyLines(decision))
}

func decisionRequestAnswerCommand(parent issueRef, decisionID string) string {
	repo := parent.Repository
	if repo == "" {
		repo = inferGitHubRepoFromIssueURL(parent.URL)
	}
	repoArg := ""
	if repo != "" {
		repoArg = " --repo " + repo
	}
	return fmt.Sprintf("run-weaver decision answer%s '#%d' %s <option>", repoArg, parent.Number, decisionID)
}

func decisionQuickReplyLines(decision campaignDecision) string {
	options := trimStringSlice(decision.Options)
	if len(options) == 0 {
		return "```text\nrun-weaver-decision:" + decision.ID + ":<option>\n```"
	}
	lines := make([]string, 0, len(options))
	for _, option := range options {
		lines = append(lines, campaignDecisionAnswerMarker(decision.ID, option))
	}
	return "```text\n" + strings.Join(lines, "\n") + "\n```"
}

func markdownList(values []string) string {
	if len(values) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, "- "+value)
	}
	return strings.Join(lines, "\n")
}

type campaignPlannerOutput struct {
	Tasks     []campaignPlannerTask     `json:"tasks"`
	Decisions []campaignPlannerDecision `json:"decisions"`
}

type campaignPlannerTask struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Dependencies []string `json:"dependencies"`
}

type campaignPlannerDecision struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Context          string   `json:"context"`
	Evidence         []string `json:"evidence"`
	Options          []string `json:"options"`
	OptionDetails    []string `json:"optionDetails"`
	Recommendation   string   `json:"recommendation"`
	Impact           []string `json:"impact"`
	Reversibility    string   `json:"reversibility"`
	BlockedTasks     []string `json:"blockedTasks"`
	CanContinueTasks []string `json:"canContinueTasks"`
}

func parseCampaignPlannerOutput(data []byte) (campaignPlan, error) {
	var raw campaignPlannerOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return campaignPlan{}, fmt.Errorf("parse campaign planner JSON: %w", err)
	}
	if len(raw.Tasks) == 0 {
		return campaignPlan{}, fmt.Errorf("campaign planner returned no tasks")
	}
	tasks := make([]campaignTask, 0, len(raw.Tasks))
	taskIDs := map[string]bool{}
	for _, rawTask := range raw.Tasks {
		id := strings.TrimSpace(rawTask.ID)
		title := strings.TrimSpace(rawTask.Title)
		body := strings.TrimSpace(rawTask.Body)
		if id == "" || title == "" || body == "" {
			return campaignPlan{}, fmt.Errorf("campaign planner task must include id, title, and body")
		}
		if taskIDs[id] {
			return campaignPlan{}, fmt.Errorf("duplicate campaign task id %q", id)
		}
		taskIDs[id] = true
		tasks = append(tasks, campaignTask{
			ID:           id,
			Title:        title,
			Body:         body,
			Status:       campaignTaskPending,
			Dependencies: trimStringSlice(rawTask.Dependencies),
			Phase:        pipelinePhasePlan,
		})
	}
	for _, task := range tasks {
		for _, dependency := range task.Dependencies {
			if dependency == task.ID {
				return campaignPlan{}, fmt.Errorf("campaign task %q cannot depend on itself", task.ID)
			}
			if !taskIDs[dependency] {
				return campaignPlan{}, fmt.Errorf("campaign task %q depends on unknown task %q", task.ID, dependency)
			}
		}
	}
	decisions := make([]campaignDecision, 0, len(raw.Decisions))
	decisionIDs := map[string]bool{}
	for _, rawDecision := range raw.Decisions {
		id := strings.TrimSpace(rawDecision.ID)
		title := strings.TrimSpace(rawDecision.Title)
		if id == "" || title == "" {
			return campaignPlan{}, fmt.Errorf("campaign decision must include id and title")
		}
		if decisionIDs[id] {
			return campaignPlan{}, fmt.Errorf("duplicate campaign decision id %q", id)
		}
		decisionIDs[id] = true
		options := trimStringSlice(rawDecision.Options)
		if len(options) == 0 {
			return campaignPlan{}, fmt.Errorf("campaign decision %q must include options", id)
		}
		context := strings.TrimSpace(rawDecision.Context)
		evidence := trimStringSlice(rawDecision.Evidence)
		optionDetails := trimStringSlice(rawDecision.OptionDetails)
		impact := trimStringSlice(rawDecision.Impact)
		reversibility := strings.TrimSpace(rawDecision.Reversibility)
		if context == "" || len(evidence) == 0 || len(optionDetails) < len(options) || len(impact) == 0 || reversibility == "" {
			return campaignPlan{}, fmt.Errorf("campaign decision %q must include context, evidence, optionDetails, impact, and reversibility", id)
		}
		blocked := trimStringSlice(rawDecision.BlockedTasks)
		canContinue := trimStringSlice(rawDecision.CanContinueTasks)
		for _, taskID := range append(append([]string{}, blocked...), canContinue...) {
			if !taskIDs[taskID] {
				return campaignPlan{}, fmt.Errorf("campaign decision %q references unknown task %q", id, taskID)
			}
		}
		decisions = append(decisions, campaignDecision{
			ID:               id,
			Title:            title,
			Status:           "pending",
			Context:          context,
			Evidence:         evidence,
			Options:          options,
			OptionDetails:    optionDetails,
			Recommendation:   strings.TrimSpace(rawDecision.Recommendation),
			Impact:           impact,
			Reversibility:    reversibility,
			BlockedTasks:     blocked,
			CanContinueTasks: canContinue,
		})
	}
	return campaignPlan{Tasks: tasks, Decisions: decisions}, nil
}

func trimStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func hasExecutableCampaignTask(tasks []campaignTask, decisions []campaignDecision) bool {
	completed := completedTaskSet(tasks)
	index := nextCampaignTaskIndex(tasks, completed)
	if index < 0 {
		return false
	}
	return campaignTaskAllowedByDecisions(tasks[index].ID, decisions)
}

func dispatchCampaignTask(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (string, bool, error) {
	if state.Campaign.Status == campaignStatusBlocked {
		return "campaign blocked", true, nil
	}
	if state.Campaign.Status == campaignStatusDone {
		return "", false, nil
	}
	if state.Campaign.Status == campaignStatusDecisionRequired {
		updated, err := refreshCampaignDecisionAnswers(ctx, deps.github, state)
		if err != nil {
			return "", true, err
		}
		if updated {
			state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
				return "", true, err
			}
			_ = deps.github.RemoveLabel(ctx, state.Campaign.Issue.Number, blockedLabel)
			_ = deps.github.AddLabel(ctx, state.Campaign.Issue.Number, runningLabel)
		}
		if state.Campaign.Status == campaignStatusDecisionRequired {
			return "campaign decision_required", true, nil
		}
	}
	completed := completedTaskSet(state.Campaign.Tasks)
	taskIndex := nextCampaignTaskIndex(state.Campaign.Tasks, completed)
	if taskIndex < 0 {
		if allCampaignTasksCompleted(state.Campaign.Tasks) {
			if hasPendingCampaignDecisions(state.Campaign.Decisions) {
				state.Campaign.Status = campaignStatusDecisionRequired
				state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
					return "", true, err
				}
				_ = markBlocked(ctx, deps.github, state.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
				return "campaign decision_required", true, nil
			}
			state.Campaign.Status = campaignStatusDone
			state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
				return "", true, err
			}
			_ = markCampaignDone(ctx, deps.github, state.Campaign)
			return fmt.Sprintf("completed campaign issue #%d", state.Campaign.Issue.Number), true, nil
		}
		state.Campaign.Status = campaignStatusDecisionRequired
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
			return "", true, err
		}
		_ = markBlocked(ctx, deps.github, state.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
		return "campaign decision_required", true, nil
	}
	task := state.Campaign.Tasks[taskIndex]
	if !campaignTaskAllowedByDecisions(task.ID, state.Campaign.Decisions) {
		state.Campaign.Status = campaignStatusDecisionRequired
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
			return "", true, err
		}
		_ = markBlocked(ctx, deps.github, state.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
		return "campaign decision_required", true, nil
	}
	stackDependencies, stackBaseBranch, err := campaignStackDependencies(ctx, deps.github, state)
	if err != nil {
		state.Campaign.Tasks[taskIndex].Status = campaignTaskBlocked
		state.Campaign.Tasks[taskIndex].RetryCount++
		state.Campaign.CurrentTaskID = ""
		state.Campaign.Status = campaignStatusBlocked
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeStateFile(opts.stateFilePath(), *state)
		_ = markBlocked(ctx, deps.github, task.IssueNumber, "campaign stack", err)
		return "", true, err
	}
	claim, err := claimIssue(ctx, deps.github, task.IssueNumber, opts.target)
	if err != nil {
		return "", true, err
	}
	if claim.Outcome != claimWon {
		return fmt.Sprintf("campaign task %s claim %s", task.ID, claim.Outcome), true, nil
	}
	spec, err := deps.worktree.PrepareForRepoWithBase(ctx, opts.target, opts.repo, claim.Issue, opts.repoURL, stackBaseBranch)
	if err != nil {
		return "", true, blockCampaignTaskStart(ctx, deps.github, opts, state, taskIndex, claim.Issue, claim.ClaimID, spec, nil, "campaign worktree", err)
	}
	runSpec := buildCodexRunSpecForRepo(opts.target, opts.repo, task.IssueNumber, spec.Path, pipelinePhasePlan)
	if err := opts.prepareCodexRunSpec(&runSpec, spec.Path); err != nil {
		return "", true, blockCampaignTaskStart(ctx, deps.github, opts, state, taskIndex, claim.Issue, claim.ClaimID, spec, nil, "campaign doppler", err)
	}
	if err := writeCampaignPromptFile(runSpec.PromptPath, state.Campaign.Issue, state.Campaign.Decisions, task, claim.Issue, pipelinePhasePlan); err != nil {
		return "", true, blockCampaignTaskStart(ctx, deps.github, opts, state, taskIndex, claim.Issue, claim.ClaimID, spec, nil, "campaign prompt", err)
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		return "", true, blockCampaignTaskStart(ctx, deps.github, opts, state, taskIndex, claim.Issue, claim.ClaimID, spec, tmux, "campaign runner", err)
	}
	state.Campaign.Status = campaignStatusRunning
	state.Campaign.CurrentTaskID = task.ID
	state.Campaign.Tasks[taskIndex].Status = campaignTaskRunning
	state.Campaign.Tasks[taskIndex].Phase = pipelinePhasePlan
	state.Job = &stateJob{
		Issue: issueRef{
			Number:     task.IssueNumber,
			Title:      claim.Issue.Title,
			URL:        claim.Issue.URL,
			Repository: opts.repo,
		},
		LabelState:     runningLabel,
		Branch:         spec.Branch,
		Worktree:       spec.Path,
		BaseBranch:     stackBaseBranch,
		Dependencies:   stackDependencies,
		ClaimID:        claim.ClaimID,
		ClaimedAt:      time.Now().UTC().Format(time.RFC3339),
		CampaignTaskID: task.ID,
		PipelinePhase:  pipelinePhasePlan,
		Tmux:           tmux,
		Codex: &codexState{
			LastMessagePath: runSpec.LastMessagePath,
			JSONLogPath:     runSpec.JSONLogPath,
		},
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("started campaign task %s issue #%d", task.ID, task.IssueNumber), true, nil
}

func campaignStackDependencies(ctx context.Context, client githubClient, state *stateFile) ([]issueDependency, string, error) {
	if state == nil || state.Campaign == nil || len(state.Campaign.CompletedTasks) == 0 {
		return nil, "", nil
	}
	previousTaskID := state.Campaign.CompletedTasks[len(state.Campaign.CompletedTasks)-1]
	previousTask, ok := campaignTaskByID(state.Campaign, previousTaskID)
	if !ok || previousTask.IssueNumber == 0 {
		return nil, "", fmt.Errorf("previous campaign task %q could not be resolved", previousTaskID)
	}
	completed := completedIssueForNumber(state, previousTask.IssueNumber)
	if completed == nil || completed.Branch == "" || completed.PRURL == "" {
		issue, err := client.ViewIssue(ctx, previousTask.IssueNumber)
		if err != nil {
			return nil, "", err
		}
		completed = completedIssueFromComments(issue)
	}
	if completed == nil || completed.Branch == "" || completed.PRURL == "" {
		return nil, "", fmt.Errorf("previous campaign task %s issue #%d is completed, but its draft PR branch or URL could not be resolved", previousTask.ID, previousTask.IssueNumber)
	}
	dependency := issueDependency{
		IssueNumber: previousTask.IssueNumber,
		Branch:      completed.Branch,
		PRURL:       completed.PRURL,
	}
	return []issueDependency{dependency}, completed.Branch, nil
}

func blockCampaignTaskStart(ctx context.Context, client githubClient, opts daemonOptions, state *stateFile, taskIndex int, issue githubIssue, claimID string, spec worktreeSpec, tmux *tmuxRef, stage string, cause error) error {
	if state != nil && state.Campaign != nil && taskIndex >= 0 && taskIndex < len(state.Campaign.Tasks) {
		state.Campaign.Tasks[taskIndex].Status = campaignTaskBlocked
		state.Campaign.Tasks[taskIndex].RetryCount++
		state.Campaign.CurrentTaskID = ""
		state.Campaign.Status = campaignStatusBlocked
	}
	message := fmt.Sprintf("%s: %s", stage, cause)
	if state != nil {
		state.Job = &stateJob{
			Issue: issueRef{
				Number:     issue.Number,
				Title:      issue.Title,
				URL:        issue.URL,
				Repository: opts.repo,
			},
			LabelState: blockedLabel,
			Branch:     spec.Branch,
			Worktree:   spec.Path,
			ClaimID:    claimID,
			ClaimedAt:  time.Now().UTC().Format(time.RFC3339),
			Tmux:       tmux,
			LastError:  &message,
		}
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeStateFile(opts.stateFilePath(), *state)
	}
	_ = markBlocked(ctx, client, issue.Number, stage, cause)
	return cause
}

func campaignTaskAllowedByDecisions(taskID string, decisions []campaignDecision) bool {
	for _, decision := range decisions {
		if decision.Status != "pending" {
			continue
		}
		if len(decision.CanContinueTasks) == 0 {
			return false
		}
		if !stringSliceContains(decision.CanContinueTasks, taskID) {
			return false
		}
	}
	return true
}

func nextCampaignTaskIndex(tasks []campaignTask, completed map[string]bool) int {
	for i, task := range tasks {
		if task.Status != campaignTaskPending {
			continue
		}
		ready := true
		for _, dep := range task.Dependencies {
			if !completed[dep] {
				ready = false
				break
			}
		}
		if ready {
			return i
		}
	}
	return -1
}

func completedTaskSet(tasks []campaignTask) map[string]bool {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == campaignTaskCompleted {
			completed[task.ID] = true
		}
	}
	return completed
}

func allCampaignTasksCompleted(tasks []campaignTask) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Status != campaignTaskCompleted {
			return false
		}
	}
	return true
}

func hasPendingCampaignDecisions(decisions []campaignDecision) bool {
	for _, decision := range decisions {
		if decision.Status == "pending" {
			return true
		}
	}
	return false
}

func refreshCampaignDecisionAnswers(ctx context.Context, client githubClient, state *stateFile) (bool, error) {
	if state == nil || state.Campaign == nil || len(state.Campaign.Decisions) == 0 {
		return false, nil
	}
	issue, err := client.ViewIssue(ctx, state.Campaign.Issue.Number)
	if err != nil {
		return false, err
	}
	updated := false
	for i := range state.Campaign.Decisions {
		decision := &state.Campaign.Decisions[i]
		if decision.Status != "pending" {
			continue
		}
		answer, ok := campaignDecisionAnswerFromComments(issue.Comments, *decision)
		if !ok {
			continue
		}
		decision.Answer = answer
		decision.Status = "answered"
		updated = true
	}
	if updated && !hasPendingCampaignDecisions(state.Campaign.Decisions) {
		if allCampaignTasksCompleted(state.Campaign.Tasks) {
			state.Campaign.Status = campaignStatusDone
		} else {
			state.Campaign.Status = campaignStatusPlanned
		}
	}
	return updated, nil
}

func campaignDecisionAnswerFromComments(comments []githubComment, decision campaignDecision) (string, bool) {
	for i := len(comments) - 1; i >= 0; i-- {
		matches := decisionAnswerRE.FindAllStringSubmatch(comments[i].Body, -1)
		for j := len(matches) - 1; j >= 0; j-- {
			match := matches[j]
			if len(match) != 3 || match[1] != decision.ID {
				continue
			}
			if campaignDecisionOptionValid(decision, match[2]) {
				return match[2], true
			}
		}
	}
	return "", false
}

func campaignDecisionByID(decisions []campaignDecision, id string) *campaignDecision {
	for i := range decisions {
		if decisions[i].ID == id {
			return &decisions[i]
		}
	}
	return nil
}

func campaignDecisionOptionValid(decision campaignDecision, option string) bool {
	for _, candidate := range decision.Options {
		if candidate == option {
			return true
		}
	}
	return false
}

func readyCampaignTaskIDs(tasks []campaignTask, completed map[string]bool) []string {
	var ids []string
	for _, task := range tasks {
		if task.Status != campaignTaskPending {
			continue
		}
		ready := true
		for _, dep := range task.Dependencies {
			if !completed[dep] {
				ready = false
				break
			}
		}
		if ready {
			ids = append(ids, task.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func taskIDsBlockedByDecision(tasks []campaignTask, canContinue []string) []string {
	canContinueSet := map[string]bool{}
	for _, id := range canContinue {
		canContinueSet[id] = true
	}
	var blocked []string
	for _, task := range tasks {
		if !canContinueSet[task.ID] {
			blocked = append(blocked, task.ID)
		}
	}
	sort.Strings(blocked)
	return blocked
}

func markCampaignTaskCompleted(state *stateFile, prURL string) {
	if state == nil || state.Campaign == nil || state.Job == nil || state.Job.CampaignTaskID == "" {
		return
	}
	taskID := state.Job.CampaignTaskID
	for i := range state.Campaign.Tasks {
		if state.Campaign.Tasks[i].ID != taskID {
			continue
		}
		state.Campaign.Tasks[i].Status = campaignTaskCompleted
		state.Campaign.Tasks[i].Phase = pipelinePhaseVerify
		state.Campaign.Tasks[i].PRURL = prURL
		state.Campaign.CurrentTaskID = ""
		break
	}
	if !stringSliceContains(state.Campaign.CompletedTasks, taskID) {
		state.Campaign.CompletedTasks = append(state.Campaign.CompletedTasks, taskID)
	}
	state.Campaign.PRs = append(state.Campaign.PRs, campaignPR{TaskID: taskID, URL: prURL})
	if allCampaignTasksCompleted(state.Campaign.Tasks) && !hasPendingCampaignDecisions(state.Campaign.Decisions) {
		state.Campaign.Status = campaignStatusDone
	} else if allCampaignTasksCompleted(state.Campaign.Tasks) {
		state.Campaign.Status = campaignStatusDecisionRequired
	} else {
		state.Campaign.Status = campaignStatusPlanned
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func advanceCampaignPipeline(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (bool, string, error) {
	if state == nil || state.Job == nil || state.Job.CampaignTaskID == "" {
		return false, "", nil
	}
	nextPhase := nextCampaignPipelinePhase(state.Job.PipelinePhase)
	if nextPhase == "" {
		return false, "", nil
	}
	task, ok := campaignTaskByID(state.Campaign, state.Job.CampaignTaskID)
	if !ok {
		return false, "", fmt.Errorf("campaign task %s not found in state", state.Job.CampaignTaskID)
	}
	runSpec := buildCodexRunSpecForRepo(opts.target, opts.repo, state.Job.Issue.Number, state.Job.Worktree, nextPhase)
	if err := opts.prepareCodexRunSpec(&runSpec, state.Job.Worktree); err != nil {
		blockCampaignTaskPhase(ctx, deps.github, opts, state, task.ID, "campaign doppler", err)
		return true, "", err
	}
	issue := githubIssue{
		Number:   state.Job.Issue.Number,
		Title:    state.Job.Issue.Title,
		URL:      state.Job.Issue.URL,
		Body:     task.Body,
		Comments: nil,
	}
	if err := writeCampaignPromptFile(runSpec.PromptPath, state.Campaign.Issue, state.Campaign.Decisions, task, issue, nextPhase); err != nil {
		blockCampaignTaskPhase(ctx, deps.github, opts, state, task.ID, "campaign prompt", err)
		return true, "", err
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		blockCampaignTaskPhase(ctx, deps.github, opts, state, task.ID, "campaign runner", err)
		return true, "", err
	}
	state.Job.PipelinePhase = nextPhase
	state.Job.Tmux = tmux
	state.Job.Codex = &codexState{
		LastMessagePath: runSpec.LastMessagePath,
		JSONLogPath:     runSpec.JSONLogPath,
	}
	state.Campaign.CurrentTaskID = task.ID
	state.Campaign.Status = campaignStatusRunning
	setCampaignTaskPhase(state.Campaign, task.ID, nextPhase)
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		return true, "", err
	}
	return true, fmt.Sprintf("advanced campaign task %s to %s", task.ID, nextPhase), nil
}

func blockCampaignTaskPhase(ctx context.Context, client githubClient, opts daemonOptions, state *stateFile, taskID, stage string, cause error) {
	markCampaignTaskRetry(state.Campaign, taskID)
	if state.Campaign != nil {
		state.Campaign.Status = campaignStatusBlocked
		state.Campaign.CurrentTaskID = ""
	}
	message := fmt.Sprintf("%s: %s", stage, cause)
	if state.Job != nil {
		state.Job.LabelState = blockedLabel
		state.Job.LastError = &message
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = writeStateFile(opts.stateFilePath(), *state)
	if state.Job != nil {
		_ = markBlocked(ctx, client, state.Job.Issue.Number, stage, cause)
	}
}

func nextCampaignPipelinePhase(phase string) string {
	switch phase {
	case "", pipelinePhasePlan:
		return pipelinePhaseImplement
	case pipelinePhaseImplement:
		return pipelinePhaseReview
	case pipelinePhaseReview:
		return pipelinePhaseVerify
	default:
		return ""
	}
}

func campaignTaskByID(campaign *stateCampaign, id string) (campaignTask, bool) {
	if campaign == nil {
		return campaignTask{}, false
	}
	for _, task := range campaign.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return campaignTask{}, false
}

func setCampaignTaskPhase(campaign *stateCampaign, id, phase string) {
	if campaign == nil {
		return
	}
	for i := range campaign.Tasks {
		if campaign.Tasks[i].ID == id {
			campaign.Tasks[i].Phase = phase
			campaign.Tasks[i].Status = campaignTaskRunning
			return
		}
	}
}

func markCampaignTaskRetry(campaign *stateCampaign, id string) {
	if campaign == nil {
		return
	}
	for i := range campaign.Tasks {
		if campaign.Tasks[i].ID == id {
			campaign.Tasks[i].RetryCount++
			campaign.Tasks[i].Status = campaignTaskBlocked
			return
		}
	}
}

func markCampaignDone(ctx context.Context, client githubClient, campaign *stateCampaign) error {
	if campaign == nil {
		return nil
	}
	_ = client.RemoveLabel(ctx, campaign.Issue.Number, runningLabel)
	_ = client.RemoveLabel(ctx, campaign.Issue.Number, blockedLabel)
	if err := client.EnsureLabel(ctx, doneLabel); err != nil {
		return err
	}
	if err := client.AddLabel(ctx, campaign.Issue.Number, doneLabel); err != nil {
		return err
	}
	body := fmt.Sprintf(`run-weaver completed this campaign.

- completed tasks: %d/%d
- draft PRs: %d
`, len(campaign.CompletedTasks), len(campaign.Tasks), len(campaign.PRs))
	return client.Comment(ctx, campaign.Issue.Number, body)
}
