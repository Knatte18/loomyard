// rows_test.go pins the package's import set and the refusal of forbidden characters in a row's steer and notice.

package shellguard

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is the closed set of non-stdlib imports the package admits.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/agentname",
	"github.com/Knatte18/loomyard/internal/gateslot",
}

// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in allowedImports.
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scankit.AssertImportAllowlist(t, "internal/shellguard", allowedImports...)
}

// TestCheckTexts plants each forbidden character in a row's steer and notice and asserts checkTexts refuses it naming the row;
// it also asserts the shipped rows, rendered over a sample refuse list, pass checkTexts and state the gate rule and the edit rule in one notice each, that the refuse-raw steer carries the full gate rule while its notice names only the refusal and the tier and card scopes, that the python and shell-edit steers carry the full edit rule, that the Codex python steer opens with the shared refusal, and that the raw-go steer names the done: false retry of lym gate card-verify.
func TestCheckTexts(t *testing.T) {
	tiergate.Require(t, 0)
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
			if row.Name == "python" && !strings.HasPrefix(row.SteerFor(engineCodex), pythonRefusal) {
				t.Errorf("python Codex steer %q: want it to open with the refusal %q", row.SteerFor(engineCodex), pythonRefusal)
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
		name string
		row  Row
	}{
		{"SteerSingleQuote", Row{Steer: "it's"}},
		{"SteerDoubleQuote", Row{Steer: `say "x"`}},
		{"SteerBackslash", Row{Steer: `a\b`}},
		{"NoticeSingleQuote", Row{Notice: "it's"}},
		{"NoticeDoubleQuote", Row{Notice: `say "x"`}},
		{"NoticeBackslash", Row{Notice: `a\b`}},
		{"NoticeLineBreak", Row{Notice: "one\ntwo"}},
		{"EngineSteerSingleQuote", Row{EngineTexts: map[string]EngineText{engineCodex: {Steer: "it's"}}}},
		{"EngineNoticeLineBreak", Row{EngineTexts: map[string]EngineText{engineCodex: {Notice: "one\ntwo"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			row := tt.row
			row.Name = "planted"
			err := checkTexts([]Row{row})
			if err == nil || !strings.Contains(err.Error(), `"planted"`) {
				t.Errorf("checkTexts() = %v; want an error naming the row", err)
			}
		})
	}
}

// TestRows_EngineTexts pins the per-engine texts.
// On Codex no row's steer or notice names a Claude tool, the python and shell-edit steers carry the Codex edit rule, and one notice states it.
// An engine without its own text, Claude's included, gets the plain steer and notice.
func TestRows_EngineTexts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	codexEditRuleNotices := 0
	for _, row := range Rows("with a long timeout", []string{"go test"}) {
		steer, notice := row.SteerFor(engineCodex), row.NoticeFor(engineCodex)
		for _, claudeTool := range []string{"Read", "Edit", "Write", "Agent tool"} {
			if strings.Contains(steer, claudeTool) || strings.Contains(notice, claudeTool) {
				t.Errorf("%s Codex steer %q or notice %q names the Claude tool %q", row.Name, steer, notice, claudeTool)
			}
		}
		if (row.Name == "python" || row.Name == "shell-edit") && !strings.Contains(steer, codexEditToolsRule) {
			t.Errorf("%s Codex steer %q: want it to carry the Codex edit rule %q", row.Name, steer, codexEditToolsRule)
		}
		if strings.Contains(notice, codexEditToolsRule) {
			codexEditRuleNotices++
		}
		if got := row.SteerFor("claude"); got != row.Steer {
			t.Errorf("%s SteerFor(claude) = %q; want the plain steer %q", row.Name, got, row.Steer)
		}
		if got := row.NoticeFor("claude"); got != row.Notice {
			t.Errorf("%s NoticeFor(claude) = %q; want the plain notice %q", row.Name, got, row.Notice)
		}
	}
	if codexEditRuleNotices != 1 {
		t.Errorf("Codex notices state the Codex edit rule %d times; want once", codexEditRuleNotices)
	}
}

// TestRows_RawGoSteerNamesOnlyUnrefusedCompileChecks asserts the raw go row's steer and notice offer each package-scoped compile check only while no refuse-raw prefix refuses it.
func TestRows_RawGoSteerNamesOnlyUnrefusedCompileChecks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		refuseRaw []string
		want      string
	}{
		{"NoRefuseList", nil, " A compile check names its packages, as in go build ./internal/x or go vet ./internal/x, never ./... or all. Gate rule"},
		{"GoTestRefused", []string{"go test"}, " A compile check names its packages, as in go build ./internal/x or go vet ./internal/x, never ./... or all. Gate rule"},
		{"GoBuildRefused", []string{"go test", "go build"}, " A compile check names its packages, as in go vet ./internal/x, never ./... or all. Gate rule"},
		{"PrefixLongerThanForm", []string{"go build ./internal/x extra"}, " A compile check names its packages, as in go build ./internal/x or go vet ./internal/x, never ./... or all. Gate rule"},
		{"EveryGoCommandRefused", []string{"go"}, "are refused here. Gate rule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, row := range Rows("with a long timeout", tt.refuseRaw) {
				if row.Name != "raw-go" {
					continue
				}
				if !strings.Contains(row.Steer, tt.want) {
					t.Errorf("raw-go steer with refuse list %q = %q; want it to contain %q", tt.refuseRaw, row.Steer, tt.want)
				}
				if row.Notice != row.Steer+"." {
					t.Errorf("raw-go notice = %q; want the steer and a full stop", row.Notice)
				}
			}
		})
	}
}
