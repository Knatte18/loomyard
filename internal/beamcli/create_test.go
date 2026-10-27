//go:build tier0

// create_test.go drives the create sequence over fake deps: each refusal with its text, a fresh create running its steps in order with the run start last, and a re-run finishing only the steps whose result is not yet there.

package beamcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	here      = "here"
	there     = "there"
	primeName = "prime"
)

// createWorld is the fake hub every createDeps seam reads and writes, and the log of the calls that changed it.
type createWorld struct {
	owners   boardengine.Owners
	beams    map[string]string
	parents  map[string]string
	pairs    map[string]bool
	onOrigin map[string]bool
	lost     map[string]string
	// primeWinner is the machine whose root claim wins over this one's, empty when this machine's claim stands.
	primeWinner string
	acting      string
	// runs holds the state of each slug's batten run; a slug with no entry has no status file.
	runs map[string]shedengine.State
	// envelope is what the first pass answers, and startErr what its start fails with.
	envelope map[string]any
	startErr error
	// startAnchors is the anchor of each StartRun call.
	startAnchors []string
	calls        []string
}

// newCreateWorld returns a hub owned by here with a grouping project, a beam project owned by there, a task, a note and a done project, acted on from the prime.
func newCreateWorld() *createWorld {
	return &createWorld{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"group":  {IsProject: true},
			"theirs": {IsProject: true, Beam: true, Owner: there},
			"outer":  {IsProject: true, Beam: true, Owner: here},
			"task1":  {IsTask: true},
			"note1":  {},
			"old":    {IsProject: true, Done: true},
		}},
		beams:    map[string]string{},
		parents:  map[string]string{},
		pairs:    map[string]bool{},
		onOrigin: map[string]bool{},
		runs:     map[string]shedengine.State{},
		acting:   primeName,
		envelope: map[string]any{"ok": true, "pid": float64(4242)},
	}
}

func (w *createWorld) deps() createDeps {
	return createDeps{
		Acting:      w.acting,
		PrimeName:   primeName,
		Machine:     func() (string, error) { return here, nil },
		ReadOwners:  func() (boardengine.Owners, error) { return w.owners, nil },
		ParentOf:    func(slug string) (string, error) { return w.parents[slug], nil },
		ParentBeam:  func(slug string) (string, error) { return w.beams[slug], nil },
		PairPresent: func(slug string) bool { return w.pairs[slug] },
		BranchOnOrigin: func(slug string) (bool, error) {
			return w.onOrigin[slug], nil
		},
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			if req.Prime {
				w.calls = append(w.calls, "root")
				if w.primeWinner != "" {
					w.owners.Prime = w.primeWinner
					return boardengine.OwnerWrite{Lost: map[string]string{boardengine.OwnerWritePrime: w.primeWinner}}, nil
				}
				w.owners.Prime = req.Machine
				return boardengine.OwnerWrite{Recorded: []string{boardengine.OwnerWritePrime}}, nil
			}
			w.calls = append(w.calls, "owner")
			slug := req.Slugs[0]
			if owner, lost := w.lost[slug]; lost {
				return boardengine.OwnerWrite{Lost: map[string]string{slug: owner}}, nil
			}
			entry := w.owners.Entries[slug]
			entry.Owner, entry.Beam = req.Machine, req.Beam
			w.owners.Entries[slug] = entry
			return boardengine.OwnerWrite{Recorded: []string{slug}}, nil
		},
		RunStatus: func(slug string) (shedengine.State, bool, error) {
			state, found := w.runs[slug]
			return state, found, nil
		},
		StartRun: func(slug, anchor string) (map[string]any, error) {
			w.calls = append(w.calls, "start")
			w.startAnchors = append(w.startAnchors, anchor)
			return w.envelope, w.startErr
		},
		WorktreePath: func(slug string) string { return "/hub/" + slug },
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		slug   string
		anchor string
		setup  func(w *createWorld)
		// wantErr is empty for a create that finishes, else every substring the refusal names.
		wantErr   []string
		wantCalls []string
		// wantAnchors is the anchor of each run start.
		wantAnchors []string
		// check judges the envelope of a create that finishes.
		check func(t *testing.T, got createResult)
	}{
		{
			name:    "CreateFromAnotherWorktreeRefusedNamingTheParentsPath",
			slug:    "group",
			setup:   func(w *createWorld) { w.acting = "elsewhere" },
			wantErr: []string{"/hub/prime", "lym beam create group"},
		},
		{
			name: "NestedBeamFromThePrimeRefusedNamingTheParentBeamsPath",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
				w.pairs["outer"] = true
			},
			wantErr: []string{"/hub/outer", "lym beam create group"},
		},
		{
			name: "NestedBeamWhoseParentBeamHasNoPairHereNamesItsCreate",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
			},
			wantErr: []string{"outer", "lym beam create outer", "lym beam create group"},
		},
		{
			name: "NestedBeamWhoseParentBeamHasNoPairHereNamesItsAdoptWhenOnOrigin",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
				w.onOrigin["outer"] = true
			},
			wantErr: []string{"outer", "lym adopt outer", "lym beam create group"},
		},
		{name: "AbsentEntryRefused", slug: "ghost", wantErr: []string{"lym board upsert", "project"}},
		{name: "NoteRefused", slug: "note1", wantErr: []string{"lym board upsert", "project"}},
		{name: "TaskRefused", slug: "task1", wantErr: []string{"is a task", "lym board upsert", "lym beam create <that slug>"}},
		{name: "DoneProjectRefused", slug: "old", wantErr: []string{"done", "new project entry"}},
		{name: "BeamOwnedElsewhereRefused", slug: "theirs", wantErr: []string{there, "lym beam create theirs", "lym adopt theirs"}},
		{
			name:    "RootOwnedElsewhereRefusedNamingAdoptRoot",
			slug:    "group",
			setup:   func(w *createWorld) { w.owners.Prime = there },
			wantErr: []string{there, "lym adopt root"},
		},
		{
			name: "ParentBeamOwnedElsewhereRefusedNamingItsAdopt",
			slug: "group",
			setup: func(w *createWorld) {
				w.acting = "outer"
				w.beams["group"] = "outer"
				w.parents["group"] = "outer"
				w.pairs["outer"] = true
				w.owners.Entries["outer"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			},
			wantErr: []string{there, "lym adopt outer"},
		},
		{
			name:      "FreshCreateClaimsTheRootRecordsTheOwnerAndStartsTheRunWithItsAnchor",
			slug:      "group",
			anchor:    "y",
			setup:     func(w *createWorld) { w.owners.Prime = "" },
			wantCalls: []string{"root", "owner", "start"},
			// The owner step carries the beam flag and the pawl recipe in the one write.
			wantAnchors: []string{"y"},
			check: func(t *testing.T, got createResult) {
				if !got.RootClaimed || !got.OwnerRecorded || !got.RunStarted || got.RunLive || got.FirstPass["pid"] != float64(4242) {
					t.Errorf("create envelope = %+v; want the root claimed, the owner recorded and the run started with its first pass", got)
				}
			},
		},
		{
			name:        "RootAlreadyClaimedSkipsTheClaim",
			slug:        "group",
			wantCalls:   []string{"owner", "start"},
			wantAnchors: []string{""},
			check: func(t *testing.T, got createResult) {
				if got.RootClaimed {
					t.Errorf("create envelope = %+v; want no root claim over a recorded root owner", got)
				}
			},
		},
		{
			name: "NestedBeamUnderAParentBeamOfThisMachineStartsItsRun",
			slug: "group",
			setup: func(w *createWorld) {
				w.acting = "outer"
				w.beams["group"] = "outer"
				w.parents["group"] = "outer"
				w.pairs["outer"] = true
			},
			wantCalls:   []string{"owner", "start"},
			wantAnchors: []string{""},
		},
		{
			name: "RootClaimWonByAnotherMachineRefusedNamingAdoptRoot",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Prime = ""
				w.primeWinner = there
			},
			wantErr:   []string{there, "lym adopt root"},
			wantCalls: []string{"root"},
		},
		{
			name: "OwnerWriteLostToAnotherMachineRefusedNamingAdoptTheBeam",
			slug: "group",
			setup: func(w *createWorld) {
				w.lost = map[string]string{"group": there}
			},
			wantErr:   []string{there, "lym adopt group"},
			wantCalls: []string{"owner"},
		},
		{
			name: "RerunAfterTheOwnerStepSkipsIt",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
			},
			wantCalls:   []string{"start"},
			wantAnchors: []string{""},
		},
		{
			name: "PawlRunInFlightIsReportedLiveAndStartsNothing",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
				w.runs["group"] = shedengine.StateRunning
			},
			check: func(t *testing.T, got createResult) {
				if !got.RunLive || got.RunStarted || !strings.Contains(got.WayForward, "lym batten status group") {
					t.Errorf("create envelope = %+v; want the run live, none started and `lym batten status group` named", got)
				}
			},
		},
		{
			name:   "FlaggedBeamWithNoRunStatusStartsItsRunAgainWithTheAnchorTyped",
			slug:   "group",
			anchor: "y",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
			},
			wantCalls:   []string{"start"},
			wantAnchors: []string{"y"},
			check: func(t *testing.T, got createResult) {
				if got.OwnerRecorded || !got.RunStarted || !strings.Contains(got.WayForward, "lym batten status group") {
					t.Errorf("create envelope = %+v; want no owner write, the run started and `lym batten status group` named", got)
				}
			},
		},
		{
			name: "RefusedFirstPassIsTheVerbsErrorCarryingTheEnvelopesError",
			slug: "group",
			setup: func(w *createWorld) {
				w.envelope = map[string]any{"ok": false, "error": "a hub pause covers the prime"}
			},
			wantErr:     []string{"a hub pause covers the prime", "lym beam create group"},
			wantCalls:   []string{"owner", "start"},
			wantAnchors: []string{""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newCreateWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := create(w.deps(), tt.slug, tt.anchor)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("create(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("create(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if strings.Contains(err.Error(), "lym beam promote") {
					t.Errorf("create(%q) error = %q; want no lym beam promote", tt.slug, err.Error())
				}
				if got.RunStarted {
					t.Errorf("create(%q) envelope = %+v; want no run started by a refused create", tt.slug, got)
				}
			} else if err != nil {
				t.Fatalf("create(%q) = %v; want it to finish", tt.slug, err)
			}
			if !slices.Equal(w.calls, tt.wantCalls) {
				t.Errorf("create(%q) ran %v; want %v", tt.slug, w.calls, tt.wantCalls)
			}
			if !slices.Equal(w.startAnchors, tt.wantAnchors) {
				t.Errorf("create(%q) started the run with the anchors %q; want %q", tt.slug, w.startAnchors, tt.wantAnchors)
			}
			if len(tt.wantErr) == 0 && got.Worktree != "/hub/"+tt.slug {
				t.Errorf("create(%q) envelope = %+v; want the beam's worktree", tt.slug, got)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestCreate_StepFailureNamesTheRerun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fail      func(deps *createDeps)
		want      string
		wantOwner bool
	}{
		{
			name: "OwnerWriteFailure",
			fail: func(deps *createDeps) {
				deps.SetOwners = func(boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
					return boardengine.OwnerWrite{}, errors.New("push refused")
				}
			},
			want: "push refused",
		},
		{
			name: "RunStartFailure",
			fail: func(deps *createDeps) {
				deps.StartRun = func(string, string) (map[string]any, error) { return nil, errors.New("arm failed") }
			},
			want:      "arm failed",
			wantOwner: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newCreateWorld()
			deps := w.deps()
			tt.fail(&deps)

			got, err := create(deps, "group", "")

			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "lym beam create group") {
				t.Fatalf("create = %v; want the failure %q and a re-run of `lym beam create group`", err, tt.want)
			}
			if got.OwnerRecorded != tt.wantOwner || got.RunStarted {
				t.Errorf("result = %+v; want the owner recorded = %v and no run started", got, tt.wantOwner)
			}
		})
	}
}
