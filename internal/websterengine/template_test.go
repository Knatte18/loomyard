// template_test.go pins webster's producer prompt assets (webster-template-master, the composed
// fork/recovery templates, and webster-body-verify-fix) against the Go contracts they key off
// of — the template-parser-co-versioning decision applied here: the master template's digest-field
// bullet list is pinned against webster's own Digest field set and order, the outcome-file bullet
// list against the outcome schema, and the fork/recovery templates' report-schema section against
// the minimal fork-return contract's field set (status, head_sha, deviations) — all as
// literal-statement and exact-field-list assertions, plus stencil.Fill/ FillOptional round-trips proving every
// required marker and RenderForkPrompt/RenderRecoveryPrompt round-trips proving the
// fork-context-hygiene Shared Decision: a thin in-session fork prompt that injects nothing already
// inherited from Master, a full cold-start recovery prompt, and card content delivered by a
// SourcePath pointer rather than inlined fields.
// Every asset is read at call time via stencilstore.Read from a stencils directory this file seeds itself: newTestStencilsDir seeds a t.TempDir() through `stencilkit`, per the runtime-read-not-embed Shared Decision.
// Every test here is untagged and spawn-free: no subprocess exec, no git, no fixture trees (beyond
// a plain t.TempDir() PATTERN.md fixture and the seeded stencils t.TempDir() itself) — only
// on-disk bytes read via stencilstore.Read, stencil.Fill/FillOptional, and
// RenderForkPrompt/RenderRecoveryPrompt/RenderProgress, per the batch's own
// test-tiers-and-hermetic-git decision.

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
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// testCardGates is the placeholder per-card gate list passed as RenderForkPrompt's and RenderRecoveryPrompt's cardGates parameter.
const testCardGates = "- `_lyx/plan/01-gate.md`: `go build ./... && go test ./...`"

// testOutcomePath and testSummaryPath are the placeholder contract-file paths passed as RenderForkPrompt's and RenderVerifyFixPrompt's outcomePath and summaryPath parameters.
const (
	testOutcomePath = "/lyx/webster/outcome.yaml"
	testSummaryPath = "/lyx/webster/summary.md"
)

// testVerifyFixPromptPath is the placeholder fixer-prompt path passed as RenderMasterPrompt's verifyFixPromptPath parameter.
const testVerifyFixPromptPath = "/lyx/webster/prompts/verify-fix.md"

// newTestSpecsDir returns a real, non-empty directory to pass as RenderForkPrompt's/
// RenderRecoveryPrompt's specsDir parameter, alongside newTestStencilsDir. A real directory rather
// than an empty-string placeholder is required: specs_dir is a required marker in the shared
// implementer-job body, so an empty value would compile and pass today only because the body
// carries no marker yet, and would start failing at run time the moment a later batch inserts it.
func newTestSpecsDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// mustMasterTemplate, mustForkTemplate, and mustRecoveryTemplate wrap the matching accessor with a t.Fatalf on error, so call sites unrelated to the error path itself stay terse.
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

// seedHubStencils seeds every registry stencil under hub's real fabricengine.StencilsDir(hub) location, the geometry the render functions derive internally before reading through stencilstore.Read.
func seedHubStencils(t *testing.T, hub string) {
	t.Helper()
	stencilkit.SeedInto(t, fabricengine.StencilsDir(hub))
}

// testLayout returns the told anchor root and stencils directory for a real t.TempDir() hub, seeded
// with webster's five stencils at fabricengine.StencilsDir(hub) — every
// RenderForkPrompt/RenderRecoveryPrompt/RenderMasterPrompt test in this file that does not itself
// exercise pattern_directive's active branch uses this fixture. The returned anchor root's worktree
// subdirectory is never created on disk, so pattern.Directive's stat on the never-existing
// PATTERN.md path always resolves PATTERN inactive, matching every one of these tests'
// pre-existing expectation of an empty pattern_directive.
func testLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	return filepath.Join(hub, "worktree"), fabricengine.StencilsDir(hub)
}

// patternActiveLayout returns the told anchor root and stencils directory for a real t.TempDir() hub
// that contains a real PATTERN.md file directly under the returned root, so pattern.Directive
// returns non-empty — mirroring pattern.Directive's own told-root read (see
// internal/pattern/pattern_test.go's writePatternFile fixture) —
// and seeded with webster's five stencils at fabricengine.StencilsDir(hub) like testLayout.
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

