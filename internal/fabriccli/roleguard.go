// roleguard.go refuses the mutating `lym fabric` verbs from a sandboxed task-session role.
// It classifies each subcommand as a reader that a sandboxed role may run, or as a verb it may not.
// A merge task's writer may also run the merge verbs, a docs run's darn writer may add and remove its own bench pairs,
// and any sandboxed role may run every verb in a hub under its own worktree's sandbox root.
// A detached push passes only for the role's own pair, a pair under that root,
// or a bench pair its darn writer's run may touch.
// The refusal text is built here as well.

package fabriccli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/shedrun"
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
			if !isOwnWorktree(worktree, name) {
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

// strandHubWorktree returns the location of worktree and reports whether it passes three checks.
// worktree is a worktree root, its name passes named, and its hub records the strand name's shortname.
func strandHubWorktree(worktree string, name agentname.Name, named func(worktreeName string) bool) (*lymcwd.Location, bool) {
	location, err := lymcwd.ResolveWorktree(worktree)
	if err != nil || !named(location.WorktreeName) || location.WorktreePath() != worktree {
		return nil, false
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoltDir(location.HubPath))
	return location, found && shortname == name.Shortname
}

// isOwnWorktree reports whether worktree is the root of the worktree the strand name identifies.
// That worktree is named by the name's slug and sits in the hub whose recorded shortname is the name's shortname.
func isOwnWorktree(worktree string, name agentname.Name) bool {
	_, ok := strandHubWorktree(worktree, name, func(worktreeName string) bool { return worktreeName == name.Slug })
	return ok
}

// pairLocation returns the location of the worktree dir belongs to, as that worktree or its records sibling.
// It reports false unless that worktree's name passes named and it sits in the strand name's hub.
// dir is judged where it lands, with the symlinks of its deepest existing ancestor resolved.
func pairLocation(dir string, name agentname.Name, named func(worktreeName string) bool) (*lymcwd.Location, bool) {
	current, ok := resolveThroughExistingAncestor(dir)
	if !ok {
		return nil, false
	}
	return strandHubWorktree(worktreeOwningRecordsDir(current), name, named)
}

// ownPairLocation returns the location of the strand name's own worktree for dir.
// It reports false unless dir is that worktree or its records sibling.
func ownPairLocation(dir string, name agentname.Name) (*lymcwd.Location, bool) {
	return pairLocation(dir, name, func(worktreeName string) bool { return worktreeName == name.Slug })
}

// isOwnPairPath reports whether dir is the worktree the strand name identifies, or that worktree's records sibling.
func isOwnPairPath(dir string, name agentname.Name) bool {
	_, ok := ownPairLocation(dir, name)
	return ok
}

// benchPairAdmitted reports whether the run of the task worktree at task may touch the bench pair benchSlug.
// It may when the run's identity record there names the darn recipe and the docs profile,
// and no batten run directory on the prime names benchSlug, which would make the pair another run's.
// The run's seed writes the record with the profile the run took, the entry's own or the recipe's default.
// The board is not read, so a slug that only a board entry names is not caught.
// A non-nil error says why an otherwise admitted run is barred from the pair, or which read failed.
func benchPairAdmitted(task *lymcwd.Location, benchSlug string) (bool, error) {
	run, found, err := shedrun.ReadRunUsageRecord(shedrun.UsageDir(task))
	if err != nil {
		return false, err
	}
	if !found || run.Recipe != shedrun.RecipeDarn || run.Profile != runprofile.DocsProfile {
		return false, nil
	}
	prime, err := hubgeom.PrimeWithoutGit(task)
	// A repository whose refs are not files can only be listed by git itself.
	if errors.Is(err, dotgit.ErrRefStorage) {
		prime, err = primeThroughGit(task)
	}
	if err != nil {
		return false, err
	}
	_, err = os.Stat(shedrun.RunDir(prime, benchSlug))
	switch {
	case err == nil:
		return false, fmt.Errorf("a batten run directory names %s, so that pair is another run's; take another number", benchSlug)
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("read the batten run directory of %s: %w", benchSlug, err)
	}
	return true, nil
}

// primeThroughGit returns the location of the hub's prime worktree, as git's own worktree list names it.
func primeThroughGit(location *lymcwd.Location) (*lymcwd.Location, error) {
	name, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, err
	}
	return lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, name))
}

// isBenchPairPath reports whether the strand name's darn writer may push dir.
// dir must be a bench pair of that writer's task, or the pair's records sibling.
// The bench pair sits in the writer's own hub, and the run of the writer's own worktree in that hub may touch it.
func isBenchPairPath(dir string, name agentname.Name) bool {
	if !darnengine.IsWriter(name) {
		return false
	}
	pair, ok := pairLocation(dir, name, func(worktreeName string) bool { return darnengine.IsBenchPairSlug(name.Slug, worktreeName) })
	if !ok {
		return false
	}
	task, ok := ownPairLocation(filepath.Join(pair.HubPath, name.Slug), name)
	if !ok {
		return false
	}
	admitted, err := benchPairAdmitted(task, pair.WorktreeName)
	return err == nil && admitted
}

