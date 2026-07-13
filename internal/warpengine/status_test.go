//go:build integration

// status_test.go covers the warp paired-view Status: pair fields populated, in-sync vs
// drifted detection, junction health reflected, and host-pollution flagging for both
// _lyx (remediable) and _raddle (report-only).

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupStatusFixture prepares a CopyPairedLocal fixture with warp config seeded and the
// host _lyx junction created so that Status can resolve layouts and check junction health.
func setupStatusFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	f := lyxtest.CopyPairedLocal(t)
	slug := filepath.Base(f.Hub)

	// Seed warp config into the weft prime so LoadConfig resolves through the junction.
	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"warp": ConfigTemplate(),
	})

	// Create the host _lyx junction pointing to the weft's _lyx directory.
	// This replicates the production topology: host _lyx → weft _lyx.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	return f
}

// TestStatus_InSyncVsDrifted asserts that InSync is true for a healthy pair and false
// (with a non-empty DriftReason) when the weft worktree is on a different branch.
func TestStatus_InSyncVsDrifted(t *testing.T) {
	t.Parallel()

	f := setupStatusFixture(t)
	w := New(Config{})

	// Pre-check: verify Status populates the core pair fields for a healthy pair.
	result, err := w.Status(f.Layout)
	if err != nil {
		t.Fatalf("Status() (pre-check) error = %v; want nil", err)
	}

	if len(result.Pairs) == 0 {
		t.Fatalf("Status().Pairs is empty; want at least one pair")
	}

	// Find the pair for the hub worktree specifically.
	var pair *PairStatus
	for i := range result.Pairs {
		if filepath.Clean(result.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			pair = &result.Pairs[i]
			break
		}
	}
	if pair == nil {
		t.Fatalf("Status() (pre-check): no pair found for hub worktree %s", f.Hub)
	}

	// Host and weft paths must be populated.
	if pair.HostWorktree == "" {
		t.Errorf("PairStatus.HostWorktree is empty; want non-empty")
	}
	if pair.WeftWorktree == "" {
		t.Errorf("PairStatus.WeftWorktree is empty; want non-empty")
	}

	// JSON-boundary paths must be forward-slash even on Windows (issue #37). Check the
	// raw field value directly -- filepath.Clean would re-normalize forward slashes back
	// to OS-native backslash and silently defeat this assertion.
	if strings.Contains(pair.HostWorktree, "\\") {
		t.Errorf("PairStatus.HostWorktree = %q; want no backslash separators", pair.HostWorktree)
	}
	if strings.Contains(pair.WeftWorktree, "\\") {
		t.Errorf("PairStatus.WeftWorktree = %q; want no backslash separators", pair.WeftWorktree)
	}

	// Branch fields must be populated (both start on "main" in the fixture).
	if pair.HostBranch == "" {
		t.Errorf("PairStatus.HostBranch is empty; want non-empty")
	}
	if pair.WeftBranch == "" {
		t.Errorf("PairStatus.WeftBranch is empty; want non-empty")
	}
	if pair.HostBranch != pair.WeftBranch {
		t.Errorf("HostBranch=%q, WeftBranch=%q; want equal for healthy pair", pair.HostBranch, pair.WeftBranch)
	}

	// Create a diverging branch on the weft only, then switch the weft to it.
	// This simulates branch drift: host stays on main, weft moves to drifted.
	lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "checkout", "-b", "drifted")

	// Re-invoke Status() for the drifted state.
	result2, err := w.Status(f.Layout)
	if err != nil {
		t.Fatalf("Status() (drifted) error = %v; want nil", err)
	}

	var driftedPair *PairStatus
	for i := range result2.Pairs {
		if filepath.Clean(result2.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			driftedPair = &result2.Pairs[i]
			break
		}
	}
	if driftedPair == nil {
		t.Fatalf("Status() (drifted): no pair found for hub worktree %s", f.Hub)
	}

	// The pair must be reported as out of sync with a non-empty reason.
	if driftedPair.InSync {
		t.Errorf("InSync = true for drifted pair; want false")
	}
	if driftedPair.DriftReason == "" {
		t.Errorf("DriftReason is empty for drifted pair; want non-empty description")
	}
	if !strings.Contains(driftedPair.DriftReason, "drifted") && !strings.Contains(driftedPair.DriftReason, "main") {
		t.Errorf("DriftReason = %q; want reference to branch names", driftedPair.DriftReason)
	}
}

