//go:build tier0

// land_test.go drives the land sequence over fake deps: the refusal from inside the beam, the entries that are not a landable beam, the work-remaining refusal passed through, and the window run started on a clean beam.

package beamcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// landWorld is the fake hub every landDeps seam reads, and the log of the landing runs started.
type landWorld struct {
	owners     boardengine.Owners
	pairs      map[string]bool
	origins    map[string]fabricengine.Origin
	refusal    error
	envelope   map[string]any
	startErr   error
	started    []string
	refusedFor []string
}

// newLandWorld returns a hub with a beam "b" whose pair is here and marked, forked from the worktree "outer", and a task, a grouping project and a done beam.
func newLandWorld() *landWorld {
	return &landWorld{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"b":      {IsProject: true, Beam: true, Owner: here},
			"task1":  {IsTask: true},
			"group":  {IsProject: true},
			"landed": {IsProject: true, Beam: true, Done: true},
			"note1":  {},
		}},
		pairs:    map[string]bool{"b": true, "landed": true},
		origins:  map[string]fabricengine.Origin{"b": {ParentWorktree: "outer", Beam: true}, "landed": {Beam: true}},
		envelope: map[string]any{"ok": true, "window": "batten:b"},
	}
}

func (w *landWorld) deps() landDeps {
	return landDeps{
		ReadOwners:  func() (boardengine.Owners, error) { return w.owners, nil },
		PairPresent: func(slug string) bool { return w.pairs[slug] },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			origin, found := w.origins[slug]
			return origin, found, nil
		},
		ParentWorktree: func(slug string) (string, error) { return w.origins[slug].ParentWorktree, nil },
		Refusal: func(slug string) error {
			w.refusedFor = append(w.refusedFor, slug)
			return w.refusal
		},
		StartLanding: func(slug string) (map[string]any, error) {
			w.started = append(w.started, slug)
			return w.envelope, w.startErr
		},
	}
}

func TestLand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		acting string
		slug   string
		setup  func(w *landWorld)
		// wantErr is empty for a landing that starts, else every substring the refusal names.
		wantErr []string
		// wantRefusalRead says the work-remaining refusal was applied.
		wantRefusalRead bool
		wantStarted     bool
	}{
		{
			name:    "FromInsideTheBeamRefusedNamingTheParentWorktree",
			acting:  "b",
			slug:    "b",
			wantErr: []string{"the beam itself", "lyx beam land b", "outer"},
		},
		{name: "AbsentEntryRefused", acting: "prime", slug: "ghost", wantErr: []string{"not a beam", "lyx board list"}},
		{name: "NoteRefused", acting: "prime", slug: "note1", wantErr: []string{"not a beam"}},
		{name: "TaskRefusedNamingItsOwnLanding", acting: "prime", slug: "task1", wantErr: []string{"lyx batten run task1", "lyx beam promote task1"}},
		{name: "GroupingProjectRefusedNamingCreate", acting: "prime", slug: "group", wantErr: []string{"grouping project", "lyx beam create group"}},
		{name: "DoneBeamRefused", acting: "prime", slug: "landed", wantErr: []string{"already landed"}},
		{
			name:   "BeamWithNoPairHereRefusedNamingCreateAndAdopt",
			acting: "prime",
			slug:   "b",
			setup: func(w *landWorld) {
				w.pairs["b"] = false
			},
			wantErr: []string{"no pair on this machine", "lyx beam create b", "lyx adopt b"},
		},
		{
			name:   "PairWithNoOriginRecordRefusedNamingReconcile",
			acting: "prime",
			slug:   "b",
			setup: func(w *landWorld) {
				delete(w.origins, "b")
			},
			wantErr: []string{"lyx fabric reconcile"},
		},
		{
			name:   "UnmarkedPairRefusedNamingCreate",
			acting: "prime",
			slug:   "b",
			setup: func(w *landWorld) {
				w.origins["b"] = fabricengine.Origin{ParentWorktree: "outer"}
			},
			wantErr: []string{"beam mark", "lyx beam create b"},
		},
		{
			name:   "WorkRemainingRefusalPassesThroughAndStartsNothing",
			acting: "prime",
			slug:   "b",
			setup: func(w *landWorld) {
				w.refusal = errors.New("the open entry m is a member of the beam")
			},
			wantErr:         []string{"the open entry m is a member of the beam"},
			wantRefusalRead: true,
		},
		{
			name:            "CleanBeamStartsTheWindowRun",
			acting:          "prime",
			slug:            "b",
			wantRefusalRead: true,
			wantStarted:     true,
		},
		{
			name:            "LandsFromAnotherBeam",
			acting:          "sibling",
			slug:            "b",
			wantRefusalRead: true,
			wantStarted:     true,
		},
		{
			name:   "RefusedWindowRunNamesTheRerun",
			acting: "prime",
			slug:   "b",
			setup: func(w *landWorld) {
				w.envelope = map[string]any{"ok": false, "error": "a hub pause covers the prime"}
			},
			wantErr:         []string{"a hub pause covers the prime", "lyx beam land b"},
			wantRefusalRead: true,
			wantStarted:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := land(w.deps(), tt.acting, tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("land(%q, %q) = nil; want a refusal naming %v", tt.acting, tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("land(%q, %q) error = %q; want it to name %q", tt.acting, tt.slug, err.Error(), want)
					}
				}
			} else if err != nil {
				t.Fatalf("land(%q, %q) = %v; want the window run started", tt.acting, tt.slug, err)
			} else if got["window"] != "batten:b" {
				t.Errorf("land(%q, %q) envelope = %v; want the window run's envelope", tt.acting, tt.slug, got)
			}
			if read := len(w.refusedFor) > 0; read != tt.wantRefusalRead {
				t.Errorf("land(%q, %q) applied the work-remaining refusal = %v; want %v", tt.acting, tt.slug, read, tt.wantRefusalRead)
			}
			if started := len(w.started) > 0; started != tt.wantStarted {
				t.Errorf("land(%q, %q) started a landing = %v; want %v", tt.acting, tt.slug, started, tt.wantStarted)
			}
		})
	}
}
