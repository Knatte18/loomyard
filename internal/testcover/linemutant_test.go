package testcover_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// lineMutantSource is the fixture file the line mutant tests plan over.
// Line 13 holds a condition and an integer constant.
// Line 20 holds a call into the logger package, and line 21 a string constant.
// Line 22 holds a use of an embedded file, line 26 an increment and line 31 an integer constant alone.
// Line 35 holds an && chain and an integer constant, and line 39 a comparison and an integer constant.
// Line 43 holds a call on a local named like the logger package, and line 47 a call through an aliased logger import.
// Line 51 holds a rune constant.
// Line 55 opens an if that only logs, line 56 logs inside it, and line 58 opens an if that logs and returns.
// Line 66 compares runtime.GOOS with another platform.
// Line 69 compares it with this one, whose name the fixture writes over HOSTOS.
// Line 76 opens an if-else whose branches only log, and line 81 an if with an else-if, both only logging.
// Line 89 opens an if with an init statement whose body only logs.
// Line 95 compares runtime.GOOS with another platform, the literal first.
// Line 99 does so on a parameter named runtime.
// Line 107 opens an if whose condition calls a function and whose body only logs.
const lineMutantSource = `package lines
import "runtime"
import (
	_ "embed"

	"github.com/Knatte18/loomyard/internal/logger"
)
import logalias "github.com/Knatte18/loomyard/internal/logger"
//go:embed lines.txt
var banner string

func Check(n int) bool {
	if n > 3 {
		return true
	}
	return false
}

func Report(n int) string {
	logger.Info("reporting")
	label := "count"
	return label + banner
}

func Spare(n int) int {
	n++
	return n
}

func Limit() int {
	return 7
}

func Guard(a bool) bool {
	return a && probe(5)
}

func probe(n int) bool {
	return n > 0
}

func Shadow(logger interface{ Info(string) }) {
	logger.Info("shadow")
}

func Aliased() {
	logalias.Info("aliased")
}

func Rune() rune {
	return 'a'
}

func Throttle(n int) int {
	if n > 10 {
		logger.Info("many", "n", n+1)
	}
	if n > 20 {
		logger.Info("too many")
		return 20
	}
	return n
}

func Platform() int {
	if runtime.GOOS == "plan9" {
		return 1
	}
	if runtime.GOOS != "HOSTOS" {
		return 2
	}
	return 0
}

func Branches(n int) {
	if n > 1 {
		logger.Info("one")
	} else {
		logger.Info("other")
	}
	if n > 2 {
		logger.Info("two")
	} else if n > 4 {
		logger.Info("four")
	}
}

func Initialised(n int) {
	if m := n + 1; m > 2 {
		logger.Info("init")
	}
}

func Reversed() bool {
	return "plan9" == runtime.GOOS
}

func Shadowed(runtime struct{ GOOS string }) bool {
	return runtime.GOOS == "plan9"
}

func tick(step int) int {
	return step
}

func Counted() {
	if tick(1) > 10 {
		logger.Info("many")
	}
}
`

// Indexes of the fixture's blocks in lineMutantBlocks, in the order a coverage profile lists them.
const (
	blockCheckHead = iota
	blockCheckThen
	blockCheckTail
	blockReport
	blockSpare
	blockLimit
)

