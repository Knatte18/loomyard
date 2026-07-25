//go:build integration

// clone_differential_test.go runs warpengine.CloneHub and fabricengine.CloneHub
// back to back against the same local bare fixtures and asserts equivalent end
// state: same hub directory layout, same resolvedBoardURL contract (derived and
// explicit), geometry round-tripping through hubgeometry.Resolve on both sides,
// and strict-abort equivalence on a failing clone. The one normalized delta is
// the weft primary's branch: warp's sits on the host's own branch name ("main"),
// fabric's sits on fabricengine.WeftBranchName of that same name ("main-weft").
//
// Package fabricengine_test (external test package) because this file imports
// both warpengine and fabricengine; it shares the single TestMain declared in
// testmain_test.go (package fabricengine) per the test-package-discipline
// decision.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// makeBareRemote creates a bare git repository with a single commit on the main
// branch, mirroring warpengine's clone_integration_test.go helper of the same
// name (duplicated here, not imported, since that helper is unexported in a
// different package).
//
// It initializes a bare repo at <dir>/<name>.git, then seeds it by initializing
// a working repository, creating and committing a README, and pushing back to
// the bare repo. Returns the path to the bare repository.
func makeBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	lyxtest.MustRun(t, bare, "git", "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "init", "-b", "main")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.name", "Test")

	bareURL := filepath.ToSlash(bare)
	lyxtest.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "add", "README.md")
	lyxtest.MustRun(t, tempWork, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// currentBranch returns the branch checked out at repoPath via `git branch
// --show-current`, failing the test on any git error.
func currentBranch(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current in %s: %v", repoPath, err)
	}
	return strings.TrimSpace(string(out))
}

// TestCloneHub_DifferentialEquivalence runs warpengine.CloneHub and
// fabricengine.CloneHub against one shared set of local bare fixtures, cloning
// into two separate parent directories, and asserts the two engines produce
// equivalent end state modulo the deliberate weft-primary-branch delta.
func TestCloneHub_DifferentialEquivalence(t *testing.T) {
	t.Run("DerivedBoardURL", func(t *testing.T) {
		fixtureDir := t.TempDir()
		hostBare := makeBareRemote(t, fixtureDir, "myrepo")
		weftBare := makeBareRemote(t, fixtureDir, "myrepo-weft")
		_ = makeBareRemote(t, fixtureDir, "myrepo-weft.wiki")

		warpCwd := t.TempDir()
		fabricCwd := t.TempDir()

		warpHub, warpBoardURL, err := warpengine.CloneHub(warpCwd, hostBare, weftBare, "")
		if err != nil {
			t.Fatalf("warpengine.CloneHub: %v", err)
		}
		fabricHub, fabricBoardURL, err := fabricengine.CloneHub(fabricCwd, hostBare, weftBare, "")
		if err != nil {
			t.Fatalf("fabricengine.CloneHub: %v", err)
		}

		assertEquivalentClones(t, warpHub, fabricHub, warpBoardURL, fabricBoardURL)
	})

	t.Run("ExplicitBoardURL", func(t *testing.T) {
		fixtureDir := t.TempDir()
		hostBare := makeBareRemote(t, fixtureDir, "myrepo")
		weftBare := makeBareRemote(t, fixtureDir, "myrepo-weft")
		explicitBoardBare := makeBareRemote(t, fixtureDir, "myboard")

		warpCwd := t.TempDir()
		fabricCwd := t.TempDir()

		warpHub, warpBoardURL, err := warpengine.CloneHub(warpCwd, hostBare, weftBare, explicitBoardBare)
		if err != nil {
			t.Fatalf("warpengine.CloneHub: %v", err)
		}
		fabricHub, fabricBoardURL, err := fabricengine.CloneHub(fabricCwd, hostBare, weftBare, explicitBoardBare)
		if err != nil {
			t.Fatalf("fabricengine.CloneHub: %v", err)
		}

		if warpBoardURL != explicitBoardBare || fabricBoardURL != explicitBoardBare {
			t.Errorf("resolvedBoardURL: warp=%q fabric=%q; want both %q", warpBoardURL, fabricBoardURL, explicitBoardBare)
		}

		assertEquivalentClones(t, warpHub, fabricHub, warpBoardURL, fabricBoardURL)
	})
}

