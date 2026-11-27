// cli.go builds the `lym machine` cobra group with its `name` and `start` verbs.

package machinecli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/termwindow"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// forgetFormerFlag is the name verb's flag that drops a former name.
const forgetFormerFlag = "forget-former"

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
	pull func(boardDir string) (fabriclayout.BoltSkip, string, error)
	// unpushed reports whether the board at boardDir holds commits origin lacks.
	unpushed func(boardDir string) (bool, error)
	// readOwners reads the owners the local board at boardDir records.
	readOwners func(boardDir string) (boardengine.Owners, error)
	// setOwners makes the owner write on the board at boardDir.
	setOwners func(boardDir string, req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	// register registers a machine in the board at boardDir and reports whether it wrote.
	register func(boardDir string, reg boardengine.Registration) (bool, error)
	// newID returns a new random machine id.
	newID func() (string, error)
	// references lists the records of the hub at hub that still name name.
	references func(hub, name string) ([]formerReference, error)
	// liveHubEntries lists the hubs the machine record lists as live, with their shortnames.
	liveHubEntries func() ([]hubregistry.Entry, error)
	// lookupHub returns the path of the one hub the machine record holds under shortname.
	lookupHub func(shortname string) (string, error)
	// hubStart returns the prime of the hub at hub and the start deps over it, whose owner reads go through the registry.
	hubStart func(hub string) (*lymcwd.Location, dobbyengine.StartDeps, error)
	// identity returns this machine's identity, which reads a recorded machine name as this machine or another.
	identity func() (machinename.Identity, error)
	// lymBinary returns the path of the running lym binary, which the children, the terminals and the launchers run.
	lymBinary func() (string, error)
	// beamStart runs `<lymPath> <args>` as a child in dir and returns the worktree entries its `beam start` envelope reports.
	beamStart func(lymPath, dir string, args []string) ([]beamStartEntry, error)
	// openWorkspace opens the VS Code workspace file at path.
	openWorkspace func(path string) error
	// terminal returns the opener of a terminal window attached to a worktree's reed session, or why none resolves.
	terminal func() (func(dir, title, lymPath string) error, error)
	// desktop returns this machine's desktop for the launchers, or why it has none.
	desktop func() (desktoplauncher.Desktop, error)
}

// productionDeps returns the seams over the per-user machine file, this host's hostname and the real hubs.
func productionDeps() deps {
	return deps{
		locate:   machinename.FilePath,
		hostName: machinename.FromThisHost,
		hubs: func() ([]string, error) {
			dir, err := recordDir()
			if err != nil {
				return nil, err
			}
			return liveHubs(dir)
		},
		cwdHub:     hubOfDir,
		pull:       pullBoard,
		unpushed:   func(boardDir string) (bool, error) { return fabricengine.NewBolt(boardDir).HasUnpushed() },
		readOwners: boardengine.ReadOwners,
		setOwners:  boardengine.SetOwners,
		register:   boardengine.RegisterMachine,
		newID:      machinename.NewID,
		references: hubReferences,
		liveHubEntries: func() ([]hubregistry.Entry, error) {
			dir, err := recordDir()
			if err != nil {
				return nil, err
			}
			return liveHubEntries(dir)
		},
		lookupHub: lookupHub,
		hubStart: func(hub string) (*lymcwd.Location, dobbyengine.StartDeps, error) {
			prime, err := primeOfHub(hub)
			if err != nil {
				return nil, dobbyengine.StartDeps{}, err
			}
			return prime, hubgeom.DobbyStart(prime), nil
		},
		identity:      machinename.ReadIdentity,
		lymBinary:     selfexec.LymBinary,
		beamStart:     runBeamStart,
		openWorkspace: func(path string) error { return vscode.Launch(path) },
		terminal: func() (func(dir, title, lymPath string) error, error) {
			launcher, err := termwindow.Resolve()
			if err != nil {
				return nil, err
			}
			return launcher.OpenAttach, nil
		},
		desktop: desktoplauncher.Locate,
	}
}

// pullBoard brings the board at boardDir level with its upstream, with the skip options the owner write builds.
func pullBoard(boardDir string) (fabriclayout.BoltSkip, string, error) {
	cfg := boardengine.ApplySkipEnv(boardengine.Config{Path: boardDir})
	opts := fabricengine.SyncOptions{SkipGit: cfg.SkipGit, SkipPush: cfg.SkipPush}
	return fabricengine.NewBolt(boardDir).Pull(opts, fabricengine.NewMutations(filepath.Dir(boardDir)))
}

