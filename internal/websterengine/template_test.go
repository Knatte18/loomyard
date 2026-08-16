// template_test.go pins webster's producer prompt assets (webster-template-master, the composed
// fork/recovery templates, and webster-template-integration) against the Go contracts they key off
// of — the template-parser-co-versioning decision applied here: the master template's digest-field
// bullet list is pinned against webster's own Digest field set and order, the outcome-file bullet
// list against the outcome schema, and the fork/recovery templates' report-schema section against
// the minimal fork-return contract's field set (status, head_sha, deviations) — all as
// literal-statement and exact-field-list assertions, plus stencil.Fill/ FillOptional round-trips proving every
// required marker and RenderForkPrompt/RenderRecoveryPrompt round-trips proving the
// fork-context-hygiene Shared Decision: a thin in-session fork prompt that injects nothing already
// inherited from Master, a full cold-start recovery prompt, and card content delivered by a
// SourcePath pointer rather than inlined fields.
// Every asset is read at call time via stencilstore.Read from a stencils directory this file seeds
// itself: newTestStencilsDir builds a t.TempDir() from the shipped stencils package defaults, per
// the runtime-read-not-embed Shared Decision.
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newTestStencilsDir builds a t.TempDir() seeded with webster's five stencils, copied byte-for-byte
// from the stencils package's embedded defaults (unstamped), and returns the directory to pass as
// stencilsDir.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	websterDir := filepath.Join(dir, "webster")
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", websterDir, err)
	}
	files := map[string][]byte{
		"webster-template-master.md":      stencils.WebsterTemplateMaster,
		"webster-template-integration.md": stencils.WebsterTemplateIntegration,
		"webster-prefix-fork.md":          stencils.WebsterPrefixFork,
		"webster-prefix-recovery.md":      stencils.WebsterPrefixRecovery,
		"webster-body-implementer.md":     stencils.WebsterBodyImplementer,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(websterDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
	return dir
}

// mustMasterTemplate, mustIntegrationTemplate, mustForkTemplate, and mustRecoveryTemplate wrap the
// matching accessor with a t.Fatalf on error, so call sites unrelated to the error path itself stay
// terse.
func mustMasterTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.MasterTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("MasterTemplate(%q) = _, %v; want nil error", stencilsDir, err)
	}
	return got
}

