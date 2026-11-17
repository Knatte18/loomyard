// promptengine_enforcement_test.go enforces that a prompt rendered for one engine never launches on another:
// every production launch-spec literal names the engine its prompt was rendered for or marks the prompt hand-written.
// Only `lym shuttle run` marks a prompt hand-written.

package shuttleengine

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// shuttleengineImportSuffix ends the import path of the package that declares Spec.
	shuttleengineImportSuffix = "/internal/shuttleengine"
	// handWrittenDirectory is the one directory whose launch specs may mark a prompt hand-written.
	handWrittenDirectory = "internal/shuttlecli"
)

// promptEngineViolations returns "file:line: reason" for every production Spec literal under base that breaks the rule.
// A literal that sets Prompt must set PromptEngine or HandWrittenPrompt.
// HandWrittenPrompt is set only in handWrittenDirectory.
// A literal that sets no Prompt, such as an attach probe's, is skipped.
// It also returns the count of files scanned.
// The scan is syntactic: it sees a literal of the package-qualified Spec type, not a spec filled field by field.
func promptEngineViolations(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var violations []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		file := f.AST(t, 0)
		qualifier := shuttleengineQualifier(file)
		if qualifier == "" {
			return
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !isSpecType(literal.Type, qualifier) {
				return true
			}
			keys := literalKeys(literal)
			line := f.FileSet().Position(literal.Pos()).Line
			if keys["HandWrittenPrompt"] && path.Dir(f.Rel) != handWrittenDirectory {
				violations = append(violations, fmt.Sprintf("%s:%d: marks its prompt hand-written outside %s", f.Rel, line, handWrittenDirectory))
			}
			if keys["Prompt"] && !keys["PromptEngine"] && !keys["HandWrittenPrompt"] {
				violations = append(violations, fmt.Sprintf("%s:%d: sets Prompt but neither PromptEngine nor HandWrittenPrompt", f.Rel, line))
			}
			return true
		})
	})
	return violations, scanned
}

// shuttleengineQualifier returns the name the file imports internal/shuttleengine under.
// It returns "" when the file does not import it.
func shuttleengineQualifier(file *ast.File) string {
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasSuffix(importPath, shuttleengineImportSuffix) {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path.Base(importPath)
	}
	return ""
}

// isSpecType reports whether expr is the type qualifier.Spec.
func isSpecType(expr ast.Expr, qualifier string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Spec" {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == qualifier
}

// literalKeys returns the field names literal sets by key.
func literalKeys(literal *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); ok {
			keys[key.Name] = true
		}
	}
	return keys
}

// TestPromptEngineRule scans the module's production files for launch-spec literals that set a prompt.
// It verifies that each names its prompt engine or marks the prompt hand-written.
// It also verifies that only internal/shuttlecli marks one hand-written.
// It also verifies the scan flags each break of the rule and passes a literal that keeps it.
//
//testtiming:keep scans every production launch-spec literal for its prompt engine, which no other test reads
//lym:guard
func TestPromptEngineRule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const header = "package x\n\nimport \"github.com/Knatte18/loomyard/internal/shuttleengine\"\n\n"
	tests := []struct {
		name string
		rel  string
		body string
		want []string
	}{
		{"PromptWithoutEngine", "internal/x/x.go", `var s = shuttleengine.Spec{Prompt: "p"}`, []string{"internal/x/x.go:5: sets Prompt but neither PromptEngine nor HandWrittenPrompt"}},
		{"PromptWithEngine", "internal/x/x.go", `var s = shuttleengine.Spec{Prompt: "p", PromptEngine: "claude"}`, nil},
		{"ProbeWithoutPrompt", "internal/x/x.go", `var s = shuttleengine.Spec{OutputFiles: nil}`, nil},
		{"HandWrittenInShuttleCLI", "internal/shuttlecli/x.go", `var s = shuttleengine.Spec{Prompt: "p", HandWrittenPrompt: true}`, nil},
		{"HandWrittenElsewhere", "internal/x/x.go", `var s = shuttleengine.Spec{Prompt: "p", HandWrittenPrompt: true}`, []string{"internal/x/x.go:5: marks its prompt hand-written outside internal/shuttlecli"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			abs := filepath.Join(base, filepath.FromSlash(tt.rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(header+tt.body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, _ := promptEngineViolations(t, base)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("promptEngineViolations = %q; want %q", got, tt.want)
			}
		})
	}

	t.Run("RealModule", func(t *testing.T) {
		t.Parallel()
		got, scanned := promptEngineViolations(t, scankit.Root(t))
		scankit.RequireFloor(t, scanned, 1, "module-wide prompt-engine scan")
		for _, violation := range got {
			t.Errorf("launch-spec literal breaks the prompt-engine rule: %s; way forward: render the prompt for the engine the spec's model choice resolves to and set it as PromptEngine", violation)
		}
	})
}
