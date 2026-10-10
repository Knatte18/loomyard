// index_test.go pins RenderIndex's line forms and audience filtering over a synthetic cobra tree.

package clihelp

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// indexFixtureCommand builds a command with an audience, a Short and, when run is true, a RunE.
func indexFixtureCommand(use, short, audience string, run bool) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	if audience != "" {
		cmd.Annotations = map[string]string{AudienceAnnotation: audience}
	}
	if run {
		cmd.RunE = func(*cobra.Command, []string) error { return nil }
	}
	return cmd
}

// TestRenderIndex_FormsAndFiltering pins the group and leaf forms, the flag run, the runnable-group forms, the pruning of groups outside the audience and the skips, per audience.
func TestRenderIndex_FormsAndFiltering(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "lyx"}
	root.PersistentFlags().Bool("verbose", false, "")
	root.PersistentFlags().Bool("json", false, "")

	// A plain group with a leaf that has a flag run, one flag already spelled in Use and a hidden flag.
	board := indexFixtureCommand("board", "track tasks", "", false)
	get := indexFixtureCommand("get --id <id>", "read an entry", AudienceOperator, true)
	get.Flags().String("slug", "", "")
	get.Flags().String("id", "", "")
	get.Flags().String("secret", "", "")
	_ = get.Flags().MarkHidden("secret")
	list := indexFixtureCommand("list", "list entries", AudienceOperator, true)
	list.Flags().Bool("all", false, "")
	board.Annotations = map[string]string{IndexNoteAnnotation: "(payload legend)"}
	board.AddCommand(get, list)

	// A runnable group in the operator audience, with children nested below it.
	config := indexFixtureCommand("config [<module>]", "edit config", AudienceOperator, true)
	config.Flags().Bool("print", false, "")
	config.AddCommand(indexFixtureCommand("reconcile", "reconcile config", AudienceOperator, true))

	// A runnable group outside the audience, rendered in group form above a child that is in it.
	reed := indexFixtureCommand("reed [<name>]", "manage strands", AudienceRole, true)
	reed.AddCommand(indexFixtureCommand("up", "start reed", AudienceOperator, true))

	// A group whose only flags are persistent ones stays a group and lists none of them.
	burler := indexFixtureCommand("burler", "run reviews", "", true)
	burler.PersistentFlags().String("target-dir", "", "")
	burler.AddCommand(indexFixtureCommand("run", "run a review", AudienceRole, true))

	// A group none of whose leaves is in the audience, a hidden internal leaf, an unannotated leaf.
	loom := indexFixtureCommand("loom", "drive looms", "", false)
	watch := indexFixtureCommand("watch", "poll", AudienceInternal, true)
	watch.Hidden = true
	loom.AddCommand(watch, indexFixtureCommand("plain", "no audience", "", true))

	completion := indexFixtureCommand("completion", "shell completion", AudienceOperator, false)
	completion.AddCommand(indexFixtureCommand("bash", "bash completion", AudienceOperator, true))

	root.AddCommand(board, config, reed, burler, loom, completion)

	tests := []struct {
		audience string
		want     string
	}{
		{AudienceOperator, strings.Join([]string{
			"- `board`: track tasks (payload legend)",
			"  - `get --id <id>` `--slug`: read an entry",
			"  - `list` `--all`: list entries",
			"- `config [<module>]` `--print`: edit config",
			"  - `reconcile`: reconcile config",
			"- `reed`: manage strands",
			"  - `up`: start reed",
			"",
		}, "\n")},
		{AudienceRole, strings.Join([]string{
			"- `burler`: run reviews",
			"  - `run`: run a review",
			"- `reed [<name>]`: manage strands",
			"",
		}, "\n")},
		{AudienceInternal, strings.Join([]string{
			"- `loom`: drive looms",
			"  - `watch`: poll",
			"",
		}, "\n")},
	}
	for _, tt := range tests {
		t.Run(tt.audience, func(t *testing.T) {
			t.Parallel()
			if got := RenderIndex(root, tt.audience); got != tt.want {
				t.Errorf("RenderIndex(%q) =\n%s\nwant\n%s", tt.audience, got, tt.want)
			}
		})
	}
}
