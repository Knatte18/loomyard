//go:build integration

// lyxcwd_test.go covers Location resolution against a real git checkout: Resolve's record-wins + strict cwd-equals-anchor gate, the marker-absent "." fallback, ResolveWorktree's gate-free counterpart used by internal callers that resolve geometry from a worktree root rather than an acting cwd, the stale-marker refusal, and the ErrNotAGitRepo path for directories outside a git repo.

package lyxcwd_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// writeAnchor writes the recorded .lyx-anchor marker into hub's board
// directory, creating the board directory if needed. hub here is the
// lyxcwd.Location.HubPath value (the container directory), not a worktree root.
func writeAnchor(t *testing.T, hub, anchor string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(anchor), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// requireOutsideAnchor fails unless Resolve(cwd) returns no layout and an error wrapping ErrCwdOutsideAnchor.
func requireOutsideAnchor(t *testing.T, cwd string) {
	t.Helper()

	layout, err := lyxcwd.Resolve(cwd)
	if layout != nil {
		t.Errorf("Resolve(%q) returned non-nil layout; want nil", cwd)
	}
	if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
		t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", cwd, err)
	}
}

// requireAnchorRel fails unless Resolve(cwd) succeeds with the given AnchorRel.
func requireAnchorRel(t *testing.T, cwd, want string) {
	t.Helper()

	layout, err := lyxcwd.Resolve(cwd)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v; want nil", cwd, err)
	}
	if layout.AnchorRel != want {
		t.Errorf("Resolve(%q).AnchorRel = %q; want %q", cwd, layout.AnchorRel, want)
	}
}

