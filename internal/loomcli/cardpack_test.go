// cardpack_test.go pins the `card-pack` loom verb: the markdown it prints for one card against the tree as it stands, its trace lines, and its refusals.

package loomcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// degradedIndex is a planindex.Index whose KickstartPack lists the card's glyph unresolved beside an error wrapping planindex.ErrQuarryUnavailable.
type degradedIndex struct {
	planindex.Index
}

func (degradedIndex) KickstartPack(_ *planparser.Plan, cards []planparser.Card, _ string) ([]planindex.CardPack, error) {
	err := fmt.Errorf("%w: boom", planindex.ErrQuarryUnavailable)
	return []planindex.CardPack{{
		Card: cards[0],
		Pack: planindex.Pack{Unresolved: []planindex.PackUnresolved{{Ref: "internal/foo#Baz", Reason: err.Error()}}},
	}}, err
}

// TestCardPackCmd drives the verb over a hand-populated loomCLI:
// a pack of resolved and unresolved glyphs, the same pack after an uncommitted edit, a pack of plan handles alone, a glyph-less plan, a degraded index, and every refusal, which logs no card pack line.
// It captures the logger, which is process-global, so it does not call t.Parallel.
func TestCardPackCmd(t *testing.T) {
	const bazFile = "package foo\n\nfunc Baz() {}\n"
	bazCard := plankit.Card{
		Number:  1,
		Slug:    "alpha",
		Groups:  []plankit.Group{{Label: "Edit", Targets: []string{"internal/foo#Baz"}}},
		Uses:    []string{"internal/missing#Nope"},
		Summary: "change Baz",
	}
	handleCard := plankit.Card{
		Number:  1,
		Slug:    "alpha",
		Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/foo/x.go", "plan:internal/foo#Bar` -> `func Bar()"}}},
		Summary: "add Bar",
	}
	plainCard := plankit.Card{
		Number:  1,
		Slug:    "alpha",
		Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/foo/x.txt"}}},
		Summary: "add x",
	}
	goPlan := func(cards ...plankit.Card) *plankit.Plan {
		return &plankit.Plan{Approved: true, Language: "go", Cards: cards}
	}

	tests := []struct {
		name  string
		plan  *plankit.Plan
		files map[string]string
		index func(planindex.Index) planindex.Index
		args  []string
		// rewrite is applied to the worktree after the first call, and the verb is called again.
		rewrite map[string]string
		// remove lists worktree files, slash-separated, deleted before the first call.
		remove []string

		wantExit int
		// wantStdout, when set, is the exact stdout.
		wantStdout string
		wantOut    []string
		wantNotOut []string
		// wantError is a substring of the failure envelope's error, which ends with the way forward.
		wantError string
		wantLog   []string
		// wantAfter holds substrings of the second call's stdout.
		wantAfter []string
	}{
		{
			name:  "a card's glyphs print as its pack subsection and log the counts",
			plan:  goPlan(bazCard),
			files: map[string]string{"internal/foo/a.go": bazFile},
			args:  []string{"1"},
			wantStdout: "### `_lyx/plan/01-alpha.md`\n\n" +
				"- `internal/foo#Baz`\n" +
				"  - `internal/foo/a.go:3-3` — func Baz()\n\n" +
				"#### Unresolved\n\n" +
				"These refs of this card did not resolve to a declaration, each with its reason.\n\n" +
				"- `internal/missing#Nope` — not_found\n",
			wantLog: []string{`msg="webster: card pack"`, "card=1 resolved=1 unresolved=1"},
		},
		{
			name:      "an uncommitted edit above a glyph shifts the printed span",
			plan:      goPlan(bazCard),
			files:     map[string]string{"internal/foo/a.go": bazFile},
			args:      []string{"1"},
			rewrite:   map[string]string{"internal/foo/a.go": "package foo\n\nvar added = 1\n\nfunc Baz() {}\n"},
			wantOut:   []string{"`internal/foo/a.go:3-3`"},
			wantAfter: []string{"`internal/foo/a.go:5-5` — func Baz()"},
		},
		{
			name:       "a card of plain paths and a plan handle prints the empty line and the handle unresolved",
			plan:       goPlan(handleCard),
			args:       []string{"1"},
			wantOut:    []string{"no member glyphs on this card", "#### Unresolved", "`plan:internal/foo#Bar`"},
			wantNotOut: []string{"- `internal/foo#"},
			wantLog:    []string{`msg="webster: card pack"`, "card=1 resolved=0 unresolved=1"},
		},
		{
			name:       "a plan whose language opts out of glyphs prints the empty line alone",
			plan:       &plankit.Plan{Approved: true, Language: "none", Cards: []plankit.Card{plainCard}},
			args:       []string{"1"},
			wantOut:    []string{"no member glyphs on this card"},
			wantNotOut: []string{"#### Unresolved"},
			wantLog:    []string{`msg="webster: card pack"`, "resolved=0 unresolved=0"},
		},
		{
			name:  "an index that cannot answer keeps the degraded pack and warns",
			plan:  goPlan(bazCard),
			files: map[string]string{"internal/foo/a.go": bazFile},
			index: func(planindex.Index) planindex.Index { return degradedIndex{} },
			args:  []string{"1"},
			wantOut: []string{
				"- `internal/foo#Baz` — " + planindex.ErrQuarryUnavailable.Error() + ": boom",
			},
			wantLog: []string{"level=WARN", `msg="webster: card pack"`},
		},
		{
			name:      "an argument that is not an integer is refused",
			plan:      goPlan(bazCard),
			args:      []string{"x"},
			wantExit:  1,
			wantError: `"x" is not a card number; way forward: locate the card's glyphs`,
		},
		{
			name:      "card zero is refused",
			plan:      goPlan(bazCard),
			args:      []string{"0"},
			wantExit:  1,
			wantError: `"0" is not a card number; way forward: locate the card's glyphs`,
		},
		{
			name:     "no argument is refused by the argument count",
			plan:     goPlan(bazCard),
			wantExit: 1,
		},
		{
			name:      "a card number the plan lacks is refused",
			plan:      goPlan(bazCard),
			args:      []string{"9"},
			wantExit:  1,
			wantError: `has no card 9; way forward: locate the card's glyphs`,
		},
		{
			name:      "an anchor without a plan directory is refused",
			args:      []string{"1"},
			wantExit:  1,
			wantError: `does not exist; way forward: locate the card's glyphs`,
		},
		{
			name:      "a plan whose Card Index names a card without a file is refused",
			plan:      goPlan(bazCard),
			args:      []string{"1"},
			remove:    []string{"_lyx/plan/01-alpha.md"},
			wantExit:  1,
			wantError: `way forward: locate the card's glyphs`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			worktree := plankit.Repo(t, tt.files)
			if tt.plan != nil {
				plankit.Write(t, planparser.PlanDir(worktree), *tt.plan)
			}
			var index planindex.Index = planglyph.NewIndex(fabricengine.NewReferenceRule())
			if tt.index != nil {
				index = tt.index(index)
			}
			c := &loomCLI{env: shedrecipe.Env{
				PlanIndex:    index,
				AnchorPath:   worktree,
				WorktreeRoot: worktree,
				StencilsDir:  stencilkit.Seed(t),
			}}

			for _, rel := range tt.remove {
				if err := os.Remove(filepath.Join(worktree, filepath.FromSlash(rel))); err != nil {
					t.Fatalf("remove %s: %v", rel, err)
				}
			}

			var out bytes.Buffer
			exit := clihelp.Execute(c.cardPackCmd(), &out, tt.args)
			if exit != tt.wantExit {
				t.Fatalf("exit = %d; want %d (output: %q)", exit, tt.wantExit, out.String())
			}

			if tt.wantExit != 0 {
				if tt.wantError != "" {
					env := envelope.Decode(t, out.String())
					if env.OK {
						t.Errorf("envelope ok = true; want a failure envelope: %q", out.String())
					}
					if !strings.Contains(env.Error, tt.wantError) {
						t.Errorf("error = %q; want it to contain %q", env.Error, tt.wantError)
					}
				}
				if strings.Contains(logs.String(), `msg="webster: card pack"`) {
					t.Errorf("a refused call logged a card pack line:\n%s", logs.String())
				}
				return
			}

			got := out.String()
			if tt.wantStdout != "" {
				if got != tt.wantStdout {
					t.Errorf("stdout =\n%q\nwant\n%q", got, tt.wantStdout)
				}
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(got, want) {
					t.Errorf("stdout lacks %q:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotOut {
				if strings.Contains(got, notWant) {
					t.Errorf("stdout holds %q:\n%s", notWant, got)
				}
			}
			for _, want := range tt.wantLog {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("log lacks %q:\n%s", want, logs.String())
				}
			}

			if tt.rewrite == nil {
				return
			}
			plankit.WriteTree(t, worktree, tt.rewrite)
			var again bytes.Buffer
			if exit := clihelp.Execute(c.cardPackCmd(), &again, tt.args); exit != 0 {
				t.Fatalf("second call exit = %d; want 0 (output: %q)", exit, again.String())
			}
			for _, want := range tt.wantAfter {
				if !strings.Contains(again.String(), want) {
					t.Errorf("second call stdout lacks %q:\n%s", want, again.String())
				}
			}
		})
	}
}
