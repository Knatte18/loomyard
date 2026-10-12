// tolerantrecords_test.go enforces `PATTERN-tolerant-records`: no production file calls encoding/json's DisallowUnknownFields, so no reader of a Go-written JSON record refuses a key a newer binary added.
// The scan sees the method name at a call site only; yaml.v3's KnownFields is outside it.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// tolerantRecordsMinScannedFiles is the floor on production files the walk must visit, so a misconfigured walk fails loudly rather than passing on a near-empty scan.
const tolerantRecordsMinScannedFiles = 200

// strictDecodeFindings parses data as Go and returns one message per call expression whose selector is DisallowUnknownFields, with its position.
// A comment or a string that names the method is never a finding.
func strictDecodeFindings(filename string, data []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, 0)
	if err != nil {
		return nil, err
	}

	var findings []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "DisallowUnknownFields" {
			findings = append(findings, fmt.Sprintf("%s: calls DisallowUnknownFields", fset.Position(call.Pos())))
		}
		return true
	})
	return findings, nil
}

// TestTolerantRecords_NoStrictJSONDecode enforces `PATTERN-tolerant-records`.
//
//lyx:guard
func TestTolerantRecords_NoStrictJSONDecode(t *testing.T) {
	t.Run("detector", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			src  string
			want int
		}{
			{"strict decoder call", "func f(r io.Reader) { json.NewDecoder(r).DisallowUnknownFields() }\n", 1},
			{"named only in a comment", "// DisallowUnknownFields is not called here.\nfunc f() {}\n", 0},
			{"named only in a string", "var s = \"DisallowUnknownFields\"\n", 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				findings, err := strictDecodeFindings(tt.name+".go", []byte("package p\n\nimport (\n\t\"encoding/json\"\n\t\"io\"\n)\n\nvar _ = json.Valid\nvar _ io.Reader\n\n"+tt.src))
				if err != nil {
					t.Fatal(err)
				}
				if len(findings) != tt.want {
					t.Errorf("findings = %q; want %d", findings, tt.want)
				}
			})
		}
	})

	t.Run("production files decode records tolerantly", func(t *testing.T) {
		t.Parallel()
		var failures []string
		scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
			findings, err := strictDecodeFindings(f.Rel, f.Data)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", f.Rel, err))
				return
			}
			for _, finding := range findings {
				failures = append(failures, finding+"; decode tolerantly, and rewrite the record through internal/state")
			}
		})
		scankit.RequireFloor(t, scanned, tolerantRecordsMinScannedFiles, "tolerant-records guard")

		sort.Strings(failures)
		if len(failures) > 0 {
			t.Errorf("`PATTERN-tolerant-records` violated:\n%s", strings.Join(failures, "\n"))
		}
	})
}
