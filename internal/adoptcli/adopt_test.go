//go:build tier0

// adopt_test.go drives the takeover sequence over fake deps: the refusals before anything runs, the reconcile stops, the per-entry stops and their two ways forward, the pair add, the owner write, the pause lift, the run resume and the halted-child resume.

package adoptcli

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

const (
	here  = "here"
	there = "there"
)

// world is the fake hub every deps seam reads and writes.
type world struct {
	owners       boardengine.Owners
	report       fabricengine.OriginReport
	reconcileErr error
	reconciled   bool
	pairs        map[string]bool
	live         map[string]bool
	parents      map[string]string
	addErr       map[string]error
	dropTags     []string
	dropped      []string
	setErr       error
	lost         map[string]string
	ownerReq     boardengine.OwnerRequest
	record       pauserecord.Record
	lifted       []tenderengine.EntryRef
	runs         map[string]shedengine.Status
	resumeErr    map[string]error
	resumed      []string
	children     map[string]shedengine.Status
	childResumed []string
}

// newWorld returns a hub with two tasks owned by another machine, each with a pair and a running batten run, an idle task, a done task and a note.
func newWorld() *world {
	running := shedengine.Status{State: shedengine.StateRunning}
	return &world{
		owners: boardengine.Owners{Prime: there, Entries: map[string]boardengine.OwnerEntry{
			"alpha": {Owner: there, IsTask: true, RunHeld: true},
			"beta":  {Owner: there, IsTask: true, RunHeld: true},
			"idle":  {IsTask: true},
			"done1": {Owner: there, IsTask: true, Done: true},
			"note1": {},
		}},
		report: fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{
			{Slug: "alpha", State: fabricengine.EntryOwnedElsewhere, Owner: there},
			{Slug: "beta", State: fabricengine.EntryOwnedElsewhere, Owner: there},
		}},
		pairs:     map[string]bool{"alpha": true, "beta": true},
		live:      map[string]bool{},
		parents:   map[string]string{},
		addErr:    map[string]error{},
		resumeErr: map[string]error{},
		runs:      map[string]shedengine.Status{"alpha": running, "beta": running},
		children:  map[string]shedengine.Status{},
	}
}

// stopBranch adds a judged branch of slug to the report.
func (w *world) stopBranch(slug string, state fabricengine.BranchState) {
	w.report.Branches = append(w.report.Branches, fabricengine.BranchOutcome{Branch: "code-" + slug, Repo: "code", Slug: slug, State: state})
}

func (w *world) deps() adoptDeps {
	return adoptDeps{
		Machine:    func() (string, error) { return here, nil },
		ReadOwners: func() (boardengine.Owners, error) { return w.owners, nil },
		ParentOf:   func(slug string) (string, error) { return w.parents[slug], nil },
		Reconcile: func() (fabricengine.OriginReport, error) {
			w.reconciled = true
			return w.report, w.reconcileErr
		},
		LocalPair: func(slug string) bool { return w.pairs[slug] },
		LivePair:  func(slug string) (bool, error) { return w.live[slug], nil },
		AddPair: func(slug string) (fabricengine.AddResult, error) {
			if err := w.addErr[slug]; err != nil {
				return fabricengine.AddResult{}, err
			}
			w.pairs[slug] = true
			return fabricengine.AddResult{Slug: slug}, nil
		},
		DropLocal: func(_ *fabricengine.Mutations, slug string) (fabricengine.DropLocalResult, error) {
			w.dropped = append(w.dropped, slug)
			return fabricengine.DropLocalResult{Tags: w.dropTags}, nil
		},
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			w.ownerReq = req
			if w.setErr != nil {
				return boardengine.OwnerWrite{}, w.setErr
			}
			write := boardengine.OwnerWrite{Lost: w.lost}
			for _, slug := range req.Slugs {
				if _, lost := w.lost[slug]; lost {
					continue
				}
				entry := w.owners.Entries[slug]
				entry.Owner = req.Machine
				w.owners.Entries[slug] = entry
				write.Recorded = append(write.Recorded, slug)
			}
			if req.Prime {
				w.owners.Prime = req.Machine
				write.Recorded = append(write.Recorded, boardengine.OwnerWritePrime)
			}
			return write, nil
		},
		PauseRecord: func() (pauserecord.Record, error) { return w.record, nil },
		Lift: func(ref tenderengine.EntryRef) error {
			w.lifted = append(w.lifted, ref)
			w.record.Live = slices.DeleteFunc(slices.Clone(w.record.Live), func(entry pauserecord.Entry) bool {
				return entry.Scope == ref.Scope && entry.Caller == ref.Caller
			})
			return nil
		},
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			status, found := w.runs[slug]
			return status, found, nil
		},
		ResumeRun: func(slug string) (map[string]any, error) {
			w.resumed = append(w.resumed, slug)
			if err := w.resumeErr[slug]; err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			status, found := w.children[slug]
			return status, found, nil
		},
		ResumeChild: func(slug string) error {
			w.childResumed = append(w.childResumed, slug)
			return nil
		},
	}
}

