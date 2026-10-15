// modelchoicemapping_enforcement_test.go enforces PATTERN-model-choice-mapping:
// only NewModelChoice reads a modelspec.Resolved field outside internal/modelspec,
// so every resolved model key reaches a spawn as a ModelChoice.

package shuttleengine

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	// modelspecImportSuffix ends the import path of the package that declares Resolved.
	modelspecImportSuffix = "/internal/modelspec"
	// modelspecDirectory is the one directory whose files may read a Resolved field freely.
	modelspecDirectory = "internal/modelspec/"
	// mappingFile and mappingFunction name the one function outside modelspec that reads the fields.
	mappingFile     = "internal/shuttleengine/modelchoice.go"
	mappingFunction = "NewModelChoice"
)

// resolvedFields are the modelspec.Resolved fields no production code outside the mapping reads.
var resolvedFields = []string{"Engine", "Model", "Params"}

// modelChoiceMappingViolations returns "file:line" for every read of a modelspec.Resolved field in the production files under base, outside internal/modelspec and NewModelChoice, with the count of files scanned.
// The scan is syntactic: it sees a Resolved held by an identifier that is assigned from a Resolve call, declared with the type, or ranged over from a map or slice of it.
// A Resolved reached through a struct field or a call chain is outside its sight.
func modelChoiceMappingViolations(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var violations []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, modelspecDirectory) {
			return
		}
		file := f.AST(t, 0)
		qualifier := modelspecQualifier(file)
		if qualifier == "" {
			return
		}
		resolved := resolvedIdentifiers(file, qualifier)
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && f.Rel == mappingFile && function.Name.Name == mappingFunction {
				continue
			}
			ast.Inspect(declaration, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, ok := selector.X.(*ast.Ident)
				if ok && resolved[receiver.Name] && slices.Contains(resolvedFields, selector.Sel.Name) {
					violations = append(violations, fmt.Sprintf("%s:%d", f.Rel, f.FileSet().Position(selector.Pos()).Line))
				}
				return true
			})
		}
	})
	return violations, scanned
}

// modelspecQualifier returns the name the file imports internal/modelspec under, or "" when it does not import it.
func modelspecQualifier(file *ast.File) string {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasSuffix(path, modelspecImportSuffix) {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "modelspec"
	}
	return ""
}

