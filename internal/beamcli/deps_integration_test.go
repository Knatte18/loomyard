// deps_integration_test.go drives hubLandDeps's seams against a real hub pair and real git.

package beamcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubLandDeps_ReadyReads drives the seams that `ready` judges a beam by over a real hub pair.
// It reads the beam's pull_request through the per-task lookup, the beam's own worktree first and its parents' after.
// It reads whether the beam holds the parent's commits and whether it conflicts with them.
// It also reads which beam commits a merged pull request lacks and the change description's findings.
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
	beamLayer := configengine.ConfigFile(beam.AnchorPath(), "beam")
	if err := os.MkdirAll(filepath.Dir(beamLayer), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(beamLayer), err)
	}
	if err := os.WriteFile(beamLayer, []byte("pull_request: \"trail\"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", beamLayer, err)
	}
	if mode, _, err := deps.PullRequestMode("b"); mode != "trail" || err != nil {
		t.Errorf("PullRequestMode(b) with the beam's own beam.yaml = %q, %v; want trail and no error", mode, err)
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
	if conflicts, err := deps.ParentConflicts("b"); conflicts || err != nil {
		t.Errorf("ParentConflicts(b) = %t, %v for a parent commit on a file the beam leaves alone; want false", conflicts, err)
	}

	// With no upstream the parent is judged by its local tip alone.
	// The unset fails for a parent that tracks nothing, which leaves it as wanted.
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

	gitkit.CommitFile(t, h.PrimeWorktree(), "shared.txt", "parent\n", "Parent writes shared")
	gitkit.CommitFile(t, worktree, "shared.txt", "beam\n", "Beam writes shared")
	if conflicts, err := deps.ParentConflicts("b"); !conflicts || err != nil {
		t.Errorf("ParentConflicts(b) = %t, %v after both sides wrote shared.txt apart; want true", conflicts, err)
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
	if err := os.WriteFile(descriptionPath, []byte("# Ship the beam\n\nGroups the work.\n\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** beam\n"), 0o644); err != nil {
		t.Fatalf("write the description: %v", err)
	}
	if findings, err := deps.DescriptionFindings("b"); len(findings) != 0 || err != nil {
		t.Errorf("DescriptionFindings(b) = %v, %v for a valid description; want none", findings, err)
	}
}

// TestHubLandDeps_HeldSlugs drives the HeldSlugs seam over a real hub pair.
// A slug a run directory holds and a slug a task branch holds are both listed.
func TestHubLandDeps_HeldSlugs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	if err := os.MkdirAll(shedrun.RunDir(prime, "held-run"), 0o755); err != nil {
		t.Fatalf("create the run directory: %v", err)
	}
	gitkit.Git(t, h.PrimeWorktree(), "branch", "held-branch")

	held, err := hubLandDeps(prime).HeldSlugs()
	if err != nil {
		t.Fatalf("HeldSlugs() error = %v; want nil", err)
	}
	for _, slug := range []string{"held-run", "held-branch"} {
		if !slices.Contains(held, slug) {
			t.Errorf("HeldSlugs() = %v; want it to hold %q", held, slug)
		}
	}
}

// TestHubLandDeps_ParentUnmergedReadsTheUpstreamTip drives ParentUnmerged over a hub whose parent has an upstream.
// The push skip is unset, and the origin is a local bare repository.
// A commit another clone pushed to the origin is fetched and judged.
// A beam holding the local tip is therefore unmerged until it merges the upstream tip in.
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

