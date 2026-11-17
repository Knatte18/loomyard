// cardfixer_test.go covers the card fixer of the detached runner.
// It checks which requests hand a red verdict to a cold fixer.
// It also checks the runner's fixer phase over a scripted gate tree and a scripted fixer.

package gatecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHandsOff(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cfg := gateslot.Config{Gates: map[string]gateslot.GateKey{"card": {Fixer: "sonnet[medium]"}, "batch": {}}}
	card := []string{"--gate=card", "--card=1"}
	tests := []struct {
		name    string
		request detachRequest
		slug    string
		wantKey string
		want    bool
	}{
		{"CardKeyWithAFixer", detachRequest{Verb: verbTest, Args: card, Role: "implementer"}, "task", "card", true},
		{"RoleOfAnOlderFront", detachRequest{Verb: verbTest, Args: card}, "task", "card", true},
		{"GateFixerRole", detachRequest{Verb: verbTest, Args: card, Role: "gate-fixer"}, "task", "", false},
		{"NumberedGateFixerRole", detachRequest{Verb: verbTest, Args: card, Role: "gate-fixer-2"}, "task", "", false},
		{"VerifyFixerRole", detachRequest{Verb: verbTest, Args: card, Role: "verify-fixer"}, "task", "", false},
		{"NoGateWord", detachRequest{Verb: verbTest, Args: []string{"--card=1"}}, "task", "", false},
		{"KeyWithoutAFixer", detachRequest{Verb: verbTest, Args: []string{"--gate=batch", "--card=1"}}, "task", "", false},
		{"UnknownKey", detachRequest{Verb: verbTest, Args: []string{"--gate=other"}}, "task", "", false},
		{"EmptySlug", detachRequest{Verb: verbTest, Args: card}, "", "", false},
		{"CardVerifyVerb", detachRequest{Verb: verbCardVerify, Args: []string{"1"}}, "task", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, got := handsOff(tt.request, cfg, tt.slug)
			if got != tt.want || key != tt.wantKey {
				t.Errorf("handsOff() = %q, %v; want %q, %v", key, got, tt.wantKey, tt.want)
			}
		})
	}
}

// fakeCardFixer is a scripted gatefixer.Fixer.
// It records the spec it is handed and the phase the run was in at its spawn and at its first wait,
// and it answers each call as its fields say.
type fakeCardFixer struct {
	run detachRun

	spawnErr error
	awaitErr error
	stopErr  error
	// result is the answer of a wait; a zero Status answers StatusRunning.
	result gatefixer.Result
	// beforeAwait, when set, runs at the first wait.
	beforeAwait func()

	mu           sync.Mutex
	specs        []gatefixer.Spec
	phaseAtSpawn string
	phaseAtAwait string
	stopped      []string
	awaited      bool
	awaitedGuids []string
}

func (f *fakeCardFixer) phase() string {
	phase, _ := readRunFile[runPhase](f.run.path(phaseFile))
	return phase.State
}

func (f *fakeCardFixer) SpawnOrAttach(store gatefixer.Store, spec gatefixer.Spec) (*gatefixer.Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, spec)
	f.phaseAtSpawn = f.phase()
	if f.spawnErr != nil {
		return nil, false, f.spawnErr
	}
	record := &gatefixer.Record{StrandGUID: "strand-1", SpawnedAt: "2026-01-01T00:00:00Z", ReportPath: spec.ReportPath, ReplyPath: spec.ReplyPath}
	if err := store.Save(record); err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func (f *fakeCardFixer) Await(rec *gatefixer.Record, _, _ time.Duration) (*gatefixer.Result, error) {
	f.mu.Lock()
	first := !f.awaited
	f.awaited = true
	f.awaitedGuids = append(f.awaitedGuids, rec.StrandGUID)
	if first {
		f.phaseAtAwait = f.phase()
	}
	f.mu.Unlock()
	if first && f.beforeAwait != nil {
		f.beforeAwait()
	}
	if f.awaitErr != nil {
		return nil, f.awaitErr
	}
	result := f.result
	if result.Status == "" {
		// A fixer still running answers after a short tick, so the wait loop does not spin.
		time.Sleep(10 * time.Millisecond)
		result.Status = gatefixer.StatusRunning
	}
	return &result, nil
}

