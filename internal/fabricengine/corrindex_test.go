// corrindex_test.go — unit tests for the git-free correspondence-index component.
// Every test here uses an explicit t.TempDir() file path and never spawns git, per the
// correspondence index layering decision.

package fabricengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// TestCorrIndex_RecordReloadRoundTrip asserts that entries recorded through one corrIndex handle
// are visible after reloading the same path into a fresh handle, and that exact() misses a warp SHA
// that was never recorded.
//
//testtiming:keep entries surviving a reload into a fresh handle and exact() missing a never-recorded warp SHA; coverage of its blocks by other tests does not show an assertion of this
func TestCorrIndex_RecordReloadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")

	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	if err := ix.record(corrEntry{WarpSHA: "warp1", WeftSHA: "weft1", WarpSeq: 1}); err != nil {
		t.Fatalf("record() error = %v", err)
	}
	if err := ix.record(corrEntry{WarpSHA: "warp2", WeftSHA: "weft2", WarpSeq: 2}); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	reloaded, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (reload) error = %v", err)
	}

	got1, ok := reloaded.exact("warp1")
	if !ok || got1.WeftSHA != "weft1" {
		t.Errorf("reloaded.exact(warp1) = %+v, %v; want {weft1 ...}, true", got1, ok)
	}
	got2, ok := reloaded.exact("warp2")
	if !ok || got2.WeftSHA != "weft2" {
		t.Errorf("reloaded.exact(warp2) = %+v, %v; want {weft2 ...}, true", got2, ok)
	}
	if _, ok := reloaded.exact("nonexistent"); ok {
		t.Errorf("reloaded.exact(nonexistent) ok = true; want false")
	}
}

// TestCorrIndex_RecordUpsertOverwritesWeftSHA asserts that recording a second entry for an
// already-present warp SHA overwrites its weft SHA rather than appending a duplicate.
//
//testtiming:keep recording a known warp SHA again overwriting its weft SHA instead of appending; coverage of its blocks by other tests does not show an assertion of this
func TestCorrIndex_RecordUpsertOverwritesWeftSHA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}

	if err := ix.record(corrEntry{WarpSHA: "warp1", WeftSHA: "weft-old", WarpSeq: 1}); err != nil {
		t.Fatalf("record() error = %v", err)
	}
	if err := ix.record(corrEntry{WarpSHA: "warp1", WeftSHA: "weft-new", WarpSeq: 1}); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	got, ok := ix.exact("warp1")
	if !ok {
		t.Fatalf("exact(warp1) ok = false; want true")
	}
	if got.WeftSHA != "weft-new" {
		t.Errorf("exact(warp1).WeftSHA = %q; want %q", got.WeftSHA, "weft-new")
	}
	if len(ix.entries()) != 1 {
		t.Errorf("entries() = %v; want exactly one entry (upsert, not append)", ix.entries())
	}
}