// TestResolve_AnchorScenario copies one git checkout and runs each resolution case as a step over it, in the order below; each step starts from the anchor marker state the step before it left.
// The steps run serially, and the top-level test calls t.Parallel and no step does, because the steps share the one checkout and its recorded marker.
//
// Steps: no marker recorded; a root (".") anchor; a subpath ("backend") anchor; a stale pre-rename marker.
func TestResolve_AnchorScenario(t *testing.T) {
	t.Parallel()

	fix := gitkit.CopyRepo(t)
	root := fix.Repo

	base, err := lyxcwd.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve(root) error = %v; want nil", err)
	}

	subDir := filepath.Join(root, "sub", "nested")
	backendDir := filepath.Join(root, "backend")
	deeperDir := filepath.Join(backendDir, "deeper")
	frontendDir := filepath.Join(root, "frontend")
	for _, dir := range []string{subDir, deeperDir, frontendDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// With no anchor recorded, AnchorRel falls back to "." with no error at the worktree root, never to a cwd-derived relative path, which would make the Location name a lie.
	// The strict gate applies unconditionally, so a subdirectory errors.
	if !t.Run("no marker recorded", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(root)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v; want nil", root, err)
		}
		if layout == nil {
			t.Fatal("Resolve() returned nil layout")
		}
		if layout.AnchorRel != "." {
			t.Errorf("layout.AnchorRel = %q; want %q (no-anchor fallback)", layout.AnchorRel, ".")
		}
		if layout.WorktreePath() != filepath.Clean(root) {
			t.Errorf("layout.WorktreePath() = %q; want %q", layout.WorktreePath(), filepath.Clean(root))
		}
		if want := filepath.Dir(root); layout.HubPath != want {
			t.Errorf("layout.HubPath = %q; want %q", layout.HubPath, want)
		}
		// RepoName is derived by trimming HubSuffix off the container directory's base name; this fixture's container has no "-LYXHUB" suffix, so RepoName is simply its base name unchanged.
		if want := strings.TrimSuffix(filepath.Base(layout.HubPath), fabricengine.HubSuffix); layout.RepoName != want {
			t.Errorf("layout.RepoName = %q; want %q", layout.RepoName, want)
		}

		requireOutsideAnchor(t, subDir)
	}) {
		return
	}

	// A root anchor resolves from exactly the worktree root, and the strict gate rejects a subdirectory of it.
	if !t.Run("root anchor", func(t *testing.T) {
		writeAnchor(t, base.HubPath, ".")

		requireAnchorRel(t, root, ".")
		requireOutsideAnchor(t, subDir)
	}) {
		return
	}

	// A subpath anchor resolves from exactly the anchored directory; a descendant, a sibling and the repo root above it are hard errors wrapping ErrCwdOutsideAnchor.
	// ResolveWorktree from a worktree root that sits ABOVE the anchor returns the recorded subpath and no ErrCwdOutsideAnchor: this gate-free behavior is what distinguishes it from Resolve, and is the exact geometry fabricengine's layout fallback hits.
	if !t.Run("subpath anchor", func(t *testing.T) {
		writeAnchor(t, base.HubPath, "backend")

		requireAnchorRel(t, backendDir, "backend")
		for name, cwd := range map[string]string{
			"descendant of the anchored directory": deeperDir,
			"sibling directory of the anchor":      frontendDir,
			"repo root above a subpath anchor":     root,
		} {
			t.Run(name, func(t *testing.T) {
				requireOutsideAnchor(t, cwd)
			})
		}

		layout, err := lyxcwd.ResolveWorktree(root)
		if err != nil {
			t.Fatalf("ResolveWorktree(%q) error = %v; want nil (no gate applied)", root, err)
		}
		if layout.AnchorRel != "backend" {
			t.Errorf("ResolveWorktree(%q).AnchorRel = %q; want %q", root, layout.AnchorRel, "backend")
		}
	}) {
		return
	}

	// The read side refuses a hub that recorded its subpath under the pre-rename marker name and never migrated.
	// Falling back to "." there re-anchors the whole repo at its root, after which fabric's own repair verb wires a second junction set at that root, so both the gated and the gate-free resolver must refuse instead.
	if !t.Run("stale marker", func(t *testing.T) {
		boardDir := fabricengine.BoardDir(base.HubPath)
		if err := os.Remove(filepath.Join(boardDir, lyxcwd.AnchorFileName)); err != nil {
			t.Fatalf("remove %s: %v", lyxcwd.AnchorFileName, err)
		}
		stalePath := filepath.Join(boardDir, lyxcwd.StaleAnchorFileName)
		if err := os.WriteFile(stalePath, []byte("backend\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", stalePath, err)
		}

		if !t.Run("Resolve refuses", func(t *testing.T) {
			layout, err := lyxcwd.Resolve(root)
			if layout != nil {
				t.Errorf("Resolve(%q) returned non-nil layout; want nil", root)
			}
			if !errors.Is(err, lyxcwd.ErrStaleAnchorMarker) {
				t.Errorf("Resolve(%q) error = %v; want wrapped ErrStaleAnchorMarker", root, err)
			}
		}) {
			return
		}

		if !t.Run("ResolveWorktree refuses", func(t *testing.T) {
			layout, err := lyxcwd.ResolveWorktree(root)
			if layout != nil {
				t.Errorf("ResolveWorktree(%q) returned non-nil layout; want nil", root)
			}
			if !errors.Is(err, lyxcwd.ErrStaleAnchorMarker) {
				t.Errorf("ResolveWorktree(%q) error = %v; want wrapped ErrStaleAnchorMarker", root, err)
			}
		}) {
			return
		}

		t.Run("renamed marker beside it resolves normally", func(t *testing.T) {
			writeAnchor(t, base.HubPath, ".")
			requireAnchorRel(t, root, ".")
		})
	}) {
		return
	}

	// A start reached through a symlink resolves to the real root's Location, as `git rev-parse --show-toplevel` resolves it, so one hub never gets two hub paths.
	// A link to a subdirectory has no repository among its lexical parents, so only a walk from the resolved start finds the root.
	// Windows links are junctions, which filepath.EvalSymlinks leaves unresolved as a mount point.
	if !t.Run("symlinked start", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("fslink creates a junction on Windows, which is not resolved")
		}
		linkDir := t.TempDir()
		rootLink := filepath.Join(linkDir, "root-link")
		subLink := filepath.Join(linkDir, "sub-link")
		for link, target := range map[string]string{rootLink: root, subLink: subDir} {
			if err := fslink.CreateDirLink(link, target); err != nil {
				t.Fatalf("link %s -> %s: %v", link, target, err)
			}
		}
		want := lyxcwd.Location{RepoName: base.RepoName, HubPath: base.HubPath, WorktreeName: base.WorktreeName, AnchorRel: "."}

		got, err := lyxcwd.Resolve(rootLink)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v; want nil", rootLink, err)
		}
		if *got != want {
			t.Errorf("Resolve(%q) = %+v; want %+v", rootLink, *got, want)
		}

		got, err = lyxcwd.ResolveWorktree(subLink)
		if err != nil {
			t.Fatalf("ResolveWorktree(%q) error = %v; want nil", subLink, err)
		}
		if *got != want {
			t.Errorf("ResolveWorktree(%q) = %+v; want %+v", subLink, *got, want)
		}
	}) {
		return
	}

	// A linked worktree added beside the checkout resolves to its own root with the same Location fields a clone would give, including after its gitfile is rewritten to a relative gitdir.
	linkedDir := filepath.Join(filepath.Dir(root), "linked")
	adminDir := filepath.Join(root, ".git", "worktrees", "linked")
	if !t.Run("linked worktree", func(t *testing.T) {
		gitkit.Git(t, root, "worktree", "add", "-b", "linked", linkedDir)

		want := &lyxcwd.Location{RepoName: base.RepoName, HubPath: base.HubPath, WorktreeName: "linked", AnchorRel: "."}
		requireLocation := func(t *testing.T) {
			t.Helper()
			got, err := lyxcwd.Resolve(linkedDir)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v; want nil", linkedDir, err)
			}
			if *got != *want {
				t.Errorf("Resolve(%q) = %+v; want %+v", linkedDir, *got, *want)
			}
		}

		t.Run("absolute gitdir", requireLocation)
		t.Run("relative gitdir", func(t *testing.T) {
			relative, err := filepath.Rel(linkedDir, adminDir)
			if err != nil {
				t.Fatalf("rel %s: %v", adminDir, err)
			}
			if err := os.WriteFile(filepath.Join(linkedDir, ".git"), []byte("gitdir: "+filepath.ToSlash(relative)+"\n"), 0o644); err != nil {
				t.Fatalf("rewrite gitfile: %v", err)
			}
			requireLocation(t)
		})
	}) {
		return
	}

	// An empty .git directory in a subdirectory is skipped as git skips it, so the walk reaches the enclosing root, where the strict gate still rejects the subdirectory.
	if !t.Run("empty .git directory in a subdirectory", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(subDir, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		requireOutsideAnchor(t, subDir)
	}) {
		return
	}

	// A gitfile naming a removed git dir, and a linked worktree whose commondir target was removed, are both refused with the bare sentinel rather than resolved to the enclosing checkout.
	t.Run("pruned gitfile and missing commondir", func(t *testing.T) {
		prunedDir := filepath.Join(root, "pruned")
		if err := os.MkdirAll(prunedDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(prunedDir, ".git"), []byte("gitdir: "+filepath.Join(root, "gone")+"\n"), 0o644); err != nil {
			t.Fatalf("write gitfile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(adminDir, "commondir"), []byte("../gone\n"), 0o644); err != nil {
			t.Fatalf("rewrite commondir: %v", err)
		}

		for name, cwd := range map[string]string{
			"gitfile naming a removed git dir":              prunedDir,
			"linked worktree with a removed commondir path": linkedDir,
		} {
			t.Run(name, func(t *testing.T) {
				layout, err := lyxcwd.Resolve(cwd)
				if layout != nil {
					t.Errorf("Resolve(%q) returned non-nil layout; want nil", cwd)
				}
				if !errors.Is(err, lyxcwd.ErrNotAGitRepo) || err.Error() != lyxcwd.ErrNotAGitRepo.Error() {
					t.Errorf("Resolve(%q) error = %v; want the bare ErrNotAGitRepo", cwd, err)
				}
			})
		}
	})
}

// TestResolve_NotAGitRepo verifies that Resolve in a non-git temp directory returns ErrNotAGitRepo.
func TestResolve_NotAGitRepo(t *testing.T) {
	t.Parallel()

	nonGitDir := t.TempDir()

	layout, err := lyxcwd.Resolve(nonGitDir)

	if layout != nil {
		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
	}

	if !errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
	}

	// Pin the bare-sentinel behavior: git's raw stderr must never leak into the
	// error text, and no other content may be appended to the sentinel message.
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Resolve() error = %q; must not contain raw git stderr (\"fatal:\")", err.Error())
	}
	if err.Error() != lyxcwd.ErrNotAGitRepo.Error() {
		t.Errorf("Resolve() error = %q; want exactly %q", err.Error(), lyxcwd.ErrNotAGitRepo.Error())
	}
}
