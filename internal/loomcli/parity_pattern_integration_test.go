//go:build tier1

// parity_pattern_integration_test.go pins the Gate Self-Check Parity Invariant for the plan gate's pattern-headroom check, which reads the overview at a merge base through a real git repository.

package loomcli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// TestGateParity_PatternHeadroom drives NewPlanGate and `validate-plan` over a branch that already grew PATTERN.md under the headroom line, with a card fencing an entry line.
// Both read the base through loomshed.PatternBaseReader over the told merge base, so a line that grows the projection past the line is stuck on both, and an equal-size replacement passes on both.
func TestGateParity_PatternHeadroom(t *testing.T) {
	t.Parallel()
	const shortEntry = "- `PATTERN-a` — short."
	tests := []struct {
		name       string
		fencedLine string
		want       parityVerdict
	}{
		{"line growing the projection past the headroom line", "- `PATTERN-a` — " + strings.Repeat("l", 200), verdictStuck},
		{"line replacing a bullet at equal size", "- `PATTERN-a` — other.", verdictDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath, worktreeRoot := t.TempDir(), t.TempDir()
			gitkit.Git(t, worktreeRoot, "init", "-b", "main")
			gitkit.Git(t, worktreeRoot, "config", "user.email", "test@test.com")
			gitkit.Git(t, worktreeRoot, "config", "user.name", "Test")
			firstCommit := gitkit.CommitFile(t, worktreeRoot, "PATTERN.md", patternOverviewOfSize(pattern.HeadroomBytes-100, shortEntry), "overview under the line")
			gitkit.CommitFile(t, worktreeRoot, "PATTERN.md", patternOverviewOfSize(pattern.HeadroomBytes-60, shortEntry), "overview grown, still under the line")

			plankit.Write(t, filepath.Join(anchorPath, "_lyx", "plan"), plankit.Plan{
				Approved: true,
				Language: "none",
				Cards: []plankit.Card{{
					Number:  1,
					Slug:    "validate-fixture",
					Summary: "a minimal fixture card",
					Groups:  []plankit.Group{{Label: "Create", Targets: []string{"fixture-output.txt"}}},
					Intent:  "minimal fixture card for validate-plan tests.\n\n```\n" + tt.fencedLine + "\n```",
					Commit:  "1: validate-fixture",
				}},
			})
			c := &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
				PlanIndex:       planglyph.NewIndex(fabricengine.NewReferenceRule()),
				AnchorPath:      anchorPath,
				WorktreeRoot:    worktreeRoot,
				VerifyMergeBase: func() (string, error) { return firstCommit, nil },
			}}

			pc := loomshed.PlanContext{PatternBase: loomshed.PatternBaseReader(c.env.WorktreeRoot, c.env.VerifyMergeBase)}
			result, err := loomshed.NewPlanGate(c.env.AnchorPath, c.env.WorktreeRoot, c.env.RunScratchDir, planglyph.NewIndex(fabricengine.NewReferenceRule()), pc)()
			pv := producerVerdict(result, err)
			var out bytes.Buffer
			clihelp.Execute(c.validatePlanCmd(), &out, nil)
			cv := cliVerdict(envelope.Decode(t, out.String()))

			if pv != tt.want || cv != tt.want {
				t.Errorf("verdicts: gate = %q (result=%+v, err=%v), CLI = %q (raw=%q); want both %q", pv, result, err, cv, out.String(), tt.want)
			}
			if tt.want == verdictStuck && (!strings.Contains(result.Findings, "pattern-headroom") || !strings.Contains(out.String(), "pattern-headroom")) {
				t.Errorf("want pattern-headroom in both the gate findings %q and the verb output %q", result.Findings, out.String())
			}
		})
	}
}

// patternOverviewOfSize returns a PATTERN.md of exactly total bytes: a heading, one filler line, then entry.
func patternOverviewOfSize(total int, entry string) string {
	const heading = "# PATTERN\n"
	tail := entry + "\n"
	return heading + strings.Repeat("x", total-len(heading)-len("\n\n")-len(tail)) + "\n\n" + tail
}
