//go:build tier0

// land_test.go drives the land and ready sequences over fake deps: the entries that are not a landable beam, the pawl run, owner and ready refusals, the work-remaining refusal passed through, the land request written for a beam waiting at Beam-Await, and the ready record written once.

package beamcli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// landNow is the time the fake hub's clock reads.
var landNow = time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)

// landWorld is the fake hub every landDeps seam reads, and the log of the land requests written.
type landWorld struct {
	owners  boardengine.Owners
	parents map[string]string
	pairs   map[string]bool
	origins map[string]fabricengine.Origin
	// children holds each beam's pawl run status; a beam with no entry has no pawl run.
	children map[string]shedengine.Status
	// ready holds each beam whose ready record flags it, and marked the ready records written.
	ready      map[string]bool
	marked     []pawlengine.ReadyRecord
	runID      string
	refusal    error
	refusedFor []string
	requests   []pawlengine.LandRequest
}

// newLandWorld returns a hub owned by here with a beam "b" whose pair is here and marked and whose pawl run waits at Beam-Await, a task, a grouping project and a done beam.
func newLandWorld() *landWorld {
	return &landWorld{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"b":      {IsProject: true, Beam: true, Owner: here},
			"task1":  {IsTask: true},
			"group":  {IsProject: true},
			"landed": {IsProject: true, Beam: true, Done: true},
			"note1":  {},
		}},
		parents:  map[string]string{},
		pairs:    map[string]bool{"b": true, "landed": true},
		origins:  map[string]fabricengine.Origin{"b": {Beam: true}, "landed": {Beam: true}},
		children: map[string]shedengine.Status{"b": {State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}},
		runID:    "2026-02-28T08:00:00Z",
		ready:    map[string]bool{"b": true},
	}
}

func (w *landWorld) deps() landDeps {
	return landDeps{
		Machine:     func() (string, error) { return here, nil },
		ReadOwners:  func() (boardengine.Owners, error) { return w.owners, nil },
		ParentOf:    func(slug string) (string, error) { return w.parents[slug], nil },
		PairPresent: func(slug string) bool { return w.pairs[slug] },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			origin, found := w.origins[slug]
			return origin, found, nil
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			status, found := w.children[slug]
			return status, found, nil
		},
		RunID: func(string) (string, error) { return w.runID, nil },
		Ready: func(slug string) (bool, error) { return w.ready[slug], nil },
		MarkReady: func(slug string, record pawlengine.ReadyRecord) (string, error) {
			w.marked = append(w.marked, record)
			return "/hub/" + slug + "/_lym/landing/beam-ready.json", nil
		},
		Refusal: func(slug string) error {
			w.refusedFor = append(w.refusedFor, slug)
			return w.refusal
		},
		WriteRequest: func(slug string, req pawlengine.LandRequest) (string, error) {
			w.requests = append(w.requests, req)
			return "/hub/" + slug + "/.lym/beam/land-request.json", nil
		},
		Now: func() time.Time { return landNow },
	}
}

