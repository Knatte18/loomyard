package shedadapters

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// factsFinding is one finding line of a review fixture.
type factsFinding struct {
	severity burlerengine.Severity
	class    string
}

// writeFactsReview writes round's review file with the given findings, choosing the verdict the parser's consistency rule demands.
func writeFactsReview(t *testing.T, dir string, round int, findings ...factsFinding) {
	t.Helper()
	verdict := "APPROVED"
	var b strings.Builder
	for i, f := range findings {
		if f.severity == burlerengine.SeverityBlocking {
			verdict = "BLOCKING"
		}
		fmt.Fprintf(&b, "  - id: F%d\n    severity: %s\n", i+1, f.severity)
		if f.class != "" {
			fmt.Fprintf(&b, "    class: %s\n", f.class)
		}
		fmt.Fprintf(&b, "    location: a.go\n    summary: finding %d\n", i+1)
	}
	list := "findings: []\n"
	if len(findings) > 0 {
		list = "findings:\n" + b.String()
	}
	content := fmt.Sprintf("---\nverdict: %s\n%s---\nprose\n", verdict, list)
	if err := os.WriteFile(filepath.Join(dir, reportName(round)), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile review round %d = %v; want nil", round, err)
	}
}

// writeFactsLedger writes round's ledger with the given entries as key:status:rounds triples.
func writeFactsLedger(t *testing.T, dir string, round int, entries ...string) {
	t.Helper()
	var b strings.Builder
	for _, e := range entries {
		parts := strings.SplitN(e, ":", 3)
		fmt.Fprintf(&b, "  - key: %s\n    status: %s\n    rounds: %s\n", parts[0], parts[1], parts[2])
	}
	list := "ledger: []\n"
	if len(entries) > 0 {
		list = "ledger:\n" + b.String()
	}
	content := fmt.Sprintf("---\nround: %d\n%s---\nprose\n", round, list)
	if err := os.WriteFile(ledgerPath(dir, round), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile ledger round %d = %v; want nil", round, err)
	}
}

