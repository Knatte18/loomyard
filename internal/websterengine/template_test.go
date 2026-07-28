// template_test.go pins webster's two embedded prompt templates
// (master-template.md, fork-template.md) against the Go contracts they
// key off of — the template-parser-co-versioning decision applied here:
// the master template's digest-field bullet list is pinned against
// webster's own Digest field set and order, the outcome-file bullet list
// against the outcome schema, and the fork template's report-schema
// section against the minimal fork-return contract's field set (status,
// head_sha, deviations) — all as literal-statement and exact-field-list
// assertions in the same style as builderengine/template_test.go, plus
// stencil.Fill round-trips proving every required marker and
// RenderForkPrompt round-trips proving the plan-level context injection
// (## Shared Decisions always, ## Rename mechanic only for a Moves-bearing
// batch). Every test here is untagged and spawn-free: no subprocess exec,
// no git, no fixture trees — only embedded bytes, stencil.Fill, and
// RenderForkPrompt/RenderProgress, per the batch's own
// test-tiers-and-hermetic-git decision.

package websterengine_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// testLayout returns a *hubgeometry.Layout anchored at a fixed, non-existent
// "/worktree" path — every RenderForkPrompt test in this file that does not
// itself exercise pattern_directive uses this fixture, since
// pattern.Directive's os.Stat on a path that never exists on disk always
// resolves PATTERN inactive, matching every one of these tests' pre-existing
// expectation of an empty pattern_directive.
func testLayout() *hubgeometry.Layout {
	return &hubgeometry.Layout{Cwd: "/worktree", WorktreeRoot: "/worktree", RelPath: "."}
}

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Mirrors builderengine/template_test.go's helper of the
// same name (package-local — the two packages deliberately do not share a
// test-helper package).
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// requireNotContains fails the test, naming the forbidden needle, if text
// contains it — the negative half of requireContains, used to pin the
// absence of every dropped v2 concept (oversized, chain, ## Scope).
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
// master-template.md. Mirrors builderengine/template_test.go's helper of
// the same name.
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

// extractSection returns the trimmed text strictly between heading (matched
// by trimmed equality) and the next "## " heading or EOF — used to isolate
// one rendered section's own body from the rest of the prompt for an
// exact-content assertion.
func extractSection(text, heading string) string {
	lines := strings.Split(text, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
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
// placeholder, so a test can fill the template cleanly or delete one key at
// a time to prove stencil.Fill's per-marker error.
func masterTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":             "01 — json-flag — add the --json flag",
		"progress":                "none",
		"outcome_path":            "/lyx/webster/outcome.yaml",
		"summary_path":            "/lyx/webster/summary.md",
		"integration_prompt_path": "/lyx/webster/prompts/integration.md",
		"self_fix_cap":            "2",
		"poll_wait_s":             "480",
	}
}

// forkTemplateMarkerValues returns a values map with every one of
// ForkTemplate's six required top-level markers set to a non-empty
// placeholder, mirroring masterTemplateMarkerValues, plus rename_mechanic
// set to the empty string: the one branch-internal marker, reached only
// inside the template's own {{if .rename_mechanic}} block — present (so the
// condition itself evaluates cleanly under stencil's missingkey=error) but
// empty (so the branch is not taken), exactly RenderForkPrompt's own
// non-Moves-batch behavior. pattern_directive is the one top-level OPTIONAL
// marker (filled via stencil.FillOptional, distinct from rename_mechanic's
// branch-internal mechanism) and is set to a non-empty placeholder here too.
func forkTemplateMarkerValues() map[string]string {
	return map[string]string{
		"cards":             "### Card 2 — list-tests\n\n**What:** add tests\n**Context:** none\n**Edits:**\n- `list_test.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none",
		"report_path":       "/webster/reports/02-list-tests.yaml",
		"self_fix_cap":      "2",
		"worktree_root":     "/worktree",
		"prev_digest":       "01-json-flag: done head_sha=abc123",
		"shared_decisions":  "none",
		"rename_mechanic":   "",
		"pattern_directive": "## Constraints — do this before you write any code\n\n- Read _pattern/PATTERN.md.",
	}
}

