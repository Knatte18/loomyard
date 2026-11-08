// template_test.go pins the master template's digest-field and outcome-key lists against webster's Go definitions.
// It also pins the marker sets through stencil.Fill/FillOptional round-trips.
// It also pins the composition of the fork and recovery templates.
// It also pins the rendered prompts' Go-filled values and Go-chosen fragments.
// Every asset is read at call time via stencilstore.Read from a stencils directory this file seeds itself:
// newTestStencilsDir seeds a t.TempDir() through `stencilkit`.
// Every test here is tier0 and spawn-free:
// it runs no subprocess and no git, and it builds no fixture trees.
// Its only fixtures are a plain t.TempDir() PATTERN.md and the seeded stencils directory.
// It calls stencilstore.Read, stencil.Fill/FillOptional, RenderForkPrompt, RenderRecoveryPrompt, and RenderProgress.

package websterengine_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// testCardGates is a placeholder per-card gate list for the card-gates input of the rendered fork and recovery prompts.
const testCardGates = "- `_lym/plan/01-gate.md`:\n  - `lym gate test --wait --tier 0 --card 1-gate`"

// testBatchGate is a placeholder batch gate for the batch-gate input of the rendered fork and recovery prompts.
const testBatchGate = "- `lym gate test --wait --tier 0 --tier 1 --card 1-gate`"

// testFixGate is a placeholder gate call for the fix-gate input of the rendered verify-fix prompt.
const testFixGate = "lym gate test --wait --tier 0"

// testOutcomePath and testSummaryPath are placeholder contract-file paths.
// They stand in for the outcome and summary inputs of the rendered fork and verify-fix prompts.
const (
	testOutcomePath = "/lym/webster/outcome.yaml"
	testSummaryPath = "/lym/webster/summary.md"
)

// testVerifyFixPromptPath is a placeholder fixer-prompt path for the verify-fix input of the rendered master prompt.
const testVerifyFixPromptPath = "/lym/webster/prompts/verify-fix.md"

// newTestSpecsDir returns a real, non-empty directory for the specs-dir input of the fork and recovery prompts.
// It must be real because specs_dir is a required marker in the shared implementer-job body.
// An empty value passes only while the body carries no such marker;
// once the marker is added, an empty value fails at run time.
func newTestSpecsDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// mustMasterTemplate, mustForkTemplate, and mustRecoveryTemplate wrap their accessor and call t.Fatalf on error.
// Call sites that do not test the error path stay short.
func mustMasterTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.MasterTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("MasterTemplate(%q) = _, %v; want nil error", stencilsDir, err)
	}
	return got
}

func mustForkTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.ForkTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("ForkTemplate(%q) = _, %v; want nil error", stencilsDir, err)
	}
	return got
}

func mustRecoveryTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.RecoveryTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("RecoveryTemplate(%q) = _, %v; want nil error", stencilsDir, err)
	}
	return got
}

func mustImplementerBodyTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.ImplementerBodyTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("ImplementerBodyTemplate(%q) = _, %v; want nil error", stencilsDir, err)
	}
	return got
}

// testLayout returns the told anchor root and stencils directory for a real t.TempDir() hub.
// The hub is seeded with webster's stencils at fabricengine.StencilsDir(hub).
// Prompt renders that do not exercise pattern_directive's active branch use this fixture.
// The anchor root's worktree subdirectory is never created on disk.
// Its absence means pattern.Directive's stat on the PATTERN.md path always resolves PATTERN inactive.
// That matches the empty pattern_directive these tests expect.
func testLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	return filepath.Join(hub, "worktree"), fabricengine.StencilsDir(hub)
}

// patternActiveLayout returns the told anchor root and stencils directory for a real t.TempDir() hub.
// A real PATTERN.md file sits directly under the returned root, so pattern.Directive returns non-empty.
// The hub is seeded with webster's stencils at fabricengine.StencilsDir(hub).
func patternActiveLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	dir := filepath.Join(hub, "worktree")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte("# PATTERN\n\nsome constraints\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
	return filepath.Join(hub, "worktree"), fabricengine.StencilsDir(hub)
}

// frictionActiveLayout returns the told anchor root and stencils directory for a real t.TempDir() hub.
// The hub is seeded with every registry stencil, including the friction-directive pair.
// A non-empty notePath needs that pair to resolve a real friction.Directive.
func frictionActiveLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	return filepath.Join(hub, "worktree"), fabricengine.StencilsDir(hub)
}

// patternActiveMissingPatternStencilsLayout returns a PATTERN-active anchor root and stencils directory.
// Its pattern-directive stencils are removed, so a call site's hoisted pattern.Directive read fails.
func patternActiveMissingPatternStencilsLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	anchorRoot, stencilsDir = patternActiveLayout(t)
	stencilkit.Remove(t, stencilsDir, "pattern-directive-implementer", "pattern-directive-review-fix", "pattern-directive-orchestrator")
	return anchorRoot, stencilsDir
}

// stripFrictionMarker drops the literal "{{.friction_directive}}" line from the named stencil under stencilsDir.
// The result is the marker-free template that WarnIfMarkerAbsent exists to warn about.
// It lets a test prove that a composer still renders, without error, when its own stencil carries no marker.
func stripFrictionMarker(t *testing.T, stencilsDir, stencilName string) {
	t.Helper()
	path := stencilstore.Path(stencilsDir, stencilName)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
	}
	stripped := strings.ReplaceAll(string(content), "{{.friction_directive}}\n", "")
	if stripped == string(content) {
		t.Fatalf("stripFrictionMarker(%q): marker literal not found in stencil bytes", stencilName)
	}
	if err := os.WriteFile(path, []byte(stripped), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// requireContains fails the test, naming the missing needle, if text does not contain it.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// requireNotContains fails the test, naming the forbidden needle, if text contains it.
func requireNotContains(t *testing.T, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Errorf("output unexpectedly contains %q; want it fully removed", needle)
	}
}

// extractBacktickBullets returns, in order, the backtick-quoted token of each "- `token`" bullet line in a section.
// The section runs from heading, matched by trimmed equality, to the next "## " heading or EOF.
func extractBacktickBullets(text, heading string) []string {
	lines := strings.Split(text, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}

	bulletRe := regexp.MustCompile("^-\\s+`([^`]+)`$")
	var tokens []string
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "## ") {
			break
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			tokens = append(tokens, m[1])
		}
	}
	return tokens
}

// digestSectionHeading and outcomeKeysHeadingSub name the headings whose bullet lists are the pinned field sets.
// Extraction stays inside those sections, because prose elsewhere backtick-quotes some of the same field names.
// Reading the whole body would pull those names into the pinned sets.
const (
	digestSectionHeading  = "## Read ONLY the digest fields — quoted here, exactly"
	outcomeKeysHeadingSub = "`{{.outcome_path}}` itself carries exactly these three keys, quoted here, exactly:"
)

