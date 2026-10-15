// seam_enforcement_test.go enforces `PATTERN-creel-store`'s import half: production code in this package never imports reed or shuttle, so every keystroke goes through the Delivery seam.

package creelengine

import (
	"fmt"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	reedImportPath    = "github.com/Knatte18/loomyard/internal/reedengine"
	shuttleImportPath = "github.com/Knatte18/loomyard/internal/shuttleengine"
)

// deliverySeamImportFindings parses data's imports and returns one message per import of reedengine, of shuttleengine or of any path under it.
func deliverySeamImportFindings(filename string, data []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var findings []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if path == reedImportPath || path == shuttleImportPath || strings.HasPrefix(path, shuttleImportPath+"/") {
			findings = append(findings, fmt.Sprintf("%s: imports %s", fset.Position(spec.Pos()), path))
		}
	}
	return findings, nil
}

// TestCreelStore_DeliverySeamImports enforces that internal/creelengine reaches a pane only through its Delivery seam.
func TestCreelStore_DeliverySeamImports(t *testing.T) {
	t.Run("detector", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			path string
		}{
			{"reedengine", reedImportPath},
			{"a package under shuttleengine", shuttleImportPath + "/claudeengine"},
			{"shuttleengine itself", shuttleImportPath},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				findings, err := deliverySeamImportFindings("synthetic.go", []byte("package p\n\nimport _ \""+tt.path+"\"\n"))
				if err != nil {
					t.Fatal(err)
				}
				if len(findings) != 1 || !strings.Contains(findings[0], tt.path) {
					t.Errorf("findings = %q; want one naming %s", findings, tt.path)
				}
			})
		}
	})

	t.Run("production files import neither reed nor shuttle", func(t *testing.T) {
		t.Parallel()
		var failures []string
		scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/creelengine"}, Shallow: true}, func(f *scankit.File) {
			findings, err := deliverySeamImportFindings(f.Rel, f.Data)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", f.Rel, err))
				return
			}
			failures = append(failures, findings...)
		})
		scankit.RequireFloor(t, scanned, 1, "creel delivery seam scan")
		if len(failures) > 0 {
			t.Errorf("`PATTERN-creel-store` violated; reach a pane through the Delivery seam:\n%s", strings.Join(failures, "\n"))
		}
	})
}
