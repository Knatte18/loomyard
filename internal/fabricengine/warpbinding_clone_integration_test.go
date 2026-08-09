//go:build integration

// warpbinding_clone_integration_test.go proves CloneHub's warp-binding conflict rule and the pre-hub
// probe's three-way failure taxonomy (absent, found, hard error) end to end against real local
// bare-repo fixtures.
// It also covers the old-order bootstrap guard, --reset in both argument forms, and the ordering
// guarantee that a two-argument re-clone against an existing hub never runs the probe at all.
// Every fixture helper it uses — makeBareRemote, makeEmptyBareRemote, commitFileOnBranch, gitOutput,
// and assertBoardIsWeftWorktree — is reused directly from clone_adopt_test.go; this file adds only
// seedWeftBinding, the one fixture shape none of those helpers already produce.
//
// Package fabricengine_test to isolate real end-to-end git fixtures from the package's internal unit
// tests; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// seedWeftBinding commits a .lyx-warp file containing warpURL plus a trailing newline onto
// bareRemote's main branch, mirroring what a prior clone would have left there.
// It wraps commitFileOnBranch so every test in this file states the binding it is seeding in terms
// of the URL alone, not the record's on-disk shape.
func seedWeftBinding(t *testing.T, dir, bareRemote, warpURL string) {
	t.Helper()
	commitFileOnBranch(t, dir, bareRemote, "main", fabricengine.WarpBindingFileName, warpURL+"\n")
}

// probeDirPrefixLiteral mirrors warpProbeDirPrefix (warpprobe.go), duplicated here because this
// card's tests need it before card 12 adds the export_test.go re-export that later tests in this same
// file read the prefix through instead.
const probeDirPrefixLiteral = ".lyx-clone-probe-"

// noProbeResidueInParent asserts that clonParent contains neither a directory named after
// derivedName-HUB nor any directory whose name begins with the probe's throwaway-clone prefix — the
// filesystem property the pre-hub probe exists to guarantee on every failure path.
func noProbeResidueInParent(t *testing.T, cloneParent string, derivedNames ...string) {
	t.Helper()

	for _, name := range derivedNames {
		hubPath := fabricengine.HubPath(cloneParent, name)
		if _, err := os.Stat(hubPath); err == nil {
			t.Errorf("hub directory %s exists; want no hub created", hubPath)
		}
	}

	entries, err := os.ReadDir(cloneParent)
	if err != nil {
		t.Fatalf("read clone parent %s: %v", cloneParent, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), probeDirPrefixLiteral) {
			t.Errorf("probe residue %s survives in clone parent; want it removed", entry.Name())
		}
	}
}

// TestCloneHub_BootstrapWritesBinding asserts that a two-argument clone against a weft fixture with
// no recorded binding, run with ForceBootstrap, writes the record at the board root with the supplied
// URL and tracks it on the board worktree.
func TestCloneHub_BootstrapWritesBinding(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "bootstrap-warp")
	weftBare := makeBareRemote(t, fixtures, "bootstrap-weft")
	warpURL := filepath.ToSlash(warpBare)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        warpURL,
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if !res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = false; want true")
	}
	if res.WarpURL != warpURL {
		t.Errorf("res.WarpURL = %q; want %q", res.WarpURL, warpURL)
	}

	// CloneHub itself never commits the record (the engine returns typed values, the CLI layer
	// commits through Bolt), so "tracked" here means visible to git as a real, un-ignored file in
	// the board worktree — --others --exclude-standard is `git ls-files`'s form for that, as opposed
	// to a plain `git ls-files`, which would only ever report a path already staged or committed.
	seen := gitOutput(t, res.BoardDir, "ls-files", "--others", "--exclude-standard", fabricengine.WarpBindingFileName)
	if seen != fabricengine.WarpBindingFileName {
		t.Errorf("git ls-files --others %s in board dir = %q; want it visible in the worktree", fabricengine.WarpBindingFileName, seen)
	}
}

