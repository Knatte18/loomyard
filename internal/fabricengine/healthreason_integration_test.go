//go:build integration

// healthreason_integration_test.go is card 5's regression guard for HealthReason: one case per
// HealthCause, asserting Healthy returns the typed cause and its fabric-worded Detail — the
// equivalence loomengine/preflight.go now switches on instead of substring-matching a reason string.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom of
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestHealthy_ReasonCauses exercises all five HealthCause values against a real paired fixture, one
// subtest per cause, and asserts both Cause and Detail — never one "junction-ish" catch-all case.
func TestHealthy_ReasonCauses(t *testing.T) {
	t.Run("BranchMismatch", func(t *testing.T) {
		t.Parallel()

		fixture := newFabricFixture(t)
		l := fixture.Layout
		slug := filepath.Base(fixture.Hub)

		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
			t.Fatalf("WireJunctions: %v", err)
		}
		gitkit.MustRun(t, fixture.Hub, "git", "checkout", "-b", "off-branch")

		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			t.Fatalf("Healthy: %v", err)
		}
		if ok {
			t.Fatalf("Healthy = true; want false (branch mismatch)")
		}
		wantDetail := "fabric out of sync: on off-branch (want off-branch-weft)"
		if reason.Cause != fabricengine.CauseBranchMismatch || reason.Detail != wantDetail {
			t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseBranchMismatch, wantDetail)
		}
	})

	t.Run("ConfigLoadFailed", func(t *testing.T) {
		t.Parallel()

		fixture := newFabricFixture(t)
		l := fixture.Layout
		slug := filepath.Base(fixture.Hub)

		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
			t.Fatalf("WireJunctions: %v", err)
		}
		configPath := configengine.ConfigFile(fabricengine.BoardDir(l.HubPath), "fabric")
		if err := os.WriteFile(configPath, []byte("not: [valid: yaml"), 0o644); err != nil {
			t.Fatalf("corrupt repo-wide fabric config: %v", err)
		}

		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			t.Fatalf("Healthy: %v", err)
		}
		if ok {
			t.Fatalf("Healthy = true; want false (config load failed)")
		}
		if reason.Cause != fabricengine.CauseConfigLoadFailed {
			t.Errorf("Healthy reason.Cause = %q; want %q", reason.Cause, fabricengine.CauseConfigLoadFailed)
		}
		wantPrefix := "junction check unavailable: cannot load fabric.yaml: "
		if len(reason.Detail) < len(wantPrefix) || reason.Detail[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Healthy reason.Detail = %q; want prefix %q", reason.Detail, wantPrefix)
		}
	})

	t.Run("JunctionMissing", func(t *testing.T) {
		t.Parallel()

		fixture := newFabricFixture(t)
		l := fixture.Layout
		slug := filepath.Base(fixture.Hub)

		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
			t.Fatalf("WireJunctions: %v", err)
		}
		link := fabricengine.WarpLyxLinkHere(l)
		if err := fslink.Remove(link); err != nil {
			t.Fatalf("remove junction: %v", err)
		}

		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			t.Fatalf("Healthy: %v", err)
		}
		if ok {
			t.Fatalf("Healthy = true; want false (junction missing)")
		}
		wantDetail := "_lyx junction missing"
		if reason.Cause != fabricengine.CauseJunctionMissing || reason.Detail != wantDetail {
			t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseJunctionMissing, wantDetail)
		}
	})

	t.Run("NotAJunction", func(t *testing.T) {
		t.Parallel()

		fixture := newFabricFixture(t)
		l := fixture.Layout
		slug := filepath.Base(fixture.Hub)

		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
			t.Fatalf("WireJunctions: %v", err)
		}
		link := fabricengine.WarpLyxLinkHere(l)
		if err := fslink.Remove(link); err != nil {
			t.Fatalf("remove junction: %v", err)
		}
		if err := os.Mkdir(link, 0o755); err != nil {
			t.Fatalf("mkdir real dir in junction's place: %v", err)
		}

		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			t.Fatalf("Healthy: %v", err)
		}
		if ok {
			t.Fatalf("Healthy = true; want false (not a junction)")
		}
		wantDetail := "_lyx is not a junction"
		if reason.Cause != fabricengine.CauseNotAJunction || reason.Detail != wantDetail {
			t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseNotAJunction, wantDetail)
		}
	})

	t.Run("JunctionPointsElsewhere", func(t *testing.T) {
		t.Parallel()

		fixture := newFabricFixture(t)
		l := fixture.Layout
		slug := filepath.Base(fixture.Hub)

		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
			t.Fatalf("WireJunctions: %v", err)
		}
		link := fabricengine.WarpLyxLinkHere(l)
		if err := fslink.Remove(link); err != nil {
			t.Fatalf("remove junction: %v", err)
		}
		wrongTarget := filepath.Join(filepath.Dir(fabricengine.WeftLyxDir(l)), "not-the-weft-target")
		if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
			t.Fatalf("mkdir wrong target: %v", err)
		}
		if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
			t.Fatalf("seed wrong-target junction: %v", err)
		}

		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			t.Fatalf("Healthy: %v", err)
		}
		if ok {
			t.Fatalf("Healthy = true; want false (junction points elsewhere)")
		}
		wantDetail := "_lyx junction points elsewhere"
		if reason.Cause != fabricengine.CauseJunctionPointsElsewhere || reason.Detail != wantDetail {
			t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseJunctionPointsElsewhere, wantDetail)
		}
	})
}

