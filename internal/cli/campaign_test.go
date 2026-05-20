package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCampaignPlannerOutputBuildsTasksAndDecisionGates(t *testing.T) {
	plan, err := parseCampaignPlannerOutput([]byte(`{
		"tasks": [
			{"id": "task-build-planner", "title": "Build planner", "body": "Implement planner.", "dependencies": []},
			{"id": "task-add-dispatcher", "title": "Add dispatcher", "body": "Implement dispatcher.", "dependencies": ["task-build-planner"]}
		],
		"decisions": [
			{
				"id": "decision-retry-policy",
				"title": "Choose retry policy",
				"context": "Retry policy changes how failed jobs are resumed.",
				"evidence": ["docs/progress.md lists rate limit resume as implemented."],
				"options": ["approve", "revise", "stop"],
				"optionDetails": ["approve: use the proposed retry policy", "revise: provide a different retry limit", "stop: leave retry policy unchanged"],
				"recommendation": "approve",
				"impact": ["Implementation affects daemon retry behavior."],
				"reversibility": "Can be changed later by updating daemon policy and docs.",
				"blockedTasks": ["task-add-dispatcher"],
				"canContinueTasks": ["task-build-planner"]
			}
		]
	}`))

	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("tasks = %#v, want 2", plan.Tasks)
	}
	if plan.Tasks[0].ID != "task-build-planner" {
		t.Fatalf("task id = %q", plan.Tasks[0].ID)
	}
	if len(plan.Decisions) != 1 || plan.Decisions[0].ID != "decision-retry-policy" {
		t.Fatalf("decisions = %#v", plan.Decisions)
	}
	if len(plan.Tasks[1].Dependencies) != 1 || plan.Tasks[1].Dependencies[0] != "task-build-planner" {
		t.Fatalf("dependencies = %#v", plan.Tasks[1].Dependencies)
	}
	if len(plan.Decisions[0].BlockedTasks) != 1 || plan.Decisions[0].BlockedTasks[0] != "task-add-dispatcher" {
		t.Fatalf("blocked tasks = %#v", plan.Decisions[0].BlockedTasks)
	}
	if plan.Decisions[0].Context == "" || len(plan.Decisions[0].Evidence) != 1 || len(plan.Decisions[0].OptionDetails) != 3 || len(plan.Decisions[0].Impact) != 1 || plan.Decisions[0].Reversibility == "" {
		t.Fatalf("decision report fields = %#v", plan.Decisions[0])
	}
}

