// cli.go builds the cobra command tree for the burler module and the RunCLI seam that runs it over an io.Writer.
// The parent command's PersistentPreRunE resolves cwd and runs one preflight.ResolveMode probe.
// It then calls c.wire (wiring.go).
// wire selects hub or standalone mode and builds the whole engine stack for the winning mode.
// It stores the resolved ingredients on burlerCLI once per invocation.
// A non-nil ResolveMode error refuses the run before c.wire is called.
// Refusal is a resolution verdict, not a wiring choice.
// burlercli is the module's claudeengine wiring point, mirroring the Shuttle Provider-Seam Invariant.

package burlercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/spf13/cobra"
)

// burlerCLI is the receiver of the run verb.
type burlerCLI struct {
	// engine is the constructed burlerengine.Engine the run verb closes over.
	engine *burlerengine.Engine

	// languageSkills are the hub's language skills that a code round's seats load after burler's own skills.
	// Hub mode loads them from the hub's language.yaml; standalone mode leaves them empty.
	languageSkills []string

	// cwd is the directory that resolvePersistentPreRun read through lymcwd.CwdFrom.
	// The wiring step resolves --target-dir and --stencils-dir against the same base,
	// so the run verb resolves --profile against it too, not against the process cwd.
	// Under an in-process driver the process cwd is a different directory.
	cwd string

	// reedUp brings the standalone reed session up, idempotently.
	// The run verb calls it right before it drives a round.
	// Only standalone mode sets it, because standalone mode has no other way to a live session:
	// `lym reed up` requires lymcwd.Resolve in its pre-run, so it runs only in hub mode.
	// The standalone session's socket is "lym-<hash8>", with its state under the derived state directory,
	// so that session can only be booted in-process.
	// Hub mode leaves it nil, and bringing reed up stays the operator's (or loom's) own act.
	reedUp func() error

	// stencilsDirFlag and targetDirFlag hold the raw values of the --stencils-dir and --target-dir persistent flags.
	// An empty value means the flag was not passed.
	// Each mode computes its own default in the wiring step (wiring.go), not in a zero-value fallback here.
	stencilsDirFlag string
	targetDirFlag   string

	// mode, stateDir, and stencilsDir are CLI-level facts about how the stack was wired.
	// They are not results of a review round, so they live here instead of in the round's result.
	// run.go's success envelope reads them off this receiver.
	mode        string
	stateDir    string
	stencilsDir string
}

// resolvePersistentPreRun resolves cwd and calls preflight.ResolveMode(cwd).
// It delegates the mode decision and the engine stack construction to c.wire (wiring.go).
// A non-nil ResolveMode error is handled here rather than in wire: it is a resolution verdict, not a wiring choice.
// It is surfaced verbatim and aborts the pre-run before c.wire is called.
// It is a separate method so a test can call it on a burlerCLI and inspect the populated fields.
// The group command skips resolution when it runs bare or on an unknown subcommand, through clihelp.GroupRunE.
// Neither path needs a git repository.
// The validate-review verb skips resolution too: it reads one told path and needs no hub, mode or engine wiring.
func (c *burlerCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "burler" || cmd.Name() == validateReviewVerb {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	c.cwd = cwd

	loc, mode, err := preflight.ResolveMode(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	if err := c.wire(loc, mode, cwd, c.stencilsDirFlag, c.targetDirFlag); err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
}

// Command returns the cobra command tree for the burler module.
func Command() *cobra.Command {
	c := &burlerCLI{}

	parent := &cobra.Command{
		Use:   "burler",
		Short: "run one review+fix round over an artifact (the burler round worker)",
		Long: `burler drives one review+fix round over an artifact with two sessions run
one after the other: a reviewer (role burler-review) reviews the target
against a fasit (a source of truth) and writes a structured review file
(verdict + findings, each finding carrying its fix), then, once lym has
accepted the review, a fixer (role burler-fix) validates what the
reviewer found, fixes it and writes a fixer report. A review with no findings
spawns no fixer. What to review, what to judge it against, and how the round is
allowed to write its fixes are all supplied as a profile YAML file — burler
itself carries zero domain logic about the artifact under review.

Modes:
  burler runs in hub mode inside a lym hub worktree, and in standalone mode
  anywhere else -- a plain git checkout with no lym hub beside it. Two
  persistent flags cross that boundary: --stencils-dir is optional and
  read-only in BOTH modes (hub default: the hub's own stencils dir;
  standalone default: the derived state directory's own _lym/stencils);
  --target-dir is standalone-only, defaults to the current directory, and is
  refused in hub mode, where the anchor path is structurally the target. A
  relative value for either is resolved against the current directory, and
  the standalone target is lifted to the root of the git repository
  containing it, so standing in a subdirectory reviews the same repository,
  from the same state directory and reed session, as standing at its root.

Example:
  lym burler run --profile profile.yaml

Example (standalone, outside any lym hub):
  lym burler run --profile profile.yaml --target-dir /path/to/repo`,
		// RunE is set so that bare "lym burler" lists the subcommands.
		// It also makes "lym burler bogus" emit a JSON error envelope instead of cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&c.stencilsDirFlag, "stencils-dir", "",
		"override the stencils directory read at call time (read-only in both modes; hub default: the hub's own stencils dir; standalone default: the derived state directory's _lym/stencils)")
	parent.PersistentFlags().StringVar(&c.targetDirFlag, "target-dir", "",
		"standalone-only: the directory burler reviews against; defaults to the current directory, and either way is lifted to the containing repository's root; refused in hub mode, where the anchor path is already the target")

	parent.AddCommand(c.runCmd())
	parent.AddCommand(c.validateReviewCmd())

	return parent
}

// RunCLI runs the burler CLI with args, writes its output to out, and returns the exit code.
// It reads the working directory from the process.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the burler CLI with args, writes its output to out, and returns the exit code.
// An empty cwd reads the process working directory; any other value is seeded as the working directory.
// The empty case is kept separate because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
