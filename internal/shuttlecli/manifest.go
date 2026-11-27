// manifest.go implements the `manifest` shuttle verb, which prints what each engine's launch gives one seat.
// The verb fails when the compared engine's launch lacks an item.

package shuttlecli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/shuttleparity"
	"github.com/spf13/cobra"
)

// worktreeManifestReport returns the report function of the worktree at layout, whose agent names geom holds.
// Each call builds the seats on the hub's stencils and specs and on the worktree's configuration.
// Its engines run no plugin hook and no sandbox probe.
func worktreeManifestReport(layout *lymcwd.Location, geom reedengine.Geometry, cfg shuttleengine.Config, dirs configengine.Dirs) func(name string) (shuttleparity.ManifestReport, error) {
	return func(name string) (shuttleparity.ManifestReport, error) {
		registry, err := modelspec.LoadRegistry(dirs)
		if err != nil {
			return shuttleparity.ManifestReport{}, fmt.Errorf("load the model registry: %w", err)
		}
		engines, err := shuttleengines.NewWithSeams(cfg, dirs, shuttleengines.LaunchSeams{RunHook: shuttleparity.UnrunHooks, SandboxProbe: shuttleparity.ConfiguredSandbox})
		if err != nil {
			return shuttleparity.ManifestReport{}, err
		}
		return shuttleparity.Report(shuttleparity.SeatEnv{
			StencilsDir: fabricengine.StencilsDir(layout.HubPath),
			SpecsDir:    fabricengine.SpecsDir(layout.HubPath),
			Registry:    registry,
			Location:    layout,
			Dirs:        dirs,
			Shortname:   geom.NameShortname,
			Slug:        layout.WorktreeName,
			ParentName:  geom.ParentName,
		}, name, engines, cfg)
	}
}

// manifestCmd builds the `manifest` subcommand, which prints each engine's launch manifest for one seat.
func (c *shuttleCLI) manifestCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "manifest <seat>",
		Short:       "show what each engine's launch gives one seat",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `manifest prepares one seat's launch on every engine, on this worktree's
configuration, and prints each engine's launch manifest with the parity
verdict: the items of Claude's manifest that Codex's lacks, listed as missing.
A seat is named by its strand role; an unknown name is refused listing the seats.

It exits non-zero when an item is missing. An engine that cannot start the seat
is named with its unmet requirements, which alone never fail the verb.

Bounds: it starts no session and runs no plugin hook or sandbox probe. It writes
nothing outside a temporary directory it removes, but the Codex home a Codex
launch preparation readies (its links, trust record and config.toml), and it
keeps no manifest copy. Plugin contexts are listed by plugin and hook, unrun;
only a run's own record says whether one was delivered. The size check of
Codex's developer instructions leaves plugin contexts out, so a seat that
passes here can still be refused at a real launch.

Example:
  lym shuttle manifest discussion`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			report, err := c.manifestReport(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			fields := manifestFields(report)
			if len(report.Missing) == 0 {
				clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
				return nil
			}
			var missing []string
			for _, item := range report.Missing {
				missing = append(missing, fmt.Sprintf("%s %s", item.Channel, item.Key))
			}
			message := fmt.Sprintf("the %s launch of seat %q lacks %s; way forward: give the Codex engine a delivery for each missing item in internal/shuttleengine/codexengine, and run no Codex bench of this seat until the verb passes",
				shuttleparity.ComparedEngine, report.Seat, strings.Join(missing, ", "))
			clihelp.SetExit(cmd.Context(), output.ErrFields(out, message, fields))
			return nil
		},
	}
}

// manifestFields renders report as the envelope's fields.
// A manifest is printed without its stored verdict, which lists every unrun plugin context as omitted.
func manifestFields(report shuttleparity.ManifestReport) map[string]any {
	manifests := map[string]any{}
	for engine, manifest := range report.Manifests {
		manifests[engine] = map[string]any{
			"engine":       manifest.Engine,
			"role":         manifest.Role,
			"promptEngine": manifest.PromptEngine,
			"items":        manifest.Items,
		}
	}
	unmet := []string{}
	for engine, requirements := range report.Unmet {
		var names []string
		for _, requirement := range requirements {
			names = append(names, fmt.Sprint(requirement))
		}
		unmet = append(unmet, fmt.Sprintf("%s cannot start this seat: unmet requirements %s", engine, strings.Join(names, ", ")))
	}
	slices.Sort(unmet)
	return map[string]any{
		"seat":      report.Seat,
		"manifests": manifests,
		"missing":   append([]shuttleengine.ManifestItem{}, report.Missing...),
		"unmet":     unmet,
	}
}
