// config.go — configuration for the batcher module.
//
// Loads batcher.yaml through the layer stack and builds the batchifier its active profile names.

package batcher

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name.
// It is a constant rather than a parameter, since loading always reads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the name of the active profile and the named profiles.
type config struct {
	Active   string             `yaml:"active"`
	Profiles map[string]profile `yaml:"profiles"`
}

// profile is one named entry under batcher.yaml's profiles: key.
// The cost parameters are pointers so a missing key is told apart from a zero.
// AloneAbove is a retired cost parameter;
// it is read only so a profile still carrying it is refused, not silently ignored.
// file and layer are the highest layer file holding the profile;
// they let a retired-key refusal name the file that carries the key.
type profile struct {
	Batchifier string             `yaml:"batchifier"`
	AloneAbove *float64           `yaml:"alone_above"`
	Budget     *float64           `yaml:"budget"`
	MaxCards   *int               `yaml:"max_cards"`
	Weights    map[string]float64 `yaml:"weights"`

	file  string
	layer configengine.Layer
}

// ErrRetiredKey marks a batcher.yaml profile that carries a retired key, one the cost model does not use;
// such an error is also marked configengine.ErrInvalid.
var ErrRetiredKey = errors.New("retired key")

// retiredKeyError builds the refusal for a profile that carries a retired key, naming the layer file holding it.
// It wraps ErrRetiredKey, is marked configengine.ErrInvalid, and ends in the way forward that layer calls for:
// delete the key by hand or run lym config reconcile --apply for a worktree or local file.
// Only a reconcile of the default layer rewrites the repo default.
// Its way forward is to delete the key by hand.
// It is also to run lym config reconcile --layer default --apply from the hub's prime worktree.
// A session lym refuses that verb and reports status: FAILED, so the orch runs it.
// A profile the template does not name is kept whole by reconcile, so its way forward is the hand delete alone.
func retiredKeyError(file string, layer configengine.Layer, profileName, key string) error {
	if file == "" {
		file = moduleName + ".yaml"
	}
	wayForward := fmt.Sprintf("delete %s from the profile by hand", key)
	if templateNamesProfile(profileName) {
		if layer == configengine.LayerDefault {
			wayForward = fmt.Sprintf(`%s, or run "lym config reconcile --layer default --apply" from the hub's prime worktree, which removes a key the template lacks; a session lym refuses the verb from reports status: FAILED and the orch runs it`, wayForward)
		} else {
			wayForward = fmt.Sprintf(`%s, or run "lym config reconcile --apply", which removes a key the template lacks`, wayForward)
		}
	}
	return configengine.MarkInvalid(fmt.Errorf("%s profile %q carries %s, which no longer exists (%w); way forward: %s", file, profileName, key, ErrRetiredKey, wayForward))
}

// templateNamesProfile reports whether the shipped template holds a profile of the given name.
func templateNamesProfile(profileName string) bool {
	var template struct {
		Profiles map[string]any `yaml:"profiles"`
	}
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &template); err != nil {
		return false
	}
	_, named := template.Profiles[profileName]
	return named
}

// ConfigPerNameMaps returns the batcher.yaml keys whose entries are named by the operator:
// a layer's profiles add to or override the template's by name, each profile merging key by key,
// and a name the template lacks is kept through reconcile and --set.
func ConfigPerNameMaps() []string {
	return []string{"profiles"}
}

// ConfigSpec returns batcher's layered-load spec.
// Both keys, the active profile name and the profiles per-name map, only shape batching and meet no category,
// so the local layer may hold either.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:        moduleName,
		Template:    []byte(ConfigTemplate()),
		PerNameMaps: ConfigPerNameMaps(),
		Categories: map[string]configengine.Category{
			"active":   configengine.CategoryNone,
			"profiles": configengine.CategoryNone,
		},
	}
}

// Active loads batcher.yaml through the layer stack under dirs and builds the batchifier its active: profile names.
// An empty active: and active: identity both resolve to the identity batchifier, even with no profile of that name;
// a configured profile named identity wins over that default.
// An absent base layer resolves the embedded ConfigTemplate() instead of erroring,
// but a layer file that exists and is invalid still errors.
// An active: naming no profile is an error, as is a profile of an unknown batchifier kind.
// A cost profile with a missing or invalid parameter is an error too.
// A retired alone_above, master_base or startup_context in any layer is an error as well.
// Each such error is marked configengine.ErrInvalid.
// A retired-key error names the layer file that carries the key and the way forward that file calls for.
// The caller must resolve dirs first; Active never resolves the working directory itself.
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

// loadConfig loads and unmarshals batcher.yaml through the layer stack under dirs.
// It degrades to the embedded template on proven absence.
// Each profile records the highest layer file holding it.
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
	origins, err := profileOrigins(stack.Layers)
	if err != nil {
		return config{}, err
	}
	for name, prof := range cfg.Profiles {
		if origin, held := origins[name]; held {
			prof.file, prof.layer = origin.Path, origin.Layer
			cfg.Profiles[name] = prof
		}
	}
	return cfg, nil
}

// profileOrigins maps each profile name a layer file holds under profiles: to the highest layer holding it.
// A layer whose profiles: is no mapping is an error marked configengine.ErrInvalid.
func profileOrigins(layers []configengine.LayerDoc) (map[string]configengine.LayerDoc, error) {
	origins := map[string]configengine.LayerDoc{}
	for _, layer := range layers {
		var document struct {
			Profiles map[string]yaml.Node `yaml:"profiles"`
		}
		if err := yaml.Unmarshal(layer.Bytes, &document); err != nil {
			return nil, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config %s: %w", layer.Path, err))
		}
		for name := range document.Profiles {
			origins[name] = layer
		}
	}
	return origins, nil
}

// unmarshalConfig decodes batcher.yaml's bytes, marking a failure configengine.ErrInvalid.
func unmarshalConfig(data []byte) (config, error) {
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config: %w", err))
	}
	return cfg, nil
}