// injectedPathsAreOwn reports whether the strand may push every non-empty path a detached push of cmd names.
// It may push its own worktree, that worktree's records sibling and a path under its own worktree's sandbox root.
// A darn writer on a docs run may also push a bench pair of its task or that pair's records sibling.
// A relative path is read against the directory the invocation acts in.
func injectedPathsAreOwn(cmd *cobra.Command, name agentname.Name, paths ...string) bool {
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return false
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		if !isOwnPairPath(path, name) && !inOwnSandboxRoot(path, name) && !isBenchPairPath(path, name) {
			return false
		}
	}
	return true
}

// benchPairVerbs are the verbs a darn writer may run on a bench pair of its own task.
var benchPairVerbs = []string{"add", "remove"}

// benchPairVerb reports whether this invocation of cmd adds or removes a bench pair of the strand name's darn writer.
// The one argument is a bench pair slug of the writer's task.
// The verb acts in the writer's own worktree or its records sibling,
// and an add takes neither --parent nor --adopt-local, so the pair forks from the task branch.
// The run of that worktree must be one that may touch the pair.
// A non-nil error says why that run is barred from it.
func benchPairVerb(cmd *cobra.Command, name agentname.Name) (bool, error) {
	if !darnengine.IsWriter(name) || !slices.Contains(benchPairVerbs, cmd.Name()) {
		return false, nil
	}
	args := cmd.Flags().Args()
	if len(args) != 1 || !darnengine.IsBenchPairSlug(name.Slug, args[0]) {
		return false, nil
	}
	if cmd.Name() == "add" {
		parent, _ := cmd.Flags().GetString("parent")
		adoptLocal, _ := cmd.Flags().GetBool("adopt-local")
		if parent != "" || adoptLocal {
			return false, nil
		}
	}
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return false, err
	}
	location, ok := ownPairLocation(cwd, name)
	if !ok {
		return false, nil
	}
	return benchPairAdmitted(location, args[0])
}

// benchPairWayForward is the clause a darn writer's refused add or remove carries.
// It names the bench pairs the writer may make itself.
func benchPairWayForward(taskSlug string) string {
	return fmt.Sprintf("; a darn writer on a run with profile %s adds and removes its own bench pairs itself: "+
		"a slug of the form %s<N> such as %s1, run from its task worktree, as `lym fabric add %s1` with no --parent or --adopt-local",
		runprofile.DocsProfile, taskSlug+darnengine.BenchPairSlugInfix, taskSlug+darnengine.BenchPairSlugInfix, taskSlug+darnengine.BenchPairSlugInfix)
}

// taskSessionRefusal is the refusal of `lym fabric <verb>` from the task session strandName, naming its ways forward.
func taskSessionRefusal(verb, strandName string) string {
	return fmt.Sprintf("lym fabric %s is refused from the task session %s outside a hub under its worktree's %s; "+
		"build a scratch hub as a hubforge test fixture instead, or run it in a hub made with `lym harden sandbox new` under <worktree>/%s, "+
		"and when the work itself needs this verb, stop and report status: FAILED so the run ends stuck and the orch runs it",
		verb, strandName, sandboxRootRel, sandboxRootRel)
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
// The detached push child every commit spawns inherits the strand name and carries the hidden path flags.
// So a push passes when every path it names is the role's own worktree, that worktree's records sibling,
// or a path under the role's own sandbox root; a path flag on any other subcommand is refused.
// The hidden teardown child a removal spawns inherits the strand name the same way, so it passes too.
// A merge task's writer passes on the verbs its work is, since the merge of the parent branch is that task.
// A darn writer on a docs run passes on adding and removing its own bench pairs from its task worktree.
// It also passes on a push of such a pair, so a docs bench makes its pairs without the orch.
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
		if cmd.Name() != "push" {
			return fmt.Sprintf("lym fabric %s refuses the path flags that only a detached push passes, from the task session %s", cmd.Name(), strandName)
		}
		if injectedPathsAreOwn(cmd, name, injectedWeft, injectedWarp) {
			return ""
		}
		return taskSessionRefusal(cmd.Name(), strandName)
	}
	benchAdmitted, benchErr := benchPairVerb(cmd, name)
	if readerVerb(cmd) || (darnengine.IsMergeTaskWriter(name) && mergeTaskVerb(cmd)) || benchAdmitted {
		return ""
	}
	if target, err := verbTargetDir(cmd); err == nil && inOwnSandboxRoot(target, name) {
		return ""
	}
	refusal := taskSessionRefusal(cmd.Name(), strandName)
	switch {
	case benchErr != nil:
		// The way forward would name the command just run, so the reason it is barred stands in its place.
		refusal += "; " + benchErr.Error()
	case darnengine.IsWriter(name) && slices.Contains(benchPairVerbs, cmd.Name()):
		refusal += benchPairWayForward(name.Slug)
	}
	return refusal
}
