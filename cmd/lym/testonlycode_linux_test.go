// testonlycode_linux_test.go guards `PATTERN-test-only-code`:
// every function of a production file is reachable from a main package of the module, outside the test-support packages.
// The analysis is deadcode's: rapid type analysis over SSA, rooted at each main package's init and main.
// Every function of a test-support package is a root too, so a function only such a package calls counts as reached.
// It reads the Linux build alone, so the file compiles on Linux only.

package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// loomyardModulePath is the module path the guard strips to name a package by its directory.
const loomyardModulePath = "github.com/Knatte18/loomyard"

// mainPackagePatterns name every main package of the module, the roots of the analysis.
var mainPackagePatterns = []string{"./cmd/...", "./tools/..."}

// testSupportPackages are the packages of production files that serve tests alone.
// Each carries the reason it sits outside internal/testkit.
// A package matches its own directory and every directory below it.
var testSupportPackages = []scankit.Entry{
	{Key: "internal/testkit", Why: "the kits tests of several packages share"},
	{Key: "internal/gitkit", Why: "test git plumbing that spawns git, which internal/testkit's import rules bar"},
	{Key: "internal/hubforge", Why: "hub fixtures built through fabriccli, which internal/testkit's import rules bar"},
}

// testOnlyCodeAllowed admits a function of a production file that no main reaches.
// Its key is `<directory>.<Func>` or `<directory>.<Type>.<Method>`.
// An entry is a reference or helper the tests of other packages share:
// a _test.go file of its package cannot serve them, and a kit cannot hold it.
// A kit cannot hold it because it reads its package's own state or writes a record its package alone writes.
// The reason names the tests that use it.
var testOnlyCodeAllowed = []scankit.Entry{
	{Key: "internal/loomrecipe.RecipeEngines", Why: "oracle of internal/shedrecipe's coverage guard test"},
	{Key: "internal/shedrecipe.Names", Why: "oracle of internal/shedbuild's engine test and internal/shedrecipe's coverage guard test"},
	{Key: "internal/driftengine.UpdateScope", Why: "writes the scope record driftengine alone writes, for internal/driftcli's and internal/loomcli's tests"},
	{Key: "internal/fabricengine.RecordPushFailure", Why: "writes the failed-push record fabric alone writes, for internal/fabriccli's, internal/orchcli's and internal/tendercli's tests"},
	{Key: "internal/fabricvocab.SideTokens", Why: "the fabric vocabulary the enforcement tests of internal/planparser, internal/partsparser and internal/reportengine check against"},
	{Key: "internal/fabricvocab.BareTokenInRootDoc", Why: "the fabric vocabulary check internal/lymcwd's enforcement test runs"},
	{Key: "internal/fabricvocab.IsOwner", Why: "the fabric vocabulary check internal/lymcwd's enforcement test runs"},
	{Key: "internal/hibernateengine.TerminalPhases", Why: "reads the package's terminal phase list for internal/fabriccli's hibernate daemon test"},
	{Key: "internal/seatengine.Prompts", Why: "oracle of the seat prompts for the tests of the packages that start seats"},
	{Key: "internal/shortcutcli.Load", Why: "reads the embedded shortcut file for cmd/lym's shortcut test"},
	{Key: "internal/shuttleengine/claudeengine.New", Why: "builds the engine, unexported state included, for the tests of the packages that drive it"},
	{Key: "internal/slackengine.WriteSetup", Why: "writes the credentials file slackengine alone writes, for internal/slackcli's and internal/beamcli's tests"},
	{Key: "internal/websterengine.ConflictRecordRel", Why: "oracle of the conflict record path in internal/loomrecipe's recipe test"},
	{Key: "internal/websterengine.Fingerprint", Why: "oracle of the plan fingerprint in internal/webstercli's tests"},
	{Key: "internal/websterengine.WriteReport", Why: "writes the fork-return report websterengine alone writes, for internal/webstercli's tests"},
}

// testOnlyCodeMinFunctions is the vacuous-scan floor for the functions of production files the guard examines.
const testOnlyCodeMinFunctions = 1000

// unreachableFunction is one function of a production file that no main package reaches.
type unreachableFunction struct {
	// key names the function as `<directory>.<Func>` or `<directory>.<Type>.<Method>`.
	key string
	// position is the declaration's file, line and column.
	position token.Position
}

// TestTestOnlyCode_EveryProductionFunctionIsReachableFromAMain fails for each production function no main reaches.
// The test-support packages and the allowlist are exempt.
// It also fails for an allowlist entry that matches no unreachable function, so the list tracks the tree.
//
//lym:guard
func TestTestOnlyCode_EveryProductionFunctionIsReachableFromAMain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	unreachable, examined := unreachableProductionFunctions(t, scankit.Root(t))
	scankit.RequireFloor(t, examined, testOnlyCodeMinFunctions, "test-only code guard")

	allowed := scankit.NewAllowlist(testOnlyCodeAllowed)
	for _, fn := range unreachable {
		if allowed.Allowed(fn.key) {
			continue
		}
		t.Errorf("%s: %s is reached by no main package of the module -- a function only tests call lives in a _test.go file or a test-support package, never a production file (see `PATTERN-test-only-code`); delete it with the tests that test only it, or move it into a _test.go file", fn.position, fn.key)
	}
	allowed.RequireNoStale(t)
}