// TestCloneHub_DerivesWarpFromBinding asserts that a one-argument clone against a weft whose main
// carries a record produces the same geometry as the two-argument form, deriving the hub name and
// warp from the recorded binding alone.
func TestCloneHub_DerivesWarpFromBinding(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "derive-warp")
	weftBare := makeBareRemote(t, fixtures, "derive-weft")
	warpURL := filepath.ToSlash(warpBare)
	seedWeftBinding(t, fixtures, weftBare, warpURL)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	wantHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(warpURL))
	if res.HubPath != wantHubPath {
		t.Errorf("res.HubPath = %q; want %q", res.HubPath, wantHubPath)
	}
	if res.Anchor != "." {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, ".")
	}
	if filepath.Base(res.BoardDir) != "_board" {
		t.Errorf("res.BoardDir = %q; want it to end in _board", res.BoardDir)
	}
	if res.WeftBase == "" {
		t.Errorf("res.WeftBase is empty; want a resolved weft-side base directory")
	}
	if res.PrimeCwd == "" {
		t.Errorf("res.PrimeCwd is empty; want a resolved prime warp worktree path")
	}

	weftPrime := filepath.Join(res.HubPath, fabricengine.DeriveWarpName(warpURL)+"-weft")
	assertBoardIsWeftWorktree(t, res.HubPath, weftPrime, "main")

	if res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = true; want false (derived from an existing record, not written fresh)")
	}
}

// TestCloneHub_MatchingBindingIsNoOp asserts that a two-argument clone whose supplied URL is
// byte-identical to the recorded binding succeeds and leaves the record's bytes unchanged, both on
// disk and as committed.
func TestCloneHub_MatchingBindingIsNoOp(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "match-warp")
	weftBare := makeBareRemote(t, fixtures, "match-weft")
	warpURL := filepath.ToSlash(warpBare)
	seedWeftBinding(t, fixtures, weftBare, warpURL)

	beforeBlob := gitOutput(t, weftBare, "show", "main:"+fabricengine.WarpBindingFileName)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: warpURL,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = true; want false (matching binding is a no-op)")
	}

	onDisk, err := os.ReadFile(filepath.Join(res.BoardDir, fabricengine.WarpBindingFileName))
	if err != nil {
		t.Fatalf("read on-disk record: %v", err)
	}
	if strings.TrimSpace(string(onDisk)) != warpURL {
		t.Errorf("on-disk record = %q; want %q", strings.TrimSpace(string(onDisk)), warpURL)
	}

	afterBlob := gitOutput(t, weftBare, "show", "main:"+fabricengine.WarpBindingFileName)
	if afterBlob != beforeBlob {
		t.Errorf("committed record changed: before = %q, after = %q; want unchanged", beforeBlob, afterBlob)
	}
}

// TestCloneHub_NormalizedBindingMatch asserts that a supplied URL differing from the record only by a
// trailing .git is still treated as matching: the clone succeeds and the record is unchanged.
func TestCloneHub_NormalizedBindingMatch(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "normalize-warp")
	weftBare := makeBareRemote(t, fixtures, "normalize-weft")
	warpURL := filepath.ToSlash(warpBare)
	seedWeftBinding(t, fixtures, weftBare, warpURL)

	suppliedURL := strings.TrimSuffix(warpURL, ".git")
	if suppliedURL == warpURL {
		t.Fatalf("fixture warp URL %q has no .git suffix to strip", warpURL)
	}

	beforeBlob := gitOutput(t, weftBare, "show", "main:"+fabricengine.WarpBindingFileName)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: suppliedURL,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = true; want false (normalized match is a no-op)")
	}

	afterBlob := gitOutput(t, weftBare, "show", "main:"+fabricengine.WarpBindingFileName)
	if afterBlob != beforeBlob {
		t.Errorf("committed record changed: before = %q, after = %q; want unchanged", beforeBlob, afterBlob)
	}
}

// TestCloneHub_ConflictLeavesNoHub asserts that a two-argument clone whose supplied URL names a
// different warp than the record fails, names both URLs plus "refusing to re-point" in the error, and
// — the property the pre-hub probe buys — leaves no hub directory or probe residue behind for either
// URL's derived name.
func TestCloneHub_ConflictLeavesNoHub(t *testing.T) {
	fixtures := t.TempDir()

	recordedWarpBare := makeBareRemote(t, fixtures, "conflict-recorded-warp")
	suppliedWarpBare := makeBareRemote(t, fixtures, "conflict-supplied-warp")
	weftBare := makeBareRemote(t, fixtures, "conflict-weft")
	recordedURL := filepath.ToSlash(recordedWarpBare)
	suppliedURL := filepath.ToSlash(suppliedWarpBare)
	seedWeftBinding(t, fixtures, weftBare, recordedURL)

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: suppliedURL,
	})
	if err == nil {
		t.Fatalf("CloneHub() with a conflicting warp URL should have failed")
	}
	if !strings.Contains(err.Error(), recordedURL) || !strings.Contains(err.Error(), suppliedURL) {
		t.Errorf("CloneHub() error = %q; want it to contain both %q and %q", err.Error(), recordedURL, suppliedURL)
	}
	if !strings.Contains(err.Error(), "refusing to re-point") {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "refusing to re-point")
	}

	noProbeResidueInParent(t, cloneParent,
		fabricengine.DeriveWarpName(recordedURL),
		fabricengine.DeriveWarpName(suppliedURL))
}

