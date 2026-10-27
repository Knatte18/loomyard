//go:build tier1

// addparent_integration_test.go covers Add with AddOptions.Parent over a hubforge hub: the fork from a named pair, the beam mark, the refusals that precede any mutation and the dirt rule.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// gitOut returns the trimmed stdout of git args in dir, failing the test on any git error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := gitexec.Run(args, dir)
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(out)
}

func TestAdd_FromParentPair(t *testing.T) {
	t.Parallel()
	const tracked = "tracked.txt"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	gitkit.CommitFile(t, h.PrimeWorktree(), tracked, "original\n", "add tracked file")
	addSkippingPush := func(slug, parent string, beam bool) (fabricengine.AddResult, error) {
		return h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true, Parent: parent, Beam: beam})
	}

	// The pairs exist before the prime turns dirty, since a plain Add refuses a dirty source.
	for _, slug := range []string{"beam", "detached", "switched"} {
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	}
	beamPath := fabricengine.WorktreePath(l, "beam")
	gitkit.CommitFile(t, beamPath, "beam-only.txt", "beam\n", "commit on the beam")
	beamHead := gitOut(t, beamPath, "rev-parse", "HEAD")
	beamRecords := fabricengine.RecordsBranchName(gitOut(t, beamPath, "rev-parse", "--abbrev-ref", "HEAD"))
	beamWeftPath := fabricengine.RecordsWorktreePath(l, "beam")
	beamRecordsTip := gitOut(t, beamWeftPath, "rev-parse", beamRecords)

	// Both worktrees carry tracked dirt: a named parent checks neither.
	for _, dir := range []string{h.PrimeWorktree(), beamPath} {
		if err := os.WriteFile(filepath.Join(dir, tracked), []byte("changed\n"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", dir, err)
		}
	}

	assertNothingCreated := func(t *testing.T, slug string) {
		t.Helper()
		for _, path := range []string{fabricengine.WorktreePath(l, slug), fabricengine.RecordsWorktreePath(l, slug)} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s exists after the refusal (stat error %v); want it absent", path, err)
			}
		}
		if out := gitOut(t, h.PrimeWorktree(), "branch", "--list", "*"+slug+"*"); out != "" {
			t.Errorf("warp branches matching %q after the refusal = %q; want none", slug, out)
		}
		if out := gitOut(t, beamWeftPath, "branch", "--list", "*"+slug+"*"); out != "" {
			t.Errorf("records branches matching %q after the refusal = %q; want none", slug, out)
		}
	}

	t.Run("forks from the parent's last commit and records branch", func(t *testing.T) {
		const slug = "member"
		if _, err := addSkippingPush(slug, "beam", false); err != nil {
			t.Fatalf("Add(%q, Parent beam): %v", slug, err)
		}
		memberPath := fabricengine.WorktreePath(l, slug)
		if got := gitOut(t, memberPath, "rev-parse", "HEAD"); got != beamHead {
			t.Errorf("member HEAD = %s; want the beam's committed HEAD %s", got, beamHead)
		}
		if got := gitOut(t, memberPath, "show", "HEAD:"+tracked); got != "original" {
			t.Errorf("member %s = %q; want the committed content, not the parent's dirt", tracked, got)
		}
		memberWeftPath := fabricengine.RecordsWorktreePath(l, slug)
		if out, err := gitexec.Run([]string{"merge-base", "--is-ancestor", beamRecordsTip, "HEAD"}, memberWeftPath); err != nil {
			t.Errorf("beam's records tip %s is not an ancestor of the member's records HEAD: %v %s", beamRecordsTip, err, out)
		}
		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want a record", slug, origin, found, err)
		}
		want := fabricengine.Origin{ParentBranch: gitOut(t, beamPath, "rev-parse", "--abbrev-ref", "HEAD"), ParentWorktree: "beam"}
		if origin != want {
			t.Errorf("member origin record = %+v; want %+v", origin, want)
		}
	})

	t.Run("writes the beam mark when asked", func(t *testing.T) {
		const slug = "member-beam"
		if _, err := addSkippingPush(slug, "beam", true); err != nil {
			t.Fatalf("Add(%q, Parent beam, Beam): %v", slug, err)
		}
		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found || !origin.Beam {
			t.Errorf("ReadOriginFor(%q) = (%+v, %v, %v); want a record with the beam mark", slug, origin, found, err)
		}
	})

	t.Run("refuses a parent that cannot be forked from before any mutation", func(t *testing.T) {
		tests := []struct {
			name   string
			parent func(t *testing.T) string
			want   []string
		}{
			{"UnknownParent", func(t *testing.T) string { return "no-such-pair" }, []string{"no-such-pair", "lym fabric pairs"}},
			{"PrimeName", func(t *testing.T) string { return l.WorktreeName }, []string{l.WorktreeName, "drop --parent"}},
			{"DetachedParent", func(t *testing.T) string {
				gitOut(t, fabricengine.WorktreePath(l, "detached"), "checkout", "--detach")
				return "detached"
			}, []string{"detached", "lym fabric checkout ", "detached HEAD"}},
			{"SwitchedParent", func(t *testing.T) string {
				gitOut(t, fabricengine.WorktreePath(l, "switched"), "checkout", "-b", "elsewhere")
				return "switched"
			}, []string{"switched", "\"elsewhere\"", "lym fabric checkout "}},
			{"EmptyForkPointMarker", func(t *testing.T) string {
				marker := filepath.Join(beamWeftPath, lymcwd.AnchorFileName)
				if err := os.WriteFile(marker, []byte("  \n"), 0o644); err != nil {
					t.Fatalf("truncate %s: %v", marker, err)
				}
				t.Cleanup(func() { gitOut(t, beamWeftPath, "restore", lymcwd.AnchorFileName) })
				return "beam"
			}, []string{lymcwd.AnchorFileName, "git -C"}},
			{"MoveRecordOnForkPoint_LockFree", func(t *testing.T) string {
				writeMoveRecord(t, beamWeftPath, fabricengine.MoveRecord{A: ".", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})
				return "beam"
			}, []string{"interrupted", "lym fabric anchor beam --set x"}},
			{"MoveRecordOnForkPoint_LockHeld", func(t *testing.T) string {
				writeMoveRecord(t, beamWeftPath, fabricengine.MoveRecord{A: ".", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})
				holdMoveLock(t, beamWeftPath)
				return "beam"
			}, []string{"starting in another process", "run the verb again"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				parent := tt.parent(t)
				slug := "refused-" + strings.ToLower(tt.name)
				_, err := addSkippingPush(slug, parent, false)
				if err == nil {
					t.Fatalf("Add(%q, Parent %q) = nil; want a refusal", slug, parent)
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Add(%q, Parent %q) error = %v; want it to contain %q", slug, parent, err, want)
					}
				}
				assertNothingCreated(t, slug)
			})
		}
	})

	t.Run("without a parent a dirty source still refuses", func(t *testing.T) {
		_, err := addSkippingPush("no-parent", "", false)
		var refusal *fabricengine.ErrSourceDirty
		if !errors.As(err, &refusal) {
			t.Fatalf("Add without Parent from a dirty prime error = %v; want *ErrSourceDirty", err)
		}
		assertNothingCreated(t, "no-parent")
	})

	t.Run("a forked pair takes the fork point's anchor", func(t *testing.T) {
		const anchor = "x"
		hubforge.ReanchorPair(t, h, "beam", anchor)
		gitOut(t, beamPath, "checkout", "--", tracked)
		beamLocation, err := lymcwd.ResolveWorktree(beamPath)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", beamPath, err)
		}
		if beamLocation.AnchorRel != anchor {
			t.Fatalf("beam AnchorRel = %q; want %q after ReanchorPair", beamLocation.AnchorRel, anchor)
		}

		tests := []struct {
			name string
			add  func(slug string) error
		}{
			{"NamedParent", func(slug string) error {
				_, err := addSkippingPush(slug, "beam", false)
				return err
			}},
			{"ActingWorktreeAsForkPoint", func(slug string) error {
				_, err := h.Topology.Add(beamLocation, slug, fabricengine.AddOptions{SkipPush: true})
				return err
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				slug := "anchored-" + strings.ToLower(tt.name)
				if err := tt.add(slug); err != nil {
					t.Fatalf("Add(%q): %v", slug, err)
				}

				weftPath := fabricengine.RecordsWorktreePath(l, slug)
				marker, err := os.ReadFile(filepath.Join(weftPath, lymcwd.AnchorFileName))
				if err != nil {
					t.Fatalf("read the pair's marker: %v", err)
				}
				if got := strings.TrimSpace(string(marker)); got != anchor {
					t.Errorf("%s in the pair's records worktree = %q; want %q", lymcwd.AnchorFileName, got, anchor)
				}

				pairPath := fabricengine.WorktreePath(l, slug)
				for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
					link := filepath.Join(pairPath, anchor, name)
					if isLink, err := fslink.IsLink(link); err != nil || !isLink {
						t.Errorf("%s: IsLink = (%v, %v); want a junction at the fork point's anchor", link, isLink, err)
					}
				}
				wantScratch := filepath.Join(weftPath, anchor, lymdirs.DotLymDirName, "scratch")
				if got, err := fslink.PointsTo(filepath.Join(pairPath, ".scratch")); err != nil || filepath.Clean(got) != wantScratch {
					t.Errorf("scratch link of %s = (%q, %v); want it to point at %s", slug, got, err, wantScratch)
				}

				if _, err := os.Stat(filepath.Join(weftPath, anchor, fabricengine.OriginRecordRel())); err != nil {
					t.Errorf("origin record at %s/%s in the records worktree: %v", anchor, fabricengine.OriginRecordRel(), err)
				}
				if _, err := os.Stat(filepath.Join(h.Path, "_launchers", anchor, slug)); err != nil {
					t.Errorf("launcher directory under _launchers/%s/%s: %v", anchor, slug, err)
				}
			})
		}
	})
}
