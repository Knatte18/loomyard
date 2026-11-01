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
// it also asserts the shipped rows, rendered over a sample refuse list, pass checkTexts and state the gate rule and the edit rule in one notice each, that the refuse-raw steer carries the full gate rule while its notice names only the refusal and the tier and card scopes, that the python and shell-edit steers carry the full edit rule, and that the raw-go steer names the done: false retry of lym gate card-verify.
func TestCheckTexts(t *testing.T) {
	t.Parallel()
	t.Run("ShippedRows", func(t *testing.T) {
		t.Parallel()
		found := false
		gateRuleNotices, editRuleNotices := 0, 0
		for _, row := range Rows("with a long timeout", []string{"go test", "dotnet test"}) {
			if err := checkTexts([]Row{row}); err != nil {
				t.Errorf("checkTexts(%q row) = %v; want nil", row.Name, err)
			}
			if strings.Contains(row.Notice, "Gate rule") {
				gateRuleNotices++
			}
			if strings.Contains(row.Notice, editToolsRule) {
				editRuleNotices++
			}
			if (row.Name == "python" || row.Name == "shell-edit") && !strings.Contains(row.Steer, editToolsRule) {
				t.Errorf("%s steer %q: want it to carry the edit rule %q", row.Name, row.Steer, editToolsRule)
			}
			if row.Name == "raw-go" {
				for _, want := range []string{"done: false", "lym gate card-verify"} {
					if !strings.Contains(row.Steer, want) {
						t.Errorf("raw-go steer %q: want it to name %q", row.Steer, want)
					}
				}
			}
			if row.Name != "refuse-raw" {
				continue
			}
			found = true
			for _, want := range []string{"Gate rule", "lym gate test", "--tier", "--card", "with a long timeout", "done: false", "lym gate card-verify"} {
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
		if editRuleNotices != 1 {
			t.Errorf("Rows() notices state the edit rule %d times; want once", editRuleNotices)
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
