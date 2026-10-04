//go:build integration

// wiring_rework_integration_test.go drives wire()'s real plan, webster and rework seams over a real fabric pair,
// so PR-Rework archives each generation, and its round commit lands, exactly as the earlier rows left the tree at the records HEAD.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
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

// generationSession is a fake rework session that writes the next plan generation and its coverage file.
type generationSession struct {
	t        *testing.T
	planDir  string
	first    int
	cards    [][2]string
	coverage string
}

func (s generationSession) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	writeReworkPlan(s.t, s.planDir, s.first, s.cards)
	if err := os.WriteFile(s.coverage, []byte("finding 1: the new card\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return shedengine.Done, shedengine.OutputPointer{Path: s.coverage}, nil
}

// TestWire_Real_ReworkRoundsArchiveGenerations drives two PR-Rework rounds through wire()'s real seams.
// Each round is one records commit carrying both the moved-from deletions and the archive, webster's directory is emptied,
// and the live plan holds only the newest generation.
func TestWire_Real_ReworkRoundsArchiveGenerations(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const slug = "reworkgenerations"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	recordsSibling := hub.PairWeftSibling(slug)

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	planDir := planparser.PlanDir(location.AnchorPath())

	// Generation 0 is planned, built by webster and reviewed; every row has committed its records.
	writeReworkPlan(t, planDir, 1, [][2]string{{"first-card", "as planned."}})
	if err := c.env.CommitPlan(); err != nil {
		t.Fatalf("CommitPlan() = %v; want nil", err)
	}
	websterDir := websterengine.Dir(location.AnchorPath())
	reviewsDir := loomengine.LoomReviewsDir(location)
	writeTestFile(t, filepath.Join(websterDir, "summary.md"), "summary\n")
	if err := c.env.CommitWebster(); err != nil {
		t.Fatalf("CommitWebster() = %v; want nil", err)
	}
	writeTestFile(t, filepath.Join(reviewsDir, "plan", "report.md"), "plan review\n")
	writeTestFile(t, filepath.Join(reviewsDir, "webster", "report.md"), "webster review\n")
	if _, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LoomReviewsDirRel()}, "reviews", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit reviews: %v", err)
	}

	round := func(n int, rejectedAt string, first int, cards [][2]string) {
		t.Helper()
		before := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
		deps := c.env.Rework
		deps.ReadRejection = func() (loomshed.PendingRejection, bool, error) {
			return loomshed.PendingRejection{PRNumber: 3, HeadSHA: "abc123", RejectedAt: rejectedAt, Findings: "fix it\n"}, true, nil
		}
		deps.ClearRejection = func() error { return nil }
		session := generationSession{t: t, planDir: planDir, first: first, cards: cards, coverage: filepath.Join(t.TempDir(), "coverage.md")}
		outcome, ptr, err := loomshed.NewPRRework(loomshed.NamePRRework, func(loomshed.ReworkTold) shedengine.ShedProducer { return session }, deps).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("round %d: PR-Rework Call = %v, %q, %v; want Done", n, outcome, ptr.Reason, err)
		}
		after := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
		if want := fmt.Sprintf("%d", atoiOrFail(t, before)+1); strings.TrimSpace(after) != want {
			t.Errorf("round %d: records commit count %s -> %s; want exactly one commit", n, strings.TrimSpace(before), strings.TrimSpace(after))
		}
	}

	round1Prior := filepath.ToSlash(filepath.Join(loomengine.LoomReworkDirRel(), "round-1", "prior-generation"))
	round(1, "2026-09-30T10:00:00Z", 2, [][2]string{{"second-card", "fixes the finding."}})
	changed := gitkit.Git(t, recordsSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
	for _, want := range []string{
		"D\t" + planparser.PlanDirRel() + "/01-first-card.md",
		"A\t" + round1Prior + "/plan/01-first-card.md",
		"A\t" + round1Prior + "/webster/summary.md",
		"D\t" + websterengine.DirRel() + "/summary.md",
		"A\t" + round1Prior + "/reviews/plan/report.md",
		"D\t" + filepath.ToSlash(loomengine.LoomReviewsDirRel()) + "/webster/report.md",
		"A\t" + filepath.ToSlash(loomengine.LoomReworkDirRel()) + "/round-1/record.json",
		"A\t" + planparser.PlanDirRel() + "/02-second-card.md",
	} {
		if !strings.Contains(changed, want) {
			t.Errorf("round 1 commit lacks %q; got:\n%s", want, changed)
		}
	}
	if entries, err := os.ReadDir(websterDir); err != nil || len(entries) != 0 {
		t.Errorf("webster dir after round 1 = %v, %v; want empty", entries, err)
	}

	// Webster runs again over generation 1 and records its run.
	writeTestFile(t, filepath.Join(websterDir, "summary.md"), "generation one summary\n")
	if err := c.env.CommitWebster(); err != nil {
		t.Fatalf("CommitWebster() = %v; want nil", err)
	}
	round(2, "2026-09-30T12:00:00Z", 3, [][2]string{{"third-card", "fixes the second finding."}})

	if got := dirEntryNames(t, planDir); strings.Join(got, ",") != "00-overview.md,03-third-card.md" {
		t.Errorf("live plan = %v; want generation 2 only", got)
	}
	if got := dirEntryNames(t, filepath.Join(location.AnchorPath(), loomengine.LoomReworkDirRel(), "round-1", "prior-generation", "plan")); strings.Join(got, ",") != "00-overview.md,01-first-card.md" {
		t.Errorf("round-1 archived plan = %v; want generation 0", got)
	}
	if got := dirEntryNames(t, filepath.Join(location.AnchorPath(), loomengine.LoomReworkDirRel(), "round-2", "prior-generation", "plan")); strings.Join(got, ",") != "00-overview.md,02-second-card.md" {
		t.Errorf("round-2 archived plan = %v; want generation 1", got)
	}
	if entries, err := os.ReadDir(websterDir); err != nil || len(entries) != 0 {
		t.Errorf("webster dir after round 2 = %v, %v; want empty", entries, err)
	}
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

// dirEntryNames lists the entry names of dir.
func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
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

// proseSession is a fake rework session whose new generation is one Prosa card on a markdown file.
type proseSession struct {
	t        *testing.T
	planDir  string
	first    int
	coverage string
}

func (s proseSession) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	writeReworkPlan(s.t, s.planDir, s.first, [][2]string{{"docs-card", "words only."}})
	card := fmt.Sprintf("# Card %d — docs-card\n\n**Prosa:**\n- `docs/note.md`\n\n**Intent:** words only.\n", s.first)
	writeTestFile(s.t, filepath.Join(s.planDir, fmt.Sprintf("%02d-docs-card.md", s.first)), card)
	writeTestFile(s.t, s.coverage, "finding 1: the note\n")
	return shedengine.Done, shedengine.OutputPointer{Path: s.coverage}, nil
}