// TestHealthy_UnbornWeftBranchIsAVerdictNotAnAbort covers the state a hub is in immediately after
// cloning against an empty remote: the weft primary sits on a branch with zero commits.
// `git rev-parse --abbrev-ref HEAD` exits 128 there, which used to make Healthy return a hard Go
// error — an ABORTED preflight rather than a classified verdict — even though the pair is correctly
// wired and correctly paired.
func TestHealthy_UnbornWeftBranchIsAVerdictNotAnAbort(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	weftWorktree := fabricengine.WeftWorktree(l)
	warpBranch, err := readBranchForTest(t, l.WorktreePath())
	if err != nil {
		t.Fatalf("read warp branch: %v", err)
	}
	weftBranch := fabricengine.WeftBranchName(warpBranch)

	// Re-create the pair's weft branch as an orphan so it carries no commits at all — exactly the
	// shape suffixWeftPrimaryBranch leaves behind on a clone against an empty remote.
	gitkit.MustRun(t, weftWorktree, "git", "checkout", "--orphan", "unborn-scratch")
	gitkit.MustRun(t, weftWorktree, "git", "branch", "-D", weftBranch)
	gitkit.MustRun(t, weftWorktree, "git", "checkout", "--orphan", weftBranch)

	// The orphan checkouts emptied the weft working tree, taking the junction targets with them;
	// re-wiring restores exactly the wiring a real post-clone hub has, leaving the unborn branch as
	// the only difference from a healthy pair.
	names, err := fabricengine.RepoWiredNames(l)
	if err != nil {
		t.Fatalf("RepoWiredNames: %v", err)
	}
	if err := fabricengine.WireJunctions(l, filepath.Base(l.WorktreePath()), names); err != nil {
		t.Fatalf("re-wire junctions: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(l)
	if err != nil {
		t.Fatalf("Healthy on an unborn weft branch = error %v; want a verdict, not an abort", err)
	}
	if !ok {
		t.Errorf("Healthy = false (reason %+v); want true — the pair is correctly wired and paired", reason)
	}
}

// readBranchForTest reads the current branch of the worktree at path for test setup.
func readBranchForTest(t *testing.T, path string) (string, error) {
	t.Helper()
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, path)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", os.ErrInvalid
	}
	return strings.TrimSpace(stdout), nil
}
