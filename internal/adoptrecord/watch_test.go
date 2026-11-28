// watch_test.go pins the under-adopt accessor: which worktrees an adopt record covers,
// and a record or board that does not read.

package adoptrecord_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWatchCovers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owners := boardengine.Owners{Prime: "m2", Entries: map[string]boardengine.OwnerEntry{
		"free":  {IsProject: true, Beam: true},
		"owned": {IsProject: true, Beam: true, Owner: "m2"},
		"nest":  {IsProject: true, Beam: true},
		"deep":  {IsProject: true, Beam: true},
		"torn":  {IsProject: true, Beam: true},
	}}
	parents := map[string]string{"free": "", "owned": "", "nest": "free", "deep": "owned"}
	parentOf := func(slug string) (string, error) {
		if slug == "torn" {
			return "", errors.New("parent unreadable")
		}
		return parents[slug], nil
	}
	running := func(subjects ...string) *adoptrecord.Record {
		return &adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: subjects}
	}
	tests := []struct {
		name   string
		record *adoptrecord.Record
		// file is planted at the record's path instead of a written record.
		file      []byte
		ownersErr error
		slug      string
		want      bool
	}{
		{name: "NoRecordCoversNothing"},
		{name: "ADoneRecordCoversNothing", record: &adoptrecord.Record{State: adoptrecord.StateDone, Subjects: []string{"root"}}},
		{name: "ARootAdoptCoversThePrime", record: running("root"), want: true},
		{name: "ARootAdoptCoversABeamWithNoOwnerOfItsOwn", record: running("root"), slug: "free", want: true},
		{name: "ARootAdoptLeavesABeamWithItsOwnOwner", record: running("root"), slug: "owned"},
		{name: "ABeamAdoptCoversTheBeamListedWithNoOwnerOfItsOwn", record: running("free"), slug: "free", want: true},
		{name: "ABeamAdoptCoversABeamWithNoOwnerNestedUnderTheListedBeam", record: running("free"), slug: "nest", want: true},
		{name: "ARootAdoptStopsAtTheFirstBeamWithItsOwnOwner", record: running("root"), slug: "deep"},
		{name: "AFailingParentReadCountsAsNone", record: running("root"), slug: "torn"},
		{
			name:   "AStoppedBeamAdoptStillCoversTheBeam",
			record: &adoptrecord.Record{State: adoptrecord.StateStopped, Subjects: []string{"owned"}},
			slug:   "owned", want: true,
		},
		{
			name:   "ThePrimeOwnerWriteEndsTheRootCover",
			record: &adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: []string{"root"}, Inputs: adoptrecord.Inputs{PrimeRecorded: true}},
		},
		{
			name:   "ABeamOwnerWriteEndsItsCover",
			record: &adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: []string{"owned"}, Inputs: adoptrecord.Inputs{OwnerStood: []string{"owned"}}},
			slug:   "owned",
		},
		{name: "AnUndecodableRecordCountsAsNone", file: []byte(`{not json`)},
		{name: "AnUnreadableBoardCountsAsNone", record: running("root"), ownersErr: errors.New("board unreadable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			if tt.record != nil {
				if err := adoptrecord.Write(hub, *tt.record); err != nil {
					t.Fatal(err)
				}
			}
			if tt.file != nil {
				path := adoptrecord.Path(hub)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, tt.file, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			watch := adoptrecord.NewWatch(hub,
				func() (boardengine.Owners, error) { return owners, tt.ownersErr },
				parentOf)

			if got := watch.Covers(tt.slug); got != tt.want {
				t.Errorf("Covers(%q) = %v; want %v", tt.slug, got, tt.want)
			}
		})
	}
}