// TestStatus_JunctionHealth asserts that JunctionHealthy reflects the actual junction
// state: true when the junction is intact, false (with a JunctionReason) when it is broken.
func TestStatus_JunctionHealth(t *testing.T) {
	t.Parallel()

	f := setupStatusFixture(t)
	slug := filepath.Base(f.Hub)

	// Verify the junction is reported healthy for the freshly-wired fixture.
	w := New(Config{})
	result, err := w.Status(f.Layout)
	if err != nil {
		t.Fatalf("Status() [healthy] error = %v; want nil", err)
	}

	var pair *PairStatus
	for i := range result.Pairs {
		if filepath.Clean(result.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			pair = &result.Pairs[i]
			break
		}
	}
	if pair == nil {
		t.Fatalf("Status(): no pair found for hub worktree %s", f.Hub)
	}
	if !pair.JunctionHealthy {
		t.Errorf("JunctionHealthy = false for freshly-wired junction; want true. Reason: %s", pair.JunctionReason)
	}
	if pair.JunctionReason != "" {
		t.Errorf("JunctionReason = %q for healthy junction; want empty", pair.JunctionReason)
	}

	// Break the junction by removing it, then re-run Status and check the report.
	hostLink := f.Layout.HostLyxLink(slug)
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("Remove junction for broken-junction sub-test: %v", err)
	}

	// Rebuild layout since the junction removal may affect resolution.
	brokenLayout, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("Resolve after junction removal: %v", err)
	}

	result2, err := w.Status(brokenLayout)
	if err != nil {
		t.Fatalf("Status() [broken] error = %v; want nil", err)
	}

	var brokenPair *PairStatus
	for i := range result2.Pairs {
		if filepath.Clean(result2.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			brokenPair = &result2.Pairs[i]
			break
		}
	}
	if brokenPair == nil {
		t.Fatalf("Status() [broken]: no pair found for hub worktree %s", f.Hub)
	}
	if brokenPair.JunctionHealthy {
		t.Errorf("JunctionHealthy = true after junction removal; want false")
	}
	if brokenPair.JunctionReason == "" {
		t.Errorf("JunctionReason is empty for broken junction; want non-empty description")
	}
}

// TestStatus_LyxPollutionDetected asserts that force-adding a _lyx path to the host index
// is detected as pollution and the PollutionEntry carries a git rm --cached remedy.
// injectLinkPollution stages relPath into hostDir's index as a tracked regular
// file without writing through the _lyx/_raddle link. On Windows those links are
// git-transparent junctions, so a plain `git add -f` of a path under them works;
// on Linux they are symlinks and git refuses any pathspec "beyond a symbolic
// link" (both `git add` and `git hash-object` of a path under the link). Hashing
// the content from a scratch file outside the link and injecting the index entry
// with `git update-index --add --cacheinfo` reproduces the identical
// polluted-index state on either OS — which is all detectHostPollution's
// `git ls-files` reads.
func injectLinkPollution(t *testing.T, hostDir, relPath, content string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "pollution-blob")
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatalf("write pollution blob source: %v", err)
	}
	sha, stderr, code, err := gitexec.RunGit([]string{"hash-object", "-w", src}, hostDir)
	if err != nil || code != 0 {
		t.Fatalf("hash-object %s: err=%v code=%d stderr=%q", src, err, code, stderr)
	}
	cacheinfo := fmt.Sprintf("100644,%s,%s", strings.TrimSpace(sha), relPath)
	if _, stderr, code, err := gitexec.RunGit([]string{"update-index", "--add", "--cacheinfo", cacheinfo}, hostDir); err != nil || code != 0 {
		t.Fatalf("update-index --cacheinfo %s: err=%v code=%d stderr=%q", cacheinfo, err, code, stderr)
	}
}

