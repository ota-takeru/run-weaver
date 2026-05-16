package cli

import (
	"context"
	"strings"
	"testing"
)

func TestFilterClaimableIssues(t *testing.T) {
	issues := []githubIssue{
		{Number: 1, Labels: []githubLabel{{Name: readyLabel}}},
		{Number: 2, Labels: []githubLabel{{Name: readyLabel}, {Name: runningLabel}}},
		{Number: 3, Labels: []githubLabel{{Name: readyLabel}, {Name: blockedLabel}}},
	}

	got := filterClaimableIssues(issues)

	if len(got) != 1 || got[0].Number != 1 {
		t.Fatalf("filtered = %#v, want only issue 1", got)
	}
}

func TestClaimIssueWinsWhenLatestClaimIsOwnComment(t *testing.T) {
	client := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Test issue",
		Labels: []githubLabel{{Name: readyLabel}},
	})

	result, err := claimIssueWithID(context.Background(), client, 42, "wsl", "run-weaver:wsl:test")

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != claimWon {
		t.Fatalf("outcome = %s, want %s", result.Outcome, claimWon)
	}
	if !client.added[runningLabel] {
		t.Fatal("running label was not added")
	}
	for _, label := range []string{runningLabel, doneLabel, blockedLabel} {
		if !client.ensured[label] {
			t.Fatalf("%s label was not ensured", label)
		}
	}
	if len(client.comments) != 1 || !strings.Contains(client.comments[0].Body, "run-weaver-claim:run-weaver:wsl:test") {
		t.Fatalf("comments = %#v, want claim comment", client.comments)
	}
}

func TestClaimIssueLosesWhenAnotherClaimIsLatest(t *testing.T) {
	client := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Test issue",
		Labels: []githubLabel{{Name: readyLabel}},
	})
	client.afterComment = func() {
		client.comments = append(client.comments, githubComment{
			Body: "<!-- run-weaver-claim:run-weaver:windows:other -->",
		})
	}

	result, err := claimIssueWithID(context.Background(), client, 42, "wsl", "run-weaver:wsl:test")

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != claimLost {
		t.Fatalf("outcome = %s, want %s", result.Outcome, claimLost)
	}
}

func TestClaimIssueSkipsManagedLabel(t *testing.T) {
	client := newFakeGitHubClient(githubIssue{
		Number: 42,
		Title:  "Test issue",
		Labels: []githubLabel{{Name: readyLabel}, {Name: runningLabel}},
	})

	result, err := claimIssueWithID(context.Background(), client, 42, "wsl", "run-weaver:wsl:test")

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != claimSkipped {
		t.Fatalf("outcome = %s, want %s", result.Outcome, claimSkipped)
	}
	if client.added[runningLabel] {
		t.Fatal("running label should not be added for skipped issue")
	}
}

type fakeGitHubClient struct {
	issue        githubIssue
	added        map[string]bool
	removed      map[string]bool
	comments     []githubComment
	afterComment func()
	draftPR      draftPRSpec
	ensured      map[string]bool
}

func newFakeGitHubClient(issue githubIssue) *fakeGitHubClient {
	return &fakeGitHubClient{
		issue:    issue,
		added:    map[string]bool{},
		removed:  map[string]bool{},
		ensured:  map[string]bool{},
		comments: append([]githubComment(nil), issue.Comments...),
	}
}

func (c *fakeGitHubClient) ListReadyIssues(context.Context) ([]githubIssue, error) {
	return filterClaimableIssues([]githubIssue{c.issue}), nil
}

func (c *fakeGitHubClient) ViewIssue(context.Context, int) (githubIssue, error) {
	issue := c.issue
	issue.Comments = append([]githubComment(nil), c.comments...)
	return issue, nil
}

func (c *fakeGitHubClient) AddLabel(_ context.Context, _ int, label string) error {
	c.added[label] = true
	c.issue.Labels = append(c.issue.Labels, githubLabel{Name: label})
	return nil
}

func (c *fakeGitHubClient) EnsureLabel(_ context.Context, label string) error {
	c.ensured[label] = true
	return nil
}

func (c *fakeGitHubClient) RemoveLabel(_ context.Context, _ int, label string) error {
	c.removed[label] = true
	return nil
}

func (c *fakeGitHubClient) Comment(_ context.Context, _ int, body string) error {
	c.comments = append(c.comments, githubComment{Body: body})
	if c.afterComment != nil {
		c.afterComment()
	}
	return nil
}

func (c *fakeGitHubClient) CreateDraftPR(_ context.Context, spec draftPRSpec) (string, error) {
	c.draftPR = spec
	return "https://github.com/example/repo/pull/1", nil
}
