// hub.go builds hubforge's real hub fixture, a warp/weft bare pair copied per scenario.
// The pair comes from a template built once per test binary, and the clone is wired through fabriccli.CloneAndWire.
// The result is a fully wired fabric hub, with junctions and a repo-wide fabric.yaml.
// The Hub type and its path accessors are what tests use to run a verb against that fixture.

package hubforge

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// The warp/weft bare pair is built exactly once per test binary.
// bareTemplateOnce guards that build, and warpBareTemplate and weftBareTemplate hold the paths it sets.
var (
	bareTemplateOnce sync.Once
	warpBareTemplate string
	weftBareTemplate string
)

// fixtureBuildMu guards fixtureBuilds, the number of hub builds in flight.
// The process environment carries gitkit.FixtureGitEnv while that count is above zero.
var (
	fixtureBuildMu sync.Mutex
	fixtureBuilds  int
)

// markFixtureBuild marks the process environment as building a fixture,
// so the git a build spawns through production code carries gitkit.FixtureGitEnv.
// Concurrent builds share the mark: the first starter sets it and the last release unsets it.
func markFixtureBuild() func() {
	fixtureBuildMu.Lock()
	defer fixtureBuildMu.Unlock()

	fixtureBuilds++
	if fixtureBuilds == 1 {
		if err := os.Setenv(gitkit.FixtureGitEnv, "1"); err != nil {
			panic(err)
		}
	}

	return func() {
		fixtureBuildMu.Lock()
		defer fixtureBuildMu.Unlock()

		fixtureBuilds--
		if fixtureBuilds == 0 {
			if err := os.Unsetenv(gitkit.FixtureGitEnv); err != nil {
				panic(err)
			}
		}
	}
}

// TestShortname is the repo shortname every hub built by this package is cloned with.
// The clone records it in .lym-shortname.
const TestShortname = "tst"

// buildBareTemplate builds the warp and weft bare remotes that copyBares copies, once per test binary.
// A scratch work repo builds the warp content, and git init --bare backs each remote.
// The remotes have fsmonitor, auto-maintenance and gc.auto disabled, and their hook samples stripped.
// Every git spawn panics on failure rather than swallowing it.
//
// The warp bare carries a root README plus backend/, nested/ and wts/some-task/ subdirectories in one commit.
// One template serves every anchor NewHub's callers request.
// Anchor resolution in fabricengine.CloneHub hard-errors unless the subpath is a directory in the warp's history.
// The check runs at clone time, so an anchor this commit does not carry can never be requested.
// A "."-anchored hub ignores the rest.
//
// Two constraints shape the template.
// First, git init --bare leaves HEAD on master while the pushed branch is main,
// so the warp bare's HEAD is re-pointed after the push.
// Second, the weft bare stays empty and is never pushed to.
// A push would trip CloneHub's bootstrap guard, `probe.WeftLooksLikeWeft`.
// That guard refuses a weft candidate whose history looks warp-shaped.
func buildBareTemplate() (warpBare, weftBare string) {
	defer markFixtureBuild()()

	bareTemplateOnce.Do(func() {
		tmpDir, err := lymtemp.MkdirTemp("hubforge-bare-*")
		if err != nil {
			panic(err)
		}

		scratch := filepath.Join(tmpDir, "scratch")
		if err := os.Mkdir(scratch, 0o755); err != nil {
			panic(err)
		}
		initScratchRepo(scratch)

		if err := os.WriteFile(filepath.Join(scratch, "README"), []byte("hubforge warp template\n"), 0o644); err != nil {
			panic(err)
		}
		backendDir := filepath.Join(scratch, "backend")
		if err := os.Mkdir(backendDir, 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(backendDir, "README"), []byte("hubforge backend anchor\n"), 0o644); err != nil {
			panic(err)
		}
		// nested and wts/some-task/ exist so that NewHub can anchor a hub at those paths.
		nestedDir := filepath.Join(scratch, "nested")
		if err := os.Mkdir(nestedDir, 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(nestedDir, "README"), []byte("hubforge nested anchor\n"), 0o644); err != nil {
			panic(err)
		}
		wtsTaskDir := filepath.Join(scratch, "wts", "some-task")
		if err := os.MkdirAll(wtsTaskDir, 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(wtsTaskDir, "README"), []byte("hubforge wts/some-task anchor\n"), 0o644); err != nil {
			panic(err)
		}
		commitAll(scratch, "hubforge: seed warp template")

		warp := filepath.Join(tmpDir, "warp-bare")
		initBareRepo(warp)
		mustGit(scratch, "remote", "add", "origin", warp)
		mustGit(scratch, "push", "origin", "main")
		mustGit(warp, "symbolic-ref", "HEAD", "refs/heads/main")

		weft := filepath.Join(tmpDir, "weft-bare")
		initBareRepo(weft)

		warpBareTemplate = warp
		weftBareTemplate = weft
	})

	return warpBareTemplate, weftBareTemplate
}