func TestStatus_LyxPollutionDetected(t *testing.T) {
	t.Parallel()

	f := setupStatusFixture(t)

	// Stage a polluting file under _lyx in the host index. In normal operation
	// _lyx is a link (Windows junction / Linux symlink) excluded from tracking,
	// but an accidental force-track can bypass that. The index entry is injected
	// via plumbing so the setup is filesystem-agnostic — a plain `git add -f` of
	// a path under the link is refused on Linux ("beyond a symbolic link"); see
	// injectLinkPollution.
	injectLinkPollution(t, f.Hub, "_lyx/accidental.txt", "polluted")

	w := New(Config{})
	result, err := w.Status(f.Layout)
	if err != nil {
		t.Fatalf("Status() error = %v; want nil", err)
	}

	var pair *PairStatus
	for i := range result.Pairs {
		if filepath.Clean(result.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			pair = &result.Pairs[i]
			break
		}
	}
	if pair == nil {
		t.Fatalf("Status(): no pair found for hub worktree %s", f.Hub)
	}

	// At least one pollution entry must be present.
	if len(pair.Pollution) == 0 {
		t.Fatalf("Pollution is empty after force-add of _lyx file; want at least one entry")
	}

	// The pollution entry must carry a remedy (not report-only) because it is under _lyx.
	found := false
	for _, pe := range pair.Pollution {
		if strings.HasPrefix(pe.Path, "_lyx") {
			found = true
			if pe.ReportOnly {
				t.Errorf("Pollution entry for %q has ReportOnly=true; want false (should have remedy)", pe.Path)
			}
			if pe.Remedy == "" {
				t.Errorf("Pollution entry for %q has empty Remedy; want git rm --cached suggestion", pe.Path)
			}
			if !strings.Contains(pe.Remedy, "git") {
				t.Errorf("Remedy %q does not reference git; want git rm --cached command", pe.Remedy)
			}
			break
		}
	}
	if !found {
		t.Errorf("no pollution entry with _lyx prefix found; Pollution = %+v", pair.Pollution)
	}

	// Phase 2: _raddle pollution (report-only, no remedy in this release). Same
	// plumbing injection — git indexes forward-slash paths on every OS, so this
	// reproduces the polluted-index state whether _raddle is a junction or a
	// symlink. ls-files reports staged entries, so no commit is needed.
	injectLinkPollution(t, f.Hub, "_raddle/overview.md", "raddle polluted")

	// Re-invoke Status() for the _raddle pollution check.
	result2, err := w.Status(f.Layout)
	if err != nil {
		t.Fatalf("Status() (raddle phase) error = %v; want nil", err)
	}

	var pair2 *PairStatus
	for i := range result2.Pairs {
		if filepath.Clean(result2.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			pair2 = &result2.Pairs[i]
			break
		}
	}
	if pair2 == nil {
		t.Fatalf("Status() (raddle phase): no pair found for hub worktree %s", f.Hub)
	}

	if len(pair2.Pollution) == 0 {
		t.Fatalf("Pollution is empty after force-add of _raddle file; want at least one entry")
	}

	found2 := false
	for _, pe := range pair2.Pollution {
		if strings.HasPrefix(pe.Path, "_raddle") {
			found2 = true
			if !pe.ReportOnly {
				t.Errorf("Pollution entry for %q has ReportOnly=false; want true (_raddle is report-only)", pe.Path)
			}
			if pe.Remedy != "" {
				t.Errorf("Pollution entry for %q has Remedy=%q; want empty (report-only)", pe.Path, pe.Remedy)
			}
			break
		}
	}
	if !found2 {
		t.Errorf("no pollution entry with _raddle prefix found; Pollution = %+v", pair2.Pollution)
	}
}
