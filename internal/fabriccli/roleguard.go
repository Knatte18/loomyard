// roleguard.go refuses the mutating `lyx fabric` verbs from a sandboxed task-session role.
// It classifies each subcommand as a reader a sandboxed role may run or a verb it may not.
// It also builds the refusal text addWeftVerbs's pre-run writes.

package fabriccli

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/spf13/cobra"
)

// readerVerb reports whether this invocation of cmd only reads, so a sandboxed role may run it.
// A verb is a reader when fabricengine names it one and its arguments keep it so:
// shortname with no argument, prune and cleanup without --apply.
// Any other subcommand, one added later included, is not a reader.
func readerVerb(cmd *cobra.Command) bool {
	if !slices.Contains(fabricengine.FabricReaderVerbs(), cmd.Name()) {
		return false
	}
	switch cmd.Name() {
	case "shortname":
		return cmd.Flags().NArg() == 0
	case "prune", "cleanup":
		apply, _ := cmd.Flags().GetBool("apply")
		return !apply
	}
	return true
}

// sandboxedRoleRefusal returns the refusal text when strandName names a sandboxed role running a verb it must not, and "" when the invocation may go on.
// A sandboxed role is a name that parses with a slug and a role other than the driver and the orch.
// An unset, empty, unparseable or slug-free name passes.
// The detached push child every commit spawns carries a hidden bypass flag and inherits the strand name, so a push with a bypass flag passes;
// a bypass flag on any other subcommand is refused.
// The guard reads the strand name and nothing else, so it stops a mistaken call, not a session that rewrites the variable.
func sandboxedRoleRefusal(cmd *cobra.Command, strandName string) string {
	name, err := agentname.Parse(strandName)
	if err != nil || name.Slug == "" {
		return ""
	}
	if agentname.MatchesRole(name.Role, agentname.RoleDriver) || agentname.MatchesRole(name.Role, agentname.RoleOrch) {
		return ""
	}
	injectedWeft, _ := cmd.Flags().GetString("weft-path")
	injectedWarp, _ := cmd.Flags().GetString("warp-path")
	if injectedWeft != "" || injectedWarp != "" {
		if cmd.Name() == "push" {
			return ""
		}
		return fmt.Sprintf("lyx fabric %s refuses the path flags that only a detached push passes, from the task session %s", cmd.Name(), strandName)
	}
	if readerVerb(cmd) {
		return ""
	}
	return fmt.Sprintf("lyx fabric %s is refused from the task session %s; build a scratch hub as a hubforge test fixture instead, and when the work itself needs this verb, stop and report status: FAILED so the run ends stuck and the orch runs it", cmd.Name(), strandName)
}
