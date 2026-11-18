// recordhistory_integration_test.go drives the decision record's history read over real fabric commits of a forged hub.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitRecordVersion writes data as the discussion record of location and commits it with trailers.
// It returns the commit's SHA.
func commitRecordVersion(t *testing.T, location *lymcwd.Location, record runRecord, data string, trailers ...fabricengine.Trailer) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(record.path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record.path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(""), location, []string{record.rel}, "record version", trailers, fabricengine.EnvSyncOptions())
	if err != nil {
		t.Fatalf("commit the record: %v", err)
	}
	return sha
}

// TestReplacedDecisionRecord_ReadsTheNewestReplaceOfTheRecordMadeAfterTheSnapshot covers the seam over real commits.
// A replace that names another path is no base however new it is.
// A replace at or before the snapshot head is none either.
// A branch with no replace of the record finds nothing.
func TestReplacedDecisionRecord_ReadsTheNewestReplaceOfTheRecordMadeAfterTheSnapshot(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// since picks the snapshot's records head from the commits the history holds.
		since func(plain, older, newest, tip string) string
		// withoutReplaces commits the first two replaces as plain commits.
		withoutReplaces bool
		// want is the record the seam reads, and empty means found false.
		want string
	}{
		{"TheNewestReplaceOfTheRecordWinsOverANewerReplaceOfAnotherPath", func(plain, _, _, _ string) string { return plain }, false, historyY},
		{"AReplaceAfterAnEarlierReplaceIsRead", func(_, older, _, _ string) string { return older }, false, historyY},
		{"AnEmptySinceHeadAdmitsAnyReplace", func(_, _, _, _ string) string { return "" }, false, historyY},
		{"AReplaceAtTheSinceHeadIsNotAfterIt", func(_, _, newest, _ string) string { return newest }, false, ""},
		{"NoReplaceIsAfterTheTip", func(_, _, _, tip string) string { return tip }, false, ""},
		{"NoReplaceOfTheRecordFindsNothing", func(_, _, _, _ string) string { return "" }, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, location := beamHub(t)
			beam := location(testBeam)
			record := runRecordOf(beam, "")
			replace := func(rel, data string) []fabricengine.Trailer {
				if tt.withoutReplaces && rel == record.rel {
					return nil
				}
				return []fabricengine.Trailer{{Key: recordReplacedTrailerKey, Value: recordReplacedValue(decisionByParent, rel, []byte(data))}}
			}

			plain := commitRecordVersion(t, beam, record, historyWithE)
			older := commitRecordVersion(t, beam, record, historyX, replace(record.rel, historyX)...)
			newest := commitRecordVersion(t, beam, record, historyY, replace(record.rel, historyY)...)
			commitRecordVersion(t, beam, record, historyWithE, replace("other/record.md", historyWithE)...)
			tip := commitRecordVersion(t, beam, record, historyX)

			got, found, err := replacedDecisionRecord(beam, record.path)(tt.since(plain, older, newest, tip))

			if err != nil {
				t.Fatalf("replacedDecisionRecord() error = %v; want nil", err)
			}
			if found != (tt.want != "") || string(got) != tt.want {
				t.Errorf("replacedDecisionRecord() = %q, %v; want %q, found %v", got, found, tt.want, tt.want != "")
			}
		})
	}
}

// TestRecordHistory_CutsAtTheNewestReplace asserts the history read stops at a replace commit's bytes.
// An entry the replace dropped is no finding, while an entry added after it and then dropped by a sweep still is.
func TestRecordHistory_CutsAtTheNewestReplace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	_, location := beamHub(t)
	beam := location(testBeam)
	record := runRecordOf(beam, "")
	history := recordHistory(beam, record)
	replaced := fabricengine.Trailer{Key: recordReplacedTrailerKey, Value: recordReplacedValue(decisionByParent, record.rel, []byte(historyX))}

	commitRecordVersion(t, beam, record, historyWithE)
	commitRecordVersion(t, beam, record, historyX, replaced)
	versions, err := history()
	if err != nil {
		t.Fatalf("history() error = %v; want nil", err)
	}
	if findings := discussionparser.CheckAddedDecisionsKept([]byte(historyX), versions); len(findings) != 0 {
		t.Errorf("findings after the replace = %v; want none for the entry it dropped", findings)
	}

	commitRecordVersion(t, beam, record, historyY)
	commitRecordVersion(t, beam, record, historyX)
	versions, err = history()
	if err != nil {
		t.Fatalf("history() error = %v; want nil", err)
	}
	findings := discussionparser.CheckAddedDecisionsKept([]byte(historyX), versions)
	if len(findings) != 1 || !strings.Contains(findings[0].Detail, "): Y`") {
		t.Errorf("findings after a sweep dropped Y = %v; want one naming Y", findings)
	}
}
