//go:build integration

// verify_integration_test.go exercises Verify against a real scratch repo and real shell processes, so it carries the integration tag.

package verifytree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// newScratch returns Paths over a fresh one-commit repo whose verify directory lies outside the worktree.
func newScratch(t *testing.T) Paths {
	t.Helper()
	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	return NewPaths(worktree, t.TempDir())
}

func mustVerify(t *testing.T, p Paths, command string) Result {
	t.Helper()
	res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, command)
	if err != nil {
		t.Fatalf("Verify(%q): %v", command, err)
	}
	return res
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestVerify_PerCommandRecord covers the per-command record: a round command's pass leaves the plan-verify entry in place and skips on a repeat, a write drops another command's entry naming a different tree while keeping the base command's and replacing its own, LatestPass returns the pass's commit, and an old-format record reads as none.
// The steps share one repo and run in order, so the test is parallel as a whole and no step is.
func TestVerify_PerCommandRecord(t *testing.T) {
	t.Parallel()

	p := newScratch(t)
	const plan, roundA, roundB = "true", ": round a", ": round b"
	verifyWithBase := func(command string) Result {
		t.Helper()
		res, err := Verify(context.Background(), p, Site{Label: "Webster-Burler gate", BaseCommand: plan}, command)
		if err != nil {
			t.Fatalf("Verify(%q): %v", command, err)
		}
		return res
	}
	head := func() string {
		t.Helper()
		commit, err := headCommit(p.Worktree)
		if err != nil {
			t.Fatal(err)
		}
		return commit
	}

	if !t.Run("a round command's pass leaves the plan entry in place and a repeat skips", func(t *testing.T) {
		mustVerify(t, p, plan)
		if res := verifyWithBase(roundA); res.Status != StatusPassed {
			t.Fatalf("first round Verify = %q; want %q", res.Status, StatusPassed)
		}
		if res := verifyWithBase(roundA); res.Status != StatusSkipped {
			t.Errorf("second round Verify = %q; want %q", res.Status, StatusSkipped)
		}
		if res := mustVerify(t, p, plan); res.Status != StatusSkipped {
			t.Errorf("plan Verify after the round pass = %q; want %q", res.Status, StatusSkipped)
		}
		pass, ok := LatestPass(p, plan)
		if !ok || pass.Commit != head() || pass.Tree == "" || pass.VerifiedAt.IsZero() {
			t.Errorf("LatestPass(plan) = (%+v, %v); want the pass at commit %s", pass, ok, head())
		}
	}) {
		return
	}

	planCommit := head()
	gitkit.CommitFile(t, p.Worktree, "b.txt", "b\n", "second")

	if !t.Run("a write drops another command's stale entry and keeps the base command's", func(t *testing.T) {
		if res := verifyWithBase(roundB); res.Status != StatusPassed {
			t.Fatalf("Verify = %q; want %q", res.Status, StatusPassed)
		}
		if _, ok := LatestPass(p, roundA); ok {
			t.Error("the entry of another command naming a different tree survived")
		}
		if pass, ok := LatestPass(p, plan); !ok || pass.Commit != planCommit {
			t.Errorf("LatestPass(plan) = (%+v, %v); want the base entry at commit %s", pass, ok, planCommit)
		}
		if pass, ok := LatestPass(p, roundB); !ok || pass.Commit != head() {
			t.Errorf("LatestPass(round b) = (%+v, %v); want commit %s", pass, ok, head())
		}
	}) {
		return
	}

	gitkit.CommitFile(t, p.Worktree, "c.txt", "c\n", "third")

	if !t.Run("a write replaces its own command's entry", func(t *testing.T) {
		verifyWithBase(roundB)
		if pass, ok := LatestPass(p, roundB); !ok || pass.Commit != head() {
			t.Errorf("LatestPass(round b) = (%+v, %v); want commit %s", pass, ok, head())
		}
	}) {
		return
	}

	t.Run("an old-format record reads as none", func(t *testing.T) {
		old := "tree: abc\ncommand: " + plan + "\nverified_at: 2026-01-01T00:00:00Z\n"
		if err := os.WriteFile(p.Record, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		if pass, ok := LatestPass(p, plan); ok {
			t.Errorf("LatestPass on an old-format record = (%+v, true); want none", pass)
		}
	})
}

// TestVerify_Scenario is a scenario over one scratch repo, run as named steps in one order, each reaching one rule of Verify: a dirty tree is refused, a record is written only after a pass and only when HEAD did not move during the run, the marker is present only while the command runs, a skip needs a record naming HEAD's tree and the same command, each run writes its own log and pruning bounds them while keeping the failure record's, and DirtyPaths names special and renamed paths verbatim.
// The steps share one repo, so the test is parallel as a whole and no step is.
// The steps up to the first pass rely on no record existing yet, so they run before it; each step after it relies on the record the one before left, and the last step relies on being last because it dirties the tree.
func TestVerify_Scenario(t *testing.T) {
	t.Parallel()

	p := newScratch(t)

	if !t.Run("a dirty tree is refused with its paths and runs nothing", func(t *testing.T) {
		stray := filepath.Join(p.Worktree, "stray.txt")
		if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		ran := filepath.Join(t.TempDir(), "ran")
		res := mustVerify(t, p, "touch "+ran)
		if res.Status != StatusDirty || res.Log != "" {
			t.Fatalf("Verify = (%q, log %q); want (%q, no log)", res.Status, res.Log, StatusDirty)
		}
		if len(res.Dirty) != 1 || res.Dirty[0] != "stray.txt" {
			t.Errorf("Dirty = %v; want [stray.txt]", res.Dirty)
		}
		if fileExists(ran) {
			t.Error("the command ran on a dirty tree")
		}
		if fileExists(p.Record) {
			t.Error("a record was written for a dirty tree")
		}
		// Clean again for the steps after this one.
		if err := os.Remove(stray); err != nil {
			t.Fatal(err)
		}
	}) {
		return
	}

	if !t.Run("a failing command writes no record and leaves no marker", func(t *testing.T) {
		res := mustVerify(t, p, "exit 3")
		if res.Status != StatusFailed || res.ExitCode != 3 {
			t.Fatalf("Verify = (%q, %d); want (%q, 3)", res.Status, res.ExitCode, StatusFailed)
		}
		if fileExists(p.Record) {
			t.Error("a record was written after a failure")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a failing command")
		}
	}) {
		return
	}

	if !t.Run("a command printing the tier timeout line fails as timed out with no record and no marker", func(t *testing.T) {
		line := gateslot.TierTimeoutLine("tier2", 30*time.Minute)
		res := mustVerify(t, p, "echo '"+strings.ReplaceAll(line, "'", `'\''`)+"'; exit 1")
		if res.Status != StatusFailed || !res.TimedOut || res.ExitCode != 1 {
			t.Fatalf("Verify = (%q, timedOut %v, %d); want (%q, true, 1)", res.Status, res.TimedOut, res.ExitCode, StatusFailed)
		}
		if !strings.Contains(res.Detail, "tier2") || !strings.Contains(res.Detail, "30m0s") {
			t.Errorf("Detail = %q; want it to name the tier and its timeout", res.Detail)
		}
		if fileExists(p.Record) {
			t.Error("a record was written after a timeout")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a timeout")
		}
	}) {
		return
	}

	if !t.Run("a commit during the run leaves no record", func(t *testing.T) {
		command := "echo c > c.txt && git add c.txt && git commit -q -m mid-run"
		if res := mustVerify(t, p, command); res.Status != StatusPassed {
			t.Fatalf("Verify = %q; want %q", res.Status, StatusPassed)
		}
		if fileExists(p.Record) {
			t.Error("a record was written although HEAD moved during the run")
		}
	}) {
		return
	}

	if !t.Run("a cancelled run leaves no record and no marker, and a pass then writes the record", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Verify(ctx, p, Site{Label: "Publish"}, "true"); err == nil {
			t.Fatal("Verify with a cancelled ctx returned no error")
		}
		if fileExists(p.Record) {
			t.Error("a record was written by a cancelled run")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a cancelled run")
		}

		if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
			t.Fatalf("Verify after the cancelled run = %q; want %q", res.Status, StatusPassed)
		}
		if !fileExists(p.Record) {
			t.Error("no record after a pass")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a passing command")
		}
	}) {
		return
	}

	if !t.Run("a record naming the tree and the command skips the run", func(t *testing.T) {
		if res := mustVerify(t, p, "true"); res.Status != StatusSkipped || res.Log != "" {
			t.Errorf("second Verify = (%q, log %q); want (%q, no log)", res.Status, res.Log, StatusSkipped)
		}
	}) {
		return
	}

	if !t.Run("a different command runs", func(t *testing.T) {
		if res := mustVerify(t, p, "exit 0"); res.Status != StatusPassed {
			t.Errorf("Verify with a different command = %q; want %q", res.Status, StatusPassed)
		}
	}) {
		return
	}

	if !t.Run("the marker is present while the command runs", func(t *testing.T) {
		seen := filepath.Join(t.TempDir(), "seen")
		mustVerify(t, p, "cp "+p.Marker+" "+seen)
		m, ok, err := ReadMarker(seen)
		if err != nil || !ok {
			t.Fatalf("ReadMarker of the copy = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		if m.Site != "webster verify" {
			t.Errorf("marker site = %q; want %q", m.Site, "webster verify")
		}
		if want := "cp " + p.Marker + " " + seen; m.Command != want {
			t.Errorf("marker command = %q; want %q", m.Command, want)
		}
	}) {
		return
	}

	if !t.Run("a new commit makes the next verify run", func(t *testing.T) {
		gitkit.CommitFile(t, p.Worktree, "b.txt", "b\n", "second")
		if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
			t.Errorf("Verify after a new commit = %q; want %q", res.Status, StatusPassed)
		}
	}) {
		return
	}

	if !t.Run("two consecutive runs each leave their own log", func(t *testing.T) {
		first := mustVerify(t, p, "echo first; exit 1")
		second := mustVerify(t, p, "echo second; exit 1")
		if first.Log == "" || first.Log == second.Log {
			t.Fatalf("logs = %q and %q; want two distinct logs", first.Log, second.Log)
		}
		for _, tc := range []struct {
			res  Result
			want string
		}{{first, "first\n"}, {second, "second\n"}} {
			if !IsLogPath(p, tc.res.Log) {
				t.Errorf("IsLogPath(%q) = false; want a verify log in %s", tc.res.Log, p.Dir)
			}
			got, err := os.ReadFile(tc.res.Log)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("log %s = %q; want %q", tc.res.Log, got, tc.want)
			}
		}
	}) {
		return
	}

	if !t.Run("a run past the retention bound prunes the oldest log but keeps the one the failure record names", func(t *testing.T) {
		logs, err := listLogs(p.Dir)
		if err != nil {
			t.Fatal(err)
		}
		for n := logs[0].number + 1; n <= logs[0].number+verifyLogKeep-len(logs); n++ {
			if err := os.WriteFile(filepath.Join(p.Dir, logPrefix+strconv.Itoa(n)+logSuffix), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		oldest := logs[len(logs)-1].path

		res := mustVerify(t, p, "exit 1")
		if fileExists(oldest) {
			t.Errorf("oldest log %s survived the run past the bound", oldest)
		}
		if !fileExists(res.Log) {
			t.Errorf("the run's own log %s was pruned", res.Log)
		}

		logs, err = listLogs(p.Dir)
		if err != nil {
			t.Fatal(err)
		}
		named := logs[len(logs)-1].path
		if err := WriteLandingFailure(p, LandingFailure{Kind: FailureKindPlanVerify, LogPath: named, Head: "abc"}); err != nil {
			t.Fatal(err)
		}
		mustVerify(t, p, "exit 1")
		if !fileExists(named) {
			t.Errorf("log %s the failure record names was pruned", named)
		}
		after, err := listLogs(p.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != verifyLogKeep+1 {
			t.Errorf("%d logs after pruning; want %d, the newest plus the named one", len(after), verifyLogKeep+1)
		}
		if err := RemoveLandingFailure(p); err != nil {
			t.Fatal(err)
		}

		// A record an older build left under its old file name names a log that pruning does not keep.
		oldRecord := filepath.Join(p.Dir, "publish-failure.yaml")
		if err := os.WriteFile(oldRecord, []byte("kind: plan_verify\nlog_path: "+named+"\nhead: abc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, found, err := ReadLandingFailure(p); found || err != nil {
			t.Errorf("ReadLandingFailure over a record under the old name = (_, %v, %v); want (_, false, nil)", found, err)
		}
		mustVerify(t, p, "exit 1")
		if fileExists(named) {
			t.Errorf("log %s a record under the old name names was kept by pruning", named)
		}
	}) {
		return
	}

	t.Run("DirtyPaths names special and renamed paths verbatim", func(t *testing.T) {
		gitkit.Git(t, p.Worktree, "mv", "a.txt", "renamed.txt")
		special := "café -> x.txt"
		if err := os.WriteFile(filepath.Join(p.Worktree, special), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := DirtyPaths(p.Worktree)
		if err != nil {
			t.Fatalf("DirtyPaths: %v", err)
		}
		want := []string{"renamed.txt", special}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("DirtyPaths = %q; want %q", got, want)
		}
	})
}

// TestVerify_DropsStrandName proves a verify command never sees the strand name or an inherited prebuilt-lyx path.
// It sets both variables with t.Setenv, which is process-global state, so it is not parallel.
func TestVerify_DropsStrandName(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "verify-test-strand")
	t.Setenv(gateslot.PrebuiltLyxEnv, "/stale/lyx")

	p := newScratch(t)
	seen := filepath.Join(t.TempDir(), "env")
	command := "printf '%s|%s' \"$" + agentname.StrandNameEnv + "\" \"$" + gateslot.PrebuiltLyxEnv + "\" > " + seen
	res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, command)
	if err != nil || res.Status != StatusPassed {
		t.Fatalf("Verify = (%+v, %v); want a pass", res, err)
	}
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "|" {
		t.Errorf("command saw %s and %s as %q; want both dropped", agentname.StrandNameEnv, gateslot.PrebuiltLyxEnv, data)
	}
}

// pauseEnv is one TestVerify_PauseHold row's scratch repo, pause record, hold and the files its commands write outside the worktree.
type pauseEnv struct {
	t       *testing.T
	p       Paths
	record  string
	hold    *PauseHold
	polls   atomic.Int64
	counter string
	term    string
	release string
}

// newPauseEnv returns an env whose hold judges slug over a fresh record, with a short Poll and a fast Sleep.
func newPauseEnv(t *testing.T, slug string, slugEntryExempt func() bool) *pauseEnv {
	t.Helper()
	files := t.TempDir()
	e := &pauseEnv{t: t, p: newScratch(t), record: filepath.Join(files, "record.json"), counter: filepath.Join(files, "ran"), term: filepath.Join(files, "term"), release: filepath.Join(files, "release")}
	e.useHold(NewPauseHold(pauserecord.Geometry{RecordPath: e.record, Slug: slug}, slugEntryExempt))
	// A command still looping on the release file when a row fails must not outlive the row.
	t.Cleanup(func() { _ = os.WriteFile(e.release, nil, 0o644) })
	return e
}

// useHold makes hold the env's hold, shortening its timing seams and counting each PastGrace poll.
func (e *pauseEnv) useHold(hold *PauseHold) {
	hold.Poll = 10 * time.Millisecond
	hold.Sleep = func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(5 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	pastGrace := hold.PastGrace
	hold.PastGrace = func() bool {
		defer e.polls.Add(1)
		return pastGrace()
	}
	e.hold = hold
}

// pause adds a live entry of scope paused now whose grace ends at graceEnd.
func (e *pauseEnv) pause(scope string, graceEnd time.Time) {
	e.t.Helper()
	_, err := pauserecord.Update(e.record, time.Now(), func(r *pauserecord.Record) error {
		r.Live = append(r.Live, pauserecord.Entry{Scope: scope, PausedAt: time.Now(), GraceEnd: graceEnd, Caller: "EXTERNAL"})
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// resume moves every live entry to the resumed list.
func (e *pauseEnv) resume() {
	e.t.Helper()
	_, err := pauserecord.Update(e.record, time.Now(), func(r *pauserecord.Record) error {
		for _, entry := range r.Live {
			entry.ResumedAt = time.Now()
			r.Resumed = append(r.Resumed, entry)
		}
		r.Live = nil
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// pauseOutcome is what one Verify call returned.
type pauseOutcome struct {
	result Result
	err    error
}

// verifyAsync runs Verify of command under the env's hold in a goroutine and returns its outcome channel.
func (e *pauseEnv) verifyAsync(ctx context.Context, command string) <-chan pauseOutcome {
	out := make(chan pauseOutcome, 1)
	go func() {
		result, err := Verify(ctx, e.p, Site{Label: "webster verify", Hold: e.hold}, command)
		out <- pauseOutcome{result, err}
	}()
	return out
}

// waitFor polls cond every 10 ms until it holds, failing the test after ten seconds.
func (e *pauseEnv) waitFor(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *pauseEnv) markerState() string {
	e.t.Helper()
	marker, live, err := ReadMarker(e.p.Marker)
	if err != nil {
		e.t.Fatal(err)
	}
	if !live {
		return ""
	}
	return marker.State
}

// lines counts the lines of the file at path, zero when it does not exist.
func lines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

func (e *pauseEnv) assertNoMarker() {
	e.t.Helper()
	if state := e.markerState(); state != "" {
		e.t.Errorf("a %q marker is live after Verify returned; want none", state)
	}
}

// pollsBeyond waits until the hold has polled PastGrace twice more.
func (e *pauseEnv) pollsBeyond() {
	e.t.Helper()
	target := e.polls.Load() + 2
	e.waitFor("two more PastGrace polls", func() bool { return e.polls.Load() >= target })
}

// quickCommand appends a line to the env's counter file and exits.
func (e *pauseEnv) quickCommand() string { return "echo ran >> " + e.counter }

// holdOpenCommand appends a line to the counter file, then runs until the release file exists.
func (e *pauseEnv) holdOpenCommand() string {
	return "echo ran >> " + e.counter + "; while [ ! -f " + e.release + " ]; do sleep 0.05; done"
}

// stoppableCommand appends a line to the counter file and, on its first run only, sleeps until a SIGTERM makes its trap write a line to the term file and exit.
func (e *pauseEnv) stoppableCommand() string {
	return "trap 'echo term >> " + e.term + "; exit 0' TERM; echo ran >> " + e.counter + `; [ "$(wc -l < ` + e.counter + `)" -ge 2 ] && exit 0; sleep 60 & wait`
}

// TestVerify_PauseHold table-drives Verify under a pause hold over a scratch repo and a temp pause record:
// the hold at entry, the stop at the grace end and the start over after each, a caller's cancel, the parent-lock exemption and the readings that never stop a verify.
// It is not parallel as a whole because one row captures the process-global logger; its other rows are.
func TestVerify_PauseHold(t *testing.T) {
	stopAndWait := func(t *testing.T, e *pauseEnv) <-chan pauseOutcome {
		t.Helper()
		out := e.verifyAsync(context.Background(), e.stoppableCommand())
		e.waitFor("the first run to start", func() bool { return lines(t, e.counter) == 1 })
		e.pause(pauserecord.ScopeHub, time.Now())
		e.waitFor("the paused marker", func() bool { return e.markerState() == MarkerStatePaused })
		if got := lines(t, e.term); got != 1 {
			t.Errorf("SIGTERM trap lines = %d; want 1, the stopped run's trap", got)
		}
		if !fileExists(filepath.Join(e.p.Dir, "verify-1.log")) {
			t.Errorf("the stopped run's verify-1.log is gone")
		}
		return out
	}
	heldAtEntry := func(t *testing.T, e *pauseEnv, command string) <-chan pauseOutcome {
		t.Helper()
		e.pause(pauserecord.ScopeHub, time.Now().Add(time.Hour))
		out := e.verifyAsync(context.Background(), command)
		e.waitFor("the paused marker", func() bool { return e.markerState() == MarkerStatePaused })
		if got := lines(t, e.counter); got != 0 {
			t.Errorf("command runs while held = %d; want 0", got)
		}
		return out
	}

	tests := []struct {
		name   string
		serial bool
		run    func(t *testing.T, e *pauseEnv)
	}{
		{"a Verify entered under a live pause waits with a paused marker, then runs the command once and passes after the resume", false, func(t *testing.T, e *pauseEnv) {
			out := heldAtEntry(t, e, e.quickCommand())
			e.resume()
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 1 {
				t.Errorf("command runs = %d; want 1", n)
			}
			e.assertNoMarker()
		}},
		{"a tree dirtied while held returns dirty after the resume and runs nothing", false, func(t *testing.T, e *pauseEnv) {
			out := heldAtEntry(t, e, e.quickCommand())
			if err := os.WriteFile(filepath.Join(e.p.Worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			e.resume()
			got := <-out
			if got.err != nil || got.result.Status != StatusDirty {
				t.Fatalf("Verify = (%+v, %v); want StatusDirty", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 0 {
				t.Errorf("command runs = %d; want 0", n)
			}
			e.assertNoMarker()
		}},
		{"a command running past a pause's grace end gets SIGTERM, keeps its log, and runs again from the dirty check after the resume", false, func(t *testing.T, e *pauseEnv) {
			out := stopAndWait(t, e)
			e.resume()
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 2 {
				t.Errorf("command runs = %d; want 2", n)
			}
			if !fileExists(filepath.Join(e.p.Dir, "verify-1.log")) || !fileExists(filepath.Join(e.p.Dir, "verify-2.log")) {
				t.Errorf("verify logs in %s: want the stopped run's and the rerun's", e.p.Dir)
			}
			e.assertNoMarker()
		}},
		{"a tree dirtied during the stop's pause returns dirty after the resume and runs no second command", false, func(t *testing.T, e *pauseEnv) {
			out := stopAndWait(t, e)
			if err := os.WriteFile(filepath.Join(e.p.Worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			e.resume()
			got := <-out
			if got.err != nil || got.result.Status != StatusDirty {
				t.Fatalf("Verify = (%+v, %v); want StatusDirty", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 1 {
				t.Errorf("command runs = %d; want 1, the stopped run", n)
			}
			e.assertNoMarker()
		}},
		{"a record naming HEAD's tree written during the stop's pause skips after the resume", false, func(t *testing.T, e *pauseEnv) {
			out := stopAndWait(t, e)
			tree, err := headTree(e.p.Worktree)
			if err != nil {
				t.Fatal(err)
			}
			commit, err := headCommit(e.p.Worktree)
			if err != nil {
				t.Fatal(err)
			}
			rec := record{Entries: []entry{{Command: e.stoppableCommand(), Tree: tree, Commit: commit, VerifiedAt: time.Now()}}}
			if err := writeRecord(e.p.Record, rec); err != nil {
				t.Fatal(err)
			}
			e.resume()
			got := <-out
			if got.err != nil || got.result.Status != StatusSkipped {
				t.Fatalf("Verify = (%+v, %v); want StatusSkipped", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 1 {
				t.Errorf("command runs = %d; want 1, the stopped run", n)
			}
			e.assertNoMarker()
		}},
		{"a pause resumed before its grace end lets the running command finish and record its result", false, func(t *testing.T, e *pauseEnv) {
			out := e.verifyAsync(context.Background(), e.holdOpenCommand())
			e.waitFor("the command to start", func() bool { return lines(t, e.counter) == 1 })
			e.pause(pauserecord.ScopeHub, time.Now().Add(time.Hour))
			e.pollsBeyond()
			e.resume()
			e.pollsBeyond()
			if err := os.WriteFile(e.release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 1 {
				t.Errorf("command runs = %d; want 1", n)
			}
			if _, ok := LatestPass(e.p, e.holdOpenCommand()); !ok {
				t.Errorf("no pass recorded for the finished command")
			}
		}},
		{"a caller's cancel while held returns the cancel, runs no command and leaves no marker", false, func(t *testing.T, e *pauseEnv) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.pause(pauserecord.ScopeHub, time.Now().Add(time.Hour))
			out := e.verifyAsync(ctx, e.quickCommand())
			e.waitFor("the paused marker", func() bool { return e.markerState() == MarkerStatePaused })
			cancel()
			got := <-out
			if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("Verify error = %v; want one wrapping context.Canceled", got.err)
			}
			if n := lines(t, e.counter); n != 0 {
				t.Errorf("command runs = %d; want 0", n)
			}
			e.assertNoMarker()
		}},
		{"a slug entry under an exempt hold neither holds the command nor stops it past its grace end", false, func(t *testing.T, e *pauseEnv) {
			e.useHold(NewPauseHold(pauserecord.Geometry{RecordPath: e.record, Slug: "alpha"}, func() bool { return true }))
			e.pause("alpha", time.Now())
			out := e.verifyAsync(context.Background(), e.holdOpenCommand())
			e.waitFor("the command to start", func() bool { return lines(t, e.counter) == 1 })
			if state := e.markerState(); state != MarkerStateRunning {
				t.Errorf("marker state = %q; want %q", state, MarkerStateRunning)
			}
			e.pollsBeyond()
			if err := os.WriteFile(e.release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
			if n := lines(t, e.counter); n != 1 {
				t.Errorf("command runs = %d; want 1, never stopped", n)
			}
		}},
		{"a hold over no pause geometry runs the command at once and never stops it", false, func(t *testing.T, e *pauseEnv) {
			e.useHold(NewPauseHold(pauserecord.Geometry{}, nil))
			out := e.verifyAsync(context.Background(), e.holdOpenCommand())
			e.waitFor("the command to start", func() bool { return lines(t, e.counter) == 1 })
			e.pollsBeyond()
			if err := os.WriteFile(e.release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
		}},
		{"an undecodable pause record runs the command at once with a running marker and logs one Warn across the polls", true, func(t *testing.T, e *pauseEnv) {
			logs := logcapture.CaptureVerbose(t)
			if err := os.WriteFile(e.record, []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
			out := e.verifyAsync(context.Background(), e.holdOpenCommand())
			e.waitFor("the command to start", func() bool { return lines(t, e.counter) == 1 })
			if state := e.markerState(); state != MarkerStateRunning {
				t.Errorf("marker state = %q; want %q", state, MarkerStateRunning)
			}
			e.pollsBeyond()
			if err := os.WriteFile(e.release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			got := <-out
			if got.err != nil || got.result.Status != StatusPassed {
				t.Fatalf("Verify = (%+v, %v); want a pass", got.result, got.err)
			}
			if n := strings.Count(logs.String(), "not stopping the verify"); n != 1 {
				t.Errorf("PastGrace Warn lines = %d; want 1; logs: %s", n, logs.String())
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.serial {
				t.Parallel()
			}
			tt.run(t, newPauseEnv(t, "", nil))
		})
	}
}
