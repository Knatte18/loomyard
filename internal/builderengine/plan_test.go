// plan_test.go covers ParsePlan's overview-parsing behavior: frontmatter
// decoding (required keys, unknown-key and duplicate-key rejection) and
// Batch Index parsing (both dash styles, framing extraction, unparseable
// lines). Per-batch file parsing gains its own test coverage once
// parseBatchFile is no longer a stub.

package builderengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// writeOverview writes a minimal 00-overview.md with the given content into
// a fresh temp plan directory and returns that directory's path.
func writeOverview(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "00-overview.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write overview fixture: %v", err)
	}
	return dir
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

	dir := writeOverview(t, validOverview)

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
	dir := writeOverview(t, overview)

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
