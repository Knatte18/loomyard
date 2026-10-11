// prompt.go composes a half's prompt: it reads the half's instruction stencils from stencilsDir at call time via stencilstore.Read — the explore, review and review-format stencils for the review half, the fix step and review-format stencils for the fix half, and the focus directive block when the round carries one — each with only its own marker subset, and returns the seat's stencil marker values plus the rendered (path, content) instruction pairs the half (Engine.Review or Engine.Fix) writes to disk.
// The seat stencil itself is filled by the seats engine from those values.
// composeReviewPrompt is called only after (*Profile).validateReview has run, and composeFixPrompt after (*Profile).validateFix, so every path field they read is already a cleaned absolute path and p.clusterLenses (when ClusterFan was set) is already resolved.
// They do no filesystem access beyond the directory check formatFileSet already performs on each Target/Fasit path and the stencilstore.Read calls.
// They take the instruction paths as plain string parameters rather than the engine's Geometry, so they never gain geometry awareness of their own;
// the calling half computes the directive, the stencils directory, and the paths.
//
// composeReviewPrompt warns, without failing, when the explore stencil lacks the focus directive marker while a directive was computed for it.

package burlerengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// instructionFile is one rendered instruction asset paired with the absolute path its half will write it to.
// The compose functions return these so the half can materialize them under a fresh per-round directory before handing its shuttle run only its seat stencil.
type instructionFile struct {
	Path    string
	Content string
}

// halfPrompt is what composing one half's prompt yields: the marker values of the half's seat stencil and the instruction files to write.
type halfPrompt struct {
	Values map[string]string
	Files  []instructionFile
}

// reviewFilePaths are the absolute paths the review half's three instruction files are written to.
type reviewFilePaths struct {
	Explore string
	Review  string
	Format  string
}

// reviewFilesIn returns the review half's instruction file paths for the instruction directory dir.
func reviewFilesIn(dir string) reviewFilePaths {
	return reviewFilePaths{
		Explore: filepath.Join(dir, "instruction-1-explore.md"),
		Review:  filepath.Join(dir, "instruction-2-review.md"),
		Format:  filepath.Join(dir, "review-format.md"),
	}
}

// fixFilePaths are the absolute paths the fix half's two instruction files are written to.
type fixFilePaths struct {
	Format string
	Fix    string
}

// fixFilesIn returns the fix half's instruction file paths for the instruction directory dir.
func fixFilesIn(dir string) fixFilePaths {
	return fixFilePaths{
		Format: filepath.Join(dir, "review-format.md"),
		Fix:    filepath.Join(dir, "instruction-fix.md"),
	}
}

// reviewSeatValues returns the stencil marker values of the reviewer seat for p, naming the instruction files at paths.
func reviewSeatValues(p *Profile, paths reviewFilePaths) map[string]string {
	return map[string]string{
		"instruction_1_path": paths.Explore,
		"instruction_2_path": paths.Review,
		"review_path":        p.ReviewPath,
	}
}

// fixSeatValues returns the stencil marker values of the fixer seat for p, naming the instruction files at paths.
func fixSeatValues(p *Profile, paths fixFilePaths) map[string]string {
	return map[string]string{
		"review_path":          p.ReviewPath,
		"review_format_path":   paths.Format,
		"instruction_fix_path": paths.Fix,
	}
}

// renderReviewFormat renders the review-file format stencil, which carries no marker, into the file content both halves point at.
func renderReviewFormat(stencilsDir string) (string, error) {
	template, err := stencilstore.Read(stencilsDir, "burler-review-format")
	if err != nil {
		return "", err
	}
	format, err := stencil.Fill(template, nil)
	if err != nil {
		return "", fmt.Errorf("burler: compose prompt: %w", err)
	}
	return string(format), nil
}

