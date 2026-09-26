// friction_test.go covers the Tier 2 friction wiring this batch's runCmd/startCmd call sites own: the
// once-per-task clear-and-create split ensureFrictionDirAfterSeed implements for the shared bootstrap, run's own
// unconditional ensure, and the reflection-trigger decision shouldReflectFriction/reflectFriction
// implement for runCmd (only RunBlocked with a non-empty directory reflects in loomPostRun; the done
// path reflects inside the Friction-Reflect row). Every test here is untagged Tier 1: it spawns no subprocess, drives no
// real git operation, builds no real hub fixture, and contains no time.Sleep at or above one second.

package loomcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestEnsureFrictionDirAfterSeed_NilErrorClearsThenCreates asserts a genuine first seed (nil seedErr)
// clears a pre-populated friction directory before recreating it empty.
func TestEnsureFrictionDirAfterSeed_NilErrorClearsThenCreates(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	notePath := filepath.Join(dir, "some-note.md")
	if err := os.WriteFile(notePath, []byte("pre-existing note"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", notePath, err)
	}

	ensureFrictionDirAfterSeed(dir, nil)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(%q) = %v; want the directory to exist after ensure", dir, err)
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Errorf("Stat(%q) = %v; want the pre-existing note to have been cleared", notePath, err)
	}
}

// TestEnsureFrictionDirAfterSeed_ErrSeedExistsLeavesNotesUntouched asserts an ErrSeedExists re-entry
// -- the crash-resume guarantee -- leaves existing notes untouched but still ensures the directory
// exists.
func TestEnsureFrictionDirAfterSeed_ErrSeedExistsLeavesNotesUntouched(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	notePath := filepath.Join(dir, "some-note.md")
	if err := os.WriteFile(notePath, []byte("pre-existing note"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", notePath, err)
	}

	ensureFrictionDirAfterSeed(dir, loomshed.ErrSeedExists)

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want the pre-existing note to survive a resume", notePath, err)
	}
	if string(content) != "pre-existing note" {
		t.Errorf("note content = %q; want it untouched", content)
	}
}

// TestEnsureFrictionDirAfterSeed_EmptyDirSkipsBothOperations asserts an empty frictionDir is a no-op:
// Tier 2 is off and neither the clear nor the ensure runs.
func TestEnsureFrictionDirAfterSeed_EmptyDirSkipsBothOperations(t *testing.T) {
	t.Parallel()

	// No panic and no filesystem effect is the whole assertion here: an empty dir gives
	// os.RemoveAll/os.MkdirAll nothing to act on, and this call must not attempt either.
	ensureFrictionDirAfterSeed("", nil)
	ensureFrictionDirAfterSeed("", loomshed.ErrSeedExists)
}

// TestEnsureFrictionDirAfterSeed_MkdirAllFailureDoesNotError asserts a failed create (frictionDir's
// parent is a regular file, not a directory) leaves the call's own outcome unchanged: no panic, no
// returned error -- friction.EnsureDir's own contract is to warn and continue, never to fail the
// caller.
func TestEnsureFrictionDirAfterSeed_MkdirAllFailureDoesNotError(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", blocker, err)
	}
	dir := filepath.Join(blocker, "friction")

	// The assertion is that this returns at all, without panicking: ensureFrictionDirAfterSeed has no
	// error return, so a caller (startCmd) proceeds with the seed's own outcome regardless of whether
	// the directory could actually be created.
	ensureFrictionDirAfterSeed(dir, nil)
}

// TestRunEnsuresAbsentFrictionDir asserts the same friction.EnsureDir call run.go makes at
// startup creates an absent friction directory before the run proceeds -- mirrored here directly
// against the package runCmd calls, since run's own RunE is not independently invocable without a
// real Shed.
func TestRunEnsuresAbsentFrictionDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) = %v; want the directory absent before the ensure", dir, err)
	}

	friction.EnsureDir(dir)

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Stat(%q) = %v; want the directory to exist after EnsureDir", dir, err)
	}
}

