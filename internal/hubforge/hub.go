// hub.go builds hubforge's real hub fixture: a sync.Once-cached warp/weft bare-repo template built
// once per test binary, copied fresh per scenario via copyBares, and driven through
// fabriccli.CloneAndWire by NewHub into a real, fully-wired fabric hub — junctions, repo-wide
// fabric.yaml, and all — that a hostile-state cell can run a verb against.
// The Hub type and its geometry accessors are the external interface batches 3-7 consume.

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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// bareTemplateOnce guards buildBareTemplate so the warp/weft bare pair is built exactly once per test
// binary; warpBareTemplate and weftBareTemplate hold the resulting paths.
var (
	bareTemplateOnce sync.Once
	warpBareTemplate string
	weftBareTemplate string
)

// buildBareTemplate builds, once per test binary, the pushed-to warp bare and the genuinely empty
// weft bare that NewHub's factory clones copies of.
// It mirrors gitkit's buildWarpHub/buildWeftPrime pattern: a scratch work repo builds the content,
// git init --bare backs each remote, fsmonitor/auto-maintenance/gc.auto are disabled and hook samples
// stripped exactly as gitkit.initRepo/initBareRemote do, and every git spawn panics on failure rather
// than swallowing it.
//
// The warp bare carries a root README plus backend/, nested/ and wts/some-task/ subdirectories in
// the same commit, so one template serves every anchor NewHub's callers request — an anchor not
// carried here can never be requested from NewHub, since CloneHub's anchor-resolution step
// hard-errors unless the requested Subpath already exists as a directory in the warp's committed
// history at clone time.
// A "."-anchored hub simply ignores the rest.
// Two gotchas are encoded here and nowhere else.
// First, `git init --bare` leaves HEAD on master while the branch just pushed is main, so the warp
// bare needs its HEAD re-pointed after the push — gitkit's own bares never hit this, because
// initBareRemote adds a bare as origin but never pushes to it, so the mismatch is never observable
// there.
// Second, the weft bare must stay genuinely empty and is never pushed to: pushing anything to it would
// trip CloneHub's bootstrap guard at clone.go:172 (`!probe.WeftLooksLikeWeft`), which refuses a weft
// candidate whose history looks warp-shaped — the warp bare, by contrast, must have content pushed.
func buildBareTemplate() (warpBare, weftBare string) {
	bareTemplateOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "hubforge-bare-*")
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
		// nested and wts/some-task/ exist purely so a caller can anchor a hub there:
		// fabricengine.CloneHub's anchor-resolution step hard-errors unless the requested Subpath
		// already exists as a directory in the warp's committed history at clone time, so any anchor
		// this template does not carry here can never be requested from NewHub, no matter what a
		// caller does after NewHub returns.
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
		// Gotcha 1: re-point HEAD onto the branch actually pushed.
		mustGit(warp, "symbolic-ref", "HEAD", "refs/heads/main")

		// Gotcha 2: left empty on purpose, never pushed to.
		weft := filepath.Join(tmpDir, "weft-bare")
		initBareRepo(weft)

		warpBareTemplate = warp
		weftBareTemplate = weft
	})

	return warpBareTemplate, weftBareTemplate
}

// copyBares copies the cached bare-pair template into tb's own tb.TempDir(), so each scenario pushes
// to its own copy and cells never race.
// Copying is the deliberate choice over building fresh bares per scenario, which would cost a full
// git init --bare plus work repo plus commit plus push per cell.
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

