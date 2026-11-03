// add_adoptonly_integration_test.go covers Add under AddOptions.AdoptOnly over hubforge hubs: the re-making of a live pair whose task worktree is gone, the typed answers for a pair that cannot be re-made, and the reuse of a records worktree that stands.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// adoptOnlyOptions are the options batten's resume passes: it adopts the local branch and forks nothing.
var adoptOnlyOptions = fabricengine.AddOptions{AdoptLocal: true, AdoptOnly: true}

// pushedAdoptPair returns a hub holding slug's pair with both branches pushed.
func pushedAdoptPair(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	return h
}

// dropTaskWorktree removes slug's task worktree and leaves its local branch.
func dropTaskWorktree(h *hubforge.Hub, slug string) {
	mustGit(h.PrimeWorktree(), "worktree", "remove", "--force", fabricengine.WorktreePath(h.Location, slug))
}

// dropRecordsWorktree removes slug's records worktree and its local weft branch.
func dropRecordsWorktree(h *hubforge.Hub, slug string) {
	mustGit(h.PrimeRecords(), "worktree", "remove", "--force", fabricengine.RecordsWorktreePath(h.Location, slug))
	mustGit(h.PrimeRecords(), "branch", "-D", fabricengine.RecordsBranchName(slug))
}

// goneLocally returns a hub whose slug pair is pushed and has lost both worktrees and both local branches, leaving the branches on origin.
func goneLocally(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := pushedAdoptPair(t, slug)
	dropTaskWorktree(h, slug)
	dropRecordsWorktree(h, slug)
	mustGit(h.PrimeWorktree(), "branch", "-D", slug)
	if err := os.RemoveAll(fabricengine.LauncherDir(h.Location, slug)); err != nil {
		t.Fatalf("remove the launchers of %s: %v", slug, err)
	}
	return h
}

// addAdoptOnly runs the adopt-only Add of slug.
func addAdoptOnly(h *hubforge.Hub, slug string) (fabricengine.AddResult, error) {
	return h.Topology.Add(h.Location, slug, adoptOnlyOptions)
}

// requireRemade asserts slug's task worktree stands on the task branch and its records worktree on the weft branch.
func requireRemade(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if got := gitkit.CurrentBranch(t, fabricengine.WorktreePath(h.Location, slug)); got != slug {
		t.Errorf("task worktree branch = %q; want %q", got, slug)
	}
	if got, want := gitkit.CurrentBranch(t, fabricengine.RecordsWorktreePath(h.Location, slug)), fabricengine.RecordsBranchName(slug); got != want {
		t.Errorf("records worktree branch = %q; want %q", got, want)
	}
}

// requireNotLive asserts err is an *ErrNotLive matching ErrPairNotLive that names branch and records.
func requireNotLive(t *testing.T, err error, branch string, records bool) {
	t.Helper()

	if !errors.Is(err, fabricengine.ErrPairNotLive) {
		t.Fatalf("Add error = %v; want it to match ErrPairNotLive", err)
	}
	var notLive *fabricengine.ErrNotLive
	if !errors.As(err, &notLive) || notLive.Branch != branch || notLive.Records != records {
		t.Errorf("Add error = %#v; want *ErrNotLive{Branch: %q, Records: %v}", err, branch, records)
	}
}

