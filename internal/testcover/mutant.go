// mutant.go plans the mutants of the blocks a package's tests cover and writes them as one switchable build.
// A mutant is one small change at one site of the package's own source; the variable MutantEnv picks the one that is on.

package testcover

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// MutantEnv names the environment variable that picks the mutant a mutant binary runs with; unset or 0 turns none on.
const MutantEnv = "LYM_TESTCOVER_MUTANT"

// mutantFileName is the file the overlay adds to the package: the switch and the helpers the mutated sites call.
const mutantFileName = "lym_testcover_mutant.go"

// overlayFileName is the `go build -overlay` file WriteOverlay writes.
const overlayFileName = "overlay.json"

// The mutation operators.
const (
	OperatorNegateCondition  = "negate condition"
	OperatorNegateComparison = "negate comparison"
	OperatorDropStatement    = "drop statement"
	OperatorChangeConstant   = "change constant"
)

// mutantHelpers is the source of the file the overlay adds, after its package clause.
// Each helper changes its value only while the mutant whose ID it gets is on.
// The import names carry the prefix so they never meet a name the package declares.
const mutantHelpers = `
import (
	lymmutantos "os"
	lymmutantstrconv "strconv"
)

var lymMutantActive, _ = lymmutantstrconv.Atoi(lymmutantos.Getenv("` + MutantEnv + `"))

func lymMutantBool[T ~bool](v T, id int) T {
	if id == lymMutantActive {
		return !v
	}
	return v
}

func lymMutantKeep(id int) bool {
	return id != lymMutantActive
}

func lymMutantInt[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr](v T, id int) T {
	if id == lymMutantActive {
		return v + 1
	}
	return v
}

func lymMutantString[T ~string](v T, id int) T {
	if id == lymMutantActive {
		return v + "~"
	}
	return v
}
`

// Mutant is one small change at one site of a package's own source.
type Mutant struct {
	// ID is the value of MutantEnv that turns the mutant on; IDs count from 1.
	ID int
	// File is the site's file name within the package directory, and Line the line the site starts on.
	File string
	Line int
	// Operator is one of the Operator constants.
	Operator string
	// Block is the rendered key of the coverage block holding the site.
	Block string

	// lastLine is the line the site ends on.
	lastLine int
	// start and end are the site's byte offsets in its file.
	start, end int
	// prefix and suffix are the text the mutant inserts before and after the site.
	prefix, suffix string
}

// MutantPlan holds the mutants of one package and the source they change.
type MutantPlan struct {
	// Mutants are ordered by file, then by site.
	Mutants []Mutant
	// dir is the package directory and packageName the name its files declare.
	dir, packageName string
	// sources holds the text of each file a mutant changes, by name.
	sources map[string][]byte
}

// site is one place of a file a mutant can change, before it is placed in a block and given an ID.
type site struct {
	operator   string
	start, end int
	// helper is the function the site's value passes through; empty for a dropped statement.
	helper string
}

