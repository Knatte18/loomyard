// changedfuncs.go finds the functions a diff changes, from the file contents of both sides of the diff.

package testreach

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// initFuncName is the name of the functions a package may declare any number of times.
const initFuncName = "init"

// Module names the Go module a coverage map holds.
type Module struct {
	// Dir is the absolute root directory of the module.
	Dir string
	// Rel is the module's worktree-relative slash path, empty or "." for a module at the worktree root.
	Rel string
	// Path is the module's import path.
	Path string
}

// FileSet maps a worktree-relative slash path to the contents of the file at that path.
type FileSet map[string][]byte

// ChangedFunc is one function a diff adds or changes.
type ChangedFunc struct {
	// Key names the function by import path, file name and function name, `Type.Method` for a method.
	Key testcover.FuncKey
	// Hash is the testcover.ContentHash of the function's declaration text on the head side.
	Hash string
	// New is set when the base side holds no function the function matches.
	New bool
}

// declaredFunc is one function declaration of one side, with the key that matches it to the other side.
type declaredFunc struct {
	key     testcover.FuncKey
	hash    string
	matchID string
}

// ChangedFuncs returns the functions of head that are new or whose declaration text differs from base, sorted by key.
// Both sets hold the files of every package whose functions the caller wants compared,
// so a function that moved between files of one package is seen on both sides.
// A function is matched across the sets by package and name, whatever its file.
// A name that either side defines in more than one file of its package is matched by package, file and name instead.
// Build-constrained variants are such a name.
// The init functions of one file are matched by package, file and their order in that file.
// A function only base holds is no changed function.
// Only production Go files of the module count; a file that does not parse is an error naming it.
func ChangedFuncs(module Module, base, head FileSet) ([]ChangedFunc, error) {
	baseFuncs, err := declaredFuncs(module, base)
	if err != nil {
		return nil, err
	}
	headFuncs, err := declaredFuncs(module, head)
	if err != nil {
		return nil, err
	}
	ambiguous := ambiguousNames(baseFuncs, headFuncs)
	baseByID := map[string]declaredFunc{}
	for _, fn := range baseFuncs {
		baseByID[matchID(fn, ambiguous)] = fn
	}
	var changed []ChangedFunc
	for _, fn := range headFuncs {
		before, found := baseByID[matchID(fn, ambiguous)]
		if found && before.hash == fn.hash {
			continue
		}
		changed = append(changed, ChangedFunc{Key: fn.key, Hash: fn.hash, New: !found})
	}
	slices.SortStableFunc(changed, func(a, b ChangedFunc) int { return testcover.CompareFuncKeys(a.Key, b.Key) })
	return changed, nil
}

// matchID returns the text that matches fn to the same function of the other side.
func matchID(fn declaredFunc, ambiguous map[string]bool) string {
	nameID := fn.key.Package + "|" + fn.key.Func
	if fn.key.Func == initFuncName || ambiguous[nameID] {
		return fn.key.Package + "|" + fn.key.File + "|" + fn.matchID
	}
	return nameID
}

// ambiguousNames returns the package-and-name texts that either side declares in more than one file.
func ambiguousNames(sides ...[]declaredFunc) map[string]bool {
	ambiguous := map[string]bool{}
	for _, side := range sides {
		files := map[string]string{}
		for _, fn := range side {
			nameID := fn.key.Package + "|" + fn.key.Func
			if file, seen := files[nameID]; seen && file != fn.key.File {
				ambiguous[nameID] = true
			}
			files[nameID] = fn.key.File
		}
	}
	return ambiguous
}

// declaredFuncs returns the functions of the production Go files of files that lie in the module, in path order.
func declaredFuncs(module Module, files FileSet) ([]declaredFunc, error) {
	return declaredFuncsWhere(module, files, packageImportPath, func(*ast.FuncDecl) bool { return true })
}

// declaredFuncsWhere returns the functions keep accepts of the files importPath places in the module, in path order.
func declaredFuncsWhere(module Module, files FileSet, importPath func(Module, string) (string, bool), keep func(*ast.FuncDecl) bool) ([]declaredFunc, error) {
	var funcs []declaredFunc
	for _, name := range slices.Sorted(maps.Keys(files)) {
		pkg, ok := importPath(module, name)
		if !ok {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, name, files[name], parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		file := fset.File(parsed.Pos())
		inits := 0
		for _, decl := range parsed.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || !keep(fn) {
				continue
			}
			funcName := funcKeyName(fn)
			id := funcName
			if funcName == initFuncName {
				id = fmt.Sprintf("%s#%d", initFuncName, inits)
				inits++
			}
			text := files[name][file.Offset(fn.Pos()):file.Offset(fn.End())]
			funcs = append(funcs, declaredFunc{
				key:     testcover.FuncKey{Package: pkg, File: path.Base(name), Func: funcName},
				hash:    testcover.ContentHash(text),
				matchID: id,
			})
		}
	}
	return funcs, nil
}

// packageImportPath returns the import path of the package that holds the file at the worktree-relative slash path name.
// It reports false for a file that is no production Go file of the module.
// A test file and a file outside the module are such files.
// So is a file under testdata or a directory opening with "." or "_".
func packageImportPath(module Module, name string) (string, bool) {
	if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
		return "", false
	}
	return directoryImportPath(module, name)
}

// directoryImportPath returns the import path of the directory that holds a file.
// name is the file's worktree-relative slash path.
// It reports false for a file outside the module, and for one under testdata or a directory opening with "." or "_".
func directoryImportPath(module Module, name string) (string, bool) {
	rel := name
	if module.Rel != "" && module.Rel != "." {
		var inside bool
		if rel, inside = strings.CutPrefix(name, module.Rel+"/"); !inside {
			return "", false
		}
	}
	dir := path.Dir(rel)
	if dir == "." {
		return module.Path, true
	}
	for _, segment := range strings.Split(dir, "/") {
		if segment == "testdata" || strings.HasPrefix(segment, ".") || strings.HasPrefix(segment, "_") {
			return "", false
		}
	}
	return module.Path + "/" + dir, true
}

// funcKeyName spells a function's name as a block key does: `Type.Method` for a method, the bare name otherwise.
func funcKeyName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// receiverTypeName strips the pointer, parentheses and type arguments from a receiver type.
func receiverTypeName(expr ast.Expr) string {
	for {
		switch x := expr.(type) {
		case *ast.StarExpr:
			expr = x.X
		case *ast.ParenExpr:
			expr = x.X
		case *ast.IndexExpr:
			expr = x.X
		case *ast.IndexListExpr:
			expr = x.X
		case *ast.Ident:
			return x.Name
		default:
			return types.ExprString(expr)
		}
	}
}
