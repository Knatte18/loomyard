// config.go — configuration for the batcher module.
//
// Loads batcher.yaml through the layer stack and builds the batchifier a named profile describes.

package batcher

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name.
// It is a constant rather than a parameter, since loading always reads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the named profiles.
type config struct {
	Profiles map[string]profile `yaml:"profiles"`
}

// profile is one named entry under the batcher.profiles key.
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

// ErrRetiredKey marks a batcher.yaml key the module no longer reads, such as a cost parameter the model dropped;
// such an error is also marked configengine.ErrInvalid.
var ErrRetiredKey = errors.New("retired key")

// retiredKeyError builds the refusal for a layer file that sets the retired key name, naming that file.
// It wraps ErrRetiredKey, is marked configengine.ErrInvalid, and ends in the way forward that layer calls for:
// delete the key by hand, or, when reconcileRemoves says a reconcile drops it, run that layer's reconcile.
// Only a reconcile of the default layer rewrites the repo default.
// Its way forward is to delete the key by hand.
// It is also to run lym config reconcile --layer default --apply from the hub's prime worktree.
// A session lym refuses that verb and reports status: FAILED, so the orch runs it.
// A non-empty instead opens the way forward with where the setting lives now.
func retiredKeyError(file string, layer configengine.Layer, name string, reconcileRemoves bool, instead string) error {
	if file == "" {
		file = moduleName + ".yaml"
	}
	wayForward := fmt.Sprintf("delete %s by hand", name)
	if reconcileRemoves {
		if layer == configengine.LayerDefault {
			wayForward = fmt.Sprintf(`%s, or run "lym config reconcile --layer default --apply" from the hub's prime worktree, which removes a key the template lacks; a session lym refuses the verb from reports status: FAILED and the orch runs it`, wayForward)
		} else {
			wayForward = fmt.Sprintf(`%s, or run "lym config reconcile --apply", which removes a key the template lacks`, wayForward)
		}
	}
	if instead != "" {
		wayForward = instead + "; then " + wayForward
	}
	return configengine.MarkInvalid(fmt.Errorf("%s sets %s, which no longer exists (%w); way forward: %s", file, name, ErrRetiredKey, wayForward))
}

// retiredProfileKeyError builds the refusal for a profile that carries the retired key below it.
// A reconcile removes the key only from a profile the template names;
// it keeps a profile of the operator's own whole, so that profile's way forward is the hand delete alone.
func retiredProfileKeyError(file string, layer configengine.Layer, profileName, key string) error {
	return retiredKeyError(file, layer, profileKeyName(profileName, key), templateNamesProfile(profileName), "")
}

// retiredActiveError builds the refusal for a layer file that still sets the retired top-level active key.
// Each implementer now names its own profile, so the way forward opens with those keys.
func retiredActiveError(file string, layer configengine.Layer) error {
	return retiredKeyError(file, layer, keyName(retiredActiveKey), true, activeInstead())
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
// The profiles per-name map only shapes batching and meets no category, so the local layer may hold it.
// The retired active key held no category either, so a local file holding it reaches the load's own refusal,
// and its way forward names the keys that choose a profile now;
// a task's run settings that set it are refused with the same keys as the way forward.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:        moduleName,
		Template:    []byte(ConfigTemplate()),
		PerNameMaps: ConfigPerNameMaps(),
		Categories: map[string]configengine.Category{
			profilesKey: configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			retiredActiveKey: configengine.CategoryNone,
		},
		RetiredWayForward: map[string]string{
			retiredActiveKey: activeInstead(),
		},
	}
}

// activeInstead names where the profile choice the retired active key held is set now.
func activeInstead() string {
	return fmt.Sprintf("name each implementer's profile in its own config instead, %s and %s",
		configengine.KeyName(websterModule, ChoiceKey), configengine.KeyName(weaverModule, ChoiceKey))
}

