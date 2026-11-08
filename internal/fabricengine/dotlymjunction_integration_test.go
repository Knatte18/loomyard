// dotlymjunction_integration_test.go covers the .lym junction lifecycle: wiring, unwiring, and adoption.
// Wiring creates a warp↔weft junction and seeds both sides' git-exclude; unwiring reverses only the warp-side wiring.
// Adoption moves a pre-existing real warp-side .lym into the weft target instead of hard-erroring.
//
// The cases are labelled: (a) lifecycle; (b) seeding order; (c) adoption; (d) adoption collision;
// (e) refusal to reach into other real junction-named directories; (f) merge of a directory present on both sides.
// (d) and (f) are one rule: merge a directory collision and refuse every other kind.
// Cases (c) to (f) are asserted together:
// an adoption that over-reaches passes (c) while breaking the hard refusal for _lym and any other junction name.
// That refusal keeps adoption away from the user's hand-authored content.
//
// The file is package fabricengine_test, reusing gitkit.GitStatusPorcelain and the TestMain in testmain_test.go.
//
// Every case builds its hub with hubforge.CopyHub, which relocates a template built through CloneAndWire.
// The prime pair's .lym junction and both sides' git-exclude are seeded before any test body runs.
// resetDotLymJunction removes that junction for a case whose subject is wiring from scratch or adoption.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// resetDotLymJunction removes the prime pair's already-wired .lym junction, so a test starts from an unwired pair.
// Without the reset, wiring finds the junction in place and takes its idempotent no-op branch:
// the creation and adoption paths never run.
// It returns the warp-side .lym link path it cleared, so the caller can reuse it.
func resetDotLymJunction(t *testing.T, l *lymcwd.Location, slug string) string {
	t.Helper()

	link := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)
	if err := fslink.Remove(link); err != nil {
		t.Fatalf("reset .lym junction at %s: %v", link, err)
	}
	return link
}

// TestDotLymJunction_LifecycleWiresSeedsBothExcludesAndUnwires covers the .lym junction's lifecycle.
// Wiring creates the junction pointing at <weft>/<AnchorRel>/.lym.
// It seeds ".lym" into the warp's .git/info/exclude and ".lym/" into the weft's,
// and it seeds "/.scratch" into the warp's.
// Unwiring removes the junction and the warp .lym entry but keeps the scratch line.
//
//testtiming:keep wiring creating the .lym junction to the weft, seeding the warp and weft excludes and the warp "/.scratch" line, and unwiring removing them but the scratch line; coverage of its blocks by other tests does not show an assertion of this
func TestDotLymJunction_LifecycleWiresSeedsBothExcludesAndUnwires(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName}
	resetDotLymJunction(t, l, slug)

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	junctions := fabricengine.WarpJunctions(l, slug, names)
	var dotLym fabricengine.WarpJunction
	for _, j := range junctions {
		if j.Name == lymdirs.DotLymDirName {
			dotLym = j
		}
	}
	if dotLym.Name == "" {
		t.Fatalf("WarpJunctions(%v) has no %q entry: %+v", names, lymdirs.DotLymDirName, junctions)
	}

	wantTarget := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)
	if dotLym.Target != wantTarget {
		t.Errorf("WarpJunction(.lym).Target = %q; want %q", dotLym.Target, wantTarget)
	}

	isLink, err := fslink.IsLink(dotLym.Link)
	if err != nil || !isLink {
		t.Fatalf(".lym junction at %s is not a link after WireJunctions: isLink=%v err=%v", dotLym.Link, isLink, err)
	}
	resolved, err := fslink.PointsTo(dotLym.Link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", dotLym.Link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(wantTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", wantTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf(".lym junction resolves to %q; want %q", resolved, wantResolved)
	}

	warpPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, lymdirs.DotLymDirName)
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); !containsLine(lines, warpPattern) {
		t.Fatalf("warp .git/info/exclude does not contain %q after WireJunctions: %v", warpPattern, lines)
	}

	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); !containsLine(lines, "/.scratch") {
		t.Fatalf("warp .git/info/exclude does not contain %q after WireJunctions: %v", "/.scratch", lines)
	}

	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	if lines := gitkit.ExcludeLines(t, weftPath); !containsLine(lines, lymdirs.DotLymDirName+"/") {
		t.Fatalf("weft .git/info/exclude does not contain %q after WireJunctions: %v", lymdirs.DotLymDirName+"/", lines)
	}

	result, err := fabricengine.UnwireJunctions(l, slug, names)
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}
	if !containsLine(result.JunctionsRemoved, lymdirs.DotLymDirName) {
		t.Errorf("UnwireJunctions JunctionsRemoved = %v; want it to contain %q", result.JunctionsRemoved, lymdirs.DotLymDirName)
	}
	if _, statErr := os.Lstat(dotLym.Link); !os.IsNotExist(statErr) {
		t.Errorf(".lym junction %s still exists after UnwireJunctions", dotLym.Link)
	}
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); containsLine(lines, lymdirs.DotLymDirName) {
		t.Errorf("warp .git/info/exclude still contains %q after UnwireJunctions: %v", lymdirs.DotLymDirName, lines)
	}
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); !containsLine(lines, "/.scratch") {
		t.Errorf("warp .git/info/exclude lost %q after UnwireJunctions: %v", "/.scratch", lines)
	}
}

