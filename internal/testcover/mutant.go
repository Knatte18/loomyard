// mutant.go plans the mutants of the blocks a package's tests cover and writes them as one switchable build.
// A mutant is one small change at one site of the package's own source; the variable MutantEnv picks the one that is on.

package testcover

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// MutantEnv names the environment variable that picks the mutant a mutant binary runs with; unset or 0 turns none on.
const MutantEnv = "LYM_TESTCOVER_MUTANT"

// mutantFileName is the file the overlay adds to the package: the switch and the helpers the mutated sites call.
const mutantFileName = "lym_testcover_mutant.go"

// overlayFileName is the `go build -overlay` file WriteOverlay writes.
const overlayFileName = "overlay.json"

// embedDirective opens the comment that fills a package-level variable from files at build time.
const embedDirective = "//go:embed "

// The mutation operators.
const (
	OperatorNegateCondition  = "negate condition"
	OperatorNegateComparison = "negate comparison"
	// OperatorDeleteClause replaces an operand of a && or || chain with the value that ignores it.
	// The operand is still evaluated, and only its value is replaced, so a side effect of the operand stays.
	OperatorDeleteClause   = "delete clause"
	OperatorDropStatement  = "drop statement"
	OperatorChangeConstant = "change constant"
	// OperatorChangeEmbeddedLine marks one line of an embedded file with a ~.
	// On a line whose first `: ` or `=` has text after it, the mark opens that value and the key stays,
	// so a loader that refuses an unknown key does not fail every test that loads the file.
	OperatorChangeEmbeddedLine = "change embedded line"
)

// stepHelper is the helper an integer or rune constant's mutant calls.
// The mutant writes the constant c as `(c + 1>>stepHelper(id))`, which adds one while the mutant is on.
// The untyped 1 of a shift whose count is not constant takes the type c takes there,
// so the sum keeps the type of the constant it replaces, a byte or a rune included.
const stepHelper = "lymMutantStep"

