// mutation_test.go covers what the mutation report step records of one mutation call's findings.

package gatecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestMutationStep_RecordFindings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	source := []byte("package calc\n")
	if err := os.MkdirAll(filepath.Join(root, "calc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "calc", "calc.go"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	module := goModule{dir: root, rel: ".", path: "example.com/m"}
	found := mutationFindings{
		funcs: []testreach.ChangedFunc{{
			Key: testcover.FuncKey{Package: "example.com/m/calc", File: "calc.go", Func: "Clamp"}, Hash: "clamp-hash", New: true,
		}},
		outcome: mutateOutcome{
			counts:    MutationCounts{Run: 1, Killed: 1},
			unjudged:  []MutationUnjudgedMutant{{MutationMutant: MutationMutant{File: "calc/calc.go", Line: 3, Operator: "negate", Function: "Clamp"}, Reason: "timed out"}},
			notJudged: []MutationNotJudged{{Package: "calc/sub", Reason: "plan failed"}},
		},
		notJudged:     []MutationNotJudged{{Package: "calc", Reason: "pre-check"}},
		refreshErrors: []string{"refresh: one package failed"},
	}
	wantUnjudged := []MutationUnjudgedMutant{{MutationMutant: MutationMutant{File: "calc/calc.go", Line: 3, Operator: "negate", Function: "Clamp"}, Reason: "timed out"}}
	wantNotJudged := []MutationNotJudged{{Package: "calc", Reason: "pre-check"}, {Package: "calc/sub", Reason: "plan failed"}}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want MutationReport
	}{
		{
			name: "AFailedCallRecordsItsErrorsAndPartialCountsButNoFunctionOrFileHash",
			ctx:  context.Background(),
			err:  errors.New("acquire a gate slot: refused"),
			want: MutationReport{
				Errors:    []string{"refresh: one package failed", "acquire a gate slot: refused"},
				Mutants:   MutationCounts{Run: 1, Killed: 1},
				Unjudged:  wantUnjudged,
				NotJudged: wantNotJudged,
			},
		},
		{
			name: "ACancelledCallRecordsTheCancellationInPlaceOfTheContextError",
			ctx:  cancelled,
			err:  context.Canceled,
			want: MutationReport{
				Errors:    []string{"refresh: one package failed", reasonCancelled},
				Mutants:   MutationCounts{Run: 1, Killed: 1},
				Unjudged:  wantUnjudged,
				NotJudged: wantNotJudged,
			},
		},
		{
			name: "ASuccessfulCallRecordsItsFunctionsAndTheFileHashesOfTheirPackages",
			ctx:  context.Background(),
			want: MutationReport{
				Errors:     []string{"refresh: one package failed"},
				Mutants:    MutationCounts{Run: 1, Killed: 1},
				Unjudged:   wantUnjudged,
				NotJudged:  wantNotJudged,
				Functions:  []MutationFunction{{Package: "calc", File: "calc/calc.go", Func: "Clamp", Hash: "clamp-hash", New: true}},
				FileHashes: map[string]map[string]string{"calc": {"calc.go": testcover.ContentHash(source)}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got MutationReport
			mutationStep{reach: reachStep{ctx: tt.ctx, root: root}}.recordFindings(&got, module, found, tt.err)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("recordFindings() report mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
