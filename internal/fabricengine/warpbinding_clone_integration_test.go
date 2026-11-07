// warpbinding_clone_integration_test.go proves CloneHub's warp-binding conflict rule end to end.
// It runs against real local bare-repo fixtures.
// It also proves the pre-hub probe's three-way failure taxonomy: absent, found, or hard error.
// It covers the old-order bootstrap guard and --reset in both argument forms.
// The refusals under --reset must leave the existing hub in place.
// It also checks that a two-argument re-clone against an existing hub never runs the probe.
// Its fixture helpers are reused from clone_adopt_test.go:
// makeBareRemote, makeEmptyBareRemote, commitFileOnBranch, gitOutput, and assertBoardIsWeftWorktree.
// This file adds only seedWeftBinding, the one fixture shape none of them produces.
//
// It lives in package fabricengine_test, apart from the internal unit tests, to isolate its real git fixtures.
// It shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedWeftBinding commits a .lym-warp file containing warpURL and a trailing newline onto bareRemote's main branch.
// It stands in for the record a prior clone would have left there.
// Callers supply only the URL, not the record's on-disk shape.
func seedWeftBinding(t *testing.T, dir, bareRemote, warpURL string) {
	t.Helper()
	commitFileOnBranch(t, dir, bareRemote, "main", fabricengine.WarpBindingFileName, warpURL+"\n")
}

// noProbeResidueInParent asserts that cloneParent holds no hub for the derived names and no probe leftovers.
// The pre-hub probe exists to guarantee this on every failure path.
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
		if strings.HasPrefix(entry.Name(), fabricengine.WarpProbeDirPrefixForTest) {
			t.Errorf("probe residue %s survives in clone parent; want it removed", entry.Name())
		}
	}
}

// TestCloneHub_BootstrapWritesBinding asserts that a forced clone of an unbound weft writes the record;
// the record sits at the board root with the supplied URL and is tracked on the board worktree.
func TestCloneHub_BootstrapWritesBinding(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "bootstrap-warp")
	weftBare := makeBareRemote(t, fixtures, "bootstrap-weft")
	warpURL := filepath.ToSlash(warpBare)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        warpURL,
		ForceBootstrap: true,
		Shortname:      "tst",
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

	// CloneHub returns typed values and never commits the record; the CLI layer commits it through Bolt.
	// Here "tracked" means git sees it in the board worktree as a real, un-ignored file.
	// --others --exclude-standard is the `git ls-files` form for that;
	// a plain `git ls-files` reports only paths already staged or committed.
	seen := gitOutput(t, res.BoardDir, "ls-files", "--others", "--exclude-standard", fabricengine.WarpBindingFileName)
	if seen != fabricengine.WarpBindingFileName {
		t.Errorf("git ls-files --others %s in board dir = %q; want it visible in the worktree", fabricengine.WarpBindingFileName, seen)
	}
}