func (f *fakeCardFixer) Stop(rec *gatefixer.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, rec.StrandGUID)
	return f.stopErr
}

// fixerRig is one run directory under a key directory.
// It holds the scripted gate tree, fixer, stopper and tree-identity seam that a run of the fixer phase reaches.
type fixerRig struct {
	t       *testing.T
	run     detachRun
	keyDir  string
	store   keyFixerStore
	fixer   *fakeCardFixer
	stencil string

	// on and kitErr are what the cardFixer seam answers.
	on     bool
	kitErr error
	// codes are the exit codes of the gate calls in order; the last repeats.
	codes []int
	calls atomic.Int32
	// treeAtCall holds the tree record found on disk when each gate call ran.
	treeMu     sync.Mutex
	treeAtCall []treeRecord
	// recordStatusAtCall holds the status of the key directory's fixer record when each gate call ran;
	// empty means live or absent.
	recordStatusAtCall []string
	// silent makes a gate call exit without writing a failure report, as a refusal does.
	silent bool
	// closing is written after the failure report of every gate call, as the closing line a call ends with.
	closing string

	stopperErr   error
	stoppedGuids []string
	treeDirs     []string
}

var identityAfterFix = treeRecord{Tree: "tree-after-fix", Dirty: []string{"fixed.go"}}

