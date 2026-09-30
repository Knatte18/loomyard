// description_test.go exercises ValidateDescription's finding table and LandingMessage.

package summaryparser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/summaryparser"
)

func TestValidateDescription_Table(t *testing.T) {
	tests := []struct {
		name    string
		content *string
		want    []string
	}{
		{"missing", nil, []string{summaryparser.CheckMissing}},
		{"empty", ptr(""), []string{summaryparser.CheckEmpty}},
		{"no heading", ptr("plain text\n"), []string{summaryparser.CheckNoHeading}},
		{"blank title", ptr("# \nbody\n"), []string{summaryparser.CheckNoHeading}},
		{"long title", ptr("# " + strings.Repeat("a", 73) + "\n\nbody\n"), []string{summaryparser.CheckTitleTooLong}},
		{"empty body", ptr("# Title\n\n  \n"), []string{summaryparser.CheckEmptyBody}},
		{"trailer", ptr("# Title\n\nbody\n\nCo-Authored-By: X <x@y>\n"), []string{summaryparser.CheckCoAuthorTrail}},
		{"trailer lower", ptr("# Title\n\nbody\n  co-authored-by: X\n"), []string{summaryparser.CheckCoAuthorTrail}},
		{"ok", ptr("# Title\n\nbody\n"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	// A directory at the path is a read failure other than not-exist.
	if _, err := summaryparser.ValidateDescription(t.TempDir()); err == nil {
		t.Fatal("error = nil; want a read error")
	}
}

func TestFinding_Error(t *testing.T) {
	if got := (summaryparser.Finding{Check: "a", Detail: "b"}).Error(); got != "a: b" {
		t.Errorf("Error() = %q", got)
	}
	var _ error = summaryparser.Finding{}
}

func TestLandingMessage(t *testing.T) {
	s := &summaryparser.Summary{Title: "T", Body: "\nbody\n"}
	want := "T\n\nbody\n\n\nCo-Authored-By: A <a@b>"
	if got := s.LandingMessage("A <a@b>"); got != want || strings.Count(got, "Co-Authored-By:") != 1 {
		t.Errorf("LandingMessage = %q; want %q", got, want)
	}
	empty := &summaryparser.Summary{Title: "T"}
	if got := empty.LandingMessage("A"); got != "T\n\nCo-Authored-By: A" {
		t.Errorf("empty-body LandingMessage = %q", got)
	}
}

func ptr(s string) *string { return &s }
