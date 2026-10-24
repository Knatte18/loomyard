//go:build tier0

// create_test.go drives the create sequence over fake deps: each refusal with its text, a fresh create running its four steps in order, and a re-run finishing only the steps whose result is not yet there.

package beamcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

const (
	here  = "here"
	there = "there"
)

// createWorld is the fake hub every createDeps seam reads and writes, and the log of the calls that changed it.
type createWorld struct {
	owners   boardengine.Owners
	beams    map[string]string
	pairs    map[string]bool
	marked   map[string]bool
	onOrigin map[string]bool
	lost     map[string]string
	addOpts  fabricengine.AddOptions
	calls    []string
}

// newCreateWorld returns a hub owned by here with a grouping project, a beam project owned by there, a task, a note and a done project.
func newCreateWorld() *createWorld {
	return &createWorld{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"group":  {IsProject: true},
			"theirs": {IsProject: true, Beam: true, Owner: there},
			"task1":  {IsTask: true},
			"note1":  {},
			"old":    {IsProject: true, Done: true},
		}},
		beams:    map[string]string{},
		pairs:    map[string]bool{},
		marked:   map[string]bool{},
		onOrigin: map[string]bool{},
	}
}

func (w *createWorld) deps() createDeps {
	return createDeps{
		Machine:    func() (string, error) { return here, nil },
		ReadOwners: func() (boardengine.Owners, error) { return w.owners, nil },
		ParentOf:   func(string) (string, error) { return "", nil },
		ParentBeam: func(slug string) (string, error) { return w.beams[slug], nil },
		PairPresent: func(slug string) bool {
			return w.pairs[slug]
		},
		BranchOnOrigin: func(slug string) (bool, error) { return w.onOrigin[slug], nil },
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			w.calls = append(w.calls, "owner")
			slug := req.Slugs[0]
			if owner, lost := w.lost[slug]; lost {
				return boardengine.OwnerWrite{Lost: map[string]string{slug: owner}}, nil
			}
			entry := w.owners.Entries[slug]
			entry.Owner, entry.Beam = req.Machine, req.Beam
			if req.ToProject {
				entry.IsProject, entry.IsTask = true, false
			}
			w.owners.Entries[slug] = entry
			return boardengine.OwnerWrite{Recorded: []string{slug}}, nil
		},
		AddPair: func(slug string, opts fabricengine.AddOptions) (fabricengine.AddResult, error) {
			w.calls = append(w.calls, "add")
			w.addOpts = opts
			w.pairs[slug], w.marked[slug] = true, opts.Beam
			return fabricengine.AddResult{}, nil
		},
		MarkBeam: func(_ *fabricengine.Mutations, slug string) (bool, error) {
			if w.marked[slug] {
				return false, nil
			}
			w.calls = append(w.calls, "mark")
			w.marked[slug] = true
			return true, nil
		},
		SpawnPush: func(string) error {
			w.calls = append(w.calls, "push")
			return nil
		},
		RegenerateWorkspace: func(*fabricengine.Mutations) (string, error) {
			w.calls = append(w.calls, "workspace")
			return "/hub/repo.code-workspace", nil
		},
		WorktreePath: func(slug string) string { return "/hub/" + slug },
		HubPath:      "/hub",
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slug  string
		setup func(w *createWorld)
		// wantErr is empty for a create that finishes, else every substring the refusal names.
		wantErr   []string
		wantCalls []string
		wantAdd   fabricengine.AddOptions
	}{
		{name: "AbsentEntryRefused", slug: "ghost", wantErr: []string{"lyx board upsert", "project"}},
		{name: "NoteRefused", slug: "note1", wantErr: []string{"lyx board upsert", "project"}},
		{name: "TaskRefused", slug: "task1", wantErr: []string{"lyx beam promote task1"}},
		{name: "DoneProjectRefused", slug: "old", wantErr: []string{"done", "new project entry"}},
		{name: "BeamOwnedElsewhereRefused", slug: "theirs", wantErr: []string{there, "lyx beam create theirs", "lyx adopt theirs"}},
		{
			name:      "FreshCreateRunsTheFourStepsInOrder",
			slug:      "group",
			wantCalls: []string{"owner", "add", "push", "workspace"},
			wantAdd:   fabricengine.AddOptions{Beam: true},
		},
		{
			name: "NestedBeamForksFromItsParentBeam",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
				w.pairs["outer"] = true
			},
			wantCalls: []string{"owner", "add", "push", "workspace"},
			wantAdd:   fabricengine.AddOptions{Parent: "outer", Beam: true},
		},
		{
			name: "RerunAfterTheOwnerStepSkipsIt",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
			},
			wantCalls: []string{"add", "push", "workspace"},
			wantAdd:   fabricengine.AddOptions{Beam: true},
		},
		{
			name: "RerunAfterThePairStepAddsNoSecondPair",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
				w.pairs["group"], w.marked["group"] = true, true
			},
			wantCalls: []string{"push", "workspace"},
		},
		{
			name: "BeamProjectWithNoOwnerGetsOneAndItsPair",
			slug: "group",
			setup: func(w *createWorld) {
				w.owners.Entries["group"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			},
			wantCalls: []string{"owner", "add", "push", "workspace"},
			wantAdd:   fabricengine.AddOptions{Beam: true},
		},
		{
			name: "UnmarkedPairIsMarkedAndNotAdded",
			slug: "group",
			setup: func(w *createWorld) {
				w.pairs["group"] = true
			},
			wantCalls: []string{"owner", "mark", "push", "workspace"},
		},
		{
			name: "ParentBeamAbsentOffOriginNamesItsCreate",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
			},
			wantErr:   []string{"outer", "lyx beam create outer"},
			wantCalls: []string{"owner"},
		},
		{
			name: "ParentBeamAbsentOnOriginNamesItsAdopt",
			slug: "group",
			setup: func(w *createWorld) {
				w.beams["group"] = "outer"
				w.onOrigin["outer"] = true
			},
			// The owner write comes first, and the refusal stops the pair step.
			wantErr:   []string{"outer", "lyx adopt outer"},
			wantCalls: []string{"owner"},
		},
		{
			name: "OwnerWriteLostToAnotherMachineStopsTheCreate",
			slug: "group",
			setup: func(w *createWorld) {
				w.lost = map[string]string{"group": there}
			},
			wantErr:   []string{there, "lyx adopt group"},
			wantCalls: []string{"owner"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newCreateWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := create(w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("create(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("create(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if !slices.Equal(w.calls, tt.wantCalls) {
					t.Errorf("create(%q) ran %v; want %v", tt.slug, w.calls, tt.wantCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("create(%q) = %v; want it to finish", tt.slug, err)
			}
			if !slices.Equal(w.calls, tt.wantCalls) {
				t.Errorf("create(%q) ran %v; want %v", tt.slug, w.calls, tt.wantCalls)
			}
			if slices.Contains(tt.wantCalls, "add") && (w.addOpts.Parent != tt.wantAdd.Parent || w.addOpts.Beam != tt.wantAdd.Beam) {
				t.Errorf("create(%q) added the pair with %+v; want %+v", tt.slug, w.addOpts, tt.wantAdd)
			}
			if got.Worktree != "/hub/"+tt.slug || !strings.Contains(got.WayForward, "lyx orch start") || !strings.Contains(got.WayForward, got.Worktree) {
				t.Errorf("create(%q) envelope = %+v; want the beam's worktree and `lyx orch start` there", tt.slug, got)
			}
		})
	}
}

func TestCreate_StepFailureNamesTheRerun(t *testing.T) {
	t.Parallel()

	w := newCreateWorld()
	deps := w.deps()
	deps.SpawnPush = func(string) error { return errors.New("spawn failed") }

	got, err := create(deps, "group")

	if err == nil || !strings.Contains(err.Error(), "spawn failed") || !strings.Contains(err.Error(), "lyx beam create group") {
		t.Fatalf("create = %v; want the push failure and a re-run of `lyx beam create group`", err)
	}
	if !got.OwnerRecorded || !got.PairAdded || got.PushSpawned {
		t.Errorf("result = %+v; want the owner and the pair done and no push", got)
	}
}
