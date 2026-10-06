//go:build integration

// gitscratch_test.go builds the real scratch git repository the integration-tier tests use
// where the behavior under test is git itself: head capture across commits, merge commits, dirtiness, ignore rules and blobs.
// Every other webster test runs over a fakeGit (gitfake_test.go) and spawns no process.

package websterengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// newScratchRepo returns an initialised, empty git repository under t.TempDir() with a committer identity.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitkit.Git(t, dir, "init")
	gitkit.Git(t, dir, "config", "user.name", "Test User")
	gitkit.Git(t, dir, "config", "user.email", "test@example.com")

	return dir
}
