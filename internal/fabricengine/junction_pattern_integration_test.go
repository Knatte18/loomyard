// junction_pattern_integration_test.go covers junction wiring, health, and unwiring in a repository with two junctions:
// _lym and a second, non-_lym junction.
// It checks the health of each site: reconcile, status, and drift.
// It also checks per-junction refusal and repoint and unwiring across every junction.
// Wiring is idempotent, including the upgrade from one junction to two.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// resetWarpJunction removes the warp-side junction named name, if one is wired.
// A test uses it to start from an unwired state, not from the junction the hub fixture wires for _lym.
// The test can then exercise seedLymJunction's real-directory refusal, creation, and no-op unwire paths.
func resetWarpJunction(t *testing.T, l *lymcwd.Location, slug, name string) string {
	t.Helper()

	link := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, name)
	if err := fslink.Remove(link); err != nil {
		t.Fatalf("reset %s junction at %s: %v", name, link, err)
	}
	return link
}

// extraFabricConfigYAML is the repo-wide fabric.yaml override that tests seed.
// It names "_extra" as the pathspec, instead of the default from fabricengine.ConfigTemplate().
// Production code that consumes RepoWiredNames reads fabric.yaml to decide which optional junction it expects wired.
// A test that wires "_extra" with WireJunctions must also set RepoWiredNames to "_extra" to match the disk.
// A case seeds the file with hubforge.SeedFabricConfig, or with seedRepoWideExtraFabricConfig.
const extraFabricConfigYAML = "branch_prefix: \"\"\npathspec: _extra\n"

// seedRepoWideExtraFabricConfig overwrites the repo-wide fabric.yaml at fabricengine.BoardDir(hub)
// with extraFabricConfigYAML.
// It takes a bare hub path rather than a *hubforge.Hub.
func seedRepoWideExtraFabricConfig(t testing.TB, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte(extraFabricConfigYAML), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// TestWireJunctions_MaterialisesMissingWeftTarget checks that seedLymJunction creates a missing weft-side target.
// The junction then resolves at once.
// A second WireJunctions call on the same worktree must also succeed, as the checkout and reconcile path needs.
//
//testtiming:keep WireJunctions creating a missing weft target so the junction resolves at once and a second call succeeds; coverage of its blocks by other tests does not show an assertion of this
func TestWireJunctions_MaterialisesMissingWeftTarget(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	target := fabricengine.WeftLymDirFor(l, slug)

	// The weft-prime template pre-seeds _lym/config/placeholder; remove the whole target directory,
	// so the junction points at a target that does not exist, as a checkout or reconcile leaves it.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove weft target %s: %v", target, err)
	}

	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions with missing weft target: %v", err)
	}

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("weft target %s not materialised: stat err=%v", target, err)
	}

	link := fabricengine.CodeLymLink(l, slug)
	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	if _, err := fslink.PointsTo(link); err != nil {
		t.Errorf("junction at %s does not resolve immediately after WireJunctions: %v", link, err)
	}

	// A second WireJunctions on the same worktree must succeed.
	// The checkout and reconcile path makes that call, and the link-exists branch must cope with a still-missing target.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("second WireJunctions = %v; want nil (self-repair path)", err)
	}
}

// TestWireJunctions_RefusesRealWarpDirectory checks that WireJunctions refuses a real, non-link
// directory at the warp junction path: fabric never moves or deletes user content.
// The returned error names the offending path and the remedy, re-running `lym fabric reconcile`.
//
//testtiming:keep a real directory at the warp junction path being refused with the path and the reconcile remedy named; coverage of its blocks by other tests does not show an assertion of this
func TestWireJunctions_RefusesRealWarpDirectory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	link := resetWarpJunction(t, l, slug, lymdirs.LymDirName)

	// Seed a real, non-link directory at the warp junction path, the mistake of creating _lym by hand.
	// The error must guide an operator away from it, and so must the same mistake under an optional junction name.
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatalf("mkdir real warp dir %s: %v", link, err)
	}
	marker := filepath.Join(link, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"})
	if err == nil {
		t.Fatal("WireJunctions = nil; want error refusing a real warp directory")
	}

	msg := err.Error()
	if !strings.Contains(msg, link) {
		t.Errorf("error %q does not name the offending path %q", msg, link)
	}
	if !strings.Contains(msg, "lym fabric reconcile") {
		t.Errorf("error %q does not name the re-run-`lym fabric reconcile` remedy", msg)
	}

	// The real directory and its content must stay untouched, since fabric never deletes or moves user content.
	content, readErr := os.ReadFile(marker)
	if readErr != nil {
		t.Fatalf("read marker after refused WireJunctions: %v", readErr)
	}
	if string(content) != "real content" {
		t.Errorf("marker content changed: %q", string(content))
	}
}

