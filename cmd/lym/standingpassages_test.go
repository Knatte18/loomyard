// standingpassages_test.go covers the passage reader the root command installs, over a fixture Bolt checkout.

package main

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReadStandingPassages is not parallel: it installs the process-wide passage reader.
func TestReadStandingPassages(t *testing.T) {
	tiergate.Require(t, 0)

	bolt := t.TempDir()
	store := heddleengine.Store{Entries: []heddleengine.Entry{
		{Slug: "fixture-passage", Text: "passage text", Order: 1, Targets: []string{"agents-prompt"}},
		{Slug: "fixture-file", Text: "file text", Order: 2, Targets: []string{"agents-file"}},
	}}
	if err := heddleengine.Save(bolt, store); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	t.Cleanup(shuttleengine.InstallPassageReader(readStandingPassages))

	standing := shuttleengine.StandingPassages(bolt, "claude", "")

	// The shipped entries reach a session too, so only the fixture's own are compared.
	ofFixture := func(passages []shuttleengine.Passage) []shuttleengine.Passage {
		return slices.DeleteFunc(passages, func(p shuttleengine.Passage) bool {
			return p.Slug != "fixture-passage" && p.Slug != "fixture-file"
		})
	}
	want := []shuttleengine.Passage{{Slug: "fixture-passage", Text: "passage text"}}
	if got := ofFixture(standing.Passages); !slices.Equal(got, want) {
		t.Errorf("Passages = %+v; want %+v", got, want)
	}
	want = []shuttleengine.Passage{{Slug: "fixture-file", Text: "file text"}}
	if got := ofFixture(standing.FileEntries); !slices.Equal(got, want) {
		t.Errorf("FileEntries = %+v; want %+v", got, want)
	}
}
