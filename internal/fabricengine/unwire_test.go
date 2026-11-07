// unwire_test.go tests fabricengine.Unwire, the per-worktree teardown verb.
// It covers full on-disk junction removal, including a stale junction absent from the pathspec;
// weft-side preservation of `_lym` and its optional junction; idempotency on a never-wired warp;
// and preservation of the repo-wide weft:main records for a later reconcile re-wire.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

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
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestUnwire_RemovesOnDiskJunctionsIncludingStale proves that Unwire enumerates junctions on disk, not configured names:
// it removes every fabric junction present for the worktree, including `_other`.
// The repo-wide pathspec does not name `_other`, so a config-driven teardown would have left it behind.
// It also leaves the root scratch link and its exclude line in place.
//
//testtiming:keep Unwire removing every on-disk fabric junction including one absent from the pathspec while the root scratch link stays; coverage of its blocks by other tests does not show an assertion of this
func TestUnwire_RemovesOnDiskJunctionsIncludingStale(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "unwire-removes-stale"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	// Set the repo-wide pathspec to "_extra", replacing the template default; Add then wires `_extra`.
	seedRepoWideExtraFabricConfig(t, l.HubPath)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// Wire the desired trio (`_lym`, `.lym`, `_extra`) plus a stale name (`_other`) that is on disk but in no pathspec.
	// Unwire's on-disk scan must remove it too, unlike a teardown driven by the configured names.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", ".lym", "_extra", "_other"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	want := []string{"_other", lymdirs.DotLymDirName, lymdirs.LymDirName, "_extra"}
	sort.Strings(want)

	// The dry run reports exactly what the applied run removes, and removes nothing itself.
	dry, err := fabricengine.Unwire(warpLayout.WorktreePath(), false)
	if err != nil {
		t.Fatalf("dry-run Unwire() = %v; want nil", err)
	}
	dryGot := slices.Clone(dry.JunctionsRemoved)
	sort.Strings(dryGot)
	if !slices.Equal(dryGot, want) {
		t.Errorf("dry run JunctionsRemoved (sorted) = %v; want %v", dryGot, want)
	}
	if dry.GitExclude != "reverted" {
		t.Errorf("dry run GitExclude = %q; want %q", dry.GitExclude, "reverted")
	}
	if dry.Applied {
		t.Error("dry run Applied = true; want false")
	}
	if dry.Mutated().Len() != 0 {
		t.Errorf("dry run Mutations = %v; want none", dry.Mutated().Entries())
	}
	for _, name := range want {
		link := filepath.Join(warpLayout.WorktreePath(), name)
		if _, statErr := os.Lstat(link); statErr != nil {
			t.Errorf("junction %s missing after the dry run: %v", link, statErr)
		}
	}

	res, err := fabricengine.Unwire(warpLayout.WorktreePath(), true)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}
	if !res.Applied {
		t.Error("applied run Applied = false; want true")
	}

	got := slices.Clone(res.JunctionsRemoved)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("res.JunctionsRemoved (sorted) = %v; want %v", got, want)
	}

	for _, name := range []string{"_other", lymdirs.DotLymDirName, lymdirs.LymDirName, "_extra"} {
		link := filepath.Join(warpLayout.WorktreePath(), name)
		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s still exists after Unwire (stat err: %v)", link, statErr)
		}
	}

	scratchRoot := filepath.Join(warpLayout.WorktreePath(), ".scratch")
	wantScratchTarget := filepath.Join(fabricengine.RecordsWorktreePath(warpLayout, slug), warpLayout.AnchorRel, lymdirs.DotLymDirName, "scratch")
	if raw, rawErr := fslink.RawTarget(scratchRoot); rawErr != nil || filepath.Clean(raw) != wantScratchTarget {
		t.Errorf("root .scratch after Unwire: RawTarget = %q, err %v; want a link to %q", raw, rawErr, wantScratchTarget)
	}
	if lines := gitkit.ExcludeLines(t, warpLayout.WorktreePath()); !containsLine(lines, "/.scratch") {
		t.Errorf("warp .git/info/exclude lost %q after Unwire: %v", "/.scratch", lines)
	}
}

