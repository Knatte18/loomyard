//go:build integration

// stencilseed_integration_test.go pins stencilSeedTarget's hub-presence gate against the defect this
// task closes -- a plain repository with no hub-level sibling used to seed a fictional hub -- and
// against a narrowing onto preflight.Wired, which would stop seeding in three real-hub situations
// that seed correctly today.
// It sits beside stencilenvelope_integration_test.go, which already drives seedStencils and
// seedStencilsAt end-to-end and stays unedited: both of its assertions run behind the
// testing.Testing() guard and are unaffected by the gate this file exercises.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestStencilSeedTarget_PlainRepoHasNoHub is the negative row and the defect this task exists to
// close: a plain git repository with no hub-level sibling beside it must not be treated as a seed
// target, and stencilSeedTarget must not have caused any hub-level _board directory to spring into
// existence beside it.
func TestStencilSeedTarget_PlainRepoHasNoHub(t *testing.T) {
	repoDir := t.TempDir()
	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, repoDir); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	ctx := lyxcwd.WithCwd(t.Context(), repoDir)
	hub, worktree, ok := stencilSeedTarget(ctx)
	if ok {
		t.Errorf("stencilSeedTarget(plain repo) ok = true, hub = %q, worktree = %q; want ok = false", hub, worktree)
	}

	// The absent path is pinned via fabricengine.BoardDir applied to the repository's parent
	// directory, not a joined "_board" literal, per the geometry-literal enforcement walk.
	wantAbsent := fabricengine.BoardDir(filepath.Dir(repoDir))
	if _, err := os.Stat(wantAbsent); !os.IsNotExist(err) {
		t.Errorf("stat %s after stencilSeedTarget(plain repo) = %v; want it still absent -- a plain repo with no hub-level sibling must never cause a hub-level _board directory to be created", wantAbsent, err)
	}
}

// TestStencilSeedTarget_RealHubCases covers the three positive rows a real hub answers ok = true for.
// Every one of these would fail if the gate were preflight.Wired instead of preflight.HubPresent,
// which is exactly why they are here: without them the gate could be narrowed to always-false in the
// common cases and nothing would catch it.
func TestStencilSeedTarget_RealHubCases(t *testing.T) {
	tests := []struct {
		name string
		// cwd returns the directory to inject as the resolved cwd, and wantWorktree returns the
		// worktree path stencilSeedTarget is expected to return for it.
		cwd          func(t *testing.T, h *hubforge.Hub) string
		wantWorktree func(h *hubforge.Hub) string
	}{
		{
			name: "OrdinaryWorktreeInHub",
			cwd: func(t *testing.T, h *hubforge.Hub) string {
				return h.PrimeWorktree()
			},
			wantWorktree: func(h *hubforge.Hub) string {
				return h.PrimeWorktree()
			},
		},
		{
			name: "HubLevelBoardDirectory",
			cwd: func(t *testing.T, h *hubforge.Hub) string {
				return h.BoardDir()
			},
			wantWorktree: func(h *hubforge.Hub) string {
				return fabricengine.BoardDir(h.Path)
			},
		},
		{
			name: "WorktreeWithPairedSiblingRemoved",
			cwd: func(t *testing.T, h *hubforge.Hub) string {
				if err := os.RemoveAll(h.PrimeWeft()); err != nil {
					t.Fatalf("remove paired-sibling worktree: %v", err)
				}
				return h.PrimeWorktree()
			},
			wantWorktree: func(h *hubforge.Hub) string {
				return h.PrimeWorktree()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.NewHub(t, ".")
			cwd := tt.cwd(t, h)

			ctx := lyxcwd.WithCwd(t.Context(), cwd)
			hub, worktree, ok := stencilSeedTarget(ctx)
			if !ok {
				t.Fatalf("stencilSeedTarget(%s) ok = false; want true", tt.name)
			}
			if hub != h.Path {
				t.Errorf("stencilSeedTarget(%s) hub = %q; want %q", tt.name, hub, h.Path)
			}
			wantWorktree := tt.wantWorktree(h)
			if worktree != wantWorktree {
				t.Errorf("stencilSeedTarget(%s) worktree = %q; want %q", tt.name, worktree, wantWorktree)
			}
		})
	}
}
