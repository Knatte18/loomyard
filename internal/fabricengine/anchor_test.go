// anchor_test.go — unit tests for the pure reachableAnchor walk.
// Every test here drives reachableAnchor against a hand-built []corrEntry slice and a fake
// in-memory reachable predicate (a map[string]bool closure);
// no git is spawned, per the Test Tier Purity Invariant for an untagged Tier-1 file — modeled on
// corrindex_test.go's git-free style.

package fabricengine

import (
	"errors"
	"testing"
)

// mapReachable returns a reachable predicate backed by a map.
func mapReachable(reachableSHAs map[string]bool) func(string) (bool, error) {
	return func(warpSHA string) (bool, error) {
		return reachableSHAs[warpSHA], nil
	}
}

// TestReachableAnchor covers the walk's outcomes: the newest entry reachable, the nearest-older
// entry one and several steps back, no surviving anchor and an empty index.
//
//testtiming:keep the reachableAnchor walk's outcomes (newest, nearest-older one and several steps back, none, empty index); coverage of its blocks by other tests does not show an assertion of this
func TestReachableAnchor(t *testing.T) {
	t.Parallel()

	threeEntries := []corrEntry{
		{WarpSHA: "w1", WeftSHA: "f1", WarpSeq: 1},
		{WarpSHA: "w2", WeftSHA: "f2", WarpSeq: 2},
		{WarpSHA: "w3", WeftSHA: "f3", WarpSeq: 3},
	}
	fourEntries := append(append([]corrEntry{}, threeEntries...), corrEntry{WarpSHA: "w4", WeftSHA: "f4", WarpSeq: 4})

	tests := []struct {
		name      string
		entries   []corrEntry
		reachable map[string]bool
		wantFound bool
		wantSHA   string
	}{
		{"newest reachable", threeEntries, map[string]bool{"w1": true, "w2": true, "w3": true}, true, "w3"},
		{"single back", threeEntries, map[string]bool{"w1": true, "w2": true, "w3": false}, true, "w2"},
		{"multi back", fourEntries, map[string]bool{"w1": true, "w2": false, "w3": false, "w4": false}, true, "w1"},
		{"none reachable", threeEntries[:2], map[string]bool{"w1": false, "w2": false}, false, ""},
		{"empty slice", nil, nil, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, found, err := reachableAnchor(tc.entries, mapReachable(tc.reachable))
			if err != nil {
				t.Fatalf("reachableAnchor() error = %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("reachableAnchor() found = %v; want %v", found, tc.wantFound)
			}
			if tc.wantFound {
				if got.WarpSHA != tc.wantSHA {
					t.Errorf("reachableAnchor() = %+v; want entry %s", got, tc.wantSHA)
				}
			} else if got != (corrEntry{}) {
				t.Errorf("reachableAnchor() entry = %+v; want zero value", got)
			}
		})
	}
}

// TestReachableAnchor_PredicateErrorPropagatesAndStopsWalk asserts that a reachable error aborts
// the walk immediately.
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
