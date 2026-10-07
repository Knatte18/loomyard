//go:build integration

// adoptremote_integration_test.go drives the fabric CLI verbs that take a pair's weft branch from origin when it exists only there, against one real hub as an ordered scenario.
// Origin-only branches are pushed into the hub's bare from a scratch clone, and an unreachable origin is simulated by pointing the weft repo's origin URL at a missing path.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// originMarkerFile is the file an origin-only weft branch carries, so a test can tell the branch adopted from origin from one forked locally.
const originMarkerFile = "origin-marker.txt"

// pushOriginOnlyWeftBranch pushes weftBranch to the hub's weft bare from a scratch clone, carrying originMarkerFile, and returns its tip.
// With archiveTag set, it also pushes an archive/<slug>/<tip> tag covering that tip.
func pushOriginOnlyWeftBranch(t *testing.T, h *hubforge.Hub, slug, weftBranch string, archiveTag bool) string {
	t.Helper()

	clone := t.TempDir()
	gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.WeftBare, ".")
	gitkit.MustRun(t, clone, "git", "checkout", "--quiet", "-b", weftBranch)
	tip := gitkit.CommitFile(t, clone, originMarkerFile, "from origin\n", "origin-only weft branch")
	gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
	if archiveTag {
		tag := "archive/" + slug + "/" + tip[:12]
		gitkit.MustRun(t, clone, "git", "tag", tag, tip)
		gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", "refs/tags/"+tag)
	}
	return tip
}

// addPairWithLocalWarpBranch adds a pair for slug and creates the local warp branch branch, whose weft counterpart the caller controls.
func addPairWithLocalWarpBranch(t *testing.T, h *hubforge.Hub, slug, branch string) {
	t.Helper()

	if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
		t.Fatalf("add %s exit = %d; output: %s", slug, code, output)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", branch)
}

// requireOnOriginalBranches asserts the pair's warp and weft worktrees are still on the pair's own branches.
func requireOnOriginalBranches(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if got := gitkit.CurrentBranch(t, h.PairWarpWorktree(slug)); got != slug {
		t.Errorf("warp branch = %q; want %q (unchanged)", got, slug)
	}
	if got, want := gitkit.CurrentBranch(t, h.PairWeftSibling(slug)), fabricengine.WeftBranchName(slug); got != want {
		t.Errorf("weft branch = %q; want %q (unchanged)", got, want)
	}
}

// TestRunCLI_AdoptRemoteWeftScenario runs the checkout checks over one hub.
// Steps run serially, each on its own pair; the step that removes the weft repo's origin comes last.
// The scenario calls t.Parallel as a whole; no step does, because they share the one hub.
func TestRunCLI_AdoptRemoteWeftScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"CheckoutAdoptsOriginOnlyWeftBranch", func(t *testing.T) {
			// A weft branch present only on origin is adopted as a local branch tracking it, with or without an archive tag covering its tip.
			for _, tc := range []struct {
				slug, branch string
				archiveTag   bool
			}{
				{"co-adopt", "co-adopt-b", false},
				{"co-archived", "co-archived-b", true},
			} {
				addPairWithLocalWarpBranch(t, h, tc.slug, tc.branch)
				weftBranch := fabricengine.WeftBranchName(tc.branch)
				tip := pushOriginOnlyWeftBranch(t, h, tc.slug, weftBranch, tc.archiveTag)

				if code, output := runFabric(t, h.PairWarpWorktree(tc.slug), "checkout", tc.branch); code != 0 {
					t.Fatalf("checkout %s (archiveTag=%v) exit = %d; output: %s", tc.branch, tc.archiveTag, code, output)
				}

				weft := h.PairWeftSibling(tc.slug)
				if got := gitkit.CurrentBranch(t, weft); got != weftBranch {
					t.Errorf("weft branch = %q; want %q", got, weftBranch)
				}
				if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
					t.Errorf("weft HEAD = %s; want origin's tip %s", got, tip)
				}
				if got := gitkit.Git(t, weft, "rev-parse", "--abbrev-ref", weftBranch+"@{upstream}"); got != "origin/"+weftBranch {
					t.Errorf("upstream of %s = %q; want origin/%s", weftBranch, got, weftBranch)
				}
				if !strings.Contains(gitkit.Git(t, weft, "ls-files"), originMarkerFile) {
					t.Errorf("weft worktree does not track %s; want the origin branch's content", originMarkerFile)
				}
			}
		}},
		{"CheckoutRefusesUnreachableOrigin", func(t *testing.T) {
			// With origin unreachable and no local weft branch, checkout stops naming origin instead of forking, leaving both sides where they were.
			const slug, branch = "co-unreachable", "co-unreachable-b"
			addPairWithLocalWarpBranch(t, h, slug, branch)

			weftRepo := h.PrimeWeft()
			gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", filepath.Join(h.Container, "missing-origin"))
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", h.WeftBare)

			code, output := runFabric(t, h.PairWarpWorktree(slug), "checkout", branch)
			if code == 0 {
				t.Fatalf("checkout with an unreachable origin exit = 0; want non-zero\noutput: %s", output)
			}
			if !strings.Contains(output, "origin") {
				t.Errorf("output %q does not name origin", output)
			}
			requireOnOriginalBranches(t, h, slug)
			if gitkit.BranchExists(t, weftRepo, fabricengine.WeftBranchName(branch)) {
				t.Errorf("local weft branch %q exists after the refused checkout; want none", fabricengine.WeftBranchName(branch))
			}
		}},
		{"CheckoutForksWithoutOriginRemote", func(t *testing.T) {
			// A weft repo with no origin remote skips the origin step and forks the branch as before.
			const slug, branch = "co-noorigin", "co-noorigin-b"
			addPairWithLocalWarpBranch(t, h, slug, branch)

			weftRepo := h.PrimeWeft()
			gitkit.MustRun(t, weftRepo, "git", "remote", "remove", "origin")
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "add", "origin", h.WeftBare)

			if code, output := runFabric(t, h.PairWarpWorktree(slug), "checkout", branch); code != 0 {
				t.Fatalf("checkout without an origin remote exit = %d; output: %s", code, output)
			}
			weftBranch := fabricengine.WeftBranchName(branch)
			if got := gitkit.CurrentBranch(t, h.PairWeftSibling(slug)); got != weftBranch {
				t.Errorf("weft branch = %q; want the forked %q", got, weftBranch)
			}
			if strings.Contains(strings.Join(gitkit.LsFiles(t, h.PairWeftSibling(slug)), "\n"), originMarkerFile) {
				t.Errorf("forked weft branch tracks %s; want a fork of the pair's own branch", originMarkerFile)
			}
		}},
	}

	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			break
		}
	}
}