// TestMasterTemplate_QuotesDigestFieldsAndNoOthers asserts the master
// template's digest-field bullet list names exactly webster's own six
// Digest field names (json tags), in the struct's own declared order — no
// fewer, no extras — the mechanical half of "Master reads only the minimal
// fork-return digest".
func TestMasterTemplate_QuotesDigestFieldsAndNoOthers(t *testing.T) {
	text := string(websterengine.MasterTemplate())

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

// TestMasterTemplate_QuotesOutcomeSchemaKeys asserts the master template's
// outcome-file bullet list names exactly the three outcome.yaml schema keys,
// immediately followed by the literal yaml block spelling out their values,
// and separately names summary_path's own "# <title>" first-line rule.
func TestMasterTemplate_QuotesOutcomeSchemaKeys(t *testing.T) {
	text := string(websterengine.MasterTemplate())

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

// TestMasterTemplate_ForbidsWeftGitModelAndNamedSubagents asserts the
// embedded master template's bytes carry the load-bearing never-touch-the-
// weft, never-self-edit, never-/model, and never-named-subagent statements
// in prose, so an edit that silently waters down any one of these fails
// this test rather than only a human review — the Weft Git Invariant's
// prompt-template half plus webster's own fork-discipline bans.
func TestMasterTemplate_ForbidsWeftGitModelAndNamedSubagents(t *testing.T) {
	text := string(websterengine.MasterTemplate())

	requireContains(t, text, "NEVER run any git command against the weft")
	requireContains(t, text, "NEVER edit, create, or delete any file other than")
	requireContains(t, text, "NEVER use a `/model` switch")
	requireContains(t, text, "NEVER spawn a non-fork or named subagent")

	// The weft ban must cover read-only references too — a Master's
	// orientation `find` on the physical weft path permanently wedged
	// record-batch's whole-session audit in round fable-r1 (F3): the ban the
	// audit enforces and the ban the template states must be the same ban,
	// and the template must tell Master what a violation MEANS (terminal
	// stuck, never worked around).
	requireContains(t, text, "NEVER reference that physical weft path in ANY command")
	requireContains(t, text, "not even a read-only `ls`, `find`, `cat`, or `readlink`")
	requireContains(t, text, "The `_lyx` path is your one sanctioned window")
	requireContains(t, text, "## A policy violation ends your run as stuck")
	requireContains(t, text, "NEVER work around a violation")
	requireContains(t, text, "The audit is")
}

// TestMasterTemplate_GroundsHarnessRealityAgainstInjectionRefusal asserts
// the master template's bytes carry the harness-grounding statements that
// preempt the observed live spawn-killer (round fable-r1, crucible): on
// current Claude Code, a freshly spawned Master classified the injected
// orchestration prompt as suspicious content, reasoned "no `lyx` tool is
// in my toolset", and ended its turn asking — which the shuttle file
// contract classifies asking, killing the run (~40% of real spawns). The
// template must state that the prompt is real and delivered by `lyx
// webster run`, that `lyx` is a CLI driven via the Bash tool (never a
// listed tool), and that the session gets its bearings via `lyx webster
// status` rather than ending its turn to ask.
//
// The grounding is deliberately EVIDENCE-FIRST and NEUTRAL, not defensive:
// round opus-r2 (crucible) live-observed a Master quote the prior fix's own
// anti-refusal phrasing — an imperative "never by asking" plus "no human
// reads this session, so a clarifying question is never answered; it just
// ends the run as failed" — back as the very injection tell that made it
// refuse ("a manipulation tactic, not something a legitimate harness needs
// to say"). A block that preempts the "is this real?" doubt and forbids
// asking pattern-matches the exact move an injection would make. So the
// grounding leads with orientation (the two read-only checks and what they
// prove) and states the non-interactive execution mode as a plain fact
// (there is no chat partner to field a question), never as a prohibition on
// asking or a warning about fabricated/injected content.
func TestMasterTemplate_GroundsHarnessRealityAgainstInjectionRefusal(t *testing.T) {
	text := string(websterengine.MasterTemplate())

	requireContains(t, text, "get your bearings against the real state on disk")
	requireContains(t, text, "non-interactively by `lyx webster run`")
	requireContains(t, text, "it is an ordinary CLI binary")
	requireContains(t, text, "RUNNING it with your")
	requireContains(t, text, "run `lyx webster status`")
	// Evidence-first is the load-bearing part: a skeptical model is handed a
	// concrete first move (read-only orientation) and told exactly what the
	// two checks prove, so it reaches "proceed" from verifiable facts rather
	// than being asked to trust the document.
	requireContains(t, text, "confirm the harness, the run state, and the plan are all present")
	// The non-interactive mode is stated as a neutral operational fact, never
	// as an imperative "never ask": a message that forbids asking is itself
	// what round opus-r2 saw quoted back as an injection tell.
	requireContains(t, text, "there is no chat partner on the other end")
}

// TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder asserts the
// embedded template's bytes carry every rung of the begin-batch -> fork ->
// await-batch -> record-batch sequence, verbatim prompt forwarding, the
// backgrounded-fork wait discipline (forks return immediately on current
// Claude Code, so Master must block on await-batch instead of ever ending
// its turn mid-batch — round fable-r1's F1), and the flat-model recovery
// ladder (no_report re-fork-once with the same prompt file, stuck ->
// recover-batch, running -> re-call until terminal) in prose. There is no
// deferred-verify chain rung any more (see TestTemplates_NoV2TokensRemain).
func TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder(t *testing.T) {
	text := string(websterengine.MasterTemplate())

	requireContains(t, text, "`begin-batch` before every fork")
	requireContains(t, text, `subagent_type: "fork"`)
	requireContains(t, text, "with no name")
	requireContains(t, text, "forwarded verbatim")
	// The master prompt is inherited by every fork, so it must disambiguate
	// role up front and send an inheriting fork away — otherwise the fork
	// runs Master's await-batch loop and deadlocks (round fable-r1 live).
	requireContains(t, text, "you are an **IMPLEMENTER")
	requireContains(t, text, "STOP reading this Master prompt")
	// The banner must also preempt the observed live failure mode: a fork
	// reasoning "this fork instruction contradicts my inherited history, so I
	// must be the Master" and dismissing the spawn instruction (round
	// fable-r3 live — both forks of a run did exactly this and forged
	// Master's contract files).
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

	// A terminal dead recovery result must have an explicit ladder rung —
	// otherwise Master improvises (round fable-r1's F10). Both stuck and dead
	// recovery terminals end the run stuck.
	requireContains(t, text, "OR `status: dead`")

	// A bracket-verb weft-sync error must have its own explicit rung — without
	// it Master improvises and retries the verb, double-committing (round
	// opus-r2 live). It ends the run stuck; the run is resumable.
	requireContains(t, text, "weft sync")
	requireContains(t, text, "do not retry the verb")

	// A resumed run over a crash that landed between the fork's report and
	// record-batch finds the report already on disk; without an explicit rung
	// Master exhausts all three verbs and declares the finished batch stuck
	// (round fable-r3 live). The rung routes it to record-batch (fork batch)
	// or recover-batch (recovery batch).
	requireContains(t, text, "already has a report")
	requireContains(t, text, "consume that report")

	// The progress trail lists EVERY terminal status (RenderProgress renders
	// done, stuck, and dead alike), so the read rule must branch by status —
	// a blanket "skip every reported batch" would skip a half-built stuck
	// batch and build the rest of the plan on top of it (round fable-r3).
	requireContains(t, text, "`done` → skip")
	requireContains(t, text, "`stuck` → its fork reported stuck")
	requireContains(t, text, "`dead` → its recovery already failed")

	// The integration report has no await verb — the template must spell out
	// the concrete bounded foreground poll, or a literal-minded Master has no
	// defined wait mechanism for the integration fork (crucible round
	// fable-r1's F15: "the same way you wait for a batch's own report"
	// pointed at await-batch, which cannot target integration.yaml).
	requireContains(t, text, "there is no")
	requireContains(t, text, "await verb for the integration report")
	requireContains(t, text, "never end your turn")
}

// TestMasterTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when
// every one of MasterTemplate's six required markers is supplied, and fails
// — naming the marker — when any single one is absent.
func TestMasterTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(websterengine.MasterTemplate(), masterTemplateMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"batch_index", "progress", "outcome_path", "summary_path", "integration_prompt_path", "self_fix_cap", "poll_wait_s"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := masterTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(websterengine.MasterTemplate(), values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestForkTemplate_PinsReportSchemaKeys asserts the embedded fork template's
// bytes carry the minimal fork-return contract's field names verbatim
// (status, head_sha, deviations — never the v2 report's tests/stuck_reason/
// out_of_scope grammar) plus the fresh-read rule statement and the
// host-commit-per-card statement, so a silent edit to any of these fails
// here rather than only a human review.
func TestForkTemplate_PinsReportSchemaKeys(t *testing.T) {
	text := string(websterengine.ForkTemplate())

	requireContains(t, text, "status:")
	requireContains(t, text, "head_sha:")
	requireContains(t, text, "deviations:")

	requireContains(t, text, "## The FRESH-READ rule")
	requireContains(t, text, "Commit the card to the HOST repo")
	requireContains(t, text, "One commit per card is the norm")

	// The fork inherits Master's loop instructions; it must be told forcefully
	// NOT to drive the webster loop itself (a fork that polls await-batch for
	// its own report deadlocks — nobody writes it; found live in round
	// fable-r1's first clean await-batch drive).
	requireContains(t, text, "NEVER run any `lyx webster` command")
	requireContains(t, text, "YOU are the one who WRITES that report")

	// The v2 report grammar (done/stuck/tests/out_of_scope) must be gone —
	// the report is deliberately minimal under the flat card-list model.
	requireNotContains(t, text, "out_of_scope:")
	requireNotContains(t, text, "tests: green")
}

// TestForkTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds
// when every one of ForkTemplate's six required markers plus the optional
// pattern_directive marker is supplied, and fails — naming the marker —
// when any single REQUIRED one is absent. rename_mechanic and
// pattern_directive are both excluded from this deletion sweep, via two
// different mechanisms: rename_mechanic is a branch-internal marker, never
// required at the top level (see
// TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch);
// pattern_directive is the one top-level OPTIONAL marker, exempted via
// stencil.FillOptional's optional list, so deleting it must not error
// either.
func TestForkTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.FillOptional(websterengine.ForkTemplate(), forkTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"cards", "report_path", "self_fix_cap", "worktree_root", "prev_digest", "shared_decisions"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := forkTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
			if err == nil {
				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestForkTemplate_PatternDirectiveOptional asserts pattern_directive
// behaves as an optional marker: an empty value renders cleanly with no
// leftover `{{`, no orphan `## Constraints` heading, and no stray
// blank-line block where the directive would have sat, and a non-empty
// value places the directive block ahead of the first work instruction
// ("## You are the IMPLEMENTER"). A third case pins this file's own extra
// requirement: an empty pattern_directive together with an empty
// rename_mechanic renders with neither an orphan `## Constraints` heading
// nor an orphan `## Rename mechanic` heading.
func TestForkTemplate_PatternDirectiveOptional(t *testing.T) {
	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
		values := forkTemplateMarkerValues()
		values["pattern_directive"] = ""
		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
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
		// Scope the stray-blank-line check to the region immediately
		// preceding the first work heading — the pattern_directive
		// insertion point — rather than the whole document: this file's
		// own pre-existing {{if .rename_mechanic}} block (unrelated to
		// pattern_directive) collapses to a wider gap of its own when
		// rename_mechanic is empty, which is not this test's concern.
		beforeHeading := text[:strings.Index(text, "## You are the IMPLEMENTER")]
		if strings.HasSuffix(beforeHeading, "\n\n\n\n") {
			t.Errorf("rendered output contains a stray blank-line block ahead of the first work heading: %q", beforeHeading)
		}
	})

	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
		values := forkTemplateMarkerValues()
		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		directiveIdx := strings.Index(text, values["pattern_directive"])
		workIdx := strings.Index(text, "## You are the IMPLEMENTER")
		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
		}
	})

	t.Run("empty pattern_directive and empty rename_mechanic together leave neither heading orphaned", func(t *testing.T) {
		values := forkTemplateMarkerValues()
		values["pattern_directive"] = ""
		values["rename_mechanic"] = ""
		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
		}
		text := string(got)
		if strings.Contains(text, "## Constraints") {
			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
		}
		if strings.Contains(text, "## Rename mechanic") {
			t.Errorf("rendered output contains an orphan ## Rename mechanic heading: %q", text)
		}
	})
}