// copyBares copies the cached bare-pair template into tb's own tb.TempDir(), so each scenario pushes
// to its own copy and no two cells race.
// Copying is cheaper than building fresh bares per scenario.
// Building them fresh would run an init, a work repo, a commit and a push for every cell.
func copyBares(tb testing.TB) (warpBare, weftBare string) {
	tb.Helper()

	templateWarp, templateWeft := buildBareTemplate()
	container := tb.TempDir()

	warpBare = filepath.Join(container, "warp-bare")
	if err := copyDirRecursive(templateWarp, warpBare); err != nil {
		tb.Fatalf("copy warp bare template: %v", err)
	}
	weftBare = filepath.Join(container, "weft-bare")
	if err := copyDirRecursive(templateWeft, weftBare); err != nil {
		tb.Fatalf("copy weft bare template: %v", err)
	}
	return warpBare, weftBare
}

// Hub is a real, fully-wired fabric hub: a warp/weft pair cloned from its own copy of the bare template.
// The pair is cloned through fabriccli.CloneAndWire, ready for a verb under test to run against.
type Hub struct {
	// Path is the hub root, the <name>-LYMHUB container directory.
	Path string
	// Anchor is the resolved AnchorRel value, "." or "backend".
	Anchor string
	// Location is this hub's *lymcwd.Location, obtained from lymcwd.Resolve.
	// It is never constructed by hand.
	Location *lymcwd.Location
	// Topology is the fabricengine.Topology handle every pair-creating verb call goes through.
	Topology *fabricengine.Topology
	// CodeBare is this hub's own copy of the warp bare remote.
	CodeBare string
	// RecordsBare is this hub's own copy of the weft bare remote.
	RecordsBare string
	// WeftBase is the anchor-joined weft directory, copied verbatim from CloneResult.WeftBase.
	// fabricengine's CloneHub computes it as the weft worktree joined with the anchor.
	// It is the path a module loader reads config from at this hub's anchor.
	// It is not the same as PrimeRecords, which returns the un-anchored weft root:
	// the two coincide at the "." anchor and differ at any other anchor.
	// Config written to the un-anchored root at another anchor is a file no module loader reads,
	// and no error reports it.
	// hubforge.SeedConfig therefore writes to this path.
	WeftBase string
	// Container is the tb.TempDir() the hub was cloned into.
	Container string
	// Mutations is the mutation record fabriccli.CloneAndWire produced while building this hub.
	// It is copied verbatim from CloneResult.Mutated().
	// A test asserts the record's shape through it, without rebuilding the bare-template machinery.
	Mutations fabricengine.Mutations
}

// PrimeWorktree returns the path to this hub's prime warp worktree, the warp repository itself rather than a pair.
func (h *Hub) PrimeWorktree() string {
	return h.Location.WorktreePath()
}

// PrimeRecords returns the path to the weft sibling paired with the prime warp worktree.
// It is the un-anchored weft worktree root, not the anchor-joined path.
// To seed config, use h.WeftBase with hubforge.SeedConfig instead.
// At a non-"." anchor, config written to this path is a file no module loader reads, and no error reports it.
func (h *Hub) PrimeRecords() string {
	return fabricengine.RecordsWorktree(h.Location)
}

// BoardDir returns the path to this hub's Bolt checkout directory, the repo-wide weft:main checkout.
func (h *Hub) BoardDir() string {
	return fabricengine.BoardDir(h.Path)
}

