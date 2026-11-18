// adopt_test.go drives the foreground adopt over fake deps, from the refusals to the beam steps.

package adoptcli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	here  = "here"
	there = "there"
)

// testNow is the time the fake clock reads.
var testNow = time.Unix(3000, 0)

// world is the fake hub every deps seam reads and writes.
type world struct {
	owners  boardengine.Owners
	pairs   map[string]bool
	parents map[string]string
	addErr  map[string]error
	// local and origin are the batten run records by slug on this machine and at origin.
	local  map[string]shedengine.Status
	origin map[string]shedengine.Status
	// moving is the target anchor of the move record standing in each worktree, "" keying the prime.
	moving map[string]string
	// hold is the hold the adopt write leaves standing; nil makes it write an adopt hold over the subjects.
	hold     *boardengine.Hold
	writeErr error
	writeReq boardengine.AdoptRequest
	// takeErr fails the takeover of its slug, and takenFor maps a slug taken over to its subject.
	takeErr  map[string]error
	takenFor map[string]string
	// catchUpErr and workspaceErr fail their step.
	catchUpErr   error
	workspaceErr error
	// check is the adopt check's result; the zero value is a green check of every subject of the hold.
	check       holdCheckResult
	checkErr    error
	checkNoOrch bool
	// onPull runs after each Bolt pull, counted from one.
	onPull func(w *world, pull int)
	pulls  int
	// order records the calls in the order they ran.
	// It holds pull, writeadopt, addpair:<slug>, takeover:<slug> and catchup.
	// It then holds deploy, workspace, check:<first> and beamup:<worktree>.
	order     []string
	beamUps   []string
	noOrch    []bool
	beamUpErr map[string]error
	// held names the beams whose pawl run holds the landing hold, and heldErr the beams whose hold reads fail.
	held    map[string]bool
	heldErr map[string]error
	// deploy is what the deploy-state read answers, and deployStateErr what it fails with.
	// deployErr is what the deploy fails with.
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
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"alpha": {Owner: there, IsProject: true, Beam: true, RunHeld: true},
			"beta":  {Owner: there, IsProject: true, Beam: true, RunHeld: true},
			"idle":  {IsTask: true},
			"done1": {IsTask: true, Done: true},
			"note1": {},
		}},
		pairs:     map[string]bool{"alpha": true, "beta": true},
		parents:   map[string]string{},
		addErr:    map[string]error{},
		local:     map[string]shedengine.Status{"alpha": running, "beta": running},
		origin:    map[string]shedengine.Status{"alpha": running, "beta": running},
		moving:    map[string]string{},
		takeErr:   map[string]error{},
		takenFor:  map[string]string{},
		beamUpErr: map[string]error{},
		held:      map[string]bool{},
		heldErr:   map[string]error{},
	}
}

// calls returns the calls the adopt made, without the Bolt pulls.
func (w *world) calls() []string {
	return slices.DeleteFunc(slices.Clone(w.order), func(call string) bool { return call == "pull" })
}

// called reports whether the call ran.
func (w *world) called(call string) bool { return slices.Contains(w.order, call) }

// calledWithPrefix reports whether any call opens with prefix.
func (w *world) calledWithPrefix(prefix string) bool {
	return slices.ContainsFunc(w.order, func(call string) bool { return strings.HasPrefix(call, prefix) })
}

// cloneOwners returns owners with its own maps and holds,
// so a hook that rewrites the board between two reads leaves the copy an earlier read returned as it was.
func cloneOwners(owners boardengine.Owners) boardengine.Owners {
	return boardengine.Owners{Prime: owners.Prime, Entries: maps.Clone(owners.Entries), Holds: slices.Clone(owners.Holds)}
}

func (w *world) deps() adoptDeps {
	return adoptDeps{
		Machine:    func() (string, error) { return here, nil },
		ReadOwners: func() (boardengine.Owners, error) { return cloneOwners(w.owners), nil },
		PullBolt: func() error {
			w.pulls++
			w.order = append(w.order, "pull")
			if w.onPull != nil {
				w.onPull(w, w.pulls)
			}
			return nil
		},
		ParentOf: func(slug string) (string, error) { return w.parents[slug], nil },
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			status, found := w.local[slug]
			return status, found, nil
		},
		OriginRun: func(slug, _ string) (shedengine.Status, bool, error) {
			status, found := w.origin[slug]
			return status, found, nil
		},
		LocalPair: func(slug string) bool { return w.pairs[slug] },
		AddPair: func(slug string) (fabricengine.AddResult, error) {
			w.order = append(w.order, "addpair:"+slug)
			if err := w.addErr[slug]; err != nil {
				return fabricengine.AddResult{}, err
			}
			w.pairs[slug] = true
			return fabricengine.AddResult{Slug: slug}, nil
		},
		WriteAdopt: w.writeAdopt,
		TakeOver: func(slug, subject string) (fabricengine.TakeOverResult, error) {
			w.order = append(w.order, "takeover:"+slug)
			if err := w.takeErr[slug]; err != nil {
				return fabricengine.TakeOverResult{}, err
			}
			w.takenFor[slug] = subject
			status := w.origin[slug]
			status.Takeover = &shedengine.Takeover{Machine: here, Subject: subject}
			w.origin[slug] = status
			return fabricengine.TakeOverResult{}, nil
		},
		CatchUp: func() error {
			w.order = append(w.order, "catchup")
			return w.catchUpErr
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
		DeployState: func() (deployState, error) { return w.deploy, w.deployStateErr },
		RunDeploy: func(command string) (string, error) {
			w.order = append(w.order, "deploy")
			w.deployed = append(w.deployed, command)
			return deployLogName, w.deployErr
		},
		Workspace: func() error {
			w.order = append(w.order, "workspace")
			return w.workspaceErr
		},
		HoldCheck: w.holdCheck,
		BeamUp: func(worktree string, noOrch bool) (orchcli.BeamUpReport, error) {
			w.order = append(w.order, "beamup:"+worktree)
			w.beamUps = append(w.beamUps, worktree)
			w.noOrch = append(w.noOrch, noOrch)
			return orchcli.BeamUpReport{Worktree: worktree, Dobby: "started", Action: "relaunched"}, w.beamUpErr[worktree]
		},
		LandingHeld: func(slug string) (bool, error) { return w.held[slug], w.heldErr[slug] },
		PrimeName:   "prime",
		Now:         func() time.Time { return testNow },
	}
}