func TestParseCampaignPlannerOutputRejectsInvalidPlans(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"invalid json", `{not-json`},
		{"empty tasks", `{"tasks":[]}`},
		{"duplicate task", `{"tasks":[{"id":"task-a","title":"A","body":"A"},{"id":"task-a","title":"B","body":"B"}]}`},
		{"unknown dependency", `{"tasks":[{"id":"task-a","title":"A","body":"A","dependencies":["task-missing"]}]}`},
		{"unknown decision task", `{"tasks":[{"id":"task-a","title":"A","body":"A"}],"decisions":[{"id":"decision-a","title":"A","options":["approve"],"blockedTasks":["task-missing"]}]}`},
		{"decision missing report", `{"tasks":[{"id":"task-a","title":"A","body":"A"}],"decisions":[{"id":"decision-a","title":"A","options":["approve"],"blockedTasks":["task-a"]}]}`},
		{"decision incomplete option details", `{"tasks":[{"id":"task-a","title":"A","body":"A"}],"decisions":[{"id":"decision-a","title":"A","context":"A","evidence":["A"],"options":["approve","stop"],"optionDetails":["approve: A"],"impact":["A"],"reversibility":"A","blockedTasks":["task-a"]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCampaignPlannerOutput([]byte(tc.body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestProcessOneIssueStartsCampaignPlannerOnly(t *testing.T) {
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
	if !strings.Contains(result, "started campaign planner for issue #7") {
		t.Fatalf("result = %q", result)
	}
	if len(github.createdIssues) != 0 {
		t.Fatalf("created issues = %#v, want no child task before planner completes", github.createdIssues)
	}
	state, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Campaign == nil || state.Campaign.Status != campaignStatusPlanning || state.Campaign.Planner == nil {
		t.Fatalf("campaign state = %#v", state.Campaign)
	}
	if state.Campaign.Tasks != nil {
		t.Fatalf("campaign tasks = %#v, want nil until planner completes", state.Campaign.Tasks)
	}
	promptPath := strings.TrimSuffix(state.Campaign.Planner.Codex.LastMessagePath, "last-message.txt") + "campaign-planner-prompt.md"
	if !strings.Contains(readFileString(t, promptPath), "Return only valid JSON") {
		t.Fatalf("planner prompt was not written")
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCampaignPlannerCompletionCreatesChildIssues(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	lastMessage := filepath.Join(tempDir, "planner-last-message.json")
	if err := os.WriteFile(lastMessage, []byte(`{
		"tasks": [
			{"id": "task-add-planner", "title": "Add planner", "body": "Implement planner.", "dependencies": []},
			{"id": "task-add-dispatcher", "title": "Add dispatcher", "body": "Implement dispatcher.", "dependencies": ["task-add-planner"]}
		],
		"decisions": [
			{"id": "decision-api", "title": "Choose API", "context": "API choice affects dispatcher implementation.", "evidence": ["Issue asks to choose an API before dispatcher work."], "options": ["approve", "revise", "stop"], "optionDetails": ["approve: use proposed API", "revise: provide API requirements", "stop: do not implement dispatcher"], "recommendation": "approve", "impact": ["Blocks dispatcher implementation."], "reversibility": "Can be changed before dispatcher PR is merged.", "blockedTasks": ["task-add-dispatcher"], "canContinueTasks": ["task-add-planner"]}
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanning,
			Planner: &campaignPlanner{
				LastMessagePath: lastMessage,
				Tmux:            &tmuxRef{Session: "run-weaver", Window: "missing"},
			},
		},
		CompletedIssues: []completedIssue{{
			Issue:  issueRef{Number: 3, Title: "Done"},
			Branch: "codex/issue-3-done",
			PRURL:  "https://github.com/example/repo/pull/3",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 7, Title: "Campaign"})

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "planned campaign issue #7 with 2 tasks") {
		t.Fatalf("result = %q", result)
	}
	if len(github.createdIssues) != 2 {
		t.Fatalf("created issues = %#v, want 2 child tasks", github.createdIssues)
	}
	if !strings.Contains(github.createdIssues[0].Body, "Parent campaign: #7") {
		t.Fatalf("child body = %q", github.createdIssues[0].Body)
	}
	if !slicesEqual(github.createdIssues[0].Labels, []string{readyLabel, campaignTaskLabel}) {
		t.Fatalf("child labels = %#v", github.createdIssues[0].Labels)
	}
	decisionComment := github.comments[len(github.comments)-1].Body
	if !strings.Contains(decisionComment, "## evidence") || !strings.Contains(decisionComment, "## blocked tasks") {
		t.Fatalf("comments = %#v, want decision request", github.comments)
	}
	if !strings.Contains(decisionComment, "run-weaver decision answer") || !strings.Contains(decisionComment, "run-weaver-decision:decision-api:approve") {
		t.Fatalf("decision comment = %q, want CLI command and mobile quick reply", decisionComment)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Campaign == nil || updated.Campaign.Status != campaignStatusPlanned || len(updated.Campaign.Tasks) != 2 || len(updated.Campaign.Decisions) != 1 {
		t.Fatalf("campaign state = %#v", updated.Campaign)
	}
	if len(updated.CompletedIssues) != 1 || updated.CompletedIssues[0].Issue.Number != 3 {
		t.Fatalf("completed issues = %#v", updated.CompletedIssues)
	}
}

func TestCampaignPlannerInvalidOutputBlocksParent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	lastMessage := filepath.Join(tempDir, "planner-last-message.json")
	if err := os.WriteFile(lastMessage, []byte(`{"tasks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanning,
			Planner: &campaignPlanner{
				LastMessagePath: lastMessage,
				Tmux:            &tmuxRef{Session: "run-weaver", Window: "missing"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 7, Title: "Campaign"})

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err == nil {
		t.Fatal("expected planner error")
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Planner.LastError == nil {
		t.Fatalf("campaign = %#v", updated.Campaign)
	}
	if !github.added[blockedLabel] {
		t.Fatal("campaign parent should be marked blocked")
	}
}

func TestCampaignPlannerMissingOutputBlocksParent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	restoreCommands := stubCommandEnvironment(t, map[string]error{
		commandKey("tmux", "has-session", "-t", "run-weaver:missing"): errFakeCommand,
	})
	defer restoreCommands()
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanning,
			Planner: &campaignPlanner{
				LastMessagePath: filepath.Join(tempDir, "missing-last-message.json"),
				Tmux:            &tmuxRef{Session: "run-weaver", Window: "missing"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 7, Title: "Campaign"})

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newTmuxRunner(&fakeCommandRunner{}),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err == nil {
		t.Fatal("expected planner error")
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Planner.LastError == nil {
		t.Fatalf("campaign = %#v", updated.Campaign)
	}
	if !strings.Contains(*updated.Campaign.Planner.LastError, "did not produce") {
		t.Fatalf("last error = %q", *updated.Campaign.Planner.LastError)
	}
	if !github.added[blockedLabel] {
		t.Fatal("campaign parent should be marked blocked")
	}
}

func TestCampaignPlannerMissingOutputKeepsPlanningForDirectRunner(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("LOCALAPPDATA", filepath.Join(tempDir, "localappdata"))
	if err := writeStateFile(defaultStateFile("windows"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "windows",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusPlanning,
			Planner: &campaignPlanner{
				LastMessagePath: filepath.Join(tempDir, "missing-last-message.json"),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	github := newFakeGitHubClient(githubIssue{Number: 7, Title: "Campaign"})

	result, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(&fakeCommandRunner{}),
		runner:   newDirectRunner("windows", &fakeCommandRunner{}),
	}, daemonOptions{target: "windows", repoURL: "https://github.com/example/repo.git"})

	if err != nil {
		t.Fatal(err)
	}
	if result != "campaign planning" {
		t.Fatalf("result = %q", result)
	}
	updated, readErr := readStateFile(defaultStateFile("windows"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusPlanning {
		t.Fatalf("campaign = %#v", updated.Campaign)
	}
	if github.added[blockedLabel] {
		t.Fatal("direct runner planner should not be marked blocked while output is still absent")
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

func TestDispatchCampaignTaskStacksOnPreviousCompletedCampaignTask(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	github := newFakeGitHubClient(githubIssue{Number: 102, Title: "Add dispatcher", URL: "https://github.com/example/repo/issues/102", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 101, Title: "Add planner", Labels: []githubLabel{{Name: doneLabel}}},
		{Number: 102, Title: "Add dispatcher", URL: "https://github.com/example/repo/issues/102", Labels: []githubLabel{{Name: readyLabel}}},
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:          issueRef{Number: 7, Title: "Campaign"},
			Status:         campaignStatusPlanned,
			CompletedTasks: []string{"task-add-planner"},
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskCompleted,
				Phase:       pipelinePhaseVerify,
				PRURL:       "https://github.com/example/repo/pull/101",
			}, {
				ID:          "task-add-dispatcher",
				Title:       "Add dispatcher",
				IssueNumber: 102,
				Status:      campaignTaskPending,
				Phase:       pipelinePhasePlan,
			}},
		},
		CompletedIssues: []completedIssue{{
			Issue:  issueRef{Number: 101, Title: "Add planner"},
			Branch: "codex/issue-101-add-planner",
			PRURL:  "https://github.com/example/repo/pull/101",
		}},
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
	if !strings.Contains(result, "started campaign task task-add-dispatcher issue #102") {
		t.Fatalf("result = %q", result)
	}
	clonePath := filepath.Join(tempDir, "data", "run-weaver", "src")
	worktreePath := filepath.Join(tempDir, "data", "run-weaver", "worktrees", "issue-102")
	if !commands.ran("git", "-C", clonePath, "worktree", "add", "-B", "codex/issue-102-add-dispatcher", worktreePath, "origin/codex/issue-101-add-planner") {
		t.Fatalf("commands = %#v, want stacked campaign worktree base", commands.calls)
	}
	updated, err := readStateFile(defaultStateFile("wsl"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Job == nil || updated.Job.BaseBranch != "codex/issue-101-add-planner" || len(updated.Job.Dependencies) != 1 {
		t.Fatalf("job = %#v, want campaign stack dependency", updated.Job)
	}
	if updated.Job.Dependencies[0].IssueNumber != 101 || updated.Job.Dependencies[0].PRURL != "https://github.com/example/repo/pull/101" {
		t.Fatalf("dependencies = %#v", updated.Job.Dependencies)
	}
}

func TestCampaignStackDependenciesFallbackToCompletedComment(t *testing.T) {
	state := &stateFile{
		Campaign: &stateCampaign{
			CompletedTasks: []string{"task-add-planner"},
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskCompleted,
			}},
		},
	}
	github := newFakeGitHubClient(githubIssue{
		Number: 101,
		Title:  "Add planner",
		Comments: []githubComment{{
			Body: "run-weaver completed this issue.\n\n- draft PR: https://github.com/example/repo/pull/101\n- branch: codex/issue-101-add-planner\n",
		}},
	})

	dependencies, baseBranch, err := campaignStackDependencies(context.Background(), github, state)

	if err != nil {
		t.Fatal(err)
	}
	if baseBranch != "codex/issue-101-add-planner" || len(dependencies) != 1 {
		t.Fatalf("base = %q dependencies = %#v", baseBranch, dependencies)
	}
}

func TestDispatchCampaignTaskBlocksWhenPreviousStackBranchMissing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
	github := newFakeGitHubClient(githubIssue{Number: 102, Title: "Add dispatcher", URL: "https://github.com/example/repo/issues/102", Labels: []githubLabel{{Name: readyLabel}}})
	github.issues = []githubIssue{
		{Number: 101, Title: "Add planner", Labels: []githubLabel{{Name: doneLabel}}},
		{Number: 102, Title: "Add dispatcher", URL: "https://github.com/example/repo/issues/102", Labels: []githubLabel{{Name: readyLabel}}},
	}
	state := stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:          issueRef{Number: 7, Title: "Campaign"},
			Status:         campaignStatusPlanned,
			CompletedTasks: []string{"task-add-planner"},
			Tasks: []campaignTask{{
				ID:          "task-add-planner",
				Title:       "Add planner",
				IssueNumber: 101,
				Status:      campaignTaskCompleted,
			}, {
				ID:          "task-add-dispatcher",
				Title:       "Add dispatcher",
				IssueNumber: 102,
				Status:      campaignTaskPending,
				Phase:       pipelinePhasePlan,
			}},
		},
	}
	if err := writeStateFile(defaultStateFile("wsl"), state); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommandRunner{}

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err == nil || !strings.Contains(err.Error(), "draft PR branch or URL could not be resolved") {
		t.Fatalf("err = %v, want missing stack branch error", err)
	}
	if commands.ranPrefix("git", "clone") || commands.ranPrefix("git", "-C") {
		t.Fatalf("commands = %#v, should not prepare worktree before stack dependency is resolved", commands.calls)
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Tasks[1].Status != campaignTaskBlocked {
		t.Fatalf("campaign = %#v, want blocked missing stack branch", updated.Campaign)
	}
	if !github.added[blockedLabel] || github.added[runningLabel] {
		t.Fatalf("labels added = %#v, want blocked without running claim", github.added)
	}
}

func TestDispatchCampaignTaskBlocksWhenRunnerFails(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
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
	commands := &fakeCommandRunner{failNewWindow: true}

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git"})

	if err == nil {
		t.Fatal("expected runner error")
	}
	if !github.added[blockedLabel] || !github.removed[runningLabel] {
		t.Fatalf("labels added=%#v removed=%#v, want child blocked", github.added, github.removed)
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Tasks[0].Status != campaignTaskBlocked {
		t.Fatalf("campaign = %#v, want blocked task", updated.Campaign)
	}
	if updated.Job == nil || updated.Job.LabelState != blockedLabel || updated.Job.LastError == nil || !strings.Contains(*updated.Job.LastError, "campaign runner") {
		t.Fatalf("job = %#v, want blocked runner error", updated.Job)
	}
}

func TestDispatchCampaignTaskBlocksWhenDopplerCheckFails(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_DATA_HOME", tempDir+"/data")
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
	commands := &fakeCommandRunner{}
	originalLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name == "doppler" {
			return "", errFakeCommand
		}
		return originalLookPath(name)
	}
	defer func() { lookPath = originalLookPath }()

	_, err := processOneIssue(daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl", repoURL: "https://github.com/example/repo.git", dopplerMode: dopplerModeRequired})

	if err == nil {
		t.Fatal("expected doppler error")
	}
	if !github.added[blockedLabel] || !github.removed[runningLabel] {
		t.Fatalf("labels added=%#v removed=%#v, want child blocked", github.added, github.removed)
	}
	if commands.ranPrefix("tmux", "new-window") {
		t.Fatalf("commands = %#v, should not start codex", commands.calls)
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Tasks[0].Status != campaignTaskBlocked {
		t.Fatalf("campaign = %#v, want blocked task", updated.Campaign)
	}
	if updated.Job == nil || updated.Job.LabelState != blockedLabel || updated.Job.LastError == nil || !strings.Contains(*updated.Job.LastError, "campaign doppler") {
		t.Fatalf("job = %#v, want blocked doppler error", updated.Job)
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

func TestCompleteCampaignTaskBlocksWhenNextPhaseRunnerFails(t *testing.T) {
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
	commands := &fakeCommandRunner{failNewWindow: true}
	github := newFakeGitHubClient(githubIssue{Number: 101, Title: "Add planner"})

	_, completed, err := completeCurrentJob(context.Background(), daemonDeps{
		github:   github,
		worktree: newWorktreeManager(commands),
		runner:   newTmuxRunner(commands),
	}, daemonOptions{target: "wsl"})

	if err == nil {
		t.Fatal("expected runner error")
	}
	if !completed {
		t.Fatal("campaign phase failure should be handled in this cycle")
	}
	if !github.added[blockedLabel] || !github.removed[runningLabel] {
		t.Fatalf("labels added=%#v removed=%#v, want child blocked", github.added, github.removed)
	}
	updated, readErr := readStateFile(defaultStateFile("wsl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if updated.Campaign.Status != campaignStatusBlocked || updated.Campaign.Tasks[0].Status != campaignTaskBlocked {
		t.Fatalf("campaign = %#v, want blocked task", updated.Campaign)
	}
	if updated.Job == nil || updated.Job.LabelState != blockedLabel || updated.Job.LastError == nil || !strings.Contains(*updated.Job.LastError, "campaign runner") {
		t.Fatalf("job = %#v, want blocked runner error", updated.Job)
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
				ID:      "decision-choose-api",
				Title:   "Choose API",
				Status:  "pending",
				Options: []string{"approve", "revise", "stop"},
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

func TestDecisionAnswerCommentIgnoresUnknownOption(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	github := newFakeGitHubClient(githubIssue{
		Number: 7,
		Title:  "Campaign",
		Comments: []githubComment{{
			Body: "run-weaver-decision:decision-choose-api:bogus",
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
				ID:      "decision-choose-api",
				Title:   "Choose API",
				Status:  "pending",
				Options: []string{"approve", "revise", "stop"},
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
	if updated.Campaign.Decisions[0].Status != "pending" || updated.Campaign.Decisions[0].Answer != "" {
		t.Fatalf("decision = %#v, want still pending", updated.Campaign.Decisions[0])
	}
}
