//go:build integration

// stencilseed_integration_test.go pins the root pre-run's stencil-seed path against real hubs.
// stencilSeedTarget's hub-presence gate is pinned against the defect that a plain repository with no hub-level sibling used to seed a fictional hub, and against a narrowing onto preflight.Wired, which would stop seeding in three real-hub situations that seed correctly today.
// seedStencilsAt is driven directly because seedStencils is a no-op under testing.Testing():
// the first call seeds and commits both subtrees, and a second call writes nothing.
// A command driven through the root is asserted to leave the hub unseeded under go test and never to widen its envelope with a mutations or partial key.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestStencilSeedTarget_PlainRepoHasNoHub is the negative row and the defect this task exists to
// close: a plain git repository with no hub-level sibling beside it must not be treated as a seed
// target, and stencilSeedTarget must not have caused any hub-level _board directory to spring into
// existence beside it.
func TestStencilSeedTarget_PlainRepoHasNoHub(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	gitkit.Git(t, repoDir, "init")

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

// TestStencilSeeding_HubScenario drives seedStencilsAt and stencilSeedTarget against one hubforge hub.
// The steps run serially in this order and the first-seed steps rely on the first seed that the "first seed" step performs; the last step removes a worktree, so it stays last.
// The test calls t.Parallel but no step does, because the steps share the one hub fixture and its mutations.
func TestStencilSeeding_HubScenario(t *testing.T) {
	t.Parallel()

	hub := hubforge.NewHub(t, ".")
	worktree := hub.PrimeWorktree()
	stencilsDir := fabricengine.StencilsDir(hub.Path)
	specsDir := fabricengine.SpecsDir(hub.Path)
	specsSubtreeRel := fabricengine.SpecsSubtreeRel()
	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	specNames := []string{"loom-plan-spec"}

	if _, err := os.Stat(stencilsDir); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: %s already exists", stencilsDir)
	}

	if !t.Run("first seed writes the stencils and commits them", func(t *testing.T) {
		seedStencilsAt(hub.Path, worktree)

		if _, err := os.Stat(discussionPath); err != nil {
			t.Fatalf("stat %s after seedStencilsAt: %v; want the seeded stencil to exist", discussionPath, err)
		}
		if status := gitkit.GitStatusPorcelain(t, hub.BoardDir()); status != "" {
			t.Errorf("git status --porcelain in %s = %q after seedStencilsAt; want a clean tree, the seeded stencils committed", hub.BoardDir(), status)
		}
	}) {
		return
	}

	// Relies on the first seed: the specs subtree is what that call wrote alongside the stencils.
	if !t.Run("first seed also seeds the specs subtree untouched", func(t *testing.T) {
		for _, name := range specNames {
			path := stencilstore.Path(specsDir, name)
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s after seedStencilsAt: %v; want the seeded spec %q to exist", path, err, name)
			}

			hash, ok := stencilstore.ParseStamp(onDisk)
			if !ok || hash == "" {
				t.Errorf("stencilstore.ParseStamp(%s) = %q, %v; want a parseable, non-empty hash", path, hash, ok)
			}

			if state := stencilstore.Classify(onDisk, true, onDisk); state != stencilstore.StateUntouched {
				t.Errorf("stencilstore.Classify(%s) = %v; want StateUntouched immediately after seeding", path, state)
			}
		}

		attrsPath := filepath.Join(specsDir, ".gitattributes")
		if _, err := os.Stat(attrsPath); err != nil {
			t.Errorf("stat %s after seedStencilsAt: %v; want a seeded .gitattributes in the specs subtree", attrsPath, err)
		}
	}) {
		return
	}

	// Relies on the first seed: git ls-files rather than os.Stat catches a write-but-never-stage defect.
	if !t.Run("seeded specs files are tracked in the board repository", func(t *testing.T) {
		stdout, stderr, exitCode, err := gitexec.RunGit([]string{"ls-files", specsSubtreeRel}, hub.BoardDir())
		if err != nil || exitCode != 0 {
			t.Fatalf("git ls-files %s (in %s) failed: %v (exit code %d); stderr: %s", specsSubtreeRel, hub.BoardDir(), err, exitCode, stderr)
		}
		if stdout == "" {
			t.Fatalf("git ls-files %s (in %s) returned nothing; want the seeded specs files to be tracked", specsSubtreeRel, hub.BoardDir())
		}

		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		for _, name := range specNames {
			want := filepath.ToSlash(filepath.Join(specsSubtreeRel, stencilstore.RelPath(name)))
			if !slices.Contains(lines, want) {
				t.Errorf("git ls-files %s (in %s) = %q; want it to list %s", specsSubtreeRel, hub.BoardDir(), stdout, want)
			}
		}
	}) {
		return
	}

	// Relies on the first seed: the second call must leave what the first produced untouched.
	if !t.Run("second seed writes nothing", func(t *testing.T) {
		before := map[string][]byte{discussionPath: nil}
		for _, name := range specNames {
			before[stencilstore.Path(specsDir, name)] = nil
		}
		for path := range before {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s after first seedStencilsAt: %v", path, err)
			}
			before[path] = content
		}

		seedStencilsAt(hub.Path, worktree)

		for path, want := range before {
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s after second seedStencilsAt: %v", path, err)
			}
			if string(got) != string(want) {
				t.Errorf("%s changed on a second seedStencilsAt run; want it byte-identical to the first run's output", path)
			}
		}

		if status := gitkit.GitStatusPorcelain(t, hub.BoardDir()); status != "" {
			t.Errorf("git status --porcelain in %s after a second seedStencilsAt run = %q; want a clean tree", hub.BoardDir(), status)
		}
	}) {
		return
	}

	// Every one of these target rows would fail if the gate were preflight.Wired instead of preflight.HubPresent, which is why they are here.
	targets := []struct {
		name string
		// cwd returns the directory to inject as the resolved cwd; it may mutate the hub, so the rows run in order and the destructive row is last.
		cwd          func(t *testing.T) string
		wantWorktree string
	}{
		{
			name:         "OrdinaryWorktreeInHub",
			cwd:          func(t *testing.T) string { return hub.PrimeWorktree() },
			wantWorktree: hub.PrimeWorktree(),
		},
		{
			name:         "HubLevelBoardDirectory",
			cwd:          func(t *testing.T) string { return hub.BoardDir() },
			wantWorktree: fabricengine.BoardDir(hub.Path),
		},
		{
			name: "WorktreeWithPairedSiblingRemoved",
			cwd: func(t *testing.T) string {
				if err := os.RemoveAll(hub.PrimeRecords()); err != nil {
					t.Fatalf("remove paired-sibling worktree: %v", err)
				}
				return hub.PrimeWorktree()
			},
			wantWorktree: hub.PrimeWorktree(),
		},
	}
	for _, tt := range targets {
		if !t.Run("stencilSeedTarget "+tt.name, func(t *testing.T) {
			ctx := lyxcwd.WithCwd(t.Context(), tt.cwd(t))
			gotHub, gotWorktree, ok := stencilSeedTarget(ctx)
			if !ok {
				t.Fatalf("stencilSeedTarget(%s) ok = false; want true", tt.name)
			}
			if gotHub != hub.Path {
				t.Errorf("stencilSeedTarget(%s) hub = %q; want %q", tt.name, gotHub, hub.Path)
			}
			if gotWorktree != tt.wantWorktree {
				t.Errorf("stencilSeedTarget(%s) worktree = %q; want %q", tt.name, gotWorktree, tt.wantWorktree)
			}
		}) {
			return
		}
	}
}

