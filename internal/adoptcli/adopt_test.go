//go:build tier0

// adopt_test.go drives the takeover sequence over fake deps: the refusals before anything runs, the reconcile stops, the per-entry stops and their two ways forward, the pair add, the owner write, the pause lift, the run resume, the halted-child resume, the beam steps and the adopt check start.

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
	"github.com/Knatte18/loomyard/internal/orchcli"
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
	// moving is the target anchor of the move record standing in each worktree, "" keying the prime.
	moving map[string]string
	// order is the run resumes, child resumes, beam steps and suite starts in the order they came, and suiteErr what the suite start answers.
	order    []string
	suiteErr error
	// beamUps are the worktrees the beam steps were run for, and beamUpErr what the steps of each worktree answer.
	beamUps   []string
	beamUpErr map[string]error
	// held names the beams whose pawl run holds the landing hold, and heldErr the beams whose hold reads fail.
	held    map[string]bool
	heldErr map[string]error
	// race, when set, runs on the owners right after the owner write lands, as another machine's write does.
	race func(*boardengine.Owners)
}

// newWorld returns a hub this machine owns the prime of, with two beam projects owned by another machine, each with a pair and a running pawl run, an idle task, a done task and a note.
func newWorld() *world {
	running := shedengine.Status{State: shedengine.StateRunning}
	return &world{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"alpha": {Owner: there, IsProject: true, Beam: true, RunHeld: true},
			"beta":  {Owner: there, IsProject: true, Beam: true, RunHeld: true},
			"idle":  {IsTask: true},
			"done1": {IsTask: true, Done: true},
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
		moving:    map[string]string{},
		beamUpErr: map[string]error{},
		held:      map[string]bool{},
		heldErr:   map[string]error{},
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
			if w.race != nil {
				w.race(&w.owners)
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
			w.order = append(w.order, "resume:"+slug)
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
			w.order = append(w.order, "child:"+slug)
			return nil
		},
		MoveInProgress: func(worktree string) (string, bool, error) {
			target, standing := w.moving[worktree]
			if !standing {
				return "", false, nil
			}
			name := worktree
			if name == "" {
				name = "main"
			}
			return "a change of the anchor of " + name + " was interrupted; run `lym fabric anchor " + name + " --set " + target + "` to finish it", true, nil
		},
		BeamUp: func(worktree string) (orchcli.BeamUpReport, error) {
			w.order = append(w.order, "beamup:"+worktree)
			w.beamUps = append(w.beamUps, worktree)
			return orchcli.BeamUpReport{Worktree: worktree, Dobby: "started", Action: "relaunched"}, w.beamUpErr[worktree]
		},
		LandingHeld: func(slug string) (bool, error) { return w.held[slug], w.heldErr[slug] },
		PrimeName:   "prime",
		StartSuite: func() (suiteStart, error) {
			w.order = append(w.order, "suite")
			if w.suiteErr != nil {
				return suiteStart{}, w.suiteErr
			}
			return testSuite, nil
		},
	}
}