// TestCloneHub_UnboundWeftNamesTwoArgForm asserts that a one-argument clone against a weft with no
// recorded binding fails naming both the unbound condition and the two-argument remedy, and creates
// no hub.
func TestCloneHub_UnboundWeftNamesTwoArgForm(t *testing.T) {
	fixtures := t.TempDir()

	weftBare := makeBareRemote(t, fixtures, "unbound-weft")

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
	})
	if err == nil {
		t.Fatalf("CloneHub() against an unbound weft with no warp URL should have failed")
	}
	if !strings.Contains(err.Error(), "has no recorded warp binding") {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "has no recorded warp binding")
	}
	if !strings.Contains(err.Error(), "lyx fabric clone <weft-url> <warp-url>") {
		t.Errorf("CloneHub() error = %q; want it to contain the two-argument remedy", err.Error())
	}

	entries, err := os.ReadDir(cloneParent)
	if err != nil {
		t.Fatalf("read clone parent %s: %v", cloneParent, err)
	}
	if len(entries) != 0 {
		t.Errorf("clone parent %s is not empty; want no hub created", cloneParent)
	}
}

// TestCloneHub_EmptyWeftRemoteTaxonomy covers both argument forms against a genuinely empty
// (unborn-HEAD) weft remote: the one-argument form reports unbound rather than a git error, and the
// two-argument form bootstraps successfully and writes the record, preserving today's orphan-create
// path through ensureBoardWorktree.
func TestCloneHub_EmptyWeftRemoteTaxonomy(t *testing.T) {
	t.Run("OneArgumentReportsUnbound", func(t *testing.T) {
		fixtures := t.TempDir()
		weftBare := makeEmptyBareRemote(t, fixtures, "empty-unbound-weft")

		cloneParent := t.TempDir()
		_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
		})
		if err == nil {
			t.Fatalf("CloneHub() against an empty weft with no warp URL should have failed")
		}
		if !strings.Contains(err.Error(), "has no recorded warp binding") {
			t.Errorf("CloneHub() error = %q; want it to report unbound, not a git error", err.Error())
		}
	})

	t.Run("TwoArgumentBootstraps", func(t *testing.T) {
		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "empty-bootstrap-warp")
		weftBare := makeEmptyBareRemote(t, fixtures, "empty-bootstrap-weft")
		warpURL := filepath.ToSlash(warpBare)

		cloneParent := t.TempDir()
		res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
			WarpURL: warpURL,
		})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil (empty weft remotes bootstrap through ensureBoardWorktree's orphan path)", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

		if !res.WarpBindingRecorded {
			t.Errorf("res.WarpBindingRecorded = false; want true")
		}
		if res.WarpURL != warpURL {
			t.Errorf("res.WarpURL = %q; want %q", res.WarpURL, warpURL)
		}
	})
}

// TestCloneHub_UnreachableWeftIsHardError asserts that a clone against a weft path that does not
// exist fails in both argument forms with an error carrying the "probe weft " prefix, never reports
// "unbound", and never bootstraps.
func TestCloneHub_UnreachableWeftIsHardError(t *testing.T) {
	fixtures := t.TempDir()
	nonExistentWeft := filepath.ToSlash(filepath.Join(fixtures, "does-not-exist.git"))

	t.Run("OneArgument", func(t *testing.T) {
		cloneParent := t.TempDir()
		_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: nonExistentWeft,
		})
		if err == nil {
			t.Fatalf("CloneHub() against an unreachable weft should have failed")
		}
		if !strings.Contains(err.Error(), "probe weft ") {
			t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "probe weft ")
		}
		if strings.Contains(err.Error(), "has no recorded warp binding") {
			t.Errorf("CloneHub() error = %q; want it not to report unbound", err.Error())
		}
		if _, statErr := os.Stat(cloneParent); statErr != nil {
			t.Fatalf("stat clone parent: %v", statErr)
		}
		entries, err := os.ReadDir(cloneParent)
		if err != nil {
			t.Fatalf("read clone parent: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("clone parent is not empty; want no hub created")
		}
	})

	t.Run("TwoArgument", func(t *testing.T) {
		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "unreachable-warp")
		nonExistentWeft := filepath.ToSlash(filepath.Join(fixtures, "does-not-exist.git"))

		cloneParent := t.TempDir()
		_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: nonExistentWeft,
			WarpURL: filepath.ToSlash(warpBare),
		})
		if err == nil {
			t.Fatalf("CloneHub() against an unreachable weft should have failed")
		}
		if !strings.Contains(err.Error(), "probe weft ") {
			t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "probe weft ")
		}
		if strings.Contains(err.Error(), "has no recorded warp binding") {
			t.Errorf("CloneHub() error = %q; want it not to report unbound", err.Error())
		}
		noProbeResidueInParent(t, cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))
	})
}

