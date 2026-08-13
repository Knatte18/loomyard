//go:build integration

// junction_pattern_integration_test.go covers the per-junction generalisation
// this batch makes to seedLyxJunction, unseedLyxJunction, checkJunctionHealth,
// and Healthy's inline junction check. From card 15 onward, WarpJunctions
// returns two entries (_lyx and a second, non-_lyx junction), so this file's
// cases now exercise a genuinely two-junction world: every generalisation
// from batch 3 — health check (per-site: reconcile, status, drift),
// per-junction refusal/repoint, unwire/remove looping, and wiring idempotency
// (including the single-junction-to-two upgrade path) — is proven against a
// real second, non-_lyx junction, not merely a loop of length one.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"fmt"
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// resetWarpJunction removes an already-wired warp-side junction named name (if any) so a test can
// exercise seedLyxJunction's real-directory-refusal, creation, or no-op-unwire path against a
// genuinely unwired starting point, rather than observing the wiring hubforge.NewHub's CloneAndWire
// already performed for _lyx.
func resetWarpJunction(t *testing.T, l *lyxcwd.Location, slug, name string) string {
	t.Helper()

	link := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, name)
	if err := fslink.Remove(link); err != nil {
		t.Fatalf("reset %s junction at %s: %v", name, link, err)
	}
	return link
}

// extraFabricConfigYAML is the repo-wide fabric.yaml override this file seeds wherever a case needs a
// pathspec naming "_extra" instead of fabricengine.ConfigTemplate()'s own default.
// RepoWiredNames-consuming production code (Healthy, Reconcile, Status, Add) reads this file to
// decide which optional junction it expects wired, so any test that wires "_extra" explicitly via
// WireJunctions must also point RepoWiredNames at "_extra" for that production code to agree with
// what is actually on disk.
// A hubforge.Hub-backed case seeds it via hubforge.SeedFabricConfig; the two cases still built on
// newFabricFixture's gitkit.PairedFixture shape (migrated separately, in the batch's reconcile card)
// seed it via seedRepoWideExtraFabricConfig below, which takes a bare hub path rather than a
// *hubforge.Hub.
const extraFabricConfigYAML = "branch_prefix: \"\"\npathspec: _extra\n"

// seedRepoWideExtraFabricConfig overwrites the repo-wide fabric.yaml at fabricengine.BoardDir(hub)
// with extraFabricConfigYAML. It is the bare-hub-path counterpart to hubforge.SeedFabricConfig, for
// the two newFabricFixture-based cases in this file that do not yet hold a *hubforge.Hub.
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

// readExcludeLines resolves and reads the warp worktree's .git/info/exclude
// file, mirroring the resolution logic seedGitExclude/unseedGitExclude use
// (git rev-parse --git-path info/exclude, joined with the worktree path if
// relative) so this test observes the same path the production code writes.
func readExcludeLines(t *testing.T, l *lyxcwd.Location, slug string) []string {
	t.Helper()

	worktreePath := fabricengine.WorktreePath(l, slug)
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if err != nil || exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed: %v (exit %d)", err, exitCode)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read exclude file: %v", err)
	}
	return strings.Split(string(content), "\n")
}

// TestWireJunctions_MaterialisesMissingWeftTarget is card 6's regression guard: seedLyxJunction
// must create the weft-side target directory when it is missing (the
// checkout/reconcile-left-dangling shape), leaving a junction that resolves immediately, and a
// second WireJunctions call on the same worktree must succeed rather than hard-erroring — the bug
// this card fixes.
func TestWireJunctions_MaterialisesMissingWeftTarget(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	target := fabricengine.WeftLyxDirFor(l, slug)

	// The weft-prime template pre-seeds _lyx/config/placeholder; remove the
	// whole target directory so it genuinely does not exist, matching the
	// state a checkout/reconcile-created junction points at today.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove weft target %s: %v", target, err)
	}

	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions with missing weft target: %v", err)
	}

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("weft target %s not materialised: stat err=%v", target, err)
	}

	link := fabricengine.WarpLyxLink(l, slug)
	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	if _, err := fslink.PointsTo(link); err != nil {
		t.Errorf("junction at %s does not resolve immediately after WireJunctions: %v", link, err)
	}

	// A second WireJunctions on the same worktree must succeed: this is the
	// checkout/reconcile path that hard-errored before this card, because the
	// link-exists branch could not resolve a still-missing target.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("second WireJunctions = %v; want nil (self-repair path)", err)
	}
}

