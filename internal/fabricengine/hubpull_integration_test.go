//go:build tier1

// hubpull_integration_test.go drives PullHub against a hubforge hub whose origin a second clone moves:
// every side moved and then up to date, a stopped side beside the sides that still move, a beam with no upstream, each pair pulled inside its lock, and Bolt waiting for the board write lock.
// Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// The slugs of the pairs the hub pull tests add: a beam, a task pair that is not a beam, and a second beam.
const (
	hubPullBeam       = "b"
	hubPullTaskPair   = "t"
	hubPullSecondBeam = "b2"
)

// recordingPairLock returns a lock seam that records the worktree of every pair it locks, and the list it records into.
// For a worktree pulled names, the seam fails t unless the named file is absent when the lock is taken and present when it is released.
// This pins the pair's pull inside its lock.
func recordingPairLock(t *testing.T, pulled map[string]string) (fabricengine.PairLock, *[]string) {
	t.Helper()
	var locked []string
	return func(pair *lymcwd.Location) (func(), error) {
		locked = append(locked, pair.WorktreeName)
		path, checked := pulled[pair.WorktreeName]
		if checked {
			if _, err := os.Stat(path); err == nil {
				t.Errorf("%s holds %s before its lock is taken; want the pull inside the lock", pair.WorktreeName, path)
			}
		}
		return func() {
			if !checked {
				return
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s lacks %s when its lock is released; want the pull inside the lock", pair.WorktreeName, path)
			}
		}, nil
	}, &locked
}

// moveOrigin commits one file on the checked-out branch of dir and pushes it to the same-named branch of origin.
func moveOrigin(t *testing.T, dir, rel string) {
	t.Helper()
	gitkit.CommitFile(t, dir, rel, "moved on origin", "move "+rel)
	gitkit.MustRun(t, dir, "git", "push", "origin", "HEAD")
}

// moveBeamOnOrigin commits one file on the code branch of the beam slug through a fresh clone of codeBare and pushes it.
func moveBeamOnOrigin(t *testing.T, codeBare, slug string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "beam-clone")
	gitkit.MustRun(t, filepath.Dir(clone), "git", "clone", "--branch", slug, codeBare, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	moveOrigin(t, clone, "beam-moved.txt")
}

// sideOutcomes returns each side as "kind worktree outcome", in pull order.
func sideOutcomes(sides []fabricengine.HubPullSide) []string {
	var out []string
	for _, side := range sides {
		out = append(out, string(side.Kind)+" "+side.Worktree+" "+string(side.Outcome))
	}
	return out
}

func TestPullHub_MovesEverySideOriginMovedThenReportsUpToDate(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, hubPullBeam, fabricengine.AddOptions{Beam: true})
	hubforge.AddPair(t, h, hubPullTaskPair)
	hubforge.AddPairWith(t, h, hubPullSecondBeam, fabricengine.AddOptions{Beam: true})
	second := hubforge.CloneSecond(t, h)
	moveOrigin(t, second.BoardDir(), "board-moved.txt")
	moveOrigin(t, second.PrimeWorktree(), "prime-code-moved.txt")
	moveBeamOnOrigin(t, h.CodeBare, hubPullBeam)
	moveBeamOnOrigin(t, h.CodeBare, hubPullSecondBeam)

	prime := h.Location.WorktreeName
	lockPair, locked := recordingPairLock(t, map[string]string{
		prime:             filepath.Join(h.PrimeWorktree(), "prime-code-moved.txt"),
		hubPullBeam:       filepath.Join(h.PairCodeWorktree(hubPullBeam), "beam-moved.txt"),
		hubPullSecondBeam: filepath.Join(h.PairCodeWorktree(hubPullSecondBeam), "beam-moved.txt"),
	})
	res, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{}, lockPair)
	if err != nil {
		t.Fatalf("PullHub() error = %v", err)
	}
	board := filepath.Base(h.BoardDir())
	want := []string{"board " + board + " moved", "prime " + prime + " moved", "beam " + hubPullBeam + " moved", "beam " + hubPullSecondBeam + " moved"}
	if got := sideOutcomes(res.Sides); !slices.Equal(got, want) {
		t.Errorf("PullHub() sides = %v; want %v", got, want)
	}
	if res.Stopped() {
		t.Errorf("PullHub() Stopped() = true; want false")
	}
	if want := []string{prime, hubPullBeam, hubPullSecondBeam}; !slices.Equal(*locked, want) {
		t.Errorf("locked pairs = %v; want %v", *locked, want)
	}
	for _, path := range []string{
		filepath.Join(h.BoardDir(), "board-moved.txt"),
		filepath.Join(h.PrimeWorktree(), "prime-code-moved.txt"),
		filepath.Join(h.PairCodeWorktree(hubPullBeam), "beam-moved.txt"),
		filepath.Join(h.PairCodeWorktree(hubPullSecondBeam), "beam-moved.txt"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Stat(%s) error = %v; want the pulled file", path, err)
		}
	}

	lockPair, _ = recordingPairLock(t, nil)
	again, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{}, lockPair)
	if err != nil {
		t.Fatalf("second PullHub() error = %v", err)
	}
	want = []string{"board " + board + " up_to_date", "prime " + prime + " up_to_date", "beam " + hubPullBeam + " up_to_date", "beam " + hubPullSecondBeam + " up_to_date"}
	if got := sideOutcomes(again.Sides); !slices.Equal(got, want) {
		t.Errorf("second PullHub() sides = %v; want %v", got, want)
	}
	if again.Mutated().Len() != 0 {
		t.Errorf("second PullHub() mutations = %v; want none", again.Mutated().Entries())
	}
}

