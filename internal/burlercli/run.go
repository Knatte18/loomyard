// run.go implements the `run` burler verb: the profile-YAML-and-flags-to-Run mapper that turns a
// "lyx burler run" invocation into a blocking burlerengine.Engine.Run call and prints its Result as
// a single JSON envelope.
// It also owns decodeProfile, the strict YAML decode that maps a profile file 1:1 onto
// burlerengine.Profile.

package burlercli

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// fileSetYAML mirrors burlerengine.FileSet's YAML shape.
type fileSetYAML struct {
	Paths        []string `yaml:"paths"`
	Instructions string   `yaml:"instructions"`
}

// profileYAML mirrors a profile file's top-level shape, using kebab-case keys.
type profileYAML struct {
	Target            fileSetYAML `yaml:"target"`
	Fasit             fileSetYAML `yaml:"fasit"`
	Rubric            string      `yaml:"rubric"`
	FixScope          string      `yaml:"fix-scope"`
	ToolUse           bool        `yaml:"tool-use"`
	ClusterFan        string      `yaml:"cluster-fan"`
	ReviewPath        string      `yaml:"review-path"`
	FixerReportPath   string      `yaml:"fixer-report-path"`
	PriorReviews      []string    `yaml:"prior-reviews"`
	PriorFixerReports []string    `yaml:"prior-fixer-reports"`
}

// decodeProfile strictly decodes a profile file into burlerengine.Profile
// using Decoder.KnownFields(true). Content validation is Profile.validate's job.
func decodeProfile(data []byte) (burlerengine.Profile, error) {
	var parsed profileYAML

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return burlerengine.Profile{}, fmt.Errorf("burler: profile YAML: %w", err)
	}

	return burlerengine.Profile{
		Target: burlerengine.FileSet{
			Paths:        parsed.Target.Paths,
			Instructions: parsed.Target.Instructions,
		},
		Fasit: burlerengine.FileSet{
			Paths:        parsed.Fasit.Paths,
			Instructions: parsed.Fasit.Instructions,
		},
		Rubric:            parsed.Rubric,
		FixScope:          burlerengine.FixScope(parsed.FixScope),
		ToolUse:           parsed.ToolUse,
		ClusterFan:        parsed.ClusterFan,
		ReviewPath:        parsed.ReviewPath,
		FixerReportPath:   parsed.FixerReportPath,
		PriorReviews:      parsed.PriorReviews,
		PriorFixerReports: parsed.PriorFixerReports,
	}, nil
}

// runCmd builds the `run` subcommand. It validates --profile manually (rather
// than via MarkFlagRequired) to route the flag error through SetExit.
func (c *burlerCLI) runCmd() *cobra.Command {
	var (
		profilePath string
		model       string
		effort      string
		round       string
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "run one review+fix round from a profile YAML file",
		Long: `run reads a profile YAML file describing one review+fix round — what to
review, what to judge it against, and how the round is allowed to write its
fixes — drives the round through the real shuttle substrate, and prints its
Result as a single JSON envelope.

Example profile YAML:
  target:
    paths: ["docs/overview.md"]
    instructions: ""
  fasit:
    paths: ["_mill/discussion.md"]
    instructions: ""
  rubric: |
    BLOCKING: the doc contradicts the discussion's pinned decisions.
    MEDIUM: a decision is described but its rationale is missing.
    LOW: wording is unclear but not misleading.
    NIT: minor formatting.
  fix-scope: source
  tool-use: false
  cluster-fan: ""  # naming a fan from burler.yaml activates cluster review, one fork per fan entry
  review-path: _lyx/burler/review.md
  fixer-report-path: _lyx/burler/fixer-report.md
  prior-reviews: []
  prior-fixer-reports: []

Example invocation:
  lyx burler run --profile profile.yaml

--model/--effort override the provider's model/reasoning-effort; empty
defers to the provider default. --timeout overrides the shuttle config's
run-timeout; zero defers to the config default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// Validate flag shape before ever touching c.engine (still
			// unpopulated when config resolution aborted), so a missing
			// --profile is reported as its own flag error rather than being
			// swallowed by the PersistentPreRunE abort's already-recorded
			// exit code.
			if profilePath == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "burler: --profile is required"))
				return nil
			}

			// A failing PersistentPreRunE has already written an error
			// response and recorded the exit code; short-circuit rather
			// than touch c.engine, which is unpopulated on that path.
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			data, err := os.ReadFile(profilePath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("burler: read --profile: %v", err)))
				return nil
			}

			profile, err := decodeProfile(data)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			opts := burlerengine.RunOpts{
				Model:   model,
				Effort:  effort,
				Timeout: timeout,
				Round:   round,
			}

			result, err := c.engine.Run(profile, opts)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, resultEnvelope(result, c.mode, c.stateDir, c.stencilsDir)))
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "path to the profile YAML file describing this round (required)")
	cmd.Flags().StringVar(&model, "model", "", "provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&effort, "effort", "", "reasoning-effort override; empty defers to the provider default")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "wall-clock deadline before an in-progress run is classified as timed out (0 = config default)")

	return cmd
}

// resultEnvelope maps a burlerengine.Result onto the JSON envelope run's RunE prints via
// output.Ok. It is a separate function (rather than an inline map literal) so its shape
// is directly unit-testable without needing a live c.engine.Run call. result.ForkAudit
// is nil for a non-cluster round (or one that never reached done) — forkCount guards
// that nil rather than dereferencing Forks on it.
//
// mode, stateDir, and stencilsDir are CLI-level facts about how the stack was wired, taken as
// parameters rather than read off a receiver so this function stays directly unit-testable without a
// live c.engine.Run call. They are emitted in BOTH modes deliberately: a mode field that existed only
// in standalone could not be used to tell the two modes apart, which is its whole purpose, and
// stencilsDir is equally worth reporting in a hub run pointed at an experimental stencil set via the
// flag. stateDir is the empty string in hub mode, where no derived state directory exists.
//
// This is the third named exception to hub byte-identity: it is an output-shape-only change, no path
// resolves differently and nothing new is written in hub mode, and the keys are additive so no
// existing consumer breaks.
func resultEnvelope(result burlerengine.Result, mode, stateDir, stencilsDir string) map[string]any {
	forkCount := 0
	if result.ForkAudit != nil {
		forkCount = len(result.ForkAudit.Forks)
	}

	return map[string]any{
		"outcome":              string(result.Outcome),
		"verdict":              string(result.Verdict),
		"reviewPath":           result.ReviewPath,
		"fixerReportPath":      result.FixerReportPath,
		"sessionId":            result.SessionID,
		"strandGuid":           result.StrandGUID,
		"lastAssistantMessage": result.LastAssistantMessage,
		"clusterWarnings":      result.ClusterWarnings,
		"forkCount":            forkCount,
		"mode":                 mode,
		"stateDir":             stateDir,
		"stencilsDir":          stencilsDir,
	}
}
