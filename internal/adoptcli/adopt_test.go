// adopt_test.go drives the takeover sequence over fake deps, from the refusals to the adopt check start.

package adoptcli

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
	// tasks are the board's entries the hold step reads for released tasks.
	tasks []boardengine.Task
	// holds are the holds files by subject and tries the fix-try counts by subject.
	holds map[string][]string
	tries map[string]int
	// moving is the target anchor of the move record standing in each worktree, "" keying the prime.
	moving map[string]string
	// order records the steps in the order they ran: holds:<subject>, pause, owners, beam steps, and suite:<subject>.
	// suite is the check start's answer for any subject, and suiteErr is the error it fails with, by subject.
	order    []string
	suite    suiteStart
	suiteErr map[string]error
	// beamUps are the worktrees the beam steps were run for, and beamUpErr what the steps of each worktree answer.
	beamUps   []string
	beamUpErr map[string]error
	// held names the beams whose pawl run holds the landing hold, and heldErr the beams whose hold reads fail.
	held    map[string]bool
	heldErr map[string]error
	// checkStates are the states the fake check read answers by subject, none for a subject not listed.
	// escalation marks the slugs the arbiter admits to escalation.
	checkStates map[string]string
	escalation  map[string]bool
	// runErrAfterOwners makes the batten run record read fail once the owner write ran.
	runErrAfterOwners error
	// afterReconcile, when set, runs on the owners right after the reconcile.
	// It stands in for a write from another machine that the reconcile brought in.
	afterReconcile func(*boardengine.Owners)
	// race, when set, runs on the owners right after the owner write lands, as another machine's write does.
	race func(*boardengine.Owners)
	// deploy is what the deploy-state read answers, and deployStateErr is what it fails with.
	// deployErr is what the deploy fails with.
	// deployed holds the commands it ran, each also appended to order as deploy.
	deploy         deployState
	deployStateErr error
	deployErr      error
	deployed       []string
}

// newWorld returns a hub this machine owns the prime of.
// It holds two beam projects owned by another machine, each with a pair and a running pawl run.
// It also holds an idle task, a done task and a note.
func newWorld() *world {
	running := shedengine.Status{State: shedengine.StateRunning}
	return &world{
		suite:    testSuite,
		suiteErr: map[string]error{},
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
		pairs:       map[string]bool{"alpha": true, "beta": true},
		live:        map[string]bool{},
		parents:     map[string]string{},
		addErr:      map[string]error{},
		holds:       map[string][]string{},
		tries:       map[string]int{},
		runs:        map[string]shedengine.Status{"alpha": running, "beta": running},
		moving:      map[string]string{},
		beamUpErr:   map[string]error{},
		held:        map[string]bool{},
		heldErr:     map[string]error{},
		checkStates: map[string]string{},
		escalation:  map[string]bool{},
	}
}

// stopBranch adds a judged branch of slug to the report.
func (w *world) stopBranch(slug string, state fabricengine.BranchState) {
	w.report.Branches = append(w.report.Branches, fabricengine.BranchOutcome{Branch: "code-" + slug, Repo: "code", Slug: slug, State: state})
}

// cloneOwners returns owners with its own entry map,
// so a hook that rewrites the board between two reads leaves the copy an earlier read returned as it was.
func cloneOwners(owners boardengine.Owners) boardengine.Owners {
	return boardengine.Owners{Prime: owners.Prime, Entries: maps.Clone(owners.Entries)}
}