// masterTemplateMarkerValues returns a values map with every required marker set to a non-empty placeholder.
// The optional markers, pattern_directive and friction_directive, are set too.
// A test fills the template cleanly, or deletes one key at a time to check the error for each missing marker.
func masterTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":            "01 — json-flag — add the --json flag",
		"progress":               "none",
		"remaining":              "01-a",
		"outcome_path":           "/lym/webster/outcome.yaml",
		"summary_path":           "/lym/webster/summary.md",
		"verify_fix_prompt_path": "/lym/webster/prompts/verify-fix.md",
		"plan_dir":               "_lym/plan",
		"self_fix_cap":           "2",
		"parent_decisions":       "none",
		"pattern_directive":      "## Constraints — do this before you fork anything\n\n- Read the overview.",
		"friction_directive":     "## Friction note — optional, only if something went wrong\n\nWrite it to /lym/webster/friction/webster-master.md.",
		"parent_directive":       "## Your parent\n\nNo parent is recorded for this run.",
		"edit_directive":         "## Editing files\n\nChange files only with Edit or Write.",
	}
}

// forkTemplateMarkerValues returns a values map with every required marker set to a non-empty placeholder.
// It omits the markers the master template already supplies, such as pattern_directive,
// and it sets friction_directive, the fork template's optional marker.
func forkTemplateMarkerValues() map[string]string {
	return map[string]string{
		"card_pointers":      "- `_lym/plan/02-list-tests.md`",
		"card_gates":         testCardGates,
		"batch_gate":         testBatchGate,
		"report_path":        "/webster/reports/02-list-tests.yaml",
		"self_fix_cap":       "2",
		"worktree_root":      "/worktree",
		"prev_digest":        "01-json-flag: done head_sha=abc123",
		"specs_dir":          "/hub/repo/" + fabricengine.BoltDirName + "/_lym/specs",
		"gate_tiers":         "- tier1: unit tests of `x`",
		"rename_tools":       "- rename `y` through the language server",
		"outcome_path":       testOutcomePath,
		"summary_path":       testSummaryPath,
		"friction_directive": "## Friction note — optional, only if something went wrong\n\nWrite it to /webster/friction/02-list-tests.md.",
	}
}

// recoveryTemplateMarkerValues returns a values map with every required marker set to a non-empty placeholder.
// It starts from the fork template's values,
// and it adds the optional markers pattern_directive, parent_directive and edit_directive.
func recoveryTemplateMarkerValues() map[string]string {
	values := forkTemplateMarkerValues()
	values["pattern_directive"] = "## Constraints — do this before you write any code\n\n- Read the overview."
	values["parent_directive"] = "## Your parent\n\nNo parent is recorded for this run."
	values["edit_directive"] = "## Editing files\n\nChange files only with Edit or Write."
	return values
}

// cardWithSourcePath returns a minimal card with SourcePath set to the plan directory and NN-<slug>.md filename.
// Fixture batches are built by hand and never pass through the plan parser, so the path is set here.
func cardWithSourcePath(number int, slug, summary string) planparser.Card {
	return planparser.Card{
		Number:     number,
		Slug:       slug,
		Title:      slug,
		Summary:    summary,
		SourcePath: fmt.Sprintf("%s/%02d-%s.md", planparser.PlanDirRel(), number, slug),
	}
}

// assertCardPointerIsRelative fails the test if got does not contain sourcePath.
// It also fails if the pointer has an absolute prefix, such as a leaked t.TempDir() path.
// A prefix like that means the renderer built the pointer from an absolute base instead of printing it verbatim.
func assertCardPointerIsRelative(t *testing.T, got, sourcePath string) {
	t.Helper()
	requireContains(t, got, sourcePath)
	if filepath.IsAbs(sourcePath) {
		t.Errorf("card SourcePath %q is absolute; want a worktree-relative token", sourcePath)
	}
	if strings.HasPrefix(sourcePath, "/") {
		t.Errorf("card SourcePath %q leaks an absolute prefix", sourcePath)
	}
}

// renderFork renders the fork prompt for batch with the shared placeholder paths.
// root is both the anchor root and the prompt worktree root,
// and the plan directory is derived under it.
func renderFork(batch batcher.Batch, root, stencilsDir, specsDir, prevDigest string, selfFixCap int, notePath string) (string, error) {
	prompt, err := websterengine.RenderForkPrompt(batch, testCardGates, testBatchGate, prevDigest, "/reports/01-alpha.yaml", filepath.Join(root, "_lym", "plan"), root, stencilsDir, specsDir, "none", nil, testOutcomePath, testSummaryPath, selfFixCap, notePath, false)
	return string(prompt), err
}

// renderForkWithCardFixer renders the fork prompt for batch.
// Its card gate goes through a cold fixer.
func renderForkWithCardFixer(batch batcher.Batch, root, stencilsDir, specsDir string) (string, error) {
	prompt, err := websterengine.RenderForkPrompt(batch, testCardGates, testBatchGate, "", "/reports/01-alpha.yaml", filepath.Join(root, "_lym", "plan"), root, stencilsDir, specsDir, "none", nil, testOutcomePath, testSummaryPath, 2, "", true)
	return string(prompt), err
}

// renderForkWithLists renders the fork prompt for batch with the given gate tiers and rename tools.
func renderForkWithLists(batch batcher.Batch, root, stencilsDir, specsDir, gateTiers string, renameTools []string) (string, error) {
	prompt, err := websterengine.RenderForkPrompt(batch, testCardGates, testBatchGate, "", "/reports/01-alpha.yaml", filepath.Join(root, "_lym", "plan"), root, stencilsDir, specsDir, gateTiers, renameTools, testOutcomePath, testSummaryPath, 2, "", false)
	return string(prompt), err
}

// testGateTiers is the rendered tier and gate-list lines a prompt takes.
// The first line holds a template marker that must reach the prompt verbatim.
const testGateTiers = "- tier1: unit tests of `{{.x}}`\n- gate card: [1]"

// testRenameTools are two rename tools.
// The first tool holds a template marker that must reach the prompt verbatim.
var testRenameTools = []string{"rename Go symbols with `{{.x}}`", "rename files with `git mv`"}

// testRenameToolsBullets is the rename tool list rendered as the consecutive bullets a prompt carries.
const testRenameToolsBullets = "- rename Go symbols with `{{.x}}`\n- rename files with `git mv`\n"

// renderRecovery renders the recovery prompt for batch with the shared placeholder paths.
// The plan directory and prompt worktree root derive from anchorRoot, and PATTERN.md is read from repoRoot.
func renderRecovery(batch batcher.Batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, notePath, parent string) (string, error) {
	return renderRecoveryWithUncommitted(batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, "", notePath, parent, "none", nil, nil)
}

