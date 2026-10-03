// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership and its Shuttle Provider-Seam Invariant: production code in internal/orchengine takes every absolute path it operates on from its caller, resolves no geometry, and never reaches provider specifics.
//
// The allowlist is a membership list rather than a bare denylist, mirroring internal/battenshed:
// it catches the excluded imports and anything else that would drag them in.

package orchengine

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// orchengineAllowedImports are the only non-stdlib import paths production code in this package may use.
var orchengineAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"gopkg.in/yaml.v3",
}

const (
	// orchengineDeniedLyxcwdImport is excluded by the Told-Geometry Invariant.
	orchengineDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"
	// orchengineDeniedClaudeImport is excluded by the Shuttle Provider-Seam Invariant.
	orchengineDeniedClaudeImport = "github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// TestSeamInvariants_AllowlistOnly verifies that every non-test .go file in this package imports only stdlib or an entry in orchengineAllowedImports,
// and it separately names the two denied imports so a violation reports the rule it breaks.
func TestSeamInvariants_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/orchengine", orchengineAllowedImports...)

	var lyxcwdFound, claudeFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/orchengine"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			switch strings.Trim(imp.Path.Value, `"`) {
			case orchengineDeniedLyxcwdImport:
				lyxcwdFound = append(lyxcwdFound, f.Rel)
			case orchengineDeniedClaudeImport:
				claudeFound = append(claudeFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "orchengine denied-import scan")
	if len(lyxcwdFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", orchengineDeniedLyxcwdImport, lyxcwdFound)
	}
	if len(claudeFound) > 0 {
		t.Errorf("Shuttle Provider-Seam Invariant violated; %s imported directly in: %v", orchengineDeniedClaudeImport, claudeFound)
	}
}
