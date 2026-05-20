package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type worktreeManager struct {
	commands commandRunner
}

type worktreeSpec struct {
	CloneDir string
	Path     string
	Branch   string
}

const codexSubagentGuidance = "Use Codex built-in subagents for suitable investigation, review, or delegable subtasks when the repository AGENTS.md does not prohibit them, higher-priority runtime instructions allow them, and the runtime provides them. If subagents are unavailable or prohibited, continue with a self-review."

const documentationConflictGuidance = `Documentation conflict policy:
- Treat current-state and handoff files such as docs/progress.md, docs/handoff.md, and docs/process-log.md as operational documentation. Do not create task dependencies or stacked PRs solely because multiple tasks may update those files.
- Prefer GitHub Issue comments, PR bodies, and run-weaver state for task-local progress, verification notes, and handoff details. If the repository requires operational docs updates, keep them minimal and task-local.
- Treat semantic documentation such as README files, docs/architecture.md, docs/cli.md, docs/github-issue-flow.md, docs/decision-log.md, docs/adr/, migrations, lockfiles, public APIs, shared services, and shared tests as dependency signals when multiple tasks change the same behavior or decision.
- Campaign task PRs are stacked by the dispatcher in completion order by default. Planner dependencies should express semantic or blocking dependencies, not mechanical PR stacking.
- For campaigns with several implementation tasks, prefer one final documentation consolidation task for operational current-state docs instead of making every implementation task depend on every other task only to avoid operational-doc conflicts.`

func newWorktreeManager(commands commandRunner) worktreeManager {
	if commands == nil {
		commands = execCommandRunner{}
	}
	return worktreeManager{commands: commands}
}

func (m worktreeManager) Prepare(ctx context.Context, target string, issue githubIssue, repoURL string) (worktreeSpec, error) {
	return m.PrepareForRepo(ctx, target, "", issue, repoURL)
}

func (m worktreeManager) PrepareForRepo(ctx context.Context, target, repository string, issue githubIssue, repoURL string) (worktreeSpec, error) {
	return m.PrepareForRepoWithBase(ctx, target, repository, issue, repoURL, "")
}

func (m worktreeManager) PrepareForRepoWithBase(ctx context.Context, target, repository string, issue githubIssue, repoURL, baseBranch string) (worktreeSpec, error) {
	spec := buildWorktreeSpecForRepo(target, repository, issue)
	if err := m.ensureClone(ctx, spec.CloneDir, repoURL); err != nil {
		return spec, err
	}
	if err := os.MkdirAll(filepath.Dir(spec.Path), 0o755); err != nil {
		return spec, err
	}
	if _, err := os.Stat(filepath.Join(spec.Path, ".git")); err == nil {
		return spec, nil
	}
	baseRef := "origin/HEAD"
	if baseBranch != "" {
		baseRef = "origin/" + baseBranch
	}
	if err := m.commands.Run(ctx, "git", "-C", spec.CloneDir, "worktree", "add", "-B", spec.Branch, spec.Path, baseRef); err != nil {
		return spec, err
	}
	return spec, nil
}

func (m worktreeManager) PushBranch(ctx context.Context, worktree, branch string) error {
	return m.commands.Run(ctx, "git", "-C", worktree, "push", "-u", "origin", branch)
}

func (m worktreeManager) FetchOrigin(ctx context.Context, worktree string) error {
	return m.commands.Run(ctx, "git", "-C", worktree, "fetch", "origin")
}

func (m worktreeManager) MergeBase(ctx context.Context, worktree, baseRef string) error {
	return m.commands.Run(ctx, "git", "-C", worktree, "merge", "--no-edit", baseRef)
}