// assertEquivalentClones compares the two hub trees for structural and
// geometry equivalence, normalizing the one known delta: fabric's weft primary
// branch carries the WeftBranchName suffix that warp's does not.
func assertEquivalentClones(t *testing.T, warpHub, fabricHub, warpBoardURL, fabricBoardURL string) {
	t.Helper()

	// Same hub directory basename (both derived from the same host URL/name).
	if filepath.Base(warpHub) != filepath.Base(fabricHub) {
		t.Errorf("hub dir basename: warp=%q fabric=%q; want equal", filepath.Base(warpHub), filepath.Base(fabricHub))
	}

	// Both resolved board URLs must be identical strings (same weftBare input,
	// same derivation or same explicit override on both sides).
	if warpBoardURL != fabricBoardURL {
		t.Errorf("resolvedBoardURL: warp=%q fabric=%q; want equal", warpBoardURL, fabricBoardURL)
	}

	// Host clone, weft sibling, and _board must all exist under both hubs.
	warpHostPath := filepath.Join(warpHub, "myrepo")
	fabricHostPath := filepath.Join(fabricHub, "myrepo")
	for _, path := range []string{warpHostPath, fabricHostPath} {
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			t.Fatalf("host clone missing .git at %s: %v", path, err)
		}
	}

	warpBoardPath := hubgeometry.BoardDir(warpHub)
	fabricBoardPath := hubgeometry.BoardDir(fabricHub)
	for _, path := range []string{warpBoardPath, fabricBoardPath} {
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			t.Fatalf("board clone missing .git at %s: %v", path, err)
		}
	}

	// Geometry round-trips through hubgeometry.Resolve on both sides.
	warpLayout, err := hubgeometry.Resolve(warpHostPath)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(warp host): %v", err)
	}
	fabricLayout, err := hubgeometry.Resolve(fabricHostPath)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(fabric host): %v", err)
	}

	if warpLayout.PrimeName() != fabricLayout.PrimeName() {
		t.Errorf("PrimeName: warp=%q fabric=%q; want equal", warpLayout.PrimeName(), fabricLayout.PrimeName())
	}
	if filepath.Base(warpLayout.WeftRepoRoot()) != filepath.Base(fabricLayout.WeftRepoRoot()) {
		t.Errorf("WeftRepoRoot basename: warp=%q fabric=%q; want equal",
			filepath.Base(warpLayout.WeftRepoRoot()), filepath.Base(fabricLayout.WeftRepoRoot()))
	}

	// The normalized delta: fabric's weft primary sits on WeftBranchName of
	// warp's weft primary branch.
	warpWeftBranch := currentBranch(t, warpLayout.WeftRepoRoot())
	fabricWeftBranch := currentBranch(t, fabricLayout.WeftRepoRoot())
	if want := fabricengine.WeftBranchName(warpWeftBranch); fabricWeftBranch != want {
		t.Errorf("fabric weft primary branch = %q; want %q (fabricengine.WeftBranchName(%q))",
			fabricWeftBranch, want, warpWeftBranch)
	}
}

// TestCloneHub_DifferentialStrictAbort asserts that a failing weft URL leaves
// no residual hub directory on either side, each torn down through its own
// module's RemoveAll teardown seam.
func TestCloneHub_DifferentialStrictAbort(t *testing.T) {
	fixtureDir := t.TempDir()
	hostBare := makeBareRemote(t, fixtureDir, "myrepo")
	nonExistentWeft := filepath.Join(fixtureDir, "nonexistent-weft.git")
	_ = makeBareRemote(t, fixtureDir, "myrepo-weft.wiki")

	warpCwd := t.TempDir()
	fabricCwd := t.TempDir()

	warpHub, _, warpErr := warpengine.CloneHub(warpCwd, hostBare, nonExistentWeft, "")
	if warpErr == nil {
		t.Fatalf("warpengine.CloneHub should have failed with non-existent weft")
	}
	if _, err := os.Stat(warpHub); err == nil {
		t.Errorf("warp hub directory should have been removed after clone failure: %s", warpHub)
	}

	fabricHub, _, fabricErr := fabricengine.CloneHub(fabricCwd, hostBare, nonExistentWeft, "")
	if fabricErr == nil {
		t.Fatalf("fabricengine.CloneHub should have failed with non-existent weft")
	}
	if _, err := os.Stat(fabricHub); err == nil {
		t.Errorf("fabric hub directory should have been removed after clone failure: %s", fabricHub)
	}
}
