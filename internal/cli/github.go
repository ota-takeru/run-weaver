package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	readyLabel   = "run-weaver:ready"
	runningLabel = "running"
	doneLabel    = "done"
	blockedLabel = "blocked"
)

var managedLabels = map[string]struct{}{
	runningLabel: {},
	doneLabel:    {},
	blockedLabel: {},
}

type githubClient interface {
	ListReadyIssues(context.Context) ([]githubIssue, error)
	ViewIssue(context.Context, int) (githubIssue, error)
	AddLabel(context.Context, int, string) error
	RemoveLabel(context.Context, int, string) error
	Comment(context.Context, int, string) error
}

type ghClient struct {
	repo string
}

type githubIssue struct {
	Number   int             `json:"number"`
	Title    string          `json:"title"`
	URL      string          `json:"url"`
	Labels   []githubLabel   `json:"labels"`
	Comments []githubComment `json:"comments"`
}

type githubLabel struct {
	Name string `json:"name"`
}

type githubComment struct {
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}

func (c ghClient) ListReadyIssues(ctx context.Context) ([]githubIssue, error) {
	out, err := c.run(ctx, "issue", "list", "--state", "open", "--label", readyLabel, "--json", "number,title,url,labels", "--limit", "100")
	if err != nil {
		return nil, err
	}
	var issues []githubIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse gh issue list: %w", err)
	}
	return filterClaimableIssues(issues), nil
}

func (c ghClient) ViewIssue(ctx context.Context, number int) (githubIssue, error) {
	out, err := c.run(ctx, "issue", "view", strconv.Itoa(number), "--json", "number,title,url,labels,comments")
	if err != nil {
		return githubIssue{}, err
	}
	var issue githubIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return githubIssue{}, fmt.Errorf("parse gh issue view: %w", err)
	}
	return issue, nil
}

func (c ghClient) AddLabel(ctx context.Context, number int, label string) error {
	_, err := c.run(ctx, "issue", "edit", strconv.Itoa(number), "--add-label", label)
	return err
}

func (c ghClient) RemoveLabel(ctx context.Context, number int, label string) error {
	_, err := c.run(ctx, "issue", "edit", strconv.Itoa(number), "--remove-label", label)
	return err
}

func (c ghClient) Comment(ctx context.Context, number int, body string) error {
	_, err := c.run(ctx, "issue", "comment", strconv.Itoa(number), "--body", body)
	return err
}

func (c ghClient) run(ctx context.Context, args ...string) ([]byte, error) {
	fullArgs := make([]string, 0, len(args)+2)
	fullArgs = append(fullArgs, args...)
	if c.repo != "" {
		fullArgs = append(fullArgs, "--repo", c.repo)
	}
	cmd := exec.CommandContext(ctx, "gh", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func filterClaimableIssues(issues []githubIssue) []githubIssue {
	filtered := make([]githubIssue, 0, len(issues))
	for _, issue := range issues {
		if hasManagedLabel(issue) {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

func hasManagedLabel(issue githubIssue) bool {
	for _, label := range issue.Labels {
		if _, ok := managedLabels[label.Name]; ok {
			return true
		}
	}
	return false
}
