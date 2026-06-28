// help_test.go pins the documented payload schema visible via --help for every
// board leaf command. Each test drives RunCLI with --help and asserts that the
// Long output contains the documented field names and does NOT contain any
// removed token (id_or_slug, phase, group) that would signal a stale description.

package board_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// runHelp invokes RunCLI for a single leaf command with --help and returns the
// combined stdout. Help output does not require a seeded cwd because cobra
// intercepts --help before PersistentPreRunE executes.
func runHelp(t *testing.T, verb string) string {
	t.Helper()
	var buf bytes.Buffer
	board.RunCLI(&buf, []string{verb, "--help"})
	return buf.String()
}

// TestHelpSchema_LeafCommands asserts that each board leaf command's --help
// output contains the documented field names for the post-batch-1 schema and
// does not contain any removed token (id_or_slug, phase, group).
func TestHelpSchema_LeafCommands(t *testing.T) {
	// removedTokens are field names that were present in the old schema and must
	// not appear in any --help output after the batch-1 rename.
	removedTokens := []string{"id_or_slug", "phase", "group"}

	tests := []struct {
		name           string
		verb           string
		mustContain    []string // field names or tokens that must appear in the Long
		mustNotContain []string // overrides removedTokens for a specific command (merged)
	}{
		{
			name: "upsert",
			verb: "upsert",
			mustContain: []string{
				"slug",
				"title",
				"brief",
				"body",
				"depends_on",
				"isolated",
				"deferred",
				"status",
			},
		},
		{
			name: "upsert-batch",
			verb: "upsert-batch",
			mustContain: []string{
				"tasks",
				"slug",
			},
		},
		{
			name: "set-status",
			verb: "set-status",
			mustContain: []string{
				"slug",
				"id",
				"status",
			},
		},
		{
			name: "remove",
			verb: "remove",
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "get",
			verb: "get",
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "merge",
			verb: "merge",
			mustContain: []string{
				"remove_slugs",
				"upsert",
				"set_status",
				"slug",
				"id",
				"status",
			},
		},
		{
			name: "set-deps",
			verb: "set-deps",
			mustContain: []string{
				"slug",
				"depends_on",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpText := runHelp(t, tt.verb)

			// Each listed field name must appear somewhere in the help output.
			for _, token := range tt.mustContain {
				if !strings.Contains(helpText, token) {
					t.Errorf("RunCLI(%q --help) help text does not contain %q\noutput:\n%s",
						tt.verb, token, helpText)
				}
			}

			// No removed token from the old schema must appear in any command's help.
			for _, bad := range removedTokens {
				if strings.Contains(helpText, bad) {
					t.Errorf("RunCLI(%q --help) help text must not contain removed token %q\noutput:\n%s",
						tt.verb, bad, helpText)
				}
			}
		})
	}
}
