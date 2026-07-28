//go:build integration

// junction_pattern_integration_test.go covers the per-junction generalisation
// this batch makes to seedLyxJunction, unseedLyxJunction, checkJunctionHealth,
// and PairInSync's inline junction check. From card 15 onward, HostJunctions
// returns two entries (_lyx and _pattern), so this file's cases now exercise
// a genuinely two-junction world: every generalisation from batch 3 — health
// check (per-site: reconcile, status, drift), per-junction refusal/repoint,
// unwire/remove looping, and wiring idempotency (including the legacy
// single-junction-to-two upgrade path) — is proven against a real second,
// non-_lyx junction, not merely a loop of length one.
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

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// readExcludeLines resolves and reads the host worktree's .git/info/exclude
// file, mirroring the resolution logic seedGitExclude/unseedGitExclude use
// (git rev-parse --git-path info/exclude, joined with the worktree path if
// relative) so this test observes the same path the production code writes.
func readExcludeLines(t *testing.T, l *hubgeometry.Layout, slug string) []string {
	t.Helper()

	worktreePath := l.WorktreePath(slug)
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

// TestWireJunctions_MaterialisesMissingWeftTarget is card 6's regression
// guard: seedLyxJunction must create the weft-side target directory when it
// is missing (the checkout/reconcile-left-dangling shape), leaving a junction
// that resolves immediately, and a second WireJunctions call on the same
// worktree must succeed rather than hard-erroring — the bug this card fixes.
func TestWireJunctions_MaterialisesMissingWeftTarget(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	target := l.WeftLyxDirFor(slug)

	// The weft-prime template pre-seeds _lyx/config/placeholder; remove the
	// whole target directory so it genuinely does not exist, matching the
	// state a checkout/reconcile-created junction points at today.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove weft target %s: %v", target, err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions with missing weft target: %v", err)
	}

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("weft target %s not materialised: stat err=%v", target, err)
	}

	link := l.HostLyxLink(slug)
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
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("second WireJunctions = %v; want nil (self-repair path)", err)
	}
}

// TestWireJunctions_RefusesRealHostDirectory is card 7's regression guard: a
// real, non-link directory sitting at the host junction path is still
// refused — fabric never moves or deletes user content — and the returned
// error names both the offending path and the re-run-`lyx init` remedy this
// card's reworded message introduces, replacing the old "migrate via the
// hub-creator" clause that pointed at a tool that does not address this case.
func TestWireJunctions_RefusesRealHostDirectory(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostLyxLink(slug)

	// Seed a real, non-link directory at the host junction path — the
	// "created _lyx by hand" mistake this card's message must guide an
	// operator away from (and, per the batch scope, the same mistake an
	// operator makes hand-authoring _pattern content).
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatalf("mkdir real host dir %s: %v", link, err)
	}
	marker := filepath.Join(link, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug)
	if err == nil {
		t.Fatal("WireJunctions = nil; want error refusing a real host directory")
	}

	msg := err.Error()
	if !strings.Contains(msg, link) {
		t.Errorf("error %q does not name the offending path %q", msg, link)
	}
	if !strings.Contains(msg, "lyx init") {
		t.Errorf("error %q does not name the re-run-`lyx init` remedy", msg)
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

// TestUnwireJunctions_ReportsAndClearsEveryJunction is card 8's base-case
// regression guard: wiring then unwiring reports every junction Name in
// UnwireResult.JunctionsRemoved and removes every corresponding line from
// .git/info/exclude. From card 15 onward HostJunctions returns two entries
// (_lyx and _pattern), so this now runs against a genuinely two-junction
// world — the precondition batch 5's second junction depends on this
// machinery already handling correctly.
func TestUnwireJunctions_ReportsAndClearsEveryJunction(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}
	if lines := readExcludeLines(t, l, slug); !containsLine(lines, hubgeometry.LyxDirName) {
		t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", hubgeometry.LyxDirName, lines)
	}
	if lines := readExcludeLines(t, l, slug); !containsLine(lines, hubgeometry.PatternDirName) {
		t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", hubgeometry.PatternDirName, lines)
	}

	result, err := fabricengine.UnwireJunctions(l, slug)
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}

	if want := []string{hubgeometry.LyxDirName, hubgeometry.PatternDirName}; !slices.Equal(result.JunctionsRemoved, want) {
		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
	}
	if !result.ExcludeChanged {
		t.Error("ExcludeChanged = false; want true")
	}

	lyxLink := l.HostLyxLink(slug)
	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", lyxLink)
	}
	patternLink := l.HostPatternLink(slug)
	if _, statErr := os.Lstat(patternLink); !os.IsNotExist(statErr) {
		t.Errorf("junction %s still exists after UnwireJunctions", patternLink)
	}
	if lines := readExcludeLines(t, l, slug); containsLine(lines, hubgeometry.LyxDirName) {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", hubgeometry.LyxDirName, lines)
	}
	if lines := readExcludeLines(t, l, slug); containsLine(lines, hubgeometry.PatternDirName) {
		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", hubgeometry.PatternDirName, lines)
	}
}

