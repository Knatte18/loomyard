// fabricref_test.go covers CheckCardFabricReference through the real location-free rule from internal/fabricengine, over plans parsed from a temp directory.

package planparser_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// TestCheckCardFabricReference covers the spans scanned and the ones that are not: each span kind carrying a sibling path or a spelling fails naming its place and the matched text, while inline code and prose carrying a spelling, a quoted search pattern and a plan without either pass.
func TestCheckCardFabricReference(t *testing.T) {
	t.Parallel()

	siblingPath := fabricengine.WeftWorktree(&lyxcwd.Location{HubPath: "/hub", WorktreeName: "task"})
	siblingName := "/" + filepath.Base(siblingPath)
	pathCommand := "cat " + siblingPath + "/_lyx/plan.md"
	spellingCommand := "lyx fabric sync"

	cardWith := func(intent string) plankit.Card {
		return plankit.Card{
			Number: 1, Slug: "alpha", Intent: intent,
			Groups: []plankit.Group{{Label: "Edit", Targets: []string{"internal/x/x.go"}}},
		}
	}
	parsed := func(framing string, intent string, verify string) *planparser.Plan {
		spec := plankit.Plan{Framing: framing, Cards: []plankit.Card{cardWith(intent)}}
		if verify != "" {
			spec.Sections = []plankit.Section{{Heading: "verify:", Body: verify}}
		}
		dir := filepath.Join(t.TempDir(), "plan")
		plankit.Write(t, dir, spec)
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() failed: %v", err)
		}
		return plan
	}
	fenced := func(cmd string) string { return "run:\n```sh\n" + cmd + "\n```" }

	cardVerify := parsed("", "alpha", "")
	cardVerify.Cards[0].Verify = pathCommand
	cardVerifySpelling := parsed("", "alpha", "")
	cardVerifySpelling.Cards[0].Verify = spellingCommand

	tests := []struct {
		name string
		plan *planparser.Plan
		// want holds the substrings each finding's Detail must carry; empty means the plan passes.
		want []string
	}{
		{"sibling path in a card Verify", cardVerify, []string{"card 1-alpha", siblingName}},
		{"spelling in a card Verify", cardVerifySpelling, []string{"card 1-alpha", "lyx fabric"}},
		{"sibling path in the overview verify section", parsed("", "alpha", pathCommand), []string{`"## verify:" section`, siblingName}},
		{"spelling in the overview verify section", parsed("", "alpha", spellingCommand), []string{`"## verify:" section`, "lyx fabric"}},
		{"sibling path in a card fenced block", parsed("", fenced(pathCommand), ""), []string{"card 1-alpha", siblingName}},
		{"spelling in a card fenced block", parsed("", fenced(spellingCommand), ""), []string{"card 1-alpha", "lyx fabric"}},
		{"spelling in an overview fenced block", parsed(fenced(spellingCommand), "alpha", ""), []string{"the overview", "lyx fabric"}},
		{"sibling path in an inline span", parsed("", "run `"+pathCommand+"` first", ""), []string{"card 1-alpha", siblingName}},
		{"spelling in an inline span passes", parsed("", "run `"+spellingCommand+"` first", ""), nil},
		{"spelling in prose passes", parsed("", "run "+spellingCommand+" later", ""), nil},
		{"exempt spelling in a fenced block fails", parsed("", fenced("lyx.exe weft push"), ""), []string{"lyx.exe"}},
		{"quoted search pattern in a fenced block passes", parsed("", fenced(`grep -n "lyx fabric" docs/overview.md`), ""), nil},
		{"plan without either passes", parsed("", fenced("go test ./..."), "go build ./..."), nil},
	}

	matcher := fabricengine.NewReferenceRule()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := planparser.CheckCardFabricReference(tt.plan, matcher)
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("CheckCardFabricReference() = %v; want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("CheckCardFabricReference() = %v; want one finding", got)
			}
			if got[0].Check != "card-fabric-reference" {
				t.Errorf("Check = %q; want card-fabric-reference", got[0].Check)
			}
			for _, part := range tt.want {
				if !strings.Contains(got[0].Detail, part) {
					t.Errorf("Detail = %q; want it to contain %q", got[0].Detail, part)
				}
			}
		})
	}
}