// mutantHelpers is the source of the file the overlay adds, after its package clause.
// Each helper changes its value only while the mutant whose ID it gets is on.
// The import names carry the prefix so they never meet a name the package declares.
const mutantHelpers = `
import (
	lymmutantos "os"
	lymmutantstrconv "strconv"
	lymmutantstrings "strings"
)

var lymMutantActive, _ = lymmutantstrconv.Atoi(lymmutantos.Getenv("` + MutantEnv + `"))

func lymMutantBool[T ~bool](v T, id int) T {
	if id == lymMutantActive {
		return !v
	}
	return v
}

func lymMutantDropOr[T ~bool](v T, id int) T {
	if id == lymMutantActive {
		return false
	}
	return v
}

func lymMutantDropAnd[T ~bool](v T, id int) T {
	if id == lymMutantActive {
		return true
	}
	return v
}

func lymMutantKeep(id int) bool {
	return id != lymMutantActive
}

func ` + stepHelper + `(id int) int {
	if id == lymMutantActive {
		return 0
	}
	return 1
}

func lymMutantString[T ~string](v T, id int) T {
	if id == lymMutantActive {
		return v + "~"
	}
	return v
}

func lymMutantLine[T ~string](v T, line, id int) T {
	if id != lymMutantActive {
		return v
	}
	lines := lymmutantstrings.SplitAfter(string(v), "\n")
	text := lines[line-1]
	at := len(text) - len(lymmutantstrings.TrimLeft(text, " \t"))
	separator := -1
	for _, mark := range []string{": ", "="} {
		if i := lymmutantstrings.Index(text, mark); i >= 0 && (separator < 0 || i < separator) {
			separator = i
		}
	}
	if separator >= 0 {
		value := lymmutantstrings.TrimLeft(text[separator+1:], " \t")
		if lymmutantstrings.TrimSpace(value) != "" {
			at = len(text) - len(value)
		}
	}
	lines[line-1] = text[:at] + "~" + text[at:]
	return T(lymmutantstrings.Join(lines, ""))
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
	// Blocks are the rendered keys of the coverage blocks whose run reaches the site, ascending.
	// A site of a function body has the one block holding it;
	// a site of a package-level variable's value has each block that reads the variable.
	// It also has each block that reads a variable whose value names it.
	Blocks []string

	// lastLine is the line the site ends on.
	lastLine int
	// start and end are the site's byte offsets in its file.
	start, end int
	// helper is the function the site's value passes through, empty for a dropped statement;
	// args are the arguments the helper takes between the value and the ID.
	helper, args string
	// noise is set for a site inside a statement that only logs or inside a comparison of runtime.GOOS with another platform.
	noise bool
}

// InBlocks reports whether blocks holds one of the mutant's blocks, so a run of those blocks reaches the mutant.
func (m Mutant) InBlocks(blocks BlockSet) bool {
	return slices.ContainsFunc(m.Blocks, func(key string) bool {
		_, held := blocks[key]
		return held
	})
}

// opening returns the text the mutant inserts before its site.
func (m Mutant) opening() string {
	switch m.helper {
	case "":
		return fmt.Sprintf("if lymMutantKeep(%d) { ", m.ID)
	case stepHelper:
		return "("
	}
	return m.helper + "("
}

// closing returns the text the mutant inserts after its site.
func (m Mutant) closing() string {
	switch {
	case m.helper == "":
		return " }"
	case m.helper == stepHelper:
		return fmt.Sprintf(" + 1>>%s(%d))", stepHelper, m.ID)
	case m.args != "":
		return fmt.Sprintf(", %s, %d)", m.args, m.ID)
	}
	return fmt.Sprintf(", %d)", m.ID)
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

// site is one place of a file a mutant can change, before it is placed in its blocks and given an ID.
type site struct {
	operator   string
	start, end int
	// helper and args are the Mutant fields of the same names.
	helper, args string
	// variables names the package-level variables whose value holds the site; it is empty for a site of a function body.
	variables []string
	// noise is the Mutant field of the same name.
	noise bool
}

// packageFile is one parsed non-test Go file of a package.
type packageFile struct {
	name      string
	text      []byte
	file      *ast.File
	tokenFile *token.File
}

// packageDecls is what the mutation sites depend on in a package's package-level declarations.
type packageDecls struct {
	// constants maps each constant whose value is a literal to the helper that changes its value.
	constants map[string]string
	// embedded maps each string variable an embed directive fills from one file to that file's non-blank lines.
	embedded map[string][]int
	// variables holds the names of the package-level variables.
	variables map[string]bool
}

// PlanMutants returns the mutants of the blocks of the package at importPath that targets holds.
// profile is a profile of the package's coverage binary, which lists each of its blocks, covered or not.
// targets holds rendered block keys, keyed as KeyBlocks keys every block of the package's own files in profile.
// The files are read from the package directory dir, and so is each file an embed directive names.
// A site of a function body belongs to the innermost block holding its start.
// A site of a package-level variable's value belongs to each block that names the variable.
// It also belongs to each block that names a variable whose value names it.
// A site is a mutant when one of its blocks is a target.
// Function bodies hold these sites: each if and for condition, and each other comparison;
// each operand of a chain of && or of ||, which the mutant deletes;
// each call, assignment or increment statement of a statement list;
// each integer, rune or string literal, and each use of a package-level constant whose value is one;
// and, at each use of a string variable an embed directive fills from one file, each non-blank line of that file.
// A deleted operand is still evaluated, and only its value is replaced, so a side effect of the operand stays.
// The literals of a package-level variable's value are sites too.
// A literal of a constant declaration, an array length or a field tag stays, since it must be constant.
// So does a composite literal's key unless it is a string literal, which only a map key can be.
// So does a literal or constant of an expression built from integer and rune constants alone.
// The step rewrite makes such an expression non-constant, so an intermediate that overflows its type changes the value.
// A file that does not parse, an embedded file that cannot be read, or a block outside its file is an error.
func PlanMutants(dir, importPath string, profile []ProfileBlock, targets BlockSet) (*MutantPlan, error) {
	own := ownBlocks(profile, importPath)
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
	blockAt := func(name string, offset int) (string, bool) {
		innermost := -1
		for i, b := range spans[name] {
			if b.start <= offset && offset < b.end && (innermost < 0 || b.end-b.start < spans[name][innermost].end-spans[name][innermost].start) {
				innermost = i
			}
		}
		if innermost < 0 {
			return "", false
		}
		return spans[name][innermost].key, true
	}

	files, err := parsePackageFiles(dir, sources)
	if err != nil {
		return nil, err
	}
	decls, err := scanPackageDecls(dir, files)
	if err != nil {
		return nil, err
	}
	readers := map[string]map[string]bool{}
	addReader := func(variable, key string) bool {
		if readers[variable][key] {
			return false
		}
		if readers[variable] == nil {
			readers[variable] = map[string]bool{}
		}
		readers[variable][key] = true
		return true
	}
	valueReads := map[string][]string{}
	for _, f := range files {
		for variable, offsets := range variableReads(f.file, f.tokenFile, decls) {
			for _, offset := range offsets {
				if key, ok := blockAt(f.name, offset); ok {
					addReader(variable, key)
				}
			}
		}
		for variable, names := range variableValueReads(f.file, decls) {
			valueReads[variable] = append(valueReads[variable], names...)
		}
	}
	// A variable a reader's value names is read wherever the reader is, however many variables lie between.
	for grew := true; grew; {
		grew = false
		for variable, names := range valueReads {
			for _, name := range names {
				for key := range readers[variable] {
					if addReader(name, key) {
						grew = true
					}
				}
			}
		}
	}

	plan := &MutantPlan{dir: dir, sources: map[string][]byte{}}
	for _, f := range files {
		plan.packageName = f.file.Name.Name
		for _, s := range fileSites(f.file, f.tokenFile, decls) {
			var blocks []string
			if len(s.variables) == 0 {
				if key, ok := blockAt(f.name, s.start); ok {
					blocks = []string{key}
				}
			} else {
				for _, variable := range s.variables {
					blocks = append(blocks, slices.Collect(maps.Keys(readers[variable]))...)
				}
				slices.Sort(blocks)
				blocks = slices.Compact(blocks)
			}
			if !slices.ContainsFunc(blocks, func(key string) bool { _, target := targets[key]; return target }) {
				continue
			}
			plan.Mutants = append(plan.Mutants, Mutant{
				File: f.name, Line: f.tokenFile.Line(f.tokenFile.Pos(s.start)), lastLine: f.tokenFile.Line(f.tokenFile.Pos(s.end)),
				Operator: s.operator, Blocks: blocks, start: s.start, end: s.end, helper: s.helper, args: s.args, noise: s.noise,
			})
			plan.sources[f.name] = f.text
		}
	}
	for i := range plan.Mutants {
		plan.Mutants[i].ID = i + 1
	}
	return plan, nil
}

// ownBlocks returns the blocks of profile that lie in the files of the package at importPath, each position once.
func ownBlocks(profile []ProfileBlock, importPath string) []ProfileBlock {
	var own []ProfileBlock
	seen := map[string]bool{}
	for _, block := range PackageBlocks(profile, importPath) {
		if !seen[block.Position()] {
			seen[block.Position()] = true
			own = append(own, block)
		}
	}
	return own
}

// parsePackageFiles parses every file sources holds, in name order.
// It also parses every other non-test Go file of dir that the default build context takes;
// a build tag set through GOFLAGS is not read, so a file gated by one may be skipped or its alternative scanned.
// A file of sources keeps the text read there.
func parsePackageFiles(dir string, sources map[string]*sourceFile) ([]packageFile, error) {
	names := slices.Collect(maps.Keys(sources))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || sources[name] != nil {
			continue
		}
		if match, err := build.Default.MatchFile(dir, name); err != nil {
			return nil, err
		} else if match {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	files := make([]packageFile, len(names))
	for i, name := range names {
		var text []byte
		if src := sources[name]; src != nil {
			text = src.text
		} else if text, err = os.ReadFile(filepath.Join(dir, name)); err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), text, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files[i] = packageFile{name: name, text: text, file: file, tokenFile: fset.File(file.Pos())}
	}
	return files, nil
}

// scanPackageDecls reads the package-level constants and variables of files.
// It reads each file an embed directive names from dir.
func scanPackageDecls(dir string, files []packageFile) (packageDecls, error) {
	decls := packageDecls{constants: map[string]string{}, embedded: map[string][]int{}, variables: map[string]bool{}}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					switch {
					case name.Name == "_":
					case gen.Tok == token.VAR:
						decls.variables[name.Name] = true
					case len(value.Values) == len(value.Names):
						if lit, ok := value.Values[i].(*ast.BasicLit); ok && literalHelper(lit.Kind) != "" {
							decls.constants[name.Name] = literalHelper(lit.Kind)
						}
					}
				}
				pattern, ok := embeddedFile(gen, value)
				if !ok {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, pattern))
				if err != nil {
					return packageDecls{}, fmt.Errorf("read the embedded file of %s: %w", value.Names[0].Name, err)
				}
				for i, line := range bytes.SplitAfter(data, []byte("\n")) {
					if len(bytes.TrimSpace(line)) > 0 {
						decls.embedded[value.Names[0].Name] = append(decls.embedded[value.Names[0].Name], i+1)
					}
				}
			}
		}
	}
	return decls, nil
}

// embeddedFile returns the file an embed directive fills value from.
// It reports false unless value is one string variable that one plain file name fills.
func embeddedFile(gen *ast.GenDecl, value *ast.ValueSpec) (string, bool) {
	typeName, ok := value.Type.(*ast.Ident)
	if gen.Tok != token.VAR || len(value.Names) != 1 || !ok || typeName.Name != "string" {
		return "", false
	}
	doc := value.Doc
	if doc == nil && len(gen.Specs) == 1 {
		doc = gen.Doc
	}
	if doc == nil {
		return "", false
	}
	for _, comment := range doc.List {
		pattern, found := strings.CutPrefix(comment.Text, embedDirective)
		pattern = strings.TrimSpace(pattern)
		if found && len(strings.Fields(pattern)) == 1 && !strings.ContainsAny(pattern, "*?[\"`") {
			return pattern, true
		}
	}
	return "", false
}

