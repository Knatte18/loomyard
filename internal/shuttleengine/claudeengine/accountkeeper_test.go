// accountkeeper_test.go tests the Claude engine's pick of a new worktree's account.

package claudeengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestClaude_PickAccount pins when a pick writes, what it writes and what it leaves alone.
func TestClaude_PickAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	clock := time.Unix(2_000_000, 0)
	earlier, later := clock.Add(-time.Hour), clock.Add(time.Hour)
	// withPins is a machine-layer claude.yaml of the accounts a and b under the given weights and pins.
	withPins := func(weightA, weightB int, pins string) func(root string) string {
		return func(root string) string {
			return fmt.Sprintf("accounts:\n  - {name: a, config_dir: %s, weight: %d}\n  - {name: b, config_dir: %s, weight: %d}\n%s",
				filepath.Join(root, "a"), weightA, filepath.Join(root, "b"), weightB, pins)
		}
	}
	both := withPins(1, 1, "")
	task := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}
	listedAB := func(counts map[string]float64) accountFile {
		return accountFile{Listed: []string{"a", "b"}, Weights: map[string]int{"a": 1, "b": 1}, Counts: counts}
	}

	type pickRow struct {
		name     string
		machine  func(root string) string
		worktree shuttleengine.Worktree
		// namesErr is what the hub's worktree names call fails with.
		namesErr  error
		file      accountFile
		now       time.Time
		keepEntry bool
		// namesUnderLock makes the names call fail the test unless the account file's lock is held while it runs.
		namesUnderLock bool
		// noLogin leaves the accounts without credentials files.
		noLogin bool
		wantErr bool
		// check judges the account file after the pick; a nil check demands that no file was written.
		check func(t *testing.T, file accountFile)
	}
	tests := []pickRow{
		{name: "OneAccountOfPositiveWeightWritesNothing", machine: withPins(1, 0, ""), worktree: task},
		{name: "ThePrimeWritesNothing", machine: both, worktree: shuttleengine.Worktree{Name: "prime", Kind: shuttleengine.WorktreePrime}},
		{name: "AHubPinWritesNothing", machine: withPins(1, 1, "account_pins:\n  hub: a\n"), worktree: task},
		{name: "ABeamUnderABeamsPinWritesNothing", machine: withPins(1, 1, "account_pins:\n  beams: a\n"), worktree: shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeBeam}},
		{
			name: "ATaskWorktreeGetsAnEntryAndItsAccountsCountRisesByOne", machine: both, worktree: task, now: clock,
			check: func(t *testing.T, file accountFile) {
				account := file.Worktrees["t"]
				if (account != "a" && account != "b") || file.Counts[account] != 1 || file.Counts["a"]+file.Counts["b"] != 1 || !file.Picked["t"].Equal(clock) {
					t.Errorf("account file = %+v; want an entry for t on a or b, its count 1, and Picked t at %v", file, clock)
				}
			},
		},
		{
			name: "ATaskUnderABeamsPinIsPicked", machine: withPins(1, 1, "account_pins:\n  beams: a\n"), worktree: task, now: clock,
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] == "" {
					t.Errorf("account file = %+v; want an entry for the task, which a beams pin does not cover", file)
				}
			},
		},
		{
			name: "AnEntryOfAWorktreeTheHubLacksIsDroppedAndOneItHoldsIsKept", machine: both, worktree: task, now: clock,
			file: accountFile{Worktrees: map[string]string{"gone": "a", "other": "b"}, Picked: map[string]time.Time{"gone": earlier, "other": earlier}},
			check: func(t *testing.T, file accountFile) {
				want := map[string]string{"other": "b"}
				delete(file.Worktrees, "t")
				if !reflect.DeepEqual(file.Worktrees, want) {
					t.Errorf("entries besides t's = %v; want %v", file.Worktrees, want)
				}
			},
		},
		{
			name: "APickOverAnExistingEntryReplacesIt", machine: both, worktree: task, now: clock,
			file: accountFile{
				Worktrees: map[string]string{"t": "a"}, Picked: map[string]time.Time{"t": earlier},
				Listed: []string{"a", "b"}, Weights: map[string]int{"a": 1, "b": 1}, Counts: map[string]float64{"a": 5, "b": 0},
			},
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] != "b" || file.Counts["b"] != 1 || !file.Picked["t"].Equal(clock) {
					t.Errorf("account file = %+v; want t moved onto b, whose count rose to 1, picked at %v", file, clock)
				}
			},
		},
		{
			name: "APickNotedBeforeAnEarlierPicksWriteKeepsItsEntryAndAddsNoCount", machine: both, worktree: task, now: clock,
			file: accountFile{
				Worktrees: map[string]string{"t": "a"}, Picked: map[string]time.Time{"t": later},
				Listed: []string{"a", "b"}, Weights: map[string]int{"a": 1, "b": 1}, Counts: map[string]float64{"a": 7},
			},
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] != "a" || !reflect.DeepEqual(file.Counts, map[string]float64{"a": 7}) || !file.Picked["t"].Equal(later) {
					t.Errorf("account file = %+v; want the earlier pick's entry, count and time unchanged", file)
				}
			},
		},
		{name: "AFailedNamesCallWritesNothing", machine: both, worktree: task, namesErr: errors.New("git worktree list failed"), wantErr: true},
		{
			name: "TheHubsWorktreeNamesAreReadUnderTheAccountFilesLock", machine: both, worktree: task, now: clock, namesUnderLock: true,
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] == "" {
					t.Errorf("account file = %+v; want an entry for t", file)
				}
			},
		},
		{
			name: "WithKeepEntryAnEntryStaysAndAddsNoCount", machine: both, worktree: task, now: clock, keepEntry: true,
			file: accountFile{Worktrees: map[string]string{"t": "a"}, Picked: map[string]time.Time{"t": earlier}, Counts: map[string]float64{"a": 7}},
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] != "a" || !reflect.DeepEqual(file.Counts, map[string]float64{"a": 7}) {
					t.Errorf("account file = %+v; want t kept on a with the counts unchanged", file)
				}
			},
		},
		{
			name: "WithKeepEntryAWorktreeWithoutAnEntryIsPicked", machine: both, worktree: task, now: clock, keepEntry: true,
			file: listedAB(map[string]float64{"a": 2, "b": 0}),
			check: func(t *testing.T, file accountFile) {
				if file.Worktrees["t"] != "b" || file.Counts["b"] != 1 {
					t.Errorf("account file = %+v; want t picked onto b", file)
				}
			},
		},
	}
	if runtime.GOOS != "darwin" {
		tests = append(tests, pickRow{
			name: "NoLoggedInCandidateRemovesTheEntry", machine: both, worktree: task, now: clock, noLogin: true,
			file: accountFile{Worktrees: map[string]string{"t": "a"}, Picked: map[string]time.Time{"t": earlier}},
			check: func(t *testing.T, file accountFile) {
				if _, held := file.Worktrees["t"]; held || len(file.Counts) != 0 {
					t.Errorf("account file = %+v; want no entry for t and no count", file)
				}
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, board, anchor, accountDir := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			for _, name := range []string{"a", "b"} {
				if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
					t.Fatalf("create the config directory of %s: %v", name, err)
				}
				if !tt.noLogin {
					if err := os.WriteFile(filepath.Join(root, name, credentialsFileName), nil, 0o600); err != nil {
						t.Fatalf("write the credentials file of %s: %v", name, err)
					}
				}
			}
			writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate())
			writeClaudeLayer(t, configengine.MachineFile(board, "claude"), tt.machine(root))

			worktree := tt.worktree
			worktree.AccountDir = accountDir
			worktree.Names = func() ([]string, error) {
				if tt.namesUnderLock {
					probe, acquired, err := lock.TryAcquireWriteLock(lockPathOf(accountFilePath(accountDir)))
					switch {
					case err != nil:
						t.Errorf("probe the account file's lock: %v", err)
					case acquired:
						_ = probe.Release()
						t.Error("the account file's lock was free while the hub's worktree names were read; want it held")
					}
				}
				return []string{"prime", "t", "other"}, tt.namesErr
			}
			engine := NewForWorktree(shuttleengine.Config{}, configengine.Dirs{Board: board, Anchor: anchor}, worktree)
			engine.now = func() time.Time { return tt.now }
			if !reflect.DeepEqual(tt.file, accountFile{}) {
				if err := updateAccountFile(accountDir, func(accountFile) (accountFile, error) { return tt.file, nil }); err != nil {
					t.Fatalf("seed the account file: %v", err)
				}
			}

			err := engine.PickAccount(tt.keepEntry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PickAccount(%v) = %v; want error %v", tt.keepEntry, err, tt.wantErr)
			}
			file, readErr := readAccountFile(accountDir)
			if readErr != nil {
				t.Fatalf("read the account file: %v", readErr)
			}
			if tt.check == nil {
				if !reflect.DeepEqual(file, tt.file) {
					t.Errorf("account file = %+v; want it as it was, %+v", file, tt.file)
				}
				return
			}
			tt.check(t, file)
		})
	}
}
