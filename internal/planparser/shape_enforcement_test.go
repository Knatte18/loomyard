// shape_enforcement_test.go is the requirement-4 capstone: the two AST-based boundary-enforcement
// scans that prove the Ref-Shape Registry Invariant (CONSTRAINTS.md) holds over both packages'
// production files, not just at the registry's own file boundary. It follows the same idiom as
// internal/cliwire/bannedecl_enforcement_test.go and this package's own shape_test.go: stdlib
// go/parser only, repo root resolved from runtime.Caller(0), production files only -- every
// _test.go file in either package is skipped by design, because a test fixture legitimately spells
// a raw "plan:" ref or calls classifyRef directly.
//
// The refKind scan flags any ast.Ident naming classifyRef, the refKind type, or one of its declared
// constants, outside classify.go and shape.go -- the two files the Ref-Shape Registry Invariant
// names as the sole legal home for a refKind comparison or a classifyRef call. refKindName and
// lookup are deliberately NOT in the banned set: calling into shape.go's own exported-within-package
// API is always legal, because the ban is on comparing/switching over kinds and on classifying
// directly, not on consuming the registry's answer.
//
// The plan:-op scan flags open-coded "plan:" string surgery -- a strings.HasPrefix/TrimPrefix call
// or a string concatenation whose operand is HandlePrefix or a "plan:" literal -- outside
// classify.go, shape.go, and internal/planparser/handle.go (the handle grammar's own declared
// owner, per Decision: exported-surface-placement). No internal/planglyph file is exempt from this
// scan, internal/planglyph/handle.go included: that shared basename is exactly where the invariant
// bites, since a planglyph file re-deriving the "plan:" strip rather than calling into
// planparser's exported handle vocabulary (IsHandleRef/HandleBody/HandleUnit/NewHandle/...) would
// be a second copy of the grammar handle.go already owns.

package planparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// bannedRefKindIdentifiers returns the refKind scan's banned identifier set: every constant
// classify.go's refKind const block declares -- built dynamically via
// refKindConstNamesInDeclarationOrder (shape_test.go), so a fifth refKind is banned automatically
// the moment it is added -- plus the two fixed names "refKind" (the type itself) and "classifyRef"
// (the sole classifier).
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

// enforcementScanTarget is one production .go file under scan: its parsed AST plus its
// repository-relative path, rendered POSIX-style, for exempt-set comparison and failure reporting.
type enforcementScanTarget struct {
	astFile *ast.File
	relPath string
}

// productionGoFiles walks dir (a repository-relative package directory, resolved against
// repoRoot) and returns one enforcementScanTarget per production .go file it contains --
// every *_test.go file is skipped, exactly as the cliwire precedent skips one.
func productionGoFiles(t *testing.T, repoRoot, dir string) []enforcementScanTarget {
	t.Helper()

	var targets []enforcementScanTarget
	absDir := filepath.Join(repoRoot, filepath.FromSlash(dir))
	err := filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			t.Fatalf("relativize %s against %s: %v", path, repoRoot, err)
		}

		targets = append(targets, enforcementScanTarget{astFile: astFile, relPath: filepath.ToSlash(relPath)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", absDir, err)
	}
	return targets
}

// scanTargets resolves the repository root and returns both packages' own production-file targets,
// from runtime.Caller(0), so the scan always runs against the checked-out tree this test file
// itself lives in, never a hard-coded absolute path.
func scanTargets(t *testing.T) []enforcementScanTarget {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine planparser source directory location")
	}
	planparserDir := filepath.Dir(thisFile)
	internalDir := filepath.Dir(planparserDir)
	repoRoot := filepath.Dir(internalDir)

	var targets []enforcementScanTarget
	targets = append(targets, productionGoFiles(t, repoRoot, "internal/planparser")...)
	targets = append(targets, productionGoFiles(t, repoRoot, "internal/planglyph")...)
	return targets
}

// refKindScanExempt is the refKind scan's exempt set: the two files the Ref-Shape Registry
// Invariant names as the registry's own declared home. No internal/planglyph file is ever in this
// set -- planglyph cannot legally reference an unexported planparser identifier at all, so its
// production files are scanned purely as a name-shape check (a local identifier that happens to
// collide with a banned name would still trip it).
var refKindScanExempt = map[string]bool{
	"internal/planparser/classify.go": true,
	"internal/planparser/shape.go":    true,
}

// planOpScanExempt is the plan:-op scan's exempt set: classify.go and shape.go (the registry's own
// home) plus internal/planparser/handle.go, the handle grammar's declared owner
// (Decision: exported-surface-placement). internal/planglyph/handle.go is deliberately NOT in this
// set, despite sharing a basename with the exempt planparser file -- that shared basename is
// exactly where the invariant bites, so planglyph's own handle.go is scanned like any other
// planglyph production file.
var planOpScanExempt = map[string]bool{
	"internal/planparser/classify.go": true,
	"internal/planparser/shape.go":    true,
	"internal/planparser/handle.go":   true,
}

