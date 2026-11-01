// partcheck_test.go covers the parts gate, the part check with its gate and Accept, and the join's Go half, over the real plan index.

package loomshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// splitRecord is a decision record holding the decisions D1 to D3 and a PATTERN changes section no part overview carries.
const splitRecord = validDecisionRecord + `
## PATTERN changes

- ` + "`PATTERN-sole-parsers`" + ` — changes: a changed line.
`

// splitRecordWithDecisions is a decision record whose Decisions section holds the entries D1 to D3, which the parts gate checks the parts file against.
const splitRecordWithDecisions = `# Decision record

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

// splitPartsFile returns a parts file of the parts core and wiring and two shared spots, core exporting coreExports.
func splitPartsFile(coreExports ...string) string {
	exports := ""
	if len(coreExports) > 0 {
		exports = "- Exports:\n"
		for _, export := range coreExports {
			exports += "  - `" + export + "`\n"
		}
	}
	return "# Parts: Add widget\n\n## Split note\n\nThe task splits in two.\n\n" +
		"## Part: core\n\n- Decisions: D1\n- Takes in: the widget request\n- Delivers: the widget type\n" + exports +
		"- Area:\n  - `internal/x`\n\n" +
		"## Part: wiring\n\n- Decisions: D2, D3\n- Takes in: the widget type\n- Delivers: the wiring\n- Depends on: core\n- Area:\n  - `internal/y`\n\n" +
		"## Shared spot: `internal/reg/reg.go`\n\n- Decisions: D3\n- Needed by: core, wiring\n- Needs: a registry row\n- Exports:\n  - `plan:internal/reg#Registered`\n\n" +
		"## Shared spot: `cmd/main.go`\n\n- Needed by: wiring\n- Needs: the wire\n"
}

// splitWorktree returns a worktree holding the code the parts' cards edit.
func splitWorktree(t *testing.T) string {
	t.Helper()
	return plankit.Repo(t, map[string]string{
		"internal/x/x.go":        "package x\n\nfunc Existing() {}\n",
		"internal/d/d.go":        "package d\n\ntype T struct{}\n",
		"internal/reg/reg.go":    "package reg\n",
		"cmd/main.go":            "package main\n",
		"internal/y/existing.go": "package y\n",
	})
}

// writeSplit writes the parts file text under anchorPath.
func writeSplit(t *testing.T, anchorPath, text string) {
	t.Helper()
	if err := os.MkdirAll(partsparser.Dir(anchorPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	if err := os.WriteFile(partsparser.File(anchorPath), []byte(text), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}
}

// splitCard returns a one-group part card.
func splitCard(number int, slug, label string, targets ...string) plankit.Card {
	return plankit.Card{Number: number, Slug: slug, ImpactSummary: "impact", Groups: []plankit.Group{{Label: label, Targets: targets}}}
}

// writeSplitPart writes the plan of the part named part under anchorPath.
func writeSplitPart(t *testing.T, anchorPath, part string, cards ...plankit.Card) {
	t.Helper()
	plankit.Write(t, partsparser.PartDir(anchorPath, part), plankit.Plan{Language: "go", Framing: "Framing.", Cards: cards})
}

// coreCreatingBothExports is a core part plan that creates the handle and the file its contract exports.
func coreCreatingBothExports() plankit.Card {
	return splitCard(1, "core", "Create", "plan:internal/x#NewThing` -> `func NewThing()", "internal/x/newfile.go")
}

// wiringUsing is a wiring part plan whose card creates a handle and uses the given refs.
func wiringUsing(uses ...string) plankit.Card {
	card := splitCard(1, "wire", "Create", "plan:internal/y#Wire` -> `func Wire()")
	card.Uses = uses
	return card
}

func TestNewPartsGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	recordPath := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(recordPath, []byte(splitRecordWithDecisions), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}

	t.Run("a valid parts file passes", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		writeSplit(t, anchorPath, splitPartsFile("plan:internal/x#NewThing"))
		result, err := NewPartsGate(anchorPath, recordPath, 6)()
		if err != nil || !result.Passed {
			t.Errorf("gate() = %+v, %v; want a pass", result, err)
		}
	})

	t.Run("a check finding fails with its line", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		writeSplit(t, anchorPath, splitPartsFile("plan:internal/x#NewThing"))
		result, err := NewPartsGate(anchorPath, recordPath, 1)()
		if err != nil || result.Passed {
			t.Fatalf("gate() = %+v, %v; want a failed gate", result, err)
		}
		if !strings.HasPrefix(result.Findings, "parts-over-max: ") {
			t.Errorf("gate() Findings = %q; want a parts-over-max line", result.Findings)
		}
	})

	t.Run("an unreadable record is the gate's error", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		writeSplit(t, anchorPath, splitPartsFile())
		if _, err := NewPartsGate(anchorPath, filepath.Join(t.TempDir(), "absent.md"), 6)(); err == nil {
			t.Errorf("gate() error = nil; want the record's read error")
		}
	})
}