// TestWireJunctions_RefusesRealWarpDirectory is card 7's regression guard: a real, non-link
// directory sitting at the warp junction path is still refused — fabric never moves or deletes user
// content — and the returned error names both the offending path and the re-run-`lyx fabric
// reconcile` remedy this card's reworded message introduces, replacing the old "migrate via the
// hub-creator" clause that pointed at a tool that does not address this case.
func TestWireJunctions_RefusesRealWarpDirectory(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	link := resetWarpJunction(t, l, slug, lyxdirs.LyxDirName)

	// Seed a real, non-link directory at the warp junction path — the
	// "created _lyx by hand" mistake this card's message must guide an
	// operator away from (and, per the batch scope, the same mistake an
	// operator makes hand-authoring content under an optional junction name).
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatalf("mkdir real warp dir %s: %v", link, err)
	}
	marker := filepath.Join(link, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"})
	if err == nil {
		t.Fatal("WireJunctions = nil; want error refusing a real warp directory")
	}

	msg := err.Error()
	if !strings.Contains(msg, link) {
		t.Errorf("error %q does not name the offending path %q", msg, link)
	}
	if !strings.Contains(msg, "lyx fabric reconcile") {
		t.Errorf("error %q does not name the re-run-`lyx fabric reconcile` remedy", msg)
	}

	// The real directory and its content must be untouched: fabric never
	// deletes or moves user content on this guard's account.
	content, readErr := os.ReadFile(marker)
	if readErr != nil {
		t.Fatalf("read marker after refused WireJunctions: %v", readErr)
	}
	if string(content) != "real content" {
		t.Errorf("marker content changed: %q", string(content))
	}
}

// TestUnwireJunctions_ReportsAndClearsEveryJunction is card 8's base-case regression guard: wiring
// then unwiring reports every junction Name in UnwireResult.JunctionsRemoved and removes every
// corresponding line from .git/info/exclude.
// From card 15 onward WarpJunctions returns two entries (_lyx and a second, non-_lyx junction), so
// this now runs against a genuinely two-junction world — the precondition batch 5's second junction
// depends on this machinery already handling correctly.
func TestUnwireJunctions_ReportsAndClearsEveryJunction(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName

	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}
	for _, name := range []string{lyxdirs.LyxDirName, "_extra"} {
		pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, name)
		if lines := readExcludeLines(t, l, slug); !containsLine(lines, pattern) {
			t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", pattern, lines)
		}
	}

	result, err := fabricengine.UnwireJunctions(l, slug, []string{"_lyx", "_extra"})
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}

	if want := []string{lyxdirs.LyxDirName, "_extra"}; !slices.Equal(result.JunctionsRemoved, want) {
		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
	}
	if !result.ExcludeChanged {
		t.Error("ExcludeChanged = false; want true")
	}

	lyxLink := fabricengine.WarpLyxLink(l, slug)
	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", lyxLink)
	}
	extraLink := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", extraLink)
	}
	if lines := readExcludeLines(t, l, slug); containsLine(lines, lyxdirs.LyxDirName) {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", lyxdirs.LyxDirName, lines)
	}
	if lines := readExcludeLines(t, l, slug); containsLine(lines, "_extra") {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", "_extra", lines)
	}
}

// TestUnwireJunctions_AlreadyUnwiredIsNoOp asserts that unwiring a worktree whose junctions were
// never wired (or already unwired) is a legitimate no-op: an empty JunctionsRemoved and a nil
// error, never an error.
func TestUnwireJunctions_AlreadyUnwiredIsNoOp(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName

	// hubforge.NewHub's CloneAndWire already wires _lyx; unwire it once first so this test
	// exercises the already-unwired no-op path against a genuinely unwired worktree, rather than
	// UnwireJunctions's ordinary first-time removal of the pre-wired _lyx junction.
	if _, err := fabricengine.UnwireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("pre-unwire: %v", err)
	}

	result, err := fabricengine.UnwireJunctions(l, slug, []string{"_lyx", "_extra"})
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