// TestWire_Real_PlanReviewSkipFollowsGenerationClass asserts Env.SkipPlanReview, over a real fabric pair, answers true after a round whose new generation is all-Prosa on .md files and false after a round whose generation carries an Edit card.
func TestWire_Real_PlanReviewSkipFollowsGenerationClass(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const slug = "reworkskip"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairWarpWorktree(slug))
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

	round := func(rejectedAt string, session shedengine.ShedProducer) {
		t.Helper()
		deps := c.env.Rework
		deps.ReadRejection = func() (loomshed.PendingRejection, bool, error) {
			return loomshed.PendingRejection{PRNumber: 3, HeadSHA: "abc123", RejectedAt: rejectedAt, Findings: "fix it\n"}, true, nil
		}
		deps.ClearRejection = func() error { return nil }
		outcome, ptr, err := loomshed.NewPRRework(loomshed.NamePRRework, func(loomshed.ReworkTold) shedengine.ShedProducer { return session }, deps).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("PR-Rework Call = %v, %q, %v; want Done", outcome, ptr.Reason, err)
		}
	}

	round("2026-09-30T10:00:00Z", proseSession{t: t, planDir: planDir, first: 2, coverage: filepath.Join(t.TempDir(), "coverage.md")})
	if skip, err := c.env.SkipPlanReview(); err != nil || !skip {
		t.Errorf("SkipPlanReview after an all-Prosa generation = %v, %v; want true, nil", skip, err)
	}

	round("2026-09-30T12:00:00Z", generationSession{t: t, planDir: planDir, first: 3, cards: [][2]string{{"third-card", "changes source."}}, coverage: filepath.Join(t.TempDir(), "coverage2.md")})
	if skip, err := c.env.SkipPlanReview(); err != nil || skip {
		t.Errorf("SkipPlanReview after a generation with a source card = %v, %v; want false, nil", skip, err)
	}
}