// frictionActiveLayout returns the told anchor root and stencils directory for a real t.TempDir() hub seeded with every registry stencil, including the friction-directive pair a non-empty notePath call needs to resolve a real friction.Directive.
func frictionActiveLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	return filepath.Join(hub, "worktree"), fabricengine.StencilsDir(hub)
}

// patternActiveMissingPatternStencilsLayout returns the told anchor root and stencils directory like patternActiveLayout — PATTERN active, every stencil seeded — but then removes the pattern-directive stencils, so a call site's hoisted pattern.Directive read fails.
func patternActiveMissingPatternStencilsLayout(t *testing.T) (anchorRoot, stencilsDir string) {
	t.Helper()
	anchorRoot, stencilsDir = patternActiveLayout(t)
	stencilkit.Remove(t, stencilsDir, "pattern-directive-implementer", "pattern-directive-review-fix", "pattern-directive-orchestrator")
	return anchorRoot, stencilsDir
}

// stripFrictionMarker rewrites the named webster stencil under stencilsDir, dropping the literal
// "{{.friction_directive}}" line from its bytes — the marker-free-template fixture WarnIfMarkerAbsent
// exists to warn about, used to prove a composer still renders successfully (never errors) when its
// own stencil carries no marker at all.
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

// requireContains fails the test, naming the missing needle, if text does
// not contain it.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// requireNotContains fails the test, naming the forbidden needle, if text
// contains it — the negative half of requireContains, used to pin the
// absence of every dropped batch-era concept (oversized, chain, ## Scope) and every
// concept the fork-context-hygiene Shared Decision moved out of the thin
// fork prompt (## Shared Decisions, ## Rename mechanic).
func requireNotContains(t *testing.T, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Errorf("output unexpectedly contains %q; want it fully removed", needle)
	}
}

// extractBacktickBullets returns, in order, the single backtick-quoted
// token from every "- `token`" bullet line appearing strictly between
// heading (matched by trimmed equality) and the next "## " heading or EOF —
// the shape both the digest-field and outcome-key bullet lists take in
// webster-template-master.
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

// digestSectionHeading and outcomeKeysHeadingSub name the two headings whose
// bullet lists the digest-fields and outcome-schema tests below scope their
// extraction to — never the whole template body, since prose elsewhere
// legitimately backtick-quotes a subset of the same field names without
// that being an "other" field leaking into either pinned set.
const (
	digestSectionHeading  = "## Read ONLY the digest fields — quoted here, exactly"
	outcomeKeysHeadingSub = "`{{.outcome_path}}` itself carries exactly these three keys, quoted here, exactly:"
)

// masterTemplateMarkerValues returns a values map with every one of
// MasterTemplate's seven required top-level markers set to a non-empty
// placeholder, plus pattern_directive and friction_directive — the two
// optional markers, filled via stencil.FillOptional — set to a placeholder
// too, so a test can fill the template cleanly or delete one key at a time
// to prove stencil.FillOptional's per-marker error.
func masterTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":            "01 — json-flag — add the --json flag",
		"progress":               "none",
		"remaining":              "01-a",
		"outcome_path":           "/lyx/webster/outcome.yaml",
		"summary_path":           "/lyx/webster/summary.md",
		"verify_fix_prompt_path": "/lyx/webster/prompts/verify-fix.md",
		"plan_dir":               "_lyx/plan",
		"self_fix_cap":           "2",
		"pattern_directive":      "## Constraints — do this before you fork anything\n\n- Read the overview.",
		"friction_directive":     "## Friction note — optional, only if something went wrong\n\nWrite it to /lyx/webster/friction/webster-master.md.",
		"parent_directive":       "## Your parent\n\nNo parent is recorded for this run.",
		"edit_directive":         "## Editing files\n\nChange files only with Edit or Write.",
	}
}

