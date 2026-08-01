// anchor_test.go — unit tests for the pure reachableAnchor walk. Every test
// here drives reachableAnchor against a hand-built []corrEntry slice and a
// fake in-memory reachable predicate (a map[string]bool closure); no git is
// spawned, per the Test Tier Purity Invariant for an untagged Tier-1 file —
// modeled on corrindex_test.go's git-free style.

package fabricengine

import (
	"errors"
	"testing"
)

// mapReachable returns a reachable predicate backed by a plain map: a warp
// SHA present and true in reachableSHAs is reachable, anything else is not.
func mapReachable(reachableSHAs map[string]bool) func(string) (bool, error) {
	return func(warpSHA string) (bool, error) {
		return reachableSHAs[warpSHA], nil
	}
}

// TestReachableAnchor_NewestReachable covers the common case: the newest
// (highest-WarpSeq) entry is itself reachable, so it wins immediately without
// the walk needing to consider any older entry.
func TestReachableAnchor_NewestReachable(t *testing.T) {
	entries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
		{WarpSHA: "w3", WeftSHA: "f3", WarpSeq: 3},
	}
	reachable := mapReachable(map[string]bool{"w1": true, "w2": true, "w3": true})

	got, found, err := reachableAnchor(entries, reachable)
	if err != nil {
		t.Fatalf("reachableAnchor() error = %v", err)
	}
	if !found {
		t.Fatalf("reachableAnchor() found = false; want true")
	}
	if got.WarpSHA != "w3" {
		t.Errorf("reachableAnchor() = %+v; want the newest entry w3", got)
	}
}

// TestReachableAnchor_SingleBack covers the nearest-older case one step back:
// the newest entry is unreachable (rebased away), the one directly before it
// still is.
func TestReachableAnchor_SingleBack(t *testing.T) {
	entries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
		{WarpSHA: "w3", WeftSHA: "f3", WarpSeq: 3},
	}
	reachable := mapReachable(map[string]bool{"w1": true, "w2": true, "w3": false})

	got, found, err := reachableAnchor(entries, reachable)
	if err != nil {
		t.Fatalf("reachableAnchor() error = %v", err)
	}
	if !found {
		t.Fatalf("reachableAnchor() found = false; want true")
	}
	if got.WarpSHA != "w2" {
		t.Errorf("reachableAnchor() = %+v; want the nearest-older reachable entry w2", got)
	}
}

// TestReachableAnchor_MultiBack covers the nearest-older case several steps
// back: only the oldest entry survives, and the walk must not stop early at
// any of the unreachable newer ones.
func TestReachableAnchor_MultiBack(t *testing.T) {
	entries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
		{WarpSHA: "w3", WeftSHA: "f3", WarpSeq: 3},
		{WarpSHA: "w4", WeftSHA: "f4", WarpSeq: 4},
	}
	reachable := mapReachable(map[string]bool{"w1": true, "w2": false, "w3": false, "w4": false})

	got, found, err := reachableAnchor(entries, reachable)
	if err != nil {
		t.Fatalf("reachableAnchor() error = %v", err)
	}
	if !found {
		t.Fatalf("reachableAnchor() found = false; want true")
	}
	if got.WarpSHA != "w1" {
		t.Errorf("reachableAnchor() = %+v; want the oldest surviving entry w1", got)
	}
}

// TestReachableAnchor_NoneReachable covers the no-surviving-anchor case: every
// recorded entry has been rewritten away, so the walk finds nothing.
func TestReachableAnchor_NoneReachable(t *testing.T) {
	entries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
	}
	reachable := mapReachable(map[string]bool{"w1": false, "w2": false})

	got, found, err := reachableAnchor(entries, reachable)
	if err != nil {
		t.Fatalf("reachableAnchor() error = %v", err)
	}
	if found {
		t.Errorf("reachableAnchor() found = true; want false")
	}
	if got != (corrEntry{}) {
		t.Errorf("reachableAnchor() entry = %+v; want zero value", got)
	}
}

// TestReachableAnchor_EmptySlice covers the empty-index case: no correspondence
// has ever been recorded, so there is nothing to walk.
func TestReachableAnchor_EmptySlice(t *testing.T) {
	got, found, err := reachableAnchor(nil, mapReachable(nil))
	if err != nil {
		t.Fatalf("reachableAnchor() error = %v", err)
	}
	if found {
		t.Errorf("reachableAnchor() found = true; want false")
	}
	if got != (corrEntry{}) {
		t.Errorf("reachableAnchor() entry = %+v; want zero value", got)
	}
}

// TestReachableAnchor_PredicateErrorPropagatesAndStopsWalk asserts that a
// reachable error aborts the walk immediately: the error is returned
// unchanged (never swallowed), and no entry older than the one that errored
// is ever consulted.
func TestReachableAnchor_PredicateErrorPropagatesAndStopsWalk(t *testing.T) {
	wantErr := errors.New("boom")
	entries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
	}

	visited := make(map[string]bool)
	reachable := func(warpSHA string) (bool, error) {
		visited[warpSHA] = true
		if warpSHA == "w2" {
			return false, wantErr
		}
		return true, nil
	}

	_, found, err := reachableAnchor(entries, reachable)
	if !errors.Is(err, wantErr) {
		t.Fatalf("reachableAnchor() error = %v; want errors.Is(err, wantErr)", err)
	}
	if found {
		t.Errorf("reachableAnchor() found = true; want false on error")
	}
	if visited["w1"] {
		t.Errorf("reachableAnchor() consulted w1 after w2's predicate errored; want the walk to stop at the error")
	}
}
