// cli.go builds the beam cobra command with its create, ready, land, start and stop verbs and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

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

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the beam module.
func Command() *cobra.Command {
	beam := &cobra.Command{
		Use:   "beam",
		Short: "make, flag ready, land, start and stop beams, the long-lived pairs that group board work, and show their dobbies and orchs",
		Long: `A beam is a long-lived pair that groups board work: its tasks fork from the
beam's own branch and land back into it, and it is one unit of two parts, its dobby
and its orch, that run on the machine owning it. The dobby drives the runs of the
beam's tasks, and the orch is the session that works the beam with the operator.
"lym beam start" brings both up and "lym beam stop" takes both down. Create and
status run from the hub's prime worktree and from any beam, ready and land from
anywhere in the hub, and all act on the prime's location.`,
		RunE: clihelp.GroupRunE,
	}
	beam.AddCommand(createCmd(), readyCmd(), landCmd(), startCmd(), stopCmd(), statusCmd(), dobbyCmd())
	return beam
}

// statusResult is the envelope of `beam status`: one entry per local beam, the prime's first, and the adopt check when the hub holds a record of one.
type statusResult struct {
	Dobbies    []statusEntry       `json:"dobbies"`
	AdoptCheck *adoptcli.CheckView `json:"adopt_check,omitempty"`
}

// statusEntry is one beam's view in `beam status`: the dobby's structured view and the lines a beam's window draws for it, and the orch's view, or why either did not read.
type statusEntry struct {
	Worktree  string            `json:"worktree"`
	View      *dobbyengine.View `json:"view,omitempty"`
	Lines     []string          `json:"lines"`
	Error     string            `json:"error,omitempty"`
	Orch      *orchView         `json:"orch,omitempty"`
	OrchError string            `json:"orch_error,omitempty"`
}

// statusCmd returns the `beam status` verb.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "show the dobby and the orch of the prime and of every local beam",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Prints, once, what a beam's window draws for a dobby, for the hub's prime and
for every local beam: whether the dobby is alive and its max_runs, each run it
drives with its state, stage, time in the stage and last pass, and each released task it has not
started with what the task waits on -- open dependencies, another machine's
ownership, a pause, a refused start and its retry, or max_runs. Each entry
carries the structured view and its rendered lines, and the beam's orch: whether its
strand is live, whether its watcher is live and the session id it recorded.
The adopt_check section shows the check a lym adopt started: its state, the tier in flight, the
tiers done and the paths of its log and output, and is absent when the hub holds no record of one.
It starts and changes nothing; its one write is the empty orch directory its watcher probe creates.

It runs from the prime and from any beam.

Example:
  lym beam status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerb(cmd, func(_ context.Context, _ string, prime *lymcwd.Location) (any, error) {
				return dobbyStatus(prime, time.Now())
			})
		},
	}
}

// dobbyStatus reads the view of the prime's dobby and orch and of the dobby and orch of every local beam at now.
// A view that does not read is reported in its entry and joins the returned error, and the others still read.
func dobbyStatus(prime *lymcwd.Location, now time.Time) (statusResult, error) {
	beams, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return statusResult{}, fmt.Errorf("lym beam status: list the local beams: %w", err)
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return statusResult{}, fmt.Errorf("lym beam status: locate the pause record: %w", err)
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
		orch, err := readOrchView(prime, worktree)
		if err != nil {
			entry.OrchError = err.Error()
			failures = append(failures, fmt.Errorf("%s: orch: %w", label, err))
		} else {
			entry.Orch = &orch
		}
		result.Dobbies = append(result.Dobbies, entry)
	}
	check, err := adoptcli.ReadCheckView(prime.HubPath)
	if err != nil {
		failures = append(failures, fmt.Errorf("adopt check: %w", err))
	}
	result.AdoptCheck = check
	if err := errors.Join(failures...); err != nil {
		return result, fmt.Errorf("lym beam status: %w; way forward: fix the cause each names, then run `lym beam status` again", err)
	}
	return result, nil
}

// readDobbyView reads the view of the dobby of the worktree named worktree at now.
func readDobbyView(prime *lymcwd.Location, worktree string, now time.Time) (dobbyengine.View, error) {
	reader, err := battencli.NewDobbyViewReader(prime, worktree)
	if err != nil {
		return dobbyengine.View{}, err
	}
	return reader.Read(now)
}