// forkTemplateMarkerValues returns a values map with every one of the
// composed thin fork template's five required top-level markers set to a
// non-empty placeholder — the fork-context-hygiene Shared Decision's marker
// set: card_pointers replaces the old inlined cards field, and
// shared_decisions/rename_mechanic/pattern_directive are gone entirely
// (nothing already inherited from Master is re-injected) — plus
// friction_directive, the fork template's own one optional marker.
func forkTemplateMarkerValues() map[string]string {
	return map[string]string{
		"card_pointers":      "- `_lyx/plan/02-list-tests.md`",
		"card_gates":         testCardGates,
		"report_path":        "/webster/reports/02-list-tests.yaml",
		"self_fix_cap":       "2",
		"worktree_root":      "/worktree",
		"prev_digest":        "01-json-flag: done head_sha=abc123",
		"specs_dir":          "/hub/repo/_board/_lyx/specs",
		"outcome_path":       testOutcomePath,
		"summary_path":       testSummaryPath,
		"friction_directive": "## Friction note — optional, only if something went wrong\n\nWrite it to /webster/friction/02-list-tests.md.",
	}
}

// recoveryTemplateMarkerValues returns a values map with every one of the
// composed recovery template's five required top-level markers set to a
// non-empty placeholder (mirroring forkTemplateMarkerValues), plus
// pattern_directive and friction_directive — the recovery template's own two
// optional markers.
func recoveryTemplateMarkerValues() map[string]string {
	values := forkTemplateMarkerValues()
	values["pattern_directive"] = "## Constraints — do this before you write any code\n\n- Read the overview."
	values["parent_directive"] = "## Your parent\n\nNo parent is recorded for this run."
	values["edit_directive"] = "## Editing files\n\nChange files only with Edit or Write."
	return values
}

// cardWithSourcePath returns a minimal planparser.Card with SourcePath set
// by hand: the inline fixture batches in this file are hand-built
// batcher.Batch{Cards: [...]} values, not produced by ParsePlan, so
// SourcePath — normally computed by planparser from planparser.PlanDirRel()
// plus the card's own NN-<slug>.md filename — must be set explicitly here.
func cardWithSourcePath(number int, slug, summary string) planparser.Card {
	return planparser.Card{
		Number:     number,
		Slug:       slug,
		Title:      slug,
		Summary:    summary,
		SourcePath: fmt.Sprintf("%s/%02d-%s.md", planparser.PlanDirRel(), number, slug),
	}
}

// assertCardPointerIsRelative fails the test if got does not contain the
// card's own SourcePath, or if that pointer is not worktree-relative (an
// absolute prefix, or a leaked t.TempDir() path, would mean the renderer
// re-composed the pointer against some absolute base instead of rendering
// planparser's own token verbatim).
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

// renderFork renders the fork prompt for batch with the shared placeholder paths; root is both the
// anchor root and the prompt worktree root, and the plan directory is derived under it.
func renderFork(batch batcher.Batch, root, stencilsDir, specsDir, prevDigest string, selfFixCap int, notePath string) (string, error) {
	got, err := websterengine.RenderForkPrompt(batch, testCardGates, prevDigest, "/reports/01-alpha.yaml", filepath.Join(root, "_lyx", "plan"), root, stencilsDir, specsDir, testOutcomePath, testSummaryPath, selfFixCap, notePath)
	return string(got), err
}

// renderRecovery renders the recovery prompt for batch with the shared placeholder paths; the plan
// directory and prompt worktree root derive from anchorRoot, and PATTERN.md is read from repoRoot.
func renderRecovery(batch batcher.Batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, notePath, parent string) (string, error) {
	return renderRecoveryWithUncommitted(batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, "", notePath, parent)
}

// renderRecoveryWithUncommitted is renderRecovery with the caller-rendered uncommitted-paths block.
func renderRecoveryWithUncommitted(batch batcher.Batch, repoRoot, anchorRoot, stencilsDir, specsDir, failureDigest, uncommittedPaths, notePath, parent string) (string, error) {
	got, err := websterengine.RenderRecoveryPrompt(batch, testCardGates, "", failureDigest, uncommittedPaths, "/reports/01-alpha.yaml", repoRoot, filepath.Join(anchorRoot, "_lyx", "plan"), anchorRoot, stencilsDir, specsDir, 2, notePath, parent)
	return string(got), err
}

