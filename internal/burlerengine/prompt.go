// prompt.go composes the prompt of one burler half:
// the instruction files it writes and the marker values of its seat stencil.
// The compose functions read their stencils at call time and take the instruction paths as plain strings,
// so they know nothing of the engine's geometry.
// The calling half computes the directive, the stencils directory and the paths.
// composeReviewPrompt runs only after validateReview, and composeFixPrompt runs only after validateFix.
// Every path they read is therefore a cleaned absolute path.
// They do no filesystem access beyond the directory checks of the target and fasit paths and the stencil reads.
//
// composeReviewPrompt warns, without failing, when the explore stencil lacks a focus marker for a computed directive.

package burlerengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// instructionFile is one rendered instruction asset paired with the absolute path its half will write it to.
// The compose functions return these so the half can materialize them in a fresh per-round directory.
// The half then hands its shuttle run only its seat stencil.
type instructionFile struct {
	Path    string
	Content string
}

// halfPrompt is the result of composing one half's prompt:
// the marker values of its seat stencil and the instruction files to write.
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

// fillInstruction fills the named instruction stencil from values, reading it from stencilsDir.
// The stencil's blocks and the engine block for engine are read with it.
// The names in optional may be empty.
func fillInstruction(stencilsDir, engine, name string, values map[string]string, optional []string) ([]byte, error) {
	template, blocks, err := stencilstore.ReadSet(stencilsDir, engine, name)
	if err != nil {
		return nil, err
	}
	filled, err := stencil.FillWith(template, blocks, values, optional)
	if err != nil {
		return nil, fmt.Errorf("burler: compose prompt: %w", err)
	}
	return filled, nil
}

// renderReviewFormat renders the review-file format stencil for engine into the content both halves point at.
// That stencil carries no marker.
func renderReviewFormat(stencilsDir, engine string) (string, error) {
	format, err := fillInstruction(stencilsDir, engine, "burler-review-format", nil, nil)
	if err != nil {
		return "", err
	}
	return string(format), nil
}

