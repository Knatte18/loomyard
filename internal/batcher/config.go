// config.go — configuration for the batcher module.
//
// Defines the package-local config type mirroring batcher.yaml's keys, ConfigOpenMaps, ConfigSpec, and Active, which uses internal/configengine.LoadOrTemplate with ConfigSpec() to resolve batcher's layered config, degrading to the embedded template on proven absence, then builds the batchifier the active profile names.
// Also defines MigrateConfig, the pure rewrite that removes a batcher.yaml's retired keys.

package batcher

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name, passed to configengine.LoadOrTemplate — never
// a parameter, since Active always loads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the name of the active profile and the named profiles.
type config struct {
	Active   string             `yaml:"active"`
	Profiles map[string]profile `yaml:"profiles"`
}

// profile is one named entry under batcher.yaml's profiles: key.
// The cost parameters are pointers so a missing key is told apart from a zero.
// AloneAbove is the retired cost parameter, read only so a profile still carrying it is refused rather than silently ignored.
// file and layer are the layer file the stack took the profiles: map from, kept so a retired-key refusal names the file that carries the key.
type profile struct {
	Batchifier string             `yaml:"batchifier"`
	AloneAbove *float64           `yaml:"alone_above"`
	Budget     *float64           `yaml:"budget"`
	MaxCards   *int               `yaml:"max_cards"`
	Weights    map[string]float64 `yaml:"weights"`

	file  string
	layer configengine.Layer
}

// ErrRetiredKey marks a batcher.yaml profile that still carries a key the cost model no longer has;
// such an error is also marked configengine.ErrInvalid.
var ErrRetiredKey = errors.New("retired key")

// retiredKeyError builds the refusal for a profile still carrying the retired key, naming the layer file that carries it.
// It wraps ErrRetiredKey, is marked configengine.ErrInvalid, and ends in the way forward that layer calls for:
// lyx config reconcile --apply for a worktree or local file, and for the repo default, which only the hub-wide reconcile rewrites, lyx fabric reconcile run by the orch.
func retiredKeyError(file string, layer configengine.Layer, profileName, key string) error {
	if file == "" {
		file = moduleName + ".yaml"
	}
	wayForward := `run "lyx config reconcile --apply"`
	if layer == configengine.LayerDefault {
		wayForward = `run "lyx fabric reconcile"; a session lyx refuses the verb from reports status: FAILED and the orch runs it`
	}
	return configengine.MarkInvalid(fmt.Errorf("%s profile %q carries %s, which no longer exists (%w); way forward: %s", file, profileName, key, ErrRetiredKey, wayForward))
}

// ConfigOpenMaps returns the batcher.yaml keys whose entries are the operator's own, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"profiles"}
}

// ConfigSpec returns batcher's layered-load spec.
// Both keys, the active profile name and the profiles open map, only shape batching and meet no category, so the local layer may hold either.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     moduleName,
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"active":   configengine.CategoryNone,
			"profiles": configengine.CategoryNone,
		},
	}
}

// Active loads batcher.yaml through the layer stack under dirs and builds the batchifier its active: profile names.
// An empty active: and active: identity resolve to the identity batchifier even when no profile of that name is configured;
// a configured profile named identity wins over that default.
// An absent base layer resolves the embedded ConfigTemplate(), whose active: names the cautious profile, instead of erroring;
// a layer file that exists but is invalid still errors, and so does an active: naming no profile, a profile of an unknown batchifier kind, or a cost profile with a missing or invalid parameter, or a retired alone_above, master_base or startup_context in any layer, each marked configengine.ErrInvalid.
// A retired-key error names the layer file that carries the key and the way forward that file calls for.
// dirs must already be told by the caller — Active never resolves cwd itself (see
// PATTERN-cwd-resolution).
func Active(dirs configengine.Dirs) (Batcher, error) {
	cfg, err := loadConfig(dirs)
	if err != nil {
		return nil, err
	}

	name := cfg.Active
	if name == "" {
		name = DefaultName
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		if name == DefaultName {
			return Identity(), nil
		}
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml: active %q names no profile under profiles:", name))
	}
	construct, ok := constructors[prof.Batchifier]
	if !ok {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q has unknown batchifier %q", name, prof.Batchifier))
	}
	return construct(name, prof)
}

