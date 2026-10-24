//go:build tier1

// hubrepourl_integration_test.go proves HubRepoURL's source order over a copied hub:
// the recorded warp binding first, the warp worktree's origin remote second, and an error when neither exists.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestHubRepoURL_BindingThenOriginThenError walks one hub through the three sources in turn, removing each before the next step.
func TestHubRepoURL_BindingThenOriginThenError(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	bindingPath := filepath.Join(h.BoardDir(), fabricengine.WarpBindingFileName)
	const bound = "https://github.com/owner/bound-repo.git"
	if err := os.WriteFile(bindingPath, []byte(bound+"\n"), 0o644); err != nil {
		t.Fatalf("write binding: %v", err)
	}

	got, err := fabricengine.HubRepoURL(h.Location)
	if err != nil || got != bound {
		t.Fatalf("HubRepoURL() with a binding = %q, %v; want %q, nil", got, err, bound)
	}

	if err := os.Remove(bindingPath); err != nil {
		t.Fatalf("remove binding: %v", err)
	}
	origin := gitkit.Git(t, h.Location.WorktreePath(), "remote", "get-url", "origin")
	got, err = fabricengine.HubRepoURL(h.Location)
	if err != nil || got != origin {
		t.Fatalf("HubRepoURL() without a binding = %q, %v; want the warp origin %q, nil", got, err, origin)
	}

	gitkit.MustRun(t, h.Location.WorktreePath(), "git", "remote", "remove", "origin")
	got, err = fabricengine.HubRepoURL(h.Location)
	if err == nil {
		t.Fatalf("HubRepoURL() with neither source = %q, nil; want an error", got)
	}
	for _, source := range []string{fabricengine.WarpBindingFileName, "origin"} {
		if !strings.Contains(err.Error(), source) {
			t.Errorf("HubRepoURL() with neither source error = %q; want it to name %q", err, source)
		}
	}
}