// TestCloneHub_DerivesWarpFromBinding asserts that a one-argument clone yields the two-argument geometry;
// the weft's main carries the record, and the hub name and warp come from it alone.
func TestCloneHub_DerivesWarpFromBinding(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
	if filepath.Base(res.BoardDir) != fabricengine.BoltDirName {
		t.Errorf("res.BoardDir = %q; want it to end in %s", res.BoardDir, fabricengine.BoltDirName)
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

// TestCloneHub_MatchingBindingIsNoOp asserts that a two-argument clone with the exact recorded URL succeeds;
// the record's bytes stay unchanged on disk and as committed.
func TestCloneHub_MatchingBindingIsNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// TestCloneHub_NormalizedBindingMatch asserts that a URL differing only by a trailing .git still matches;
// the clone succeeds and the record is unchanged.
func TestCloneHub_NormalizedBindingMatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// TestCloneHub_ConflictLeavesNoHub asserts that a two-argument clone naming a different warp than the record fails;
// the error names both URLs and "refusing to re-point".
// No hub directory or probe residue is left for either URL's derived name, which the pre-hub probe guarantees.
func TestCloneHub_ConflictLeavesNoHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
	if !errors.Is(err, fabricengine.ErrWarpBindingMismatch) {
		t.Errorf("CloneHub() error = %v; want errors.Is ErrWarpBindingMismatch", err)
	}

	noProbeResidueInParent(t, cloneParent,
		fabricengine.DeriveWarpName(recordedURL),
		fabricengine.DeriveWarpName(suppliedURL))
}

// TestCloneHub_UnboundWeftNamesTwoArgForm asserts that a one-argument clone of an unbound weft fails;
// the error names both the unbound condition and the two-argument remedy, and no hub is created.
//
//testtiming:keep a one-argument clone against an unbound weft failing with the unbound condition and the two-argument remedy named and no hub created; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_UnboundWeftNamesTwoArgForm(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	weftBare := makeBareRemote(t, fixtures, "unbound-weft")

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
	})
	if err == nil {
		t.Fatalf("CloneHub() against an unbound weft with no warp URL should have failed")
	}
	if !errors.Is(err, fabricengine.ErrNoWarpBinding) {
		t.Errorf("CloneHub() error = %v; want errors.Is ErrNoWarpBinding", err)
	}
	if !strings.Contains(err.Error(), "lym fabric clone <weft-url> <warp-url>") {
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

// TestCloneHub_EmptyWeftRemoteTaxonomy covers both argument forms against an empty (unborn-HEAD) weft remote;
// the one-argument form reports unbound rather than a git error.
// The two-argument form bootstraps and writes the record, on the orphan-create path through ensureBoardWorktree.
func TestCloneHub_EmptyWeftRemoteTaxonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
		if !errors.Is(err, fabricengine.ErrNoWarpBinding) {
			t.Errorf("CloneHub() error = %v; want it to report unbound, not a git error", err)
		}
	})

	t.Run("TwoArgumentBootstraps", func(t *testing.T) {
		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "empty-bootstrap-warp")
		weftBare := makeEmptyBareRemote(t, fixtures, "empty-bootstrap-weft")
		warpURL := filepath.ToSlash(warpBare)

		cloneParent := t.TempDir()
		res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(weftBare),
			WarpURL:   warpURL,
			Shortname: "tst",
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

// TestCloneHub_UnreachableWeftIsHardError asserts that a clone against a missing weft path fails;
// in both argument forms the error carries the "probe weft " prefix, never reports unbound, and never bootstraps.
func TestCloneHub_UnreachableWeftIsHardError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
		if errors.Is(err, fabricengine.ErrNoWarpBinding) {
			t.Errorf("CloneHub() error = %v; want it not to report unbound", err)
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
		if errors.Is(err, fabricengine.ErrNoWarpBinding) {
			t.Errorf("CloneHub() error = %v; want it not to report unbound", err)
		}
		noProbeResidueInParent(t, cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))
	})
}

// TestCloneHub_AbsenceDiscriminatorDistinguishesMissingFromBroken asserts the absent-versus-broken split:
// a weft whose HEAD lacks the record is absent, so the one-argument form reports unbound;
// a weft whose HEAD object is unreadable is a hard error.
// That split is why the ls-tree discriminator exists.
func TestCloneHub_AbsenceDiscriminatorDistinguishesMissingFromBroken(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
		if !errors.Is(err, fabricengine.ErrNoWarpBinding) {
			t.Errorf("CloneHub() error = %v; want it to report unbound", err)
		}
	})

	t.Run("BrokenHeadIsHardError", func(t *testing.T) {
		fixtures := t.TempDir()
		sourceBare := makeBareRemote(t, fixtures, "broken-source-weft")

		// Clone the valid bare weft fixture into a scratch bare repo.
		// Then remove the object HEAD resolves to, so `git ls-tree HEAD` itself fails.
		brokenBare := filepath.Join(fixtures, "broken-weft.git")
		gitkit.MustRun(t, fixtures, "git", "clone", "--bare", filepath.ToSlash(sourceBare), filepath.Base(brokenBare))

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
		if errors.Is(err, fabricengine.ErrNoWarpBinding) {
			t.Errorf("CloneHub() error = %v; want it not to report unbound", err)
		}
	})
}