// containsLine reports whether lines contains name as a trimmed, line-exact
// match, mirroring the comparison seedGitExclude/unseedGitExclude use.
func containsLine(lines []string, name string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// TestDetectWarpPollution_LyxTrackedAsRestorable is card 18's regression guard, re-pointed at the
// _lyx-only layout batch 5 delivers: a tracked path under _lyx in the warp index must be reported as
// pollution with an automated restore remedy (git rm --cached plus a reminder to restore the
// junction/exclude entry) — every pollution class this scan reports now carries that remedy, since
// there is no report-only class left.
func TestDetectWarpPollution_LyxTrackedAsRestorable(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Track a file under _lyx directly in the warp worktree's index — the
	// "hand-authored _lyx content accidentally committed to warp" mistake
	// this scan exists to catch.
	warpLyxDir := filepath.Join(l.WorktreePath(), lyxdirs.LyxDirName)
	if err := os.MkdirAll(warpLyxDir, 0o755); err != nil {
		t.Fatalf("mkdir warp _lyx dir: %v", err)
	}
	trackedFile := filepath.Join(warpLyxDir, "PATTERN.md")
	if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitkit.MustRun(t, l.WorktreePath(), "git", "add", "--", lyxdirs.LyxDirName)
	gitkit.MustRun(t, l.WorktreePath(), "git", "commit", "-m", "accidentally track _lyx")

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status returned no pairs")
	}

	wantPath := pattern.PathspecFile
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

// TestDetectWarpPollution_ScanErrorIsNonFatal pins Status' non-fatal-and-continue handling of a
// detectWarpPollution failure: the pair still comes back with a synthetic "<scan error: ...>"
// pollution entry carrying an empty Remedy, rather than Status failing the pair outright. The
// empty Remedy alone is what signals "no automated remedy applies here," so this test guards
// that the signal survives on its own.
func TestDetectWarpPollution_ScanErrorIsNonFatal(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Corrupt only the warp worktree's index file so `git ls-files` fails
	// inside detectWarpPollution, forcing Status down its scan-error branch,
	// while leaving `git worktree list` (which List uses ahead of the
	// per-pair pollution scan) unaffected — it does not read the index.
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

	topology := fabricengine.NewTopology(fabricengine.Config{})
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

// TestDetectWarpPollution_RaddleNoLongerReported pins the observable behaviour change this batch
// delivers: a tracked path under _raddle in the warp index is no longer reported as pollution at
// all, now that the _raddle classification branch is deleted and the scan's git-ls-files pathspec no
// longer names "_raddle".
func TestDetectWarpPollution_RaddleNoLongerReported(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

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

	topology := fabricengine.NewTopology(fabricengine.Config{})
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

// TestHealthy_JunctionDriftShapes is card 11's regression guard: each of Healthy's three
// junction-drift shapes — missing, not-a-link, and points-elsewhere — produces reason wording
// naming the junction, aligned with checkJunctionHealth's wording (card 10) for the same shape.
// From card 15 onward, each drift shape is exercised against BOTH junctions (_lyx and a second,
// non-_lyx junction), proving Healthy's WarpJunctionsHere() loop — drift.go does not share
// checkJunctionHealth's code path, so this is not redundant with reconcile.go's or status.go's own
// coverage — reports the correct wording for the second, non-_lyx junction too.
func TestHealthy_JunctionDriftShapes(t *testing.T) {
	shapes := []struct {
		name       string
		corrupt    func(t *testing.T, link, target string)
		wantCause  fabricengine.HealthCause
		wantDetail func(dirName string) string
	}{
		{
			name: "Missing",
			corrupt: func(t *testing.T, link, target string) {
				if err := fslink.Remove(link); err != nil {
					t.Fatalf("remove junction: %v", err)
				}
			},
			wantCause: fabricengine.CauseJunctionMissing,
			wantDetail: func(dirName string) string {
				return fmt.Sprintf("%s junction missing", dirName)
			},
		},
		{
			name: "NotALink",
			corrupt: func(t *testing.T, link, target string) {
				if err := fslink.Remove(link); err != nil {
					t.Fatalf("remove junction: %v", err)
				}
				if err := os.Mkdir(link, 0o755); err != nil {
					t.Fatalf("mkdir real dir in junction's place: %v", err)
				}
			},
			wantCause: fabricengine.CauseNotAJunction,
			wantDetail: func(dirName string) string {
				return fmt.Sprintf("%s is not a junction", dirName)
			},
		},
		{
			name: "PointsElsewhere",
			corrupt: func(t *testing.T, link, target string) {
				if err := fslink.Remove(link); err != nil {
					t.Fatalf("remove junction: %v", err)
				}
				wrongTarget := filepath.Join(filepath.Dir(target), "not-the-weft-target")
				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
					t.Fatalf("mkdir wrong target: %v", err)
				}
				if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
					t.Fatalf("seed wrong-target junction: %v", err)
				}
			},
			wantCause: fabricengine.CauseJunctionPointsElsewhere,
			wantDetail: func(dirName string) string {
				return fmt.Sprintf("%s junction points elsewhere", dirName)
			},
		},
	}

	junctions := []struct {
		name      string
		dirName   string
		linkFor   func(l *lyxcwd.Location) string
		targetFor func(l *lyxcwd.Location) string
	}{
		{
			name:      "Lyx",
			dirName:   lyxdirs.LyxDirName,
			linkFor:   func(l *lyxcwd.Location) string { return fabricengine.WarpLyxLinkHere(l) },
			targetFor: func(l *lyxcwd.Location) string { return fabricengine.WeftLyxDir(l) },
		},
		{
			name:    "Extra",
			dirName: "_extra",
			linkFor: func(l *lyxcwd.Location) string {
				return filepath.Join(l.WorktreePath(), l.AnchorRel, "_extra")
			},
			targetFor: func(l *lyxcwd.Location) string {
				return filepath.Join(fabricengine.WeftWorktree(l), l.AnchorRel, "_extra")
			},
		},
	}

	for _, j := range junctions {
		for _, tt := range shapes {
			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				h := hubforge.NewHub(t, ".")
				hubforge.SeedFabricConfig(t, h, extraFabricConfigYAML)

				l := h.Location
				slug := l.WorktreeName
				// Wire the full RepoWiredNames set, .lyx included: Healthy below reads
				// RepoWiredNames(l) internally, and a .lyx junction absent on disk would report
				// CauseJunctionMissing for .lyx before this subtest's own corruption is ever
				// reached.
				if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", ".lyx", "_extra"}); err != nil {
					t.Fatalf("WireJunctions: %v", err)
				}

				link := j.linkFor(l)
				target := j.targetFor(l)
				tt.corrupt(t, link, target)

				ok, reason, err := fabricengine.Healthy(l)
				if err != nil {
					t.Fatalf("Healthy: %v", err)
				}
				if ok {
					t.Errorf("Healthy = true; want false (%s)", tt.name)
				}
				wantDetail := tt.wantDetail(j.dirName)
				if reason.Cause != tt.wantCause || reason.Detail != wantDetail {
					t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, tt.wantCause, wantDetail)
				}
			})
		}
	}
}

