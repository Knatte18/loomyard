//go:build integration

// dotlyxjunction_integration_test.go covers the .lyx junction lifecycle this batch introduces: wiring
// creates a real warp↔weft junction and seeds both sides' git-exclude, unwiring reverses only the
// warp-side wiring, and a pre-existing real warp-side .lyx is adopted into the weft target rather
// than hard-erroring — the branch that makes the very first `reconcile` after this change survive
// against every worktree that predates it.
//
// (a) lifecycle, (b) seeding order, (c) adoption, (d) adoption collision, (e) adoption's refusal
// to over-reach into other real junction-named directories, and (f) adoption's recursive merge of a
// directory present on both sides all live in this one file: (c), (d), (e) and (f) especially must
// be asserted together, because an adoption branch that over-reaches passes (c) while silently
// breaking the hard refusal for _lyx and any other junction name — the guard whose whole purpose is
// never touching what might be the user's hand-authored content — and because (d) and (f) are the
// two halves of one rule, merge a dir/dir collision and refuse every other kind.
//
// Package fabricengine_test to reuse gitkit.GitStatusPorcelain; shares the single TestMain in
// testmain_test.go. readWeftExcludeLines resolves the weft-side exclude file the same way
// seedWeftArtifactExcludes does, mirroring junction_pattern_integration_test.go's readExcludeLines
// for the warp side.
//
// Every case in this file builds its hub via hubforge.NewHub, whose CloneAndWire call already wires
// the prime pair's .lyx junction (and seeds both sides' git-exclude) before any test body runs —
// unlike the old fixture, which never wired anything. resetDotLyxJunction tears that pre-wiring back
// down to a real, unwired starting point wherever a case's own subject is the wire-from-scratch or
// adoption path; the lifecycle and exclude-ordering cases below use it for exactly that reason.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// resetDotLyxJunction removes the prime pair's already-wired .lyx junction so a test can exercise
// seedLyxJunction's creation or adoption path against a genuinely unwired starting point, rather than
// observing WireJunctions's idempotent no-op branch against a hub hubforge.NewHub already wired.
// It returns the warp-side .lyx link path it just cleared, for the caller to reuse.
func resetDotLyxJunction(t *testing.T, l *lyxcwd.Location, slug string) string {
	t.Helper()

	link := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
	if err := fslink.Remove(link); err != nil {
		t.Fatalf("reset .lyx junction at %s: %v", link, err)
	}
	return link
}

// readWeftExcludeLines resolves and reads a weft worktree's .git/info/exclude file, mirroring the
// resolution logic seedWeftArtifactExcludes uses (git rev-parse --git-path info/exclude, joined with
// the weft path if relative).
func readWeftExcludeLines(t *testing.T, weftPath string) []string {
	t.Helper()

	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, weftPath)
	if err != nil || exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude in %s failed: %v (exit %d)", weftPath, err, exitCode)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(weftPath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read weft exclude file: %v", err)
	}
	return strings.Split(string(content), "\n")
}

