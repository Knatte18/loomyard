// render_test.go pins the values Go fills into Weaver's batch attempt prompt.
// It reads the shipped template from a seeded stencils directory and runs no subprocess.

package weaverengine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// batchTestGate returns a gate config holding the `card` and `batch` lists a batch prompt renders.
func batchTestGate() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}, "tier1": {About: "integration"}},
		Gates: map[string]gateslot.GateKey{
			"card":  {Tiers: []int{0}, Scope: "card"},
			"batch": {Tiers: []int{0, 1}, Scope: "card"},
		},
	}
}

// batchTestCards returns the two cards of the batch the prompt tests render.
func batchTestCards() []planparser.Card {
	return []planparser.Card{
		{Number: 1, Slug: "one", SourcePath: "_lym/plan/01-one.md"},
		{Number: 2, Slug: "two", SourcePath: "_lym/plan/02-two.md"},
	}
}

func TestRenderBatchPrompt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stencilsDir := stencilkit.Seed(t)

	tests := []struct {
		name string
		edit func(in *weaverengine.BatchPromptInputs)
		want []string
		deny []string
	}{
		{
			name: "FirstAttemptNamesTheBatchAndReadsNone",
			edit: func(in *weaverengine.BatchPromptInputs) {},
			want: []string{
				"- `_lym/plan/01-one.md`:\n  - `lym gate test --gate card --card 1-one`",
				"- `_lym/plan/02-two.md`:\n  - `lym gate test --gate card --card 2-two`",
				"- `lym gate test --gate batch --card 1-one --card 2-two`",
				"`/run/journal.md`",
				"`/run/decision.md`",
				"`/run/reports/01-one-a1.yaml`",
				"## Why this attempt exists\n\nnone\n",
				"## What the worktree holds\n\nnone\n",
			},
			deny: []string{"--card 2-two --main-packages", "lym webster", "lym weaver"},
		},
		{
			name: "RetryCarriesTheDigestAndTheUncommittedPaths",
			edit: func(in *weaverengine.BatchPromptInputs) {
				in.FailureDigest = "- the gate failed on TestX"
				in.UncommittedPaths = "internal/x/x.go"
			},
			want: []string{
				"## Why this attempt exists\n\n- the gate failed on TestX\n",
				"## What the worktree holds\n\ninternal/x/x.go\n",
			},
		},
		{
			name: "LastBatchRendersTheMainPackages",
			edit: func(in *weaverengine.BatchPromptInputs) { in.LastBatch = true },
			want: []string{"- `lym gate test --gate batch --card 1-one --card 2-two --main-packages`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			promptsDir := t.TempDir()
			in := weaverengine.BatchPromptInputs{
				StencilsDir:        stencilsDir,
				PromptsDir:         promptsDir,
				Label:              "01-one",
				Attempt:            1,
				Cards:              batchTestCards(),
				PlanDirDisplay:     "_lym/plan",
				Gate:               batchTestGate(),
				RepoRoot:           t.TempDir(),
				DecisionRecordPath: "/run/decision.md",
				JournalPath:        "/run/journal.md",
				ReportPath:         "/run/reports/01-one-a1.yaml",
			}
			tt.edit(&in)

			got, err := weaverengine.RenderBatchPrompt(in)
			if err != nil {
				t.Fatalf("RenderBatchPrompt() error = %v; want nil", err)
			}
			text := string(got)
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("RenderBatchPrompt() lacks %q", want)
				}
			}
			for _, deny := range tt.deny {
				if strings.Contains(text, deny) {
					t.Errorf("RenderBatchPrompt() holds %q; want it absent", deny)
				}
			}

			written, err := os.ReadFile(filepath.Join(promptsDir, "01-one-a1.md"))
			if err != nil {
				t.Fatalf("read the written prompt: %v", err)
			}
			if !bytes.Equal(written, got) {
				t.Errorf("the prompt file differs from the returned bytes")
			}
		})
	}

	t.Run("GateConfigWithoutBatchListRefuses", func(t *testing.T) {
		t.Parallel()

		gate := batchTestGate()
		delete(gate.Gates, "batch")
		_, err := weaverengine.RenderBatchPrompt(weaverengine.BatchPromptInputs{
			StencilsDir: stencilsDir, PromptsDir: t.TempDir(), Label: "01-one", Attempt: 1,
			Cards: batchTestCards(), PlanDirDisplay: "_lym/plan", Gate: gate, RepoRoot: t.TempDir(),
		})
		if err == nil || !strings.Contains(err.Error(), `gates holds no key "batch"`) {
			t.Errorf("RenderBatchPrompt() error = %v; want one naming the missing batch key", err)
		}
	})
}
