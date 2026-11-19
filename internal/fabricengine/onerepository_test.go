// onerepository_test.go covers the layout read and the records geometry it implies, over small hermetic repositories.
// It is an internal-package test because oneRepositoryAt and recordsBranchIn are unexported.
// The tests rely on the package TestMain in testmain_test.go, which runs gitkit.HermeticGitEnv().

package fabricengine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestOneRepositoryAt_NoWorktreeOnDiskReadsAsTwoRepositoriesQuietly pins the layout read of a path with no worktree.
// Such a path, a project or a task not started, answers false and logs nothing.
// It swaps the process-global log output, so it does not run in parallel.
func TestOneRepositoryAt_NoWorktreeOnDiskReadsAsTwoRepositoriesQuietly(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name string
		// path returns the path to read inside dir.
		path func(t *testing.T, dir string) string
	}{
		{"MissingPath", func(t *testing.T, dir string) string { return filepath.Join(dir, "missing") }},
		{"DirectoryWithoutGit", func(t *testing.T, dir string) string { return dir }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			logger.SetOutput(&logged)
			t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
			path := tt.path(t, t.TempDir())

			if oneRepositoryAt(path) {
				t.Errorf("oneRepositoryAt(%s) = true; want false", path)
			}
			if got := logged.String(); got != "" {
				t.Errorf("oneRepositoryAt(%s) logged %q; want nothing", path, got)
			}
		})
	}
}

// geometryRepository creates a repository named name in hub, on main with one commit, and returns its path.
func geometryRepository(t *testing.T, hub, name string) string {
	t.Helper()

	dir := filepath.Join(hub, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitkit.Git(t, dir, "init", "-q", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, "README", "code", "init")
	return dir
}

func TestRecordsGeometry_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "slug"
	const branch = "task"
	// geometry is what the records geometry functions answer for one location.
	type geometry struct {
		oneRepository       bool
		recordsWorktree     string
		recordsWorktreePath string
		recordsRepoRoot     string
		lockDir             string
		recordsBranch       string
	}

	tests := []struct {
		name string
		// build makes the repositories in hub.
		// It returns the location to ask about, the path whose layout is read and the geometry wanted.
		build func(t *testing.T, hub string) (*lymcwd.Location, string, geometry)
	}{
		{"OneRepository", func(t *testing.T, hub string) (*lymcwd.Location, string, geometry) {
			prime := geometryRepository(t, hub, "prime")
			gitkit.CommitFile(t, prime, lymcwd.AnchorFileName, ".\n", "record the anchor")
			gitkit.Git(t, prime, "branch", BoltBranchName)
			gitkit.Git(t, prime, "worktree", "add", "-q", "-b", branch, filepath.Join(hub, slug))
			worktree := filepath.Join(hub, slug)
			return &lymcwd.Location{HubPath: hub, WorktreeName: slug, AnchorRel: "."}, worktree, geometry{
				oneRepository:       true,
				recordsWorktree:     worktree,
				recordsWorktreePath: worktree,
				recordsRepoRoot:     prime,
				lockDir:             filepath.Join(prime, ".git", "worktrees", slug, recordsLockDirName),
				recordsBranch:       branch,
			}
		}},
		{"OneRepositoryPrimeOnABranchWithoutTheMarker", func(t *testing.T, hub string) (*lymcwd.Location, string, geometry) {
			prime := geometryRepository(t, hub, "prime")
			gitkit.Git(t, prime, "checkout", "-q", "-b", BoltBranchName)
			gitkit.CommitFile(t, prime, lymcwd.AnchorFileName, ".\n", "record the anchor")
			gitkit.Git(t, prime, "checkout", "-q", "main")
			return &lymcwd.Location{HubPath: hub, WorktreeName: "prime", AnchorRel: "."}, prime, geometry{
				oneRepository:       true,
				recordsWorktree:     prime,
				recordsWorktreePath: prime,
				recordsRepoRoot:     prime,
				lockDir:             filepath.Join(prime, ".git", recordsLockDirName),
				recordsBranch:       branch,
			}
		}},
		{"OneRepositoryBoltCheckout", func(t *testing.T, hub string) (*lymcwd.Location, string, geometry) {
			prime := geometryRepository(t, hub, "prime")
			gitkit.Git(t, prime, "checkout", "-q", "-b", BoltBranchName)
			gitkit.CommitFile(t, prime, lymcwd.AnchorFileName, ".\n", "record the anchor")
			gitkit.Git(t, prime, "checkout", "-q", "main")
			bolt := filepath.Join(hub, BoltDirName)
			gitkit.Git(t, prime, "worktree", "add", "-q", bolt, BoltBranchName)
			return &lymcwd.Location{HubPath: hub, WorktreeName: BoltDirName, AnchorRel: "."}, bolt, geometry{
				oneRepository:       true,
				recordsWorktree:     bolt,
				recordsWorktreePath: bolt,
				recordsRepoRoot:     prime,
				lockDir:             filepath.Join(prime, ".git", "worktrees", BoltDirName, recordsLockDirName),
				recordsBranch:       branch,
			}
		}},
		{"TwoRepositoryCodeWorktree", func(t *testing.T, hub string) (*lymcwd.Location, string, geometry) {
			prime := geometryRepository(t, hub, "prime")
			worktree := filepath.Join(hub, slug)
			gitkit.Git(t, prime, "worktree", "add", "-q", "-b", branch, worktree)
			sibling := weftname.SiblingPath(hub, slug)
			return &lymcwd.Location{HubPath: hub, WorktreeName: slug, AnchorRel: "."}, worktree, geometry{
				recordsWorktree:     sibling,
				recordsWorktreePath: sibling,
				recordsRepoRoot:     weftname.SiblingPath(hub, "prime"),
				lockDir:             filepath.Join(sibling, weftLockDirName),
				recordsBranch:       RecordsBranchName(branch),
			}
		}},
		{"TwoRepositorySiblingCarryingAMarker", func(t *testing.T, hub string) (*lymcwd.Location, string, geometry) {
			geometryRepository(t, hub, "prime")
			sibling := geometryRepository(t, hub, "prime"+weftname.Suffix)
			gitkit.CommitFile(t, sibling, lymcwd.AnchorFileName, ".\n", "record the anchor")
			return &lymcwd.Location{HubPath: hub, WorktreeName: "prime", AnchorRel: "."}, sibling, geometry{
				recordsWorktree:     sibling,
				recordsWorktreePath: sibling,
				recordsRepoRoot:     sibling,
				lockDir:             filepath.Join(sibling, weftLockDirName),
				recordsBranch:       RecordsBranchName(branch),
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve the hub directory: %v", err)
			}
			location, probe, want := tt.build(t, hub)

			recordsRepoRoot, err := RecordsRepoRoot(location)
			if err != nil {
				t.Fatalf("RecordsRepoRoot() error = %v; want nil", err)
			}
			got := geometry{
				oneRepository:       oneRepositoryAt(probe),
				recordsWorktree:     RecordsWorktree(location),
				recordsWorktreePath: RecordsWorktreePath(location, location.WorktreeName),
				recordsRepoRoot:     recordsRepoRoot,
				lockDir:             RecordsLockDirPath(RecordsWorktree(location)),
				recordsBranch:       recordsBranchIn(location, branch),
			}
			if got != want {
				t.Errorf("records geometry = %+v; want %+v", got, want)
			}
		})
	}
}