// localNames returns every name fn declares itself:
// its receiver, parameters and results, and each name its body declares.
// A use of such a name in fn may refer to the local, so no use of it there counts as a use of a package-level name.
func localNames(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	addIdents := func(exprs ...ast.Expr) {
		for _, expr := range exprs {
			if ident, ok := expr.(*ast.Ident); ok {
				names[ident.Name] = true
			}
		}
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			for _, name := range n.Names {
				names[name.Name] = true
			}
		case *ast.ValueSpec:
			for _, name := range n.Names {
				names[name.Name] = true
			}
		case *ast.TypeSpec:
			names[n.Name.Name] = true
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				addIdents(n.Lhs...)
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				addIdents(n.Key, n.Value)
			}
		}
		return true
	})
	return names
}

// literalHelper returns the helper that changes a literal of kind, or "" for a kind no mutant changes.
func literalHelper(kind token.Token) string {
	switch kind {
	case token.INT, token.CHAR:
		return stepHelper
	case token.STRING:
		return "lymMutantString"
	}
	return ""
}

// markFixed records in fixed the literals and identifiers below node that must stay as written:
// those of a constant declaration, an array length, a field's names and tag, and a selected name.
// A composite key stays too, unless it is a string literal.
func markFixed(node ast.Node, fixed map[ast.Node]bool) {
	var held []ast.Node
	switch n := node.(type) {
	case *ast.GenDecl:
		if n.Tok == token.CONST {
			held = append(held, n)
		}
	case *ast.ArrayType:
		if n.Len != nil {
			held = append(held, n.Len)
		}
	case *ast.KeyValueExpr:
		if lit, ok := n.Key.(*ast.BasicLit); !ok || lit.Kind != token.STRING {
			held = append(held, n.Key)
		}
	case *ast.Field:
		for _, name := range n.Names {
			held = append(held, name)
		}
		if n.Tag != nil {
			held = append(held, n.Tag)
		}
	case *ast.SelectorExpr:
		held = append(held, n.Sel)
	}
	for _, h := range held {
		ast.Inspect(h, func(inner ast.Node) bool {
			switch inner.(type) {
			case *ast.BasicLit, *ast.Ident:
				fixed[inner] = true
			}
			return true
		})
	}
}

