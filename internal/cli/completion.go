package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const rateLimitResumePrompt = `Continue the interrupted work from the previous session. Review the existing worktree, git diff, and prior context before making changes. Preserve previous progress and continue toward the original GitHub Issue goal.`
const conflictResolveMaxAttempts = 1

const rateLimitCommentMarkerPrefix = "run-weaver-rate-limit:"

func completeCurrentJob(ctx context.Context, deps daemonDeps, opts daemonOptions) (string, bool, error) {
	statePath := opts.stateFilePath()
	state, err := readStateFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	if state.Job == nil || state.Job.LabelState != runningLabel || state.Job.Codex == nil {
		return "", false, nil
	}
	if resumed, err := resumeRateLimitedCodex(ctx, deps, opts, state); resumed || err != nil {
		return "resumed rate-limited Codex session", true, err
	}
	if failed, err := blockFailedCodexStartup(ctx, deps, opts, state); failed || err != nil {
		return "blocked failed Codex startup", true, err
	}
	if !codexCompletionReady(opts.target, state.Job) {
		return "", false, nil
	}
	resolvedConflict := false
	if state.Job.PipelinePhase == conflictResolvePhase {
		if err := validateConflictResolution(ctx, deps.worktree, state.Job.Worktree); err != nil {
			blockCompletionJob(ctx, deps, opts, state, "conflict resolution", err)
			return "", true, err
		}
		if state.Job.CampaignTaskID != "" {
			state.Job.PipelinePhase = pipelinePhaseVerify
		} else {
			state.Job.PipelinePhase = ""
		}
		resolvedConflict = true
	}
	if !resolvedConflict {
		if advanced, result, err := advanceCampaignPipeline(ctx, deps, opts, state); advanced || err != nil {
			return result, true, err
		}
	}

	committed, err := deps.worktree.CommitAll(ctx, state.Job.Worktree, fmt.Sprintf("Implement issue #%d", state.Job.Issue.Number))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git commit", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git commit", err)
		return "", true, err
	}
	if !committed {
		err := fmt.Errorf("codex completed without file changes")
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "codex result", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "codex result", err)
		return "", true, err
	}

	if started, err := startConflictResolutionIfNeeded(ctx, deps, opts, state); started {
		return fmt.Sprintf("started conflict resolution for issue #%d", state.Job.Issue.Number), true, err
	} else if err != nil {
		return "", true, err
	}

	if err := deps.worktree.PushBranch(ctx, state.Job.Worktree, state.Job.Branch); err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "git push", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "git push", err)
		return "", true, err
	}

	prURL, err := deps.github.CreateDraftPR(ctx, buildStackedDraftPRSpec(githubIssue{
		Number: state.Job.Issue.Number,
		Title:  state.Job.Issue.Title,
		URL:    state.Job.Issue.URL,
	}, state.Job.Branch, state.Job.Dependencies))
	if err != nil {
		_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "draft PR", err)
		_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "draft PR", err)
		return "", true, err
	}
	if err := markDone(ctx, deps.github, state.Job.Issue.Number, prURL, state.Job.Branch, state.Job.Worktree); err != nil {
		return "", true, err
	}

	completedIssueNumber := state.Job.Issue.Number
	wasCampaignTask := state.Job.CampaignTaskID != ""
	markCampaignTaskCompleted(state, prURL)
	if state.Campaign != nil && state.Campaign.Status == campaignStatusDecisionRequired {
		_ = markBlocked(ctx, deps.github, state.Campaign.Issue.Number, "campaign decision", fmt.Errorf("decision required"))
	}
	if state.Campaign != nil && state.Campaign.Status == campaignStatusDone {
		_ = markCampaignDone(ctx, deps.github, state.Campaign)
	}
	state.Job.LabelState = doneLabel
	state.Job.LastError = nil
	recordCompletedIssue(state, completedIssue{
		Issue:  state.Job.Issue,
		Branch: state.Job.Branch,
		PRURL:  prURL,
	})
	if wasCampaignTask {
		state.Job = nil
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(statePath, *state); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("completed issue #%d with draft PR %s", completedIssueNumber, prURL), true, nil
}

