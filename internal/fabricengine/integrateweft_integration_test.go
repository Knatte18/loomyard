//go:build tier1

// integrateweft_integration_test.go covers the weft integration that carries local commits onto a re-anchored upstream, over a plain records repository with a bare origin.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// integrateFixture is a records repository named like a pair's weft, with a bare origin and a second clone standing for the other machine.
type integrateFixture struct {
	t     *testing.T
	weft  string
	other string
}

// newIntegrateFixture commits base, a map of path to content, on the weft and publishes it.
func newIntegrateFixture(t *testing.T, base map[string]string) *integrateFixture {
	t.Helper()

	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	gitkit.Git(t, root, "init", "--bare", "-q", bare)
	weft := filepath.Join(root, "task-weft")
	gitkit.Git(t, root, "init", "-q", weft)
	gitkit.Git(t, weft, "remote", "add", "origin", bare)
	for _, rel := range slices.Sorted(maps.Keys(base)) {
		gitkit.CommitFile(t, weft, rel, base[rel], "base "+rel)
	}
	gitkit.Git(t, weft, "push", "-u", "origin", "main")
	other := filepath.Join(root, "other")
	gitkit.Git(t, root, "clone", "-q", bare, other)
	return &integrateFixture{t: t, weft: weft, other: other}
}

// upstreamReanchors moves the directory from to to on the other machine, sets the marker to the new anchor, pushes, and returns the fetched upstream tip.
func (f *integrateFixture) upstreamReanchors(from, to string) string {
	f.t.Helper()

	if err := os.MkdirAll(filepath.Join(f.other, filepath.Dir(to)), 0o755); err != nil {
		f.t.Fatalf("create the parent of %s: %v", to, err)
	}
	gitkit.Git(f.t, f.other, "mv", from, to)
	if err := os.WriteFile(filepath.Join(f.other, lymcwd.AnchorFileName), []byte(to+"\n"), 0o644); err != nil {
		f.t.Fatalf("write the marker on the other machine: %v", err)
	}
	gitkit.Git(f.t, f.other, "add", "-A")
	gitkit.Git(f.t, f.other, "commit", "-q", "-m", "other machine re-anchors")
	gitkit.Git(f.t, f.other, "push", "-q", "origin", "main")
	gitkit.Git(f.t, f.weft, "fetch", "-q", "origin")
	return gitkit.RevParse(f.t, f.weft, "origin/main")
}

// upstreamCommits commits rel with content on the other machine, pushes, fetches into the weft and returns the fetched upstream tip.
func (f *integrateFixture) upstreamCommits(rel, content string) string {
	f.t.Helper()

	gitkit.CommitFile(f.t, f.other, rel, content, "other machine "+rel)
	gitkit.Git(f.t, f.other, "push", "origin", "main")
	gitkit.Git(f.t, f.weft, "fetch", "-q", "origin")
	return gitkit.RevParse(f.t, f.weft, "origin/main")
}

// files lists the tracked paths of rev in the weft.
func (f *integrateFixture) files(rev string) []string {
	f.t.Helper()

	return strings.Fields(gitkit.Git(f.t, f.weft, "ls-tree", "-r", "--name-only", rev))
}