// renderRecoveryWithUncommitted renders the recovery prompt for batch.
// It takes the caller-rendered uncommitted-paths block, the hub's verify tiers and the declared PATTERN changes.
func renderRecoveryWithUncommitted(batch batcher.Batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, uncommittedPaths, notePath, parent, gateTiers string, renameTools, patternChanges []string) (string, error) {
	got, err := websterengine.RenderRecoveryPrompt(batch, testCardGates, testBatchGate, "", failureDigest, uncommittedPaths, "/reports/01-alpha.yaml", repoRoot, filepath.Join(anchorRoot, "_lym", "plan"), anchorRoot, stencilsDir, specsDir, gateTiers, renameTools, patternChanges, 2, notePath, parent, false)
	return string(got), err
}

// renderRecoveryWithCardFixer renders the recovery prompt for batch.
// Its card gate goes through a cold fixer.
func renderRecoveryWithCardFixer(batch batcher.Batch, repoRoot, anchorRoot, stencilsDir, specsDir string) (string, error) {
	got, err := websterengine.RenderRecoveryPrompt(batch, testCardGates, testBatchGate, "", "", "", "/reports/01-alpha.yaml", repoRoot, filepath.Join(anchorRoot, "_lym", "plan"), anchorRoot, stencilsDir, specsDir, "none", nil, nil, 2, "", "", true)
	return string(got), err
}

// renderMaster renders the master prompt for batches with a nil state.
// It fills in the shared placeholder paths and the declared PATTERN changes.
func renderMaster(batches []batcher.Batch, anchorRoot, repoRoot, stencilsDir, notePath, parent string, patternChanges []string) (string, error) {
	return renderMasterWithDecisions(batches, anchorRoot, repoRoot, stencilsDir, notePath, parent, patternChanges, nil)
}

// renderMasterWithDecisions renders the master prompt with the parent's added decisions.
func renderMasterWithDecisions(batches []batcher.Batch, anchorRoot, repoRoot, stencilsDir, notePath, parent string, patternChanges, parentDecisions []string) (string, error) {
	got, err := websterengine.RenderMasterPrompt(batches, nil, testOutcomePath, testSummaryPath, testVerifyFixPromptPath, filepath.Join(anchorRoot, "_lym", "plan"), 2, anchorRoot, repoRoot, stencilsDir, patternChanges, parentDecisions, notePath, parent)
	return string(got), err
}

// renderVerifyFix renders the fixer prompt for reportPath with the shared placeholder paths and testFixGate.
func renderVerifyFix(reportPath, anchorRoot, stencilsDir, notePath string) (string, error) {
	got, err := websterengine.RenderVerifyFixPrompt(reportPath, anchorRoot, filepath.Join(anchorRoot, "_lym", "plan"), stencilsDir, testOutcomePath, testSummaryPath, notePath, testFixGate)
	return string(got), err
}

// testReplyPath is the placeholder reply path the cold fixer prompts are rendered with:
// the verify fixer's through gatefixer.RenderPrompt, the plan fixer's through RenderPlanFixPrompt.
const testReplyPath = "/lym/webster/verify-fix-reply.md"

// coldFixerInputs returns the gatefixer.PromptInputs of the cold verify fixer for reportPath.
// It fills in the shared placeholder paths, testReplyPath, testFixGate and the `webster-body-verify-fix` stencil.
func coldFixerInputs(reportPath, repoRoot, anchorRoot, stencilsDir, notePath, parent string, patternChanges []string) gatefixer.PromptInputs {
	return gatefixer.PromptInputs{
		StencilsDir: stencilsDir,
		BodyStencil: "webster-body-verify-fix",
		BodyValues: map[string]string{
			"worktree_root": anchorRoot,
			"plan_dir":      "_lym/plan",
			"outcome_path":  testOutcomePath,
			"summary_path":  testSummaryPath,
			"fix_gate":      testFixGate,
		},
		ReportPath:     reportPath,
		ReplyPath:      testReplyPath,
		RepoRoot:       repoRoot,
		PatternChanges: patternChanges,
		ParentName:     parent,
		NotePath:       notePath,
	}
}

// renderColdFixer renders the cold verify fixer prompt for reportPath through gatefixer.RenderPrompt.
func renderColdFixer(reportPath, repoRoot, anchorRoot, stencilsDir, notePath, parent string, patternChanges []string) (string, error) {
	got, err := gatefixer.RenderPrompt(coldFixerInputs(reportPath, repoRoot, anchorRoot, stencilsDir, notePath, parent, patternChanges))
	return string(got), err
}

// promptCheck is one rendered prompt and the assertions its case makes on it.
// wantErr expects the render to fail.
// Otherwise every contains needle is present, every notContains needle is absent,
// and the inOrder needles appear in that order.
type promptCheck struct {
	text        string
	err         error
	wantErr     bool
	contains    []string
	notContains []string
	inOrder     []string
}

func (c promptCheck) verify(t *testing.T) {
	t.Helper()
	if c.wantErr {
		if c.err == nil {
			t.Fatal("render error = nil; want an error")
		}
		return
	}
	if c.err != nil {
		t.Fatalf("render = _, %v; want nil error", c.err)
	}
	for _, needle := range c.contains {
		requireContains(t, c.text, needle)
	}
	for _, needle := range c.notContains {
		requireNotContains(t, c.text, needle)
	}
	last := -1
	for _, needle := range c.inOrder {
		idx := strings.Index(c.text, needle)
		if idx == -1 || idx <= last {
			t.Errorf("%q (idx %d) does not follow the previous ordered needle (idx %d)", needle, idx, last)
		}
		last = idx
	}
}

// TestMasterTemplate_QuotesBulletLists asserts the master template's two bullet lists match the code they quote.
// The digest-field list names exactly the Digest struct's json-tag field names, in declared order.
// The outcome-file list names exactly the outcome.yaml schema keys, with no fewer and no extras.
//
//testtiming:keep pins the master template's digest-field and outcome-key bullet lists matching the Digest fields and outcome schema exactly, in order; the composition test checks markers and sections, not these lists
func TestMasterTemplate_QuotesBulletLists(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	cases := []struct {
		name    string
		heading string
		want    []string
	}{
		{"digest fields", digestSectionHeading, []string{"batch", "status", "head_sha", "deviations", "departures", "renames", "dead_reason", "need", "elapsed_s"}},
		{"outcome schema keys", outcomeKeysHeadingSub, []string{"outcome", "stuck_reason", "batches_done"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractBacktickBullets(text, tc.heading)
			if len(got) != len(tc.want) {
				t.Fatalf("bullets = %v (%d); want %v (%d)", got, len(got), tc.want, len(tc.want))
			}
			for i, field := range tc.want {
				if got[i] != field {
					t.Errorf("bullet %d = %q; want %q", i, got[i], field)
				}
			}
		})
	}
}

