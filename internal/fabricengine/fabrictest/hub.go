//go:build integration

// hub.go builds the bare-pair template every cell in this harness clones from, and the
// CloneAndWire-backed factory that turns a fresh copy of that template into a real fabric hub.
//
// The template is built once per test binary (sync.Once-cached), then copied into each scenario's
// own tb.TempDir() via copyBares, so scenarios never race over a shared push target.
// The factory calls fabriccli.CloneAndWire — never fabricengine.CloneHub alone, and never a
// hand-rolled wiring sequence — because CloneHub alone produces a partial hub (warp clone, weft
// clone, board, anchor marker, warp binding, but no junctions and no repo-wide fabric.yaml), which
// leaves three of the destructive gate's eight path-ownership kinds structurally unreachable.

package fabrictest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// mustGit runs a git subcommand in dir, panicking on non-zero exit — the template-build equivalent of
// lyxtest's own unexported mustGit, needed because the bare-pair template is built once per test
// binary via sync.Once, outside any single test's testing.TB.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}

// bareTemplateOnce guards the one-per-test-binary construction of warpBareTemplate and
// weftBareTemplate.
var (
	bareTemplateOnce sync.Once
	warpBareTemplate string
	weftBareTemplate string
)

// buildBareTemplate constructs the warp and weft bare-pair template once per test binary: a warp
// bare carrying a pushed main branch with both a root README and a backend/ subdirectory (so one
// template serves both the "." and "backend" anchors), and a weft bare left genuinely empty.
//
// Two gotchas are encoded here and nowhere else:
//
//  1. git init --bare leaves HEAD on master even once main is pushed to it, so the builder must run
//     `git symbolic-ref HEAD refs/heads/main` in the bare after the push — without this, a clone of
//     the bare checks out the wrong (nonexistent) branch.
//  2. the weft bare must be left genuinely empty and never pushed to, or CloneHub's bootstrap guard
//     at clone.go:172 (!probe.WeftLooksLikeWeft) refuses it outright.
//
// lyxtest's own bares cannot be reused for this: initBareRemote creates them and adds them as origin
// but never pushes, which is exactly why the symbolic-ref gotcha never arises there — this template
// needs a bare that has actually been pushed to, and lyxtest's has not.
func buildBareTemplate() {
	bareTemplateOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "fabrictest-baretmpl-*")
		if err != nil {
			panic(err)
		}

		// Build the scratch work repo on main, with a root README and a backend/
		// subdirectory holding at least one committed file, so the same warp bare
		// serves a "." anchor and a "backend" subpath anchor alike.
		work := filepath.Join(tmpDir, "work")
		if err := os.Mkdir(work, 0o755); err != nil {
			panic(err)
		}
		mustGit(work, "init", "-q", "-b", "main")
		mustGit(work, "config", "user.email", "test@test.com")
		mustGit(work, "config", "user.name", "Test")
		mustGit(work, "config", "core.fsmonitor", "false")
		mustGit(work, "config", "maintenance.auto", "false")
		mustGit(work, "config", "gc.auto", "0")

		if err := os.WriteFile(filepath.Join(work, "README"), []byte("fabrictest warp template"), 0o644); err != nil {
			panic(err)
		}
		backendDir := filepath.Join(work, "backend")
		if err := os.MkdirAll(backendDir, 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(backendDir, "README"), []byte("fabrictest backend template"), 0o644); err != nil {
			panic(err)
		}
		mustGit(work, "add", ".")
		mustGit(work, "commit", "-q", "-m", "init")

		// Bare pair, sibling of work: warpBare is pushed to and then re-pointed onto
		// main; weftBare is created and left empty, never pushed.
		warpBare := filepath.Join(tmpDir, "warp-bare")
		mustGit(tmpDir, "init", "-q", "--bare", warpBare)
		mustGit(warpBare, "config", "core.fsmonitor", "false")
		mustGit(warpBare, "config", "maintenance.auto", "false")
		mustGit(warpBare, "config", "gc.auto", "0")
		stripHookSamplesFor(warpBare)

		mustGit(work, "remote", "add", "origin", warpBare)
		mustGit(work, "push", "-q", "origin", "main")
		// Gotcha 1: HEAD is still "master" here even though main is what was pushed.
		mustGit(warpBare, "symbolic-ref", "HEAD", "refs/heads/main")

		weftBare := filepath.Join(tmpDir, "weft-bare")
		mustGit(tmpDir, "init", "-q", "--bare", weftBare)
		mustGit(weftBare, "config", "core.fsmonitor", "false")
		mustGit(weftBare, "config", "maintenance.auto", "false")
		mustGit(weftBare, "config", "gc.auto", "0")
		stripHookSamplesFor(weftBare)
		// Gotcha 2: weftBare is deliberately never pushed to — CloneHub's bootstrap
		// guard refuses a weft candidate that already looks weft-shaped.

		warpBareTemplate = warpBare
		weftBareTemplate = weftBare
	})
}

