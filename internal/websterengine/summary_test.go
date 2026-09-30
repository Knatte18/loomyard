// summary_test.go exercises ArchiveStaleSummary's rename/preserve/no-op/collision behavior, the
// same archive-never-refuse coverage shape outcome.go's own tests apply, here applied to the
// final-summary artifact instead of outcome.yaml. ParseSummary's own accept/reject coverage moved
// to internal/summaryparser/summary_test.go, the artifact's read contract's sole owner.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/summaryparser"
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

// summaryFixedClock returns a func() time.Time that always returns t,
// letting a test pin ArchiveStaleSummary's timestamp deterministically
// instead of racing the real clock.
func summaryFixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestArchiveStaleSummary_AbsentFileIsNoOp asserts archiving a webster dir with no summary.md at
// all returns ("", nil) — not an error — per the discussion's "absent file -> no-op" rule.
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

// TestArchiveStaleSummary_RenamesAndPreservesContent asserts a present summary.md is renamed (never
// copied-and-left, never deleted) to summary-<UTC-compact-timestamp>.md in the same directory, with
// its content preserved byte-for-byte, and the original path no longer exists.
func TestArchiveStaleSummary_RenamesAndPreservesContent(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, summaryparser.FileName)
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

// TestArchiveStaleSummary_SameSecondCollisionAppendsSuffix asserts a second archive call whose
// now() truncates to the same compact timestamp does not clobber the first archive: it appends a
// numeric suffix instead, per the discussion's collision rule.
func TestArchiveStaleSummary_SameSecondCollisionAppendsSuffix(t *testing.T) {
	dir := t.TempDir()
	clk := summaryFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

	writeSummaryFile(t, filepath.Join(dir, summaryparser.FileName), "# First\n")
	first, err := websterengine.ArchiveStaleSummary(dir, clk)
	if err != nil {
		t.Fatalf("first ArchiveStaleSummary() error = %v; want nil", err)
	}

	// A fresh summary.md, written after the first was archived away, is
	// itself archived a second time within the same clock-second.
	writeSummaryFile(t, filepath.Join(dir, summaryparser.FileName), "# Second\n")
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

// readSummaryFile returns the summary.md content under dir.
func readSummaryFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(summaryparser.Path(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

const failedSectionHead = "\n\n## Integration suite failed\n\nThe plan-level `## verify:` suite failed. SHA-bisect localized the failure to card `03-c` (commit `abc123`).\n"

// TestAppendIntegrationFailure_RegressionsListedWithTails asserts each regressing identity is appended with its tail in a fenced block after the localized-card sentence.
func TestAppendIntegrationFailure_RegressionsListedWithTails(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFile(t, summaryparser.Path(dir), "# S\n")
	regs := []websterengine.IntegrationFailure{
		{ID: "TestA", Kind: websterengine.FailureKindTest, Tail: "a boom\n"},
		{ID: "pkg/b", Kind: websterengine.FailureKindPackage, Tail: "b boom"},
	}
	if err := websterengine.AppendIntegrationFailure(dir, "03-c", "abc123", regs); err != nil {
		t.Fatalf("AppendIntegrationFailure: %v", err)
	}
	want := "# S\n" + failedSectionHead +
		"\nRegressing failures:\n" +
		"\n### `TestA`\n\n```\na boom\n```\n" +
		"\n### `pkg/b`\n\n```\nb boom\n```\n"
	if got := readSummaryFile(t, dir); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// TestAppendIntegrationFailure_NilRegressionsUnchanged asserts nil regressions yield today's section.
func TestAppendIntegrationFailure_NilRegressionsUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFile(t, summaryparser.Path(dir), "# S\n")
	if err := websterengine.AppendIntegrationFailure(dir, "03-c", "abc123", nil); err != nil {
		t.Fatalf("AppendIntegrationFailure: %v", err)
	}
	if got, want := readSummaryFile(t, dir), "# S\n"+failedSectionHead; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

const triageSectionHead = "\n\n## Integration suite triage\n\nThe plan-level `## verify:` suite failed, but webster's triage did not attribute the failure to this run.\n"

// TestAppendIntegrationTriage_BothCategories asserts both sub-lists are written by identity.
func TestAppendIntegrationTriage_BothCategories(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFile(t, summaryparser.Path(dir), "# S\n")
	if err := websterengine.AppendIntegrationTriage(dir, []string{"TestF1", "TestF2"}, []string{"TestP"}); err != nil {
		t.Fatalf("AppendIntegrationTriage: %v", err)
	}
	want := "# S\n" + triageSectionHead +
		"\nFlaky (passed on rerun):\n\n- `TestF1`\n- `TestF2`\n" +
		"\nPre-existing (already failing at the plan's starting commit):\n\n- `TestP`\n"
	if got := readSummaryFile(t, dir); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// TestAppendIntegrationTriage_OnlyFlaky asserts no pre-existing sub-list appears when that category is empty.
func TestAppendIntegrationTriage_OnlyFlaky(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFile(t, summaryparser.Path(dir), "# S\n")
	if err := websterengine.AppendIntegrationTriage(dir, []string{"TestF1"}, nil); err != nil {
		t.Fatalf("AppendIntegrationTriage: %v", err)
	}
	want := "# S\n" + triageSectionHead + "\nFlaky (passed on rerun):\n\n- `TestF1`\n"
	if got := readSummaryFile(t, dir); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// TestAppendIntegrationTriage_EmptyIsNoOp asserts empty lists leave the file untouched.
func TestAppendIntegrationTriage_EmptyIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFile(t, summaryparser.Path(dir), "# S\n")
	if err := websterengine.AppendIntegrationTriage(dir, nil, nil); err != nil {
		t.Fatalf("AppendIntegrationTriage: %v", err)
	}
	if got := readSummaryFile(t, dir); got != "# S\n" {
		t.Errorf("summary = %q, want untouched", got)
	}
}
