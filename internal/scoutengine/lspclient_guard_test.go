// lspclient_guard_test.go is a narrow file-scoped guard on internal/scoutengine/lspclient.go: the
// ported stdio LSP client carries no lyx dependency except logging, keeping it liftable back out of
// lyx behind a single logging dependency.
// It guards this one file only — it must never be generalized into a per-file allowed-set table,
// which would be an allowlist through the back door.

package scoutengine

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLSPClientGuard_StdlibAndLoggerOnly verifies that
// internal/scoutengine/lspclient.go imports only the standard library plus
// internal/logger — no lyx dependency except logging.
func TestLSPClientGuard_StdlibAndLoggerOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine scoutengine source directory location")
	}
	pkgDir := filepath.Dir(file)
	target := filepath.Join(pkgDir, "lspclient.go")

	if _, err := os.Stat(target); err != nil {
		t.Fatalf("lspclient.go not found at %s; the guard's hardcoded target path must track the file: %v", target, err)
	}

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, target, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse lspclient.go: %v", err)
	}

	const allowedLyxImport = "github.com/Knatte18/loomyard/internal/logger"

	var failures []string

	for _, imp := range astFile.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		// A stdlib import path has no '.' in its first path segment
		// (e.g. "fmt", "os", "go/parser") — a domain that would need a
		// registered TLD (e.g. "github.com/...") always contains one.
		firstSegment := importPath
		if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
			firstSegment = importPath[:idx]
		}
		isStdlib := !strings.Contains(firstSegment, ".")

		if isStdlib || importPath == allowedLyxImport {
			continue
		}

		failures = append(failures, importPath)
	}

	if len(failures) > 0 {
		t.Errorf("lspclient.go must carry no lyx dependency except logging; found: %v", failures)
	}
}