func startConflictResolutionIfNeeded(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (bool, error) {
	if state == nil || state.Job == nil {
		return false, nil
	}
	baseRef := conflictBaseRef(state.Job)
	if err := deps.worktree.FetchOrigin(ctx, state.Job.Worktree); err != nil {
		blockCompletionJob(ctx, deps, opts, state, "git fetch", err)
		return false, err
	}
	if err := deps.worktree.MergeBase(ctx, state.Job.Worktree, baseRef); err == nil {
		return false, nil
	}
	files, err := deps.worktree.UnmergedFiles(ctx, state.Job.Worktree)
	if err != nil {
		blockCompletionJob(ctx, deps, opts, state, "base merge", err)
		return false, err
	}
	if len(files) == 0 {
		err := fmt.Errorf("base merge failed without unmerged files")
		blockCompletionJob(ctx, deps, opts, state, "base merge", err)
		return false, err
	}
	if reason := highRiskConflictReason(files); reason != "" {
		err := fmt.Errorf("high-risk conflict: %s; files: %s", reason, strings.Join(files, ", "))
		blockCompletionJob(ctx, deps, opts, state, "base merge", err)
		return false, err
	}
	if state.Job.ConflictRetryCount >= conflictResolveMaxAttempts {
		err := fmt.Errorf("conflict resolution retry limit reached; files: %s", strings.Join(files, ", "))
		blockCompletionJob(ctx, deps, opts, state, "conflict resolution", err)
		return false, err
	}
	runSpec := buildCodexRunSpecForRepo(opts.target, opts.repo, state.Job.Issue.Number, state.Job.Worktree, conflictResolvePhase)
	if err := opts.prepareCodexRunSpec(&runSpec, state.Job.Worktree); err != nil {
		blockCompletionJob(ctx, deps, opts, state, "conflict resolution", err)
		return false, err
	}
	if err := writeConflictResolvePromptFile(runSpec.PromptPath, state.Job, baseRef, files); err != nil {
		blockCompletionJob(ctx, deps, opts, state, "conflict resolution", err)
		return false, err
	}
	tmux, err := deps.runner.StartCodex(ctx, runSpec)
	if err != nil {
		blockCompletionJob(ctx, deps, opts, state, "conflict resolution", err)
		return false, err
	}
	state.Job.PipelinePhase = conflictResolvePhase
	state.Job.ConflictRetryCount++
	state.Job.Tmux = tmux
	state.Job.Codex = &codexState{
		LastMessagePath: runSpec.LastMessagePath,
		JSONLogPath:     runSpec.JSONLogPath,
	}
	message := fmt.Sprintf("base merge conflicted with %s; conflict resolution attempt %d started for %s", baseRef, state.Job.ConflictRetryCount, strings.Join(files, ", "))
	state.Job.LastError = &message
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		return false, err
	}
	return true, nil
}

func blockCompletionJob(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile, stage string, cause error) {
	if state != nil && state.Job != nil && state.Job.CampaignTaskID != "" {
		blockCampaignTaskPhase(ctx, deps.github, opts, state, state.Job.CampaignTaskID, stage, cause)
		return
	}
	if state == nil || state.Job == nil {
		return
	}
	_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, stage, cause)
	_ = writeBlockedState(opts, githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, stage, cause)
}

func conflictBaseRef(job *stateJob) string {
	if job != nil && job.BaseBranch != "" {
		return "origin/" + job.BaseBranch
	}
	return "origin/HEAD"
}

func highRiskConflictReason(files []string) string {
	for _, file := range files {
		base := strings.ToLower(filepath.Base(file))
		clean := strings.ToLower(filepath.ToSlash(file))
		switch base {
		case "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "composer.lock", "gemfile.lock", "cargo.lock":
			return "lockfile conflict"
		}
		if strings.HasPrefix(clean, ".github/workflows/") {
			return "workflow conflict"
		}
		if strings.Contains(clean, "migration") || strings.Contains(clean, "migrations/") {
			return "migration conflict"
		}
	}
	return ""
}

func validateConflictResolution(ctx context.Context, worktree worktreeManager, path string) error {
	files, err := worktree.UnmergedFiles(ctx, path)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return fmt.Errorf("conflict markers still need resolution in %s", strings.Join(files, ", "))
	}
	changedFiles, err := worktree.ChangedFiles(ctx, path)
	if err != nil {
		return err
	}
	if files, err := filesWithConflictMarkers(path, changedFiles); err != nil {
		return err
	} else if len(files) > 0 {
		return fmt.Errorf("conflict markers remain in %s", strings.Join(files, ", "))
	}
	return worktree.commands.Run(ctx, "git", "-C", path, "diff", "--check")
}

func filesWithConflictMarkers(root string, candidates []string) ([]string, error) {
	var matches []string
	for _, file := range candidates {
		clean := filepath.Clean(file)
		if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || filepath.IsAbs(clean) {
			continue
		}
		path := filepath.Join(root, clean)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		if strings.Contains(text, "<<<<<<<") || strings.Contains(text, ">>>>>>>") {
			matches = append(matches, filepath.ToSlash(clean))
		}
	}
	return matches, nil
}