// TestTemplates_FillRequiresEveryRequiredMarker asserts that every template fills when all markers are supplied.
// It also asserts that a fill missing any one required marker fails, naming that marker.
// The optional markers are excluded from the deletion sweep: deleting one must not error.
//
//testtiming:keep pins each template refusing a fill missing any single required marker, naming it, and accepting each optional marker absent; the render tests always supply every marker
func TestTemplates_FillRequiresEveryRequiredMarker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stencilsDir := newTestStencilsDir(t)

	cases := []struct {
		name     string
		template func(*testing.T, string) []byte
		values   func() map[string]string
		optional []string
		required []string
	}{
		{
			name:     "master",
			template: mustMasterTemplate,
			values:   masterTemplateMarkerValues,
			optional: []string{"pattern_directive", "friction_directive", "parent_directive"},
			required: []string{"batch_index", "progress", "outcome_path", "summary_path", "verify_fix_prompt_path", "plan_dir", "self_fix_cap", "parent_decisions", "edit_directive"},
		},
		{
			name:     "fork",
			template: mustForkTemplate,
			values:   forkTemplateMarkerValues,
			optional: []string{"friction_directive", "card_gate_fixer"},
			required: []string{"card_pointers", "card_gates", "batch_gate", "report_path", "self_fix_cap", "worktree_root", "prev_digest", "specs_dir", "gate_tiers", "rename_tools", "outcome_path", "summary_path"},
		},
		{
			name:     "recovery",
			template: mustRecoveryTemplate,
			values:   recoveryTemplateMarkerValues,
			optional: []string{"pattern_directive", "failure_digest", "uncommitted_paths", "card_gate_fixer", "friction_directive", "parent_directive"},
			required: []string{"card_pointers", "card_gates", "batch_gate", "report_path", "self_fix_cap", "worktree_root", "prev_digest", "specs_dir", "gate_tiers", "rename_tools", "edit_directive"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Run("all markers supplied", func(t *testing.T) {
				t.Parallel()
				if _, err := stencil.FillOptional(tc.template(t, stencilsDir), tc.values(), tc.optional); err != nil {
					t.Fatalf("stencil.FillOptional() = %v; want nil", err)
				}
			})
			for _, marker := range tc.required {
				t.Run("missing "+marker, func(t *testing.T) {
					t.Parallel()
					values := tc.values()
					delete(values, marker)
					_, err := stencil.FillOptional(tc.template(t, stencilsDir), values, tc.optional)
					if err == nil {
						t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
					}
					if !strings.Contains(err.Error(), marker) {
						t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
					}
				})
			}
		})
	}
}

// TestTemplates_Composition asserts the composed templates contain the implementer body's exact pre-Fill bytes.
// The composed templates carry no stencil banner.
// The stencil files are re-read on every call instead of cached.
// A missing stencils directory is a hard error naming the stencil, not a fallback to an embedded default.
func TestTemplates_Composition(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("fork and recovery share the implementer body", func(t *testing.T) {
		t.Parallel()
		stencilsDir := newTestStencilsDir(t)

		body := []byte(stencil.StripLeadingComment(string(mustImplementerBodyTemplate(t, stencilsDir))))
		if len(body) == 0 {
			t.Fatalf("ImplementerBodyTemplate() = empty; want non-empty shared body bytes")
		}
		for name, composed := range map[string][]byte{"ForkTemplate": mustForkTemplate(t, stencilsDir), "RecoveryTemplate": mustRecoveryTemplate(t, stencilsDir)} {
			if !bytes.Contains(composed, body) {
				t.Errorf("%s() does not contain ImplementerBodyTemplate()'s bytes", name)
			}
			// Every asset carries a real `lym-stencil:` banner stamp.
			// A leak here means the composition stopped stripping the second asset's banner.
			requireNotContains(t, string(composed), "lym-stencil:")
			requireNotContains(t, string(composed), "<!--")
		}
	})

	t.Run("reads reflect on-disk edits", func(t *testing.T) {
		t.Parallel()
		dir := newTestStencilsDir(t)

		beforeFork := mustForkTemplate(t, dir)
		beforeRecovery := mustRecoveryTemplate(t, dir)

		forkPrefixPath := filepath.Join(dir, "webster", "webster-prefix-fork.md")
		editedPrefix := append(append([]byte{}, stencils.WebsterPrefixFork...), []byte("\n\nEDITED FORK PREFIX MARKER\n")...)
		if err := os.WriteFile(forkPrefixPath, editedPrefix, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", forkPrefixPath, err)
		}

		afterPrefixEditFork := mustForkTemplate(t, dir)
		if bytes.Equal(afterPrefixEditFork, beforeFork) {
			t.Errorf("ForkTemplate() unchanged after editing webster-prefix-fork.md; want the on-disk edit to reach the composed output")
		}
		afterPrefixEditRecovery := mustRecoveryTemplate(t, dir)
		if !bytes.Equal(afterPrefixEditRecovery, beforeRecovery) {
			t.Errorf("RecoveryTemplate() changed after editing webster-prefix-fork.md; want it unaffected by a fork-only prefix edit")
		}

		bodyPath := filepath.Join(dir, "webster", "webster-body-implementer.md")
		editedBody := append(append([]byte{}, stencils.WebsterBodyImplementer...), []byte("\n\nEDITED SHARED BODY MARKER\n")...)
		if err := os.WriteFile(bodyPath, editedBody, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", bodyPath, err)
		}

		afterBodyEditFork := mustForkTemplate(t, dir)
		if bytes.Equal(afterBodyEditFork, afterPrefixEditFork) {
			t.Errorf("ForkTemplate() unchanged after editing the shared webster-body-implementer.md; want the on-disk edit to reach the composed output")
		}
		afterBodyEditRecovery := mustRecoveryTemplate(t, dir)
		if bytes.Equal(afterBodyEditRecovery, afterPrefixEditRecovery) {
			t.Errorf("RecoveryTemplate() unchanged after editing the shared webster-body-implementer.md; want the on-disk edit to reach both composed prompts")
		}
	})

	t.Run("missing stencils directory is a hard error", func(t *testing.T) {
		t.Parallel()
		missingDir := filepath.Join(t.TempDir(), "does-not-exist")

		_, err := websterengine.MasterTemplate(missingDir)
		if err == nil {
			t.Fatalf("MasterTemplate(%q) error = nil; want an error naming the missing stencil", missingDir)
		}
		requireContains(t, err.Error(), "webster-template-master")
	})
}

