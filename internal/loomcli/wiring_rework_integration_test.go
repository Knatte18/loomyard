//go:build integration

// wiring_rework_integration_test.go drives wire()'s real plan seams over a real fabric pair,
// so the Plan-Review skip seam reads the rework rounds an earlier run left at the records HEAD.

package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// writeReworkPlan writes a plan generation whose cards are numbered from first, keyed by slug in index order.
func writeReworkPlan(t *testing.T, planDir string, first int, cards [][2]string) {
	t.Helper()
	plan := plankit.Plan{Approved: true, Language: "none", FirstCard: first, Framing: "Framing."}
	for i, c := range cards {
		plan.Cards = append(plan.Cards, plankit.Card{
			Number:  first + i,
			Slug:    c[0],
			Summary: fmt.Sprintf("card %d", first+i),
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{fmt.Sprintf("internal/%s/new.go", strings.ReplaceAll(c[0], "-", ""))}}},
			Intent:  c[1],
		})
	}
	plankit.Write(t, planDir, plan)
}

// writeTestFile writes body at path, creating parent directories.
func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// atoiOrFail parses s, a git count's output, as an integer.
func atoiOrFail(t *testing.T, s string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil {
		t.Fatalf("parse count %q: %v", s, err)
	}
	return n
}

// commitReworkRound leaves what an earlier rework round left at the records HEAD.
// That is the plan generation writePlan writes, committed, and round n's record.json carrying first and class, committed under the rework directory.
func commitReworkRound(t *testing.T, c *loomCLI, location *lyxcwd.Location, n, first int, class string, writePlan func()) {
	t.Helper()
	writePlan()
	if err := c.env.CommitPlan(); err != nil {
		t.Fatalf("CommitPlan() = %v; want nil", err)
	}
	record := fmt.Sprintf(`{"pr_number":3,"head_sha":"abc123","rejected_at":"2026-09-30T10:00:00Z","first_card":%d,"class":%q}`+"\n", first, class)
	writeTestFile(t, filepath.Join(loomengine.LoomReworkDir(location), fmt.Sprintf("round-%d", n), "record.json"), record)
	if _, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LoomReworkDirRel()}, fmt.Sprintf("rework round %d", n), fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit rework round %d: %v", n, err)
	}
}

// TestWire_Real_PlanReviewSkipFollowsGenerationClass asserts Env.SkipPlanReview, over a real fabric pair, answers true after a recorded round whose generation is all-Prosa on .md files and false after one whose generation carries a source card.
func TestWire_Real_PlanReviewSkipFollowsGenerationClass(t *testing.T) {
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "reworkskip"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	if c.env.SkipPlanReview == nil {
		t.Fatal("c.env.SkipPlanReview = nil; want PlanReviewSkippable over the rework deps")
	}
	planDir := planparser.PlanDir(location.AnchorPath())
	writeReworkPlan(t, planDir, 1, [][2]string{{"first-card", "as planned."}})
	if err := c.env.CommitPlan(); err != nil {
		t.Fatalf("CommitPlan() = %v; want nil", err)
	}

	commitReworkRound(t, c, location, 1, 2, "exempt", func() {
		writeReworkPlan(t, planDir, 2, [][2]string{{"docs-card", "words only."}})
		card := "# Card 2 — docs-card\n\n**Prosa:**\n- `docs/note.md`\n\n**Intent:** words only.\n"
		writeTestFile(t, filepath.Join(planDir, "02-docs-card.md"), card)
	})
	if skip, err := c.env.SkipPlanReview(); err != nil || !skip {
		t.Errorf("SkipPlanReview after an all-Prosa generation = %v, %v; want true, nil", skip, err)
	}

	commitReworkRound(t, c, location, 2, 3, "required", func() {
		writeReworkPlan(t, planDir, 3, [][2]string{{"third-card", "changes source."}})
	})
	if skip, err := c.env.SkipPlanReview(); err != nil || skip {
		t.Errorf("SkipPlanReview after a generation with a source card = %v, %v; want false, nil", skip, err)
	}
}