func mustIntegrationTemplate(t *testing.T, stencilsDir string) []byte {
	t.Helper()
	got, err := websterengine.IntegrationTemplate(stencilsDir)
	if err != nil {
		t.Fatalf("IntegrationTemplate(%q) = _, %v; want nil error", stencilsDir, err)
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

// seedHubWebsterStencils writes webster's five stencils under hub's real
// fabricengine.StencilsDir(hub) location, byte-for-byte from the stencils
// package's embedded defaults — the geometry RenderForkPrompt,
// RenderRecoveryPrompt, and RenderMasterPrompt now derive internally via
// fabricengine.StencilsDir(l.HubPath) before reading through
// stencilstore.Read.
// Split out from seedHubStencils so a missing-stencil error-path test can
// seed webster's five without also seeding the three pattern-directive
// stencils.
func seedHubWebsterStencils(t *testing.T, hub string) {
	t.Helper()
	websterDir := filepath.Join(fabricengine.StencilsDir(hub), "webster")
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", websterDir, err)
	}
	files := map[string][]byte{
		"webster-template-master.md":      stencils.WebsterTemplateMaster,
		"webster-template-integration.md": stencils.WebsterTemplateIntegration,
		"webster-prefix-fork.md":          stencils.WebsterPrefixFork,
		"webster-prefix-recovery.md":      stencils.WebsterPrefixRecovery,
		"webster-body-implementer.md":     stencils.WebsterBodyImplementer,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(websterDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
}

// seedHubPatternStencils writes the three pattern-directive stencils under hub's real
// fabricengine.StencilsDir(hub) location, byte-for-byte from the stencils package's embedded
// defaults — the same on-disk geometry RenderRecoveryPrompt's and RenderMasterPrompt's now-hoisted
// pattern.Directive call reads through stencilstore.Read.
// Split out from seedHubStencils for the same reason as seedHubWebsterStencils.
func seedHubPatternStencils(t *testing.T, hub string) {
	t.Helper()
	patternDir := filepath.Join(fabricengine.StencilsDir(hub), "pattern")
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
	}
	files := map[string][]byte{
		"pattern-directive-implementer.md":  stencils.PatternDirectiveImplementer,
		"pattern-directive-review-fix.md":   stencils.PatternDirectiveReviewFix,
		"pattern-directive-orchestrator.md": stencils.PatternDirectiveOrchestrator,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(patternDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
}

// seedHubStencils seeds hub with webster's five stencils and the three pattern-directive stencils —
// everything both testLayout and patternActiveLayout need, since seeding once here covers both.
func seedHubStencils(t *testing.T, hub string) {
	t.Helper()
	seedHubWebsterStencils(t, hub)
	seedHubPatternStencils(t, hub)
}

// testLayout returns a *lyxcwd.Location rooted at a real t.TempDir() hub,
// seeded with webster's five stencils at fabricengine.StencilsDir(hub) —
// every RenderForkPrompt/RenderRecoveryPrompt/RenderMasterPrompt test in
// this file that does not itself exercise pattern_directive's active branch
// uses this fixture. Its worktree subdirectory is never created on disk, so
// pattern.Directive's os.Stat on the never-existing PATTERN.md path always
// resolves PATTERN inactive, matching every one of these tests'
// pre-existing expectation of an empty pattern_directive.
func testLayout(t *testing.T) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	return &lyxcwd.Location{HubPath: hub, WorktreeName: "worktree", AnchorRel: "."}
}

// patternActiveLayout builds a *lyxcwd.Location rooted at a real
// t.TempDir() hub that contains a real _lyx/PATTERN.md file under its
// worktree subdirectory, so pattern.Directive returns non-empty —
// mirroring pattern.isActive's own PatternFileHere() check (see
// internal/pattern/pattern_test.go's writePatternFile/layoutAt fixtures) —
// and seeded with webster's five stencils at fabricengine.StencilsDir(hub)
// like testLayout.
func patternActiveLayout(t *testing.T) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	seedHubStencils(t, hub)
	dir := filepath.Join(hub, "worktree", "_lyx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte("# PATTERN\n\nsome constraints\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
	return &lyxcwd.Location{HubPath: hub, WorktreeName: "worktree", AnchorRel: "."}
}

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Kept package-local rather than shared, since test-helper
// packages are deliberately not shared across modules.
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
// placeholder, plus pattern_directive — the one optional marker, filled via
// stencil.FillOptional — set to a placeholder too, so a test can fill the
// template cleanly or delete one key at a time to prove
// stencil.FillOptional's per-marker error.
func masterTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":             "01 — json-flag — add the --json flag",
		"progress":                "none",
		"outcome_path":            "/lyx/webster/outcome.yaml",
		"summary_path":            "/lyx/webster/summary.md",
		"integration_prompt_path": "/lyx/webster/prompts/integration.md",
		"self_fix_cap":            "2",
		"poll_wait_s":             "480",
		"pattern_directive":       "## Constraints — do this before you fork anything\n\n- Read _lyx/PATTERN.md.",
	}
}

// forkTemplateMarkerValues returns a values map with every one of the
// composed thin fork template's five required top-level markers set to a
// non-empty placeholder — the fork-context-hygiene Shared Decision's marker
// set: card_pointers replaces the old inlined cards field, and
// shared_decisions/rename_mechanic/pattern_directive are gone entirely
// (nothing already inherited from Master is re-injected).
func forkTemplateMarkerValues() map[string]string {
	return map[string]string{
		"card_pointers": "- `_lyx/plan/02-list-tests.md`",
		"report_path":   "/webster/reports/02-list-tests.yaml",
		"self_fix_cap":  "2",
		"worktree_root": "/worktree",
		"prev_digest":   "01-json-flag: done head_sha=abc123",
	}
}

// recoveryTemplateMarkerValues returns a values map with every one of the
// composed recovery template's five required top-level markers set to a
// non-empty placeholder (mirroring forkTemplateMarkerValues), plus
// pattern_directive — the recovery template's own one optional marker.
func recoveryTemplateMarkerValues() map[string]string {
	values := forkTemplateMarkerValues()
	values["pattern_directive"] = "## Constraints — do this before you write any code\n\n- Read _lyx/PATTERN.md."
	return values
}

// cardWithSourcePath returns a minimal planparser.Card with SourcePath set
// by hand: the inline fixture batches in this file are hand-built
// batcher.Batch{Cards: [...]} values, not produced by ParsePlan, so
// SourcePath — normally computed by planparser from planparser.PlanDirRel()
// plus the card's own NN-<slug>.md filename — must be set explicitly here.
func cardWithSourcePath(number int, slug, intent string) planparser.Card {
	return planparser.Card{
		Number:     number,
		Slug:       slug,
		Title:      slug,
		Intent:     intent,
		SourcePath: fmt.Sprintf("%s/%02d-%s.md", planparser.PlanDirRel(), number, slug),
	}
}

// TestMasterTemplate_QuotesDigestFieldsAndNoOthers asserts the master template's digest-field
// bullet list names exactly webster's own six Digest field names (json tags), in the struct's own
// declared order — no fewer, no extras — the mechanical half of "Master reads only the minimal
// fork-return digest".
func TestMasterTemplate_QuotesDigestFieldsAndNoOthers(t *testing.T) {
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	want := []string{"batch", "status", "head_sha", "deviations", "dead_reason", "elapsed_s"}
	got := extractBacktickBullets(text, digestSectionHeading)

	if len(got) != len(want) {
		t.Fatalf("digest field bullets = %v (%d); want %v (%d)", got, len(got), want, len(want))
	}
	for i, field := range want {
		if got[i] != field {
			t.Errorf("digest field bullet %d = %q; want %q", i, got[i], field)
		}
	}
}

// TestMasterTemplate_QuotesOutcomeSchemaKeys asserts the master template's outcome-file bullet list
// names exactly the three outcome.yaml schema keys, immediately followed by the literal yaml block
// spelling out their values, and separately names summary_path's own "# <title>" first-line rule.
func TestMasterTemplate_QuotesOutcomeSchemaKeys(t *testing.T) {
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, outcomeKeysHeadingSub)

	want := []string{"outcome", "stuck_reason", "batches_done"}
	got := extractBacktickBullets(text, outcomeKeysHeadingSub)
	if len(got) != len(want) {
		t.Fatalf("outcome schema key bullets = %v (%d); want %v (%d)", got, len(got), want, len(want))
	}
	for i, key := range want {
		if got[i] != key {
			t.Errorf("outcome schema key bullet %d = %q; want %q", i, got[i], key)
		}
	}

	requireContains(t, text, "outcome: done | stuck | paused")
	requireContains(t, text, `stuck_reason: null | "<one line>"`)
	requireContains(t, text, "batches_done: <int>")

	requireContains(t, text, "{{.summary_path}}")
	requireContains(t, text, "first line `# <title>`")
}

