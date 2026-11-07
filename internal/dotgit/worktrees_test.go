// worktrees_test.go covers ListWorktrees on hand-laid repositories:
// entry order, HEAD and ref resolution, locked and prunable entries, the entries git leaves out, and the refusals.

package dotgit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	commitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	commitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var zeroCommit = strings.Repeat("0", 40)

// mainRepository lays out a main worktree at dir on refs/heads/main at commitA, its `.git` config holding config.
// It returns the common dir.
func mainRepository(t *testing.T, dir, config string) string {
	t.Helper()
	commonDir := filepath.Join(dir, ".git")
	writeFile(t, filepath.Join(commonDir, "config"), config)
	writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(commonDir, "refs", "heads", "main"), commitA+"\n")
	mkdirs(t, filepath.Join(commonDir, "objects"))
	return commonDir
}

// addLinked lays out the admin entry id under commonDir for a worktree at worktreeDir, with an absolute `gitdir`.
// It writes head to the entry's HEAD when head is not empty, and creates the worktree's `.git` when created is true.
// It returns the admin entry.
func addLinked(t *testing.T, commonDir, id, worktreeDir, head string, created bool) string {
	t.Helper()
	adminDir := filepath.Join(commonDir, "worktrees", id)
	writeFile(t, filepath.Join(adminDir, "gitdir"), filepath.Join(worktreeDir, ".git")+"\n")
	writeFile(t, filepath.Join(adminDir, "commondir"), "../..\n")
	if head != "" {
		writeFile(t, filepath.Join(adminDir, "HEAD"), head+"\n")
	}
	if created {
		writeFile(t, filepath.Join(worktreeDir, ".git"), "gitdir: "+adminDir+"\n")
	}
	return adminDir
}