// Named loads batcher.yaml through the layer stack under dirs and builds the batchifier the profile named name describes.
// chooser is the full name of the config key that chose name, which a refusal of the name cites.
// An empty name and identity both resolve to the identity batchifier, even with no profile of that name;
// a configured profile named identity wins over that default.
// An absent base layer resolves the embedded ConfigTemplate() instead of erroring,
// but a layer file that exists and is invalid still errors.
// A name naming no profile is an error, as is a profile of an unknown batchifier kind.
// A cost profile with a missing or invalid parameter is an error too.
// A retired active, alone_above, master_base or startup_context in any layer is an error as well.
// Each such error is marked configengine.ErrInvalid.
// A retired-key error names the layer file that carries the key and the way forward that file calls for.
// The caller must resolve dirs first; Named never resolves the working directory itself.
func Named(dirs configengine.Dirs, name, chooser string) (Batcher, error) {
	cfg, err := loadConfig(dirs)
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = DefaultName
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		if name == DefaultName {
			return Identity(), nil
		}
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml: %s %q names no profile under %s", chooser, name, keyName(profilesKey)))
	}
	return buildProfile(name, prof)
}

// LoadProfiles loads batcher.yaml through the layer stack under dirs and builds every profile it holds, keyed by name.
// It refuses what Named refuses of the file and of any one profile, so a check of the file passes
// only when every profile an implementer could name builds.
func LoadProfiles(dirs configengine.Dirs) (map[string]Batcher, error) {
	cfg, err := loadConfig(dirs)
	if err != nil {
		return nil, err
	}
	built := make(map[string]Batcher, len(cfg.Profiles))
	for _, name := range slices.Sorted(maps.Keys(cfg.Profiles)) {
		batchifier, err := buildProfile(name, cfg.Profiles[name])
		if err != nil {
			return nil, err
		}
		built[name] = batchifier
	}
	return built, nil
}

// buildProfile builds the batchifier of the profile named name through the registry.
func buildProfile(name string, prof profile) (Batcher, error) {
	construct, ok := constructors[prof.Batchifier]
	if !ok {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml: %s names an unknown batchifier %q", profileKeyName(name, batchifierLeaf), prof.Batchifier))
	}
	return construct(name, prof)
}

// loadConfig loads and unmarshals batcher.yaml through the layer stack under dirs.
// It degrades to the embedded template on proven absence.
// It refuses a layer that still sets the retired active key, naming the highest such layer.
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

	stack, err := configengine.StackLayers(dirs, spec)
	if err != nil {
		return config{}, err
	}
	origins, err := layerOrigins(stack.Layers)
	if err != nil {
		return config{}, err
	}
	if origins.active != nil {
		return config{}, retiredActiveError(origins.active.Path, origins.active.Layer)
	}
	for name, prof := range cfg.Profiles {
		if origin, held := origins.profiles[name]; held {
			prof.file, prof.layer = origin.Path, origin.Layer
			cfg.Profiles[name] = prof
		}
	}
	return cfg, nil
}

// origins records the highest layer holding each batcher.yaml unit a refusal names.
type origins struct {
	// profiles maps each profile name a layer holds under profiles: to the highest layer holding it.
	profiles map[string]configengine.LayerDoc
	// active is the highest layer that sets the retired active key; nil when none does.
	active *configengine.LayerDoc
}

// layerOrigins finds, across layers in stack order, the highest layer holding each profile and the retired active key.
// A layer whose profiles: is no mapping is an error marked configengine.ErrInvalid.
func layerOrigins(layers []configengine.LayerDoc) (origins, error) {
	found := origins{profiles: map[string]configengine.LayerDoc{}}
	for _, layer := range layers {
		var document struct {
			Active   *yaml.Node           `yaml:"active"`
			Profiles map[string]yaml.Node `yaml:"profiles"`
		}
		if err := yaml.Unmarshal(layer.Bytes, &document); err != nil {
			return origins{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config %s: %w", layer.Path, err))
		}
		if document.Active != nil {
			found.active = &layer
		}
		for name := range document.Profiles {
			found.profiles[name] = layer
		}
	}
	return found, nil
}

// unmarshalConfig decodes batcher.yaml's bytes, marking a failure configengine.ErrInvalid.
func unmarshalConfig(data []byte) (config, error) {
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batcher config: %w", err))
	}
	return cfg, nil
}