// TestShouldReflectFriction covers every combination of the resolved friction directory and the run
// outcome shouldReflectFriction gates runCmd's reflection call on: RunPaused never triggers it
// regardless of the directory, an empty directory never triggers it regardless of outcome, and
// only RunBlocked triggers it when the directory is non-empty: RunDone reflects inside the
// Friction-Reflect row, not in loomPostRun.
//
// The non-nil-err path is not exercised here because it is structurally unreachable: runCmd's RunE
// already returns on a non-nil shed.Run error before this decision is ever consulted, so there is no
// outcome/err combination this function itself could be called with to represent it.
func TestShouldReflectFriction(t *testing.T) {
	tests := []struct {
		name        string
		frictionDir string
		outcome     shedengine.RunOutcome
		want        bool
	}{
		{"Done_DirSet", "/tmp/friction", shedengine.RunDone, false},
		{"Blocked_DirSet", "/tmp/friction", shedengine.RunBlocked, true},
		{"Paused_DirSet", "/tmp/friction", shedengine.RunPaused, false},
		{"Done_DirEmpty", "", shedengine.RunDone, false},
		{"Blocked_DirEmpty", "", shedengine.RunBlocked, false},
		{"Paused_DirEmpty", "", shedengine.RunPaused, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReflectFriction(tt.frictionDir, tt.outcome); got != tt.want {
				t.Errorf("shouldReflectFriction(%q, %q) = %v; want %v", tt.frictionDir, tt.outcome, got, tt.want)
			}
		})
	}
}

// TestReflectFriction_DepsValidationFailureReportsFailed asserts a malformed Deps -- a relative
// frictionDir here, which frictionengine.Reflect's own validateDeps rejects -- is logged and reported
// as frictionengine.StatusFailed on c.reflectFriction's return, never surfaced as an error and never
// "filed": frictionengine has no such status, since nothing in Go parses the agent's own report file.
func TestReflectFriction_DepsValidationFailureReportsFailed(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	c := &loomCLI{
		location:    loc,
		frictionDir: "relative-friction-dir", // not absolute: fails Deps validation deliberately
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}},
	}

	got := c.reflectFriction(false)

	if got != frictionengine.StatusFailed {
		t.Errorf("c.reflectFriction(false) = %q; want %q", got, frictionengine.StatusFailed)
	}
	if got == "filed" {
		t.Error("c.reflectFriction(false) = \"filed\"; that status must never exist")
	}
}

// TestReflectFriction_SkipsWhenAnotherDriverHoldsTheReflectionLock is the regression guard for the
// blocked path's second-driver window.
//
// shedengine.Run releases the run lock on return, and the blocked-path reflection fires after that
// return -- so for the whole of the reflection agent's life (friction_timeout_min, thirty minutes in
// the shipped template) the run lock reads as free and a second "lyx loom start" spawns a second driver.
// That second driver is a legitimate resume of a halted run, but its own reflection would archive
// the friction directory out from under the first one's live agent while both held the same
// reflection-report.md as a declared output.
//
// The lock is held here by a separate acquisition standing in for that other driver, and the
// assertion is that this call skips rather than waits: the other reflection already covers these
// notes, and blocking would hold a driver open for another agent's whole deadline.
func TestReflectFriction_SkipsWhenAnotherDriverHoldsTheReflectionLock(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	loc := &lyxcwd.Location{HubPath: hub, WorktreeName: "warp", AnchorRel: "."}

	lockPath := loomengine.LoomFrictionLock(loc)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(lockPath), err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("TryAcquireWriteLock(%q) = %v; want nil", lockPath, err)
	}
	if !acquired {
		t.Fatalf("TryAcquireWriteLock(%q) did not acquire a fresh lock", lockPath)
	}
	t.Cleanup(func() { _ = held.Release() })

	// Deliberately a relative friction directory, which frictionengine.Reflect's own validateDeps
	// rejects: reaching Reflect at all would report StatusFailed, so StatusSkipped can only mean the
	// lock check returned before it.
	c := &loomCLI{
		location:    loc,
		frictionDir: "relative-friction-dir",
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}},
	}

	if got := c.reflectFriction(false); got != frictionengine.StatusSkipped {
		t.Errorf("c.reflectFriction(false) with the reflection lock already held = %q; want %q -- a second driver must not reflect over the same notes", got, frictionengine.StatusSkipped)
	}
}

