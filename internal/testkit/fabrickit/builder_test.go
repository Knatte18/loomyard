// builder_test.go tests that the builder places every setter's value and that a built snapshot is its own copy.

package fabrickit_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBuilder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	everySetter := func(b *fabrickit.Builder, value string) *fabrickit.Builder {
		return b.Phases(fabriccore.Phase(value)).
			LocalRef(fabriccore.StoreCode, "refs/heads/"+value, value).
			OriginRef(fabriccore.StoreRecords, "refs/heads/"+value, value).
			Worktree(fabriclayout.WorktreeEntry{Path: value}).
			Anchor(value, value).
			RecordsStore(fabriccore.RecordsStore{Present: true, Heads: value}).
			PendingTeardown(fabriclayout.PendingTeardown{Slug: value}).
			Outcome(fabriccore.Outcome{Detail: value, Action: fabriccore.Action{Paths: []string{value}}}).
			ReadFailure(fabriccore.ReadFailure{Detail: value})
	}
	wantOne := fabriccore.Snapshot{
		Phases: []fabriccore.Phase{"one"},
		Stores: map[fabriccore.Store]fabriccore.StoreRefs{
			fabriccore.StoreCode:    {Local: fabriccore.Refs{"refs/heads/one": "one"}},
			fabriccore.StoreRecords: {Origin: fabriccore.Refs{"refs/heads/one": "one"}, OriginRead: true},
		},
		Worktrees:        []fabriclayout.WorktreeEntry{{Path: "one"}},
		Anchors:          map[string]string{"one": "one"},
		RecordsStore:     fabriccore.RecordsStore{Present: true, Heads: "one"},
		PendingTeardowns: []fabriclayout.PendingTeardown{{Slug: "one"}},
		Outcomes:         []fabriccore.Outcome{{Detail: "one", Action: fabriccore.Action{Paths: []string{"one"}}}},
		ReadFailures:     []fabriccore.ReadFailure{{Detail: "one"}},
	}
	wantBoth := fabriccore.Snapshot{
		Phases: []fabriccore.Phase{"two"},
		Stores: map[fabriccore.Store]fabriccore.StoreRefs{
			fabriccore.StoreCode: {Local: fabriccore.Refs{"refs/heads/one": "one", "refs/heads/two": "two"}},
			fabriccore.StoreRecords: {
				Origin:     fabriccore.Refs{"refs/heads/one": "one", "refs/heads/two": "two"},
				OriginRead: true,
			},
		},
		Worktrees:        []fabriclayout.WorktreeEntry{{Path: "one"}, {Path: "two"}},
		Anchors:          map[string]string{"one": "one", "two": "two"},
		RecordsStore:     fabriccore.RecordsStore{Present: true, Heads: "two"},
		PendingTeardowns: []fabriclayout.PendingTeardown{{Slug: "one"}, {Slug: "two"}},
		Outcomes: []fabriccore.Outcome{
			{Detail: "one", Action: fabriccore.Action{Paths: []string{"one"}}},
			{Detail: "two", Action: fabriccore.Action{Paths: []string{"two"}}},
		},
		ReadFailures: []fabriccore.ReadFailure{{Detail: "one"}, {Detail: "two"}},
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"EverySetterPlacesItsValue", func(t *testing.T) {
			if got := everySetter(fabrickit.NewSnapshot(), "one").Build(); !reflect.DeepEqual(got, wantOne) {
				t.Errorf("Build() = %+v; want %+v", got, wantOne)
			}
		}},
		{"SeamIsSetAndABuiltSnapshotKeepsIt", func(t *testing.T) {
			b := fabrickit.NewSnapshot().Seam(fabriccore.SeamRunLocks, true)
			built := b.Build()
			b.Seam(fabriccore.SeamRunLocks, false).Seam(fabriccore.SeamStatusCodec, true)
			want := map[fabriccore.RequiredSeam]bool{fabriccore.SeamRunLocks: true}
			if !reflect.DeepEqual(built.Seams, want) {
				t.Errorf("Seams after later setters = %v; want %v", built.Seams, want)
			}
		}},
		{"BuiltSnapshotSharesNothing", func(t *testing.T) {
			b := everySetter(fabrickit.NewSnapshot(), "one")
			built := b.Build()
			everySetter(b, "two")
			if !reflect.DeepEqual(built, wantOne) {
				t.Errorf("snapshot after later setters = %+v; want %+v", built, wantOne)
			}
			built = b.Build()
			built.Phases[0] = "overwritten"
			built.Worktrees[0].Path = "overwritten"
			built.PendingTeardowns[0].Slug = "overwritten"
			built.Outcomes[0].Action.Paths[0] = "overwritten"
			built.Outcomes[0].Detail = "overwritten"
			built.ReadFailures[0].Detail = "overwritten"
			if got := b.Build(); !reflect.DeepEqual(got, wantBoth) {
				t.Errorf("Build() after overwriting a built snapshot = %+v; want %+v", got, wantBoth)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