// writeAdopt is the fake adopt write: it records the request and leaves the hold standing with each subject owned here.
func (w *world) writeAdopt(req boardengine.AdoptRequest) (boardengine.AdoptWrite, error) {
	w.order = append(w.order, "writeadopt")
	w.writeReq = req
	if w.writeErr != nil {
		return boardengine.AdoptWrite{}, w.writeErr
	}
	subjects := slices.Clone(req.Slugs)
	recorded := slices.Clone(req.Slugs)
	if req.Prime {
		subjects = append(subjects, boardengine.RootSubject)
		recorded = append(recorded, boardengine.OwnerWritePrime)
	}
	var hold boardengine.Hold
	if w.hold != nil {
		hold = *w.hold
	} else {
		hold = boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: req.Machine, At: req.Now}
		for _, subject := range subjects {
			prior := w.owners.Prime
			if subject != boardengine.RootSubject {
				prior = w.owners.Entries[subject].Owner
			}
			hold.Subjects = append(hold.Subjects, boardengine.HoldSubject{Subject: subject, PriorOwner: prior})
		}
	}
	for _, slug := range req.Slugs {
		entry := w.owners.Entries[slug]
		entry.Owner = req.Machine
		w.owners.Entries[slug] = entry
	}
	if req.Prime {
		w.owners.Prime = req.Machine
	}
	w.owners.Holds = []boardengine.Hold{hold}
	return boardengine.AdoptWrite{Hold: hold, Recorded: recorded}, nil
}

