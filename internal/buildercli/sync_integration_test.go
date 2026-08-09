//go:build integration

// sync_integration_test.go covers fabricSync's composed behavior against real
// git repositories -- the seams sync_test.go's guard-ordering assertions
// cannot reach. Two scenarios: Fabric.Commit's error-branch contract (a
// commit that lands but fails its correspondence record must still be
// reported as committed=true alongside the error, never swallowed into a
// false "no commit was made"), and machine-local artifacts under ".lyx"
// actually staying out of every commit at every layout.AnchorRel depth
// simply by living outside the committed "_lyx" pathspec, which only real
// git can decide.

package buildercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newWarpWeftPair builds a hub with warp and warp-weft git repos plus an
// uncommitted _lyx change in weft, returning the layout and weft path.
// AnchorRel is "."; use newWarpWeftPairAt for nested layouts.
func newWarpWeftPair(t *testing.T) (*lyxcwd.Location, string) {
	t.Helper()
	return newWarpWeftPairAt(t, ".")
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml
// Fabric.Commit's classify step reads via RepoWiredNames (the `weft:main`
// base at fabricengine.BoardDir(hub)) -- required since fabricSync moved onto
// Fabric.Commit, which resolves the wired name-set itself rather than
// trusting a caller-built pathspec. Mirrors
// commit_integration_test.go's seedFabricConfig in package fabricengine,
// duplicated here since that helper is unexported in a different package.
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

// seedFabricAnchor records relPath as the .lyx-anchor marker so
// Fabric.Commit resolves the correct geometry for nested layouts.
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

// newWarpWeftPairAt is newWarpWeftPair with an explicit AnchorRel: the
// weft-side _lyx is mirrored at <weft>/<relPath>/_lyx, and seeds builder
// and webster state with machine-local artifacts so callers can assert
// what the commit includes and excludes.
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

	// Uncommitted changes for CommitWeft to stage: durable state under "_lyx".
	builderDir := filepath.Join(weft, relPath, lyxdirs.LyxDirName, "builder")
	if err := os.MkdirAll(builderDir, 0o755); err != nil {
		t.Fatalf("mkdir weft _lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(builderDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write weft state.json: %v", err)
	}

	// Machine-local artifacts under ".lyx" -- the mirrored scratch subpath, outside the committed
	// "_lyx" pathspec entirely.
	builderScratchDir := filepath.Join(weft, relPath, lyxdirs.DotLyxDirName, "builder")
	if err := os.MkdirAll(builderScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir weft .lyx: %v", err)
	}
	for name, content := range map[string]string{
		"run.lock":                  "lock",
		builderengine.PauseFlagName: "paused",
	} {
		if err := os.WriteFile(filepath.Join(builderScratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write weft %s: %v", name, err)
		}
	}

	// Webster's tree: durable state rides a builder commit, not machine-local.
	websterDir := filepath.Join(weft, relPath, lyxdirs.LyxDirName, "webster")
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("mkdir weft webster dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write weft webster state.json: %v", err)
	}

	// Webster's machine-local artifacts, mirrored under ".lyx" the same way.
	websterScratchDir := filepath.Join(weft, relPath, lyxdirs.DotLyxDirName, "webster")
	if err := os.MkdirAll(filepath.Join(websterScratchDir, "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir weft webster scratch dir: %v", err)
	}
	for name, content := range map[string]string{
		websterengine.PauseFlagName:      "paused",
		filepath.Join("prompts", "1.md"): "rendered fork prompt",
	} {
		if err := os.WriteFile(filepath.Join(websterScratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write weft webster %s: %v", name, err)
		}
	}

	seedRepoWideFabricConfig(t, hub)
	seedFabricAnchor(t, hub, filepath.ToSlash(relPath))

	return &lyxcwd.Location{
		HubPath:      hub,
		WorktreeName: filepath.Base(warp),
		AnchorRel:    relPath,
	}, weft
}

// TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails proves that when
// RecordCorrespondence fails after the commit lands, fabricSync reports (true, err), not (false,
// err).
func TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")
	layout, weft := newWarpWeftPair(t)

	// A directory where RecordCorrespondence expects its index file makes
	// the record step fail after the commit has already landed.
	if err := os.MkdirAll(filepath.Join(weft, ".git", "fabric-corrindex.json"), 0o755); err != nil {
		t.Fatalf("squat corrindex path: %v", err)
	}

	committed, err := fabricSync(layout, "corr-fail probe")

	if err == nil {
		t.Fatal("fabricSync() error = nil; want the RecordCorrespondence failure propagated")
	}
	if !committed {
		t.Error("fabricSync() committed = false; want true, the commit landed before the record step failed")
	}

	// The commit must genuinely exist with the builder message stem -- the
	// committed=true report above is about this commit, not a phantom.
	subject := strings.TrimSpace(mustGit(t, weft, "log", "-1", "--format=%s"))
	if subject != "builder: corr-fail probe" {
		t.Errorf("weft HEAD subject = %q; want %q", subject, "builder: corr-fail probe")
	}
}

// TestFabricSync_CommitsAtEveryRelPathDepth proves that machine-local transients (locks, pause
// flags, rendered prompts) stay excluded from REAL git commits at every AnchorRel depth simply by
// living under ".lyx", outside the committed "_lyx" pathspec.
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

			committed, err := fabricSync(layout, "depth probe")
			if err != nil {
				t.Fatalf("fabricSync() error = %v; want nil", err)
			}
			if !committed {
				t.Fatalf("fabricSync() committed = false; want true -- the pathspec staged nothing at RelPath %q, so the fabric sync was a silent no-op", tt.relPath)
			}

			// git reports paths with forward slashes regardless of OS.
			base := lyxdirs.LyxDirName
			scratchBase := lyxdirs.DotLyxDirName
			if tt.relPath != "." {
				prefix := filepath.ToSlash(tt.relPath) + "/"
				base = prefix + lyxdirs.LyxDirName
				scratchBase = prefix + lyxdirs.DotLyxDirName
			}
			committedFiles := strings.Fields(mustGit(t, weft, "show", "--name-only", "--format=", "HEAD"))

			// Webster's durable state rides the commit; machine-local artifacts don't.
			wantPresent := []string{
				base + "/builder/state.json",
				base + "/webster/state.json",
			}
			wantAbsent := []string{
				scratchBase + "/builder/run.lock",
				scratchBase + "/builder/" + builderengine.PauseFlagName,
				scratchBase + "/webster/" + websterengine.PauseFlagName,
				scratchBase + "/webster/prompts/1.md",
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

			// Verify excluded artifacts stay untracked, not just omitted.
			for _, absent := range wantAbsent {
				if tracked := strings.TrimSpace(mustGit(t, weft, "ls-files", "--", absent)); tracked != "" {
					t.Errorf("weft ls-files %q = %q; want it untracked", absent, tracked)
				}
			}
		})
	}
}
