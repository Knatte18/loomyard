//go:build tier0

// detachfront_test.go covers the join decision of a capped `lym gate test` call's front, which needs no process and no hub.

package gatecli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

func TestJoinable(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	tree := treeRecord{Tree: "tree-one", Dirty: []string{"a.go", "b.go"}}
	running := joinState{
		lease:      now.Add(time.Minute),
		current:    tree,
		recorded:   tree,
		process:    processRecord{PID: 42, Start: "start-a"},
		hasProcess: true,
	}
	live := func(pid int) bool { return pid == 42 }
	readsStart := func(start string) func(int) (string, bool) {
		return func(int) (string, bool) { return start, true }
	}

	created := time.Unix(1_600_000_000, 123)
	windowsReading := func(creationTime func(int) (time.Time, bool)) func(int) (string, bool) {
		return func(pid int) (string, bool) { return startReading("windows", pid, nil, creationTime) }
	}
	windowsRunning := running
	windowsRunning.process.Start = strconv.FormatInt(created.UnixNano(), 10)

	tests := []struct {
		name    string
		state   joinState
		live    func(int) bool
		reading func(int) (string, bool)
		want    bool
		// wantNoProbe is set when the decision must read neither liveness nor start reading.
		wantNoProbe bool
	}{
		{name: "LiveRunnerWithTheRecordedStart", state: running, reading: readsStart("start-a"), want: true},
		{name: "ReusedPidWithAnotherStart", state: running, reading: func(int) (string, bool) { return "start-b", true }},
		{name: "StartReadingUnavailable", state: running, reading: func(int) (string, bool) { return "", false }},
		{name: "DeadPid", state: running, live: func(int) bool { return false }, reading: readsStart("start-a")},
		{name: "NoRunnerClaimedYet", state: joinState{lease: running.lease, current: tree, recorded: tree}, reading: readsStart("start-a")},
		{
			name:    "WindowsPidWithTheRecordedCreationTime",
			state:   windowsRunning,
			reading: windowsReading(func(int) (time.Time, bool) { return created, true }),
			want:    true,
		},
		{
			name:    "WindowsPidWithAnotherCreationTime",
			state:   windowsRunning,
			reading: windowsReading(func(int) (time.Time, bool) { return created.Add(time.Second), true }),
		},
		{
			name:    "WindowsCreationTimeUnreadable",
			state:   windowsRunning,
			reading: windowsReading(func(int) (time.Time, bool) { return time.Time{}, false }),
		},
		{name: "EndedRunWithoutTheMark", state: withExit(running, detachExit{Code: 7}, false), want: true, wantNoProbe: true},
		{name: "EndedRunWithTheMark", state: withExit(running, detachExit{Code: 1, Abandoned: true}, false), wantNoProbe: true},
		{name: "PassedLease", state: withLease(running, now.Add(-time.Second)), reading: readsStart("start-a")},
		{name: "LeaseEndingNow", state: withLease(running, now), reading: readsStart("start-a")},
		{name: "DeliveredRun", state: withExit(running, detachExit{}, true), wantNoProbe: true},
		{name: "ChangedTreeWithTheSameDirtyPaths", state: withCurrent(running, treeRecord{Tree: "tree-two", Dirty: tree.Dirty}), reading: readsStart("start-a")},
		{name: "ChangedDirtyPathsOnTheSameTree", state: withCurrent(running, treeRecord{Tree: tree.Tree, Dirty: []string{"a.go"}}), reading: readsStart("start-a")},
		{name: "UnreadableTree", state: withCurrent(joinState{lease: running.lease, process: running.process, hasProcess: true}, treeRecord{}), reading: readsStart("start-a")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			probes := 0
			liveFn := tt.live
			if liveFn == nil {
				liveFn = live
			}
			readingFn := tt.reading
			if readingFn == nil {
				readingFn = func(int) (string, bool) { return "", false }
			}
			got := joinable(tt.state, now,
				func(pid int) bool { probes++; return liveFn(pid) },
				func(pid int) (string, bool) { probes++; return readingFn(pid) })
			if got != tt.want {
				t.Errorf("joinable() = %v; want %v", got, tt.want)
			}
			if tt.wantNoProbe && probes != 0 {
				t.Errorf("joinable() probed the runner %d times; want none", probes)
			}
		})
	}
}