func TestLand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a land that writes its request, else every substring the refusal names.
		wantErr []string
		// wantRefusalRead says the work-remaining refusal was applied.
		wantRefusalRead bool
	}{
		{name: "AbsentEntryRefused", slug: "ghost", wantErr: []string{"not a beam", "lym board list"}},
		{name: "NoteRefused", slug: "note1", wantErr: []string{"not a beam"}},
		{name: "TaskRefusedNamingItsOwnLanding", slug: "task1", wantErr: []string{"lym batten run task1", "lym board upsert", "lym beam create <that slug>"}},
		{name: "GroupingProjectRefusedNamingCreate", slug: "group", wantErr: []string{"grouping project", "lym beam create group"}},
		{name: "DoneBeamRefused", slug: "landed", wantErr: []string{"already landed"}},
		{
			name: "BeamWithNoPairHereRefusedNamingCreateAndAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.pairs["b"] = false
			},
			wantErr: []string{"no pair on this machine", "lym beam create b", "lym adopt b"},
		},
		{
			name: "PairWithNoOriginRecordRefusedNamingReconcile",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.origins, "b")
			},
			wantErr: []string{"lym fabric reconcile"},
		},
		{
			name: "UnmarkedPairRefusedNamingCreate",
			slug: "b",
			setup: func(w *landWorld) {
				w.origins["b"] = fabricengine.Origin{}
			},
			wantErr: []string{"beam mark", "lym beam create b"},
		},
		{
			name: "BeamWithNoPawlRunRefusedNamingCreate",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.children, "b")
			},
			wantErr: []string{"no pawl run", "lym beam create b"},
		},
		{
			name: "RunAwayFromBeamAwaitRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Parent-Align"}
			},
			wantErr: []string{"Parent-Align", pawlengine.NameBeamAwait, "lym batten status b"},
		},
		{
			name: "PausedRunAtBeamAwaitRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StatePaused, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantErr: []string{"lym batten status b"},
		},
		{
			name: "BeamOwnedElsewhereRefusedNamingAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			},
			wantErr: []string{"owned by machine there", "lym adopt b"},
		},
		{
			name: "ParentOwnedElsewhereRefusedNamingRootAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.owners.Prime = there
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			},
			wantErr: []string{"parent of b is owned by machine there", "lym adopt root"},
		},
		{
			name: "NestedBeamsParentOwnedElsewhereRefusedNamingItsBeam",
			slug: "inner",
			setup: func(w *landWorld) {
				w.owners.Entries["outer"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
				w.owners.Entries["inner"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
				w.parents["inner"] = "outer"
				w.pairs["inner"] = true
				w.origins["inner"] = fabricengine.Origin{Beam: true}
				w.children["inner"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantErr: []string{"parent of inner is owned by machine there", "lym adopt outer"},
		},
		{
			name: "UnflaggedBeamRefusedNamingReady",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.ready, "b")
			},
			wantErr: []string{"not flagged ready", "lym beam ready b"},
		},
		{
			name: "WorkRemainingRefusalPassesThroughAndWritesNothing",
			slug: "b",
			setup: func(w *landWorld) {
				w.refusal = errors.New("the open entry m is a member of the beam")
			},
			wantErr:         []string{"the open entry m is a member of the beam"},
			wantRefusalRead: true,
		},
		{name: "CleanBeamWritesOneRequest", slug: "b", wantRefusalRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := land(w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("land(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("land(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if strings.Contains(err.Error(), "lym beam promote") {
					t.Errorf("land(%q) error = %q; want no lym beam promote", tt.slug, err.Error())
				}
				if len(w.requests) != 0 {
					t.Errorf("land(%q) wrote %d requests on a refusal; want none", tt.slug, len(w.requests))
				}
			} else {
				if err != nil {
					t.Fatalf("land(%q) = %v; want the request written", tt.slug, err)
				}
				if got["request"] != "/hub/b/.lym/beam/land-request.json" || got["run_id"] != w.runID || !strings.Contains(got["way_forward"].(string), "lym batten status b") {
					t.Errorf("land(%q) = %v; want the request's path, the run id and `lym batten status b`", tt.slug, got)
				}
				want := []pawlengine.LandRequest{{RunID: w.runID, RequestedAt: landNow}}
				if len(w.requests) != 1 || w.requests[0] != want[0] {
					t.Errorf("land(%q) wrote requests %v; want exactly %v", tt.slug, w.requests, want)
				}
			}
			if read := len(w.refusedFor) > 0; read != tt.wantRefusalRead {
				t.Errorf("land(%q) applied the work-remaining refusal = %v; want %v", tt.slug, read, tt.wantRefusalRead)
			}
		})
	}
}

func TestMarkReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a ready that succeeds, else every substring the refusal names.
		wantErr []string
		// wantAlready says the beam read as flagged before the call, so nothing is written.
		wantAlready bool
	}{
		{name: "TaskRefusedNamingItsOwnLanding", slug: "task1", wantErr: []string{"lym beam ready:", "lym batten run task1"}},
		{name: "GroupingProjectRefusedNamingCreate", slug: "group", wantErr: []string{"lym beam ready:", "lym beam create group"}},
		{
			name: "BeamWithNoPairHereRefusedNamingCreateAndAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.pairs["b"] = false
			},
			wantErr: []string{"lym beam ready:", "lym beam create b", "lym adopt b"},
		},
		{
			name: "UnflaggedBeamWritesOneRecordNamingIt",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.ready, "b")
			},
		},
		{name: "FlaggedBeamWritesNothing", slug: "b", wantAlready: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := markReady(w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("markReady(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("markReady(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if len(w.marked) != 0 {
					t.Errorf("markReady(%q) wrote %d records on a refusal; want none", tt.slug, len(w.marked))
				}
				return
			}
			if err != nil {
				t.Fatalf("markReady(%q) = %v; want nil", tt.slug, err)
			}
			if got["already"] != tt.wantAlready || !strings.Contains(got["way_forward"].(string), "lym batten status b") {
				t.Errorf("markReady(%q) = %v; want already %t and `lym batten status b`", tt.slug, got, tt.wantAlready)
			}
			var want []pawlengine.ReadyRecord
			if !tt.wantAlready {
				want = []pawlengine.ReadyRecord{{Slug: "b", SetAt: landNow}}
			}
			if !reflect.DeepEqual(w.marked, want) {
				t.Errorf("markReady(%q) wrote records %v; want %v", tt.slug, w.marked, want)
			}
		})
	}
}
