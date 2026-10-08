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
	const lint = " && lyx loom lint-comments"

	t.Run("integration step covers each package directory", func(t *testing.T) {
		root := t.TempDir()
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#", "internal/b/b.go#Run"}})
		want := base + " && go test -tags integration ./internal/a ./internal/b" + lint
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
		want := base + " && go test -tags integration ./internal/kept" + lint
		if got := cardGateCommand(plan, card, root); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("directory with no Go files drops out", func(t *testing.T) {
		root := t.TempDir()
		writeGateFile(t, root, "contracts/stencils/x.md")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"contracts/stencils/x.md"}})
		if got := cardGateCommand(plan, card, root); got != base+lint {
			t.Errorf("cardGateCommand() = %q; want %q", got, base+lint)
		}
	})

	t.Run("directory holding a Go file on disk is kept", func(t *testing.T) {
		root := t.TempDir()
		writeGateFile(t, root, "testdata/helper.go")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"testdata/notes.md"}})
		want := base + " && go test -tags integration ./testdata" + lint
		if got := cardGateCommand(plan, card, root); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("worktree root package is spelled dot", func(t *testing.T) {
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"main.go"}})
		want := base + " && go test -tags integration ." + lint
		if got := cardGateCommand(plan, card, t.TempDir()); got != want {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("nested module", func(t *testing.T) {
		edit := func(refs ...string) planparser.TargetGroup {
			return planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: refs}
		}
		create := func(refs ...string) planparser.TargetGroup {
			return planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: refs}
		}
		numbered := func(number int, groups ...planparser.TargetGroup) planparser.Card {
			card := gateCard(groups...)
			card.Number = number
			return card
		}
		tests := []struct {
			name  string
			disk  []string
			cards []planparser.Card
			index int
			want  string
		}{
			{
				name:  "directory inside an on-disk module beside a root directory",
				disk:  []string{"nested/go.mod"},
				cards: []planparser.Card{numbered(1, edit("internal/a/a.go#", "nested/pkg/p.go#"))},
				want:  base + " && go test -tags integration ./internal/a && go -C nested test -tags integration ./pkg" + lint,
			},
			{
				name:  "the module's own directory is spelled dot",
				disk:  []string{"nested/go.mod"},
				cards: []planparser.Card{numbered(1, edit("nested/main.go#"))},
				want:  base + " && go -C nested test -tags integration ." + lint,
			},
			{
				name:  "one step per module in first-seen order",
				disk:  []string{"m1/go.mod", "m2/go.mod"},
				cards: []planparser.Card{numbered(1, edit("m2/a/a.go#", "m1/b/b.go#", "m2/c/c.go#"))},
				want:  base + " && go -C m2 test -tags integration ./a ./c && go -C m1 test -tags integration ./b" + lint,
			},
			{
				name:  "module created by this card",
				cards: []planparser.Card{numbered(1, create("newmod/go.mod", "newmod/pkg/p.go#"))},
				want:  base + " && go -C newmod test -tags integration ./pkg" + lint,
			},
			{
				name: "module created by an earlier card",
				cards: []planparser.Card{
					numbered(1, create("newmod/go.mod")),
					numbered(2, edit("newmod/pkg/p.go#")),
				},
				index: 1,
				want:  base + " && go -C newmod test -tags integration ./pkg" + lint,
			},
			{
				name: "module created by a later card stays a root argument",
				cards: []planparser.Card{
					numbered(1, edit("newmod/pkg/p.go#")),
					numbered(2, create("newmod/go.mod")),
				},
				want: base + " && go test -tags integration ./newmod/pkg" + lint,
			},
			{
				name: "module moved by a Rename pair",
				disk: []string{"old/go.mod"},
				cards: []planparser.Card{numbered(1,
					planparser.TargetGroup{
						Type:  planparser.CardTypeRename,
						Refs:  []string{"old/go.mod", "moved/go.mod"},
						Pairs: []planparser.MovePair{{Old: "old/go.mod", New: "moved/go.mod"}},
					},
					edit("moved/pkg/p.go#"),
				)},
				want: base + " && go -C moved test -tags integration ./pkg" + lint,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				root := t.TempDir()
				for _, file := range tt.disk {
					writeGateFile(t, root, file)
				}
				nestedPlan := &planparser.Plan{Language: "go", Cards: tt.cards}
				if got := cardGateCommand(nestedPlan, tt.cards[tt.index], root); got != tt.want {
					t.Errorf("cardGateCommand() = %q; want %q", got, tt.want)
				}
			})
		}
	})
}

//testtiming:keep pins the per-card gate line shape re-rooted onto the plan display and the bare build-and-test gate of a card with no Go target; the begin-batch prompt test checks one card's command
func TestRenderCardGates_OneLinePerCard(t *testing.T) {
	cards := []planparser.Card{
		gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#"}}),
		{Number: 2, Slug: "docs", SourcePath: "_lyx/plan/02-docs.md"},
	}
	got := renderCardGates(&planparser.Plan{Language: "go"}, cards, "_lyx/plan", t.TempDir())
	want := "- `_lyx/plan/01-gate.md`: `go build ./... && go test ./... && go test -tags integration ./internal/a && lyx loom lint-comments`\n" +
		"- `_lyx/plan/02-docs.md`: `go build ./... && go test ./... && lyx loom lint-comments`"
	if got != want {
		t.Errorf("renderCardGates() =\n%s\nwant\n%s", got, want)
	}
}