// TestUnwire_PreservesWeftLymAndOptionalContent proves that Unwire leaves the weft side alone:
// the weft `_lym` and `.lym` keep their content, WeftContent reports "preserved", and the weft log does not change.
// The log is compared as a whole, not counted, so a setup commit on the weft branch cannot break the check.
//
//testtiming:keep weft-side _lym and .lym surviving Unwire with their content, WeftContent "preserved" and no clear commit in the weft log; coverage of its blocks by other tests does not show an assertion of this
func TestUnwire_PreservesWeftLymAndOptionalContent(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "unwire-preserves-lym-and-extra"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", ".lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed real content on the weft side of every junction: WireJunctions only
	// materializes the target directories, it writes no files.
	weftLymDir := fabricengine.WeftLymDirFor(warpLayout, slug)
	if err := os.WriteFile(filepath.Join(weftLymDir, "marker.txt"), []byte("lym state"), 0o644); err != nil {
		t.Fatalf("seed weft _lym content: %v", err)
	}
	weftDotLymDir := filepath.Join(fabricengine.RecordsWorktreePath(warpLayout, slug), warpLayout.AnchorRel, lymdirs.DotLymDirName)
	dotLymFile := filepath.Join(weftDotLymDir, "scratch.txt")
	if err := os.WriteFile(dotLymFile, []byte("scratch state"), 0o644); err != nil {
		t.Fatalf("seed weft .lym content: %v", err)
	}
	weftExtraDir := filepath.Join(fabricengine.RecordsWorktreePath(warpLayout, slug), warpLayout.AnchorRel, "_extra")
	extraFile := filepath.Join(weftExtraDir, "notes.md")
	if err := os.WriteFile(extraFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("seed weft _extra content: %v", err)
	}

	weftWorktree := fabricengine.RecordsWorktreePath(warpLayout, slug)
	logBefore, _, exitCode, err := gitexec.RunGit([]string{"log", "--format=%s"}, weftWorktree)
	if err != nil || exitCode != 0 {
		t.Fatalf("git log (before Unwire) failed: %v (exit %d)", err, exitCode)
	}

	res, err := fabricengine.Unwire(warpLayout.WorktreePath(), true)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if res.WeftContent != "preserved" {
		t.Errorf("res.WeftContent = %q; want %q", res.WeftContent, "preserved")
	}

	// Unwire never touches the weft `_lym` content.
	content, err := os.ReadFile(filepath.Join(weftLymDir, "marker.txt"))
	if err != nil {
		t.Fatalf("read weft _lym marker after Unwire: %v", err)
	}
	if string(content) != "lym state" {
		t.Errorf("weft _lym marker content changed after Unwire: %q", string(content))
	}

	// Unwire never touches the weft `.lym` content either.
	content, err = os.ReadFile(dotLymFile)
	if err != nil {
		t.Fatalf("read weft .lym scratch file after Unwire: %v", err)
	}
	if string(content) != "scratch state" {
		t.Errorf("weft .lym scratch content changed after Unwire: %q", string(content))
	}

	// Unwire never touches the weft `_extra` content either;
	// the `_lym` and `.lym` assertions above pin the same property.
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
	if strings.Contains(logAfter, "lym fabric unwire: clear _lym") {
		t.Error("weft log carries a \"lym fabric unwire: clear _lym\" commit; want none — Unwire must never commit to weft")
	}
}

// TestUnwireVerbResult_HasNoGitignoreField asserts, via reflection, that UnwireVerbResult carries no
// Gitignore field — the CLI envelope (internal/fabriccli/unwire.go's output.Ok map, built directly
// from this struct's fields) can then never carry a "gitignore" key.
//
//testtiming:keep UnwireVerbResult carrying no Gitignore field so the CLI envelope never gains a gitignore key; coverage of its blocks by other tests does not show an assertion of this
func TestUnwireVerbResult_HasNoGitignoreField(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	typ := reflect.TypeOf(fabricengine.UnwireVerbResult{})
	if _, ok := typ.FieldByName("Gitignore"); ok {
		t.Errorf("UnwireVerbResult has a Gitignore field; want it removed so the CLI envelope carries no gitignore key")
	}
}

