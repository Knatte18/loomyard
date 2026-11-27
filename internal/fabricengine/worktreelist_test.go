// worktreelist_test.go covers List against real repositories.
// It covers the prunable flag of a deleted worktree, the bare-repo rejection and the errors.
// The parsing and mapping of worktree records is covered with no git in worktreelist_parse_test.go.

package fabricengine_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestList_RejectsBareRepository asserts that List fails on a bare repository, naming it as bare.
func TestList_RejectsBareRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bareRepo := filepath.Join(t.TempDir(), "bare.git")
	gitkit.MustRun(t, t.TempDir(), "git", "init", "--bare", bareRepo)

	entries, err := fabricengine.List(bareRepo)
	if err == nil {
		t.Fatalf("List() error = nil; want error containing 'bare'")
	}
	if !strings.Contains(err.Error(), "bare") {
		t.Errorf("List() error = %q; want error containing 'bare'", err.Error())
	}
	if entries != nil {
		t.Errorf("List() entries = %v; want nil", entries)
	}
}

// TestList_ParsesPrunable covers the Prunable field that List reports for a deleted worktree.
// A worktree deleted without `git worktree prune` reports Prunable == true.
// The hub's prime entry, which is never deleted, reports Prunable == false.
//
//testtiming:keep List reporting a deleted-but-unpruned worktree as Prunable and the prime as not; coverage of its blocks by other tests does not show an assertion of this
func TestList_ParsesPrunable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hub := h.PrimeWorktree()

	wtPath := filepath.Join(filepath.Dir(hub), "wt-prunable")
	gitkit.MustRun(t, hub, "git", "worktree", "add", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("RemoveAll(%q): %v", wtPath, err)
	}

	entries, err := fabricengine.List(hub)
	if err != nil {
		t.Fatalf("List() error = %v; want nil", err)
	}

	var prunableCount int
	var primePrunable bool
	for _, e := range entries {
		if e.Prunable {
			prunableCount++
		}
		if e.Main {
			primePrunable = e.Prunable
		}
	}
	if prunableCount != 1 {
		t.Errorf("List() prunable entry count = %d; want 1", prunableCount)
	}
	if primePrunable {
		t.Error("List() prime entry Prunable = true; want false")
	}
}

// TestList_Errors asserts that List fails on two directories.
// One is outside any repository, and one does not exist inside one.
// The error names the source directory and why.
//
//testtiming:keep List failing with the source directory and the reason in its error; coverage of its blocks by other tests does not show an assertion of this
func TestList_Errors(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// sourceDir returns the directory List is given.
		sourceDir func(t *testing.T) string
		// wantReason is the error text naming why, empty when wantErrIs names it.
		wantReason string
		wantErrIs  error
	}{
		{
			name:       "OutsideAnyRepository",
			sourceDir:  func(t *testing.T) string { return t.TempDir() },
			wantReason: "not a git repository",
		},
		{
			name: "MissingDirectoryInsideARepository",
			sourceDir: func(t *testing.T) string {
				repo := t.TempDir()
				gitkit.Git(t, repo, "init")
				return filepath.Join(repo, "gone")
			},
			wantErrIs: fs.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sourceDir := tt.sourceDir(t)

			entries, err := fabricengine.List(sourceDir)
			if err == nil {
				t.Fatalf("List(%q) error = nil; want an error", sourceDir)
			}
			if want := fmt.Sprintf("%q", sourceDir); !strings.Contains(err.Error(), want) {
				t.Errorf("List(%q) error = %q; want substring %q", sourceDir, err.Error(), want)
			}
			if tt.wantReason != "" && !strings.Contains(err.Error(), tt.wantReason) {
				t.Errorf("List(%q) error = %q; want substring %q", sourceDir, err.Error(), tt.wantReason)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("List(%q) error = %v; want one wrapping %v", sourceDir, err, tt.wantErrIs)
			}
			if entries != nil {
				t.Errorf("List(%q) entries = %v; want nil", sourceDir, entries)
			}
		})
	}
}
