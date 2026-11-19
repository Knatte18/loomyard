// parentask_lookup_test.go covers how a run's one ask record is searched by seat tag and by seed.

package burlerengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParentAskRecord_LatestLookups proves each lookup answers the latest ask of its kind, whatever others followed it.
// A tag or seed no ask carries answers false.
func TestParentAskRecord_LatestLookups(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	record := burlerengine.ParentAskRecord{Asks: []burlerengine.ParentAsk{
		{Number: 1, Seed: "a", Tag: "[x]", Round: 1},
		{Number: 2, Seed: "b", Tag: "[y]", Round: 1},
		{Number: 3, Seed: "a", Tag: "[x]", Round: 2},
		{Number: 4, Seed: "b", Tag: "[y]", Round: 2},
	}}
	tests := []struct {
		name       string
		lookup     func(string) (burlerengine.ParentAsk, bool)
		key        string
		wantNumber int
	}{
		{name: "latest by tag", lookup: record.LatestTagged, key: "[x]", wantNumber: 3},
		{name: "latest by tag after another tag", lookup: record.LatestTagged, key: "[y]", wantNumber: 4},
		{name: "a tag no ask carries", lookup: record.LatestTagged, key: "[z]"},
		{name: "latest by seed", lookup: record.LatestOfSeed, key: "a", wantNumber: 3},
		{name: "a seed no ask carries", lookup: record.LatestOfSeed, key: "c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ask, found := tt.lookup(tt.key)
			if found != (tt.wantNumber != 0) || ask.Number != tt.wantNumber {
				t.Errorf("lookup(%q) = ask %d, found %v; want ask %d", tt.key, ask.Number, found, tt.wantNumber)
			}
		})
	}
}
