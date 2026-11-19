// linemutant.go plans the mutants of the lines a diff adds or changes, in the blocks a map's tests cover.
// It takes the sites PlanMutants scans and keeps one per changed line.

package testcover

import (
	"bytes"
	"go/token"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// loggerImportPath is the package whose dropped calls get no line mutant, since a log line changes nothing a test checks.
const loggerImportPath = "github.com/Knatte18/loomyard/internal/logger"

// PlanLineMutants returns the mutants of the lines changed names, in the blocks covered names.
// profile is the package's block table from its coverage binary, as PlanMutants takes it.
// changed maps a file name of the package to its changed 1-based line numbers.
// covered holds the rendered block keys, as KeyBlocks and BlockKey.String render them, that a map's measured tests cover.
// A mutant lands on a changed line only inside a block covered holds.
// At most one lands on a line: the first site of the operator order.
// The order is a negated condition or comparison, a deleted operand of a && or || chain.
// Then come a dropped statement and a shifted integer or rune constant.
// A string constant, a line of an embedded file and a dropped call into the logger package get none.
// A block that holds a changed line and that covered does not name gets no mutant and comes back in the second result.
// A block holds a line when it covers a byte of it that is neither blank nor a lone brace,
// so a block that only shares a brace with a changed line's block does not hold it.
// The plan serves WriteOverlay, BuildMutantBinary, UnbuildableMutants and MutantEnv as one from PlanMutants does.
// A file that does not parse, or a block outside its file, is an error.
func PlanLineMutants(dir, importPath string, profile []ProfileBlock, changed map[string][]int, covered BlockSet) (*MutantPlan, []BlockKey, error) {
	plan, err := PlanMutants(dir, importPath, profile, covered)
	if err != nil {
		return nil, nil, err
	}
	type changedLine struct {
		file string
		line int
	}
	first := map[changedLine]int{}
	for i, mutant := range plan.Mutants {
		if !slices.Contains(changed[mutant.File], mutant.Line) || !mutant.mutatesBehavior() {
			continue
		}
		key := changedLine{mutant.File, mutant.Line}
		if at, taken := first[key]; !taken || operatorRank(mutant.Operator) < operatorRank(plan.Mutants[at].Operator) {
			first[key] = i
		}
	}
	mutants := make([]Mutant, 0, len(first))
	sources := map[string][]byte{}
	for _, i := range slices.Sorted(maps.Values(first)) {
		mutant := plan.Mutants[i]
		mutant.ID = len(mutants) + 1
		mutants = append(mutants, mutant)
		sources[mutant.File] = plan.sources[mutant.File]
	}
	plan.Mutants, plan.sources = mutants, sources

	uncovered, err := uncoveredChangedBlocks(dir, importPath, profile, changed, covered)
	if err != nil {
		return nil, nil, err
	}
	return plan, uncovered, nil
}

// FuncOf returns the function the mutant's first block lies in, as BlockKey.Func names it.
func (m Mutant) FuncOf() string {
	// A rendered key is `file|function|hash|ordinal`, and neither a file name nor a function name holds a bar.
	return strings.SplitN(m.Blocks[0], "|", 3)[1]
}

// mutatesBehavior reports whether the mutant is of a kind a line mutant keeps.
func (m Mutant) mutatesBehavior() bool {
	switch {
	case m.Operator == OperatorChangeEmbeddedLine:
		return false
	case m.Operator == OperatorChangeConstant && m.helper == literalHelper(token.STRING):
		return false
	}
	return m.callee != loggerImportPath
}

// operatorRank orders the operators a line mutant chooses among, the first to be chosen lowest.
func operatorRank(operator string) int {
	switch operator {
	case OperatorNegateCondition, OperatorNegateComparison:
		return 0
	case OperatorDeleteClause:
		return 1
	case OperatorDropStatement:
		return 2
	}
	return 3
}

// uncoveredChangedBlocks returns the blocks of the package at importPath that hold a changed line and that covered lacks.
// They come in the profile's order, each once.
func uncoveredChangedBlocks(dir, importPath string, profile []ProfileBlock, changed map[string][]int, covered BlockSet) ([]BlockKey, error) {
	own := ownBlocks(profile, importPath)
	keys, err := KeyBlocks(dir, own)
	if err != nil {
		return nil, err
	}
	files := map[string]*sourceFile{}
	var uncovered []BlockKey
	for i, block := range own {
		if _, isCovered := covered[keys[i].String()]; isCovered {
			continue
		}
		name := path.Base(block.File)
		src, parsed := files[name]
		if !parsed {
			if src, err = parseSourceFile(filepath.Join(dir, name)); err != nil {
				return nil, err
			}
			files[name] = src
		}
		for _, line := range changed[name] {
			holds, err := blockHoldsLine(src, block, line)
			if err != nil {
				return nil, err
			}
			if holds {
				uncovered = append(uncovered, keys[i])
				break
			}
		}
	}
	return uncovered, nil
}

// blockHoldsLine reports whether the block covers a byte of the 1-based line that is neither blank nor a lone brace.
func blockHoldsLine(src *sourceFile, block ProfileBlock, line int) (bool, error) {
	if line < block.StartLine || line > block.EndLine {
		return false, nil
	}
	start, err := src.offset(block.StartLine, block.StartCol)
	if err != nil {
		return false, err
	}
	end, err := src.offset(block.EndLine, block.EndCol)
	if err != nil {
		return false, err
	}
	lineStart, err := src.offset(line, 1)
	if err != nil {
		return false, err
	}
	lineEnd := len(src.text)
	if line < src.file.LineCount() {
		if lineEnd, err = src.offset(line+1, 1); err != nil {
			return false, err
		}
	}
	code := bytes.TrimSpace(src.text[max(start, lineStart):min(end, lineEnd)])
	return len(code) > 0 && string(code) != "{" && string(code) != "}", nil
}