// Hub is a real, fully-wired fabric hub built by NewHub: a warp/weft pair cloned from a hub-owned copy
// of the bare template and driven through fabriccli.CloneAndWire, ready for a verb under test to run
// against.
type Hub struct {
	// Path is the hub root, the <name>-HUB container directory.
	Path string
	// Anchor is the resolved AnchorRel value, "." or "backend".
	Anchor string
	// Location is this hub's *lyxcwd.Location, obtained from lyxcwd.Resolve — never constructed by
	// hand, per the Cwd Resolution Invariant.
	Location *lyxcwd.Location
	// Topology is the fabricengine.Topology handle every pair-creating verb call goes through.
	Topology *fabricengine.Topology
	// WarpBare is this hub's own copy of the warp bare remote.
	WarpBare string
	// WeftBare is this hub's own copy of the weft bare remote.
	WeftBare string
	// WeftBase is the anchor-joined weft directory, populated verbatim from CloneResult.WeftBase —
	// fabricengine's CloneHub computes it as filepath.Join(WeftWorktree(l), l.AnchorRel), never
	// re-derived here.
	// It is deliberately not the same thing as PrimeWeft(), which returns the un-anchored weft
	// worktree root: the two coincide at the "." anchor and diverge at "backend", where writing
	// config to the un-anchored path produces a file no module loader ever reads, with no error at
	// all.
	// hubforge.SeedConfig writes here for exactly that reason.
	WeftBase string
	// Container is the tb.TempDir() the hub was cloned into.
	Container string
	// Mutations is the mutation record fabriccli.CloneAndWire produced while building this hub,
	// populated verbatim from CloneResult.Mutated(), never re-derived — exposed so a test can assert
	// the record's shape without rebuilding the bare-template machinery this package owns.
	Mutations fabricengine.Mutations
}

// PrimeWorktree returns the path to this hub's prime warp worktree — the warp repository itself, not
// a pair.
func (h *Hub) PrimeWorktree() string {
	return h.Location.WorktreePath()
}

// PrimeWeft returns the path to the weft sibling paired with the prime warp worktree — the
// un-anchored weft worktree root, not the anchor-joined path.
// A caller reaching for this to seed config should use h.WeftBase and hubforge.SeedConfig instead:
// at a non-"." anchor, writing to the path this method returns produces a file no module loader ever
// reads, with no error at all.
func (h *Hub) PrimeWeft() string {
	return fabricengine.WeftWorktree(h.Location)
}

// BoardDir returns the path to this hub's _board data directory, the repo-wide weft:main checkout.
func (h *Hub) BoardDir() string {
	return fabricengine.BoardDir(h.Path)
}

// PairWarpWorktree returns the path to slug's warp worktree.
func (h *Hub) PairWarpWorktree(slug string) string {
	return fabricengine.WorktreePath(h.Location, slug)
}

// PairWeftSibling returns the path to slug's weft sibling worktree.
func (h *Hub) PairWeftSibling(slug string) string {
	return fabricengine.WeftWorktreePath(h.Location, slug)
}

// PairPortalLink returns the path to slug's mirrored portal junction link.
func (h *Hub) PairPortalLink(slug string) string {
	return fabricengine.PortalLink(h.Location, slug)
}

// PairLauncherDir returns the path to slug's mirrored launcher directory.
func (h *Hub) PairLauncherDir(slug string) string {
	return fabricengine.LauncherDir(h.Location, slug)
}

// NewHub builds a real fabric hub at anchor ("." or "backend"): it copies the cached bare template
// into tb's own temp dir, then drives fabriccli.CloneAndWire against the copies.
// It calls CloneAndWire, and never replicates the wiring sequence by hand, because
// fabricengine.CloneHub alone produces a partial hub — warp clone, weft clone, board, anchor marker,
// warp binding, but no junctions and no repo-wide fabric.yaml — which leaves three of the gate's eight
// path-ownership kinds (ownedWiredJunction, ownedDriftedWiredJunction, ownedUnderGeometryRoot)
// structurally unreachable.
// The hub CloneAndWire returns arrives with its weft prime worktree clean: each registered
// non-"fabric" module's config is committed on the weft primary branch, rather than carried as
// untracked content.
// It calls tb.Fatalf on any error.
func NewHub(tb testing.TB, anchor string) *Hub {
	tb.Helper()

	warpBare, weftBare := copyBares(tb)
	container := tb.TempDir()

	subpath := ""
	if anchor != "." {
		subpath = anchor
	}

	res, err := fabriccli.CloneAndWire(container, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: subpath,
	})
	if err != nil {
		tb.Fatalf("NewHub: CloneAndWire: %v", err)
	}

	loc, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		tb.Fatalf("NewHub: lyxcwd.Resolve(%s): %v", res.PrimeCwd, err)
	}

	// Registered after both copyBares and the container := tb.TempDir() call above, so LIFO
	// cleanup ordering runs junction removal before Go's own tb.TempDir() cleanup removes the
	// container — a junction left wired when os.RemoveAll walks into it is a Win11 correctness bug,
	// not a POSIX one, so this must hold on every platform even though only Windows can observe it
	// directly.
	registerTeardown(tb, res.HubPath)

	return &Hub{
		Path:      res.HubPath,
		Anchor:    res.Anchor,
		Location:  loc,
		Topology:  fabricengine.NewTopology(fabricengine.Config{}),
		WarpBare:  warpBare,
		WeftBare:  weftBare,
		WeftBase:  res.WeftBase,
		Container: container,
		Mutations: res.Mutated(),
	}
}

