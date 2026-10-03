// registration_test.go — membership oracle for the module registry.

package configreg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	modulePrefix     = "github.com/Knatte18/loomyard/"
	templateIdentity = "ConfigTemplate"
	minScannedFiles  = 100
)

// TestRegistration_MatchesDeclarers fails when a package under internal/ declares a package-level ConfigTemplate that Modules() does not reference,
// or when Modules() references a ConfigTemplate no package declares.
func TestRegistration_MatchesDeclarers(t *testing.T) {
	declared := map[string]bool{}
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal"}}, func(f *scankit.File) {
		file := f.AST(t, parser.SkipObjectResolution)
		if declaresConfigTemplate(file) {
			declared[path.Dir(f.Rel)] = true
		}
	})
	scankit.RequireFloor(t, scanned, minScannedFiles, "production files under internal/")

	registered := registeredPackages(t)

	for dir := range declared {
		if !registered[dir] {
			t.Errorf("%s declares %s but Modules() does not register it", dir, templateIdentity)
		}
	}
	for dir := range registered {
		if !declared[dir] {
			t.Errorf("Modules() references %s.%s but no package declares it", dir, templateIdentity)
		}
	}
}

func declaresConfigTemplate(file *ast.File) bool {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == templateIdentity {
				return true
			}
		case *ast.GenDecl:
			if d.Tok != token.VAR && d.Tok != token.CONST {
				continue
			}
			for _, spec := range d.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if name.Name == templateIdentity {
						return true
					}
				}
			}
		}
	}
	return false
}

// registeredPackages returns the module-relative directory of every package whose ConfigTemplate configreg.go's Modules() references.
func registeredPackages(t *testing.T) map[string]bool {
	t.Helper()
	registered := map[string]bool{}
	found := false
	scankit.Walk(t, scankit.Options{Roots: []string{"internal/configreg"}, Shallow: true}, func(f *scankit.File) {
		if path.Base(f.Rel) != "configreg.go" {
			return
		}
		found = true
		file := f.AST(t, 0)
		importPaths := map[string]string{}
		for _, imp := range file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("configreg.go: bad import %s: %v", imp.Path.Value, err)
			}
			name := path.Base(p)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			importPaths[name] = p
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "Modules" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != templateIdentity {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				p, ok := importPaths[ident.Name]
				if !ok || !strings.HasPrefix(p, modulePrefix) {
					t.Errorf("Modules() references %s.%s, which is not a module-local import", ident.Name, templateIdentity)
					return true
				}
				registered[strings.TrimPrefix(p, modulePrefix)] = true
				return true
			})
		}
	})
	if !found {
		t.Fatal("configreg.go not found")
	}
	if len(registered) == 0 {
		t.Fatal("Modules() references no ConfigTemplate")
	}
	return registered
}
