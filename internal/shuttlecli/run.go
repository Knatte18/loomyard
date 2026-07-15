// run.go implements the `run` shuttle verb: the flag-to-Spec mapper that
// turns a "lyx shuttle run" invocation into a blocking
// shuttleengine.Runner.Run call and prints its classified outcome as a
// single JSON envelope.

package shuttlecli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/muxengine/render"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// runCmd builds the `run` subcommand: validates the --prompt/--prompt-file
// XOR and the --output-file requirement before ever touching c.runner (so a
// flag-shape mistake is reported as a flag error even when config
// resolution has already aborted), builds a shuttleengine.Spec from the
// remaining flags, and blocks on c.runner.Run(spec) until a terminal
// outcome is reached. Every classified outcome (done/asking/died/timeout)
// is data, not an error: run prints output.Ok and exits 0 regardless of
// which outcome came back. Only a mechanism failure (reading --prompt-file,
// spec validation inside the engine, or Run itself erroring) goes through
// output.Err.
func (c *shuttleCLI) runCmd() *cobra.Command {
	var (
		prompt      string
		promptFile  string
		outputFiles []string
		model       string
		effort      string
		interactive bool
		role        string
		round       string
		parent      string
		anchor      string
		focus       bool
		shrink      bool
		timeout     time.Duration
		keepPane    bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "run one agent turn and block until it reaches a classified outcome",
		Long: `run starts one shuttle agent, blocks until it reaches a classified
outcome (done/asking/died/timeout), and prints that outcome as a single JSON
envelope. A run's output files ARE its return value: "done" means every
--output-file entry now exists, "asking" means the agent ended its turn with
a question instead. An --output-file entry may be absolute or relative — a
relative path resolves against the WORKTREE ROOT, not the shell's cwd — and
must not already exist when the run starts: a stale file would satisfy the
contract immediately, so the run is rejected instead.

Example (autonomous, two output files):
  lyx shuttle run --prompt "review this diff" --output-file review.md --output-file findings.json

Example (interactive, agent may ask clarifying questions):
  lyx shuttle run --prompt-file task.md --output-file result.md --interactive

--effort overrides the provider's reasoning-effort level; empty defers to
the provider default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// Validate flag shape before ever touching c.runner (still
			// unpopulated when config resolution aborted), so a bad flag
			// combination is reported as its own flag error rather than
			// being swallowed by the PersistentPreRunE abort's already-
			// recorded exit code.
			havePrompt := prompt != ""
			havePromptFile := promptFile != ""
			if havePrompt == havePromptFile {
				msg := "exactly one of --prompt or --prompt-file is required"
				if havePrompt {
					msg = "--prompt and --prompt-file are mutually exclusive"
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if len(outputFiles) == 0 {
				clihelp.SetExit(cmd.Context(), output.Err(out, "--output-file must be given at least once"))
				return nil
			}

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			promptText := prompt
			if havePromptFile {
				data, err := os.ReadFile(promptFile)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("read --prompt-file: %v", err)))
					return nil
				}
				promptText = string(data)
			}

			spec := shuttleengine.Spec{
				Prompt:      promptText,
				OutputFiles: outputFiles,
				Model:       model,
				Effort:      effort,
				Interactive: interactive,
				Role:        role,
				Round:       round,
				Parent:      parent,
				Display: render.Display{
					Anchor:                   render.Anchor(anchor),
					Focus:                    focus,
					ShrinkWhenWaitingOnChild: shrink,
				},
				Timeout:  timeout,
				KeepPane: keepPane,
			}

			result, err := c.runner.Run(spec)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":              string(result.Outcome),
				"sessionId":            result.SessionID,
				"guid":                 result.StrandGUID,
				"lastAssistantMessage": result.LastAssistantMessage,
				"runDir":               result.RunDir,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&prompt, "prompt", "", "task prompt text (mutually exclusive with --prompt-file)")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "path to a file whose contents become the task prompt")
	cmd.Flags().StringArrayVar(&outputFiles, "output-file", nil, "output file the agent must write (repeatable; required at least once; must not already exist; relative paths resolve against the worktree root)")
	cmd.Flags().StringVar(&model, "model", "", "provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&effort, "effort", "", "reasoning-effort override; empty defers to the provider default")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "run interactively (the agent may ask questions); default is autonomous")
	cmd.Flags().StringVar(&role, "role", "", "role token used to fill the strand-name template")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().StringVar(&parent, "parent", "", "parent strand's guid")
	cmd.Flags().StringVar(&anchor, "anchor", string(render.AnchorBelowParent), "placement: below-parent|hidden")
	cmd.Flags().BoolVar(&focus, "focus", true, "give this strand psmux input focus")
	cmd.Flags().BoolVar(&shrink, "shrink", true, "collapse this strand to a compact strip once a descendant is present")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "wall-clock deadline before an in-progress run is classified as timed out (0 = config default)")
	cmd.Flags().BoolVar(&keepPane, "keep-pane", false, `leave the strand and its pane alive after a "done" outcome`)

	return cmd
}