// TestHubLandDeps_DraftThenRecordStampsAndCommits asserts the DraftDescription seam lists a member's landing.
// It also asserts that the draft skips a records-only commit.
// Recording the draft stamps the landing record with the beam's slug and the time, and both files go in one commit.
// The describer agent's facts list the same landing, and a draft recorded from its own path replaces the description.
// Under the template's landing.yaml, both recorders and the findings seam refuse a line holding a forbidden term.
func TestHubLandDeps_DraftThenRecordStampsAndCommits(t *testing.T) {
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

	deps := hubLandDeps(prime)
	path, err := deps.DraftDescription("b")
	if err != nil {
		t.Fatalf("DraftDescription() error = %v; want nil", err)
	}
	// The draft's Merge danger placeholders are refused on purpose; the orch replaces them with a call before recording.
	draft, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the draft: %v", err)
	}
	called := strings.NewReplacer("one-way or two-way", "two-way", "one word", "widgets").Replace(string(draft))
	if err := os.WriteFile(path, []byte(called), 0o644); err != nil {
		t.Fatalf("write the orch's merge danger call: %v", err)
	}
	if recorded, err := deps.RecordDescription("b", "", at); err != nil || recorded != path {
		t.Fatalf("RecordDescription() = %q, %v; want %q, nil", recorded, err, path)
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

	facts, err := beamDescribeFacts(prime, beam, "b")
	if err != nil {
		t.Fatalf("beamDescribeFacts() = _, %v; want nil", err)
	}
	origin, _, err := deps.OriginRecord("b")
	if err != nil {
		t.Fatalf("OriginRecord(b) = _, _, %v; want nil", err)
	}
	if facts.DescriptionPath != path || facts.ParentBranch != origin.ParentBranch || facts.Branch != gitkit.CurrentBranch(t, worktree) {
		t.Errorf("beamDescribeFacts() = %+v; want the description %s, the beam's branch and the parent %s", facts, path, origin.ParentBranch)
	}
	if !strings.Contains(facts.Changes, "#12 Add widgets") || strings.Contains(facts.Changes, "bookkeeping") {
		t.Errorf("beamDescribeFacts().Changes = %q; want the landing listed and not the records-only commit", facts.Changes)
	}

	rewritten := "# Ship the beam\n\nGroups the work and its widgets.\n\n- #12 Add widgets\n\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** widgets\n"
	draftPath := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(draftPath, []byte(rewritten), 0o644); err != nil {
		t.Fatalf("write the describer's draft: %v", err)
	}
	if recorded, err := deps.RecordDescription("b", draftPath, at.Add(time.Hour)); err != nil || recorded != path {
		t.Fatalf("RecordDescription(from the draft) = %q, %v; want %q, nil", recorded, err, path)
	}
	if description, err := os.ReadFile(path); err != nil || string(description) != rewritten {
		t.Errorf("description after the rewrite = %q, %v; want the draft %q", description, err, rewritten)
	}

	landingConfig := configengine.ConfigFile(h.BoardDir(), "landing")
	if err := os.MkdirAll(filepath.Dir(landingConfig), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(landingConfig), err)
	}
	if err := os.WriteFile(landingConfig, []byte(landingshed.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write %s: %v", landingConfig, err)
	}
	forbidden := strings.Replace(rewritten, "Groups the work and its widgets.", "Run lym.", 1)
	if err := os.WriteFile(draftPath, []byte(forbidden), 0o644); err != nil {
		t.Fatalf("write the describer's draft: %v", err)
	}
	if _, err := deps.RecordDescription("b", draftPath, at); err == nil || !strings.Contains(err.Error(), summaryparser.CheckForbiddenTerm) {
		t.Errorf("RecordDescription(from a draft naming lym) = %v; want the %s refusal", err, summaryparser.CheckForbiddenTerm)
	}
	if err := os.WriteFile(path, []byte(forbidden), 0o644); err != nil {
		t.Fatalf("write the description: %v", err)
	}
	if _, err := deps.RecordDescription("b", "", at); err == nil || !strings.Contains(err.Error(), summaryparser.CheckForbiddenTerm) {
		t.Errorf("RecordDescription(of a description naming lym) = %v; want the %s refusal", err, summaryparser.CheckForbiddenTerm)
	}
	if findings, err := deps.DescriptionFindings("b"); err != nil || len(findings) != 1 || !strings.Contains(findings[0], summaryparser.CheckForbiddenTerm) {
		t.Errorf("DescriptionFindings(b) = %v, %v for a description naming lym; want the one %s finding", findings, err, summaryparser.CheckForbiddenTerm)
	}
}
