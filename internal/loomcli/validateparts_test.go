// validateparts_test.go covers the `validate-parts` verb and `validate-plan --part`, driven against the leaf commands on a hand-populated *loomCLI.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// splitDecisionRecord is a decision record holding the decisions D1 to D3, which the parts file names.
const splitDecisionRecord = `# Decision record

## Goal

Goal.

## Scope

Scope.

## Decisions

### D1 — first

Text.

### D2 — second

Text.

### D3 — third

Text.

## Constraints

Constraints.

## Auto-mode assumptions

Assumptions.

## Open risks

Risks.

## Acceptance criteria

Criteria.
`

// splitPartsText is a parts file of the parts core and wiring, core exporting coreExport.
const splitPartsText = "# Parts: Add widget\n\n## Split note\n\nThe task splits in two.\n\n" +
	"## Part: core\n\n- Decisions: D1\n- Takes in: the widget request\n- Delivers: the widget type\n- Exports:\n  - `plan:internal/x#NewThing`\n- Area:\n  - `internal/x`\n\n" +
	"## Part: wiring\n\n- Decisions: D2, D3\n- Takes in: the widget type\n- Delivers: the wiring\n- Depends on: core\n- Area:\n  - `internal/y`\n"

// partsFixture writes the parts file, a decision record and the two config files tapestry.yaml's load reads under anchorPath, with partsMax in tapestry.yaml, and returns a *loomCLI wired for both verbs.
func partsFixture(t *testing.T, anchorPath, partsText, partsMax string) *loomCLI {
	t.Helper()

	seedLoomConfig(t, anchorPath)
	configDir := filepath.Join(anchorPath, "_lym", "config")
	tapestry := strings.Replace(loomengine.TapestryConfigTemplate(), "parts_max: 6", "parts_max: "+partsMax, 1)
	if err := os.WriteFile(filepath.Join(configDir, "tapestry.yaml"), []byte(tapestry), 0o644); err != nil {
		t.Fatalf("WriteFile(tapestry.yaml) = %v; want nil", err)
	}
	recordPath := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(recordPath, []byte(splitDecisionRecord), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
	}
	if err := os.MkdirAll(partsparser.Dir(anchorPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	if err := os.WriteFile(partsparser.File(anchorPath), []byte(partsText), 0o644); err != nil {
		t.Fatalf("WriteFile(parts file) = %v; want nil", err)
	}

	return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
		PlanIndex:          planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:         anchorPath,
		WorktreeRoot:       plankit.Repo(t, map[string]string{"internal/x/x.go": "package x\n", "internal/y/y.go": "package y\n"}),
		DecisionRecordPath: recordPath,
	}}
}

// writePartPlan writes the plan of the part named part: one card creating target and reading uses.
func writePartPlan(t *testing.T, anchorPath, part, target string, uses ...string) {
	t.Helper()

	card := plankit.Card{Number: 1, Slug: "card", ImpactSummary: "impact", Groups: []plankit.Group{{Label: "Create", Targets: []string{target}}}, Uses: uses}
	plankit.Write(t, partsparser.PartDir(anchorPath, part), plankit.Plan{Language: "go", Framing: "Framing.", Cards: []plankit.Card{card}})
}

func TestValidatePartsCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		partsText  string
		partsMax   string
		wantExit   int
		wantErrMsg string
		wantInBody string
	}{
		{name: "AValidPartsFilePasses", partsText: splitPartsText, partsMax: "6", wantExit: 0, wantInBody: "parts_path"},
		{name: "ACheckFindingFails", partsText: splitPartsText, partsMax: "1", wantExit: 1, wantErrMsg: "loom: parts file is not yet valid", wantInBody: "parts-over-max: "},
		{
			name:      "AnExportTheWorktreeHoldsFails",
			partsText: strings.Replace(splitPartsText, "  - `plan:internal/x#NewThing`\n", "  - `plan:internal/x#NewThing`\n  - `internal/x/x.go`\n", 1),
			partsMax:  "6", wantExit: 1, wantErrMsg: "loom: parts file is not yet valid", wantInBody: "export-existing",
		},
		{name: "AnInvalidPartsMaxIsTheConfigError", partsText: splitPartsText, partsMax: "0", wantExit: 1, wantErrMsg: "parts_max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := partsFixture(t, t.TempDir(), tt.partsText, tt.partsMax)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePartsCmd(), &out, nil)

			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d; want %d (output: %q)", exitCode, tt.wantExit, out.String())
			}
			if tt.wantErrMsg != "" {
				envelope.RequireErr(t, out.String(), tt.wantErrMsg)
			}
			if tt.wantInBody != "" && !strings.Contains(out.String(), tt.wantInBody) {
				t.Errorf("output = %q; want it to contain %q", out.String(), tt.wantInBody)
			}
		})
	}
}

func TestValidatePlanCmd_Part(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		recipe   string
		part     string
		target   string
		uses     []string
		wantExit int
		// want is a substring the output carries.
		want string
	}{
		{name: "APartPasses", recipe: shedrun.RecipeTapestry, part: "wiring", target: "plan:internal/y#Wire` -> `func Wire()", uses: []string{"plan:internal/x#NewThing"}, wantExit: 0, want: "plan_dir"},
		{name: "ADanglingHandleFailsWithItsFinding", recipe: shedrun.RecipeTapestry, part: "wiring", target: "plan:internal/y#Wire` -> `func Wire()", uses: []string{"plan:internal/z#Nope"}, wantExit: 1, want: "loom: plan is not yet valid"},
		{name: "AnUnknownPartIsRefusedNamingTheHeldParts", recipe: shedrun.RecipeTapestry, part: "nowhere", wantExit: 1, want: "core, wiring"},
		{name: "ARecipeWithNoPartsRowIsRefused", recipe: shedrun.RecipeDarn, part: "core", wantExit: 1, want: "has no Parts-Plan-Write row"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			c := partsFixture(t, anchorPath, splitPartsText, "6")
			c.recipe = tt.recipe
			if tt.target != "" {
				writePartPlan(t, anchorPath, tt.part, tt.target, tt.uses...)
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePlanCmd(), &out, []string{"--part", tt.part})

			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d; want %d (output: %q)", exitCode, tt.wantExit, out.String())
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("output = %q; want it to contain %q", out.String(), tt.want)
			}
			if tt.name == "ADanglingHandleFailsWithItsFinding" && !strings.Contains(out.String(), "handle-dangling") {
				t.Errorf("output = %q; want it to carry handle-dangling", out.String())
			}
		})
	}
}
