// plan_test.go covers ParsePlan's overview-parsing behavior (frontmatter
// decoding, Batch Index parsing, framing extraction), its per-batch
// file-parsing behavior (frontmatter, Scope, Cards, verify: sections, and
// the "one or the other, never both" verify rule), and a full round-trip
// over the hand-written testdata/plan-valid fixture.

package builderengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// writePlanFiles writes every entry of files (keyed by filename, e.g.
// "00-overview.md") into a fresh temp plan directory and returns that
// directory's path.
func writePlanFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write plan fixture %s: %v", name, err)
		}
	}
	return dir
}

// writeOverview writes a minimal 00-overview.md with the given content into
// a fresh temp plan directory and returns that directory's path. Used only
// by tests that never reach per-batch file parsing (overview-level errors).
func writeOverview(t *testing.T, content string) string {
	t.Helper()
	return writePlanFiles(t, map[string]string{"00-overview.md": content})
}

// minimalBatchFile is a syntactically complete batch file body: a Scope
// entry, one card with a Where line, and a verify: command — enough to
// satisfy parseBatchFile without exercising any of its optional paths.
func minimalBatchFile(scopePath, wherePath, verifyCommand string) string {
	return "# Batch\n\n## Scope\n\n- " + scopePath + "\n\n## Cards\n\n### Card 1\n\n**Where:** " + wherePath + "\n\n## verify:\n\n" + verifyCommand + "\n"
}

// validOverview is a worked-example-shaped overview with two Batch Index
// entries, used as the base fixture for the positive-path tests below.
const validOverview = `---
format: 1
approved: true
---

# Plan: add --json to lyx board list

Add a --json output mode to lyx board list, emitting one JSON object per row.

## Batch Index

- 01 — json-flag — add the --json flag and envelope emission to boardcli list
- 02 — list-tests — cover --json in boardcli list tests and update help-tree pins
`

func TestParsePlan_Overview(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": validOverview,
		"01-json-flag.md": minimalBatchFile(
			"internal/boardcli/list.go", "internal/boardcli/list.go", "go build ./...",
		),
		"02-list-tests.md": minimalBatchFile(
			"internal/boardcli/list_test.go", "internal/boardcli/list_test.go", "go test ./...",
		),
	})

	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	if plan.Dir != dir {
		t.Errorf("plan.Dir = %q; want %q", plan.Dir, dir)
	}
	if plan.Format != 1 {
		t.Errorf("plan.Format = %d; want 1", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}
	wantFraming := "Add a --json output mode to lyx board list, emitting one JSON object per row."
	if plan.Framing != wantFraming {
		t.Errorf("plan.Framing = %q; want %q", plan.Framing, wantFraming)
	}

	if len(plan.Batches) != 2 {
		t.Fatalf("len(plan.Batches) = %d; want 2", len(plan.Batches))
	}

	want := []builderengine.PlanBatch{
		{Number: 1, Slug: "json-flag", Intent: "add the --json flag and envelope emission to boardcli list", File: "01-json-flag.md"},
		{Number: 2, Slug: "list-tests", Intent: "cover --json in boardcli list tests and update help-tree pins", File: "02-list-tests.md"},
	}
	for i, w := range want {
		got := plan.Batches[i]
		if got.Number != w.Number || got.Slug != w.Slug || got.Intent != w.Intent || got.File != w.File {
			t.Errorf("plan.Batches[%d] = %+v; want %+v", i, got, w)
		}
	}
}

func TestParsePlan_Overview_ASCIIDashSeparators(t *testing.T) {
	t.Parallel()

	const overview = `---
format: 1
approved: true
---

# Plan: ascii dash variant

Framing paragraph.

## Batch Index

- 01 - single-dash - intent using a single ASCII hyphen
- 02 -- double-dash -- intent using a double ASCII hyphen
`
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md":    overview,
		"01-single-dash.md": minimalBatchFile("a.go", "a.go", "go build ./..."),
		"02-double-dash.md": minimalBatchFile("b.go", "b.go", "go build ./..."),
	})

	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if len(plan.Batches) != 2 {
		t.Fatalf("len(plan.Batches) = %d; want 2", len(plan.Batches))
	}
	if plan.Batches[0].Slug != "single-dash" {
		t.Errorf("plan.Batches[0].Slug = %q; want %q", plan.Batches[0].Slug, "single-dash")
	}
	if plan.Batches[1].Slug != "double-dash" {
		t.Errorf("plan.Batches[1].Slug = %q; want %q", plan.Batches[1].Slug, "double-dash")
	}
}