// PairCodeWorktree returns the path to slug's warp worktree.
func (h *Hub) PairCodeWorktree(slug string) string {
	return fabricengine.WorktreePath(h.Location, slug)
}

// PairRecordsSibling returns the path to slug's weft sibling worktree.
func (h *Hub) PairRecordsSibling(slug string) string {
	return fabricengine.RecordsWorktreePath(h.Location, slug)
}

// PairLauncherDir returns the path to slug's mirrored launcher directory, at slug's own anchor.
// It panics when slug's recorded anchor is unusable, which no fixture produces.
func (h *Hub) PairLauncherDir(slug string) string {
	pair, err := lymcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		panic(fmt.Sprintf("hubforge: read the anchor of %q: %v", slug, err))
	}
	return fabricengine.LauncherDir(pair, slug)
}

// NewHub builds a real fabric hub at anchor ("." or "backend").
// It copies the cached bare template into tb's own temp dir, then drives fabriccli.CloneAndWire against the copies.
// It calls CloneAndWire rather than replicating the wiring sequence by hand.
// fabricengine.CloneHub alone yields a partial hub: warp and weft clones, a board, an anchor marker and a warp binding.
// It has no junctions and no repo-wide fabric.yaml.
// Without those, these path-ownership kinds are unreachable:
// `ownedWiredJunction`, `ownedDriftedWiredJunction` and `ownedUnderGeometryRoot`.
// The hub CloneAndWire returns has a clean weft prime worktree.
// Each per-worktree module's repo default is committed on the board,
// and the prime carries only the per-worktree config files the records repository already held.
// The clone's build stamp is removed, so a start verb driven in a test walks the hub as on a binary change.
// It calls tb.Fatalf on any error.
func NewHub(tb testing.TB, anchor string) *Hub {
	tb.Helper()

	defer markFixtureBuild()()

	warpBare, weftBare := copyBares(tb)
	container := tb.TempDir()

	hub, _ := cloneHub(tb, anchor, warpBare, weftBare, container)

	// Registered after copyBares and the container's tb.TempDir() call.
	// LIFO cleanup then runs junction removal before Go's tb.TempDir() cleanup removes the container.
	// A junction still wired when os.RemoveAll walks into it is a Win11 correctness bug, not a POSIX one.
	// The order must hold on every platform, even though only Windows can observe it directly.
	registerTeardown(tb, hub.Path)

	return hub
}

// CloneSecond clones a second hub from h's bares, at h's anchor, into a container of tb's own.
// It stands for a second machine working on the same remote.
// The second hub shares h's remotes and nothing else, so a push from either reaches the other through them.
// It calls tb.Fatalf on any error.
func CloneSecond(tb testing.TB, h *Hub) *Hub {
	tb.Helper()

	defer markFixtureBuild()()

	container := tb.TempDir()
	second, _ := cloneHub(tb, h.Anchor, h.CodeBare, h.RecordsBare, container)
	registerTeardown(tb, second.Path)
	return second
}

// cloneHub drives fabriccli.CloneAndWire at anchor against the given bares, into the existing container directory.
// It returns the Hub and the prime cwd its Location was resolved from.
// It calls tb.Fatalf on any error.
func cloneHub(tb testing.TB, anchor, warpBare, weftBare, container string) (*Hub, string) {
	tb.Helper()

	subpath := ""
	if anchor != "." {
		subpath = anchor
	}

	res, err := fabriccli.CloneAndWire(container, fabricengine.CloneOptions{
		WeftURL:   filepath.ToSlash(weftBare),
		WarpURL:   filepath.ToSlash(warpBare),
		Subpath:   subpath,
		Shortname: TestShortname,
	})
	if err != nil {
		tb.Fatalf("NewHub: CloneAndWire: %v", err)
	}

	// The clone's walk stamped the build.
	// Removing the stamp makes a start verb driven in a test walk the hub as on a binary change.
	// The seeded repo defaults stay committed.
	stamp := hubreconcile.Geometry{BoardDir: res.BoardDir}.StampPath()
	if err := os.Remove(stamp); err != nil && !os.IsNotExist(err) {
		tb.Fatalf("NewHub: remove the clone's build stamp %s: %v", stamp, err)
	}

	// The clone's walk hands the board's push to a detached sync, which a test binary never spawns.
	// So the hub-wide commit is pushed here, and the board starts clean against its upstream.
	if err := fabricengine.NewBolt(res.BoardDir).Push(fabricengine.SyncOptions{}); err != nil {
		tb.Fatalf("NewHub: push the clone's board commit: %v", err)
	}

	loc, err := lymcwd.Resolve(res.PrimeCwd)
	if err != nil {
		tb.Fatalf("NewHub: lymcwd.Resolve(%s): %v", res.PrimeCwd, err)
	}

	forgetCloneCorrespondence(tb, res.WeftBase, loc)

	return &Hub{
		Path:        res.HubPath,
		Anchor:      res.Anchor,
		Location:    loc,
		Topology:    fabricengine.NewTopology(fabricengine.Config{}),
		CodeBare:    warpBare,
		RecordsBare: weftBare,
		WeftBase:    res.WeftBase,
		Container:   container,
		Mutations:   res.Mutated(),
	}, res.PrimeCwd
}