// TestUnwireJunctions_AlreadyUnwiredIsNoOp asserts that unwiring a worktree
// whose junctions were never wired (or already unwired) is a legitimate
// no-op: an empty JunctionsRemoved and a nil error, never an error.
func TestUnwireJunctions_AlreadyUnwiredIsNoOp(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)

	result, err := fabricengine.UnwireJunctions(l, slug)
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

// TestDetectHostPollution_PatternTrackedAsRestorable is card 18's regression
// guard: a tracked path under _pattern in the host index must be reported as
// pollution with the same automated restore remedy _lyx pollution gets (git
// rm --cached plus a reminder to restore the junction/exclude entry) — never
// report-only like _raddle, since _pattern has a junction from card 15
// onward.
func TestDetectHostPollution_PatternTrackedAsRestorable(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Track a file under _pattern directly in the host worktree's index —
	// the "hand-authored _pattern content accidentally committed to host"
	// mistake this scan exists to catch.
	hostPatternDir := filepath.Join(l.WorktreeRoot, hubgeometry.PatternDirName)
	if err := os.MkdirAll(hostPatternDir, 0o755); err != nil {
		t.Fatalf("mkdir host _pattern dir: %v", err)
	}
	trackedFile := filepath.Join(hostPatternDir, "PATTERN.md")
	if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "add", "--", hubgeometry.PatternDirName)
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "commit", "-m", "accidentally track _pattern")

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status returned no pairs")
	}

	const wantPath = "_pattern/PATTERN.md"
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
	if found.ReportOnly {
		t.Errorf("PollutionEntry for %q is ReportOnly; want a restorable (automated-remedy) entry, matching _lyx", wantPath)
	}
	if found.Remedy == "" {
		t.Errorf("PollutionEntry for %q has empty Remedy; want the same git rm --cached remedy _lyx gets", wantPath)
	}
	if !strings.Contains(found.Remedy, "rm --cached") {
		t.Errorf("PollutionEntry for %q remedy = %q; want it to contain \"rm --cached\"", wantPath, found.Remedy)
	}
}

// TestPairInSync_JunctionDriftShapes is card 11's regression guard: each of
// PairInSync's three junction-drift shapes — missing, not-a-link, and
// points-elsewhere — produces reason wording naming the junction, aligned
// with checkJunctionHealth's wording (card 10) for the same shape. From card
// 15 onward, each drift shape is exercised against BOTH junctions (_lyx and
// _pattern), proving PairInSync's HostJunctionsHere() loop — drift.go does
// not share checkJunctionHealth's code path, so this is not redundant with
// reconcile.go's or status.go's own coverage — reports the correct wording
// for the second, non-_lyx junction too.
func TestPairInSync_JunctionDriftShapes(t *testing.T) {
	shapes := []struct {
		name       string
		corrupt    func(t *testing.T, link, target string)
		wantReason func(dirName string) string
	}{
		{
			name: "Missing",
			corrupt: func(t *testing.T, link, target string) {
				if err := fslink.Remove(link); err != nil {
					t.Fatalf("remove junction: %v", err)
				}
			},
			wantReason: func(dirName string) string {
				return fmt.Sprintf("host %s junction missing", dirName)
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
			wantReason: func(dirName string) string {
				return fmt.Sprintf("host %s is not a junction", dirName)
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
			wantReason: func(dirName string) string {
				return fmt.Sprintf("host %s junction points elsewhere", dirName)
			},
		},
	}

	junctions := []struct {
		name      string
		dirName   string
		linkFor   func(l *hubgeometry.Layout) string
		targetFor func(l *hubgeometry.Layout) string
	}{
		{
			name:      "Lyx",
			dirName:   hubgeometry.LyxDirName,
			linkFor:   func(l *hubgeometry.Layout) string { return l.HostLyxLinkHere() },
			targetFor: func(l *hubgeometry.Layout) string { return l.WeftLyxDir() },
		},
		{
			name:      "Pattern",
			dirName:   hubgeometry.PatternDirName,
			linkFor:   func(l *hubgeometry.Layout) string { return l.HostPatternLinkHere() },
			targetFor: func(l *hubgeometry.Layout) string { return l.WeftPatternDir() },
		},
	}

	for _, j := range junctions {
		for _, tt := range shapes {
			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				fixture := lyxtest.CopyPairedLocal(t)
				lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
					"fabric": fabricengine.ConfigTemplate(),
				})
				lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

				l := fixture.Layout
				slug := filepath.Base(fixture.Hub)
				if err := fabricengine.WireJunctions(l, slug); err != nil {
					t.Fatalf("WireJunctions: %v", err)
				}

				link := j.linkFor(l)
				target := j.targetFor(l)
				tt.corrupt(t, link, target)

				ok, reason, err := fabricengine.PairInSync(l)
				if err != nil {
					t.Fatalf("PairInSync: %v", err)
				}
				if ok {
					t.Errorf("PairInSync = true; want false (%s)", tt.name)
				}
				wantReason := tt.wantReason(j.dirName)
				if reason != wantReason {
					t.Errorf("PairInSync reason = %q; want %q", reason, wantReason)
				}
			})
		}
	}
}