func TestPullHub_StoppedSideLeavesTheOthersPulled(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, hubPullBeam, fabricengine.AddOptions{Beam: true})
	hubforge.AddPairWith(t, h, hubPullSecondBeam, fabricengine.AddOptions{SkipPush: true, Beam: true})
	second := hubforge.CloneSecond(t, h)
	moveOrigin(t, second.BoardDir(), "board-moved.txt")
	moveOrigin(t, second.PrimeWorktree(), "prime-code-moved.txt")
	moveBeamOnOrigin(t, h.CodeBare, hubPullBeam)
	tracked := strings.TrimSpace(gitkit.Git(t, h.PrimeWorktree(), "ls-files"))
	tracked = strings.Split(tracked, "\n")[0]
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), tracked), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPair, locked := recordingPairLock(t, nil)
	res, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{}, lockPair)
	if err != nil {
		t.Fatalf("PullHub() error = %v", err)
	}
	prime := h.Location.WorktreeName
	want := []string{"board " + filepath.Base(h.BoardDir()) + " moved", "prime " + prime + " stopped", "beam " + hubPullBeam + " moved", "beam " + hubPullSecondBeam + " up_to_date"}
	if got := sideOutcomes(res.Sides); !slices.Equal(got, want) {
		t.Fatalf("PullHub() sides = %v; want %v", got, want)
	}
	if !res.Stopped() {
		t.Errorf("PullHub() Stopped() = false; want true")
	}
	if got, want := res.Sides[1].Detail, fabricengine.ErrWarpDirty.Error(); got != want {
		t.Errorf("prime detail = %q; want %q", got, want)
	}
	if got, want := res.Sides[3].Detail, "the code branch has no upstream to pull from"; got != want {
		t.Errorf("beam detail = %q; want %q", got, want)
	}
	if want := []string{prime, hubPullBeam}; !slices.Equal(*locked, want) {
		t.Errorf("locked pairs = %v; want the prime and the moving beam, since a beam with no upstream takes no lock", *locked)
	}
	if _, err := os.Stat(filepath.Join(h.PairCodeWorktree(hubPullBeam), "beam-moved.txt")); err != nil {
		t.Errorf("Stat(beam-moved.txt) error = %v; want the beam after the stopped prime pulled", err)
	}
	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "prime-code-moved.txt")); err == nil {
		t.Errorf("the dirty prime's code moved; want it left where it was")
	}
}

func TestBoltPull_HoldsTheBoardWriteLockWhileItPulls(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	second := hubforge.CloneSecond(t, h)
	moveOrigin(t, second.BoardDir(), "board-moved.txt")
	// Holding the push lock, which Bolt.Pull takes after the board write lock, parks the pull with the board write lock already held.
	// The test then observes the lock instead of waiting out a clock.
	pushLock, err := lock.AcquireWriteLock(filepath.Join(h.BoardDir(), gitrepo.PushLockFileName))
	if err != nil {
		t.Fatalf("take the push lock: %v", err)
	}
	before := gitkit.RevParse(t, h.BoardDir(), "HEAD")

	type pullResult struct {
		skip fabricengine.BoltSkip
		err  error
	}
	done := make(chan pullResult, 1)
	go func() {
		skip, _, err := fabricengine.NewBolt(h.BoardDir()).Pull(fabricengine.SyncOptions{}, fabricengine.NewMutations(h.Location.HubPath))
		done <- pullResult{skip, err}
	}()

	writeLockPath := filepath.Join(h.BoardDir(), fabricengine.BoardWriteLockFile)
	deadline := time.Now().Add(30 * time.Second)
	for {
		contender, acquired, err := lock.TryAcquireWriteLock(writeLockPath)
		if err != nil {
			t.Fatalf("probe the board write lock: %v", err)
		}
		if !acquired {
			break
		}
		if err := contender.Release(); err != nil {
			t.Fatalf("release the probe: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("Bolt.Pull() never took the board write lock")
		}
		time.Sleep(time.Millisecond)
	}
	if got := gitkit.RevParse(t, h.BoardDir(), "HEAD"); got != before {
		t.Errorf("board HEAD = %s while the pull was parked; want it unchanged at %s", got, before)
	}

	if err := pushLock.Release(); err != nil {
		t.Fatalf("release the push lock: %v", err)
	}
	if got := <-done; got.skip != "" || got.err != nil {
		t.Fatalf("Bolt.Pull() = (%q, %v); want a clean pull once the push lock is released", got.skip, got.err)
	}
	if _, err := os.Stat(filepath.Join(h.BoardDir(), "board-moved.txt")); err != nil {
		t.Errorf("Stat(board-moved.txt) error = %v; want the pulled file", err)
	}
	free, acquired, err := lock.TryAcquireWriteLock(writeLockPath)
	if err != nil || !acquired {
		t.Fatalf("board write lock after the pull = (acquired %v, %v); want it released", acquired, err)
	}
	_ = free.Release()
}

// firstTrackedFile returns the first file git tracks in dir.
func firstTrackedFile(t *testing.T, dir string) string {
	t.Helper()
	return strings.Split(strings.TrimSpace(gitkit.Git(t, dir, "ls-files")), "\n")[0]
}

func TestPullHub_HonorsSkipGitWithEverySideUpToDate(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, hubPullBeam, fabricengine.AddOptions{Beam: true})
	second := hubforge.CloneSecond(t, h)
	moveOrigin(t, second.PrimeWorktree(), "prime-code-moved.txt")

	lockPair, _ := recordingPairLock(t, nil)
	res, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{SkipGit: true}, lockPair)
	if err != nil {
		t.Fatalf("PullHub() error = %v", err)
	}
	prime := h.Location.WorktreeName
	want := []string{"board " + filepath.Base(h.BoardDir()) + " up_to_date", "prime " + prime + " up_to_date", "beam " + hubPullBeam + " up_to_date"}
	if got := sideOutcomes(res.Sides); !slices.Equal(got, want) {
		t.Errorf("PullHub() sides = %v; want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "prime-code-moved.txt")); err == nil {
		t.Errorf("the prime pulled with git skipped; want it left where it was")
	}
}

