// coalesce_test.go covers coalescePush's loop-exit contract with scripted step closures: no real
// git is spawned, only a flock on a t.TempDir() path, so this file stays untagged per the Test Tier
// Purity Invariant.

package fabricengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestCoalescePush_LoopsWhileProgressed asserts the loop-exit contract: a step returning
// progressed=true a fixed number of times before returning false runs exactly one more time than
// the number of true returns (the final, no-progress call that ends the loop), and a step that
// reports no-progress on its very first call runs exactly once — proving the loop does not spin
// depending on any external "still unpushed" signal, only on step's own return value.
//
//testtiming:keep coalescePush's loop-exit contract driven only by the step's own progressed return; coverage of its blocks by other tests does not show an assertion of this
func TestCoalescePush_LoopsWhileProgressed(t *testing.T) {
	tests := []struct {
		name          string
		progressCount int
		wantCalls     int
	}{
		{"NoProgressImmediately", 0, 1},
		{"ProgressesTwiceThenStops", 2, 3},
		{"ProgressesFiveTimesThenStops", 5, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockPath := filepath.Join(t.TempDir(), "push.lock")

			calls := 0
			step := func() (bool, error) {
				calls++
				return calls <= tt.progressCount, nil
			}

			if err := coalescePush(lockPath, step); err != nil {
				t.Fatalf("coalescePush() error = %v; want nil", err)
			}
			if calls != tt.wantCalls {
				t.Errorf("step call count = %d; want %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestCoalescePush_StepErrorAbortsImmediately asserts a step returning a non-nil error stops the
// loop on that call — no further step calls — and coalescePush returns that exact error to the
// caller.
func TestCoalescePush_StepErrorAbortsImmediately(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "push.lock")
	wantErr := errors.New("boom")

	calls := 0
	step := func() (bool, error) {
		calls++
		return true, wantErr
	}

	err := coalescePush(lockPath, step)
	if !errors.Is(err, wantErr) {
		t.Fatalf("coalescePush() error = %v; want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("step call count = %d; want 1 (loop must abort on the first error)", calls)
	}
}

// TestCoalescePush_ReleasesLockOnReturn asserts coalescePush releases its absorbing lock before
// returning: a second, independent AcquireWriteLock on the same path must succeed without blocking
// once coalescePush has returned.
//
//testtiming:keep coalescePush releasing its absorbing lock before it returns; coverage of its blocks by other tests does not show an assertion of this
func TestCoalescePush_ReleasesLockOnReturn(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "push.lock")

	step := func() (bool, error) { return false, nil }
	if err := coalescePush(lockPath, step); err != nil {
		t.Fatalf("coalescePush() error = %v; want nil", err)
	}

	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock() after coalescePush returned error = %v; want nil (lock must be released)", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

// pendingFlagFixture is a hub holding one records worktree and its paired code worktree, with the records worktree's lock directory created.
type pendingFlagFixture struct {
	hub      string
	records  string
	code     string
	lockDir  string
	lockPath string
}

// encodeFlag returns flag as the JSON a pending flag file holds.
func encodeFlag(t *testing.T, flag PushSpawn) []byte {
	t.Helper()
	content, err := json.Marshal(flag)
	if err != nil {
		t.Fatalf("Marshal(%+v): %v", flag, err)
	}
	return content
}

// newPendingFlagFixture lays out hub/feat and hub/feat-weft, the pair a flag's paths are validated against.
func newPendingFlagFixture(t *testing.T) pendingFlagFixture {
	t.Helper()
	hub := t.TempDir()
	records := weftname.SiblingPath(hub, "feat")
	lockDir := RecordsLockDirPath(records)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", lockDir, err)
	}
	return pendingFlagFixture{
		hub:      hub,
		records:  records,
		code:     filepath.Join(hub, "feat"),
		lockDir:  lockDir,
		lockPath: filepath.Join(lockDir, weftPushLockFile),
	}
}

// TestPushDeferral_PutsBackTheUnpushedFlags covers the deferral a detached push child installs:
// every owed side not yet pushed gets its flag back with its path and slugs, a pushed side's flag stays off, and the exit runs once.
func TestPushDeferral_PutsBackTheUnpushedFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		pushed    map[string]bool
		wantFlags []string
	}{
		{"NeitherPushed", map[string]bool{}, []string{pushSideCode, pushSideRecords}},
		{"CodePushed", map[string]bool{pushSideCode: true}, []string{pushSideRecords}},
		{"BothPushed", map[string]bool{pushSideCode: true, pushSideRecords: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newPendingFlagFixture(t)
			owed := map[string]PushSpawn{
				pushSideCode:    {WarpPath: f.code, PauseSlug: "feat", TeardownSlug: "feat", ParentLocked: true},
				pushSideRecords: {WeftPath: f.records, PauseSlug: "feat", TeardownSlug: "feat", ParentLocked: true},
			}
			exits := 0
			pushDeferral(f.lockDir, owed, tt.pushed, func() { exits++ })()

			if exits != 1 {
				t.Errorf("exit ran %d times; want 1", exits)
			}
			for _, side := range []string{pushSideCode, pushSideRecords} {
				flag, found, err := takePending(f.lockDir, side)
				if err != nil {
					t.Fatalf("takePending(%s): %v", side, err)
				}
				wantFound := slices.Contains(tt.wantFlags, side)
				if found != wantFound {
					t.Errorf("flag of %s on disk = %v; want %v", side, found, wantFound)
				}
				if found && flag != owed[side] {
					t.Errorf("flag of %s = %+v; want %+v", side, flag, owed[side])
				}
			}
		})
	}
}

// TestCoalesceFlagged_PendingFlags covers the pending-flag protocol over a temp lock directory and a step closure:
// a missed first acquire leaves the flag to the holder;
// a flag set during a step makes the holder run again;
// a foreign side's flag is respawned only when it validates;
// and an owed side's flag is only ever taken.
// It swaps the package's spawn variable and the logger's output, so it is not parallel.
func TestCoalesceFlagged_PendingFlags(t *testing.T) {
	tryAcquire := lock.TryAcquireWriteLock

	t.Run("HeldLockLeavesFlagAndRunsNoStep", func(t *testing.T) {
		f := newPendingFlagFixture(t)
		other, err := lock.AcquireWriteLock(f.lockPath)
		if err != nil {
			t.Fatalf("AcquireWriteLock: %v", err)
		}
		defer func() { _ = other.Release() }()

		steps := 0
		step := func() (bool, error) { steps++; return false, nil }
		owed := map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}
		acquired, err := coalesceFlagged(f.lockDir, owed, tryAcquire, step)
		if acquired || err != nil {
			t.Errorf("coalesceFlagged() = (%v, %v); want (false, nil)", acquired, err)
		}
		if steps != 0 {
			t.Errorf("step ran %d times; want 0", steps)
		}
		if _, err := os.Stat(pendingFlagPath(f.lockDir, pushSideRecords)); err != nil {
			t.Errorf("pending.records after a missed acquire: %v; want it left on disk", err)
		}
	})

	t.Run("FlagSetDuringStepRunsStepAgain", func(t *testing.T) {
		f := newPendingFlagFixture(t)
		owed := map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}
		steps := 0
		step := func() (bool, error) {
			steps++
			if steps == 1 {
				if err := markPending(f.lockDir, owed); err != nil {
					t.Fatalf("markPending: %v", err)
				}
			}
			return false, nil
		}
		acquired, err := coalesceFlagged(f.lockDir, owed, tryAcquire, step)
		if !acquired || err != nil {
			t.Errorf("coalesceFlagged() = (%v, %v); want (true, nil)", acquired, err)
		}
		if steps != 2 {
			t.Errorf("step ran %d times; want 2", steps)
		}
		if _, err := os.Stat(pendingFlagPath(f.lockDir, pushSideRecords)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("pending.records after the call: %v; want it gone", err)
		}
	})

	t.Run("MissedReacquireAfterStepReportsAcquiredAndKeepsFlag", func(t *testing.T) {
		f := newPendingFlagFixture(t)
		owed := map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}
		step := func() (bool, error) { return false, markPending(f.lockDir, owed) }
		calls := 0
		acquire := func(lockPath string) (*lock.FileLock, bool, error) {
			calls++
			if calls > 1 {
				return nil, false, nil
			}
			return tryAcquire(lockPath)
		}
		acquired, err := coalesceFlagged(f.lockDir, owed, acquire, step)
		if !acquired || err != nil {
			t.Errorf("coalesceFlagged() = (%v, %v); want (true, nil)", acquired, err)
		}
		if _, err := os.Stat(pendingFlagPath(f.lockDir, pushSideRecords)); err != nil {
			t.Errorf("pending.records after a missed re-acquire: %v; want it left for the current holder", err)
		}
	})

	t.Run("StepErrorReleasesTheLock", func(t *testing.T) {
		f := newPendingFlagFixture(t)
		wantErr := errors.New("boom")
		step := func() (bool, error) { return false, wantErr }
		_, err := coalesceFlagged(f.lockDir, map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}, tryAcquire, step)
		if !errors.Is(err, wantErr) {
			t.Fatalf("coalesceFlagged() error = %v; want %v", err, wantErr)
		}
		again, ok, err := tryAcquire(f.lockPath)
		if err != nil || !ok {
			t.Fatalf("second try-acquire = (%v, %v); want it to succeed", ok, err)
		}
		_ = again.Release()
	})

	t.Run("OwedFlagsAreOnlyTaken", func(t *testing.T) {
		tests := []struct {
			name  string
			owed  func(f pendingFlagFixture) map[string]PushSpawn
			side  string
			bytes func(f pendingFlagFixture) []byte
		}{
			{
				name: "CorruptRecordsFlagSetByStep",
				owed: func(f pendingFlagFixture) map[string]PushSpawn {
					return map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}
				},
				side:  pushSideRecords,
				bytes: func(pendingFlagFixture) []byte { return []byte("{not json") },
			},
			{
				name: "CodeFlagOfAnUnpairedPath",
				owed: func(f pendingFlagFixture) map[string]PushSpawn {
					return map[string]PushSpawn{pushSideCode: {WarpPath: filepath.Join(f.hub, "unpaired")}}
				},
				side: pushSideCode,
				bytes: func(f pendingFlagFixture) []byte {
					return encodeFlag(t, PushSpawn{WarpPath: filepath.Join(f.hub, "unpaired")})
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f := newPendingFlagFixture(t)
				var logged bytes.Buffer
				logger.SetOutput(&logged)
				t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
				var spawned []PushSpawn
				original := spawnDetachedPushFn
				spawnDetachedPushFn = func(req PushSpawn) error { spawned = append(spawned, req); return nil }
				t.Cleanup(func() { spawnDetachedPushFn = original })

				steps := 0
				step := func() (bool, error) {
					steps++
					if steps == 1 {
						if err := os.WriteFile(pendingFlagPath(f.lockDir, tt.side), tt.bytes(f), 0o644); err != nil {
							t.Fatalf("WriteFile: %v", err)
						}
					}
					return false, nil
				}
				acquired, err := coalesceFlagged(f.lockDir, tt.owed(f), tryAcquire, step)
				if !acquired || err != nil {
					t.Errorf("coalesceFlagged() = (%v, %v); want (true, nil)", acquired, err)
				}
				if steps != 2 {
					t.Errorf("step ran %d times; want 2 (an owed side's flag set during the step is taken and pushed again)", steps)
				}
				if _, err := os.Stat(pendingFlagPath(f.lockDir, tt.side)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("flag after the call: %v; want it gone", err)
				}
				if len(spawned) != 0 {
					t.Errorf("spawned %+v; want nothing", spawned)
				}
				if strings.Contains(logged.String(), "WARN") {
					t.Errorf("logged %q; want no warning for an owed side's flag", logged.String())
				}
			})
		}
	})

	t.Run("ForeignFlagRespawn", func(t *testing.T) {
		for _, side := range []string{pushSideCode, pushSideRecords} {
			t.Run(side, func(t *testing.T) {
				tests := []struct {
					name string
					// edit changes the valid flag; raw, when set, replaces its encoded bytes.
					edit      func(f pendingFlagFixture, flag *PushSpawn)
					raw       []byte
					wantSpawn bool
					// wantWarn is a substring of the Warn, empty when none is expected.
					wantWarn string
				}{
					{name: "ValidFlagWithForeignWorkDir", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.WorkDir = filepath.Join(f.hub, "elsewhere") }, wantSpawn: true},
					{name: "ParentLockedMarkKept", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.ParentLocked = true }, wantSpawn: true},
					{name: "OppositeSidePathDropped", edit: func(f pendingFlagFixture, flag *PushSpawn) { setOppositePath(side, f, flag) }, wantSpawn: true},
					{name: "PathOfAnotherWorktree", edit: func(f pendingFlagFixture, flag *PushSpawn) {
						setSidePath(side, filepath.Join(f.hub, "badworktree"), flag)
					}, wantWarn: "badworktree"},
					{name: "PauseSlugDotDot", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.PauseSlug = ".." }, wantWarn: "pending." + side},
					{name: "PauseSlugWithSeparator", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.PauseSlug = "a/b" }, wantWarn: "pending." + side},
					{name: "TeardownSlugDotDot", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.TeardownSlug = ".." }, wantWarn: "pending." + side},
					{name: "TeardownSlugWithSeparator", edit: func(f pendingFlagFixture, flag *PushSpawn) { flag.TeardownSlug = "a/b" }, wantWarn: "pending." + side},
					{name: "NotJSON", raw: []byte("{not json"), wantWarn: "pending." + side},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						f := newPendingFlagFixture(t)
						var logged bytes.Buffer
						logger.SetOutput(&logged)
						t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
						var spawned []PushSpawn
						original := spawnDetachedPushFn
						spawnDetachedPushFn = func(req PushSpawn) error { spawned = append(spawned, req); return nil }
						t.Cleanup(func() { spawnDetachedPushFn = original })

						flag := PushSpawn{PauseSlug: "pause", TeardownSlug: "teardown"}
						setSidePath(side, validSidePath(side, f), &flag)
						if tt.edit != nil {
							tt.edit(f, &flag)
						}
						content := encodeFlag(t, flag)
						if tt.raw != nil {
							content = tt.raw
						}
						if err := os.WriteFile(pendingFlagPath(f.lockDir, side), content, 0o644); err != nil {
							t.Fatalf("WriteFile: %v", err)
						}

						// The holder owes the side the flag is not for.
						owed := map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}
						if side == pushSideRecords {
							owed = map[string]PushSpawn{pushSideCode: {WarpPath: f.code}}
						}
						steps := 0
						step := func() (bool, error) { steps++; return false, nil }
						acquired, err := coalesceFlagged(f.lockDir, owed, tryAcquire, step)
						if !acquired || err != nil {
							t.Fatalf("coalesceFlagged() = (%v, %v); want (true, nil)", acquired, err)
						}
						if steps != 1 {
							t.Errorf("step ran %d times; want 1", steps)
						}
						if _, err := os.Stat(pendingFlagPath(f.lockDir, side)); !errors.Is(err, os.ErrNotExist) {
							t.Errorf("flag after the call: %v; want it removed", err)
						}

						var want []PushSpawn
						if tt.wantSpawn {
							req := PushSpawn{PauseSlug: "pause", TeardownSlug: "teardown", ParentLocked: flag.ParentLocked, WorkDir: BoardDir(f.hub)}
							setSidePath(side, validSidePath(side, f), &req)
							// The child's coalescing push requires the records path, so a code-side respawn carries it too.
							req.WeftPath = f.records
							want = []PushSpawn{req}
						}
						if !reflect.DeepEqual(spawned, want) {
							t.Errorf("spawned %+v; want %+v", spawned, want)
						}
						if tt.wantWarn == "" && strings.Contains(logged.String(), "WARN") {
							t.Errorf("logged %q; want no warning", logged.String())
						}
						if tt.wantWarn != "" && !strings.Contains(logged.String(), tt.wantWarn) {
							t.Errorf("logged %q; want a warning naming %q", logged.String(), tt.wantWarn)
						}
					})
				}
			})
		}
	})
}

// validSidePath returns the path a valid flag of side carries in f: the paired code worktree or the records worktree.
func validSidePath(side string, f pendingFlagFixture) string {
	if side == pushSideCode {
		return f.code
	}
	return f.records
}

// setSidePath sets the path field flag carries for side.
func setSidePath(side, path string, flag *PushSpawn) {
	if side == pushSideCode {
		flag.WarpPath = path
		return
	}
	flag.WeftPath = path
}

// setOppositePath sets the path field of the side a flag of side does not carry.
func setOppositePath(side string, f pendingFlagFixture, flag *PushSpawn) {
	if side == pushSideCode {
		flag.WeftPath = f.records
		return
	}
	flag.WarpPath = f.code
}
