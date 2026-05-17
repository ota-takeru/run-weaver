package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanCampaignBuildsTasksAndDecisionGates(t *testing.T) {
	plan := planCampaign(githubIssue{
		Number: 1,
		Title:  "Campaign",
		Body: strings.Join([]string{
			"- Build planner",
			"- Decision gate: choose retry policy",
			"- Add dispatcher depends: task-build-planner",
		}, "\n"),
	})

	if len(plan.Tasks) != 2 {
		t.Fatalf("tasks = %#v, want 2", plan.Tasks)
	}
	if plan.Tasks[0].ID != "task-build-planner" {
		t.Fatalf("task id = %q", plan.Tasks[0].ID)
	}
	if len(plan.Decisions) != 1 || plan.Decisions[0].ID != "decision-decision-gate-choose-retry-policy" {
		t.Fatalf("decisions = %#v", plan.Decisions)
	}
	if len(plan.Tasks[1].Dependencies) != 1 || plan.Tasks[1].Dependencies[0] != "task-build-planner" {
		t.Fatalf("dependencies = %#v", plan.Tasks[1].Dependencies)
	}
	if !strings.Contains(plan.Tasks[0].Body, "Campaign context:") || !strings.Contains(plan.Tasks[0].Body, "Add dispatcher") {
		t.Fatalf("task body = %q, want campaign context", plan.Tasks[0].Body)
	}
}

func TestProcessOneIssuePlansCampaignAndCreatesChildIssues(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	github := newFakeGitHubClient(githubIssue{
		Number: 7,
		Title:  "Campaign",
		Body:   "- Add planner\n- Decision gate: choose API",
		URL:    "https://github.com/example/repo/issues/7",
		Labels: []githubLabel{{Name: readyLabel}, {Name: campaignLabel}},
	})

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "planned campaign issue #7") {
		t.Fatalf("result = %q", result)
	}
	if len(github.createdIssues) != 1 {
		t.Fatalf("created issues = %#v, want 1 child task", github.createdIssues)
	}
	if !strings.Contains(github.createdIssues[0].Body, "Parent campaign: #7") {
		t.Fatalf("child body = %q", github.createdIssues[0].Body)
	}
	if !slicesEqual(github.createdIssues[0].Labels, []string{readyLabel, campaignTaskLabel}) {
		t.Fatalf("child labels = %#v", github.createdIssues[0].Labels)
	}
	state, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Campaign == nil || len(state.Campaign.Tasks) != 1 || len(state.Campaign.Decisions) != 1 {
		t.Fatalf("campaign state = %#v", state.Campaign)
	}
	if !strings.Contains(github.comments[len(github.comments)-1].Body, "## blocked tasks") {
		t.Fatalf("comments = %#v, want decision request", github.comments)
	}
}

func TestProcessOneIssuePlansCampaignPreservesCompletedIssues(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		CompletedIssues: []completedIssue{{
			Issue:  issueRef{Number: 3, Title: "Done"},
			Branch: "codex/issue-3-done",
			PRURL:  "https://github.com/example/repo/pull/3",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{
		Number: 7,
		Title:  "Campaign",
		Body:   "- Add planner",
		URL:    "https://github.com/example/repo/issues/7",
		Labels: []githubLabel{{Name: readyLabel}, {Name: campaignLabel}},
	})

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.CompletedIssues) != 1 || updated.CompletedIssues[0].Issue.Number != 3 {
		t.Fatalf("completed issues = %#v", updated.CompletedIssues)
	}
}