func TestParsePlan_Overview_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		noFile     bool
		wantSubstr string
	}{
		{
			name:       "missing overview file",
			noFile:     true,
			wantSubstr: "not found",
		},
		{
			name:       "missing frontmatter entirely",
			content:    "# Plan: no frontmatter\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: "missing required frontmatter",
		},
		{
			name:       "missing format key",
			content:    "---\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: `missing required key "format"`,
		},
		{
			name:       "missing approved key",
			content:    "---\nformat: 1\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: `missing required key "approved"`,
		},
		{
			name:       "unknown frontmatter key",
			content:    "---\nformat: 1\napproved: true\nextra: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: "field extra not found",
		},
		{
			name:       "duplicate frontmatter key",
			content:    "---\nformat: 1\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: "already defined",
		},
		{
			name:       "unterminated frontmatter fence",
			content:    "---\nformat: 1\napproved: true\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: "unterminated frontmatter fence",
		},
		{
			name:       "missing batch index heading",
			content:    "---\nformat: 1\napproved: true\n---\n\n# Plan\n\nFraming.\n",
			wantSubstr: `missing "## Batch Index" heading`,
		},
		{
			name:       "unparseable batch index line",
			content:    "---\nformat: 1\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- not a valid entry\n",
			wantSubstr: "unparseable batch index line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var dir string
			if tt.noFile {
				dir = t.TempDir()
			} else {
				dir = writeOverview(t, tt.content)
			}

			_, err := builderengine.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan(%q) error = nil; want error containing %q", dir, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ParsePlan(%q) error = %q; want substring %q", dir, err.Error(), tt.wantSubstr)
			}
			if !strings.HasPrefix(err.Error(), "builder:") {
				t.Errorf("ParsePlan(%q) error = %q; want \"builder:\" prefix", dir, err.Error())
			}
		})
	}
}

// singleBatchOverview is a one-batch overview used by the per-batch-file
// tests below, which only care about batch "01-only.md"'s own content.
const singleBatchOverview = `---
format: 1
approved: true
---

# Plan: single batch

Framing.

## Batch Index

- 01 — only — the only batch
`

// parseSingleBatch writes singleBatchOverview plus one "01-only.md" batch
// file with the given body and returns the parsed PlanBatch (t.Fatal on any
// ParsePlan error).
func parseSingleBatch(t *testing.T, batchBody string) builderengine.PlanBatch {
	t.Helper()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": singleBatchOverview,
		"01-only.md":     batchBody,
	})
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if len(plan.Batches) != 1 {
		t.Fatalf("len(plan.Batches) = %d; want 1", len(plan.Batches))
	}
	return plan.Batches[0]
}

func TestParsePlan_BatchFile_ScopeCardsVerify(t *testing.T) {
	t.Parallel()

	body := "# 01 — only\n\n## Intent\n\nProse for the implementer, never stored on PlanBatch.\n\n" +
		"## Scope\n\n- internal/boardcli/list.go\n- internal/boardengine/rows.go\n\n" +
		"## Cards\n\n### Card 1 — flag + row struct\n\n**What:** add a flag.\n" +
		"**Where:** internal/boardcli/list.go, internal/boardengine/rows.go\n\n" +
		"### Card 2 — emission path\n\n**What:** wire it up.\n**Where:** internal/boardcli/list.go\n\n" +
		"## verify:\n\ngo test ./internal/boardcli/... ./internal/boardengine/...\n"

	batch := parseSingleBatch(t, body)

	wantScope := []string{"internal/boardcli/list.go", "internal/boardengine/rows.go"}
	if !slices.Equal(batch.Scope, wantScope) {
		t.Errorf("batch.Scope = %v; want %v", batch.Scope, wantScope)
	}

	wantWhere := []string{
		"internal/boardcli/list.go", "internal/boardengine/rows.go", "internal/boardcli/list.go",
	}
	if !slices.Equal(batch.WhereFiles, wantWhere) {
		t.Errorf("batch.WhereFiles = %v; want %v", batch.WhereFiles, wantWhere)
	}

	if batch.CardCount != 2 {
		t.Errorf("batch.CardCount = %d; want 2", batch.CardCount)
	}

	wantVerify := "go test ./internal/boardcli/... ./internal/boardengine/..."
	if batch.VerifyCommand != wantVerify {
		t.Errorf("batch.VerifyCommand = %q; want %q", batch.VerifyCommand, wantVerify)
	}
	if batch.VerifyDeferred {
		t.Errorf("batch.VerifyDeferred = true; want false")
	}
	if batch.Intent != "the only batch" {
		t.Errorf("batch.Intent = %q; want %q (index-sourced, not the body's \"## Intent\" section)", batch.Intent, "the only batch")
	}
}

