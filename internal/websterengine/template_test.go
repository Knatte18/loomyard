// template_test.go pins webster's two embedded prompt templates
// (master-template.md, fork-template.md) against the Go contracts they
// key off of — the template-parser-co-versioning decision applied here:
// the master template's digest-field bullet list is pinned against
// builderengine.Digest's exact field set and order, the outcome-file bullet
// list against the outcome schema, and the fork template's report-schema
// section against builderengine.ParseReport's field set — all as literal-
// statement and exact-field-list assertions in the same style as
// builderengine/template_test.go, plus stencil.Fill round-trips proving
// every required marker. Every test here is untagged and spawn-free: no
// exec.Command, no git, no fixture trees — only embedded bytes and
// stencil.Fill, per the batch's own test-tiers-and-hermetic-git decision.

package websterengine_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

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

// digestSectionHeading and outcomeKeysHeadingSub name the two headings whose
// bullet lists the digest-fields and outcome-schema tests below scope their
// extraction to — never the whole template body, since prose elsewhere
// legitimately backtick-quotes a subset of the same field names (e.g.
// `stuck_reason` names both a digest field and an outcome-file key) without
// that being an "other" field leaking into either pinned set.
const (
	digestSectionHeading  = "## Read ONLY the digest fields — quoted here, exactly"
	outcomeKeysHeadingSub = "`{{.outcome_path}}` itself carries exactly these three keys, quoted here, exactly:"
)

// masterTemplateMarkerValues returns a values map with every one of
// MasterTemplate's six required top-level markers set to a non-empty
// placeholder, so a test can fill the template cleanly or delete one key at
// a time to prove stencil.Fill's per-marker error.
func masterTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":  "01 — json-flag — add the --json flag",
		"progress":     "none",
		"outcome_path": "/lyx/webster/outcome.yaml",
		"summary_path": "/lyx/webster/summary.md",
		"self_fix_cap": "2",
		"poll_wait_s":  "480",
	}
}

// forkTemplateMarkerValues returns a values map with every one of
// ForkTemplate's six required top-level markers set to a non-empty
// placeholder, mirroring masterTemplateMarkerValues.
func forkTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_file":    "/plan/02-list-tests.md",
		"batch_name":    "02-list-tests",
		"report_path":   "/webster/reports/02-list-tests.yaml",
		"self_fix_cap":  "2",
		"worktree_root": "/worktree",
		"prev_digest":   "01-json-flag: done tests=green files_changed=3",
	}
}

// TestMasterTemplate_QuotesDigestFieldsAndNoOthers asserts the master
// template's digest-field bullet list names exactly the ten pinned
// builderengine.Digest field names, in the struct's own declared order — no
// fewer, no extras — the mechanical half of "Master reads only distilled
// digests".
func TestMasterTemplate_QuotesDigestFieldsAndNoOthers(t *testing.T) {
	text := string(websterengine.MasterTemplate())

	want := []string{
		"batch", "status", "tests", "stuck_reason", "out_of_scope",
		"drift_unreported", "files_changed", "dirty", "dead_reason", "elapsed_s",
	}
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
}

// TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder asserts the
// embedded template's bytes carry every rung of the begin-batch -> fork ->
// record-batch sequence, verbatim prompt forwarding, and the recovery
// ladder (no_report re-fork-once, stuck -> recover-batch, running ->
// re-call until terminal, stuck chain -> restart-chain) in prose.
func TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder(t *testing.T) {
	text := string(websterengine.MasterTemplate())

	requireContains(t, text, "`begin-batch` before every fork")
	requireContains(t, text, `subagent_type: "fork"`)
	requireContains(t, text, "with no name")
	requireContains(t, text, "forwarded verbatim")
	requireContains(t, text, "`record-batch` after the fork returns")
	requireContains(t, text, "re-call `recover-batch` until terminal")

	requireContains(t, text, "Drive it STRICTLY in order")
	requireContains(t, text, "re-fork the same batch once")
	requireContains(t, text, "--restart-chain")
	requireContains(t, text, `"paused": true`)
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

	for _, marker := range []string{"batch_index", "progress", "outcome_path", "summary_path", "self_fix_cap", "poll_wait_s"} {
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
// bytes carry the batch-report schema's field names verbatim — the
// ParseReport co-versioning half — plus the fresh-read rule statement and
// the host-commit-per-card statement, so a silent edit to any of these
// fails here rather than only a human review.
func TestForkTemplate_PinsReportSchemaKeys(t *testing.T) {
	text := string(websterengine.ForkTemplate())

	requireContains(t, text, "batch:")
	requireContains(t, text, "status:")
	requireContains(t, text, "tests:")
	requireContains(t, text, "stuck_reason:")
	requireContains(t, text, "out_of_scope:")

	requireContains(t, text, "## The FRESH-READ rule")
	requireContains(t, text, "Commit the card to the HOST repo")
	requireContains(t, text, "One commit per card is the norm")
}

// TestForkTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when
// every one of ForkTemplate's six required markers is supplied, and fails —
// naming the marker — when any single one is absent.
func TestForkTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(websterengine.ForkTemplate(), forkTemplateMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"batch_file", "batch_name", "report_path", "self_fix_cap", "worktree_root", "prev_digest"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := forkTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(websterengine.ForkTemplate(), values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty asserts
// RenderForkPrompt renders the literal "none (first batch)" sentinel into
// {{.prev_digest}} when prevDigest is empty (batch 1's own call site never
// has a preceding digest to pass), and passes a non-empty prevDigest
// through verbatim otherwise — the fork prompt's cross-batch digest context
// is always Go-rendered from the caller's own persisted value, never
// re-derived here.
func TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty(t *testing.T) {
	batch := builderengine.PlanBatch{Number: 1, Slug: "seam-extensions", File: "01-seam-extensions.md"}

	t.Run("empty prevDigest renders the first-batch sentinel", func(t *testing.T) {
		got, err := websterengine.RenderForkPrompt(batch, "/plan", "", "/reports/01-seam-extensions.yaml", "/worktree", 2)
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
		digest := "01-seam-extensions: done tests=green files_changed=4"
		got, err := websterengine.RenderForkPrompt(batch, "/plan", digest, "/reports/02-webster-foundation.yaml", "/worktree", 2)
		if err != nil {
			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
		}
		requireContains(t, string(got), digest)
	})
}

// TestRenderProgress_ListsOnlyTerminalBatches asserts RenderProgress lists
// exactly the batches whose persisted BatchState is Terminal, one
// "NN-slug: status" line per batch in plan order, omitting any batch with
// no BatchState entry yet or one recorded but not yet terminal — never
// re-parsing a report file, only ever reading the persisted record.
func TestRenderProgress_ListsOnlyTerminalBatches(t *testing.T) {
	plan := &builderengine.Plan{
		Batches: []builderengine.PlanBatch{
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
