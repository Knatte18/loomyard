// prompt.go composes the burler round prompt: it builds the eight marker
// values the embedded template (template.go) requires and fills it via
// internal/stencil. composePrompt is called only after (*Profile).validate
// has run, so every path field it reads is already a cleaned absolute path.

package burlerengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// composePrompt builds the burler round prompt for p by composing each of
// the template's eight top-level marker values (path lists, fix-scope
// rules, tool-use rules, the prior-rounds block) and filling
// reviewPromptTemplate with them via stencil.Fill.
func composePrompt(p *Profile) (string, error) {
	values := map[string]string{
		"target":            formatFileSet(p.Target),
		"fasit":             formatFileSet(p.Fasit),
		"rubric":            p.Rubric,
		"fix_scope_rules":   fixScopeRules(p),
		"tool_use_rules":    toolUseRules(p.ToolUse),
		"prior_rounds":      priorRoundsBlock(p),
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
