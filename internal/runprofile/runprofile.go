// runprofile.go — the profiles.yaml template, its layered-load spec, and the loader and lookups of the hub's run profiles and each recipe's default profile.

package runprofile

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// ModuleName is the config module name of the profiles file, profiles.yaml.
const ModuleName = "profiles"

// DefaultsEntry is the reserved top-level entry of profiles.yaml that maps a recipe name to the profile a task of that recipe takes when it names none.
const DefaultsEntry = "defaults"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the seed content of profiles.yaml: the starting profiles and defaults a hub's first reconcile writes when the file is absent.
func ConfigTemplate() string {
	return configTemplate
}

// ConfigSpec returns the module's layered-load spec.
// It is hub-wide, and each profile, like the defaults entry, is one whole entry set from a JSON object.
// No entry is open to the local layer: each template entry is classified cross-pair, since every pair naming it reads it, and a hub-wide module refuses an unclassified one.
func ConfigSpec() configengine.Spec {
	var templateEntries map[string]any
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &templateEntries); err != nil {
		panic(fmt.Sprintf("runprofile: parse the embedded template: %v", err))
	}
	categories := make(map[string]configengine.Category, len(templateEntries))
	for name := range templateEntries {
		categories[name] = configengine.CategoryCrossPair
	}
	return configengine.Spec{
		Name:         ModuleName,
		Template:     []byte(ConfigTemplate()),
		HubWide:      true,
		WholeEntries: true,
		JSONEntries:  true,
		SeedOnly:     true,
		Categories:   categories,
	}
}

// Profile maps a recipe name to that recipe's settings: module name to a mapping of that module's config keys, the shape of a board task's run_settings.
type Profile map[string]map[string]any

// Profiles holds the hub's run profiles by name and, under Defaults, the profile name each recipe takes when a task names none.
type Profiles struct {
	ByName   map[string]Profile
	Defaults map[string]string
}

// Names returns the profile names in sorted order.
func (p Profiles) Names() []string {
	return slices.Sorted(maps.Keys(p.ByName))
}

// Lookup returns the named profile.
// An unknown name is an error naming the known profiles and the way forward.
func (p Profiles) Lookup(name string) (Profile, error) {
	profile, ok := p.ByName[name]
	if !ok {
		return nil, fmt.Errorf("unknown run profile %q (known: %s); way forward: name a known profile in the task's profile field, or add it with \"lym config profiles --set %s='<json object>'\" from the hub's prime worktree", name, strings.Join(p.Names(), ", "), name)
	}
	return profile, nil
}

// Settings returns the named profile's settings for recipe, module name to keys.
// An unknown name, or a profile holding no block for recipe, is an error naming the way forward.
func (p Profiles) Settings(name, recipe string) (map[string]any, error) {
	profile, err := p.Lookup(name)
	if err != nil {
		return nil, err
	}
	settings, ok := profile[recipe]
	if !ok {
		return nil, fmt.Errorf("run profile %q holds no %s block (it covers: %s); way forward: name a profile that covers %s in the task's profile field, or add a %s block to %q with \"lym config profiles\" from the hub's prime worktree", name, recipe, strings.Join(slices.Sorted(maps.Keys(profile)), ", "), recipe, recipe, name)
	}
	return settings, nil
}

// Default returns the profile name recipe takes when a task names none, empty when the hub names no default for it.
func (p Profiles) Default(recipe string) string {
	return p.Defaults[recipe]
}

// Load reads the hub's run profiles through the layer stack under dirs.
// The merged layers are decoded as they stand, never filled from the template, so a profile removed from the file stays removed;
// with no layer file present the template's profiles are returned.
// Each profile must be a mapping of recipe name to a mapping of module name to a mapping of keys,
// and each default must name a profile holding a block for its recipe; an error names the entry and the file.
func Load(dirs configengine.Dirs) (Profiles, error) {
	stack, err := configengine.StackLayers(dirs, ConfigSpec())
	if err != nil {
		return Profiles{}, fmt.Errorf("runprofile: %w", err)
	}
	document, source := stack.Doc, "the merged layers"
	if len(stack.Layers) == 0 {
		document, source = []byte(ConfigTemplate()), "the profiles template"
	}
	return decode(document, source)
}

// decode parses a profiles document into Profiles, an empty or comments-only document yielding none.
// source names the document in an error.
func decode(document []byte, source string) (Profiles, error) {
	profiles := Profiles{ByName: map[string]Profile{}, Defaults: map[string]string{}}
	var raw map[string]any
	if err := yaml.NewDecoder(bytes.NewReader(document)).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return profiles, nil
		}
		return Profiles{}, configengine.MarkInvalid(fmt.Errorf("runprofile: parse %s: %w", source, err))
	}
	for name, value := range raw {
		if name == "" {
			return Profiles{}, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: a profile name must not be empty", source))
		}
		if name == DefaultsEntry {
			continue
		}
		profile, err := decodeProfile(name, value)
		if err != nil {
			return Profiles{}, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: %w; way forward: give each profile one block per recipe, recipe name to module name to keys, with \"lym config profiles\"", source, err))
		}
		profiles.ByName[name] = profile
	}
	if value, ok := raw[DefaultsEntry]; ok {
		defaults, err := decodeDefaults(value, profiles)
		if err != nil {
			return Profiles{}, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: %w; way forward: map each recipe to a profile that covers it under %s with \"lym config profiles\"", source, err, DefaultsEntry))
		}
		profiles.Defaults = defaults
	}
	return profiles, nil
}

// decodeProfile checks that value, the profile called name, is a mapping of recipe name to a mapping of module name to a mapping of keys, and returns it.
func decodeProfile(name string, value any) (Profile, error) {
	recipes, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("profile %q must be a mapping of recipe name to settings", name)
	}
	profile := make(Profile, len(recipes))
	for recipe, settings := range recipes {
		modules, ok := settings.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("profile %q: recipe %q must be a mapping of module name to keys", name, recipe)
		}
		for module, keys := range modules {
			if _, ok := keys.(map[string]any); !ok {
				return nil, fmt.Errorf("profile %q: recipe %q: module %q must be a mapping of keys", name, recipe, module)
			}
		}
		profile[recipe] = modules
	}
	return profile, nil
}

// decodeDefaults checks that value is a mapping of recipe name to the name of a profile in profiles holding a block for that recipe, and returns it.
// The DefaultsEntry entry is never read as a profile, since decode parses it only through this function.
func decodeDefaults(value any, profiles Profiles) (map[string]string, error) {
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a mapping of recipe name to profile name", DefaultsEntry)
	}
	defaults := make(map[string]string, len(entries))
	for recipe, entry := range entries {
		name, ok := entry.(string)
		if !ok {
			return nil, fmt.Errorf("%s: recipe %q must name a profile", DefaultsEntry, recipe)
		}
		profile, ok := profiles.ByName[name]
		if !ok {
			return nil, fmt.Errorf("%s: recipe %q names unknown profile %q", DefaultsEntry, recipe, name)
		}
		if _, ok := profile[recipe]; !ok {
			return nil, fmt.Errorf("%s: recipe %q names profile %q, which holds no %s block", DefaultsEntry, recipe, name, recipe)
		}
		defaults[recipe] = name
	}
	return defaults, nil
}
