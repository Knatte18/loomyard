// prepare.go runs each engine's Prepare on one spec per engine and compares the manifests the engines write.
// It also holds the two seams a manifest-only launch runs under, the prompt-side items and the unmet requirements.

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

// The names of the two engines the comparison runs between.
// An engine's name is a registry key, so it may appear outside the engine's own directory.
const (
	// ReferenceEngine is the engine whose manifest the comparison asks the other engine to hold.
	ReferenceEngine = "claude"
	// ComparedEngine is the engine whose manifest the comparison checks.
	ComparedEngine = "codex"
)

// editDirectiveKey is the key of the prompt-side edit directive item.
const editDirectiveKey = "edit-directive"

// UnrunHooks is a plugin hook runner that runs nothing.
// It returns shuttleengine.ErrHookUnrun itself.
// Each plugin session-start context is then listed by plugin and hook as omitted with that reason,
// and no engine logs a hook to fix.
func UnrunHooks(run shuttleengine.HookRun) ([]byte, error) {
	return nil, shuttleengine.ErrHookUnrun
}

// ConfiguredSandbox is a sandbox probe that runs nothing and reports a sandbox that starts.
// A launch then lists the sandbox mode codex.yaml configures: auto and workspace-write settle to workspace-write.
func ConfiguredSandbox(bin, home, dir string) error {
	return nil
}

// PromptItems returns the manifest items an engine cannot see of its own launch, since it is told no stencils directory.
// It holds the edit-directive item when prompt carries engine's copy of the directive read from stencilsDir.
// It holds none otherwise.
func PromptItems(stencilsDir, engine, prompt string) []shuttleengine.ManifestItem {
	directive, err := editdirective.Directive(stencilsDir, engine)
	if err != nil || directive == "" || !strings.Contains(prompt, directive) {
		return nil
	}
	return []shuttleengine.ManifestItem{{Channel: shuttleengine.ChannelEditDirective, Key: editDirectiveKey, Delivery: shuttleengine.DeliveryDelivered}}
}

// Unmet returns the requirements of spec that engine does not meet, in the order a start refuses them.
// A spec that authorizes fork subagents requires fork support.
// An engine that answers no requirement meets none.
func Unmet(engine shuttleengine.Engine, spec shuttleengine.Spec) []shuttleengine.Requirement {
	required := slices.Clone(spec.Requires)
	if spec.ForkSubagents && !slices.Contains(required, shuttleengine.RequirementForks) {
		required = append(required, shuttleengine.RequirementForks)
	}
	supporter, _ := engine.(shuttleengine.RequirementSupporter)
	var unmet []shuttleengine.Requirement
	for _, requirement := range required {
		if supporter == nil || !supporter.Supports(requirement) {
			unmet = append(unmet, requirement)
		}
	}
	return unmet
}

// PrepareEach runs each engine of engines' Prepare with its own spec from specs, keyed by engine name.
// Each engine prepares into <dir>/<engine>.
// It first expands each spec's skills with cfg.Agents when the spec names a role, as a launch does.
// It reads each record back and adds the prompt-side items of stencilsDir, and returns the manifests by engine name.
// A Prepare error, an expansion error and an engine that writes no manifest are errors naming the engine.
// Prepare refuses no requirement, so a spec an engine cannot start is prepared like any other.
func PrepareEach(engines shuttleengine.EngineSet, specs map[string]shuttleengine.Spec, stencilsDir, dir string, cfg shuttleengine.Config) (map[string]shuttleengine.Manifest, error) {
	manifests := make(map[string]shuttleengine.Manifest, len(specs))
	for _, name := range engines.Names() {
		engine, err := engines.Lookup(name)
		if err != nil {
			return nil, err
		}
		spec, ok := specs[name]
		if !ok {
			return nil, fmt.Errorf("shuttleparity: engine %s: no spec was given for it; way forward: give PrepareEach one spec per engine of the set", name)
		}
		if spec.ConfigRole != "" {
			skills, err := cfg.Agents.Skills(spec.ConfigRole, spec.Skills)
			if err != nil {
				return nil, fmt.Errorf("shuttleparity: engine %s: expand the skills of role %q: %w", name, spec.ConfigRole, err)
			}
			spec.Skills = skills
		}
		runDir := filepath.Join(dir, name)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return nil, fmt.Errorf("shuttleparity: engine %s: make its run directory: %w", name, err)
		}
		if _, err := engine.Prepare(runDir, spec, cfg); err != nil {
			return nil, fmt.Errorf("shuttleparity: engine %s: prepare: %w", name, err)
		}
		manifest, found, err := shuttleengine.ReadManifest(filepath.Join(runDir, shuttleengine.ManifestFileName))
		if err != nil {
			return nil, fmt.Errorf("shuttleparity: engine %s: read its manifest: %w", name, err)
		}
		if !found {
			return nil, fmt.Errorf("shuttleparity: engine %s: its Prepare wrote no manifest; way forward: have that engine's Prepare write its manifest through shuttleengine.WriteManifest", name)
		}
		manifest.Items = append(manifest.Items, PromptItems(stencilsDir, name, spec.Prompt)...)
		manifests[name] = manifest
	}
	return manifests, nil
}

// Compare returns the compared items of the reference engine's manifest that the compared engine's manifest lacks.
// It matches by channel and key alone, so an omitted or unrun context counts as present.
// The fork deny belongs to fork support,
// so the result drops it when the compared engine does not meet the fork requirement.
func Compare(engines shuttleengine.EngineSet, manifests map[string]shuttleengine.Manifest) []shuttleengine.ManifestItem {
	missing := shuttleengine.MissingItems(manifests[ReferenceEngine], manifests[ComparedEngine])
	compared, _ := engines.Lookup(ComparedEngine)
	if supporter, ok := compared.(shuttleengine.RequirementSupporter); ok && supporter.Supports(shuttleengine.RequirementForks) {
		return missing
	}
	return slices.DeleteFunc(missing, func(item shuttleengine.ManifestItem) bool {
		return item.Channel == shuttleengine.ChannelGuard && item.Key == shuttleengine.WebsterForkDenyKey
	})
}
