// cli_helper_test.go holds the verb-command builder the tier0 and tier1 tests of this package share.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// battenVerbCommand builds the single shedverbs command named verb ("run", "step", "status", or "pause"), armed against c.specFor(verb) assigned onto c.spec.
// That is the seam arm.go's specFor/arm split exists for, letting a test fill a Spec with no resolution and no git spawn.
// It sets Args: cobra.ExactArgs(1), a stricter arity than Command() itself sets on these four verbs (cobra.MaximumNArgs(1)) but a harmless one for these tests: every caller supplies exactly one positional.
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