// markConstantIntegerExpression records in fixed the literals and constant names below expr.
// It does so only when expr is built from integer and rune constants alone.
// The step rewrite of one of them makes the expression non-constant, so the compiler no longer evaluates it exactly.
// An intermediate value that overflows the type of the context then changes the result with no mutant on.
// locals holds the names the enclosing function declares; a use of one of them is no package-level constant.
func markConstantIntegerExpression(expr *ast.BinaryExpr, locals map[string]bool, decls packageDecls, fixed map[ast.Node]bool) {
	if !isConstantInteger(expr, locals, decls) {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.BasicLit, *ast.Ident:
			fixed[n] = true
		}
		return true
	})
}

// isConstantInteger reports whether expr is built from integer and rune literals and package-level constants of one.
// It looks through parentheses, unary operators and binary operators.
func isConstantInteger(expr ast.Expr, locals map[string]bool, decls packageDecls) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT || e.Kind == token.CHAR
	case *ast.Ident:
		return !locals[e.Name] && decls.constants[e.Name] == stepHelper
	case *ast.ParenExpr:
		return isConstantInteger(e.X, locals, decls)
	case *ast.UnaryExpr:
		return isConstantInteger(e.X, locals, decls)
	case *ast.BinaryExpr:
		return isConstantInteger(e.X, locals, decls) && isConstantInteger(e.Y, locals, decls)
	}
	return false
}

