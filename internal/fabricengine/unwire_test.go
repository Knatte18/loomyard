//go:build integration

// unwire_test.go ports initengine/undo_test.go's coverage to
// fabricengine.Unwire, the new per-worktree teardown verb: full on-disk
// junction removal (including a stale junction absent from the current
// pathspec, proving on-disk-scan enumeration rather than a config
// name-set), weft-side preservation for both _lyx and its optional junction,
// idempotency on a never-wired host, and preservation of the repo-wide
// weft:main records for a later reconcile re-wire.
//
// Package fabricengine_test to reuse newFabricFixture/seedRepoWideFabricConfig
// from reconcile_stale_registration_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestUnwire_RemovesOnDiskJunctionsIncludingStale proves the on-disk-scan enumeration property:
// Unwire removes every fabric junction present on disk for the worktree, including a junction
// (`_extra`) absent from the repo-wide pathspec that a config-driven name-set would have left
// behind.
func TestUnwire_RemovesOnDiskJunctionsIncludingStale(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-removes-stale"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(host): %v", err)
	}

	// Wire the desired trio (_lyx, .lyx, _extra — topology.Add above already wired the
	// repo-wide set, .lyx included) plus a stale name (_other) that is present on disk but
	// absent from any pathspec — Unwire's on-disk scan must remove it too, unlike a
	// config-name-set-driven teardown.
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", ".lyx", "_extra", "_other"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	res, err := fabricengine.Unwire(hostLayout.WorktreePath())
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	got := slices.Clone(res.JunctionsRemoved)
	sort.Strings(got)
	want := []string{"_other", lyxdirs.DotLyxDirName, lyxdirs.LyxDirName, "_extra"}
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("res.JunctionsRemoved (sorted) = %v; want %v", got, want)
	}

	for _, name := range []string{"_other", lyxdirs.DotLyxDirName, lyxdirs.LyxDirName, "_extra"} {
		link := filepath.Join(hostLayout.WorktreePath(), name)
		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s still exists after Unwire (stat err: %v)", link, statErr)
		}
	}
}

// TestUnwire_PreservesWeftLyxAndOptionalContent rewrites the old _lyx-clearing coverage into a
// preservation test: after Unwire, both weft-side `_lyx` and `.lyx` still exist with their content,
// WeftContent reports "preserved", and the weft branch carries no "lyx fabric unwire: clear _lyx"
// commit — the last is asserted by inspecting the weft log, not by counting commits, since an
// unrelated commit landing on the weft branch during setup would otherwise make a bare commit-count
// assertion fragile.
func TestUnwire_PreservesWeftLyxAndOptionalContent(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-preserves-lyx-and-extra"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", ".lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed real content on the weft side of every junction: WireJunctions only
	// materializes the target directories, it writes no files.
	weftLyxDir := fabricengine.WeftLyxDirFor(hostLayout, slug)
	if err := os.WriteFile(filepath.Join(weftLyxDir, "marker.txt"), []byte("lyx state"), 0o644); err != nil {
		t.Fatalf("seed weft _lyx content: %v", err)
	}
	weftDotLyxDir := filepath.Join(fabricengine.WeftWorktreePath(hostLayout, slug), hostLayout.AnchorRel, lyxdirs.DotLyxDirName)
	dotLyxFile := filepath.Join(weftDotLyxDir, "scratch.txt")
	if err := os.WriteFile(dotLyxFile, []byte("scratch state"), 0o644); err != nil {
		t.Fatalf("seed weft .lyx content: %v", err)
	}
	weftExtraDir := filepath.Join(fabricengine.WeftWorktreePath(hostLayout, slug), hostLayout.AnchorRel, "_extra")
	extraFile := filepath.Join(weftExtraDir, "notes.md")
	if err := os.WriteFile(extraFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("seed weft _extra content: %v", err)
	}

	weftWorktree := fabricengine.WeftWorktreePath(hostLayout, slug)
	logBefore, _, exitCode, err := gitexec.RunGit([]string{"log", "--format=%s"}, weftWorktree)
	if err != nil || exitCode != 0 {
		t.Fatalf("git log (before Unwire) failed: %v (exit %d)", err, exitCode)
	}

	res, err := fabricengine.Unwire(hostLayout.WorktreePath())
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if res.WeftContent != "preserved" {
		t.Errorf("res.WeftContent = %q; want %q", res.WeftContent, "preserved")
	}

	// _lyx content is deliberately never touched by Unwire.
	content, err := os.ReadFile(filepath.Join(weftLyxDir, "marker.txt"))
	if err != nil {
		t.Fatalf("read weft _lyx marker after Unwire: %v", err)
	}
	if string(content) != "lyx state" {
		t.Errorf("weft _lyx marker content changed after Unwire: %q", string(content))
	}

	// .lyx content is deliberately never touched by Unwire either.
	content, err = os.ReadFile(dotLyxFile)
	if err != nil {
		t.Fatalf("read weft .lyx scratch file after Unwire: %v", err)
	}
	if string(content) != "scratch state" {
		t.Errorf("weft .lyx scratch content changed after Unwire: %q", string(content))
	}

	// _extra content is deliberately never touched by Unwire — this is the same load-bearing
	// property the _lyx and .lyx assertions above pin, and after this task _lyx/PATTERN.md is
	// exactly the kind of hand-authored content this preservation protects.
	content, err = os.ReadFile(extraFile)
	if err != nil {
		t.Fatalf("read notes.md after Unwire: %v", err)
	}
	if string(content) != "# constraints\n" {
		t.Errorf("notes.md content changed after Unwire: %q", string(content))
	}

	logAfter, _, exitCode, err := gitexec.RunGit([]string{"log", "--format=%s"}, weftWorktree)
	if err != nil || exitCode != 0 {
		t.Fatalf("git log (after Unwire) failed: %v (exit %d)", err, exitCode)
	}
	if logBefore != logAfter {
		t.Errorf("weft log changed by Unwire: before=%q after=%q", logBefore, logAfter)
	}
	if strings.Contains(logAfter, "lyx fabric unwire: clear _lyx") {
		t.Error("weft log carries a \"lyx fabric unwire: clear _lyx\" commit; want none — Unwire must never commit to weft")
	}
}