// PlanMutants returns the mutants of the blocks of the package at importPath that targets holds.
// profile is a profile of the package's coverage binary, which lists each of its blocks, covered or not.
// targets holds rendered block keys, keyed as KeyBlocks keys every block of the package's own files in profile.
// The files are read from the package directory dir.
// A site belongs to the innermost block holding its start, and it is a mutant when that block is a target.
// The sites lie in function bodies.
// They are each if and for condition, and each other comparison.
// They also include each call, assignment or increment statement of a statement list.
// Each integer, rune or string literal is a site too.
// A literal of a constant declaration, an array length, a composite key or a field tag stays, since it must be constant.
// A file that does not parse, or a block outside its file, is an error.
func PlanMutants(dir, importPath string, profile []ProfileBlock, targets BlockSet) (*MutantPlan, error) {
	var own []ProfileBlock
	seen := map[string]bool{}
	for _, block := range PackageBlocks(profile, importPath) {
		if !seen[block.Position()] {
			seen[block.Position()] = true
			own = append(own, block)
		}
	}
	keys, err := KeyBlocks(dir, own)
	if err != nil {
		return nil, err
	}
	type span struct {
		start, end int
		key        string
	}
	spans := map[string][]span{}
	sources := map[string]*sourceFile{}
	for i, block := range own {
		name := path.Base(block.File)
		src, parsed := sources[name]
		if !parsed {
			if src, err = parseSourceFile(filepath.Join(dir, name)); err != nil {
				return nil, err
			}
			sources[name] = src
		}
		start, err := src.offset(block.StartLine, block.StartCol)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", block.Position(), err)
		}
		end, err := src.offset(block.EndLine, block.EndCol)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", block.Position(), err)
		}
		spans[name] = append(spans[name], span{start: start, end: end, key: keys[i].String()})
	}

	plan := &MutantPlan{dir: dir, sources: map[string][]byte{}}
	for _, name := range slices.Sorted(maps.Keys(spans)) {
		if !slices.ContainsFunc(spans[name], func(s span) bool { _, target := targets[s.key]; return target }) {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), sources[name].text, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		plan.packageName = file.Name.Name
		tokenFile := fset.File(file.Pos())
		for _, s := range fileSites(file, tokenFile) {
			innermost := -1
			for i, b := range spans[name] {
				if b.start <= s.start && s.start < b.end && (innermost < 0 || b.end-b.start < spans[name][innermost].end-spans[name][innermost].start) {
					innermost = i
				}
			}
			if innermost < 0 {
				continue
			}
			key := spans[name][innermost].key
			if _, target := targets[key]; !target {
				continue
			}
			plan.Mutants = append(plan.Mutants, Mutant{
				File: name, Line: tokenFile.Line(tokenFile.Pos(s.start)), lastLine: tokenFile.Line(tokenFile.Pos(s.end)),
				Operator: s.operator, Block: key, start: s.start, end: s.end,
				prefix: s.helper,
			})
		}
		plan.sources[name] = sources[name].text
	}
	for i := range plan.Mutants {
		m := &plan.Mutants[i]
		m.ID = i + 1
		if m.prefix == "" {
			m.prefix, m.suffix = fmt.Sprintf("if lymMutantKeep(%d) { ", m.ID), " }"
		} else {
			m.prefix, m.suffix = m.prefix+"(", fmt.Sprintf(", %d)", m.ID)
		}
	}
	return plan, nil
}

// fileSites returns the mutation sites of the function bodies of file, ordered by start, the longer first at one start.
func fileSites(file *ast.File, tokenFile *token.File) []site {
	var sites []site
	add := func(node ast.Node, operator, helper string) {
		sites = append(sites, site{operator: operator, start: tokenFile.Offset(node.Pos()), end: tokenFile.Offset(node.End()), helper: helper})
	}
	conditions := map[ast.Expr]bool{}
	fixed := map[*ast.BasicLit]bool{}
	markFixed := func(node ast.Expr) {
		if node == nil {
			return
		}
		ast.Inspect(node, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				fixed[lit] = true
			}
			return true
		})
	}
	addStatements := func(list []ast.Stmt) {
		for _, stmt := range list {
			if droppable(stmt) {
				add(stmt, OperatorDropStatement, "")
			}
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.GenDecl:
				if n.Tok == token.CONST {
					ast.Inspect(n, func(inner ast.Node) bool {
						if lit, ok := inner.(*ast.BasicLit); ok {
							fixed[lit] = true
						}
						return true
					})
				}
			case *ast.ArrayType:
				markFixed(n.Len)
			case *ast.KeyValueExpr:
				markFixed(n.Key)
			case *ast.Field:
				if n.Tag != nil {
					fixed[n.Tag] = true
				}
			case *ast.IfStmt:
				conditions[n.Cond] = true
				add(n.Cond, OperatorNegateCondition, "lymMutantBool")
			case *ast.ForStmt:
				if n.Cond != nil {
					conditions[n.Cond] = true
					add(n.Cond, OperatorNegateCondition, "lymMutantBool")
				}
			case *ast.BinaryExpr:
				if isComparison(n.Op) && !conditions[n] {
					add(n, OperatorNegateComparison, "lymMutantBool")
				}
			case *ast.BasicLit:
				if fixed[n] {
					break
				}
				switch n.Kind {
				case token.INT, token.CHAR:
					add(n, OperatorChangeConstant, "lymMutantInt")
				case token.STRING:
					add(n, OperatorChangeConstant, "lymMutantString")
				}
			case *ast.BlockStmt:
				addStatements(n.List)
			case *ast.CaseClause:
				addStatements(n.Body)
			case *ast.CommClause:
				addStatements(n.Body)
			}
			return true
		})
	}
	slices.SortStableFunc(sites, func(a, b site) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end))
	})
	return sites
}

