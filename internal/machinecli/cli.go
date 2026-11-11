// cli.go builds the `lym machine` cobra group with its `name` verb and holds the seams the verb runs through.
// The RunCLI and RunCLIIn seams run that group with its output written to an io.Writer.

package machinecli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// deps holds the seams the verb runs through, so a test hands in a file path, a hostname and hubs of its own.
// A hand-built deps sets every field the path under test calls.
// The show calls only locate and hostName.
type deps struct {
	// locate returns the machine file's path and its writer's lock's path.
	// It also reports whether a directory resolved, and the error of a relative variable.
	locate func() (file, lock string, ok bool, err error)
	// hostName returns the hostname rule's name of this host.
	hostName func() (string, error)
	// hubs returns the paths of the hubs a sweep reaches through the machine record.
	hubs func() ([]string, error)
	// cwdHub returns the hub the directory lies in, and false when it lies in none.
	cwdHub func(dir string) (string, bool)
	// pull brings the board at boardDir level with its upstream and reports a skip with its detail.
	pull func(boardDir string) (fabricengine.BoltSkip, string, error)
	// unpushed reports whether the board at boardDir holds commits origin lacks.
	unpushed func(boardDir string) (bool, error)
	// readOwners reads the owners the local board at boardDir records.
	readOwners func(boardDir string) (boardengine.Owners, error)
	// setOwners makes the owner write on the board at boardDir.
	setOwners func(boardDir string, req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
}

// productionDeps returns the seams over the per-user machine file, this host's hostname and the real hubs.
func productionDeps() deps {
	return deps{
		locate:   machinename.FilePath,
		hostName: machinename.FromThisHost,
		hubs: func() ([]string, error) {
			dir, err := standalonestate.UserStateDir()
			if err != nil {
				return nil, fmt.Errorf("the per-user state directory of the machine record of hubs does not resolve: %w", err)
			}
			return liveHubs(dir)
		},
		cwdHub:     hubOfDir,
		pull:       pullBoard,
		unpushed:   func(boardDir string) (bool, error) { return fabricengine.NewBolt(boardDir).HasUnpushed() },
		readOwners: boardengine.ReadOwners,
		setOwners:  boardengine.SetOwners,
	}
}

// pullBoard brings the board at boardDir level with its upstream, with the skip options the owner write builds.
func pullBoard(boardDir string) (fabricengine.BoltSkip, string, error) {
	cfg := boardengine.ApplySkipEnv(boardengine.Config{Path: boardDir})
	opts := fabricengine.SyncOptions{SkipGit: cfg.SkipGit, SkipPush: cfg.SkipPush}
	return fabricengine.NewBolt(boardDir).Pull(opts, fabricengine.NewMutations(filepath.Dir(boardDir)))
}

// Command returns the cobra command for `lym machine`: the bare group and its name verb.
func Command() *cobra.Command {
	return newCommand(productionDeps())
}

// newCommand builds the `lym machine` group over d.
func newCommand(d deps) *cobra.Command {
	machine := &cobra.Command{
		Use:   "machine",
		Short: "show or set this machine's name",
		Long: `machine shows or sets this machine's name, the name lym records as the owner of
its runs, projects and the hub's prime.`,
		RunE: clihelp.GroupRunE,
	}
	machine.AddCommand(&cobra.Command{
		Use:         "name [<name>]",
		Short:       "show this machine's name, or set it and sweep the owners recorded under its former names",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `name with no argument shows this machine's name, whether it comes from the
machine file or from the hostname, the file's path and the former names the file
records. It writes nothing.

name <name> sets the name in the machine file, keeping the former names, then
rewrites, on every live hub of the machine record and the hub of the working
directory, the owners recorded under those former names to the new name. It
refuses a name that a board records as another machine's owner. A hub whose
sweep fails or keeps an owner makes it exit non-zero; re-run it with the same
name. The result lists the recoveries each state needs.

Example:
  lym machine name
  lym machine name osl46`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				clihelp.SetExit(cmd.Context(), showName(out, d))
				return nil
			}
			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), runSet(out, d, cwd, args[0]))
			return nil
		},
	})
	return machine
}

// RunCLI runs the `lym machine` command with args, writes its output to out, and returns its exit code.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the `lym machine` command with cwd as its working directory.
// An empty cwd reads the process directory instead, since seeding an empty directory into the context panics.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