// TestCloneHub_BackfillsBindingOnPreBindingHub asserts that an anchor-only weft gets its record backfilled;
// given an explicit warp URL, it writes the record and reports WarpBindingRecorded == true without ForceBootstrap.
// The anchor alone satisfies the old-order guard.
func TestCloneHub_BackfillsBindingOnPreBindingHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "backfill-warp")
	weftBare := makeBareRemote(t, fixtures, "backfill-weft")
	warpURL := filepath.ToSlash(warpBare)
	commitFileOnBranch(t, fixtures, weftBare, "main", lymcwd.AnchorFileName, ".\n")

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

// TestCloneHub_OldOrderInvocationIsRefused asserts that the old argument order is refused:
// the test passes an ordinary source repo (no lym markers) as the weft and a real repo as the warp;
// with ForceBootstrap false, the call creates no hub and leaves the rejected repo untouched.
//
//testtiming:keep the pre-change argument order being refused with no hub created and the rejected repo untouched; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_OldOrderInvocationIsRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	ordinarySourceBare := makeBareRemote(t, fixtures, "old-order-source")
	warpBare := makeBareRemote(t, fixtures, "old-order-warp")

	beforeCommitCount := gitOutput(t, ordinarySourceBare, "rev-list", "--count", "--all")
	beforeRefs := gitOutput(t, ordinarySourceBare, "for-each-ref", "--format=%(refname)")

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:   filepath.ToSlash(ordinarySourceBare),
		WarpURL:   filepath.ToSlash(warpBare),
		Shortname: "tst",
	})
	if err == nil {
		t.Fatalf("CloneHub() with the pre-change argument order should have been refused")
	}
	if !strings.Contains(err.Error(), "refusing to bootstrap") {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "refusing to bootstrap")
	}
	if !strings.Contains(err.Error(), "check the argument order") {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "check the argument order")
	}
	if want := lymcwd.AnchorFileName + " on weft:main or main-weft"; !strings.Contains(err.Error(), want) {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), want)
	}

	noProbeResidueInParent(t, cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

	afterCommitCount := gitOutput(t, ordinarySourceBare, "rev-list", "--count", "--all")
	afterRefs := gitOutput(t, ordinarySourceBare, "for-each-ref", "--format=%(refname)")
	if afterCommitCount != beforeCommitCount {
		t.Errorf("rejected repo commit count changed: before = %s, after = %s; want unchanged", beforeCommitCount, afterCommitCount)
	}
	if afterRefs != beforeRefs {
		t.Errorf("rejected repo ref list changed: before = %q, after = %q; want unchanged", beforeRefs, afterRefs)
	}
}

// TestCloneHub_AnchorBearingWeftPassesGuard asserts that an anchor-only weft passes the old-order guard;
// it bootstraps with ForceBootstrap false, so the guard is checked on its own, not through the backfill.
// It stays a separate test so a guard regression fails under its own name.
func TestCloneHub_AnchorBearingWeftPassesGuard(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// branch names the weft branch that carries the marker.
		branch string
	}{
		{"MarkerAtHead", "main"},
		{"MarkerOnlyOnMainWeft", "main-weft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()

			warpBare := makeBareRemote(t, fixtures, "guard-pass-warp")
			weftBare := makeBareRemote(t, fixtures, "guard-pass-weft")
			if tt.branch != "main" {
				seedWeftBranchFromMain(t, fixtures, weftBare, tt.branch)
			}
			commitFileOnBranch(t, fixtures, weftBare, tt.branch, lymcwd.AnchorFileName, ".\n")

			cloneParent := t.TempDir()
			res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
				WeftURL: filepath.ToSlash(weftBare),
				WarpURL: filepath.ToSlash(warpBare),
			})
			if err != nil {
				t.Fatalf("CloneHub() error = %v; want nil (an anchor-bearing weft passes the guard without ForceBootstrap)", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		})
	}
}

