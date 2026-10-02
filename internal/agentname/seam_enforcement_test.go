// seam_enforcement_test.go enforces the Agent Name Invariant's leaf clause:
// production code in internal/agentname imports the standard library only.

package agentname

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentNameInvariant_StdlibOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine agentname source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures []string
	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		astFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			firstSegment, _, _ := strings.Cut(importPath, "/")
			if strings.Contains(firstSegment, ".") {
				rel, _ := filepath.Rel(pkgDir, path)
				failures = append(failures, rel+": "+importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk agentname directory: %v", err)
	}
	if len(failures) > 0 {
		t.Errorf("Agent Name Invariant violated; non-stdlib imports found: %v", failures)
	}
}
