//go:build integration

// sync_integration_test.go covers fabricSync's composed behavior against real
// git repositories. Two
// scenarios: Fabric.Commit's error-branch contract (a commit that lands but
// fails its correspondence record must still be reported as committed=true
// alongside the error, never swallowed into a false "no commit was made"),
// and machine-local artifacts under ".lyx" actually staying out of every
// commit at every layout.AnchorRel depth simply by living outside the
// committed "_lyx" pathspec, which only real git can decide.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newWarpWeftPair builds a hub with "warp" and "warp-weft" git repos and returns the layout.
func newWarpWeftPair(t *testing.T) (*lyxcwd.Location, string) {
	t.Helper()
	return newWarpWeftPairAt(t, ".")
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml.
func seedRepoWideFabricConfig(t *testing.T, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// seedFabricAnchor records relPath as the .lyx-anchor marker under hub's
// board directory, so Fabric.Commit's own lyxcwd.ResolveWorktree(warpPath)
// call resolves l.AnchorRel to relPath instead of falling back to a
// cwd-derived "." -- Commit re-resolves geometry from f.warpPath itself
// rather than trusting the *lyxcwd.Location fabricSync already holds, so
// a nested-AnchorRel fixture must record the anchor for real git to classify
// correctly.
func seedFabricAnchor(t *testing.T, hub, relPath string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(relPath), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// newWarpWeftPairAt is newWarpWeftPair with an explicit layout.AnchorRel: the
// weft-side _lyx is seeded at <weft>/<relPath>/_lyx, mirroring the warp's own
// repo-subpath geometry, and the returned layout's AnchorPath() points at the
// matching warp subdirectory. Alongside state.json it seeds the three machine-local
// artifacts (an advisory *.lock file, the pause flag, and a rendered fork prompt) at the
// mirrored subpath under ".lyx", outside the committed "_lyx" pathspec, so a caller can
// assert on what the commit did and did not pick up. It also seeds a sibling module's
// tree in the same shared geometry -- every module sharing one "_lyx"/".lyx" pair --
// carrying that module's own durable state.json under "_lyx" plus its pause flag under
// ".lyx", so a caller can assert that a webster commit keeps the sibling module's runtime
// state out while still carrying its durable state.
func newWarpWeftPairAt(t *testing.T, relPath string) (*lyxcwd.Location, string) {
	t.Helper()

	hub := t.TempDir()
	warp := filepath.Join(hub, "warp")
	weft := filepath.Join(hub, "warp-weft")
	for _, dir := range []string{warp, weft} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		mustGit(t, dir, "init")
		mustGit(t, dir, "config", "user.name", "Test User")
		mustGit(t, dir, "config", "user.email", "test@example.com")
	}
	commitFile(t, warp, "base.txt", "base", "warp base commit")
	commitFile(t, weft, "base.txt", "base", "weft base commit")

	// Uncommitted changes under the webster pathspec, so CommitWeft has
	// something real to commit.
	websterDir := filepath.Join(weft, relPath, lyxdirs.LyxDirName, "webster")
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("mkdir weft _lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write weft state.json: %v", err)
	}

	// The three machine-local artifacts, mirrored under ".lyx" -- outside the committed pathspec.
	websterScratchDir := filepath.Join(weft, relPath, lyxdirs.DotLyxDirName, "webster")
	if err := os.MkdirAll(filepath.Join(websterScratchDir, "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir weft .lyx: %v", err)
	}
	for name, content := range map[string]string{
		"mutate.lock":                    "lock",
		websterengine.PauseFlagName:      "paused",
		filepath.Join("prompts", "1.md"): "rendered fork prompt",
	} {
		if err := os.WriteFile(filepath.Join(websterScratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write weft %s: %v", name, err)
		}
	}

	// The sibling loom module's own tree, in the same shared geometry: its
	// durable state must still ride a webster commit, its run lock must not.
	loomDir := filepath.Join(weft, relPath, lyxdirs.LyxDirName, "loom")
	if err := os.MkdirAll(loomDir, 0o755); err != nil {
		t.Fatalf("mkdir weft loom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loomDir, "status.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write weft loom status.json: %v", err)
	}

	loomScratchDir := filepath.Join(weft, relPath, lyxdirs.DotLyxDirName, "loom")
	if err := os.MkdirAll(loomScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir weft loom scratch dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loomScratchDir, "run.lock"), []byte("locked"), 0o644); err != nil {
		t.Fatalf("write weft loom run lock: %v", err)
	}

	seedRepoWideFabricConfig(t, hub)
	seedFabricAnchor(t, hub, filepath.ToSlash(relPath))

	return &lyxcwd.Location{
		HubPath:      hub,
		WorktreeName: filepath.Base(warp),
		AnchorRel:    relPath,
	}, weft
}

// TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails proves the Fabric.Commit error
// branch passes committed through instead of forcing it to false: with a directory squatting on the
// correspondence index path, the weft commit itself lands but RecordCorrespondence fails, and
// fabricSync must report (true, err) -- the commit is real, and Fabric.Commit's contract says the
// caller gets to know that alongside the error.
func TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")
	layout, weft := newWarpWeftPair(t)

	// A directory where RecordCorrespondence expects its index file makes
	// the record step fail after the commit has already landed.
	if err := os.MkdirAll(filepath.Join(weft, ".git", "fabric-corrindex.json"), 0o755); err != nil {
		t.Fatalf("squat corrindex path: %v", err)
	}

	open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }
	committed, err := fabricSync(open, layout.AnchorRel, "corr-fail probe")

	if err == nil {
		t.Fatal("fabricSync() error = nil; want the RecordCorrespondence failure propagated")
	}
	if !committed {
		t.Error("fabricSync() committed = false; want true, the commit landed before the record step failed")
	}

	// The commit must genuinely exist with the webster message stem -- the
	// committed=true report above is about this commit, not a phantom.
	subject := strings.TrimSpace(mustGit(t, weft, "log", "-1", "--format=%s"))
	if subject != "webster: corr-fail probe" {
		t.Errorf("weft HEAD subject = %q; want %q", subject, "webster: corr-fail probe")
	}
}

