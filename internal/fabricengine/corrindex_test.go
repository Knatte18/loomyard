// corrindex_test.go — unit tests for the git-free correspondence-index
// component. Every test here uses an explicit t.TempDir() file path and never
// spawns git, per the correspondence index layering decision.

package fabricengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCorrIndex_RecordReloadRoundTrip asserts that entries recorded through
// one corrIndex handle are visible after reloading the same path into a fresh
// handle.
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
}

// TestCorrIndex_LoadMissingFileIsEmpty asserts that loading a path with no
// existing file yields an empty index rather than an error.
func TestCorrIndex_LoadMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	if got := ix.entries(); len(got) != 0 {
		t.Errorf("entries() = %v; want empty", got)
	}
}

// TestCorrIndex_RecordUpsertOverwritesWeftSHA asserts that recording a second
// entry for an already-present warp SHA overwrites its weft SHA rather than
// appending a duplicate.
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

// TestCorrIndex_ExactHitAndMiss covers exact() for a recorded warp SHA and an
// unrecorded one.
func TestCorrIndex_ExactHitAndMiss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	if err := ix.record(corrEntry{WarpSHA: "warp1", WeftSHA: "weft1", WarpSeq: 1}); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	if got, ok := ix.exact("warp1"); !ok || got.WeftSHA != "weft1" {
		t.Errorf("exact(warp1) = %+v, %v; want {weft1 ...}, true", got, ok)
	}
	if _, ok := ix.exact("nonexistent"); ok {
		t.Errorf("exact(nonexistent) ok = true; want false")
	}
}

// TestCorrIndex_NearestAtOrBefore covers the binary-search "nearest older"
// lookup: an empty index, a target below every recorded seq, an exact-seq
// hit, and a between-seqs hit.
func TestCorrIndex_NearestAtOrBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
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
}

// TestCorrIndex_NearestAtOrBefore_SharedSeqLastRecordedWins asserts that when
// multiple entries share the qualifying WarpSeq, nearestAtOrBefore returns the
// last one recorded, per record's stable-sort ordering guarantee.
func TestCorrIndex_NearestAtOrBefore_SharedSeqLastRecordedWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}

	if err := ix.record(corrEntry{WarpSHA: "first", WeftSHA: "f1", WarpSeq: 10}); err != nil {
		t.Fatalf("record() error = %v", err)
	}
	if err := ix.record(corrEntry{WarpSHA: "second", WeftSHA: "f2", WarpSeq: 10}); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	got, ok := ix.nearestAtOrBefore(10)
	if !ok {
		t.Fatalf("nearestAtOrBefore(10) ok = false; want true")
	}
	if got.WarpSHA != "second" {
		t.Errorf("nearestAtOrBefore(10) = %q; want %q (last recorded)", got.WarpSHA, "second")
	}
}

// TestCorrIndex_PersistenceIsAtomic asserts that after every record() call,
// the backing file is fully written and parses as valid JSON — never
// half-written or transiently corrupt.
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
