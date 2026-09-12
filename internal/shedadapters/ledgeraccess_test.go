// ledgeraccess_test.go is an untagged Tier-1 suite covering shedadapters's exported ledger surface
// only -- IsLedgerPath and ReadLedger, plus the Ledger/LedgerEntry model they hand back.
// parseLedger's own grammar cases are already covered by bouncerfiles_test.go and are not duplicated
// here.

package shedadapters

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestIsLedgerPath_AcceptsLedgerPathOutput asserts the predicate against the real ledgerPath helper
// across several rounds, rather than a hand-typed filename, so a future filename change cannot pass
// this test while breaking discovery.
func TestIsLedgerPath_AcceptsLedgerPathOutput(t *testing.T) {
	for _, round := range []int{1, 2, 7, 42} {
		path := ledgerPath("/tmp/run", round)
		if !IsLedgerPath(path) {
			t.Errorf("IsLedgerPath(%q) = false; want true (round %d)", path, round)
		}
	}
}

// TestIsLedgerPath_RejectsSiblings asserts the predicate rejects each of the four same-round,
// same-directory siblings that verdictPath, focusPath, roundReviewPath, and roundFixerReportPath
// produce -- rejecting the roundReviewPath spelling is the predicate's sharpest job, since
// BurlerProducer publishes it into a history entry's output pointer on its Stuck returns.
func TestIsLedgerPath_RejectsSiblings(t *testing.T) {
	const runDir = "/tmp/run"
	const round = 3

	siblings := map[string]string{
		"verdictPath":          verdictPath(runDir, round),
		"focusPath":            focusPath(runDir, round),
		"roundReviewPath":      roundReviewPath(runDir, round),
		"roundFixerReportPath": roundFixerReportPath(runDir, round),
	}
	for name, path := range siblings {
		if IsLedgerPath(path) {
			t.Errorf("IsLedgerPath(%s = %q) = true; want false", name, path)
		}
	}
}

// TestIsLedgerPath_RejectsUnrelated covers the remaining reject cases: a plain non-round filename, a
// directory-shaped path, and the empty string.
func TestIsLedgerPath_RejectsUnrelated(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"plain_filename", filepath.Join("/tmp/run", "decision-record.md")},
		{"directory_shaped", "/tmp/run"},
		{"empty_string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsLedgerPath(tt.path) {
				t.Errorf("IsLedgerPath(%q) = true; want false", tt.path)
			}
		})
	}
}

// wellFormedLedgerContent returns a well-formed ledger file body naming round in its frontmatter,
// with one open and one resolved entry.
func wellFormedLedgerContent(round int) string {
	return "---\n" +
		"round: " + strconv.Itoa(round) + "\n" +
		"ledger:\n" +
		"  - key: finding-a\n" +
		"    rounds: [1, 2]\n" +
		"    status: open\n" +
		"  - key: finding-b\n" +
		"    rounds: [1]\n" +
		"    status: resolved\n" +
		"---\n" +
		"prose narrative\n"
}

// writeFixture writes content to path, failing the test on any error.
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file %s: %v", path, err)
	}
}

// TestReadLedger_WellFormedRoundTrips verifies a well-formed ledger file round-trips into the
// exported model with its entries intact.
func TestReadLedger_WellFormedRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := ledgerPath(dir, 4)
	writeFixture(t, path, wellFormedLedgerContent(4))

	got, err := ReadLedger(path)
	if err != nil {
		t.Fatalf("ReadLedger(%q) error = %v; want nil", path, err)
	}
	if got.Round != 4 {
		t.Errorf("ReadLedger().Round = %d; want 4", got.Round)
	}
	want := []LedgerEntry{
		{Key: "finding-a", Rounds: []int{1, 2}, Status: "open"},
		{Key: "finding-b", Rounds: []int{1}, Status: "resolved"},
	}
	if len(got.Entries) != len(want) {
		t.Fatalf("ReadLedger().Entries = %v; want %v", got.Entries, want)
	}
	for i := range want {
		gotEntry, wantEntry := got.Entries[i], want[i]
		if gotEntry.Key != wantEntry.Key || gotEntry.Status != wantEntry.Status || len(gotEntry.Rounds) != len(wantEntry.Rounds) {
			t.Errorf("ReadLedger().Entries[%d] = %+v; want %+v", i, gotEntry, wantEntry)
			continue
		}
		for j := range wantEntry.Rounds {
			if gotEntry.Rounds[j] != wantEntry.Rounds[j] {
				t.Errorf("ReadLedger().Entries[%d].Rounds = %v; want %v", i, gotEntry.Rounds, wantEntry.Rounds)
				break
			}
		}
	}
}

// TestReadLedger_MalformedReportsErrorAndZeroModel verifies a malformed ledger file reports an error
// and a zero model.
func TestReadLedger_MalformedReportsErrorAndZeroModel(t *testing.T) {
	dir := t.TempDir()
	path := ledgerPath(dir, 1)
	writeFixture(t, path, "not a valid ledger file at all")

	got, err := ReadLedger(path)
	if err == nil {
		t.Fatal("ReadLedger() error = nil; want a parse error")
	}
	if got.Round != 0 || got.Entries != nil {
		t.Errorf("ReadLedger() = %+v; want the zero Ledger alongside the error", got)
	}
}

// TestReadLedger_AbsentFileReportsErrorWithoutPanicking verifies an absent ledger file reports an
// error without panicking.
func TestReadLedger_AbsentFileReportsErrorWithoutPanicking(t *testing.T) {
	dir := t.TempDir()
	path := ledgerPath(dir, 1)

	got, err := ReadLedger(path)
	if err == nil {
		t.Fatal("ReadLedger() error = nil; want a read error for an absent file")
	}
	if got.Round != 0 || got.Entries != nil {
		t.Errorf("ReadLedger() = %+v; want the zero Ledger alongside the error", got)
	}
}

// TestReadLedger_RoundMismatchReportsError verifies a file written at round 4's path whose
// frontmatter says round: 3 reports an error rather than returning a model claiming round 3.
func TestReadLedger_RoundMismatchReportsError(t *testing.T) {
	dir := t.TempDir()
	path := ledgerPath(dir, 4)
	writeFixture(t, path, wellFormedLedgerContent(3))

	got, err := ReadLedger(path)
	if err == nil {
		t.Fatal("ReadLedger() error = nil; want a round-mismatch error")
	}
	if got.Round != 0 || got.Entries != nil {
		t.Errorf("ReadLedger() = %+v; want the zero Ledger alongside the error", got)
	}
}