// TestFabricSync_CommitsAtEveryRelPathDepth proves every machine-local transient (locks, the
// sibling loom module's run lock, webster's rendered fork prompts) stays uncommitted by REAL git
// at every layout.AnchorRel depth, not merely absent from some in-memory pathspec shape.
// Exclusion is now structural: every machine-local artifact lives under ".lyx" at the mirrored
// subpath, entirely outside the committed "_lyx" pathspec fabricSync passes -- no exclude-pattern
// machinery is involved.
// It is also the cross-module regression guard: a webster commit must hold back the sibling loom
// module's run lock too, since every module sharing one geometry shares one ".lyx" scratch tree
// the same way it shares one "_lyx" durable tree.
// Each excluded artifact is asserted both absent from the commit AND still untracked via `git
// ls-files` -- proving it never reached the pathspec at all, not merely an already-tracked file
// happening to be omitted from this one commit.
// WEFT_SKIP_PUSH is set because the scratch weft repo has no remote;
// the commit half is what is under test.
func TestFabricSync_CommitsAtEveryRelPathDepth(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
	}{
		{name: "worktree root", relPath: "."},
		{name: "one segment", relPath: "sub"},
		{name: "two segments", relPath: "wts/some-task"},
		{name: "three segments", relPath: "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEFT_SKIP_GIT", "")
			t.Setenv("WEFT_SKIP_PUSH", "1")
			layout, weft := newWarpWeftPairAt(t, tt.relPath)
			open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }

			committed, err := fabricSync(open, layout.AnchorRel, "depth probe")
			if err != nil {
				t.Fatalf("fabricSync() error = %v; want nil", err)
			}
			if !committed {
				t.Fatalf("fabricSync() committed = false; want true -- the pathspec staged nothing at RelPath %q, so the fabric sync was a silent no-op", tt.relPath)
			}

			// git always reports commit contents with forward slashes,
			// regardless of the OS-native separators the layout carries.
			base := lyxdirs.LyxDirName
			scratchBase := lyxdirs.DotLyxDirName
			if tt.relPath != "." {
				prefix := filepath.ToSlash(tt.relPath) + "/"
				base = prefix + lyxdirs.LyxDirName
				scratchBase = prefix + lyxdirs.DotLyxDirName
			}
			committedFiles := strings.Fields(mustGit(t, weft, "show", "--name-only", "--format=", "HEAD"))

			// Loom's durable status.json rides a webster commit (the two
			// modules share one _lyx); only the machine-local artifacts of
			// EITHER module are held back.
			wantPresent := []string{
				base + "/webster/state.json",
				base + "/loom/status.json",
			}
			wantAbsent := []string{
				scratchBase + "/webster/mutate.lock",
				scratchBase + "/webster/" + websterengine.PauseFlagName,
				scratchBase + "/webster/prompts/1.md",
				scratchBase + "/loom/run.lock",
			}
			for _, present := range wantPresent {
				if !containsString(committedFiles, present) {
					t.Errorf("weft commit at RelPath %q = %v; want it to contain %q", tt.relPath, committedFiles, present)
				}
			}
			for _, absent := range wantAbsent {
				if containsString(committedFiles, absent) {
					t.Errorf("weft commit at RelPath %q = %v; want it to EXCLUDE the machine-local %q", tt.relPath, committedFiles, absent)
				}
			}

			// The excluded artifacts must also stay untracked, not merely be
			// left out of this one commit.
			for _, absent := range wantAbsent {
				if tracked := strings.TrimSpace(mustGit(t, weft, "ls-files", "--", absent)); tracked != "" {
					t.Errorf("weft ls-files %q = %q; want it untracked", absent, tracked)
				}
			}
		})
	}
}
