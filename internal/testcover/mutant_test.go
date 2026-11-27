package testcover_test

import (
	"cmp"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

const suffix = "!"

var names = []string{"a", "b"}

//go:embed fix.txt
var text string

func Pick(a, b bool, s string) string {
	if a && b || s == names[0] {
		return s + suffix
	}
	return text
}

func Shadow(text string) string {
	suffix := text
	names := suffix
	return names
}

func Chain(a, b, c bool) bool {
	return a && b && c
}

var set = map[string]int{"k": 1}

var keyed = []string{0: "a"}

var base = []string{"x"}

var all = append(base, "y")

func Reads() int {
	return len(set) + len(keyed) + len(all)
}

func Tagged(a bool) string {
	if a {
		return tag
	}
	return tag
}

func Half() uint64 {
	return (1 << 63) / 2
}
`

// mutantVariables is the fixture file that holds only a package-level variable, which two blocks of mutantSource read.
const mutantVariables = "package fix\n\nvar tag = \"t\"\n"

// mutantEmbedded is the file the fixture's embed directive names; lines 1 and 3 are not blank.
const mutantEmbedded = "one\n\n  two\n"

// Indexes of the fixture's blocks in mutantBlocks, in the order a coverage profile lists them.
const (
	blockClassifyHead = iota
	blockClassifyNegative
	blockClassifyTail
	blockCountHead
	blockCountLoop
	blockCountTail
	blockJoinSplitStatement
	blockPickHead
	blockPickTrue
	blockPickTail
	blockShadow
	blockChain
	blockReads
	blockTaggedTrue
	blockTaggedTail
	blockHalf
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
		{"{\n\tif a && b", "names[0] {"},
		{"{\n\t\treturn s + suffix", "suffix\n\t}"},
		{"return text\n}", "return text\n}"},
		{"{\n\tsuffix := text", "return names\n}"},
		{"{\n\treturn a && b && c", "a && b && c\n}"},
		{"{\n\treturn len(set)", "len(all)\n}"},
		{"return tag\n\t}", "return tag\n\t}"},
		{"return tag\n}", "return tag\n}"},
		{"{\n\treturn (1 << 63)", "/ 2\n}"},
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

// planFixture writes mutantSource and its embedded file into a package directory.
// It plans the mutants of the blocks targets indexes, and returns the rendered key of each fixture block too.
func planFixture(t *testing.T, targets ...int) (string, *testcover.MutantPlan, []string) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"fix.go": mutantSource, "fixvars.go": mutantVariables, "fix.txt": mutantEmbedded} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
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
	rendered := make([]string, len(keys))
	for i, key := range keys {
		rendered[i] = key.String()
	}
	return dir, plan, rendered
}

//testtiming:keep pins each site's operator, line and blocks and the sites the mutator leaves alone
func TestPlanMutants(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// site is a mutant's line and operator, with its blocks as indexes of the fixture's blocks.
	type site struct {
		Line     int
		Operator string
		Blocks   []int
	}
	tests := []struct {
		name    string
		targets []int
		// file is the file every mutant is in; fix.go when empty.
		file string
		want []site
	}{
		{
			name:    "a condition and its literals become mutants, and a site of a block no target holds does not",
			targets: []int{blockClassifyHead},
			want: []site{
				{4, testcover.OperatorNegateCondition, []int{blockClassifyHead}},
				{4, testcover.OperatorChangeConstant, []int{blockClassifyHead}},
			},
		},
		{
			name:    "a string literal is a constant to change",
			targets: []int{blockClassifyNegative, blockClassifyTail},
			want: []site{
				{5, testcover.OperatorChangeConstant, []int{blockClassifyNegative}},
				{7, testcover.OperatorChangeConstant, []int{blockClassifyTail}},
			},
		},
		{
			name:    "a declaring assignment and a for post statement are kept while their literals and the condition change",
			targets: []int{blockCountHead},
			want: []site{
				{11, testcover.OperatorChangeConstant, []int{blockCountHead}},
				{12, testcover.OperatorChangeConstant, []int{blockCountHead}},
				{12, testcover.OperatorNegateCondition, []int{blockCountHead}},
			},
		},
		{
			name:    "an assignment statement is dropped",
			targets: []int{blockCountLoop},
			want:    []site{{13, testcover.OperatorDropStatement, []int{blockCountLoop}}},
		},
		{
			name:    "a constant declaration and a blank assignment hold no mutant",
			targets: []int{blockCountTail},
		},
		{
			name:    "each operand of a boolean chain is deleted, and a package variable's literals belong to the block that reads it",
			targets: []int{blockPickHead},
			want: []site{
				{29, testcover.OperatorChangeConstant, []int{blockPickHead}},
				{29, testcover.OperatorChangeConstant, []int{blockPickHead}},
				{35, testcover.OperatorNegateCondition, []int{blockPickHead}},
				{35, testcover.OperatorDeleteClause, []int{blockPickHead}},
				{35, testcover.OperatorDeleteClause, []int{blockPickHead}},
				{35, testcover.OperatorDeleteClause, []int{blockPickHead}},
				{35, testcover.OperatorDeleteClause, []int{blockPickHead}},
				{35, testcover.OperatorNegateComparison, []int{blockPickHead}},
				{35, testcover.OperatorChangeConstant, []int{blockPickHead}},
			},
		},
		{
			name:    "a use of a package constant is a constant to change, and a package variable no target reads holds no mutant",
			targets: []int{blockPickTrue},
			want:    []site{{36, testcover.OperatorChangeConstant, []int{blockPickTrue}}},
		},
		{
			name:    "a use of an embedded string is a mutant for each non-blank line of its file",
			targets: []int{blockPickTail},
			want: []site{
				{38, testcover.OperatorChangeEmbeddedLine, []int{blockPickTail}},
				{38, testcover.OperatorChangeEmbeddedLine, []int{blockPickTail}},
			},
		},
		{
			name:    "a parameter or local named as a package constant or variable is no use of it",
			targets: []int{blockShadow},
		},
		{
			name:    "each operand of a chain of three is deleted once",
			targets: []int{blockChain},
			want: []site{
				{48, testcover.OperatorDeleteClause, []int{blockChain}},
				{48, testcover.OperatorDeleteClause, []int{blockChain}},
				{48, testcover.OperatorDeleteClause, []int{blockChain}},
			},
		},
		{
			name:    "a map's string key and value change, an array index stays, and a variable only another variable's value names is read where that one is",
			targets: []int{blockReads},
			want: []site{
				{51, testcover.OperatorChangeConstant, []int{blockReads}},
				{51, testcover.OperatorChangeConstant, []int{blockReads}},
				{53, testcover.OperatorChangeConstant, []int{blockReads}},
				{55, testcover.OperatorChangeConstant, []int{blockReads}},
				{57, testcover.OperatorChangeConstant, []int{blockReads}},
			},
		},
		{
			name:    "a variable of a file holding no block belongs to every block that reads it, though only the later one is a target",
			targets: []int{blockTaggedTail},
			file:    "fixvars.go",
			want:    []site{{3, testcover.OperatorChangeConstant, []int{blockTaggedTrue, blockTaggedTail}}},
		},
		{
			name:    "the literals of an expression of integer constants alone stay, as its intermediate value may overflow once it is not constant",
			targets: []int{blockHalf},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, plan, keys := planFixture(t, tt.targets...)
			var got []site
			for i, m := range plan.Mutants {
				if m.ID != i+1 || m.File != cmp.Or(tt.file, "fix.go") {
					t.Errorf("mutant %d = %+v; want ID %d in %s", i, m, i+1, cmp.Or(tt.file, "fix.go"))
				}
				if !slices.IsSorted(m.Blocks) {
					t.Errorf("mutant %d blocks = %v; want them ascending", i, m.Blocks)
				}
				// The keys order by their hash, so the fixture's block indexes are compared in index order.
				var blocks []int
				for _, key := range m.Blocks {
					blocks = append(blocks, slices.Index(keys, key))
				}
				slices.Sort(blocks)
				got = append(got, site{m.Line, m.Operator, blocks})
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

	dir, plan, _ := planFixture(t, blockClassifyHead, blockClassifyNegative, blockClassifyTail, blockCountLoop, blockPickHead, blockPickTrue, blockPickTail)
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
		`[]string{"a", "b"}`, `[]string{lymMutantString("a", 6), lymMutantString("b", 7)}`,
		"if a && b || s == names[0] {",
		"if lymMutantBool(lymMutantDropOr(lymMutantDropAnd(a, 10) && lymMutantDropAnd(b, 11), 9) || "+
			"lymMutantDropOr(lymMutantBool(s == names[(0 + 1>>lymMutantStep(14))], 13), 12), 8) {",
		"return s + suffix", "return s + lymMutantString(suffix, 15)",
		"return text\n}", "return lymMutantLine(lymMutantLine(text, 3, 17), 1, 16)\n}",
	).Replace(mutantSource)
	if string(mutated) != want {
		t.Errorf("mutated fix.go =\n%s\nwant, with the dropped mutant 2 left out, two mutants of one site nested in order "+
			"and no line added,\n%s", mutated, want)
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

	// The targets hold mutants 1 and 2 on line 4 and 3 on line 5 of fix.go, and mutant 4 on line 3 of fixvars.go.
	targets := []int{blockClassifyHead, blockClassifyNegative, blockTaggedTail}
	refused := func(file string, line int) error {
		return &testcover.MutantBuildError{Lines: []testcover.SourceLine{{File: file, Line: line}}}
	}
	tests := []struct {
		name       string
		targets    []int
		err        error
		dropped    map[int]bool
		wholeFiles bool
		want       []int
	}{
		{name: "every mutant spanning a refused line", targets: targets, err: refused("fix.go", 4), want: []int{1, 2}},
		{name: "a dropped mutant stays dropped", targets: targets, err: refused("fix.go", 4), dropped: map[int]bool{1: true}, want: []int{2}},
		{name: "every mutant of the file when no mutant spans its refused line", targets: targets, err: refused("fix.go", 1), want: []int{1, 2, 3}},
		{name: "every mutant of the file of a refused line with wholeFiles", targets: targets, err: refused("fix.go", 4), wholeFiles: true, want: []int{1, 2, 3}},
		{name: "every mutant left for a line of a file holding none", targets: targets, err: refused("other.go", 5), dropped: map[int]bool{2: true}, want: []int{1, 3, 4}},
		{name: "none once every mutant is dropped", targets: targets, err: refused("other.go", 5), dropped: map[int]bool{1: true, 2: true, 3: true, 4: true}},
		{name: "an error that names no line", targets: targets, err: errors.New("mutant binary: timed out")},
		{name: "a mutant spanning two lines when the second is refused", targets: []int{blockJoinSplitStatement}, err: refused("fix.go", 23), want: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, plan, _ := planFixture(t, tt.targets...)
			if got := plan.UnbuildableMutants(tt.err, tt.dropped, tt.wholeFiles); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnbuildableMutants = %v; want %v", got, tt.want)
			}
		})
	}
}