// droppable reports whether dropping stmt can change what the code does and still compiles:
// a call other than panic, an increment,
// or an assignment that declares nothing and writes a name other than the blank one.
func droppable(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		call, ok := s.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		ident, ok := call.Fun.(*ast.Ident)
		return !ok || ident.Name != "panic"
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			return false
		}
		return slices.ContainsFunc(s.Lhs, func(lhs ast.Expr) bool {
			ident, ok := lhs.(*ast.Ident)
			return !ok || ident.Name != "_"
		})
	case *ast.IncDecStmt:
		return true
	}
	return false
}

// isComparison reports whether op compares two values.
func isComparison(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

// WriteOverlay writes the mutated files of the plan and the file of the helpers into outDir.
// It returns the path of the `go build -overlay` file that puts them in place of the package's own.
// A mutant that dropped holds is left out, and a file left with no mutant is not replaced.
func (p *MutantPlan) WriteOverlay(outDir string, dropped map[int]bool) (string, error) {
	replace := map[string]string{}
	byFile := map[string][]Mutant{}
	for _, m := range p.Mutants {
		if !dropped[m.ID] {
			byFile[m.File] = append(byFile[m.File], m)
		}
	}
	for name, mutants := range byFile {
		written := filepath.Join(outDir, name)
		if err := os.WriteFile(written, applyMutants(p.sources[name], mutants), 0o644); err != nil {
			return "", err
		}
		replace[filepath.Join(p.dir, name)] = written
	}
	helpers := filepath.Join(outDir, mutantFileName)
	if err := os.WriteFile(helpers, []byte("package "+p.packageName+"\n"+mutantHelpers), 0o644); err != nil {
		return "", err
	}
	replace[filepath.Join(p.dir, mutantFileName)] = helpers
	data, err := json.Marshal(map[string]map[string]string{"Replace": replace})
	if err != nil {
		return "", err
	}
	overlay := filepath.Join(outDir, overlayFileName)
	return overlay, os.WriteFile(overlay, data, 0o644)
}

// applyMutants returns text with the prefix and suffix of each of mutants inserted around its site.
// Every insertion adds no line, so a line of the result is the line of text it came from.
// At one offset a closing suffix goes before an opening prefix;
// suffixes close the inner site first and prefixes open the outer site first.
func applyMutants(text []byte, mutants []Mutant) []byte {
	type insertion struct {
		offset int
		// opening is set for a prefix; length is the site's length.
		opening bool
		length  int
		text    string
	}
	var insertions []insertion
	for _, m := range mutants {
		insertions = append(insertions,
			insertion{offset: m.start, opening: true, length: m.end - m.start, text: m.prefix},
			insertion{offset: m.end, length: m.end - m.start, text: m.suffix})
	}
	slices.SortStableFunc(insertions, func(a, b insertion) int {
		if a.offset != b.offset {
			return cmp.Compare(a.offset, b.offset)
		}
		if a.opening != b.opening {
			if a.opening {
				return 1
			}
			return -1
		}
		if a.opening {
			return cmp.Compare(b.length, a.length)
		}
		return cmp.Compare(a.length, b.length)
	})
	var out strings.Builder
	at := 0
	for _, ins := range insertions {
		out.Write(text[at:ins.offset])
		out.WriteString(ins.text)
		at = ins.offset
	}
	out.Write(text[at:])
	return []byte(out.String())
}

// UnbuildableMutants returns the IDs of the mutants outside dropped that span a source line the compiler refused.
// It returns none when err holds no *MutantBuildError or names no line a mutant spans.
func (p *MutantPlan) UnbuildableMutants(err error, dropped map[int]bool) []int {
	var buildErr *MutantBuildError
	if !errors.As(err, &buildErr) {
		return nil
	}
	var ids []int
	for _, m := range p.Mutants {
		if dropped[m.ID] {
			continue
		}
		if slices.ContainsFunc(buildErr.Lines, func(l SourceLine) bool {
			return l.File == m.File && m.Line <= l.Line && l.Line <= m.lastLine
		}) {
			ids = append(ids, m.ID)
		}
	}
	return ids
}