// TestMasterTemplate_ForbidsLyxGitModelAndNamedSubagents asserts the embedded master template's
// bytes carry the load-bearing never-touch-`_lyx`, never-self-edit, never-/model, and
// never-named-subagent statements in prose, so an edit that silently waters down any one of these
// fails this test rather than only a human review — the Cwd Resolution Invariant's prompt-template
// half plus webster's own fork-discipline bans.
func TestMasterTemplate_ForbidsLyxGitModelAndNamedSubagents(t *testing.T) {
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, "NEVER run any git command against `_lyx`")
	requireContains(t, text, "NEVER edit, create, or delete any file other than")
	requireContains(t, text, "NEVER use a `/model` switch")
	requireContains(t, text, "NEVER spawn a non-fork or named subagent")

	// `_lyx` is read and written as ordinary files — one repo, one worktree —
	// so the positive rule states what Master DOES (read/write `_lyx/...`
	// paths as ordinary files) rather than warning it off a second physical
	// path, and the template must still tell Master what a policy violation
	// MEANS (terminal stuck, never worked around).
	requireContains(t, text, "`_lyx` holds plan and state files")
	requireContains(t, text, "read and write them as ordinary files through `_lyx/...` paths")
	requireContains(t, text, "You never run git against `_lyx`; it is committed for you.")
	requireContains(t, text, "## A policy violation ends your run as stuck")
	requireContains(t, text, "NEVER work around a violation")
	requireContains(t, text, "The audit is")
}

// TestMasterTemplate_GroundsHarnessRealityAgainstInjectionRefusal asserts the master template's
// bytes carry the harness-grounding statements that preempt the observed live spawn-killer (round
// fable-r1, crucible): on current Claude Code, a freshly spawned Master classified the injected
// orchestration prompt as suspicious content, reasoned "no `lyx` tool is in my toolset", and ended
// its turn asking — which the shuttle file contract classifies asking, killing the run (~40% of
// real spawns).
// The template must state that the prompt is real and delivered by `lyx webster run`, that `lyx` is
// a CLI driven via the Bash tool (never a listed tool), and that the session gets its bearings via
// `lyx webster status` rather than ending its turn to ask.
func TestMasterTemplate_GroundsHarnessRealityAgainstInjectionRefusal(t *testing.T) {
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, "get your bearings against the real state on disk")
	requireContains(t, text, "non-interactively by `lyx webster run`")
	requireContains(t, text, "it is an ordinary CLI binary")
	requireContains(t, text, "RUNNING it with your")
	requireContains(t, text, "run `lyx webster status`")
	requireContains(t, text, "confirm the harness, the run state, and the plan are all present")
	requireContains(t, text, "there is no chat partner on the other end")
}

// TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder asserts the embedded template's bytes
// carry every rung of the begin-batch -> fork -> await-batch -> record-batch sequence, verbatim
// prompt forwarding, the backgrounded-fork wait discipline, and the flat-model recovery ladder in
// prose.
func TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder(t *testing.T) {
	text := string(mustMasterTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, "`begin-batch` before every fork")
	requireContains(t, text, `subagent_type: "fork"`)
	requireContains(t, text, "with no name")
	requireContains(t, text, "forwarded verbatim")
	requireContains(t, text, "you are an **IMPLEMENTER")
	requireContains(t, text, "STOP reading this Master prompt")
	requireContains(t, text, "AUTHORITATIVE")
	requireContains(t, text, "never evidence that you are the Master")
	requireContains(t, text, "this instruction is authoritative")

	requireContains(t, text, "BACKGROUNDED agent")
	requireContains(t, text, "call `lyx webster await-batch <NN>`")
	requireContains(t, text, "`await-batch` re-called in the foreground until the report lands")
	requireContains(t, text, "NEVER background it")
	requireContains(t, text, "a turn ended mid-batch kills the whole run")
	requireContains(t, text, "`record-batch` once the fork has delivered")
	requireContains(t, text, "re-call `recover-batch` until terminal")

	requireContains(t, text, "Drive it STRICTLY in order")
	requireContains(t, text, "re-fork the same batch once")
	requireContains(t, text, "SAME prompt file and no new `begin-batch`")
	requireContains(t, text, `"paused": true`)

	requireContains(t, text, "OR `status: dead`")

	requireContains(t, text, "## A fabric-sync error ends your run as stuck")
	requireContains(t, text, "fabric sync")
	requireContains(t, text, "do not retry the verb")

	requireContains(t, text, "already has a report")
	requireContains(t, text, "consume that report")

	requireContains(t, text, "`done` → skip")
	requireContains(t, text, "`stuck` → its fork reported stuck")
	requireContains(t, text, "`dead` → its recovery already failed")

	requireContains(t, text, "there is no")
	requireContains(t, text, "await verb for the integration report")
	requireContains(t, text, "never end your turn")
}

// TestMasterTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds when every one of
// MasterTemplate's seven required markers plus the optional pattern_directive marker is supplied,
// and fails — naming the marker — when any single REQUIRED one is absent.
// pattern_directive is deliberately excluded from this deletion sweep: it is the one optional
// marker (see the template's own banner comment), so deleting it must not error.
func TestMasterTemplate_FillsWithAllMarkers(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.FillOptional(mustMasterTemplate(t, stencilsDir), masterTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"batch_index", "progress", "outcome_path", "summary_path", "integration_prompt_path", "self_fix_cap", "poll_wait_s"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := masterTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.FillOptional(mustMasterTemplate(t, stencilsDir), values, []string{"pattern_directive"})
			if err == nil {
				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestMasterTemplate_PatternDirectiveOptional asserts pattern_directive behaves as an optional
// marker: an empty value renders cleanly with no leftover `{{`, no orphan `## Constraints` heading,
// and no stray blank-line block where the directive would have sat, and a non-empty value places
// the directive block ahead of the first work instruction ("## Orientation").
func TestMasterTemplate_PatternDirectiveOptional(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
		values := masterTemplateMarkerValues()
		values["pattern_directive"] = ""
		got, err := stencil.FillOptional(mustMasterTemplate(t, stencilsDir), values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		if strings.Contains(text, "{{") {
			t.Errorf("rendered output contains leftover {{: %q", text)
		}
		if strings.Contains(text, "## Constraints") {
			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
		}
		if strings.Contains(text, "\n\n\n\n") {
			t.Errorf("rendered output contains a stray blank-line block: %q", text)
		}
	})

	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
		values := masterTemplateMarkerValues()
		got, err := stencil.FillOptional(mustMasterTemplate(t, stencilsDir), values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		directiveIdx := strings.Index(text, values["pattern_directive"])
		workIdx := strings.Index(text, "## Orientation")
		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
		}
	})
}

// TestForkTemplate_PinsReportSchemaKeys asserts the embedded, composed fork template's bytes carry
// the minimal fork-return contract's field names verbatim (status, head_sha, deviations — never
// the superseded report's tests/stuck_reason/out_of_scope grammar) plus the fresh-read rule
// statement and the commit-per-card statement, so a silent edit to any of these fails here rather
// than only a human review.
func TestForkTemplate_PinsReportSchemaKeys(t *testing.T) {
	text := string(mustForkTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, "status:")
	requireContains(t, text, "head_sha:")
	requireContains(t, text, "deviations:")

	requireContains(t, text, "## The FRESH-READ rule")
	requireContains(t, text, "Commit the card to the repo")
	requireContains(t, text, "One commit per card is the norm")

	// The fork inherits Master's loop instructions; it must be told forcefully
	// NOT to drive the webster loop itself.
	requireContains(t, text, "NEVER run any `lyx webster` command")
	requireContains(t, text, "YOU are the one who WRITES that report")

	// The superseded report grammar (done/stuck/tests/out_of_scope) must be gone —
	// the report is deliberately minimal under the flat card-list model.
	requireNotContains(t, text, "out_of_scope:")
	requireNotContains(t, text, "tests: green")
}