// withExit returns state with an exit file holding exit, and delivered as given.
func withExit(state joinState, exit detachExit, delivered bool) joinState {
	state.exit, state.hasExit, state.delivered = exit, true, delivered
	return state
}

// withLease returns state with its lease ending at deadline.
func withLease(state joinState, deadline time.Time) joinState {
	state.lease = deadline
	return state
}

// withCurrent returns state with the tree identity the front sees now set to current.
func withCurrent(state joinState, current treeRecord) joinState {
	state.current = current
	return state
}

// callRecords returns the records of one finished call of the gate key with the listed tiers: a record per tier, each with a command and an equal share of wall, the call starting at begin.
func callRecords(key, id string, begin time.Time, wall time.Duration, tiers ...int) []gateslot.RunRecord {
	share := wall / time.Duration(len(tiers))
	var records []gateslot.RunRecord
	for i, tier := range tiers {
		start := begin.Add(time.Duration(i) * share)
		records = append(records, gateslot.RunRecord{
			GateKey: key, CallID: id, TierList: tiers, Tier: tier, Command: "run",
			WaitStart: start, Start: start, End: start.Add(share),
		})
	}
	return records
}

// mutateTier returns records after mutate changed every non-build record of the tier.
func mutateTier(records []gateslot.RunRecord, tier int, mutate func(*gateslot.RunRecord)) []gateslot.RunRecord {
	out := slices.Clone(records)
	for i := range out {
		if !out[i].Build && out[i].Tier == tier {
			mutate(&out[i])
		}
	}
	return out
}

// mutateBuild returns records after mutate changed every build record.
func mutateBuild(records []gateslot.RunRecord, mutate func(*gateslot.RunRecord)) []gateslot.RunRecord {
	out := slices.Clone(records)
	for i := range out {
		if out[i].Build {
			mutate(&out[i])
		}
	}
	return out
}

func TestLongWait(t *testing.T) {
	t.Parallel()

	const key = "finalize"
	begin := time.Unix(1_700_000_000, 0)
	// calls returns count full calls of the given wall time, the first starting offset after begin and each a minute after the one before.
	calls := func(prefix string, offset time.Duration, count int, wall time.Duration) []gateslot.RunRecord {
		var records []gateslot.RunRecord
		for i := range count {
			records = append(records, callRecords(key, prefix+strconv.Itoa(i), begin.Add(offset+time.Duration(i)*time.Minute), wall, 0, 1)...)
		}
		return records
	}
	const later = time.Hour
	fast := calls("fast", 0, 3, 5*time.Second)
	// slow holds slow calls that start after the fast ones, so a slow call that counted would decide the median.
	slow := calls("slow", later, 3, time.Minute)
	slowWithBuild := slices.Clone(slow)
	for i := range 3 {
		start := begin.Add(later + time.Duration(i)*time.Minute)
		slowWithBuild = append(slowWithBuild, gateslot.RunRecord{GateKey: key, CallID: "slow" + strconv.Itoa(i), TierList: []int{0, 1}, Tier: 0, Build: true, Command: "build", Start: start, End: start.Add(time.Second)})
	}

	tests := []struct {
		name      string
		records   []gateslot.RunRecord
		maxReruns int
		want      bool
	}{
		{name: "ThreeFullCallsOutlastingTheBreakEven", records: calls("c", 0, 3, 15*time.Second), maxReruns: 10, want: true},
		{name: "ThreeFullCallsWithinALargerBreakEven", records: calls("c", 0, 3, 15*time.Second), maxReruns: 20},
		{name: "TwoFullCalls", records: calls("c", 0, 2, time.Hour), maxReruns: 10},
		{name: "NoCalls", maxReruns: 10},
		{name: "CarriedTierLeavesTheCallOut", records: slices.Concat(fast, mutateTier(slow, 0, func(r *gateslot.RunRecord) { r.Carried = true })), maxReruns: 10},
		{name: "EmptyScopeTierLeavesTheCallOut", records: slices.Concat(fast, mutateTier(slow, 1, func(r *gateslot.RunRecord) { r.Command = "" })), maxReruns: 10},
		{name: "AbandonedLastTierLeavesTheCallOut", records: slices.Concat(fast, mutateTier(slow, 1, func(r *gateslot.RunRecord) { r.Abandoned = true })), maxReruns: 10},
		{
			name:      "AbandonedBuildLeavesTheCallOut",
			records:   slices.Concat(fast, mutateBuild(slowWithBuild, func(r *gateslot.RunRecord) { r.Abandoned = true })),
			maxReruns: 10,
		},
		{name: "MissingSecondListedTierLeavesTheCallOut", records: slices.Concat(fast, slices.DeleteFunc(slices.Clone(slow), func(r gateslot.RunRecord) bool { return r.Tier == 1 })), maxReruns: 10},
		{name: "WallRunsFromTheBuildStartWhereThereIsNoWaitStart", records: buildFirstCalls(key, begin, 3, 15*time.Second), maxReruns: 14, want: true},
		{name: "FailedBuildWithBlockedTiersIsAFullCall", records: blockedCalls(key, begin, 3, 15*time.Second), maxReruns: 10, want: true},
		{
			name:      "OtherKeysAndRecordsWithoutACallIDAreIgnored",
			records:   slices.Concat(fast, relabeled(slow, "other-key", true), relabeled(slow, key, false)),
			maxReruns: 10,
		},
		{name: "OnlyTheLatestFiveCallsCount", records: slices.Concat(calls("old", 0, 6, time.Second), calls("new", time.Hour, 5, time.Minute)), maxReruns: 10, want: true},
		{
			name:      "FourCallsTakeTheAverageOfTheMiddleTwo",
			records:   slices.Concat(calls("a", 0, 2, 4*time.Second), calls("b", time.Hour, 2, 16*time.Second)),
			maxReruns: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := longWait(tt.records, key, time.Second, tt.maxReruns); got != tt.want {
				t.Errorf("longWait() = %v; want %v", got, tt.want)
			}
		})
	}
}

