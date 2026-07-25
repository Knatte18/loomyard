// corrindex.go — the git-free correspondence-index component: a sorted,
// locally-persisted cache over the warp<->weft SHA correspondence that fabric's
// Warp-SHA trailers (trailer.go) record authoritatively. Per the correspondence
// index layering decision, this component takes an explicit file path and never
// touches git itself; the fabric layer (built on top, in a later batch) owns
// gitdir resolution, WarpSeq computation, and the trailer scan that rebuilds
// this cache from scratch. Keeping git out of this file is what lets its tests
// stay untagged Tier-1 (no git spawn) under the Test Tier Purity Invariant.

package fabricengine

import (
	"sort"

	"github.com/Knatte18/loomyard/internal/state"
)

// corrEntry is one warp<->weft SHA correspondence record: the warp commit SHA,
// the weft commit SHA whose trailer names it, and the warp SHA's first-parent
// commit count (WarpSeq), which orders entries for the "nearest older"
// binary-search lookup.
type corrEntry struct {
	WarpSHA string `json:"warp_sha"`
	WeftSHA string `json:"weft_sha"`
	WarpSeq int    `json:"warp_seq"`
}

// corrIndex is a sorted, file-backed cache of corrEntry records, ordered by
// WarpSeq ascending. It is a pure derived-state cache over the weft commit
// trailers, which remain the sole source of truth; the fabric layer's
// RebuildIndex can always reconstruct an equivalent corrIndex from a full
// trailer scan.
type corrIndex struct {
	path string
	recs []corrEntry
}

// loadCorrIndex loads the corrIndex persisted at path, via
// state.ReadJSON[[]corrEntry] under path+".lock". A missing file is not an
// error — it yields an empty index, since a fresh worktree pair has recorded
// no correspondence yet.
func loadCorrIndex(path string) (*corrIndex, error) {
	recs, _, err := state.ReadJSON[[]corrEntry](path, path+".lock")
	if err != nil {
		return nil, err
	}
	// Defensive re-sort: record() always persists in WarpSeq order, but
	// sorting on load keeps the in-memory invariant true even if the file was
	// ever hand-edited or written by a future caller that didn't.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].WarpSeq < recs[j].WarpSeq })
	return &corrIndex{path: path, recs: recs}, nil
}

// record upserts e by WarpSHA: an existing entry for the same warp SHA is
// replaced (its weft SHA and sequence updated); a new warp SHA is appended.
// The resulting slice is re-sorted by WarpSeq with sort.SliceStable, so that
// among entries sharing a WarpSeq, insertion order is preserved and the
// just-recorded entry sorts after any pre-existing entries at the same seq —
// which is what makes nearestAtOrBefore's "last recorded wins" guarantee hold.
// The updated slice is persisted via state.WriteJSON before the in-memory
// index is updated, so a write failure leaves the index unchanged.
func (ix *corrIndex) record(e corrEntry) error {
	next := make([]corrEntry, 0, len(ix.recs)+1)
	for _, existing := range ix.recs {
		if existing.WarpSHA == e.WarpSHA {
			// Drop the stale entry for this warp SHA; the fresh one is
			// appended below, after every entry that was already present.
			continue
		}
		next = append(next, existing)
	}
	next = append(next, e)
	sort.SliceStable(next, func(i, j int) bool { return next[i].WarpSeq < next[j].WarpSeq })

	if err := state.WriteJSON(ix.path, ix.path+".lock", next); err != nil {
		return err
	}
	ix.recs = next
	return nil
}

// exact reports the corrEntry recorded for warpSHA, if any.
func (ix *corrIndex) exact(warpSHA string) (corrEntry, bool) {
	for _, e := range ix.recs {
		if e.WarpSHA == warpSHA {
			return e, true
		}
	}
	return corrEntry{}, false
}

// nearestAtOrBefore returns the entry with the greatest WarpSeq <= seq, using
// a binary search (sort.Search) over the WarpSeq-sorted slice rather than a
// sequential scan. When multiple entries share that greatest qualifying seq,
// it returns the last one recorded (see record's ordering guarantee). Reports
// ok=false when the index is empty or every entry's WarpSeq exceeds seq.
func (ix *corrIndex) nearestAtOrBefore(seq int) (corrEntry, bool) {
	// idx is the first index whose WarpSeq exceeds seq; everything before it
	// qualifies, so idx-1 is the greatest qualifying entry.
	idx := sort.Search(len(ix.recs), func(i int) bool { return ix.recs[i].WarpSeq > seq })
	if idx == 0 {
		return corrEntry{}, false
	}
	return ix.recs[idx-1], true
}

// entries returns a copy of the index's current WarpSeq-sorted entries, for
// callers (e.g. RebuildIndex equality assertions) that need to compare index
// contents without risking mutation of the index's own backing slice.
func (ix *corrIndex) entries() []corrEntry {
	cp := make([]corrEntry, len(ix.recs))
	copy(cp, ix.recs)
	return cp
}
