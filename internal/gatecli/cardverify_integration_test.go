//go:build tier1

// cardverify_integration_test.go drives `lyx gate card-verify` in-process over a git worktree outside every hub holding a plan, whose card lines spawn a shell.
// It is Tier 2: it spawns git and the shell.

package gatecli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

func TestGateCardVerify(t *testing.T) {
	t.Parallel()

	card := func(number int, slug, verify string) plankit.Card {
		return plankit.Card{Number: number, Slug: slug, Verify: verify, Groups: []plankit.Group{{Label: "Edit", Targets: []string{"x.go"}}}}
	}
	// worktree returns a git worktree outside every hub holding a plan of cards, or no plan when cards is empty.
	worktree := func(t *testing.T, cards ...plankit.Card) string {
		t.Helper()
		dir := t.TempDir()
		gitkit.Git(t, dir, "init", "-q")
		if len(cards) > 0 {
			plankit.Write(t, planparser.PlanDir(dir), plankit.Plan{Approved: true, Cards: cards})
		}
		return dir
	}

	tests := []struct {
		name      string
		cards     []plankit.Card
		id        string
		wantExit  int
		wantParts []string
	}{
		{
			name:      "an unknown id is refused naming the plan's ids",
			cards:     []plankit.Card{card(1, "alpha", "true"), card(2, "beta", "true")},
			id:        "9",
			wantExit:  1,
			wantParts: []string{"names no card", "1-alpha, 2-beta", "way forward"},
		},
		{
			name:      "a worktree with no plan is refused naming the task worktree",
			id:        "1",
			wantExit:  1,
			wantParts: []string{"holds no plan", "run it from the task worktree whose _lyx/plan holds the card"},
		},
		{
			name:      "a card with no verify line passes saying so",
			cards:     []plankit.Card{card(1, "alpha", "")},
			id:        "1",
			wantParts: []string{"card 1-alpha has no **Verify:** line"},
		},
		{
			name:      "a line's output reaches the verb's output with its exit code",
			cards:     []plankit.Card{card(1, "alpha", "echo from-the-line; exit 3")},
			id:        "1-alpha",
			wantExit:  3,
			wantParts: []string{"from-the-line", "exited 3"},
		},
		{
			name:     "a worktree outside every hub runs the line unslotted and passes",
			cards:    []plankit.Card{card(1, "alpha", "true")},
			id:       "01",
			wantExit: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if got := RunCLIIn(worktree(t, tt.cards...), &out, []string{"card-verify", tt.id}); got != tt.wantExit {
				t.Errorf("card-verify %s exit = %d, output %q; want %d", tt.id, got, out.String(), tt.wantExit)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(out.String(), part) {
					t.Errorf("card-verify %s output = %q; want it to hold %q", tt.id, out.String(), part)
				}
			}
		})
	}

	t.Run("the verb is annotated for the role audience", func(t *testing.T) {
		t.Parallel()
		if got := cardVerifyCmd().Annotations[clihelp.AudienceAnnotation]; got != clihelp.AudienceRole {
			t.Errorf("audience annotation = %q; want %q", got, clihelp.AudienceRole)
		}
	})
}