// composeReviewPrompt builds the review half's prompt for p: the explore step, the review step and the review-file format.
// patternDirective is the caller-resolved PATTERN directive for the explore step.
// The reviewer's files hold no fixer-side text; p is assumed already validated by validateReview.
func composeReviewPrompt(stencilsDir string, p *Profile, patternDirective string, paths reviewFilePaths) (halfPrompt, error) {
	exploreTemplate, err := stencilstore.Read(stencilsDir, "burler-step-1-explore")
	if err != nil {
		return halfPrompt{}, err
	}
	focusDirective, err := focusDirectiveBlock(stencilsDir, p.FocusDirective)
	if err != nil {
		return halfPrompt{}, err
	}
	warnIfFocusMarkerAbsent(exploreTemplate, "burler-step-1-explore", focusDirective)
	explore, err := stencil.FillOptional(exploreTemplate, map[string]string{
		"pattern_directive": patternDirective,
		focusMarkerName:     focusDirective,
		"target":            formatFileSet(p.Target),
		"fasit":             formatFileSet(p.Fasit),
		"rubric":            p.Rubric,
		"tool_use_rules":    toolUseRules(p.ToolUse),
	}, []string{"pattern_directive", focusMarkerName})
	if err != nil {
		return halfPrompt{}, fmt.Errorf("burler: compose prompt: %w", err)
	}

	reviewTemplate, err := stencilstore.Read(stencilsDir, "burler-step-review")
	if err != nil {
		return halfPrompt{}, err
	}
	review, err := stencil.Fill(reviewTemplate, map[string]string{
		"cluster_rules":      clusterRulesBlock(p),
		"review_path":        p.ReviewPath,
		"review_format_path": paths.Format,
		"prior_rounds":       priorRoundsBlock(p),
	})
	if err != nil {
		return halfPrompt{}, fmt.Errorf("burler: compose prompt: %w", err)
	}

	format, err := renderReviewFormat(stencilsDir)
	if err != nil {
		return halfPrompt{}, err
	}
	return halfPrompt{
		Values: reviewSeatValues(p, paths),
		Files: []instructionFile{
			{Path: paths.Explore, Content: string(explore)},
			{Path: paths.Review, Content: string(review)},
			{Path: paths.Format, Content: format},
		},
	}, nil
}

// composeFixPrompt builds the fix half's prompt for p: the review-file format and the fix step.
// frictionDirective is the caller-resolved Tier 2 note directive, empty when Tier 2 is off, and work is the fix step's statement of what to fix (see fixWork).
// The fixer's files hold no rubric, fasit, PATTERN directive, focus file, prior rounds or explore step; p is assumed already validated by validateFix.
func composeFixPrompt(stencilsDir string, p *Profile, frictionDirective, work string, paths fixFilePaths) (halfPrompt, error) {
	fixTemplate, err := stencilstore.Read(stencilsDir, "burler-step-fix")
	if err != nil {
		return halfPrompt{}, err
	}
	friction.WarnIfMarkerAbsent(fixTemplate, "burler-step-fix", frictionDirective)
	fix, err := stencil.FillOptional(fixTemplate, map[string]string{
		friction.MarkerName: frictionDirective,
		"work":              work,
		"fix_scope_rules":   fixScopeRules(p),
		"review_path":       p.ReviewPath,
		"fixer_report_path": p.FixerReportPath,
	}, []string{friction.MarkerName})
	if err != nil {
		return halfPrompt{}, fmt.Errorf("burler: compose prompt: %w", err)
	}

	format, err := renderReviewFormat(stencilsDir)
	if err != nil {
		return halfPrompt{}, err
	}
	return halfPrompt{
		Values: fixSeatValues(p, paths),
		Files: []instructionFile{
			{Path: paths.Format, Content: format},
			{Path: paths.Fix, Content: string(fix)},
		},
	}, nil
}

// fixWork returns the fix step's required work statement.
// With no gate-findings path the review's findings are the work;
// with one, the review holds no findings but the round's gates failed, so the work is what that file names.
func fixWork(reviewPath, gateFindingsPath string) string {
	if gateFindingsPath == "" {
		return fmt.Sprintf("Fix every finding of the review at `%s`.", reviewPath)
	}
	return fmt.Sprintf("The review at `%s` holds no findings, but the round's gates failed on the target as it stands. Fix what the gate findings in `%s` name.", reviewPath, gateFindingsPath)
}

// focusDirectiveBlock renders the focus-directive stencil for focusPath, the value injected as the explore step's optional focus_directive marker.
// An empty focusPath returns "" without reading anything:
// the round has no directive, and the marker renders as nothing.
func focusDirectiveBlock(stencilsDir, focusPath string) (string, error) {
	if focusPath == "" {
		return "", nil
	}
	template, err := stencilstore.Read(stencilsDir, "burler-focus-directive")
	if err != nil {
		return "", err
	}
	block, err := stencil.Fill(template, map[string]string{"focus_path": focusPath})
	if err != nil {
		return "", fmt.Errorf("burler: compose prompt: %w", err)
	}
	return string(block), nil
}

// focusMarkerName is the marker the explore stencil carries for the focus directive block.
// composeReviewPrompt's value-map key, its FillOptional optional-names entry, and warnIfFocusMarkerAbsent's literal all derive from it, so they cannot drift apart.
const focusMarkerName = "focus_directive"