// TestUnwire_NeverWiredWarpIsIdempotentNoOp verifies that a second Unwire call is a clean, error-free no-op.
// The prime pair arrives paired and already wired, so the first call does real teardown work.
//
//testtiming:keep a second Unwire call against an already torn-down pair being a clean no-op; coverage of its blocks by other tests does not show an assertion of this
func TestUnwire_NeverWiredWarpIsIdempotentNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	res, err := fabricengine.Unwire(h.PrimeWorktree(), true)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}
	if res.WeftContent != "preserved" {
		t.Errorf("res.WeftContent = %q; want %q (the prime pair's weft sibling is genuinely present)", res.WeftContent, "preserved")
	}
	if len(res.JunctionsRemoved) == 0 {
		t.Error("res.JunctionsRemoved is empty; want the prime pair's already-wired junctions removed")
	}

	// Running it again is a clean, identical no-op: nothing left to remove.
	res2, err := fabricengine.Unwire(h.PrimeWorktree(), true)
	if err != nil {
		t.Fatalf("second Unwire() = %v; want nil", err)
	}
	if res2.WeftContent != "preserved" {
		t.Errorf("second res.WeftContent = %q; want %q", res2.WeftContent, "preserved")
	}
	if len(res2.JunctionsRemoved) != 0 {
		t.Errorf("second res.JunctionsRemoved = %v; want empty", res2.JunctionsRemoved)
	}

	// A dry run on the torn-down pair has nothing to report either.
	dry, err := fabricengine.Unwire(h.PrimeWorktree(), false)
	if err != nil {
		t.Fatalf("dry-run Unwire() = %v; want nil", err)
	}
	if len(dry.JunctionsRemoved) != 0 {
		t.Errorf("dry run JunctionsRemoved = %v; want empty", dry.JunctionsRemoved)
	}
	if dry.GitExclude != "unchanged" {
		t.Errorf("dry run GitExclude = %q; want %q", dry.GitExclude, "unchanged")
	}
}

// TestUnwire_PreservesRepoWideRecords proves Unwire's per-worktree scope: the repo-wide weft:main
// records (.lym-anchor, <BoardDir>/_lym/config/fabric.yaml) survive a worktree's Unwire untouched,
// so a later `lym fabric reconcile` can still re-wire it.
//
//testtiming:keep the repo-wide weft:main records surviving a worktree's Unwire so a later reconcile can re-wire; coverage of its blocks by other tests does not show an assertion of this
func TestUnwire_PreservesRepoWideRecords(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "unwire-preserves-repo-wide-records"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// The prime's weft root holds the committed anchor marker clone and the repo-wide fabric.yaml seeded on weft:main.
	boardDir := fabricengine.BoardDir(l.HubPath)
	anchorPath := filepath.Join(h.PrimeRecords(), lymcwd.AnchorFileName)
	fabricConfigPath := configengine.ConfigFile(boardDir, "fabric")

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := fabricengine.Unwire(warpLayout.WorktreePath(), true); err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if _, statErr := os.Stat(anchorPath); statErr != nil {
		t.Errorf(".lym-anchor missing after Unwire: %v", statErr)
	}
	if _, statErr := os.Stat(fabricConfigPath); statErr != nil {
		t.Errorf("repo-wide fabric.yaml missing after Unwire: %v", statErr)
	}
	warpLymLink := filepath.Join(warpLayout.WorktreePath(), lymdirs.LymDirName)
	if _, statErr := os.Lstat(warpLymLink); !os.IsNotExist(statErr) {
		t.Errorf("warp _lym junction %s still exists after Unwire (stat err: %v)", warpLymLink, statErr)
	}
}

