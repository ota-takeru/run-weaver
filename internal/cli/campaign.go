package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	campaignStatusPlanned          = "planned"
	campaignStatusRunning          = "running"
	campaignStatusDecisionRequired = "decision_required"
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

func planCampaign(issue githubIssue) campaignPlan {
	lines := strings.Split(issue.Body, "\n")
	var tasks []campaignTask
	var decisions []campaignDecision
	seenTasks := map[string]int{}
	seenDecisions := map[string]int{}
	for _, line := range lines {
		item, ok := roadmapItem(line)
		if !ok {
			continue
		}
		title, deps := splitDependencies(item)
		if isDecisionGate(title) {
			id := uniqueCampaignID("decision", title, seenDecisions)
			decisions = append(decisions, campaignDecision{
				ID:               id,
				Title:            title,
				Status:           "pending",
				Options:          []string{"approve", "revise", "stop"},
				Recommendation:   "approve",
				BlockedTasks:     []string{},
				CanContinueTasks: []string{},
			})
			continue
		}
		id := uniqueCampaignID("task", title, seenTasks)
		tasks = append(tasks, campaignTask{
			ID:           id,
			Title:        title,
			Body:         campaignTaskBody(item, issue.Body),
			Status:       campaignTaskPending,
			Dependencies: deps,
			Phase:        pipelinePhasePlan,
		})
	}
	if len(tasks) == 0 && strings.TrimSpace(issue.Body) != "" {
		title := issue.Title
		if title == "" {
			title = "Campaign task"
		}
		tasks = append(tasks, campaignTask{
			ID:     uniqueCampaignID("task", title, seenTasks),
			Title:  title,
			Body:   strings.TrimSpace(issue.Body),
			Status: campaignTaskPending,
			Phase:  pipelinePhasePlan,
		})
	}
	linkDecisionTasks(tasks, decisions)
	return campaignPlan{Tasks: tasks, Decisions: decisions}
}

func campaignTaskBody(item, campaignBody string) string {
	item = strings.TrimSpace(item)
	campaignBody = strings.TrimSpace(campaignBody)
	if campaignBody == "" || campaignBody == item {
		return item
	}
	return item + "\n\nCampaign context:\n" + campaignBody
}

var roadmapBulletRE = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+(?:\[[ xX]\]\s*)?(.+?)\s*$`)
var decisionAnswerRE = regexp.MustCompile(`run-weaver-decision:([a-z0-9-]+):([A-Za-z0-9_-]+)`)

func roadmapItem(line string) (string, bool) {
	matches := roadmapBulletRE.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	item := strings.TrimSpace(matches[1])
	item = strings.TrimPrefix(item, "#")
	item = strings.TrimSpace(item)
	return item, item != ""
}

func splitDependencies(item string) (string, []string) {
	for _, marker := range []string{"depends:", "Depends:", "after:", "After:"} {
		index := strings.Index(item, marker)
		if index < 0 {
			continue
		}
		title := strings.TrimSpace(item[:index])
		rawDeps := strings.Split(strings.TrimSpace(item[index+len(marker):]), ",")
		deps := make([]string, 0, len(rawDeps))
		for _, dep := range rawDeps {
			dep = strings.TrimSpace(dep)
			if dep != "" {
				deps = append(deps, dep)
			}
		}
		return title, deps
	}
	return item, nil
}

func isDecisionGate(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "decision") ||
		strings.Contains(lower, "gate") ||
		strings.Contains(title, "判断") ||
		strings.Contains(title, "決定")
}

func uniqueCampaignID(prefix, title string, seen map[string]int) string {
	base := prefix + "-" + slugify(title)
	seen[base]++
	if seen[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, seen[base])
}

func linkDecisionTasks(tasks []campaignTask, decisions []campaignDecision) {
	if len(decisions) == 0 {
		return
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	for i := range decisions {
		decisions[i].BlockedTasks = append([]string(nil), taskIDs...)
	}
}

func processCampaign(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (string, bool, error) {
	if state != nil && state.Campaign != nil {
		if state.Job != nil {
			return "", false, nil
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
	planned, err := createCampaignState(ctx, deps.github, opts, claim.Issue)
	if err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "campaign planner", err)
		return "", true, err
	}
	planned.CompletedIssues = append([]completedIssue(nil), currentCompletedIssues(state)...)
	if err := writeStateFile(opts.stateFilePath(), *planned); err != nil {
		_ = markBlocked(ctx, deps.github, issue.Number, "campaign state file", err)
		return "", true, err
	}
	if planned.Campaign.Status == campaignStatusDecisionRequired {
		_ = markBlocked(ctx, deps.github, issue.Number, "campaign decision", fmt.Errorf("decision required"))
	}
	return fmt.Sprintf("planned campaign issue #%d with %d tasks", issue.Number, len(planned.Campaign.Tasks)), true, nil
}

func createCampaignState(ctx context.Context, client githubClient, opts daemonOptions, issue githubIssue) (*stateFile, error) {
	plan := planCampaign(issue)
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
			Body:   campaignChildIssueBody(issue, task),
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
	canContinue := readyCampaignTaskIDs(tasks, map[string]bool{})
	for i := range decisions {
		decisions[i].CanContinueTasks = canContinue
		decisions[i].BlockedTasks = taskIDsBlockedByDecision(tasks, canContinue)
		if err := client.Comment(ctx, issue.Number, decisionRequestBody(decisions[i])); err != nil {
			return nil, err
		}
	}
	status := campaignStatusPlanned
	if len(decisions) > 0 && len(canContinue) == 0 {
		status = campaignStatusDecisionRequired
	}
	state := &stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        opts.target,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Daemon:        stateDaemon{Service: "run-weaver.service"},
		Job:           nil,
		Campaign: &stateCampaign{
			Issue: issueRef{
				Number:     issue.Number,
				Title:      issue.Title,
				URL:        issue.URL,
				Repository: opts.repo,
			},
			Status:    status,
			Tasks:     tasks,
			Decisions: decisions,
		},
	}
	return state, nil
}

func campaignChildIssueBody(parent githubIssue, task campaignTask) string {
	return fmt.Sprintf(`# Campaign Task