// forgetCloneCorrespondence drops the Warp-SHA trailer that the clone's walk put on the weft prime's reconcile commit.
// It also drops the correspondence entry that came with the trailer.
// A fresh fixture hub starts with no warp-to-weft correspondence, so a scenario sees only what it records itself.
// The commit is local to the prime's branch, so rewording it leaves no remote diverged.
func forgetCloneCorrespondence(tb testing.TB, weftBase string, loc *lymcwd.Location) {
	tb.Helper()

	message := gitkit.Git(tb, weftBase, "log", "-1", "--format=%B")
	if !strings.Contains(message, fabricengine.WarpSHATrailerKey+":") {
		return
	}
	subject := gitkit.Git(tb, weftBase, "log", "-1", "--format=%s")
	gitkit.MustRun(tb, weftBase, "git", "commit", "--amend", "--allow-empty", "-m", subject)

	f, err := fabricengine.Open(loc)
	if err != nil {
		tb.Fatalf("NewHub: fabricengine.Open(%s): %v", weftBase, err)
	}
	if err := f.RebuildIndex(); err != nil {
		tb.Fatalf("NewHub: RebuildIndex after rewording the clone's reconcile commit: %v", err)
	}
}

// registerTeardown installs a tb.Cleanup that removes every junction under hubPath.
// The cleanup runs before Go's own tb.TempDir() cleanup removes the container directory the hub lives in.
// It walks the hub root with filepath.WalkDir, calling fslink.IsLink on every entry.
// fslink.Remove removes each link found.
//
// Discovery is slug-free by design: the walk never consults a slug list.
// The live-state matrix's pairs are created by the verb under test, and some are destroyed by it.
// Enumerating worktrees through fabric would need fabric to still work on a hub those tests corrupt.
//
// A missing hub directory, such as a worktree removed by hand mid-test, yields no entries from WalkDir.
// fslink.Remove is documented idempotent, so neither case needs a special branch.
//
// Errors from WalkDir, IsLink and Remove are reported with tb.Logf and never with tb.Fatalf or tb.Errorf:
// teardown must not fail a test that already passed.
func registerTeardown(tb testing.TB, hubPath string) {
	tb.Helper()

	tb.Cleanup(func() {
		walkErr := filepath.WalkDir(hubPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				tb.Logf("hubforge: teardown walk %s: %v", path, err)
				return nil
			}

			isLink, err := fslink.IsLink(path)
			if err != nil {
				tb.Logf("hubforge: teardown IsLink %s: %v", path, err)
				return nil
			}
			if !isLink {
				return nil
			}

			if err := fslink.Remove(path); err != nil {
				tb.Logf("hubforge: teardown Remove %s: %v", path, err)
			}
			// WalkDir reports a link as a non-directory entry, which it never descends into.
			// Returning nil here therefore keeps the walk out of it.
			// Returning filepath.SkipDir here would skip the remaining files in the containing directory.
			// That would leave the sibling junctions wired.
			return nil
		})
		if walkErr != nil {
			tb.Logf("hubforge: teardown walk %s: %v", hubPath, walkErr)
		}
	})
}