// TestDotLymJunction_WeftExcludeSeededBeforeFirstWrite checks the weft-side exclude's ordering.
// The check covers the window after wiring and before any weft-git verb runs:
// a file written into the warp-side .lym must not make `.lym` itself appear in the weft's `git status --porcelain`.
// Seeding the weft-side exclude only from `ensureWeftLockDir` would pass every other test in this file.
// It would leave scratch as untracked dirt during the window that trips Remove's no-force dirty gate.
//
// The check looks for `.lym` specifically rather than a blank status:
// a real hub's weft worktree already carries untracked `_lym/` config, which `configsync.ReconcileAll` materializes.
// That dirt is unrelated to this test's subject, `.lym`'s exclude ordering.
//
//testtiming:keep the weft exclude being seeded before the first write so .lym never shows as untracked dirt; coverage of its blocks by other tests does not show an assertion of this
func TestDotLymJunction_WeftExcludeSeededBeforeFirstWrite(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName}
	resetDotLymJunction(t, l, slug)

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpDotLym := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)
	if err := os.WriteFile(filepath.Join(warpDotLym, "scratch.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatalf("write into warp .lym: %v", err)
	}

	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	status := gitkit.GitStatusPorcelain(t, weftPath)
	if strings.Contains(status, lymdirs.DotLymDirName) {
		t.Errorf("git status --porcelain in weft worktree = %q; want no %q entry (the .lym exclude must exist before any write)", status, lymdirs.DotLymDirName)
	}
}

// TestDotLymJunction_AdoptsPreExistingRealDotLym covers adoption of a pre-existing real .lym directory that holds files.
// The directory moves into the weft target and a junction replaces it,
// and a second reconcile (WireJunctions re-run) is a no-op.
//
//testtiming:keep a pre-existing real .lym directory being moved into the weft target and replaced by a junction with a no-op second wiring; coverage of its blocks by other tests does not show an assertion of this
func TestDotLymJunction_AdoptsPreExistingRealDotLym(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName}

	warpDotLym := resetDotLymJunction(t, l, slug)
	if err := os.MkdirAll(filepath.Join(warpDotLym, "webster"), 0o755); err != nil {
		t.Fatalf("mkdir pre-existing real .lym: %v", err)
	}
	if err := os.WriteFile(filepath.Join(warpDotLym, "webster", "state.json.lock"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("seed pre-existing .lym content: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions (adoption): %v", err)
	}

	isLink, err := fslink.IsLink(warpDotLym)
	if err != nil || !isLink {
		t.Fatalf(".lym at %s is not a junction after adoption: isLink=%v err=%v", warpDotLym, isLink, err)
	}

	weftDotLym := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)
	adopted := filepath.Join(weftDotLym, "webster", "state.json.lock")
	content, err := os.ReadFile(adopted)
	if err != nil {
		t.Fatalf("read adopted content at %s: %v", adopted, err)
	}
	if string(content) != "lock" {
		t.Errorf("adopted content = %q; want %q", string(content), "lock")
	}

	// A second WireJunctions call finds a link, not a real directory, and takes the existing continue-path untouched:
	// it must succeed with no error.
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