// testSuite is what the fake suite start answers.
var testSuite = suiteStart{PID: 42, Log: "suite.log"}

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
	running := shedengine.Status{State: shedengine.StateRunning}
	withKid := func(w *world, parent string, status shedengine.Status) {
		w.owners.Entries["kid"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
		w.parents["kid"] = parent
		w.children["kid"] = status
	}

	// withBeam adds a beam project owned by another machine and a task of it with a running batten run, both read as owned elsewhere by the reconcile.
	withBeam := func(w *world) {
		w.owners.Entries["payments"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
		w.owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
		w.parents["t1"] = "payments"
		w.runs["t1"] = running
		w.report.Entries = append(w.report.Entries,
			fabricengine.EntryOutcome{Slug: "payments", State: fabricengine.EntryOwnedElsewhere, Owner: there},
			fabricengine.EntryOutcome{Slug: "t1", State: fabricengine.EntryOwnedElsewhere, Owner: there})
	}

	// withoutAlphaAndBeta leaves a board without the two default beams.
	withoutAlphaAndBeta := func(w *world) {
		delete(w.owners.Entries, "alpha")
		delete(w.owners.Entries, "beta")
		delete(w.runs, "alpha")
		delete(w.runs, "beta")
	}

	// requireNothingRan fails the test when a step ran, which a refusal ahead of the origin reconcile leaves undone.
	requireNothingRan := func(t *testing.T, w *world) {
		t.Helper()
		if w.reconciled || w.ownerReq.Machine != "" || len(w.resumed) != 0 || len(w.dropped) != 0 || len(w.order) != 0 {
			t.Errorf("a refused call ran: reconciled %v, owner request %+v, resumed %v, dropped %v, steps %v; want nothing", w.reconciled, w.ownerReq, w.resumed, w.dropped, w.order)
		}
	}

	tests := []struct {
		name    string
		req     request
		setup   func(*world)
		wantErr string
		check   func(*testing.T, *world, outcome)
	}{
		{name: "RefusesUnknownSlug", req: request{Slugs: []string{"nope"}}, wantErr: "lym board list", check: func(t *testing.T, w *world, _ outcome) {
			if w.reconciled {
				t.Errorf("the reconcile ran after a refusal before anything runs")
			}
		}},
		{name: "StandingMoveRecordOnThePrimeRefusesNamingTheSetAndTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving[""] = "sub" }, wantErr: "lym fabric anchor main --set sub", check: func(t *testing.T, w *world, _ outcome) {
			requireNothingRan(t, w)
		}},
		{name: "StandingMoveRecordOnANamedEntrysWorktreeRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving["alpha"] = "sub" }, wantErr: "lym fabric anchor alpha --set sub", check: func(t *testing.T, w *world, _ outcome) {
			requireNothingRan(t, w)
		}},
		{name: "StandingMoveRecordOnANamedEntrysParentRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.parents["alpha"] = "gamma"
			w.moving["gamma"] = "sub"
		}, wantErr: "lym fabric anchor gamma --set sub` to finish it; then re-run lym adopt with the same arguments", check: func(t *testing.T, w *world, _ outcome) {
			requireNothingRan(t, w)
		}},
		{name: "StandingMoveRecordOnAnUnrelatedWorktreeDoesNotRefuse", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving["unrelated"] = "sub" }, check: func(t *testing.T, _ *world, got outcome) {
			if !slugOf(t, got, "alpha").OwnerRecorded {
				t.Errorf("alpha = %+v, want it adopted beside a record in an unrelated worktree", slugOf(t, got, "alpha"))
			}
		}},
		{name: "RefusesNote", req: request{Slugs: []string{"note1"}}, wantErr: "lym board promote"},
		{name: "RefusesDoneTaskNamingResume", req: request{Slugs: []string{"done1"}}, wantErr: "lym tender resume done1"},
		{name: "RefusesKeepWithDrop", req: request{Slugs: []string{"alpha"}, KeepLocal: true, DropLocal: true}, wantErr: "pass one of them"},
		{name: "RefusesNoSlugWithoutRootOrAll", req: request{}, wantErr: "or pass --all"},
		{name: "RefusesATaskNamingItsBeam", req: request{Slugs: []string{"t1"}}, setup: withBeam, wantErr: "`lym adopt payments`", check: func(t *testing.T, w *world, _ outcome) {
			requireNothingRan(t, w)
		}},
		{name: "RefusesARootTaskNamingRoot", req: request{Slugs: []string{"idle"}}, wantErr: "`lym adopt root`", check: func(t *testing.T, w *world, _ outcome) {
			requireNothingRan(t, w)
		}},
		{name: "RefusesAGroupingProjectBelowRootNamingRootAndBeamCreate", req: request{Slugs: []string{"grp"}}, setup: func(w *world) {
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
		}, wantErr: "`lym adopt root` moves its tasks, and `lym beam create grp` makes it a beam"},
		{name: "RefusesAGroupingProjectBelowABeamNamingTheBeamAndBeamCreate", req: request{Slugs: []string{"grp"}}, setup: func(w *world) {
			withBeam(w)
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
			w.parents["grp"] = "payments"
		}, wantErr: "`lym adopt payments` moves its tasks, and `lym beam create grp` makes it a beam"},
		{name: "TheRootWordNamesRoot", req: request{Slugs: []string{"root"}}, check: func(t *testing.T, w *world, got outcome) {
			if !w.ownerReq.Prime || !got.PrimeRecorded || len(w.ownerReq.Slugs) != 0 {
				t.Errorf("owner request = %+v, prime recorded = %v; want the prime's owner alone", w.ownerReq, got.PrimeRecorded)
			}
		}},
		{name: "UnreachableOriginRefusesNamingTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.report.Unreachable = true }, wantErr: "once origin answers", check: func(t *testing.T, w *world, _ outcome) {
			if w.ownerReq.Machine != "" {
				t.Errorf("an owner was written after an unreachable origin")
			}
		}},
		{name: "ReconcileErrorRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.reconcileErr = errors.New("ls-remote broke") }, wantErr: "ls-remote broke; repair what the error names, then re-run lym adopt"},
		{name: "BoltStopRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabricengine.BranchDiverged, WayForward: "reset bolt"}}
		}, wantErr: "reset bolt"},
		{name: "PrimeRecordsAnchorStopRefusesWithItsWayForward", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "records-main", Repo: "records", Prime: true, State: fabricengine.BranchAnchorStopped, Anchor: "sub", WayForward: "run the set"}}
		}, wantErr: "records-main (anchor_stopped): run the set", check: func(t *testing.T, w *world, _ outcome) {
			if w.ownerReq.Machine != "" {
				t.Errorf("an owner was written after a stop on the prime's records branch")
			}
		}},
		{name: "PrimeRecordsStopRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "records-main", Repo: "records", Prime: true, State: fabricengine.BranchDirty, WayForward: "commit the records"}}
		}, wantErr: "commit the records; then re-run lym adopt"},
		{name: "DivergedBranchStopsOneSlugWhileAnotherIsAdopted", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.stopBranch("alpha", fabricengine.BranchDiverged) }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.Stopped || alpha.OwnerRecorded || len(alpha.Branches) != 1 || alpha.Branches[0].State != "diverged" {
				t.Errorf("alpha = %+v, want stopped on its diverged branch with no owner", alpha)
			}
			for _, way := range []string{"lym adopt alpha --keep-local", "lym adopt alpha --drop-local"} {
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
		{name: "AnchorStoppedRecordsBranchStopsTheSlugBeforeItsOwnerWrite", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = append(w.report.Branches, fabricengine.BranchOutcome{Branch: "records-alpha", Repo: "records", Slug: "alpha", State: fabricengine.BranchAnchorStopped, Anchor: "sub", WayForward: "run the set"})
		}, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.Stopped || alpha.OwnerRecorded || len(alpha.Branches) != 1 || alpha.Branches[0] != (branchStop{Branch: "records-alpha", State: "anchor_stopped"}) {
				t.Errorf("alpha = %+v, want stopped on records-alpha in state anchor_stopped with no owner", alpha)
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
			if tag < 0 || remove < tag || !strings.Contains(message, "lym adopt beta") || strings.Contains(message, "lym fabric checkout") {
				t.Errorf("leftover refusal = %q, want the archive tag before git branch -D, the re-run and no fabric checkout", message)
			}
		}},
		{name: "AddsTheMissingPair", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { w.pairs["beta"] = false }, check: func(t *testing.T, w *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !beta.PairAdded || !beta.OwnerRecorded || !w.pairs["beta"] {
				t.Errorf("beta = %+v, want the pair added and the owner recorded", beta)
			}
		}},
		{name: "RefusedPushResumesNothingAndNamesTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.setErr = errors.New("board: the push was refused") }, wantErr: "then re-run lym adopt with the same arguments", check: func(t *testing.T, w *world, _ outcome) {
			if len(w.resumed) != 0 {
				t.Errorf("resumed %v after a refused owner write", w.resumed)
			}
		}},
		{name: "LostOwnerWriteStartsNoRun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.lost = map[string]string{"alpha": "third"} }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Error, "third") || !strings.Contains(alpha.Error, "lym adopt alpha") || alpha.OwnerRecorded {
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
			if !beta.PauseLifted || !strings.Contains(beta.PauseStanding, "lym tender resume beta") {
				t.Errorf("beta = %+v, want the owner pause lifted and the unlabelled one named with lym tender resume beta", beta)
			}
		}},
		{name: "HubPauseStandingIsNamedWithBareResume", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.record.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, Caller: "operator"}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if standing := slugOf(t, got, "alpha").PauseStanding; !strings.Contains(standing, "`lym tender resume`") {
				t.Errorf("pause standing = %q, want the hub pause named with lym tender resume", standing)
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
		{name: "BeamWithNoRunRecordGetsItsOwnerAndNoRun", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { delete(w.runs, "beta") }, check: func(t *testing.T, w *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !beta.OwnerRecorded || beta.Run != "none" || len(w.resumed) != 0 {
				t.Errorf("beta = %+v, resumed = %v, want the owner and no run", beta, w.resumed)
			}
		}},
		{name: "OwnerHaltedChildResumesWhenRootIsAdopted", req: request{Root: true}, setup: func(w *world) {
			withKid(w, "", ownerHalted)
			withoutAlphaAndBeta(w)
		}, check: func(t *testing.T, w *world, got outcome) {
			if !slices.Equal(w.childResumed, []string{"kid"}) || !slugOf(t, got, "kid").ChildResumed {
				t.Errorf("children resumed %v, want kid", w.childResumed)
			}
			if !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Errorf("owner request = %+v, want the prime's owner alone", w.ownerReq)
			}
		}},
		{name: "OwnerHaltedChildResumesWhenItsBeamIsAdopted", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { withKid(w, "alpha", ownerHalted) }, check: func(t *testing.T, w *world, _ outcome) {
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
		{name: "ParentTakenByARaceCarriesTheFinalizeWarning", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.owners.Entries["gamma"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			w.parents["alpha"] = "gamma"
			w.race = func(owners *boardengine.Owners) {
				owners.Entries["gamma"] = boardengine.OwnerEntry{Owner: "third", IsProject: true, Beam: true}
			}
		}, check: func(t *testing.T, _ *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Warning, "third") || !strings.Contains(alpha.Warning, "Finalize") || !strings.Contains(alpha.Warning, "lym adopt gamma") {
				t.Errorf("warning = %q, want the parent's owner, Finalize and the adopt of the parent", alpha.Warning)
			}
			if alpha.Run != ownedElsewhereRun || alpha.OwnedBy != "third" || alpha.Error != "" {
				t.Errorf("alpha = %+v, want its pawl run left, reported owned elsewhere by the parent's owner, with no error", alpha)
			}
		}},
		{name: "AllNamesRootAndEveryOpenBeamProjectAndNoTaskOrGroupingProject", req: request{All: true}, setup: func(w *world) {
			w.owners.Entries["base"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			w.owners.Entries["ownerless"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			w.owners.Entries["landed"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there, Done: true}
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
		}, check: func(t *testing.T, w *world, _ outcome) {
			if !slices.Equal(w.ownerReq.Slugs, []string{"alpha", "base", "beta", "ownerless"}) || !w.ownerReq.Prime || !w.ownerReq.Takeover {
				t.Errorf("owner request = %+v, want root and the open beams alpha, base, beta, ownerless with takeover, passing over the idle and done tasks, the done beam and the grouping project", w.ownerReq)
			}
		}},
		{name: "NamedBeamIsAdoptedWithItsPairAdded", req: request{Slugs: []string{"base"}}, setup: func(w *world) {
			w.owners.Entries["base"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
		}, check: func(t *testing.T, w *world, got outcome) {
			if base := slugOf(t, got, "base"); !base.PairAdded || !base.OwnerRecorded || !w.pairs["base"] || base.Run != "none" {
				t.Errorf("base = %+v, want the pair added, the owner recorded and no run", base)
			}
			if !slices.Equal(w.ownerReq.Slugs, []string{"base"}) {
				t.Errorf("owner request slugs = %v, want the beam alone, its members keeping their own owners", w.ownerReq.Slugs)
			}
		}},
		{name: "BeamMovesItsMembersRunAndRecordsItsOwnOwnerAlone", req: request{Slugs: []string{"payments"}}, setup: withBeam, check: func(t *testing.T, w *world, got outcome) {
			t1 := slugOf(t, got, "t1")
			if t1.Run != "resumed" || t1.OwnerRecorded || !slices.Equal(w.resumed, []string{"t1"}) {
				t.Errorf("t1 = %+v, resumed = %v, want its run resumed with no owner of its own", t1, w.resumed)
			}
			if !slices.Equal(w.ownerReq.Slugs, []string{"payments"}) || w.ownerReq.Prime {
				t.Errorf("owner request = %+v, want the beam alone", w.ownerReq)
			}
		}},
		{name: "RootMovesARootTasksRunAndLeavesANestedBeamsMemberAlone", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["kid1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
			w.runs["kid1"] = running
			w.pairs["kid1"] = true
			w.report.Entries = append(w.report.Entries, fabricengine.EntryOutcome{Slug: "kid1", State: fabricengine.EntryOwnedElsewhere, Owner: there})
			withBeam(w)
			w.owners.Entries["payments"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
		}, check: func(t *testing.T, w *world, _ outcome) {
			if !slices.Equal(w.resumed, []string{"kid1"}) || !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Errorf("resumed = %v, owner request = %+v, want the root task's run resumed and the prime's owner alone, the beam's member t1 left", w.resumed, w.ownerReq)
			}
		}},
		{name: "BeamPawlRunIsResumedWhenRootIsAdoptedWithTheBeamAlreadyOwnedHere", req: request{Root: true}, setup: func(w *world) {
			w.owners.Prime = there
			w.owners.Entries["alpha"] = boardengine.OwnerEntry{Owner: here, IsProject: true, Beam: true, RunHeld: true}
			delete(w.owners.Entries, "beta")
			delete(w.runs, "beta")
		}, check: func(t *testing.T, w *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); alpha.Run != "resumed" || !slices.Equal(w.resumed, []string{"alpha"}) {
				t.Errorf("alpha = %+v, resumed = %v, want its pawl run resumed once root is this machine's", alpha, w.resumed)
			}
		}},
		{name: "BeamPawlRunIsLeftWhileAnotherMachineOwnsItsParent", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.owners.Prime = there }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.OwnerRecorded || alpha.Run != ownedElsewhereRun || alpha.OwnedBy != there || alpha.Error != "" || len(w.resumed) != 0 {
				t.Errorf("alpha = %+v, resumed = %v, want the beam adopted and its pawl run reported owned elsewhere by the prime's owner, with no error", alpha, w.resumed)
			}
		}},
		{name: "NestedBeamOwnedElsewhereUnderAdoptedRootIsReportedOwnedElsewhere", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["nested"] = boardengine.OwnerEntry{Owner: "third", IsProject: true, Beam: true, RunHeld: true}
			w.runs["nested"] = running
		}, check: func(t *testing.T, w *world, got outcome) {
			nested := slugOf(t, got, "nested")
			if nested.Run != ownedElsewhereRun || nested.OwnedBy != "third" || nested.Error != "" || nested.PairAdded || len(w.resumed) != 0 {
				t.Errorf("nested = %+v, resumed = %v, want its run reported owned elsewhere by third, with no error, pair or resume", nested, w.resumed)
			}
		}},
		{name: "MovedTaskWithLocalCommitsStopsNamingItsBeamAndHoldsTheBeamBack", req: request{Slugs: []string{"payments"}}, setup: func(w *world) {
			withBeam(w)
			w.pairs["t1"] = true
			w.stopBranch("t1", fabricengine.BranchDiverged)
		}, check: func(t *testing.T, w *world, got outcome) {
			t1 := slugOf(t, got, "t1")
			if !t1.Stopped {
				t.Fatalf("t1 = %+v, want it stopped on its diverged branch", t1)
			}
			for _, way := range []string{"`lym adopt payments --keep-local`", "`lym adopt payments --drop-local`"} {
				if !strings.Contains(t1.WayForward, way) {
					t.Errorf("t1 way forward %q lacks %q", t1.WayForward, way)
				}
			}
			if strings.Contains(t1.WayForward, "lym adopt t1") || w.ownerReq.Machine != "" || len(w.resumed) != 0 {
				t.Errorf("way forward = %q, owner request = %+v, resumed = %v; want no adopt of the task and nothing written or resumed", t1.WayForward, w.ownerReq, w.resumed)
			}
		}},
		{name: "BeamStepsRunAfterTheResumesForThePrimeAndEachAdoptedBeamThenOneSuiteStart", req: request{Slugs: []string{"alpha", "beta"}, Root: true}, check: func(t *testing.T, w *world, got outcome) {
			if want := []string{"resume:alpha", "resume:beta", "beamup:prime", "beamup:alpha", "beamup:beta", "suite"}; !slices.Equal(w.order, want) || got.PrimeBeamWarning != "" {
				t.Errorf("steps = %v, prime warning = %q; want %v and no warning", w.order, got.PrimeBeamWarning, want)
			}
			if got.PrimeBeamSteps == nil || got.PrimeBeamSteps.Dobby != "started" {
				t.Errorf("prime beam steps = %+v; want the prime's report", got.PrimeBeamSteps)
			}
			for _, slug := range []string{"alpha", "beta"} {
				if steps := slugOf(t, got, slug).BeamSteps; steps == nil || steps.Dobby != "started" || steps.Action != "relaunched" {
					t.Errorf("%s beam steps = %+v; want the beam's report", slug, steps)
				}
			}
			if got.Suite == nil || *got.Suite != testSuite || got.SuiteWarning != "" {
				t.Errorf("suite = %+v, warning = %q; want %+v and no warning", got.Suite, got.SuiteWarning, testSuite)
			}
		}},
		{name: "PrimeOwnerRecordedAloneRunsThePrimesBeamStepsAndStartsTheSuiteLast", req: request{Root: true}, setup: withoutAlphaAndBeta, check: func(t *testing.T, w *world, got outcome) {
			if !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Fatalf("owner request = %+v, want the prime's owner alone", w.ownerReq)
			}
			if !slices.Equal(w.beamUps, []string{"prime"}) || got.PrimeBeamSteps == nil {
				t.Errorf("beam steps run for %v, prime steps %+v; want the prime alone", w.beamUps, got.PrimeBeamSteps)
			}
			if got.Suite == nil || len(w.order) == 0 || w.order[len(w.order)-1] != "suite" {
				t.Errorf("suite = %+v, steps = %v; want one suite start as the last step", got.Suite, w.order)
			}
		}},
		{name: "HeldBeamHasItsOwnerWrittenNoBeamStepsAndNamesTheBattenStatus", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.held["alpha"] = true }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.OwnerRecorded || alpha.BeamSteps != nil || !strings.Contains(alpha.WayForward, "lym batten status alpha") || strings.Contains(alpha.WayForward, "lym adopt") {
				t.Errorf("alpha = %+v; want its owner written, no beam steps and `lym batten status alpha` alone", alpha)
			}
			if !slices.Equal(w.beamUps, []string{"beta"}) {
				t.Errorf("beam steps run for %v; want beta alone", w.beamUps)
			}
		}},
		{name: "HeldBeamWhoseParentAnotherMachineOwnsAlsoNamesTheParentsAdopt", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.held["alpha"] = true
			w.owners.Prime = there
		}, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			for _, want := range []string{"lym batten status alpha", "`lym adopt root`"} {
				if !strings.Contains(alpha.WayForward, want) {
					t.Errorf("alpha way forward = %q; want it to name %q", alpha.WayForward, want)
				}
			}
			if len(w.beamUps) != 0 {
				t.Errorf("beam steps run for %v; want none for a held beam", w.beamUps)
			}
		}},
		{name: "UnreadableLandingHoldWarnsNamingBeamStartAndRunsNoStepsForThatBeam", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.heldErr["alpha"] = errors.New("board unreadable") }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Warning, "board unreadable") || !strings.Contains(alpha.Warning, "lym beam start alpha") || !slices.Equal(w.beamUps, []string{"beta"}) {
				t.Errorf("alpha warning = %q, beam steps run for %v; want the cause and `lym beam start alpha`, and beta's steps alone", alpha.Warning, w.beamUps)
			}
		}},
		{name: "FailingBeamStepWarnsNamingBeamStartWhileTheOtherSlugsStepsRun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.beamUpErr["alpha"] = errors.New("alpha: spawn failed") }, check: func(t *testing.T, w *world, got outcome) {
			alpha, beta := slugOf(t, got, "alpha"), slugOf(t, got, "beta")
			if !strings.Contains(alpha.Warning, "spawn failed") || !strings.Contains(alpha.Warning, "lym beam start alpha") || !alpha.OwnerRecorded {
				t.Errorf("alpha = %+v; want the cause and `lym beam start alpha` beside an adopted alpha", alpha)
			}
			if beta.Warning != "" || beta.BeamSteps == nil || !slices.Equal(w.beamUps, []string{"alpha", "beta"}) {
				t.Errorf("beta = %+v, beam steps run for %v; want beta's steps to run after alpha's failure with no warning", beta, w.beamUps)
			}
		}},
		{name: "FailingPrimeBeamStepWarnsNamingBeamStartRoot", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.beamUpErr["prime"] = errors.New("prime: spawn failed")
		}, check: func(t *testing.T, _ *world, got outcome) {
			if !strings.Contains(got.PrimeBeamWarning, "spawn failed") || !strings.Contains(got.PrimeBeamWarning, "lym beam start root") || !got.PrimeRecorded {
				t.Errorf("prime warning = %q, recorded = %v; want the cause and `lym beam start root` beside the recorded prime", got.PrimeBeamWarning, got.PrimeRecorded)
			}
		}},
		{name: "CallThatRecordsNoOwnerStartsNoSuite", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.stopBranch("alpha", fabricengine.BranchDiverged) }, check: func(t *testing.T, w *world, got outcome) {
			if slices.Contains(w.order, "suite") || got.Suite != nil || got.SuiteWarning != "" {
				t.Errorf("steps = %v, suite = %+v, warning = %q; want no suite start", w.order, got.Suite, got.SuiteWarning)
			}
		}},
		{name: "FailedSuiteStartIsTheWarningNamingTheGate", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.suiteErr = errors.New("exec failed") }, check: func(t *testing.T, w *world, got outcome) {
			want := "the adopt check was not started (exec failed); way forward: run lym fabric reconcile in the prime, then lym gate test --gate adopt there"
			if got.SuiteWarning != want || got.Suite != nil {
				t.Errorf("suite = %+v, warning = %q; want no suite and %q", got.Suite, got.SuiteWarning, want)
			}
			if alpha := slugOf(t, got, "alpha"); !alpha.OwnerRecorded || alpha.Run != "resumed" || alpha.Error != "" {
				t.Errorf("alpha = %+v, want it adopted and resumed beside the failed suite start", alpha)
			}
			if !slices.Equal(w.beamUps, []string{"alpha"}) {
				t.Errorf("beam steps run for %v; want the adopted beam alone, the prime not being adopted", w.beamUps)
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