func (m worktreeManager) UnmergedFiles(ctx context.Context, worktree string) ([]string, error) {
	out, err := m.commands.Output(ctx, "git", "-C", worktree, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	return parseGitPathLines(out), nil
}

func (m worktreeManager) ChangedFiles(ctx context.Context, worktree string) ([]string, error) {
	diffOut, err := m.commands.Output(ctx, "git", "-C", worktree, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	untrackedOut, err := m.commands.Output(ctx, "git", "-C", worktree, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var files []string
	for _, file := range append(parseGitPathLines(diffOut), parseGitPathLines(untrackedOut)...) {
		if !seen[file] {
			seen[file] = true
			files = append(files, file)
		}
	}
	return files, nil
}

func parseGitPathLines(out []byte) []string {
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func (m worktreeManager) CommitAll(ctx context.Context, worktree, message string) (bool, error) {
	status, err := m.commands.Output(ctx, "git", "-C", worktree, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(status)) == "" {
		return false, nil
	}
	if err := m.commands.Run(ctx, "git", "-C", worktree, "add", "-A"); err != nil {
		return false, err
	}
	if err := m.commands.Run(ctx, "git", "-C", worktree, "commit", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

func (m worktreeManager) ensureClone(ctx context.Context, cloneDir, repoURL string) error {
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
		return m.commands.Run(ctx, "git", "-C", cloneDir, "fetch", "origin")
	}
	if repoURL == "" {
		return fmt.Errorf("repository URL is required to create Codex clone")
	}
	if err := os.MkdirAll(filepath.Dir(cloneDir), 0o755); err != nil {
		return err
	}
	return m.commands.Run(ctx, "git", "clone", repoURL, cloneDir)
}

func buildWorktreeSpec(target string, issue githubIssue) worktreeSpec {
	return buildWorktreeSpecForRepo(target, "", issue)
}

func buildWorktreeSpecForRepo(target, repository string, issue githubIssue) worktreeSpec {
	root := dataRootForRepo(target, repository)
	return worktreeSpec{
		CloneDir: filepath.Join(root, "src"),
		Path:     filepath.Join(root, "worktrees", "issue-"+strconv.Itoa(issue.Number)),
		Branch:   "codex/issue-" + strconv.Itoa(issue.Number) + "-" + slugify(issue.Title),
	}
}

func writePromptFile(path string, issue githubIssue) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# GitHub Issue #%d

Title: %s
URL: %s

Body:
%s

Relevant comments:
%s

Implement the requested change using the issue title, body, and relevant human comments as the task source. If the body is empty but the title clearly describes a concrete change, proceed from the title. Ignore run-weaver claim/status comments. Only block when the title, body, and human comments together are not specific enough to identify an actionable change.

Follow the repository AGENTS.md instructions, run focused verification, and leave a concise final summary.

%s

%s
`, issue.Number, issue.Title, issue.URL, emptyAsNone(strings.TrimSpace(issue.Body)), relevantIssueComments(issue.Comments), documentationConflictGuidance, codexSubagentGuidance)
	return os.WriteFile(path, []byte(body), 0o600)
}

func writeCampaignPromptFile(path string, campaign issueRef, decisions []campaignDecision, task campaignTask, issue githubIssue, phase string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# Campaign Task Pipeline

Campaign: #%d %s
Task ID: %s
Pipeline phase: %s

GitHub Issue #%d
Title: %s
URL: %s

Task body:
%s

Relevant comments:
%s

Campaign decisions:
%s

Follow the repository AGENTS.md instructions. For the plan phase, produce and execute the minimum planning work needed before implementation. For implement, make the focused code change. For review, inspect the current task changes and fix regressions. For verify, run focused verification and fix task-local failures. Do not move to unrelated campaign tasks in this session.

%s

%s
`, campaign.Number, campaign.Title, task.ID, emptyAsNone(phase), issue.Number, issue.Title, issue.URL, emptyAsNone(strings.TrimSpace(issue.Body)), relevantIssueComments(issue.Comments), campaignDecisionPromptSummary(decisions), documentationConflictGuidance, codexSubagentGuidance)
	return os.WriteFile(path, []byte(body), 0o600)
}

func campaignDecisionPromptSummary(decisions []campaignDecision) string {
	if len(decisions) == 0 {
		return "none"
	}
	lines := make([]string, 0, len(decisions)*6)
	for _, decision := range decisions {
		lines = append(lines, fmt.Sprintf("- %s: %s", decision.ID, emptyAsNone(decision.Title)))
		lines = append(lines, fmt.Sprintf("  status: %s", emptyAsUnknown(decision.Status)))
		if decision.Answer != "" {
			lines = append(lines, fmt.Sprintf("  answer: %s", decision.Answer))
		}
		if decision.Recommendation != "" {
			lines = append(lines, fmt.Sprintf("  recommendation: %s", decision.Recommendation))
		}
		if len(decision.OptionDetails) > 0 {
			lines = append(lines, "  option details:")
			for _, detail := range decision.OptionDetails {
				lines = append(lines, "    - "+detail)
			}
		}
		if len(decision.BlockedTasks) > 0 {
			lines = append(lines, "  blocked tasks: "+strings.Join(decision.BlockedTasks, ", "))
		}
		if len(decision.CanContinueTasks) > 0 {
			lines = append(lines, "  can continue tasks: "+strings.Join(decision.CanContinueTasks, ", "))
		}
	}
	return strings.Join(lines, "\n")
}

func writeCampaignPlannerPromptFile(path string, issue githubIssue) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# Campaign Planner

GitHub Campaign Issue #%d
Title: %s
URL: %s

Issue body:
%s

Relevant comments:
%s

You are planning a run-weaver Campaign. Inspect the repository before planning. Prefer repository documentation, roadmap files, accepted decisions, and architecture notes over the issue body when they conflict. Treat progress, handoff, and process-log files as useful current-state inputs, not as reasons by themselves to serialize unrelated implementation tasks. Useful places often include README files, docs/, roadmap, progress, handoff, architecture, and issue-flow documents, but do not assume fixed filenames.

%s

Return only valid JSON. Do not include Markdown fences or explanatory text.

The JSON schema is:
{
  "tasks": [
    {
      "id": "task-short-kebab-case-id",
      "title": "PR-sized task title",
      "body": "Concrete task instructions for Codex. Include enough repo context for implementation.",
      "dependencies": ["task-id-this-task-depends-on"]
    }
  ],
  "decisions": [
    {
      "id": "decision-short-kebab-case-id",
      "title": "Decision needed from the human",
      "context": "Neutral summary of why this decision is needed and what constraint triggered it.",
      "evidence": ["Concrete repo file, issue, log, existing decision, or command result that supports the need for this decision."],
      "options": ["approve", "revise", "stop"],
      "optionDetails": ["approve: what will happen if approved", "revise: what information is needed", "stop: what work remains blocked"],
      "recommendation": "approve",
      "impact": ["Implementation, operations, security, cost, migration, or schedule impact."],
      "reversibility": "Whether the choice can be changed later and what rollback or migration would require.",
      "blockedTasks": ["task-id-blocked-by-this-decision"],
      "canContinueTasks": ["task-id-that-can-run-before-the-answer"]
    }
  ]
}

Rules:
- Make each task roughly one draft PR.
- Use task IDs for dependencies.
- Apply the documentation conflict policy above when deciding dependencies. Operational current-state docs alone should not force stacked PRs; semantic docs, decisions, migrations, public APIs, shared services, or shared tests should.
- If multiple tasks need to update operational current-state docs, add a final documentation consolidation task rather than chaining all implementation tasks only because of docs/progress.md, docs/handoff.md, or docs/process-log.md.
- Do not ask for human approval of the whole plan.
- Add decisions only where implementation would otherwise require a product, architecture, cost, secret, account, permission, or irreversible choice.
- If the request or docs are too ambiguous to plan safely, return a decision that blocks the ambiguous tasks instead of guessing.
- For every decision, provide objective context, evidence, option details, impact, and reversibility. Separate observed facts from recommendation. Do not include secrets, token values, or raw private log contents.
`, issue.Number, issue.Title, issue.URL, emptyAsNone(strings.TrimSpace(issue.Body)), relevantIssueComments(issue.Comments), documentationConflictGuidance+"\n\n"+codexSubagentGuidance)
	return os.WriteFile(path, []byte(body), 0o600)
}

func relevantIssueComments(comments []githubComment) string {
	lines := make([]string, 0, len(comments))
	for _, comment := range comments {
		text := strings.TrimSpace(comment.Body)
		if text == "" || isRunWeaverComment(text) {
			continue
		}
		lines = append(lines, "- "+text)
	}
	if len(lines) == 0 {
		return "none"
	}
	return strings.Join(lines, "\n")
}

var nonSlugCharRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.ToLower(value)
	slug = nonSlugCharRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "task"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		return "task"
	}
	return slug
}