// resolvedIdentifiers returns the identifiers in file that hold a modelspec.Resolved.
func resolvedIdentifiers(file *ast.File, qualifier string) map[string]bool {
	resolved := map[string]bool{}
	// holders are identifiers of a map, slice or array whose element is a Resolved.
	holders := map[string]bool{}
	mentionsResolved := func(expression ast.Expr) bool {
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := selector.X.(*ast.Ident); ok && x.Name == qualifier && selector.Sel.Name == "Resolved" {
				found = true
			}
			return !found
		})
		return found
	}
	isCollection := func(expression ast.Expr) bool {
		switch expression.(type) {
		case *ast.MapType, *ast.ArrayType:
			return true
		}
		return false
	}
	declare := func(names []*ast.Ident, typeExpression ast.Expr) {
		if typeExpression == nil || !mentionsResolved(typeExpression) {
			return
		}
		for _, name := range names {
			if isCollection(typeExpression) {
				holders[name.Name] = true
			} else {
				resolved[name.Name] = true
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Field:
			declare(n.Names, n.Type)
		case *ast.ValueSpec:
			declare(n.Names, n.Type)
			for i, value := range n.Values {
				if i < len(n.Names) && resolveCall(value) {
					resolved[n.Names[i].Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, value := range n.Rhs {
				name, ok := identifierAt(n.Lhs, i)
				if !ok {
					continue
				}
				if resolveCall(value) {
					resolved[name] = true
				}
				if literal, ok := value.(*ast.CompositeLit); ok && literal.Type != nil && isCollection(literal.Type) && mentionsResolved(literal.Type) {
					holders[name] = true
				}
			}
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		source, ok := loop.X.(*ast.Ident)
		value, valueOK := loop.Value.(*ast.Ident)
		if ok && valueOK && holders[source.Name] {
			resolved[value.Name] = true
		}
		return true
	})
	return resolved
}

// resolveCall reports whether expression is a call of a method named Resolve.
func resolveCall(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Resolve"
}

// identifierAt returns the name of the identifier at index i of expressions, and whether there is one.
func identifierAt(expressions []ast.Expr, i int) (string, bool) {
	if i >= len(expressions) {
		return "", false
	}
	identifier, ok := expressions[i].(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

// TestModelChoiceMappingRule verifies that no production file outside internal/modelspec and NewModelChoice reads a modelspec.Resolved field.
// It also verifies that the scan finds a violation for each way the scan can see a Resolved.
//
//testtiming:keep scans every production file for a modelspec.Resolved field read outside the mapping, which no other test reads
func TestModelChoiceMappingRule(t *testing.T) {
	t.Parallel()

	const header = "package x\n\nimport \"github.com/Knatte18/loomyard/internal/modelspec\"\n\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"ReadsModelAfterResolve", map[string]string{"internal/x/x.go": header + "func f(reg modelspec.Registry, spec string) string {\n\tresolved, _ := reg.Resolve(spec)\n\treturn resolved.Model\n}\n"}, []string{"internal/x/x.go:7"}},
		{"ReadsParamsAfterResolve", map[string]string{"internal/x/x.go": header + "func f(reg modelspec.Registry, spec string) string {\n\tresolved, _ := reg.Resolve(spec)\n\treturn resolved.Params[\"effort\"]\n}\n"}, []string{"internal/x/x.go:7"}},
		{"ReadsEngineAfterResolve", map[string]string{"internal/x/x.go": header + "func f(reg modelspec.Registry, spec string) string {\n\tresolved, _ := reg.Resolve(spec)\n\treturn resolved.Engine\n}\n"}, []string{"internal/x/x.go:7"}},
		{"ReadsModelOfParameter", map[string]string{"internal/x/x.go": header + "func f(r modelspec.Resolved) string {\n\treturn r.Model\n}\n"}, []string{"internal/x/x.go:6"}},
		{"ReadsEngineOfVar", map[string]string{"internal/x/x.go": header + "func f() string {\n\tvar r modelspec.Resolved\n\treturn r.Engine\n}\n"}, []string{"internal/x/x.go:7"}},
		{"ReadsParamsOfRangeValue", map[string]string{"internal/x/x.go": header + "func f(m map[string]modelspec.Resolved) string {\n\tfor _, v := range m {\n\t\treturn v.Params[\"effort\"]\n\t}\n\treturn \"\"\n}\n"}, []string{"internal/x/x.go:7"}},
		{"PassesResolvedWhole", map[string]string{"internal/x/x.go": header + "func f(reg modelspec.Registry, spec string) {\n\tresolved, _ := reg.Resolve(spec)\n\tuse(resolved)\n}\n\nfunc use(modelspec.Resolved) {}\n"}, nil},
		{"ReadsInsideModelspec", map[string]string{"internal/modelspec/x.go": header + "func f(r modelspec.Resolved) string {\n\treturn r.Model\n}\n"}, nil},
		{"ReadsInsideNewModelChoice", map[string]string{mappingFile: header + "func NewModelChoice(resolved modelspec.Resolved) string {\n\treturn resolved.Model\n}\n"}, nil},
		{"ReadsInAnotherFunctionOfTheMappingFile", map[string]string{mappingFile: header + "func other(resolved modelspec.Resolved) string {\n\treturn resolved.Model\n}\n"}, []string{mappingFile + ":6"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			for rel, content := range tt.files {
				abs := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, _ := modelChoiceMappingViolations(t, base)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("modelChoiceMappingViolations = %v; want %v", got, tt.want)
			}
		})
	}

	t.Run("RealModule", func(t *testing.T) {
		t.Parallel()
		got, scanned := modelChoiceMappingViolations(t, scankit.Root(t))
		scankit.RequireFloor(t, scanned, 1, "module-wide model-choice mapping scan")
		if len(got) > 0 {
			t.Errorf("PATTERN-model-choice-mapping violated: only NewModelChoice reads a modelspec.Resolved field, but one is read at: %v", got)
		}
	})
}
