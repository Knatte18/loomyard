// config.go — configuration for the perch module.
//
// Defines the Config type mirroring perch.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the perch config file; perch never reads config
// files or knows their on-disk layout itself. judge_model is a model-spec
// string (docs/reference/model-spec.md); ResolveModelSpec is the ONE shared
// implementation LoadConfigWithRegistry and perchcli's decodeProfile both
// call, so the two config surfaces (perch.yaml and profile files) can never
// diverge on grammar, resolution, or the perch-layer params check.

package perchengine

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved perch.yaml configuration: the model and
// effort the progress judge and the asking-triage call default to, and the
// engine-wide default milestone cap ladder a profile may override.
//
// JudgeModel and JudgeEffort stay byte-identical to their pre-migration Go
// shape — this is the engine-facing API the discussion pins as unchanged —
// but on disk perch.yaml now carries a single judge_model model-spec string;
// JudgeEffort's yaml binding is removed (the key no longer exists in the
// file) and its value is instead unpacked by LoadConfigWithRegistry after
// resolving judge_model against the registry.
type Config struct {
	JudgeModel  string `yaml:"judge_model"`
	JudgeEffort string `yaml:"-"`
	RoundCaps   []int  `yaml:"round_caps"`
}

// ResolveModelSpec parses spec against modelspec's grammar and resolves it
// against reg, unpacking the result into the (model, effort) string pair
// perch's Config and Profile structs both carry. It is the single shared
// implementation both perch.yaml's judge_model (config.go) and profile
// files' judge-model/model fields (perchcli's decodeProfile) resolve
// through, so a spec's error shape is structurally identical at both call
// sites rather than merely copy-paste-similar.
//
// After modelspec.Parse (grammar) and reg.Resolve (alias lookup, or the
// escape form's registry-free pass-through, with bracket-over-default
// precedence), every key in the Resolved.Params map OTHER than "effort" is
// rejected loud, naming the offending key and the spec string: this is a
// perch-layer restriction, not a modelspec guarantee — modelspec's own
// known-params set also allows "version" for consumers (e.g. builder) that
// have a field to carry it into, but perch's Config/Profile have no Version
// field, so a spec using one here is an operator mistake, not a value
// perch can silently drop.
func ResolveModelSpec(spec string, reg modelspec.Registry) (model, effort string, err error) {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return "", "", err
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return "", "", err
	}
	for key := range resolved.Params {
		if key != "effort" {
			return "", "", fmt.Errorf("perch: model-spec %q sets param %q, which perch does not accept (only \"effort\" is supported)", spec, key)
		}
	}
	// An absent effort key yields the zero value "", which is exactly
	// today's semantics for a bare alias with no registry default (e.g.
	// the built-in, default-free "haiku") — the provider's own default
	// effort applies.
	return resolved.Model, resolved.Params["effort"], nil
}

// LoadConfigWithRegistry loads and resolves perch.yaml exactly like
// LoadConfig, but resolves judge_model against an ALREADY-LOADED reg
// instead of loading models.yaml itself. Callers that also resolve other
// model-spec fields in the same invocation (perchcli, which resolves
// profile fields too) call this directly so models.yaml is read exactly
// once per invocation; LoadConfig below is the additive convenience wrapper
// that loads its own registry for callers with nothing else to resolve.
//
// The resolved bytes are decoded with a strict yaml.Decoder —
// KnownFields(true) — rather than plain yaml.Unmarshal: an old perch.yaml
// still carrying a leftover judge_effort key (from before this migration)
// fails loud here with yaml's unknown-field error, since Config no longer
// binds that key to any field. This is the actual enforcement point for the
// migration's fail-loud posture — configengine.Load's MissingKeys check
// only rejects keys the template requires that the file lacks, never extra
// keys the file carries, so deleting judge_effort from template.yaml alone
// would NOT have rejected an old file.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfigWithRegistry(baseDir, module string, reg modelspec.Registry) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the perch-specific
		// hint, matching reedengine/shuttleengine's shape so every module
		// surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(resolved))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal perch config: %w", err)
	}

	// Resolution runs once here, at config load — never deferred to a
	// profile's default-chain lookup — so cfg.JudgeModel/JudgeEffort carry
	// already-resolved values by the time Profile.validate reads them.
	model, effort, err := ResolveModelSpec(cfg.JudgeModel, reg)
	if err != nil {
		return Config{}, fmt.Errorf("perch config key \"judge_model\": %w", err)
	}
	cfg.JudgeModel = model
	cfg.JudgeEffort = effort

	return cfg, nil
}

// LoadConfig loads and unmarshals configuration for the perch module.
//
// It loads models.yaml itself via modelspec.LoadRegistry(baseDir) — the
// same anchor LoadConfigWithRegistry's caller would use — and delegates.
// The module name is threaded through by the caller (never hardcoded to
// "perch" here), mirroring reedengine.LoadConfig and shuttleengine.LoadConfig.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	reg, err := modelspec.LoadRegistry(baseDir)
	if err != nil {
		return Config{}, err
	}
	return LoadConfigWithRegistry(baseDir, module, reg)
}