// TestTemplates_NoV2TokensRemain asserts neither embedded template carries
// any of the three dropped v2 concepts — oversized batches, deferred-verify
// chains, and the per-batch "## Scope" section — anywhere in its bytes.
func TestTemplates_NoV2TokensRemain(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{"master", string(websterengine.MasterTemplate())},
		{"fork", string(websterengine.ForkTemplate())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireNotContains(t, strings.ToLower(tc.text), "oversized")
			requireNotContains(t, strings.ToLower(tc.text), "chain")
			requireNotContains(t, tc.text, "## Scope")
		})
	}
}

// testPlan returns a minimal *planparser.Plan carrying sharedDecisions and
// renameMechanic as its plan-level sections, for RenderForkPrompt tests.
func testPlan(sharedDecisions, renameMechanic string) *planparser.Plan {
	return &planparser.Plan{
		SharedDecisions: sharedDecisions,
		RenameMechanic:  renameMechanic,
	}
}

// TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty asserts
// RenderForkPrompt renders the literal "none (first batch)" sentinel into
// {{.prev_digest}} when prevDigest is empty (the first executed batch's own
// call site never has a preceding digest to pass), and passes a non-empty
// prevDigest through verbatim otherwise — the fork prompt's cross-batch
// digest context is always Go-rendered from the caller's own persisted
// value, never re-derived here.
func TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty(t *testing.T) {
	plan := testPlan("", "")
	batch := batcher.Batch{Cards: []planparser.Card{
		{Number: 1, Slug: "seam-extensions", Title: "seam-extensions", Intent: "add the seam"},
	}}

	t.Run("empty prevDigest renders the first-batch sentinel", func(t *testing.T) {
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-seam-extensions.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "none (first batch)")
	})

	t.Run("non-empty prevDigest passes through verbatim", func(t *testing.T) {
		// The template's own prose explains the sentinel's literal text
		// (see fork-template.md's "Prior-batch context" section), so its
		// presence in the rendered output is expected regardless of
		// prevDigest; what this case actually proves is that the supplied
		// digest line itself reaches the rendered prompt verbatim.
		digest := "01-seam-extensions: done head_sha=abc123"
		got, err := websterengine.RenderForkPrompt(plan, batch, digest, "/reports/02-webster-foundation.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), digest)
	})
}

