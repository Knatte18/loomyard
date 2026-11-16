// sourcescan.go statically marks the tests whose check reads the module's source files through its scan kit.
// Mutating the package's code cannot show what such a check pins.

package testcover

import (
	"go/ast"
	"math"
)

// scanKitRel is the module-relative import path of the module's source scanning kit.
const scanKitRel = "internal/testkit/scankit"

// ReasonSourceCheck is why a mutation check does not judge a test whose check reads the module's source files.
const ReasonSourceCheck = "checks the module's source files, which no mutant changes"

// ScanSourceReaders returns the names of the Test functions in pkgDir's _test.go files that reach the scan kit.
// A test reaches it when it references the kit, directly or through a followed call of a same-package function or method.
// module is the module's import path.
// A Go file of pkgDir that cannot be read or parsed is an error.
func ScanSourceReaders(module, pkgDir string) (map[string]bool, error) {
	src, err := loadPkgSource(pkgDir, true)
	if err != nil {
		return nil, err
	}
	kit := module + "/" + scanKitRel
	// memo holds each settled answer.
	// following holds the functions on the current call path, each at its depth, so a call cycle ends.
	// reaches also returns the lowest depth of a followed function its walk met, or math.MaxInt for none.
	// A false answer that rests on a followed function above fn is no settled answer, so it stays out of memo.
	memo := map[*funcInfo]bool{}
	following := map[*funcInfo]int{}
	var reaches func(fn *funcInfo, depth int) (bool, int)
	reaches = func(fn *funcInfo, depth int) (bool, int) {
		if found, seen := memo[fn]; seen {
			return found, math.MaxInt
		}
		if at, onPath := following[fn]; onPath {
			return false, at
		}
		following[fn] = depth
		found, lowest := false, math.MaxInt
		if fn.decl.Body != nil {
			ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && fn.imports[id.Name] == kit {
						found = true
					}
				case *ast.CallExpr:
					var callees []*funcInfo
					switch callee := unwrapCallee(x.Fun).(type) {
					case *ast.Ident:
						callees = src.funcs[callee.Name]
					case *ast.SelectorExpr:
						// A call on an imported package names that package's function, never a method of this one.
						if id, ok := callee.X.(*ast.Ident); !ok || fn.imports[id.Name] == "" {
							callees = src.methods[callee.Sel.Name]
						}
					}
					for _, callee := range callees {
						if found {
							break
						}
						reached, met := reaches(callee, depth+1)
						found = reached
						lowest = min(lowest, met)
					}
				}
				return !found
			})
		}
		delete(following, fn)
		if found || lowest >= depth {
			memo[fn] = found
		}
		return found, lowest
	}
	readers := map[string]bool{}
	for name, fns := range src.funcs {
		for _, fn := range fns {
			if !fn.isTest || !isTestEntry(name) {
				continue
			}
			if found, _ := reaches(fn, 0); found {
				readers[name] = true
			}
		}
	}
	return readers, nil
}