func newFixerRig(t *testing.T, codes ...int) *fixerRig {
	t.Helper()
	keyDir := t.TempDir()
	run := detachRun{dir: filepath.Join(keyDir, "run-1")}
	if err := os.Mkdir(run.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLiveLease(t, run)
	return &fixerRig{t: t, run: run, keyDir: keyDir, store: keyFixerStore{dir: keyDir}, fixer: &fakeCardFixer{run: run, result: gatefixer.Result{Status: gatefixer.StatusDone, Reply: "FIXER REPLY"}}, stencil: stencilkit.Seed(t), on: true, codes: codes}
}

func (r *fixerRig) gateTree() *cobra.Command {
	root := &cobra.Command{Use: "gate", Args: cobra.ArbitraryArgs}
	root.AddCommand(&cobra.Command{
		Use:                "test",
		Args:               cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, _ []string) error {
			call := int(r.calls.Add(1))
			tree, _ := readRunFile[treeRecord](r.run.path(treeFile))
			r.treeMu.Lock()
			r.treeAtCall = append(r.treeAtCall, tree)
			record, _ := readRunFile[gatefixer.Record](filepath.Join(r.keyDir, fixerRecordFile))
			r.recordStatusAtCall = append(r.recordStatusAtCall, record.Status)
			r.treeMu.Unlock()
			if !r.silent {
				_, _ = cmd.ErrOrStderr().Write([]byte("failure report " + string(rune('0'+call)) + "\n" + r.closing))
			}
			code := r.codes[min(call, len(r.codes))-1]
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	})
	return root
}

func (r *fixerRig) seams() detachSeams {
	return detachSeams{
		cardFixer: func(detachRequest) (cardFixerKit, bool, error) {
			if r.kitErr != nil || !r.on {
				return cardFixerKit{}, false, r.kitErr
			}
			return cardFixerKit{fixer: r.fixer, timeout: time.Minute, worktreeRoot: "/work", stencilsDir: r.stencil}, true, nil
		},
		fixerStopper: func(detachRequest) (func(string) error, error) {
			return func(guid string) error {
				r.stoppedGuids = append(r.stoppedGuids, guid)
				return r.stopperErr
			}, nil
		},
		treeIdentity: func(dir string) treeRecord {
			r.treeDirs = append(r.treeDirs, dir)
			return identityAfterFix
		},
	}
}

func (r *fixerRig) runRequest(request detachRequest) detachExit {
	r.t.Helper()
	return r.run.runWithFixer(context.Background(), request, r.gateTree, r.seams())
}

func (r *fixerRig) stderr() string {
	r.t.Helper()
	data, err := os.ReadFile(r.run.path(stderrFile))
	if err != nil {
		r.t.Fatalf("read the run's stderr: %v", err)
	}
	return string(data)
}

func (r *fixerRig) storedRecord() gatefixer.Record {
	r.t.Helper()
	record, found := readRunFile[gatefixer.Record](filepath.Join(r.keyDir, fixerRecordFile))
	if !found {
		r.t.Fatal("the key directory holds no fixer record")
	}
	return record
}

func TestRunWithFixer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	request := detachRequest{Verb: verbTest, Args: []string{"--gate=card", "--card=3-core", "--card=4"}, Cwd: "/work", Role: "implementer"}
	liveRecord := &gatefixer.Record{StrandGUID: "old-strand", SpawnedAt: "2026-01-01T00:00:00Z"}

	t.Run("GreenCallSpawnsNone", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 0)
		if exit := rig.runRequest(request); exit != (detachExit{}) {
			t.Errorf("exit = %+v; want a pass", exit)
		}
		if len(rig.fixer.specs) != 0 || rig.calls.Load() != 1 {
			t.Errorf("spawned %d fixers over %d gate calls; want none over one", len(rig.fixer.specs), rig.calls.Load())
		}
	})

	t.Run("RedCallIsFixedAndRerun", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 1, 2)
		exit := rig.runRequest(request)
		if exit != (detachExit{Code: 2}) {
			t.Errorf("exit = %+v; want the re-run's code 2", exit)
		}
		if len(rig.fixer.specs) != 1 || rig.calls.Load() != 2 {
			t.Fatalf("spawned %d fixers over %d gate calls; want one over two", len(rig.fixer.specs), rig.calls.Load())
		}
		if rig.fixer.phaseAtSpawn != phaseFixing {
			t.Errorf("phase at the spawn = %q; want %q", rig.fixer.phaseAtSpawn, phaseFixing)
		}

		spec := rig.fixer.specs[0]
		noParent, err := parentdirective.Directive(rig.stencil, "", false)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"lym gate test --gate=card --card=3-core --card=4", "3-core, 4", rig.run.path(fixerReplyFile), "/work", noParent} {
			if !strings.Contains(string(spec.Prompt), want) {
				t.Errorf("the fixer's prompt lacks %q", want)
			}
		}
		if spec.Role != gatefixer.RoleGateFixer || spec.ReplyPath != rig.run.path(fixerReplyFile) || spec.Timeout != time.Minute {
			t.Errorf("spec = role %q, reply %q, timeout %v; want the gate fixer role, the run's reply file and the kit's timeout", spec.Role, spec.ReplyPath, spec.Timeout)
		}
		report, err := os.ReadFile(rig.run.path(fixerReportFile))
		if err != nil || string(report) != "failure report 1\n" {
			t.Errorf("fixer report = %q, %v; want the first run's stderr", report, err)
		}

		stderr := rig.stderr()
		if !strings.Contains(stderr, "failure report 2") || !strings.Contains(stderr, "## Cold fixer reply\n\nFIXER REPLY") || strings.Contains(stderr, "failure report 1") {
			t.Errorf("stderr = %q; want the re-run's report followed by the fixer's reply", stderr)
		}
		if got := rig.treeAtCall[1]; got.Tree != identityAfterFix.Tree || len(got.Dirty) != 1 || got.Dirty[0] != "fixed.go" {
			t.Errorf("tree record at the re-run = %+v; want the identity the seam answered", got)
		}
		if len(rig.treeDirs) != 1 || rig.treeDirs[0] != "/work" {
			t.Errorf("tree identity read in %v; want the kit's worktree root once", rig.treeDirs)
		}
		if record := rig.storedRecord(); record.Status != gatefixer.StatusDone {
			t.Errorf("fixer record status = %q; want %q", record.Status, gatefixer.StatusDone)
		}
		if len(rig.recordStatusAtCall) != 2 || rig.recordStatusAtCall[1] != "" {
			t.Errorf("fixer record status at each gate call = %q; want it still live at the re-run", rig.recordStatusAtCall)
		}
	})

	t.Run("NonZeroExitWithNoStderrSpawnsNone", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 1)
		rig.silent = true
		if exit := rig.runRequest(request); exit != (detachExit{Code: 1}) {
			t.Errorf("exit = %+v; want the first exit", exit)
		}
		if len(rig.fixer.specs) != 0 || rig.calls.Load() != 1 {
			t.Errorf("spawned %d fixers over %d gate calls; want none over one", len(rig.fixer.specs), rig.calls.Load())
		}
		if stderr := rig.stderr(); stderr != "" {
			t.Errorf("stderr = %q; want it empty, so the guard is what stopped the hand-off", stderr)
		}
	})

	t.Run("RunnerCrashLineSpawnsNone", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 134)
		rig.closing = gateslot.RunnerCrashLine("tier1 (exit 134, SIGABRT)") + "\n"
		if exit := rig.runRequest(request); exit != (detachExit{Code: 134}) {
			t.Errorf("exit = %+v; want the first exit 134", exit)
		}
		if len(rig.fixer.specs) != 0 || rig.calls.Load() != 1 {
			t.Errorf("spawned %d fixers over %d gate calls; want none over one", len(rig.fixer.specs), rig.calls.Load())
		}
		if stderr := rig.stderr(); strings.Contains(stderr, "Cold fixer reply") {
			t.Errorf("stderr = %q; want no fixer reply for a crash", stderr)
		}
	})

	t.Run("DeadFixerCarriesTheDeadNote", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 1, 1)
		rig.fixer.result = gatefixer.Result{Status: gatefixer.StatusDead, DeadReason: gatefixer.DeadReasonAsking}
		if exit := rig.runRequest(request); exit != (detachExit{Code: 1}) {
			t.Errorf("exit = %+v; want the re-run's code 1", exit)
		}
		if stderr := rig.stderr(); !strings.Contains(stderr, "ended without a reply (asking)") || !strings.Contains(stderr, "left uncommitted is in the worktree") {
			t.Errorf("stderr = %q; want the dead note naming the reason and the uncommitted change", stderr)
		}
		if record := rig.storedRecord(); record.Status != gatefixer.StatusDead || record.DeadReason != gatefixer.DeadReasonAsking {
			t.Errorf("fixer record = %q %q; want dead for asking", record.Status, record.DeadReason)
		}
	})

	t.Run("SpawnErrorEndsWithTheFirstVerdict", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 3)
		rig.fixer.spawnErr = errors.New("provider would not start")
		if exit := rig.runRequest(request); exit != (detachExit{Code: 3}) {
			t.Errorf("exit = %+v; want the first verdict's code 3", exit)
		}
		stderr := rig.stderr()
		for _, want := range []string{"failure report 1", "provider would not start", "no cold fixer ran"} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q; want it to hold %q", stderr, want)
			}
		}
		if rig.calls.Load() != 1 {
			t.Errorf("gate calls = %d; want the first run alone", rig.calls.Load())
		}
	})

	for _, tt := range []struct {
		name   string
		on     bool
		kitErr error
	}{
		{"TurnedDownRequestSpawnsNone", false, nil},
		{"CardFixerErrorSpawnsNone", false, errors.New("config unreadable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rig := newFixerRig(t, 1)
			rig.on, rig.kitErr = tt.on, tt.kitErr
			if exit := rig.runRequest(request); exit != (detachExit{Code: 1}) {
				t.Errorf("exit = %+v; want the first verdict", exit)
			}
			if len(rig.fixer.specs) != 0 || rig.calls.Load() != 1 {
				t.Errorf("spawned %d fixers over %d gate calls; want none over one", len(rig.fixer.specs), rig.calls.Load())
			}
		})
	}

	t.Run("LiveRecordIsWaitedOnBeforeTheGateRuns", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 0)
		if err := rig.store.Save(liveRecord); err != nil {
			t.Fatal(err)
		}
		var callsAtWait int32 = -1
		rig.fixer.beforeAwait = func() { callsAtWait = rig.calls.Load() }
		if exit := rig.runRequest(request); exit != (detachExit{}) {
			t.Errorf("exit = %+v; want a pass", exit)
		}
		if rig.fixer.phaseAtAwait != phaseFixing || callsAtWait != 0 {
			t.Errorf("at the wait phase = %q with %d gate calls; want %q before any", rig.fixer.phaseAtAwait, callsAtWait, phaseFixing)
		}
		if len(rig.fixer.specs) != 0 || len(rig.fixer.awaitedGuids) != 1 || rig.fixer.awaitedGuids[0] != "old-strand" {
			t.Errorf("spawned %d, waited on %v; want a wait on the old strand and no spawn", len(rig.fixer.specs), rig.fixer.awaitedGuids)
		}
		if record := rig.storedRecord(); record.Status != gatefixer.StatusDone {
			t.Errorf("fixer record status = %q; want %q", record.Status, gatefixer.StatusDone)
		}
	})

	for _, tt := range []struct {
		name     string
		on       bool
		awaitErr error
	}{
		{"NoKitStopsTheStrand", false, nil},
		{"FailedWaitStopsTheStrand", true, errors.New("events unreadable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rig := newFixerRig(t, 0)
			rig.on = tt.on
			rig.fixer.awaitErr = tt.awaitErr
			if err := rig.store.Save(liveRecord); err != nil {
				t.Fatal(err)
			}
			if exit := rig.runRequest(request); exit != (detachExit{}) {
				t.Errorf("exit = %+v; want a pass", exit)
			}
			if len(rig.stoppedGuids) != 1 || rig.stoppedGuids[0] != "old-strand" {
				t.Errorf("stopped %v; want the old strand through the stopper", rig.stoppedGuids)
			}
			if record := rig.storedRecord(); record.Status != gatefixer.StatusDead || record.DeadReason != gatefixer.DeadReasonDied {
				t.Errorf("fixer record = %q %q; want dead, died", record.Status, record.DeadReason)
			}
			if len(rig.fixer.specs) != 0 || rig.calls.Load() != 1 {
				t.Errorf("spawned %d fixers over %d gate calls; want none over one", len(rig.fixer.specs), rig.calls.Load())
			}
		})
	}

	t.Run("FailedStopLeavesTheRecordLiveAndEndsTheCall", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 0)
		rig.on = false
		rig.stopperErr = errors.New("reed unreachable")
		if err := rig.store.Save(liveRecord); err != nil {
			t.Fatal(err)
		}
		if exit := rig.runRequest(request); exit != (detachExit{Code: 1}) {
			t.Errorf("exit = %+v; want exit 1", exit)
		}
		if rig.calls.Load() != 0 {
			t.Errorf("gate calls = %d; want the gate not to run over a fixer's edits", rig.calls.Load())
		}
		if stderr := rig.stderr(); !strings.Contains(stderr, "old-strand") || !strings.Contains(stderr, "way forward") {
			t.Errorf("stderr = %q; want the strand and the way forward", stderr)
		}
		if record := rig.storedRecord(); record.Status != "" {
			t.Errorf("fixer record status = %q; want it live", record.Status)
		}
	})

	// This subtest waits out the runner's one-second lease read, a package constant.
	t.Run("LapsedLeaseDuringTheFixerStopsIt", func(t *testing.T) {
		t.Parallel()
		rig := newFixerRig(t, 1)
		rig.fixer.result = gatefixer.Result{}
		rig.fixer.beforeAwait = func() {
			if err := rig.run.expireLease(time.Now()); err != nil {
				t.Error(err)
			}
		}
		if exit := rig.runRequest(request); exit != (detachExit{Code: 1, Abandoned: true}) {
			t.Errorf("exit = %+v; want {1, abandoned}", exit)
		}
		if len(rig.fixer.stopped) != 1 || rig.fixer.stopped[0] != "strand-1" {
			t.Errorf("stopped %v; want the fixer's strand", rig.fixer.stopped)
		}
		if stderr := rig.stderr(); !strings.Contains(stderr, abandonedLine(verbTest)) {
			t.Errorf("stderr = %q; want the abandoned line", stderr)
		}
		if record := rig.storedRecord(); record.Status != gatefixer.StatusDead || record.DeadReason != gatefixer.DeadReasonDied {
			t.Errorf("fixer record = %q %q; want dead, died", record.Status, record.DeadReason)
		}
		if rig.calls.Load() != 1 {
			t.Errorf("gate calls = %d; want no re-run", rig.calls.Load())
		}
	})
}