// TestRenderForkPrompt_RendersWhatProseOverIntent asserts the rendered card
// block's "**What:**" carries the card file's own What prose — the concrete
// implementer instruction — and falls back to the index Intent only when the
// card carries no prose. A cold recovery strand inherits no session context,
// so the rendered prompt is its whole instruction; rendering the one-line
// Intent under the "**What:**" label silently dropped the real specification
// (found in crucible round fable-r3).
func TestRenderForkPrompt_RendersWhatProseOverIntent(t *testing.T) {
	plan := testPlan("", "")

	t.Run("What prose wins over Intent", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{{
			Number: 1, Slug: "alpha", Title: "alpha",
			Intent: "create r3a.md marker",
			What:   "Create `r3a.md` containing the single line `OK-A` and commit it.",
		}}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "**What:** Create `r3a.md` containing the single line `OK-A` and commit it.")
		if strings.Contains(string(got), "**What:** create r3a.md marker") {
			t.Errorf("rendered prompt carries the index Intent under **What:** despite the card having prose")
		}
	})

	t.Run("empty What falls back to Intent", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{{
			Number: 1, Slug: "alpha", Title: "alpha", Intent: "create r3a.md marker",
		}}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "**What:** create r3a.md marker")
	})
}

// TestRenderForkPrompt_RendersPinnedCommitSubject asserts a card's pinned
// Commit: subject reaches the rendered card block — the implementer only
// ever sees this block, so an unrendered pin would be validated by
// planparser's commit-subject-mismatch check yet silently unusable (found in
// crucible round fable-r3) — and that a card without a pin renders no
// Commit: line at all (the fork template's own step 3 states the N: <name>
// default).
func TestRenderForkPrompt_RendersPinnedCommitSubject(t *testing.T) {
	plan := testPlan("", "")

	t.Run("pinned Commit renders verbatim", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{{
			Number: 1, Slug: "alpha", Title: "alpha", Intent: "add the flag",
			Commit: "1: json-flag",
		}}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "**Commit:** 1: json-flag")
	})

	t.Run("fork template states the default subject convention", func(t *testing.T) {
		// The N: <name> default is the plan's resume-trail invariant
		// (plan-format-v3.md "Numbering and commit subject"); the implementer
		// only ever sees the fork template, so the convention must be stated
		// there, not merely in the format doc.
		requireContains(t, string(websterengine.ForkTemplate()), "The commit subject is `N: <name>`")
	})

	t.Run("no pin renders no Commit line", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{{
			Number: 1, Slug: "alpha", Title: "alpha", Intent: "add the flag",
		}}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		// "**Commit:** " with a trailing space is the rendered field line's
		// shape; the template's own step-3 prose mentions `**Commit:**` only
		// backtick-wrapped, so it never matches this needle.
		if strings.Contains(string(got), "**Commit:** ") {
			t.Errorf("rendered prompt carries a **Commit:** field line for a card with no pinned subject")
		}
	})
}