// startResult is the envelope of `beam start`: the outcome per beam.
type startResult struct {
	Beams []startEntry `json:"beams"`
}

// startEntry is one beam's outcome of `beam start`: the dobby's outcome and detail, what the orch start did, and the error that stopped it.
// Reconcile, FailedPushes, OwnerPauses and Mutations are what the one prepared beam gathered for the hub, and are absent on the others.
type startEntry struct {
	Worktree         string                     `json:"worktree"`
	Outcome          string                     `json:"outcome,omitempty"`
	Detail           string                     `json:"detail,omitempty"`
	Action           string                     `json:"action,omitempty"`
	PromptSource     string                     `json:"prompt_source,omitempty"`
	AbandonedSession string                     `json:"abandoned_session,omitempty"`
	Warnings         []string                   `json:"warnings,omitempty"`
	Error            string                     `json:"error,omitempty"`
	Reconcile        *fabricengine.OriginReport `json:"reconcile,omitempty"`
	FailedPushes     []map[string]any           `json:"failed_pushes,omitempty"`
	OwnerPauses      []map[string]string        `json:"owner_pauses,omitempty"`
	Mutations        []fabricengine.Mutation    `json:"mutations,omitempty"`
}

// startCmd returns the `beam start` verb.
func startCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:         "start [<slug>]",
		Short:       "bring a beam up: its dobby and its orch",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Brings a beam up: its dobby, the daemon that starts the beam's released tasks and
drives their batten runs, and its orch, the session that works the beam with the
operator. With no slug it acts on the worktree it runs in, the prime or a beam; the slug
"root" names the prime, and any other slug names that beam from anywhere in the hub.
With --all it acts on the prime and on every local beam this machine owns.

The orch resumes the session it recorded, else starts from the last handoff note, else
from a fresh prompt. A dobby or orch that is live already is left alone, so the verb is
idempotent. Before anything starts it reconciles the hub's config once after a binary
change and the hub's branches against origin, claims a prime no owner is recorded for,
and pauses the runs another machine owns. A beam another machine owns, or one whose
landing hold stands, is refused with its way forward; "lym adopt <beam>" takes a beam
over, and "lym adopt root" the prime.

It refuses a task pair, naming the beam it belongs to, and a slug with no pair here.
It prints the outcome per beam and exits non-zero when any beam failed to come up.

Example:
  lym beam start
  lym beam start payments
  lym beam start --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			result, err := start(cmd, slug, all)
			return finishVerb(cmd, result, err)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "start the prime and every local beam this machine owns")
	return cmd
}

// start runs `beam start` for slug, or for every owned beam with all.
// It resolves its targets before the reconcile, so a refusal of the target touches no state.
// A nil result is a refusal made before any beam was tried.
func start(cmd *cobra.Command, slug string, all bool) (*startResult, error) {
	if all && slug != "" {
		return nil, fmt.Errorf("lym beam start: a slug and --all are exclusive; way forward: drop the slug to start every owned beam, or drop --all to start %s alone", slug)
	}
	location, err := actingLocation(cmd, "start")
	if err != nil {
		return nil, err
	}
	targets, err := startTargets(location, slug, all)
	if err != nil {
		return nil, err
	}
	if len(targets) > 0 {
		if geometry, inHub := hubgeom.ReconcileGeometry(targets[0].Location); inHub {
			if err := hubreconcile.Ensure(geometry, hubreconcile.Options{}); err != nil {
				return nil, fmt.Errorf("lym beam start: %w", err)
			}
		}
	}
	index := func() (string, error) { return clihelp.RenderIndex(cmd.Root(), clihelp.AudienceOperator), nil }
	result, err := startBeams(targets, func(target beamTarget, prepare bool) (orchcli.BeamUpReport, error) {
		return orchcli.BeamUp(target.Location, orchcli.BeamUpOptions{Prepare: prepare, Index: index})
	})
	if err != nil {
		return &result, fmt.Errorf("lym beam start: %w; way forward: fix the cause each names, then run `lym beam start` again", err)
	}
	return &result, nil
}

