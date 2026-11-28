// worker_test.go drives the worker's own behavior over the fake world:
// the lock, the record, the resume and the merged envelope.

package adoptcli

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunWorker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alphaRequest := request{Slugs: []string{"alpha"}}

	// seed writes a record of an adopt of alpha whose worker is gone, with the steps up to last ended and an empty part each.
	seed := func(t *testing.T, w *world, last string, edit func(*adoptrecord.Record)) {
		t.Helper()
		record := adoptrecord.Record{
			Request:    adoptrecord.Request(alphaRequest),
			Subjects:   []string{"alpha"},
			PID:        workerPID + 1,
			StartTicks: workerTicks,
			Start:      workerTime.Add(-time.Hour),
			State:      adoptrecord.StateRunning,
			Inputs:     adoptrecord.Inputs{Adoptable: []string{"alpha"}, OwnerStood: []string{"alpha"}},
		}
		index := slices.Index(adoptrecord.Steps, last)
		for _, step := range adoptrecord.Steps[:index+1] {
			record.Steps = append(record.Steps, adoptrecord.StepRecord{Name: step, EndedAt: workerTime, Outcome: json.RawMessage("{}")})
		}
		record.InFlight = adoptrecord.Steps[index+1]
		if edit != nil {
			edit(&record)
		}
		if err := adoptrecord.Write(w.dir, record); err != nil {
			t.Fatalf("Write(adopt record) = %v", err)
		}
	}

	requireErr := func(t *testing.T, err error, parts ...string) {
		t.Helper()
		if err == nil {
			t.Fatalf("runWorker error = nil, want one containing %q", parts)
		}
		for _, part := range parts {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("runWorker error = %q, want it to contain %q", err, part)
			}
		}
	}

	requireRecordUnchanged := func(t *testing.T, w *world, before adoptrecord.Record) {
		t.Helper()
		if after := recordOf(t, w); !reflect.DeepEqual(after, before) {
			t.Errorf("record = %+v, want it unchanged from %+v", after, before)
		}
	}

	ranStep := func(w *world, prefixes ...string) bool {
		return slices.ContainsFunc(w.order, func(step string) bool {
			return slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(step, prefix) })
		})
	}

	staleBuild := deployState{Command: "go run ./tools/deploy", Installed: "87553450ec14", Head: "2d319fe5", Behind: true}

	tests := []struct {
		name string
		run  func(t *testing.T, w *world)
	}{
		{"FreshRunWritesTheRunningRecordAtReconcileBeforeAnyStep", func(t *testing.T, w *world) {
			deps := w.workerDeps()
			inner := deps.WriteRecord
			var first *adoptrecord.Record
			stepsAtFirstSave := -1
			deps.WriteRecord = func(record adoptrecord.Record) error {
				if first == nil {
					first, stepsAtFirstSave = &record, len(w.order)
				}
				return inner(record)
			}
			if _, _, err := runWorker(deps, alphaRequest); err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			if first == nil || stepsAtFirstSave != 0 || first.State != adoptrecord.StateRunning || first.InFlight != adoptrecord.StepReconcile ||
				!first.Request.SameSubjects(alphaRequest.recorded()) || !slices.Equal(first.Subjects, []string{"alpha"}) || first.PID != workerPID || first.StartTicks != workerTicks {
				t.Errorf("first save = %+v after %d steps, want the running record at reconcile with the request and subjects before any step", first, stepsAtFirstSave)
			}
		}},
		{"WorkerPassesEveryPauseFromBeforeItsFirstStepUntilItReturns", func(t *testing.T, w *world) {
			deps := w.workerDeps()
			restored := false
			deps.ExemptPause = func() func() {
				w.order = append(w.order, "exempt")
				return func() { restored = true }
			}
			if _, _, err := runWorker(deps, alphaRequest); err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			if len(w.order) < 2 || w.order[0] != "exempt" || w.order[1] != "reconcile" || !restored {
				t.Errorf("steps = %v, restored %v; want the exemption before the reconcile and restored at the end", w.order, restored)
			}
		}},
		{"HeldBackRecordWhoseStopNoLongerHoldsIsJudgedAgainAndEndsDone", func(t *testing.T, w *world) {
			w.live["alpha"] = true
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err == nil {
				t.Fatalf("first runWorker error = nil, want the held-back stop")
			}
			w.live["alpha"] = false

			_, record, err := runWorker(w.workerDeps(), alphaRequest)
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			reconciles := 0
			for _, step := range w.order {
				if step == "reconcile" {
					reconciles++
				}
			}
			if record.State != adoptrecord.StateDone || len(record.Inputs.HeldBack) != 0 || !slices.Equal(record.Inputs.OwnerStood, []string{"alpha"}) || reconciles != 2 {
				t.Errorf("record = state %q, held back %v, owner stood %v after %d reconciles; want done with alpha's owner written after a second reconcile",
					record.State, record.Inputs.HeldBack, record.Inputs.OwnerStood, reconciles)
			}
		}},
		{"HeldBackRecordWhoseStopStillHoldsStopsAgainWithTheSameWayForward", func(t *testing.T, w *world) {
			w.live["alpha"] = true
			_, _, firstErr := runWorker(w.workerDeps(), alphaRequest)
			if firstErr == nil {
				t.Fatalf("first runWorker error = nil, want the held-back stop")
			}
			first := recordOf(t, w)

			_, record, err := runWorker(w.workerDeps(), alphaRequest)
			if err == nil || err.Error() != firstErr.Error() {
				t.Fatalf("second runWorker error = %v, want the same stop as %v", err, firstErr)
			}
			reconciles := 0
			for _, step := range w.order {
				if step == "reconcile" {
					reconciles++
				}
			}
			if record.State != adoptrecord.StateStopped || !slices.Equal(record.Inputs.HeldBack, first.Inputs.HeldBack) || record.WayForward != first.WayForward || reconciles != 2 {
				t.Errorf("record = state %q, held back %v, way forward %q after %d reconciles; want it stopped again as before after a second reconcile",
					record.State, record.Inputs.HeldBack, record.WayForward, reconciles)
			}
		}},
		{"HeldBackRecordRejudgedKeepsItsPausedSlugs", func(t *testing.T, w *world) {
			w.live["alpha"] = true
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err == nil {
				t.Fatalf("first runWorker error = nil, want the held-back stop")
			}
			stopped := recordOf(t, w)
			stopped.Inputs.Paused = []string{"alpha"}
			if err := adoptrecord.Write(w.dir, stopped); err != nil {
				t.Fatalf("Write(adopt record) = %v", err)
			}
			w.live["alpha"] = false

			_, record, err := runWorker(w.workerDeps(), alphaRequest)
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			if !slices.Equal(record.Inputs.Paused, []string{"alpha"}) || len(w.record.Live) != 0 {
				t.Errorf("paused slugs = %v, live pause entries = %+v; want alpha kept as paused and not paused again", record.Inputs.Paused, w.record.Live)
			}
		}},
		{"RecordThroughOwnerResumesAtDeployAndEndsDone", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepOwner, nil)
			_, record, err := runWorker(w.workerDeps(), alphaRequest)
			if err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			if want := []string{"beamup:alpha", "suite:alpha"}; !slices.Equal(w.order, want) || record.State != adoptrecord.StateDone {
				t.Errorf("steps = %v, record state %q; want %v after the owner step and a done record", w.order, record.State, want)
			}
		}},
		{"RecordThroughPairsRunsTheReconcileAgainAndListsEachStepOnce", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepPairs, nil)
			_, record, err := runWorker(w.workerDeps(), alphaRequest)
			if err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			var listed []string
			for _, step := range record.Steps {
				listed = append(listed, step.Name)
			}
			if want := []string{"reconcile", "pause", "owners", "beamup:alpha", "suite:alpha"}; !slices.Equal(w.order, want) || !slices.Equal(listed, adoptrecord.Steps) {
				t.Errorf("steps = %v, listed = %v; want %v and each of %v once", w.order, listed, want, adoptrecord.Steps)
			}
		}},
		{"PausedSlugWhoseEntryTheOperatorLiftedIsNotPausedAgain", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepPairs, func(record *adoptrecord.Record) { record.Inputs.Paused = []string{"alpha"} })
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			if len(w.record.Live) != 0 {
				t.Errorf("live entries = %+v, want none for a slug the record lists as paused", w.record.Live)
			}
		}},
		{"HeldLockReturnsAtOnceAndLeavesTheRecord", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepOwner, nil)
			before := recordOf(t, w)
			release, acquired, err := tryAdoptLock(w.dir)
			if err != nil || !acquired {
				t.Fatalf("tryAdoptLock = %v, %v; want the lock", acquired, err)
			}
			t.Cleanup(func() { _ = release() })

			_, _, err = runWorker(w.workerDeps(), alphaRequest)
			requireErr(t, err, "lym beam status")
			requireRecordUnchanged(t, w, before)
			if len(w.order) != 0 {
				t.Errorf("steps = %v, want none under a held lock", w.order)
			}
		}},
		{"OtherSubjectsAreRefusedNamingTheRecordedAdopt", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepOwner, func(record *adoptrecord.Record) {
				record.Request = adoptrecord.Request{Slugs: []string{"beta"}}
				record.Subjects = []string{"beta"}
			})
			before := recordOf(t, w)
			_, _, err := runWorker(w.workerDeps(), alphaRequest)
			requireErr(t, err, "beta", adoptrecord.StepDeploy, "`lym adopt beta`", "lym adopt --discard")
			requireRecordUnchanged(t, w, before)
			if len(w.order) != 0 {
				t.Errorf("steps = %v, want none for other subjects", w.order)
			}
		}},
		{"RunningRecordOfThisProcessIsRefusedAsAFollow", func(t *testing.T, w *world) {
			ticks, _ := proc.StartTime(os.Getpid())
			seed(t, w, adoptrecord.StepOwner, func(record *adoptrecord.Record) { record.PID, record.StartTicks = os.Getpid(), ticks })
			before := recordOf(t, w)
			deps := w.workerDeps()
			deps.Alive = adoptrecord.WorkerAlive
			_, _, err := runWorker(deps, alphaRequest)
			requireErr(t, err, "lym beam status", "lym adopt --discard")
			requireRecordUnchanged(t, w, before)
			if len(w.order) != 0 {
				t.Errorf("steps = %v, want none for a record whose worker runs", w.order)
			}
		}},
		{"StandingMoveRecordRefusesTheResumeAtDeploy", func(t *testing.T, w *world) {
			w.moving["alpha"] = "sub"
			seed(t, w, adoptrecord.StepOwner, nil)
			_, _, err := runWorker(w.workerDeps(), alphaRequest)
			requireErr(t, err, "lym fabric anchor alpha --set sub")
			if record := recordOf(t, w); ranStep(w, "deploy") || record.State != adoptrecord.StateStopped || record.InFlight != adoptrecord.StepDeploy {
				t.Errorf("steps = %v, record = state %q, in flight %q; want no deploy and the record stopped at deploy", w.order, record.State, record.InFlight)
			}
		}},
		{"InvalidRequestIsRefusedBeforeTheLockAndAnyStep", func(t *testing.T, w *world) {
			release, acquired, err := tryAdoptLock(w.dir)
			if err != nil || !acquired {
				t.Fatalf("tryAdoptLock = %v, %v; want the lock", acquired, err)
			}
			t.Cleanup(func() { _ = release() })

			_, _, err = runWorker(w.workerDeps(), request{Slugs: []string{"alpha"}, KeepLocal: true, DropLocal: true})
			requireErr(t, err, "pass one of them")
			if _, found, readErr := adoptrecord.Read(w.dir); readErr != nil || found || len(w.order) != 0 {
				t.Errorf("record found = %v (%v), steps = %v; want no record and no step", found, readErr, w.order)
			}
		}},
		{"ResumeKeepsTheRecordedNoOrch", func(t *testing.T, w *world) {
			seed(t, w, adoptrecord.StepOwner, func(record *adoptrecord.Record) { record.Request.NoOrch = true })
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err != nil {
				t.Fatalf("runWorker error = %v, want none", err)
			}
			if len(w.noOrch) == 0 || slices.Contains(w.noOrch, false) {
				t.Errorf("noOrch arguments = %v, want every beam step call to keep the orch down", w.noOrch)
			}
		}},
		{"ResumeAddingKeepLocalToAHeldBackRecordRestartsAtReconcileAndEndsDone", func(t *testing.T, w *world) {
			w.live["alpha"] = true
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err == nil {
				t.Fatalf("first runWorker error = nil, want the held-back stop")
			}
			if record := recordOf(t, w); record.State != adoptrecord.StateStopped || len(record.Inputs.HeldBack) == 0 {
				t.Fatalf("record = state %q, held back %v; want it stopped holding alpha back", record.State, record.Inputs.HeldBack)
			}

			_, record, err := runWorker(w.workerDeps(), request{Slugs: []string{"alpha"}, KeepLocal: true})
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			reconciles := 0
			for _, step := range w.order {
				if step == "reconcile" {
					reconciles++
				}
			}
			if record.State != adoptrecord.StateDone || len(record.Inputs.HeldBack) != 0 || !record.Request.KeepLocal || reconciles != 2 {
				t.Errorf("record = state %q, held back %v, request %+v after %d reconciles; want done, nothing held back, --keep-local and a second reconcile",
					record.State, record.Inputs.HeldBack, record.Request, reconciles)
			}
		}},
		{"ResumeAddingDropLocalToAKeepLocalRecordRecordsDropLocalAlone", func(t *testing.T, w *world) {
			w.setErr = errors.New("board: the push was refused")
			if _, _, err := runWorker(w.workerDeps(), request{Slugs: []string{"alpha"}, KeepLocal: true}); err == nil {
				t.Fatalf("first runWorker error = nil, want the owner write refusal")
			}
			w.setErr = nil

			_, record, err := runWorker(w.workerDeps(), request{Slugs: []string{"alpha"}, DropLocal: true})
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			if want := (adoptrecord.Request{Slugs: []string{"alpha"}, DropLocal: true}); !reflect.DeepEqual(record.Request, want) || record.State != adoptrecord.StateDone {
				t.Errorf("record = request %+v, state %q; want %+v and done", record.Request, record.State, want)
			}
		}},
		{"DoneEnvelopeMergesThePartsOfStepsFromTwoProcesses", func(t *testing.T, w *world) {
			w.deploy = staleBuild
			w.deployErr = errors.New("exit code 1")
			w.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "code", Prime: true, Worktree: "/hub/main", State: fabricengine.BranchRewritten}}
			reconcileRecord := fabricengine.NewMutations("/hub")
			reconcileRecord.Append(fabricengine.KindPathRemoved, "/hub/stale", "gone")
			w.report.Mutations = reconcileRecord.Snapshot()
			req := request{Slugs: []string{"alpha"}, Root: true}
			if _, _, err := runWorker(w.workerDeps(), req); err == nil {
				t.Fatalf("first runWorker error = nil, want the failed deploy")
			}
			w.deployErr = nil

			got, record, err := runWorker(w.workerDeps(), req)
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			wantWarning := "main is rewritten against origin and was left unmoved: run lym fabric pull in /hub/main, then re-run lym adopt with the same arguments"
			if alpha := slugOf(t, got, "alpha"); !alpha.OwnerRecorded || !got.PrimeRecorded || got.Deploy == nil || !slices.Equal(got.Warnings, []string{wantWarning}) || record.State != adoptrecord.StateDone {
				t.Errorf("alpha = %+v, prime recorded = %v, deploy = %+v, warnings = %q; want the first process's owner step carried into the done envelope", alpha, got.PrimeRecorded, got.Deploy, got.Warnings)
			}
			if got.Reconcile.Mutations.Len() != 0 || len(got.Mutations) != 1 || !got.Partial {
				t.Errorf("reconcile mutations = %d, mutations = %d, partial = %v; want the record in the top-level field alone", got.Reconcile.Mutations.Len(), len(got.Mutations), got.Partial)
			}
		}},
		{"ResumeRestartingAtReconcileReportsThePauseTheEarlierProcessSet", func(t *testing.T, w *world) {
			w.setErr = errors.New("board: the push was refused")
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err == nil {
				t.Fatalf("first runWorker error = nil, want the owner write refusal")
			}
			w.setErr = nil

			got, _, err := runWorker(w.workerDeps(), alphaRequest)
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			if alpha := slugOf(t, got, "alpha"); !alpha.AdoptPaused || alpha.Run != pausedRun || !strings.Contains(alpha.PauseStanding, "lym tender resume alpha") {
				t.Errorf("alpha = %+v; want the first process's pause carried into the envelope", alpha)
			}
		}},
		{"ResumeOfAnAllAdoptKeepsTheRecordedSubjectsWhenABeamIsMadeMeanwhile", func(t *testing.T, w *world) {
			w.setErr = errors.New("board: the push was refused")
			if _, _, err := runWorker(w.workerDeps(), request{All: true}); err == nil {
				t.Fatalf("first runWorker error = nil, want the owner write refusal")
			}
			w.setErr = nil
			w.owners.Entries["gamma"] = boardengine.OwnerEntry{Owner: there, IsProject: true, Beam: true, RunHeld: true}
			w.pairs["gamma"] = true
			delete(w.owners.Entries, "beta")

			if _, _, err := runWorker(w.workerDeps(), request{All: true}); err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			if !slices.Contains(w.ownerReq.Slugs, "alpha") || slices.Contains(w.ownerReq.Slugs, "gamma") || slices.Contains(w.ownerReq.Slugs, "beta") {
				t.Errorf("owner write = %v; want the recorded beams the board still holds, not gamma made since nor beta gone since", w.ownerReq.Slugs)
			}
		}},
		{"ResumeStoppedBeforeItsStepsRunAgainStillReportsTheEarlierProcessPause", func(t *testing.T, w *world) {
			w.setErr = errors.New("board: the push was refused")
			if _, _, err := runWorker(w.workerDeps(), alphaRequest); err == nil {
				t.Fatalf("first runWorker error = nil, want the owner write refusal")
			}
			w.setErr = nil
			w.reconcileErr = errors.New("origin: the fetch failed")

			got, _, err := runWorker(w.workerDeps(), alphaRequest)
			if err == nil {
				t.Fatalf("second runWorker error = nil, want the reconcile stop")
			}
			if alpha := slugOf(t, got, "alpha"); !alpha.AdoptPaused || !strings.Contains(alpha.PauseStanding, "lym tender resume alpha") {
				t.Errorf("alpha = %+v; want the earlier process's pause kept in the stopped envelope", alpha)
			}
		}},
		{"StoppedStepPartLeavesTheRecordOnceTheStepSucceeds", func(t *testing.T, w *world) {
			w.deploy = staleBuild
			w.deployErr = errors.New("exit code 1")
			req := request{Slugs: []string{"alpha"}, Root: true}
			if _, _, err := runWorker(w.workerDeps(), req); err == nil {
				t.Fatalf("first runWorker error = nil, want the failed deploy")
			}
			if stopped := recordOf(t, w); len(stopped.StoppedPart) == 0 {
				t.Fatalf("stopped part = empty, want the part the failed deploy gathered")
			}
			w.deployErr = nil

			got, record, err := runWorker(w.workerDeps(), req)
			if err != nil {
				t.Fatalf("second runWorker error = %v, want none", err)
			}
			if len(record.StoppedPart) != 0 || got.Deploy == nil || record.State != adoptrecord.StateDone {
				t.Errorf("stopped part = %s, deploy = %+v, state %q; want the part cleared and the deploy held once by its step", record.StoppedPart, got.Deploy, record.State)
			}
		}},
		{"HeldBackStopNamesTheAdoptsSubjectsSoItsOwnWayForwardResumesTheRecord", func(t *testing.T, w *world) {
			w.live["alpha"] = true
			if _, _, err := runWorker(w.workerDeps(), request{Slugs: []string{"alpha", "beta"}}); err == nil {
				t.Fatalf("first runWorker error = nil, want the held-back stop")
			}
			record := recordOf(t, w)
			quoted := strings.Split(record.WayForward, "`")
			if len(quoted) < 3 {
				t.Fatalf("way forward = %q; want the keep-local command in backticks", record.WayForward)
			}
			var called adoptrecord.Request
			for _, word := range strings.Fields(quoted[1])[2:] {
				switch word {
				case "--keep-local":
					called.KeepLocal = true
				case "root":
					called.Root = true
				case "--all":
					called.All = true
				default:
					called.Slugs = append(called.Slugs, word)
				}
			}
			verdict, err := judgeStanding(record, true, record.ReadState(func(int, string) bool { return false }), called)
			if err != nil || verdict != verdictResume || !called.KeepLocal {
				t.Errorf("judgeStanding(%q) = %q, %v; want the record resumed", quoted[1], verdict, err)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld()
			w.dir = t.TempDir()
			tt.run(t, w)
		})
	}
}
