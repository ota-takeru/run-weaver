package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type decisionAnswerRequest struct {
	Target      string
	Repository  string
	IssueNumber int
	DecisionID  string
	Option      string
}

func runDecision(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printDecisionUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "answer":
		return runDecisionAnswer(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printDecisionUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown decision command: %s\n\n", args[0])
		printDecisionUsage(stderr)
		return exitUsage
	}
}

func runDecisionAnswer(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("decision answer", stderr)
	target := fs.String("target", defaultStatusTarget(), "target environment: wsl or windows")
	repo := fs.String("repo", "", "GitHub repository, in owner/repo form")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 3 {
		fmt.Fprintln(stderr, "decision answer requires <campaign-issue> <decision-id> <option>")
		printDecisionUsage(stderr)
		return exitUsage
	}
	if !validateTarget(*target, true, stderr) {
		return exitUsage
	}
	issueNumber, err := parseCampaignIssueNumber(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "decision answer error: %v\n", err)
		return exitUsage
	}
	req := decisionAnswerRequest{
		Target:      *target,
		Repository:  strings.TrimSpace(*repo),
		IssueNumber: issueNumber,
		DecisionID:  strings.TrimSpace(fs.Arg(1)),
		Option:      strings.TrimSpace(fs.Arg(2)),
	}
	if req.DecisionID == "" || req.Option == "" {
		fmt.Fprintln(stderr, "decision answer requires non-empty decision-id and option")
		return exitUsage
	}
	resolvedRepo, err := validateCampaignDecisionAnswer(req)
	if err != nil {
		fmt.Fprintf(stderr, "decision answer error: %v\n", err)
		return exitConfigMissing
	}
	req.Repository = resolvedRepo
	if err := postCampaignDecisionAnswer(context.Background(), ghClient{repo: req.Repository}, req); err != nil {
		fmt.Fprintf(stderr, "decision answer error: %v\n", err)
		return exitConfigMissing
	}
	fmt.Fprintf(stdout, "answered campaign #%d decision %s with %s\n", req.IssueNumber, req.DecisionID, req.Option)
	return exitOK
}

func validateCampaignDecisionAnswer(req decisionAnswerRequest) (string, error) {
	state, resolvedRepo, err := findCampaignState(req.Target, req.Repository, req.IssueNumber)
	if err != nil {
		return "", err
	}
	if state.Campaign == nil || state.Campaign.Issue.Number != req.IssueNumber {
		return "", fmt.Errorf("campaign #%d was not found in local state", req.IssueNumber)
	}
	decision := campaignDecisionByID(state.Campaign.Decisions, req.DecisionID)
	if decision == nil {
		return "", fmt.Errorf("decision %q was not found in campaign #%d", req.DecisionID, req.IssueNumber)
	}
	if decision.Status != "pending" {
		return "", fmt.Errorf("decision %q is already %s", req.DecisionID, emptyAsUnknown(decision.Status))
	}
	if !campaignDecisionOptionValid(*decision, req.Option) {
		return "", fmt.Errorf("invalid option %q for decision %q; expected one of: %s", req.Option, req.DecisionID, strings.Join(decision.Options, ", "))
	}
	if resolvedRepo == "" {
		resolvedRepo = state.Campaign.Issue.Repository
	}
	if resolvedRepo == "" {
		resolvedRepo = inferGitHubRepoFromIssueURL(state.Campaign.Issue.URL)
	}
	return resolvedRepo, nil
}

func findCampaignState(target, repository string, issueNumber int) (*stateFile, string, error) {
	if strings.TrimSpace(repository) != "" {
		state, err := readStateFile(stateFileForRepo(target, repository))
		if err != nil {
			return nil, "", err
		}
		if state.Campaign != nil && state.Campaign.Issue.Number == issueNumber {
			return state, repository, nil
		}
		return nil, "", fmt.Errorf("campaign #%d was not found for repository %s", issueNumber, repository)
	}
	if state, err := readStateFile(defaultStateFile(target)); err == nil {
		if state.Campaign != nil && state.Campaign.Issue.Number == issueNumber {
			return state, inferRepoFromState(state), nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	config, err := readRepoConfig(defaultRepoConfigFile(target))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("campaign #%d was not found in local state", issueNumber)
		}
		return nil, "", err
	}
	var matchedState *stateFile
	var matchedRepo string
	for _, entry := range enabledRepoEntries(config) {
		state, err := readStateFile(stateFileForRepo(target, entry.Repository))
		if err != nil {
			continue
		}
		if state.Campaign != nil && state.Campaign.Issue.Number == issueNumber {
			if matchedState != nil {
				return nil, "", fmt.Errorf("campaign #%d exists in multiple repository states; pass --repo", issueNumber)
			}
			matchedState = state
			matchedRepo = entry.Repository
		}
	}
	if matchedState == nil {
		return nil, "", fmt.Errorf("campaign #%d was not found in local state", issueNumber)
	}
	return matchedState, matchedRepo, nil
}

func postCampaignDecisionAnswer(ctx context.Context, client githubClient, req decisionAnswerRequest) error {
	return client.Comment(ctx, req.IssueNumber, campaignDecisionAnswerBody(req.DecisionID, req.Option))
}

func campaignDecisionAnswerBody(decisionID, option string) string {
	return fmt.Sprintf(`run-weaver decision answer.

- decision: %s
- option: %s

%s
`, decisionID, option, campaignDecisionAnswerMarker(decisionID, option))
}

func campaignDecisionAnswerMarker(decisionID, option string) string {
	return fmt.Sprintf("run-weaver-decision:%s:%s", decisionID, option)
}

func parseCampaignIssueNumber(value string) (int, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "#")
	if strings.Contains(value, "/issues/") {
		parts := strings.Split(value, "/issues/")
		value = parts[len(parts)-1]
		if idx := strings.Index(value, "/"); idx >= 0 {
			value = value[:idx]
		}
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid campaign issue %q", value)
	}
	return number, nil
}

func printDecisionUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: run-weaver decision answer [--repo owner/repo] <campaign-issue> <decision-id> <option>")
}