// TestUnwireJunctions_ReportsAndClearsEveryJunction checks that unwiring reports every junction.
// Each junction's Name appears in UnwireResult.JunctionsRemoved, and its line is removed from .git/info/exclude.
// The test runs against two junctions: _lym and a second, non-_lym junction.
//
//testtiming:keep unwiring reporting every junction in JunctionsRemoved and clearing every exclude line; coverage of its blocks by other tests does not show an assertion of this
func TestUnwireJunctions_ReportsAndClearsEveryJunction(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName

	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}
	for _, name := range []string{lymdirs.LymDirName, "_extra"} {
		pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, name)
		if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); !containsLine(lines, pattern) {
			t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", pattern, lines)
		}
	}

	result, err := fabricengine.UnwireJunctions(l, slug, []string{"_lym", "_extra"})
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}

	if want := []string{lymdirs.LymDirName, "_extra"}; !slices.Equal(result.JunctionsRemoved, want) {
		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
	}
	if !result.ExcludeChanged {
		t.Error("ExcludeChanged = false; want true")
	}

	lymLink := fabricengine.CodeLymLink(l, slug)
	if _, statErr := os.Lstat(lymLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", lymLink)
	}
	extraLink := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", extraLink)
	}
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); containsLine(lines, lymdirs.LymDirName) {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", lymdirs.LymDirName, lines)
	}
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); containsLine(lines, "_extra") {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", "_extra", lines)
	}
}

// TestUnwireJunctions_AlreadyUnwiredIsNoOp checks that unwiring an already unwired worktree is a no-op.
// Unwiring returns an empty JunctionsRemoved and a nil error.
//
//testtiming:keep a second UnwireJunctions returning an empty JunctionsRemoved, ExcludeChanged false and a nil error, which no other test asserts
func TestUnwireJunctions_AlreadyUnwiredIsNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName

	// Unwire _lym once first: hubforge's CloneAndWire already wires it.
	// The call below then runs the no-op path on a worktree with no junction left to remove.
	if _, err := fabricengine.UnwireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("pre-unwire: %v", err)
	}

	result, err := fabricengine.UnwireJunctions(l, slug, []string{"_lym", "_extra"})
	if err != nil {
		t.Fatalf("UnwireJunctions on never-wired worktree = %v; want nil", err)
	}
	if len(result.JunctionsRemoved) != 0 {
		t.Errorf("JunctionsRemoved = %v; want empty", result.JunctionsRemoved)
	}
	if result.ExcludeChanged {
		t.Error("ExcludeChanged = true; want false")
	}
}

