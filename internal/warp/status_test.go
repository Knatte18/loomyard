//go:build integration

// status_test.go covers the warp paired-view Status: pair fields populated, in-sync vs
// drifted detection, junction health reflected, and host-pollution flagging for both
// _lyx (remediable) and _codeguide (report-only).

package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
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
	brokenLayout, err := paths.Resolve(f.Hub)
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
func TestStatus_LyxPollutionDetected(t *testing.T) {
	t.Parallel()

	f := setupStatusFixture(t)

	// Force-add a file under _lyx to the host index. In normal operation _lyx is a
	// junction (excluded from tracking), but an accidental git add -f can bypass that.
	// We create a plain file named _lyx/accidental.txt and force-add it.
	lyxDir := filepath.Join(f.Hub, "_lyx")

	// The junction exists at this path; we need to place a polluting tracked file there.
	// Write a file into the weft _lyx dir (reachable via the junction) and stage it in the host.
	accidentalPath := filepath.Join(lyxDir, "accidental.txt")
	if err := os.WriteFile(accidentalPath, []byte("polluted"), 0o644); err != nil {
		t.Fatalf("WriteFile accidental.txt: %v", err)
	}

	// git add -f bypasses .gitignore / git-exclude so the junction exclusion is overridden.
	lyxtest.MustRun(t, f.Hub, "git", "add", "-f", accidentalPath)

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
}

// TestStatus_CodeguidePollutionReportOnly asserts that a force-added _codeguide path is
// flagged as report-only (no junction to restore in this release).
func TestStatus_CodeguidePollutionReportOnly(t *testing.T) {
	t.Parallel()

	f := setupStatusFixture(t)

	// Create and force-add a file under _codeguide in the host worktree.
	codeguideDir := filepath.Join(f.Hub, "_codeguide")
	if err := os.MkdirAll(codeguideDir, 0o755); err != nil {
		t.Fatalf("MkdirAll _codeguide: %v", err)
	}
	codeguidePath := filepath.Join(codeguideDir, "overview.md")
	if err := os.WriteFile(codeguidePath, []byte("codeguide polluted"), 0o644); err != nil {
		t.Fatalf("WriteFile _codeguide/overview.md: %v", err)
	}

	// Force-add to bypass any exclusion; _codeguide may not be excluded yet.
	lyxtest.MustRun(t, f.Hub, "git", "add", "-f", codeguidePath)

	// Commit so git ls-files picks it up as tracked (ls-files shows staged and committed files).
	lyxtest.MustRun(t, f.Hub, "git", "commit", "-m", "test: force-add codeguide pollution")

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

	if len(pair.Pollution) == 0 {
		t.Fatalf("Pollution is empty after force-add of _codeguide file; want at least one entry")
	}

	found := false
	for _, pe := range pair.Pollution {
		if strings.HasPrefix(pe.Path, "_codeguide") {
			found = true
			if !pe.ReportOnly {
				t.Errorf("Pollution entry for %q has ReportOnly=false; want true (_codeguide is report-only)", pe.Path)
			}
			if pe.Remedy != "" {
				t.Errorf("Pollution entry for %q has Remedy=%q; want empty (report-only)", pe.Path, pe.Remedy)
			}
			break
		}
	}
	if !found {
		t.Errorf("no pollution entry with _codeguide prefix found; Pollution = %+v", pair.Pollution)
	}
}

// assertBranch is a test helper that reads the current branch of a git worktree at dir
// and fails the test if it cannot be determined.
func assertBranch(t *testing.T, dir string) string {
	t.Helper()
	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("readBranch(%s): err=%v exit=%d", dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}
