// seams_test.go drives the real reads and the lock behind the pusher's start, and its restart episodes.
// The fake-driven tests of Start and Run leave these out.

package runpusher

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pairTargetIn returns the target of the pair task in a hub under a fresh temp directory.
// The pair's worktree directory holds a gitdir file that reads as a linked worktree.
func pairTargetIn(t *testing.T) Target {
	t.Helper()

	hub := t.TempDir()
	worktree := filepath.Join(hub, "task")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("create the worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+filepath.Join(hub, "main", ".git", "worktrees", "task")+"\n"), 0o644); err != nil {
		t.Fatalf("write the gitdir file: %v", err)
	}
	return Target{Location: &lymcwd.Location{HubPath: hub, WorktreeName: "task"}, Slug: "task", Pair: true}
}

func TestPusherLock_ExcludesASecondPusherUntilReleased(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := LockPath(pairTargetIn(t))

	release, acquired, err := tryLock(path)
	if err != nil || !acquired {
		t.Fatalf("first tryLock() = _, %v, %v; want it acquired", acquired, err)
	}
	if free, err := newStarter().lockFree(path); err != nil || free {
		t.Errorf("lockFree() while held = %v, %v; want false, nil", free, err)
	}
	if _, second, err := tryLock(path); err != nil || second {
		t.Errorf("second tryLock() acquired = %v, %v; want false, nil", second, err)
	}

	release()

	if free, err := newStarter().lockFree(path); err != nil || !free {
		t.Errorf("lockFree() after the release = %v, %v; want true, nil", free, err)
	}
	third, acquired, err := tryLock(path)
	if err != nil || !acquired {
		t.Fatalf("third tryLock() = _, %v, %v; want it acquired after the release", acquired, err)
	}
	third()
}

// removals change a pair's worktree so that it no longer stands.
var removals = []struct {
	name   string
	remove func(path string) error
}{
	{"TheDirectoryIsGone", os.RemoveAll},
	{"TheDirectoryStandsWithoutItsGitEntry", func(path string) error { return os.Remove(filepath.Join(path, ".git")) }},
	{"ThePathIsARegularFile", func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return os.WriteFile(path, nil, 0o644)
	}},
}

func TestWorktreeGone_ReadsWhetherTheWorktreeStands(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := pairTargetIn(t).Location.WorktreePath()
	if gone, err := worktreeGone(path); err != nil || gone {
		t.Errorf("worktreeGone() of a standing worktree = %v, %v; want false, nil", gone, err)
	}
	for _, tt := range removals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := pairTargetIn(t).Location.WorktreePath()
			if err := tt.remove(path); err != nil {
				t.Fatalf("remove the worktree: %v", err)
			}

			if gone, err := worktreeGone(path); err != nil || !gone {
				t.Errorf("worktreeGone() = %v, %v; want true, nil", gone, err)
			}
		})
	}
}

// TestTargetOf_AWorktreeNoLongerStandingIsAPairThatStartsNoPusher covers a run whose pair no longer stands:
// its target reads as the pair with no error, and its start and refusal reads answer false with no error.
func TestTargetOf_AWorktreeNoLongerStandingIsAPairThatStartsNoPusher(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, tt := range removals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location := pairTargetIn(t).Location
			if err := tt.remove(location.WorktreePath()); err != nil {
				t.Fatalf("remove the worktree: %v", err)
			}

			target, err := TargetOf(location, "task")

			want := Target{Location: location, Slug: "task", Pair: true}
			if err != nil || target != want {
				t.Fatalf("TargetOf() = %+v, %v; want %+v, nil", target, err, want)
			}
			if started, err := StartIfAhead(target); err != nil || started {
				t.Errorf("StartIfAhead() = %v, %v; want false, nil", started, err)
			}
			if refused, err := TargetRefused(target); err != nil || refused {
				t.Errorf("TargetRefused() = %v, %v; want false, nil", refused, err)
			}
		})
	}
}

func TestHandedOver_ReadsTheMarkOfTheRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	target := pairTargetIn(t)
	if marked, err := handedOver(target); err != nil || marked {
		t.Errorf("handedOver() with no mark = %v, %v; want false, nil", marked, err)
	}

	mark := fabriclayout.HandOverMark{Machine: "oslo", At: "2026-11-15T09:00:00Z", Written: time.Now()}
	if err := fabricengine.WriteHandOverMark(target.Location.HubPath, target.Slug, mark); err != nil {
		t.Fatalf("write the hand-over mark: %v", err)
	}

	if marked, err := handedOver(target); err != nil || !marked {
		t.Errorf("handedOver() with the run's mark = %v, %v; want true, nil", marked, err)
	}
}

func TestPauseCovers_ReadsTheLiveEntryOfThePair(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// scope is the scope of the live entry written, empty for no record.
		scope string
		want  bool
	}{
		{"NoRecordCoversNothing", "", false},
		{"LiveEntryOfThePairCoversIt", "task", true},
		{"LiveEntryOfAnotherPairDoesNot", "other", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := pairTargetIn(t)
			if tt.scope != "" {
				geometry, err := hubgeom.PauseGeometry(target.Location)
				if err != nil {
					t.Fatalf("PauseGeometry() = %v; want nil", err)
				}
				content, err := json.Marshal(pauserecord.Record{Live: []pauserecord.Entry{{Scope: tt.scope, Caller: "EXTERNAL", PausedAt: time.Now().Add(-time.Minute)}}})
				if err != nil {
					t.Fatalf("marshal the pause record: %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(geometry.RecordPath), 0o755); err != nil {
					t.Fatalf("create the pause record directory: %v", err)
				}
				if err := os.WriteFile(geometry.RecordPath, content, 0o644); err != nil {
					t.Fatalf("write the pause record: %v", err)
				}
			}

			covered, err := pauseCovers(target)

			if err != nil || covered != tt.want {
				t.Errorf("pauseCovers() = %v, %v; want %v, nil", covered, err, tt.want)
			}
		})
	}
}

func TestRestartPusher_CountsEachFailureOfAnEpisodeAndClosesItOnASuccessfulStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const key = "restart-pusher-episode"
	failing := func() (bool, error) { return false, errors.New("no pusher") }

	RestartPusher(failing, key)
	if got := restartEpisodes.Ticks()[key]; got != 1 {
		t.Errorf("failed ticks after one failed start = %d; want 1", got)
	}
	RestartPusher(failing, key)
	if got := restartEpisodes.Ticks()[key]; got != 2 {
		t.Errorf("failed ticks after two failed starts = %d; want 2, one episode", got)
	}

	RestartPusher(func() (bool, error) { return true, nil }, key)

	if _, open := restartEpisodes.Ticks()[key]; open {
		t.Errorf("episode after a successful start is still open; want it closed")
	}
}