// startTargets resolves what `beam start` brings up: every beam this machine owns with all, else the one beam slug names.
func startTargets(location *lymcwd.Location, slug string, all bool) ([]beamTarget, error) {
	if !all {
		target, err := resolveBeamTarget(location, slug)
		if err != nil {
			return nil, err
		}
		return []beamTarget{target}, nil
	}
	primeOfHub, err := resolveBeamTarget(location, rootSlug)
	if err != nil {
		return nil, err
	}
	prime := primeOfHub.Location
	owned, err := dobbyengine.OwnedWorktrees(hubgeom.DobbyStart(prime))
	if err != nil {
		return nil, fmt.Errorf("lym beam start: %w", err)
	}
	targets, err := ownedTargets(prime, owned)
	if err != nil {
		return nil, fmt.Errorf("lym beam start: %w", err)
	}
	return targets, nil
}

// stopCmd returns the `beam stop` verb.
func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop [<slug>]",
		Short:       "take a beam down: its orch and its dobby",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Takes a beam down: removes its orch strand, after which the orch's watcher exits on
its own, and ends its dobby. With no slug it acts on the worktree it runs in, the prime
or a beam; the slug "root" names the prime, and any other slug names that beam from
anywhere in the hub. Each part stops only when it is live, so a second call stops
nothing, and the next "lym beam start" resumes the orch's recorded session.

It refuses a task pair, naming the beam it belongs to, and a slug with no pair here.
It prints what it stopped.

Example:
  lym beam stop
  lym beam stop payments`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			result, err := stop(cmd, slug)
			return finishVerb(cmd, result, err)
		},
	}
}

// stop runs `beam stop` for slug.
// A nil result is a refusal made before anything was stopped.
func stop(cmd *cobra.Command, slug string) (*orchcli.BeamDownReport, error) {
	location, err := actingLocation(cmd, "stop")
	if err != nil {
		return nil, err
	}
	target, err := resolveBeamTarget(location, slug)
	if err != nil {
		return nil, err
	}
	report, err := orchcli.BeamDown(target.Location)
	if err != nil {
		return &report, fmt.Errorf("lym beam stop: %w", err)
	}
	return &report, nil
}

// actingLocation resolves the worktree the command runs in, refusing outside a hub with the resolver's own error and where to run the verb.
func actingLocation(cmd *cobra.Command, verb string) (*lymcwd.Location, error) {
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("lym beam %s: %w", verb, err)
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, fmt.Errorf("lym beam %s: %w; way forward: run it from the hub's prime worktree or from a beam", verb, err)
	}
	return location, nil
}

// finishVerb prints a verb's result in the standard envelope and records its exit.
// A nil result is a refusal made before the verb did anything, and prints no result.
func finishVerb[T any](cmd *cobra.Command, result *T, err error) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	switch {
	case err != nil && result == nil:
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
	case err != nil:
		clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"result": result}))
	default:
		clihelp.SetExit(ctx, output.Ok(out, map[string]any{"result": result}))
	}
	return nil
}

// dobbyCmd returns the hidden `beam dobby` verb, which `lym beam start` launches as a detached process.
func dobbyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "dobby <worktree-root>",
		Short:       "run the dobby daemon of the worktree at <worktree-root>",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `Runs the dobby of the hub's prime or of a local beam until it is signalled or another
