package websterengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func writeGateFile(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func gateCard(groups ...planparser.TargetGroup) planparser.Card {
	return planparser.Card{Number: 1, Slug: "gate", SourcePath: "_lyx/plan/01-gate.md", TargetGroups: groups}
}

func TestCardGateCommand(t *testing.T) {
	plan := &planparser.Plan{Language: "go"}
	const base = "go build ./... && go test ./..."

	t.Run("integration step covers each package directory", func(t *testing.T) {
		root := t.TempDir()
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#", "internal/b/b.go#Run"}})
		want := base + " && go test -tags integration ./internal/a ./internal/b"
		if got := cardGateCommand(plan, card, root); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("Delete-only directory drops out", func(t *testing.T) {
		root := t.TempDir()
		writeGateFile(t, root, "internal/gone/g.go")
		card := gateCard(
			planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"internal/gone/g.go#"}},
			planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/kept/k.go#"}},
		)
		want := base + " && go test -tags integration ./internal/kept"
		if got := cardGateCommand(plan, card, root); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("directory with no Go files drops out", func(t *testing.T) {
		root := t.TempDir()
		writeGateFile(t, root, "contracts/stencils/x.md")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"contracts/stencils/x.md"}})
		if got := cardGateCommand(plan, card, root); got != base {
			t.Errorf("cardGateCommand() = %q; want %q", got, base)
		}
	})

	t.Run("directory holding a Go file on disk is kept", func(t *testing.T) {
		root := t.TempDir()
		writeGateFile(t, root, "testdata/helper.go")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"testdata/notes.md"}})
		want := base + " && go test -tags integration ./testdata"
		if got := cardGateCommand(plan, card, root); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("worktree root package is spelled dot", func(t *testing.T) {
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"main.go"}})
		want := base + " && go test -tags integration ."
		if got := cardGateCommand(plan, card, t.TempDir()); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})
}

func TestRenderCardGates_OneLinePerCard(t *testing.T) {
	cards := []planparser.Card{
		gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#"}}),
		{Number: 2, Slug: "docs", SourcePath: "_lyx/plan/02-docs.md"},
	}
	got := renderCardGates(&planparser.Plan{Language: "go"}, cards, "_lyx/plan", t.TempDir())
	want := "- `_lyx/plan/01-gate.md`: `go build ./... && go test ./... && go test -tags integration ./internal/a`\n" +
		"- `_lyx/plan/02-docs.md`: `go build ./... && go test ./...`"
	if got != want {
		t.Errorf("renderCardGates() =\n%s\nwant\n%s", got, want)
	}
}
