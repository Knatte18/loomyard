// corrindex.go holds the correspondence index: a sorted, file-backed cache of the warp<->weft SHA correspondence.
// The index takes an explicit file path and never touches git.
// The fabric layer owns gitdir resolution, WarpSeq computation, and the trailer scan that rebuilds the cache.
// Keeping git out of this file keeps its tests untagged Tier-1 (no git spawn) under the Test Tier Purity Invariant.

package fabricengine

import (
	"sort"

	"github.com/Knatte18/loomyard/internal/state"
)

// corrEntry is one warp<->weft SHA correspondence record with WarpSeq for "nearest older" lookup.
type corrEntry struct {
	WarpSHA string `json:"warp_sha"`
	WeftSHA string `json:"weft_sha"`
	WarpSeq int    `json:"warp_seq"`
}

// corrIndex is a sorted, file-backed cache of correspondence entries, ordered by WarpSeq.
// Pure derived state; weft trailers remain the sole source of truth.
type corrIndex struct {
	path string
	recs []corrEntry
}

// loadCorrIndex reads the index at path under a lock on path+".lock".
// A missing file yields an empty index, the state of a fresh warp/weft pair with no correspondence yet.
func loadCorrIndex(path string) (*corrIndex, error) {
	recs, _, err := state.ReadJSON[[]corrEntry](path, path+".lock")
	if err != nil {
		return nil, err
	}
	// Re-sort defensively: the file may have been hand-edited.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].WarpSeq < recs[j].WarpSeq })
	return &corrIndex{path: path, recs: recs}, nil
}

// record upserts e by WarpSHA, replacing any existing entry with the same WarpSHA.
// It re-sorts by WarpSeq, so the last recorded entry wins among entries with equal WarpSeq.
// The upsert composes from the freshly read file, never from ix.recs.
// ix.recs can be stale when another process writes after this handle loads.
// Composing from it would clobber that write.
// The write happens before ix.recs changes, so a failed write leaves the index unchanged.
// After a successful write, ix.recs reflects the file and may hold entries other processes recorded.
// The write serialises against other writes to the file, but not against the scan-to-write span of RebuildIndex:
// a scan that reads before this write and writes after it still loses this entry.
func (ix *corrIndex) record(e corrEntry) error {
	var next []corrEntry
	err := state.UpdateJSON(ix.path, ix.path+".lock", func(cur []corrEntry, _ bool) ([]corrEntry, error) {
		next = make([]corrEntry, 0, len(cur)+1)
		for _, existing := range cur {
			if existing.WarpSHA == e.WarpSHA {
				continue
			}
			next = append(next, existing)
		}
		next = append(next, e)
		sort.SliceStable(next, func(i, j int) bool { return next[i].WarpSeq < next[j].WarpSeq })
		return next, nil
	})
	if err != nil {
		return err
	}
	ix.recs = next
	return nil
}

// exact reports whether and which entry is recorded for warpSHA.
func (ix *corrIndex) exact(warpSHA string) (corrEntry, bool) {
	for _, e := range ix.recs {
		if e.WarpSHA == warpSHA {
			return e, true
		}
	}
	return corrEntry{}, false
}

// nearestAtOrBefore returns the entry with the greatest WarpSeq at or before seq.
// When several entries share that WarpSeq, it returns the last recorded one.
// ok is false when no entry qualifies.
func (ix *corrIndex) nearestAtOrBefore(seq int) (corrEntry, bool) {
	idx := sort.Search(len(ix.recs), func(i int) bool { return ix.recs[i].WarpSeq > seq })
	if idx == 0 {
		return corrEntry{}, false
	}
	return ix.recs[idx-1], true
}

// entries returns a copy of the WarpSeq-sorted entries, so callers cannot alias the index's backing slice.
func (ix *corrIndex) entries() []corrEntry {
	cp := make([]corrEntry, len(ix.recs))
	copy(cp, ix.recs)
	return cp
}