// TestRenderPrompts asserts the rendered fork, recovery, master and fixer prompts.
// Every required marker resolves, and the fork prompt carries none of the content it must keep out.
// The told roots land where they belong.
// A missing or empty input fails the render instead of producing a blank path.
func TestRenderPrompts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alpha := batcher.Batch{Cards: []planparser.Card{cardWithSourcePath(1, "alpha", "add the flag")}}
	seam := batcher.Batch{Cards: []planparser.Card{cardWithSourcePath(1, "seam-extensions", "add the seam")}}
	seamBatches := []batcher.Batch{seam}

	cases := []struct {
		name   string
		render func(t *testing.T) promptCheck
	}{
		{"fork carries the card gates and no leftover marker", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderFork(alpha, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			return promptCheck{text: text, err: err,
				contains:    []string{testCardGates, testBatchGate, testOutcomePath, testSummaryPath},
				notContains: []string{"{{.card_gates}}", "{{.batch_gate}}"}}
		}},
		{"recovery carries the card gates and no leftover marker", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err,
				contains:    []string{testCardGates, testBatchGate},
				notContains: []string{"{{.card_gates}}", "{{.batch_gate}}"}}
		}},
		{"fork with the card fixer on carries its paragraph after the card gates", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderForkWithCardFixer(alpha, root, stencilsDir, newTestSpecsDir(t))
			return promptCheck{text: text, err: err,
				inOrder:     []string{testCardGates + "\n\n   A card gate call that fails has already been through a cold fixer", "\n\n   A failing gate call closes"},
				notContains: []string{"{{"}}
		}},
		{"recovery with the card fixer on carries its paragraph after the card gates", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecoveryWithCardFixer(alpha, root, root, stencilsDir, newTestSpecsDir(t))
			return promptCheck{text: text, err: err,
				inOrder:     []string{testCardGates + "\n\n   A card gate call that fails has already been through a cold fixer", "\n\n   A failing gate call closes"},
				notContains: []string{"{{"}}
		}},
		{"fork and recovery with the card fixer off carry no paragraph", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			fork, err := renderFork(alpha, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			if err != nil {
				return promptCheck{err: err}
			}
			recovery, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: fork + recovery, err: err,
				contains:    []string{testCardGates + "\n\n   A failing gate call closes"},
				notContains: []string{"cold fixer", "{{"}}
		}},
		{"fork self-fix section carries the self-fix cap", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderFork(alpha, root, stencilsDir, newTestSpecsDir(t), "", 7, "")
			if err != nil {
				return promptCheck{err: err}
			}
			_, rest, ok := strings.Cut(text, "## Bounded self-fix, then stop")
			if !ok {
				t.Fatalf("rendered prompt has no \"## Bounded self-fix, then stop\" section")
			}
			section, _, _ := strings.Cut(rest, "\n## ")
			return promptCheck{text: section, contains: []string{"`7`"}}
		}},
		{"fork renders the first-batch sentinel for an empty prevDigest", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderFork(seam, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			return promptCheck{text: text, err: err, contains: []string{"none (first batch)"}}
		}},
		{"fork passes a non-empty prevDigest through verbatim", func(t *testing.T) promptCheck {
			const digest = "01-seam-extensions: done head_sha=abc123"
			root, stencilsDir := testLayout(t)
			text, err := renderFork(seam, root, stencilsDir, newTestSpecsDir(t), digest, 2, "")
			return promptCheck{text: text, err: err, contains: []string{digest}}
		}},
		{"fork states the specs dir", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			specsDir := newTestSpecsDir(t)
			if !filepath.IsAbs(specsDir) {
				t.Fatalf("newTestSpecsDir(t) = %q; want an absolute path", specsDir)
			}
			text, err := renderFork(seam, root, stencilsDir, specsDir, "", 2, "")
			return promptCheck{text: text, err: err, contains: []string{specsDir}, notContains: []string{"{{.specs_dir}}"}}
		}},
		{"recovery states the specs dir", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			specsDir := newTestSpecsDir(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, specsDir, "", "", "")
			return promptCheck{text: text, err: err, contains: []string{specsDir}, notContains: []string{"{{.specs_dir}}"}}
		}},
		{"fork refuses an empty specs dir", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			_, err := renderFork(seam, root, stencilsDir, "", "", 2, "")
			return promptCheck{err: err, wantErr: true}
		}},
		{"recovery refuses an empty specs dir", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			_, err := renderRecovery(alpha, root, root, stencilsDir, "", "", "", "")
			return promptCheck{err: err, wantErr: true}
		}},
		{"fork omits shared decisions and points at the card", func(t *testing.T) promptCheck {
			card := cardWithSourcePath(1, "json-flag", "add the --json flag")
			root, stencilsDir := testLayout(t)
			text, err := renderFork(batcher.Batch{Cards: []planparser.Card{card}}, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			if err == nil {
				assertCardPointerIsRelative(t, text, card.SourcePath)
			}
			return promptCheck{text: text, err: err, notContains: []string{"## Shared Decisions"}}
		}},
		{"fork omits the rename mechanic and points at the card", func(t *testing.T) promptCheck {
			card := cardWithSourcePath(4, "helptree-rename", "rename the row mapper")
			card.Type = planparser.CardTypeRename
			pair := planparser.MovePair{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"}
			card.Pairs = []planparser.MovePair{pair}
			card.Targets = []string{pair.Old, pair.New}
			root, stencilsDir := testLayout(t)
			text, err := renderFork(batcher.Batch{Cards: []planparser.Card{card}}, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			if err == nil {
				assertCardPointerIsRelative(t, text, card.SourcePath)
			}
			return promptCheck{text: text, err: err, notContains: []string{"## Rename mechanic"}}
		}},
		{"fork fills worktree_root from the prompt worktree root", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderFork(alpha, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			return promptCheck{text: text, err: err, contains: []string{root}}
		}},
		{"fork takes worktree_root from the prompt worktree root when the anchor differs", func(t *testing.T) promptCheck {
			_, stencilsDir := testLayout(t)
			const promptWorktreeRoot = "/standalone/state/worktree"
			text, err := renderFork(alpha, promptWorktreeRoot, stencilsDir, newTestSpecsDir(t), "", 2, "")
			return promptCheck{text: text, err: err, contains: []string{promptWorktreeRoot}, notContains: []string{"/hub/master-builder"}}
		}},
		{"recovery fills worktree_root from the prompt worktree root", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{root}}
		}},
		{"recovery takes worktree_root from the prompt worktree root when the anchor differs", func(t *testing.T) promptCheck {
			layoutRoot, stencilsDir := testLayout(t)
			const promptWorktreeRoot = "/standalone/state/worktree"
			text, err := renderRecovery(alpha, layoutRoot, promptWorktreeRoot, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{promptWorktreeRoot}, notContains: []string{layoutRoot}}
		}},
		{"recovery points a cold strand at the overview with PATTERN inactive", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err,
				contains:    []string{alpha.Cards[0].SourcePath},
				notContains: []string{"{{", "## Constraints"}}
		}},
		{"recovery carries the PATTERN overview when PATTERN is active", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"some constraints", "## Constraints"}}
		}},
		{"recovery carries a declared PATTERN change under the declared-changes heading", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "", "", "none", nil, []string{"- `PATTERN-x` adds: a rule"})
			return promptCheck{text: text, err: err, inOrder: []string{"### Declared PATTERN changes", "- `PATTERN-x` adds: a rule"}}
		}},
		{"recovery reads PATTERN.md from the told repo root", func(t *testing.T) promptCheck {
			repoRoot, stencilsDir := patternActiveLayout(t)
			text, err := renderRecovery(alpha, repoRoot, filepath.Join(repoRoot, "backend"), stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"some constraints"}}
		}},
		{"recovery refuses a missing pattern-directive stencil", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveMissingPatternStencilsLayout(t)
			_, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{err: err, wantErr: true}
		}},
		{"recovery renders a failure digest verbatim", func(t *testing.T) promptCheck {
			const digest = "- fork wrote outside its worktree\n- suspect path: internal/x/y.go"
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), digest, "", "")
			return promptCheck{text: text, err: err, contains: []string{digest}, notContains: []string{"{{.failure_digest}}"}}
		}},
		{"recovery renders none for an empty failure digest", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"Why this batch is being recovered\n\nnone\n"}}
		}},
		{"recovery renders the uncommitted-paths block verbatim", func(t *testing.T) promptCheck {
			const block = "Written by this run:\n- internal/x/y.go\n\nNot written by this run:\n- notes.txt"
			root, stencilsDir := testLayout(t)
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", block, "", "", "none", nil, nil)
			return promptCheck{text: text, err: err, contains: []string{"What the worktree holds\n\n" + block + "\n"}, notContains: []string{"{{.uncommitted_paths}}"}}
		}},
		{"fork renders each rename tool as a bullet and the plan verify command", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderForkWithLists(alpha, root, stencilsDir, newTestSpecsDir(t), "none", testRenameTools)
			return promptCheck{text: text, err: err, contains: []string{testRenameToolsBullets}, notContains: []string{"{{.rename_tools}}"}}
		}},
		{"recovery renders each rename tool as a bullet and the plan verify command", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "", "", "none", testRenameTools, nil)
			return promptCheck{text: text, err: err, contains: []string{testRenameToolsBullets}, notContains: []string{"{{.rename_tools}}"}}
		}},
		{"fork renders none for no rename tools and names no plan verify command", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderForkWithLists(alpha, root, stencilsDir, newTestSpecsDir(t), "none", nil)
			return promptCheck{text: text, err: err, contains: []string{"surgical Edits.\n\nnone\n"}, notContains: []string{"\n- \n", "verify command follows"}}
		}},
		{"recovery renders none for no rename tools and names no plan verify command", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "", "", "none", nil, nil)
			return promptCheck{text: text, err: err, contains: []string{"surgical Edits.\n\nnone\n"}, notContains: []string{"\n- \n", "verify command follows"}}
		}},
		{"fork renders the gate tier lines verbatim", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderForkWithLists(alpha, root, stencilsDir, newTestSpecsDir(t), testGateTiers, nil)
			return promptCheck{text: text, err: err, contains: []string{"follow:\n\n" + testGateTiers + "\n\n"}, notContains: []string{"{{.gate_tiers}}"}}
		}},
		{"recovery renders the gate tier lines verbatim", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "", "", testGateTiers, nil, nil)
			return promptCheck{text: text, err: err, contains: []string{"follow:\n\n" + testGateTiers + "\n\n"}, notContains: []string{"{{.gate_tiers}}"}}
		}},
		{"recovery renders none for a clean worktree", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"What the worktree holds\n\nnone\n"}}
		}},
		{"fork reads the card files, locates by quarry and ends on the batch report", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderFork(alpha, root, stencilsDir, newTestSpecsDir(t), "", 2, "")
			return promptCheck{text: text, err: err,
				notContains: []string{"kick-start", "Kick-start", "card-pack", "## Start here"},
				inOrder: []string{
					"## The FRESH-READ rule\n\nNo code reaches you from Master",
					"1. Read the card file.\n   It is your whole instruction for that card.",
					"   Locate them with one `lym quarry resolve <glyph>...` call",
					"`lym loom lint-pattern` and `lym quarry resolve` are always allowed",
					"does not make it `FAILED`, except a failure in the packages the last batch's `--main-packages` call adds.\n",
				}}
		}},
		{"recovery reads the card files and locates by quarry", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err,
				notContains: []string{"kick-start", "Kick-start", "card-pack", "## Start here"},
				inOrder: []string{
					"## The FRESH-READ rule\n\nNo code reaches you from Master",
					"1. Read the card file.",
					"   Locate them with one `lym quarry resolve <glyph>...` call",
				}}
		}},
		{"master never fills worktree_root", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, notContains: []string{"worktree_root", root}}
		}},
		{"master renders each parent decision in order", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			first := "### Added after Discussion (parent, 2026-10-26): Reset the batch\n\nDecision: reset batch 06 to its start.\n\nRationale: the finding is unrecoverable."
			second := "### Added after Discussion (arbiter, 2026-10-26): Keep the seam\n\nDecision: keep it.\n\nRationale: two callers."
			text, err := renderMasterWithDecisions(seamBatches, root, root, stencilsDir, "", "", nil, []string{first, second})
			return promptCheck{text: text, err: err,
				inOrder:     []string{first + "\n\n" + second},
				notContains: []string{"{{.parent_decisions}}"}}
		}},
		{"master renders none for no parent decision", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			if err != nil {
				return promptCheck{err: err}
			}
			withNone, err := renderMasterWithDecisions(seamBatches, root, root, stencilsDir, "", "", nil, []string{"none"})
			if err != nil {
				return promptCheck{err: err}
			}
			if text != withNone {
				t.Error("master prompt with no parent decision differs from the one rendered with the decision none")
			}
			return promptCheck{text: text, notContains: []string{"{{.parent_decisions}}"}}
		}},
		{"master names the fixer prompt", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, contains: []string{testVerifyFixPromptPath}}
		}},
		{"master lists the batches in the sequenced order it was handed", func(t *testing.T) promptCheck {
			batches := []batcher.Batch{
				{Cards: []planparser.Card{cardWithSourcePath(2, "second", "do the second thing")}},
				{Cards: []planparser.Card{cardWithSourcePath(1, "first", "do the first thing")}},
			}
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(batches, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, inOrder: []string{"02 — second", "01 — first"}}
		}},
		{"master with PATTERN inactive renders no constraints block", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err,
				notContains: []string{"{{", "## Constraints", "\n\n\n\n"}}
		}},
		{"master places the PATTERN directive ahead of the first work instruction", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, inOrder: []string{"some constraints", "## Orientation"}}
		}},
		{"master carries a declared PATTERN change under the declared-changes heading", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", []string{"- `PATTERN-x` adds: a rule"})
			return promptCheck{text: text, err: err, inOrder: []string{"### Declared PATTERN changes", "- `PATTERN-x` adds: a rule", "## Orientation"}}
		}},
		{"master reads PATTERN.md from the told repo root", func(t *testing.T) promptCheck {
			repoRoot, stencilsDir := patternActiveLayout(t)
			text, err := renderMaster(seamBatches, filepath.Join(repoRoot, "backend"), repoRoot, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, contains: []string{"some constraints"}}
		}},
		{"master does not read PATTERN.md from the anchor root", func(t *testing.T) promptCheck {
			repoRoot, stencilsDir := patternActiveLayout(t)
			anchorRoot := filepath.Join(repoRoot, "backend")
			text, err := renderMaster(seamBatches, anchorRoot, anchorRoot, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err, notContains: []string{"some constraints"}}
		}},
		{"master refuses a missing pattern-directive stencil", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveMissingPatternStencilsLayout(t)
			_, err := renderMaster(seamBatches, root, root, stencilsDir, "", "", nil)
			return promptCheck{err: err, wantErr: true}
		}},
		{"fixer prompt names the gate report and the contract files", func(t *testing.T) promptCheck {
			const reportPath = "/lym/webster/reports/verify-gate.yaml"
			root, stencilsDir := testLayout(t)
			text, err := renderVerifyFix(reportPath, root, stencilsDir, "")
			return promptCheck{text: text, err: err,
				contains:    []string{reportPath, testOutcomePath, testSummaryPath, "_lym/plan", "run `" + testFixGate + "` as a foreground Bash call"},
				notContains: []string{"{{", testReplyPath}}
		}},
		{"fixer prompt refuses an empty report path", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			_, err := renderVerifyFix("", root, stencilsDir, "")
			return promptCheck{err: err, wantErr: true}
		}},
		{"cold fixer prompt names the gate report, the reply file and the contract files", func(t *testing.T) promptCheck {
			const reportPath = "/lym/webster/reports/verify-gate.yaml"
			root, stencilsDir := testLayout(t)
			text, err := renderColdFixer(reportPath, root, root, stencilsDir, "", "", nil)
			return promptCheck{text: text, err: err,
				contains:    []string{reportPath, testReplyPath, testOutcomePath, testSummaryPath, "_lym/plan", testFixGate},
				notContains: []string{"{{", "## Constraints"}}
		}},
		{"cold fixer prompt carries a declared PATTERN change when PATTERN is active", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderColdFixer("/reports/verify-gate.yaml", root, root, stencilsDir, "", "", []string{"- `PATTERN-x` adds: a rule"})
			return promptCheck{text: text, err: err, contains: []string{"some constraints"}, inOrder: []string{"### Declared PATTERN changes", "- `PATTERN-x` adds: a rule"}}
		}},
		{"cold fixer prompt refuses an empty reply path", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			inputs := coldFixerInputs("/reports/verify-gate.yaml", root, root, stencilsDir, "", "", nil)
			inputs.ReplyPath = ""
			_, err := gatefixer.RenderPrompt(inputs)
			return promptCheck{err: err, wantErr: true}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.render(t).verify(t)
		})
	}
}

