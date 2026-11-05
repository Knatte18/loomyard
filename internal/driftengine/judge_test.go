// judge_test.go — tier 0 tests for the `judgement` gate entry, over a judge request written to a temporary anchor.

package driftengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestJudgementGate covers the entry passing a valid file and failing, with the findings the judge is re-prompted with, on a missing file and on a file carrying another spawn id.
func TestJudgementGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const valid = "spawn_id: spawn-1\n" +
		"cells:\n" +
		"  - id: c1\n" +
		"    ruling: matches\n" +
		"    reason: the new value is the one the why expects\n"
	tests := []struct {
		name         string
		judgement    string
		written      bool
		wantPassed   bool
		wantFindings string
	}{
		{"ValidFile", valid, true, true, ""},
		{"MissingFile", "", false, false, "judgement.yaml is missing"},
		{"AnotherSpawnID", strings.Replace(valid, "spawn-1", "spawn-0", 1), true, false, `spawn_id "spawn-0" is not this spawn's "spawn-1"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := Deps{Paths: NewPaths(t.TempDir())}
			request := JudgeRequest{SpawnID: "spawn-1", Mode: DiffLines, Cells: []JudgeCell{{ID: "c1", Case: "a.txt", Kind: CellChanged}}}
			if err := WriteJudgeRequest(d.Paths, request); err != nil {
				t.Fatalf("WriteJudgeRequest() = %v; want nil", err)
			}
			if tt.written {
				if err := os.MkdirAll(filepath.Dir(d.Paths.Judgement), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(d.Paths.Judgement, []byte(tt.judgement), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			result, err := NewJudgementGate(d)()
			if err != nil {
				t.Fatalf("gate() = _, %v; want nil error", err)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("gate().Passed = %v; want %v (findings %q)", result.Passed, tt.wantPassed, result.Findings)
			}
			if !strings.Contains(result.Findings, tt.wantFindings) || (tt.wantFindings == "" && result.Findings != "") {
				t.Errorf("gate().Findings = %q; want it to hold %q", result.Findings, tt.wantFindings)
			}
		})
	}
}
