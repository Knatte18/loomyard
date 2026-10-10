// cli_test.go drives the help module through a small root with the help command mounted the way cmd/lyx's newRoot mounts it.

package helpcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
)

// newFixtureRoot builds a root with one operator group, one role leaf and one internal leaf, and mounts the help command like newRoot does.
func newFixtureRoot() *cobra.Command {
	run := func(*cobra.Command, []string) error { return nil }
	annotate := func(audience string) map[string]string {
		return map[string]string{clihelp.AudienceAnnotation: audience}
	}

	root := &cobra.Command{Use: "lyx", Short: "fixture root", Long: "fixture root long"}
	board := &cobra.Command{Use: "board", Short: "track tasks", RunE: clihelp.GroupRunE}
	board.AddCommand(&cobra.Command{Use: "get <id>", Short: "read an entry", Annotations: annotate(clihelp.AudienceOperator), RunE: run})
	root.AddCommand(
		board,
		&cobra.Command{Use: "switch <name>", Short: "switch strands", Annotations: annotate(clihelp.AudienceRole), RunE: run},
		&cobra.Command{Use: "watch", Short: "poll in the background", Annotations: annotate(clihelp.AudienceInternal), RunE: run},
	)
	root.SetHelpCommand(Command())
	root.InitDefaultHelpCmd()
	return root
}

// TestHelpCommand_Behaviors pins topic help, the index per audience, and the refusals.
func TestHelpCommand_Behaviors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantCode int
		// wantExact is the whole output when non-empty; wantContains are substrings otherwise.
		wantExact    string
		wantContains []string
	}{
		{name: "bare help prints the root help", args: []string{"help"}, wantContains: []string{"fixture root long", "board"}},
		{name: "help with a topic prints that command's help", args: []string{"help", "board"}, wantContains: []string{"track tasks", "get"}},
		{name: "index defaults to the operator audience", args: []string{"help", "index"}, wantExact: "- `board`: track tasks\n  - `get <id>`: read an entry\n- `help [<command>...]`: print a command's help, or the command index\n  - `index` `--audience`: print the one-line-per-command index of this binary\n"},
		{name: "index --audience role", args: []string{"help", "index", "--audience", "role"}, wantExact: "- `switch <name>`: switch strands\n"},
		{name: "index --audience internal", args: []string{"help", "index", "--audience", "internal"}, wantExact: "- `watch`: poll in the background\n"},
		{name: "unknown audience is refused naming the set", args: []string{"help", "index", "--audience", "everyone"}, wantCode: 1, wantContains: []string{`unknown audience`, "operator, role, internal"}},
		{name: "unknown topic is refused naming the word", args: []string{"help", "no-such-topic"}, wantCode: 1, wantContains: []string{"unknown subcommand", "no-such-topic"}},
		{name: "unknown word under a group is refused naming the word", args: []string{"help", "board", "bogus"}, wantCode: 1, wantContains: []string{"unknown subcommand", "bogus"}},
		{name: "completion lists the command names", args: []string{"__complete", "help", ""}, wantContains: []string{"board\ttrack tasks", "help\t", "switch\t", "watch\t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := clihelp.Execute(newFixtureRoot(), &out, tt.args)
			got := out.String()
			if code != tt.wantCode {
				t.Errorf("exit = %d; want %d. output:\n%s", code, tt.wantCode, got)
			}
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("output =\n%s\nwant\n%s", got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output does not contain %q:\n%s", want, got)
				}
			}
		})
	}
}
