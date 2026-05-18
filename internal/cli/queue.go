package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	readyQueueStateReady             = "ready"
	readyQueueStateWaitingDependency = "waiting_dependency"
	readyQueueStateBlockedDependency = "blocked_dependency"
)

type readyIssueCandidate struct {
	Issue        githubIssue
	Dependencies []issueDependency
	BaseBranch   string
	QueueItem    readyQueueItem
}

type readyQueueItem struct {
	Issue        issueRef `json:"issue"`
	State        string   `json:"state"`
	Dependencies []int    `json:"dependencies,omitempty"`
}

func selectReadyIssue(ctx context.Context, client githubClient, state *stateFile, issues []githubIssue) (*readyIssueCandidate, []readyQueueItem, error) {
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Number < issues[j].Number
	})
	queue := make([]readyQueueItem, 0, len(issues))
	for _, listed := range issues {
		issue, err := client.ViewIssue(ctx, listed.Number)
		if err != nil {
			return nil, queue, err
		}
		dependencyNumbers, ambiguous := detectIssueDependencies(issue)
		item := readyQueueItem{
			Issue: issueRef{
				Number: issue.Number,
				Title:  issue.Title,
				URL:    issue.URL,
			},
			State:        readyQueueStateReady,
			Dependencies: dependencyNumbers,
		}
		if ambiguous {
			item.State = readyQueueStateBlockedDependency
			queue = append(queue, item)
			if err := markBlocked(ctx, client, issue.Number, "dependency", fmt.Errorf("dependency reference is ambiguous")); err != nil {
				return nil, queue, err
			}
			continue
		}
		if len(dependencyNumbers) == 0 {
			queue = append(queue, item)
			return &readyIssueCandidate{Issue: issue, QueueItem: item}, queue, nil
		}
		dependencies, blocked, waiting, err := resolveIssueDependencies(ctx, client, state, dependencyNumbers)
		if err != nil {
			return nil, queue, err
		}
		if waiting {
			item.State = readyQueueStateWaitingDependency
			queue = append(queue, item)
			continue
		}
		if blocked != "" {
			item.State = readyQueueStateBlockedDependency
			queue = append(queue, item)
			if err := markBlocked(ctx, client, issue.Number, "dependency", fmt.Errorf("%s", blocked)); err != nil {
				return nil, queue, err
			}
			continue
		}
		queue = append(queue, item)
		return &readyIssueCandidate{
			Issue:        issue,
			Dependencies: dependencies,
			BaseBranch:   dependencies[len(dependencies)-1].Branch,
			QueueItem:    item,
		}, queue, nil
	}
	return nil, queue, nil
}

func detectIssueDependencies(issue githubIssue) ([]int, bool) {
	text := issue.Title + "\n" + issue.Body
	for _, comment := range issue.Comments {
		body := strings.TrimSpace(comment.Body)
		if body == "" || isRunWeaverComment(body) {
			continue
		}
		text += "\n" + body
	}
	return extractDependencyNumbers(text)
}

var dependencyRegexps = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdepends\s*:?\s*#([0-9]+)`),
	regexp.MustCompile(`(?i)\bdepends\s+on\s+#([0-9]+)`),
	regexp.MustCompile(`(?i)\bblocked\s+by\s+#([0-9]+)`),
	regexp.MustCompile(`(?i)\bafter\s+#([0-9]+)`),
	regexp.MustCompile(`(?i)\bstacked\s+on\s+#([0-9]+)`),
	regexp.MustCompile(`依存\s*[:：]?\s*#([0-9]+)`),
	regexp.MustCompile(`#([0-9]+)\s*の後`),
}

var ambiguousDependencyRegexps = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdepends\b`),
	regexp.MustCompile(`(?i)\bblocked\s+by\b`),
	regexp.MustCompile(`(?i)\bstacked\s+on\b`),
	regexp.MustCompile(`依存`),
}

func extractDependencyNumbers(text string) ([]int, bool) {
	seen := map[int]bool{}
	var matches []dependencyMatch
	for _, pattern := range dependencyRegexps {
		for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 2 {
				continue
			}
			number, err := strconv.Atoi(text[match[2]:match[3]])
			if err != nil || seen[number] {
				continue
			}
			seen[number] = true
			matches = append(matches, dependencyMatch{index: match[0], number: number})
		}
	}
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].index < matches[j].index
		})
		numbers := make([]int, 0, len(matches))
		for _, match := range matches {
			numbers = append(numbers, match.number)
		}
		return numbers, false
	}
	for _, pattern := range ambiguousDependencyRegexps {
		if pattern.MatchString(text) {
			return nil, true
		}
	}
	return nil, false
}

type dependencyMatch struct {
	index  int
	number int
}

func resolveIssueDependencies(ctx context.Context, client githubClient, state *stateFile, numbers []int) ([]issueDependency, string, bool, error) {
	dependencies := make([]issueDependency, 0, len(numbers))
	for _, number := range numbers {
		issue, err := client.ViewIssue(ctx, number)
		if err != nil {
			return nil, "", false, err
		}
		if !hasLabel(issue, doneLabel) {
			return nil, "", true, nil
		}
		completed := completedIssueForNumber(state, number)
		if completed == nil {
			parsed := completedIssueFromComments(issue)
			completed = parsed
		}
		if completed == nil || completed.Branch == "" || completed.PRURL == "" {
			return nil, fmt.Sprintf("dependency #%d is done, but its draft PR branch or URL could not be resolved", number), false, nil
		}
		dependencies = append(dependencies, issueDependency{
			IssueNumber: number,
			Branch:      completed.Branch,
			PRURL:       completed.PRURL,
		})
	}
	return dependencies, "", false, nil
}

func completedIssueForNumber(state *stateFile, number int) *completedIssue {
	if state == nil {
		return nil
	}
	for _, completed := range state.CompletedIssues {
		if completed.Issue.Number == number {
			copy := completed
			return &copy
		}
	}
	return nil
}

var completedPRRe = regexp.MustCompile(`(?im)^\s*-\s*draft PR:\s*(\S+)`)
var completedBranchRe = regexp.MustCompile(`(?im)^\s*-\s*branch:\s*(\S+)`)

func completedIssueFromComments(issue githubIssue) *completedIssue {
	for i := len(issue.Comments) - 1; i >= 0; i-- {
		body := issue.Comments[i].Body
		if !strings.Contains(body, "run-weaver completed this issue") {
			continue
		}
		prMatch := completedPRRe.FindStringSubmatch(body)
		branchMatch := completedBranchRe.FindStringSubmatch(body)
		if len(prMatch) < 2 || len(branchMatch) < 2 {
			continue
		}
		return &completedIssue{
			Issue:  issueRef{Number: issue.Number, Title: issue.Title, URL: issue.URL},
			PRURL:  prMatch[1],
			Branch: branchMatch[1],
		}
	}
	return nil
}

func isRunWeaverComment(body string) bool {
	return strings.Contains(body, claimMarkerPrefix) ||
		strings.Contains(body, rateLimitCommentMarkerPrefix) ||
		strings.HasPrefix(body, "run-weaver blocked") ||
		strings.Contains(body, "run-weaver completed this issue")
}