// TestRenderForkPrompt_InjectsSharedDecisionsAlways asserts every fork
// prompt carries the plan's own "## Shared Decisions" body verbatim,
// regardless of the batch's own cards — a plan-level, not batch-level,
// injection — and renders the literal sentinel "none" when the plan carries
// no such section, per the fork-prompt-plan-level-context Shared Decision.
func TestRenderForkPrompt_InjectsSharedDecisionsAlways(t *testing.T) {
	batch := batcher.Batch{Cards: []planparser.Card{
		{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"},
	}}

	t.Run("non-empty SharedDecisions passes through verbatim", func(t *testing.T) {
		plan := testPlan("### Decision: json-envelope-reuse\n\n- **Decision:** reuse output.Ok.", "")
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), "json-envelope-reuse")
	})

	t.Run("empty SharedDecisions renders the none sentinel", func(t *testing.T) {
		plan := testPlan("", "")
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		section := extractSection(string(got), "## Shared Decisions")
		if !strings.HasPrefix(section, "none") {
			t.Errorf("## Shared Decisions section = %q; want it to start with the none sentinel", section)
		}
	})
}

// TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch asserts
// RenderForkPrompt injects the plan's canonical "## Rename mechanic" body
// ONLY when the batch contains a card with a non-empty Moves field — a
// non-Moves-bearing batch's rendered prompt must not carry that body text
// at all, per the fork-prompt-plan-level-context Shared Decision.
func TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch(t *testing.T) {
	renameMechanic := "1. Run `git mv <old> <new>` FIRST, before any other change to the moved file."
	plan := testPlan("", renameMechanic)

	t.Run("Moves-bearing batch gets the rename mechanic", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{
			{Number: 4, Slug: "helptree-rename", Title: "helptree-rename", Intent: "rename the row mapper",
				Moves: []planparser.MovePair{{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"}}},
		}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/04-helptree-rename.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), renameMechanic)
	})

	t.Run("non-Moves batch never gets the rename mechanic", func(t *testing.T) {
		batch := batcher.Batch{Cards: []planparser.Card{
			{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"},
		}}
		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireNotContains(t, string(got), renameMechanic)
	})
}

// TestRenderIntegrationPrompt_InjectsVerifyText asserts RenderIntegrationPrompt
// injects the plan's own plan-level "## verify:" text (plan.Verify) into the
// rendered integration-suite fork prompt verbatim, and that an empty
// plan.Verify is refused loud rather than papered over with a sentinel — a
// caller must gate this call on ShouldRunIntegration first.
func TestRenderIntegrationPrompt_InjectsVerifyText(t *testing.T) {
	plan := &planparser.Plan{Verify: "go test ./internal/boardcli/... ./cmd/lyx/..."}

	got, err := websterengine.RenderIntegrationPrompt(plan, "/reports/integration.yaml", "/worktree")
	if err != nil {
		t.Fatalf("RenderIntegrationPrompt() = _, %v; want nil error", err)
	}
	requireContains(t, string(got), plan.Verify)
}

// TestRenderIntegrationPrompt_EmptyVerifyErrors asserts RenderIntegrationPrompt
// refuses loud on a plan with no plan-level verify, rather than silently
// rendering a prompt with an empty verify command.
func TestRenderIntegrationPrompt_EmptyVerifyErrors(t *testing.T) {
	plan := &planparser.Plan{Verify: ""}

	if _, err := websterengine.RenderIntegrationPrompt(plan, "/reports/integration.yaml", "/worktree"); err == nil {
		t.Fatalf("RenderIntegrationPrompt() error = nil; want an error for a plan with no plan-level verify")
	}
}

// TestIntegrationTemplate_ForbidsPollingForOwnReport asserts both the
// integration fork's own template AND the master template's spawn directive
// carry the anti-poll clause: an integration fork that inherits Master's
// own "poll for the integration report" loop and continues it — instead of
// running the verify and writing that report itself — deadlocks the run
// via plain shell polls the lyx-webster fork hook cannot see (found live
// in crucible round fable-r1's F19).
func TestIntegrationTemplate_ForbidsPollingForOwnReport(t *testing.T) {
	integration := string(websterengine.IntegrationTemplate())
	requireContains(t, integration, "NEVER poll or wait for the integration")
	requireContains(t, integration, "YOU are the one who WRITES")

	master := string(websterengine.MasterTemplate())
	requireContains(t, master, "you do NOT poll or wait for any report file")
	requireContains(t, master, "Your FIRST action is to Read this file")
}

// TestIntegrationTemplate_CarriesNoPerCardOrCommitInstructions asserts the
// embedded integration template's bytes carry no per-card or commit
// instructions of any kind: the integration fork runs the plan-level verify
// ONCE and makes NO commit, unlike a batch's own fork template.
func TestIntegrationTemplate_CarriesNoPerCardOrCommitInstructions(t *testing.T) {
	text := string(websterengine.IntegrationTemplate())

	requireNotContains(t, text, "**Commit:**")
	requireNotContains(t, text, "One commit per card")
	requireNotContains(t, text, "{{.cards}}")
	requireContains(t, text, "implement NO cards")
	requireContains(t, text, "make NO commit")
}

// TestRenderProgress_ListsOnlyTerminalBatches asserts RenderProgress lists
// exactly the batches whose persisted BatchState is Terminal, one
// "NN-slug: status" line per batch in plan order, omitting any batch with
// no BatchState entry yet or one recorded but not yet terminal — never
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