// TestDotLymJunction_AdoptionCollisionAbortsAndLeavesBothSidesUntouched covers this case:
// an entry already present in the weft-side target makes adoption abort with an error naming the colliding path.
// Both sides stay untouched: the warp directory remains a real directory.
func TestDotLymJunction_AdoptionCollisionAbortsAndLeavesBothSidesUntouched(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName}

	warpDotLym := resetDotLymJunction(t, l, slug)
	if err := os.MkdirAll(warpDotLym, 0o755); err != nil {
		t.Fatalf("mkdir pre-existing real .lym: %v", err)
	}
	const collidingName = "conflict.txt"
	if err := os.WriteFile(filepath.Join(warpDotLym, collidingName), []byte("warp copy"), 0o644); err != nil {
		t.Fatalf("seed warp-side colliding entry: %v", err)
	}

	// Create the weft target ahead of time with a same-named entry, as an earlier adoption would have left it.
	weftDotLym := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)
	if err := os.MkdirAll(weftDotLym, 0o755); err != nil {
		t.Fatalf("mkdir weft target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(weftDotLym, collidingName), []byte("weft copy"), 0o644); err != nil {
		t.Fatalf("seed weft-side colliding entry: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug, names)
	if err == nil {
		t.Fatal("WireJunctions with a colliding adoption entry = nil; want an error")
	}
	if !strings.Contains(err.Error(), collidingName) {
		t.Errorf("error %q does not name the colliding entry %q", err.Error(), collidingName)
	}

	// Both sides untouched: the warp directory is still a real, non-link directory.
	isLink, linkErr := fslink.IsLink(warpDotLym)
	if linkErr != nil {
		t.Fatalf("IsLink(%s): %v", warpDotLym, linkErr)
	}
	if isLink {
		t.Errorf("%s became a junction despite the aborted collision; want it to remain a real directory", warpDotLym)
	}
	warpContent, readErr := os.ReadFile(filepath.Join(warpDotLym, collidingName))
	if readErr != nil {
		t.Fatalf("read warp-side colliding entry after abort: %v", readErr)
	}
	if string(warpContent) != "warp copy" {
		t.Errorf("warp-side colliding entry content changed: %q", string(warpContent))
	}

	// The weft-side copy is untouched too.
	weftContent, readErr := os.ReadFile(filepath.Join(weftDotLym, collidingName))
	if readErr != nil {
		t.Fatalf("read weft-side colliding entry after abort: %v", readErr)
	}
	if string(weftContent) != "weft copy" {
		t.Errorf("weft-side colliding entry content changed: %q", string(weftContent))
	}
}

// TestDotLymJunction_AdoptionMergesADirectoryPresentOnBothSides covers a `logs` directory present on both sides.
// Adoption must merge that directory, not refuse it.
//
// This is adoption's steady state, not an exotic input.
// `lym fabric unwire` removes the .lym junction.
// The next lym invocation in that worktree that logs at Info-or-above or exits non-zero:
// it recreates a real warp-side .lym/logs through internal/logger's durable sink.
// It sits on top of the logs directory an earlier adoption already moved into weft.
// Refusing that collision would leave `lym fabric reconcile`, the documented remedy, unable to heal the pair.
//
// The subtests pair with the refusal of a file present on both sides:
// merging directories and refusing everything else is one rule.
// A fix that merged indiscriminately would pass this test;
// it would silently destroy the refusal that keeps fabric from choosing a winner between two files.
func TestDotLymJunction_AdoptionMergesADirectoryPresentOnBothSides(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName}

	warpDotLym := resetDotLymJunction(t, l, slug)
	weftDotLym := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName)

	// Both sides hold a `logs` directory with a distinctly-named trace file and a nested subdirectory.
	// The nested subdirectory is present on both sides, so the merge must recurse rather than stop one level deep.
	for _, dir := range []string{
		filepath.Join(warpDotLym, "logs", "nested"),
		filepath.Join(weftDotLym, "logs", "nested"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	seeded := map[string]string{
		filepath.Join(warpDotLym, "logs", "trace-warp.log"):          "warp trace",
		filepath.Join(warpDotLym, "logs", "nested", "warp-only.txt"): "warp nested",
		filepath.Join(warpDotLym, "warp-root.txt"):                   "warp root",
		filepath.Join(weftDotLym, "logs", "trace-weft.log"):          "weft trace",
		filepath.Join(weftDotLym, "logs", "nested", "weft-only.txt"): "weft nested",
	}
	for path, content := range seeded {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions with a directory present on both sides = %v; want nil (the merge path)", err)
	}

	// The warp side is a junction again: the merge healed the pair.
	isLink, linkErr := fslink.IsLink(warpDotLym)
	if linkErr != nil {
		t.Fatalf("IsLink(%s): %v", warpDotLym, linkErr)
	}
	if !isLink {
		t.Fatalf("%s is not a junction after the merge; the pair is still unhealed", warpDotLym)
	}

	// Every file from both sides survives, at its merged weft-side path, with its content intact.
	want := map[string]string{
		filepath.Join(weftDotLym, "logs", "trace-warp.log"):          "warp trace",
		filepath.Join(weftDotLym, "logs", "trace-weft.log"):          "weft trace",
		filepath.Join(weftDotLym, "logs", "nested", "warp-only.txt"): "warp nested",
		filepath.Join(weftDotLym, "logs", "nested", "weft-only.txt"): "weft nested",
		filepath.Join(weftDotLym, "warp-root.txt"):                   "warp root",
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

// TestDotLymJunction_AdoptionDoesNotOverreachIntoLymOrPattern checks that the adoption branch covers only .lym.
// A real _lym or other config-driven junction directory still produces the hard refusal.
// The refusal exists so that the tool never touches what might be the user's hand-authored content.
//
//testtiming:keep a user's real _lym or other junction-named directory and its file never adopted, claimed or changed; coverage of its blocks by other tests does not show an assertion of this
func TestDotLymJunction_AdoptionDoesNotOverreachIntoLymOrPattern(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	cases := []struct {
		name    string
		dirName string
	}{
		{name: "Lym", dirName: lymdirs.LymDirName},
		{name: "Extra", dirName: "_extra"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

			l := h.Location
			slug := l.WorktreeName
			names := []string{lymdirs.LymDirName, lymdirs.DotLymDirName, "_extra"}

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
