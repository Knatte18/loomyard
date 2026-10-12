// cleanup_test.go covers CleanupHandoffs over told paths under a temp directory: which handoffs survive, when nothing is removed, and the state lock held against a cycle.

package orchengine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestCleanupHandoffs(t *testing.T) {
	t.Parallel()

	handoffs := []string{"h1.md", "h2.md", "h3.md", "partial.md"}
	type handoffCase struct {
		name string
		// state is saved before the run; nil leaves no state file.
		state         *State
		files         []string
		noHandoffsDir bool
		noOrchDir     bool
		wantKept      string
		wantRemoved   []string
		wantReason    string
	}
	tests := []handoffCase{
		{
			name:        "keeps the last handoff and removes the rest, a newer partial one included",
			state:       &State{Phase: PhaseIdle, LastHandoff: "h2.md"},
			files:       handoffs,
			wantKept:    "h2.md",
			wantRemoved: []string{"h1.md", "h3.md", "partial.md"},
		},
		{
			name:        "a last handoff recorded as a full path is matched by its file name",
			state:       &State{Phase: PhaseIdle, LastHandoff: filepath.Join("elsewhere", "h3.md")},
			files:       handoffs,
			wantKept:    "h3.md",
			wantRemoved: []string{"h1.md", "h2.md", "partial.md"},
		},
		{
			name:       "no recorded handoff removes nothing",
			state:      &State{Phase: PhaseIdle},
			files:      handoffs,
			wantReason: "no completed handoff",
		},
		{
			name:       "an absent state removes nothing",
			files:      handoffs,
			wantReason: "no completed handoff",
		},
		{
			name:       "a recorded handoff that is missing removes nothing",
			state:      &State{Phase: PhaseIdle, LastHandoff: "gone.md"},
			files:      handoffs,
			wantReason: "does not exist",
		},
		{
			name:       "an empty handoffs directory removes nothing",
			state:      &State{Phase: PhaseIdle, LastHandoff: "h1.md"},
			wantReason: "does not exist",
		},
		{
			name:          "a missing handoffs directory removes nothing",
			state:         &State{Phase: PhaseIdle, LastHandoff: "h1.md"},
			noHandoffsDir: true,
			wantReason:    "does not exist",
		},
		{
			name:      "a missing orch directory removes nothing and creates nothing",
			noOrchDir: true,
		},
	}
	for _, phase := range []Phase{PhaseHandoffRequested, PhaseClearing, PhaseResuming, PhaseCompacting} {
		tests = append(tests, handoffCase{
			name:       "phase " + string(phase) + " removes nothing",
			state:      &State{Phase: phase, LastHandoff: "h2.md"},
			files:      handoffs,
			wantReason: "phase " + string(phase),
		})
	}

	for _, tt := range tests {
		for _, apply := range []bool{false, true} {
			name := tt.name + "/dry run"
			if apply {
				name = tt.name + "/apply"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				p := testPaths(t)
				if !tt.noOrchDir {
					if tt.state != nil {
						if err := SaveState(p, *tt.state); err != nil {
							t.Fatalf("SaveState: %v", err)
						}
					}
					if !tt.noHandoffsDir {
						if err := os.MkdirAll(p.HandoffsDir, 0o755); err != nil {
							t.Fatalf("MkdirAll: %v", err)
						}
					}
					for _, file := range tt.files {
						if err := os.WriteFile(filepath.Join(p.HandoffsDir, file), []byte("note"), 0o644); err != nil {
							t.Fatalf("WriteFile: %v", err)
						}
					}
				}

				result, err := CleanupHandoffs(p, apply)
				if err != nil {
					t.Fatalf("CleanupHandoffs: %v", err)
				}

				if result.Kept != tt.wantKept {
					t.Errorf("Kept = %q, want %q", result.Kept, tt.wantKept)
				}
				if !slices.Equal(result.Removed, tt.wantRemoved) {
					t.Errorf("Removed = %v, want %v", result.Removed, tt.wantRemoved)
				}
				if (tt.wantReason == "") != (result.Reason == "") || !strings.Contains(result.Reason, tt.wantReason) {
					t.Errorf("Reason = %q, want it to contain %q", result.Reason, tt.wantReason)
				}

				if tt.noOrchDir {
					if _, err := os.Stat(p.Dir); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("the orch directory was created: %v", err)
					}
					return
				}
				wantLeft := tt.files
				if apply {
					wantLeft = nil
					for _, file := range tt.files {
						if !slices.Contains(tt.wantRemoved, file) {
							wantLeft = append(wantLeft, file)
						}
					}
				}
				var left []string
				entries, err := os.ReadDir(p.HandoffsDir)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ReadDir: %v", err)
				}
				for _, entry := range entries {
					left = append(left, entry.Name())
				}
				if !slices.Equal(left, wantLeft) {
					t.Errorf("handoffs left = %v, want %v", left, wantLeft)
				}
			})
		}
	}
}

func TestCleanupHandoffs_HoldsTheStateLockSoACycleLosesNoHandoff(t *testing.T) {
	t.Parallel()

	p := testPaths(t)
	if err := SaveState(p, State{Phase: PhaseIdle, LastHandoff: "h1.md"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(p.HandoffsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, file := range []string{"h1.md", "old.md"} {
		if err := os.WriteFile(filepath.Join(p.HandoffsDir, file), []byte("note"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	newHandoff := NewHandoffPath(p, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	cycleDone := make(chan struct{})
	beforeRemove := func() {
		go func() {
			defer close(cycleDone)
			// The watcher's transition: enter handoff-requested under the state lock, then write the new handoff.
			if err := updateState(p, func(s State) State {
				s.Phase = PhaseHandoffRequested
				s.PendingHandoff = newHandoff
				return s
			}); err != nil {
				t.Errorf("updateState: %v", err)
				return
			}
			if err := os.WriteFile(newHandoff, []byte("new note"), 0o644); err != nil {
				t.Errorf("write the new handoff: %v", err)
			}
		}()

		other, acquired, err := lock.TryAcquireWriteLock(p.StateLockPath)
		if err != nil {
			t.Errorf("probe the state lock: %v", err)
		}
		if acquired {
			_ = other.Release()
			t.Errorf("the state lock was free while cleanup was about to remove")
		}
	}

	result, err := cleanupHandoffs(p, true, beforeRemove)
	if err != nil {
		t.Fatalf("cleanupHandoffs: %v", err)
	}
	<-cycleDone

	if want := []string{"old.md"}; !slices.Equal(result.Removed, want) {
		t.Errorf("Removed = %v, want %v", result.Removed, want)
	}
	if _, err := os.Stat(newHandoff); err != nil {
		t.Errorf("the handoff the cycle wrote during cleanup was lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.HandoffsDir, "h1.md")); err != nil {
		t.Errorf("the last handoff was removed: %v", err)
	}
}