// Command returns the cobra command for `lym machine`: the bare group and its name and start verbs.
func Command() *cobra.Command {
	return newCommand(productionDeps())
}

// newCommand builds the `lym machine` group over d.
func newCommand(d deps) *cobra.Command {
	machine := &cobra.Command{
		Use:   "machine",
		Short: "show or set this machine's name, or bring back the hubs it owns",
		Long: `machine shows or sets this machine's name, the name lym records as the owner of
its runs, projects and the hub's prime, and brings back the hubs this machine owns
after a crash or a stop.`,
		RunE: clihelp.GroupRunE,
	}
	start := &cobra.Command{
		Use:         "start [<shortname>]",
		Short:       "bring back every hub this machine owns, or the hub shortname names",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `start brings back what this machine owns in every hub the machine record lists
as live, or in the one hub shortname names, from any directory. In each hub it
pulls the board and starts, through lym beam start in the hub's prime, the prime
and every beam this machine owns: the prime with --all when this machine owns it,
else each owned beam by its slug. A prime or beam another machine owns is
skipped, naming that machine, and so is one with no recorded owner: the verb
claims nothing, never runs lym s and resumes no run; the orchs and dobbies take
the runs up.

In each hub where it started a worktree it opens the hub's VS Code workspace file
and, per started worktree, a terminal window running lym reed attach. Where no
terminal launcher resolves, each started worktree carries its attach command in
the result instead. A standing adopt hold is printed with its machine and
subjects, and for this machine's own hold with the lym adopt re-run.

It keeps the desktop launchers that run it, one per hub and one for every hub: a
hub's launcher is written while this machine owns anything in the hub and removed
once it owns nothing there, a launcher whose hub the machine record no longer
lists as live is removed, and the all-hubs launcher goes with the last hub's.

A hub that does not read, or a worktree that does not start, makes it exit
non-zero; fix the cause it names and run it again.

Example:
  lym machine start
  lym machine start ly`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			shortname := ""
			if len(args) == 1 {
				shortname = args[0]
			}
			clihelp.SetExit(cmd.Context(), runStart(cmd.OutOrStdout(), d, shortname))
			return nil
		},
	}
	machine.AddCommand(start)
	name := &cobra.Command{
		Use:         "name [<name>]",
		Short:       "show this machine's name, set it and sweep the owners recorded under its former names, or forget a former name",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `name with no argument shows this machine's name, whether it comes from the
machine file or from the hostname, the file's path and the former names the file
records. It writes nothing.

name <name> sets the name in the machine file, keeping the former names and the
machine id, then registers the new name on every live hub of the machine record
and the hub of the working directory, with the old name among the machine's
former names, and rewrites there the owners recorded under those former names to
the new name. It refuses a name that a board records as another machine's owner
and a name another registered machine holds. A hub whose registration or sweep
fails or keeps an owner makes it exit non-zero; re-run it with the same name. The
result lists the recoveries each state needs.

name --forget-former <name> drops a former name the machine file holds: the name
leaves the file's names and joins its forgotten names, the id stays, and this
machine's entry in the registry of every swept hub drops it, so a registration
that another machine's current name refused goes through. It lists every record
it can see that still names the dropped name, each with the lym adopt subject
that takes it back once the registration has landed: the owners on each hub's
local board, and the takeover entries and unfinished cancel intents in the batten
records of the runs whose pair stands on this machine. It changes only this
machine's file and its own registry entry: it writes no other machine's entry and
no owner, so a record under the dropped name reads as the other machine from the
registration on. A hub whose registration fails makes it exit non-zero; re-run it
with the same name.

Example:
  lym machine name
  lym machine name osl46
  lym machine name --forget-former osl-1046`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			forgetName, _ := cmd.Flags().GetString(forgetFormerFlag)
			forgetting := cmd.Flags().Changed(forgetFormerFlag)
			switch {
			case forgetting && len(args) > 0:
				clihelp.SetExit(cmd.Context(), output.Err(out, "name takes either a name to set or --forget-former, not both; way forward: run them as two commands"))
				return nil
			case len(args) == 0 && !forgetting:
				clihelp.SetExit(cmd.Context(), showName(out, d))
				return nil
			}
			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if forgetting {
				clihelp.SetExit(cmd.Context(), runForget(out, d, cwd, forgetName))
				return nil
			}
			clihelp.SetExit(cmd.Context(), runSet(out, d, cwd, args[0]))
			return nil
		},
	}
	name.Flags().String(forgetFormerFlag, "", "drop `name` from this machine's former names, in its file and its registry entry")
	machine.AddCommand(name)
	return machine
}