// unreachableProductionFunctions loads every main package and test-support package of the module from root.
// It returns the functions of the module's production files that neither a main nor a test-support package reaches.
// The test-support packages' own functions are left out.
// It also returns how many such functions it examined.
func unreachableProductionFunctions(t *testing.T, root string) ([]unreachableFunction, int) {
	t.Helper()
	config := &packages.Config{
		Mode: packages.LoadAllSyntax | packages.NeedModule,
		Dir:  root,
	}
	patterns := append([]string{}, mainPackagePatterns...)
	for _, entry := range testSupportPackages {
		patterns = append(patterns, "./"+entry.Key+"/...")
	}
	logger.Info("cmd/lym test: spawning go list for the test-only code guard", "dir", root)
	initial, err := packages.Load(config, patterns...)
	logger.Info("cmd/lym test: go list for the test-only code guard exited", "err", err)
	if err != nil {
		t.Fatalf("load the main packages: %v", err)
	}
	if packages.PrintErrors(initial) > 0 {
		t.Fatalf("loading the main packages reported errors; see the lines above")
	}

	program, ssaPackages := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
	program.Build()
	var roots []*ssa.Function
	for _, mainPackage := range ssautil.MainPackages(ssaPackages) {
		roots = append(roots, mainPackage.Func("init"), mainPackage.Func("main"))
	}
	for _, ssaPackage := range ssaPackages {
		if ssaPackage == nil {
			continue
		}
		directory, inModule := strings.CutPrefix(ssaPackage.Pkg.Path(), loomyardModulePath+"/")
		if inModule && inTestSupportPackage(directory) {
			roots = append(roots, packageFunctions(program, ssaPackage)...)
		}
	}
	reachable := make(map[token.Position]bool)
	for fn := range rta.Analyze(roots, false).Reachable {
		if fn.Pos().IsValid() {
			reachable[program.Fset.Position(fn.Pos())] = true
		}
	}

	var unreachable []unreachableFunction
	examined := 0
	packages.Visit(initial, nil, func(p *packages.Package) {
		directory, inModule := strings.CutPrefix(p.PkgPath, loomyardModulePath+"/")
		if !inModule || inTestSupportPackage(directory) {
			return
		}
		for _, file := range p.Syntax {
			if ast.IsGenerated(file) {
				continue
			}
			for _, decl := range file.Decls {
				funcDecl, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || isMarkerMethod(funcDecl) {
					continue
				}
				examined++
				position := program.Fset.Position(funcDecl.Name.Pos())
				if !reachable[position] {
					unreachable = append(unreachable, unreachableFunction{key: directory + "." + functionName(funcDecl), position: position})
				}
			}
		}
	})
	return unreachable, examined
}

// packageFunctions returns the package-level functions of ssaPackage and the methods of its named types.
// A generic function or a method of a generic type is left out, since only its instances have a body.
// So a generic function of a test-support package is no root.
// A production function only it calls takes an allowlist entry.
func packageFunctions(program *ssa.Program, ssaPackage *ssa.Package) []*ssa.Function {
	var functions []*ssa.Function
	for _, member := range ssaPackage.Members {
		switch typed := member.(type) {
		case *ssa.Function:
			if typed.TypeParams().Len() == 0 {
				functions = append(functions, typed)
			}
		case *ssa.Type:
			named, isNamed := typed.Type().(*types.Named)
			if !isNamed || named.TypeParams().Len() > 0 || types.IsInterface(named) {
				continue
			}
			methods := program.MethodSets.MethodSet(types.NewPointer(named))
			for i := range methods.Len() {
				if method := program.MethodValue(methods.At(i)); method != nil {
					functions = append(functions, method)
				}
			}
		}
	}
	return functions
}

// inTestSupportPackage reports whether the package at directory is a test-support package or lies below one.
func inTestSupportPackage(directory string) bool {
	for _, entry := range testSupportPackages {
		if directory == entry.Key || strings.HasPrefix(directory, entry.Key+"/") {
			return true
		}
	}
	return false
}

// functionName names a declaration as `Func` or `Type.Method`, the receiver's pointer and type parameters dropped.
func functionName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return funcDecl.Name.Name
	}
	receiver := funcDecl.Recv.List[0].Type
	if star, isPointer := receiver.(*ast.StarExpr); isPointer {
		receiver = star.X
	}
	switch typed := receiver.(type) {
	case *ast.IndexExpr:
		receiver = typed.X
	case *ast.IndexListExpr:
		receiver = typed.X
	}
	if ident, isIdent := receiver.(*ast.Ident); isIdent {
		return ident.Name + "." + funcDecl.Name.Name
	}
	return funcDecl.Name.Name
}

// isMarkerMethod reports whether funcDecl is an unexported method with no parameters, no results and an empty body.
// Such a method marks its type as a member of a closed interface and is never called.
func isMarkerMethod(funcDecl *ast.FuncDecl) bool {
	return funcDecl.Recv != nil &&
		!ast.IsExported(funcDecl.Name.Name) &&
		funcDecl.Type.Params.NumFields() == 0 &&
		funcDecl.Type.Results.NumFields() == 0 &&
		funcDecl.Body != nil && len(funcDecl.Body.List) == 0
}