// slugOf returns the outcome of slug, failing the test when the envelope lacks it.
func slugOf(t *testing.T, got outcome, slug string) slugOutcome {
	t.Helper()
	for _, entry := range got.Slugs {
		if entry.Slug == slug {
			return entry
		}
	}
	t.Fatalf("outcome has no entry for %q: %+v", slug, got.Slugs)
	return slugOutcome{}
}

func TestAdopt(t *testing.T) {
	t.Parallel()

	ownerHalted := shedengine.Status{State: shedengine.StateBlocked, Error: landingshed.OwnerHaltPrefix + "the parent is owned by there"}
	withKid := func(w *world, parent string, status shedengine.Status) {
		w.owners.Entries["kid"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
		w.parents["kid"] = parent
		w.children["kid"] = status
	}

	tests := []struct {
		name    string
		req     request
		setup   func(*world)
		wantErr string
		check   func(*testing.T, *world, outcome)
	}{
		{name: "RefusesUnknownSlug", req: request{Slugs: []string{"nope"}}, wantErr: "lyx board list", check: func(t *testing.T, w *world, _ outcome) {
			if w.reconciled {
				t.Errorf("the reconcile ran after a refusal before anything runs")
			}
		}},
		{name: "RefusesNote", req: request{Slugs: []string{"note1"}}, wantErr: "lyx board promote"},
		{name: "RefusesDoneTaskNamingResume", req: request{Slugs: []string{"done1"}}, wantErr: "lyx resume done1"},
		{name: "RefusesKeepWithDrop", req: request{Slugs: []string{"alpha"}, KeepLocal: true, DropLocal: true}, wantErr: "pass one of them"},
		{name: "RefusesNoSlugWithoutPrimeOrAll", req: request{}, wantErr: "--prime or --all"},
		{name: "UnreachableOriginRefusesNamingTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.report.Unreachable = true }, wantErr: "once origin answers", check: func(t *testing.T, w *world, _ outcome) {
			if w.ownerReq.Machine != "" {
				t.Errorf("an owner was written after an unreachable origin")
			}
		}},
		{name: "ReconcileErrorRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.reconcileErr = errors.New("ls-remote broke") }, wantErr: "ls-remote broke; repair what the error names, then re-run lyx adopt"},
		{name: "BoltStopRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabricengine.BranchDiverged, WayForward: "reset bolt"}}
		}, wantErr: "reset bolt"},
		{name: "PrimeRecordsStopRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "records-main", Repo: "records", Prime: true, State: fabricengine.BranchDirty, WayForward: "commit the records"}}
		}, wantErr: "commit the records; then re-run lyx adopt"},
		{name: "DivergedBranchStopsOneSlugWhileAnotherIsAdopted", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.stopBranch("alpha", fabricengine.BranchDiverged) }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.Stopped || alpha.OwnerRecorded || len(alpha.Branches) != 1 || alpha.Branches[0].State != "diverged" {
				t.Errorf("alpha = %+v, want stopped on its diverged branch with no owner", alpha)
			}
			for _, way := range []string{"lyx adopt alpha --keep-local", "lyx adopt alpha --drop-local"} {
				if !strings.Contains(alpha.WayForward, way) {
					t.Errorf("alpha way forward %q lacks %q", alpha.WayForward, way)
				}
			}
			if !slices.Equal(w.ownerReq.Slugs, []string{"beta"}) || !slugOf(t, got, "beta").OwnerRecorded {
				t.Errorf("owner write = %v, beta = %+v, want beta alone", w.ownerReq.Slugs, slugOf(t, got, "beta"))
			}
		}},
		{name: "NeedsReplayRecordsBranchStopsTheSlug", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.stopBranch("alpha", fabricengine.BranchNeedsReplay) }, check: func(t *testing.T, _ *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); !alpha.Stopped || alpha.Branches[0].State != "needs_replay" {
				t.Errorf("alpha = %+v, want stopped on needs_replay", alpha)
			}
		}},
		{name: "LivePairStopsTheSlug", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.live["alpha"] = true }, check: func(t *testing.T, _ *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); !alpha.Stopped || !alpha.Live {
				t.Errorf("alpha = %+v, want stopped on a live pair", alpha)
			}
		}},
		{name: "KeepLocalRecordsTheOwnerOverLocalCommitsAndALivePair", req: request{Slugs: []string{"alpha"}, KeepLocal: true}, setup: func(w *world) {
			w.stopBranch("alpha", fabricengine.BranchDiverged)
			w.live["alpha"] = true
		}, check: func(t *testing.T, w *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); !alpha.OwnerRecorded || alpha.Stopped {
				t.Errorf("alpha = %+v, want the owner recorded", alpha)
			}
			if len(w.dropped) != 0 {
				t.Errorf("--keep-local dropped %v", w.dropped)
			}
		}},
		{name: "DropLocalDropsThenRecordsTheOwner", req: request{Slugs: []string{"alpha"}, DropLocal: true}, setup: func(w *world) {
			w.stopBranch("alpha", fabricengine.BranchAhead)
			w.dropTags = []string{"archive/alpha/abc123"}
		}, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !slices.Equal(w.dropped, []string{"alpha"}) || !alpha.OwnerRecorded || !slices.Equal(alpha.Dropped, w.dropTags) {
				t.Errorf("dropped = %v, alpha = %+v, want the tags kept and the owner recorded", w.dropped, alpha)
			}
		}},
		{name: "DropLocalRefusesALivePairNamingKeepLocal", req: request{Slugs: []string{"alpha"}, DropLocal: true}, setup: func(w *world) { w.live["alpha"] = true }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Error, "--keep-local") || alpha.OwnerRecorded || len(w.dropped) != 0 {
				t.Errorf("alpha = %+v, dropped = %v, want a refusal naming --keep-local and no drop", alpha, w.dropped)
			}
		}},
		{name: "RefusedAddStopsItsSlugWhileAnotherIsAdopted", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) {
			w.pairs["beta"] = false
			w.addErr["beta"] = errors.New("worktree directory already exists")
		}, check: func(t *testing.T, _ *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !strings.Contains(beta.Error, "worktree directory already exists") || beta.OwnerRecorded {
				t.Errorf("beta = %+v, want fabric's refusal and no owner", beta)
			}
			if !slugOf(t, got, "alpha").OwnerRecorded {
				t.Errorf("alpha was not adopted beside the refused beta")
			}
		}},
		{name: "LeftoverBranchRefusalNamesTheArchiveTagBeforeBranchDelete", req: request{Slugs: []string{"beta"}}, setup: func(w *world) {
			w.pairs["beta"] = false
			w.addErr["beta"] = &fabricengine.ErrBranchExists{Branch: "task/beta"}
		}, check: func(t *testing.T, _ *world, got outcome) {
			message := slugOf(t, got, "beta").Error
			tag, remove := strings.Index(message, "git tag archive/beta/"), strings.Index(message, "git branch -D task/beta")
			if tag < 0 || remove < tag || !strings.Contains(message, "lyx adopt beta") || strings.Contains(message, "lyx fabric checkout") {
				t.Errorf("leftover refusal = %q, want the archive tag before git branch -D, the re-run and no fabric checkout", message)
			}
		}},
		{name: "AddsTheMissingPair", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { w.pairs["beta"] = false }, check: func(t *testing.T, w *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !beta.PairAdded || !beta.OwnerRecorded || !w.pairs["beta"] {
				t.Errorf("beta = %+v, want the pair added and the owner recorded", beta)
			}
		}},
		{name: "RefusedPushResumesNothingAndNamesTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.setErr = errors.New("board: the push was refused") }, wantErr: "then re-run lyx adopt with the same arguments", check: func(t *testing.T, w *world, _ outcome) {
			if len(w.resumed) != 0 {
				t.Errorf("resumed %v after a refused owner write", w.resumed)
			}
		}},
		{name: "LostOwnerWriteStartsNoRun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.lost = map[string]string{"alpha": "third"} }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Error, "third") || !strings.Contains(alpha.Error, "lyx adopt alpha") || alpha.OwnerRecorded {
				t.Errorf("alpha = %+v, want the new owner and the re-run named", alpha)
			}
			if !slices.Equal(w.resumed, []string{"beta"}) {
				t.Errorf("resumed %v, want beta alone", w.resumed)
			}
		}},
		{name: "HubWithNoPrimeOwnerRecordsThisMachineAsPrimeOwner", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.owners.Prime = "" }, check: func(t *testing.T, w *world, got outcome) {
			if !w.ownerReq.Prime || !got.PrimeRecorded {
				t.Errorf("prime requested = %v, recorded = %v, want both", w.ownerReq.Prime, got.PrimeRecorded)
			}
		}},
		{name: "OwnerPauseIsLiftedWhileAnUnlabelledPauseStands", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) {
			at := time.Unix(100, 0)
			w.record.Live = []pauserecord.Entry{
				{Scope: "alpha", Caller: pauserecord.OwnerCaller, PausedAt: at},
				{Scope: "beta", Caller: pauserecord.OwnerCaller, PausedAt: at},
				{Scope: "beta", Caller: "strand", PausedAt: at},
			}
		}, check: func(t *testing.T, w *world, got outcome) {
			if len(w.lifted) != 2 || w.lifted[0] != (tenderengine.EntryRef{Scope: "alpha", Caller: pauserecord.OwnerCaller, PausedAt: time.Unix(100, 0)}) {
				t.Errorf("lifted = %+v, want both owner entries by scope, caller and time", w.lifted)
			}
			alpha, beta := slugOf(t, got, "alpha"), slugOf(t, got, "beta")
			if !alpha.PauseLifted || alpha.PauseStanding != "" {
				t.Errorf("alpha = %+v, want its pause lifted and none standing", alpha)
			}
			if !beta.PauseLifted || !strings.Contains(beta.PauseStanding, "lyx resume beta") {
				t.Errorf("beta = %+v, want the owner pause lifted and the unlabelled one named with lyx resume beta", beta)
			}
		}},
		{name: "HubPauseStandingIsNamedWithBareResume", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.record.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, Caller: "operator"}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if standing := slugOf(t, got, "alpha").PauseStanding; !strings.Contains(standing, "`lyx resume`") {
				t.Errorf("pause standing = %q, want the hub pause named with lyx resume", standing)
			}
		}},
		{name: "ResumesEachRunOnceAndReportsAFailureBesideTheNext", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.resumeErr["alpha"] = errors.New("window failed") }, check: func(t *testing.T, w *world, got outcome) {
			if !slices.Equal(w.resumed, []string{"alpha", "beta"}) {
				t.Errorf("resumed %v, want each slug once in order", w.resumed)
			}
			if alpha := slugOf(t, got, "alpha"); !strings.Contains(alpha.Error, "window failed") || alpha.Run == "resumed" {
				t.Errorf("alpha = %+v, want the failure on its slug", alpha)
			}
			if beta := slugOf(t, got, "beta"); beta.Run != "resumed" || beta.Error != "" {
				t.Errorf("beta = %+v, want its run resumed", beta)
			}
		}},
		{name: "TaskWithNoRunRecordGetsItsOwnerAndNoRun", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { delete(w.runs, "beta") }, check: func(t *testing.T, w *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !beta.OwnerRecorded || beta.Run != "none" || len(w.resumed) != 0 {
				t.Errorf("beta = %+v, resumed = %v, want the owner and no run", beta, w.resumed)
			}
		}},
		{name: "OwnerHaltedChildResumesWhenItsOwnSlugIsAdopted", req: request{Slugs: []string{"kid"}}, setup: func(w *world) {
			withKid(w, "", ownerHalted)
			w.owners.Entries["kid"] = boardengine.OwnerEntry{Owner: there, IsTask: true, RunHeld: true}
			w.pairs["kid"] = true
		}, check: func(t *testing.T, w *world, _ outcome) {
			if !slices.Equal(w.childResumed, []string{"kid"}) {
				t.Errorf("children resumed %v, want kid", w.childResumed)
			}
		}},
		{name: "OwnerHaltedChildResumesWhenThePrimeIsAdopted", req: request{Prime: true}, setup: func(w *world) { withKid(w, "", ownerHalted) }, check: func(t *testing.T, w *world, got outcome) {
			if !slices.Equal(w.childResumed, []string{"kid"}) || !slugOf(t, got, "kid").ChildResumed {
				t.Errorf("children resumed %v, want kid", w.childResumed)
			}
			if !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Errorf("owner request = %+v, want the prime's owner alone", w.ownerReq)
			}
		}},
		{name: "OwnerHaltedChildResumesWhenItsPairParentIsAdopted", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { withKid(w, "alpha", ownerHalted) }, check: func(t *testing.T, w *world, _ outcome) {
			if !slices.Equal(w.childResumed, []string{"kid"}) {
				t.Errorf("children resumed %v, want kid", w.childResumed)
			}
		}},
		{name: "ChildHaltedOtherwiseIsLeftAlone", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			withKid(w, "alpha", shedengine.Status{State: shedengine.StateBlocked, Error: "the plan failed"})
		}, check: func(t *testing.T, w *world, _ outcome) {
			if len(w.childResumed) != 0 {
				t.Errorf("children resumed %v, want none", w.childResumed)
			}
		}},
		{name: "ParentOwnedElsewhereCarriesTheFinalizeWarning", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.owners.Entries["gamma"] = boardengine.OwnerEntry{Owner: "third", IsTask: true}
			w.parents["alpha"] = "gamma"
		}, check: func(t *testing.T, _ *world, got outcome) {
			warning := slugOf(t, got, "alpha").Warning
			if !strings.Contains(warning, "third") || !strings.Contains(warning, "Finalize") || !strings.Contains(warning, "lyx adopt gamma") {
				t.Errorf("warning = %q, want the parent's owner, Finalize and the adopt of the parent", warning)
			}
		}},
		{name: "AllChoosesThePrimeAndOwnedOrRunningTasks", req: request{All: true}, check: func(t *testing.T, w *world, _ outcome) {
			if !slices.Equal(w.ownerReq.Slugs, []string{"alpha", "beta"}) || !w.ownerReq.Prime || !w.ownerReq.Takeover {
				t.Errorf("owner request = %+v, want the prime and alpha, beta with takeover, passing over the idle and the done task", w.ownerReq)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld()
			if tt.setup != nil {
				tt.setup(w)
			}
			got, err := adopt(w.deps(), tt.req)
			switch {
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("adopt error = %v, want it to contain %q", err, tt.wantErr)
			case tt.wantErr == "" && err != nil:
				t.Fatalf("adopt error = %v, want none", err)
			}
			if tt.check != nil {
				tt.check(t, w, got)
			}
		})
	}
}