// containsLine reports whether a line, with surrounding space trimmed, equals name exactly.
func containsLine(lines []string, name string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// TestDetectWarpPollution_LymTrackedAsRestorable checks the restore remedy for a tracked _lym path in the warp index.
// The path is reported as pollution with a git rm --cached remedy.
// The remedy also reminds to restore the junction and exclude entry.
// Every pollution class this scan reports carries a remedy.
//
//testtiming:keep a tracked _lym path in the warp index being reported with an automated restore remedy; coverage of its blocks by other tests does not show an assertion of this
func TestDetectWarpPollution_LymTrackedAsRestorable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// hubforge's CloneAndWire wires _lym as a junction and excludes it in .git/info/exclude.
	// Unwiring it first clears both, so the test can track a real _lym directory.
	// While the exclude entry stays, git add refuses the path.
	if _, err := fabricengine.UnwireJunctions(l, l.WorktreeName, []string{lymdirs.LymDirName}); err != nil {
		t.Fatalf("pre-unwire _lym: %v", err)
	}

	// Track a file under _lym directly in the warp worktree's index.
	// This is the mistake the scan exists to catch: hand-authored _lym content committed to warp.
	warpLymDir := filepath.Join(l.WorktreePath(), lymdirs.LymDirName)
	if err := os.MkdirAll(warpLymDir, 0o755); err != nil {
		t.Fatalf("mkdir warp _lym dir: %v", err)
	}
	trackedFile := filepath.Join(warpLymDir, "PATTERN.md")
	if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitkit.MustRun(t, l.WorktreePath(), "git", "add", "--", lymdirs.LymDirName)
	gitkit.MustRun(t, l.WorktreePath(), "git", "commit", "-m", "accidentally track _lym")

	topology := h.Topology
	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status returned no pairs")
	}

	wantPath := lymdirs.LymDirName + "/PATTERN.md"
	var found *fabricengine.PollutionEntry
	for i, entry := range result.Pairs[0].Pollution {
		if entry.Path == wantPath {
			found = &result.Pairs[0].Pollution[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no pollution entry for %q found in %+v", wantPath, result.Pairs[0].Pollution)
	}
	if found.Remedy == "" {
		t.Errorf("PollutionEntry for %q has empty Remedy; want an automated git rm --cached remedy", wantPath)
	}
	if !strings.Contains(found.Remedy, "rm --cached") {
		t.Errorf("PollutionEntry for %q remedy = %q; want it to contain \"rm --cached\"", wantPath, found.Remedy)
	}
}

// TestDetectWarpPollution_ScanErrorIsNonFatal checks that a detectWarpPollution failure does not fail Status.
// The pair still comes back with a synthetic "<scan error: ...>" pollution entry that has an empty Remedy.
// The empty Remedy alone signals that no automated remedy applies.
func TestDetectWarpPollution_ScanErrorIsNonFatal(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// Corrupt only the warp worktree's index file so `git ls-files` fails inside detectWarpPollution.
	// That forces Status down its scan-error branch.
	// `git worktree list` does not read the index, so it still succeeds.
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "index"}, l.WorktreePath())
	if err != nil || exitCode != 0 {
		t.Fatalf("git rev-parse --git-path index failed: %v (exit %d)", err, exitCode)
	}
	indexPath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(l.WorktreePath(), indexPath)
	}
	if err := os.WriteFile(indexPath, []byte("not a git index"), 0o644); err != nil {
		t.Fatalf("corrupt warp index file: %v", err)
	}

	topology := h.Topology
	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status returned no pairs")
	}

	pollution := result.Pairs[0].Pollution
	if len(pollution) != 1 {
		t.Fatalf("Pollution = %+v; want exactly one synthetic scan-error entry", pollution)
	}
	if !strings.Contains(pollution[0].Path, "<scan error:") {
		t.Errorf("Pollution[0].Path = %q; want it to contain the scan-error marker", pollution[0].Path)
	}
	if pollution[0].Remedy != "" {
		t.Errorf("Pollution[0].Remedy = %q; want empty for a scan-error entry", pollution[0].Remedy)
	}
}

// TestDetectWarpPollution_RaddleNoLongerReported checks that the scan skips tracked _raddle paths.
// The scan's git-ls-files pathspec does not name _raddle.
//
//testtiming:keep a tracked _raddle path no longer being reported as pollution; coverage of its blocks by other tests does not show an assertion of this
func TestDetectWarpPollution_RaddleNoLongerReported(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	warpRaddleDir := filepath.Join(l.WorktreePath(), "_raddle")
	if err := os.MkdirAll(warpRaddleDir, 0o755); err != nil {
		t.Fatalf("mkdir warp _raddle dir: %v", err)
	}
	trackedFile := filepath.Join(warpRaddleDir, "notes.md")
	if err := os.WriteFile(trackedFile, []byte("# notes\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitkit.MustRun(t, l.WorktreePath(), "git", "add", "--", "_raddle")
	gitkit.MustRun(t, l.WorktreePath(), "git", "commit", "-m", "accidentally track _raddle")

	topology := h.Topology
	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status returned no pairs")
	}

	for _, entry := range result.Pairs[0].Pollution {
		if strings.HasPrefix(entry.Path, "_raddle") {
			t.Errorf("Pollution contains %+v; _raddle is no longer a scanned pollution class", entry)
		}
	}
}

// TestStatus_ReportsOptionalJunctionUnhealthy checks that Status reports an unhealthy non-_lym junction.
// With _lym healthy and the second junction mis-pointed, Status must report JunctionHealthy false:
// it sets a JunctionReason naming that junction, and the pair is not in sync.
// Status computes its verdict on its own, separate from Healthy's, so it needs this test.
func TestStatus_ReportsOptionalJunctionUnhealthy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "status-extra-unhealthy"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	// hubforge.NewHub seeds the repo-wide config with the default pathspec of fabricengine.ConfigTemplate().
	// Override it to "_extra" so the junction Add wires is the one this test drifts.
	seedRepoWideExtraFabricConfig(t, l.HubPath)
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// _lym stays healthy; only _extra is re-pointed at an unrelated directory.
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove _extra junction: %v", err)
	}
	wrongTarget := filepath.Join(h.PrimeWorktree(), "not-the-weft-extra-dir")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir wrong target: %v", err)
	}
	if err := fslink.CreateDirLink(extraLink, wrongTarget); err != nil {
		t.Fatalf("seed wrong-target junction: %v", err)
	}

	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	warpPath := fabricengine.WorktreePath(l, slug)
	var found bool
	for _, pair := range result.Pairs {
		if pair.CodeWorktree != filepath.ToSlash(warpPath) {
			continue
		}
		found = true
		if pair.JunctionHealthy {
			t.Error("JunctionHealthy = true; want false")
		}
		if !strings.Contains(pair.JunctionReason, "_extra") {
			t.Errorf("JunctionReason = %q; want it to name %q", pair.JunctionReason, "_extra")
		}
		if pair.InSync {
			t.Error("InSync = true; want false")
		}
	}
	if !found {
		t.Fatalf("Status result has no pair for warp path %s: %+v", warpPath, result.Pairs)
	}
}

