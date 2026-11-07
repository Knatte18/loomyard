// modelspec.go declares the package's exported types and closed vocabularies.
// It carries no logic; parse.go, registry.go, load.go and template.go implement the behaviour these types describe.

// Package modelspec parses and resolves the model-spec notation.
// Agent-spawning configs use it to say which LLM runs a role.
// The contract is contracts/specs/llm-model-spec.md, and this package implements it.
//
// Grammar, in one line:
//
//	<alias>[item,item,...]        where item is  key=value  |  <effort>
//
// (registry lookup), or the same shape in escape form (no registry lookup):
//
//	<engine>:<model-id>[item,item,...]        where item is the same shape as above
//
// A Spec takes one of two shapes.
// In alias form, Alias is non-empty and Engine and Model are empty.
// In escape form, Engine and Model are both non-empty and Alias is empty.
// Exactly one of the two shapes holds for any parsed Spec: never both, never neither.
//
// Parse(s) checks grammar only and returns a Spec, so callers can lint a spec string with no registry in hand.
// Grammar errors are reported apart from unknown-alias errors.
// Registry.Resolve(spec) performs the alias lookup for alias form, or passes the escape-form pair through unchanged.
// It also applies bracket-over-default precedence and yields a Resolved.
//
// LoadRegistry(dirs) reads the models.yaml layers through configengine.StackLayers.
// A missing layer file is not an error: LoadRegistry returns the built-in fallback registry unchanged,
// so every consumer works with zero config present.
// A present layer's entries are validated.
// The stack's merged entries are laid over the built-ins by whole-entry replacement per alias.
// A file entry for "sonnet" replaces the built-in "sonnet" entirely; it does not merge field by field.
//
// Leaf import discipline: this package's production code imports a closed set of packages.
// That set is the standard library (including embed), internal/configengine, and gopkg.in/yaml.v3.
// It never imports configreg, envsource, yamlengine, lymcwd, or any feature package.
// This lets every future consumer import modelspec without an import cycle;
// configreg importing modelspec (for ConfigTemplate) is the one allowed direction.
// leaf_enforcement_test.go (TestLeafInvariant_AllowlistOnly) enforces this.
// The rule is recorded as PATTERN-leaf-packages.
//
// Consumers map Resolved onto shuttleengine.Spec themselves.
// This package does not import shuttleengine, so it cannot return one directly:
//
//	spec.Model = resolved.Model
//	spec.Effort = resolved.Params["effort"]
//	spec.Version = resolved.Params["version"]
package modelspec

import "sort"

// Spec is one parsed model-spec string, in exactly one of two shapes:
//
//   - Alias form: Alias is non-empty; Engine and Model are both empty.
//     Resolving it requires a Registry lookup.
//   - Escape form: Engine and Model are both non-empty; Alias is empty.
//     Resolving it needs no registry; the engine and model are stated outright.
//
// Params holds the bracket's key=value pairs, or nil when the spec string had no bracket.
// A Spec is produced only by Parse, which enforces that exactly one of the two shapes holds.
type Spec struct {
	// Alias is the registry key to resolve, set only in alias form.
	Alias string
	// Engine is the provider engine name, set only in escape form (e.g. "claude").
	Engine string
	// Model is the provider-side model id, set only in escape form.
	Model string
	// Params holds the bracket's key=value overrides;
	// nil when no bracket was present in the spec string.
	Params map[string]string
}

// Entry is one registry record: the engine and model that an alias resolves to.
// Its Defaults supply parameter values that a spec's bracket omits.
// The yaml tags name the keys of a models.yaml alias block, which is decoded strictly.
type Entry struct {
	// Engine names the provider engine the alias requires (e.g. "claude").
	Engine string `yaml:"engine"`
	// Model is the provider-side model string passed to that engine.
	// It is a free-form string; modelspec never validates it against any model list.
	// That lets a brand-new model be adopted through a models.yaml entry, with no recompile.
	// A new model under a known engine needs no code change.
	Model string `yaml:"model"`
	// Defaults holds parameter defaults (e.g. {"effort": "medium"}) for the keys a bracket omits.
	// Nil means no defaults.
	Defaults map[string]string `yaml:"defaults"`
}

// Registry maps a registry alias to the Entry it resolves to.
// The zero Registry (a nil map) is valid to Resolve against.
// Escape-form specs resolve normally, since they never consult the registry.
// Alias-form specs fail on it with an unknown-alias error.
type Registry map[string]Entry

// Resolved is the outcome of resolving one Spec against a Registry, or of the Spec alone in escape form.
// It holds the provider engine, the provider-side model string, and the final parameter map.
// Params is never nil: an empty map means no parameters, so callers can range over it unconditionally.
type Resolved struct {
	// Engine is the provider engine name to dispatch to.
	Engine string
	// Model is the provider-side model string to pass to that engine.
	Model string
	// Params holds the final parameter map.
	// For alias form it is the entry's Defaults overlaid by the bracket params, the bracket winning per key;
	// for escape form it is the bracket params verbatim, and it is never nil.
	Params map[string]string
}

// knownParams is the closed set of canonical parameter keys an Entry's Defaults map may use.
// Every bracket param normalizes into this key space.
// It gates param keys only, never model names or aliases.
// A brand-new model is therefore adopted through a models.yaml entry or the escape form, with no change here.
var knownParams = map[string]bool{
	"effort":  true,
	"version": true,
}

// bracketKeys maps each accepted bracket key spelling to the canonical Params key it normalizes to.
// A spelling may differ from its canonical key, as "v" maps to "version".
// Every value is one of the closed canonical parameter keys, so a bracket never yields any other Params key.
var bracketKeys = map[string]string{
	"effort": "effort",
	"v":      "version",
}

// knownEngines is the closed set of provider engine names.
// An Entry's Engine field and an escape-form prefix may name only these.
// It gates engine names only, never model names or aliases.
// A new model under a known engine, such as a new Claude model, needs no change here.
var knownEngines = map[string]bool{
	"claude": true,
	"codex":  true,
}

// EngineNames returns the closed set of provider engine names, sorted.
// The slice is a copy, so a caller cannot change the set.
func EngineNames() []string {
	names := make([]string, 0, len(knownEngines))
	for name := range knownEngines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