// TestForkTemplate_CardLoopReadsCardFileWithWhatFallback asserts the shared implementer-body text —
// reused by both the fork and recovery templates — carries the per-card loop's read-the-card-file
// instruction, the empty-What Card-Index-intent fallback (the empty-What-falls-back-to-the-
// Card-Index-intent Shared Decision), and the Commit-pin-lives-in-the-card- file wording, rather
// than any inlined-block phrasing.
func TestForkTemplate_CardLoopReadsCardFileWithWhatFallback(t *testing.T) {
	text := string(mustForkTemplate(t, newTestStencilsDir(t)))

	requireContains(t, text, "Read the card file")
	requireContains(t, text, "fall back to that card's one-line intent from the Card Index")
	requireContains(t, text, "unless the card FILE carries a `**Commit:**` line")
}

// TestForkTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when every one of the composed
// fork template's five required markers is supplied,
// and fails — naming the marker — when any single one is absent.
// The composed thin fork carries no optional or branch-internal marker at all (shared_decisions,
// rename_mechanic, and pattern_directive are gone, per the fork-context-hygiene Shared Decision),
// so this uses plain stencil.Fill rather than stencil.FillOptional.
func TestForkTemplate_FillsWithAllMarkers(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(mustForkTemplate(t, stencilsDir), forkTemplateMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"card_pointers", "report_path", "self_fix_cap", "worktree_root", "prev_digest"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := forkTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(mustForkTemplate(t, stencilsDir), values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestRecoveryTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds when every one of
// the composed recovery template's five required markers plus the optional pattern_directive marker
// is supplied,
// and fails — naming the marker — when any single REQUIRED one is absent.
// pattern_directive is excluded from the deletion sweep: it is the recovery template's one optional
// marker, so deleting it must not error.
func TestRecoveryTemplate_FillsWithAllMarkers(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.FillOptional(mustRecoveryTemplate(t, stencilsDir), recoveryTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"card_pointers", "report_path", "self_fix_cap", "worktree_root", "prev_digest"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := recoveryTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.FillOptional(mustRecoveryTemplate(t, stencilsDir), values, []string{"pattern_directive"})
			if err == nil {
				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestTemplates_ForkAndRecoveryShareImplementerBody asserts the reuse guarantee behind the
// fork-context-hygiene Shared Decision: both ForkTemplate() and RecoveryTemplate() carry
// ImplementerBodyTemplate()'s exact pre-Fill byte sequence — the shared body's raw template text,
// not a byte-equality of the two renderers' own rendered output, which legitimately diverges on
// per-caller values (card_pointers, prev_digest, and so on).
func TestTemplates_ForkAndRecoveryShareImplementerBody(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	body := mustImplementerBodyTemplate(t, stencilsDir)
	if len(body) == 0 {
		t.Fatalf("ImplementerBodyTemplate() = empty; want non-empty shared body bytes")
	}
	if !bytes.Contains(mustForkTemplate(t, stencilsDir), body) {
		t.Errorf("ForkTemplate() does not contain ImplementerBodyTemplate()'s bytes")
	}
	if !bytes.Contains(mustRecoveryTemplate(t, stencilsDir), body) {
		t.Errorf("RecoveryTemplate() does not contain ImplementerBodyTemplate()'s bytes")
	}
}

// TestTemplates_NoDroppedBatchConceptsRemain asserts neither embedded template carries any of the
// three dropped batch-era concepts — oversized batches, deferred-verify chains, and the per-batch
// "## Scope" section — anywhere in its bytes.
func TestTemplates_NoDroppedBatchConceptsRemain(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	for _, tc := range []struct {
		name string
		text string
	}{
		{"master", string(mustMasterTemplate(t, stencilsDir))},
		{"fork", string(mustForkTemplate(t, stencilsDir))},
		{"recovery", string(mustRecoveryTemplate(t, stencilsDir))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireNotContains(t, strings.ToLower(tc.text), "oversized")
			requireNotContains(t, strings.ToLower(tc.text), "chain")
			requireNotContains(t, tc.text, "## Scope")
		})
	}
}

// TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty asserts RenderForkPrompt renders the
// literal "none (first batch)" sentinel into {{.prev_digest}} when prevDigest is empty (the first
// executed batch's own call site never has a preceding digest to pass),
// and passes a non-empty prevDigest through verbatim otherwise — the fork prompt's cross-batch
// digest context is always Go-rendered from the caller's own persisted value, never re-derived
// here.
func TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty(t *testing.T) {
	batch := batcher.Batch{Cards: []planparser.Card{
		cardWithSourcePath(1, "seam-extensions", "add the seam"),
	}}

	t.Run("empty prevDigest renders the first-batch sentinel", func(t *testing.T) {
		got, err := websterengine.RenderForkPrompt(batch, "", "/reports/01-seam-extensions.yaml", testLayout(t), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "none (first batch)")
	})

	t.Run("non-empty prevDigest passes through verbatim", func(t *testing.T) {
		digest := "01-seam-extensions: done head_sha=abc123"
		got, err := websterengine.RenderForkPrompt(batch, digest, "/reports/02-webster-foundation.yaml", testLayout(t), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), digest)
	})
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

// TestRenderForkPrompt_OmitsSharedDecisions asserts the composed thin fork prompt never carries a
// "## Shared Decisions" section — that plan-level context is already in the fork's inherited Master
// context, per the fork-context-hygiene Shared Decision — and that it DOES carry the card's own
// relative SourcePath pointer.
func TestRenderForkPrompt_OmitsSharedDecisions(t *testing.T) {
	card := cardWithSourcePath(1, "json-flag", "add the --json flag")
	batch := batcher.Batch{Cards: []planparser.Card{card}}

	got, err := websterengine.RenderForkPrompt(batch, "", "/reports/01-json-flag.yaml", testLayout(t), 2)
	if err != nil {
		t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
	}
	text := string(got)

	requireNotContains(t, text, "## Shared Decisions")
	assertCardPointerIsRelative(t, text, card.SourcePath)
}

// TestRenderForkPrompt_OmitsRenameMechanic asserts the composed thin fork prompt never carries a
// "## Rename mechanic" section, regardless of whether the batch has a Moves-bearing card — that
// mechanism, like Shared Decisions, is already in the fork's inherited Master context — and that it
// DOES carry the card's own relative SourcePath pointer.
func TestRenderForkPrompt_OmitsRenameMechanic(t *testing.T) {
	card := cardWithSourcePath(4, "helptree-rename", "rename the row mapper")
	card.Moves = []planparser.MovePair{{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"}}
	batch := batcher.Batch{Cards: []planparser.Card{card}}

	got, err := websterengine.RenderForkPrompt(batch, "", "/reports/04-helptree-rename.yaml", testLayout(t), 2)
	if err != nil {
		t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
	}
	text := string(got)

	requireNotContains(t, text, "## Rename mechanic")
	assertCardPointerIsRelative(t, text, card.SourcePath)
}

// TestRenderRecoveryPrompt_InstructsColdOrientation asserts RenderRecoveryPrompt's rendered prompt
// points the cold recovery strand at `00-overview.md` and `CONSTRAINTS.md`, carries the card's own
// SourcePath pointer and the shared implementer-body text, and — for the PATTERN-active case — also
// names `_lyx/PATTERN.md` via the injected pattern_directive.
// The PATTERN-inactive case renders cleanly: no leftover `{{`, no orphan `## Constraints` heading.
func TestRenderRecoveryPrompt_InstructsColdOrientation(t *testing.T) {
	card := cardWithSourcePath(1, "alpha", "add the flag")
	batch := batcher.Batch{Cards: []planparser.Card{card}}

	t.Run("PATTERN inactive", func(t *testing.T) {
		got, err := websterengine.RenderRecoveryPrompt(batch, "", "/reports/01-alpha.yaml", testLayout(t), 2)
		if err != nil {
			t.Fatalf("RenderRecoveryPrompt() = _, %v; want nil error", err)
		}
		text := string(got)

		requireContains(t, text, "00-overview.md")
		requireContains(t, text, "CONSTRAINTS.md")
		requireContains(t, text, card.SourcePath)
		requireContains(t, text, "## Your final action: the minimal batch-report")

		if strings.Contains(text, "{{") {
			t.Errorf("rendered output contains leftover {{: %q", text)
		}
		if strings.Contains(text, "## Constraints") {
			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
		}
	})

	t.Run("PATTERN active", func(t *testing.T) {
		l := patternActiveLayout(t)
		got, err := websterengine.RenderRecoveryPrompt(batch, "", "/reports/01-alpha.yaml", l, 2)
		if err != nil {
			t.Fatalf("RenderRecoveryPrompt() = _, %v; want nil error", err)
		}
		text := string(got)

		requireContains(t, text, "_lyx/PATTERN.md")
		requireContains(t, text, "## Constraints")
	})
}

// patternActiveMissingPatternStencilsLayout builds a *lyxcwd.Location like patternActiveLayout —
// PATTERN active, webster's five stencils seeded via seedHubWebsterStencils — but deliberately omits
// the three pattern-directive stencils, so a call site's hoisted pattern.Directive read fails.
func patternActiveMissingPatternStencilsLayout(t *testing.T) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	seedHubWebsterStencils(t, hub)
	dir := filepath.Join(hub, "worktree", "_lyx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte("# PATTERN\n\nsome constraints\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
	return &lyxcwd.Location{HubPath: hub, WorktreeName: "worktree", AnchorRel: "."}
}

// TestRenderRecoveryPrompt_MissingPatternStencilErrors asserts RenderRecoveryPrompt's hoisted
// pattern.Directive call propagates a non-nil error when PATTERN is active but the pattern-directive
// stencils are absent, rather than dropping the error the hoist out of the map literal made
// checkable in the first place.
func TestRenderRecoveryPrompt_MissingPatternStencilErrors(t *testing.T) {
	card := cardWithSourcePath(1, "alpha", "add the flag")
	batch := batcher.Batch{Cards: []planparser.Card{card}}
	l := patternActiveMissingPatternStencilsLayout(t)

	if _, err := websterengine.RenderRecoveryPrompt(batch, "", "/reports/01-alpha.yaml", l, 2); err == nil {
		t.Fatal("RenderRecoveryPrompt() error = nil; want a non-nil error for a missing pattern-directive stencil")
	}
}

// TestRenderMasterPrompt_MissingPatternStencilErrors asserts RenderMasterPrompt's hoisted
// pattern.Directive call propagates a non-nil error when PATTERN is active but the pattern-directive
// stencils are absent, rather than dropping the error the hoist out of the map literal made
// checkable in the first place.
func TestRenderMasterPrompt_MissingPatternStencilErrors(t *testing.T) {
	plan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "seam-extensions"}}}
	l := patternActiveMissingPatternStencilsLayout(t)

	if _, err := websterengine.RenderMasterPrompt(plan, nil, "/lyx/webster/outcome.yaml", "/lyx/webster/summary.md", "", 2, 480, l); err == nil {
		t.Fatal("RenderMasterPrompt() error = nil; want a non-nil error for a missing pattern-directive stencil")
	}
}

// TestRenderIntegrationPrompt_InjectsVerifyText asserts RenderIntegrationPrompt injects the plan's
// own plan-level "## verify:" text (plan.Verify) into the rendered integration-suite fork prompt
// verbatim,
// and that an empty plan.Verify is refused loud rather than papered over with a sentinel — a caller
// must gate this call on ShouldRunIntegration first.
func TestRenderIntegrationPrompt_InjectsVerifyText(t *testing.T) {
	plan := &planparser.Plan{Verify: "go test ./internal/boardcli/... ./cmd/lyx/..."}

	got, err := websterengine.RenderIntegrationPrompt(plan, "/reports/integration.yaml", "/worktree", newTestStencilsDir(t))
	if err != nil {
		t.Fatalf("RenderIntegrationPrompt() = _, %v; want nil error", err)
	}
	text := string(got)
	requireContains(t, text, plan.Verify)
	requireNotContains(t, text, "## Shared Decisions")
}

// TestRenderIntegrationPrompt_EmptyVerifyErrors asserts RenderIntegrationPrompt refuses loud on a
// plan with no plan-level verify, rather than silently rendering a prompt with an empty verify
// command.
func TestRenderIntegrationPrompt_EmptyVerifyErrors(t *testing.T) {
	plan := &planparser.Plan{Verify: ""}

	if _, err := websterengine.RenderIntegrationPrompt(plan, "/reports/integration.yaml", "/worktree", newTestStencilsDir(t)); err == nil {
		t.Fatalf("RenderIntegrationPrompt() error = nil; want an error for a plan with no plan-level verify")
	}
}

// TestIntegrationTemplate_ForbidsPollingForOwnReport asserts both the integration fork's own
// template AND the master template's spawn directive carry the anti-poll clause: an integration
// fork that inherits Master's own "poll for the integration report" loop and continues it — instead
// of running the verify and writing that report itself — deadlocks the run via plain shell polls
// the lyx-webster fork hook cannot see.
func TestIntegrationTemplate_ForbidsPollingForOwnReport(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)

	integration := string(mustIntegrationTemplate(t, stencilsDir))
	requireContains(t, integration, "NEVER poll or wait for the integration")
	requireContains(t, integration, "YOU are the one who WRITES")

	master := string(mustMasterTemplate(t, stencilsDir))
	requireContains(t, master, "you do NOT poll or wait for any report file")
	requireContains(t, master, "Your FIRST action is to Read this file")
}

// TestIntegrationTemplate_CarriesNoPerCardOrCommitInstructions asserts the embedded integration
// template's bytes carry no per-card or commit instructions of any kind: the integration fork runs
// the plan-level verify ONCE and makes NO commit, unlike a batch's own fork template.
func TestIntegrationTemplate_CarriesNoPerCardOrCommitInstructions(t *testing.T) {
	text := string(mustIntegrationTemplate(t, newTestStencilsDir(t)))

	requireNotContains(t, text, "**Commit:**")
	requireNotContains(t, text, "One commit per card")
	requireNotContains(t, text, "{{.cards}}")
	requireContains(t, text, "implement NO cards")
	requireContains(t, text, "make NO commit")
}

// TestTemplates_ComposedOutputCarriesNoBannerLeak is the regression guard for the hazard this
// batch's joinTemplateAssets fix closes: it seeds the stencils directory through
// stencilstore.Reconcile, so every one of webster's five assets carries a real `lyx-stencil:` stamp
// in its banner, then asserts ForkTemplate and RecoveryTemplate's output contains no `lyx-stencil:`
// substring and no `<!--` at all — the assertion that fails if joinTemplateAssets ever stops
// stripping the second asset's banner.
func TestTemplates_ComposedOutputCarriesNoBannerLeak(t *testing.T) {
	dir := t.TempDir()
	if _, err := stencilstore.Reconcile(dir, stencils.Registry(), stencilstore.ModeProduction, ""); err != nil {
		t.Fatalf("stencilstore.Reconcile(%q) = %v; want nil error", dir, err)
	}

	fork := string(mustForkTemplate(t, dir))
	requireNotContains(t, fork, "lyx-stencil:")
	requireNotContains(t, fork, "<!--")

	recovery := string(mustRecoveryTemplate(t, dir))
	requireNotContains(t, recovery, "lyx-stencil:")
	requireNotContains(t, recovery, "<!--")
}

// TestTemplates_ComposedReadsReflectOnDiskEdits asserts the composed reads are genuinely runtime
// reads, not a cached copy: overwriting webster-prefix-fork.md changes only ForkTemplate's output,
// and overwriting webster-body-implementer.md — the body shared by both composed prompts — changes
// BOTH ForkTemplate's and RecoveryTemplate's output, since three files participate in two composed
// prompts.
func TestTemplates_ComposedReadsReflectOnDiskEdits(t *testing.T) {
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
}

// TestMasterTemplate_MissingBoardIsAHardError asserts MasterTemplate returns an error naming the
// missing stencil when the stencils directory does not exist, rather than falling back to the
// embedded default — the missing-board-is-a-hard-error Shared Decision.
func TestMasterTemplate_MissingBoardIsAHardError(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := websterengine.MasterTemplate(missingDir)
	if err == nil {
		t.Fatalf("MasterTemplate(%q) error = nil; want an error naming the missing stencil", missingDir)
	}
	requireContains(t, err.Error(), "webster-template-master")
}

// TestRenderProgress_ListsOnlyTerminalBatches asserts RenderProgress lists exactly the batches
// whose persisted BatchState is Terminal, one "NN-slug: status" line per batch in plan order,
// omitting any batch with no BatchState entry yet or one recorded but not yet terminal — never
// re-parsing a report file, only ever reading the persisted record.
func TestRenderProgress_ListsOnlyTerminalBatches(t *testing.T) {
	plan := &planparser.Plan{
		Cards: []planparser.Card{
			{Number: 1, Slug: "seam-extensions"},
			{Number: 2, Slug: "webster-foundation"},
			{Number: 3, Slug: "webster-audit-policy"},
			{Number: 4, Slug: "webster-templates"},
		},
	}

	t.Run("nil state renders none", func(t *testing.T) {
		if got := websterengine.RenderProgress(plan, nil); got != "none" {
			t.Errorf("RenderProgress(plan, nil) = %q; want %q", got, "none")
		}
	})

	t.Run("no terminal batches renders none", func(t *testing.T) {
		st := &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: false},
		}}
		if got := websterengine.RenderProgress(plan, st); got != "none" {
			t.Errorf("RenderProgress(plan, st) = %q; want %q", got, "none")
		}
	})

	t.Run("mixed terminal and in-flight batches", func(t *testing.T) {
		st := &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "seam-extensions", Terminal: true, Status: "done"},
			2: {Slug: "webster-foundation", Terminal: true, Status: "stuck"},
			3: {Slug: "webster-audit-policy", Terminal: false},
			// Batch 4 has no BatchState entry at all yet.
		}}

		want := "01-seam-extensions: done\n02-webster-foundation: stuck"
		if got := websterengine.RenderProgress(plan, st); got != want {
			t.Errorf("RenderProgress(plan, st) = %q; want %q", got, want)
		}
	})
}
