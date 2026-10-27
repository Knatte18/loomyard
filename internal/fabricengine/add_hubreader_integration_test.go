//go:build tier1

// add_hubreader_integration_test.go covers a hub-wide reader polling a pair Add is still building.
// The origin read it makes through every listed worktree leaves no real _lym that the wiring then refuses.
// ReadOrigin itself creates no .weft directory and seeds no exclude.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

func TestAdd_HubReaderBeforeWiringLeavesNoLym(t *testing.T) {
	t.Parallel()

	const slug = "half-built"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	warpLym := fabricengine.CodeLymLink(h.Location, slug)

	readHalfBuilt := false
	fabricengine.SetBeforeWireForTest(h.Topology, func() {
		worktrees, err := fabricengine.List(h.Location.WorktreePath())
		if err != nil {
			t.Errorf("List() error = %v", err)
			return
		}
		for _, worktree := range worktrees {
			if worktree.Main {
				continue
			}
			pair, err := lymcwd.ResolveWorktree(worktree.Path)
			if err != nil {
				continue
			}
			weftPath := fabricengine.RecordsWorktree(pair)
			weftDirBefore := lstatExists(t, filepath.Join(weftPath, ".weft"))
			excludeBefore := readWeftExclude(t, weftPath)
			_, found, err := fabricengine.ReadOrigin(pair)
			if err != nil {
				t.Errorf("ReadOrigin(%q) error = %v; want nil", pair.WorktreeName, err)
			}
			if pair.WorktreeName == slug {
				if got := lstatExists(t, filepath.Join(weftPath, ".weft")); got != weftDirBefore {
					t.Errorf(".weft exists in %q = %v after ReadOrigin; want it unchanged from %v", weftPath, got, weftDirBefore)
				}
				if got := readWeftExclude(t, weftPath); got != excludeBefore {
					t.Errorf("info/exclude of %q = %q after ReadOrigin; want it unchanged from %q", weftPath, got, excludeBefore)
				}
				readHalfBuilt = true
				if found {
					t.Errorf("ReadOrigin(%q) found = true; want false before the pair is wired", slug)
				}
			}
		}
		if _, err := os.Lstat(warpLym); !os.IsNotExist(err) {
			t.Errorf("Lstat(%q) error = %v; want the reader to leave no _lym before wiring", warpLym, err)
		}
	})

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	if !readHalfBuilt {
		t.Errorf("the reader never read the half-built pair %q; want it listed and resolved before wiring", slug)
	}
	if isLink, err := fslink.IsLink(warpLym); err != nil || !isLink {
		t.Errorf("IsLink(%q) = (%v, %v); want (true, nil) once Add completes", warpLym, isLink, err)
	}
}

// weftExcludePath returns the git info/exclude file that weftPath's repository uses.
func weftExcludePath(t *testing.T, weftPath string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", weftPath, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-path info/exclude in %q: %v", weftPath, err)
	}
	return strings.TrimSpace(string(out))
}

// readWeftExclude returns the content of the git info/exclude file that weftPath's repository uses, or "" when the file does not exist.
func readWeftExclude(t *testing.T, weftPath string) string {
	t.Helper()
	content, err := os.ReadFile(weftExcludePath(t, weftPath))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("ReadFile(info/exclude) in %q: %v", weftPath, err)
	}
	return string(content)
}

// lstatExists reports whether path exists, failing the test on any Lstat error other than not-exist.
func lstatExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("Lstat(%q): %v", path, err)
	}
	return err == nil
}

func TestReadOrigin_CreatesNoWeftDirectoryAndSeedsNoExclude(t *testing.T) {
	t.Parallel()

	const slug = "no-weft-dir"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", slug, err)
	}
	weftPath := fabricengine.RecordsWorktree(pair)
	weftLockDir := filepath.Join(weftPath, ".weft")
	if err := os.RemoveAll(weftLockDir); err != nil {
		t.Fatalf("RemoveAll(%q): %v", weftLockDir, err)
	}
	// Add already seeded the exclude; emptying it lets a regression that seeds it again show.
	if err := os.WriteFile(weftExcludePath(t, weftPath), nil, 0o644); err != nil {
		t.Fatalf("WriteFile(info/exclude): %v", err)
	}

	origin, found, err := fabricengine.ReadOrigin(pair)
	if err != nil || !found {
		t.Fatalf("ReadOrigin(%q) = (%+v, %v, %v); want the record read without its lock directory", slug, origin, found, err)
	}
	if lstatExists(t, weftLockDir) {
		t.Errorf("%q exists after ReadOrigin; want a read to create no .weft directory", weftLockDir)
	}
	if got := readWeftExclude(t, weftPath); got != "" {
		t.Errorf("info/exclude of %q = %q after ReadOrigin; want it still empty", weftPath, got)
	}
}
