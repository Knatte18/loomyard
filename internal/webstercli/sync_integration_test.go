// sync_integration_test.go covers fabricSync's composed behavior against real git repositories.
// It checks two things.
// First, Fabric.Commit's error branch: a commit that lands but fails its correspondence record stays committed=true.
// The error comes back alongside that, never as a false "no commit was made".
// Second, machine-local artifacts under ".lym" stay out of every commit at every layout.AnchorRel depth.
// That holds because they live outside the committed "_lym" pathspec, which only real git can decide.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newPairFixture builds a hub with a code repo and its records sibling.
// It returns the layout and the sibling's records path.
func newPairFixture(t *testing.T) (*lymcwd.Location, string) {
	t.Helper()
	return newPairFixtureAt(t, ".")
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml.
func seedRepoWideFabricConfig(t *testing.T, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lym\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// seedFabricAnchor records relPath as the .lym-anchor marker under hub's board directory.
// Fabric.Commit re-resolves its geometry from the code path, not from the *lymcwd.Location fabricSync holds.
// Without the marker, that resolution falls back to a cwd-derived ".".
// A nested AnchorRel therefore has to be recorded here for real git to classify it correctly.
func seedFabricAnchor(t *testing.T, hub, relPath string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lymcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(relPath), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// newPairFixtureAt is newPairFixture with an explicit layout.AnchorRel.
// The records-side _lym is seeded at <records>/<relPath>/_lym, mirroring the code side's repo-subpath geometry.
// The returned layout's AnchorPath() points at the matching code subdirectory.
// Next to state.json it seeds three machine-local artifacts under ".lym" at the mirrored subpath.
// They are an advisory *.lock file, the pause flag, and a rendered fork prompt.
// They sit outside the committed "_lym" pathspec, so a caller can assert what a commit did and did not pick up.
// It also seeds a sibling loom module in the same geometry: durable status.json under "_lym", run lock under ".lym".
// A caller can then assert that a webster commit holds back the loom run lock while still carrying status.json.
func newPairFixtureAt(t *testing.T, relPath string) (*lymcwd.Location, string) {
	t.Helper()

	hub := t.TempDir()
	layout := &lymcwd.Location{
		HubPath:      hub,
		WorktreeName: "pair",
		AnchorRel:    relPath,
	}
	code := layout.WorktreePath()
	records := fabricengine.RecordsWorktree(layout)
	for _, dir := range []string{code, records} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		gitkit.Git(t, dir, "init")
		gitkit.Git(t, dir, "config", "user.name", "Test User")
		gitkit.Git(t, dir, "config", "user.email", "test@example.com")
	}
	// Resolving the prime runs git in the anchor directory, which must exist in the code worktree.
	if err := os.MkdirAll(filepath.Join(code, relPath), 0o755); err != nil {
		t.Fatalf("mkdir anchor in code worktree: %v", err)
	}
	gitkit.CommitFile(t, code, "base.txt", "base", "code base commit")
	gitkit.CommitFile(t, records, "base.txt", "base", "records base commit")

	// Uncommitted changes under the webster pathspec give the records commit something to commit.
	websterDir := filepath.Join(records, relPath, lymdirs.LymDirName, "webster")
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("mkdir records _lym: %v", err)
	}
	if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write records state.json: %v", err)
	}

	// The three machine-local artifacts, mirrored under ".lym" -- outside the committed pathspec.
	websterScratchDir := filepath.Join(records, relPath, lymdirs.DotLymDirName, "webster")
	if err := os.MkdirAll(filepath.Join(websterScratchDir, "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir records .lym: %v", err)
	}
	for name, content := range map[string]string{
		"mutate.lock":                    "lock",
		"pause":                          "paused",
		filepath.Join("prompts", "1.md"): "rendered fork prompt",
	} {
		if err := os.WriteFile(filepath.Join(websterScratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write records %s: %v", name, err)
		}
	}

	// The sibling loom module's tree sits in the same shared geometry.
	// Its durable state must still ride a webster commit, but its run lock must not.
	loomDir := filepath.Join(records, relPath, lymdirs.LymDirName, "loom")
	if err := os.MkdirAll(loomDir, 0o755); err != nil {
		t.Fatalf("mkdir records loom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loomDir, "status.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write records loom status.json: %v", err)
	}

	loomScratchDir := filepath.Join(records, relPath, lymdirs.DotLymDirName, "loom")
	if err := os.MkdirAll(loomScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir records loom scratch dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loomScratchDir, "run.lock"), []byte("locked"), 0o644); err != nil {
		t.Fatalf("write records tapestry run lock: %v", err)
	}

	seedRepoWideFabricConfig(t, hub)
	seedFabricAnchor(t, hub, filepath.ToSlash(relPath))

	return layout, records
}

// TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails proves Fabric.Commit's error branch.
// It passes committed through instead of forcing it to false.
// A directory on the correspondence index path lets the records commit land, then RecordCorrespondence fails.
// fabricSync must report (true, err): the commit is real.
// Fabric.Commit's contract is that the caller learns that alongside the error.
func TestFabricSync_ReportsCommittedWhenCorrespondenceRecordFails(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "")
	t.Setenv("FABRIC_SKIP_PUSH", "")
	layout, records := newPairFixture(t)

	// A directory where RecordCorrespondence expects its index file makes the record step fail after the commit has landed.
	if err := os.MkdirAll(filepath.Join(records, ".git", "fabric-corrindex.json"), 0o755); err != nil {
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

	// The commit must genuinely exist with the webster message stem.
	// The committed=true report above is about this commit, not a phantom.
	subject := strings.TrimSpace(gitkit.Git(t, records, "log", "-1", "--format=%s"))
	if subject != "webster: corr-fail probe" {
		t.Errorf("records HEAD subject = %q; want %q", subject, "webster: corr-fail probe")
	}
}

// TestFabricSync_CommitsAtEveryRelPathDepth proves the commit at every layout.AnchorRel depth:
// no machine-local transient reaches the commit: locks, the loom run lock, or webster's fork prompts.
// Exclusion comes from placement:
// each artifact lives under ".lym" at the mirrored subpath, outside the committed "_lym" pathspec.
// It also guards across modules: a webster commit must hold back the sibling loom module's run lock too.
// Each excluded artifact is asserted both absent from the commit and still untracked via `git ls-files`.
// Untracked status proves it never reached the pathspec, not merely that this one commit omitted it.
// FABRIC_SKIP_PUSH is set because the scratch records repo has no remote; the commit half is what is under test.
func TestFabricSync_CommitsAtEveryRelPathDepth(t *testing.T) {
	tiergate.Require(t, 1)
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
			t.Setenv("FABRIC_SKIP_GIT", "")
			t.Setenv("FABRIC_SKIP_PUSH", "1")
			layout, records := newPairFixtureAt(t, tt.relPath)
			open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }

			committed, err := fabricSync(open, layout.AnchorRel, "depth probe")
			if err != nil {
				t.Fatalf("fabricSync() error = %v; want nil", err)
			}
			if !committed {
				t.Fatalf("fabricSync() committed = false; want true -- the pathspec staged nothing at RelPath %q, so the fabric sync was a silent no-op", tt.relPath)
			}

			// git reports commit contents with forward slashes, regardless of the OS-native separators the layout carries.
			base := lymdirs.LymDirName
			scratchBase := lymdirs.DotLymDirName
			if tt.relPath != "." {
				prefix := filepath.ToSlash(tt.relPath) + "/"
				base = prefix + lymdirs.LymDirName
				scratchBase = prefix + lymdirs.DotLymDirName
			}
			committedFiles := strings.Fields(gitkit.Git(t, records, "show", "--name-only", "--format=", "HEAD"))

			// Loom's durable status.json rides a webster commit, since the two modules share one _lym.
			// Only the machine-local artifacts of either module are held back.
			wantPresent := []string{
				base + "/webster/state.json",
				base + "/loom/status.json",
			}
			wantAbsent := []string{
				scratchBase + "/webster/mutate.lock",
				scratchBase + "/webster/pause",
				scratchBase + "/webster/prompts/1.md",
				scratchBase + "/loom/run.lock",
			}
			for _, present := range wantPresent {
				if !containsString(committedFiles, present) {
					t.Errorf("records commit at RelPath %q = %v; want it to contain %q", tt.relPath, committedFiles, present)
				}
			}
			for _, absent := range wantAbsent {
				if containsString(committedFiles, absent) {
					t.Errorf("records commit at RelPath %q = %v; want it to EXCLUDE the machine-local %q", tt.relPath, committedFiles, absent)
				}
			}

			// The excluded artifacts must also stay untracked, not merely be left out of this one commit.
			for _, absent := range wantAbsent {
				if tracked := strings.TrimSpace(gitkit.Git(t, records, "ls-files", "--", absent)); tracked != "" {
					t.Errorf("records ls-files %q = %q; want it untracked", absent, tracked)
				}
			}
		})
	}
}