func TestParsePlan_BatchFile_Frontmatter(t *testing.T) {
	t.Parallel()

	t.Run("oversized", func(t *testing.T) {
		t.Parallel()

		body := "---\noversized: true\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./...")
		batch := parseSingleBatch(t, body)
		if !batch.Oversized {
			t.Errorf("batch.Oversized = false; want true")
		}
	})

	t.Run("verify deferred with chain-end", func(t *testing.T) {
		t.Parallel()

		body := "---\nverify: deferred\nchain-end: 4\n---\n\n# Batch\n\n## Scope\n\n- a.go\n\n" +
			"## Cards\n\n### Card 1\n\n**Where:** a.go\n"
		batch := parseSingleBatch(t, body)
		if !batch.VerifyDeferred {
			t.Errorf("batch.VerifyDeferred = false; want true")
		}
		if batch.ChainEnd != 4 {
			t.Errorf("batch.ChainEnd = %d; want 4", batch.ChainEnd)
		}
		if batch.VerifyCommand != "" {
			t.Errorf("batch.VerifyCommand = %q; want empty", batch.VerifyCommand)
		}
	})

	t.Run("unrecognized verify sentinel", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     "---\nverify: sometimes\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./..."),
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want error naming the unrecognized verify: sentinel")
		}
		if !strings.Contains(err.Error(), `"sometimes"`) {
			t.Errorf("ParsePlan() error = %q; want it to name the unrecognized value", err.Error())
		}
	})

	t.Run("both deferred and verify section is an error", func(t *testing.T) {
		t.Parallel()

		body := "---\nverify: deferred\nchain-end: 2\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./...")
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     body,
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want error for both verify: deferred and a \"## verify:\" section")
		}
		if !strings.Contains(err.Error(), "allows only one") {
			t.Errorf("ParsePlan() error = %q; want it to name the one-or-the-other rule", err.Error())
		}
	})

	t.Run("unknown frontmatter key", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     "---\nextra: true\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./..."),
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want unknown-key rejection")
		}
		if !strings.Contains(err.Error(), "field extra not found") {
			t.Errorf("ParsePlan() error = %q; want field-not-found substring", err.Error())
		}
	})
}

func TestParsePlan_BatchFile_ScopeGlobRejected(t *testing.T) {
	t.Parallel()

	body := "# Batch\n\n## Scope\n\n- internal/**/list.go\n\n## Cards\n\n### Card 1\n\n**Where:** a.go\n\n## verify:\n\ngo build ./...\n"
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": singleBatchOverview,
		"01-only.md":     body,
	})
	_, err := builderengine.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want glob-in-scope rejection")
	}
	if !strings.Contains(err.Error(), "glob") {
		t.Errorf("ParsePlan() error = %q; want it to name the glob rejection", err.Error())
	}
}

func TestParsePlan_BatchFile_NoScopeOrCards(t *testing.T) {
	t.Parallel()

	// A batch file with neither section present is not itself a parse
	// error — Validate's own checks (verify-missing, scope-malformed) are
	// what flag an under-specified batch; ParsePlan only ever fails loud on
	// what it can mechanically read.
	body := "# Batch\n\n## verify:\n\ngo build ./...\n"
	batch := parseSingleBatch(t, body)
	if batch.Scope != nil {
		t.Errorf("batch.Scope = %v; want nil", batch.Scope)
	}
	if batch.WhereFiles != nil {
		t.Errorf("batch.WhereFiles = %v; want nil", batch.WhereFiles)
	}
	if batch.CardCount != 0 {
		t.Errorf("batch.CardCount = %d; want 0", batch.CardCount)
	}
}

func TestParsePlan_BatchFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := writeOverview(t, singleBatchOverview)
	_, err := builderengine.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want batch-file-not-found error")
	}
	if !strings.Contains(err.Error(), "batch file not found") {
		t.Errorf("ParsePlan() error = %q; want batch-file-not-found substring", err.Error())
	}
}