// TestRenderPrompts_FrictionDirective covers every renderer's friction_directive marker.
// A non-empty note path injects the composed directive verbatim.
// An empty note path composes cleanly, with no directive and no friction stencil read.
// A marker-free stencil still composes without error.
func TestRenderPrompts_FrictionDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alpha := batcher.Batch{Cards: []planparser.Card{cardWithSourcePath(1, "alpha", "add the flag")}}
	seamBatches := []batcher.Batch{{Cards: []planparser.Card{cardWithSourcePath(1, "seam-extensions", "add the seam")}}}

	cases := []struct {
		name     string
		stencil  string
		noteFile string
		render   func(anchorRoot, stencilsDir, notePath string) (string, error)
	}{
		{"fork", "webster-prefix-fork", "01-alpha.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderFork(alpha, anchorRoot, stencilsDir, newTestSpecsDir(t), "", 2, notePath)
		}},
		{"recovery", "webster-prefix-recovery", "01-alpha-recovery.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderRecovery(alpha, anchorRoot, anchorRoot, stencilsDir, newTestSpecsDir(t), "", notePath, "")
		}},
		{"master", "webster-template-master", "webster-master.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderMaster(seamBatches, anchorRoot, anchorRoot, stencilsDir, notePath, "", nil)
		}},
		{"verify-fix", "webster-prefix-verify-fix-fork", "webster-verify-fix.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderVerifyFix("/reports/verify-gate.yaml", anchorRoot, stencilsDir, notePath)
		}},
		{"verify-fix-cold", gatefixer.FrameStencil, "webster-verify-fix-cold.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderColdFixer("/reports/verify-gate.yaml", anchorRoot, anchorRoot, stencilsDir, notePath, "", nil)
		}},
		{"plan-fix", "webster-template-plan-fix", "webster-plan-fix.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			got, err := websterengine.RenderPlanFixPrompt("/lym/webster/plan-conflict.md", testReplyPath, anchorRoot, filepath.Join(anchorRoot, "_lym", "plan"), stencilsDir, filepath.Join(anchorRoot, "specs"), notePath, "")
			return string(got), err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("enabled: a non-empty note path appears in the composed prompt verbatim", func(t *testing.T) {
				t.Parallel()
				anchorRoot, stencilsDir := frictionActiveLayout(t)
				notePath := filepath.Join(anchorRoot, "_lym", "friction", tc.noteFile)
				text, err := tc.render(anchorRoot, stencilsDir, notePath)
				promptCheck{text: text, err: err, contains: []string{notePath}}.verify(t)
			})

			t.Run("disabled: an empty note path composes cleanly and reads no friction stencil", func(t *testing.T) {
				t.Parallel()
				// testLayout seeds no friction stencils at all, so a read attempt would fail loudly.
				// A clean render therefore proves the empty-notePath early return in friction.Directive, not a swallowed error.
				anchorRoot, stencilsDir := testLayout(t)
				text, err := tc.render(anchorRoot, stencilsDir, "")
				promptCheck{text: text, err: err, notContains: []string{"Friction note"}}.verify(t)
			})

			t.Run("marker-free template: a stencil with no {{.friction_directive}} literal still composes", func(t *testing.T) {
				t.Parallel()
				anchorRoot, stencilsDir := frictionActiveLayout(t)
				stripFrictionMarker(t, stencilsDir, tc.stencil)
				notePath := filepath.Join(anchorRoot, "_lym", "friction", tc.noteFile)
				_, err := tc.render(anchorRoot, stencilsDir, notePath)
				promptCheck{err: err}.verify(t)
			})
		})
	}
}

