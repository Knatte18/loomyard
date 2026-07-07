// enforcement_test.go is a repo-wide guard: it walks every package and fails
// the build if any file outside internal/hubgeometry reaches for raw cwd or top-level
// git geometry, keeping internal/hubgeometry the sole geometry owner.

package hubgeometry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestEnforcement walks the repo source tree and verifies that no source file
// outside internal/hubgeometry and cmd/lyx contains the raw cwd/root primitives
// os.Getwd or git rev-parse --show-toplevel.
func TestEnforcement(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Resolve repo root relative to this test file.
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("could not determine test file location")
		}
		// Two levels up from internal/hubgeometry/enforcement_test.go → repo root
		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))

		// Predicate: returns true if the bytes contain a banned token.
		isBanned := func(data []byte) bool {
			content := string(data)
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		var failures []string

		// Walk the entire tree.
		err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Skip .git, testdata, and _test.go files.
			if d.IsDir() && (d.Name() == ".git" || strings.Contains(d.Name(), "testdata")) {
				return filepath.SkipDir
			}
			if strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			// Only check .go files.
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
				// Determine the package-relative directory.
				relPath, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return err
				}
				pkgDir := filepath.Dir(relPath)

				// Normalize path separators to forward slashes for comparison.
				pkgDir = filepath.ToSlash(pkgDir)

				// Check allowlist: internal/hubgeometry, cmd/lyx/main.go
				isAllowed := pkgDir == "internal/hubgeometry" ||
					(pkgDir == "cmd/lyx" && d.Name() == "main.go")

				// Skip files in the allowlist (they are allowed to contain banned tokens).
				if isAllowed {
					return nil
				}

				// Check the file for banned tokens.
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if isBanned(data) {
					failures = append(failures, relPath)
				}
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk repo tree: %v", err)
		}

		if len(failures) > 0 {
			t.Errorf("found banned tokens in files: %v", failures)
		}
	})

	// Sub-test: verify the predicate itself on synthetic snippets.
	t.Run("predicate", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
			want    bool
		}{
			{
				name:    "os.Getwd",
				content: "x := os.Getwd()",
				want:    true,
			},
			{
				name:    "--show-toplevel",
				content: `git rev-parse --show-toplevel`,
				want:    true,
			},
			{
				name:    "clean",
				content: "fmt.Println(hello)",
				want:    false,
			},
		}

		isBanned := func(content string) bool {
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := isBanned(tt.content)
				if got != tt.want {
					t.Errorf("isBanned(%q) = %v, want %v", tt.content, got, tt.want)
				}
			})
		}
	})
}