// TestSeedStencils_NoOpAndEnvelopeUnderRoot drives one read-only command through the root against a real hub and asserts both that seedStencils stays a no-op under testing.Testing() -- without that guard every untagged cmd/lyx test driving a Runnable command would spawn git -- and that the emitted JSON object carries neither a "mutations" nor a "partial" key, so a pre-run seed never widens a command's envelope.
// It chdirs, which is process-global state, so it does not run in parallel.
func TestSeedStencils_NoOpAndEnvelopeUnderRoot(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	stencilsDir := fabricengine.StencilsDir(hub.Path)
	if _, err := os.Stat(stencilsDir); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: %s already exists", stencilsDir)
	}

	t.Chdir(hub.PrimeWorktree())

	var out bytes.Buffer
	code := run([]string{"board", "--board-path", hub.BoardDir(), "list"}, &out)
	if code != 0 {
		t.Fatalf("run([board --board-path ... list]) exit = %d; want 0; output: %s", code, out.String())
	}

	if _, err := os.Stat(stencilsDir); !os.IsNotExist(err) {
		t.Errorf("stat %s after run() under go test = %v; want it still absent -- seedStencils must be a no-op under testing.Testing()", stencilsDir, err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal board list output: %v; output: %s", err, out.String())
	}
	for _, key := range []string{"mutations", "partial"} {
		if _, has := result[key]; has {
			t.Errorf("board list envelope carries a %q key; want none -- a pre-run seed must never widen a command's envelope", key)
		}
	}
}
