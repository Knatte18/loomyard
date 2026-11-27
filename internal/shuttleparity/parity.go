// parity.go prepares each engine's launch with no process run and compares the manifests.

package shuttleparity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The two engines the comparison is between: the reference engine's manifest is what the compared engine's is held to.
const (
	ReferenceEngine = "claude"
	ComparedEngine  = "codex"
)

// ConfiguredSandbox reports a sandbox that starts and runs nothing,
// so a prepared launch lists the sandbox mode its configuration names.
func ConfiguredSandbox(string, string, string) error {
	return nil
}

// UnrunHooks runs nothing and returns shuttleengine.ErrHookUnrun itself,
// so a plugin hook is listed as unrun rather than as one to fix.
// The error is never wrapped, since its text is what marks the hook unrun.
func UnrunHooks(shuttleengine.HookRun) ([]byte, error) {
	return nil, shuttleengine.ErrHookUnrun
}

// PromptItems returns the manifest items read off a prompt rendered for engine:
// the edit directive, when the prompt holds that engine's copy from stencilsDir.
func PromptItems(stencilsDir, engine, prompt string) ([]shuttleengine.ManifestItem, error) {
	directive, err := editdirective.Directive(stencilsDir, engine)
	if err != nil {
		return nil, fmt.Errorf("shuttleparity: %w", err)
	}
	if !strings.Contains(prompt, directive) {
		return nil, nil
	}
	return []shuttleengine.ManifestItem{{
		Channel:  shuttleengine.ManifestChannelEditDirective,
		Key:      editdirective.EngineBlock,
		Delivery: shuttleengine.ManifestDelivered,
	}}, nil
}

// Unmet returns the requirements of spec that engine does not meet, with forks counted for a spec that forks subagents.
// A start of spec on engine is refused when the result is not empty.
func Unmet(engine shuttleengine.Engine, spec shuttleengine.Spec) []shuttleengine.Requirement {
	required := slices.Clone(spec.Requires)
	if spec.ForkSubagents && !slices.Contains(required, shuttleengine.RequirementForks) {
		required = append(required, shuttleengine.RequirementForks)
	}
	supporter, supports := engine.(shuttleengine.RequirementSupporter)
	var unmet []shuttleengine.Requirement
	for _, requirement := range required {
		if !supports || !supporter.Supports(requirement) {
			unmet = append(unmet, requirement)
		}
	}
	return unmet
}

// PrepareEach prepares the launch of every engine of set and returns its manifest, keyed by engine name.
// specs holds each engine's spec, whose prompt was rendered for it.
// A spec with a config role gets that role's skills from cfg ahead of its own, as a start does.
// Each engine prepares into its own directory under dir, and its manifest gains the prompt items of its spec.
func PrepareEach(set shuttleengine.EngineSet, cfg shuttleengine.Config, stencilsDir, dir string, specs map[string]shuttleengine.Spec) (map[string]shuttleengine.Manifest, error) {
	manifests := map[string]shuttleengine.Manifest{}
	for _, name := range set.Names() {
		spec := specs[name]
		if spec.ConfigRole != "" {
			skills, err := cfg.Agents.Skills(spec.ConfigRole, spec.Skills)
			if err != nil {
				return nil, fmt.Errorf("shuttleparity: expand the skills of role %q: %w", spec.ConfigRole, err)
			}
			spec.Skills = skills
		}
		engine, err := set.Lookup(name)
		if err != nil {
			return nil, fmt.Errorf("shuttleparity: %w", err)
		}
		runDir := filepath.Join(dir, name)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return nil, fmt.Errorf("shuttleparity: make the run directory of engine %q: %w", name, err)
		}
		if _, err := engine.Prepare(runDir, spec, cfg); err != nil {
			return nil, fmt.Errorf("shuttleparity: prepare the launch of engine %q: %w", name, err)
		}
		manifest, found, err := shuttleengine.ReadManifest(runDir)
		if err != nil {
			return nil, fmt.Errorf("shuttleparity: read the launch manifest of engine %q: %w", name, err)
		}
		if !found {
			return nil, fmt.Errorf("shuttleparity: engine %q wrote no launch manifest; way forward: have that engine's Prepare write its manifest through shuttleengine.WriteManifest", name)
		}
		items, err := PromptItems(stencilsDir, name, spec.Prompt)
		if err != nil {
			return nil, err
		}
		manifest.Items = append(manifest.Items, items...)
		manifests[name] = manifest
	}
	return manifests, nil
}

// Compare returns the compared items of the reference engine's manifest that the compared engine's lacks.
// The fork deny is left out when the compared engine of engines does not meet the fork requirement.
// An engine that forks no subagent has nothing to deny them.
func Compare(engines shuttleengine.EngineSet, manifests map[string]shuttleengine.Manifest) []shuttleengine.ManifestItem {
	missing := shuttleengine.MissingItems(manifests[ReferenceEngine], manifests[ComparedEngine])
	// A set with no compared engine meets no requirement.
	compared, _ := engines.Lookup(ComparedEngine)
	if supporter, ok := compared.(shuttleengine.RequirementSupporter); ok && supporter.Supports(shuttleengine.RequirementForks) {
		return missing
	}
	return slices.DeleteFunc(missing, func(item shuttleengine.ManifestItem) bool {
		return item.Channel == shuttleengine.ManifestChannelGuard && item.Key == shuttleengine.WebsterForkDenyKey
	})
}
