package websterengine

import (
	"os"
	"path/filepath"
	"slices"
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
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	const lint = "lyx loom lint-comments"

	t.Run("one step covers each package directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#", "internal/b/b.go#Run"}})
		want := []string{"lyx gate test ./internal/a ./internal/b", lint}
		if got := cardGateCommand(plan, card, root); !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("Delete-only directory drops out", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeGateFile(t, root, "internal/gone/g.go")
		card := gateCard(
			planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"internal/gone/g.go#"}},
			planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/kept/k.go#"}},
		)
		want := []string{"lyx gate test ./internal/kept", lint}
		if got := cardGateCommand(plan, card, root); !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("directory with no Go files runs the lint alone", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeGateFile(t, root, "contracts/stencils/x.md")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"contracts/stencils/x.md"}})
		want := []string{lint}
		if got := cardGateCommand(plan, card, root); !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("directory holding a Go file on disk is kept", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeGateFile(t, root, "testdata/helper.go")
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"testdata/notes.md"}})
		want := []string{"lyx gate test ./testdata", lint}
		if got := cardGateCommand(plan, card, root); !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("worktree root package is spelled dot", func(t *testing.T) {
		t.Parallel()
		card := gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"main.go"}})
		want := []string{"lyx gate test .", lint}
		if got := cardGateCommand(plan, card, t.TempDir()); !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("nested module", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			disk  []string
			cards []planparser.Card
			index int
			want  []string
		}{
			{
				name:  "directory inside an on-disk module beside a root directory",
				disk:  []string{"nested/go.mod"},
				cards: []planparser.Card{numberedGateCard(1, gateEdit("internal/a/a.go#", "nested/pkg/p.go#"))},
				want:  []string{"lyx gate test ./internal/a", "lyx gate test -C nested ./pkg", lint},
			},
			{
				name:  "the module's own directory is spelled dot",
				disk:  []string{"nested/go.mod"},
				cards: []planparser.Card{numberedGateCard(1, gateEdit("nested/main.go#"))},
				want:  []string{"lyx gate test -C nested .", lint},
			},
			{
				name:  "one step per module in first-seen order",
				disk:  []string{"m1/go.mod", "m2/go.mod"},
				cards: []planparser.Card{numberedGateCard(1, gateEdit("m2/a/a.go#", "m1/b/b.go#", "m2/c/c.go#"))},
				want:  []string{"lyx gate test -C m2 ./a ./c", "lyx gate test -C m1 ./b", lint},
			},
			{
				name:  "module created by this card",
				cards: []planparser.Card{numberedGateCard(1, gateCreate("newmod/go.mod", "newmod/pkg/p.go#"))},
				want:  []string{"lyx gate test -C newmod ./pkg", lint},
			},
			{
				name: "module created by an earlier card",
				cards: []planparser.Card{
					numberedGateCard(1, gateCreate("newmod/go.mod")),
					numberedGateCard(2, gateEdit("newmod/pkg/p.go#")),
				},
				index: 1,
				want:  []string{"lyx gate test -C newmod ./pkg", lint},
			},
			{
				name: "module created by a later card stays a root argument",
				cards: []planparser.Card{
					numberedGateCard(1, gateEdit("newmod/pkg/p.go#")),
					numberedGateCard(2, gateCreate("newmod/go.mod")),
				},
				want: []string{"lyx gate test ./newmod/pkg", lint},
			},
			{
				name: "module moved by a Rename pair",
				disk: []string{"old/go.mod"},
				cards: []planparser.Card{numberedGateCard(1,
					planparser.TargetGroup{
						Type:  planparser.CardTypeRename,
						Refs:  []string{"old/go.mod", "moved/go.mod"},
						Pairs: []planparser.MovePair{{Old: "old/go.mod", New: "moved/go.mod"}},
					},
					gateEdit("moved/pkg/p.go#"),
				)},
				want: []string{"lyx gate test -C moved ./pkg", lint},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				for _, file := range tt.disk {
					writeGateFile(t, root, file)
				}
				nestedPlan := &planparser.Plan{Language: "go", Cards: tt.cards}
				if got := cardGateCommand(nestedPlan, tt.cards[tt.index], root); !slices.Equal(got, tt.want) {
					t.Errorf("cardGateCommand() = %q; want %q", got, tt.want)
				}
			})
		}
	})
}

func gateEdit(refs ...string) planparser.TargetGroup {
	return planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: refs}
}

func gateCreate(refs ...string) planparser.TargetGroup {
	return planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: refs}
}

func numberedGateCard(number int, groups ...planparser.TargetGroup) planparser.Card {
	card := gateCard(groups...)
	card.Number = number
	return card
}

//testtiming:keep pins the per-card gate bullet shape re-rooted onto the plan display, one sub-bullet per step, and the lint-only gate of a card with no Go target; the begin-batch prompt test checks one card's steps
func TestRenderCardGates_OneBulletPerCardWithAStepSubBulletEach(t *testing.T) {
	t.Parallel()

	cards := []planparser.Card{
		gateCard(planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#"}}),
		{Number: 2, Slug: "docs", SourcePath: "_lyx/plan/02-docs.md"},
	}
	got := renderCardGates(&planparser.Plan{Language: "go"}, cards, "_lyx/plan", t.TempDir())
	want := "- `_lyx/plan/01-gate.md`:\n" +
		"  - `lyx gate test ./internal/a`\n" +
		"  - `lyx loom lint-comments`\n" +
		"- `_lyx/plan/02-docs.md`:\n" +
		"  - `lyx loom lint-comments`"
	if got != want {
		t.Errorf("renderCardGates() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderBatchGate(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}

	tests := []struct {
		name  string
		disk  []string
		cards []planparser.Card
		want  string
	}{
		{
			name: "a two-card batch unions its directories across the root and a nested module",
			disk: []string{"nested/go.mod"},
			cards: []planparser.Card{
				numberedGateCard(1, gateEdit("internal/a/a.go#", "nested/pkg/p.go#")),
				numberedGateCard(2, gateEdit("internal/a/b.go#", "internal/b/b.go#", "nested/other/o.go#")),
			},
			want: "The batch's Go packages:\n" +
				"- the root module: `./internal/a ./internal/b`\n" +
				"- the module `nested`: `./pkg ./other`\n" +
				"\n" +
				"The batch gate:\n" +
				"- `lyx gate test --tags integration ./internal/a ./internal/b`\n" +
				"- `lyx gate test -C nested --tags integration ./pkg ./other`",
		},
		{
			name:  "a batch with no Go package has no batch gate",
			cards: []planparser.Card{numberedGateCard(1, planparser.TargetGroup{Type: planparser.CardTypeProsa, Refs: []string{"contracts/stencils/x.md"}})},
			want:  "This batch has no Go package, so it has no batch gate.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, file := range tt.disk {
				writeGateFile(t, root, file)
			}
			if got := renderBatchGate(plan, tt.cards, root); got != tt.want {
				t.Errorf("renderBatchGate() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