// TestReflectFriction_ReleasesTheLockForTheNextDriver asserts the lock is not leaked: once a
// reflection returns, a later one against the same task must be able to take it. Without the
// release, the first halt of a task would permanently suppress every later reflection in it.
func TestReflectFriction_ReleasesTheLockForTheNextDriver(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	c := &loomCLI{
		location:    loc,
		frictionDir: "relative-friction-dir",
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}},
	}

	if got := c.reflectFriction(false); got != frictionengine.StatusFailed {
		t.Fatalf("first c.reflectFriction(false) = %q; want %q", got, frictionengine.StatusFailed)
	}
	if got := c.reflectFriction(false); got != frictionengine.StatusFailed {
		t.Errorf("second c.reflectFriction(false) = %q; want %q -- the first call must have released the reflection lock, not held it for the process's life", got, frictionengine.StatusFailed)
	}
}

// newRelativeFrictionCLI builds a receiver whose relative frictionDir makes frictionengine.Reflect
// fail Deps validation: StatusFailed proves a reflection was attempted, StatusSkipped that it was not.
func newRelativeFrictionCLI(t *testing.T, armedVerb string) *loomCLI {
	t.Helper()
	return &loomCLI{
		location:    &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."},
		frictionDir: "relative-friction-dir",
		armedVerb:   armedVerb,
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}},
	}
}

// TestReflectFrictionRow_SkipsWhenTierTwoOff asserts an empty frictionDir never reflects.
func TestReflectFrictionRow_SkipsWhenTierTwoOff(t *testing.T) {
	t.Parallel()

	c := newRelativeFrictionCLI(t, "run")
	c.frictionDir = ""

	if got := c.reflectFrictionRow(); got != frictionengine.StatusSkipped {
		t.Errorf("reflectFrictionRow() = %q; want %q", got, frictionengine.StatusSkipped)
	}
	if c.rowFrictionStatus != frictionengine.StatusSkipped {
		t.Errorf("rowFrictionStatus = %q; want %q", c.rowFrictionStatus, frictionengine.StatusSkipped)
	}
}