// TestCloneHub_ForceBootstrapOverridesGuard asserts that ForceBootstrap passes an ordinary source repo;
// the clone succeeds and writes the record, so the flag is the only way through the guard.
func TestCloneHub_ForceBootstrapOverridesGuard(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	ordinarySourceBare := makeBareRemote(t, fixtures, "force-override-source")
	warpBare := makeBareRemote(t, fixtures, "force-override-warp")
	warpURL := filepath.ToSlash(warpBare)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(ordinarySourceBare),
		WarpURL:        warpURL,
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil (ForceBootstrap is the only way through the old-order guard)", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if !res.WarpBindingRecorded {
		t.Errorf("res.WarpBindingRecorded = false; want true")
	}
	if res.WarpURL != warpURL {
		t.Errorf("res.WarpURL = %q; want %q", res.WarpURL, warpURL)
	}
}

// TestCloneHub_ResetInBothArgumentForms covers --reset in both invocation shapes.
// The two-argument subtest re-clones the same pair with Reset true and expects the second call to succeed.
// The one-argument subtest covers the normal invocation once a weft is bound:
// re-clone with only the weft URL and Reset true tears the hub down and re-creates it;
// the warp is still derived from the recorded binding.
func TestCloneHub_ResetInBothArgumentForms(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("TwoArgument", func(t *testing.T) {
		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "reset-two-warp")
		weftBare := makeBareRemote(t, fixtures, "reset-two-weft")
		warpURL := filepath.ToSlash(warpBare)

		cloneParent := t.TempDir()
		opts := fabricengine.CloneOptions{
			WeftURL:        filepath.ToSlash(weftBare),
			WarpURL:        warpURL,
			ForceBootstrap: true,
			Shortname:      "tst",
		}
		res, err := fabricengine.CloneHub(cloneParent, opts)
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

		opts.Reset = true
		res2, err := fabricengine.CloneHub(cloneParent, opts)
		if err != nil {
			t.Fatalf("second CloneHub() with Reset error = %v; want nil", err)
		}
		if res2.HubPath != res.HubPath {
			t.Errorf("res2.HubPath = %q; want %q (same hub geometry after reset)", res2.HubPath, res.HubPath)
		}
		if _, statErr := os.Stat(res2.HubPath); statErr != nil {
			t.Errorf("hub %s does not exist after reset re-clone: %v", res2.HubPath, statErr)
		}
	})

	t.Run("OneArgument", func(t *testing.T) {
		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "reset-one-warp")
		weftBare := makeBareRemote(t, fixtures, "reset-one-weft")
		warpURL := filepath.ToSlash(warpBare)

		// CloneHub never commits the record; the CLI layer does, through Bolt.
		// The "bound hub" precondition is therefore seeded directly onto weft:main.
		// That matches what a prior clone and the CLI's commit leave behind.
		seedWeftBinding(t, fixtures, weftBare, warpURL)

		cloneParent := t.TempDir()
		firstRes, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
		})
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		originalHubPath := firstRes.HubPath
		t.Cleanup(func() { _ = os.RemoveAll(originalHubPath) })

		// The hub name derives from the recorded binding, so the re-clone resolves to the first clone's hub path.
		res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
			Reset:   true,
		})
		if err != nil {
			t.Fatalf("one-argument CloneHub() with Reset error = %v; want nil", err)
		}
		if res.HubPath != originalHubPath {
			t.Errorf("res.HubPath = %q; want %q (torn down and re-created at the same derived path)", res.HubPath, originalHubPath)
		}
		if res.WarpURL != warpURL {
			t.Errorf("res.WarpURL = %q; want %q (still derived from the recorded binding)", res.WarpURL, warpURL)
		}
		if _, statErr := os.Stat(res.HubPath); statErr != nil {
			t.Errorf("hub %s does not exist after one-argument reset re-clone: %v", res.HubPath, statErr)
		}
	})
}