// variableReads finds each identifier of file's function bodies that names a package-level variable.
// It returns their offsets by variable name.
func variableReads(file *ast.File, tokenFile *token.File, decls packageDecls) map[string][]int {
	reads := map[string][]int{}
	fixed := map[ast.Node]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := localNames(fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			markFixed(n, fixed)
			if ident, ok := n.(*ast.Ident); ok && !fixed[ident] && !locals[ident.Name] && decls.variables[ident.Name] {
				reads[ident.Name] = append(reads[ident.Name], tokenFile.Offset(ident.Pos()))
			}
			return true
		})
	}
	return reads
}

// variableValueReads finds, for each package-level variable of file, the package-level variables its value names.
func variableValueReads(file *ast.File, decls packageDecls) map[string][]string {
	reads := map[string][]string{}
	fixed := map[ast.Node]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			for _, expr := range value.Values {
				ast.Inspect(expr, func(n ast.Node) bool {
					markFixed(n, fixed)
					if ident, ok := n.(*ast.Ident); ok && !fixed[ident] && decls.variables[ident.Name] {
						for _, name := range value.Names {
							if name.Name != "_" {
								reads[name.Name] = append(reads[name.Name], ident.Name)
							}
						}
					}
					return true
				})
			}
		}
	}
	return reads
}

// fileSites returns the mutation sites of the function bodies and package-level variable values of file.
// They are ordered by start, the longer first at one start.
// A site inside a statement that only logs is marked noise.
// So is one inside a comparison of runtime.GOOS with another platform.
func fileSites(file *ast.File, tokenFile *token.File, decls packageDecls) []site {
	var sites []site
	add := func(node ast.Node, operator, helper, args string, variables []string) {
		sites = append(sites, site{
			operator: operator, start: tokenFile.Offset(node.Pos()), end: tokenFile.Offset(node.End()),
			helper: helper, args: args, variables: variables,
		})
	}
	// noise holds the start and end offsets of each node whose sites are marked noise.
	var noise [][2]int
	addNoise := func(node ast.Node) {
		noise = append(noise, [2]int{tokenFile.Offset(node.Pos()), tokenFile.Offset(node.End())})
	}
	conditions := map[ast.Expr]bool{}
	fixed := map[ast.Node]bool{}
	imported := importedPackages(file)
	addStatements := func(list []ast.Stmt, locals map[string]bool) {
		for _, stmt := range list {
			if logsOnly(stmt, locals, imported) {
				addNoise(stmt)
			}
			if droppable(stmt) {
				add(stmt, OperatorDropStatement, "", "", nil)
			}
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			if decl.Tok != token.VAR {
				continue
			}
			for _, spec := range decl.Specs {
				value := spec.(*ast.ValueSpec)
				for i, expr := range value.Values {
					names := value.Names
					if len(value.Values) == len(value.Names) {
						names = names[i : i+1]
					}
					var variables []string
					for _, name := range names {
						if name.Name != "_" {
							variables = append(variables, name.Name)
						}
					}
					if len(variables) == 0 {
						continue
					}
					ast.Inspect(expr, func(n ast.Node) bool {
						markFixed(n, fixed)
						switch n := n.(type) {
						case *ast.FuncLit:
							return false
						case *ast.BinaryExpr:
							markConstantIntegerExpression(n, nil, decls, fixed)
						case *ast.BasicLit:
							if helper := literalHelper(n.Kind); helper != "" && !fixed[n] {
								add(n, OperatorChangeConstant, helper, "", variables)
							}
						}
						return true
					})
				}
			}
		case *ast.FuncDecl:
			if decl.Body == nil {
				continue
			}
			locals := localNames(decl)
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				markFixed(n, fixed)
				switch n := n.(type) {
				case *ast.IfStmt:
					conditions[n.Cond] = true
					add(n.Cond, OperatorNegateCondition, "lymMutantBool", "", nil)
				case *ast.ForStmt:
					if n.Cond != nil {
						conditions[n.Cond] = true
						add(n.Cond, OperatorNegateCondition, "lymMutantBool", "", nil)
					}
				case *ast.BinaryExpr:
					markConstantIntegerExpression(n, locals, decls, fixed)
					if comparesOtherPlatform(n, locals, imported) {
						addNoise(n)
					}
					if isComparison(n.Op) && !conditions[n] {
						add(n, OperatorNegateComparison, "lymMutantBool", "", nil)
					}
					addClauses(n, func(operand ast.Expr, helper string) { add(operand, OperatorDeleteClause, helper, "", nil) })
				case *ast.BasicLit:
					if helper := literalHelper(n.Kind); helper != "" && !fixed[n] {
						add(n, OperatorChangeConstant, helper, "", nil)
					}
				case *ast.Ident:
					if fixed[n] || locals[n.Name] {
						break
					}
					if helper, ok := decls.constants[n.Name]; ok {
						add(n, OperatorChangeConstant, helper, "", nil)
					}
					for _, line := range decls.embedded[n.Name] {
						add(n, OperatorChangeEmbeddedLine, "lymMutantLine", strconv.Itoa(line), nil)
					}
				case *ast.BlockStmt:
					addStatements(n.List, locals)
				case *ast.CaseClause:
					addStatements(n.Body, locals)
				case *ast.CommClause:
					addStatements(n.Body, locals)
				}
				return true
			})
		}
	}
	for i := range sites {
		sites[i].noise = slices.ContainsFunc(noise, func(span [2]int) bool { return span[0] <= sites[i].start && sites[i].end <= span[1] })
	}
	slices.SortStableFunc(sites, func(a, b site) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end))
	})
	return sites
}