// TestDotLyxJunction_LifecycleWiresSeedsBothExcludesAndUnwires covers (a): wiring creates the .lyx
// junction pointing at <weft>/<AnchorRel>/.lyx, seeds ".lyx" into the warp's .git/info/exclude AND
// ".lyx/" into the weft's, and unwiring removes the junction and the warp entry.
func TestDotLyxJunction_LifecycleWiresSeedsBothExcludesAndUnwires(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}
	resetDotLyxJunction(t, l, slug)

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	junctions := fabricengine.WarpJunctions(l, slug, names)
	var dotLyx fabricengine.WarpJunction
	for _, j := range junctions {
		if j.Name == lyxdirs.DotLyxDirName {
			dotLyx = j
		}
	}
	if dotLyx.Name == "" {
		t.Fatalf("WarpJunctions(%v) has no %q entry: %+v", names, lyxdirs.DotLyxDirName, junctions)
	}

	wantTarget := filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
	if dotLyx.Target != wantTarget {
		t.Errorf("WarpJunction(.lyx).Target = %q; want %q", dotLyx.Target, wantTarget)
	}

	isLink, err := fslink.IsLink(dotLyx.Link)
	if err != nil || !isLink {
		t.Fatalf(".lyx junction at %s is not a link after WireJunctions: isLink=%v err=%v", dotLyx.Link, isLink, err)
	}
	resolved, err := fslink.PointsTo(dotLyx.Link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", dotLyx.Link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(wantTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", wantTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf(".lyx junction resolves to %q; want %q", resolved, wantResolved)
	}

	warpPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, lyxdirs.DotLyxDirName)
	if lines := readExcludeLines(t, l, slug); !containsLine(lines, warpPattern) {
		t.Fatalf("warp .git/info/exclude does not contain %q after WireJunctions: %v", warpPattern, lines)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	if lines := readWeftExcludeLines(t, weftPath); !containsLine(lines, lyxdirs.DotLyxDirName+"/") {
		t.Fatalf("weft .git/info/exclude does not contain %q after WireJunctions: %v", lyxdirs.DotLyxDirName+"/", lines)
	}

	result, err := fabricengine.UnwireJunctions(l, slug, names)
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}
	if !containsLine(result.JunctionsRemoved, lyxdirs.DotLyxDirName) {
		t.Errorf("UnwireJunctions JunctionsRemoved = %v; want it to contain %q", result.JunctionsRemoved, lyxdirs.DotLyxDirName)
	}
	if _, statErr := os.Lstat(dotLyx.Link); !os.IsNotExist(statErr) {
		t.Errorf(".lyx junction %s still exists after UnwireJunctions", dotLyx.Link)
	}
	if lines := readExcludeLines(t, l, slug); containsLine(lines, lyxdirs.DotLyxDirName) {
		t.Errorf("warp .git/info/exclude still contains %q after UnwireJunctions: %v", lyxdirs.DotLyxDirName, lines)
	}
}

// TestDotLyxJunction_WeftExcludeSeededBeforeFirstWrite covers (b), the regression test for the
// ordering hole: after wiring and before any weft-git verb runs, writing a file into the warp-side
// .lyx must not make `.lyx` itself appear in the weft worktree's `git status --porcelain`. Seeding
// the weft-side exclude only from ensureWeftLockDir would pass every other test in this file while
// leaving scratch as untracked dirt during the window that trips Remove's no-force dirty gate.
//
// This checks for `.lyx` specifically rather than requiring the whole status blank: a real hub's weft
// worktree already carries its own untracked `_lyx/` config — hubforge.NewHub's CloneAndWire
// materializes per-worktree config via configsync.ReconcileAll without committing it — which is
// unrelated dirt this test's own subject, `.lyx`'s exclude ordering, was never about.
func TestDotLyxJunction_WeftExcludeSeededBeforeFirstWrite(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}
	resetDotLyxJunction(t, l, slug)

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpDotLyx := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
	if err := os.WriteFile(filepath.Join(warpDotLyx, "scratch.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatalf("write into warp .lyx: %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	status := gitkit.GitStatusPorcelain(t, weftPath)
	if strings.Contains(status, lyxdirs.DotLyxDirName) {
		t.Errorf("git status --porcelain in weft worktree = %q; want no %q entry (the .lyx exclude must exist before any write)", status, lyxdirs.DotLyxDirName)
	}
}

// TestDotLyxJunction_AdoptsPreExistingRealDotLyx covers (c): a pre-existing real .lyx directory
// holding files is moved into the weft target and replaced by a junction, and a second reconcile
// (WireJunctions re-run) is a no-op.
func TestDotLyxJunction_AdoptsPreExistingRealDotLyx(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}

	warpDotLyx := resetDotLyxJunction(t, l, slug)
	if err := os.MkdirAll(filepath.Join(warpDotLyx, "webster"), 0o755); err != nil {
		t.Fatalf("mkdir pre-existing real .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(warpDotLyx, "webster", "state.json.lock"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("seed pre-existing .lyx content: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions (adoption): %v", err)
	}

	isLink, err := fslink.IsLink(warpDotLyx)
	if err != nil || !isLink {
		t.Fatalf(".lyx at %s is not a junction after adoption: isLink=%v err=%v", warpDotLyx, isLink, err)
	}

	weftDotLyx := filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
	adopted := filepath.Join(weftDotLyx, "webster", "state.json.lock")
	content, err := os.ReadFile(adopted)
	if err != nil {
		t.Fatalf("read adopted content at %s: %v", adopted, err)
	}
	if string(content) != "lock" {
		t.Errorf("adopted content = %q; want %q", string(content), "lock")
	}

	// A second WireJunctions call finds a link, not a real directory, and takes
	// the existing continue-path untouched: it must succeed with no error.
	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions (second, post-adoption) = %v; want nil (idempotent)", err)
	}
	content, err = os.ReadFile(adopted)
	if err != nil {
		t.Fatalf("read adopted content after second WireJunctions: %v", err)
	}
	if string(content) != "lock" {
		t.Errorf("adopted content changed after second WireJunctions: %q", string(content))
	}
}