// buildFirstCalls returns count calls of wall time, each with a build record that has no wait start followed by a tier record that waited for its slot after the build.
func buildFirstCalls(key string, begin time.Time, count int, wall time.Duration) []gateslot.RunRecord {
	var records []gateslot.RunRecord
	for i := range count {
		start := begin.Add(time.Duration(i) * time.Minute)
		id := "build" + strconv.Itoa(i)
		records = append(records,
			gateslot.RunRecord{GateKey: key, CallID: id, TierList: []int{0}, Tier: 0, Build: true, Command: "build", Start: start, End: start.Add(2 * time.Second)},
			gateslot.RunRecord{GateKey: key, CallID: id, TierList: []int{0}, Tier: 0, Command: "run", WaitStart: start.Add(2 * time.Second), Start: start.Add(2 * time.Second), End: start.Add(wall)},
		)
	}
	return records
}

// blockedCalls returns count calls of wall time whose build failed and whose tier records carry the failed build as their blocker.
func blockedCalls(key string, begin time.Time, count int, wall time.Duration) []gateslot.RunRecord {
	var records []gateslot.RunRecord
	for i := range count {
		start := begin.Add(time.Duration(i) * time.Minute)
		id := "blocked" + strconv.Itoa(i)
		records = append(records,
			gateslot.RunRecord{GateKey: key, CallID: id, TierList: []int{0}, Tier: 0, Build: true, Command: "build", ExitCode: 1, Start: start, End: start.Add(wall)},
			gateslot.RunRecord{GateKey: key, CallID: id, TierList: []int{0}, Tier: 0, BlockedBy: "build.log", WaitStart: start, Start: start.Add(wall), End: start.Add(wall)},
		)
	}
	return records
}

// relabeled returns records under another gate key, or with their call ids cleared when keepIDs is false.
func relabeled(records []gateslot.RunRecord, key string, keepIDs bool) []gateslot.RunRecord {
	out := slices.Clone(records)
	for i := range out {
		out[i].GateKey = key
		if !keepIDs {
			out[i].CallID = ""
		}
	}
	return out
}