// TestReconcile_RepairsPatternOnlyDrift is the reconcile.go site of card 19's
// per-site health-check coverage: with _lyx healthy and _pattern missing,
// Reconcile must repair (ReconcileActionJunctionRepointed) rather than report
// ReconcileActionAlreadyHealthy. reconcile.go does not share drift.go's
// PairInSync code path (see checkJunctionHealth), so this is not redundant
// with TestPairInSync_JunctionDriftShapes above.
func TestReconcile_RepairsPatternOnlyDrift(t *testing.T) {
	t.Parallel()

	const slug = "reconcile-pattern-only-drift"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}

	// _lyx stays healthy; only _pattern goes missing.
	patternLink := hostLayout.HostPatternLinkHere()
	if err := fslink.Remove(patternLink); err != nil {
		t.Fatalf("remove _pattern junction: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	weftPath := l.WeftWorktreePath(slug)
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

	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
		t.Errorf("_pattern junction %s not repaired by Reconcile: isLink=%v err=%v", patternLink, isLink, err)
	}
}

// TestStatus_ReportsPatternJunctionUnhealthy is the status.go site of card
// 19's per-site health-check coverage: with _lyx healthy and _pattern
// mis-pointed, Status must report JunctionHealthy false, a JunctionReason
// naming _pattern, and the pair not in sync. status.go computes its verdict
// inline (see status.go's own doc comment) rather than sharing drift.go's
// PairInSync, so this is not redundant with TestPairInSync_JunctionDriftShapes.
func TestStatus_ReportsPatternJunctionUnhealthy(t *testing.T) {
	t.Parallel()

	const slug = "status-pattern-unhealthy"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}

	// _lyx stays healthy; only _pattern is re-pointed at an unrelated directory.
	patternLink := hostLayout.HostPatternLinkHere()
	if err := fslink.Remove(patternLink); err != nil {
		t.Fatalf("remove _pattern junction: %v", err)
	}
	wrongTarget := filepath.Join(fixture.Hub, "not-the-weft-pattern-dir")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir wrong target: %v", err)
	}
	if err := fslink.CreateDirLink(patternLink, wrongTarget); err != nil {
		t.Fatalf("seed wrong-target junction: %v", err)
	}

	result, err := topology.Status(l)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	hostPath := l.WorktreePath(slug)
	var found bool
	for _, pair := range result.Pairs {
		if pair.HostWorktree != filepath.ToSlash(hostPath) {
			continue
		}
		found = true
		if pair.JunctionHealthy {
			t.Error("JunctionHealthy = true; want false")
		}
		if !strings.Contains(pair.JunctionReason, hubgeometry.PatternDirName) {
			t.Errorf("JunctionReason = %q; want it to name %q", pair.JunctionReason, hubgeometry.PatternDirName)
		}
		if pair.InSync {
			t.Error("InSync = true; want false")
		}
	}
	if !found {
		t.Fatalf("Status result has no pair for host path %s: %+v", hostPath, result.Pairs)
	}
}

// TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth is the wiring-idempotency
// upgrade path: a worktree with _lyx already wired and _pattern not yet wired
// at all (the shape every pre-card-15 worktree is in) completes a
// WireJunctions call without error and adds only the missing _pattern
// junction — it must not error, and it must not need to touch the
// already-healthy _lyx junction to succeed.
func TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)

	// Wire both junctions once, then simulate the legacy pre-upgrade state by
	// removing _pattern only — _lyx is fully healthy, _pattern was never wired.
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions (initial): %v", err)
	}
	patternLink := l.HostPatternLink(slug)
	if err := fslink.Remove(patternLink); err != nil {
		t.Fatalf("remove _pattern junction to simulate legacy worktree: %v", err)
	}

	lyxLink := l.HostLyxLink(slug)
	lyxResolvedBefore, err := fslink.PointsTo(lyxLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) before upgrade: %v", lyxLink, err)
	}

	// The upgrade call: must complete without error and wire only the missing
	// _pattern junction.
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions (upgrade): %v", err)
	}

	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
		t.Fatalf("_pattern junction %s not wired by upgrade call: isLink=%v err=%v", patternLink, isLink, err)
	}
	lyxResolvedAfter, err := fslink.PointsTo(lyxLink)
	if err != nil {
		t.Fatalf("PointsTo(%s) after upgrade: %v", lyxLink, err)
	}
	if lyxResolvedBefore != lyxResolvedAfter {
		t.Errorf("_lyx junction target changed across upgrade call: before=%s after=%s", lyxResolvedBefore, lyxResolvedAfter)
	}

	// Wiring twice (the already-upgraded worktree) is a no-op.
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions (idempotent re-run): %v", err)
	}
}
