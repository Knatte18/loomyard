// cli.go builds the cobra command tree of the harden module: the sandbox verbs over a hardener run's throwaway hubs.

package hardencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// The names the sandbox's `.dev-bin` lym is found by, relative to the worktree root.
const (
	devBinDirName = ".dev-bin"
	devBinLymName = "lym"
)

// sandboxShortname is the repository short name a sandbox hub is cloned under.
const sandboxShortname = "sbx"

// singleRepoFlag is the flag of `lym harden sandbox new` that builds a one-repository hub.
const singleRepoFlag = "single-repo"

// notHardenerWayForward closes the refusal of a verb run outside a hardener run's worktree.
const notHardenerWayForward = "way forward: run lym harden sandbox in a hardener run's worktree"

// cliDeps holds the seams a test replaces.
type cliDeps struct {
	// Deploy rebuilds the `.dev-bin` lym from the worktree at worktreeRoot.
	Deploy func(worktreeRoot string) error
}

// Command returns the cobra command tree for the harden module.
func Command() *cobra.Command {
	return newCommand(cliDeps{Deploy: deployDevBin})
}

// deployDevBin rebuilds the `.dev-bin` lym with `go run ./tools/deploy -dev` in the worktree, and waits for it.
func deployDevBin(worktreeRoot string) error {
	cmd := exec.Command("go", "run", "./tools/deploy", "-dev")
	cmd.Dir = worktreeRoot
	logger.Info("hardencli: deploying the .dev-bin lym", "dir", worktreeRoot)
	if combined, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go run ./tools/deploy -dev: %w: %s", err, combined)
	}
	return nil
}

// newCommand builds the harden command tree over deps.
func newCommand(deps cliDeps) *cobra.Command {
	harden := &cobra.Command{
		Use:   "harden",
		Short: "work with the sandbox hubs of a hardener run",
		Long: `harden holds the verbs over the throwaway sandbox hubs a hardener run's live
tests run against. They run in the task worktree of a hardener run.`,
		RunE: clihelp.GroupRunE,
	}
	sandbox := &cobra.Command{
		Use:   "sandbox",
		Short: "make, list or clear the sandbox hubs of the worktree",
		Long: `sandbox makes, lists and clears the throwaway hubs under the worktree's sandbox
root. Each verb refuses outside a hardener run's worktree; clear also runs where
the root exists.`,
		RunE: clihelp.GroupRunE,
	}
	sandbox.AddCommand(newVerb(deps), listVerb(), clearVerb())
	harden.AddCommand(sandbox)
	return harden
}

// sandboxTarget is the worktree a verb works in and the sandbox root under it.
type sandboxTarget struct {
	location *lymcwd.Location
	root     string
}

// resolveTarget resolves the worktree the verb runs in and admits it.
// A hardener run's worktree is admitted, and so is, for a clear, any worktree whose sandbox root exists.
// It prints the refusal and aborts, and returns false, otherwise.
func resolveTarget(cmd *cobra.Command, clear bool) (sandboxTarget, bool) {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	refuse := func(err error) (sandboxTarget, bool) {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return sandboxTarget{}, false
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return refuse(err)
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return refuse(err)
	}
	target := sandboxTarget{location: location, root: hardenshed.SandboxRoot(location.WorktreePath())}
	seed, found, err := shedrun.ReadSeed(location, shedrun.SelfRunID)
	if err != nil {
		return refuse(err)
	}
	if found && seed.Recipe == shedrun.RecipeHardener {
		return target, true
	}
	if clear {
		if _, err := os.Stat(target.root); err == nil {
			return target, true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return refuse(err)
		}
	}
	return refuse(fmt.Errorf("lym harden sandbox: the worktree %s holds no hardener run; %s", location.WorktreeName, notHardenerWayForward))
}

// newVerb builds `lym harden sandbox new`.
func newVerb(deps cliDeps) *cobra.Command {
	var singleRepo bool
	cmd := &cobra.Command{
		Use:         "new",
		Short:       "build a throwaway sandbox hub under the worktree's sandbox root",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `new builds a throwaway hub under the worktree's sandbox root, with no GitHub
remote, and records it in the root's hub list. By default the hub is a
two-repository hub cloned from two local bare repositories; with --single-repo
it is a one-repository hub cloned from one local bare repository, which then
holds the code on its branches and the records and board under refs/lym/. Its
prime records commit a loom.yaml with Tier 2 self-reporting off. The worktree's
.dev-bin lym is rebuilt only when it is not built from the worktree's HEAD or is
modified. It prints the hub's path, the absolute path of the .dev-bin lym and
whether the hub is a one-repository hub.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			target, ok := resolveTarget(cmd, false)
			if !ok {
				return nil
			}
			hub, err := hardenshed.NewSandbox(sandboxDeps(target, deps, singleRepo))
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(cmd.Context(), 1)
				return nil
			}
			output.Ok(cmd.OutOrStdout(), map[string]any{"hub": hub.Hub, "lym": hub.Lym, "single_repo": singleRepo})
			return nil
		},
	}
	cmd.Flags().BoolVar(&singleRepo, singleRepoFlag, false, "build a one-repository hub from one local bare repository, its records under refs/lym/")
	return cmd
}

// listVerb builds `lym harden sandbox list`.
func listVerb() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "list the sandbox hubs under the worktree's sandbox root",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long:        `list prints the hubs the worktree's sandbox root records, in the order they were made.`,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			target, ok := resolveTarget(cmd, false)
			if !ok {
				return nil
			}
			hubs, err := hardenshed.ListSandbox(target.root)
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(cmd.Context(), 1)
				return nil
			}
			output.Ok(cmd.OutOrStdout(), map[string]any{"hubs": append([]string{}, hubs...)})
			return nil
		},
	}
}

