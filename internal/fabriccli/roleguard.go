// roleguard.go refuses the mutating `lym fabric` verbs from a sandboxed task-session role.
// It classifies each subcommand as a reader that a sandboxed role may run, or as a verb it may not.
// A merge task's writer may also run the merge verbs,
// and any sandboxed role may run every verb in a hub under its own worktree's sandbox root.
// The refusal text is built here as well.

package fabriccli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
	"github.com/spf13/cobra"
)

// sandboxRootRel is a worktree's sandbox root, relative to the worktree: the directory a hardener run makes its hubs in.
var sandboxRootRel = filepath.Join(lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName)

// verbTargetDir returns the directory this invocation of cmd acts in.
// That is the cwd carried on its context, and for clone the destination its --into flag names.
func verbTargetDir(cmd *cobra.Command) (string, error) {
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return "", err
	}
	if cmd.Name() != "clone" {
		return cwd, nil
	}
	into, _ := cmd.Flags().GetString("into")
	return cloneDestination(cwd, into), nil
}

// resolveThroughExistingAncestor returns path with the symlinks of its deepest existing ancestor resolved.
// The part below that ancestor, which does not exist yet, is re-joined unchanged.
// It reports false when no ancestor can be resolved.
// It also reports false when a path on the way exists but cannot be resolved, such as a dangling link.
func resolveThroughExistingAncestor(path string) (string, bool) {
	current := filepath.Clean(path)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), true
		}
		if _, statErr := os.Lstat(current); statErr == nil {
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}
}

// worktreeOwningRecordsDir returns the worktree whose records sibling is the directory dir.
// A directory that is not a records sibling, or whose worktree is missing, comes back unchanged.
func worktreeOwningRecordsDir(dir string) string {
	owner, hasSuffix := strings.CutSuffix(filepath.Base(dir), weftname.Suffix)
	if !hasSuffix || owner == "" {
		return dir
	}
	ownerPath := filepath.Join(filepath.Dir(dir), owner)
	if info, err := os.Stat(ownerPath); err != nil || !info.IsDir() {
		return dir
	}
	return ownerPath
}

// inOwnSandboxRoot reports whether dir lies at or under the sandbox root of the worktree that the strand name identifies.
// The sandbox root is `.lym/harden-sandbox`.
// That worktree is named by the name's slug and sits in the hub whose recorded shortname is the name's shortname.
// dir is judged where it lands, with the symlinks of its deepest existing ancestor resolved.
// A link therefore counts where it points.
// A clone's destination that does not exist yet counts where it would be made.
// A worktree's `.lym` is a link into its records sibling.
// A dir in its resolved form therefore maps back to the worktree it belongs to.
// The sandbox root the worktree itself holds must be the directory dir lies under.
func inOwnSandboxRoot(dir string, name agentname.Name) bool {
	current, ok := resolveThroughExistingAncestor(dir)
	if !ok {
		return false
	}
	for {
		container := filepath.Dir(filepath.Dir(current))
		if filepath.Join(container, sandboxRootRel) == current {
			worktree := worktreeOwningRecordsDir(container)
			location, err := lymcwd.ResolveWorktree(worktree)
			if err != nil || location.WorktreeName != name.Slug || location.WorktreePath() != worktree {
				return false
			}
			shortname, found := fabricengine.ReadShortname(fabricengine.BoltDir(location.HubPath))
			if !found || shortname != name.Shortname {
				return false
			}
			root, ok := resolveThroughExistingAncestor(filepath.Join(worktree, sandboxRootRel))
			return ok && root == current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// mergeTaskVerb reports whether this invocation of cmd is one a merge task's writer needs in its own worktree:
// merge-in, merge-stage, and merge in its --continue and --abort forms.
// The merge form that takes a branch is not one of them: a run's Finalize performs it.
func mergeTaskVerb(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "merge-in", "merge-stage":
		return true
	case "merge":
		concludes, _ := cmd.Flags().GetBool("continue")
		aborts, _ := cmd.Flags().GetBool("abort")
		return concludes || aborts
	}
	return false
}

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

// sandboxedRoleRefusal returns the refusal text when strandName names a sandboxed role running a verb it must not.
// It returns "" when the invocation may go on.
// A sandboxed role is a name that parses with a slug and a role other than the driver and the orch.
// An unset, empty, unparseable or slug-free name passes.
// The detached push child every commit spawns inherits the strand name and carries a hidden bypass flag,
// so a push with a bypass flag passes; a bypass flag on any other subcommand is refused.
// The hidden teardown child a removal spawns inherits the strand name the same way, so it passes too.
// A merge task's writer passes on the verbs its work is, since the merge of the parent branch is that task.
// Any verb passes when the directory it acts in lies under the sandbox root of the role's own worktree,
// so a hardener round drives its own sandbox hubs while a verb aimed at the real hub stays refused.
// The guard reads the strand name and the paths the call names, so it stops a mistaken call.
// It does not stop a session that rewrites the variable or makes a sandbox root of its own.
func sandboxedRoleRefusal(cmd *cobra.Command, strandName string) string {
	name, err := agentname.Parse(strandName)
	if err != nil || name.Slug == "" {
		return ""
	}
	if agentname.MatchesRole(name.Role, agentname.RoleDriver) || agentname.MatchesRole(name.Role, agentname.RoleOrch) {
		return ""
	}
	if cmd.Name() == fabricengine.TeardownChildVerb {
		return ""
	}
	injectedWeft, _ := cmd.Flags().GetString("weft-path")
	injectedWarp, _ := cmd.Flags().GetString("warp-path")
	if injectedWeft != "" || injectedWarp != "" {
		if cmd.Name() == "push" {
			return ""
		}
		return fmt.Sprintf("lym fabric %s refuses the path flags that only a detached push passes, from the task session %s", cmd.Name(), strandName)
	}
	if readerVerb(cmd) || (darnengine.IsMergeTaskWriter(name) && mergeTaskVerb(cmd)) {
		return ""
	}
	if target, err := verbTargetDir(cmd); err == nil && inOwnSandboxRoot(target, name) {
		return ""
	}
	return fmt.Sprintf("lym fabric %s is refused from the task session %s outside a hub under its worktree's %s; build a scratch hub as a hubforge test fixture instead, and when the work itself needs this verb, stop and report status: FAILED so the run ends stuck and the orch runs it", cmd.Name(), strandName, sandboxRootRel)
}
