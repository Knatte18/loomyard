//go:build tier1

// seam_integration_test.go proves the worktree seam, installed in fabric, renders what a pull takes in and ignores a Bolt pull.
// It installs the process-wide seam, so it runs without t.Parallel.

package heddleengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// pushHeddleFile commits a heddle.json holding store to the branch of the hub's Bolt checkout on the records remote, through a clone of its own.
func pushHeddleFile(t *testing.T, hub *hubforge.Hub, store heddleengine.Store) {
	t.Helper()
	branch := gitkit.CurrentBranch(t, hub.BoardDir())
	clone := filepath.Join(t.TempDir(), "bolt-clone")
	gitkit.Git(t, filepath.Dir(clone), "clone", "--branch", branch, hub.RecordsBare, clone)
	if err := heddleengine.Save(clone, store); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, clone, "add", "--", heddleengine.FileName)
	gitkit.Git(t, clone, "commit", "-m", "heddle store", "--", heddleengine.FileName)
	gitkit.Git(t, clone, "push", "origin", branch)
}

// pullBolt pulls the hub's Bolt checkout from its remote.
func pullBolt(t *testing.T, hub *hubforge.Hub) {
	t.Helper()
	if _, _, err := fabricengine.NewBolt(hub.BoardDir()).Pull(fabricengine.SyncOptions{}, fabricengine.NewMutations(hub.Path)); err != nil {
		t.Fatalf("Bolt Pull() = %v", err)
	}
}

// stripExcludeLines removes the lines a seeded exclude list left in the exclude file of the repository checked out at worktree.
func stripExcludeLines(t *testing.T, worktree string, paths []string) {
	t.Helper()
	commonDir := gitkit.Git(t, worktree, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(worktree, commonDir)
	}
	excludePath := filepath.Join(commonDir, "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(content), "\n") {
		if !slices.Contains(paths, strings.TrimPrefix(line, "/")) {
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(excludePath, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSeam_PullTakeIn pulls a commit that changes AGENTS.md into a hub whose Bolt checkout holds a codex-file entry, and expects the render to follow the take-in and a Bolt pull to render nothing.
func TestSeam_PullTakeIn(t *testing.T) {
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	second := hubforge.CloneSecond(t, hub)
	entry := func(text string) heddleengine.Store {
		return heddleengine.Store{Entries: []heddleengine.Entry{{Slug: "codex", Text: text, Order: 1, Targets: []string{"codex-file"}}}}
	}
	pushHeddleFile(t, hub, entry("codex text"))
	pullBolt(t, hub)
	t.Cleanup(fabricengine.InstallWorktreeSeam(heddleengine.Seam{}))

	excluded := []string{"CLAUDE.local.md", "AGENTS.override.md", ".heddle"}
	stripExcludeLines(t, hub.PrimeWorktree(), excluded)
	gitkit.CommitFile(t, second.PrimeWorktree(), "AGENTS.md", "new rules\n", "change AGENTS.md")
	gitkit.Git(t, second.PrimeWorktree(), "push", "origin", "HEAD")

	if _, err := hubforge.OpenFabric(t, hub).Pull(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("Pull() = %v", err)
	}
	overridePath := filepath.Join(hub.PrimeWorktree(), "AGENTS.override.md")
	rendered, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("AGENTS.override.md was not rendered by the pull: %v", err)
	}
	if want := heddleengine.Marker + "\n\nnew rules\n\ncodex text\n"; string(rendered) != want {
		t.Errorf("AGENTS.override.md = %q; want %q", rendered, want)
	}
	exclude := gitkit.Git(t, hub.PrimeWorktree(), "rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(hub.PrimeWorktree(), exclude)
	}
	lines, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range excluded {
		if !slices.Contains(strings.Split(string(lines), "\n"), "/"+path) {
			t.Errorf("exclude line for %s is missing after the pull", path)
		}
	}
	if status := gitkit.Git(t, hub.PrimeWorktree(), "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("git status in the prime = %q; want it empty", status)
	}

	pushHeddleFile(t, second, entry("changed text"))
	pullBolt(t, hub)
	after, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(rendered) {
		t.Errorf("AGENTS.override.md after a Bolt pull = %q; want it unchanged", after)
	}
}
