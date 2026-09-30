// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership and its
// Shuttle Provider-Seam Invariant: production code in internal/orchengine takes every absolute
// path it operates on from its caller, resolves no geometry, and never reaches provider specifics.
//
// The allowlist is a membership list rather than a bare denylist, mirroring internal/battenshed:
// it catches the excluded imports and anything else that would drag them in.

package orchengine

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// orchengineAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var orchengineAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/configengine":  true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
	"github.com/Knatte18/loomyard/internal/state":         true,
	"github.com/Knatte18/loomyard/internal/lock":          true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
	"github.com/Knatte18/loomyard/internal/stencil":       true,
	"github.com/Knatte18/loomyard/internal/stencilstore":  true,
	"gopkg.in/yaml.v3": true,
}

const (
	// orchengineDeniedLyxcwdImport is excluded by the Told-Geometry Invariant.
	orchengineDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"
	// orchengineDeniedClaudeImport is excluded by the Shuttle Provider-Seam Invariant.
	orchengineDeniedClaudeImport = "github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// TestSeamInvariants_AllowlistOnly verifies that every non-test .go file in this package imports
// only stdlib or an entry in orchengineAllowedImports, and separately names the two denied imports
// so a violation reports the rule it breaks.
func TestSeamInvariants_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine orchengine source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures, lyxcwdFound, claudeFound []string

	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(pkgDir, path)
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			switch importPath {
			case orchengineDeniedLyxcwdImport:
				lyxcwdFound = append(lyxcwdFound, relPath)
			case orchengineDeniedClaudeImport:
				claudeFound = append(claudeFound, relPath)
			}

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || orchengineAllowedImports[importPath] {
				continue
			}
			failures = append(failures, relPath+": "+importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk orchengine directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("import allowlist violated; imports outside the allowlist found: %v", failures)
	}
	if len(lyxcwdFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", orchengineDeniedLyxcwdImport, lyxcwdFound)
	}
	if len(claudeFound) > 0 {
		t.Errorf("Shuttle Provider-Seam Invariant violated; %s imported directly in: %v", orchengineDeniedClaudeImport, claudeFound)
	}
}