func TestListWorktrees(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// build lays out the repository under root and returns the directory to list from and the wanted entries.
		build          func(t *testing.T, root string) (start string, want []Worktree)
		needsSymlink   bool
		wantErrIs      error
		wantErrContain string
	}{
		{
			name: "main worktree alone",
			build: func(t *testing.T, root string) (string, []Worktree) {
				mainRepository(t, filepath.Join(root, "repo"), "")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
				}
			},
		},
		{
			name: "linked worktrees follow sorted by path with branch, detached, locked and prunable entries",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "refs", "heads", "task"), commitB+"\n")
				addLinked(t, commonDir, "z", filepath.Join(root, "b-branch"), "ref: refs/heads/task", true)
				addLinked(t, commonDir, "a", filepath.Join(root, "c-detached"), commitB, true)
				locked := addLinked(t, commonDir, "m", filepath.Join(root, "a-locked"), "ref: refs/heads/main", false)
				writeFile(t, filepath.Join(locked, "locked"), "moved\n")
				addLinked(t, commonDir, "p", filepath.Join(root, "d-gone"), commitA, false)
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "a-locked"), Head: commitA, Branch: "refs/heads/main", Locked: true},
					{Path: filepath.Join(root, "b-branch"), Head: commitB, Branch: "refs/heads/task"},
					{Path: filepath.Join(root, "c-detached"), Head: commitB},
					{Path: filepath.Join(root, "d-gone"), Head: commitA, Prunable: true},
				}
			},
		},
		{
			name: "a branch only in packed-refs resolves to its commit",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "packed-refs"),
					"# pack-refs with: peeled fully-peeled sorted\n"+commitB+" refs/heads/packed\n^"+commitA+"\n")
				addLinked(t, commonDir, "packed", filepath.Join(root, "packed"), "ref: refs/heads/packed", true)
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "packed"), Head: commitB, Branch: "refs/heads/packed"},
				}
			},
		},
		{
			name: "a branch with no commit yet is the all-zero commit",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/unborn\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: zeroCommit, Branch: "refs/heads/unborn"},
				}
			},
		},
		{
			name: "a sha256 repository's all-zero commit has 64 digits",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "[extensions]\n\tobjectFormat = sha256\n")
				writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/unborn\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: strings.Repeat("0", 64), Branch: "refs/heads/unborn"},
				}
			},
		},
		{
			name: "an entry whose gitdir file is missing or empty is left out",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				missing := addLinked(t, commonDir, "missing", filepath.Join(root, "missing"), commitA, true)
				if err := os.Remove(filepath.Join(missing, "gitdir")); err != nil {
					t.Fatalf("remove gitdir: %v", err)
				}
				empty := addLinked(t, commonDir, "empty", filepath.Join(root, "empty"), commitA, true)
				writeFile(t, filepath.Join(empty, "gitdir"), " \n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
				}
			},
		},
		{
			name: "an entry without a HEAD file is detached at the all-zero commit",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				addLinked(t, commonDir, "adding", filepath.Join(root, "adding"), "", true)
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "adding"), Head: zeroCommit},
				}
			},
		},
		{
			name: "a relative gitdir resolves against its admin entry",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				adminDir := addLinked(t, commonDir, "rel", filepath.Join(root, "rel"), commitB, true)
				writeFile(t, filepath.Join(adminDir, "gitdir"), "../../../../rel/.git\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "rel"), Head: commitB},
				}
			},
		},
		{
			name:         "a relative gitdir reached through a symlinked admin entry resolves against the real entry",
			needsSymlink: true,
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				adminDir := addLinked(t, commonDir, "rel", filepath.Join(root, "rel"), commitB, true)
				writeFile(t, filepath.Join(adminDir, "gitdir"), "../../../../rel/.git\n")
				mkdirs(t, filepath.Join(root, "deep"))
				if err := os.Symlink(filepath.Join(root, "repo"), filepath.Join(root, "deep", "link")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return filepath.Join(root, "deep", "link"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "rel"), Head: commitB},
				}
			},
		},
		{
			name:         "a relative gitdir whose target does not exist resolves its existing prefix and is prunable",
			needsSymlink: true,
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				adminDir := addLinked(t, commonDir, "gone", filepath.Join(root, "gone"), commitB, false)
				writeFile(t, filepath.Join(adminDir, "gitdir"), "../../../../linkdir/gone/.git\n")
				mkdirs(t, filepath.Join(root, "real"))
				if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linkdir")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "real", "gone"), Head: commitB, Prunable: true},
				}
			},
		},
		{
			name: "a plain file under worktrees is skipped",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "worktrees", "stray"), "not an admin entry\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
				}
			},
		},
		{
			name: "an explicit files ref storage lists normally",
			build: func(t *testing.T, root string) (string, []Worktree) {
				mainRepository(t, filepath.Join(root, "repo"), "[extensions]\n\trefStorage = files\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
				}
			},
		},
		{
			name: "a symbolic ref chain of five links resolves to the last ref of the chain",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/link1\n")
				for link := 1; link <= 4; link++ {
					writeFile(t, filepath.Join(commonDir, "refs", "heads", fmt.Sprintf("link%d", link)),
						fmt.Sprintf("ref: refs/heads/link%d\n", link+1))
				}
				writeFile(t, filepath.Join(commonDir, "refs", "heads", "link5"), commitB+"\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitB, Branch: "refs/heads/link5"},
				}
			},
		},
		{
			name: "a symbolic ref chain of six links is an error",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/link1\n")
				for link := 1; link <= 5; link++ {
					writeFile(t, filepath.Join(commonDir, "refs", "heads", fmt.Sprintf("link%d", link)),
						fmt.Sprintf("ref: refs/heads/link%d\n", link+1))
				}
				writeFile(t, filepath.Join(commonDir, "refs", "heads", "link6"), commitA+"\n")
				return filepath.Join(root, "repo"), nil
			},
			wantErrContain: "symbolic refs nest deeper",
		},
		{
			name: "a ref below a loose ref file falls through to packed-refs",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				writeFile(t, filepath.Join(commonDir, "HEAD"), "ref: refs/heads/a/b\n")
				writeFile(t, filepath.Join(commonDir, "refs", "heads", "a"), commitA+"\n")
				writeFile(t, filepath.Join(commonDir, "packed-refs"), commitB+" refs/heads/a/b\n")
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitB, Branch: "refs/heads/a/b"},
				}
			},
		},
		{
			name: "linked paths sort bytewise without core.ignorecase",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "[core]\n\tignorecase = false\n")
				addLinked(t, commonDir, "lower", filepath.Join(root, "a-wt"), commitA, true)
				addLinked(t, commonDir, "upper", filepath.Join(root, "B-wt"), commitB, true)
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "B-wt"), Head: commitB},
					{Path: filepath.Join(root, "a-wt"), Head: commitA},
				}
			},
		},
		{
			name: "linked paths sort case-folded under core.ignorecase",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "[core]\n\tignorecase = true ; folded\n")
				addLinked(t, commonDir, "lower", filepath.Join(root, "a-wt"), commitA, true)
				addLinked(t, commonDir, "upper", filepath.Join(root, "B-wt"), commitB, true)
				return filepath.Join(root, "repo"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "a-wt"), Head: commitA},
					{Path: filepath.Join(root, "B-wt"), Head: commitB},
				}
			},
		},
		{
			name: "a subdirectory of a linked worktree lists the whole repository",
			build: func(t *testing.T, root string) (string, []Worktree) {
				commonDir := mainRepository(t, filepath.Join(root, "repo"), "")
				addLinked(t, commonDir, "task", filepath.Join(root, "task"), commitB, true)
				mkdirs(t, filepath.Join(root, "task", "src"))
				return filepath.Join(root, "task", "src"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
					{Path: filepath.Join(root, "task"), Head: commitB},
				}
			},
		},
		{
			name:         "a symlinked path to the main worktree lists its resolved path",
			needsSymlink: true,
			build: func(t *testing.T, root string) (string, []Worktree) {
				mainRepository(t, filepath.Join(root, "repo"), "")
				if err := os.Symlink(filepath.Join(root, "repo"), filepath.Join(root, "link")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return filepath.Join(root, "link"), []Worktree{
					{Path: filepath.Join(root, "repo"), Head: commitA, Branch: "refs/heads/main"},
				}
			},
		},
		{
			name: "a reftable repository is ErrRefStorage",
			build: func(t *testing.T, root string) (string, []Worktree) {
				mainRepository(t, filepath.Join(root, "repo"), "[core]\n\tbare = false\n[extensions]\n\trefStorage = reftable\n")
				return filepath.Join(root, "repo"), nil
			},
			wantErrIs: ErrRefStorage,
		},
		{
			name: "a directory that does not exist is an error with no walk-up",
			build: func(t *testing.T, root string) (string, []Worktree) {
				mainRepository(t, filepath.Join(root, "repo"), "")
				return filepath.Join(root, "repo", "gone"), nil
			},
			wantErrIs: fs.ErrNotExist,
		},
		{
			name: "a directory in no repository is ErrNotRepository",
			build: func(t *testing.T, root string) (string, []Worktree) {
				mkdirs(t, filepath.Join(root, "plain"))
				return filepath.Join(root, "plain"), nil
			},
			wantErrIs: ErrNotRepository,
		},
		{
			name: "a bare repository is refused",
			build: func(t *testing.T, root string) (string, []Worktree) {
				bare := filepath.Join(root, "bare.git")
				writeFile(t, filepath.Join(bare, "config"), "[core]\n\tbare = true\n")
				writeFile(t, filepath.Join(bare, "HEAD"), "ref: refs/heads/main\n")
				mkdirs(t, filepath.Join(bare, "objects"), filepath.Join(bare, "refs"))
				return bare, nil
			},
			wantErrContain: "bare repositories are not supported",
		},
	}

	for _, tt := range tests {
		if tt.needsSymlink && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve temp dir: %v", err)
			}
			start, want := tt.build(t, root)

			got, err := ListWorktrees(start)
			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("ListWorktrees(%s) error = %v; want %v", start, err, tt.wantErrIs)
				}
				return
			case tt.wantErrContain != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Fatalf("ListWorktrees(%s) error = %v; want one containing %q", start, err, tt.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListWorktrees(%s): %v", start, err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ListWorktrees(%s) mismatch (-want +got):\n%s", start, diff)
			}
		})
	}
}
