// help_test.go pins the documented payload schema visible via --help for every
// board leaf command. Each test drives RunCLI with --help and asserts that the
// Long output contains the documented field names and does NOT contain any
// removed token (id_or_slug, phase, group) that would signal a stale description.

package boardcli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
)

// runHelp invokes RunCLI for a leaf command (identified by one or more path
// segments, e.g. "upsert" or "notes", "upsert") with --help and returns the
// combined stdout. Help output does not require a seeded cwd because cobra
// intercepts --help before PersistentPreRunE executes.
func runHelp(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	boardcli.RunCLI(&buf, append(args, "--help"))
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
		args           []string
		mustContain    []string // field names or tokens that must appear in the Long
		mustNotContain []string // overrides removedTokens for a specific command (merged)
	}{
		{
			name: "upsert",
			args: []string{"upsert"},
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
			args: []string{"upsert-batch"},
			mustContain: []string{
				"tasks",
				"slug",
			},
		},
		{
			name: "set-status",
			args: []string{"set-status"},
			mustContain: []string{
				"slug",
				"id",
				"status",
			},
		},
		{
			name: "remove",
			args: []string{"remove"},
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "get",
			args: []string{"get"},
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "merge",
			args: []string{"merge"},
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
			args: []string{"set-deps"},
			mustContain: []string{
				"slug",
				"depends_on",
			},
		},
		{
			name: "notes upsert",
			args: []string{"notes", "upsert"},
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
			name: "notes set-status",
			args: []string{"notes", "set-status"},
			mustContain: []string{
				"slug",
				"id",
				"status",
			},
		},
		{
			name: "notes remove",
			args: []string{"notes", "remove"},
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "notes get",
			args: []string{"notes", "get"},
			mustContain: []string{
				"slug",
				"id",
			},
		},
		{
			name: "notes merge",
			args: []string{"notes", "merge"},
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
			name: "notes set-deps",
			args: []string{"notes", "set-deps"},
			mustContain: []string{
				"slug",
				"depends_on",
			},
		},
		{
			name: "promote-note",
			args: []string{"promote-note"},
			mustContain: []string{
				"slug",
				"id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpText := runHelp(t, tt.args...)

			// Each listed field name must appear somewhere in the help output.
			for _, token := range tt.mustContain {
				if !strings.Contains(helpText, token) {
					t.Errorf("RunCLI(%v --help) help text does not contain %q\noutput:\n%s",
						tt.args, token, helpText)
				}
			}

			// No removed token from the old schema must appear in any command's help.
			for _, bad := range removedTokens {
				if strings.Contains(helpText, bad) {
					t.Errorf("RunCLI(%v --help) help text must not contain removed token %q\noutput:\n%s",
						tt.args, bad, helpText)
				}
			}
		})
	}
}