// composeReviewPrompt builds the review half's prompt for p, rendered for the reviewer's engine:
// the explore step, the review step and the review-file format.
// patternDirective is the caller-resolved PATTERN directive for the explore step.
// assistRules is the chair's consolidation part of an assisted round, rendered after the cluster rules.
// It is empty for any other round, which leaves the rendered prompt unchanged.
// specsDir holds the deployed lens report spec a fanned round's reviewer saves its fork reports in the format of.
// The reviewer's files hold no fixer-side text; p is assumed already validated by validateReview.
func composeReviewPrompt(stencilsDir, specsDir, engine string, p *Profile, patternDirective, assistRules string, paths reviewFilePaths) (halfPrompt, error) {
	exploreTemplate, exploreBlocks, err := stencilstore.ReadSet(stencilsDir, engine, "burler-step-1-explore")
	if err != nil {
		return halfPrompt{}, err
	}
	focusDirective, err := focusDirectiveBlock(stencilsDir, engine, p.FocusDirective)
	if err != nil {
		return halfPrompt{}, err
	}
	warnIfFocusMarkerAbsent(exploreTemplate, "burler-step-1-explore", focusDirective)
	forkReading, err := forkReadingBlock(stencilsDir, engine, p)
	if err != nil {
		return halfPrompt{}, err
	}
	explore, err := stencil.FillWith(exploreTemplate, exploreBlocks, map[string]string{
		"pattern_directive": patternDirective,
		focusMarkerName:     focusDirective,
		"target":            formatFileSet(p.Target),
		"fasit":             formatFileSet(p.Fasit),
		"rubric":            p.Rubric,
		"tool_use_rules":    toolUseRules(p.ToolUse),
		"fork_reading":      forkReading,
	}, []string{"pattern_directive", focusMarkerName, "fork_reading"})
	if err != nil {
		return halfPrompt{}, fmt.Errorf("burler: compose prompt: %w", err)
	}

	clusterRules, err := clusterRulesBlock(stencilsDir, specsDir, engine, p)
	if err != nil {
		return halfPrompt{}, err
	}
	review, err := fillInstruction(stencilsDir, engine, "burler-step-review", map[string]string{
		"cluster_rules":      clusterRules,
		assistRulesMarker:    assistRules,
		"review_path":        p.ReviewPath,
		"review_format_path": paths.Format,
		"prior_rounds":       priorRoundsBlock(p),
	}, []string{assistRulesMarker})
	if err != nil {
		return halfPrompt{}, err
	}

	format, err := renderReviewFormat(stencilsDir, engine)
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

// composeFixPrompt builds the fix half's prompt for p, rendered for the fixer's engine:
// the review-file format and the fix step.
// frictionDirective is the caller-resolved Tier 2 note directive, empty when Tier 2 is off.
// work is the fix step's statement of what to fix.
// specsDir holds the deployed lens report spec a fixer fan's fixer saves its fork reports in the format of.
// The fixer's files hold no rubric, fasit, PATTERN directive, focus file, prior rounds or explore step;
// p is assumed already validated by validateFix.
func composeFixPrompt(stencilsDir, specsDir, engine string, p *Profile, frictionDirective, work string, paths fixFilePaths) (halfPrompt, error) {
	fixTemplate, fixBlocks, err := stencilstore.ReadSet(stencilsDir, engine, "burler-step-fix")
	if err != nil {
		return halfPrompt{}, err
	}
	friction.WarnIfMarkerAbsent(fixTemplate, "burler-step-fix", frictionDirective)
	selfCheck, err := selfCheckBlock(stencilsDir, specsDir, engine, p)
	if err != nil {
		return halfPrompt{}, err
	}
	fix, err := stencil.FillWith(fixTemplate, fixBlocks, map[string]string{
		friction.MarkerName: frictionDirective,
		"work":              work,
		"fix_scope_rules":   fixScopeRules(p),
		"review_path":       p.ReviewPath,
		"fixer_report_path": p.FixerReportPath,
		"self_check":        selfCheck,
	}, []string{friction.MarkerName, "self_check"})
	if err != nil {
		return halfPrompt{}, fmt.Errorf("burler: compose prompt: %w", err)
	}

	format, err := renderReviewFormat(stencilsDir, engine)
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
// With decisionsRouted set, the review's decision-class findings went to the run's parent, not the fixer.
func fixWork(reviewPath, gateFindingsPath string, decisionsRouted bool) string {
	switch {
	case gateFindingsPath == "" && !decisionsRouted:
		return fmt.Sprintf("Fix every finding of the review at `%s`.", reviewPath)
	case gateFindingsPath == "":
		return fmt.Sprintf("Fix every finding of the review at `%s` but its `decision`-class findings. %s", reviewPath, routedDecisionsLine)
	case !decisionsRouted:
		return fmt.Sprintf("The review at `%s` holds no findings, but the round's gates failed on the target as it stands. Fix what the gate findings in `%s` name.", reviewPath, gateFindingsPath)
	}
	return fmt.Sprintf("The review at `%s` holds no findings but `decision`-class ones, and the round's gates failed on the target as it stands. Fix what the gate findings in `%s` name. %s", reviewPath, gateFindingsPath, routedDecisionsLine)
}

// routedDecisionsLine tells a fixer the review's decision-class findings went to the run's parent, not to it.
const routedDecisionsLine = "Those went to your parent, who makes each call: you neither fix, dispute nor defer them, and your fixer-report leaves them out."

// focusDirectiveBlock renders the focus-directive stencil for engine and focusPath.
// The result is the value injected as the explore step's optional focus_directive marker.
// An empty focusPath returns "" without reading anything:
// the round has no directive, and the marker renders as nothing.
func focusDirectiveBlock(stencilsDir, engine, focusPath string) (string, error) {
	if focusPath == "" {
		return "", nil
	}
	block, err := fillInstruction(stencilsDir, engine, "burler-focus-directive", map[string]string{"focus_path": focusPath}, nil)
	if err != nil {
		return "", err
	}
	return string(block), nil
}

// focusMarkerName is the marker the explore stencil carries for the focus directive block.
// The value-map key, optional-names entry and literal all derive from this name, so they cannot drift apart.
const focusMarkerName = "focus_directive"

// warnIfFocusMarkerAbsent logs a Warn when a round computed a focus directive,
// but the explore stencil carries no marker to render it into.
// focusBlock is the rendered directive text, not the focus path:
// an empty block means the round has no directive, so there is nothing to lose and nothing to warn about.
// Otherwise, when template does not contain the marker literal, it logs the stencil and the marker,
// and the message points the operator at "lym stencil diff" and "lym stencil sync".
// It never errors, and the round proceeds without the focus text.
func warnIfFocusMarkerAbsent(template []byte, stencilName, focusBlock string) {
	if focusBlock == "" {
		return
	}
	literal := "{{." + focusMarkerName + "}}"
	if bytes.Contains(template, []byte(literal)) {
		return
	}
	logger.Warn("burler: stencil is missing the focus directive marker; a computed directive will render as nothing -- see \"lym stencil diff\" and \"lym stencil sync\"", "stencil", stencilName, "marker", literal)
}

// formatFileSet renders a FileSet as the template expects.
// Each path is one backtick-wrapped bullet, and a directory entry is annotated "(a directory — everything under it)".
// The Instructions text follows the bullets when it is non-empty.
// fs.Paths are assumed already validated to exist.
// The directory check therefore reports a real answer rather than guessing from the path string.
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
// FixScopeOverlay gets the overlay rules.
// The write surface is exactly the target paths plus the fixer-report, never the review file.
// It runs no git at all, since the loop owner commits.
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
		var lensReports string
		if p.FixFan != "" {
			lensReports = " and the self-check's lens reports"
		}
		return fmt.Sprintf(
			"Write surface: exactly the target paths%s plus the fixer-report (`%s`)%s — "+
				"nothing else — and you run no git commands at all; the loop owner commits "+
				"these files at the round boundary.",
			namedTargetPaths(p.Target.Paths), p.FixerReportPath, lensReports)
	default:
		// Validation rejects every other value before the prompt is composed, so this branch is unreachable in practice.
		// It exists so the switch fails loud, not silently renders an empty fix-scope block.
		// That matters only if that invariant is ever violated.
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

// toolUseRules returns the evidence-gathering prose of the explore and review steps.
// With toolUse true, it authorizes driving the real substrate.
// With toolUse false, it restricts those steps to read-only analysis.
func toolUseRules(toolUse bool) string {
	if toolUse {
		return "Drive the real substrate: build, run, test what you review — this is where " +
			"the real defects hide."
	}
	return "Read-only analysis in the explore and review steps: read files, run nothing."
}

// priorRoundsBlock returns the clean-room hydration prose.
// When p carries no prior-round files, it returns the first-round fallback.
// Otherwise, it returns a bullet list of the prior review and fixer-report paths, plus the clean-room rule.
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

// clusterRulesBlock renders the review step's cluster-rules value for engine from the cluster stencils.
// When p carries no ClusterFan, the value is the solo part's body.
// Otherwise it is the fanned part, filled with p's lens list and the lens report path beside p's review.
// The lens report spec path is resolved under specsDir.
// An assistant's fanned part is its own, whose spawn and in-turn wait come from engine's engine block.
// The value carries no trailing newline, since the review step's marker sits on a line of its own.
// Its clusterLenses are assumed already resolved, and this function never resolves them itself.
func clusterRulesBlock(stencilsDir, specsDir, engine string, p *Profile) (string, error) {
	name, values := "burler-cluster-solo", map[string]string(nil)
	if p.ClusterFan != "" {
		name = "burler-cluster-fan"
		if p.assistant {
			name = assistClusterFanStencil
		}
		values = map[string]string{
			"lens_list":             lensList(p.clusterLenses),
			"lens_report_path":      lensreport.PathPattern(p.ReviewPath),
			"lens_report_spec_path": stencilstore.Path(specsDir, lensreport.SpecName),
		}
	}
	rules, err := fillInstruction(stencilsDir, engine, name, values, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(rules), "\n"), nil
}

// forkReadingBlock renders the fork-reading stencil for engine, as the explore step's optional fork_reading marker.
// A round with no ClusterFan returns "" without reading anything.
// The value leads with the blank line that separates it from the tool-use rules.
// The stencil loader drops a stencil's leading blank lines, so this function adds the separator itself,
// and the trailing newline is left to the explore stencil.
func forkReadingBlock(stencilsDir, engine string, p *Profile) (string, error) {
	if p.ClusterFan == "" {
		return "", nil
	}
	reading, err := fillInstruction(stencilsDir, engine, "burler-fork-reading", nil, nil)
	if err != nil {
		return "", err
	}
	return "\n\n" + strings.TrimRight(string(reading), "\n"), nil
}

// selfCheckBlock renders the fix self-check stencil for engine, for the fix step's optional self_check marker.
// A profile with no FixFan returns "" without reading anything.
// Otherwise it fills the stencil with the deployed fix-check rubric path and p's fixer lenses.
// The lens report path pattern derives from p's fixer report, and the spec path lies under specsDir.
// it ends in a blank line, which the fix step relies on to set off the heading that follows its marker.
// p.fixLenses is assumed already resolved by (*Profile).validateFix.
func selfCheckBlock(stencilsDir, specsDir, engine string, p *Profile) (string, error) {
	if p.FixFan == "" {
		return "", nil
	}
	block, err := fillInstruction(stencilsDir, engine, "burler-fix-self-check", map[string]string{
		"rubric_path":           stencilstore.Path(stencilsDir, "burler-fix-check-rubric"),
		"lens_list":             lensList(p.fixLenses),
		"lens_report_path":      lensreport.PathPattern(p.FixerReportPath),
		"lens_report_spec_path": stencilstore.Path(specsDir, lensreport.SpecName),
	}, nil)
	if err != nil {
		return "", err
	}
	return string(block), nil
}

// lensList renders resolved as one "- <name>: <text>" bullet per lens in fan order, each bullet ending in a newline.
func lensList(resolved []lenses.Lens) string {
	var list strings.Builder
	for _, lens := range resolved {
		list.WriteString("- ")
		list.WriteString(lens.Name)
		list.WriteString(": ")
		list.WriteString(lens.Text)
		list.WriteString("\n")
	}
	return list.String()
}
