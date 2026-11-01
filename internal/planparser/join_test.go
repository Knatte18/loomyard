//go:build tier0

// join_test.go covers JoinParts: the joined plan's numbering, rewrites and overview, the export forms ExportMet admits, and each refusal leaving the plan directory unchanged.

package planparser_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// partCard returns a one-group part card with an impact summary, so it passes the format checks on its own.
func partCard(number int, slug, label string, targets ...string) plankit.Card {
	return plankit.Card{
		Number: number, Slug: slug, ImpactSummary: "impact",
		Commit: itoa(number) + ": " + slug,
		Groups: []plankit.Group{{Label: label, Targets: targets}},
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

// writePart writes plan as the part directory <root>/<name> and returns its path.
func writePart(t *testing.T, root, name string, plan plankit.Plan) string {
	t.Helper()
	dir := filepath.Join(root, name)
	plankit.Write(t, dir, plan)
	return dir
}

func TestJoinParts(t *testing.T) {
	t.Parallel()

	const (
		resignArrow  = "internal/b#Member` -> `func Member(x int)"
		declaration  = "plan:internal/d#T.Foo` -> `func (t *T) Foo()"
		renameBullet = "`internal/e#Old` -> `plan:internal/e#New`"
	)

	t.Run("parts join into one numbered plan", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		worktree := plankit.Repo(t, map[string]string{"internal/a/existing.go": "package a\n", "internal/c/gone.go": "package c\n"})
		parts := []planparser.JoinPart{
			{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{
				Sections: []plankit.Section{{Heading: "Shared Decisions", Body: "### Decision: alpha-choice\n\nAlpha decided."}},
				Cards: []plankit.Card{
					partCard(1, "first", "Create", "internal/a/one.go"),
					func() plankit.Card {
						card := partCard(2, "second", "Edit", "internal/a/existing.go")
						card.Intent = "Build on 1-first."
						return card
					}(),
				},
			}), Exports: []string{"internal/a/one.go"}},
			{Name: "beta", Dir: writePart(t, root, "beta", plankit.Plan{Cards: []plankit.Card{partCard(1, "resign", "Edit", resignArrow)}}), Exports: []string{"internal/b#Member"}},
			{Name: "gamma", Dir: writePart(t, root, "gamma", plankit.Plan{Cards: []plankit.Card{partCard(1, "remove", "Delete", "internal/c/gone.go")}}), Exports: []string{"internal/c/gone.go"}},
			{Name: "delta", Dir: writePart(t, root, "delta", plankit.Plan{Cards: []plankit.Card{partCard(1, "declare", "Create", declaration)}}), Exports: []string{"plan:internal/d#Foo"}},
			{Name: "epsilon", Dir: writePart(t, root, "epsilon", plankit.Plan{
				Sections: []plankit.Section{{Heading: "Rename mechanic", Body: "this part's own text"}},
				Cards:    []plankit.Card{partCard(1, "rename", "Rename", renameBullet)},
			})},
		}

		planDir := filepath.Join(t.TempDir(), "plan")
		if err := os.MkdirAll(filepath.Join(planDir, "archive-old"), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		for name, content := range map[string]string{"09-stale.md": "stale", "00-overview.md": "stale", "archive-old/00-overview.md": "kept"} {
			if err := os.WriteFile(filepath.Join(planDir, name), []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
			}
		}

		result, err := planparser.JoinParts(planDir, "Joined task", parts)
		if err != nil {
			t.Fatalf("JoinParts() = _, %v; want nil error", err)
		}

		wantCards := []planparser.JoinedCard{
			{Part: "alpha", From: 1, To: 1, File: "01-first.md"}, {Part: "alpha", From: 2, To: 2, File: "02-second.md"},
			{Part: "beta", From: 1, To: 3, File: "03-resign.md"}, {Part: "gamma", From: 1, To: 4, File: "04-remove.md"},
			{Part: "delta", From: 1, To: 5, File: "05-declare.md"}, {Part: "epsilon", From: 1, To: 6, File: "06-rename.md"},
		}
		if len(result.Cards) != len(wantCards) {
			t.Fatalf("JoinParts().Cards = %+v; want %+v", result.Cards, wantCards)
		}
		for i, want := range wantCards {
			if result.Cards[i] != want {
				t.Errorf("JoinParts().Cards[%d] = %+v; want %+v", i, result.Cards[i], want)
			}
		}

		files := snapshot(t, planDir)
		if _, stale := files["09-stale.md"]; stale || files["archive-old/00-overview.md"] != "kept" {
			t.Errorf("plan directory files = %v; want the stale card file gone and the archive kept", len(files))
		}
		if !strings.Contains(files["02-second.md"], "Build on 1-first.") {
			t.Errorf("02-second.md = %q; want the card id of its own part's first card kept", files["02-second.md"])
		}
		moved := files["03-resign.md"]
		if !strings.HasPrefix(moved, "# Card 3 — resign") || !strings.Contains(moved, "`3: resign`") {
			t.Errorf("03-resign.md = %q; want the heading and Commit prefix renumbered to 3", moved)
		}
		overview := files["00-overview.md"]
		for _, want := range []string{
			"approved: false", "language: go", "# Plan: Joined task", "`alpha`, `beta`, `gamma`, `delta`, `epsilon`",
			"3 — resign — resign", "## Shared Decisions\n\n### Part alpha\n\n### Decision: alpha-choice",
			"## Rename mechanic\n\n1. For a file pair",
		} {
			if !strings.Contains(overview, want) {
				t.Errorf("00-overview.md = %q; want it to contain %q", overview, want)
			}
		}
		if strings.Contains(overview, "this part's own text") || strings.Contains(overview, "### Part epsilon") {
			t.Errorf("00-overview.md = %q; want the canonical rename mechanic and no shared decisions of epsilon", overview)
		}

		joined, err := planparser.ParsePlan(planDir)
		if err != nil {
			t.Fatalf("ParsePlan(joined) = _, %v; want nil error", err)
		}
		for _, finding := range planparser.ValidateFormat(joined, worktree) {
			t.Errorf("ValidateFormat(joined) finding %s: %s; want none", finding.Check, finding.Detail)
		}
	})

	refusals := []struct {
		name   string
		parts  func(t *testing.T, root string) []planparser.JoinPart
		wantIn []string
	}{
		{
			name: "language differs",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				return []planparser.JoinPart{
					{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{Cards: []plankit.Card{partCard(1, "a", "Create", "internal/a/a.go")}})},
					{Name: "beta", Dir: writePart(t, root, "beta", plankit.Plan{Language: "none", Cards: []plankit.Card{partCard(1, "b", "Create", "internal/b/b.go")}})},
				}
			},
			wantIn: []string{`"alpha"`, `"beta"`, `"go"`, `"none"`},
		},
		{
			name: "root differs",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				return []planparser.JoinPart{
					{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{Root: "internal", Cards: []plankit.Card{partCard(1, "a", "Create", "a/a.go")}})},
					{Name: "beta", Dir: writePart(t, root, "beta", plankit.Plan{Cards: []plankit.Card{partCard(1, "b", "Create", "internal/b/b.go")}})},
				}
			},
			wantIn: []string{`"alpha"`, `"beta"`, `"internal"`},
		},
		{
			name: "an export no card delivers",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				return []planparser.JoinPart{
					{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{Cards: []plankit.Card{partCard(1, "a", "Create", "internal/a/a.go")}}), Exports: []string{"internal/a/missing.go"}},
				}
			},
			wantIn: []string{`"alpha"`, `"internal/a/missing.go"`},
		},
		{
			name: "a new file path two parts create",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				return []planparser.JoinPart{
					{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{Cards: []plankit.Card{partCard(1, "a", "Create", "internal/dup/new.go")}})},
					{Name: "beta", Dir: writePart(t, root, "beta", plankit.Plan{Cards: []plankit.Card{partCard(1, "b", "Create", "internal/dup/new.go")}})},
				}
			},
			wantIn: []string{`"alpha"`, `"beta"`, `internal/dup/new.go`},
		},
		{
			name: "a handle two parts create",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				declare := "plan:internal/d#Foo` -> `func Foo()"
				return []planparser.JoinPart{
					{Name: "alpha", Dir: writePart(t, root, "alpha", plankit.Plan{Cards: []plankit.Card{partCard(1, "a", "Create", declare)}})},
					{Name: "beta", Dir: writePart(t, root, "beta", plankit.Plan{Cards: []plankit.Card{partCard(1, "b", "Create", declare)}})},
				}
			},
			wantIn: []string{`"alpha"`, `"beta"`, `plan:internal/d#Foo`},
		},
		{
			name: "a part with no plan",
			parts: func(t *testing.T, root string) []planparser.JoinPart {
				return []planparser.JoinPart{{Name: "alpha", Dir: filepath.Join(root, "alpha")}}
			},
			wantIn: []string{`"alpha"`},
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			planDir := filepath.Join(t.TempDir(), "plan")
			if err := os.MkdirAll(planDir, 0o755); err != nil {
				t.Fatalf("MkdirAll() = %v; want nil", err)
			}
			if err := os.WriteFile(filepath.Join(planDir, "01-old.md"), []byte("old"), 0o644); err != nil {
				t.Fatalf("WriteFile() = %v; want nil", err)
			}
			before := snapshot(t, planDir)

			_, err := planparser.JoinParts(planDir, "Joined", tt.parts(t, t.TempDir()))
			if !errors.Is(err, planparser.ErrJoinRefused) {
				t.Fatalf("JoinParts() error = %v; want it to wrap ErrJoinRefused", err)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("JoinParts() error = %q; want it to contain %q", err.Error(), want)
				}
			}
			if after := snapshot(t, planDir); !maps.Equal(before, after) {
				t.Errorf("plan directory after a refused join = %v; want it unchanged from %v", after, before)
			}
		})
	}
}
