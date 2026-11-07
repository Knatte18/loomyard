// cli_helper_test.go holds the verb-command builder the tier0 and tier1 tests of this package share.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// battenVerbCommand returns the shedverbs command named verb, with c.spec set to a Spec built for that verb.
// The Spec is filled with no resolution and no git spawn, so a test can run the command without either.
// It sets Args to cobra.ExactArgs(1), since every caller supplies exactly one positional.
func battenVerbCommand(c *battenCLI, verb string) *cobra.Command {
	spec := c.specFor(verb)
	c.spec = &spec
	for _, cmd := range shedverbs.Verbs(battenVerbTexts, c.spec) {
		if cmd.Name() == verb {
			cmd.Args = cobra.ExactArgs(1)
			return cmd
		}
	}
	panic("battenVerbCommand: no shedverbs command named " + verb)
}