// TestCloneHub_AbsenceDiscriminatorDistinguishesMissingFromBroken asserts that a weft whose HEAD
// exists but lacks the record is classified absent (the one-argument form reports unbound, not a git
// error), while a weft whose HEAD object is unreadable is a hard error — the property that justifies
// the ls-tree discriminator existing at all.
func TestCloneHub_AbsenceDiscriminatorDistinguishesMissingFromBroken(t *testing.T) {
	t.Run("MissingRecordReportsUnbound", func(t *testing.T) {
		fixtures := t.TempDir()
		weftBare := makeBareRemote(t, fixtures, "absent-weft")

		cloneParent := t.TempDir()
		_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
		})
		if err == nil {
			t.Fatalf("CloneHub() against a weft with no record should have failed")
		}
		if !strings.Contains(err.Error(), "has no recorded warp binding") {
			t.Errorf("CloneHub() error = %q; want it to report unbound", err.Error())
		}
	})

	t.Run("BrokenHeadIsHardError", func(t *testing.T) {
		fixtures := t.TempDir()
		sourceBare := makeBareRemote(t, fixtures, "broken-source-weft")

		// Clone the valid bare weft fixture into a scratch bare repo, then corrupt the object HEAD
		// resolves to, so `git ls-tree HEAD` itself fails.
		brokenBare := filepath.Join(fixtures, "broken-weft.git")
		lyxtest.MustRun(t, fixtures, "git", "clone", "--bare", filepath.ToSlash(sourceBare), filepath.Base(brokenBare))

		headCommit := gitOutput(t, brokenBare, "rev-parse", "HEAD")
		if headCommit == "" {
			t.Fatalf("resolve HEAD of broken-weft fixture: empty rev-parse output")
		}
		objectPath := filepath.Join(brokenBare, "objects", headCommit[:2], headCommit[2:])
		if _, statErr := os.Stat(objectPath); statErr != nil {
			t.Skipf("cannot locate loose object file for HEAD %s (likely packed on this platform): %v", headCommit, statErr)
		}
		if err := os.Remove(objectPath); err != nil {
			t.Fatalf("remove HEAD object %s: %v", objectPath, err)
		}

		cloneParent := t.TempDir()
		_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(brokenBare),
		})
		if err == nil {
			t.Fatalf("CloneHub() against a weft with an unreadable HEAD object should have failed")
		}
		if !strings.Contains(err.Error(), "probe weft ") {
			t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "probe weft ")
		}
		if strings.Contains(err.Error(), "has no recorded warp binding") {
			t.Errorf("CloneHub() error = %q; want it not to report unbound", err.Error())
		}
	})
}

// TestCloneHub_BackfillsBindingOnPreBindingHub asserts that a weft carrying .lyx-anchor but no
// .lyx-warp — a hub cloned before this change — writes the record and reports
// WarpBindingRecorded == true given an explicit warp URL, with no ForceBootstrap: the anchor alone
// satisfies the old-order guard.
func TestCloneHub_BackfillsBindingOnPreBindingHub(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "backfill-warp")
	weftBare := makeBareRemote(t, fixtures, "backfill-weft")
	warpURL := filepath.ToSlash(warpBare)
	commitFileOnBranch(t, fixtures, weftBare, "main", lyxcwd.AnchorFileName, ".\n")

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: warpURL,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil (an anchor-bearing weft passes the guard without ForceBootstrap)", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if !res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = false; want true (backfilling a pre-binding hub)")
	}
	if res.WarpURL != warpURL {
		t.Errorf("res.WarpURL = %q; want %q", res.WarpURL, warpURL)
	}
}