// renderMaster renders Merriam's prompt for batches with a nil state and the shared placeholder paths.
func renderMaster(batches []batcher.Batch, anchorRoot, repoRoot, stencilsDir, notePath, parent string) (string, error) {
	got, err := websterengine.RenderMasterPrompt(batches, nil, testOutcomePath, testSummaryPath, testVerifyFixPromptPath, filepath.Join(anchorRoot, "_lyx", "plan"), 2, anchorRoot, repoRoot, stencilsDir, notePath, parent)
	return string(got), err
}

// renderVerifyFix renders the fixer prompt for reportPath with the shared placeholder paths.
func renderVerifyFix(reportPath, anchorRoot, stencilsDir, notePath string) (string, error) {
	got, err := websterengine.RenderVerifyFixPrompt(reportPath, anchorRoot, filepath.Join(anchorRoot, "_lyx", "plan"), stencilsDir, testOutcomePath, testSummaryPath, notePath)
	return string(got), err
}

// promptCheck is one rendered prompt and the assertions its case makes on it: wantErr expects the
// render to fail, otherwise every contains needle is present, every notContains needle is absent,
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

// TestMasterTemplate_QuotesBulletLists asserts the master template's digest-field bullet list names
// exactly webster's own six Digest field names (json tags) in the struct's declared order, and its
// outcome-file bullet list names exactly the outcome.yaml schema keys — no fewer, no extras — the
// mechanical half of "Master reads only the minimal fork-return digest".
//
//testtiming:keep pins the master template's digest-field and outcome-key bullet lists matching the Digest fields and outcome schema exactly, in order; the composition test checks markers and sections, not these lists
func TestMasterTemplate_QuotesBulletLists(t *testing.T) {
	t.Parallel()
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	cases := []struct {
		name    string
		heading string
		want    []string
	}{
		{"digest fields", digestSectionHeading, []string{"batch", "status", "head_sha", "deviations", "dead_reason", "elapsed_s"}},
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

// TestTemplates_FillRequiresEveryRequiredMarker asserts stencil.FillOptional succeeds for each of
// the master, composed fork and composed recovery templates when every required marker plus the
// optional ones is supplied, and fails — naming the marker — when any single required one is absent.
// The optional markers are excluded from the deletion sweep: deleting one must not error.
//
//testtiming:keep pins each template refusing a fill missing any single required marker, naming it, and accepting each optional marker absent; the render tests always supply every marker
func TestTemplates_FillRequiresEveryRequiredMarker(t *testing.T) {
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
			required: []string{"batch_index", "progress", "outcome_path", "summary_path", "verify_fix_prompt_path", "plan_dir", "self_fix_cap", "edit_directive"},
		},
		{
			name:     "fork",
			template: mustForkTemplate,
			values:   forkTemplateMarkerValues,
			optional: []string{"friction_directive"},
			required: []string{"card_pointers", "card_gates", "report_path", "self_fix_cap", "worktree_root", "prev_digest", "specs_dir", "outcome_path", "summary_path"},
		},
		{
			name:     "recovery",
			template: mustRecoveryTemplate,
			values:   recoveryTemplateMarkerValues,
			optional: []string{"pattern_directive", "failure_digest", "uncommitted_paths", "friction_directive", "parent_directive"},
			required: []string{"card_pointers", "card_gates", "report_path", "self_fix_cap", "worktree_root", "prev_digest", "specs_dir", "edit_directive"},
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

// TestTemplates_Composition asserts the composed fork and recovery templates carry the shared
// implementer body's exact pre-Fill bytes and no stencil banner, re-read the stencil files on every
// call instead of caching them, and that a missing stencils directory is a hard error naming the
// stencil rather than a fallback to an embedded default.
func TestTemplates_Composition(t *testing.T) {
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
			// Every asset carries a real `lyx-stencil:` banner stamp, so a leak here means the
			// composition stopped stripping the second asset's banner.
			requireNotContains(t, string(composed), "lyx-stencil:")
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

// TestRenderPrompts asserts the rendered fork, recovery, master and fixer prompts: every required
// marker resolves, nothing the fork-context-hygiene Shared Decision moved out of the fork prompt
// leaks back in, the told roots land where they belong, and a missing or empty input fails the
// render instead of producing a blank path.
func TestRenderPrompts(t *testing.T) {
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
				contains:    []string{testCardGates, testOutcomePath, testSummaryPath},
				notContains: []string{"{{.card_gates}}", "this card's package's unit tests"}}
		}},
		{"recovery carries the card gates and no leftover marker", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err,
				contains:    []string{testCardGates},
				notContains: []string{"{{.card_gates}}", "this card's package's unit tests"}}
		}},
		{"fork self-fix section counts card-caused failures", func(t *testing.T) promptCheck {
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
			return promptCheck{text: section, contains: []string{
				"at most `7` in-session fix attempts",
				"in any other test run you make, counts as that card's gate failure when the card's change caused it",
				"the failing test exercises code or files the card changed",
				"a repo scan or enforcement test flags a file the card wrote",
				"under the same `7`-attempt bound as any gate failure, and report `status: FAILED` when the bound runs out",
				"A failure the card did not cause is never yours to fix and never a `FAILED` on its own: name it in your final reply to Master",
			}}
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
				contains:    []string{"00-overview.md", alpha.Cards[0].SourcePath, "## Your final action: the minimal batch-report"},
				notContains: []string{"CONSTRAINTS.md", "{{", "## Constraints"}}
		}},
		{"recovery carries the PATTERN overview when PATTERN is active", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"some constraints", "## Constraints"}}
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
			text, err := renderRecoveryWithUncommitted(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", block, "", "")
			return promptCheck{text: text, err: err, contains: []string{"What the worktree holds\n\n" + block + "\n"}, notContains: []string{"{{.uncommitted_paths}}"}}
		}},
		{"recovery renders none for a clean worktree", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderRecovery(alpha, root, root, stencilsDir, newTestSpecsDir(t), "", "", "")
			return promptCheck{text: text, err: err, contains: []string{"What the worktree holds\n\nnone\n"}}
		}},
		{"master never fills worktree_root", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "")
			return promptCheck{text: text, err: err, notContains: []string{"worktree_root", root}}
		}},
		{"master names the fixer prompt and carries no integration fork", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "")
			return promptCheck{text: text, err: err,
				contains:    []string{testVerifyFixPromptPath, "Gate findings recorded at", "## Verify gate fixes"},
				notContains: []string{"integration fork", "integration.yaml"}}
		}},
		{"master lists the batches in the sequenced order it was handed", func(t *testing.T) promptCheck {
			batches := []batcher.Batch{
				{Cards: []planparser.Card{cardWithSourcePath(2, "second", "do the second thing")}},
				{Cards: []planparser.Card{cardWithSourcePath(1, "first", "do the first thing")}},
			}
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(batches, root, root, stencilsDir, "", "")
			return promptCheck{text: text, err: err, inOrder: []string{"02 — second", "01 — first"}}
		}},
		{"master with PATTERN inactive renders no constraints block", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "")
			return promptCheck{text: text, err: err, notContains: []string{"{{", "## Constraints", "\n\n\n\n"}}
		}},
		{"master places the PATTERN directive ahead of the first work instruction", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveLayout(t)
			text, err := renderMaster(seamBatches, root, root, stencilsDir, "", "")
			return promptCheck{text: text, err: err, inOrder: []string{"some constraints", "## Orientation"}}
		}},
		{"master reads PATTERN.md from the told repo root", func(t *testing.T) promptCheck {
			repoRoot, stencilsDir := patternActiveLayout(t)
			text, err := renderMaster(seamBatches, filepath.Join(repoRoot, "backend"), repoRoot, stencilsDir, "", "")
			return promptCheck{text: text, err: err, contains: []string{"some constraints"}}
		}},
		{"master does not read PATTERN.md from the anchor root", func(t *testing.T) promptCheck {
			repoRoot, stencilsDir := patternActiveLayout(t)
			anchorRoot := filepath.Join(repoRoot, "backend")
			text, err := renderMaster(seamBatches, anchorRoot, anchorRoot, stencilsDir, "", "")
			return promptCheck{text: text, err: err, notContains: []string{"some constraints"}}
		}},
		{"master refuses a missing pattern-directive stencil", func(t *testing.T) promptCheck {
			root, stencilsDir := patternActiveMissingPatternStencilsLayout(t)
			_, err := renderMaster(seamBatches, root, root, stencilsDir, "", "")
			return promptCheck{err: err, wantErr: true}
		}},
		{"fixer prompt names the gate report and the contract files", func(t *testing.T) promptCheck {
			const reportPath = "/lyx/webster/reports/verify-gate.yaml"
			root, stencilsDir := testLayout(t)
			text, err := renderVerifyFix(reportPath, root, stencilsDir, "")
			return promptCheck{text: text, err: err,
				contains:    []string{reportPath, testOutcomePath, testSummaryPath, "`_lyx/plan`"},
				notContains: []string{"{{"}}
		}},
		{"fixer prompt refuses an empty report path", func(t *testing.T) promptCheck {
			root, stencilsDir := testLayout(t)
			_, err := renderVerifyFix("", root, stencilsDir, "")
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

// TestRenderPrompts_FrictionDirective covers every renderer's friction_directive marker: a
// non-empty note path injects the composed directive verbatim, an empty note path composes cleanly
// with no directive and no friction stencil read, and a marker-free stencil still composes
// without error.
func TestRenderPrompts_FrictionDirective(t *testing.T) {
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
			return renderMaster(seamBatches, anchorRoot, anchorRoot, stencilsDir, notePath, "")
		}},
		{"verify-fix", "webster-body-verify-fix", "webster-verify-fix.md", func(anchorRoot, stencilsDir, notePath string) (string, error) {
			return renderVerifyFix("/reports/verify-gate.yaml", anchorRoot, stencilsDir, notePath)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("enabled: a non-empty note path appears in the composed prompt verbatim", func(t *testing.T) {
				t.Parallel()
				anchorRoot, stencilsDir := frictionActiveLayout(t)
				notePath := filepath.Join(anchorRoot, "_lyx", "friction", tc.noteFile)
				text, err := tc.render(anchorRoot, stencilsDir, notePath)
				promptCheck{text: text, err: err, contains: []string{notePath}}.verify(t)
			})

			t.Run("disabled: an empty note path composes cleanly and reads no friction stencil", func(t *testing.T) {
				t.Parallel()
				// testLayout seeds no friction stencils at all, so a read attempt here would fail
				// loud — a clean render proves friction.Directive's empty-notePath early return, not
				// a swallowed error.
				anchorRoot, stencilsDir := testLayout(t)
				text, err := tc.render(anchorRoot, stencilsDir, "")
				promptCheck{text: text, err: err, notContains: []string{"Friction note"}}.verify(t)
			})

			t.Run("marker-free template: a stencil with no {{.friction_directive}} literal still composes", func(t *testing.T) {
				t.Parallel()
				anchorRoot, stencilsDir := frictionActiveLayout(t)
				stripFrictionMarker(t, stencilsDir, tc.stencil)
				notePath := filepath.Join(anchorRoot, "_lyx", "friction", tc.noteFile)
				_, err := tc.render(anchorRoot, stencilsDir, notePath)
				promptCheck{err: err}.verify(t)
			})
		})
	}
}

