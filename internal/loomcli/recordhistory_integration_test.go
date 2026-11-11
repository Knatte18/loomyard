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
func commitRecordVersion(t *testing.T, location *lymcwd.Location, record runRecord, data string, trailers ...fabricengine.Trailer) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(record.path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record.path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(""), location, []string{record.rel}, "record version", trailers, fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit the record: %v", err)
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