// holdCheck is the fake adopt check: a zero check result is green for every subject of the hold and clears it.
func (w *world) holdCheck(first string, noOrch bool) (holdCheckResult, error) {
	w.order = append(w.order, "check:"+first)
	w.checkNoOrch = noOrch
	if w.check.State != "" || w.checkErr != nil {
		return w.check, w.checkErr
	}
	result := holdCheckResult{State: checkGreen, Green: true, Ran: true, States: map[string]string{}}
	for _, hold := range w.owners.Holds {
		for _, subject := range hold.Subjects {
			result.States[subject.Subject] = checkGreen
		}
	}
	w.owners.Holds = nil
	return result, nil
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
	tiergate.Require(t, 0)
	t.Parallel()

	running := shedengine.Status{State: shedengine.StateRunning}

	// withBeam adds a beam project owned by another machine and a task of it with a running batten run.
	withBeam := func(w *world) {
		w.owners.Entries["payments"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
		w.owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
		w.parents["t1"] = "payments"
		w.local["t1"], w.origin["t1"] = running, running
		w.pairs["t1"] = true
	}

	// withoutAlphaAndBeta leaves a board without the two default beams.
	withoutAlphaAndBeta := func(w *world) {
		for _, slug := range []string{"alpha", "beta"} {
			delete(w.owners.Entries, slug)
			delete(w.local, slug)
			delete(w.origin, slug)
			delete(w.pairs, slug)
		}
	}

	// withFixTask adds the task of an adopt-fix hold, a child of alpha with a running run and a pair here.
	withFixTask := func(w *world) {
		w.owners.Entries["fix1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
		w.parents["fix1"] = "alpha"
		w.local["fix1"], w.origin["fix1"] = running, running
		w.pairs["fix1"] = true
	}

	// tookOver marks the run of slug as taken over at origin by machine.
	tookOver := func(w *world, slug, machine string) {
		status := w.origin[slug]
		status.Takeover = &shedengine.Takeover{Machine: machine, Subject: "alpha"}
		w.origin[slug] = status
	}

	// holdOver returns an adopt hold of this machine over subjects, each of whose prior owner is prior.
	holdOver := func(prior string, subjects ...string) *boardengine.Hold {
		hold := &boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: here, At: testNow}
		for _, subject := range subjects {
			hold.Subjects = append(hold.Subjects, boardengine.HoldSubject{Subject: subject, PriorOwner: prior})
		}
		return hold
	}

	// fixHold returns an adopt-fix hold of this machine over alpha, whose task is fix1 and whose filer is filer.
	fixHold := func(filer string) *boardengine.Hold {
		hold := holdOver(there, "alpha")
		hold.Kind, hold.FixTask, hold.FixMachine = boardengine.HoldKindAdoptFix, "fix1", filer
		hold.Subjects[0].Tries = 1
		return hold
	}

	// staleBuild is a deploy state whose prime head is ahead of the running lym's build, with a deploy command set.
	staleBuild := deployState{Command: "go run ./tools/deploy", Installed: "87553450ec14", Head: "2d319fe5", Behind: true}

	// requireNoWrite fails the test when the adopt wrote a hold, which a refusal ahead of the write leaves undone.
	requireNoWrite := func(t *testing.T, w *world, _ outcome, _ error) {
		t.Helper()
		if len(w.order) != 0 {
			t.Errorf("a refused call ran %v; want nothing", w.order)
		}
	}

	// requireNoDeploy fails the test when the deploy ran or the envelope names one.
	requireNoDeploy := func(t *testing.T, w *world, got outcome, _ error) {
		t.Helper()
		if len(w.deployed) != 0 || got.Deploy != nil {
			t.Errorf("deployed %v, envelope deploy %+v; want no deploy", w.deployed, got.Deploy)
		}
	}

	// failedCheck is the result of an adopt check whose alpha failed and filed the fix task fix1.
	failedCheck := holdCheckResult{
		State: checkFailed, Ran: true, States: map[string]string{"alpha": checkFailed}, FixTask: "fix1",
		Lines: []string{"the adopt check of alpha failed at tier0; fix task fix1 is filed"},
	}

	tests := []struct {
		name    string
		req     request
		setup   func(*world)
		wantErr string
		check   func(*testing.T, *world, outcome, error)
	}{
		{name: "RefusesUnknownSlug", req: request{Slugs: []string{"nope"}}, wantErr: "lym board list", check: requireNoWrite},
		{name: "StandingMoveRecordOnThePrimeRefusesNamingTheSetAndTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving[""] = "sub" }, wantErr: "lym fabric anchor main --set sub", check: requireNoWrite},
		{name: "StandingMoveRecordOnANamedEntrysWorktreeRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving["alpha"] = "sub" }, wantErr: "lym fabric anchor alpha --set sub", check: requireNoWrite},
		{name: "StandingMoveRecordOnANamedEntrysParentRefuses", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.parents["alpha"] = "gamma"
			w.moving["gamma"] = "sub"
		}, wantErr: "lym fabric anchor gamma --set sub` to finish it; then re-run lym adopt with the same arguments", check: requireNoWrite},
		{name: "StandingMoveRecordOnAnUnrelatedWorktreeDoesNotRefuse", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.moving["unrelated"] = "sub" }, check: func(t *testing.T, w *world, got outcome, _ error) {
			if !slugOf(t, got, "alpha").OwnerRecorded {
				t.Errorf("alpha = %+v, want it adopted beside a record in an unrelated worktree", slugOf(t, got, "alpha"))
			}
		}},
		{name: "RefusesNote", req: request{Slugs: []string{"note1"}}, wantErr: "lym board promote"},
		{name: "RefusesDoneTaskNamingResume", req: request{Slugs: []string{"done1"}}, wantErr: "lym tender resume done1"},
		{name: "RefusesNoSlugWithoutRootOrAll", req: request{}, wantErr: "or pass --all"},
		{name: "TakeHoldWithAllRefusesNamingBeamStatus", req: request{All: true, TakeHold: true}, wantErr: "lym beam status", check: func(t *testing.T, w *world, _ outcome, err error) {
			requireNoWrite(t, w, outcome{}, err)
			if strings.Contains(err.Error(), "--take-hold") {
				t.Errorf("error %q spells the take-hold flag, which an agent may read", err)
			}
		}},
		{name: "RefusesATaskNamingItsBeam", req: request{Slugs: []string{"t1"}}, setup: withBeam, wantErr: "`lym adopt payments`", check: requireNoWrite},
		{name: "RefusesARootTaskNamingRoot", req: request{Slugs: []string{"idle"}}, wantErr: "`lym adopt root`", check: requireNoWrite},
		{name: "RefusesAGroupingProjectBelowRootNamingRootAndBeamCreate", req: request{Slugs: []string{"grp"}}, setup: func(w *world) {
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
		}, wantErr: "`lym adopt root` moves its tasks, and `lym beam create grp` makes it a beam"},
		{name: "RefusesAGroupingProjectBelowABeamNamingTheBeamAndBeamCreate", req: request{Slugs: []string{"grp"}}, setup: func(w *world) {
			withBeam(w)
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
			w.parents["grp"] = "payments"
		}, wantErr: "`lym adopt payments` moves its tasks, and `lym beam create grp` makes it a beam"},
		{name: "TheRootWordNamesRoot", req: request{Slugs: []string{"root"}}, check: func(t *testing.T, w *world, got outcome, _ error) {
			if !w.writeReq.Prime || !got.PrimeRecorded || len(w.writeReq.Slugs) != 0 {
				t.Errorf("adopt request = %+v, prime recorded = %v; want the prime alone", w.writeReq, got.PrimeRecorded)
			}
		}},
		{name: "APlainAdoptOfOtherSubjectsUnderThisMachinesHoldRefusesNamingTheHoldsSubjects", req: request{Slugs: []string{"beta"}}, setup: func(w *world) {
			w.writeErr = &boardengine.HoldRefusal{Reason: "this machine's hold covers alpha, not the subjects adopted now", WayForward: "run `lym adopt alpha` first"}
		}, wantErr: "lym adopt alpha", check: func(t *testing.T, w *world, _ outcome, _ error) {
			if !slices.Equal(w.order, []string{"writeadopt"}) {
				t.Errorf("calls = %v; want the refused write alone", w.order)
			}
		}},
		{name: "AnotherMachinesHoldRefusesNamingItsMachine", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.writeErr = &boardengine.HoldRefusal{Reason: `machine "there" holds the hub for an adopt`, Machine: there, WayForward: "wait for that machine's adopt to finish, and if that machine has stopped, ask the operator"}
		}, wantErr: `machine "there"`, check: func(t *testing.T, w *world, _ outcome, err error) {
			if strings.Contains(err.Error(), "take-hold") || !slices.Equal(w.order, []string{"writeadopt"}) {
				t.Errorf("error = %q, calls = %v; want no take-hold flag read and nothing after the write", err, w.order)
			}
		}},
		{name: "UnreachableOriginRefusesTheWriteNamingTheRerun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.writeErr = fmt.Errorf("board: %w", boardengine.ErrOwnerOriginUnreachable)
		}, wantErr: "re-run `lym adopt alpha` once origin answers"},
		{name: "RefusedWriteNamesTheRerunAndRunsNothingMore", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.writeErr = errors.New("board: the push was refused")
		}, wantErr: "re-run `lym adopt alpha`", check: func(t *testing.T, w *world, _ outcome, _ error) {
			if !slices.Equal(w.order, []string{"writeadopt"}) {
				t.Errorf("calls = %v; want the refused write alone", w.order)
			}
		}},
		{name: "RefusedAddOfAnAdoptedBeamsPairEndsNamingTheRerun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) {
			w.pairs["beta"] = false
			w.addErr["beta"] = errors.New("worktree directory already exists")
		}, wantErr: "worktree directory already exists; re-run `lym adopt alpha beta`", check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.calledWithPrefix("takeover:") || w.called("catchup") {
				t.Errorf("calls = %v; want nothing after the refused add", w.order)
			}
		}},
		{name: "LeftoverBranchRefusalNamesTheArchiveTagBeforeBranchDelete", req: request{Slugs: []string{"beta"}}, setup: func(w *world) {
			w.pairs["beta"] = false
			w.addErr["beta"] = &fabricengine.ErrBranchExists{Branch: "task/beta"}
		}, wantErr: "git branch -D task/beta", check: func(t *testing.T, _ *world, _ outcome, err error) {
			message := err.Error()
			tag, remove := strings.Index(message, "git tag archive/beta/"), strings.Index(message, "git branch -D task/beta")
			if tag < 0 || remove < tag || !strings.Contains(message, "lym adopt beta") || strings.Contains(message, "lym fabric checkout") {
				t.Errorf("leftover refusal = %q, want the archive tag before git branch -D, the re-run and no fabric checkout", message)
			}
		}},
		{name: "AnAdoptOnAMachineWithNoLocalPairAddsThePairBeforeTheTakeover", req: request{Slugs: []string{"beta"}}, setup: func(w *world) { w.pairs["beta"] = false }, check: func(t *testing.T, w *world, got outcome, _ error) {
			add, take := slices.Index(w.order, "addpair:beta"), slices.Index(w.order, "takeover:beta")
			if add < 0 || take < add || !slugOf(t, got, "beta").PairAdded || !w.pairs["beta"] {
				t.Errorf("calls = %v, beta = %+v; want the pair added once before the takeover", w.order, slugOf(t, got, "beta"))
			}
			if count := len(slices.DeleteFunc(slices.Clone(w.order), func(call string) bool { return call != "addpair:beta" })); count != 1 {
				t.Errorf("addpair:beta ran %d times; want once", count)
			}
		}},
		{name: "AllNamesRootAndEveryOpenBeamProjectAndNoTaskOrGroupingProject", req: request{All: true}, setup: func(w *world) {
			w.owners.Entries["base"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			w.owners.Entries["ownerless"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			w.owners.Entries["landed"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there, Done: true}
			w.owners.Entries["grp"] = boardengine.OwnerEntry{IsProject: true}
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if !slices.Equal(w.writeReq.Slugs, []string{"alpha", "base", "beta", "ownerless"}) || !w.writeReq.Prime {
				t.Errorf("adopt request = %+v, want root and the open beams alpha, base, beta, ownerless, passing over the idle and done tasks, the done beam and the grouping project", w.writeReq)
			}
		}},
		{name: "BeamMovesItsMembersRunAndRecordsItsOwnOwnerAlone", req: request{Slugs: []string{"payments"}}, setup: withBeam, check: func(t *testing.T, w *world, got outcome, _ error) {
			if w.takenFor["t1"] != "payments" || !slices.Equal(w.writeReq.Slugs, []string{"payments"}) || w.writeReq.Prime {
				t.Errorf("taken for = %v, adopt request = %+v; want t1 moved with payments and the beam's owner alone", w.takenFor, w.writeReq)
			}
			if t1 := slugOf(t, got, "t1"); t1.Run != takenOverRun || t1.OwnerRecorded {
				t.Errorf("t1 = %+v, want its run taken over with no owner of its own", t1)
			}
		}},
		{name: "RootMovesARootTasksRun", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["kid1"] = boardengine.OwnerEntry{IsTask: true, RunHeld: true}
			w.local["kid1"], w.origin["kid1"] = running, running
			w.pairs["kid1"] = true
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.takenFor["kid1"] != boardengine.RootSubject {
				t.Errorf("taken for = %v; want kid1 moved with root", w.takenFor)
			}
		}},
		{name: "NestedBeamOwnedElsewhereUnderAdoptedRootIsReportedOwnedElsewhere", req: request{Root: true}, setup: func(w *world) {
			withoutAlphaAndBeta(w)
			w.owners.Prime = there
			w.owners.Entries["nested"] = boardengine.OwnerEntry{Owner: "third", IsProject: true, Beam: true, RunHeld: true}
			w.local["nested"], w.origin["nested"] = running, running
		}, check: func(t *testing.T, w *world, got outcome, _ error) {
			nested := slugOf(t, got, "nested")
			if nested.Run != ownedElsewhereRun || nested.OwnedBy != "third" || nested.Error != "" || nested.PairAdded || w.called("takeover:nested") {
				t.Errorf("nested = %+v, calls = %v, want its run reported owned elsewhere by third, with no error, pair or takeover", nested, w.order)
			}
		}},
		{name: "ARunWhosePriorOwnerIsThisMachineIsNotMoved", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.hold = holdOver(here, "alpha") }, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.called("takeover:alpha") || !w.called("catchup") {
				t.Errorf("calls = %v; want no takeover of a run whose prior owner is this machine, and the catch-up", w.order)
			}
		}},
		{name: "ARunWhoseOriginRecordNamesAnotherMachinesTakeoverIsMoved", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.hold = holdOver(here, "alpha")
			tookOver(w, "alpha", "third")
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if !w.called("takeover:alpha") {
				t.Errorf("calls = %v; want alpha moved by the other machine's takeover entry", w.order)
			}
		}},
		{name: "ARunWhoseOriginRecordNamesThisMachinesTakeoverIsNotMoved", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { tookOver(w, "alpha", here) }, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.called("takeover:alpha") {
				t.Errorf("calls = %v; want no takeover of a run an earlier pass took", w.order)
			}
		}},
		{name: "AReRunAfterAKillAtTheOwnerWriteMovesTheSameRunsAndEndsAsAnUninterruptedAdopt", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) {
			// The first pass wrote the hold and the owners, then died: the hold names each subject's prior owner.
			w.hold = holdOver(there, "alpha", "beta")
			for _, slug := range []string{"alpha", "beta"} {
				w.owners.Entries[slug] = boardengine.OwnerEntry{Owner: here, IsProject: true, Beam: true, RunHeld: true}
			}
			w.owners.Holds = []boardengine.Hold{*w.hold}
		}, check: func(t *testing.T, w *world, got outcome, _ error) {
			fresh := newWorld()
			freshGot, freshErr := runForeground(fresh.deps(), request{Slugs: []string{"alpha", "beta"}}, io.Discard)
			if freshErr != nil || !slices.Equal(w.calls(), fresh.calls()) || !reflect.DeepEqual(got.Slugs, freshGot.Slugs) {
				t.Errorf("re-run calls = %v, slugs = %+v; want the uninterrupted adopt's %v, %+v (error %v)", w.calls(), got.Slugs, fresh.calls(), freshGot.Slugs, freshErr)
			}
		}},
		{name: "AReRunUnderItsOwnAdoptFixHoldKeepsKindTaskTriesAndFilesNoSecondTask", req: request{Slugs: []string{"alpha"}}, wantErr: "fix1", setup: func(w *world) {
			withFixTask(w)
			w.hold = fixHold(here)
			tookOver(w, "alpha", here)
			w.check = failedCheck
		}, check: func(t *testing.T, w *world, got outcome, _ error) {
			if w.calledWithPrefix("takeover:") || !w.called("catchup") {
				t.Errorf("calls = %v; want no takeover of the fix task's run and the catch-up", w.order)
			}
			if got.Hold == nil || got.Hold.Kind != boardengine.HoldKindAdoptFix || got.Hold.FixTask != "fix1" || got.Hold.Subjects[0].Tries != 1 || got.FixTask != "fix1" {
				t.Errorf("hold = %+v, fix task = %q; want the adopt-fix hold kept with its task and tries", got.Hold, got.FixTask)
			}
		}},
		{name: "TakeHoldRunsEveryStepUnderTheRewrittenHold", req: request{Slugs: []string{"alpha"}, TakeHold: true}, setup: func(w *world) { w.hold = holdOver(there, "alpha") }, check: func(t *testing.T, w *world, _ outcome, _ error) {
			want := []string{"writeadopt", "takeover:alpha", "catchup", "workspace", "check:alpha", "beamup:alpha"}
			if !w.writeReq.TakeHold || !slices.Equal(w.calls(), want) {
				t.Errorf("take hold = %v, calls = %v; want %v", w.writeReq.TakeHold, w.calls(), want)
			}
		}},
		{name: "TakeHoldOfAnAdoptFixHoldMovesTheFixTasksRun", req: request{Slugs: []string{"alpha"}, TakeHold: true}, setup: func(w *world) {
			withFixTask(w)
			w.pairs["fix1"] = false
			w.hold = fixHold(there)
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			add, take := slices.Index(w.order, "addpair:fix1"), slices.Index(w.order, "takeover:fix1")
			if add < 0 || take < add {
				t.Errorf("calls = %v; want the fix task's pair added and its run taken over", w.order)
			}
		}},
		{name: "TakeHoldBackFromTheMachineThatTookTheFixTasksRunMovesItAgain", req: request{Slugs: []string{"alpha"}, TakeHold: true}, setup: func(w *world) {
			withFixTask(w)
			w.hold = fixHold(here)
			tookOver(w, "alpha", there)
			tookOver(w, "fix1", there)
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.called("addpair:fix1") || !w.called("takeover:fix1") {
				t.Errorf("calls = %v; want no pair added and the fix task's run taken over again", w.order)
			}
		}},
		{name: "AReRunAfterAKillFollowingATakeHoldMovesTheFixTasksRun", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			withFixTask(w)
			w.hold = fixHold(there)
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if !w.called("takeover:fix1") {
				t.Errorf("calls = %v; want the fix task's run taken over", w.order)
			}
		}},
		{name: "AFailingTakeOverEndsNamingTheRunAndTheHoldsRerun", req: request{Slugs: []string{"alpha", "beta"}}, wantErr: "the takeover of run alpha failed: push broke; re-run `lym adopt alpha beta`", setup: func(w *world) {
			w.takeErr["alpha"] = errors.New("push broke")
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if w.called("catchup") || w.called("takeover:beta") {
				t.Errorf("calls = %v; want nothing after the failed takeover", w.order)
			}
		}},
		{name: "AHoldTakenOverByAnotherMachineStopsAtTheNextReReadNamingItAndStartsNoDobby", req: request{Slugs: []string{"alpha"}}, wantErr: "machine there holds the hub now", setup: func(w *world) {
			w.onPull = func(w *world, pull int) {
				if pull == 3 {
					w.owners.Holds = []boardengine.Hold{{Kind: boardengine.HoldKindAdopt, Machine: there, Subjects: []boardengine.HoldSubject{{Subject: "alpha"}}}}
				}
			}
		}, check: func(t *testing.T, w *world, _ outcome, err error) {
			if !strings.Contains(err.Error(), "nothing more runs here") || w.called("catchup") || len(w.beamUps) != 0 {
				t.Errorf("error = %q, calls = %v; want the stop before the catch-up, saying nothing more runs here, and no dobby", err, w.order)
			}
		}},
		{name: "AHoldClearedMidAdoptStopsAndStartsNothing", req: request{Slugs: []string{"alpha"}}, wantErr: "it cleared meanwhile", setup: func(w *world) {
			w.onPull = func(w *world, pull int) {
				if pull == 3 {
					w.owners.Holds = nil
				}
			}
		}, check: func(t *testing.T, w *world, _ outcome, err error) {
			if !strings.Contains(err.Error(), "nothing more runs here") || w.called("catchup") || len(w.beamUps) != 0 {
				t.Errorf("error = %q, calls = %v; want the stop before the catch-up and no dobby", err, w.order)
			}
		}},
		{name: "AHoldTakenOverDuringTheCheckStartsNoDobby", req: request{Slugs: []string{"alpha"}}, wantErr: "machine there took it", setup: func(w *world) {
			w.check = holdCheckResult{State: holdStateLost, HoldLost: true, Ran: true, Lines: []string{"the hold of this machine no longer stands: machine there took it, so nothing was brought up here"}}
		}, check: func(t *testing.T, w *world, _ outcome, err error) {
			if !strings.Contains(err.Error(), "nothing more runs here") || len(w.beamUps) != 0 {
				t.Errorf("error = %q, beam steps run for %v; want the stop naming the machine and no dobby", err, w.beamUps)
			}
		}},
		{name: "AFailingCatchUpEndsBeforeTheDeployNamingTheRerun", req: request{Slugs: []string{"alpha"}, Root: true}, wantErr: "origin broke; re-run `lym adopt alpha root`", setup: func(w *world) {
			w.catchUpErr = errors.New("origin broke")
			w.deploy = staleBuild
		}, check: func(t *testing.T, w *world, got outcome, err error) {
			requireNoDeploy(t, w, got, err)
			if w.calledWithPrefix("check:") {
				t.Errorf("calls = %v; want no check after the failed catch-up", w.order)
			}
		}},
		{name: "AnUnreachableOriginAtTheCatchUpNamesTheRerunOnceItAnswers", req: request{Slugs: []string{"alpha"}}, wantErr: "once origin answers", setup: func(w *world) {
			w.catchUpErr = fmt.Errorf("catch-up: %w", dobbyengine.ErrOriginUnreachable)
		}},
		{name: "AFailingWorkspaceIsAWarningAndTheAdoptGoesOn", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) { w.workspaceErr = errors.New("workspace locked") }, check: func(t *testing.T, w *world, got outcome, _ error) {
			if !slices.ContainsFunc(got.Warnings, func(line string) bool {
				return strings.Contains(line, "workspace locked") && strings.Contains(line, "lym fabric reconcile")
			}) || !w.called("check:alpha") {
				t.Errorf("warnings = %q, calls = %v; want a warning naming lym fabric reconcile and the check run", got.Warnings, w.order)
			}
		}},
		{name: "AGreenCheckBringsEachSubjectUpAndReturnsOk", req: request{Slugs: []string{"alpha", "beta"}, Root: true}, check: func(t *testing.T, w *world, got outcome, err error) {
			if err != nil || !slices.Equal(w.beamUps, []string{"prime", "alpha", "beta"}) || !w.called("check:root") {
				t.Errorf("error = %v, beam steps run for %v, calls = %v; want ok, the prime then each beam brought up after root's check", err, w.beamUps, w.order)
			}
			if got.Hold != nil || len(w.owners.Holds) != 0 || got.PrimeCheck != checkGreen || slugOf(t, got, "alpha").Check != checkGreen {
				t.Errorf("hold = %+v, prime check = %q, alpha = %+v; want the hold cleared and every subject's check green", got.Hold, got.PrimeCheck, slugOf(t, got, "alpha"))
			}
		}},
		{name: "AFailingCheckBringsEachSubjectUpThenExitsNonZeroNamingTheFixTask", req: request{Slugs: []string{"alpha", "beta"}}, wantErr: "fix task fix1 stands", setup: func(w *world) { w.check = failedCheck }, check: func(t *testing.T, w *world, got outcome, _ error) {
			if !slices.Equal(w.beamUps, []string{"alpha", "beta"}) || got.FixTask != "fix1" || got.Hold == nil || slugOf(t, got, "alpha").Check != checkFailed {
				t.Errorf("beam steps run for %v, fix task = %q, hold = %+v; want both beams up, the fix task named and the hold standing", w.beamUps, got.FixTask, got.Hold)
			}
		}},
		{name: "ACheckThatDidNotRunEndsNonZeroAndStartsNoDobby", req: request{Slugs: []string{"alpha"}}, wantErr: "wait for it", setup: func(w *world) {
			w.check = holdCheckResult{State: holdStateAlreadyRunning, Lines: []string{"the adopt check of alpha is already running; way forward: wait for it"}}
		}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if len(w.beamUps) != 0 {
				t.Errorf("beam steps run for %v; want none while the check runs elsewhere", w.beamUps)
			}
		}},
		{name: "StepsPrintInOrder", req: request{Slugs: []string{"alpha", "beta"}, Root: true}, setup: func(w *world) {
			w.owners.Prime = there
			w.deploy = staleBuild
		}, check: func(t *testing.T, w *world, got outcome, _ error) {
			want := []string{"writeadopt", "takeover:alpha", "takeover:beta", "catchup", "deploy", "workspace", "check:root", "beamup:prime", "beamup:alpha", "beamup:beta"}
			if !slices.Equal(w.calls(), want) {
				t.Errorf("calls = %v; want %v", w.calls(), want)
			}
			if got.Deploy == nil || !got.PrimeRecorded {
				t.Errorf("deploy = %+v, prime recorded = %v; want the deploy named and the prime recorded", got.Deploy, got.PrimeRecorded)
			}
		}},
		{name: "NoOrchLeavesTheOrchDown", req: request{Slugs: []string{"alpha", "beta"}, Root: true, NoOrch: true}, check: func(t *testing.T, w *world, _ outcome, _ error) {
			if want := []bool{true, true, true}; !slices.Equal(w.noOrch, want) || !w.checkNoOrch {
				t.Errorf("noOrch arguments = %v, check noOrch = %v; want true for the prime, both beams and the check", w.noOrch, w.checkNoOrch)
			}
		}},
		{name: "HeldBeamHasItsOwnerWrittenNoBeamStepsAndNamesTheBattenStatus", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.held["alpha"] = true }, check: func(t *testing.T, w *world, got outcome, _ error) {
			alpha := slugOf(t, got, "alpha")
			if !alpha.OwnerRecorded || alpha.BeamSteps != nil || !strings.Contains(alpha.WayForward, "lym batten status alpha") || strings.Contains(alpha.WayForward, "lym adopt") {
				t.Errorf("alpha = %+v; want its owner written, no beam steps and `lym batten status alpha` alone", alpha)
			}
			if !strings.Contains(alpha.WayForward, "`lym loom resume` in its pair") || strings.Contains(alpha.WayForward, "lym tender resume") {
				t.Errorf("alpha way forward = %q; want `lym loom resume` in its pair alone, since no pause stands", alpha.WayForward)
			}
			if !slices.Equal(w.beamUps, []string{"beta"}) {
				t.Errorf("beam steps run for %v; want beta alone", w.beamUps)
			}
		}},
		{name: "HeldBeamWhoseParentAnotherMachineOwnsAlsoNamesTheParentsAdopt", req: request{Slugs: []string{"alpha"}}, setup: func(w *world) {
			w.held["alpha"] = true
			w.owners.Prime = there
		}, check: func(t *testing.T, w *world, got outcome, _ error) {
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
		{name: "UnreadableLandingHoldWarnsNamingBeamStartAndRunsNoStepsForThatBeam", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.heldErr["alpha"] = errors.New("board unreadable") }, check: func(t *testing.T, w *world, got outcome, _ error) {
			alpha := slugOf(t, got, "alpha")
			if !strings.Contains(alpha.Warning, "board unreadable") || !strings.Contains(alpha.Warning, "lym beam start alpha") || !slices.Equal(w.beamUps, []string{"beta"}) {
				t.Errorf("alpha warning = %q, beam steps run for %v; want the cause and `lym beam start alpha`, and beta's steps alone", alpha.Warning, w.beamUps)
			}
		}},
		{name: "FailingBeamStepWarnsNamingBeamStartWhileTheOtherSlugsStepsRun", req: request{Slugs: []string{"alpha", "beta"}}, setup: func(w *world) { w.beamUpErr["alpha"] = errors.New("alpha: spawn failed") }, check: func(t *testing.T, w *world, got outcome, _ error) {
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
		}, check: func(t *testing.T, _ *world, got outcome, _ error) {
			if !strings.Contains(got.PrimeBeamWarning, "spawn failed") || !strings.Contains(got.PrimeBeamWarning, "lym beam start root") || !got.PrimeRecorded {
				t.Errorf("prime warning = %q, recorded = %v; want the cause and `lym beam start root` beside the recorded prime", got.PrimeBeamWarning, got.PrimeRecorded)
			}
		}},
		{name: "RootDeploysALymOlderThanThePrimesHeadAfterTheAdoptWrite", req: request{Root: true}, setup: func(w *world) { w.deploy = staleBuild }, check: func(t *testing.T, w *world, got outcome, _ error) {
			if !slices.Equal(w.deployed, []string{staleBuild.Command}) {
				t.Errorf("deployed %v; want %q once", w.deployed, staleBuild.Command)
			}
			if deploy, write := slices.Index(w.order, "deploy"), slices.Index(w.order, "writeadopt"); write < 0 || deploy < write {
				t.Errorf("calls = %v; want the adopt write before the deploy", w.order)
			}
			want := deployOutcome{Installed: staleBuild.Installed, Head: staleBuild.Head, Log: deployLogName}
			if got.Deploy == nil || *got.Deploy != want {
				t.Errorf("deploy = %+v; want %+v", got.Deploy, want)
			}
		}},
		{name: "AFailedDeployEndsWithTheHoldStandingAndNoCheckOrBeamStep", req: request{Slugs: []string{"alpha"}, Root: true}, wantErr: "the landing deploy failed: exit code 1; log: deploy.log", setup: func(w *world) {
			w.deploy = staleBuild
			w.deployErr = errors.New("exit code 1")
		}, check: func(t *testing.T, w *world, got outcome, err error) {
			if !strings.Contains(err.Error(), "the hold stands") || !strings.Contains(err.Error(), "lym adopt alpha root") {
				t.Errorf("error = %q; want the hold standing and the re-run named", err)
			}
			if w.called("workspace") || w.calledWithPrefix("check:") || len(w.beamUps) != 0 || got.Hold == nil || got.Deploy == nil {
				t.Errorf("calls = %v, hold = %+v, deploy = %+v; want no step after the failed deploy and the hold standing", w.order, got.Hold, got.Deploy)
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
		{name: "AnUnreadableDeployStateIsAWarningAndTheAdoptGoesOn", req: request{Root: true}, setup: func(w *world) { w.deployStateErr = errors.New("the installed lym carries no VCS stamp") }, check: func(t *testing.T, w *world, got outcome, err error) {
			requireNoDeploy(t, w, got, err)
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
			var steps strings.Builder
			got, err := runForeground(w.deps(), tt.req, &steps)
			switch {
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("runForeground error = %v, want it to contain %q", err, tt.wantErr)
			case tt.wantErr == "" && err != nil:
				t.Fatalf("runForeground error = %v, want none", err)
			}
			if tt.name == "StepsPrintInOrder" {
				want := "adopt: hold done\nadopt: pairs done\nadopt: takeover done\nadopt: catchup done\nadopt: deploy done\nadopt: workspace done\nadopt: check done\nadopt: beam done\n"
				if steps.String() != want {
					t.Errorf("steps printed %q; want %q", steps.String(), want)
				}
			}
			if tt.check != nil {
				tt.check(t, w, got, err)
			}
		})
	}
}