// TestRenderPrompts_ParentDirective covers the parent directive in the master, recovery and cold fixer prompts.
// A told name renders the parent variant, an empty one the no-parent variant, and the fork prompt carries neither.
func TestRenderPrompts_ParentDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	batches := []batcher.Batch{{Cards: []planparser.Card{cardWithSourcePath(1, "alpha", "add the flag")}}}
	anchorRoot, stencilsDir := testLayout(t)
	master := func(parent string) string {
		got, err := renderMaster(batches, anchorRoot, anchorRoot, stencilsDir, "", parent, nil)
		if err != nil {
			t.Fatalf("RenderMasterPrompt() = _, %v; want nil error", err)
		}
		return got
	}
	recovery := func(parent string) string {
		got, err := renderRecovery(batches[0], anchorRoot, anchorRoot, stencilsDir, newTestSpecsDir(t), "", "", parent)
		if err != nil {
			t.Fatalf("RenderRecoveryPrompt() = _, %v; want nil error", err)
		}
		return got
	}

	coldFixer := func(parent string) string {
		got, err := renderColdFixer("/reports/verify-gate.yaml", anchorRoot, anchorRoot, stencilsDir, "", parent, nil)
		if err != nil {
			t.Fatalf("gatefixer.RenderPrompt() = _, %v; want nil error", err)
		}
		return got
	}

	const parent = "ab:orch"
	for name, render := range map[string]func(string) string{"master": master, "recovery": recovery, "cold fixer": coldFixer} {
		withParent, without := render(parent), render("")
		if !strings.Contains(withParent, "`"+parent+"`") {
			t.Errorf("%s prompt with a parent does not name %q", name, parent)
		}
		requireNotContains(t, withParent, "No parent is recorded")
		requireContains(t, without, "No parent is recorded")
		requireContains(t, withParent, "Edit or Write")
		requireContains(t, without, "Edit or Write")
		requireNotContains(t, without, "Your parent is")
		if strings.Contains(withParent, "{{") || strings.Contains(without, "{{") {
			t.Errorf("%s prompt contains a leftover {{ marker", name)
		}
	}

	fork, err := renderFork(batches[0], anchorRoot, stencilsDir, newTestSpecsDir(t), "", 2, "")
	if err != nil {
		t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
	}
	requireNotContains(t, fork, "## Your parent")
}

