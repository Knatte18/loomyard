//go:build integration

// junction_repoint_test.go proves WireJunctions (via seedLyxJunction) repairs
// a corrupted host _lyx junction — one that dangles, or resolves to the wrong
// target — instead of refusing it as a real pre-existing directory. A live
// review round found the pre-fix code fell through the "predates weft"
// refusal for both drift shapes, and Reconcile's documented contract
// ("handles both the missing-junction and the wrong-target cases") was false
// as a result; only the missing-junction case had test coverage
// (TestReconcile_DifferentialEquivalence/JunctionRepointed). This file adds
// the two drift shapes that were unrepairable pre-fix. Fabric-only: warp's
// seedLyxJunction has the same unfixed gap, so a differential harness would
// diverge here.
//
// From card 15 onward, seedLyxJunction's per-junction repair loop runs over
// two junctions (_lyx and _pattern), so this file also carries the _pattern
// counterparts of both repoint cases: the per-junction refusal/repair
// behaviour must hold identically for the second, non-_lyx junction, which
// has no _lyx-shaped shortcut to fall back on.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWireJunctions_RepointsWrongTargetJunction points the host _lyx junction
// at an unrelated (but real) directory instead of the weft _lyx dir, then
// asserts WireJunctions removes and recreates it at the correct target
// instead of refusing it as pre-existing user content.
func TestWireJunctions_RepointsWrongTargetJunction(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostLyxLink(slug)
	correctTarget := l.WeftLyxDirFor(slug)

	// Point the junction at an unrelated real directory instead.
	wrongTarget := filepath.Join(t.TempDir(), "not-the-weft-lyx-dir")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir wrong target: %v", err)
	}
	if err := os.RemoveAll(link); err != nil {
		t.Fatalf("remove existing junction: %v", err)
	}
	if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
		t.Fatalf("seed wrong-target junction: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	resolved, err := fslink.PointsTo(link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(correctTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
	}
}

// TestWireJunctions_RepointsWrongTargetJunction_Pattern is the _pattern
// counterpart of TestWireJunctions_RepointsWrongTargetJunction: the host
// _pattern junction, not _lyx, is pointed at an unrelated real directory, and
// WireJunctions must re-point it at the correct weft _pattern target — the
// same per-junction repair behaviour, exercised against the second junction.
func TestWireJunctions_RepointsWrongTargetJunction_Pattern(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostPatternLink(slug)
	correctTarget := l.WeftPatternDirFor(slug)

	// Point the junction at an unrelated real directory instead.
	wrongTarget := filepath.Join(t.TempDir(), "not-the-weft-pattern-dir")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir wrong target: %v", err)
	}
	if err := os.RemoveAll(link); err != nil {
		t.Fatalf("remove existing junction: %v", err)
	}
	if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
		t.Fatalf("seed wrong-target junction: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	resolved, err := fslink.PointsTo(link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(correctTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
	}
}

// TestWireJunctions_RepointsDanglingJunction points the host _lyx junction at
// a target that does not exist, then asserts WireJunctions removes and
// recreates it at the correct target instead of refusing it.
func TestWireJunctions_RepointsDanglingJunction(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostLyxLink(slug)
	correctTarget := l.WeftLyxDirFor(slug)

	danglingTarget := filepath.Join(t.TempDir(), "does-not-exist")
	if err := os.RemoveAll(link); err != nil {
		t.Fatalf("remove existing junction: %v", err)
	}
	if err := fslink.CreateDirLink(link, danglingTarget); err != nil {
		t.Fatalf("seed dangling junction: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	resolved, err := fslink.PointsTo(link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(correctTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
	}
}

// TestWireJunctions_RepointsDanglingJunction_Pattern is the _pattern
// counterpart of TestWireJunctions_RepointsDanglingJunction: the host
// _pattern junction, not _lyx, dangles (points at a nonexistent target), and
// WireJunctions must re-point it at the correct weft _pattern target rather
// than refusing it — the same per-junction repair behaviour, exercised
// against the second junction.
func TestWireJunctions_RepointsDanglingJunction_Pattern(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostPatternLink(slug)
	correctTarget := l.WeftPatternDirFor(slug)

	danglingTarget := filepath.Join(t.TempDir(), "does-not-exist-pattern")
	if err := os.RemoveAll(link); err != nil {
		t.Fatalf("remove existing junction: %v", err)
	}
	if err := fslink.CreateDirLink(link, danglingTarget); err != nil {
		t.Fatalf("seed dangling junction: %v", err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	resolved, err := fslink.PointsTo(link)
	if err != nil {
		t.Fatalf("PointsTo(%s): %v", link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(correctTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
	}
	if resolved != wantResolved {
		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
	}
}