machine owns the beam. It is launched by "lym beam start", never by hand.`,
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
// A worktree dobby cannot be placed is refused before anything is registered or locked, and the registration waits for the lock, so a dobby that finds another one holding it ends the call with nothing registered.
func runDobby(ctx context.Context, out io.Writer, root string) int {
	return runDobbyWith(ctx, out, root, hubDobbyDeps, orchcli.NotifyFromScoped)
}

// runDobbyWith is runDobby over the seams that wire the dobby over the hub and post a notice to the nearest live orch.
// Each exit it takes before dobbyengine.Run leaves a line in the dobby's log, and one after a re-exec also a notice.
// Where selfexec execs, it moves the process's standard output and error onto the beam's `dobby.out`, and rotates that file once it holds the lock.
func runDobbyWith(
	ctx context.Context, out io.Writer, root string,
	wire func(prime *lymcwd.Location, geom dobbyengine.Geometry) (dobbyengine.Deps, error),
	notify func(from *lymcwd.Location, slug, program, line string) (bool, error),
) int {
	location, prime, geom, err := placeDobby(root)
	if err != nil {
		if location != nil {
			traceRefusedStart(location, err, notify)
		}
		return output.Err(out, err.Error())
	}
	if selfexecs() {
		if err := reopenStdio(geom.OutPath); err != nil {
			logger.Warn("beamcli: the dobby's output was not moved onto its output file", "path", geom.OutPath, "error", err)
		}
	}
	deps, err := wire(prime, geom)
	if err != nil {
		traceFailedWiring(geom, err, notify)
		return output.Err(out, fmt.Sprintf("lym beam dobby: wire the dobby over the hub: %v", err))
	}
	deps.Locked = func() error {
		if err := registerDobby(fabricengine.RunProcDir(prime.HubPath), geom); err != nil {
			return fmt.Errorf("register the dobby: %w", err)
		}
		if selfexecs() {
			rotateOutput(geom.OutPath)
		}
		return nil
	}
	switch err := dobbyengine.Run(ctx, geom, deps); {
	case errors.Is(err, dobbyengine.ErrDobbyRunning):
		return output.Ok(out, map[string]any{"already_running": true})
	case err != nil:
		return output.Err(out, fmt.Sprintf("lym beam dobby: %v", err))
	}
	return output.Ok(out, map[string]any{"stopped": true})
}

// placeDobby resolves the worktree at root into its own location, the prime's location and the dobby's geometry.
// Only the prime and a local pair whose origin record carries the beam mark are placed, as `lym beam start` reads them; any other directory is refused naming that verb.
// A refusal made once the worktree resolved still returns its location, which is nil for a refusal before that.
func placeDobby(root string) (*lymcwd.Location, *lymcwd.Location, dobbyengine.Geometry, error) {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("lym beam dobby: "+format+"; way forward: run `lym beam start`, which starts the dobby of the prime and of each beam this machine owns", args...)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, dobbyengine.Geometry{}, refuse("resolve %s: %v", root, err)
	}
	location, err := lymcwd.ResolveWorktree(absolute)
	if err != nil {
		return nil, nil, dobbyengine.Geometry{}, refuse("%s is not a worktree of a hub: %v", absolute, err)
	}
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return location, nil, dobbyengine.Geometry{}, refuse("%s is not a worktree a dobby drives: %v", absolute, err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return location, nil, dobbyengine.Geometry{}, refuse("cannot tell the prime of the hub of %s: %v", absolute, err)
	}
	prime := location
	if location.WorktreeName != primeName {
		origin, found, err := fabricengine.ReadOrigin(location)
		if err != nil {
			return location, nil, dobbyengine.Geometry{}, refuse("cannot read the origin record of %s: %v", location.WorktreeName, err)
		}
		if !found || !origin.Beam {
			return location, nil, dobbyengine.Geometry{}, refuse("%s is neither the prime nor a beam", location.WorktreeName)
		}
		if prime, err = lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName)); err != nil {
			return location, nil, dobbyengine.Geometry{}, refuse("resolve the prime of the hub: %v", err)
		}
	}
	geom, err := hubgeom.DobbyGeometry(prime, location.WorktreeName)
	if err != nil {
		return location, nil, dobbyengine.Geometry{}, refuse("%v", err)
	}
	return location, prime, geom, nil
}

// createCmd returns the `beam create` verb.
func createCmd() *cobra.Command {
	var anchor string
	cmd := &cobra.Command{
		Use:         "create <slug>",
		Short:       "make a project entry a beam: record its owner and start its pawl run",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turns the board's project entry <slug> into a beam. The entry's title, brief and
labels are board writing, so the verb makes no entry: make the project first with
"lym board upsert".

It runs from the beam's parent worktree, the prime or the parent beam of a nested
beam, and refuses elsewhere naming that worktree's path and the same command there.

Steps, in order, each skipped when its result is already there, so a re-run
finishes a stopped create:
  1. claim the root for this machine when the board records no root owner
  2. record this machine as the project's owner with the beam flag and the pawl recipe
  3. start the beam's pawl run as a detached first pass, which its parent's dobby
     drives on: its Worktree-Create makes the pair, and its Beam-Up marks it and
     starts the beam's dobby and orch

A pawl run in flight is reported live and starts nothing. A first pass a covering
pause refused leaves no run, and the same "lym beam create <slug> [--anchor <dir>]"
after the lift starts it. A flagged beam left by a first pass that died takes the same
--anchor again, since its anchor reached no run record. Dobby never starts a beam
project, so only this verb, a hand pass or "lym adopt" starts or resumes its run.

With --anchor <dir> the new pair takes <dir> of the code checkout as its anchor
instead of its parent's; the directory must be tracked in the commit the beam forks
from. It is recorded in the run's seed, and a pair that already exists sits at its
own anchor, which "lym fabric anchor" moves.

The envelope names "lym batten status <slug>" as the way to follow the run.

Example:
  lym beam create payments
  lym beam create payments --anchor services/payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(_ context.Context, acting string, prime *lymcwd.Location) (any, error) {
				deps, err := hubCreateDeps(prime)
				if err != nil {
					return nil, err
				}
				deps.Acting = acting
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
		Short:       "ask a beam's pawl run to land the beam into its parent",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Asks the pawl run of the beam <slug> to land it: the verb writes a land request
naming that run into the beam worktree's .lym/beam directory, which the run's
Beam-Await row consumes. The run then writes the beam's description, merges the beam
into its parent worktree's branch and removes the beam's pair, whose board entry is
then done. Follow the landing with "lym batten status <slug>".

It runs anywhere in the hub, a task pair and the beam itself included, and refuses,
each naming its way forward:
  - an entry that is not an open beam project, or a beam with no pair here
  - a beam with no pawl run, or whose run is not waiting at Beam-Await
  - a beam, or a beam's parent, that another machine owns
  - a beam not flagged ready with "lym beam ready <slug>"
  - work under the beam: an open entry whose parent is the beam, through grouping
    projects, and a local pair forked from the beam
Land or cancel the members first.

Example:
  lym beam land payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			result, err := onHubLandDeps(cmd, "lym beam land", func(deps landDeps) (map[string]any, error) { return land(deps, args[0]) })
			return finishVerb(cmd, result, err)
		},
	}
}

// readyCmd returns the `beam ready` verb.
func readyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "ready <slug>",
		Short:       "flag a beam ready to be merged into its parent, so its dobby lands it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Flags the beam <slug> ready to be merged into its parent: the verb writes a
ready record naming the beam into the beam worktree's _lym/landing directory,
beside the beam's change description, and commits it. The flag does not say the
project is finished: once it is set and nothing works under the beam, the beam's
dobby asks its pawl run to land it, as "lym beam land" does, and the landing
removes the beam's pair. Without the flag the dobby never lands the beam, and
"lym beam land" refuses it. A beam already flagged is reported as such.

The beam's orch or the operator runs it, from anywhere in the hub; dobby never
does. It refuses an entry that is not an open beam project with a pair here,
naming its way forward.

Example:
  lym beam ready payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			result, err := onHubLandDeps(cmd, "lym beam ready", func(deps landDeps) (map[string]any, error) { return markReady(deps, args[0]) })
			return finishVerb(cmd, result, err)
		},
	}
}

// onHubLandDeps runs verb, `beam land` or `beam ready`, over the land deps of the hub the command's cwd belongs to.
// It resolves the hub's prime from the cwd's own worktree, so a task pair reaches the verb's own refusals.
// A nil result is a refusal made before the request was written.
func onHubLandDeps(cmd *cobra.Command, verb string, run func(deps landDeps) (map[string]any, error)) (*map[string]any, error) {
	location, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	acting, err := lymcwd.Resolve(location)
	if err != nil {
		return nil, fmt.Errorf("%s: %w; way forward: run it inside a worktree of the hub", verb, err)
	}
	primeName, err := fabricengine.PrimeName(acting)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot verify the hub's prime worktree: %w", verb, err)
	}
	prime, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(acting, primeName))
	if err != nil {
		return nil, fmt.Errorf("%s: resolve the hub's prime worktree: %w", verb, err)
	}
	result, err := run(hubLandDeps(prime))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// runVerb resolves the prime from the command's cwd, runs the verb over it and prints the verb's result in the standard envelope.
// The verb is told the name of the worktree it was run from beside the prime's location.
// A verb's error is printed with the result it returned beside it.
func runVerb(cmd *cobra.Command, run func(ctx context.Context, acting string, prime *lymcwd.Location) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	fail := func(err error) error {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return fail(err)
	}
	location, err := lymcwd.Resolve(cwd)
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

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