func TestChooseCap(t *testing.T) {
	t.Parallel()

	const (
		short = time.Second
		long  = 9 * time.Second
	)
	begin := time.Unix(1_700_000_000, 0)
	callsOf := func(wall time.Duration) []gateslot.RunRecord {
		var records []gateslot.RunRecord
		for i := range 3 {
			records = append(records, callRecords("finalize", "call"+strconv.Itoa(i), begin.Add(time.Duration(i)*time.Minute), wall, 0)...)
		}
		return records
	}
	base := frontCall{cap: short, longCap: long, maxReruns: 10, gateKey: "finalize", recordsDir: "records"}

	tests := []struct {
		name      string
		call      frontCall
		records   []gateslot.RunRecord
		readErr   error
		want      time.Duration
		wantReads int
	}{
		{name: "SlowFullCallsTakeTheLongCap", call: base, records: callsOf(time.Minute), want: long, wantReads: 1},
		{name: "FastFullCallsKeepTheShortCap", call: base, records: callsOf(2 * time.Second), want: short, wantReads: 1},
		{name: "NoGateKeyKeepsTheShortCapWithoutARead", call: withoutKey(base), records: callsOf(time.Minute), want: short},
		{name: "NoBreakEvenKeepsTheShortCapWithoutARead", call: withoutReruns(base), records: callsOf(time.Minute), want: short},
		{name: "ReadErrorKeepsTheShortCap", call: base, readErr: errors.New("unreadable"), want: short, wantReads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reads []string
			got := chooseCap(tt.call, func(dir string) ([]gateslot.RunRecord, error) {
				reads = append(reads, dir)
				return tt.records, tt.readErr
			})
			if got != tt.want {
				t.Errorf("chooseCap() = %s; want %s", got, tt.want)
			}
			if len(reads) != tt.wantReads || (len(reads) > 0 && reads[0] != "records") {
				t.Errorf("chooseCap() read %q; want %d reads of the call's records directory", reads, tt.wantReads)
			}
		})
	}
}

// withoutKey returns call without a gate key.
func withoutKey(call frontCall) frontCall {
	call.gateKey = ""
	return call
}

// withoutReruns returns call without a break-even.
func withoutReruns(call frontCall) frontCall {
	call.maxReruns = 0
	return call
}

// failingWriter is a writer whose every write fails.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

func TestFrontReplay_DeliveredMark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		out           io.Writer
		wantDelivered bool
	}{
		{"OutputCopied", io.Discard, true},
		{"StdoutWriterFails", failingWriter{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := detachRun{dir: t.TempDir()}
			if err := os.WriteFile(run.path(stdoutFile), []byte("result\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			f := &front{out: tt.out, stderr: io.Discard}
			if code := f.replay(run, detachExit{Code: 3}); code != 3 {
				t.Errorf("replay = %d; want the run's code 3", code)
			}
			_, err := os.Stat(run.path(deliveredFile))
			if delivered := err == nil; delivered != tt.wantDelivered {
				t.Errorf("delivered mark present = %v; want %v", delivered, tt.wantDelivered)
			}
		})
	}
}

func TestFrontPruneOldRuns(t *testing.T) {
	t.Parallel()

	keyDir := t.TempDir()
	old := time.Now().Add(-detachRunMaxAge - time.Hour)
	makeRun := func(name string, modTime time.Time) detachRun {
		run := detachRun{dir: filepath.Join(keyDir, name)}
		if err := os.Mkdir(run.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(run.dir, modTime, modTime); err != nil {
			t.Fatal(err)
		}
		return run
	}
	current := makeRun("current", old)
	makeRun("stale", old)
	makeRun("recent", time.Now())
	liveRunner := makeRun("live", old)
	start, ok := processReading(os.Getpid())
	if !ok {
		t.Fatal("cannot read this process's start")
	}
	if err := liveRunner.claim(os.Getpid(), start); err != nil {
		t.Fatal(err)
	}
	// claim added a file, which moved the directory's modification time.
	if err := os.Chtimes(liveRunner.dir, old, old); err != nil {
		t.Fatal(err)
	}

	(&front{keyDir: keyDir}).pruneOldRuns(current)

	for name, wantKept := range map[string]bool{"current": true, "stale": false, "recent": true, "live": true} {
		_, err := os.Stat(filepath.Join(keyDir, name))
		if kept := err == nil; kept != wantKept {
			t.Errorf("run %q kept = %v; want %v", name, kept, wantKept)
		}
	}
}
