package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

const claimMarkerPrefix = "run-weaver-claim:"

var claimMarkerRE = regexp.MustCompile(`<!--\s*run-weaver-claim:([^ ]+)\s*-->`)

type claimOutcome string

const (
	claimWon     claimOutcome = "won"
	claimLost    claimOutcome = "lost"
	claimSkipped claimOutcome = "skipped"
)

type claimResult struct {
	Outcome claimOutcome
	ClaimID string
	Issue   githubIssue
}

func claimIssue(ctx context.Context, client githubClient, number int, target string) (claimResult, error) {
	return claimIssueWithID(ctx, client, number, target, newClaimID(target, time.Now().UTC()))
}

func claimIssueWithID(ctx context.Context, client githubClient, number int, target, claimID string) (claimResult, error) {
	issue, err := client.ViewIssue(ctx, number)
	if err != nil {
		return claimResult{}, err
	}
	if hasManagedLabel(issue) {
		return claimResult{Outcome: claimSkipped, ClaimID: claimID, Issue: issue}, nil
	}

	if err := ensureManagedLabels(ctx, client); err != nil {
		return claimResult{}, err
	}
	if err := client.AddLabel(ctx, number, runningLabel); err != nil {
		return claimResult{}, err
	}
	_ = client.RemoveLabel(ctx, number, doneLabel)
	_ = client.RemoveLabel(ctx, number, blockedLabel)

	if err := client.Comment(ctx, number, claimCommentBody(claimID, target, issue)); err != nil {
		return claimResult{}, err
	}

	refreshed, err := client.ViewIssue(ctx, number)
	if err != nil {
		return claimResult{}, err
	}
	if latestClaimID(refreshed.Comments) != claimID {
		return claimResult{Outcome: claimLost, ClaimID: claimID, Issue: refreshed}, nil
	}
	return claimResult{Outcome: claimWon, ClaimID: claimID, Issue: refreshed}, nil
}

func ensureManagedLabels(ctx context.Context, client githubClient) error {
	for _, label := range []string{runningLabel, doneLabel, blockedLabel} {
		if err := client.EnsureLabel(ctx, label); err != nil {
			return err
		}
	}
	return nil
}

func claimCommentBody(claimID, target string, issue githubIssue) string {
	return fmt.Sprintf(`<!-- %s%s -->
run-weaver claimed this issue.

- claim ID: %s
- target: %s
- issue: #%d
- started at: %s
`, claimMarkerPrefix, claimID, claimID, target, issue.Number, time.Now().UTC().Format(time.RFC3339))
}

func latestClaimID(comments []githubComment) string {
	for i := len(comments) - 1; i >= 0; i-- {
		matches := claimMarkerRE.FindStringSubmatch(comments[i].Body)
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return ""
}

func newClaimID(target string, now time.Time) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("run-weaver:%s:%s", target, now.Format("20060102T150405Z"))
	}
	return fmt.Sprintf("run-weaver:%s:%s:%s", target, now.Format("20060102T150405Z"), hex.EncodeToString(random))
}
