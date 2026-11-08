// shape_enforcement_test.go enforces the `PATTERN-sole-parsers` rule with AST scans.
// The scans read the production files of internal/planparser and internal/planglyph.
// The refKind scan flags names of classifyRef, refKind and its constants outside classify.go and shape.go;
// it does not ban consuming the registry's answers.
// The plan:-op scan flags open-coded "plan:" string surgery outside those two files and internal/planparser/handle.go.
// Test files are skipped by design, since a test fixture legitimately spells a raw "plan:" ref or calls classifyRef.

package planparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// bannedRefKindIdentifiers returns the refKind scan's banned identifier set.
// It holds the type name refKind, the sole classifier classifyRef, and every refKind constant.
// The constants are read from classify.go through refKindConstNamesInDeclarationOrder (shape_helper_test.go),
// so a constant added later is banned without a list edit.
func bannedRefKindIdentifiers(t *testing.T) map[string]bool {
	t.Helper()

	banned := map[string]bool{
		"refKind":     true,
		"classifyRef": true,
	}
	for _, name := range refKindConstNamesInDeclarationOrder(t) {
		banned[name] = true
	}
	return banned
}

// enforcementScanMinFiles is the plausible floor for the number of production .go files the scanned packages hold;
// below it a scan has read the wrong directories.
const enforcementScanMinFiles = 10

// enforcementScanTarget is one production .go file under scan: its parsed AST and its repository-relative path.
// The path is rendered POSIX-style, for exempt-set comparison and failure reporting.
type enforcementScanTarget struct {
	astFile *ast.File
	relPath string
}

// scanTargets walks the production files of internal/planparser and internal/planglyph from the module root.
// It also returns how many files the walk read, so a caller can check that it read the expected directories.
func scanTargets(t *testing.T) ([]enforcementScanTarget, int) {
	t.Helper()

	var targets []enforcementScanTarget
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/planparser", "internal/planglyph"}}, func(f *scankit.File) {
		targets = append(targets, enforcementScanTarget{astFile: f.AST(t, 0), relPath: f.Rel})
	})
	return targets, scanned
}

// refKindScanExempt lists the production files that may name the refKind identifiers:
// classify.go and shape.go, the files that declare the ref-shape registry.
// No internal/planglyph file is listed, because planglyph cannot reference an unexported planparser identifier.
// Planglyph files are scanned as a name-shape check only:
// a local identifier that collides with a banned name still trips the scan.
var refKindScanExempt = []scankit.Entry{
	{Key: "internal/planparser/classify.go", Why: "the registry's own declared home"},
	{Key: "internal/planparser/shape.go", Why: "the registry's own declared home"},
}

// planOpScanExempt lists the production files that may open-code "plan:" string surgery.
// classify.go and shape.go are the home of the ref-shape registry.
// internal/planparser/handle.go owns the handle grammar.
// internal/planglyph/handle.go is not listed, though it shares a basename with internal/planparser/handle.go.
// The shared basename is where the check applies.
// So planglyph's handle.go is scanned like any other planglyph production file.
var planOpScanExempt = []scankit.Entry{
	{Key: "internal/planparser/classify.go", Why: "the registry's own declared home"},
	{Key: "internal/planparser/shape.go", Why: "the registry's own declared home"},
	{Key: "internal/planparser/handle.go", Why: "the handle grammar's declared owner"},
}

// refKindScanFile returns the name of every banned identifier that appears in an already-parsed production file.
// The match is on the AST alone, so a comment naming a banned identifier never trips it.
func refKindScanFile(astFile *ast.File, banned map[string]bool) []string {
	var hits []string
	ast.Inspect(astFile, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if banned[ident.Name] {
			hits = append(hits, ident.Name)
		}
		return true
	})
	return hits
}

// isHandlePrefixIdent reports whether e names HandlePrefix, bare or as a selector such as planparser.HandlePrefix.
func isHandlePrefixIdent(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "HandlePrefix"
	case *ast.SelectorExpr:
		return x.Sel != nil && x.Sel.Name == "HandlePrefix"
	}
	return false
}

// isPlanColonLiteral reports whether e is the string literal "plan:", decoded via strconv.Unquote,
// so the comparison is against the literal's value rather than its quoted source text.
func isPlanColonLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	v, err := strconv.Unquote(lit.Value)
	return err == nil && v == "plan:"
}

// isHandlePrefixOrPlanLiteral reports whether e is a HandlePrefix reference or a "plan:" literal.
// Either form must not be the second argument of strings.HasPrefix or strings.TrimPrefix, nor an operand of +.
func isHandlePrefixOrPlanLiteral(e ast.Expr) bool {
	return isHandlePrefixIdent(e) || isPlanColonLiteral(e)
}

func isStringsHasPrefixOrTrimPrefixCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "strings" {
		return false
	}
	return sel.Sel.Name == "HasPrefix" || sel.Sel.Name == "TrimPrefix"
}