func writeConflictResolvePromptFile(path string, job *stateJob, baseRef string, files []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# Conflict Resolution

GitHub Issue #%d
Title: %s
URL: %s

Base ref:
%s

Conflicted files:
%s

Resolve only the merge conflicts in the listed files. Preserve both the original issue change and the incoming base change when they are compatible. Do not add unrelated features or broad refactors. Do not edit files that are unrelated to this conflict unless required to make the resolution compile or pass focused checks.

After resolving, run focused verification if practical. Leave a concise final summary with the files resolved and checks run.
`, job.Issue.Number, job.Issue.Title, job.Issue.URL, baseRef, "- "+strings.Join(files, "\n- "))
	return os.WriteFile(path, []byte(body), 0o600)
}

func blockFailedCodexStartup(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (bool, error) {
	if state == nil || state.Job == nil || state.Job.Codex == nil {
		return false, nil
	}
	if tmuxState(opts.target, state.Job.Tmux) == "window_exists" {
		return false, nil
	}
	if state.Job.Codex.LastMessagePath != "" {
		if _, err := os.Stat(state.Job.Codex.LastMessagePath); err == nil {
			return false, nil
		}
	}
	if !codexLogLooksCommandNotFound(state.Job.Codex.JSONLogPath) {
		return false, nil
	}
	err := fmt.Errorf("Codex command failed to start; check the run-weaver service PATH and reinstall or restart the service")
	issue := githubIssue{Number: state.Job.Issue.Number, Title: state.Job.Issue.Title, URL: state.Job.Issue.URL}
	_ = markBlocked(ctx, deps.github, state.Job.Issue.Number, "codex startup", err)
	if writeErr := writeBlockedState(opts, issue, state.Job.ClaimID, worktreeSpec{Path: state.Job.Worktree, Branch: state.Job.Branch}, state.Job.Tmux, "codex startup", err); writeErr != nil {
		return true, writeErr
	}
	return true, nil
}

func recordCompletedIssue(state *stateFile, completed completedIssue) {
	if state == nil || completed.Issue.Number == 0 {
		return
	}
	for i := range state.CompletedIssues {
		if state.CompletedIssues[i].Issue.Number == completed.Issue.Number {
			state.CompletedIssues[i] = completed
			return
		}
	}
	state.CompletedIssues = append(state.CompletedIssues, completed)
}

func resumeRateLimitedCodex(ctx context.Context, deps daemonDeps, opts daemonOptions, state *stateFile) (bool, error) {
	if state == nil || state.Job == nil || state.Job.Codex == nil || state.Job.Codex.JSONLogPath == "" || state.Job.Codex.LastMessagePath == "" {
		return false, nil
	}
	if tmuxState(opts.target, state.Job.Tmux) == "window_exists" {
		return false, nil
	}
	if _, err := os.Stat(state.Job.Codex.LastMessagePath); err == nil {
		return false, nil
	}
	if !codexLogLooksRateLimited(state.Job.Codex.JSONLogPath) {
		return false, nil
	}
	sessionID := stringPtrValue(state.Job.Codex.SessionID)
	if sessionID == "" {
		sessionID = codexSessionIDFromLog(state.Job.Codex.JSONLogPath)
	}
	if sessionID == "" {
		sessionID = "--last"
	}
	at := time.Now().UTC()
	atText := at.Format(time.RFC3339)
	attempt := state.Job.RetryCount + 1
	lastError := ""
	if err := commentRateLimitResume(ctx, deps.github, state.Job, attempt, sessionID, at); err != nil {
		lastError = fmt.Sprintf("rate limit detected; failed to comment on issue: %s", err)
	} else {
		state.Job.LastGitHubCommentAt = atText
	}
	resumePromptPath := state.Job.Codex.LastMessagePath + ".resume.md"
	if err := os.WriteFile(resumePromptPath, []byte(rateLimitResumePrompt), 0o600); err != nil {
		return false, err
	}
	spec := buildCodexRunSpecForRepo(opts.target, opts.repo, state.Job.Issue.Number, state.Job.Worktree, state.Job.PipelinePhase)
	spec.PromptPath = resumePromptPath
	spec.JSONLogPath = state.Job.Codex.JSONLogPath
	spec.LastMessagePath = state.Job.Codex.LastMessagePath
	spec.ResumeSessionID = sessionID
	if err := opts.prepareCodexRunSpec(&spec, state.Job.Worktree); err != nil {
		return false, err
	}
	tmux, err := deps.runner.StartCodex(ctx, spec)
	if err != nil {
		message := fmt.Sprintf("rate limit resume attempt %d failed to start: %s", attempt, err)
		state.Job.LastError = &message
		state.Job.RetryCount = attempt
		state.UpdatedAt = atText
		_ = writeStateFile(opts.stateFilePath(), *state)
		return false, err
	}
	state.Job.Tmux = tmux
	state.Job.RetryCount = attempt
	if lastError != "" {
		state.Job.LastError = &lastError
	} else {
		message := fmt.Sprintf("Codex hit a rate limit; resume attempt %d started with session %s", attempt, displayResumeSession(sessionID))
		state.Job.LastError = &message
	}
	if sessionID != "--last" {
		state.Job.Codex.SessionID = &sessionID
	}
	state.UpdatedAt = atText
	if err := writeStateFile(opts.stateFilePath(), *state); err != nil {
		return false, err
	}
	return true, nil
}

func commentRateLimitResume(ctx context.Context, client githubClient, job *stateJob, attempt int, sessionID string, at time.Time) error {
	if client == nil || job == nil || job.Issue.Number == 0 {
		return nil
	}
	body := fmt.Sprintf(`<!-- %s%s:%d -->
run-weaver detected a Codex rate limit interruption and started an automatic resume attempt.

- attempt: %d
- session: %s
- worktree: %s
- json log: %s
- state: running
- detected at: %s

No secret values are included in this comment. If this keeps repeating, check the Codex account limit and the local JSONL log before removing the running label.
`, rateLimitCommentMarkerPrefix, job.ClaimID, attempt, attempt, displayResumeSession(sessionID), job.Worktree, job.Codex.JSONLogPath, at.Format(time.RFC3339))
	return client.Comment(ctx, job.Issue.Number, body)
}

func displayResumeSession(sessionID string) string {
	if sessionID == "--last" || sessionID == "" {
		return "last available session"
	}
	return sessionID
}

func codexLogLooksRateLimited(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			if textLooksRateLimited(line) {
				return true
			}
			continue
		}
		if codexJSONLineLooksRateLimited(value) {
			return true
		}
	}
	return false
}

func codexJSONLineLooksRateLimited(value any) bool {
	typed, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if textLooksRateLimited(jsonFieldText(typed["error"])) {
		return true
	}
	eventType := strings.ToLower(jsonFieldText(typed["type"]))
	if !strings.Contains(eventType, "error") && !strings.Contains(eventType, "fail") {
		return false
	}
	return textLooksRateLimited(jsonFieldText(typed["message"])) ||
		textLooksRateLimited(jsonFieldText(typed["details"])) ||
		textLooksRateLimited(jsonFieldText(typed["error"]))
}

func jsonFieldText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any, []any:
		data, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(data)
	default:
		return ""
	}
}

func textLooksRateLimited(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "rate limit") || strings.Contains(text, "rate_limit") || strings.Contains(text, "ratelimit")
}

func codexLogLooksCommandNotFound(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			if textLooksCommandNotFound(line) {
				return true
			}
			continue
		}
		if codexJSONLineLooksCommandNotFound(value) {
			return true
		}
	}
	return false
}

func codexJSONLineLooksCommandNotFound(value any) bool {
	typed, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if textLooksCommandNotFound(jsonFieldText(typed["error"])) {
		return true
	}
	eventType := strings.ToLower(jsonFieldText(typed["type"]))
	if !strings.Contains(eventType, "error") && !strings.Contains(eventType, "fail") {
		return false
	}
	return textLooksCommandNotFound(jsonFieldText(typed["message"])) ||
		textLooksCommandNotFound(jsonFieldText(typed["details"])) ||
		textLooksCommandNotFound(jsonFieldText(typed["error"]))
}

func textLooksCommandNotFound(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "codex: command not found") ||
		strings.Contains(text, "codex: not found") ||
		strings.Contains(text, "the term 'codex' is not recognized")
}

func codexSessionIDFromLog(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			continue
		}
		if sessionID := findSessionID(value); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func findSessionID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "session_id" || key == "sessionId" {
				if text, ok := nested.(string); ok {
					return text
				}
			}
		}
		for _, nested := range typed {
			if found := findSessionID(nested); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := findSessionID(nested); found != "" {
				return found
			}
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func codexCompletionReady(target string, job *stateJob) bool {
	if job == nil || job.Codex == nil || job.Codex.LastMessagePath == "" {
		return false
	}
	if _, err := os.Stat(job.Codex.LastMessagePath); err != nil {
		return false
	}
	return tmuxState(target, job.Tmux) != "window_exists"
}

func markDone(ctx context.Context, client githubClient, issueNumber int, prURL, branch, worktree string) error {
	_ = client.RemoveLabel(ctx, issueNumber, runningLabel)
	_ = client.RemoveLabel(ctx, issueNumber, blockedLabel)
	if err := client.EnsureLabel(ctx, doneLabel); err != nil {
		return err
	}
	if err := client.AddLabel(ctx, issueNumber, doneLabel); err != nil {
		return err
	}
	body := fmt.Sprintf(`run-weaver completed this issue.

- draft PR: %s
- branch: %s
- worktree: %s
`, prURL, branch, worktree)
	return client.Comment(ctx, issueNumber, body)
}
