// literals_test.go scans production code for launch-spec literals, seat tables and seats the parity tests do not cover.

package shuttleparity

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

// unscannedDirectories are the places whose launch specs the parity tests leave out.
// The orch spec names one engine, and the shuttle verbs launch a hand-written prompt or no role.
// The test-support packages launch nothing in production.
var unscannedDirectories = []string{"internal/orchcli", "internal/orchengine", "internal/shuttlecli", "internal/testkit"}

// unlisted is what a scan found that listed does not cover, each entry `file:line: <glyph>`.
type unlisted struct {
	specs []string
	seats []string
}

// unlistedLiterals scans the production files under base.
// It returns the literals whose enclosing function listed does not hold.
// A launch-spec literal is a Spec literal that sets Prompt.
// A seat literal is a non-empty Table literal or a Seat literal that sets Stencil.
// It also returns the count of files scanned.
// The scan is syntactic: it sees a literal of a package-qualified type, not a value filled field by field.
func unlistedLiterals(t *testing.T, base string, listed map[string]bool) (unlisted, int) {
	t.Helper()
	var found unlisted
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		dir := path.Dir(f.Rel)
		for _, skipped := range unscannedDirectories {
			if dir == skipped || strings.HasPrefix(dir, skipped+"/") {
				return
			}
		}
		file := f.AST(t, 0)
		qualifiers := importQualifiers(file)
		for _, decl := range file.Decls {
			glyph := dir + "#" + declName(decl)
			// An element literal of a slice or map may leave its type out, so it takes its container's element type.
			elided := map[*ast.CompositeLit]ast.Expr{}
			ast.Inspect(decl, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				literalType := literal.Type
				if literalType == nil {
					literalType = elided[literal]
				}
				recordElidedElements(literal, literalType, elided)
				if listed[glyph] {
					return true
				}
				at := fmt.Sprintf("%s:%d: %s", f.Rel, f.FileSet().Position(literal.Pos()).Line, glyph)
				switch qualifiedName(literalType, qualifiers) {
				case "shuttleengine.Spec":
					if literalKeys(literal)["Prompt"] {
						found.specs = append(found.specs, at)
					}
				case "seatengine.Table":
					if len(literal.Elts) > 0 {
						found.seats = append(found.seats, at)
					}
				case "seatengine.Seat":
					if literalKeys(literal)["Stencil"] {
						found.seats = append(found.seats, at)
					}
				}
				return true
			})
		}
	})
	return found, scanned
}

// importQualifiers maps the name file imports each of the module's scanned packages under to the package's own name.
func importQualifiers(file *ast.File) map[string]string {
	qualifiers := map[string]string{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		for _, pkg := range []string{"shuttleengine", "seatengine"} {
			if !strings.HasSuffix(importPath, "/internal/"+pkg) {
				continue
			}
			name := pkg
			if imp.Name != nil {
				name = imp.Name.Name
			}
			qualifiers[name] = pkg
		}
	}
	return qualifiers
}

// qualifiedName returns `<package>.<Type>` for a type of a scanned package, a pointer to one included, and "" otherwise.
func qualifiedName(expr ast.Expr, qualifiers map[string]string) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok || qualifiers[receiver.Name] == "" {
		return ""
	}
	return qualifiers[receiver.Name] + "." + selector.Sel.Name
}

// recordElidedElements notes the element type of each element literal of literal that names no type of its own.
func recordElidedElements(literal *ast.CompositeLit, literalType ast.Expr, elided map[*ast.CompositeLit]ast.Expr) {
	var element ast.Expr
	switch container := literalType.(type) {
	case *ast.ArrayType:
		element = container.Elt
	case *ast.MapType:
		element = container.Value
	default:
		return
	}
	for _, expr := range literal.Elts {
		if pair, ok := expr.(*ast.KeyValueExpr); ok {
			expr = pair.Value
		}
		if inner, ok := expr.(*ast.CompositeLit); ok && inner.Type == nil {
			elided[inner] = element
		}
	}
}

// declName returns the glyph name of decl: `<Func>`, `<Receiver>.<Method>`, or the first name of a variable declaration.
func declName(decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			return d.Name.Name
		}
		receiver := d.Recv.List[0].Type
		if star, ok := receiver.(*ast.StarExpr); ok {
			receiver = star.X
		}
		if generic, ok := receiver.(*ast.IndexExpr); ok {
			receiver = generic.X
		}
		return receiver.(*ast.Ident).Name + "." + d.Name.Name
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok {
				return value.Names[0].Name
			}
		}
	}
	return ""
}

// literalKeys returns the field names literal sets by key.
func literalKeys(literal *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, element := range literal.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			if key, ok := pair.Key.(*ast.Ident); ok {
				keys[key.Name] = true
			}
		}
	}
	return keys
}

