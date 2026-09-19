// selvagepane_enforcement_test.go enforces the Selvage-confinement rule this batch's extraction
// exists to satisfy: every identifier naming Selvage lives in selvagepane.go, with two narrow
// carve-outs elsewhere. selvagepane.go owns the whole Selvage seam; state.go is allowed only for the
// SelvagePaneID field declaration ReedState carries (state.go's subject is the persisted record in
// general, not Selvage specifically); config.go is allowed only for SelvageConfig and Config.Selvage,
// the resolved-config plumbing every module's config file carries for its own knobs.
//
// The match is case-insensitive on purpose: a case-sensitive rule would silently permit exactly the
// selvagePaneID-shaped parameters and locals this extraction exists to eliminate from the four host
// files, since Go identifiers conventionally start a local or parameter with a lowercase letter.
//
// Honest residual: the call-position exemption below covers same-package calls only, so a
// Selvage-named function exported from another package would be exempt at its call site and
// unscanned at its declaration. That is latent rather than open today -- internal/reedengine/render/
// exports the type Selvage (caught here as a composite-literal type, since a type reference is never
// a call's Fun) and no Selvage-named function, so nothing currently exploits the gap.
//
// This file introduces the file-level allowlist pattern to the repository: no existing enforcement
// test here uses one. The nearest precedent is internal/gitkit/callerset_enforcement_test.go's single
// allowed-directory const -- the same allow shape, at coarser (directory rather than file) granularity.
// internal/cliwire/bannedecl_enforcement_test.go is cited only for what it actually supplies: the AST
// walk, the runtime.Caller(0) repository-root resolution, and the house style of recording a residual
// in a doc comment -- its own policy is a ban-list, the inverse of an allowlist, so it is not itself an
// allowlist precedent.

package reedengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// selvagePaneAllowlist names the files in this package permitted to carry a Selvage-naming
// identifier outside a call position or composite-literal key.
var selvagePaneAllowlist = map[string]bool{
	"selvagepane.go": true,
	"state.go":       true,
	"config.go":      true,
}

// TestSelvageIdentifiersConfinedToSelvagePane verifies that no file in internal/reedengine, other
// than the three named in selvagePaneAllowlist, declares or references a Selvage-naming identifier
// outside the two exemptions selvageIdentViolations knows about.
// It spawns no process, so it carries no build tag.
// It resolves the repository root from runtime.Caller(0) exactly as the two precedent enforcement
// tests do, then parses every non-_test.go .go file directly inside internal/reedengine -- never
// internal/reedengine/render/, and never any sibling package.
func TestSelvageIdentifiersConfinedToSelvagePane(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine reedengine source directory location")
	}
	reedengineDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(reedengineDir)) // internal/reedengine -> internal -> repo root
	scanDir := filepath.Join(repoRoot, "internal", "reedengine")

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		t.Fatalf("read %s: %v", scanDir, err)
	}

	var failures []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if selvagePaneAllowlist[name] {
			continue
		}

		path := filepath.Join(scanDir, name)
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		for _, violation := range selvageIdentViolations(fset, astFile) {
			failures = append(failures, name+" "+violation)
		}
	}

	if len(failures) > 0 {
		t.Errorf("Selvage-confinement check violated: %d identifier(s) outside selvagepane.go/state.go/config.go carry \"selvage\" (case-insensitive): %v -- move the code owning them into selvagepane.go", len(failures), failures)
	}
}

// TestSelvageIdentViolations_ExemptionsAndFlags drives selvageIdentViolations directly over
// synthetic source parsed from an in-memory string, rather than from disk, so the two exemptions stay
// verified rather than assumed and keep being verified after the extraction lands.
func TestSelvageIdentViolations_ExemptionsAndFlags(t *testing.T) {
	const src = `package fake

func example(e *engine, st *state, selvagePaneID string) {
	_ = render.Selvage{}
	_ = e.cfg.Selvage
	_ = st.SelvagePaneID
	_ = render.Params{Selvage: x}
	e.ensureSelvagePaneLocked(st)
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fake.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := selvageIdentViolations(fset, astFile)

	countSuffix := func(suffix string) int {
		n := 0
		for _, v := range got {
			if strings.HasSuffix(v, suffix) {
				n++
			}
		}
		return n
	}

	// Flagged: the render.Selvage{} composite-literal TYPE and the e.cfg.Selvage selector each end
	// the violation string in ": Selvage" -- two occurrences, neither of them the composite-literal
	// KEY in render.Params{Selvage: x}, which selveageIdentViolations must exempt.
	if n := countSuffix(": Selvage"); n != 2 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 2 violations ending \": Selvage\" (the render.Selvage{} composite-literal type and the e.cfg.Selvage selector), got %d", got, n)
	}
	if n := countSuffix(": SelvagePaneID"); n != 1 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 1 violation ending \": SelvagePaneID\" (the st.SelvagePaneID selector), got %d", got, n)
	}
	if n := countSuffix(": selvagePaneID"); n != 1 {
		t.Errorf("selvageIdentViolations() = %v; want exactly 1 violation ending \": selvagePaneID\" (the parameter), got %d", got, n)
	}
	if len(got) != 4 {
		t.Errorf("selvageIdentViolations() = %v (%d violations); want exactly 4 -- the render.Params{Selvage: x} composite-literal KEY and the e.ensureSelvagePaneLocked(st) call-position identifier must both be exempt", got, len(got))
	}
}

// selvageIdentViolations returns one formatted "position: identifier" string per *ast.Ident in
// astFile whose strings.ToLower form contains "selvage", except for two exemptions.
//
// Exemption (i) is the function position of a call expression: the Fun of an *ast.CallExpr, whether
// a bare f(...) identifier or the Sel of an x.f(...) selector. A host file is allowed to CALL a
// Selvage-named helper, since that is exactly what this extraction leaves behind at every host call
// site.
//
// Exemption (ii) is a composite-literal field key: the Key of a *ast.KeyValueExpr appearing inside a
// *ast.CompositeLit. A host file is allowed to populate a struct field literally named Selvage (e.g.
// render.Params{Selvage: x}), since the field's own declaration -- not its use as a key -- is what
// carries the name.
//
// Everything else is flagged: field selectors, composite-literal types, func/method/type/var
// declarations, struct field declarations, parameters and locals. Comments and string literals are
// never read, so a package-doc paragraph naming Selvage and a `yaml:"selvage"` struct tag both pass
// untouched -- go/parser does not turn either into an *ast.Ident.
func selvageIdentViolations(fset *token.FileSet, astFile *ast.File) []string {
	exempt := make(map[*ast.Ident]bool)

	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			switch fun := node.Fun.(type) {
			case *ast.Ident:
				exempt[fun] = true
			case *ast.SelectorExpr:
				exempt[fun.Sel] = true
			}
		case *ast.CompositeLit:
			for _, elt := range node.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					exempt[key] = true
				}
			}
		}
		return true
	})

	var violations []string
	ast.Inspect(astFile, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if exempt[ident] {
			return true
		}
		if !strings.Contains(strings.ToLower(ident.Name), "selvage") {
			return true
		}
		violations = append(violations, fset.Position(ident.Pos()).String()+": "+ident.Name)
		return true
	})

	return violations
}
