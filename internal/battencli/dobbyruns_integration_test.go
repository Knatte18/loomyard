// dobbyruns_integration_test.go pins what DobbyRuns reads of a run:
// its history, reason, kept cancel, hand-over mark, owner and Bolt hold.
// Each row runs one status and one board over its own hubforge hub.

package battencli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDobbyRuns_ReadsTheKeptCancelTheHandOverMarkTheOwnerAndTheBoltHold(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const history = `"error":"waits on the child","history":[{"producer":"Seed-Child","outcome":"done"},{"producer":"Run-Shed","outcome":"stuck","repeats":2}]`
	const running = `{"state":"running","current_producer":"Run-Shed",` + history + `}`
	const keptCancel = `{"state":"cancelled","current_producer":"Run-Shed",` + history + `,"cancel":{"started_at":"2026-11-15T09:00:00Z",` +
		`"machine":"other-machine","remove":false,"finish":{"ended_at":"2026-11-15T09:30:00Z","remove":false}}}`
	const entry = `{"id":0,"slug":"subject","title":"S","kind":"task","labels":["bug"],"status":"draft"}`
	const ownedByRegisteredMachine = `"prime_owner":"other-machine","machines":{"0123456789abcdef0123456789abcdef":{"name":"other-machine"}},`
	const adoptHold = `"holds":[{"kind":"adopt","machine":"other-machine","at":"2026-11-17T09:00:00Z","subjects":[]}],`

	type reading struct {
		pairKept       bool
		keptSince      string
		handOverMarked bool
		ownedElsewhere bool
		boltHeld       bool
		historyLen     int
		lastRepeats    int
		reason         string
	}
	// told is the history and reason every status carries, with change applied.
	told := func(change reading) reading {
		change.historyLen, change.lastRepeats, change.reason = 2, 2, "waits on the child"
		return change
	}
	tests := []struct {
		name     string
		status   string
		members  string
		markMade bool
		want     reading
	}{
		{name: "RunningRunWithNoneOfThemReadsNone", status: running, want: told(reading{})},
		{name: "CancelledWithKeepAndFinishReadsKeptSinceTheFinish", status: keptCancel,
			want: told(reading{pairKept: true, keptSince: "2026-11-15T09:30:00Z"})},
		{name: "HandedOverRunOwnedByAnotherRegisteredMachineReadsMarkedAndElsewhere", status: running, members: ownedByRegisteredMachine, markMade: true,
			want: told(reading{handOverMarked: true, ownedElsewhere: true})},
		{name: "RunUnderABoltHoldReadsHeld", status: running, members: adoptHold,
			want: told(reading{boltHeld: true})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "subject")
			for path, content := range map[string]string{shedrun.SeedFile(h.Location, "subject"): `{"recipe":"batten","driver":"go"}`, shedrun.StatusFile(h.Location, "subject"): tt.status} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create %s: %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			writeHubBoard(t, h, `{"version":1,`+tt.members+`"entries":[`+entry+`]}`)
			if tt.markMade {
				mark := fabriclayout.HandOverMark{Machine: "other-machine", At: "2026-11-15T09:00:00Z", Written: time.Date(2026, 11, 15, 9, 5, 0, 0, time.UTC)}
				if err := fabricengine.WriteHandOverMark(h.Path, "subject", mark); err != nil {
					t.Fatalf("write the hand-over mark: %v", err)
				}
			}

			views, err := DobbyRuns(h.Location, dobbyengine.Geometry{BoardDir: h.BoardDir()}, NewDobbyParents(h.Location), filepath.Join(t.TempDir(), "pause.json"), runpusher.StartIfAhead)

			if err != nil {
				t.Fatalf("DobbyRuns() error = %v; want nil", err)
			}
			if len(views) != 1 || views[0].Slug != "subject" {
				t.Fatalf("DobbyRuns() = %+v; want the run subject alone", views)
			}
			got := reading{pairKept: views[0].PairKept, handOverMarked: views[0].HandOverMarked, ownedElsewhere: views[0].OwnedElsewhere, boltHeld: views[0].BoltHeld,
				historyLen: views[0].HistoryLen, lastRepeats: views[0].LastRepeats, reason: views[0].Error}
			if !views[0].KeptSince.IsZero() {
				got.keptSince = views[0].KeptSince.UTC().Format(time.RFC3339)
			}
			if got != tt.want {
				t.Errorf("DobbyRuns() reading = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestDobbyRuns_StartsThePusherOfOnlyAPairThisMachineOwns asks for the pusher of an owned run's pair and of no other.
func TestDobbyRuns_StartsThePusherOfOnlyAPairThisMachineOwns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const entry = `{"id":0,"slug":"subject","title":"S","kind":"task","labels":["bug"],"status":"draft"}`
	const ownedByRegisteredMachine = `"prime_owner":"other-machine","machines":{"0123456789abcdef0123456789abcdef":{"name":"other-machine"}},`

	tests := []struct {
		name     string
		members  string
		markMade bool
		want     []string
	}{
		{name: "RunOwnedHereStartsThePusherOfItsPair", want: []string{"subject/subject"}},
		{name: "RunOwnedByAnotherMachineStartsNone", members: ownedByRegisteredMachine, markMade: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "subject")
			for path, content := range map[string]string{
				shedrun.SeedFile(h.Location, "subject"):   `{"recipe":"batten","driver":"go"}`,
				shedrun.StatusFile(h.Location, "subject"): `{"state":"running","current_producer":"Run-Shed"}`,
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create %s: %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			writeHubBoard(t, h, `{"version":1,`+tt.members+`"entries":[`+entry+`]}`)
			if tt.markMade {
				mark := fabriclayout.HandOverMark{Machine: "other-machine", At: "2026-11-15T09:00:00Z", Written: time.Date(2026, 11, 15, 9, 5, 0, 0, time.UTC)}
				if err := fabricengine.WriteHandOverMark(h.Path, "subject", mark); err != nil {
					t.Fatalf("write the hand-over mark: %v", err)
				}
			}
			var started []string
			start := func(target runpusher.Target) (bool, error) {
				started = append(started, target.Location.WorktreeName+"/"+target.Slug)
				return false, nil
			}

			_, err := DobbyRuns(h.Location, dobbyengine.Geometry{BoardDir: h.BoardDir()}, NewDobbyParents(h.Location), filepath.Join(t.TempDir(), "pause.json"), start)

			if err != nil {
				t.Fatalf("DobbyRuns() error = %v; want nil", err)
			}
			if !slices.Equal(started, tt.want) {
				t.Errorf("pushers asked for = %q; want %q", started, tt.want)
			}
		})
	}
}