// listedFunctions returns the glyphs of the functions whose literals the parity tests cover.
// They are the producer table's functions and the seat catalog's builders and holders.
// The seat composer is covered by the seat catalog, and the two row-config functions hold seats an operator writes.
func listedFunctions() map[string]bool {
	listed := map[string]bool{
		"internal/seatengine#seatSpec":      true,
		"internal/shedrecipe#multiLLMEntry": true,
		"internal/shedrecipe#multiLLMSeat":  true,
	}
	for _, p := range producers(parityFixture{}) {
		listed[p.Function] = true
	}
	for _, table := range seatTables {
		listed[table.Builder] = true
		for _, holder := range table.Holders {
			listed[holder] = true
		}
	}
	return listed
}

// TestLaunchSpecLiterals_AreListed scans the module for literals the parity tests would not reach.
// It also verifies the scan reports each kind of unlisted literal and skips what launches nothing.
//
//lym:guard
func TestLaunchSpecLiterals_AreListed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const header = "package x\n\nimport (\n\t\"github.com/Knatte18/loomyard/internal/seatengine\"\n\t\"github.com/Knatte18/loomyard/internal/shuttleengine\"\n)\n\n"
	const spec = `func build() { _ = shuttleengine.Spec{Prompt: "p"} }`
	tests := []struct {
		name      string
		rel       string
		body      string
		listed    string
		wantSpecs []string
		wantSeats []string
	}{
		{name: "an unlisted spec", rel: "internal/x/x.go", body: spec, wantSpecs: []string{"internal/x/x.go:8: internal/x#build"}},
		{name: "a listed spec", rel: "internal/x/x.go", body: spec, listed: "internal/x#build"},
		{name: "a probe with no prompt", rel: "internal/x/x.go", body: `func build() { _ = shuttleengine.Spec{Role: "r"} }`},
		{name: "the shuttle verbs", rel: "internal/shuttlecli/x.go", body: spec},
		{name: "the test support", rel: "internal/testkit/kit/x.go", body: spec},
		{name: "the orch verbs", rel: "internal/orchcli/x.go", body: spec},
		{name: "the orch engine", rel: "internal/orchengine/x.go", body: spec},
		{name: "a table", rel: "internal/x/x.go", body: `func build() { _ = seatengine.Table{Segment: "s"} }`, wantSeats: []string{"internal/x/x.go:8: internal/x#build"}},
		{name: "an empty table", rel: "internal/x/x.go", body: `func build() { _ = seatengine.Table{} }`},
		{name: "a pointer seat", rel: "internal/x/x.go", body: `func build() { _ = &seatengine.Seat{Stencil: "s"} }`, wantSeats: []string{"internal/x/x.go:8: internal/x#build"}},
		{name: "a seat with its type left out", rel: "internal/x/x.go", body: `func build() { _ = []seatengine.Seat{{Stencil: "s"}} }`, wantSeats: []string{"internal/x/x.go:8: internal/x#build"}},
		{name: "a seat with no stencil", rel: "internal/x/x.go", body: `func build() { _ = seatengine.Seat{} }`},
		{name: "a method", rel: "internal/x/x.go", body: `func (b *Box) build() { _ = shuttleengine.Spec{Prompt: "p"} }`, wantSpecs: []string{"internal/x/x.go:8: internal/x#Box.build"}},
		{name: "a function literal", rel: "internal/x/x.go", body: `func build() { _ = func() { _ = shuttleengine.Spec{Prompt: "p"} } }`, listed: "internal/x#build"},
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
			got, _ := unlistedLiterals(t, base, map[string]bool{tt.listed: true})
			if strings.Join(got.specs, "\n") != strings.Join(tt.wantSpecs, "\n") || strings.Join(got.seats, "\n") != strings.Join(tt.wantSeats, "\n") {
				t.Errorf("unlistedLiterals() = specs %q, seats %q; want specs %q, seats %q", got.specs, got.seats, tt.wantSpecs, tt.wantSeats)
			}
		})
	}

	t.Run("RealModule", func(t *testing.T) {
		t.Parallel()
		got, scanned := unlistedLiterals(t, scankit.Root(t), listedFunctions())
		scankit.RequireFloor(t, scanned, 1, "module-wide launch-spec literal scan")
		for _, literal := range got.specs {
			t.Errorf("unlisted launch-spec literal: %s; way forward: add the function to producers in internal/shuttleparity, or, for a probe, set no Prompt", literal)
		}
		for _, literal := range got.seats {
			t.Errorf("unlisted seat literal: %s; way forward: give a new table builder its own seatTables entry in internal/shuttleparity with Builder, Holders and Build, so its seats reach the parity test; add a helper that holds an existing builder's literal to that builder's Holders, with a Build that reaches the seat it adds", literal)
		}
	})
}