// TestReconcile_RepairsOptionalJunctionOnlyDrift is the reconcile.go site of card 19's per-site
// health-check coverage: with _lyx healthy and the second, non-_lyx junction missing, Reconcile must
// repair (ReconcileActionJunctionRepointed) rather than report ReconcileActionAlreadyHealthy.
// reconcile.go does not share drift.go's Healthy code path (see checkJunctionHealth), so this is
// not redundant with TestHealthy_JunctionDriftShapes above.
func TestReconcile_RepairsOptionalJunctionOnlyDrift(t *testing.T) {
	t.Parallel()

	const slug = "reconcile-extra-only-drift"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	// newFabricFixture seeds the repo-wide config with fabricengine.ConfigTemplate()'s own
	// default pathspec; override it to "_extra" so Add's own RepoWiredNames-driven wiring (and
	// Reconcile's below) agrees with the junction name this test drifts.
	seedRepoWideExtraFabricConfig(t, l.HubPath)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	// _lyx stays healthy; only _extra goes missing.
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove _extra junction: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	var found bool
	for _, pair := range result.Pairs {
		if pair.WeftWorktree != filepath.ToSlash(weftPath) {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionJunctionRepointed {
			t.Errorf("Action = %q; want %q (not %q)", pair.Action, fabricengine.ReconcileActionJunctionRepointed, fabricengine.ReconcileActionAlreadyHealthy)
		}
		if pair.Error != "" {
			t.Errorf("Error = %q; want empty", pair.Error)
		}
	}
	if !found {
		t.Fatalf("Reconcile result has no pair for weft path %s: %+v", weftPath, result.Pairs)
	}

	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Errorf("_extra junction %s not repaired by Reconcile: isLink=%v err=%v", extraLink, isLink, err)
	}
}