// TestDotLyxJunction_AdoptionCollisionAbortsAndLeavesBothSidesUntouched covers (d): an entry already
// present in the weft-side target aborts adoption with an error naming the colliding path and leaves
// both sides untouched — the warp directory remains a real directory.
func TestDotLyxJunction_AdoptionCollisionAbortsAndLeavesBothSidesUntouched(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}

	warpDotLyx := resetDotLyxJunction(t, l, slug)
	if err := os.MkdirAll(warpDotLyx, 0o755); err != nil {
		t.Fatalf("mkdir pre-existing real .lyx: %v", err)
	}
	const collidingName = "conflict.txt"
	if err := os.WriteFile(filepath.Join(warpDotLyx, collidingName), []byte("warp copy"), 0o644); err != nil {
		t.Fatalf("seed warp-side colliding entry: %v", err)
	}

	// Materialise the weft target ahead of time with a same-named entry — the
	// "an earlier adoption already ran" shape.
	weftDotLyx := filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)
	if err := os.MkdirAll(weftDotLyx, 0o755); err != nil {
		t.Fatalf("mkdir weft target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(weftDotLyx, collidingName), []byte("weft copy"), 0o644); err != nil {
		t.Fatalf("seed weft-side colliding entry: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug, names)
	if err == nil {
		t.Fatal("WireJunctions with a colliding adoption entry = nil; want an error")
	}
	if !strings.Contains(err.Error(), collidingName) {
		t.Errorf("error %q does not name the colliding entry %q", err.Error(), collidingName)
	}

	// Both sides untouched: the warp directory is still a real, non-link directory...
	isLink, linkErr := fslink.IsLink(warpDotLyx)
	if linkErr != nil {
		t.Fatalf("IsLink(%s): %v", warpDotLyx, linkErr)
	}
	if isLink {
		t.Errorf("%s became a junction despite the aborted collision; want it to remain a real directory", warpDotLyx)
	}
	warpContent, readErr := os.ReadFile(filepath.Join(warpDotLyx, collidingName))
	if readErr != nil {
		t.Fatalf("read warp-side colliding entry after abort: %v", readErr)
	}
	if string(warpContent) != "warp copy" {
		t.Errorf("warp-side colliding entry content changed: %q", string(warpContent))
	}

	// ...and the weft-side copy is untouched too.
	weftContent, readErr := os.ReadFile(filepath.Join(weftDotLyx, collidingName))
	if readErr != nil {
		t.Fatalf("read weft-side colliding entry after abort: %v", readErr)
	}
	if string(weftContent) != "weft copy" {
		t.Errorf("weft-side colliding entry content changed: %q", string(weftContent))
	}
}

// TestDotLyxJunction_AdoptionMergesADirectoryPresentOnBothSides covers (f), R2's regression for the
// permanently-stuck reconcile: a `logs` directory present on BOTH sides must be MERGED, not refused.
//
// This is adoption's steady state, not an exotic input. `lyx fabric unwire` removes the .lyx
// junction, and the very next lyx invocation in that worktree that logs at Info-or-above or exits
// non-zero recreates a real warp-side .lyx/logs through internal/logger's durable sink — on top of
// the logs directory an earlier adoption already moved into weft. Refusing that collision left
// `lyx fabric reconcile`, the documented remedy, unable to heal the pair ever again.
//
// The subtests are deliberately paired with the collision case above, which uses a FILE on both
// sides: merging directories and refusing everything else is one rule, and a fix that merged
// indiscriminately would pass this test while silently destroying the refusal that keeps fabric from
// choosing a winner between two files.
func TestDotLyxJunction_AdoptionMergesADirectoryPresentOnBothSides(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	slug := l.WorktreeName
	names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}

	warpDotLyx := resetDotLyxJunction(t, l, slug)
	weftDotLyx := filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName)

	// The exact shape the residual produces: `logs` on both sides, each holding a distinctly-named
	// trace file, plus a nested subdirectory present on both sides so the merge is proven recursive
	// rather than one level deep.
	for _, dir := range []string{
		filepath.Join(warpDotLyx, "logs", "nested"),
		filepath.Join(weftDotLyx, "logs", "nested"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	seeded := map[string]string{
		filepath.Join(warpDotLyx, "logs", "trace-warp.log"):          "warp trace",
		filepath.Join(warpDotLyx, "logs", "nested", "warp-only.txt"): "warp nested",
		filepath.Join(warpDotLyx, "warp-root.txt"):                   "warp root",
		filepath.Join(weftDotLyx, "logs", "trace-weft.log"):          "weft trace",
		filepath.Join(weftDotLyx, "logs", "nested", "weft-only.txt"): "weft nested",
	}
	for path, content := range seeded {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions with a directory present on both sides = %v; want nil (the merge path)", err)
	}

	// The warp side is a junction again — the whole point: reconcile healed the pair.
	isLink, linkErr := fslink.IsLink(warpDotLyx)
	if linkErr != nil {
		t.Fatalf("IsLink(%s): %v", warpDotLyx, linkErr)
	}
	if !isLink {
		t.Fatalf("%s is not a junction after the merge; the pair is still unhealed", warpDotLyx)
	}

	// Every file from both sides survives, at its merged weft-side path, with its content intact.
	want := map[string]string{
		filepath.Join(weftDotLyx, "logs", "trace-warp.log"):          "warp trace",
		filepath.Join(weftDotLyx, "logs", "trace-weft.log"):          "weft trace",
		filepath.Join(weftDotLyx, "logs", "nested", "warp-only.txt"): "warp nested",
		filepath.Join(weftDotLyx, "logs", "nested", "weft-only.txt"): "weft nested",
		filepath.Join(weftDotLyx, "warp-root.txt"):                   "warp root",
	}
	for path, wantContent := range want {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read merged %s: %v", path, readErr)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("merged %s = %q; want %q", path, string(got), wantContent)
		}
	}
}

// TestDotLyxJunction_AdoptionDoesNotOverreachIntoLyxOrPattern covers (e): a pre-existing real _lyx or
// other config-driven junction directory still produces the hard refusal — the adoption branch must
// never generalise beyond .lyx, since that guard's whole purpose is never touching what might be the
// user's hand-authored content.
func TestDotLyxJunction_AdoptionDoesNotOverreachIntoLyxOrPattern(t *testing.T) {
	cases := []struct {
		name    string
		dirName string
	}{
		{name: "Lyx", dirName: lyxdirs.LyxDirName},
		{name: "Extra", dirName: "_extra"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.NewHub(t, ".")

			l := h.Location
			slug := l.WorktreeName
			names := []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, "_extra"}

			link := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, tt.dirName)
			if err := os.RemoveAll(link); err != nil {
				t.Fatalf("remove pre-seeded %s before test setup: %v", tt.dirName, err)
			}
			if err := os.MkdirAll(link, 0o755); err != nil {
				t.Fatalf("mkdir real warp dir %s: %v", link, err)
			}
			marker := filepath.Join(link, "marker.txt")
			if err := os.WriteFile(marker, []byte("hand-authored content"), 0o644); err != nil {
				t.Fatalf("write marker file: %v", err)
			}

			err := fabricengine.WireJunctions(l, slug, names)
			if err == nil {
				t.Fatalf("WireJunctions with a real %s directory = nil; want a hard refusal", tt.dirName)
			}
			if !strings.Contains(err.Error(), link) {
				t.Errorf("error %q does not name the offending path %q", err.Error(), link)
			}

			content, readErr := os.ReadFile(marker)
			if readErr != nil {
				t.Fatalf("read marker after refused WireJunctions: %v", readErr)
			}
			if string(content) != "hand-authored content" {
				t.Errorf("marker content changed: %q", string(content))
			}
		})
	}
}
