//go:build tier0

// originowed_test.go pins the origin-owed marker a resume writes when it judged its owner without origin, and the reading of it.

package battencli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestOriginOwedMarker drives reconcileResume over a hand-populated receiver with a fake reconcile:
// an unreachable resume writes the marker and resumes, one refused for another owner on the local board writes none, a resume whose marker path cannot be written is refused naming the cause and the re-run, and a resume that reached origin leaves none.
func TestOriginOwedMarker(t *testing.T) {
	t.Parallel()

	const slug = "some-slug"
	tests := []struct {
		name          string
		report        fabricengine.OriginReport
		owner         func() *fakeOwners
		markerIsDir   bool
		wantErrors    []string
		wantMarker    bool
		wantResumable bool
	}{
		{
			name:          "an unreachable resume writes the marker and resumes",
			report:        fabricengine.OriginReport{Unreachable: true},
			owner:         func() *fakeOwners { return &fakeOwners{machine: "here", owners: ownedBy("here", "", true)} },
			wantMarker:    true,
			wantResumable: true,
		},
		{
			name:       "an unreachable resume refused for another owner on the local board writes none",
			report:     fabricengine.OriginReport{Unreachable: true},
			owner:      func() *fakeOwners { return &fakeOwners{machine: "here", owners: ownedBy("here", "there", true)} },
			wantErrors: []string{"the local board shows", "there"},
		},
		{
			name:        "an unreachable resume whose marker path cannot be written is refused naming the cause and the re-run",
			report:      fabricengine.OriginReport{Unreachable: true},
			owner:       func() *fakeOwners { return &fakeOwners{machine: "here", owners: ownedBy("here", "", true)} },
			markerIsDir: true,
			wantErrors:  []string{"could not record", "resumed without origin", "re-run lym batten run " + slug},
			wantMarker:  true,
		},
		{
			name:          "a resume that reached origin writes none",
			report:        fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryActive}}},
			owner:         func() *fakeOwners { return &fakeOwners{machine: "here", owners: ownedBy("here", "", true)} },
			wantResumable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			c.owner = tt.owner().seams()
			c.reconcile = func(string) (fabricengine.OriginReport, error) { return tt.report, nil }
			marker := shedrun.OriginOwedMarker(c.location, c.slug)
			if tt.markerIsDir {
				if err := os.MkdirAll(marker, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			err := c.reconcileResume()

			if tt.wantResumable != (err == nil) {
				t.Fatalf("reconcileResume() = %v; want resumable %v", err, tt.wantResumable)
			}
			for _, want := range tt.wantErrors {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("reconcileResume() error = %v; want it to carry %q", err, want)
				}
			}
			stands, standsErr := OriginOwed(c.location, c.slug)
			if standsErr != nil || stands != tt.wantMarker {
				t.Errorf("OriginOwed() = %v, %v; want %v, nil", stands, standsErr, tt.wantMarker)
			}
			if c.originReached != !tt.report.Unreachable {
				t.Errorf("originReached = %v; want %v", c.originReached, !tt.report.Unreachable)
			}
		})
	}
}
