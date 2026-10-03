// seam_enforcement_test.go enforces the Shared Decision "provider-seam import rule":
// internal/shuttleengine must never import internal/shuttleengine/claudeengine.
// The interface (Engine) and its value types live here;
// claudeengine imports shuttleengine and implements the interface, never the reverse — this is what
// lets a second provider engine be added without ever touching this package.

package shuttleengine

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// TestProviderSeamImportRule verifies that no non-test file in internal/shuttleengine imports
// internal/shuttleengine/claudeengine.
// It reads actual import paths through the harness parse (avoiding false positives from string literals in
// doc comments), in the style of internal/gitkit/leaf_enforcement_test.go's TestLeafInvariant_AllowlistOnly.
func TestProviderSeamImportRule(t *testing.T) {
	const bannedImport = "github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"

	var failures []string

	// Shallow, so the scan matches the rule's scope: the seam package, not subpackages like
	// claudeengine itself.
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shuttleengine"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == bannedImport {
				failures = append(failures, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shuttleengine provider-seam scan")

	if len(failures) > 0 {
		t.Errorf("provider-seam import rule violated (Shared Decision \"provider-seam import rule\"): "+
			"internal/shuttleengine must never import internal/shuttleengine/claudeengine, but found it imported in: %v", failures)
	}
}