func TestPullHub_UnreadableBeamRecordStopsOnlyThatBeam(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, hubPullBeam, fabricengine.AddOptions{Beam: true})
	hubforge.AddPairWith(t, h, hubPullSecondBeam, fabricengine.AddOptions{Beam: true})
	moveBeamOnOrigin(t, h.CodeBare, hubPullSecondBeam)

	broken, err := lymcwd.NamedWorktree(h.Location.HubPath, hubPullBeam)
	if err != nil {
		t.Fatalf("NamedWorktree(%s): %v", hubPullBeam, err)
	}
	if err := os.WriteFile(fabricengine.OriginRecordPathFor(broken, hubPullBeam), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPair, locked := recordingPairLock(t, nil)
	res, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{}, lockPair)
	if err != nil {
		t.Fatalf("PullHub() error = %v", err)
	}
	prime := h.Location.WorktreeName
	want := []string{"board " + filepath.Base(h.BoardDir()) + " up_to_date", "prime " + prime + " up_to_date", "beam " + hubPullBeam + " stopped", "beam " + hubPullSecondBeam + " moved"}
	if got := sideOutcomes(res.Sides); !slices.Equal(got, want) {
		t.Fatalf("PullHub() sides = %v; want %v", got, want)
	}
	if !strings.Contains(res.Sides[2].Detail, "origin record") {
		t.Errorf("unreadable beam detail = %q; want it to name the origin record", res.Sides[2].Detail)
	}
	if want := []string{prime, hubPullSecondBeam}; !slices.Equal(*locked, want) {
		t.Errorf("locked pairs = %v; want %v", *locked, want)
	}
}

func TestPullHub_StopsABoardSideAndAPairSideWithTheirDetail(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, hubPullBeam, fabricengine.AddOptions{Beam: true})
	second := hubforge.CloneSecond(t, h)
	moveOrigin(t, second.BoardDir(), "board-moved.txt")
	tracked := firstTrackedFile(t, h.BoardDir())
	if err := os.WriteFile(filepath.Join(h.BoardDir(), tracked), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, h.PairRecordsSibling(hubPullBeam), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))

	lockPair, _ := recordingPairLock(t, nil)
	res, err := fabricengine.PullHub(h.Location, fabricengine.SyncOptions{}, lockPair)
	if err != nil {
		t.Fatalf("PullHub() error = %v", err)
	}
	prime := h.Location.WorktreeName
	want := []string{"board " + filepath.Base(h.BoardDir()) + " stopped", "prime " + prime + " up_to_date", "beam " + hubPullBeam + " stopped"}
	if got := sideOutcomes(res.Sides); !slices.Equal(got, want) {
		t.Fatalf("PullHub() sides = %v; want %v", got, want)
	}
	for _, side := range res.Sides {
		if side.Outcome == fabricengine.HubPullStopped && side.Detail == "" {
			t.Errorf("%s side detail is empty; want the way forward", side.Kind)
		}
	}
	if got, want := res.Sides[2].Detail, "the records side's origin could not be reached; re-run the pull once it is reachable"; got != want {
		t.Errorf("beam detail = %q; want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(h.BoardDir(), "board-moved.txt")); err == nil {
		t.Errorf("the dirty board pulled; want it left where it was")
	}
}