func TestDispatchCampaignTaskStopsWhenPendingDecisionBlocksTask(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{Number: 102, Title: "Blocked task", Labels: []githubLabel{{Name: readyLabel}, {Name: campaignTaskLabel}}})
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanned,
			Tasks: []campaignTask{{
				ID:          "task-blocked",
				Title:       "Blocked task",
				IssueNumber: 102,
				Status:      campaignTaskPending,
				Phase:       pipelinePhasePlan,
			}},
			Decisions: []campaignDecision{{
				ID:               "decision-api",
				Title:            "Choose API",
				Status:           "pending",
				CanContinueTasks: []string{"task-other"},
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if result != "campaign decision_required" {
		t.Fatalf("result = %q", result)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job != nil || updated.Campaign.Status != campaignStatusDecisionRequired {
		t.Fatalf("state = %#v", updated)
	}
	if !github.added[blockedLabel] {
		t.Fatal("campaign parent should be marked blocked")
	}
}

func TestDispatchCampaignTaskStartsReadyChildIssue(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	worktree := filepath.Join(tempDir, "data", "run-weaver", "worktrees", "issue-101")
	github := newFakeGitHubClient(githubIssue{Number: 101, Title: "Add planner", URL: "https://github.com/example/repo/issues/101", Labels: []githubLabel{{Name: readyLabel}}})
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanned,
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskPending,
				Phase:       pipelinePhasePlan,
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "started campaign task task-add-planner issue #101") {
		t.Fatalf("result = %q", result)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job == nil || updated.Job.CampaignTaskID != "task-add-planner" || updated.Job.PipelinePhase != pipelinePhasePlan {
		t.Fatalf("job = %#v", updated.Job)
	}
	if updated.Campaign.CurrentTaskID != "task-add-planner" {
		t.Fatalf("current task = %q", updated.Campaign.CurrentTaskID)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err == nil {
		t.Fatal("fake worktree should not create real git metadata")
	}
}

func TestCompleteCampaignTaskAdvancesPipelinePhase(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	lastMessage := filepath.Join(tempDir, "last-message.txt")
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(tempDir, "worktrees", "issue-101")
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:          issueRef{Number: 101, Title: "Add planner"},
			LabelState:     runningLabel,
			Branch:         "codex/issue-101-add-planner",
			Worktree:       worktree,
			CampaignTaskID: "task-add-planner",
			PipelinePhase:  pipelinePhasePlan,
			Codex:          &codexState{LastMessagePath: lastMessage},
		},
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusRunning,
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				Body:        "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskRunning,
				Phase:       pipelinePhasePlan,
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{failFirstHasSession: true}
	github := newFakeGitHubClient(githubIssue{Number: 101, Title: "Add planner"})

	result, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected completed campaign task")
	}
	if !strings.Contains(result, "advanced campaign task task-add-planner to implement") {
		t.Fatalf("result = %q", result)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job == nil || updated.Job.PipelinePhase != pipelinePhaseImplement {
		t.Fatalf("job = %#v", updated.Job)
	}
	if updated.Campaign.Tasks[0].Phase != pipelinePhaseImplement {
		t.Fatalf("campaign task = %#v", updated.Campaign.Tasks[0])
	}
}

func TestCompleteCampaignTaskRecordsPRAndClearsCurrentJobAfterVerify(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := filepath.Join(tempDir, "last-message.txt")
	if err := os.WriteFile(lastMessage, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(tempDir, "worktrees", "issue-101")
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Job: &stateJob{
			Issue:          issueRef{Number: 101, Title: "Add planner"},
			LabelState:     runningLabel,
			Branch:         "codex/issue-101-add-planner",
			Worktree:       worktree,
			CampaignTaskID: "task-add-planner",
			PipelinePhase:  pipelinePhaseVerify,
			Codex:          &codexState{LastMessagePath: lastMessage},
		},
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusRunning,
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskRunning,
				Phase:       pipelinePhaseVerify,
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{outputs: map[string][]byte{
		"git\x00-C\x00" + worktree + "\x00status\x00--porcelain": []byte(" M main.go\n"),
	}}
	github := newFakeGitHubClient(githubIssue{Number: 101, Title: "Add planner"})

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected completed campaign task")
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job != nil {
		t.Fatalf("job = %#v, want cleared for next campaign dispatch", updated.Job)
	}
	if len(updated.Campaign.PRs) != 1 || updated.Campaign.PRs[0].TaskID != "task-add-planner" {
		t.Fatalf("campaign PRs = %#v", updated.Campaign.PRs)
	}
	if len(updated.Campaign.CompletedTasks) != 1 || updated.Campaign.Tasks[0].Status != campaignTaskCompleted {
		t.Fatalf("campaign = %#v", updated.Campaign)
	}
	if !strings.Contains(github.comments[len(github.comments)-1].Body, "completed this campaign") {
		t.Fatalf("comments = %#v, want campaign completion comment", github.comments)
	}
}

func TestDecisionAnswerCommentResumesCampaign(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{
		Number: 7,
		Title:  "Campaign",
		Comments: []githubComment{{
			Body: "run-weaver-decision:decision-choose-api:approve",
		}},
	})
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusDecisionRequired,
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskCompleted,
			}},
			Decisions: []campaignDecision{{
				ID:     "decision-choose-api",
				Title:  "Choose API",
				Status: "pending",
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "completed campaign issue #7") {
		t.Fatalf("result = %q", result)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Campaign.Status != campaignStatusDone || updated.Campaign.Decisions[0].Answer != "approve" {
		t.Fatalf("campaign = %#v", updated.Campaign)
	}
}