// TestUnwire_LeavesSiblingWorktreeUndirtied pins the cross-worktree scope of Unwire.
// .git/info/exclude resolves to the repo's common gitdir.
// Unwiring one worktree must not strip the exclusion that its siblings' live junctions depend on,
// or every one of them reports untracked dirt in git status and trips Remove's no-force dirty gate.
//
// Each row unwires one pair of a hub whose prime is at ".", after the rows' pairs are re-anchored.
// The row then asserts which exclude lines remain.
// A legacy bare line matches at every depth, so it stays while any worktree wires the name at any anchor;
// an anchored line stays while another worktree links the same anchored path.
func TestUnwire_LeavesSiblingWorktreeUndirtied(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const keptSlug = "unwire-sibling-kept"
	const unwiredSlug = "unwire-sibling-torn-down"

	tests := []struct {
		name          string
		unwiredAnchor string
		keptAnchor    string
		// plantBareLine writes a legacy bare `_lym` line into the exclude file beside the anchored ones.
		plantBareLine bool
		wantKept      []string
		wantStripped  []string
	}{
		{"PairsAtTheHubAnchor", ".", ".", false, []string{"/_lym"}, nil},
		{"UnwiredPairAloneAtItsAnchor", "sub", ".", true, []string{"_lym", "/_lym"}, []string{"/sub/_lym"}},
		{"KeptPairAtTheSameAnchor", "sub", "sub", false, []string{"/sub/_lym"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			for _, slug := range []string{keptSlug, unwiredSlug} {
				hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
			}
			hubforge.ReanchorPair(t, h, keptSlug, tt.keptAnchor)
			hubforge.ReanchorPair(t, h, unwiredSlug, tt.unwiredAnchor)

			keptLayout, err := lymcwd.Resolve(filepath.Join(fabricengine.WorktreePath(l, keptSlug), tt.keptAnchor))
			if err != nil {
				t.Fatalf("lymcwd.Resolve(%s): %v", keptSlug, err)
			}
			unwiredLayout, err := lymcwd.Resolve(filepath.Join(fabricengine.WorktreePath(l, unwiredSlug), tt.unwiredAnchor))
			if err != nil {
				t.Fatalf("lymcwd.Resolve(%s): %v", unwiredSlug, err)
			}

			excludePath := gitkit.Git(t, h.PrimeWorktree(), "rev-parse", "--git-path", "info/exclude")
			if !filepath.IsAbs(excludePath) {
				excludePath = filepath.Join(h.PrimeWorktree(), excludePath)
			}
			if tt.plantBareLine {
				existing, err := os.ReadFile(excludePath)
				if err != nil {
					t.Fatalf("read %s: %v", excludePath, err)
				}
				if err := os.WriteFile(excludePath, append(existing, []byte("_lym\n")...), 0o644); err != nil {
					t.Fatalf("plant the legacy bare line: %v", err)
				}
			}

			others := []string{h.PrimeWorktree(), keptLayout.WorktreePath()}
			before := make([]string, len(others))
			for i, other := range others {
				before[i] = worktreeStatusPorcelain(t, other)
			}
			if before[1] != "" {
				t.Fatalf("setup: kept worktree already dirty: %q", before[1])
			}

			if _, err := fabricengine.Unwire(unwiredLayout.AnchorPath(), true); err != nil {
				t.Fatalf("Unwire(%s): %v", unwiredSlug, err)
			}

			content, err := os.ReadFile(excludePath)
			if err != nil {
				t.Fatalf("read %s: %v", excludePath, err)
			}
			lines := strings.Split(string(content), "\n")
			for _, line := range tt.wantKept {
				if !slices.Contains(lines, line) {
					t.Errorf("exclude lacks %q after the unwire; want it kept for the worktrees still wiring it\n%s", line, content)
				}
			}
			for _, line := range tt.wantStripped {
				if slices.Contains(lines, line) {
					t.Errorf("exclude still holds %q after the unwire; want it stripped\n%s", line, content)
				}
			}
			for i, other := range others {
				if status := worktreeStatusPorcelain(t, other); status != before[i] {
					t.Errorf("git status in %s = %q; want %q — unwiring a sibling stripped its exclude entries", other, status, before[i])
				}
			}
			if isLink, err := fslink.IsLink(fabricengine.WarpLymLinkHere(keptLayout)); err != nil || !isLink {
				t.Errorf("kept worktree's _lym junction: isLink=%v err=%v; want it still wired", isLink, err)
			}
		})
	}
}

// worktreeStatusPorcelain returns the trimmed `git status --porcelain` output for the worktree at
// path, failing the test if git itself cannot run.
func worktreeStatusPorcelain(t *testing.T, path string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, path)
	if err != nil {
		t.Fatalf("git status in %s: %v", path, err)
	}
	if exitCode != 0 {
		t.Fatalf("git status in %s exited %d: %s", path, exitCode, stderr)
	}
	return strings.TrimSpace(stdout)
}