func (w *world) deps() adoptDeps {
	return adoptDeps{
		Machine:    func() (string, error) { return here, nil },
		ReadOwners: func() (boardengine.Owners, error) { return cloneOwners(w.owners), nil },
		ParentOf:   func(slug string) (string, error) { return w.parents[slug], nil },
		Reconcile: func() (fabricengine.OriginReport, error) {
			w.reconciled = true
			if w.afterReconcile != nil {
				w.afterReconcile(&w.owners)
			}
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
			w.order = append(w.order, "owners")
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
			if w.runErrAfterOwners != nil && slices.Contains(w.order, "owners") {
				return shedengine.Status{}, false, w.runErrAfterOwners
			}
			status, found := w.runs[slug]
			return status, found, nil
		},
		Tasks: func() ([]boardengine.Task, error) { return w.tasks, nil },
		UpdatePause: func(mutate func(*pauserecord.Record, time.Time) error) error {
			w.order = append(w.order, "pause")
			return mutate(&w.record, holdTime)
		},
		Holds: holdStore{
			Read: func(subject string) ([]string, error) { return w.holds[subject], nil },
			Append: func(subject string, slugs []string) error {
				w.order = append(w.order, "holds:"+subject)
				for _, slug := range slugs {
					if !slices.Contains(w.holds[subject], slug) {
						w.holds[subject] = append(w.holds[subject], slug)
					}
				}
				return nil
			},
			Remove: func(subject string, slugs []string) error {
				w.holds[subject] = slices.DeleteFunc(slices.Clone(w.holds[subject]), func(slug string) bool { return slices.Contains(slugs, slug) })
				return nil
			},
		},
		ResetTries: func(subject string) error {
			w.tries[subject] = 0
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
		CheckState: func(subject string) (string, error) {
			if state, listed := w.checkStates[subject]; listed {
				return state, nil
			}
			return checkStateNone, nil
		},
		EscalationEligible: func(slug string) bool { return w.escalation[slug] },
		DeployState:        func() (deployState, error) { return w.deploy, w.deployStateErr },
		RunDeploy: func(command string) (string, error) {
			w.order = append(w.order, "deploy")
			w.deployed = append(w.deployed, command)
			return deployLogName, w.deployErr
		},
		StartSuite: func(subject string) (suiteStart, error) {
			w.order = append(w.order, "suite:"+subject)
			if err := w.suiteErr[subject]; err != nil {
				return suiteStart{}, err
			}
			start := w.suite
			start.Subject = subject
			return start, nil
		},
	}
}

// holdTime is the time the fake pause update hands its mutation.
var holdTime = time.Unix(1000, 0)

// heldSlugs returns every slug a holds file lists, sorted.
func (w *world) heldSlugs() []string {
	var all []string
	for _, slugs := range w.holds {
		all = append(all, slugs...)
	}
	slices.Sort(all)
	return all
}

// testSuite is what the fake suite start answers unless a case sets another.
var testSuite = suiteStart{PID: 42, Log: "suite.log", Tiers: []int{0, 1, 2}}

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
	tiergate.Require(t, 0)
	t.Parallel()

	running := shedengine.Status{State: shedengine.StateRunning}

	// withBeam adds a beam project owned by another machine and a task of it with a running batten run.
	// The reconcile reads both as owned elsewhere.
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

	// staleBuild is a deploy state whose prime head is ahead of the running lym's build, with a deploy command set.
	staleBuild := deployState{Command: "go run ./tools/deploy", Installed: "87553450ec14", Head: "2d319fe5", Behind: true}

	// requireNoDeploy fails the test when the deploy ran or the envelope names one.
	requireNoDeploy := func(t *testing.T, w *world, got outcome) {
		t.Helper()
		if len(w.deployed) != 0 || got.Deploy != nil {
			t.Errorf("deployed %v, envelope deploy %+v; want no deploy", w.deployed, got.Deploy)
		}
	}

	// requireNothingRan fails the test when a step ran, which a refusal ahead of the origin reconcile leaves undone.
	requireNothingRan := func(t *testing.T, w *world) {
		t.Helper()
		if w.reconciled || w.ownerReq.Machine != "" || len(w.dropped) != 0 || len(w.order) != 0 {
			t.Errorf("a refused call ran: reconciled %v, owner request %+v, dropped %v, steps %v; want nothing", w.reconciled, w.ownerReq, w.dropped, w.order)
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
		{name: "RefusedOwnerWriteLeavesTheHoldsAndListsThem", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.setErr = errors.New("board: the push was refused") }, wantErr: "then re-run lym adopt with the same arguments", check: func(t *testing.T, w *world, got outcome) {
			if !slices.Equal(w.holds["alpha"], []string{"alpha"}) {
				t.Errorf("holds = %v after a refused owner write, want alpha's run still listed", w.holds)
			}
			if alpha := slugOf(t, got, "alpha"); !alpha.Held || alpha.Run != heldRun {
				t.Errorf("alpha = %+v, want the envelope to carry its hold", alpha)
			}
		}},
		{name: "LostOwnerWriteLeavesTheHoldAndNamesTheRerun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.lost = map[string]string{"alpha": "third"} }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Error, "third") || !strings.Contains(alpha.Error, "lym adopt alpha") || alpha.OwnerRecorded {
				t.Errorf("alpha = %+v, want the new owner and the re-run named", alpha)
			}
			if !alpha.Held || !slices.Equal(w.holds["alpha"], []string{"alpha"}) {
				t.Errorf("alpha = %+v, holds = %v, want its hold left standing", alpha, w.holds)
			}
		}},
		{name: "HubWithNoPrimeOwnerRecordsThisMachineAsPrimeOwner", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.owners.Prime = "" }, check: func(t *testing.T, w *world, got outcome) {
			if !w.ownerReq.Prime || !got.PrimeRecorded {
				t.Errorf("prime requested = %v, recorded = %v, want both", w.ownerReq.Prime, got.PrimeRecorded)
			}
		}},
		{name: "OwnerEntryBecomesTheHoldWithItsPauseTimeKept", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.record.Live = []pauserecord.Entry{{Scope: "alpha", Caller: pauserecord.OwnerCaller, PausedAt: time.Unix(100, 0)}}
		}, check: func(t *testing.T, w *world, got outcome) {
			want := []pauserecord.Entry{{Scope: "alpha", Caller: pauserecord.HoldCaller, PausedAt: time.Unix(100, 0)}}
			if !reflect.DeepEqual(w.record.Live, want) || len(w.lifted) != 0 {
				t.Errorf("live entries = %+v, lifted = %+v; want the one hold entry and no lift", w.record.Live, w.lifted)
			}
			alpha := slugOf(t, got, "alpha")
			if !alpha.Held || alpha.Run != heldRun || alpha.PauseLifted || !strings.Contains(alpha.PauseStanding, "adopt check of alpha") || strings.Contains(alpha.PauseStanding, "lym tender resume") {
				t.Errorf("alpha = %+v, want it held, not lifted, with the standing line naming the check and no resume", alpha)
			}
		}},
		{name: "AnotherCallersEntryIsKeptAndItsSlugIsNotHeld", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) {
			at := time.Unix(100, 0)
			w.record.Live = []pauserecord.Entry{
				{Scope: "alpha", Caller: pauserecord.OwnerCaller, PausedAt: at},
				{Scope: "beta", Caller: "strand", PausedAt: at},
			}
		}, check: func(t *testing.T, w *world, got outcome) {
			alpha, beta := slugOf(t, got, "alpha"), slugOf(t, got, "beta")
			if !alpha.Held || beta.Held || len(w.holds["beta"]) != 0 || !slices.Contains(w.order, "holds:beta") {
				t.Errorf("alpha = %+v, beta = %+v, holds = %v, steps = %v; want beta listed then dropped again and only alpha held", alpha, beta, w.holds, w.order)
			}
			if !strings.Contains(beta.PauseStanding, "lym tender resume beta") || !strings.Contains(beta.PauseStanding, "strand") {
				t.Errorf("beta standing = %q, want the other caller and lym tender resume beta named", beta.PauseStanding)
			}
			if i := slices.IndexFunc(w.record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == "beta" }); i < 0 || w.record.Live[i].Caller != "strand" {
				t.Errorf("live entries = %+v, want beta's entry of the other caller kept", w.record.Live)
			}
		}},
		{name: "HubPauseStandingIsNamedWithBareResume", req: request{Slugs: []string{"beta"}}, setup: func(w *world) {
			delete(w.runs, "beta")
			w.record.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, Caller: "operator"}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if standing := slugOf(t, got, "beta").PauseStanding; !strings.Contains(standing, "`lym tender resume`") {
				t.Errorf("pause standing = %q, want the hub pause named with lym tender resume", standing)
			}
		}},
		{name: "BeamWithNoRunRecordGetsItsOwnerAndNoRun", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { delete(w.runs, "beta") }, check: func(t *testing.T, w *world, got outcome) {
			if beta := slugOf(t, got, "beta"); !beta.OwnerRecorded || beta.Run != "none" || len(w.heldSlugs()) != 0 {
				t.Errorf("beta = %+v, holds = %v, want the owner and no run to hold", beta, w.holds)
			}
		}},
		{name: "RunUnderABeamOwnedElsewhereIsHeldBeforeTheOwnerWrite", req: request{Slugs: []string{"payments"}}, setup: withBeam, check: func(t *testing.T, w *world, got outcome) {
			t1 := slugOf(t, got, "t1")
			if !t1.Held || t1.Run != heldRun || t1.OwnerRecorded || !slices.Equal(w.holds["payments"], []string{"t1"}) {
				t.Errorf("t1 = %+v, holds = %v; want its run held under payments with no owner of its own", t1, w.holds)
			}
			if want := []string{"holds:payments", "pause", "owners"}; !slices.Equal(w.order[:3], want) {
				t.Errorf("steps = %v, want the holds listed and the pause updated before the owner write", w.order)
			}
			if w.ownerReq.Machine == "" || !slices.Equal(w.ownerReq.Slugs, []string{"payments"}) || w.ownerReq.Prime {
				t.Errorf("owner request = %+v, want the beam alone", w.ownerReq)
			}
		}},
		{name: "BeamTurnedToAnotherMachineByTheReconcileGetsItsRunsHeld", req: request{Slugs: []string{"payments"}}, setup: func(w *world) {
			withBeam(w)
			w.owners.Entries["payments"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
			w.afterReconcile = func(owners *boardengine.Owners) {
				owners.Entries["payments"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			}
		}, check: func(t *testing.T, w *world, got outcome) {
			if t1 := slugOf(t, got, "t1"); !t1.Held || !slices.Equal(w.holds["payments"], []string{"t1"}) {
				t.Errorf("t1 = %+v, holds = %v; want its run held by the owner the reconcile left", t1, w.holds)
			}
		}},
		{name: "ReleasedTaskOfAnAdoptedBeamIsHeldAndAFrontTaskIsNot", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			for _, slug := range []string{"rel", "front", "started", "other"} {
				w.owners.Entries[slug] = boardengine.OwnerEntry{IsTask: true}
				w.parents[slug] = "alpha"
			}
			w.parents["other"] = "beta"
			started := "running · Preflight"
			w.tasks = []boardengine.Task{
				{Slug: "rel", Kind: boardengine.KindTask},
				{Slug: "front", Kind: boardengine.KindTask, Front: true},
				{Slug: "started", Kind: boardengine.KindTask, Status: &started},
				{Slug: "other", Kind: boardengine.KindTask},
			}
		}, check: func(t *testing.T, w *world, got outcome) {
			if want := []string{"alpha", "rel"}; !slices.Equal(w.holds["alpha"], want) {
				t.Errorf("holds of alpha = %v, want %v: the pawl run and the released task, not the front, the started or another beam's task", w.holds["alpha"], want)
			}
			if rel := slugOf(t, got, "rel"); !rel.Held || rel.OwnerRecorded {
				t.Errorf("rel = %+v, want it held with no owner of its own", rel)
			}
		}},
		{name: "KeepLocalOverALivePairLeavesTheRunUnheldAndReportsItRunningHere", req: request{Slugs: []string{"alpha"}, KeepLocal: true}, setup: func(w *world) {
			w.live["alpha"] = true
			w.stopBranch("alpha", fabricengine.BranchDiverged)
		}, check: func(t *testing.T, w *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); alpha.Held || alpha.Run != runningHereRun || !alpha.OwnerRecorded || len(w.heldSlugs()) != 0 {
				t.Errorf("alpha = %+v, holds = %v; want the run unheld, running here, and the owner recorded", alpha, w.holds)
			}
		}},
		{name: "SecondAdoptRightAfterChangesNoOwnerAndReportsTheSameHolds", req: request{Slugs: []string{"alpha"}}, check: func(t *testing.T, w *world, first outcome) {
			second, err := adopt(w.deps(), request{Slugs: []string{"alpha"}})
			if err != nil {
				t.Fatalf("second adopt: %v", err)
			}
			if a, b := slugOf(t, first, "alpha"), slugOf(t, second, "alpha"); !a.Held || !b.Held || b.Run != heldRun || !slices.Equal(w.holds["alpha"], []string{"alpha"}) {
				t.Errorf("first = %+v, second = %+v, holds = %v; want the same held slug both times and one listing", a, b, w.holds)
			}
			if !reflect.DeepEqual(first.Runs, second.Runs) || len(first.Runs) == 0 {
				t.Errorf("first runs = %+v, second runs = %+v; want the same states for the runs not done", first.Runs, second.Runs)
			}
		}},
		{name: "RunsHoldEveryMovedAndHeldRunSortedBySlug", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
			w.owners.Entries["rel"] = boardengine.OwnerEntry{IsTask: true}
			w.parents["t1"], w.parents["rel"] = "alpha", "alpha"
			w.runs["t1"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Code-Bouncer"}
			w.report.Entries = append(w.report.Entries, fabricengine.EntryOutcome{Slug: "t1", State: fabricengine.EntryOwnedElsewhere, Owner: there})
			w.tasks = []boardengine.Task{{Slug: "rel", Kind: boardengine.KindTask}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			var states []string
			for _, run := range got.Runs {
				states = append(states, run.Slug+":"+run.State+":"+run.Row)
			}
			if want := []string{"alpha:waiting_for_check:", "rel:waiting_for_check:", "t1:waiting_for_check:Code-Bouncer"}; !slices.Equal(states, want) {
				t.Errorf("runs = %q; want every moved and held run sorted by slug, a released task with no row", states)
			}
			if got.Runs[2].Line != "t1: waiting for the adopt check at Code-Bouncer" {
				t.Errorf("t1 line = %q; want it to name the row it waits at", got.Runs[2].Line)
			}
		}},
		{name: "ARunRecordReadThatFailsAfterTheOwnerWriteLeavesRunsEmptyAndWarns", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.runErrAfterOwners = errors.New("run record locked")
		}, check: func(t *testing.T, w *world, got outcome) {
			if len(got.Runs) != 0 || !slices.ContainsFunc(got.Warnings, func(line string) bool {
				return strings.Contains(line, "run record locked") && strings.Contains(line, "then re-run lym adopt with the same arguments")
			}) {
				t.Errorf("runs = %+v, warnings = %q; want no runs and a warning naming the error and the re-run", got.Runs, got.Warnings)
			}
			if !slugOf(t, got, "alpha").OwnerRecorded || !slices.Contains(w.order, "owners") {
				t.Errorf("alpha = %+v; want the steps already taken to stand", slugOf(t, got, "alpha"))
			}
		}},
		{name: "ReRunWithHoldsStandingAndNoOwnerChangeStartsTheCheckAndResetsTheTries", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.owners.Entries["alpha"] = boardengine.OwnerEntry{Owner: here, IsProject: true, Beam: true, RunHeld: true}
			w.holds["alpha"] = []string{"alpha"}
			w.tries["alpha"] = 2
			w.record.Live = []pauserecord.Entry{{Scope: "alpha", Caller: pauserecord.HoldCaller}}
		}, check: func(t *testing.T, w *world, got outcome) {
			if !slices.Contains(w.order, "suite:alpha") || w.tries["alpha"] != 0 || !slugOf(t, got, "alpha").Held {
				t.Errorf("steps = %v, tries = %v, alpha = %+v; want alpha's check started, its tries reset and its hold reported", w.order, w.tries, slugOf(t, got, "alpha"))
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
			if !alpha.Held || alpha.Error != "" {
				t.Errorf("alpha = %+v, want its pawl run still held, with no error, once the parent's owner changed", alpha)
			}
		}},
		{name: "RewrittenPrimeBranchWithNoWayForwardIsAWarningNamingFabricPull", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "code", Prime: true, Worktree: "/hub/main", State: fabricengine.BranchRewritten}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if want := []string{"main is rewritten against origin and was left unmoved: run lym fabric pull in /hub/main, then re-run lym adopt with the same arguments"}; !slices.Equal(got.Warnings, want) {
				t.Errorf("warnings = %q, want %q", got.Warnings, want)
			}
		}},
		{name: "DivergedBranchOfAnAdoptedBeamCarriesItsWayForwardInParentheses", req: request{Slugs: []string{"alpha"}, KeepLocal: true}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "code-alpha", Repo: "code", Slug: "alpha", Worktree: "/hub/alpha", State: fabricengine.BranchDiverged, WayForward: "merge origin first"}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if want := []string{"code-alpha is diverged against origin and was left unmoved: run lym fabric pull in /hub/alpha (merge origin first), then re-run lym adopt with the same arguments"}; !slices.Equal(got.Warnings, want) {
				t.Errorf("warnings = %q, want %q", got.Warnings, want)
			}
		}},
		{name: "AheadPrimeBranchWithMovedUnsetIsAWarningAndMovedSetIsNot", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{
				{Branch: "main", Repo: "code", Prime: true, Worktree: "/hub/main", State: fabricengine.BranchAhead},
				{Branch: "records-main", Repo: "records", Prime: true, Worktree: "/hub/records", CodeWorktree: "/hub/main", State: fabricengine.BranchAhead, Moved: true},
			}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if len(got.Warnings) != 1 || !strings.HasPrefix(got.Warnings[0], "main is ahead against origin") {
				t.Errorf("warnings = %q, want one for the code branch alone, none for the replayed records branch", got.Warnings)
			}
		}},
		{name: "RecordsBranchOfAnAdoptedBeamNamesTheCodeWorktree", req: request{Slugs: []string{"alpha"}, KeepLocal: true}, setup: func(w *world) {
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "records-alpha", Repo: "records", Slug: "alpha", Worktree: "/hub/alpha-records", CodeWorktree: "/hub/alpha", State: fabricengine.BranchDiverged}}
		}, check: func(t *testing.T, _ *world, got outcome) {
			if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "lym fabric pull in /hub/alpha,") || strings.Contains(got.Warnings[0], "alpha-records") {
				t.Errorf("warnings = %q, want one naming lym fabric pull in the code worktree alone", got.Warnings)
			}
		}},
		{name: "StoppedRunAndUnadoptedBeamAndSharedStopGetNoWarning", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.stopBranch("alpha", fabricengine.BranchDiverged)
			w.report.Branches = append(w.report.Branches, fabricengine.BranchOutcome{Branch: "code-beta", Repo: "code", Slug: "beta", Worktree: "/hub/beta", State: fabricengine.BranchDiverged})
		}, check: func(t *testing.T, _ *world, got outcome) {
			if len(got.Warnings) != 0 {
				t.Errorf("warnings = %q, want none for a branch of a stopped run or of a beam not adopted", got.Warnings)
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
			if !t1.Held || t1.OwnerRecorded || !slices.Equal(w.holds["payments"], []string{"t1"}) {
				t.Errorf("t1 = %+v, holds = %v, want its run held under payments with no owner of its own", t1, w.holds)
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
			if !slices.Equal(w.holds["root"], []string{"kid1"}) || len(w.holds["payments"]) != 0 || !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Errorf("holds = %v, owner request = %+v, want the root task's run held under root and the prime's owner alone, the beam's member t1 left", w.holds, w.ownerReq)
			}
		}},
		{name: "BeamPawlRunIsResumedWhenRootIsAdoptedWithTheBeamAlreadyOwnedHere", req: request{Root: true}, setup: func(w *world) {
			w.owners.Prime = there
			w.owners.Entries["alpha"] = boardengine.OwnerEntry{Owner: here, IsProject: true, Beam: true, RunHeld: true}
			delete(w.owners.Entries, "beta")
			delete(w.runs, "beta")
		}, check: func(t *testing.T, w *world, got outcome) {
			if alpha := slugOf(t, got, "alpha"); alpha.Held || len(w.heldSlugs()) != 0 {
				t.Errorf("alpha = %+v, holds = %v, want a pawl run this machine already owns left unheld", alpha, w.holds)
			}
		}},
		{name: "BeamPawlRunIsLeftWhileAnotherMachineOwnsItsParent", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.owners.Prime = there }, check: func(t *testing.T, w *world, got outcome) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.OwnerRecorded || alpha.Run != ownedElsewhereRun || alpha.OwnedBy != there || alpha.Error != "" || alpha.Held {
				t.Errorf("alpha = %+v, held = %v, want the beam adopted and its pawl run reported owned elsewhere by the prime's owner, with no error", alpha, alpha.Held)
			}
		}},
		{name: "NestedBeamOwnedElsewhereUnderAdoptedRootIsReportedOwnedElsewhere", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["nested"] = boardengine.OwnerEntry{Owner: "third", IsProject: true, Beam: true, RunHeld: true}
			w.runs["nested"] = running
		}, check: func(t *testing.T, w *world, got outcome) {
			nested := slugOf(t, got, "nested")
			if nested.Run != ownedElsewhereRun || nested.OwnedBy != "third" || nested.Error != "" || nested.PairAdded || nested.Held || len(w.heldSlugs()) != 0 {
				t.Errorf("nested = %+v, holds = %v, want its run reported owned elsewhere by third, with no error, pair or hold", nested, w.holds)
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
			if strings.Contains(t1.WayForward, "lym adopt t1") || w.ownerReq.Machine != "" || len(w.heldSlugs()) != 0 {
				t.Errorf("way forward = %q, owner request = %+v, holds = %v; want no adopt of the task and nothing held or written", t1.WayForward, w.ownerReq, w.holds)
			}
		}},
		{name: "BeamStepsRunAfterTheResumesForThePrimeAndEachAdoptedBeamThenOneCheckStartPerSubject", req: request{Slugs: []string{"alpha", "beta"}, Root: true}, check: func(t *testing.T, w *world, got outcome) {
			if want := []string{"holds:alpha", "holds:beta", "pause", "owners", "beamup:prime", "beamup:alpha", "beamup:beta", "suite:root", "suite:alpha", "suite:beta"}; !slices.Equal(w.order, want) || got.PrimeBeamWarning != "" {
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
			var wantChecks []suiteStart
			for _, subject := range []string{"root", "alpha", "beta"} {
				start := testSuite
				start.Subject = subject
				wantChecks = append(wantChecks, start)
			}
			if !reflect.DeepEqual(got.Checks, wantChecks) || got.CheckWarnings != nil {
				t.Errorf("checks = %+v, warnings = %q; want %+v and no warning", got.Checks, got.CheckWarnings, wantChecks)
			}
			want := "adopt check of root started in its worktree, tiers 0, 1, 2; the prime's reed status bar shows its progress and result\n" +
				"adopt check of alpha started in its worktree, tiers 0, 1, 2; the prime's reed status bar shows its progress and result\n" +
				"adopt check of beta started in its worktree, tiers 0, 1, 2; the prime's reed status bar shows its progress and result"
			if got.SuiteMessage != want {
				t.Errorf("message = %q; want %q", got.SuiteMessage, want)
			}
		}},
		{name: "PrimeOwnerRecordedAloneRunsThePrimesBeamStepsAndStartsRootsCheckLast", req: request{Root: true}, setup: withoutAlphaAndBeta, check: func(t *testing.T, w *world, got outcome) {
			if !w.ownerReq.Prime || len(w.ownerReq.Slugs) != 0 {
				t.Fatalf("owner request = %+v, want the prime's owner alone", w.ownerReq)
			}
			if !slices.Equal(w.beamUps, []string{"prime"}) || got.PrimeBeamSteps == nil {
				t.Errorf("beam steps run for %v, prime steps %+v; want the prime alone", w.beamUps, got.PrimeBeamSteps)
			}
			if len(got.Checks) != 1 || got.Checks[0].Subject != "root" || len(w.order) == 0 || w.order[len(w.order)-1] != "suite:root" {
				t.Errorf("checks = %+v, steps = %v; want root's check as the one start and the last step", got.Checks, w.order)
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
		{name: "CallThatRecordsNoOwnerStartsNoCheck", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.stopBranch("alpha", fabricengine.BranchDiverged) }, check: func(t *testing.T, w *world, got outcome) {
			if slices.Contains(w.order, "suite:alpha") || got.Checks != nil || got.CheckWarnings != nil {
				t.Errorf("steps = %v, checks = %+v, warnings = %q; want no check start", w.order, got.Checks, got.CheckWarnings)
			}
		}},
		{name: "CheckFoundRunningWithNoTiersReadIsNamedAlreadyRunning", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.suite = suiteStart{PID: 7, AlreadyRunning: true} }, check: func(t *testing.T, _ *world, got outcome) {
			if want := "adopt check of alpha already running in its worktree; the prime's reed status bar shows its progress and result"; got.SuiteMessage != want {
				t.Errorf("message = %q; want %q", got.SuiteMessage, want)
			}
		}},
		{name: "FailedCheckStartForTheBeamOnlyIsTheBeamsWarningAndRootsCheckIsStillStarted", req: request{Slugs: []string{"alpha"}, Root: true}, setup: func(w *world) { w.suiteErr["alpha"] = errors.New("exec failed") }, check: func(t *testing.T, w *world, got outcome) {
			want := []string{"the adopt check of alpha was not started (exec failed); way forward: re-run lym adopt alpha"}
			if !slices.Equal(got.CheckWarnings, want) {
				t.Errorf("check warnings = %q; want %q", got.CheckWarnings, want)
			}
			if len(got.Checks) != 1 || got.Checks[0].Subject != "root" || !slices.Equal(w.order[len(w.order)-2:], []string{"suite:root", "suite:alpha"}) {
				t.Errorf("checks = %+v, steps = %v; want root's check started, then alpha's attempted", got.Checks, w.order)
			}
			if message := got.SuiteMessage; !strings.Contains(message, "adopt check of root") || strings.Contains(message, "alpha") {
				t.Errorf("message = %q; want one line naming root alone", message)
			}
			if alpha := slugOf(t, got, "alpha"); !alpha.OwnerRecorded || !alpha.Held || alpha.Error != "" {
				t.Errorf("alpha = %+v, want it adopted and held beside the failed check start", alpha)
			}
		}},
		{name: "RootDeploysALymOlderThanThePrimesHeadBeforeTheOwnerWriteWhenThisMachineAlreadyOwnsThePrime", req: request{Root: true}, setup: func(w *world) { w.deploy = staleBuild }, check: func(t *testing.T, w *world, got outcome) {
			if !slices.Equal(w.deployed, []string{staleBuild.Command}) {
				t.Errorf("deployed %v; want %q once", w.deployed, staleBuild.Command)
			}
			deploy, owners := slices.Index(w.order, "deploy"), slices.Index(w.order, "owners")
			if deploy < 0 || owners < deploy {
				t.Errorf("steps = %v; want the deploy before the owner write", w.order)
			}
			want := deployOutcome{Installed: staleBuild.Installed, Head: staleBuild.Head, Log: deployLogName}
			if got.Deploy == nil || *got.Deploy != want || !got.PrimeRecorded {
				t.Errorf("deploy = %+v, prime recorded = %v; want %+v and the prime recorded", got.Deploy, got.PrimeRecorded, want)
			}
		}},
		{name: "AFailedDeployRefusesBeforeTheOwnerWriteWithTheHoldsStanding", req: request{Slugs: []string{"alpha"}, Root: true}, setup: func(w *world) {
			w.deploy = staleBuild
			w.deployErr = errors.New("exit code 1")
			w.report.Mutations.AppendRef(fabricengine.KindRemoteBranchUpdated, "main", "")
		}, wantErr: "the landing deploy failed: exit code 1; log: deploy.log", check: func(t *testing.T, w *world, got outcome) {
			if wantMutations := []fabricengine.Mutation{{Kind: fabricengine.KindRemoteBranchUpdated, Target: "main"}}; !got.Partial || !reflect.DeepEqual(got.Mutations, wantMutations) {
				t.Errorf("partial = %v, mutations = %+v; want partial and the reconcile's %+v on the refusal", got.Partial, got.Mutations, wantMutations)
			}
			if w.ownerReq.Machine != "" || slices.ContainsFunc(w.order, func(step string) bool { return strings.HasPrefix(step, "suite:") || strings.HasPrefix(step, "beamup:") }) {
				t.Errorf("owner request = %+v, steps = %v; want no owner write, beam steps or check after a failed deploy", w.ownerReq, w.order)
			}
			if !slices.Equal(w.holds["alpha"], []string{"alpha"}) || got.Deploy == nil {
				t.Errorf("holds = %v, deploy = %+v; want alpha's run still held and the deploy named", w.holds, got.Deploy)
			}
		}},
		{name: "AStopHoldingRootBackDeploysNothingAndWritesNoOwner", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["kid1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
			w.runs["kid1"] = running
			w.pairs["kid1"] = true
			w.report.Entries = append(w.report.Entries, fabricengine.EntryOutcome{Slug: "kid1", State: fabricengine.EntryOwnedElsewhere, Owner: there})
			w.stopBranch("kid1", fabricengine.BranchDiverged)
			w.deploy = staleBuild
		}, check: func(t *testing.T, w *world, got outcome) {
			requireNoDeploy(t, w, got)
			if kid1 := slugOf(t, got, "kid1"); !kid1.Stopped || w.ownerReq.Machine != "" || got.PrimeRecorded {
				t.Errorf("kid1 = %+v, owner request = %+v, prime recorded = %v; want kid1 stopped under root, no owner write and the prime left", kid1, w.ownerReq, got.PrimeRecorded)
			}
		}},
		{name: "ALymBuiltFromThePrimesHeadIsNotDeployed", req: request{Root: true}, setup: func(w *world) {
			w.deploy = staleBuild
			w.deploy.Behind = false
		}, check: requireNoDeploy},
		{name: "NoDeployCommandForThePrimesBranchDeploysNothing", req: request{Root: true}, setup: func(w *world) {
			w.deploy = staleBuild
			w.deploy.Command = ""
		}, check: requireNoDeploy},
		{name: "ABeamAdoptUnderAPrimeOwnedHereDeploysNothing", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.deploy = staleBuild }, check: requireNoDeploy},
		{name: "AnUnreadableDeployStateIsAWarningAndTheAdoptGoesOn", req: request{Root: true}, setup: func(w *world) { w.deployStateErr = errors.New("the installed lym carries no VCS stamp") }, check: func(t *testing.T, w *world, got outcome) {
			requireNoDeploy(t, w, got)
			if !got.PrimeRecorded || !slices.ContainsFunc(got.Warnings, func(line string) bool {
				return strings.Contains(line, "could not be read (the installed lym carries no VCS stamp)") && strings.Contains(line, "run the hub's landing deploy")
			}) {
				t.Errorf("prime recorded = %v, warnings = %q; want the prime recorded and a warning naming the hand deploy", got.PrimeRecorded, got.Warnings)
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
