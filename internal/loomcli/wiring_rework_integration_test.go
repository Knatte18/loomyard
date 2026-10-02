//go:build integration

// wiring_rework_integration_test.go drives wire()'s real plan, webster and rework seams over a real fabric pair,
// so PR-Rework's prompt and append-only check read the plan exactly as the earlier rows left it at weft HEAD.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// reworkPlanCard renders a minimal language: none card file.
func reworkPlanCard(number int, slug, intent string) string {
	return fmt.Sprintf("# Card %d — %s\n\n**Create:**\n- `internal/%s/new.go`\n\n**Intent:** %s\n", number, slug, strings.ReplaceAll(slug, "-", ""), intent)
}

// writeReworkPlan writes the plan's overview and every card in cards, keyed by slug in index order.
func writeReworkPlan(t *testing.T, planDir string, cards [][2]string) {
	t.Helper()
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var index strings.Builder
	for i, c := range cards {
		fmt.Fprintf(&index, "%d — %s — card %d\n", i+1, c[0], i+1)
		name := fmt.Sprintf("%02d-%s.md", i+1, c[0])
		if err := os.WriteFile(filepath.Join(planDir, name), []byte(reworkPlanCard(i+1, c[0], c[1])), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	overview := fmt.Sprintf("---\nformat: 5\napproved: true\nlanguage: none\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n%s", index.String())
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatal(err)
	}
}

// appendingSession is a fake rework session that appends one card and writes its coverage file.
type appendingSession struct {
	t        *testing.T
	planDir  string
	cards    [][2]string
	coverage string
}

func (s appendingSession) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	writeReworkPlan(s.t, s.planDir, s.cards)
	if err := os.WriteFile(s.coverage, []byte("finding 1: card 2\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return shedengine.Done, shedengine.OutputPointer{Path: s.coverage}, nil
}

// TestWire_Real_ReworkAppendsOverWebsterRewrittenPlan asserts a card webster rewrote during its run does not fail PR-Rework's append-only check:
// the Webster row's commit carries the rewrite to weft HEAD, so the round that appends a card after it passes and commits.
// The rework prompt numbers that card from the plan at weft HEAD too.
func TestWire_Real_ReworkAppendsOverWebsterRewrittenPlan(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const slug = "reworkwebster"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	weftSibling := hub.PairWeftSibling(slug)

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	planDir := planparser.PlanDir(location.AnchorPath())

	writeReworkPlan(t, planDir, [][2]string{{"first-card", "as planned."}})
	if err := c.env.CommitPlan(); err != nil {
		t.Fatalf("CommitPlan() = %v; want nil", err)
	}

	// Webster rewrites the card in place during its run, then commits its run record.
	writeReworkPlan(t, planDir, [][2]string{{"first-card", "as bound by webster."}})
	websterDir := websterengine.Dir(location.AnchorPath())
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(websterDir, "summary.md"), []byte("summary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.env.CommitWebster(); err != nil {
		t.Fatalf("CommitWebster() = %v; want nil", err)
	}

	reworkStencil := filepath.Join(fabricengine.StencilsDir(location.HubPath), "loom", "loom-template-rework.md")
	if err := os.MkdirAll(filepath.Dir(reworkStencil), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reworkStencil, stencils.LoomTemplateRework, 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := c.env.ReworkSpec(loomshed.ReworkTold{FirstCard: 2})
	if err != nil {
		t.Fatalf("ReworkSpec() = %v; want nil", err)
	}
	if !strings.Contains(spec.Prompt, "Number the new cards from 2 upward") {
		t.Error("ReworkSpec().Prompt does not number the new cards from 2, one past the committed card")
	}

	deps := c.env.Rework
	deps.ReadRejection = func() (loomshed.PendingRejection, bool, error) {
		return loomshed.PendingRejection{PRNumber: 3, HeadSHA: "abc123", RejectedAt: "2026-09-30T10:00:00Z", Findings: "fix it\n"}, true, nil
	}
	deps.ClearRejection = func() error { return nil }
	deps.Rebaseline = func() error { return nil }
	session := appendingSession{
		t:        t,
		planDir:  planDir,
		cards:    [][2]string{{"first-card", "as bound by webster."}, {"second-card", "fixes the finding."}},
		coverage: filepath.Join(t.TempDir(), "coverage.md"),
	}

	outcome, ptr, err := loomshed.NewPRRework(loomshed.NamePRRework, func(loomshed.ReworkTold) shedengine.ShedProducer { return session }, deps).Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("PR-Rework Call = %v, %q, %v; want Done", outcome, ptr.Reason, err)
	}
	want := planparser.PlanDirRel() + "/02-second-card.md"
	if got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(got, want) {
		t.Errorf("weft HEAD touched %q; want it to include %q", got, want)
	}
}
