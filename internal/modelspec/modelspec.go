// modelspec.go declares the package's exported types and closed vocabularies. It
// carries no logic — Parse (parse.go), Registry.Resolve and builtins (registry.go),
// LoadRegistry (load.go), and ConfigTemplate (template.go) implement the behaviour
// this file's types describe.

// Package modelspec parses and resolves the model-spec notation every
// agent-spawning config in the stack uses to say which LLM runs a role
// (builder's roles, perch/burler reviewers and judges, loom's producers). The
// pinned contract is docs/reference/model-spec.md; this package is its as-built
// implementation.
//
// Grammar, in one line: <alias>[key=value,...] (registry lookup) or
// <engine>:<model-id>[key=value,...] (escape form, no registry lookup).
//
// Alias form vs escape form: a Spec is either an alias (Alias non-empty, Engine
// and Model both empty) or an escape-form pair (Engine and Model both non-empty,
// Alias empty) — exactly one of the two shapes holds for any parsed Spec, never
// both and never neither.
//
// Parse/Resolve split: Parse(s) checks grammar only and returns a Spec — callers
// can lint a spec string with no registry in hand, and grammar errors are
// distinguished from "unknown alias" errors. Registry.Resolve(spec) then performs
// alias lookup (alias form) or passes the escape-form pair through unchanged, and
// applies bracket-over-default precedence, yielding a Resolved.
//
// LoadRegistry fallback semantics: LoadRegistry(baseDir) reads
// hubgeometry.ConfigFile(baseDir, "models"). An absent file is NOT an error — it
// returns the built-in fallback registry unchanged, so every consumer works with
// zero config present. A present file's entries are validated and merged onto the
// built-ins by whole-entry replacement per alias (a file entry for "sonnet"
// replaces the built-in "sonnet" entirely; it does not merge field-by-field).
//
// Leaf import discipline: this package's production code imports ONLY the
// standard library (including embed), internal/hubgeometry, and gopkg.in/yaml.v3
// — never configreg, configengine, envsource, yamlengine, or any feature package.
// This lets every future consumer (builder, perch/burler/loom configs) import
// modelspec without creating an import cycle; configreg importing modelspec (for
// ConfigTemplate) is the one allowed direction. Enforced by
// leaf_enforcement_test.go (TestLeafInvariant_AllowlistOnly) and recorded as the
// Modelspec Leaf Invariant in CONSTRAINTS.md.
//
// Consumers map Resolved onto shuttleengine.Spec themselves — this package does
// not import shuttleengine, so it cannot return one directly:
//
//	spec.Model = resolved.Model
//	spec.Effort = resolved.Params["effort"]
//	spec.Version = resolved.Params["version"]
package modelspec

// Spec is one parsed model-spec string, in exactly one of two shapes:
//
//   - Alias form: Alias is non-empty; Engine and Model are both empty. Resolving
//     it requires a Registry lookup.
//   - Escape form: Engine and Model are both non-empty; Alias is empty. Resolving
//     it needs no registry — the engine and model are stated outright.
//
// Params holds the bracket part's key=value pairs, or nil when the spec string
// had no bracket at all. A Spec is produced only by Parse, which enforces that
// exactly one of the two shapes holds.
type Spec struct {
	// Alias is the registry key to resolve, set only in alias form.
	Alias string
	// Engine is the provider engine name, set only in escape form (e.g. "claude").
	Engine string
	// Model is the provider-side model id, set only in escape form.
	Model string
	// Params holds the bracket's key=value overrides, or nil when no bracket was
	// present in the spec string.
	Params map[string]string
}

// Entry is one registry record: the engine + model a registry alias resolves to,
// plus the parameter defaults applied when a spec's bracket doesn't set them. The
// yaml tags are what LoadRegistry's strict decoder binds a models.yaml alias block
// against.
type Entry struct {
	// Engine names the provider engine the alias requires (e.g. "claude").
	Engine string `yaml:"engine"`
	// Model is the provider-side model string passed to that engine. It is a
	// free-form string — modelspec never validates it against any model list,
	// which is what lets a brand-new model be adopted via a models.yaml entry
	// with no recompile.
	Model string `yaml:"model"`
	// Defaults holds parameter defaults (e.g. {"effort": "medium"}) applied when
	// a resolving spec's bracket omits that key. Nil means no defaults.
	Defaults map[string]string `yaml:"defaults"`
}

// Registry maps a registry alias to the Entry it resolves to. The zero Registry
// (nil map) is valid to Resolve against in escape form (which never consults the
// registry) but resolves no alias form spec — every alias lookup against it
// fails with an unknown-alias error naming the (empty) set of known aliases.
type Registry map[string]Entry

// Resolved is the fully resolved outcome of one Spec against one Registry (or,
// for escape form, of the Spec alone): the provider engine, the provider-side
// model string, and the final parameter map after bracket-over-default
// precedence has been applied. Params is never nil — an empty map, not nil,
// represents "no parameters" — so callers can range over it unconditionally.
type Resolved struct {
	// Engine is the provider engine name to dispatch to.
	Engine string
	// Model is the provider-side model string to pass to that engine.
	Model string
	// Params holds the final parameter map: for alias form, the registry
	// entry's Defaults overlaid by the spec's bracket params (bracket wins per
	// key); for escape form, the spec's bracket params verbatim. Never nil.
	Params map[string]string
}

// knownParams is the closed set of parameter keys a bracket or a registry
// Defaults map may use. It gates param KEYS ONLY — never model names or
// aliases — preserving the pinned new-model-without-recompile requirement: a
// brand-new model is adopted via a models.yaml entry or the escape form on an
// old binary, with no change to this set required.
var knownParams = map[string]bool{
	"effort":  true,
	"version": true,
}

// knownEngines is the closed set of provider engine names a registry Entry's
// Engine field or an escape-form prefix may name. Like knownParams, it gates
// engine NAMES ONLY — never model names or aliases — so a new model under an
// already-known engine (e.g. a new Claude model) needs no change here.
var knownEngines = map[string]bool{
	"claude": true,
}