// logsOnly reports whether stmt does nothing but log.
// Such a statement is a call into the logger package,
// or an if statement with no init statement and no call in its condition whose every branch holds only such statements.
// A call in the condition may have an effect, so such an if is not log-only.
// locals holds the names the enclosing function declares, and imported the names the file's imports bind.
func logsOnly(stmt ast.Stmt, locals map[string]bool, imported map[string]string) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return calleePackage(s, locals, imported) == loggerImportPath
	case *ast.IfStmt:
		if s.Init != nil || holdsCall(s.Cond) || !blockLogsOnly(s.Body, locals, imported) {
			return false
		}
		switch branch := s.Else.(type) {
		case nil:
			return true
		case *ast.BlockStmt:
			return blockLogsOnly(branch, locals, imported)
		default:
			return logsOnly(branch, locals, imported)
		}
	}
	return false
}

// holdsCall reports whether expr holds a call expression.
func holdsCall(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if _, isCall := n.(*ast.CallExpr); isCall {
			found = true
		}
		return !found
	})
	return found
}

// blockLogsOnly reports whether block holds a statement and every statement of it does nothing but log.
func blockLogsOnly(block *ast.BlockStmt, locals map[string]bool, imported map[string]string) bool {
	return len(block.List) > 0 && !slices.ContainsFunc(block.List, func(stmt ast.Stmt) bool { return !logsOnly(stmt, locals, imported) })
}

