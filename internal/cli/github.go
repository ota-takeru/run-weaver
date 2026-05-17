package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	readyLabel        = "run-weaver:ready"
	campaignLabel     = "run-weaver:campaign"
	campaignTaskLabel = "run-weaver:campaign-task"
	runningLabel      = "running"
	doneLabel         = "done"
	blockedLabel      = "blocked"
)

var managedLabels = map[string]struct{}{
	runningLabel: {},
	doneLabel:    {},
	blockedLabel: {},
}

type githubClient interface {
	ListReadyIssues(context.Context) ([]githubIssue, error)
	ListCampaignIssues(context.Context) ([]githubIssue, error)
	ViewIssue(context.Context, int) (githubIssue, error)
	EnsureLabel(context.Context, string) error
	AddLabel(context.Context, int, string) error
	RemoveLabel(context.Context, int, string) error
	Comment(context.Context, int, string) error
	CreateIssue(context.Context, issueCreateSpec) (githubIssue, error)
	CreateDraftPR(context.Context, draftPRSpec) (string, error)
}

type ghClient struct {
	repo string
}

type githubIssue struct {
	Number   int             `json:"number"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
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

type draftPRSpec struct {
	Title string
	Body  string
	Head  string
	Base  string
	Issue int
}

type issueCreateSpec struct {
	Title  string
	Body   string
	Labels []string
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

func (c ghClient) ListCampaignIssues(ctx context.Context) ([]githubIssue, error) {
	out, err := c.run(ctx, "issue", "list", "--state", "open", "--label", readyLabel, "--label", campaignLabel, "--json", "number,title,body,url,labels", "--limit", "100")
	if err != nil {
		return nil, err
	}
	var issues []githubIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse gh campaign issue list: %w", err)
	}
	return filterClaimableCampaignIssues(issues), nil
}

func (c ghClient) ViewIssue(ctx context.Context, number int) (githubIssue, error) {
	out, err := c.run(ctx, "issue", "view", strconv.Itoa(number), "--json", "number,title,body,url,labels,comments")
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

func (c ghClient) EnsureLabel(ctx context.Context, label string) error {
	color := "ededed"
	description := "run-weaver managed state label"
	switch label {
	case readyLabel:
		color = "1d76db"
		description = "run-weaver task intake label"
	case campaignLabel:
		color = "5319e7"
		description = "run-weaver campaign intake label"
	case campaignTaskLabel:
		color = "c5def5"
		description = "run-weaver campaign child task label"
	case runningLabel:
		color = "fbca04"
	case doneLabel:
		color = "0e8a16"
	case blockedLabel:
		color = "b60205"
	}
	_, err := c.run(ctx, "label", "create", label, "--color", color, "--description", description, "--force")
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

func (c ghClient) CreateIssue(ctx context.Context, spec issueCreateSpec) (githubIssue, error) {
	args := []string{"issue", "create", "--title", spec.Title, "--body", spec.Body}
	for _, label := range spec.Labels {
		args = append(args, "--label", label)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return githubIssue{}, err
	}
	url := strings.TrimSpace(string(out))
	number := issueNumberFromURL(url)
	if number == 0 {
		return githubIssue{}, fmt.Errorf("created issue URL did not include an issue number: %s", url)
	}
	return githubIssue{Number: number, Title: spec.Title, Body: spec.Body, URL: url, Labels: labelsFromNames(spec.Labels)}, nil
}

func (c ghClient) CreateDraftPR(ctx context.Context, spec draftPRSpec) (string, error) {
	args := []string{"pr", "create", "--draft", "--title", spec.Title, "--body", spec.Body, "--head", spec.Head}
	if spec.Base != "" {
		args = append(args, "--base", spec.Base)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func buildDraftPRSpec(issue githubIssue, branch string) draftPRSpec {
	return draftPRSpec{
		Title: "Draft: " + issue.Title,
		Body:  fmt.Sprintf("Closes #%d\n", issue.Number),
		Head:  branch,
		Issue: issue.Number,
	}
}

func buildStackedDraftPRSpec(issue githubIssue, branch string, dependencies []issueDependency) draftPRSpec {
	spec := buildDraftPRSpec(issue, branch)
	if len(dependencies) == 0 {
		return spec
	}
	base := dependencies[len(dependencies)-1]
	spec.Base = base.Branch
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Closes #%d\n", issue.Number))
	for _, dependency := range dependencies {
		body.WriteString(fmt.Sprintf("Depends on #%d", dependency.IssueNumber))
		if dependency.PRURL != "" {
			body.WriteString(": ")
			body.WriteString(dependency.PRURL)
		}
		body.WriteString("\n")
	}
	spec.Body = body.String()
	return spec
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
		if hasLabel(issue, campaignLabel) {
			continue
		}
		if hasLabel(issue, campaignTaskLabel) {
			continue
		}
		filtered = append(filtered, issue)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Number < filtered[j].Number
	})
	return filtered
}

func filterClaimableCampaignIssues(issues []githubIssue) []githubIssue {
	filtered := make([]githubIssue, 0, len(issues))
	for _, issue := range issues {
		if hasManagedLabel(issue) {
			continue
		}
		if !hasLabel(issue, campaignLabel) {
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

func hasLabel(issue githubIssue, name string) bool {
	for _, label := range issue.Labels {
		if label.Name == name {
			return true
		}
	}
	return false
}

func issueNumberFromURL(url string) int {
	parts := strings.Split(strings.TrimSpace(url), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(parts[i]); err == nil {
			return n
		}
	}
	return 0
}

func labelsFromNames(names []string) []githubLabel {
	labels := make([]githubLabel, 0, len(names))
	for _, name := range names {
		labels = append(labels, githubLabel{Name: name})
	}
	return labels
}
