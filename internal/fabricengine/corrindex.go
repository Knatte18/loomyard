// corrindex.go — the git-free correspondence-index component: a sorted, locally-persisted cache
// over the warp<->weft SHA correspondence that fabric's Warp-SHA trailers (trailer.go) record
// authoritatively.
// Per the correspondence index layering decision, this component takes an explicit file path and
// never touches git itself;
// the fabric layer (built on top, in a later batch) owns gitdir resolution, WarpSeq computation,
// and the trailer scan that rebuilds this cache from scratch.
// Keeping git out of this file is what lets its tests stay untagged Tier-1 (no git spawn) under the
// Test Tier Purity Invariant.

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

// loadCorrIndex loads the corrIndex from path via state.ReadJSON under path+".lock".
// Missing file yields empty index (fresh pair, no correspondence yet).
func loadCorrIndex(path string) (*corrIndex, error) {
	recs, _, err := state.ReadJSON[[]corrEntry](path, path+".lock")
	if err != nil {
		return nil, err
	}
	// Defensive re-sort to maintain invariant even if file was hand-edited.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].WarpSeq < recs[j].WarpSeq })
	return &corrIndex{path: path, recs: recs}, nil
}

// record upserts e by WarpSHA: replaces existing or appends new,
// re-sorting by WarpSeq so "last recorded wins" for same-seq entries.
// The upsert base is the freshly-read on-disk file, via state.UpdateJSON, never ix.recs — ix.recs
// can be stale relative to a write another process made after this handle was loaded, and composing
// from a stale base would silently clobber that write.
// Persists before in-memory update so write failure leaves index unchanged;
// after a successful record(), ix.recs reflects on-disk truth and may therefore contain entries
// another process recorded.
// This serialises record() against every other write to the file, but not against RebuildIndex's
// scan-to-write span, so the reverse interleaving — a scan that read before this record()'s write,
// followed by RebuildIndex's own later write — still loses an entry.
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

// nearestAtOrBefore returns the greatest WarpSeq <= seq via binary search,
// or the last-recorded entry if multiple share that seq, ok=false if none qualify.
func (ix *corrIndex) nearestAtOrBefore(seq int) (corrEntry, bool) {
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
