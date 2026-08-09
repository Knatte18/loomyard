//go:build integration

// open_integration_test.go covers Open(l) against a real paired fixture: the happy path, and each
// missing-side case built by deleting one half of a lyxtest.CopyPaired fixture, pinning the same
// warp-checked-first contract fabric_test.go's TestNew_MissingWarpPath/TestNew_MissingWeftPath pin
// for New, reached through a *lyxcwd.Location this time.

package fabricengine_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestOpen_HappyPath asserts that Open returns a non-nil handle on a paired fixture.
func TestOpen_HappyPath(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)

	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if f == nil {
		t.Fatal("Open() = nil; want non-nil handle")
	}
}

// TestOpen_MissingWarpWorktree asserts that a missing warp worktree errors, naming the warp path,
// and that the warp side is checked first (ahead of the sibling).
func TestOpen_MissingWarpWorktree(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)

	if err := os.RemoveAll(fixture.Hub); err != nil {
		t.Fatalf("RemoveAll(hub): %v", err)
	}

	_, err := fabricengine.Open(fixture.Layout)
	if err == nil {
		t.Fatal("Open() error = nil; want error naming the missing warp worktree")
	}
	var missingPath *fabricengine.ErrMissingPath
	if !errors.As(err, &missingPath) {
		t.Fatalf("Open() error = %v; want *ErrMissingPath", err)
	}
	if missingPath.Path != fixture.Hub {
		t.Errorf("Open() error path = %q; want %q", missingPath.Path, fixture.Hub)
	}
}

// TestOpen_MissingSiblingWorktree asserts that a missing weft sibling errors, naming the sibling
// path, when the warp worktree is present.
func TestOpen_MissingSiblingWorktree(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)

	if err := os.RemoveAll(fixture.WeftPrime); err != nil {
		t.Fatalf("RemoveAll(weftPrime): %v", err)
	}

	_, err := fabricengine.Open(fixture.Layout)
	if err == nil {
		t.Fatal("Open() error = nil; want error naming the missing sibling worktree")
	}
	var missingPath *fabricengine.ErrMissingPath
	if !errors.As(err, &missingPath) {
		t.Fatalf("Open() error = %v; want *ErrMissingPath", err)
	}
	if missingPath.Path != fixture.WeftPrime {
		t.Errorf("Open() error path = %q; want %q", missingPath.Path, fixture.WeftPrime)
	}
}
