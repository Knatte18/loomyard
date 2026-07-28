//go:build integration

// junction_pattern_integration_test.go covers the per-junction generalisation
// this batch makes to seedLyxJunction, unseedLyxJunction, checkJunctionHealth,
// and PairInSync's inline junction check. Every case here runs against
// HostJunctions' current single _lyx entry, so the whole file is a regression
// suite proving the generalised machinery behaves identically to the old
// _lyx-only code for one junction — the precondition a later batch's flip to
// two junctions depends on.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWireJunctions_MaterialisesMissingWeftTarget is card 6's regression
// guard: seedLyxJunction must create the weft-side target directory when it
// is missing (the checkout/reconcile-left-dangling shape), leaving a junction
// that resolves immediately, and a second WireJunctions call on the same
// worktree must succeed rather than hard-erroring — the bug this card fixes.
func TestWireJunctions_MaterialisesMissingWeftTarget(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	target := l.WeftLyxDirFor(slug)

	// The weft-prime template pre-seeds _lyx/config/placeholder; remove the
	// whole target directory so it genuinely does not exist, matching the
	// state a checkout/reconcile-created junction points at today.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove weft target %s: %v", target, err)
	}

	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions with missing weft target: %v", err)
	}

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("weft target %s not materialised: stat err=%v", target, err)
	}

	link := l.HostLyxLink(slug)
	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
	}
	if _, err := fslink.PointsTo(link); err != nil {
		t.Errorf("junction at %s does not resolve immediately after WireJunctions: %v", link, err)
	}

	// A second WireJunctions on the same worktree must succeed: this is the
	// checkout/reconcile path that hard-errored before this card, because the
	// link-exists branch could not resolve a still-missing target.
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("second WireJunctions = %v; want nil (self-repair path)", err)
	}
}

// TestWireJunctions_RefusesRealHostDirectory is card 7's regression guard: a
// real, non-link directory sitting at the host junction path is still
// refused — fabric never moves or deletes user content — and the returned
// error names both the offending path and the re-run-`lyx init` remedy this
// card's reworded message introduces, replacing the old "migrate via the
// hub-creator" clause that pointed at a tool that does not address this case.
func TestWireJunctions_RefusesRealHostDirectory(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	link := l.HostLyxLink(slug)

	// Seed a real, non-link directory at the host junction path — the
	// "created _lyx by hand" mistake this card's message must guide an
	// operator away from (and, per the batch scope, the same mistake an
	// operator makes hand-authoring _pattern content).
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatalf("mkdir real host dir %s: %v", link, err)
	}
	marker := filepath.Join(link, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	err := fabricengine.WireJunctions(l, slug)
	if err == nil {
		t.Fatal("WireJunctions = nil; want error refusing a real host directory")
	}

	msg := err.Error()
	if !strings.Contains(msg, link) {
		t.Errorf("error %q does not name the offending path %q", msg, link)
	}
	if !strings.Contains(msg, "lyx init") {
		t.Errorf("error %q does not name the re-run-`lyx init` remedy", msg)
	}

	// The real directory and its content must be untouched: fabric never
	// deletes or moves user content on this guard's account.
	content, readErr := os.ReadFile(marker)
	if readErr != nil {
		t.Fatalf("read marker after refused WireJunctions: %v", readErr)
	}
	if string(content) != "real content" {
		t.Errorf("marker content changed: %q", string(content))
	}
}
