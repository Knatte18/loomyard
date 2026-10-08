//go:build integration

// glyphchain_integration_test.go carries each glyph-chain shape row's legs past the plan gate through a real commit's delta, handle binding, the on-disk rewrite and the done-checks, spawning git through Delta — the reason this file carries the integration build tag.
// glyphchain_test.go holds the shape table and the untagged plan-gate legs.

package planglyph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// newGlyphChainRepo returns a delta fixture repository holding a copy of the committed fixture module, and the SHA of the commit that adds it.
func newGlyphChainRepo(t *testing.T) (*deltaFixtureRepo, string) {
	t.Helper()
	f := newDeltaFixtureRepo(t)
	if err := os.CopyFS(f.root, os.DirFS(filepath.Join("testdata", "glyphchain"))); err != nil {
		t.Fatalf("CopyFS(glyphchain fixture) failed: %v", err)
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", "base")
	return f, f.git("rev-parse", "HEAD")
}

// commitEdits applies edits to the fixture repository's files, commits them and returns the commit's SHA.
// Each non-empty old must occur exactly once in its file, so a stale edit fails loudly instead of silently changing nothing.
func (f *deltaFixtureRepo) commitEdits(edits []codeEdit) string {
	f.t.Helper()
	for _, e := range edits {
		full := filepath.Join(f.root, filepath.FromSlash(e.path))
		data, err := os.ReadFile(full)
		if err != nil {
			f.t.Fatalf("read %q: %v", full, err)
		}
		text := string(data)
		if e.old == "" {
			text += e.new
		} else {
			if n := strings.Count(text, e.old); n != 1 {
				f.t.Fatalf("edit of %s: %q occurs %d times; want exactly once", e.path, e.old, n)
			}
			text = strings.Replace(text, e.old, e.new, 1)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			f.t.Fatalf("write %q: %v", full, err)
		}
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", "edit")
	return f.git("rev-parse", "HEAD")
}

// bindCardOne binds card 1's handles of the plan in dir from the delta between base and head, requiring no finding, and returns the plan re-read from disk.
func bindCardOne(t *testing.T, f *deltaFixtureRepo, dir, base, head string) *planparser.Plan {
	t.Helper()
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	delta, err := Delta(f.root, base, head)
	if err != nil {
		t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, base, head, err)
	}
	findings, err := BindHandles(plan, dir, delta, plan.Cards[:1])
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("BindHandles(...) findings = %+v; want none", findings)
	}
	bound, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) after binding returned error: %v", dir, err)
	}
	return bound
}

// requireDoneChecksClean fails unless DoneChecks over cards reports nothing against the fixture repository's current tree.
func requireDoneChecksClean(t *testing.T, plan *planparser.Plan, cards []planparser.Card, root string) {
	t.Helper()
	findings, err := DoneChecks(plan, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("DoneChecks(...) findings = %+v; want none", findings)
	}
}

// TestGlyphChain_Delta runs each shape row's Create, Rename and Delete legs, each in its own repository, past the plan gate through a committed code change, the delta, handle binding and the done-checks.
func TestGlyphChain_Delta(t *testing.T) {
	t.Parallel()

	for _, row := range shapeRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			if row.create != nil {
				t.Run("create", func(t *testing.T) {
					t.Parallel()
					f, base := newGlyphChainRepo(t)
					dir, plan := writeGlyphPlan(t, []string{createCard(row.create), editCard(row.create.draft)})
					assertPlanGate(t, plan, f.root, nil)

					bound := bindCardOne(t, f, dir, base, f.commitEdits(row.createEdits))

					glyph := strings.TrimPrefix(row.create.canonical, "plan:")
					if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "- `"+glyph+"`\n") || strings.Contains(got, "->") {
						t.Errorf("card 1 = %q; want its declaration bullet collapsed to `%s`", got, glyph)
					}
					if got := readCardFile(t, dir, 2, "card2"); !strings.Contains(got, "`"+glyph+"`") || strings.Contains(got, planparser.HandlePrefix) {
						t.Errorf("card 2 = %q; want the bound glyph %q and no handle", got, glyph)
					}
					requireDoneChecksClean(t, bound, bound.Cards[:1], f.root)
				})
			}

			if row.rename != nil {
				t.Run("rename", func(t *testing.T) {
					t.Parallel()
					f, base := newGlyphChainRepo(t)
					dir, plan := writeGlyphPlan(t, []string{renameCard(row.glyph, row.rename.draft)}, renameMechanic)
					assertPlanGate(t, plan, f.root, nil)

					bound := bindCardOne(t, f, dir, base, f.commitEdits(row.renameEdits))

					want := fmt.Sprintf("`%s` -> `%s`", row.glyph, strings.TrimPrefix(row.rename.canonical, "plan:"))
					if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, want) {
						t.Errorf("card 1 = %q; want the pair bound to %s", got, want)
					}
					requireDoneChecksClean(t, bound, bound.Cards[:1], f.root)
				})
			}

			if row.deleteGlyphs != nil {
				t.Run("delete", func(t *testing.T) {
					t.Parallel()
					f, _ := newGlyphChainRepo(t)
					_, plan := writeGlyphPlan(t, []string{deleteCard(row.deleteGlyphs...)})

					f.commitEdits(row.deleteEdits)

					requireDoneChecksClean(t, plan, plan.Cards, f.root)
				})
			}
		})
	}
}

// TestGlyphChain_ReworkGenerations pins the rework-generations scenario: three cards share a file and validate clean at the plan gate, and once card 1's code is bound a dispatch with card 1 completed reports no blocking finding.
func TestGlyphChain_ReworkGenerations(t *testing.T) {
	t.Parallel()

	f, base := newGlyphChainRepo(t)
	dir, plan := writeGlyphPlan(t, []string{
		"**Create:**\n- `plan:shapes#Told` -> `type Told struct{}`\n\n**Edit:**\n- `shapes/shapes.go`\n\n**Intent:** one\n\n**ImpactSummary:** none\n",
		editCard("shapes/shapes.go", "plan:shapes#Told"),
		editCard("shapes/shapes.go"),
	})
	assertPlanGate(t, plan, f.root, nil)

	bound := bindCardOne(t, f, dir, base, f.commitEdits([]codeEdit{appendText(shapesFile, "\ntype Told struct{}\n")}))

	findings, err := ValidateDispatch(bound, f.root, bound.Cards[:1], nil)
	if err != nil {
		t.Fatalf("ValidateDispatch(...) returned error: %v", err)
	}
	for _, finding := range findings {
		if finding.Severity == SeverityBlocking {
			t.Errorf("ValidateDispatch reported blocking finding %+v; want none", finding)
		}
	}
}
