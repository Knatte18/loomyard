//go:build tier0

// rows_test.go pins the package's import set and the refusal of forbidden characters in a row's steer and notice.

package shellguard

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is the closed set of non-stdlib imports the package admits.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/agentname",
	"github.com/Knatte18/loomyard/internal/gateslot",
}

// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in allowedImports.
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	t.Parallel()
	scankit.AssertImportAllowlist(t, "internal/shellguard", allowedImports...)
}

// TestCheckTexts plants each forbidden character in a row's steer and notice and asserts checkTexts refuses it naming the row;
// it also asserts the shipped refuse-raw row, rendered over a sample list, passes checkTexts and names the gate verb, its tier and card scopes, the foreground phrase and the no-free-slot re-run.
func TestCheckTexts(t *testing.T) {
	t.Parallel()
	t.Run("ShippedRefuseRawRow", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, row := range Rows("with a long timeout", []string{"go test", "dotnet test"}) {
			if err := checkTexts([]Row{row}); err != nil {
				t.Errorf("checkTexts(%q row) = %v; want nil", row.Name, err)
			}
			if row.Name != "refuse-raw" {
				continue
			}
			found = true
			for _, want := range []string{"lym gate test", "--tier", "--card", "with a long timeout", "no gate slot is free"} {
				if !strings.Contains(row.Steer, want) || !strings.Contains(row.Notice, want) {
					t.Errorf("refuse-raw steer %q and notice %q: want both to name %q", row.Steer, row.Notice, want)
				}
			}
		}
		if !found {
			t.Error("Rows() with a non-empty refuse list has no refuse-raw row")
		}
	})
	tests := []struct {
		name   string
		steer  string
		notice string
	}{
		{"SteerSingleQuote", "it's", ""},
		{"SteerDoubleQuote", `say "x"`, ""},
		{"SteerBackslash", `a\b`, ""},
		{"NoticeSingleQuote", "", "it's"},
		{"NoticeDoubleQuote", "", `say "x"`},
		{"NoticeBackslash", "", `a\b`},
		{"NoticeLineBreak", "", "one\ntwo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkTexts([]Row{{Name: "planted", Steer: tt.steer, Notice: tt.notice}})
			if err == nil || !strings.Contains(err.Error(), `"planted"`) {
				t.Errorf("checkTexts() = %v; want an error naming the row", err)
			}
		})
	}
}
