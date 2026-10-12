// leaf_enforcement_test.go enforces the halt note's sole-parser invariant: production code in internal/haltnote imports the standard library and nothing else,
// and no production file outside it spells the halt note's title.

package haltnote

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/haltnote", allowedImports...)
}

func TestSoleParser_NoOtherProductionFileSpellsTheNoteTitle(t *testing.T) {
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal"}, Filter: scankit.Prod}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/haltnote/") {
			return
		}
		if strings.Contains(string(f.Data), `"loom halted "`) {
			t.Errorf("%s spells the halt note title; only internal/haltnote renders it", f.Rel)
		}
	})
	scankit.RequireFloor(t, scanned, 100, "production files under internal/")
}
