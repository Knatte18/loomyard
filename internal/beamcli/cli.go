// cli.go builds the beam cobra command with its create, promote and land verbs and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package beamcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the beam module.
func Command() *cobra.Command {
	beam := &cobra.Command{
		Use:   "beam",
		Short: "make, promote, land and start beams, the long-lived pairs that group board work, and show their dobbies",
		Long: `A beam is a long-lived pair that groups board work: its tasks fork from the
beam's own branch and land back into it, it can run an orch of its own, and its
dobby drives the runs of its tasks. Every verb runs from the hub's prime worktree
and from any beam, and acts on the prime's location.`,
		RunE: clihelp.GroupRunE,
	}
	beam.AddCommand(createCmd(), promoteCmd(), landCmd(), startCmd(), statusCmd(), dobbyCmd())
	return beam
}

// statusResult is the envelope of `beam status`: one entry per local dobby, the prime's first.
type statusResult struct {
	Dobbies []statusEntry `json:"dobbies"`
}

// statusEntry is one dobby's view in `beam status`: the structured view and the lines a beam's window draws for it, or why it did not read.
type statusEntry struct {
	Worktree string            `json:"worktree"`
	View     *dobbyengine.View `json:"view,omitempty"`
	Lines    []string          `json:"lines"`
	Error    string            `json:"error,omitempty"`
}

// statusCmd returns the `beam status` verb.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "show the dobby of the prime and of every local beam: alive, runs driven, tasks waiting and why",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Prints, once, what a beam's window draws for a dobby, for the hub's prime and
for every local beam: whether the dobby is alive and its max_runs, each run it
drives with its state, stage, time in the stage and last pass, and each released task it has not
started with what the task waits on -- open dependencies, another machine's
ownership, a pause, a refused start and its retry, or max_runs. Each entry
carries the structured view and its rendered lines. It reads files only.

It runs from the prime and from any beam.

Example:
  lyx beam status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerb(cmd, func(_ context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				return dobbyStatus(prime, time.Now())
			})
		},
	}
}

// dobbyStatus reads the view of the prime's dobby and of the dobby of every local beam at now.
// A view that does not read is reported in its entry and joins the returned error, and the others still read.
func dobbyStatus(prime *lyxcwd.Location, now time.Time) (statusResult, error) {
	beams, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return statusResult{}, fmt.Errorf("lyx beam status: list the local beams: %w", err)
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return statusResult{}, fmt.Errorf("lyx beam status: locate the pause record: %w", err)
	}
	cells := battencli.NewDobbyRunCells(prime, pause.RecordPath)
	worktrees := []string{prime.WorktreeName}
	for _, beam := range beams {
		worktrees = append(worktrees, beam.Slug)
	}
	result := statusResult{Dobbies: make([]statusEntry, 0, len(worktrees))}
	var failures []error
	for _, worktree := range worktrees {
		label := worktree
		if worktree == prime.WorktreeName {
			label = dobbyengine.PrimeLabel
		}
		entry := statusEntry{Worktree: label, Lines: []string{}}
		view, err := readDobbyView(prime, worktree, now)
		if err != nil {
			entry.Error = err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", label, err))
		} else {
			entry.View = &view
			entry.Lines = battencli.DobbyLines(label, view, cells.Of(view.Runs, now), now)
		}
		result.Dobbies = append(result.Dobbies, entry)
	}
	if err := errors.Join(failures...); err != nil {
		return result, fmt.Errorf("lyx beam status: %w; way forward: fix the cause each names, then run `lyx beam status` again", err)
	}
	return result, nil
}

// readDobbyView reads the view of the dobby of the worktree named worktree at now.
func readDobbyView(prime *lyxcwd.Location, worktree string, now time.Time) (dobbyengine.View, error) {
	reader, err := battencli.NewDobbyViewReader(prime, worktree)
	if err != nil {
		return dobbyengine.View{}, err
	}
	return reader.Read(now)
}

// startResult is the envelope of `beam start`: the outcome per worktree.
type startResult struct {
	Dobbies []startEntry `json:"dobbies"`
}

// startEntry is one worktree's outcome of `beam start`: started or live, else why the dobby was refused or failed.
type startEntry struct {
	Worktree string `json:"worktree"`
	Outcome  string `json:"outcome"`
	Detail   string `json:"detail,omitempty"`
}

// startCmd returns the `beam start` verb.
func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "start",
		Short:       "start the dobby of the prime and of every beam this machine owns",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Starts the dobby, the daemon that starts a beam's released tasks and drives their
