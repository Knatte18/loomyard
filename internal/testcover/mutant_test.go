package testcover_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// mutantSource is the fixture file the mutant tests plan over.
const mutantSource = `package fix

func Classify(n int) string {
	if n < 0 {
		return "negative"
	}
	return "non-negative"
}

func Count(xs []int) int {
	total := 0
	for i := 0; i < len(xs); i++ {
		total += xs[i]
	}
	const limit = 3
	_ = limit
	return total
}

func Join(x, y []int) int {
	var total int
	total += len(x) +
		len(y)
	return total
}
`

// Indexes of the fixture's blocks in mutantBlocks, in the order a coverage profile lists them.
const (
	blockClassifyHead = iota
	blockClassifyNegative
	blockClassifyTail
	blockCountHead
	blockCountLoop
	blockCountTail
	blockJoinSplitStatement
)

// mutantBlocks returns the coverage blocks of mutantSource, each spanning the text from one marker to the end of another.
func mutantBlocks(t *testing.T) []testcover.ProfileBlock {
	t.Helper()
	spans := [][2]string{
		{"{\n\tif n < 0", "if n < 0 {"},
		{"{\n\t\treturn \"negative\"", "\"negative\"\n\t}"},
		{"return \"non-negative\"", "\"non-negative\"\n}"},
		{"{\n\ttotal := 0", "i++ {"},
		{"{\n\t\ttotal +=", "xs[i]\n\t}"},
		{"const limit", "return total\n}"},
		{"total += len(x)", "len(y)"},
	}
	var blocks []testcover.ProfileBlock
	for _, span := range spans {
		start := strings.Index(mutantSource, span[0])
		end := start + strings.Index(mutantSource[start:], span[1]) + len(span[1])
		if start < 0 || end < start {
			t.Fatalf("the fixture holds no span from %q to %q", span[0], span[1])
		}
		startLine, startCol := lineCol(mutantSource, start)
		endLine, endCol := lineCol(mutantSource, end)
		blocks = append(blocks, testcover.ProfileBlock{
			File: "m/fix/fix.go", StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol, Statements: 1,
		})
	}
	return blocks
}

// lineCol returns the line and byte column, both counted from 1, of offset in text.
func lineCol(text string, offset int) (int, int) {
	before := text[:offset]
	return strings.Count(before, "\n") + 1, offset - strings.LastIndex(before, "\n")
}

// planFixture writes mutantSource into a package directory and plans the mutants of the blocks targets indexes.
func planFixture(t *testing.T, targets ...int) (string, *testcover.MutantPlan) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fix.go"), []byte(mutantSource), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks := mutantBlocks(t)
	keys, err := testcover.KeyBlocks(dir, blocks)
	if err != nil {
		t.Fatal(err)
	}
	set := testcover.BlockSet{}
	for _, i := range targets {
		set[keys[i].String()] = struct{}{}
	}
	plan, err := testcover.PlanMutants(dir, "m/fix", blocks, set)
	if err != nil {
		t.Fatal(err)
	}
	return dir, plan
}

//testtiming:keep pins each site's operator and line and the sites the mutator leaves alone
func TestPlanMutants(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type site struct {
		Line     int
		Operator string
	}
	tests := []struct {
		name    string
		targets []int
		want    []site
	}{
		{
			name:    "a condition and its literals become mutants, and a site of a block no target holds does not",
			targets: []int{blockClassifyHead},
			want:    []site{{4, testcover.OperatorNegateCondition}, {4, testcover.OperatorChangeConstant}},
		},
		{
			name:    "a string literal is a constant to change",
			targets: []int{blockClassifyNegative, blockClassifyTail},
			want:    []site{{5, testcover.OperatorChangeConstant}, {7, testcover.OperatorChangeConstant}},
		},
		{
			name:    "a declaring assignment and a for post statement are kept while their literals and the condition change",
			targets: []int{blockCountHead},
			want:    []site{{11, testcover.OperatorChangeConstant}, {12, testcover.OperatorChangeConstant}, {12, testcover.OperatorNegateCondition}},
		},
		{
			name:    "an assignment statement is dropped",
			targets: []int{blockCountLoop},
			want:    []site{{13, testcover.OperatorDropStatement}},
		},
		{
			name:    "a constant declaration and a blank assignment hold no mutant",
			targets: []int{blockCountTail},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, plan := planFixture(t, tt.targets...)
			var got []site
			for i, m := range plan.Mutants {
				if m.ID != i+1 || m.File != "fix.go" {
					t.Errorf("mutant %d = %+v; want ID %d in fix.go", i, m, i+1)
				}
				got = append(got, site{m.Line, m.Operator})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PlanMutants sites = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestMutantPlan_WriteOverlay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir, plan := planFixture(t, blockClassifyHead, blockClassifyNegative, blockClassifyTail, blockCountLoop)
	out := t.TempDir()
	overlay, err := plan.WriteOverlay(out, map[int]bool{2: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Replace map[string]string }
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	helpers := filepath.Join(dir, "lym_testcover_mutant.go")
	wantReplace := map[string]string{filepath.Join(dir, "fix.go"): filepath.Join(out, "fix.go"), helpers: filepath.Join(out, "lym_testcover_mutant.go")}
	if !reflect.DeepEqual(got.Replace, wantReplace) {
		t.Fatalf("overlay Replace = %v; want %v", got.Replace, wantReplace)
	}

	mutated, err := os.ReadFile(got.Replace[filepath.Join(dir, "fix.go")])
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"if n < 0 {", "if lymMutantBool(n < 0, 1) {",
		`return "negative"`, `return lymMutantString("negative", 3)`,
		`return "non-negative"`, `return lymMutantString("non-negative", 4)`,
		"total += xs[i]", "if lymMutantKeep(5) { total += xs[i] }",
	).Replace(mutantSource)
	if string(mutated) != want {
		t.Errorf("mutated fix.go =\n%s\nwant, with the dropped mutant 2 left out and no line added,\n%s", mutated, want)
	}
	switchFile, err := os.ReadFile(got.Replace[helpers])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(switchFile), "package fix\n") || !strings.Contains(string(switchFile), testcover.MutantEnv) {
		t.Errorf("helper file =\n%s\nwant one in package fix reading %s", switchFile, testcover.MutantEnv)
	}
}

func TestMutantPlan_UnbuildableMutants(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	classify := []int{blockClassifyHead, blockClassifyNegative}
	tests := []struct {
		name    string
		targets []int
		err     error
		dropped map[int]bool
		want    []int
	}{
		{name: "every mutant spanning a refused line", targets: classify, err: &testcover.MutantBuildError{Lines: []testcover.SourceLine{{File: "fix.go", Line: 4}}}, want: []int{1, 2}},
		{name: "a dropped mutant stays dropped", targets: classify, err: &testcover.MutantBuildError{Lines: []testcover.SourceLine{{File: "fix.go", Line: 4}}}, dropped: map[int]bool{1: true}, want: []int{2}},
		{name: "a line of another file", targets: classify, err: &testcover.MutantBuildError{Lines: []testcover.SourceLine{{File: "other.go", Line: 5}}}},
		{name: "an error that names no line", targets: classify, err: errors.New("mutant binary: timed out")},
		{name: "a mutant spanning two lines when the second is refused", targets: []int{blockJoinSplitStatement}, err: &testcover.MutantBuildError{Lines: []testcover.SourceLine{{File: "fix.go", Line: 23}}}, want: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, plan := planFixture(t, tt.targets...)
			if got := plan.UnbuildableMutants(tt.err, tt.dropped); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnbuildableMutants = %v; want %v", got, tt.want)
			}
		})
	}
}