Parent campaign: #%d
Task ID: %s

%s
`, parent.Number, task.ID, emptyAsNone(strings.TrimSpace(task.Body)))
}

func decisionRequestBody(decision campaignDecision) string {
	return fmt.Sprintf(`<!-- run-weaver-decision-request:%s -->
run-weaver decision request.

## options
%s

## recommendation
%s

## blocked tasks
%s

## can continue tasks
%s

To answer, add a comment containing:

run-weaver-decision:%s:<option>
`, decision.ID, markdownList(decision.Options), emptyAsNone(decision.Recommendation), markdownList(decision.BlockedTasks), markdownList(decision.CanContinueTasks), decision.ID)
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

func dispatchCampaignTask(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (string, bool, error) {
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
	claim, err := claimIssue(ctx, deps.github, task.IssueNumber, opts.target)
	if err != nil {
		return "", true, err
	}
	if claim.Outcome != claimWon {
		return fmt.Sprintf("campaign task %s claim %s", task.ID, claim.Outcome), true, nil
	}
	spec, err := deps.worktree.PrepareForRepo(ctx, opts.target, opts.repo, claim.Issue, opts.repoURL)
	if err != nil {
		state.Campaign.Tasks[taskIndex].Status = campaignTaskBlocked
		state.Campaign.Tasks[taskIndex].RetryCount++
		_ = writeStateFile(opts.stateFilePath(), *state)
		_ = markBlocked(ctx, deps.github, task.IssueNumber, "campaign worktree", err)
		return "", true, err
	}
	runSpec := buildCodexRunSpecForRepo(opts.target, opts.repo, task.IssueNumber, spec.Path, pipelinePhasePlan)
	if err := opts.prepareCodexRunSpec(&runSpec, spec.Path); err != nil {
		state.Campaign.Tasks[taskIndex].Status = campaignTaskBlocked
		state.Campaign.Tasks[taskIndex].RetryCount++
		_ = writeStateFile(opts.stateFilePath(), *state)
		_ = markBlocked(ctx, deps.github, task.IssueNumber, "campaign doppler", err)
		return "", true, err
	}
	if err := writeCampaignPromptFile(runSpec.PromptPath, state.Campaign.Issue, task, claim.Issue, pipelinePhasePlan); err != nil {
		return "", true, err
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		return "", true, err
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
	answers := decisionAnswersFromComments(issue.Comments)
	updated := false
	for i := range state.Campaign.Decisions {
		decision := &state.Campaign.Decisions[i]
		if decision.Status != "pending" {
			continue
		}
		answer, ok := answers[decision.ID]
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

func decisionAnswersFromComments(comments []githubComment) map[string]string {
	answers := map[string]string{}
	for _, comment := range comments {
		matches := decisionAnswerRE.FindAllStringSubmatch(comment.Body, -1)
		for _, match := range matches {
			if len(match) == 3 {
				answers[match[1]] = match[2]
			}
		}
	}
	return answers
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
		markCampaignTaskRetry(state.Campaign, task.ID)
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeStateFile(opts.stateFilePath(), *state)
		return true, "", err
	}
	issue := githubIssue{
		Number:   state.Job.Issue.Number,
		Title:    state.Job.Issue.Title,
		URL:      state.Job.Issue.URL,
		Body:     task.Body,
		Comments: nil,
	}
	if err := writeCampaignPromptFile(runSpec.PromptPath, state.Campaign.Issue, task, issue, nextPhase); err != nil {
		return true, "", err
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		markCampaignTaskRetry(state.Campaign, task.ID)
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeStateFile(opts.stateFilePath(), *state)
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