func TestComputeRoundFacts_CountsAndRecurringKeys(t *testing.T) {
	dir := t.TempDir()
	writeFactsReview(t, dir, 1,
		factsFinding{burlerengine.SeverityBlocking, "design"},
		factsFinding{burlerengine.SeverityMedium, "design"},
		factsFinding{burlerengine.SeverityLow, "scope"})
	writeFactsReview(t, dir, 2,
		factsFinding{burlerengine.SeverityMedium, "design"},
		factsFinding{burlerengine.SeverityNit, "consistency"})
	writeFactsReview(t, dir, 3,
		factsFinding{burlerengine.SeverityLow, "decision"})
	writeFactsReview(t, dir, 4)
	writeFactsLedger(t, dir, 1, "alpha:open:[1]", "beta:open:[1]")
	writeFactsLedger(t, dir, 2, "alpha:resolved:[1]", "beta:open:[1, 2]")
	writeFactsLedger(t, dir, 3, "alpha:open:[1, 3]", "beta:open:[1, 2, 3]", "gamma:open:[3]")

	facts := computeRoundFacts(dir, 4, reportName)

	if len(facts.Rows) != 4 {
		t.Fatalf("computeRoundFacts rows = %d; want 4", len(facts.Rows))
	}
	r1 := facts.Rows[0]
	if r1.Findings != 3 || r1.Severity[burlerengine.SeverityBlocking] != 1 || r1.Severity[burlerengine.SeverityMedium] != 1 ||
		r1.Severity[burlerengine.SeverityLow] != 1 || r1.Highest != burlerengine.SeverityBlocking ||
		r1.Class[burlerengine.ClassDesign] != 2 || r1.Class[burlerengine.ClassScope] != 1 || r1.Gating != 2 {
		t.Errorf("round 1 row = %+v; want 3 findings, one per severity but NIT, highest BLOCKING, design 2, scope 1, gating 2", r1)
	}
	r2 := facts.Rows[1]
	if r2.Findings != 2 || r2.Highest != burlerengine.SeverityMedium || r2.Class[burlerengine.ClassConsistency] != 1 || r2.Gating != 1 {
		t.Errorf("round 2 row = %+v; want 2 findings, highest MEDIUM, consistency 1, gating 1", r2)
	}
	r3 := facts.Rows[2]
	if r3.Findings != 1 || r3.Highest != burlerengine.SeverityLow || r3.Gating != 0 {
		t.Errorf("round 3 row = %+v; want 1 finding, highest LOW, gating 0", r3)
	}
	if r4 := facts.Rows[3]; r4.Findings != 0 || r4.Highest != "" || r4.Gating != 0 {
		t.Errorf("round 4 row = %+v; want no findings", r4)
	}

	if len(facts.Recurring) != 2 {
		t.Fatalf("recurring = %+v; want alpha and beta only", facts.Recurring)
	}
	alpha, beta := facts.Recurring[0], facts.Recurring[1]
	if alpha.Key != "alpha" || !alpha.Reopened || fmt.Sprint(alpha.Rounds) != "[1 3]" {
		t.Errorf("alpha = %+v; want reopened with rounds [1 3]", alpha)
	}
	if beta.Key != "beta" || beta.Reopened || fmt.Sprint(beta.Rounds) != "[1 2 3]" {
		t.Errorf("beta = %+v; want not reopened with rounds [1 2 3]", beta)
	}

	out := string(renderRoundFacts(facts))
	if !strings.HasPrefix(out, "# Round 4 facts\n") {
		t.Errorf("render heading = %q; want it to name round 4", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "- `alpha`: rounds 1, 3 (reopened)") || strings.Contains(out, "gamma") {
		t.Errorf("render = %q; want alpha listed as reopened and gamma absent", out)
	}
}

func TestComputeRoundFacts_ClasslessReviewIsAParseErrorRow(t *testing.T) {
	dir := t.TempDir()
	writeFactsReview(t, dir, 1, factsFinding{burlerengine.SeverityMedium, ""})
	writeFactsReview(t, dir, 2, factsFinding{burlerengine.SeverityLow, "scope"})

	facts := computeRoundFacts(dir, 2, reportName)
	if facts.Rows[0].Err == "" {
		t.Errorf("round 1 row = %+v; want a parse error for the missing class", facts.Rows[0])
	}
	if facts.Rows[1].Err != "" || facts.Rows[1].Findings != 1 {
		t.Errorf("round 2 row = %+v; want its counts rendered", facts.Rows[1])
	}

	if err := writeRoundFacts(dir, 2, reportName); err != nil {
		t.Fatalf("writeRoundFacts = %v; want nil", err)
	}
	raw, err := os.ReadFile(factsPath(dir, 2))
	if err != nil {
		t.Fatalf("ReadFile facts = %v; want nil", err)
	}
	if !strings.Contains(string(raw), "parse error:") {
		t.Errorf("facts file = %q; want a parse-error row", raw)
	}
}

func TestComputeRoundFacts_MissingReviewIsAParseErrorRow(t *testing.T) {
	dir := t.TempDir()
	writeFactsReview(t, dir, 2)

	facts := computeRoundFacts(dir, 2, reportName)
	if facts.Rows[0].Err == "" {
		t.Errorf("round 1 row = %+v; want an error for the missing review", facts.Rows[0])
	}
}

//testtiming:keep pins the gating rule of the round facts: MEDIUM design findings are gating, a BLOCKING scope finding is counted but not gating
func TestComputeRoundFacts_GatingRule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		findings     []factsFinding
		wantBlocking int
		wantGating   int
	}{
		{
			name: "MEDIUM design findings count as gating",
			findings: []factsFinding{
				{burlerengine.SeverityMedium, "design"},
				{burlerengine.SeverityMedium, "design"},
			},
			wantGating: 2,
		},
		{
			name:         "a BLOCKING scope finding is not gating",
			findings:     []factsFinding{{burlerengine.SeverityBlocking, "scope"}},
			wantBlocking: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFactsReview(t, dir, 1, tt.findings...)

			row := computeRoundFacts(dir, 1, reportName).Rows[0]
			if row.Severity[burlerengine.SeverityBlocking] != tt.wantBlocking || row.Gating != tt.wantGating {
				t.Errorf("round 1 row = %+v; want BLOCKING %d and gating %d", row, tt.wantBlocking, tt.wantGating)
			}
		})
	}
}

// TestWriteRoundFacts_IsDeterministic also pins the line a render without a recurring ledger key carries.
//
//testtiming:keep pins that two writes of the facts file are byte-identical and the no-recurring-key line, which the judge-call tests do not assert
func TestWriteRoundFacts_IsDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFactsReview(t, dir, 1, factsFinding{burlerengine.SeverityMedium, "design"})
	writeFactsReview(t, dir, 2, factsFinding{burlerengine.SeverityLow, "scope"})
	writeFactsLedger(t, dir, 1, "k:open:[1]", "j:open:[1]")

	if err := writeRoundFacts(dir, 2, reportName); err != nil {
		t.Fatalf("writeRoundFacts first = %v; want nil", err)
	}
	first, err := os.ReadFile(factsPath(dir, 2))
	if err != nil {
		t.Fatalf("ReadFile first = %v; want nil", err)
	}
	if !strings.Contains(string(first), "No ledger key recurs across rounds.") {
		t.Errorf("facts file = %q; want the no-recurring-keys line", first)
	}
	if err := writeRoundFacts(dir, 2, reportName); err != nil {
		t.Fatalf("writeRoundFacts second = %v; want nil", err)
	}
	second, err := os.ReadFile(factsPath(dir, 2))
	if err != nil {
		t.Fatalf("ReadFile second = %v; want nil", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("two renders differ:\n%s\n---\n%s", first, second)
	}
}

//testtiming:keep pins the facts file's name and that it is an input the judge reads, not an output a stale-output archive would move
func TestFactsPath_IsNotAJudgeOutput(t *testing.T) {
	dir := t.TempDir()
	want := factsPath(dir, 3)
	if filepath.Base(want) != "round-3-facts.md" {
		t.Errorf("factsPath base = %q; want round-3-facts.md", filepath.Base(want))
	}
	for _, out := range judgeOutputs(dir, 3) {
		if out == want {
			t.Errorf("judgeOutputs includes %q; the facts file is an input, not an output", want)
		}
	}
}
