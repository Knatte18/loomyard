// prompt.go composes the burler round prompt: it builds the nine marker
// values the embedded template (template.go) requires and fills it via
// internal/stencil. composePrompt is called only after (*Profile).validate
// has run, so every path field it reads is already a cleaned absolute path
// and p.clusterLenses (when ClusterFan was set) is already resolved.

package burlerengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// composePrompt builds the burler round prompt for p by composing each of
// the template's nine top-level marker values (path lists, fix-scope rules,
// tool-use rules, the prior-rounds block, the cluster-rules block) and
// filling reviewPromptTemplate with them via stencil.Fill.
func composePrompt(p *Profile) (string, error) {
	values := map[string]string{
		"target":            formatFileSet(p.Target),
		"fasit":             formatFileSet(p.Fasit),
		"rubric":            p.Rubric,
		"fix_scope_rules":   fixScopeRules(p),
		"tool_use_rules":    toolUseRules(p.ToolUse),
		"prior_rounds":      priorRoundsBlock(p),
		"cluster_rules":     clusterRulesBlock(p),
		"review_path":       p.ReviewPath,
		"fixer_report_path": p.FixerReportPath,
	}

	rendered, err := stencil.Fill(reviewPromptTemplate, values)
	if err != nil {
		return "", fmt.Errorf("burler: compose prompt: %w", err)
	}
	return string(rendered), nil
}

// formatFileSet renders a FileSet as the template expects: one
// backtick-wrapped absolute path per bullet — a directory entry annotated
// "(a directory — everything under it)" — followed by the Instructions text
// when non-empty. fs.Paths are assumed already validated to exist (validate
// runs before composePrompt), so the os.Stat directory check below reports
// a real answer rather than guessing from the path string.
func formatFileSet(fs FileSet) string {
	var b strings.Builder
	for _, p := range fs.Paths {
		b.WriteString("- `")
		b.WriteString(p)
		b.WriteString("`")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			b.WriteString(" (a directory — everything under it)")
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(fs.Instructions) != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fs.Instructions)
	}
	return b.String()
}

// fixScopeRules returns the write-surface and git-discipline prose for p's
// FixScope: FixScopeSource gets the commit-per-fix rules (host working
// tree, commit each fix individually, never push); FixScopeOverlay gets the
// overlay rules (write surface is exactly the target paths plus the two
// output files, no git at all — the loop owner commits). p.FixScope is
// assumed already validated to one of the two legal values.
func fixScopeRules(p *Profile) string {
	switch p.FixScope {
	case FixScopeSource:
		return "Write surface: the host working tree in this task worktree — the files the " +
			"findings point at, plus whatever the discipline requires in the same change (a " +
			"test that would have caught the bug, the module doc).\n\n" +
			"You commit each fix individually, once green, before starting the next finding. " +
			"Commit message format: `<module-or-target>: fix <finding-id> — <one-line " +
			"what/why>`. Never push."
	case FixScopeOverlay:
		return fmt.Sprintf(
			"Write surface: exactly the target paths plus the two output files (`%s`, `%s`) — "+
				"nothing else — and you run no git commands at all; the loop owner commits "+
				"these files at the round boundary.",
			p.ReviewPath, p.FixerReportPath)
	default:
		// validate rejects every other value before composePrompt is ever
		// called; this branch is unreachable in practice and exists only so
		// the switch fails loud instead of silently rendering an empty
		// fix-scope block if that invariant is ever violated.
		return fmt.Sprintf("burler: internal error: unknown FixScope %q", p.FixScope)
	}
}

// toolUseRules returns the job-A evidence-gathering prose for toolUse: true
// authorizes driving the real substrate, false restricts job A to read-only
// analysis.
func toolUseRules(toolUse bool) string {
	if toolUse {
		return "Drive the real substrate: build, run, test what you review — this is where " +
			"the real defects hide."
	}
	return "Read-only analysis in job A: read files, run nothing."
}

// priorRoundsBlock returns the clean-room hydration prose: the first-round
// fallback when p carries no prior-round files, or a bullet list of the
// prior review/fixer-report paths plus the clean-room rule when it does.
func priorRoundsBlock(p *Profile) string {
	if len(p.PriorReviews) == 0 && len(p.PriorFixerReports) == 0 {
		return "This is the first round — no prior round files exist."
	}

	var b strings.Builder
	b.WriteString("Prior-round files exist:\n\n")
	for _, r := range p.PriorReviews {
		b.WriteString("- review: `")
		b.WriteString(r)
		b.WriteString("`\n")
	}
	for _, f := range p.PriorFixerReports {
		b.WriteString("- fixer-report: `")
		b.WriteString(f)
		b.WriteString("`\n")
	}
	b.WriteString("\nForm your OWN findings first; only AFTER your review is saved may you " +
		"read the prior rounds' files, to confirm previously-fixed behaviors have not " +
		"regressed and to re-evaluate their deferred items.")
	return b.String()
}

// clusterRulesBlock returns job A's fork-cluster prose: the plain
// single-reviewer statement when p carries no ClusterFan, or the full
// phase/spawn/consolidation discipline composed from p.clusterLenses when
// it does. p.clusterLenses is assumed already resolved by
// (*Profile).validate — composePrompt never calls ResolveFan itself.
func clusterRulesBlock(p *Profile) string {
	if p.ClusterFan == "" {
		return "This is a single-reviewer round — no cluster forks. Do the full review yourself."
	}

	var lenses strings.Builder
	for _, lens := range p.clusterLenses {
		lenses.WriteString("- ")
		lenses.WriteString(lens.Name)
		lenses.WriteString(": ")
		lenses.WriteString(lens.Text)
		lenses.WriteString("\n")
	}

	return "This is a cluster round: after exploring the target fully, spawn ALL of the fork " +
		"reviewers below in a SINGLE message via the Agent tool with `subagent_type: \"fork\"`, " +
		"one per lens listed here — never pass a `name` (named forks silently lose inherited " +
		"context).\n\n" +
		"Lenses for this round:\n\n" + lenses.String() + "\n" +
		"Each fork's prompt is this boilerplate plus that lens's emphasis text below: prefer " +
		"your inherited context and fetch only what your lens needs; you are READ-ONLY — never " +
		"Write/Edit/delete any file, never run any git command, never touch the two round " +
		"output files, and never call the Agent tool yourself (forks cannot nest); return your " +
		"findings ONLY as your final message, each with a severity, a location, and a one-line " +
		"summary.\n\n" +
		"While the forks run, YOU (the handler) do your own HOLISTIC review — architecture, " +
		"cross-file invariants, CONSTRAINTS-fit — and prepare the ground truths and the " +
		"severity rubric you will judge every finding against.\n\n" +
		"Consolidation: judge every fork finding and your own holistic findings with EQUAL " +
		"skepticism, dedup findings that describe the same defect across lenses, tag every " +
		"kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move " +
		"false positives to a `## Rejected` prose section below the frontmatter with a " +
		"one-line reason each (a rejected item never appears in `findings:`), and order kept " +
		"findings by severity. The consolidated review is the ONE review file, and it must be " +
		"fully written to disk before job B touches anything — consolidation is part of job A, " +
		"not a separate step after it."
}
