// run.go implements the `run` burler verb: the profile-YAML-and-flags-to-Run
// mapper that turns a "lyx burler run" invocation into a blocking
// burlerengine.Engine.Run call and prints its Result as a single JSON
// envelope. It also owns decodeProfile, the strict YAML decode that maps a
// profile file 1:1 onto burlerengine.Profile.

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

// fileSetYAML mirrors burlerengine.FileSet's YAML shape (the target/fasit
// key of a profile file): a list of paths and/or free-form instructions.
type fileSetYAML struct {
	Paths        []string `yaml:"paths"`
	Instructions string   `yaml:"instructions"`
}

// profileYAML mirrors a profile file's top-level shape 1:1 onto
// burlerengine.Profile's fields. It exists as a separate type (rather than
// decoding straight into Profile) so the YAML key vocabulary — kebab-case,
// matching the discussion's profile-file contract — stays decoupled from
// Profile's Go field names.
type profileYAML struct {
	Target            fileSetYAML `yaml:"target"`
	Fasit             fileSetYAML `yaml:"fasit"`
	Rubric            string      `yaml:"rubric"`
	FixScope          string      `yaml:"fix-scope"`
	ToolUse           bool        `yaml:"tool-use"`
	ClusterN          int         `yaml:"cluster-n"`
	ReviewPath        string      `yaml:"review-path"`
	FixerReportPath   string      `yaml:"fixer-report-path"`
	PriorReviews      []string    `yaml:"prior-reviews"`
	PriorFixerReports []string    `yaml:"prior-fixer-reports"`
}

// decodeProfile strictly decodes a profile file's raw bytes into a
// burlerengine.Profile. Decoding uses yaml.v3's Decoder.KnownFields(true)
// per the yaml-strictness-split decision: an operator typo in a profile key
// (e.g. "fixscope:" for "fix-scope:") must fail loudly here rather than
// silently zeroing a safety-critical field. decodeProfile performs no
// content validation itself (existence checks, FixScope legality, and so
// on) — that stays the engine's job via Profile.validate, so this function's
// only responsibility is the YAML-to-struct mapping.
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
		ClusterN:          parsed.ClusterN,
		ReviewPath:        parsed.ReviewPath,
		FixerReportPath:   parsed.FixerReportPath,
		PriorReviews:      parsed.PriorReviews,
		PriorFixerReports: parsed.PriorFixerReports,
	}, nil
}

// runCmd builds the `run` subcommand: reads and strictly decodes the
// --profile file into a burlerengine.Profile, maps the four run-tuning flags
// onto a burlerengine.RunOpts, and blocks on c.engine.Run(profile, opts)
// until the round reaches a classified outcome. Every error path (profile
// read, strict decode, or a Run failure — validation, ErrClusterUnsupported,
// shuttle, verdict parse) goes through output.Err; a successful Run — for
// any outcome, not only done — is reported via output.Ok, mirroring
// shuttlecli's run.go pattern where a classified outcome is data, not an
// error.
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
    paths: ["docs/modules/burler.md"]
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
  cluster-n: 0
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

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":              string(result.Outcome),
				"verdict":              string(result.Verdict),
				"reviewPath":           result.ReviewPath,
				"fixerReportPath":      result.FixerReportPath,
				"sessionId":            result.SessionID,
				"strandGuid":           result.StrandGUID,
				"lastAssistantMessage": result.LastAssistantMessage,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "path to the profile YAML file describing this round (required)")
	cmd.Flags().StringVar(&model, "model", "", "provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&effort, "effort", "", "reasoning-effort override; empty defers to the provider default")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "wall-clock deadline before an in-progress run is classified as timed out (0 = config default)")
	_ = cmd.MarkFlagRequired("profile")

	return cmd
}