// comparesOtherPlatform reports whether expr compares runtime.GOOS with a string literal naming another platform.
// The other platform is any but the one the default build context targets, which the mutants run on.
// The comparison is == or !=, and runtime is reached through a name the file's imports bind that no local shadows.
func comparesOtherPlatform(expr *ast.BinaryExpr, locals map[string]bool, imported map[string]string) bool {
	if expr.Op != token.EQL && expr.Op != token.NEQ {
		return false
	}
	for _, sides := range [][2]ast.Expr{{expr.X, expr.Y}, {expr.Y, expr.X}} {
		selector, isSelector := sides[0].(*ast.SelectorExpr)
		literal, isLiteral := sides[1].(*ast.BasicLit)
		if !isSelector || !isLiteral || literal.Kind != token.STRING || selector.Sel.Name != "GOOS" {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || locals[pkg.Name] || imported[pkg.Name] != "runtime" {
			continue
		}
		platform, err := strconv.Unquote(literal.Value)
		return err == nil && platform != build.Default.GOOS
	}
	return false
}

// addClauses calls add with each operand of expr when expr is a && or || chain, and with the helper that deletes it:
// an operand of && turns true and one of || turns false.
// An operand that is itself a chain of the same operator passes on its own operands when it is visited.
func addClauses(expr *ast.BinaryExpr, add func(operand ast.Expr, helper string)) {
	helper := map[token.Token]string{token.LAND: "lymMutantDropAnd", token.LOR: "lymMutantDropOr"}[expr.Op]
	if helper == "" {
		return
	}
	for _, operand := range []ast.Expr{expr.X, expr.Y} {
		if inner, ok := operand.(*ast.BinaryExpr); !ok || inner.Op != expr.Op {
			add(operand, helper)
		}
	}
}

// importedPackages maps each name the imports of file bind to the import path it names.
// An import with no name binds the last element of its path, and a blank or dot import binds none.
func importedPackages(file *ast.File) map[string]string {
	imported := map[string]string{}
	for _, spec := range file.Imports {
		importPath, _ := strconv.Unquote(spec.Path.Value)
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "_" && name != "." {
			imported[name] = importPath
		}
	}
	return imported
}

// calleePackage returns the import path of the package a call statement calls into, or "" for any other statement.
// It returns "" too for a local or method call.
// Only a call through a package name the file imports counts, and a local of that name shadows it.
func calleePackage(stmt ast.Stmt, locals map[string]bool, imported map[string]string) string {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return ""
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || locals[pkg.Name] {
		return ""
	}
	return imported[pkg.Name]
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

// applyMutants returns text with the opening and closing text of each of mutants inserted around its site.
// Every insertion adds no line, so a line of the result is the line of text it came from.
// At one offset a closing goes before an opening;
// closings close the inner site first and openings open the outer site first.
// Of two mutants on one site, the earlier in mutants is the outer.
func applyMutants(text []byte, mutants []Mutant) []byte {
	type insertion struct {
		offset int
		// opening is set for an opening; length is the site's length and rank the mutant's index in mutants.
		opening      bool
		length, rank int
		text         string
	}
	var insertions []insertion
	for i, m := range mutants {
		insertions = append(insertions,
			insertion{offset: m.start, opening: true, length: m.end - m.start, rank: i, text: m.opening()},
			insertion{offset: m.end, length: m.end - m.start, rank: i, text: m.closing()})
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
			return cmp.Or(cmp.Compare(b.length, a.length), cmp.Compare(a.rank, b.rank))
		}
		return cmp.Or(cmp.Compare(a.length, b.length), cmp.Compare(b.rank, a.rank))
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

// UnbuildableMutants returns the IDs of the mutants outside dropped that err, a refused build, blames.
// Those are the mutants spanning a source line the compiler refused.
// When no mutant spans one, or wholeFiles is set, they are the mutants of each file the compiler named.
// When no mutant lies in such a file either, they are every mutant outside dropped.
// It returns none when err holds no *MutantBuildError or every mutant is in dropped.
func (p *MutantPlan) UnbuildableMutants(err error, dropped map[int]bool, wholeFiles bool) []int {
	var buildErr *MutantBuildError
	if !errors.As(err, &buildErr) {
		return nil
	}
	blamed := func(blames func(Mutant, SourceLine) bool) []int {
		var ids []int
		for _, m := range p.Mutants {
			if !dropped[m.ID] && slices.ContainsFunc(buildErr.Lines, func(l SourceLine) bool { return blames(m, l) }) {
				ids = append(ids, m.ID)
			}
		}
		return ids
	}
	var ids []int
	if !wholeFiles {
		ids = blamed(func(m Mutant, l SourceLine) bool { return l.File == m.File && m.Line <= l.Line && l.Line <= m.lastLine })
	}
	if len(ids) == 0 {
		ids = blamed(func(m Mutant, l SourceLine) bool { return l.File == m.File })
	}
	if len(ids) == 0 {
		ids = blamed(func(Mutant, SourceLine) bool { return true })
	}
	return ids
}