func TestIntegrateWeft(t *testing.T) {
	t.Parallel()
	names := []string{"_lym"}
	base := map[string]string{lymcwd.AnchorFileName: "a\n", "a/_lym/base.txt": "base\n"}

	// reanchored builds a weft with two local commits at a, whose upstream moved the content to b.
	reanchored := func(t *testing.T) (f *integrateFixture, localOne, localTwo, upstream string) {
		f = newIntegrateFixture(t, base)
		localOne = gitkit.CommitFile(t, f.weft, "a/_lym/one.txt", "one\n", "local one")
		localTwo = gitkit.CommitFile(t, f.weft, "a/_lym/two.txt", "two\n", "local two")
		upstream = f.upstreamReanchors("a", "b")
		return f, localOne, localTwo, upstream
	}

	t.Run("A re-anchored upstream gets both local commits under its anchor", func(t *testing.T) {
		t.Parallel()
		f, localOne, localTwo, upstream := reanchored(t)

		got, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "b", names, "a")
		if err != nil {
			t.Fatalf("IntegrateWeft() error = %v; want nil", err)
		}

		if !slices.Equal(got.Replayed, []string{localOne, localTwo}) || len(got.Skipped) != 0 {
			t.Errorf("IntegrateWeft() replayed %v, skipped %v; want [%s %s] and none", got.Replayed, got.Skipped, localOne, localTwo)
		}
		if head := gitkit.RevParse(t, f.weft, "HEAD"); head != got.Tip {
			t.Errorf("weft HEAD = %s; want the integrated tip %s", head, got.Tip)
		}
		if parent := gitkit.RevParse(t, f.weft, "HEAD~2"); parent != upstream {
			t.Errorf("weft HEAD~2 = %s; want the upstream tip %s under the two replayed commits", parent, upstream)
		}
		want := []string{lymcwd.AnchorFileName, "b/_lym/base.txt", "b/_lym/one.txt", "b/_lym/two.txt"}
		if files := f.files("HEAD"); !slices.Equal(files, want) {
			t.Errorf("tracked paths = %v; want %v", files, want)
		}
		if marker := gitkit.Git(t, f.weft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "b" {
			t.Errorf("marker = %q; want b", marker)
		}
		if _, err := os.Stat(filepath.Join(f.weft, "b", "_lym", "one.txt")); err != nil {
			t.Errorf("the working tree lacks the replayed file: %v", err)
		}
		if message := gitkit.Git(t, f.weft, "log", "-1", "--format=%s", "HEAD"); message != "local two" {
			t.Errorf("replayed commit subject = %q; want the local commit's", message)
		}
	})

	t.Run("A target neither side names moves the upstream's content there too", func(t *testing.T) {
		t.Parallel()
		f, _, _, upstream := reanchored(t)

		got, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "c", names, "a")
		if err != nil {
			t.Fatalf("IntegrateWeft() error = %v; want nil", err)
		}

		want := []string{lymcwd.AnchorFileName, "c/_lym/base.txt", "c/_lym/one.txt", "c/_lym/two.txt"}
		if files := f.files(got.Tip); !slices.Equal(files, want) {
			t.Errorf("tracked paths = %v; want %v", files, want)
		}
		if marker := gitkit.Git(t, f.weft, "show", got.Tip+":"+lymcwd.AnchorFileName); marker != "c" {
			t.Errorf("marker = %q; want c", marker)
		}
	})

	t.Run("A local commit that changed the marker is skipped", func(t *testing.T) {
		t.Parallel()
		f := newIntegrateFixture(t, base)
		localOne := gitkit.CommitFile(t, f.weft, "a/_lym/one.txt", "one\n", "local one")
		localMarker := gitkit.CommitFile(t, f.weft, lymcwd.AnchorFileName, "z\n", "local re-anchor")
		upstream := f.upstreamReanchors("a", "b")

		got, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "b", names, "a")
		if err != nil {
			t.Fatalf("IntegrateWeft() error = %v; want nil", err)
		}

		if !slices.Equal(got.Skipped, []string{localMarker}) || !slices.Equal(got.Replayed, []string{localOne}) {
			t.Errorf("IntegrateWeft() replayed %v, skipped %v; want [%s] and [%s]", got.Replayed, got.Skipped, localOne, localMarker)
		}
		if marker := gitkit.Git(t, f.weft, "show", got.Tip+":"+lymcwd.AnchorFileName); marker != "b" {
			t.Errorf("marker = %q; want the upstream's b", marker)
		}
	})

	t.Run("A markerless local parent re-roots from the fallback anchor", func(t *testing.T) {
		t.Parallel()
		f := newIntegrateFixture(t, map[string]string{"README.md": "readme\n"})
		gitkit.CommitFile(t, f.weft, "a/_lym/local.txt", "local\n", "local at the fallback anchor")
		f.upstreamCommits(lymcwd.AnchorFileName, "b\n")
		upstream := f.upstreamCommits("b/_lym/other.txt", "other\n")

		got, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "b", names, "a")
		if err != nil {
			t.Fatalf("IntegrateWeft() error = %v; want nil", err)
		}

		want := []string{lymcwd.AnchorFileName, "README.md", "b/_lym/local.txt", "b/_lym/other.txt"}
		slices.Sort(want)
		if files := f.files(got.Tip); !slices.Equal(files, want) {
			t.Errorf("tracked paths = %v; want %v", files, want)
		}
	})

	t.Run("A conflicting local commit stops the integration with the weft unchanged", func(t *testing.T) {
		t.Parallel()
		f := newIntegrateFixture(t, base)
		conflicting := gitkit.CommitFile(t, f.weft, "a/_lym/base.txt", "local\n", "local edit")
		gitkit.CommitFile(t, f.weft, "a/_lym/later.txt", "later\n", "local later")
		head := gitkit.RevParse(t, f.weft, "HEAD")
		f.upstreamReanchors("a", "b")
		upstream := f.upstreamCommits("b/_lym/base.txt", "upstream\n")

		_, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "b", names, "a")

		if !errors.Is(err, fabricengine.ErrIntegrateConflict) {
			t.Fatalf("IntegrateWeft() error = %v; want it to wrap ErrIntegrateConflict", err)
		}
		for _, want := range []string{conflicting, "a/_lym/base.txt", "archive/task/" + head, "reset --keep " + conflicting + "^", "lym fabric anchor task --set b"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("IntegrateWeft() error = %q; want it to contain %q", err, want)
			}
		}
		if got := gitkit.RevParse(t, f.weft, "HEAD"); got != head {
			t.Errorf("weft HEAD = %s; want %s unchanged", got, head)
		}
		if status := gitkit.GitStatusPorcelain(t, f.weft); strings.TrimSpace(status) != "" {
			t.Errorf("weft status = %q; want a clean index and working tree", status)
		}
	})

	t.Run("A tracked change on a path the result rewrites refuses at the dirtiness gate", func(t *testing.T) {
		t.Parallel()
		f, _, _, upstream := reanchored(t)
		head := gitkit.RevParse(t, f.weft, "HEAD")
		dirty := filepath.Join(f.weft, "a", "_lym", "one.txt")
		if err := os.WriteFile(dirty, []byte("uncommitted\n"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", dirty, err)
		}

		_, err := fabricengine.IntegrateWeftForTest(f.weft, upstream, "b", names, "a")

		if !errors.Is(err, fabricengine.ErrReplayDirty) {
			t.Fatalf("IntegrateWeft() error = %v; want it to wrap ErrReplayDirty", err)
		}
		if got := gitkit.RevParse(t, f.weft, "HEAD"); got != head {
			t.Errorf("weft HEAD = %s; want %s unchanged", got, head)
		}
		if data, readErr := os.ReadFile(dirty); readErr != nil || string(data) != "uncommitted\n" {
			t.Errorf("uncommitted edit after the refusal = %q, %v; want it kept", data, readErr)
		}
	})
}