// TestRenderBatchLists asserts that the batch index, progress trail and remaining list keep the slice's own order.
// They never render in ascending batch number order.
// Progress and remaining read only the persisted terminal records, skipping a nil one.
//
//testtiming:keep pins each list's exact text in slice order and its none forms; the covering prompt and run-exit tests check only that a list is present
func TestRenderBatchLists(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	batchOf := func(number int, slug, summary string) batcher.Batch {
		return batcher.Batch{Cards: []planparser.Card{cardWithSourcePath(number, slug, summary)}}
	}
	inOrder := []batcher.Batch{
		batchOf(1, "seam-extensions", "add the seam"),
		batchOf(2, "webster-foundation", "add the foundation"),
		batchOf(3, "webster-audit-policy", "add the audit policy"),
		batchOf(4, "webster-templates", "add the templates"),
	}
	reordered := []batcher.Batch{
		batchOf(3, "third", "do the third thing"),
		batchOf(1, "first", "do the first thing"),
		batchOf(2, "second", "do the second thing"),
	}
	allDone := &websterengine.State{Batches: map[int]*websterengine.BatchState{
		1: {Slug: "first", Terminal: true, Status: "done"},
		2: {Slug: "second", Terminal: true, Status: "done"},
		3: {Slug: "third", Terminal: true, Status: "done"},
	}}
	partial := &websterengine.State{Batches: map[int]*websterengine.BatchState{
		3: {Slug: "third", Terminal: true, Status: "done"},
		1: {Slug: "first", Terminal: false},
	}}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"progress with nil state is none", websterengine.RenderProgress(inOrder, nil), "none"},
		{"progress with no terminal batch is none", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: false},
		}}), "none"},
		{"progress lists terminal batches only, omitting in-flight and unrecorded ones", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: true, Status: "done"},
			2: {Slug: "webster-foundation", Terminal: true, Status: "stuck"},
			3: {Slug: "webster-audit-policy", Terminal: false},
		}}), "01-seam-extensions: done\n02-webster-foundation: stuck"},
		{"progress follows slice order", websterengine.RenderProgress(reordered, allDone), "03-third: done\n01-first: done\n02-second: done"},
		{"progress lists a waiting batch with its need and the verb that resumes it", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: true, Status: "done"},
			2: {Slug: "webster-foundation", Kind: "fork", Status: websterengine.DigestStatusWaiting, Digest: &websterengine.Digest{Status: websterengine.DigestStatusWaiting, Need: "set the tier commands"}},
			3: {Slug: "webster-audit-policy", Kind: "recovery", Status: websterengine.DigestStatusWaiting, Digest: &websterengine.Digest{Status: websterengine.DigestStatusWaiting, Need: "run lym fabric reconcile"}},
		}}), "01-seam-extensions: done\n02-webster-foundation: waiting (need: set the tier commands; resume: lym webster begin-batch 2)\n03-webster-audit-policy: waiting (need: run lym fabric reconcile; resume: lym webster recover-batch 3)"},
		// A state.json carrying an explicit null for a batch key parses to a present-but-nil entry.
		{"progress skips a nil batch record", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: nil,
			2: {Slug: "webster-foundation", Terminal: true, Status: "done"},
		}}), "02-webster-foundation: done"},
		{"progress appends a done batch's pattern departures", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: true, Status: "done", Digest: &websterengine.Digest{Departures: []string{"1: seam: kept the old name", "2: seam: skipped a scan"}}},
			2: {Slug: "webster-foundation", Terminal: true, Status: "done", Digest: &websterengine.Digest{}},
		}}), "01-seam-extensions: done (pattern departures: 1: seam: kept the old name; 2: seam: skipped a scan)\n02-webster-foundation: done"},
		{"remaining names every batch in execution order for a nil state", websterengine.RenderRemaining(reordered, nil), "03-third, 01-first, 02-second"},
		{"remaining drops terminal batches", websterengine.RenderRemaining(reordered, partial), "01-first, 02-second"},
		{"remaining is none once every batch is terminal", websterengine.RenderRemaining(reordered, allDone), "none"},
		{"batch index follows slice order", websterengine.RenderBatchIndex(reordered), "03 — third — do the third thing\n01 — first — do the first thing\n02 — second — do the second thing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("got %q; want %q", tc.got, tc.want)
			}
		})
	}
}