// TestCloneHub_ResetWithSubpathPresentInWarp asserts that a --reset clone passes the subpath check:
// the --subpath names a warp directory, and the hub is re-created anchored there.
func TestCloneHub_ResetWithSubpathPresentInWarp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemoteWithSubdir(t, fixtures, "reset-subpath-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "reset-subpath-weft")

	opts := fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        "backend",
		ForceBootstrap: true,
		Shortname:      "tst",
	}
	cloneParent := t.TempDir()
	first, err := fabricengine.CloneHub(cloneParent, opts)
	if err != nil {
		t.Fatalf("first CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })

	opts.Reset = true
	res, err := fabricengine.CloneHub(cloneParent, opts)
	if err != nil {
		t.Fatalf("CloneHub(Reset) with a subpath present in the warp error = %v; want nil", err)
	}
	if res.Anchor != "backend" {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, "backend")
	}
}

// TestCloneHub_ResetRefusalKeepsExistingHub asserts that a --reset refused before removal leaves the hub in place.
// The refusals are: a warp URL that disagrees with the recorded binding; the old-order bootstrap guard;
// a --subpath that disagrees with the recorded anchor; or a --subpath absent from the warp.
// A sentinel file written into the hub proves it survives, since a removed hub takes the file with it.
func TestCloneHub_ResetRefusalKeepsExistingHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	plantSentinel := func(t *testing.T, hubPath string) string {
		t.Helper()
		sentinel := filepath.Join(hubPath, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write sentinel %s: %v", sentinel, err)
		}
		return sentinel
	}

	t.Run("BindingMismatch", func(t *testing.T) {
		t.Parallel()

		recordedFixtures := t.TempDir()
		suppliedFixtures := t.TempDir()
		// Both warps share a basename, so the supplied URL derives the same hub path as the recorded one,
		// and it names a different repository.
		recordedWarpBare := makeBareRemote(t, recordedFixtures, "reset-refusal-warp")
		suppliedWarpBare := makeBareRemote(t, suppliedFixtures, "reset-refusal-warp")
		weftBare := makeBareRemote(t, recordedFixtures, "reset-refusal-weft")
		recordedURL := filepath.ToSlash(recordedWarpBare)
		seedWeftBinding(t, recordedFixtures, weftBare, recordedURL)

		cloneParent := t.TempDir()
		first, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
			WarpURL: recordedURL,
		})
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
		sentinel := plantSentinel(t, first.HubPath)

		_, err = fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
			WeftURL: filepath.ToSlash(weftBare),
			WarpURL: filepath.ToSlash(suppliedWarpBare),
			Reset:   true,
		})
		if !errors.Is(err, fabricengine.ErrWarpBindingMismatch) {
			t.Errorf("CloneHub(Reset) error = %v; want errors.Is ErrWarpBindingMismatch", err)
		}
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Errorf("sentinel in the existing hub: %v; want the refused reset to leave the hub in place", statErr)
		}
	})

	t.Run("OldOrderGuard", func(t *testing.T) {
		t.Parallel()

		fixtures := t.TempDir()
		ordinarySourceBare := makeBareRemote(t, fixtures, "reset-refusal-source")
		warpBare := makeBareRemote(t, fixtures, "reset-refusal-guard-warp")

		opts := fabricengine.CloneOptions{
			WeftURL:        filepath.ToSlash(ordinarySourceBare),
			WarpURL:        filepath.ToSlash(warpBare),
			ForceBootstrap: true,
			Shortname:      "tst",
		}
		cloneParent := t.TempDir()
		first, err := fabricengine.CloneHub(cloneParent, opts)
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
		sentinel := plantSentinel(t, first.HubPath)

		opts.ForceBootstrap = false
		opts.Reset = true
		_, err = fabricengine.CloneHub(cloneParent, opts)
		if err == nil || !strings.Contains(err.Error(), "refusing to bootstrap") {
			t.Errorf("CloneHub(Reset) error = %v; want it to contain %q", err, "refusing to bootstrap")
		}
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Errorf("sentinel in the existing hub: %v; want the refused reset to leave the hub in place", statErr)
		}
	})

	t.Run("SubpathDisagreesWithRecordedAnchor", func(t *testing.T) {
		t.Parallel()

		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "reset-refusal-anchor-warp")
		weftBare := makeBareRemote(t, fixtures, "reset-refusal-anchor-weft")
		commitFileOnBranch(t, fixtures, weftBare, "main", lymcwd.AnchorFileName, ".\n")

		opts := fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(weftBare),
			WarpURL:   filepath.ToSlash(warpBare),
			Shortname: "tst",
		}
		cloneParent := t.TempDir()
		first, err := fabricengine.CloneHub(cloneParent, opts)
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
		sentinel := plantSentinel(t, first.HubPath)

		opts.Subpath = "elsewhere"
		opts.Reset = true
		_, err = fabricengine.CloneHub(cloneParent, opts)
		if err == nil || !strings.Contains(err.Error(), "does not match the recorded anchor") {
			t.Errorf("CloneHub(Reset) error = %v; want it to contain %q", err, "does not match the recorded anchor")
		}
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Errorf("sentinel in the existing hub: %v; want the refused reset to leave the hub in place", statErr)
		}
		noProbeResidueInParent(t, cloneParent)
	})

	t.Run("SubpathAbsentFromWarp", func(t *testing.T) {
		t.Parallel()

		fixtures := t.TempDir()
		warpBare := makeBareRemote(t, fixtures, "reset-refusal-subpath-warp")
		weftBare := makeBareRemote(t, fixtures, "reset-refusal-subpath-weft")

		opts := fabricengine.CloneOptions{
			WeftURL:        filepath.ToSlash(weftBare),
			WarpURL:        filepath.ToSlash(warpBare),
			ForceBootstrap: true,
			Shortname:      "tst",
		}
		cloneParent := t.TempDir()
		first, err := fabricengine.CloneHub(cloneParent, opts)
		if err != nil {
			t.Fatalf("first CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
		sentinel := plantSentinel(t, first.HubPath)

		opts.Subpath = "backedn"
		opts.Reset = true
		_, err = fabricengine.CloneHub(cloneParent, opts)
		if err == nil || !strings.Contains(err.Error(), "does not exist as a directory") {
			t.Errorf("CloneHub(Reset) error = %v; want it to contain %q", err, "does not exist as a directory")
		}
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Errorf("sentinel in the existing hub: %v; want the refused reset to leave the hub in place", statErr)
		}
		noProbeResidueInParent(t, cloneParent)
	})
}

