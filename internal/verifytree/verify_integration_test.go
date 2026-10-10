//go:build integration

// verify_integration_test.go exercises Verify against a real scratch repo and real shell processes, so it carries the integration tag.

package verifytree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
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
	res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, command, Timeout, nil)
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
		res, err := Verify(context.Background(), p, Site{Label: "Webster-Burler gate", BaseCommand: plan}, command, Timeout, nil)
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

// TestVerify_Scenario is a scenario over one scratch repo, run as named steps in one order, each reaching one rule of Verify: a dirty tree is refused, a record is written only after a pass and only when HEAD did not move during the run, the marker is present only while the command runs, a skip needs a record naming HEAD's tree and the same command, and DirtyPaths names special and renamed paths verbatim.
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
		if res.Status != StatusDirty {
			t.Fatalf("Verify = %q; want %q", res.Status, StatusDirty)
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

	if !t.Run("a command outliving the timeout fails as timed out with no record and no marker", func(t *testing.T) {
		res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, "sleep 30", 300*time.Millisecond, nil)
		if err != nil {
			t.Fatalf("Verify with an expired timeout returned an error: %v", err)
		}
		if res.Status != StatusFailed || !res.TimedOut || res.ExitCode != -1 {
			t.Fatalf("Verify = (%q, timedOut %v, %d); want (%q, true, -1)", res.Status, res.TimedOut, res.ExitCode, StatusFailed)
		}
		if !strings.Contains(res.Detail, "did not finish within 300ms") {
			t.Errorf("Detail = %q; want it to name the timeout", res.Detail)
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
		if _, err := Verify(ctx, p, Site{Label: "Publish"}, "true", Timeout, nil); err == nil {
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
		if res := mustVerify(t, p, "true"); res.Status != StatusSkipped {
			t.Errorf("second Verify = %q; want %q", res.Status, StatusSkipped)
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

// TestVerify_SlotGate covers a slotted Verify over a one-slot pool the test holds: the marker reads as waiting while the slot is held, a timeout shorter than the wait does not fire during it, the command runs once the slot is released, and it sees the slot's `-p` cap appended to an existing GOFLAGS plus the inheritance variable.
// It sets GOFLAGS in the process environment, so it is not parallel.
func TestVerify_SlotGate(t *testing.T) {
	t.Setenv("GOFLAGS", "-count=1")
	p := newScratch(t)
	pool := &gateslot.Pool{
		Dir:    filepath.Join(t.TempDir(), "gate"),
		Limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1, GoParallel: 7}, nil },
		Poll:   10 * time.Millisecond,
	}
	held, err := pool.Acquire(context.Background(), gateslot.Holder{Site: "test holder"})
	if err != nil {
		t.Fatal(err)
	}

	seenEnv := filepath.Join(t.TempDir(), "env")
	command := "printf '%s\\n%s\\n' \"$GOFLAGS\" \"$LYX_GATE_SLOT\" > " + seenEnv
	const timeout = 300 * time.Millisecond
	type outcome struct {
		res Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, command, timeout, pool)
		done <- outcome{res, err}
	}()

	deadline := time.Now().Add(10 * time.Second)
	var waiting Marker
	for {
		m, ok, err := ReadMarker(p.Marker)
		if err != nil {
			t.Fatal(err)
		}
		if ok && m.State == MarkerStateWaiting {
			waiting = m
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no waiting marker while the only slot was held")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if waiting.WaitStarted.IsZero() || waiting.Command != command || waiting.Site != "webster verify" {
		t.Errorf("waiting marker = %+v; want the site and command with a wait start", waiting)
	}

	// The wait outlasts the timeout, which must not start counting before the slot is held.
	time.Sleep(2 * timeout)
	select {
	case got := <-done:
		t.Fatalf("Verify returned (%+v, %v) while the slot was held", got.res, got.err)
	default:
	}

	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil || got.res.Status != StatusPassed {
		t.Fatalf("Verify after the release = (%+v, %v); want a pass", got.res, got.err)
	}
	if _, ok := LatestPass(p, command); !ok {
		t.Error("no record after the slotted pass")
	}
	if fileExists(p.Marker) {
		t.Error("the marker survived the slotted run")
	}
	holders, err := pool.Holders()
	if err != nil || len(holders) != 0 {
		t.Errorf("Holders() after the run = (%v, %v); want the slot released", holders, err)
	}

	data, err := os.ReadFile(seenEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := "-count=1 -p=7\n" + filepath.Join(pool.Dir, "slot-1.lock") + "\n"
	if string(data) != want {
		t.Errorf("command saw GOFLAGS and slot variable %q; want %q", data, want)
	}
}
