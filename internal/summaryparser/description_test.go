// description_test.go exercises ValidateDescription's finding table and LandingMessage.

package summaryparser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestValidateDescription_Table(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const mergeDanger = "\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** docs\n"
	tests := []struct {
		name    string
		content *string
		want    []string
	}{
		{"missing", nil, []string{summaryparser.CheckMissing}},
		{"empty", ptr(""), []string{summaryparser.CheckEmpty}},
		{"no heading", ptr("plain text\n"), []string{summaryparser.CheckNoHeading}},
		{"blank title", ptr("# \nbody\n"), []string{summaryparser.CheckNoHeading}},
		{"long title", ptr("# " + strings.Repeat("a", 73) + "\n\nbody\n" + mergeDanger), []string{summaryparser.CheckTitleTooLong}},
		{"empty body", ptr("# Title\n\n  \n"), []string{summaryparser.CheckEmptyBody}},
		{"trailer", ptr("# Title\n\nbody\n" + mergeDanger + "\nCo-Authored-By: X <x@y>\n"), []string{summaryparser.CheckCoAuthorTrail}},
		{"trailer lower", ptr("# Title\n\nbody\n" + mergeDanger + "  co-authored-by: X\n"), []string{summaryparser.CheckCoAuthorTrail}},
		{"ok", ptr("# Title\n\nbody\n" + mergeDanger), nil},
		{"no merge danger", ptr("# Title\n\nbody\n"), []string{summaryparser.CheckNoMergeDanger}},
		{"merge danger as list items in backticks", ptr("# Title\n\nbody\n\n## merge danger\n\n- **Door:** `One-way`.\n- **Blast radius:** `board`.\n"), nil},
		{"labels directly under the heading", ptr("# Title\n\nbody\n\n## Merge danger\n**Door:** two-way\n**Blast radius:** docs\n"), nil},
		{
			"a level-one heading ends the section",
			ptr("# Title\n\nbody\n\n## Merge danger\n\n# Other\n\n**Door:** two-way\n**Blast radius:** docs\n"),
			[]string{summaryparser.CheckMergeDangerDoor, summaryparser.CheckMergeDangerBlastRadius},
		},
		{
			"a section only inside a fence is no section",
			ptr("# Title\n\nbody\n\n```\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** docs\n```\n"),
			[]string{summaryparser.CheckNoMergeDanger},
		},
		{
			"a fenced example does not stand in for the real section",
			ptr("# Title\n\nbody\n\n```\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** docs\n```\n\n## Merge danger\n\n**Door:** revolving\n**Blast radius:** docs\n"),
			[]string{summaryparser.CheckMergeDangerDoor},
		},
		{
			"a shorter fence line inside a fence does not close it",
			ptr("# Title\n\nbody\n\n````\n```\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** docs\n```\n````\n"),
			[]string{summaryparser.CheckNoMergeDanger},
		},
		{
			"a comment line in a fenced block does not end the section",
			ptr("# Title\n\nbody\n\n## Merge danger\n\n~~~sh\n# revert it\ngit revert HEAD\n~~~\n\n**Door:** two-way\n**Blast radius:** docs\n"),
			nil,
		},
		{"door missing", ptr("# Title\n\nbody\n\n## Merge danger\n\n**Blast radius:** docs\n"), []string{summaryparser.CheckMergeDangerDoor}},
		{"door neither way", ptr("# Title\n\nbody\n\n## Merge danger\n\n**Door:** revolving\n**Blast radius:** docs\n"), []string{summaryparser.CheckMergeDangerDoor}},
		{"blast radius missing", ptr("# Title\n\nbody\n\n## Merge danger\n\n**Door:** one-way\n"), []string{summaryparser.CheckMergeDangerBlastRadius}},
		{"blast radius two words", ptr("# Title\n\nbody\n\n## Merge danger\n\n**Door:** one-way\n**Blast radius:** the board\n"), []string{summaryparser.CheckMergeDangerBlastRadius}},
		{"blast radius empty", ptr("# Title\n\nbody\n\n## Merge danger\n\n**Door:** one-way\n**Blast radius:**\n"), []string{summaryparser.CheckMergeDangerBlastRadius}},
		{
			"labels under the next heading are outside the section",
			ptr("# Title\n\nbody\n\n## Merge danger\n\n## Notes\n\n**Door:** two-way\n**Blast radius:** docs\n"),
			[]string{summaryparser.CheckMergeDangerDoor, summaryparser.CheckMergeDangerBlastRadius},
		},
		{"a level-three heading stays inside the section", ptr("# Title\n\nbody\n\n## Merge danger\n\n### Call\n\n**Door:** two-way\n**Blast radius:** docs\n"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "summary.md")
			if tt.content != nil {
				if err := os.WriteFile(path, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := summaryparser.ValidateDescription(path)
			if err != nil {
				t.Fatalf("error = %v; want nil", err)
			}
			var names []string
			for _, f := range got {
				names = append(names, f.Check)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("checks = %v; want %v", names, tt.want)
			}
		})
	}
}

func TestValidateDescription_ReadErrorIsError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	// A directory at the path is a read failure other than not-exist.
	if _, err := summaryparser.ValidateDescription(t.TempDir()); err == nil {
		t.Fatal("error = nil; want a read error")
	}
}

func TestFinding_Error(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if got := (summaryparser.Finding{Check: "a", Detail: "b"}).Error(); got != "a: b" {
		t.Errorf("Error() = %q", got)
	}
	var _ error = summaryparser.Finding{}
}

func TestLandingMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	withBody := &summaryparser.Summary{Title: "T", Body: "\nbody\n"}
	titleOnly := &summaryparser.Summary{Title: "T"}
	tests := []struct {
		name    string
		summary *summaryparser.Summary
		credit  string
		want    string
	}{
		{"BodyAndCredit", withBody, "Created using Loomyard\nCo-Authored-By: A <a@b>", "T\n\nbody\n\n\nCreated using Loomyard\nCo-Authored-By: A <a@b>"},
		{"BodyNoCredit", withBody, "", "T\n\nbody\n"},
		{"TitleOnlyAndCredit", titleOnly, "Co-Authored-By: A", "T\n\nCo-Authored-By: A"},
		{"TitleOnlyNoCredit", titleOnly, "", "T"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.summary.LandingMessage(tt.credit); got != tt.want {
				t.Errorf("LandingMessage(%q) = %q; want %q", tt.credit, got, tt.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }
