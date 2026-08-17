// config.go — configuration for the perch module.
//
// Defines the Config type mirroring perch.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the perch config file,
// degrading to the embedded template on proven absence;
// perch never reads config files or knows their on-disk layout itself.
// judge_model is a model-spec string (contracts/specs/llm-model-spec.md);
// ResolveModelSpec is the ONE shared implementation LoadConfigWithRegistry and perchcli's
// decodeProfile both call, so the two config surfaces (perch.yaml and profile files) can never
// diverge on grammar, resolution, or the perch-layer params check.

package perchengine

import (
	"bytes"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents resolved perch.yaml configuration: judge model/effort defaults and the
// milestone cap ladder.
type Config struct {
	JudgeModel  string `yaml:"judge_model"`
	JudgeEffort string `yaml:"-"`
	RoundCaps   []int  `yaml:"round_caps"`
}

// ResolveModelSpec parses spec against modelspec's grammar and resolves it against reg, unpacking
// the result into (model, effort) pair.
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

// LoadConfigWithRegistry loads and resolves perch.yaml against an already-loaded registry.
// An absent <baseDir>/_lyx/ directory or an absent config file resolves the embedded template;
// a config file that exists but is invalid still errors.
func LoadConfigWithRegistry(baseDir, module string, reg modelspec.Registry) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
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
func LoadConfig(baseDir, module string) (Config, error) {
	reg, err := modelspec.LoadRegistry(baseDir)
	if err != nil {
		logger.Warn("perch: load model registry failed", "baseDir", baseDir, "err", err)
		return Config{}, err
	}
	return LoadConfigWithRegistry(baseDir, module, reg)
}