// TestStatus_ReportsOptionalJunctionUnhealthy is the status.go site of card 19's per-site
// health-check coverage: with _lyx healthy and the second, non-_lyx junction mis-pointed, Status must
// report JunctionHealthy false, a JunctionReason naming that junction, and the pair not in sync.
// status.go computes its verdict inline (see status.go's own doc comment) rather than sharing
// drift.go's Healthy, so this is not redundant with TestHealthy_JunctionDriftShapes.
func TestStatus_ReportsOptionalJunctionUnhealthy(t *testing.T) {
	t.Parallel()

	const slug = "status-extra-unhealthy"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	// newFabricFixture seeds the repo-wide config with fabricengine.ConfigTemplate()'s own
	// default pathspec; override it to "_extra" so Add's own RepoWiredNames-driven wiring (and
	// Status's below) agrees with the junction name this test drifts.
	seedRepoWideExtraFabricConfig(t, l.HubPath)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	// _lyx stays healthy; only _extra is re-pointed at an unrelated directory.
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove _extra junction: %v", err)
	}
	wrongTarget := filepath.Join(fixture.Hub, "not-the-weft-extra-dir")
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
		if pair.WarpWorktree != filepath.ToSlash(warpPath) {
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

// TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth is the wiring-idempotency upgrade path: a
// worktree with _lyx already wired and its second, non-_lyx junction not yet wired at all completes
// a WireJunctions call without error and adds only the missing junction — it must not error,
// and it must not need to touch the already-healthy _lyx junction to succeed.
// The mechanism this pins — a WireJunctions call adds only the missing optional junction and leaves
// a healthy _lyx alone — stays live independent of any particular worktree's history.
func TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName

	// Wire both junctions once, then remove _extra only — _lyx stays fully
	// healthy, _extra becomes the only missing junction.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (initial): %v", err)
	}
	extraLink := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove _extra junction to simulate a worktree missing it: %v", err)
	}

	lyxLink := fabricengine.WarpLyxLink(l, slug)
	lyxResolvedBefore, err := fslink.PointsTo(lyxLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) before upgrade: %v", lyxLink, err)
	}

	// The upgrade call: must complete without error and wire only the missing
	// _extra junction.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (upgrade): %v", err)
	}

	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Fatalf("_extra junction %s not wired by upgrade call: isLink=%v err=%v", extraLink, isLink, err)
	}
	lyxResolvedAfter, err := fslink.PointsTo(lyxLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) after upgrade: %v", lyxLink, err)
	}
	if lyxResolvedBefore != lyxResolvedAfter {
		t.Errorf("_lyx junction target changed across upgrade call: before=%s after=%s", lyxResolvedBefore, lyxResolvedAfter)
	}

	// Wiring twice (the already-upgraded worktree) is a no-op.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions (idempotent re-run): %v", err)
	}
}

// TestSeedGitExclude_AnchorsPatternAndReplacesLegacyBareName proves the exclude entry names exactly
// the one junction path fabric wires, not every same-named directory in the repo.
// A gitignore pattern with no slash matches at ANY depth, so the bare "_lyx" fabric used to write
// silently untracked a monorepo's own frontend/_lyx/ on a subpath-anchored hub; and a hub wired by
// an earlier binary must converge on the narrower pattern rather than keep both.
func TestSeedGitExclude_AnchorsPatternAndReplacesLegacyBareName(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName

	// Stand in for a hub wired before the narrowing: a legacy bare-name line already in the file.
	excludePath := excludeFilePath(t, l, slug)
	if err := os.WriteFile(excludePath, []byte(lyxdirs.LyxDirName+"\n"), 0o644); err != nil {
		t.Fatalf("seed legacy exclude line: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug, []string{lyxdirs.LyxDirName}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	lines := readExcludeLines(t, l, slug)
	pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, lyxdirs.LyxDirName)
	if !containsLine(lines, pattern) {
		t.Errorf(".git/info/exclude = %v; want it to contain the anchored pattern %q", lines, pattern)
	}
	if containsLine(lines, lyxdirs.LyxDirName) {
		t.Errorf(".git/info/exclude = %v; want the legacy bare %q line replaced, not kept beside the anchored pattern",
			lines, lyxdirs.LyxDirName)
	}

	// The anchored pattern must not reach a same-named directory elsewhere in the repo.
	elsewhere := filepath.Join(l.WorktreePath(), "frontend", lyxdirs.LyxDirName)
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
		t.Errorf("git sees nothing under frontend/%s; the exclude pattern reached a directory fabric never wired", lyxdirs.LyxDirName)
	}
}

// excludeFilePath resolves the repo's .git/info/exclude path for the worktree named slug, the same
// way seedGitExclude does.
func excludeFilePath(t *testing.T, l *lyxcwd.Location, slug string) string {
	t.Helper()
	worktreePath := fabricengine.WorktreePath(l, slug)
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if err != nil || exitCode != 0 {
		t.Fatalf("resolve exclude path: err=%v exit=%d stderr=%s", err, exitCode, stderr)
	}
	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatalf("mkdir exclude dir: %v", err)
	}
	return excludePath
}
