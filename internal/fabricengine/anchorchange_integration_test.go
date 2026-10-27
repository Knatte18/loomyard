//go:build tier1

// anchorchange_integration_test.go covers the change of a worktree's anchor over hubforge hubs:
// the move, every refusal, the failures that undo it, the records a killed run leaves and the re-runs that finish them, the integration of a change a second clone made, and the read of a run status at a weft's upstream.
// A hub's prime warp tracks sub/ and other/ before any pair is added, so both are directories git tracks in every pair's warp.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/proc"
)

// anchorKill is what the kill hook panics with, standing in for a process that dies in the middle of a change.
type anchorKill struct{ afterCommit bool }

// newAnchorHub builds a "." hub whose prime warp tracks sub/ and other/ before any pair is added.
func newAnchorHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.PrimeWorktree(), "sub/.keep", "", "track sub")
	gitkit.CommitFile(t, h.PrimeWorktree(), "other/.keep", "", "track other")
	return h
}

// beginAnchorChange begins the change of slug's anchor to target from the anchor the hub records for slug.
func beginAnchorChange(t *testing.T, h *hubforge.Hub, slug, target string) (*fabricengine.AnchorChange, error) {
	t.Helper()

	pair, err := lymcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		t.Fatalf("NamedWorktree(%s): %v", slug, err)
	}
	return h.Topology.BeginAnchorChange(pair, target)
}

// completeAnchorChange applies and finishes c, which must have landed, and closes it.
// The detached push the finish spawns is refused under test, which is no failure.
func completeAnchorChange(t *testing.T, c *fabricengine.AnchorChange) {
	t.Helper()

	defer c.Close()
	rec := fabricengine.NewMutations("")
	landed, err := c.Apply(rec)
	if err != nil || !landed {
		t.Fatalf("Apply() = (landed %v, %v); want it landed with no error", landed, err)
	}
	if err := c.Finish(rec); err != nil && !errors.Is(err, fabricengine.ErrSpawnUnderTest) {
		t.Fatalf("Finish() error = %v; want nil", err)
	}
}

// killAt installs a hook that panics with anchorKill at the commit or the branch move, before it when afterCommit is false and after it otherwise.
func killAt(t *testing.T, afterCommit bool) {
	t.Helper()

	fabricengine.SetAnchorFailHookForTest(t, func(after bool) error {
		if after == afterCommit {
			panic(anchorKill{afterCommit})
		}
		return nil
	})
}

// applyUntilKilled applies c, which the installed hook must kill, and closes c, leaving the record standing and the move lock free as a dead process does.
func applyUntilKilled(t *testing.T, c *fabricengine.AnchorChange) {
	t.Helper()

	defer c.Close()
	defer func() {
		switch r := recover(); r.(type) {
		case nil:
			t.Errorf("Apply() returned; want the hook to kill it")
		case anchorKill:
		default:
			panic(r)
		}
	}()
	_, _ = c.Apply(fabricengine.NewMutations(""))
}

// wiredAt reports whether the wired name sits as a link at anchor in slug's warp worktree.
func wiredAt(t *testing.T, h *hubforge.Hub, slug, anchor, name string) bool {
	t.Helper()

	isLink, err := fslink.IsLink(filepath.Join(h.PairCodeWorktree(slug), anchor, name))
	if err != nil {
		t.Fatalf("inspect %s at %s: %v", name, anchor, err)
	}
	return isLink
}

// requireNoMoveRecord fails the test when the weft at weftPath carries a move record.
func requireNoMoveRecord(t *testing.T, weftPath string) {
	t.Helper()

	if record, found, err := fabricengine.ReadMoveRecord(weftPath); err != nil || found {
		t.Errorf("ReadMoveRecord() = (%+v, %v, %v); want no record", record, found, err)
	}
}