// AddPair drives h.Topology.Add for slug against h with zero AddOptions, fataling on error.
// Several verb tests need a pair to exist before the verb under test runs.
func AddPair(tb testing.TB, h *Hub, slug string) fabricengine.AddResult {
	tb.Helper()

	return AddPairWith(tb, h, slug, fabricengine.AddOptions{})
}

// AddPairWith drives h.Topology.Add for slug against h with opts, fataling on error.
// It then pushes the new pair in the foreground unless opts skip pushing.
// Add pushes nothing itself.
// The foreground push stands in for the detached push a production caller spawns.
// Fixtures therefore see both branches on the remotes.
func AddPairWith(tb testing.TB, h *Hub, slug string, opts fabricengine.AddOptions) fabricengine.AddResult {
	tb.Helper()

	defer markFixtureBuild()()

	res, err := h.Topology.Add(h.Location, slug, opts)
	if err != nil {
		tb.Fatalf("AddPairWith(%s): %v", slug, err)
	}
	if _, err := fabricengine.CoalescePushBothAt(res.Path, fabricengine.RecordsWorktreePath(h.Location, slug), fabricengine.SyncOptions{SkipGit: opts.SkipGit, SkipPush: opts.SkipPush, BackoffSleep: opts.BackoffSleep}); err != nil {
		tb.Fatalf("AddPairWith(%s): push the new pair: %v", slug, err)
	}
	return res
}

// OpenFabric opens the fabric handle on h's prime worktree, fataling on error.
func OpenFabric(tb testing.TB, h *Hub) *fabricengine.Fabric {
	tb.Helper()

	loc, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		tb.Fatalf("OpenFabric: ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}
	f, err := fabricengine.Open(loc)
	if err != nil {
		tb.Fatalf("OpenFabric: Open(%s): %v", h.PrimeWorktree(), err)
	}
	return f
}

// initScratchRepo initializes a git repository at dir on branch main.
// It disables fsmonitor and auto-maintenance, as gitkit.initRepo does.
func initScratchRepo(dir string) {
	mustGit(dir, "init", "-b", "main")
	mustGit(dir, "config", "user.email", "test@test.com")
	mustGit(dir, "config", "user.name", "Test")
	mustGit(dir, "config", "core.fsmonitor", "false")
	mustGit(dir, "config", "maintenance.auto", "false")
	mustGit(dir, "config", "gc.auto", "0")
	stripHookSamples(filepath.Join(dir, ".git", "hooks"))
}

// initBareRepo initializes a bare git repository at dir with fsmonitor and auto-maintenance disabled.
// It panics if dir already exists.
func initBareRepo(dir string) {
	if err := os.Mkdir(dir, 0o755); err != nil {
		panic(err)
	}
	mustGit(dir, "init", "--bare")
	mustGit(dir, "config", "core.fsmonitor", "false")
	mustGit(dir, "config", "maintenance.auto", "false")
	mustGit(dir, "config", "gc.auto", "0")
	stripHookSamples(filepath.Join(dir, "hooks"))
}

// commitAll stages every change in dir and creates a commit, panicking on failure.
func commitAll(dir, message string) {
	mustGit(dir, "add", ".")
	mustGit(dir, "commit", "-m", message)
}

// mustGit runs a git subcommand in dir and panics on non-zero exit.
// It panics rather than taking a testing.TB: the template builder runs once per test binary through sync.Once.
// Not every caller has a testing.TB in scope at that point.
// Every git spawn in this file goes through it, so no git failure is silently ignored.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitkit.FixtureGitEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}

// stripHookSamples removes every *.sample file from hooksDir on a best-effort basis and ignores errors.
func stripHookSamples(hooksDir string) {
	matches, err := filepath.Glob(filepath.Join(hooksDir, "*.sample"))
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// copyDirRecursive recursively copies the directory tree at src to dest and fails on any symlink in it.
// A template must stay plain files and directories, so a symlink is an error.
func copyDirRecursive(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, rel)

		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("copyDirRecursive: symlink not allowed in template: %s", path)
		}

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err := io.Copy(destFile, srcFile); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(destPath, info.Mode())
	})
}