// refKindScanFile returns, for one already-parsed production file, every ast.Ident whose name
// appears in banned -- the refKind scan's matcher. The match is on the AST alone, so a doc comment
// naming classifyRef or a refKind constant can never trip it.
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

// isHandlePrefixIdent reports whether e is a bare reference to the identifier HandlePrefix, either
// unqualified (HandlePrefix) or package-qualified (planparser.HandlePrefix).
func isHandlePrefixIdent(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "HandlePrefix"
	case *ast.SelectorExpr:
		return x.Sel != nil && x.Sel.Name == "HandlePrefix"
	}
	return false
}

// isPlanColonLiteral reports whether e is the string literal "plan:", decoded via strconv.Unquote
// so the comparison is against the literal's actual VALUE rather than its raw quoted source text.
func isPlanColonLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	v, err := strconv.Unquote(lit.Value)
	return err == nil && v == "plan:"
}

// isHandlePrefixOrPlanLiteral reports whether e is either a HandlePrefix reference or a "plan:"
// literal -- the two operand shapes the plan:-op scan bans in operand position of a shape
// operation.
func isHandlePrefixOrPlanLiteral(e ast.Expr) bool {
	return isHandlePrefixIdent(e) || isPlanColonLiteral(e)
}

// isStringsHasPrefixOrTrimPrefixCall reports whether call invokes strings.HasPrefix or
// strings.TrimPrefix -- the two shape operations the plan:-op scan's call rule watches.
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

// planOpScanFile returns, for one already-parsed production file, one description string per
// plan:-op violation -- the plan:-op scan's matcher. It flags two shapes: a strings.HasPrefix or
// strings.TrimPrefix call whose second argument is HandlePrefix or a "plan:" literal, and a "+"
// string concatenation with HandlePrefix or a "plan:" literal as either operand. A "plan:" literal
// outside operand position of one of these two shapes -- including every comment, since comments
// are never AST nodes -- never trips this matcher.
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

// TestRefKindScan_CatchesSeededViolation is the refKind scan's own seeded self-test: a synthetic
// source string carrying a deliberate classifyRef call plus a refKindPath comparison must trip the
// matcher, proving it actually fires before the real-tree assertion below is trusted to mean
// anything.
func TestRefKindScan_CatchesSeededViolation(t *testing.T) {
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

// TestPlanOpScan_CatchesSeededViolation is the plan:-op scan's own seeded self-test: a synthetic
// source string carrying a deliberate strings.HasPrefix(x, "plan:") call must trip the matcher.
func TestPlanOpScan_CatchesSeededViolation(t *testing.T) {
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

// TestRefKindScan_RealTreeClean is the refKind scan's real-tree half: after the classify.go/shape.go
// migration, no production file in internal/planparser or internal/planglyph outside those two
// exempt files may name classifyRef, the refKind type, or one of its declared constants.
func TestRefKindScan_RealTreeClean(t *testing.T) {
	t.Parallel()

	targets := scanTargets(t)
	banned := bannedRefKindIdentifiers(t)

	var failures []string
	for _, target := range targets {
		if refKindScanExempt[target.relPath] {
			continue
		}
		for _, name := range refKindScanFile(target.astFile, banned) {
			failures = append(failures, target.relPath+": "+name)
		}
	}

	if len(failures) > 0 {
		t.Errorf("Ref-Shape Registry Invariant violated: a production file outside classify.go/shape.go "+
			"names classifyRef or a refKind identifier directly, rather than routing through lookup: %v "+
			"(see CONSTRAINTS.md's Ref-Shape Registry Invariant)", failures)
	}
}

// TestPlanOpScan_RealTreeClean is the plan:-op scan's real-tree half: after the classify.go/shape.go
// migration, no production file in internal/planparser or internal/planglyph outside classify.go,
// shape.go, and internal/planparser/handle.go may open-code "plan:" string surgery.
// internal/planglyph/handle.go is scanned like any other planglyph file -- it is NOT exempt, despite
// sharing a basename with the exempt planparser file.
func TestPlanOpScan_RealTreeClean(t *testing.T) {
	t.Parallel()

	targets := scanTargets(t)

	var failures []string
	for _, target := range targets {
		if planOpScanExempt[target.relPath] {
			continue
		}
		for _, hit := range planOpScanFile(target.astFile) {
			failures = append(failures, target.relPath+": "+hit)
		}
	}

	if len(failures) > 0 {
		t.Errorf("Ref-Shape Registry Invariant violated: a production file outside the handle grammar's "+
			"declared owners open-codes \"plan:\" string surgery instead of calling into planparser's "+
			"exported handle vocabulary: %v (see CONSTRAINTS.md's Ref-Shape Registry Invariant)", failures)
	}
}
