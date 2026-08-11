//go:build integration

// hub_test.go proves the bare-pair template builder and the CloneAndWire-backed hub factory each do
// what their doc comments claim: the template carries the symbolic-ref gotcha's fix and a genuinely
// empty weft bare, and NewHub produces a real, fully-wired fabric hub rather than a hand-assembled
// stand-in — the whole reason NewHub calls CloneAndWire instead of CloneHub alone.

package fabrictest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// runGit runs a git subcommand in dir and returns its trimmed stdout, failing the test on a non-zero
// exit.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(string(out))
}

func TestBuildBareTemplate(t *testing.T) {
	t.Parallel()

	warpBare, weftBare := buildBareTemplate()

	t.Run("WarpHEADResolvesToMain", func(t *testing.T) {
		head := runGit(t, warpBare, "symbolic-ref", "HEAD")
		if head != "refs/heads/main" {
			t.Errorf("warp bare HEAD = %q; want refs/heads/main", head)
		}
	})

	t.Run("WarpCommitCarriesRootAndBackendEntries", func(t *testing.T) {
		out := runGit(t, warpBare, "ls-tree", "-r", "--name-only", "main")
		entries := strings.Split(out, "\n")
		var hasRoot, hasBackend bool
		for _, e := range entries {
			if e == "README" {
				hasRoot = true
			}
			if strings.HasPrefix(e, "backend/") {
				hasBackend = true
			}
		}
		if !hasRoot {
			t.Errorf("warp bare main tree = %v; want a root README entry", entries)
		}
		if !hasBackend {
			t.Errorf("warp bare main tree = %v; want a backend/ entry", entries)
		}
	})

	t.Run("WeftBareIsGenuinelyEmpty", func(t *testing.T) {
		out := runGit(t, weftBare, "for-each-ref")
		if out != "" {
			t.Errorf("weft bare for-each-ref = %q; want no refs at all", out)
		}
	})
}

func TestNewHub(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "backend"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()

			h := NewHub(t, anchor)

			if _, err := os.Stat(h.PrimeWorktree()); err != nil {
				t.Errorf("prime warp worktree missing at %s: %v", h.PrimeWorktree(), err)
			}
			if _, err := os.Stat(h.PrimeWeft()); err != nil {
				t.Errorf("weft sibling missing at %s: %v", h.PrimeWeft(), err)
			}
			if _, err := os.Stat(h.BoardDir()); err != nil {
				t.Errorf("board dir missing at %s: %v", h.BoardDir(), err)
			}

			primeCwd := filepath.Join(h.PrimeWorktree(), h.Anchor)
			l, err := lyxcwd.Resolve(primeCwd)
			if err != nil {
				t.Fatalf("lyxcwd.Resolve(%s): %v", primeCwd, err)
			}
			if l.AnchorRel != anchor {
				t.Errorf("resolved AnchorRel = %q; want %q", l.AnchorRel, anchor)
			}

			if _, err := os.Stat(filepath.Join(h.BoardDir(), fabricengine.WarpBindingFileName)); err != nil {
				t.Errorf("recorded warp binding missing at %s: %v", h.BoardDir(), err)
			}

			names, err := fabricengine.WiredNames(h.BoardDir())
			if err != nil {
				t.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
			}
			for _, name := range names {
				link := filepath.Join(h.PrimeWorktree(), h.Anchor, name)
				isLink, err := fslink.IsLink(link)
				if err != nil {
					t.Errorf("fslink.IsLink(%s): %v", link, err)
					continue
				}
				if !isLink {
					t.Errorf("wired junction %s: want a link, got none", link)
				}
			}

			if _, err := os.Stat(filepath.Join(h.BoardDir(), lyxdirs.LyxDirName, "config", "fabric.yaml")); err != nil {
				t.Errorf("repo-wide fabric.yaml missing at %s: %v", h.BoardDir(), err)
			}

			const slug = "px"
			AddPair(t, h, slug)

			portal := fabricengine.PortalLink(h.Location, slug)
			if _, err := os.Stat(portal); err != nil {
				t.Errorf("pair portal link missing at %s: %v", portal, err)
			}
			if got := h.PairPortalLink(slug); got != portal {
				t.Errorf("h.PairPortalLink(%s) = %s; want %s", slug, got, portal)
			}

			launcherDir := fabricengine.LauncherDir(h.Location, slug)
			if _, err := os.Stat(launcherDir); err != nil {
				t.Errorf("pair launcher dir missing at %s: %v", launcherDir, err)
			}
			if got := h.PairLauncherDir(slug); got != launcherDir {
				t.Errorf("h.PairLauncherDir(%s) = %s; want %s", slug, got, launcherDir)
			}
		})
	}
}
