// recordslayout_integration_test.go covers originNamespaceAt against real repositories:
// the namespace it reads from the origin fetch refspecs, its quiet answer for a missing store, its cache,
// and its error for a records store whose origin config no longer maps the namespace.
// It is an internal-package test because the read and its process-wide cache are unexported.

package fabricengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newLayoutStore initialises a repository at dir with a bare origin.
// It gives the repository the namespaced fetch refspec when namespaced is set.
func newLayoutStore(t *testing.T, dir, bare string, namespaced bool) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitkit.Git(t, dir, "init", "-b", "main")
	gitkit.Git(t, dir, "remote", "add", "origin", bare)
	if namespaced {
		gitkit.Git(t, dir, "config", "--replace-all", "remote.origin.fetch", "+refs/lym/heads/*:refs/remotes/origin/*")
	}
}

// TestOriginNamespaceAt is not parallel: it swaps the process-global log output and reads the process-wide cache.
func TestOriginNamespaceAt(t *testing.T) {
	tiergate.Require(t, 1)

	var logged bytes.Buffer
	logger.SetOutput(&logged)
	t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

	bare := t.TempDir()
	gitkit.Git(t, bare, "init", "--bare", "-b", "main")
	identity := gitrepo.RefNamespace{}
	oneRepository := oneRepositoryNamespace()

	requireNamespace := func(t *testing.T, dir string, want gitrepo.RefNamespace) {
		t.Helper()
		got, err := originNamespaceAt(dir)
		if err != nil || got != want {
			t.Fatalf("originNamespaceAt(%s) = (%+v, %v); want (%+v, nil)", dir, got, err, want)
		}
	}

	t.Run("a missing path and a directory without .git answer identity and log nothing", func(t *testing.T) {
		requireNamespace(t, filepath.Join(t.TempDir(), "missing"), identity)
		requireNamespace(t, t.TempDir(), identity)
		if logged.Len() != 0 {
			t.Errorf("logged %q; want nothing", logged.String())
		}
	})

	t.Run("a store cloned with default refspecs answers identity", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "code")
		newLayoutStore(t, dir, bare, false)
		requireNamespace(t, dir, identity)
	})

	t.Run("a store with the namespaced refspec answers the one-repository namespace, read once", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "records")
		newLayoutStore(t, dir, bare, true)
		requireNamespace(t, dir, oneRepository)

		gitkit.Git(t, dir, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
		requireNamespace(t, dir, oneRepository)
	})

	t.Run("a path read while missing is read again once a namespaced store exists there", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "later")
		requireNamespace(t, dir, identity)
		newLayoutStore(t, dir, bare, true)
		requireNamespace(t, dir, oneRepository)
	})

	t.Run("a repository with no origin answers identity, cached", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "no-origin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		gitkit.Git(t, dir, "init", "-b", "main")
		requireNamespace(t, dir, identity)

		gitkit.Git(t, dir, "remote", "add", "origin", bare)
		gitkit.Git(t, dir, "config", "--replace-all", "remote.origin.fetch", "+refs/lym/heads/*:refs/remotes/origin/*")
		requireNamespace(t, dir, identity)
	})

	t.Run("a records store without its namespaced fetch refspec or its origin is an error", func(t *testing.T) {
		tests := []struct {
			name string
			// spoil faults the store's config and leaves a setting that marks it a records store.
			spoil func(t *testing.T, dir string)
			// wantErrorHas is the way forward the error must name.
			wantErrorHas string
		}{
			{
				name:         "FetchRefspecUnset",
				spoil:        func(t *testing.T, dir string) { gitkit.Git(t, dir, "config", "--unset-all", "remote.origin.fetch") },
				wantErrorHas: "config --replace-all remote.origin.fetch '+refs/lym/heads/*:refs/remotes/origin/*'",
			},
			{
				name: "CodeFetchRefspec",
				spoil: func(t *testing.T, dir string) {
					gitkit.Git(t, dir, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
				},
				wantErrorHas: "config --replace-all remote.origin.fetch '+refs/lym/heads/*:refs/remotes/origin/*'",
			},
			{
				name:         "OriginRemoved",
				spoil:        func(t *testing.T, dir string) { gitkit.Git(t, dir, "remote", "remove", "origin") },
				wantErrorHas: "remote add --no-tags origin <code-url>",
			},
			{
				name: "OnlyABranchMergeSettingLeft",
				spoil: func(t *testing.T, dir string) {
					gitkit.Git(t, dir, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
					gitkit.Git(t, dir, "config", "--unset", fabriclayout.PrimeRecordsBranchKey)
					gitkit.Git(t, dir, "config", "branch.main.merge", "refs/lym/heads/main")
				},
				wantErrorHas: "holds the upstream setting branch.main.merge refs/lym/heads/main, so it is a one-repository records store, " +
					"but no origin fetch refspec maps refs/lym/heads/; way forward: restore its fetch refspecs with ",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "records")
				newLayoutStore(t, dir, bare, true)
				gitkit.Git(t, dir, "config", fabriclayout.PrimeRecordsBranchKey, "main-weft")
				tt.spoil(t, dir)

				got, err := originNamespaceAt(dir)
				if err == nil || !strings.Contains(err.Error(), tt.wantErrorHas) {
					t.Errorf("originNamespaceAt(%s) = (%+v, %v); want an error naming %q", dir, got, err, tt.wantErrorHas)
				}
			})
		}
	})

	t.Run("a failed read is an error and is not cached", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "broken")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		gitFile := filepath.Join(dir, ".git")
		if err := os.WriteFile(gitFile, nil, 0o644); err != nil {
			t.Fatalf("write %s: %v", gitFile, err)
		}
		if got, err := originNamespaceAt(dir); err == nil {
			t.Fatalf("originNamespaceAt(%s) = (%+v, nil); want an error", dir, got)
		}

		if err := os.Remove(gitFile); err != nil {
			t.Fatalf("remove %s: %v", gitFile, err)
		}
		newLayoutStore(t, dir, bare, true)
		requireNamespace(t, dir, oneRepository)
	})
}