// TestReflectFrictionRow_SkipsWhenArmedForStep asserts step never reflects, so the notes stay put for
// ly-drive's operator-gated filing.
func TestReflectFrictionRow_SkipsWhenArmedForStep(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	note := filepath.Join(dir, "note.md")
	if err := os.WriteFile(note, []byte("a note"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", note, err)
	}
	c := newRelativeFrictionCLI(t, "step")
	c.frictionDir = dir

	if got := c.reflectFrictionRow(); got != frictionengine.StatusSkipped {
		t.Errorf("reflectFrictionRow() = %q; want %q", got, frictionengine.StatusSkipped)
	}
	if _, err := os.Stat(note); err != nil {
		t.Errorf("Stat(%q) = %v; want the note to survive a step-armed row", note, err)
	}
}

// TestReflectFrictionRow_ReflectsWhenArmedForRun asserts run with Tier 2 on attempts the reflection.
func TestReflectFrictionRow_ReflectsWhenArmedForRun(t *testing.T) {
	t.Parallel()

	c := newRelativeFrictionCLI(t, "run")

	if got := c.reflectFrictionRow(); got != frictionengine.StatusFailed {
		t.Errorf("reflectFrictionRow() = %q; want %q", got, frictionengine.StatusFailed)
	}
	if c.rowFrictionStatus != frictionengine.StatusFailed {
		t.Errorf("rowFrictionStatus = %q; want %q", c.rowFrictionStatus, frictionengine.StatusFailed)
	}
}

// TestReflectFrictionRow_WaitsOnAHeldReflectionLock asserts the row holds done back while another
// reflection holds the lock, then completes once it is released.
func TestReflectFrictionRow_WaitsOnAHeldReflectionLock(t *testing.T) {
	t.Parallel()

	c := newRelativeFrictionCLI(t, "run")

	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.lock")
	runLockPath := filepath.Join(dir, "run.lock")
	if err := loomshed.Seed(statusPath, statusLockPath, "warp", "main"); err != nil {
		t.Fatalf("Seed() = %v; want nil", err)
	}
	err := state.UpdateJSON[shedengine.Status](statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = loomshed.NameFrictionReflect
		cur.State = shedengine.StateRunning
		return cur, nil
	})
	if err != nil {
		t.Fatalf("UpdateJSON() = %v; want nil", err)
	}

	p, err := loomshed.NewFrictionReflect(loomshed.NameFrictionReflect, c.reflectFrictionRow)
	if err != nil {
		t.Fatalf("NewFrictionReflect() = %v; want nil", err)
	}
	shed := &shedengine.Shed{
		Producers:      []shedengine.ProducerDef{{Name: loomshed.NameFrictionReflect, Producer: p}},
		StatusPath:     statusPath,
		LockPath:       runLockPath,
		StatusLockPath: statusLockPath,
	}

	frictionLock := loomengine.LoomFrictionLock(c.location)
	if err := os.MkdirAll(filepath.Dir(frictionLock), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(frictionLock)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock(%q) = acquired %v, err %v; want acquired", frictionLock, acquired, err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = held.Release()
		}
	})

	type runOutcome struct {
		result shedengine.Result
		err    error
	}
	done := make(chan runOutcome, 1)
	go func() {
		res, err := shed.Run(context.Background())
		done <- runOutcome{res, err}
	}()

	select {
	case got := <-done:
		t.Fatalf("Run returned %+v while the reflection lock was held; want it to wait", got)
	case <-time.After(300 * time.Millisecond):
	}
	st, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("ReadJSON() = %v; want nil", err)
	}
	if st.State != shedengine.StateRunning || st.CurrentProducer != loomshed.NameFrictionReflect {
		t.Errorf("status = %s at %s while waiting; want running at %s", st.State, st.CurrentProducer, loomshed.NameFrictionReflect)
	}

	released = true
	_ = held.Release()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Run() error = %v; want nil", got.err)
		}
		if got.result.Outcome != shedengine.RunDone {
			t.Errorf("Run outcome = %q; want %q", got.result.Outcome, shedengine.RunDone)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after the reflection lock was released")
	}
	st, _, err = state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("ReadJSON() = %v; want nil", err)
	}
	if st.State != shedengine.StateDone {
		t.Errorf("persisted state = %s; want %s", st.State, shedengine.StateDone)
	}
	if c.rowFrictionStatus != frictionengine.StatusFailed {
		t.Errorf("rowFrictionStatus = %q; want %q", c.rowFrictionStatus, frictionengine.StatusFailed)
	}
}

// TestLoomPostRun_DoneReportsTheRowStatusWithoutReflecting asserts RunDone reports what the row
// recorded and never reflects itself.
func TestLoomPostRun_DoneReportsTheRowStatusWithoutReflecting(t *testing.T) {
	t.Parallel()

	c := newRelativeFrictionCLI(t, "run")
	c.rowFrictionStatus = "reflected"
	done := shedengine.Result{Outcome: shedengine.RunDone}

	if got := c.loomPostRun(context.Background(), done, nil)["friction"]; got != "reflected" {
		t.Errorf("friction = %v; want %q", got, "reflected")
	}

	c.rowFrictionStatus = ""
	if got := c.loomPostRun(context.Background(), done, nil)["friction"]; got != frictionengine.StatusSkipped {
		t.Errorf("friction = %v; want %q (a reflection attempt would report %q)", got, frictionengine.StatusSkipped, frictionengine.StatusFailed)
	}
}

// TestLoomPostRun_BlockedStillReflects asserts the blocked path keeps its own reflection.
func TestLoomPostRun_BlockedStillReflects(t *testing.T) {
	t.Parallel()

	blocked := shedengine.Result{Outcome: shedengine.RunBlocked}
	c := newRelativeFrictionCLI(t, "run")
	if got := c.loomPostRun(context.Background(), blocked, nil)["friction"]; got != frictionengine.StatusFailed {
		t.Errorf("friction = %v; want %q", got, frictionengine.StatusFailed)
	}

	c.frictionDir = ""
	if got := c.loomPostRun(context.Background(), blocked, nil)["friction"]; got != frictionengine.StatusSkipped {
		t.Errorf("friction = %v; want %q", got, frictionengine.StatusSkipped)
	}
}