// TestCorrIndex_NearestAtOrBefore covers loading a path with no file (an empty index, not an error),
// the binary-search "nearest older" lookup: an empty index, a target below every recorded seq, an
// exact-seq hit, a between-seqs hit and a target above every seq; and, when several entries share
// the qualifying WarpSeq, the last one recorded winning per record's stable-sort ordering guarantee.
//
//testtiming:keep the binary-search nearest-older lookup over an empty, below-range, exact, between and above-range index, and the last-recorded entry winning a shared seq; coverage of its blocks by other tests does not show an assertion of this
func TestCorrIndex_NearestAtOrBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	if got := ix.entries(); len(got) != 0 {
		t.Errorf("entries() of a never-written path = %v; want empty", got)
	}

	if _, ok := ix.nearestAtOrBefore(5); ok {
		t.Errorf("nearestAtOrBefore(5) on empty index ok = true; want false")
	}

	for _, e := range []corrEntry{
		{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
		{WarpSHA: "w20", WeftSHA: "f20", WarpSeq: 20},
		{WarpSHA: "w30", WeftSHA: "f30", WarpSeq: 30},
	} {
		if err := ix.record(e); err != nil {
			t.Fatalf("record(%+v) error = %v", e, err)
		}
	}

	if _, ok := ix.nearestAtOrBefore(5); ok {
		t.Errorf("nearestAtOrBefore(5) below all seqs: ok = true; want false")
	}

	got, ok := ix.nearestAtOrBefore(20)
	if !ok || got.WarpSHA != "w20" {
		t.Errorf("nearestAtOrBefore(20) exact-seq hit = %+v, %v; want w20, true", got, ok)
	}

	got, ok = ix.nearestAtOrBefore(25)
	if !ok || got.WarpSHA != "w20" {
		t.Errorf("nearestAtOrBefore(25) between-seqs hit = %+v, %v; want w20, true", got, ok)
	}

	got, ok = ix.nearestAtOrBefore(100)
	if !ok || got.WarpSHA != "w30" {
		t.Errorf("nearestAtOrBefore(100) above all seqs = %+v, %v; want w30, true", got, ok)
	}

	shared, err := loadCorrIndex(filepath.Join(t.TempDir(), "shared.json"))
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	for _, e := range []corrEntry{
		{WarpSHA: "first", WeftSHA: "f1", WarpSeq: 10},
		{WarpSHA: "second", WeftSHA: "f2", WarpSeq: 10},
	} {
		if err := shared.record(e); err != nil {
			t.Fatalf("record(%+v) error = %v", e, err)
		}
	}
	got, ok = shared.nearestAtOrBefore(10)
	if !ok || got.WarpSHA != "second" {
		t.Errorf("nearestAtOrBefore(10) with a shared seq = %+v, %v; want second (last recorded), true", got, ok)
	}
}

// TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite asserts that record() upserts against
// the freshly-read on-disk base rather than the handle's own possibly-stale in-memory snapshot: an
// external state.WriteJSON call (standing in for RebuildIndex's write) that lands after the handle
// is loaded but before record() runs must not be lost when record() persists.
//
//testtiming:keep record() upserting against the freshly read on-disk base so a write landed after the handle loaded survives; coverage of its blocks by other tests does not show an assertion of this
func TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")

	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}

	other := corrEntry{WarpSHA: "other", WeftSHA: "other-weft", WarpSeq: 1}
	if err := state.WriteJSON(path, path+".lock", []corrEntry{other}); err != nil {
		t.Fatalf("state.WriteJSON() error = %v", err)
	}

	mine := corrEntry{WarpSHA: "mine", WeftSHA: "mine-weft", WarpSeq: 2}
	if err := ix.record(mine); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	reloaded, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (reload) error = %v", err)
	}
	if got, ok := reloaded.exact("other"); !ok || got.WeftSHA != "other-weft" {
		t.Errorf("reloaded.exact(other) = %+v, %v; want {other-weft ...}, true", got, ok)
	}
	if got, ok := reloaded.exact("mine"); !ok || got.WeftSHA != "mine-weft" {
		t.Errorf("reloaded.exact(mine) = %+v, %v; want {mine-weft ...}, true", got, ok)
	}
}

// TestCorrIndex_PersistenceIsAtomic asserts that after every record() call, the backing file is
// fully written and parses as valid JSON — never half-written or transiently corrupt.
//
//testtiming:keep the index file parsing as complete JSON after every record() call; coverage of its blocks by other tests does not show an assertion of this
func TestCorrIndex_PersistenceIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}

	for i, e := range []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
		{WarpSHA: "w3", WeftSHA: "f3", WarpSeq: 3},
	} {
		if err := ix.record(e); err != nil {
			t.Fatalf("record(%+v) error = %v", e, err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() after record %d error = %v", i, err)
		}
		var parsed []corrEntry
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Unmarshal() after record %d error = %v; contents: %s", i, err, data)
		}
		if len(parsed) != i+1 {
			t.Errorf("after record %d: len(parsed) = %d; want %d", i, len(parsed), i+1)
		}
	}
}
