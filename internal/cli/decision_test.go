package cli

import (
	"context"
	"strings"
	"testing"
)

func TestValidateCampaignDecisionAnswerFindsRepoState(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	t.Setenv("XDG_CONFIG_HOME", tempDir+"/config")
	if err := addRepoConfigEntry(defaultRepoConfigFile("wsl"), repoEntry{
		Repository:  "example/repo",
		RepoURL:     "https://github.com/example/repo.git",
		Enabled:     true,
		DopplerMode: dopplerModeAuto,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeStateFile(stateFileForRepo("wsl", "example/repo"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign", Repository: "example/repo"},
			Status: campaignStatusDecisionRequired,
			Decisions: []campaignDecision{{
				ID:      "decision-scope",
				Title:   "Choose scope",
				Status:  "pending",
				Options: []string{"approve", "revise", "stop"},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	repo, err := validateCampaignDecisionAnswer(decisionAnswerRequest{
		Target:      "wsl",
		IssueNumber: 7,
		DecisionID:  "decision-scope",
		Option:      "approve",
	})

	if err != nil {
		t.Fatal(err)
	}
	if repo != "example/repo" {
		t.Fatalf("repo = %q, want example/repo", repo)
	}
}

func TestValidateCampaignDecisionAnswerRejectsInvalidOption(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tempDir+"/state")
	if err := writeStateFile(defaultStateFile("wsl"), stateFile{
		SchemaVersion: stateSchemaVersion,
		Target:        "wsl",
		Campaign: &stateCampaign{
			Issue:  issueRef{Number: 7, Title: "Campaign"},
			Status: campaignStatusDecisionRequired,
			Decisions: []campaignDecision{{
				ID:      "decision-scope",
				Title:   "Choose scope",
				Status:  "pending",
				Options: []string{"approve", "revise", "stop"},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := validateCampaignDecisionAnswer(decisionAnswerRequest{
		Target:      "wsl",
		IssueNumber: 7,
		DecisionID:  "decision-scope",
		Option:      "bogus",
	})

	if err == nil || !strings.Contains(err.Error(), "invalid option") {
		t.Fatalf("err = %v, want invalid option", err)
	}
}

func TestPostCampaignDecisionAnswerUsesMarker(t *testing.T) {
	github := newFakeGitHubClient(githubIssue{Number: 7, Title: "Campaign"})

	err := postCampaignDecisionAnswer(context.Background(), github, decisionAnswerRequest{
		IssueNumber: 7,
		DecisionID:  "decision-scope",
		Option:      "approve",
	})

	if err != nil {
		t.Fatal(err)
	}
	if len(github.comments) != 1 || !strings.Contains(github.comments[0].Body, "run-weaver-decision:decision-scope:approve") {
		t.Fatalf("comments = %#v, want decision marker", github.comments)
	}
}

func TestParseCampaignIssueNumber(t *testing.T) {
	for _, input := range []string{"7", "#7", "https://github.com/example/repo/issues/7"} {
		got, err := parseCampaignIssueNumber(input)
		if err != nil {
			t.Fatal(err)
		}
		if got != 7 {
			t.Fatalf("parseCampaignIssueNumber(%q) = %d, want 7", input, got)
		}
	}
}
