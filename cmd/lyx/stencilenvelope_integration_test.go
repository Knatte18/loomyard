//go:build integration

// stencilenvelope_integration_test.go covers the root pre-run seeding path in the two halves the
// testing.Testing() guard in seedStencils splits it into: seedStencilsAt is asserted directly
// (unreachable from an in-process go test any other way), and a command driven through the root is
// asserted to never widen its envelope with a mutations or partial key. It also pins the guard
// itself against silent removal.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestSeedStencilsAt_WritesAndCommits drives seedStencilsAt directly against a hubforge hub whose
// stencils directory is absent, and asserts the stencils were written to disk and committed into the
// board repo.
func TestSeedStencilsAt_WritesAndCommits(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	stencilsDir := fabricengine.StencilsDir(hub.Path)
	if _, err := os.Stat(stencilsDir); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: %s already exists", stencilsDir)
	}

	seedStencilsAt(hub.Path, hub.PrimeWorktree())

	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	if _, err := os.Stat(discussionPath); err != nil {
		t.Fatalf("stat %s after seedStencilsAt: %v; want the seeded stencil to exist", discussionPath, err)
	}

	status := gitkit.GitStatusPorcelain(t, hub.BoardDir())
	if status != "" {
		t.Errorf("git status --porcelain in %s = %q after seedStencilsAt; want a clean tree, the seeded stencils committed", hub.BoardDir(), status)
	}
}

// TestSeedStencils_NoOpUnderTest pins the testing.Testing() guard against removal: without it,
// seedStencils would spawn git as a side effect of every untagged cmd/lyx test that drives a
// Runnable command through the root.
func TestSeedStencils_NoOpUnderTest(t *testing.T) {
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
}

// TestRun_ReadOnlyCommandEnvelopeCarriesNoMutationsKey drives a read-only, non-stencil command
// through the root against a real hub and asserts the emitted JSON object carries neither a
// "mutations" nor a "partial" key -- the guard against the pre-run seeding path widening every
// command's envelope, per the mutation-record-is-logged-not-enveloped-at-the-pre-run Shared
// Decision.
func TestRun_ReadOnlyCommandEnvelopeCarriesNoMutationsKey(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	t.Chdir(hub.PrimeWorktree())

	var out bytes.Buffer
	code := run([]string{"board", "--board-path", hub.BoardDir(), "list"}, &out)
	if code != 0 {
		t.Fatalf("run([board --board-path ... list]) exit = %d; want 0; output: %s", code, out.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal board list output: %v; output: %s", err, out.String())
	}
	if _, has := result["mutations"]; has {
		t.Errorf("board list envelope carries a %q key; want none -- a pre-run seed must never widen a command's envelope", "mutations")
	}
	if _, has := result["partial"]; has {
		t.Errorf("board list envelope carries a %q key; want none -- a pre-run seed must never widen a command's envelope", "partial")
	}
}