// requireWiredOnlyAt fails the test unless slug's junctions and scratch link sit at anchor and nowhere in other.
func requireWiredOnlyAt(t *testing.T, h *hubforge.Hub, slug, anchor string, other ...string) {
	t.Helper()

	for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
		if !wiredAt(t, h, slug, anchor, name) {
			t.Errorf("%s of %s at %s is no link; want it wired there", name, slug, anchor)
		}
		for _, at := range other {
			if wiredAt(t, h, slug, at, name) {
				t.Errorf("%s of %s at %s is a link; want it unwired there", name, slug, at)
			}
		}
	}
	scratch := filepath.Join(h.PairCodeWorktree(slug), ".scratch")
	want := filepath.Join(h.PairRecordsSibling(slug), anchor, lymdirs.DotLymDirName, "scratch")
	if target, err := fslink.RawTarget(scratch); err != nil || filepath.Clean(target) != filepath.Clean(want) {
		t.Errorf("scratch link of %s = (%q, %v); want it to point at %s", slug, target, err, want)
	}
}

// requireMovedToSub fails the test unless slug's weft carries its content and its marker at sub in the HEAD commit, with a clean tree, and the pair is wired there.
func requireMovedToSub(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	weft := h.PairRecordsSibling(slug)
	if marker := gitkit.Git(t, weft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
		t.Errorf("committed marker of %s = %q; want sub", slug, marker)
	}
	for _, tracked := range gitkit.LsFiles(t, weft) {
		if tracked != lymcwd.AnchorFileName && !strings.HasPrefix(tracked, "sub/") {
			t.Errorf("weft of %s tracks %s; want everything but the marker under sub/", slug, tracked)
		}
	}
	if _, err := os.Stat(filepath.Join(weft, "sub", lymdirs.LymDirName)); err != nil {
		t.Errorf("weft of %s has no sub/%s: %v", slug, lymdirs.LymDirName, err)
	}
	if status := gitkit.GitStatusPorcelain(t, weft); status != "" {
		t.Errorf("weft of %s status = %q; want a clean tree", slug, status)
	}
	requireWiredOnlyAt(t, h, slug, "sub", ".")
	requireNoMoveRecord(t, weft)
}

