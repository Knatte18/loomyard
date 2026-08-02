//go:build integration

// weft_integration_test.go covers weftCommit's composed behavior against real
// git repositories -- the seams weft_test.go's guard-ordering assertions
// cannot reach. Two scenarios: Fabric.Commit's error-branch contract (a
// commit that lands but fails its correspondence record must still be
// reported as committed=true alongside the error, never swallowed into a
// false "no commit was made"), and the weft repo's .git/info/exclude
// actually keeping the right files out of every commit at every
// layout.RelPath depth, which only real git can decide.

package buildercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newHostWeftPair builds a hub directory holding a "host" git repo and its
// "host-weft" sibling git repo (each with one commit, so both have a HEAD),
// plus an uncommitted _lyx change in the weft worktree for weftCommit to
// stage, and returns the layout weftCommit resolves the pair from. RelPath is
// "." -- use newHostWeftPairAt for a nested layout.
func newHostWeftPair(t *testing.T) (*hubgeometry.Layout, string) {
	t.Helper()
	return newHostWeftPairAt(t, ".")
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml
// Fabric.Commit's classify step reads via RepoWiredNames (the `weft:main`
// base at hubgeometry.BoardDir(hub)) -- required since weftCommit moved onto
// Fabric.Commit, which resolves the wired name-set itself rather than
// trusting a caller-built pathspec. Mirrors
// commit_integration_test.go's seedFabricConfig in package fabricengine,
// duplicated here since that helper is unexported in a different package.
func seedRepoWideFabricConfig(t *testing.T, hub string) {
	t.Helper()

	boardDir := hubgeometry.BoardDir(hub)
	if err := os.MkdirAll(hubgeometry.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := hubgeometry.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// seedFabricAnchor records relPath as the .fabric-anchor marker under hub's
// board directory, so Fabric.Commit's own hubgeometry.ResolveWorktree(warpPath)
// call resolves l.RelPath to relPath instead of falling back to a
// cwd-derived "." -- Commit re-resolves geometry from f.warpPath itself
// rather than trusting the *hubgeometry.Layout weftCommit already holds, so
// a nested-RelPath fixture must record the anchor for real git to classify
// correctly.
func seedFabricAnchor(t *testing.T, hub, relPath string) {
	t.Helper()

	boardDir := hubgeometry.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, hubgeometry.FabricAnchorName)
	if err := os.WriteFile(anchorPath, []byte(relPath), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// newHostWeftPairAt is newHostWeftPair with an explicit layout.RelPath: the
// weft-side _lyx is seeded at <weft>/<relPath>/_lyx, mirroring the host's own
// repo-subpath geometry, and the returned layout's Cwd points at the matching
// host subdirectory. Alongside state.json it seeds the two machine-local
// artifacts the weft repo's .git/info/exclude must keep out (an advisory
// *.lock file and the pause flag) so a caller can assert on what the commit
// did and did not pick up. It also seeds a webster tree in the same _lyx -- the two round-loop
// modules share one -- carrying webster's own durable state.json plus its two
// machine-local artifacts (pause flag, rendered fork prompt), so a caller can
// assert that a BUILDER commit keeps webster's runtime state out while still
// carrying webster's durable state.
func newHostWeftPairAt(t *testing.T, relPath string) (*hubgeometry.Layout, string) {
	t.Helper()

	hub := t.TempDir()
	host := filepath.Join(hub, "host")
	weft := filepath.Join(hub, "host-weft")
	for _, dir := range []string{host, weft} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		mustGit(t, dir, "init")
		mustGit(t, dir, "config", "user.name", "Test User")
		mustGit(t, dir, "config", "user.email", "test@example.com")
	}
	commitFile(t, host, "base.txt", "base", "host base commit")
	commitFile(t, weft, "base.txt", "base", "weft base commit")

	// Uncommitted changes under the builder pathspec, so CommitWeft has
	// something real to commit -- plus the two artifacts the exclusion set
	// must keep out of that commit.
	builderDir := filepath.Join(weft, relPath, hubgeometry.LyxDirName, "builder")
	if err := os.MkdirAll(builderDir, 0o755); err != nil {
		t.Fatalf("mkdir weft _lyx: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":                "{}",
		"run.lock":                  "lock",
		builderengine.PauseFlagName: "paused",
	} {
		if err := os.WriteFile(filepath.Join(builderDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write weft %s: %v", name, err)
		}
	}

	// Webster's own tree, in the same shared _lyx: its durable state must
	// still ride a builder commit, its machine-local artifacts must not.
	websterDir := filepath.Join(weft, relPath, hubgeometry.LyxDirName, "webster")
	if err := os.MkdirAll(filepath.Join(websterDir, "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir weft webster dir: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":                     "{}",
		websterengine.PauseFlagName:      "paused",
		filepath.Join("prompts", "1.md"): "rendered fork prompt",
	} {
		if err := os.WriteFile(filepath.Join(websterDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write weft webster %s: %v", name, err)
		}
	}

	seedRepoWideFabricConfig(t, hub)
	seedFabricAnchor(t, hub, filepath.ToSlash(relPath))

	return &hubgeometry.Layout{
		Hub:          hub,
		WorktreeRoot: host,
		Cwd:          filepath.Join(host, relPath),
		RelPath:      relPath,
	}, weft
}

// TestWeftCommit_ReportsCommittedWhenCorrespondenceRecordFails proves the
// Fabric.Commit error branch passes committed through instead of forcing it
// to false: with a directory squatting on the correspondence index path, the
// weft commit itself lands but RecordCorrespondence fails, and weftCommit
// must report (true, err) -- the commit is real, and Fabric.Commit's
// contract says the caller gets to know that alongside the error.
func TestWeftCommit_ReportsCommittedWhenCorrespondenceRecordFails(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")
	layout, weft := newHostWeftPair(t)

	// A directory where RecordCorrespondence expects its index file makes
	// the record step fail after the commit has already landed.
	if err := os.MkdirAll(filepath.Join(weft, ".git", "fabric-corrindex.json"), 0o755); err != nil {
		t.Fatalf("squat corrindex path: %v", err)
	}

	committed, err := weftCommit(layout, "corr-fail probe")

	if err == nil {
		t.Fatal("weftCommit() error = nil; want the RecordCorrespondence failure propagated")
	}
	if !committed {
		t.Error("weftCommit() committed = false; want true, the commit landed before the record step failed")
	}

	// The commit must genuinely exist with the builder message stem -- the
	// committed=true report above is about this commit, not a phantom.
	subject := strings.TrimSpace(mustGit(t, weft, "log", "-1", "--format=%s"))
	if subject != "builder: corr-fail probe" {
		t.Errorf("weft HEAD subject = %q; want %q", subject, "builder: corr-fail probe")
	}
}

// TestWeftCommit_CommitsAtEveryRelPathDepth proves every machine-local
// transient (locks, both round-loop modules' pause flags, webster's
// rendered fork prompts) stays uncommitted by REAL git at every
// layout.RelPath depth, not merely absent from some in-memory pathspec
// shape. Exclusion is now enforced solely by the weft repo's
// .git/info/exclude (seeded by fabricengine.seedWeftArtifactExcludes,
// reached through Fabric.Commit's ensureWeftLockDir), not by any
// ":(exclude)" pathspec builderCLI builds itself -- weftCommit passes only
// the positive scoped _lyx pathspec. The nested case is the regression
// guard for the pre-fix, unanchored ":(exclude)*.lock" spelling this
// migration retired: that spelling's one-star pathspec false-positive-matched
// the intermediate directories leading to a multi-segment positive pathspec
// and pruned the entire subtree, so `git add` staged nothing and weftCommit
// returned (false, nil) -- a completely silent no-op. It is also the
// cross-module regression guard: a builder commit must hold back WEBSTER's
// pause flag and rendered prompts too, since both round-loop modules share
// one _lyx -- committing them pins them in weft HEAD, where webster can
// never stage their deletion (its own exclusion hides them from `git add`),
// so every other machine's weft pull materializes a pause request nobody
// made. Each excluded artifact is asserted both absent from the commit AND
// still untracked via `git ls-files` -- proving the exclude file, not merely
// an already-tracked file happening to be omitted from this one commit.
// WEFT_SKIP_PUSH is set because the scratch weft repo has no remote; the
// commit half is what is under test.
func TestWeftCommit_CommitsAtEveryRelPathDepth(t *testing.T) {
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
			layout, weft := newHostWeftPairAt(t, tt.relPath)

			committed, err := weftCommit(layout, "depth probe")
			if err != nil {
				t.Fatalf("weftCommit() error = %v; want nil", err)
			}
			if !committed {
				t.Fatalf("weftCommit() committed = false; want true -- the pathspec staged nothing at RelPath %q, so the weft commit was a silent no-op", tt.relPath)
			}

			// git always reports commit contents with forward slashes,
			// regardless of the OS-native separators the layout carries.
			base := hubgeometry.LyxDirName
			if tt.relPath != "." {
				base = filepath.ToSlash(tt.relPath) + "/" + hubgeometry.LyxDirName
			}
			committedFiles := strings.Fields(mustGit(t, weft, "show", "--name-only", "--format=", "HEAD"))

			// Webster's durable state.json rides a builder commit (the two
			// modules share one _lyx); only the machine-local artifacts of
			// EITHER module are held back.
			wantPresent := []string{
				base + "/builder/state.json",
				base + "/webster/state.json",
			}
			wantAbsent := []string{
				base + "/builder/run.lock",
				base + "/builder/" + builderengine.PauseFlagName,
				base + "/webster/" + websterengine.PauseFlagName,
				base + "/webster/prompts/1.md",
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