func TestAdd_AdoptOnly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("re-makes a pair whose worktrees and local branches are gone", func(t *testing.T) {
		t.Parallel()

		const slug = "gone"
		h := goneLocally(t, slug)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireRemade(t, h, slug)
		if _, found, err := fabricengine.ReadOriginFor(h.Location, slug); err != nil || !found {
			t.Errorf("ReadOriginFor = (found %v, %v); want the pair's run records back", found, err)
		}
	})

	t.Run("re-makes a pair over a kept local branch whose worktree registration is stale", func(t *testing.T) {
		t.Parallel()

		const slug = "stale-reg"
		h := pushedAdoptPair(t, slug)
		dropRecordsWorktree(h, slug)
		tip := gitkit.RevParse(t, h.PrimeWorktree(), slug)
		if err := os.RemoveAll(fabricengine.WorktreePath(h.Location, slug)); err != nil {
			t.Fatalf("delete the task worktree by hand: %v", err)
		}
		if err := fabricengine.PruneRegistrations(h.Location); err != nil {
			t.Fatalf("PruneRegistrations: %v", err)
		}

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireRemade(t, h, slug)
		if got := gitkit.RevParse(t, fabricengine.WorktreePath(h.Location, slug), "HEAD"); got != tip {
			t.Errorf("task worktree HEAD = %s; want the kept branch's tip %s", got, tip)
		}
	})

	t.Run("re-makes a beam's pair at the anchor its weft records", func(t *testing.T) {
		t.Parallel()

		const slug = "beam"
		h := newAtAnchorHub(t)
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{Anchor: "y", Beam: true})
		dropTaskWorktree(h, slug)
		dropRecordsWorktree(h, slug)
		mustGit(h.PrimeWorktree(), "branch", "-D", slug)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireWeftAtY(t, h, slug)
	})

	t.Run("re-makes a pair from a prime with tracked dirt", func(t *testing.T) {
		t.Parallel()

		const slug = "dirty-prime"
		h := goneLocally(t, slug)
		tracked := gitkit.LsFiles(t, h.PrimeWorktree())[0]
		if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), tracked), []byte("dirt\n"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", tracked, err)
		}

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add over a dirty prime: %v", err)
		}

		requireRemade(t, h, slug)
	})

	t.Run("re-makes a pair from a prime that a forking add would refuse", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			spoil func(t *testing.T, h *hubforge.Hub)
		}{
			{"a detached HEAD", func(t *testing.T, h *hubforge.Hub) {
				t.Helper()
				mustGit(h.PrimeWorktree(), "checkout", "--detach")
			}},
			{"an empty anchor marker", func(t *testing.T, h *hubforge.Hub) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(h.PrimeRecords(), lymcwd.AnchorFileName), nil, 0o644); err != nil {
					t.Fatalf("empty the anchor marker: %v", err)
				}
			}},
			{"a standing anchor move record", func(t *testing.T, h *hubforge.Hub) {
				t.Helper()
				writeMoveRecord(t, h.PrimeRecords(), fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: 4242})
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				const slug = "spoiled-prime"
				h := goneLocally(t, slug)
				tt.spoil(t, h)

				if _, err := addAdoptOnly(h, slug); err != nil {
					t.Fatalf("Add over a prime with %s: %v", tt.name, err)
				}

				requireRemade(t, h, slug)
			})
		}
	})

	t.Run("refuses a pair whose task branch is gone locally and on origin", func(t *testing.T) {
		t.Parallel()

		const slug = "no-task"
		h := goneLocally(t, slug)
		mustGit(h.CodeBare, "branch", "-D", slug)

		_, err := addAdoptOnly(h, slug)

		requireNotLive(t, err, slug, false)
		requireNothingCreated(t, h, slug)
	})

	t.Run("refuses a pair whose weft an archive tag covers", func(t *testing.T) {
		t.Parallel()

		const slug = "archived"
		h := removedPair(t, slug)

		_, err := addAdoptOnly(h, slug)

		requireNotLive(t, err, fabricengine.RecordsBranchName(slug), true)
		requireNothingCreated(t, h, slug)
	})

	t.Run("refuses a pair whose weft branch is gone locally and on origin", func(t *testing.T) {
		t.Parallel()

		const slug = "no-records"
		h := goneLocally(t, slug)
		mustGit(h.RecordsBare, "branch", "-D", fabricengine.RecordsBranchName(slug))

		_, err := addAdoptOnly(h, slug)

		requireNotLive(t, err, fabricengine.RecordsBranchName(slug), true)
		requireNothingCreated(t, h, slug)
	})

	t.Run("keeps a local task branch behind a newer copy on origin and writes no tracking ref for it", func(t *testing.T) {
		t.Parallel()

		const slug = "behind"
		h := pushedAdoptPair(t, slug)
		dropTaskWorktree(h, slug)
		dropRecordsWorktree(h, slug)
		tip := gitkit.RevParse(t, h.PrimeWorktree(), slug)
		pushCommitToOrigin(t, h.CodeBare, slug)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		if got := gitkit.RevParse(t, fabricengine.WorktreePath(h.Location, slug), "HEAD"); got != tip {
			t.Errorf("task worktree HEAD = %s; want the local tip %s", got, tip)
		}
		if got := gitkit.RevParse(t, h.PrimeWorktree(), "refs/remotes/origin/"+slug); got != tip {
			t.Errorf("tracking ref origin/%s = %s; want it left at %s", slug, got, tip)
		}
	})

	t.Run("writes no origin record for an adopted weft that carries none", func(t *testing.T) {
		t.Parallel()

		const slug = "no-record"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := goneLocally(t, slug)
		clone := t.TempDir()
		mustGit(clone, "clone", "--quiet", h.RecordsBare, ".")
		mustGit(clone, "checkout", "--quiet", weftBranch)
		mustGit(clone, "rm", "-q", "--", fabricengine.OriginRecordRel())
		gitkit.Git(t, clone, "commit", "-q", "-m", "drop the origin record")
		mustGit(clone, "push", "--quiet", "origin", weftBranch)
		tip := gitkit.RevParse(t, h.RecordsBare, weftBranch)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		if got := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(h.Location, slug), "HEAD"); got != tip {
			t.Errorf("weft branch = %s; want origin's tip %s with no origin-record commit on it", got, tip)
		}
		if _, found, err := fabricengine.ReadOriginFor(h.Location, slug); err != nil || found {
			t.Errorf("ReadOriginFor = (found %v, %v); want no record", found, err)
		}
	})

	t.Run("refuses a local weft branch diverging from origin's", func(t *testing.T) {
		t.Parallel()

		const slug = "diverged"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := goneLocally(t, slug)
		mustGit(h.PrimeRecords(), "branch", weftBranch, gitkit.RevParse(t, h.RecordsBare, weftBranch))
		seed := filepath.Join(t.TempDir(), "seed")
		mustGit(h.PrimeRecords(), "worktree", "add", seed, weftBranch)
		gitkit.CommitFile(t, seed, "local.txt", "local\n", "local weft work")
		mustGit(h.PrimeRecords(), "worktree", "remove", seed)
		pushCommitToOrigin(t, h.RecordsBare, weftBranch)

		_, err := addAdoptOnly(h, slug)

		requireRemoteLeftover(t, err, weftBranch)
	})

	t.Run("reuses a records worktree that stands on the weft branch", func(t *testing.T) {
		t.Parallel()

		const slug = "reuse"
		h := pushedAdoptPair(t, slug)
		dropTaskWorktree(h, slug)
		marker := filepath.Join(fabricengine.RecordsWorktreePath(h.Location, slug), "reused.marker")
		if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
			t.Fatalf("write the marker file: %v", err)
		}

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireRemade(t, h, slug)
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("the reused records worktree lost its untracked file: %v", err)
		}
	})

	t.Run("fast-forwards the weft branch of a reused records worktree to origin's tip", func(t *testing.T) {
		t.Parallel()

		const slug = "reuse-behind"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := pushedAdoptPair(t, slug)
		dropTaskWorktree(h, slug)
		pushCommitToOrigin(t, h.RecordsBare, weftBranch)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		if got, want := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(h.Location, slug), "HEAD"), gitkit.RevParse(t, h.RecordsBare, weftBranch); got != want {
			t.Errorf("weft branch = %s; want origin's tip %s", got, want)
		}
	})

	t.Run("refuses a plain directory at the records worktree path", func(t *testing.T) {
		t.Parallel()

		const slug = "records-dir"
		h := goneLocally(t, slug)
		if err := os.MkdirAll(fabricengine.RecordsWorktreePath(h.Location, slug), 0o755); err != nil {
			t.Fatalf("make the directory: %v", err)
		}

		_, err := addAdoptOnly(h, slug)

		var notPair *fabricengine.ErrRecordsWorktreeNotPair
		if !errors.As(err, &notPair) {
			t.Fatalf("Add error = %v; want *ErrRecordsWorktreeNotPair", err)
		}
	})

	t.Run("refuses a file at the task worktree path", func(t *testing.T) {
		t.Parallel()

		const slug = "task-file"
		h := goneLocally(t, slug)
		if err := os.WriteFile(fabricengine.WorktreePath(h.Location, slug), []byte("x"), 0o644); err != nil {
			t.Fatalf("write the file: %v", err)
		}

		_, err := addAdoptOnly(h, slug)

		var taken *fabricengine.ErrTaskPathTaken
		if !errors.As(err, &taken) {
			t.Fatalf("Add error = %v; want *ErrTaskPathTaken", err)
		}
	})

	t.Run("refuses a task branch another worktree holds", func(t *testing.T) {
		t.Parallel()

		const slug = "task-held"
		h := pushedAdoptPair(t, slug)
		dropTaskWorktree(h, slug)
		dropRecordsWorktree(h, slug)
		mustGit(h.PrimeWorktree(), "worktree", "add", filepath.Join(t.TempDir(), "elsewhere"), slug)

		_, err := addAdoptOnly(h, slug)

		var held *fabricengine.ErrAdoptLocalCheckedOut
		if !errors.As(err, &held) || held.Records || held.Branch != slug {
			t.Fatalf("Add error = %v; want *ErrAdoptLocalCheckedOut for the task branch", err)
		}
	})

	t.Run("refuses a records branch another records worktree holds when the task branch comes from origin", func(t *testing.T) {
		t.Parallel()

		const slug = "records-held"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := goneLocally(t, slug)
		mustGit(h.PrimeRecords(), "branch", weftBranch, gitkit.RevParse(t, h.RecordsBare, weftBranch))
		mustGit(h.PrimeRecords(), "worktree", "add", filepath.Join(t.TempDir(), "elsewhere"), weftBranch)

		_, err := addAdoptOnly(h, slug)

		var held *fabricengine.ErrAdoptLocalCheckedOut
		if !errors.As(err, &held) || !held.Records || held.Branch != weftBranch {
			t.Fatalf("Add error = %v; want *ErrAdoptLocalCheckedOut for the records branch", err)
		}
		if _, statErr := os.Lstat(fabricengine.WorktreePath(h.Location, slug)); !os.IsNotExist(statErr) {
			t.Errorf("task worktree exists after a refused Add (stat err = %v)", statErr)
		}
		if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
			t.Errorf("local task branch %q exists after a refused Add", slug)
		}
	})

	t.Run("drops the tracking ref of an adopted local task branch origin lacks", func(t *testing.T) {
		t.Parallel()

		const slug = "lacks-origin"
		h := pushedAdoptPair(t, slug)
		dropTaskWorktree(h, slug)
		dropRecordsWorktree(h, slug)
		mustGit(h.CodeBare, "branch", "-D", slug)
		requireTrackingStanding(t, h.PrimeWorktree(), slug, true)

		if _, err := addAdoptOnly(h, slug); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireTrackingStanding(t, h.PrimeWorktree(), slug, false)
	})
}

// TestAdd_AdoptOnlyRollbackKeepsReusedRecordsWorktree covers a failure after the task worktree exists: the records worktree the add reused, and its branch, stay.
// It sets a package hook, so it must not run in parallel.
func TestAdd_AdoptOnlyRollbackKeepsReusedRecordsWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SetAddFailAfterWarpHookForTest sets the package-level addFailAfterWarpHook.
	const slug = "reuse-fail"
	h := pushedAdoptPair(t, slug)
	dropTaskWorktree(h, slug)
	fabricengine.SetAddFailAfterWarpHookForTest(t, func() error { return errors.New("injected failure") })

	if _, err := addAdoptOnly(h, slug); err == nil {
		t.Fatal("Add succeeded; want the injected failure")
	}

	if got, want := gitkit.CurrentBranch(t, fabricengine.RecordsWorktreePath(h.Location, slug)), fabricengine.RecordsBranchName(slug); got != want {
		t.Errorf("records worktree branch = %q; want it left on %q", got, want)
	}
	if _, err := os.Lstat(fabricengine.WorktreePath(h.Location, slug)); !os.IsNotExist(err) {
		t.Errorf("task worktree remains after the rollback (stat err = %v)", err)
	}
}
