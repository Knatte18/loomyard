// llmtier_test.go enforces the LLM-tier half of `PATTERN-test-speed`: only `llm` test files reach an LLM, through internal/testkit/llmkit.
// The guard sees the static shape only: a `tmux`-tier test that drives `lyx` into spawning an LLM without calling llmkit is caught by review.

package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	llmkitImportPath = "github.com/Knatte18/loomyard/internal/testkit/llmkit"
	// llmkitDir is the kit's own directory, exempt from the import and LookPath rules and bound to `exec.LookPath` alone.
	llmkitDir = "internal/testkit/llmkit"
)

// llmBinaryNames are the LLM binary names a test file outside the kit may not look up on PATH.
var llmBinaryNames = []string{"claude"}

// allowedLLMTierScanData is the LLM-tier guard allowlist: module-relative file paths, or directory paths ending in "/", exempt from every rule, each with a reason.
var allowedLLMTierScanData = []scankit.Entry{
	{Key: "cmd/lyx/llmtier_test.go", Why: "contains the llmkit import path and `exec.LookPath` call shapes as its own scan data"},
}

// importsPath reports whether the file imports the path.
func importsPath(file *ast.File, path string) bool {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == path {
			return true
		}
	}
	return false
}

// stringConsts returns the file's package-level string constants declared as literals.
func stringConsts(file *ast.File) map[string]string {
	consts := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					consts[name.Name] = s
				}
			}
		}
	}
	return consts
}

// lookPathLLMBinary returns the LLM binary name a `exec.LookPath` call in the file passes as a string literal or a same-file constant, or "" when there is none.
func lookPathLLMBinary(file *ast.File) string {
	execName, ok := execImportName(file)
	if !ok {
		return ""
	}
	consts := stringConsts(file)
	found := ""
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "LookPath" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != execName {
			return true
		}
		name := ""
		switch arg := call.Args[0].(type) {
		case *ast.BasicLit:
			name, _ = strconv.Unquote(arg.Value)
		case *ast.Ident:
			name = consts[arg.Name]
		}
		for _, bin := range llmBinaryNames {
			if name == bin && found == "" {
				found = name
			}
		}
		return true
	})
	return found
}

// nonLookPathExecUse returns the first identifier of the file's `os/exec` import other than LookPath that the file uses, or "" when there is none.
func nonLookPathExecUse(file *ast.File) string {
	execName, ok := execImportName(file)
	if !ok {
		return ""
	}
	used := ""
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == execName && sel.Sel.Name != "LookPath" && used == "" {
			used = sel.Sel.Name
		}
		return true
	})
	return used
}

// canCompileWithoutLLM reports whether the constraint can be true with `llm` unset and every other tag and platform term set.
func canCompileWithoutLLM(expr constraint.Expr) bool {
	if expr == nil {
		return true
	}
	return expr.Eval(func(tag string) bool { return tag != "llm" })
}

// llmTierFailures walks the Go files under opts and returns the violating files' failure lines and the number of files scanned.
func llmTierFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, scanned int) {
	t.Helper()
	opts.Filter = scankit.All
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		inKit := strings.HasPrefix(f.Rel, llmkitDir+"/")
		isTest := strings.HasSuffix(f.Rel, "_test.go")
		if !inKit && !isTest {
			return
		}
		file := f.AST(t, parser.ParseComments|parser.SkipObjectResolution)
		if inKit {
			if used := nonLookPathExecUse(file); used != "" {
				failures = append(failures, fmt.Sprintf("%s: uses exec.%s; llmkit's `os/exec` exemption is bounded to exec.LookPath", f.Rel, used))
			}
			return
		}
		if importsPath(file, llmkitImportPath) && canCompileWithoutLLM(buildConstraint(file)) {
			failures = append(failures, fmt.Sprintf("%s: imports llmkit but can compile without the `llm` tag; constrain it to `//go:build llm`", f.Rel))
		}
		if bin := lookPathLLMBinary(file); bin != "" {
			failures = append(failures, fmt.Sprintf("%s: looks up %q with exec.LookPath outside llmkit; call llmkit.Claude from an `llm` file", f.Rel, bin))
		}
	})
	sort.Strings(failures)
	return failures, scanned
}

// TestLLMTier_OnlyLLMFilesReachAnLLM fails for a test file that imports llmkit without being confined to the `llm` tag, for a test file outside the kit that looks up an LLM binary, and for a kit file using `os/exec` beyond LookPath.
func TestLLMTier_OnlyLLMFilesReachAnLLM(t *testing.T) {
	allow := scankit.NewAllowlist(allowedLLMTierScanData)
	failures, scanned := llmTierFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 20, "llm tier guard")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestLLMTier_FixtureTrees(t *testing.T) {
	const importsKit = "package p\n\nimport _ \"github.com/Knatte18/loomyard/internal/testkit/llmkit\"\n"
	const lookPathClaude = "package p\n\nimport \"os/exec\"\n\nfunc f() { _, _ = exec.LookPath(\"claude\") }\n"
	const lookPathConst = "package p\n\nimport \"os/exec\"\n\nconst bin = \"claude\"\n\nfunc f() { _, _ = exec.LookPath(bin) }\n"
	const lookPathTmux = "package p\n\nimport \"os/exec\"\n\nfunc f() { _, _ = exec.LookPath(\"tmux\") }\n"
	const kitCommand = "package llmkit\n\nimport \"os/exec\"\n\nfunc f() { _ = exec.Command(\"claude\") }\n"
	const kitLookPath = "package llmkit\n\nimport \"os/exec\"\n\nfunc f() { _, _ = exec.LookPath(\"claude\") }\n"

	cases := []struct {
		name     string
		files    map[string]string
		wantFail int
	}{
		{"untagged file imports llmkit", map[string]string{"p/a_test.go": importsKit}, 1},
		{"tmux or llm file imports llmkit", map[string]string{"p/a_test.go": "//go:build tmux || llm\n\n" + importsKit}, 1},
		{"integration file imports llmkit", map[string]string{"p/a_test.go": "//go:build integration\n\n" + importsKit}, 1},
		{"llm file imports llmkit", map[string]string{"p/a_test.go": "//go:build llm\n\n" + importsKit}, 0},
		{"llm and platform file imports llmkit", map[string]string{"p/a_test.go": "//go:build llm && linux\n\n" + importsKit}, 0},
		{"integration file looks up claude", map[string]string{"p/a_test.go": "//go:build integration\n\n" + lookPathClaude}, 1},
		{"integration file looks up claude through a constant", map[string]string{"p/a_test.go": "//go:build integration\n\n" + lookPathConst}, 1},
		{"integration file looks up tmux", map[string]string{"p/a_test.go": "//go:build integration\n\n" + lookPathTmux}, 0},
		{"kit file runs a command", map[string]string{"internal/testkit/llmkit/llmkit.go": kitCommand}, 1},
		{"kit file only looks up a path", map[string]string{"internal/testkit/llmkit/llmkit.go": kitLookPath}, 0},
		{"kit test file looks up claude", map[string]string{"internal/testkit/llmkit/llmkit_test.go": lookPathClaude}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			for rel, body := range tc.files {
				path := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			failures, _ := llmTierFailures(t, scankit.Options{Base: base}, scankit.NewAllowlist(nil))
			if len(failures) != tc.wantFail {
				t.Fatalf("failures = %v; want %d failure(s)", failures, tc.wantFail)
			}
		})
	}
}