func TestValidatePart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	index := planglyph.NewIndex(fabricengine.NewReferenceRule())
	newThing, newFile, registered := "plan:internal/x#NewThing", "internal/x/newfile.go", "plan:internal/reg#Registered"

	tests := []struct {
		name string
		// parts is the parts file text, or empty for no file.
		parts string
		cards map[string][]plankit.Card
		part  string
		// wantCheck is the check a blocking finding must carry, empty for a pass.
		wantCheck string
		wantErr   string
	}{
		{
			name: "dependency export and needed spot export pass", parts: splitPartsFile(newThing, newFile), part: "wiring",
			cards: map[string][]plankit.Card{"wiring": {wiringUsing(newThing, newFile, registered)}},
		},
		{
			name: "dangling handle no export covers", parts: splitPartsFile(newThing), part: "wiring", wantCheck: "handle-dangling",
			cards: map[string][]plankit.Card{"wiring": {wiringUsing("plan:internal/z#Nope")}},
		},
		{
			name: "card targeting a spot", parts: splitPartsFile(newThing), part: "wiring", wantCheck: "part-spot-target",
			cards: map[string][]plankit.Card{"wiring": {splitCard(1, "wire", "Edit", "internal/reg/reg.go")}},
		},
		{
			name: "export no card creates", parts: splitPartsFile(newThing, newFile), part: "core", wantCheck: "part-export-missing",
			cards: map[string][]plankit.Card{"core": {splitCard(1, "core", "Create", "plan:internal/x#NewThing` -> `func NewThing()")}},
		},
		{
			name: "export a card re-signs", parts: splitPartsFile("internal/x#Existing"), part: "core",
			cards: map[string][]plankit.Card{"core": {splitCard(1, "core", "Edit", "internal/x#Existing` -> `func Existing(a int)")}},
		},
		{
			name: "handle export spelled without the owner chain a card declares with it", parts: splitPartsFile("plan:internal/d#Foo"), part: "core",
			cards: map[string][]plankit.Card{"core": {splitCard(1, "core", "Create", "plan:internal/d#T.Foo` -> `func (t *T) Foo()")}},
		},
		{name: "unknown part", parts: splitPartsFile(), part: "nowhere", wantErr: "no such part"},
		{name: "absent parts file", part: "core", wantErr: "lym loom goto --to Parts-Write"},
		{name: "malformed parts file", parts: "not a parts file\n", part: "core", wantErr: "lym loom validate-parts"},
		{name: "part directory that does not parse", parts: splitPartsFile(), part: "core", wantErr: "does not parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath, worktreeRoot := t.TempDir(), splitWorktree(t)
			if tt.parts != "" {
				writeSplit(t, anchorPath, tt.parts)
			}
			for part, cards := range tt.cards {
				writeSplitPart(t, anchorPath, part, cards...)
			}
			if tt.name == "part directory that does not parse" {
				if err := os.MkdirAll(partsparser.PartDir(anchorPath, "core"), 0o755); err != nil {
					t.Fatalf("MkdirAll() = %v; want nil", err)
				}
				if err := os.WriteFile(partsparser.PartOverview(anchorPath, "core"), []byte("not an overview\n"), 0o644); err != nil {
					t.Fatalf("WriteFile() = %v; want nil", err)
				}
			}

			findings, err := ValidatePart(anchorPath, worktreeRoot, tt.part, index, PlanContext{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidatePart() error = %v; want it to contain %q", err, tt.wantErr)
				}
				if tt.part == "nowhere" && (!errors.Is(err, ErrPartUnknown) || !strings.Contains(err.Error(), "core, wiring")) {
					t.Errorf("ValidatePart() error = %v; want ErrPartUnknown naming the parts core and wiring", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePart() error = %v; want nil", err)
			}
			var blocking []string
			for _, finding := range findings {
				if finding.Severity != planindex.SeverityInformational {
					blocking = append(blocking, finding.Check)
				}
			}
			if tt.wantCheck == "" && len(blocking) != 0 {
				t.Errorf("ValidatePart() blocking findings = %v; want none (%s)", blocking, formatPlanFindings(findings))
			}
			if tt.wantCheck != "" && (len(blocking) != 1 || blocking[0] != tt.wantCheck) {
				t.Errorf("ValidatePart() blocking findings = %v; want exactly %q (%s)", blocking, tt.wantCheck, formatPlanFindings(findings))
			}
		})
	}

	t.Run("a record's PATTERN changes the part overview lacks raise no pattern finding", func(t *testing.T) {
		t.Parallel()
		anchorPath, worktreeRoot := t.TempDir(), splitWorktree(t)
		writeSplit(t, anchorPath, splitPartsFile(newThing, newFile))
		writeSplitPart(t, anchorPath, "wiring", wiringUsing(newThing))
		recordPath := filepath.Join(t.TempDir(), "decision-record.md")
		if err := os.WriteFile(recordPath, []byte(splitRecord), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		findings, err := ValidatePart(anchorPath, worktreeRoot, "wiring", index, PlanContext{DecisionRecordPath: recordPath})
		if err != nil {
			t.Fatalf("ValidatePart() error = %v; want nil", err)
		}
		for _, finding := range findings {
			if strings.HasPrefix(finding.Check, "pattern-") {
				t.Errorf("ValidatePart() finding %s; want no pattern check on a part", finding.Error())
			}
		}
	})

	t.Run("the gate and Accept follow the part's findings", func(t *testing.T) {
		t.Parallel()
		anchorPath, worktreeRoot := t.TempDir(), splitWorktree(t)
		writeSplit(t, anchorPath, splitPartsFile(newThing, newFile))
		writeSplitPart(t, anchorPath, "core", coreCreatingBothExports())
		writeSplitPart(t, anchorPath, "wiring", wiringUsing("plan:internal/z#Nope"))
		if err := os.MkdirAll(filepath.Join(partsparser.Dir(anchorPath), "broken"), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}

		for _, tt := range []struct {
			part       string
			wantPassed bool
			wantFind   string
		}{
			{part: "core", wantPassed: true},
			{part: "wiring", wantFind: "handle-dangling"},
		} {
			result, err := NewPartPlanGate(anchorPath, worktreeRoot, "", tt.part, index, PlanContext{})()
			if err != nil || result.Passed != tt.wantPassed || !strings.Contains(result.Findings, tt.wantFind) {
				t.Errorf("NewPartPlanGate(%s)() = %+v, %v; want Passed %v with %q", tt.part, result, err, tt.wantPassed, tt.wantFind)
			}
			acceptErr := NewPartAccept(anchorPath, worktreeRoot, tt.part, index, PlanContext{})(shuttleengine.Result{})
			if tt.wantPassed != (acceptErr == nil) || (acceptErr != nil && !strings.Contains(acceptErr.Error(), tt.wantFind)) {
				t.Errorf("NewPartAccept(%s) = %v; want nil only for a passing part, else an error with %q", tt.part, acceptErr, tt.wantFind)
			}
		}

		// A part directory the plan parser rejects is the gate's findings, not its error.
		writeSplit(t, anchorPath, strings.Replace(splitPartsFile(newThing, newFile), "## Part: wiring", "## Part: broken", 1))
		if err := os.WriteFile(partsparser.PartOverview(anchorPath, "broken"), []byte("not an overview\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		result, err := NewPartPlanGate(anchorPath, worktreeRoot, "", "broken", index, PlanContext{})()
		if err != nil || result.Passed || result.Findings == "" {
			t.Errorf("NewPartPlanGate(broken)() = %+v, %v; want a failed gate carrying the parse error", result, err)
		}
	})
}

func TestJoinParts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	index := planglyph.NewIndex(fabricengine.NewReferenceRule())
	newThing, newFile := "plan:internal/x#NewThing", "internal/x/newfile.go"

	seed := func(t *testing.T, wiring plankit.Card) (anchorPath, worktreeRoot string) {
		t.Helper()
		anchorPath, worktreeRoot = t.TempDir(), splitWorktree(t)
		writeSplit(t, anchorPath, splitPartsFile(newThing, newFile))
		writeSplitPart(t, anchorPath, "core", coreCreatingBothExports())
		writeSplitPart(t, anchorPath, "wiring", wiring)
		return anchorPath, worktreeRoot
	}

	t.Run("two valid parts join into the plan directory", func(t *testing.T) {
		t.Parallel()
		anchorPath, worktreeRoot := seed(t, wiringUsing(newThing))
		result, err := JoinParts(anchorPath, worktreeRoot, index, PlanContext{})
		if err != nil {
			t.Fatalf("JoinParts() error = %v; want nil", err)
		}
		if len(result.Cards) != 2 || result.Cards[0].Part != "core" || result.Cards[1].Part != "wiring" {
			t.Errorf("JoinParts() cards = %+v; want core's card then wiring's", result.Cards)
		}
		if _, err := planparser.ParsePlan(planparser.PlanDir(anchorPath)); err != nil {
			t.Errorf("ParsePlan(joined) error = %v; want nil", err)
		}
	})

	t.Run("refusals name the part and a way forward", func(t *testing.T) {
		t.Parallel()
		missingOverview, missingWorktree := seed(t, wiringUsing(newThing))
		if err := os.Remove(partsparser.PartOverview(missingOverview, "wiring")); err != nil {
			t.Fatalf("Remove() = %v; want nil", err)
		}
		failing, failingWorktree := seed(t, wiringUsing("plan:internal/z#Nope"))
		absent, absentWorktree := t.TempDir(), splitWorktree(t)
		// Two parts that both create one handle pass their own checks, so the format writer's refusal is passed through.
		clash, clashWorktree := seed(t, splitCard(1, "wire", "Create", "plan:internal/x#NewThing` -> `func NewThing()"))

		for _, tt := range []struct {
			name         string
			anchor, tree string
			want         []string
			// passedThrough marks the format writer's own refusal, which JoinParts returns unchanged.
			passedThrough bool
		}{
			{"absent parts file", absent, absentWorktree, []string{"lym loom goto --to Parts-Write"}, false},
			{"missing part overview", missingOverview, missingWorktree, []string{`part "wiring"`, partsparser.PartOverview(missingOverview, "wiring")}, false},
			{"part failing its check", failing, failingWorktree, []string{`part "wiring"`, "handle-dangling", "lym loom join-parts"}, false},
			{"planparser refusal", clash, clashWorktree, []string{"both create"}, true},
		} {
			_, err := JoinParts(tt.anchor, tt.tree, index, PlanContext{})
			if err == nil {
				t.Errorf("%s: JoinParts() error = nil; want a refusal", tt.name)
				continue
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s: JoinParts() error = %q; want it to contain %q", tt.name, err, want)
				}
			}
			if tt.passedThrough && !errors.Is(err, planparser.ErrJoinRefused) {
				t.Errorf("%s: JoinParts() error = %v; want it to wrap planparser.ErrJoinRefused", tt.name, err)
			}
		}
	})
}