func TestAnchorChange(t *testing.T) {
	// Not parallel: the failure hook is process-global, so every subtest that sets it runs in turn;
	// the other subtests are parallel and resume once this function has returned and its hooks are gone.
	const mover = "mover"

	t.Run("A pair moves to sub with its content, wiring, launcher and workspace entry", func(t *testing.T) {
		t.Parallel()
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		hubforge.AddPair(t, h, "stayer")
		weft := h.PairRecordsSibling(mover)
		if err := os.WriteFile(filepath.Join(weft, lymdirs.DotLymDirName, "kept.txt"), []byte("kept\n"), 0o644); err != nil {
			t.Fatalf("write the untracked file in .lym: %v", err)
		}

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		if c.NoOp() || c.From() != "." || c.To() != "sub" {
			t.Errorf("change = (noop %v, from %q, to %q); want a move from . to sub", c.NoOp(), c.From(), c.To())
		}
		completeAnchorChange(t, c)

		requireMovedToSub(t, h, mover)
		if subject := gitkit.Git(t, weft, "log", "-1", "--format=%s"); !strings.Contains(subject, "fabric anchor") {
			t.Errorf("weft HEAD subject = %q; want the anchor move commit", subject)
		}
		if got, err := os.ReadFile(filepath.Join(weft, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want the file moved with .lym", got, err)
		}
		if _, err := os.Stat(filepath.Join(h.Path, "_launchers", "sub", mover)); err != nil {
			t.Errorf("launcher directory at sub: %v; want it", err)
		}
		if _, err := os.Stat(filepath.Join(h.Path, "_launchers", mover)); !os.IsNotExist(err) {
			t.Errorf("launcher directory at the old anchor: %v; want it gone", err)
		}
		workspace, err := os.ReadFile(fabricengine.HubWorkspacePath(h.Location))
		if err != nil || !strings.Contains(string(workspace), mover+"/sub") {
			t.Errorf("hub workspace file = (%q, %v); want it to name %s/sub", workspace, err, mover)
		}
		excludes := gitkit.ExcludeLines(t, h.PairCodeWorktree(mover))
		for _, want := range []string{"/" + lymdirs.LymDirName, "/" + lymdirs.DotLymDirName, "/sub/" + lymdirs.LymDirName} {
			if !slices.Contains(excludes, want) {
				t.Errorf("exclude lines = %v; want %q among them", excludes, want)
			}
		}
		requireWiredOnlyAt(t, h, "stayer", ".", "sub")
	})

	t.Run("A target equal to the pair's own anchor is a no-op that takes no lock", func(t *testing.T) {
		t.Parallel()
		h := newAnchorHub(t)
		prime := h.Location.WorktreeName
		hubforge.ReanchorPair(t, h, prime, "sub")
		hubforge.AddPair(t, h, "child")

		c, err := beginAnchorChange(t, h, "child", "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		defer c.Close()

		if !c.NoOp() {
			t.Errorf("NoOp() = false; want a child forked after its parent's change to sit at sub already")
		}
		rec := fabricengine.NewMutations("")
		if landed, err := c.Apply(rec); landed || err != nil || rec.Len() != 0 {
			t.Errorf("Apply() = (landed %v, %v), %d mutations; want nothing done", landed, err, rec.Len())
		}
		if err := c.Finish(rec); err != nil {
			t.Errorf("Finish() error = %v; want nil", err)
		}
		requireNoMoveRecord(t, h.PairRecordsSibling("child"))
		again, err := beginAnchorChange(t, h, "child", "sub")
		if err != nil {
			t.Fatalf("a second BeginAnchorChange() error = %v; want it to find the move lock free", err)
		}
		again.Close()
	})

	t.Run("Each refusal names its next step and leaves the weft, the wiring and the record as they were", func(t *testing.T) {
		tests := []struct {
			name   string
			target string
			setup  func(t *testing.T, h *hubforge.Hub, weft, warp string)
			want   []string
			// record reports that the setup stands a move record, which must stay as it was.
			record bool
		}{
			{
				name:   "An empty own marker",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeFile(t, filepath.Join(weft, lymcwd.AnchorFileName), "\n")
				},
				want: []string{lymcwd.AnchorFileName, "is empty"},
			},
			{
				name:   "A target the anchor validator rejects",
				target: "../out",
				want:   []string{"must stay inside the worktree", "choose another directory of the warp checkout"},
			},
			{
				name:   "A target with a reserved segment",
				target: ".scratch",
				want:   []string{"reserves", "choose another directory of the warp checkout"},
			},
			{
				name:   "A target that passes through a link",
				target: "ln",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					if err := os.Symlink("sub", filepath.Join(warp, "ln")); err != nil {
						t.Skipf("symlinks are unavailable: %v", err)
					}
				},
				want: []string{"passes through the link", "choose another directory of the warp checkout"},
			},
			{
				name:   "A directory neither the warp's HEAD nor its upstream tracks",
				target: "nowhere",
				want:   []string{"tracked neither", "commit the directory in the warp checkout first", "shared by every machine that checks it out"},
			},
			{
				name:   "A directory only the warp's fetched upstream tracks",
				target: "upstream-only",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					gitkit.CommitFile(t, warp, "upstream-only/.keep", "", "track upstream-only")
					gitkit.Git(t, warp, "push", "-q", "origin", "HEAD")
					gitkit.Git(t, warp, "reset", "-q", "--hard", "HEAD~1")
				},
				want: []string{"fetched upstream tracks it", "update the warp checkout by hand", "shared by every machine that checks it out"},
			},
			{
				name:   "A real wired name under the target",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeFile(t, filepath.Join(warp, "sub", lymdirs.LymDirName, "real.txt"), "real\n")
				},
				want: []string{"real " + lymdirs.LymDirName, "move it out of sub in the warp checkout and commit that"},
			},
			{
				name:   "A move lock held by a live change",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					pid := os.Getpid()
					startTime, _ := proc.StartTime(pid)
					writeMoveRecord(t, weft, fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: pid, StartTime: startTime})
					holdMoveLock(t, weft)
				},
				want:   []string{fmt.Sprintf("in progress in process %d", os.Getpid()), "lym fabric anchor"},
				record: true,
			},
			{
				name:   "A move lock held beside the record of a dead process",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeMoveRecord(t, weft, fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: 4242})
					holdMoveLock(t, weft)
				},
				want:   []string{"is starting in another process", "lym fabric anchor"},
				record: true,
			},
			{
				name:   "A move lock held with no record",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					holdMoveLock(t, weft)
				},
				want: []string{"is starting in another process"},
			},
			{
				name:   "A killed run's record and another target",
				target: "other",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeMoveRecord(t, weft, fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: 4242})
				},
				want:   []string{"was interrupted", "lym fabric anchor " + mover + " --set sub"},
				record: true,
			},
			{
				name:   "A weft with no committed marker",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					gitkit.Git(t, weft, "rm", "-q", lymcwd.AnchorFileName)
					gitkit.Git(t, weft, "commit", "-q", "-m", "drop the marker")
				},
				want: []string{"no committed " + lymcwd.AnchorFileName, "lym fabric reconcile"},
			},
			{
				name:   "An untracked path under the wired names",
				target: "sub",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeFile(t, filepath.Join(weft, lymdirs.LymDirName, "stray.txt"), "stray\n")
				},
				want: []string{"stray.txt", "lym fabric commit"},
			},
			{
				name:   "An uncommitted change of the marker",
				target: "other",
				setup: func(t *testing.T, h *hubforge.Hub, weft, warp string) {
					writeFile(t, filepath.Join(weft, lymcwd.AnchorFileName), "sub\n")
				},
				want: []string{"uncommitted change of " + lymcwd.AnchorFileName, "restore " + lymcwd.AnchorFileName},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newAnchorHub(t)
				hubforge.AddPair(t, h, mover)
				weft, warp := h.PairRecordsSibling(mover), h.PairCodeWorktree(mover)
				if tt.setup != nil {
					tt.setup(t, h, weft, warp)
				}
				head, status := gitkit.RevParse(t, weft, "HEAD"), gitkit.GitStatusPorcelain(t, weft)
				recordBefore, foundBefore, _ := fabricengine.ReadMoveRecord(weft)

				c, err := beginAnchorChange(t, h, mover, tt.target)

				if err == nil {
					c.Close()
					t.Fatalf("BeginAnchorChange() succeeded; want a refusal")
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("BeginAnchorChange() error = %q; want it to contain %q", err, want)
					}
				}
				if got := gitkit.RevParse(t, weft, "HEAD"); got != head {
					t.Errorf("weft HEAD = %s; want %s unchanged", got, head)
				}
				if got := gitkit.GitStatusPorcelain(t, weft); got != status {
					t.Errorf("weft status = %q; want %q unchanged", got, status)
				}
				if !wiredAt(t, h, mover, ".", lymdirs.LymDirName) {
					t.Errorf("the pair's %s junction is gone after the refusal", lymdirs.LymDirName)
				}
				recordAfter, foundAfter, _ := fabricengine.ReadMoveRecord(weft)
				if foundAfter != foundBefore || (tt.record && recordAfter.A != recordBefore.A) || (!tt.record && foundAfter) {
					t.Errorf("move record after the refusal = (%+v, %v); want it as before (%+v, %v)", recordAfter, foundAfter, recordBefore, foundBefore)
				}
			})
		}
	})

	t.Run("Close after Begin removes a record it wrote and keeps one it found", func(t *testing.T) {
		t.Parallel()
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		if _, found, _ := fabricengine.ReadMoveRecord(weft); !found {
			t.Errorf("no move record after BeginAnchorChange(); want one naming the change")
		}
		c.Close()
		requireNoMoveRecord(t, weft)

		writeMoveRecord(t, weft, fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: 4242})
		c, err = beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() over a standing record error = %v; want nil", err)
		}
		c.Close()
		record, found, err := fabricengine.ReadMoveRecord(weft)
		if err != nil || !found || record.A != "." || record.B != "sub" || record.PID != os.Getpid() {
			t.Errorf("ReadMoveRecord() = (%+v, %v, %v); want the standing record kept with this process's pid", record, found, err)
		}
	})

	t.Run("A failure before the move commit leaves the pair wired where it was", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		head := gitkit.RevParse(t, weft, "HEAD")
		hookErr := errors.New("injected failure before the commit")
		fabricengine.SetAnchorFailHookForTest(t, func(afterCommit bool) error {
			if afterCommit {
				return nil
			}
			return hookErr
		})

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		defer c.Close()
		landed, err := c.Apply(fabricengine.NewMutations(""))

		if landed || !errors.Is(err, hookErr) {
			t.Errorf("Apply() = (landed %v, %v); want landed false and the injected failure", landed, err)
		}
		if got := gitkit.RevParse(t, weft, "HEAD"); got != head {
			t.Errorf("weft HEAD = %s; want %s unchanged", got, head)
		}
		if status := gitkit.GitStatusPorcelain(t, weft); status != "" {
			t.Errorf("weft status = %q; want a clean tree", status)
		}
		requireWiredOnlyAt(t, h, mover, ".", "sub")
		requireNoMoveRecord(t, weft)
	})

	t.Run("A failure after the move commit leaves the record, and a re-run of the same target finishes at sub", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		hookErr := errors.New("injected failure after the commit")
		failing := true
		fabricengine.SetAnchorFailHookForTest(t, func(afterCommit bool) error {
			if afterCommit && failing {
				return hookErr
			}
			return nil
		})

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		landed, err := c.Apply(fabricengine.NewMutations(""))
		c.Close()

		if !landed || !errors.Is(err, hookErr) || !strings.Contains(err.Error(), "lym fabric anchor "+mover+" --set sub") {
			t.Errorf("Apply() = (landed %v, %v); want landed true, the injected failure and the re-run named", landed, err)
		}
		if _, found, _ := fabricengine.ReadMoveRecord(weft); !found {
			t.Errorf("no move record after a failure past the commit; want it standing")
		}

		failing = false
		rerun, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("the re-run's BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, rerun)
		requireMovedToSub(t, h, mover)
	})

	t.Run("A failing read of the commit after the move commit keeps the record and does not roll back", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		readErr := errors.New("injected failure reading the commit")
		fabricengine.SetWeftHeadSHAForTest(t, func(string) (string, error) { return "", readErr })

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		landed, err := c.Apply(fabricengine.NewMutations(""))
		c.Close()

		if !landed || !errors.Is(err, readErr) || !strings.Contains(err.Error(), "lym fabric anchor "+mover+" --set sub") {
			t.Errorf("Apply() = (landed %v, %v); want landed true, the read failure and the re-run named", landed, err)
		}
		if _, found, _ := fabricengine.ReadMoveRecord(weft); !found {
			t.Errorf("no move record after a failed read past the commit; want it standing")
		}
		if subject := gitkit.Git(t, weft, "log", "-1", "--format=%s"); !strings.Contains(subject, "fabric anchor") {
			t.Errorf("weft HEAD subject = %q; want the anchor move commit kept", subject)
		}
	})

	t.Run("A restore that fails keeps the record, and the re-run refuses over both .lym directories", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		fabricengine.SetAnchorFailHookForTest(t, func(afterCommit bool) error {
			if afterCommit {
				return nil
			}
			// The move already renamed .lym to sub/.lym, so a non-empty .lym makes the rename back fail.
			writeFile(t, filepath.Join(weft, lymdirs.DotLymDirName, "blocker.txt"), "blocker\n")
			return errors.New("injected failure before the commit")
		})

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		landed, err := c.Apply(fabricengine.NewMutations(""))
		c.Close()

		if landed || err == nil || !strings.Contains(err.Error(), "lym fabric anchor "+mover+" --set sub") {
			t.Errorf("Apply() = (landed %v, %v); want landed false and the re-run named", landed, err)
		}
		if _, found, _ := fabricengine.ReadMoveRecord(weft); !found {
			t.Errorf("no move record after a restore that failed; want it standing")
		}

		status := gitkit.GitStatusPorcelain(t, weft)
		_, err = beginAnchorChange(t, h, mover, "sub")
		if err == nil {
			t.Fatalf("the re-run's BeginAnchorChange() succeeded; want a refusal over both .lym directories")
		}
		for _, want := range []string{filepath.Join(weft, lymdirs.DotLymDirName), filepath.Join(weft, "sub", lymdirs.DotLymDirName), "by hand"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the re-run's error = %q; want it to contain %q", err, want)
			}
		}
		if got := gitkit.GitStatusPorcelain(t, weft); got != status {
			t.Errorf("weft status = %q; want %q, the refusal touching nothing", got, status)
		}
	})

	t.Run("A re-run after a kill before the commit finishes at sub keeping the file in .lym", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		writeFile(t, filepath.Join(weft, lymdirs.DotLymDirName, "kept.txt"), "kept\n")
		killAt(t, false)

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		applyUntilKilled(t, c)
		fabricengine.SetAnchorFailHookForTest(t, nil)

		rerun, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("the re-run's BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, rerun)
		requireMovedToSub(t, h, mover)
		if got, err := os.ReadFile(filepath.Join(weft, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want the file kept", got, err)
		}
	})

	t.Run("A re-run after a kill between the commit and the wiring finishes at sub keeping the file in .lym", func(t *testing.T) {
		h := newAnchorHub(t)
		hubforge.AddPair(t, h, mover)
		weft := h.PairRecordsSibling(mover)
		writeFile(t, filepath.Join(weft, lymdirs.DotLymDirName, "kept.txt"), "kept\n")
		killAt(t, true)

		c, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		applyUntilKilled(t, c)
		if marker := gitkit.Git(t, weft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Fatalf("committed marker after the kill = %q; want the commit to have landed", marker)
		}
		fabricengine.SetAnchorFailHookForTest(t, nil)

		rerun, err := beginAnchorChange(t, h, mover, "sub")
		if err != nil {
			t.Fatalf("the re-run's BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, rerun)
		requireMovedToSub(t, h, mover)
		if got, err := os.ReadFile(filepath.Join(weft, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want the file kept", got, err)
		}
	})

	t.Run("The integration of a change a second clone made carries the local commits", func(t *testing.T) {
		h, prime := newIntegrateHub(t)
		primeWeft := h.PrimeRecords()
		upstream := gitkit.RevParse(t, primeWeft, "@{upstream}")
		writeFile(t, filepath.Join(primeWeft, lymdirs.DotLymDirName, "kept.txt"), "kept\n")

		c, err := h.Topology.BeginAnchorChange(prime, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, c)

		if !gitkit.IsAncestor(t, primeWeft, upstream, "HEAD") {
			t.Errorf("the upstream tip %s is no ancestor of HEAD; want the local commits replayed on it", upstream)
		}
		if got := gitkit.Git(t, primeWeft, "show", "HEAD:sub/"+lymdirs.LymDirName+"/local.txt"); got != "local" {
			t.Errorf("sub/_lym/local.txt at HEAD = %q; want the local commit's file moved to sub", got)
		}
		if marker := gitkit.Git(t, primeWeft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Errorf("committed marker = %q; want sub", marker)
		}
		if got, err := os.ReadFile(filepath.Join(primeWeft, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want .lym moved with the integration", got, err)
		}
		requireWiredOnlyAt(t, h, prime.WorktreeName, "sub", ".")
		requireNoMoveRecord(t, primeWeft)
	})

	t.Run("A failure before an integration moves the branch leaves the pair wired where it was", func(t *testing.T) {
		h, prime := newIntegrateHub(t)
		primeWeft := h.PrimeRecords()
		head := gitkit.RevParse(t, primeWeft, "HEAD")
		hookErr := errors.New("injected failure before the branch moves")
		fabricengine.SetAnchorFailHookForTest(t, func(afterCommit bool) error {
			if afterCommit {
				return nil
			}
			return hookErr
		})

		c, err := h.Topology.BeginAnchorChange(prime, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		defer c.Close()
		landed, err := c.Apply(fabricengine.NewMutations(""))

		if landed || !errors.Is(err, hookErr) {
			t.Errorf("Apply() = (landed %v, %v); want landed false and the injected failure", landed, err)
		}
		if got := gitkit.RevParse(t, primeWeft, "HEAD"); got != head {
			t.Errorf("weft HEAD = %s; want %s unchanged", got, head)
		}
		if status := gitkit.GitStatusPorcelain(t, primeWeft); status != "" {
			t.Errorf("weft status = %q; want a clean tree", status)
		}
		requireWiredOnlyAt(t, h, prime.WorktreeName, ".", "sub")
		requireNoMoveRecord(t, primeWeft)
	})

	t.Run("A re-run after a killed integration on a checkout already at sub finishes", func(t *testing.T) {
		h, prime := newIntegrateHub(t)
		primeWeft := h.PrimeRecords()
		writeFile(t, filepath.Join(primeWeft, lymdirs.DotLymDirName, "kept.txt"), "kept\n")
		killAt(t, true)

		c, err := h.Topology.BeginAnchorChange(prime, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		applyUntilKilled(t, c)
		if marker := gitkit.Git(t, primeWeft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "sub" {
			t.Fatalf("committed marker after the kill = %q; want the integration to have landed", marker)
		}
		fabricengine.SetAnchorFailHookForTest(t, nil)

		rerun, err := beginAnchorChange(t, h, prime.WorktreeName, "sub")
		if err != nil {
			t.Fatalf("the re-run's BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, rerun)

		requireWiredOnlyAt(t, h, prime.WorktreeName, "sub", ".")
		requireNoMoveRecord(t, primeWeft)
		if got, err := os.ReadFile(filepath.Join(primeWeft, "sub", lymdirs.DotLymDirName, "kept.txt")); err != nil || string(got) != "kept\n" {
			t.Errorf("sub/.lym/kept.txt = (%q, %v); want the file kept", got, err)
		}
	})

	t.Run("UpstreamRunUnfinished reads the status at the fetched upstream tip", func(t *testing.T) {
		const statusRel = "_lym/shed/" + mover + "/status.json"
		tests := []struct {
			name string
			// upstreamRel and upstreamState are the status the second clone pushes, none when upstreamRel is empty.
			upstreamRel, upstreamState string
			want                       bool
		}{
			{name: "A checked-out status that reads unfinished and an upstream that reads finished is finished", upstreamRel: statusRel, upstreamState: "done", want: false},
			{name: "An upstream status that reads unfinished is unfinished", upstreamRel: statusRel, upstreamState: "running", want: true},
			{name: "An upstream with no status is finished", want: false},
			{name: "An upstream status in the legacy self directory counts", upstreamRel: "_lym/shed/self/status.json", upstreamState: "paused", want: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newAnchorHub(t)
				hubforge.AddPair(t, h, mover)
				weft := h.PairRecordsSibling(mover)
				gitkit.CommitFile(t, weft, statusRel, `{"state":"running"}`, "local run status")
				branch := gitkit.CurrentBranch(t, weft)
				other := filepath.Join(t.TempDir(), "other")
				gitkit.Git(t, filepath.Dir(other), "clone", "-q", "--branch", branch, h.RecordsBare, other)
				if tt.upstreamRel != "" {
					gitkit.CommitFile(t, other, tt.upstreamRel, fmt.Sprintf(`{"state":%q}`, tt.upstreamState), "second clone's run status")
				} else {
					gitkit.CommitFile(t, other, "unrelated.txt", "x\n", "second clone's unrelated commit")
				}
				gitkit.Git(t, other, "push", "-q", "origin", branch)
				gitkit.Git(t, weft, "fetch", "-q", "origin")
				pair, err := lymcwd.NamedWorktree(h.Path, mover)
				if err != nil {
					t.Fatalf("NamedWorktree: %v", err)
				}
				names, err := fabricengine.RepoWiredNames(pair)
				if err != nil {
					t.Fatalf("RepoWiredNames: %v", err)
				}

				got, err := fabricengine.UpstreamRunUnfinished(pair, names)

				if err != nil || got != tt.want {
					t.Errorf("UpstreamRunUnfinished() = (%v, %v); want (%v, nil)", got, err, tt.want)
				}
			})
		}
	})
}

// newIntegrateHub builds a "." hub whose prime weft holds a local commit of _lym/local.txt and whose upstream a second clone moved to sub, with the new tip fetched.
// It returns the hub and the prime's Location at its own anchor.
func newIntegrateHub(t *testing.T) (*hubforge.Hub, *lymcwd.Location) {
	t.Helper()

	h := newAnchorHub(t)
	second := hubforge.CloneSecond(t, h)
	hubforge.ReanchorPair(t, second, second.Location.WorktreeName, "sub")
	if _, err := fabricengine.CoalescePushBothAt(second.PrimeWorktree(), second.PrimeRecords(), fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("push the re-anchored prime from the second clone: %v", err)
	}
	gitkit.CommitFile(t, h.PrimeRecords(), lymdirs.LymDirName+"/local.txt", "local\n", "local commit")
	gitkit.Git(t, h.PrimeRecords(), "fetch", "-q", "origin")
	branch := gitkit.CurrentBranch(t, h.PrimeRecords())
	gitkit.Git(t, h.PrimeRecords(), "branch", "-q", "--set-upstream-to=origin/"+branch, branch)
	prime, err := lymcwd.NamedWorktree(h.Path, h.Location.WorktreeName)
	if err != nil {
		t.Fatalf("NamedWorktree: %v", err)
	}
	return h, prime
}
