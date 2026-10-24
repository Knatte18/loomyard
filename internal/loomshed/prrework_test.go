//go:build tier0

// prrework_test.go holds the committed-plan fixture the Plan-Review skip seam tests share, and exercises the plan reads over it at Tier 1: temp directories and an in-memory ReadCommitted over a map.

package loomshed

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

const (
	reworkTestHead       = "abc123"
	reworkTestRejectedAt = "2026-09-30T10:00:00Z"
)

// reworkGenCard is one card of a generation the fixture writes.
type reworkGenCard struct {
	slug, label, target string
}

// reworkFixture is one rework-round test setup: a committed one-card plan mirrored in the working tree.
type reworkFixture struct {
	t          *testing.T
	anchor     string
	planDir    string
	reworkDir  string
	reviewsDir string
	committed  map[string][]byte
	// amendmentPath is the plan amendment record's path, absent until a test writes it.
	amendmentPath string
}

func newReworkFixture(t *testing.T) *reworkFixture {
	t.Helper()
	anchor := t.TempDir()
	f := &reworkFixture{
		t:          t,
		anchor:     anchor,
		planDir:    planparser.PlanDir(anchor),
		reworkDir:  filepath.Join(anchor, "rework"),
		reviewsDir: filepath.Join(anchor, "reviews"),
		committed:  map[string][]byte{},

		amendmentPath: filepath.Join(anchor, "amendment.yaml"),
	}
	f.writeGeneration(1, reworkGenCard{"first-card", "Create", "internal/firstcard/new.go"})
	f.commitWorkingPlan()
	return f
}

// writeGeneration writes a plan whose cards are numbered from first, into the plan directory.
func (f *reworkFixture) writeGeneration(first int, cards ...reworkGenCard) {
	f.t.Helper()
	if err := os.MkdirAll(f.planDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	plan := plankit.Plan{Approved: true, Language: "none", FirstCard: first, Framing: "Framing."}
	for i, c := range cards {
		plan.Cards = append(plan.Cards, plankit.Card{
			Number:  first + i,
			Slug:    c.slug,
			Summary: fmt.Sprintf("placeholder card %d", first+i),
			Groups:  []plankit.Group{{Label: c.label, Targets: []string{c.target}}},
			Intent:  fmt.Sprintf("card %d.", first+i),
		})
	}
	plankit.Write(f.t, f.planDir, plan)
}

// commitWorkingPlan mirrors the working-tree plan files into the committed map, dropping whatever plan files were committed before.
func (f *reworkFixture) commitWorkingPlan() {
	f.t.Helper()
	prefix := planparser.PlanDirRel() + "/"
	for rel := range f.committed {
		if strings.HasPrefix(rel, prefix) {
			delete(f.committed, rel)
		}
	}
	entries, err := os.ReadDir(f.planDir)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasPrefix(e.Name(), "amendments") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.planDir, e.Name()))
		if err != nil {
			f.t.Fatal(err)
		}
		f.committed[path.Join(planparser.PlanDirRel(), e.Name())] = data
	}
}

// deps returns the told values and the committed-file reader the fixture hands the plan reads.
func (f *reworkFixture) deps() PRReworkDeps {
	return PRReworkDeps{
		PlanDir:       f.planDir,
		ReworkDir:     f.reworkDir,
		ReworkDirRel:  "rework",
		ReviewsDir:    f.reviewsDir,
		AmendmentPath: f.amendmentPath,
		ReadCommitted: func(rel string) ([]byte, bool, error) {
			data, ok := f.committed[rel]
			return data, ok, nil
		},
	}
}

var reworkNewCard = reworkGenCard{"second-card", "Create", "internal/secondcard/new.go"}

func TestNextReworkCardNumber(t *testing.T) {
	f := newReworkFixture(t)
	// The working tree's leftover second card is not committed, so it does not move the number.
	f.writeGeneration(1, reworkGenCard{"first-card", "Create", "internal/firstcard/new.go"}, reworkNewCard)
	reader := func(rel string) ([]byte, bool, error) {
		data, ok := f.committed[rel]
		return data, ok, nil
	}
	got, err := NextReworkCardNumber(f.planDir, reader)
	if err != nil || got != 2 {
		t.Errorf("NextReworkCardNumber() = %d, %v; want 2, nil", got, err)
	}
}
