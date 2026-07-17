// summary_test.go exercises ParseSummary's accept/reject table and
// ArchiveStaleSummary's rename/preserve/no-op/collision behavior, mirroring
// builderengine's own outcome_test.go coverage shape for the same act
// applied to summary.md instead of outcome.yaml.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// writeSummaryFile writes raw content to path, creating its parent
// directory first, failing the test on any error.
func writeSummaryFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// TestParseSummary_ValidParsesTitleAndBody asserts a well-formed summary.md
// (a "# <title>" heading followed by free-form narrative) parses into its
// Title and Body exactly, with the heading line itself excluded from Body.
func TestParseSummary_ValidParsesTitleAndBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, websterengine.SummaryFileName)
	writeSummaryFile(t, path, "# Added the frobnicator\n\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n")

	got, err := websterengine.ParseSummary(path)
	if err != nil {
		t.Fatalf("ParseSummary() error = %v; want nil", err)
	}
	if got.Title != "Added the frobnicator" {
		t.Errorf("ParseSummary() Title = %q; want %q", got.Title, "Added the frobnicator")
	}
	wantBody := "\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n"
	if got.Body != wantBody {
		t.Errorf("ParseSummary() Body = %q; want %q", got.Body, wantBody)
	}
}

// TestParseSummary_LeadingBlankLinesSkipped asserts a heading preceded by
// blank lines still parses — the first NON-BLANK line is what must be the
// heading, not necessarily the file's first line.
func TestParseSummary_LeadingBlankLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, websterengine.SummaryFileName)
	writeSummaryFile(t, path, "\n\n# Title after blank lines\nBody text.\n")

	got, err := websterengine.ParseSummary(path)
	if err != nil {
		t.Fatalf("ParseSummary() error = %v; want nil", err)
	}
	if got.Title != "Title after blank lines" {
		t.Errorf("ParseSummary() Title = %q; want %q", got.Title, "Title after blank lines")
	}
}

// TestParseSummary_MissingFile asserts a missing summary.md is a wrapped
// error, never a guessed nil result.
func TestParseSummary_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, websterengine.SummaryFileName)

	if _, err := websterengine.ParseSummary(path); err == nil {
		t.Fatalf("ParseSummary() error = nil; want an error for a missing file")
	}
}

// TestParseSummary_EmptyFile asserts a present-but-empty (or blank-only)
// summary.md is rejected loud rather than parsed as a title-less summary.
func TestParseSummary_EmptyFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"zero bytes", ""},
		{"blank lines only", "\n\n   \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, websterengine.SummaryFileName)
			writeSummaryFile(t, path, tt.content)

			if _, err := websterengine.ParseSummary(path); err == nil {
				t.Fatalf("ParseSummary() error = nil; want an error for %q", tt.name)
			}
		})
	}
}

// TestParseSummary_NoHeadingFirstLine asserts a file whose first non-blank
// line is not a "# " heading is rejected loud rather than silently treating
// the whole file as an untitled body.
func TestParseSummary_NoHeadingFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, websterengine.SummaryFileName)
	writeSummaryFile(t, path, "Just some narrative with no heading at all.\n")

	if _, err := websterengine.ParseSummary(path); err == nil {
		t.Fatalf("ParseSummary() error = nil; want an error for a missing heading")
	}
}

// TestParseSummary_EmptyTitle asserts a "# " heading whose title is blank
// (or whitespace-only) is rejected loud.
func TestParseSummary_EmptyTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, websterengine.SummaryFileName)
	writeSummaryFile(t, path, "#    \nBody text.\n")

	if _, err := websterengine.ParseSummary(path); err == nil {
		t.Fatalf("ParseSummary() error = nil; want an error for an empty title")
	}
}

// summaryFixedClock returns a func() time.Time that always returns t,
// letting a test pin ArchiveStaleSummary's timestamp deterministically
// instead of racing the real clock.
func summaryFixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestArchiveStaleSummary_AbsentFileIsNoOp asserts archiving a webster dir
// with no summary.md at all returns ("", nil) — not an error — per the
// discussion's "absent file -> no-op" rule.
func TestArchiveStaleSummary_AbsentFileIsNoOp(t *testing.T) {
	dir := t.TempDir()

	got, err := websterengine.ArchiveStaleSummary(dir, time.Now)
	if err != nil {
		t.Fatalf("ArchiveStaleSummary() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("ArchiveStaleSummary() = %q; want \"\" for an absent file", got)
	}
}

// TestArchiveStaleSummary_RenamesAndPreservesContent asserts a present
// summary.md is renamed (never copied-and-left, never deleted) to
// summary-<UTC-compact-timestamp>.md in the same directory, with its content
// preserved byte-for-byte, and the original path no longer exists.
func TestArchiveStaleSummary_RenamesAndPreservesContent(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, websterengine.SummaryFileName)
	content := "# Shipped the frobnicator\n\nDetails.\n"
	writeSummaryFile(t, original, content)

	clk := summaryFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))
	got, err := websterengine.ArchiveStaleSummary(dir, clk)
	if err != nil {
		t.Fatalf("ArchiveStaleSummary() error = %v; want nil", err)
	}

	wantPath := filepath.Join(dir, "summary-20260711T134500Z.md")
	if got != wantPath {
		t.Errorf("ArchiveStaleSummary() = %q; want %q", got, wantPath)
	}

	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Errorf("original summary.md still exists after archiving; want it renamed away")
	}

	archived, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", got, err)
	}
	if string(archived) != content {
		t.Errorf("archived content = %q; want %q", archived, content)
	}
}

// TestArchiveStaleSummary_SameSecondCollisionAppendsSuffix asserts a second
// archive call whose now() truncates to the same compact timestamp does not
// clobber the first archive: it appends a numeric suffix instead, per the
// discussion's collision rule.
func TestArchiveStaleSummary_SameSecondCollisionAppendsSuffix(t *testing.T) {
	dir := t.TempDir()
	clk := summaryFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

	writeSummaryFile(t, filepath.Join(dir, websterengine.SummaryFileName), "# First\n")
	first, err := websterengine.ArchiveStaleSummary(dir, clk)
	if err != nil {
		t.Fatalf("first ArchiveStaleSummary() error = %v; want nil", err)
	}

	// A fresh summary.md, written after the first was archived away, is
	// itself archived a second time within the same clock-second.
	writeSummaryFile(t, filepath.Join(dir, websterengine.SummaryFileName), "# Second\n")
	second, err := websterengine.ArchiveStaleSummary(dir, clk)
	if err != nil {
		t.Fatalf("second ArchiveStaleSummary() error = %v; want nil", err)
	}

	if first == second {
		t.Fatalf("second ArchiveStaleSummary() = %q; want a distinct path from the first %q", second, first)
	}

	wantSecond := filepath.Join(dir, "summary-20260711T134500Z-1.md")
	if second != wantSecond {
		t.Errorf("second ArchiveStaleSummary() = %q; want %q", second, wantSecond)
	}

	firstContent, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("ReadFile(first %q): %v", first, err)
	}
	if !strings.Contains(string(firstContent), "# First") {
		t.Errorf("first archive content = %q; want it to still read \"# First\"", firstContent)
	}

	secondContent, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second %q): %v", second, err)
	}
	if !strings.Contains(string(secondContent), "# Second") {
		t.Errorf("second archive content = %q; want it to read \"# Second\"", secondContent)
	}
}