// TestParsePlan_PlanValidFixture round-trips the hand-written
// testdata/plan-valid fixture exactly: every batch's number, slug, flags,
// chain-end, scope list, card count, and verify command must match the
// fixture's own byte-consistent content (see plan-format.md's worked
// example, extended with a deferred-verify chain and an oversized batch).
func TestParsePlan_PlanValidFixture(t *testing.T) {
	t.Parallel()

	plan, err := builderengine.ParsePlan(filepath.Join("testdata", "plan-valid"))
	if err != nil {
		t.Fatalf("ParsePlan(testdata/plan-valid) error = %v; want nil", err)
	}

	if plan.Format != 1 {
		t.Errorf("plan.Format = %d; want 1", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}

	want := []builderengine.PlanBatch{
		{
			Number: 1, Slug: "json-flag", File: "01-json-flag.md",
			Intent:        "add the --json flag and envelope emission to boardcli list",
			Scope:         []string{"01-json-flag.md"},
			WhereFiles:    []string{"01-json-flag.md", "01-json-flag.md"},
			CardCount:     2,
			VerifyCommand: "go test ./internal/boardcli/... ./internal/boardengine/...",
		},
		{
			Number: 2, Slug: "list-tests", File: "02-list-tests.md",
			Intent:        "cover --json in boardcli list tests and update help-tree pins",
			Scope:         []string{"02-list-tests.md"},
			WhereFiles:    []string{"02-list-tests.md", "02-list-tests.md"},
			CardCount:     2,
			VerifyCommand: "go test ./internal/boardcli/... ./cmd/lyx/...",
		},
		{
			Number: 3, Slug: "refactor-a", File: "03-refactor-a.md",
			Intent:         "start splitting the row-envelope mapper out of boardcli list",
			Scope:          []string{"03-refactor-a.md"},
			WhereFiles:     []string{"03-refactor-a.md"},
			CardCount:      1,
			VerifyDeferred: true,
			ChainEnd:       4,
		},
		{
			Number: 4, Slug: "refactor-b", File: "04-refactor-b.md",
			Intent:        "finish the mapper extraction and run the chain's real verify",
			Scope:         []string{"04-refactor-b.md"},
			WhereFiles:    []string{"04-refactor-b.md"},
			CardCount:     1,
			VerifyCommand: "go build ./...",
		},
		{
			Number: 5, Slug: "oversized", File: "05-oversized.md",
			Intent:        "rewrite boardengine's row pipeline in one atomic pass",
			Scope:         []string{"05-oversized.md"},
			WhereFiles:    []string{"05-oversized.md"},
			CardCount:     1,
			Oversized:     true,
			VerifyCommand: "go build ./... && go test ./...",
		},
	}

	if len(plan.Batches) != len(want) {
		t.Fatalf("len(plan.Batches) = %d; want %d", len(plan.Batches), len(want))
	}
	for i, w := range want {
		got := plan.Batches[i]
		if got.Number != w.Number {
			t.Errorf("plan.Batches[%d].Number = %d; want %d", i, got.Number, w.Number)
		}
		if got.Slug != w.Slug {
			t.Errorf("plan.Batches[%d].Slug = %q; want %q", i, got.Slug, w.Slug)
		}
		if got.File != w.File {
			t.Errorf("plan.Batches[%d].File = %q; want %q", i, got.File, w.File)
		}
		if got.Intent != w.Intent {
			t.Errorf("plan.Batches[%d].Intent = %q; want %q", i, got.Intent, w.Intent)
		}
		if got.Oversized != w.Oversized {
			t.Errorf("plan.Batches[%d].Oversized = %v; want %v", i, got.Oversized, w.Oversized)
		}
		if got.VerifyDeferred != w.VerifyDeferred {
			t.Errorf("plan.Batches[%d].VerifyDeferred = %v; want %v", i, got.VerifyDeferred, w.VerifyDeferred)
		}
		if got.ChainEnd != w.ChainEnd {
			t.Errorf("plan.Batches[%d].ChainEnd = %d; want %d", i, got.ChainEnd, w.ChainEnd)
		}
		if got.VerifyCommand != w.VerifyCommand {
			t.Errorf("plan.Batches[%d].VerifyCommand = %q; want %q", i, got.VerifyCommand, w.VerifyCommand)
		}
		if !slices.Equal(got.Scope, w.Scope) {
			t.Errorf("plan.Batches[%d].Scope = %v; want %v", i, got.Scope, w.Scope)
		}
		if !slices.Equal(got.WhereFiles, w.WhereFiles) {
			t.Errorf("plan.Batches[%d].WhereFiles = %v; want %v", i, got.WhereFiles, w.WhereFiles)
		}
		if got.CardCount != w.CardCount {
			t.Errorf("plan.Batches[%d].CardCount = %d; want %d", i, got.CardCount, w.CardCount)
		}
	}
}

// TestParsePlan_OtherFixturesParseCleanly confirms the plan-unapproved and
// plan-broken-chain fixtures are each well-formed at the ParsePlan level —
// they are designed to trip Validate's checks (a design-level gate), never
// ParsePlan's fail-loud parse errors.
func TestParsePlan_OtherFixturesParseCleanly(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"plan-unapproved", "plan-broken-chain"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := builderengine.ParsePlan(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("ParsePlan(testdata/%s) error = %v; want nil", name, err)
			}
		})
	}
}