// clearVerb builds `lym harden sandbox clear`.
func clearVerb() *cobra.Command {
	return &cobra.Command{
		Use:         "clear",
		Short:       "end what the sandbox hubs left running and empty the sandbox root",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `clear ends the processes the sandbox hubs registered, stops the reed servers of
the hubs the root's list records, ends the processes that name the root, and
removes the root's contents. A hub made outside the root is outside it. It
prints what it ended.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			target, ok := resolveTarget(cmd, true)
			if !ok {
				return nil
			}
			reapDeps, err := loomcli.HardenReapDeps(target.location, "lym harden sandbox clear", hardenshed.ReapClear)
			if err == nil {
				var ended []hardenshed.StrayEntry
				if ended, err = hardenshed.Reap(reapDeps); err == nil {
					output.Ok(cmd.OutOrStdout(), map[string]any{"root": target.root, "ended": append([]hardenshed.StrayEntry{}, ended...)})
					return nil
				}
			}
			output.Err(cmd.OutOrStdout(), fmt.Sprintf("%v; way forward: fix the cause, then run lym harden sandbox clear again", err))
			clihelp.Abort(cmd.Context(), 1)
			return nil
		},
	}
}

// sandboxDeps builds the seams NewSandbox needs for the worktree of target.
// The hub is cloned over two local bares, or over one with singleRepo.
// Its prime records commit the sandbox config override.
func sandboxDeps(target sandboxTarget, deps cliDeps, singleRepo bool) hardenshed.SandboxDeps {
	worktreeRoot := target.location.WorktreePath()
	lymName := devBinLymName
	if runtime.GOOS == "windows" {
		lymName += ".exe"
	}
	devBinLym := filepath.Join(worktreeRoot, devBinDirName, lymName)
	var primeCwd string
	sandbox := hardenshed.SandboxDeps{
		Root: target.root,
		Seed: fabricengine.SeedLocalBares,
		Clone: func(codeBare, recordsBare, container string) (string, error) {
			hubPath, cloneCwd, err := fabriccli.CloneAndWireFromLocalBares(container, codeBare, recordsBare, sandboxShortname)
			primeCwd = cloneCwd
			return hubPath, err
		},
		CommitOverride: func(string) error { return commitSandboxConfig(primeCwd) },
		DevBinLym:      devBinLym,
		Head: func() (string, error) {
			handle, err := fabricengine.Open(target.location)
			if err != nil {
				return "", err
			}
			return handle.HeadSHA()
		},
		Revision: func(path string) (buildvcs.Identity, error) {
			identity, err := buildvcs.OfFile(path)
			if errors.Is(err, fs.ErrNotExist) {
				return buildvcs.Identity{}, nil
			}
			return identity, err
		},
		Deploy: func() error { return deps.Deploy(worktreeRoot) },
	}
	if singleRepo {
		sandbox.Seed = func(dir string) (string, string, error) {
			origin, err := fabricengine.SeedLocalOrigin(dir)
			return origin, "", err
		}
		sandbox.Clone = func(origin, _, container string) (string, error) {
			hubPath, cloneCwd, err := fabriccli.CloneAndWireFromLocalOrigin(container, origin, sandboxShortname)
			primeCwd = cloneCwd
			return hubPath, err
		}
	}
	return sandbox
}

// commitSandboxConfig writes the sandbox's loom.yaml override into the prime at primeCwd and commits it on its records.
// The override sets loom.yaml's friction key empty, which turns Tier 2 self-reporting off in the sandbox hub.
func commitSandboxConfig(primeCwd string) error {
	prime, err := lymcwd.Resolve(primeCwd)
	if err != nil {
		return fmt.Errorf("resolve the sandbox hub's prime: %w", err)
	}
	pairs := []yamlengine.KV{{Key: "friction", Value: ""}}
	if _, err := configengine.Set(fabricengine.ConfigDirs(prime), loomengine.ConfigSpec(), configengine.LayerWorktree, pairs); err != nil {
		return fmt.Errorf("write the sandbox hub's loom.yaml: %w", err)
	}
	_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), prime, []string{configengine.ConfigFileRel("loom")}, "sandbox: turn Tier 2 self-reporting off", fabricengine.EnvSyncOptions())
	return err
}
