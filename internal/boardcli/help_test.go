// help_test.go pins the documented payload schema visible via --help for every board leaf command.
// Each test drives RunCLI with --help and asserts that the Long output contains the documented
// field names and does NOT contain any removed token (id_or_slug, phase, group) that would signal a
// stale description.

package boardcli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
)

// runHelp invokes RunCLI for a leaf command (identified by one or more path
// segments, e.g. "upsert" or "set-status") with --help and returns the
// combined stdout. Help output does not require a seeded cwd because cobra
// intercepts --help before PersistentPreRunE executes.
func runHelp(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	boardcli.RunCLI(&buf, append(args, "--help"))
	return buf.String()
}

// TestHelpSchema_LeafCommands asserts that each board leaf command's --help output contains the
// documented field names for the post-batch-1 schema and does not contain any removed token
// (id_or_slug, phase, group).
// Every slug-taking verb's help also states the limit formatted from boardengine.MaxSlugLength,
// so the sentence cannot drift from validation.
//
//testtiming:keep pins every leaf command's documented field names, removed tokens and slug-limit sentence in --help, which its covering test does not assert
func TestHelpSchema_LeafCommands(t *testing.T) {
	t.Parallel()
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
				"status",
				"kind",
				"labels",
				"recipe",
				"Example",
			},
			mustNotContain: []string{"deferred", "tier", `"type"`},
		},
		{
			name:        "upsert-batch",
			args:        []string{"upsert-batch"},
			mustContain: []string{"tasks", "slug", "Example"},
		},
		{
			name:        "set-status",
			args:        []string{"set-status"},
			mustContain: []string{"slug", "id", "status", "Example"},
		},
		{
			name:        "remove",
			args:        []string{"remove"},
			mustContain: []string{"slug", "id", "Example"},
		},
		{
			name:        "get",
			args:        []string{"get"},
			mustContain: []string{"slug", "id", `{"task"`, "Example"},
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
				"Example",
			},
		},
		{
			name:        "set-deps",
			args:        []string{"set-deps"},
			mustContain: []string{"slug", "depends_on", "Example"},
		},
		{
			name:           "promote",
			args:           []string{"promote"},
			mustContain:    []string{"slug", "id", "note", "task", "kind", "Example"},
			mustNotContain: []string{"tier"},
		},
		{
			name:           "find",
			args:           []string{"find"},
			mustContain:    []string{"--text", "kind", "slug", "title", "labels", "status"},
			mustNotContain: []string{"tier"},
		},
		{
			name:           "list",
			args:           []string{"list"},
			mustContain:    []string{"--text", "kind", "slug", "title", "labels", "status"},
			mustNotContain: []string{"tier"},
		},
		{
			name:        "prune",
			args:        []string{"prune"},
			mustContain: []string{"done", "depends_on"},
		},
		{
			name: "intake import",
			args: []string{"intake", "import"},
		},
	}
	slugLimit := fmt.Sprintf("A slug is at most %d characters.", boardengine.MaxSlugLength)
	statesSlugLimit := map[string]bool{
		"get": true, "upsert": true, "upsert-batch": true, "set-status": true, "remove": true,
		"merge": true, "promote": true, "set-deps": true, "intake import": true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			helpText := runHelp(t, tt.args...)

			if statesSlugLimit[tt.name] && !strings.Contains(helpText, slugLimit) {
				t.Errorf("RunCLI(%v --help) help text does not contain %q\noutput:\n%s", tt.args, slugLimit, helpText)
			}

			// Each listed field name must appear somewhere in the help output.
			for _, token := range tt.mustContain {
				if !strings.Contains(helpText, token) {
					t.Errorf("RunCLI(%v --help) help text does not contain %q\noutput:\n%s",
						tt.args, token, helpText)
				}
			}

			// No removed token from the old schema must appear in any command's help.
			for _, bad := range append(removedTokens, tt.mustNotContain...) {
				if strings.Contains(helpText, bad) {
					t.Errorf("RunCLI(%v --help) help text must not contain removed token %q\noutput:\n%s",
						tt.args, bad, helpText)
				}
			}
		})
	}
}