// TestCloneHub_ResetSubpathCheckReadsRecordedAnchor asserts the --subpath check of a --reset clone:
// it reads the recorded anchor from the weft branches the clone adopts it from.
// A marker only on main-weft, or on the default branch when main-weft carries none, is adopted,
// and it refuses a disagreeing --subpath and leaves the hub in place.
// A marker on a default branch that is not the warp branch is not adopted,
// so a --subpath present in the warp is accepted.
func TestCloneHub_ResetSubpathCheckReadsRecordedAnchor(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// seed shapes the weft bare's branches; the warp bare always carries a backend directory.
		seed func(t *testing.T, fixtures, weftBare string)
		// subpath is the explicit --subpath of the --reset clone.
		subpath string
		// wantRecordedAnchor is the anchor the refusal must name; empty means the reset must succeed.
		wantRecordedAnchor string
	}{
		{"MarkerOnlyOnMainWeft", func(t *testing.T, fixtures, weftBare string) {
			seedWeftBranchFromMain(t, fixtures, weftBare, "main-weft")
			commitFileOnBranch(t, fixtures, weftBare, "main-weft", lymcwd.AnchorFileName, "backend\n")
		}, "elsewhere", "backend"},
		{"MainWeftWithoutMarkerFallsBackToDefaultBranch", func(t *testing.T, fixtures, weftBare string) {
			seedWeftBranchFromMain(t, fixtures, weftBare, "main-weft")
			commitFileOnBranch(t, fixtures, weftBare, "main", lymcwd.AnchorFileName, "backend\n")
		}, "elsewhere", "backend"},
		{"MarkerOnNonWarpDefaultBranchIsNotAdopted", func(t *testing.T, fixtures, weftBare string) {
			seedWeftBranchFromMain(t, fixtures, weftBare, "main-weft")
			seedWeftBranchFromMain(t, fixtures, weftBare, "trunk")
			commitFileOnBranch(t, fixtures, weftBare, "trunk", lymcwd.AnchorFileName, "elsewhere\n")
			gitkit.Git(t, weftBare, "symbolic-ref", "HEAD", "refs/heads/trunk")
		}, "backend", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()
			warpBare := makeBareRemoteWithSubdir(t, fixtures, "reset-anchor-source-warp", "backend")
			weftBare := makeBareRemote(t, fixtures, "reset-anchor-source-weft")
			tt.seed(t, fixtures, weftBare)

			opts := fabricengine.CloneOptions{
				WeftURL:   filepath.ToSlash(weftBare),
				WarpURL:   filepath.ToSlash(warpBare),
				Shortname: "tst",
			}
			cloneParent := t.TempDir()
			first, err := fabricengine.CloneHub(cloneParent, opts)
			if err != nil {
				t.Fatalf("first CloneHub() error = %v; want nil", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
			sentinel := filepath.Join(first.HubPath, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
				t.Fatalf("write sentinel %s: %v", sentinel, err)
			}

			opts.Subpath = tt.subpath
			opts.Reset = true
			res, err := fabricengine.CloneHub(cloneParent, opts)
			if tt.wantRecordedAnchor == "" {
				if err != nil {
					t.Fatalf("CloneHub(Reset) error = %v; want nil", err)
				}
				if res.Anchor != tt.subpath {
					t.Errorf("res.Anchor = %q; want %q", res.Anchor, tt.subpath)
				}
				return
			}
			if want := "does not match the recorded anchor " + `"` + tt.wantRecordedAnchor + `"`; err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("CloneHub(Reset) error = %v; want it to contain %q", err, want)
			}
			if _, statErr := os.Stat(sentinel); statErr != nil {
				t.Errorf("sentinel in the existing hub: %v; want the refused reset to leave the hub in place", statErr)
			}
		})
	}
}