// planOpScanFile returns, for one already-parsed production file, one description per banned plan: string operation.
// It flags a strings.HasPrefix or strings.TrimPrefix call whose second argument is HandlePrefix or a "plan:" literal.
// It also flags a "+" concatenation with HandlePrefix or a "plan:" literal as either operand.
// A "plan:" literal outside those operand positions never trips it.
// Comments never trip it either, since they are not AST nodes.
func planOpScanFile(astFile *ast.File) []string {
	var hits []string
	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if isStringsHasPrefixOrTrimPrefixCall(node) && len(node.Args) == 2 && isHandlePrefixOrPlanLiteral(node.Args[1]) {
				hits = append(hits, "strings.HasPrefix/TrimPrefix against a plan: shape operand")
			}
		case *ast.BinaryExpr:
			if node.Op == token.ADD && (isHandlePrefixOrPlanLiteral(node.X) || isHandlePrefixOrPlanLiteral(node.Y)) {
				hits = append(hits, "string concatenation against a plan: shape operand")
			}
		}
		return true
	})
	return hits
}

// TestRefKindScan_CatchesSeededViolation checks that the matcher flags a synthetic source.
// The source calls classifyRef and compares against refKindPath.
// Without that check, a matcher that flags nothing would pass every real tree.
func TestRefKindScan_CatchesSeededViolation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const src = `package fakeplanparser

func offside(raw string) bool {
	return classifyRef(raw) == refKindPath
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanparser.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	banned := bannedRefKindIdentifiers(t)
	got := refKindScanFile(astFile, banned)
	if len(got) != 2 {
		t.Fatalf("refKindScanFile() = %v; want two hits (classifyRef and refKindPath)", got)
	}
	wantNames := map[string]bool{"classifyRef": true, "refKindPath": true}
	for _, name := range got {
		if !wantNames[name] {
			t.Errorf("refKindScanFile() hit %q; want one of classifyRef/refKindPath", name)
		}
	}
}

// TestPlanOpScan_CatchesSeededViolation checks that the matcher flags a synthetic source.
// The source calls strings.HasPrefix(raw, "plan:").
// Without that check, a matcher that flags nothing would pass every real tree.
func TestPlanOpScan_CatchesSeededViolation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const src = `package fakeplanparser

import "strings"

func offside(raw string) bool {
	return strings.HasPrefix(raw, "plan:")
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanparser.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := planOpScanFile(astFile)
	if len(got) != 1 {
		t.Fatalf("planOpScanFile() = %v; want exactly one hit", got)
	}
}

// TestRefKindScan_RealTreeClean checks every production file in internal/planparser and internal/planglyph.
// Only classify.go and shape.go may name classifyRef, the refKind type, or one of its declared constants.
// A narrowed gate runs it only when a scanned package or scankit changes; a whole gate always runs it.
//
//lym:guard
//lym:watch internal/planparser/*.go internal/planparser/*/*.go internal/planglyph/*.go internal/planglyph/*/*.go
//lym:watch internal/testkit/scankit/*.go
func TestRefKindScan_RealTreeClean(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	targets, scanned := scanTargets(t)
	scankit.RequireFloor(t, scanned, enforcementScanMinFiles, "refKind scan")
	banned := bannedRefKindIdentifiers(t)
	exempt := scankit.NewAllowlist(refKindScanExempt)

	var failures []string
	for _, target := range targets {
		if exempt.Allowed(target.relPath) {
			continue
		}
		for _, name := range refKindScanFile(target.astFile, banned) {
			failures = append(failures, target.relPath+": "+name)
		}
	}
	exempt.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("Ref-Shape Registry Invariant violated: a production file outside classify.go/shape.go "+
			"names classifyRef or a refKind identifier directly, rather than routing through lookup: %v "+
			"(see PATTERN-sole-parsers)", failures)
	}
}

// TestPlanOpScan_RealTreeClean checks every production file in internal/planparser and internal/planglyph.
// Only classify.go, shape.go and internal/planparser/handle.go may open-code "plan:" string surgery.
// internal/planglyph/handle.go is not exempt, though it shares a basename with internal/planparser/handle.go.
// A narrowed gate runs it only when a scanned package or scankit changes; a whole gate always runs it.
//
//lym:guard
//lym:watch internal/planparser/*.go internal/planparser/*/*.go internal/planglyph/*.go internal/planglyph/*/*.go
//lym:watch internal/testkit/scankit/*.go
func TestPlanOpScan_RealTreeClean(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	targets, scanned := scanTargets(t)
	scankit.RequireFloor(t, scanned, enforcementScanMinFiles, "plan:-op scan")
	exempt := scankit.NewAllowlist(planOpScanExempt)

	var failures []string
	for _, target := range targets {
		if exempt.Allowed(target.relPath) {
			continue
		}
		for _, hit := range planOpScanFile(target.astFile) {
			failures = append(failures, target.relPath+": "+hit)
		}
	}
	exempt.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("Ref-Shape Registry Invariant violated: a production file outside the handle grammar's "+
			"declared owners open-codes \"plan:\" string surgery instead of calling into planparser's "+
			"exported handle vocabulary: %v (see PATTERN-sole-parsers)", failures)
	}
}