// registerTeardown installs a tb.Cleanup that removes every junction under hubPath before Go's own
// tb.TempDir() cleanup removes the container directory it lives in.
// It is a plain filepath.WalkDir from the hub root, calling fslink.IsLink on every entry and
// fslink.Remove on each link found.
//
// Discovery is slug-free by design: the walk never consults a slug list, because for the live-state
// matrix the pairs are created by the verb under test and some are destroyed by it, and because
// enumerating worktrees through fabric requires fabric to still work against a hub those tests
// deliberately corrupt.
//
// A missing hub directory (a worktree removed by hand mid-test) simply yields no entries from
// WalkDir, and fslink.Remove is documented idempotent, so neither case needs a special branch.
//
// Errors from WalkDir, IsLink and Remove are reported with tb.Logf and never tb.Fatalf or
// tb.Errorf: teardown must not fail a test that already passed.
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
			// WalkDir already reports a link as a non-directory entry and never follows it, so
			// non-descent is free. Returning filepath.SkipDir here — as it would from a directory
			// callback — would instead skip the containing directory's remaining entries, since
			// SkipDir from a non-directory callback means "stop visiting siblings in this
			// directory", not "don't descend into this entry". That would leave every sibling
			// junction wired: abandoning <hub>/_portals/<slug2> onward after removing
			// <slug1>, and leaving _lyx and _board behind after removing <worktree>/.lyx.
			// This is the single most reversible mistake in this file, so it is spelled out
			// here rather than left to be rediscovered.
			return nil
		})
		if walkErr != nil {
			tb.Logf("hubforge: teardown walk %s: %v", hubPath, walkErr)
		}
	})
}

// AddPair drives h.Topology.Add for slug against h, fataling on error.
// Several verbs' Arrange funcs need a pair to exist before the verb under test runs.
func AddPair(tb testing.TB, h *Hub, slug string) fabricengine.AddResult {
	tb.Helper()

	res, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err != nil {
		tb.Fatalf("AddPair(%s): %v", slug, err)
	}
	return res
}

// initScratchRepo initializes a git repository at dir on branch main, disabling fsmonitor and
// auto-maintenance, mirroring gitkit.initRepo.
func initScratchRepo(dir string) {
	mustGit(dir, "init", "-b", "main")
	mustGit(dir, "config", "user.email", "test@test.com")
	mustGit(dir, "config", "user.name", "Test")
	mustGit(dir, "config", "core.fsmonitor", "false")
	mustGit(dir, "config", "maintenance.auto", "false")
	mustGit(dir, "config", "gc.auto", "0")
	stripHookSamples(filepath.Join(dir, ".git", "hooks"))
}

// initBareRepo initializes a bare git repository at dir, disabling fsmonitor and auto-maintenance,
// mirroring gitkit.initBareRemote.
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

// mustGit runs a git subcommand in dir, panicking on non-zero exit.
// It is this file's equivalent local helper to gitkit.MustRun: the template builder runs once per
// test binary via sync.Once, before any testing.TB is necessarily in scope for every caller, so it
// panics rather than taking a tb — the same posture gitkit's own template builders take.
// Every git spawn in this file goes through it; none is a bare exec.Command whose failure is silently
// ignored.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}

// stripHookSamples removes every *.sample file from hooksDir, best-effort, mirroring
// gitkit.stripHookSamples.
func stripHookSamples(hooksDir string) {
	matches, err := filepath.Glob(filepath.Join(hooksDir, "*.sample"))
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// copyDirRecursive recursively copies a directory tree from src to dest, refusing any symlink found
// in the tree, mirroring gitkit's own copyDirRecursive: a template must stay plain files and
// directories only.
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
