// runprofile.go — the profiles.yaml template, its layered-load spec, and the loader and lookup of the hub's run profiles.

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

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the seed content of profiles.yaml: the starting profiles a hub's first reconcile writes when the file is absent.
func ConfigTemplate() string {
	return configTemplate
}

// ConfigSpec returns the module's layered-load spec.
// It is hub-wide, and each profile is one whole entry set from a JSON object.
// No profile is open to the local layer: each template profile is classified cross-pair, since every pair naming it reads it, and a hub-wide module refuses an unclassified one.
func ConfigSpec() configengine.Spec {
	var templateProfiles map[string]any
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &templateProfiles); err != nil {
		panic(fmt.Sprintf("runprofile: parse the embedded template: %v", err))
	}
	categories := make(map[string]configengine.Category, len(templateProfiles))
	for name := range templateProfiles {
		categories[name] = configengine.CategoryCrossPair
	}
	return configengine.Spec{
		Name:         ModuleName,
		Template:     []byte(ConfigTemplate()),
		HubWide:      true,
		WholeEntries: true,
		JSONEntries:  true,
		Categories:   categories,
	}
}

// Profiles maps a profile name to its settings: module name to a mapping of that module's config keys, the shape of a board task's run_settings.
type Profiles map[string]map[string]any

// Names returns the profile names in sorted order.
func (p Profiles) Names() []string {
	return slices.Sorted(maps.Keys(p))
}

// Lookup returns the settings of the named profile.
// An unknown name is an error naming the known profiles and the way forward.
func (p Profiles) Lookup(name string) (map[string]any, error) {
	settings, ok := p[name]
	if !ok {
		return nil, fmt.Errorf("unknown run profile %q (known: %s); way forward: name a known profile in the task's profile field, or add it with \"lyx config profiles --set %s='<json object>'\" from the hub's prime worktree", name, strings.Join(p.Names(), ", "), name)
	}
	return settings, nil
}

// Load reads the hub's run profiles through the layer stack under dirs.
// The merged layers are decoded as they stand, never filled from the template, so a profile removed from the file stays removed;
// with no layer file present the template's profiles are returned.
// Each profile must be a mapping of module name to a mapping of keys, and an error names the profile and the file.
func Load(dirs configengine.Dirs) (Profiles, error) {
	stack, err := configengine.StackLayers(dirs, ConfigSpec())
	if err != nil {
		return nil, fmt.Errorf("runprofile: %w", err)
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
	var raw map[string]any
	if err := yaml.NewDecoder(bytes.NewReader(document)).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return Profiles{}, nil
		}
		return nil, configengine.MarkInvalid(fmt.Errorf("runprofile: parse %s: %w", source, err))
	}
	profiles := make(Profiles, len(raw))
	for name, value := range raw {
		if name == "" {
			return nil, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: a profile name must not be empty", source))
		}
		modules, ok := value.(map[string]any)
		if !ok {
			return nil, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: profile %q must be a mapping of module name to keys", source, name))
		}
		for module, keys := range modules {
			if _, ok := keys.(map[string]any); !ok {
				return nil, configengine.MarkInvalid(fmt.Errorf("runprofile: %s: profile %q: module %q must be a mapping of keys", source, name, module))
			}
		}
		profiles[name] = modules
	}
	return profiles, nil
}