batten runs, for the hub's prime and for every local beam this machine owns. A dobby
that is live already is left alone, so the verb is idempotent. A prime no owner is
recorded for is claimed for this machine first; a prime another machine owns is
reported with its owner, and "lyx adopt --prime" takes it over.

It runs from the prime and from any beam, and prints "started" or "live" per worktree.

Example:
  lyx beam start`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerb(cmd, func(_ context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				report, err := dobbyengine.EnsureLive(hubgeom.DobbyStart(prime))
				result := startResult{Dobbies: make([]startEntry, 0, len(report.Entries))}
				for _, entry := range report.Entries {
					result.Dobbies = append(result.Dobbies, startEntry{Worktree: entry.Worktree, Outcome: entry.Outcome, Detail: entry.Detail})
				}
				if err != nil {
					return result, fmt.Errorf("lyx beam start: %w; way forward: fix the cause each names, then run `lyx beam start` again", err)
				}
				return result, nil
			})
		},
	}
}

// dobbyCmd returns the hidden `beam dobby` verb, which `lyx beam start` launches as a detached process.
func dobbyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "dobby <worktree-root>",
		Short:       "run the dobby daemon of the worktree at <worktree-root>",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `Runs the dobby of the hub's prime or of a local beam until it is signalled or another
machine owns the beam. It is launched by "lyx beam start", never by hand.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			clihelp.SetExit(cmd.Context(), runDobby(ctx, cmd.OutOrStdout(), args[0]))
			return nil
		},
	}
}

// runDobby places the worktree at root, registers this process as its dobby and runs it until ctx ends, and prints the outcome.
// A worktree dobby cannot be placed is refused before anything is registered or locked, and a dobby that is live already ends the call at once.
func runDobby(ctx context.Context, out io.Writer, root string) int {
	prime, geom, err := placeDobby(root)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if live, err := dobbyengine.Live(geom); err == nil && live {
		return output.Ok(out, map[string]any{"already_running": true})
	}
	if err := registerDobby(fabricengine.RunProcDir(prime.HubPath), geom); err != nil {
		return output.Err(out, fmt.Sprintf("lyx beam dobby: register the dobby: %v", err))
	}
	deps, err := hubDobbyDeps(prime, geom, out)
	if err != nil {
		return output.Err(out, fmt.Sprintf("lyx beam dobby: wire the dobby over the hub: %v", err))
	}
	switch err := dobbyengine.Run(ctx, geom, deps); {
	case errors.Is(err, dobbyengine.ErrDobbyRunning):
		return output.Ok(out, map[string]any{"already_running": true})
	case err != nil:
		return output.Err(out, fmt.Sprintf("lyx beam dobby: %v", err))
	}
	return output.Ok(out, map[string]any{"stopped": true})
}

// placeDobby resolves the worktree at root into the prime's location and the dobby's geometry.
// Only the prime and a local pair whose origin record carries the beam mark are placed, as `lyx beam start` reads them; any other directory is refused naming that verb.
func placeDobby(root string) (*lyxcwd.Location, dobbyengine.Geometry, error) {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("lyx beam dobby: "+format+"; way forward: run `lyx beam start`, which starts the dobby of the prime and of each beam this machine owns", args...)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, dobbyengine.Geometry{}, refuse("resolve %s: %v", root, err)
	}
	location, err := lyxcwd.ResolveWorktree(absolute)
	if err != nil {
		return nil, dobbyengine.Geometry{}, refuse("%s is not a worktree of a hub: %v", absolute, err)
	}
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return nil, dobbyengine.Geometry{}, refuse("%s is not a worktree a dobby drives: %v", absolute, err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, dobbyengine.Geometry{}, refuse("cannot tell the prime of the hub of %s: %v", absolute, err)
	}
	prime := location
	if location.WorktreeName != primeName {
		origin, found, err := fabricengine.ReadOrigin(location)
		if err != nil {
			return nil, dobbyengine.Geometry{}, refuse("cannot read the origin record of %s: %v", location.WorktreeName, err)
		}
		if !found || !origin.Beam {
			return nil, dobbyengine.Geometry{}, refuse("%s is neither the prime nor a beam", location.WorktreeName)
		}
		if prime, err = lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName)); err != nil {
			return nil, dobbyengine.Geometry{}, refuse("resolve the prime of the hub: %v", err)
		}
	}
	geom, err := hubgeom.DobbyGeometry(prime, location.WorktreeName)
	if err != nil {
		return nil, dobbyengine.Geometry{}, refuse("%v", err)
	}
	return prime, geom, nil
}