// stripHookSamplesFor removes every *.sample hook left by `git init --bare` in bareDir/hooks,
// mirroring lyxtest's own template hygiene (best-effort; errors are ignored).
func stripHookSamplesFor(bareDir string) {
	matches, err := filepath.Glob(filepath.Join(bareDir, "hooks", "*.sample"))
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// copyBares returns an isolated copy of the cached bare-pair template, placed under tb.TempDir(), so
// each scenario pushes to its own copy and cells never race.
// Copying the bares is the deliberate choice over building fresh ones per scenario, which would cost
// a full git init --bare plus work repo plus commit plus push per cell.
func copyBares(tb testing.TB) (warpBare, weftBare string) {
	tb.Helper()

	buildBareTemplate()

	container := tb.TempDir()
	warpBare = filepath.Join(container, "warp-bare")
	weftBare = filepath.Join(container, "weft-bare")

	if err := copyDirTree(warpBareTemplate, warpBare); err != nil {
		tb.Fatalf("copy warp bare template: %v", err)
	}
	if err := copyDirTree(weftBareTemplate, weftBare); err != nil {
		tb.Fatalf("copy weft bare template: %v", err)
	}
	return warpBare, weftBare
}

// copyDirTree recursively copies src to dst, refusing to follow any symlink it encounters — the
// template tree is always plain files and directories.
func copyDirTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("copyDirTree: symlink not allowed in bare template: %s", path)
		}
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// Hub is a real, fully-wired fabric hub built by NewHub: a cloned warp/weft pair, junctioned and
// carrying a repo-wide fabric.yaml, ready to drive verbs and states against.
type Hub struct {
	// Path is the hub root (<name>-HUB).
	Path string
	// Anchor is the resolved AnchorRel value: "." or "backend".
	Anchor string
	// Location is the *lyxcwd.Location resolved from the prime warp worktree, obtained via
	// lyxcwd.Resolve and never constructed by hand, per the Cwd Resolution Invariant.
	Location *lyxcwd.Location
	// Topology is the fabricengine.Topology handle for driving worktree-topology verbs against
	// this hub.
	Topology *fabricengine.Topology
	// WarpBare is this hub's own copy of the warp bare-remote template.
	WarpBare string
	// WeftBare is this hub's own copy of the weft bare-remote template.
	WeftBare string
	// Container is the tb.TempDir() this hub was cloned into.
	Container string
}

// PrimeWorktree returns the path to the hub's prime warp worktree.
func (h *Hub) PrimeWorktree() string {
	return h.Location.WorktreePath()
}

// PrimeWeft returns the path to the weft sibling paired with the hub's prime warp worktree.
func (h *Hub) PrimeWeft() string {
	return fabricengine.WeftWorktree(h.Location)
}

// BoardDir returns the path to the hub's _board weft:main checkout.
func (h *Hub) BoardDir() string {
	return fabricengine.BoardDir(h.Path)
}

// PairWorktree returns the path to the warp worktree for a pair named slug.
func (h *Hub) PairWorktree(slug string) string {
	return fabricengine.WorktreePath(h.Location, slug)
}

// PairWeft returns the path to the weft sibling for a pair named slug.
func (h *Hub) PairWeft(slug string) string {
	return fabricengine.WeftWorktreePath(h.Location, slug)
}

// PairPortal returns the path to a pair's portal junction link.
func (h *Hub) PairPortal(slug string) string {
	return fabricengine.PortalLink(h.Location, slug)
}

// PairLauncherDir returns the path to a pair's mirrored launcher directory.
func (h *Hub) PairLauncherDir(slug string) string {
	return fabricengine.LauncherDir(h.Location, slug)
}

// NewHub clones a fresh copy of the bare-pair template and drives fabriccli.CloneAndWire against it,
// producing a real, fully-wired hub anchored at anchor ("." or "backend").
// It fatals on any error via tb.Fatalf, resolves the *lyxcwd.Location from the resulting
// CloneResult.PrimeCwd, and returns the populated Hub.
//
// NewHub calls CloneAndWire and never replicates its wiring sequence itself — CloneHub alone leaves
// junctions and the repo-wide fabric.yaml absent, which is not the "real hub" this harness needs.
func NewHub(tb testing.TB, anchor string) *Hub {
	tb.Helper()

	warpBare, weftBare := copyBares(tb)
	container := tb.TempDir()

	subpath := anchor
	if anchor == "." {
		subpath = ""
	}

	res, err := fabriccli.CloneAndWire(container, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: subpath,
	})
	if err != nil {
		tb.Fatalf("CloneAndWire: %v", err)
	}

	l, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		tb.Fatalf("lyxcwd.Resolve(%q): %v", res.PrimeCwd, err)
	}

	return &Hub{
		Path:      res.HubPath,
		Anchor:    res.Anchor,
		Location:  l,
		Topology:  fabricengine.NewTopology(fabricengine.Config{}),
		WarpBare:  warpBare,
		WeftBare:  weftBare,
		Container: container,
	}
}

// AddPair drives h.Topology.Add for slug against h, fataling on error via tb.Fatalf. Several verbs'
// Arrange funcs need an existing pair to act on, and this is the one path they create it through.
func AddPair(tb testing.TB, h *Hub, slug string) {
	tb.Helper()

	if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		tb.Fatalf("Topology.Add(%q): %v", slug, err)
	}
}
