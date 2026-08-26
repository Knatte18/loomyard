// approve_test.go covers SetApproved: the flip, no-op, and insert cases over the approved:
// frontmatter key, byte-for-byte preservation of every other frontmatter key/order and of the
// entire body, and SetApproved's error cases.

package planparser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// richOverview is a format-4 overview carrying a root: key, a framing paragraph, a Card Index, and
// a plan-level "## Shared Decisions" body section, with approved: set to approvedValue.
func richOverview(approvedValue string) string {
	return "---\n" +
		"format: 4\n" +
		"approved: " + approvedValue + "\n" +
		"root: internal/foo\n" +
		"---\n" +
		"\n" +
		"# Plan: rich\n" +
		"\n" +
		"Framing paragraph.\n" +
		"\n" +
		"## Card Index\n" +
		"\n" +
		"1 — only — the only card\n" +
		"\n" +
		"## Shared Decisions\n" +
		"\n" +
		"- **Decision:** a plan-level decision.\n"
}

func TestSetApproved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// overview is the 00-overview.md content to write, unless noOverview is set.
		overview string
		// noOverview omits 00-overview.md entirely, exercising the missing-file error case.
		noOverview bool
		// withCardFile writes a matching 01-only.md so ParsePlan can round-trip the result.
		withCardFile bool
		// wantErr requires SetApproved to return a non-nil error.
		wantErr bool
		// wantContent is the exact expected 00-overview.md content after a successful call.
		wantContent string
		// wantRoundTripApproved, when withCardFile is set, additionally parses the result and
		// asserts plan.Approved against this value.
		wantRoundTripApproved bool
	}{
		{
			name:                  "flips false to true",
			overview:              richOverview("false"),
			withCardFile:          true,
			wantContent:           richOverview("true"),
			wantRoundTripApproved: true,
		},
		{
			name:        "already true is a byte-identical no-op",
			overview:    richOverview("true"),
			wantContent: richOverview("true"),
		},
		{
			name: "missing approved key gets one inserted",
			overview: "---\n" +
				"format: 4\n" +
				"root: internal/foo\n" +
				"---\n" +
				"\n" +
				"# Plan: minimal\n" +
				"\n" +
				"Framing.\n" +
				"\n" +
				"## Card Index\n" +
				"\n" +
				"1 — only — the only card\n",
			withCardFile: true,
			wantContent: "---\n" +
				"format: 4\n" +
				"root: internal/foo\n" +
				"approved: true\n" +
				"---\n" +
				"\n" +
				"# Plan: minimal\n" +
				"\n" +
				"Framing.\n" +
				"\n" +
				"## Card Index\n" +
				"\n" +
				"1 — only — the only card\n",
			wantRoundTripApproved: true,
		},
		{
			name:       "missing overview file is an error",
			noOverview: true,
			wantErr:    true,
		},
		{
			name:     "no frontmatter fence is an error",
			overview: "# Plan: no frontmatter\n\nJust a body, no --- fence anywhere.\n",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if !tt.noOverview {
				overviewPath := filepath.Join(dir, "00-overview.md")
				if err := os.WriteFile(overviewPath, []byte(tt.overview), 0o644); err != nil {
					t.Fatalf("write fixture overview: %v", err)
				}
			}
			if tt.withCardFile {
				cardPath := filepath.Join(dir, "01-only.md")
				content := "# Card 1 — only\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder card.\n"
				if err := os.WriteFile(cardPath, []byte(content), 0o644); err != nil {
					t.Fatalf("write fixture card: %v", err)
				}
			}

			err := planparser.SetApproved(dir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("SetApproved(%q) error = nil; want non-nil", dir)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetApproved(%q) error = %v; want nil", dir, err)
			}

			got, readErr := os.ReadFile(filepath.Join(dir, "00-overview.md"))
			if readErr != nil {
				t.Fatalf("read result overview: %v", readErr)
			}
			if string(got) != tt.wantContent {
				t.Errorf("00-overview.md content after SetApproved =\n%s\nwant:\n%s", got, tt.wantContent)
			}

			if tt.withCardFile {
				plan, parseErr := planparser.ParsePlan(dir)
				if parseErr != nil {
					t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, parseErr)
				}
				if plan.Approved != tt.wantRoundTripApproved {
					t.Errorf("ParsePlan(%q).Approved = %v; want %v", dir, plan.Approved, tt.wantRoundTripApproved)
				}
			}
		})
	}
}
