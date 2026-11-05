// deps_integration_test.go drives hubLandDeps's seams against a real hub pair and real git.

package beamcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubLandDeps_ReadyReads drives the seams `ready` judges a beam by over a real hub pair: the parent's pull request mode, whether the beam holds the parent's commits, which beam commits a merged pull request lacks and the change description's findings.
func TestHubLandDeps_ReadyReads(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	worktree := h.PairCodeWorktree("b")
	beam, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("resolve the beam: %v", err)
	}
	deps := hubLandDeps(prime)
	origin, found, err := deps.OriginRecord("b")
	if err != nil || !found {
		t.Fatalf("OriginRecord(b) = %+v, %v, %v; want the beam's origin record", origin, found, err)
	}

	// The parent's origin is the fixture's local path, not on GitHub, so no rule is read.
	layer := configengine.ConfigFile(prime.AnchorPath(), "beam")
	if err := os.MkdirAll(filepath.Dir(layer), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(layer), err)
	}
	if err := os.WriteFile(layer, []byte("pull_request: \"off\"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", layer, err)
	}
	mode, rulesRequired, err := deps.PullRequestMode("b")
	if mode != "off" || rulesRequired || err != nil {
		t.Errorf("PullRequestMode(b) = %q, %t, %v; want off, no rule and no error", mode, rulesRequired, err)
	}

	gitkit.CommitFile(t, worktree, "widgets.txt", "widgets\n", "Add widgets")
	mergedHead := gitkit.Git(t, worktree, "rev-parse", "HEAD")
	if unmerged, err := deps.ParentUnmerged("b"); unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v for a beam holding the parent's tip; want false", unmerged, err)
	}
	gitkit.CommitFile(t, h.PrimeWorktree(), "parent.txt", "parent\n", "Parent commit")
	if unmerged, err := deps.ParentUnmerged("b"); !unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v after a parent commit; want true", unmerged, err)
	}

	// With no upstream the parent is judged by its local tip alone; a parent that tracks nothing already makes the unset fail, which leaves it as wanted.
	_ = exec.Command("git", "-C", h.PrimeWorktree(), "branch", "--unset-upstream").Run()
	if unmerged, err := deps.ParentUnmerged("b"); !unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v for a parent with no upstream and a commit the beam lacks; want true", unmerged, err)
	}
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the parent", origin.ParentBranch)
	if unmerged, err := deps.ParentUnmerged("b"); unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v after the merge of the parent; want false", unmerged, err)
	}

	if lacking, err := deps.MergedLacks("b", mergedHead); len(lacking) != 0 || err != nil {
		t.Errorf("MergedLacks(b) = %v, %v for a merged head followed only by a merge of the parent; want none", lacking, err)
	}
	gitkit.CommitFile(t, worktree, "gadgets.txt", "gadgets\n", "Add gadgets")
	lacking, err := deps.MergedLacks("b", mergedHead)
	if err != nil || len(lacking) != 1 || !strings.Contains(lacking[0], "Add gadgets") {
		t.Errorf("MergedLacks(b) = %v, %v after a beam commit; want the one commit named", lacking, err)
	}

	descriptionPath := summaryparser.Path(loomengine.LandingDir(beam))
	if err := os.MkdirAll(filepath.Dir(descriptionPath), 0o755); err != nil {
		t.Fatalf("create the landing directory: %v", err)
	}
	if err := os.WriteFile(descriptionPath, []byte("# Ship the beam\n"), 0o644); err != nil {
		t.Fatalf("write the description: %v", err)
	}
	findings, err := deps.DescriptionFindings("b")
	if err != nil || len(findings) != 1 || !strings.Contains(findings[0], summaryparser.CheckEmptyBody) {
		t.Errorf("DescriptionFindings(b) = %v, %v for an empty body; want the one %s finding", findings, err, summaryparser.CheckEmptyBody)
	}
	if err := os.WriteFile(descriptionPath, []byte("# Ship the beam\n\nGroups the work.\n"), 0o644); err != nil {
		t.Fatalf("write the description: %v", err)
	}
	if findings, err := deps.DescriptionFindings("b"); len(findings) != 0 || err != nil {
		t.Errorf("DescriptionFindings(b) = %v, %v for a valid description; want none", findings, err)
	}
}