// lineMutantFixture writes the line mutant fixture package into a temporary directory and returns the directory.
func lineMutantFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := strings.ReplaceAll(lineMutantSource, "HOSTOS", runtime.GOOS)
	for name, text := range map[string]string{"lines.go": source, "lines.txt": "one\ntwo\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// lineMutantBlocks returns the coverage blocks of lineMutantSource.
// Each block spans the text from one marker to the end of another.
// The head and then blocks of Check share the brace that ends line 13.
// No block starts or ends on the HOSTOS line, so its positions hold for the file the fixture writes.
func lineMutantBlocks(t *testing.T) []testcover.ProfileBlock {
	t.Helper()
	spans := [][2]string{
		{"{\n\tif n > 3", "if n > 3 {"},
		{"{\n\t\treturn true", "return true\n\t}"},
		{"return false\n}", "return false\n}"},
		{"{\n\tlogger.Info", "banner\n}"},
		{"{\n\tn++", "return n\n}"},
		{"{\n\treturn 7", "return 7\n}"},
		{"{\n\treturn a && probe(5)", "probe(5)\n}"},
		{"{\n\treturn n > 0", "n > 0\n}"},
		{"{\n\tlogger.Info(\"shadow\")", "\"shadow\")\n}"},
		{"{\n\tlogalias.Info", "\"aliased\")\n}"},
		{"{\n\treturn 'a'", "'a'\n}"},
		{"{\n\tif n > 10", "return n\n}"},
		{"{\n\tif runtime.GOOS", "return 0\n}"},
		{"{\n\tif n > 1", "\"four\")\n\t}\n}"},
		{"{\n\tif m := n + 1", "\"init\")\n\t}\n}"},
		{"{\n\treturn \"plan9\" == runtime.GOOS", "runtime.GOOS\n}"},
		{"{\n\treturn runtime.GOOS == \"plan9\"", "\"plan9\"\n}"},
		{"{\n\treturn step", "step\n}"},
		{"{\n\tif tick(1)", "\"many\")\n\t}\n}"},
	}
	var blocks []testcover.ProfileBlock
	for _, span := range spans {
		start := strings.Index(lineMutantSource, span[0])
		end := start + strings.Index(lineMutantSource[start:], span[1]) + len(span[1])
		if start < 0 || end < start {
			t.Fatalf("the fixture holds no span from %q to %q", span[0], span[1])
		}
		startLine, startCol := lineCol(lineMutantSource, start)
		endLine, endCol := lineCol(lineMutantSource, end)
		blocks = append(blocks, testcover.ProfileBlock{
			File: "m/lines/lines.go", StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol, Statements: 1,
		})
	}
	return blocks
}

func TestPlanLineMutants(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	everyLine := make([]int, strings.Count(lineMutantSource, "\n"))
	for i := range everyLine {
		everyLine[i] = i + 1
	}
	tests := []struct {
		name    string
		changed []int
		// uncoveredBlocks are the fixture blocks the covered set leaves out.
		uncoveredBlocks []int
		wantLines       []int
		wantOperators   []string
		// wantUncovered are the functions of the blocks that come back uncovered.
		wantUncovered []string
		// wantText are texts the plan's overlay of the file must hold.
		wantText []string
	}{
		{
			name: "EveryChangedLineGetsAtMostOneMutantByOperatorOrderAndNoneForAStringAnEmbeddedFileOrALoggerCall", changed: everyLine,
			wantLines: []int{13, 26, 31, 35, 39, 43, 51, 58, 60, 67, 69, 70, 72, 89, 99, 107},
			wantOperators: []string{
				testcover.OperatorNegateCondition, testcover.OperatorDropStatement, testcover.OperatorChangeConstant,
				testcover.OperatorDeleteClause, testcover.OperatorNegateComparison, testcover.OperatorDropStatement, testcover.OperatorChangeConstant,
				testcover.OperatorNegateCondition, testcover.OperatorChangeConstant, testcover.OperatorChangeConstant,
				testcover.OperatorNegateCondition, testcover.OperatorChangeConstant, testcover.OperatorChangeConstant,
				testcover.OperatorNegateCondition, testcover.OperatorNegateComparison, testcover.OperatorNegateCondition,
			},
			wantText: []string{
				"if lymMutantBool(n > 3, 1) {",
				"if lymMutantKeep(2) { n++ }",
				"return (7 + 1>>lymMutantStep(3))",
				"return lymMutantDropAnd(a, 4) && probe(5)",
				"return lymMutantBool(n > 0, 5)",
				"if lymMutantKeep(6) { logger.Info(\"shadow\") }",
				"return ('a' + 1>>lymMutantStep(7))",
			},
		},
		{
			name: "ADeletedOperandOfAChainOutranksAConstantOnItsLine", changed: []int{35},
			wantLines: []int{35}, wantOperators: []string{testcover.OperatorDeleteClause},
		},
		{
			name: "ACallOnALocalNamedLikeTheLoggerPackageGetsAMutantButACallThroughAnAliasedLoggerImportGetsNone", changed: []int{43, 47},
			wantLines: []int{43}, wantOperators: []string{testcover.OperatorDropStatement},
		},
		{
			name: "ARuneConstantIsShifted", changed: []int{51},
			wantLines: []int{51}, wantOperators: []string{testcover.OperatorChangeConstant},
		},
		{
			name: "AnUnchangedCoveredLineGetsNone", changed: []int{13},
			wantLines: []int{13}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "ALineOfALoggerCallAStringAndAnEmbeddedFileUseGetsNone", changed: []int{20, 21, 22},
		},
		{
			name: "AnIfThatOnlyLogsGetsNoneButAnIfThatAlsoReturnsKeepsItsCondition", changed: []int{55, 56, 58, 59},
			wantLines: []int{58}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "AComparisonOfTheOSWithAnotherPlatformGetsNoneButOneWithThisPlatformKeepsItsCondition", changed: []int{66, 69},
			wantLines: []int{69}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "AnIfWhoseBranchesAllOnlyLogGetsNoneThroughItsElseAndItsElseIf", changed: []int{76, 81, 83},
		},
		{
			name: "AnIfWithAnInitStatementKeepsItsConditionEvenWhenItsBodyOnlyLogs", changed: []int{89},
			wantLines: []int{89}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "AComparisonWithTheLiteralFirstNamingAnotherPlatformGetsNone", changed: []int{95},
		},
		{
			name: "AComparisonOnAParameterNamedRuntimeKeepsItsMutant", changed: []int{99},
			wantLines: []int{99}, wantOperators: []string{testcover.OperatorNegateComparison},
		},
		{
			name: "AnIfWhoseConditionCallsAFunctionKeepsItsConditionEvenWhenItsBodyOnlyLogs", changed: []int{107},
			wantLines: []int{107}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "AChangedLineInAnUncoveredBlockGetsNoMutantAndItsBlockComesBackUncovered", changed: []int{26},
			uncoveredBlocks: []int{blockSpare}, wantUncovered: []string{"Spare"},
		},
		{
			name: "ABlockThatOnlySharesABraceWithAChangedLineIsNotUncovered", changed: []int{13},
			uncoveredBlocks: []int{blockCheckThen},
			wantLines:       []int{13}, wantOperators: []string{testcover.OperatorNegateCondition},
		},
		{
			name: "ABlockHoldingAChangedLineComesBackUncoveredWhenAnotherBlockOfItsFunctionIsCovered", changed: []int{14},
			uncoveredBlocks: []int{blockCheckThen}, wantUncovered: []string{"Check"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := lineMutantFixture(t)
			blocks := lineMutantBlocks(t)
			keys, err := testcover.KeyBlocks(dir, blocks)
			if err != nil {
				t.Fatal(err)
			}
			covered := testcover.BlockSet{}
			for i, key := range keys {
				if !slices.Contains(tt.uncoveredBlocks, i) {
					covered[key.String()] = struct{}{}
				}
			}
			plan, uncovered, err := testcover.PlanLineMutants(dir, "m/lines", blocks, map[string][]int{"lines.go": tt.changed}, covered)
			if err != nil {
				t.Fatal(err)
			}
			var gotLines []int
			var gotOperators []string
			for i, mutant := range plan.Mutants {
				if mutant.ID != i+1 || mutant.File != "lines.go" {
					t.Errorf("mutant %d = %+v; want ID %d in lines.go", i, mutant, i+1)
				}
				gotLines = append(gotLines, mutant.Line)
				gotOperators = append(gotOperators, mutant.Operator)
			}
			if !slices.Equal(gotLines, tt.wantLines) || !slices.Equal(gotOperators, tt.wantOperators) {
				t.Errorf("PlanLineMutants mutants = lines %v, operators %v; want lines %v, operators %v",
					gotLines, gotOperators, tt.wantLines, tt.wantOperators)
			}
			var gotUncovered []string
			for _, key := range uncovered {
				gotUncovered = append(gotUncovered, key.Func)
			}
			if !slices.Equal(gotUncovered, tt.wantUncovered) {
				t.Errorf("PlanLineMutants uncovered functions = %v; want %v", gotUncovered, tt.wantUncovered)
			}
			if len(tt.wantText) == 0 {
				return
			}
			out := t.TempDir()
			if _, err := plan.WriteOverlay(out, nil); err != nil {
				t.Fatal(err)
			}
			mutated, err := os.ReadFile(filepath.Join(out, "lines.go"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(string(mutated), want) {
					t.Errorf("mutated lines.go =\n%s\nwant it to hold %q", mutated, want)
				}
			}
		})
	}
}
