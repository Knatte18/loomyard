// wiring_harden_integration_test.go drives the wiring of a hardener run over a real hub fixture:
// the Harden-Review segment's settings, the campaign seams and the hardener tree scope, which reads a committed diff.

package loomcli

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWire_HardenerRecipe(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "hardened"
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, hub, slug)
	writeHubGateConfig(t, hub.Path, gateConfigWithTwoTiers())
	reviewers := []string{"opus[medium]", "opus[high]", "sonnet[medium]"}
	seedHardenerConfig(t, hub.Path, `reviewer:
  - opus[medium]
  - opus[high]
  - sonnet[medium]
fixer:
  - sonnet[high]
  - opus[low]
judge: opus[low]
seed: sonnet[high]
max_bounces: 3
hard_max_bounces: 10
verify_attempts: 4
tree_scope: {"docs": "", "tools": "./tools/..."}
`)
	loc, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%s) = %v; want nil", hub.PairCodeWorktree(slug), err)
	}
	c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeHardener}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	segment := c.env.Segments[hardenReviewSegment]
	if segment.MaxBounces != 3 || segment.HardMaxBounces != 10 || segment.ConfigFile != "hardener.yaml" {
		t.Errorf("Segments[%s] = %+v; want hardener.yaml's budget 3 and hard cap 10", hardenReviewSegment, segment)
	}
	fixers := []string{"sonnet[high]", "opus[low]"}
	choice := func(spec string) shuttleengine.ModelChoice {
		t.Helper()
		parsed, err := modelspec.Parse(spec)
		if err != nil {
			t.Fatalf("Parse(%q) = %v; want nil", spec, err)
		}
		resolved, err := c.registry.Resolve(parsed)
		if err != nil {
			t.Fatalf("Resolve(%q) = %v; want nil", spec, err)
		}
		return shuttleengine.NewModelChoice(resolved)
	}
	for round := 1; round <= 9; round++ {
		gotReview, gotFix := segment.Models.Pick(round)
		if want := choice(reviewers[(round-1)%len(reviewers)]); gotReview != want {
			t.Errorf("Models.Pick(%d) reviewer = %+v; want entry %d of the list, %+v", round, gotReview, (round-1)%len(reviewers)+1, want)
		}
		if want := choice(fixers[(round-1)%len(fixers)]); gotFix != want {
			t.Errorf("Models.Pick(%d) fixer = %+v; want entry %d of the list, %+v", round, gotFix, (round-1)%len(fixers)+1, want)
		}
	}
	if want := choice("opus[low]"); segment.Bouncer.Judge != want {
		t.Errorf("Segments[%s].Bouncer.Judge = %+v; want hardener.yaml's judge %+v", hardenReviewSegment, segment.Bouncer.Judge, want)
	}
	if want := choice("sonnet[high]"); segment.Bouncer.Seed != want {
		t.Errorf("Segments[%s].Bouncer.Seed = %+v; want hardener.yaml's seed %+v", hardenReviewSegment, segment.Bouncer.Seed, want)
	}
	if c.env.Harden == nil || c.env.PreflightHarden == nil {
		t.Errorf("Env.Harden nil = %v, Env.PreflightHarden nil = %v; want both set for a hardener run", c.env.Harden == nil, c.env.PreflightHarden == nil)
	}

	gitkit.CommitFile(t, hub.PairCodeWorktree(slug), "tools/a.go", "package tools\n", "tools change")
	const wantCall = "lym gate test --wait --gate harden-burler-fix ./tools/..."
	assertTreeScope := func(when string) {
		t.Helper()
		if command, scoped, err := c.env.VerifyTreeScope("harden-burler-fix"); err != nil || !scoped || command != wantCall {
			t.Errorf("VerifyTreeScope(harden-burler-fix) %s = %q, %v, %v; want %q, true, nil", when, command, scoped, err, wantCall)
		}
	}
	assertTreeScope("over hardener.yaml's trees")
	seedDarnHubConfig(t, hub.Path, "9", "tree_scope: {}", `tree_scope: {"elsewhere": ""}`)
	assertTreeScope("after darn.yaml's tree_scope changed")
	if c.env.DarnVerifyAttempts != 4 {
		t.Errorf("Env.DarnVerifyAttempts = %d; want hardener.yaml's 4, whatever darn.yaml says", c.env.DarnVerifyAttempts)
	}

	landing := landingDeps(loc, shedrun.RecipeHardener, c.runDeps.Geom, "task/"+slug, "main", true, c.registry, c.runner, c.landingCfg, c.parentName, c.languageSkills,
		func(string, time.Time) error { return nil }, func() (string, error) { return "", nil }, c.verifySource(), parentLockDeps(loc, shedrun.SelfRunID, "", ""))
	if landing.Reap == nil {
		t.Error("landingDeps(...).Reap is nil for the hardener wiring; want the sandbox reap")
	}
}