// TestEnforcement_GeometryLiterals walks the repo source tree and verifies that no
// production file outside internal/hubgeometry constructs a geometry path token as a string
// literal in a path-construction context: a filepath.Join argument, a binary +
// operand, or a string const declaration value. Whole-token matching (exact equality,
// not substring) avoids false positives on compound names such as "_boardroom" or
// "-weft-bare". Test files (*_test.go) are excluded because test geometry is a review
// rule, not a machine-enforced invariant.
func TestEnforcement_GeometryLiterals(t *testing.T) {
	// geometryToken reports whether s is exactly one of the forbidden geometry path
	// tokens. Only internal/hubgeometry is permitted to use these in path-construction context.
	geometryToken := func(s string) bool {
		switch s {
		case "_board", "-weft", "-HUB", "_portals", "_launchers", "_raddle", "_lyx":
			return true
		}
		return false
	}

	// hasGeometryLiteralInConstructionContext reports whether the parsed AST file
	// contains a string literal whose unquoted value equals a geometry token and that
	// appears in a path-construction context:
	//   (a) an argument to filepath.Join(...)
	//   (b) an operand of a binary + expression (token.ADD)
	//   (c) the value of a string const declaration
	// Whole-token matching is enforced via exact equality after strconv.Unquote.
	hasGeometryLiteralInConstructionContext := func(f *ast.File) bool {
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			if found {
				return false
			}
			switch node := n.(type) {
			case *ast.CallExpr:
				// Context (a): filepath.Join(...) argument.
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Join" {
					break
				}
				ident, ok2 := sel.X.(*ast.Ident)
				if !ok2 || ident.Name != "filepath" {
					break
				}
				for _, arg := range node.Args {
					lit, ok3 := arg.(*ast.BasicLit)
					if !ok3 || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						found = true
						return false
					}
				}
			case *ast.BinaryExpr:
				// Context (b): binary + operand.
				if node.Op != token.ADD {
					break
				}
				for _, operand := range []ast.Expr{node.X, node.Y} {
					lit, ok := operand.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						found = true
						return false
					}
				}
			case *ast.GenDecl:
				// Context (c): string const declaration.
				if node.Tok != token.CONST {
					break
				}
				for _, spec := range node.Specs {
					valSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, val := range valSpec.Values {
						lit, ok2 := val.(*ast.BasicLit)
						if !ok2 || lit.Kind != token.STRING {
							continue
						}
						v, err := strconv.Unquote(lit.Value)
						if err == nil && geometryToken(v) {
							found = true
							return false
						}
					}
				}
			}
			return true
		})
		return found
	}

	// predicate sub-test: validates the AST detector against synthetic Go snippets
	// parsed with go/parser. Positives must be detected; negatives must not.
	t.Run("predicate", func(t *testing.T) {
		positives := []struct {
			name string
			src  string
		}{
			{
				name: "filepath.Join_arg_board",
				src:  `package p; import "path/filepath"; var _ = filepath.Join(x, "_board")`,
			},
			{
				name: "add_operand_weft",
				src:  `package p; var _ = slug + "-weft"`,
			},
			{
				name: "const_HUB",
				src:  `package p; const s = "-HUB"`,
			},
		}
		for _, tt := range positives {
			t.Run(tt.name, func(t *testing.T) {
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, "<fixture>", tt.src, parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("parse positive fixture: %v", err)
				}
				if !hasGeometryLiteralInConstructionContext(f) {
					t.Errorf("geometry literal was not detected in positive fixture:\n%s", tt.src)
				}
			})
		}

		negatives := []struct {
			name string
			src  string
		}{
			{
				name: "doc_comment_weft",
				// A comment is not an AST expression node and must never be flagged.
				src: "// Package p discusses the -weft sibling directory.\npackage p",
			},
			{
				name: "struct_field_long_weft",
				// A struct-literal field value is not a construction context.
				src: "package p\n\nvar _ = struct{ Long string }{Long: \"-weft\"}",
			},
			{
				name: "plain_non_token_string",
				// A string that does not equal any geometry token must not be flagged.
				src: `package p; var _ = "not-a-geometry-token"`,
			},
			{
				name: "add_near_token_weft_bare",
				// "-weft-bare" ≠ "-weft"; whole-token matching must reject the compound name.
				src: `package p; var _ = slug + "-weft-bare"`,
			},
			{
				name: "filepath.Join_near_token_boardroom",
				// "_boardroom" ≠ "_board"; whole-token matching must reject the compound name.
				src: `package p; import "path/filepath"; var _ = filepath.Join(x, "_boardroom")`,
			},
		}
		for _, tt := range negatives {
			t.Run(tt.name, func(t *testing.T) {
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, "<fixture>", tt.src, parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("parse negative fixture: %v", err)
				}
				if hasGeometryLiteralInConstructionContext(f) {
					t.Errorf("geometry literal was falsely detected in negative fixture:\n%s", tt.src)
				}
			})
		}
	})

	// tree-scan sub-test: walks every production Go file in the repo (excluding test
	// files and the internal/hubgeometry allowlist) and fails if any file constructs a
	// geometry token in a path context.
	t.Run("tree-scan", func(t *testing.T) {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("could not determine test file location via runtime.Caller")
		}
		// Two filepath.Dir calls walk from internal/hubgeometry/enforcement_test.go → repo root.
		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

		var scanned int
		var failures []string

		err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Skip .git and testdata directories to avoid noise.
			if d.IsDir() && (d.Name() == ".git" || strings.Contains(d.Name(), "testdata")) {
				return filepath.SkipDir
			}
			// Only scan production Go files; test files (*_test.go) are excluded because
			// test geometry is a review-only rule, not a machine-enforced invariant.
			if d.IsDir() || strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}

			relPath, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			// Allowlist: internal/hubgeometry is the sole permitted owner of geometry literals
			// in path-construction context.
			if filepath.ToSlash(filepath.Dir(relPath)) == "internal/hubgeometry" {
				return nil
			}

			fset := token.NewFileSet()
			f, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				// Skip files that cannot be parsed (e.g. build-tag-guarded platform files).
				return nil
			}
			scanned++
			if hasGeometryLiteralInConstructionContext(f) {
				failures = append(failures, filepath.ToSlash(relPath))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk repo tree: %v", err)
		}

		// Sanity check: at least one production file outside internal/hubgeometry must have
		// been scanned so a misconfigured walk (wrong root, all files skipped) cannot
		// silently produce a vacuous all-pass result.
		t.Run("scanned_non_empty", func(t *testing.T) {
			if scanned == 0 {
				t.Error("geometry-literal guard: no production Go files scanned outside internal/hubgeometry; the AST walk may be misconfigured")
			}
		})

		if len(failures) > 0 {
			t.Errorf("geometry-literal construction found outside internal/hubgeometry in:\n%v", failures)
		}
	})
}