// TestUnwireVerbResult_HasNoGitignoreField asserts, via reflection, that UnwireVerbResult carries no
// Gitignore field — the CLI envelope (internal/fabriccli/unwire.go's output.Ok map, built directly
// from this struct's fields) can then never carry a "gitignore" key.
func TestUnwireVerbResult_HasNoGitignoreField(t *testing.T) {
	typ := reflect.TypeOf(fabricengine.UnwireVerbResult{})
	if _, ok := typ.FieldByName("Gitignore"); ok {
		t.Errorf("UnwireVerbResult has a Gitignore field; want it removed so the CLI envelope carries no gitignore key")
	}
}

// TestUnwire_NeverWiredHostIsIdempotentNoOp verifies that Unwire is a clean, error-free no-op on a
// host worktree that was never fabric-paired at all — no weft sibling, no junctions — mirroring
// initengine.Undo's TestUndo_NoWeftPairing coverage.
func TestUnwire_NeverWiredHostIsIdempotentNoOp(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	host := lyxtest.CopyHostHub(t)

	res, err := fabricengine.Unwire(host.Hub)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}
	if res.WeftContent != "not_present" {
		t.Errorf("res.WeftContent = %q; want %q", res.WeftContent, "not_present")
	}
	if len(res.JunctionsRemoved) != 0 {
		t.Errorf("res.JunctionsRemoved = %v; want empty", res.JunctionsRemoved)
	}

	// Running it again is a clean, identical no-op.
	res2, err := fabricengine.Unwire(host.Hub)
	if err != nil {
		t.Fatalf("second Unwire() = %v; want nil", err)
	}
	if res2.WeftContent != "not_present" {
		t.Errorf("second res.WeftContent = %q; want %q", res2.WeftContent, "not_present")
	}
}

// TestUnwire_PreservesRepoWideRecords proves Unwire's per-worktree scope: the repo-wide weft:main
// records (.lyx-anchor, <BoardDir>/_lyx/config/fabric.yaml) survive a worktree's Unwire untouched,
// so a later `lyx fabric reconcile` can still re-wire it.
func TestUnwire_PreservesRepoWideRecords(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-preserves-repo-wide-records"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Record the anchor marker alongside the repo-wide fabric.yaml
	// newFabricFixture already seeded, mirroring what fabric clone commits
	// onto weft:main.
	boardDir := fabricengine.BoardDir(l.HubPath)
	anchorPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(".\n"), 0o644); err != nil {
		t.Fatalf("seed .lyx-anchor: %v", err)
	}
	fabricConfigPath := configengine.ConfigFile(boardDir, "fabric")

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	hostLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := fabricengine.Unwire(hostLayout.WorktreePath()); err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if _, statErr := os.Stat(anchorPath); statErr != nil {
		t.Errorf(".lyx-anchor missing after Unwire: %v", statErr)
	}
	if _, statErr := os.Stat(fabricConfigPath); statErr != nil {
		t.Errorf("repo-wide fabric.yaml missing after Unwire: %v", statErr)
	}
	hostLyxLink := filepath.Join(hostLayout.WorktreePath(), lyxdirs.LyxDirName)
	if _, statErr := os.Lstat(hostLyxLink); !os.IsNotExist(statErr) {
		t.Errorf("host _lyx junction %s still exists after Unwire (stat err: %v)", hostLyxLink, statErr)
	}
}