// createCmd returns the `beam create` verb.
func createCmd() *cobra.Command {
	var anchor string
	cmd := &cobra.Command{
		Use:         "create <slug>",
		Short:       "make a project entry a beam: record its owner and make its pair",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turns the board's project entry <slug> into a beam. The entry's title, brief and
labels are board writing, so the verb makes no entry: make the project first with
"lyx board upsert". A task becomes a beam with "lyx beam promote".

Steps, in order, each skipped when its result is already there, so a re-run
finishes a stopped create:
  1. record this machine as the project's owner with the beam flag
  2. make the pair from the prime, or from the parent beam of a nested beam,
     and mark it as a beam; an existing pair first has its anchor settled, then is marked
  3. spawn the pair's push
  4. regenerate the workspace files

With --anchor <dir> the new pair takes <dir> of the code checkout as its anchor
instead of its parent's; the directory must be tracked in the commit the beam forks
from. The pair's records content moves there under a move record, and a killed create
is finished by running the same command again. A pair that already exists sits at
its own anchor, which "lyx fabric anchor" moves.

The envelope names the beam's worktree, where "lyx orch start" gives it an orch;
the verb starts none.

Example:
  lyx beam create payments
  lyx beam create payments --anchor services/payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(_ context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubCreateDeps(prime)
				if err != nil {
					return nil, err
				}
				return create(deps, args[0], anchor)
			})
		},
	}
	cmd.Flags().StringVar(&anchor, "anchor", "", "a path relative to the code repo root, validated in the commit the beam forks from; without it the beam inherits its parent's anchor")
	return cmd
}

// landCmd returns the `beam land` verb.
func landCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "land <slug>",
		Short:       "land a beam into its parent with a short batten run",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Ends the beam <slug> with a landing run over batten and the beam recipe: the run
writes the beam's description, merges the beam into its parent worktree's branch,
and removes the beam's pair, whose board entry is then done. The landing starts as a
detached first pass, driven on by the beam's parent worktree's dobby.

It runs from the prime and from any beam but the beam itself, whose pair the landing
removes, and refuses while work remains under the beam:
  - an open entry whose parent is the beam, through grouping projects
  - a local pair forked from the beam
  - the beam's orch strand or its watcher live ("lyx orch stop" in the beam's worktree)
Each refusal names its way forward. Land or cancel the members first.

Example:
  lyx beam land payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(_ context.Context, acting string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubLandDeps(prime)
				if err != nil {
					return nil, err
				}
				return land(deps, acting, args[0])
			})
		},
	}
}

// promoteCmd returns the `beam promote` verb.
func promoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "promote <slug>",
		Short:       "turn a spawned task into a beam, keeping its pair and its commits",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turns the started task <slug> into a beam, keeping its pair and the commits on its
branch. A task that was never started has no pair to keep: make a project entry
with "lyx board upsert" and run "lyx beam create" on it. On a project entry the
verb runs the steps of "lyx beam create".

Steps, in order, each skipped when its result is already there, so a re-run
finishes a stopped promote:
  1. end the task's run: its processes and strands, with the pair kept
  2. mark the pair as a beam and spawn its push
  3. remove the run's records and clear its claim
  4. reset the pair's records to a fresh fork of its parent's, and spawn the push
  5. record this machine as the owner and turn the entry into a beam project
  6. regenerate the workspace files

The run ends for good: its code commits stay on the pair's branch, and its
discussion, plan and run records stay reachable in the records history.
The verb refuses a task another machine owns or this machine has no pair for
("lyx adopt"), a task an open task depends on ("lyx board set-deps"), a task whose
run already reached its parent ("lyx batten run"), and a root pair.

Example:
  lyx beam promote payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(ctx context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubPromoteDeps(prime)
				if err != nil {
					return nil, err
				}
				return promote(ctx, deps, args[0])
			})
		},
	}
}

// runVerb resolves the prime from the command's cwd, runs the verb over it and prints the verb's result in the standard envelope.
// The verb is told the name of the worktree it was run from beside the prime's location.
// A verb's error is printed with the result it returned beside it.
func runVerb(cmd *cobra.Command, run func(ctx context.Context, acting string, prime *lyxcwd.Location) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	fail := func(err error) error {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return fail(err)
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return fail(err)
	}
	prime, err := battencli.ResolveBattenPrime(location)
	if err != nil {
		return fail(err)
	}
	result, err := run(ctx, location.WorktreeName, prime)
	fields := map[string]any{"result": result}
	if err != nil {
		clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), fields))
		return nil
	}
	clihelp.SetExit(ctx, output.Ok(out, fields))
	return nil
}

// RunCLI is the public seam for the beam module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