// TestRenderPrompts_ParentDirective asserts the master and recovery prompts render the parent variant for a told name and the no-parent variant for an empty one, and that the fork prompt carries neither.
func TestRenderPrompts_ParentDirective(t *testing.T) {
	t.Parallel()
	batches := []batcher.Batch{{Cards: []planparser.Card{cardWithSourcePath(1, "alpha", "add the flag")}}}
	anchorRoot, stencilsDir := testLayout(t)
	master := func(parent string) string {
		got, err := renderMaster(batches, anchorRoot, anchorRoot, stencilsDir, "", parent)
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

	const parent = "ab:orch"
	for name, render := range map[string]func(string) string{"master": master, "recovery": recovery} {
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

// TestRenderBatchLists asserts the batch index, progress trail and remaining list render the
// batches in the slice's own order, never ascending batch number, and that progress and remaining
// read only the persisted terminal records, skipping a nil one.
//
//testtiming:keep pins each list's exact text in slice order and its none forms; the covering prompt and run-exit tests check only that a list is present
func TestRenderBatchLists(t *testing.T) {
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
		// A state.json carrying an explicit null for a batch key parses to a present-but-nil entry.
		{"progress skips a nil batch record", websterengine.RenderProgress(inOrder, &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: nil,
			2: {Slug: "webster-foundation", Terminal: true, Status: "done"},
		}}), "02-webster-foundation: done"},
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