// warnIfFocusMarkerAbsent logs a Warn when a round computed a focus directive but the explore stencil carries no marker to render it into.
// focusBlock is the rendered output of focusDirectiveBlock, not the focus path:
// an empty block means the round has no directive, so there is nothing to lose and nothing to warn about.
// Otherwise, when template does not contain the marker literal, it logs the stencil and marker and points the operator at "lyx stencil diff" and "lyx stencil sync".
// It never errors, and the round proceeds without the focus text.
func warnIfFocusMarkerAbsent(template []byte, stencilName, focusBlock string) {
	if focusBlock == "" {
		return
	}
	literal := "{{." + focusMarkerName + "}}"
	if bytes.Contains(template, []byte(literal)) {
		return
	}
	logger.Warn("burler: stencil is missing the focus directive marker; a computed directive will render as nothing -- see \"lyx stencil diff\" and \"lyx stencil sync\"", "stencil", stencilName, "marker", literal)
}

// formatFileSet renders a FileSet as the template expects: one
// backtick-wrapped absolute path per bullet — a directory entry annotated
// "(a directory — everything under it)" — followed by the Instructions text
// when non-empty. fs.Paths are assumed already validated to exist (validate
// runs before composeReviewPrompt), so the os.Stat directory check below reports
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

// fixScopeRules returns the write-surface and git-discipline prose for p's FixScope.
// FixScopeSource gets the commit-per-fix rules (working tree, commit each fix individually, never push).
// FixScopeOverlay gets the overlay rules (write surface is exactly the target paths plus the fixer-report, never the review file, no git at all — the loop owner commits).
// p.FixScope is assumed already validated to one of the two legal values.
func fixScopeRules(p *Profile) string {
	switch p.FixScope {
	case FixScopeSource:
		return "Write surface: the working tree in this task worktree — the files the " +
			"findings point at, plus whatever the discipline requires in the same change (a " +
			"test that would have caught the bug, the module doc).\n\n" +
			"You commit each fix individually, once green, before starting the next finding. " +
			"Commit message format: `<module-or-target>: fix <finding-id> — <one-line " +
			"what/why>`. Never push."
	case FixScopeOverlay:
		return fmt.Sprintf(
			"Write surface: exactly the target paths%s plus the fixer-report (`%s`) — "+
				"nothing else — and you run no git commands at all; the loop owner commits "+
				"these files at the round boundary.",
			namedTargetPaths(p.Target.Paths), p.FixerReportPath)
	default:
		// validateReview rejects every other value before composeReviewPrompt is ever
		// called; this branch is unreachable in practice and exists only so
		// the switch fails loud instead of silently rendering an empty
		// fix-scope block if that invariant is ever violated.
		return fmt.Sprintf("burler: internal error: unknown FixScope %q", p.FixScope)
	}
}

// namedTargetPaths returns paths as a parenthesised, backtick-wrapped list led by a space, or "" when there are none.
func namedTargetPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	quoted := make([]string, len(paths))
	for i, path := range paths {
		quoted[i] = "`" + path + "`"
	}
	return " (" + strings.Join(quoted, ", ") + ")"
}

// toolUseRules returns the evidence-gathering prose of the explore and review steps for toolUse: true
// authorizes driving the real substrate, false restricts those steps to read-only
// analysis.
func toolUseRules(toolUse bool) string {
	if toolUse {
		return "Drive the real substrate: build, run, test what you review — this is where " +
			"the real defects hide."
	}
	return "Read-only analysis in the explore and review steps: read files, run nothing."
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
		"regressed and to re-evaluate their deferred and disputed items. " +
		"If you re-raise a finding a prior fixer-report disputed, state in that finding's " +
		"**Issue:** why the dispute's evidence does not hold, amending the matching " +
		"finding in your saved review before you end your turn.")
	return b.String()
}

// clusterRulesBlock returns the review step's fork-cluster prose: the plain single-reviewer statement when p carries no ClusterFan, or the full phase/spawn/consolidation discipline composed from p.clusterLenses when it does.
// p.clusterLenses is assumed already resolved by (*Profile).validateReview — composeReviewPrompt never calls ResolveFan itself.
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
		"findings ONLY as your final message, each with a severity, a class, a location, and a " +
		"one-line summary.\n\n" +
		"While the forks run, YOU (the handler) do your own HOLISTIC review — architecture, " +
		"cross-file invariants, PATTERN-fit — and prepare the ground truths and the " +
		"severity rubric you will judge every finding against.\n\n" +
		"Consolidation: judge every fork finding and your own holistic findings with EQUAL " +
		"skepticism, dedup findings that describe the same defect across lenses, tag every " +
		"kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move " +
		"false positives to a `## Rejected` prose section below the frontmatter with a " +
		"one-line reason each (a rejected item never appears in `findings:`), and order kept " +
		"findings by severity. The consolidated review is the ONE review file, and the review is " +
		"complete once it is fully written to disk — consolidation is part of the review, " +
		"not a separate step after it."
}