// TestHubLandDeps_ParentUnmergedReadsTheUpstreamTip drives ParentUnmerged over a hub whose parent branch has an upstream and whose push skip is unset: a commit another clone pushed to the origin, a local bare repository, is fetched and judged, so a beam holding the local tip is unmerged until it merges the upstream tip in.
func TestHubLandDeps_ParentUnmergedReadsTheUpstreamTip(t *testing.T) {
	tiergate.Require(t, 1)
	// Not parallel: it clears the push skip in the process environment, which ParentUnmerged reads.
	t.Setenv("FABRIC_SKIP_PUSH", "")

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	worktree := h.PairCodeWorktree("b")
	parentBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	deps := hubLandDeps(prime)

	if unmerged, err := deps.ParentUnmerged("b"); unmerged || err != nil {
		t.Fatalf("ParentUnmerged(b) = %t, %v for a beam holding both parent tips; want false", unmerged, err)
	}

	other := filepath.Join(t.TempDir(), "other")
	gitkit.Git(t, filepath.Dir(other), "clone", h.CodeBare, other)
	gitkit.CommitFile(t, other, "upstream.txt", "upstream\n", "Upstream commit")
	gitkit.Git(t, other, "push", "origin", "HEAD:"+parentBranch)
	if unmerged, err := deps.ParentUnmerged("b"); !unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v after the origin's parent moved past the local tip; want true", unmerged, err)
	}

	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the upstream", "origin/"+parentBranch)
	if unmerged, err := deps.ParentUnmerged("b"); unmerged || err != nil {
		t.Errorf("ParentUnmerged(b) = %t, %v after the beam merged the upstream tip; want false", unmerged, err)
	}
}

// TestHubLandDeps_DescribeStampsAndCommits asserts the Describe seam lists a member's landing and not a records-only commit, stamps the landing record with the beam's slug and the time, and commits both files in one commit.
func TestHubLandDeps_DescribeStampsAndCommits(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	entry := map[string]any{"slug": "b", "kind": "project", "title": "Ship the beam", "brief": "Groups the work.", "labels": []string{"bug"}}
	if _, err := boardengine.New(cfg).UpsertTask(entry); err != nil {
		t.Fatalf("seed the beam's board entry: %v", err)
	}

	worktree := h.PairCodeWorktree("b")
	gitkit.CommitFile(t, worktree, "widgets.txt", "widgets\n", "Add widgets (#12)\n\nAdds the widgets.")
	source := filepath.Join(t.TempDir(), "record")
	if err := os.WriteFile(source, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write the record's content: %v", err)
	}
	blob := gitkit.Git(t, worktree, "hash-object", "-w", source)
	gitkit.Git(t, worktree, "update-index", "--add", "--cacheinfo", "100644,"+blob+",_lym/landing/beam-ready.json")
	gitkit.Git(t, worktree, "commit", "-m", "beam: bookkeeping")

	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	beam, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("resolve the beam: %v", err)
	}
	at := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)

	path, err := hubLandDeps(prime).Describe("b", at)
	if err != nil {
		t.Fatalf("Describe() error = %v; want nil", err)
	}

	description, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the description: %v", err)
	}
	if !strings.Contains(string(description), "#12 Add widgets") || strings.Contains(string(description), "bookkeeping") {
		t.Errorf("description = %q; want it to list the landing and not the records-only commit", description)
	}
	record, found, err := pawlengine.ReadLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), "b")
	if err != nil || !found || !record.DescribedAt.Equal(at) {
		t.Errorf("ReadLanding() = %+v, %v, %v; want the record naming b with DescribedAt %v", record, found, err, at)
	}
	committed := gitkit.Git(t, fabricengine.RecordsWorktree(beam), "show", "--name-only", "--format=", "HEAD")
	for _, file := range []string{filepath.Base(path), filepath.Base(loomengine.BeamLandingPath(beam))} {
		if !strings.Contains(committed, file) {
			t.Errorf("HEAD of the records worktree changes %q; want it to hold %s", committed, file)
		}
	}
}
