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
// it also asserts the shipped rows, rendered over a sample refuse list, pass checkTexts and state the gate rule in one notice, and that the refuse-raw steer carries the full rule while its notice names only the refusal and the tier and card scopes.
func TestCheckTexts(t *testing.T) {
	t.Parallel()
	t.Run("ShippedRefuseRawRow", func(t *testing.T) {
		t.Parallel()
		found := false
		gateRuleNotices := 0
		for _, row := range Rows("with a long timeout", []string{"go test", "dotnet test"}) {
			if err := checkTexts([]Row{row}); err != nil {
				t.Errorf("checkTexts(%q row) = %v; want nil", row.Name, err)
			}
			if strings.Contains(row.Notice, "Gate rule") {
				gateRuleNotices++
			}
			if row.Name != "refuse-raw" {
				continue
			}
			found = true
			for _, want := range []string{"Gate rule", "lym gate test", "--tier", "--card", "with a long timeout", "no gate slot is free"} {
				if !strings.Contains(row.Steer, want) {
					t.Errorf("refuse-raw steer %q: want it to name %q", row.Steer, want)
				}
			}
			wantNotice := "a raw go test or dotnet test run is refused here; run it through lym gate test, with --tier to name the tiers or --card to name a card."
			if row.Notice != wantNotice {
				t.Errorf("refuse-raw notice = %q; want %q", row.Notice, wantNotice)
			}
		}
		if !found {
			t.Error("Rows() with a non-empty refuse list has no refuse-raw row")
		}
		if gateRuleNotices != 1 {
			t.Errorf("Rows() notices state the gate rule %d times; want once", gateRuleNotices)
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