// TestCloneHub_HubExistsCheckPrecedesProbeInTwoArgForm asserts that the hub-exists check runs first:
// a second two-argument clone of an already-cloned pair, with Reset false, fails with "hub already exists",
// and it never runs the probe.
// No directory whose name begins with the probe prefix is created in the clone parent.
// That absence shows the check short-circuits before the probe touches the network.
//
// The one-argument form is not asserted here.
// There the probe must precede the hub-exists check, because the hub name is unknowable until the binding is read.
// That order is deliberate; a reader should not change it to match the two-argument order.
func TestCloneHub_HubExistsCheckPrecedesProbeInTwoArgForm(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "precedes-warp")
	weftBare := makeBareRemote(t, fixtures, "precedes-weft")
	warpURL := filepath.ToSlash(warpBare)

	cloneParent := t.TempDir()
	firstRes, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        warpURL,
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("first CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(firstRes.HubPath) })

	_, err = fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: warpURL,
	})
	if err == nil {
		t.Fatalf("second CloneHub() against an existing hub should have failed")
	}
	if !strings.Contains(err.Error(), "hub already exists") {
		t.Errorf("CloneHub() error = %q; want it to contain %q", err.Error(), "hub already exists")
	}

	entries, err := os.ReadDir(cloneParent)
	if err != nil {
		t.Fatalf("read clone parent %s: %v", cloneParent, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), fabricengine.WarpProbeDirPrefixForTest) {
			t.Errorf("probe residue %s exists; want the hub-exists check to short-circuit before the probe ever runs", entry.Name())
		}
	}
}
