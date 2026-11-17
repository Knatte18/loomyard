// manifest.go implements the `manifest` shuttle verb.
// It prints what each engine would give one seat's session.
// It fails when the Codex manifest lacks an item of Claude's.

package shuttlecli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/shuttleparity"
	"github.com/spf13/cobra"
)

// manifestStandInSlug is the task slug the seats' agent names are formed with: the verb runs under no task.
const manifestStandInSlug = "task"

// manifestMissingWayForward ends the refusal of a seat whose Codex manifest lacks an item of Claude's.
const manifestMissingWayForward = "way forward: give the Codex engine a delivery for each missing item in internal/shuttleengine/codexengine, " +
	"and run no Codex bench of this seat until the verb passes"

// manifestCmd builds the `manifest` subcommand, which prints both engines' manifests for one seat of the seat catalog.
func (c *shuttleCLI) manifestCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "manifest <seat>",
		Short:       "print what each engine gives one seat's session, and fail when Codex lacks an item of Claude's",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `manifest prepares the named seat of the seat catalog on each engine and prints the launch
manifest each engine writes: one item per skill, standing passage, instruction file, guard, hook and
directive the session is given. It exits non-zero when the Codex manifest lacks a compared item of
Claude's, naming each as <channel> <key>.
A seat an engine cannot start, such as a fork seat on Codex, is compared like any other; the output
names that engine with its unmet requirements, and they never fail the verb.

It starts no session and runs no plugin hook or sandbox probe. It writes nothing outside its
temporary directory but the Codex home a Codex Prepare readies (its links, trust record and
config.toml), and keeps no manifest copy, since it never launches.
Plugin contexts are listed by plugin and hook as unrun: only a run's own record says whether one was
delivered. The developer-instructions size check leaves plugin contexts out, so a seat it passes can
still be refused at a real launch.

Example:
  lym shuttle manifest darn`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.manifestReport(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			fields := manifestFields(result)
			if len(result.Missing) > 0 {
				missing := make([]string, len(result.Missing))
				for i, item := range result.Missing {
					missing[i] = fmt.Sprintf("%s %s", item.Channel, item.Key)
				}
				message := fmt.Sprintf("the %s manifest of seat %s lacks %s; %s", shuttleparity.ComparedEngine, result.Seat, strings.Join(missing, ", "), manifestMissingWayForward)
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, message, fields))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
}

// manifestFields returns the fields the verb prints for result.
// Each manifest is printed without its stored verdict, which lists every plugin context as omitted since none ran,
// so the verdict is the missing list alone and the plugin contexts are listed unrun by engine.
func manifestFields(result shuttleparity.ManifestReport) map[string]any {
	manifests := make(map[string]shuttleengine.Manifest, len(result.Manifests))
	unrun := map[string][]string{}
	for engine, manifest := range result.Manifests {
		for _, item := range manifest.Omitted() {
			unrun[engine] = append(unrun[engine], fmt.Sprintf("%s %s", item.Channel, item.Key))
		}
		manifest.Verdict = nil
		manifests[engine] = manifest
	}
	missing := make([]string, len(result.Missing))
	for i, item := range result.Missing {
		missing[i] = fmt.Sprintf("%s %s", item.Channel, item.Key)
	}
	unmet := map[string][]string{}
	for engine, requirements := range result.Unmet {
		for _, requirement := range requirements {
			unmet[engine] = append(unmet[engine], string(requirement))
		}
	}
	return map[string]any{"seat": result.Seat, "manifests": manifests, "missing": missing, "unrun": unrun, "unmet": unmet}
}

// manifestReportOf prepares the seat called name on both engines for the worktree layout resolves to.
// It then compares the manifests.
// The engines run under the manifest-only seams, so no plugin hook or sandbox probe runs.
func manifestReportOf(layout *lymcwd.Location, cfg shuttleengine.Config, dirs configengine.Dirs, name string) (shuttleparity.ManifestReport, error) {
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return shuttleparity.ManifestReport{}, fmt.Errorf("load the model registry: %w", err)
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoltDir(layout.HubPath))
	if !found {
		return shuttleparity.ManifestReport{}, fmt.Errorf("the hub records no shortname; way forward: record one with `lym fabric shortname <shortname>`")
	}
	parentName, err := agentname.Format(shortname, "", agentname.RoleOrch)
	if err != nil {
		return shuttleparity.ManifestReport{}, err
	}
	engines, err := shuttleengines.NewWithSeams(cfg, dirs, shuttleengines.LaunchSeams{RunHook: shuttleparity.UnrunHooks, SandboxProbe: shuttleparity.ConfiguredSandbox})
	if err != nil {
		return shuttleparity.ManifestReport{}, err
	}
	env := shuttleparity.SeatEnv{
		StencilsDir: fabricengine.StencilsDir(layout.HubPath),
		SpecsDir:    fabricengine.SpecsDir(layout.HubPath),
		Registry:    registry,
		Location:    layout,
		Dirs:        dirs,
		Shortname:   shortname,
		Slug:        manifestStandInSlug,
		ParentName:  parentName,
	}
	return shuttleparity.Report(env, name, engines, cfg)
}