// loadConfig loads and unmarshals batcher.yaml through the layer stack under dirs, degrading to the embedded template on proven absence.
// Each profile records the layer file the stack took profiles: from.
func loadConfig(dirs configengine.Dirs) (config, error) {
	spec := ConfigSpec()
	resolved, err := configengine.LoadOrTemplate(dirs, spec)
	if err != nil {
		return config{}, err
	}
	cfg, err := unmarshalConfig(resolved)
	if err != nil {
		return config{}, err
	}
	if len(cfg.Profiles) == 0 {
		return cfg, nil
	}

	stack, err := configengine.StackLayers(dirs, spec)
	if err != nil {
		return config{}, err
	}
	origin, ok := stack.Origins["profiles"]
	if !ok {
		return cfg, nil
	}
	for name, prof := range cfg.Profiles {
		prof.file, prof.layer = origin.Path, origin.Layer
		cfg.Profiles[name] = prof
	}
	return cfg, nil
}

// unmarshalConfig decodes batcher.yaml's bytes, marking a failure configengine.ErrInvalid.
func unmarshalConfig(data []byte) (config, error) {
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config: %w", err))
	}
	return cfg, nil
}

// MigrateConfig rewrites a batcher.yaml document so no profile under profiles: carries a retired key, and returns it with one sorted human-readable line per change, such as "profiles.cautious.weights.master_base: removed".
// It drops each profile's alone_above, weights.startup_context and weights.master_base;
// where a profile carried either weights key and has no orientation of its own, it adds weights.orientation at the value of the template's cautious profile, and an existing orientation is kept.
// Everything else, inside and outside profiles:, is carried through yaml node editing with its comments.
// A document with no retired key, or an empty one, comes back unchanged with no rewrites.
// A document that does not parse returns an error marked configengine.ErrInvalid.
// The loader never calls it; the migration runs only from lyx config reconcile.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	if _, err := unmarshalConfig(existing); err != nil {
		return nil, nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	profiles := mappingValue(document.Content[0], "profiles")
	if profiles == nil || profiles.Kind != yaml.MappingNode {
		return existing, nil, nil
	}
	orientation, err := templateOrientation()
	if err != nil {
		return nil, nil, err
	}

	var rewrites []string
	for i := 0; i+1 < len(profiles.Content); i += 2 {
		name, entry := profiles.Content[i].Value, profiles.Content[i+1]
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if _, removed := dropKey(entry, "alone_above"); removed {
			rewrites = append(rewrites, "profiles."+name+".alone_above: removed")
		}
		weights := mappingValue(entry, "weights")
		if weights == nil || weights.Kind != yaml.MappingNode {
			continue
		}
		// The lowest index across the drops is the one the second drop leaves valid.
		insertAt := -1
		for _, retired := range []string{"master_base", "startup_context"} {
			if index, removed := dropKey(weights, retired); removed {
				rewrites = append(rewrites, "profiles."+name+".weights."+retired+": removed")
				if insertAt < 0 || index < insertAt {
					insertAt = index
				}
			}
		}
		if insertAt >= 0 && mappingValue(weights, "orientation") == nil {
			key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "orientation"}
			weights.Content = append(weights.Content[:insertAt], append([]*yaml.Node{key, orientation}, weights.Content[insertAt:]...)...)
			rewrites = append(rewrites, "profiles."+name+".weights.orientation: added "+orientation.Value)
		}
	}
	if len(rewrites) == 0 {
		return existing, nil, nil
	}
	sort.Strings(rewrites)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, fmt.Errorf("encode migrated batcher config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode migrated batcher config: %w", err)
	}
	return out.Bytes(), rewrites, nil
}

// templateOrientation returns the orientation scalar of the template's cautious profile.
func templateOrientation() (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &document); err != nil {
		return nil, fmt.Errorf("unmarshal batcher template: %w", err)
	}
	profiles := mappingValue(document.Content[0], "profiles")
	cautious := mappingValue(profiles, "cautious")
	orientation := mappingValue(mappingValue(cautious, "weights"), "orientation")
	if orientation == nil {
		return nil, errors.New("batcher template: cautious profile has no weights.orientation")
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: orientation.Tag, Value: orientation.Value}, nil
}

// mappingValue returns the value node stored under key in mapping, or nil when mapping is nil, not a mapping or lacks the key.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// dropKey removes key and its value from mapping, reporting the index its pair held in mapping.Content.
func dropKey(mapping *yaml.Node, key string) (index int, removed bool) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return i, true
		}
	}
	return 0, false
}