// TestWireJunctions_UpgradesLymOnlyWorktreeToBoth checks the upgrade path.
// The worktree has _lym wired and the second, non-_lym junction missing.
// WireJunctions must complete without error and add only the missing junction.
// The healthy _lym junction must stay untouched.
//
//testtiming:keep a worktree with _lym wired and the second junction missing gaining only the missing one; coverage of its blocks by other tests does not show an assertion of this
func TestWireJunctions_UpgradesLymOnlyWorktreeToBoth(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName

	// Wire both junctions once, then remove _extra only — _lym stays fully
	// healthy, _extra becomes the only missing junction.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (initial): %v", err)
	}
	extraLink := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove _extra junction to simulate a worktree missing it: %v", err)
	}

	lymLink := fabricengine.CodeLymLink(l, slug)
	lymResolvedBefore, err := fslink.PointsTo(lymLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) before upgrade: %v", lymLink, err)
	}

	// The upgrade call: must complete without error and wire only the missing
	// _extra junction.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (upgrade): %v", err)
	}

	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Fatalf("_extra junction %s not wired by upgrade call: isLink=%v err=%v", extraLink, isLink, err)
	}
	lymResolvedAfter, err := fslink.PointsTo(lymLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) after upgrade: %v", lymLink, err)
	}
	if lymResolvedBefore != lymResolvedAfter {
		t.Errorf("_lym junction target changed across upgrade call: before=%s after=%s", lymResolvedBefore, lymResolvedAfter)
	}

	// Wiring twice (the already-upgraded worktree) is a no-op.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (idempotent re-run): %v", err)
	}
}

// TestSeedGitExclude_AnchorsPatternAndReplacesLegacyBareName checks the exclude entry.
// It names exactly the one junction path fabric wires, not every same-named directory in the repo.
// A gitignore pattern with no slash matches at any depth.
// A bare "_lym" line would silently untrack a monorepo's own frontend/_lym/ on a subpath-anchored hub.
// A hub that already holds the bare line must end up with the anchored pattern only.
func TestSeedGitExclude_AnchorsPatternAndReplacesLegacyBareName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName

	// Seed the exclude file with the bare-name line an older binary wrote.
	excludePath := excludeFilePath(t, l, slug)
	if err := os.WriteFile(excludePath, []byte(lymdirs.LymDirName+"\n"), 0o644); err != nil {
		t.Fatalf("seed legacy exclude line: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug, []string{lymdirs.LymDirName}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug))
	pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, lymdirs.LymDirName)
	if !containsLine(lines, pattern) {
		t.Errorf(".git/info/exclude = %v; want it to contain the anchored pattern %q", lines, pattern)
	}
	if containsLine(lines, lymdirs.LymDirName) {
		t.Errorf(".git/info/exclude = %v; want the legacy bare %q line replaced, not kept beside the anchored pattern",
			lines, lymdirs.LymDirName)
	}

	// The anchored pattern must not reach a same-named directory elsewhere in the repo.
	elsewhere := filepath.Join(l.WorktreePath(), "frontend", lymdirs.LymDirName)
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatalf("create same-named directory elsewhere: %v", err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "note.md"), []byte("tracked by the user\n"), 0o644); err != nil {
		t.Fatalf("write file under the same-named directory: %v", err)
	}
	stdout, stderr, exitCode, err := gitexec.RunGit(
		[]string{"status", "--porcelain", "--untracked-files=all", "--", "frontend"},
		l.WorktreePath(),
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("git status: err=%v exit=%d stderr=%s", err, exitCode, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("git sees nothing under frontend/%s; the exclude pattern reached a directory fabric never wired", lymdirs.LymDirName)
	}
}

// excludeFilePath resolves the .git/info/exclude path of the worktree named slug and creates its parent directory.
func excludeFilePath(t *testing.T, l *lymcwd.Location, slug string) string {
	t.Helper()
	worktreePath := fabricengine.WorktreePath(l, slug)
	excludePath := gitkit.Git(t, worktreePath, "rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatalf("mkdir exclude dir: %v", err)
	}
	return excludePath
}
