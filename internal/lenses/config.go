// config.go defines the lenses.yaml configuration: the Config decode shape, the embedded seed template, the layered loader, the fan-to-lens resolver and the whole-config load check.

package lenses

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// lensSetFiles holds the shipped language layers, one document per file under sets/.
//
//go:embed sets/*.yaml
var lensSetFiles embed.FS

// lensSets returns each shipped language layer's bytes keyed by its file's base name without .yaml.
// It panics on a read error of the embedded tree, which cannot fail for an embedded path.
func lensSets() map[string][]byte {
	entries, err := lensSetFiles.ReadDir("sets")
	if err != nil {
		panic(fmt.Sprintf("lenses: read the embedded sets directory: %v", err))
	}
	sets := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		data, err := lensSetFiles.ReadFile("sets/" + entry.Name())
		if err != nil {
			panic(fmt.Sprintf("lenses: read the embedded layer %s: %v", entry.Name(), err))
		}
		sets[strings.TrimSuffix(entry.Name(), ".yaml")] = data
	}
	return sets
}

// MaxFanLenses caps the number of lenses a single fan may resolve to, the upper bound on how many parallel fork reviewers one cluster round spawns.
const MaxFanLenses = 16

// Config is the decode shape of lenses.yaml.
type Config struct {
	// LensSet names the embedded language layer whose lenses and fans replace or add base entries by name; empty picks none.
	LensSet string `yaml:"lens_set"`
	// Lenses maps a lens name to its review prose.
	Lenses map[string]string `yaml:"lenses"`
	// Fans maps a fan name to an ordered list of lens names.
	Fans map[string][]string `yaml:"fans"`
}

// Lens is one resolved fan entry: the lens name and the review prose it contributes to a fork's prompt.
type Lens struct {
	Name string
	Text string
}

// ConfigTemplate returns the seed content for lenses.yaml: the standard lens library and its fans, embedded from template.yaml.
func ConfigTemplate() string {
	return configTemplate
}

// ConfigPerNameMaps returns the lenses.yaml keys whose entries are named by the operator: a name the template lacks is kept through reconcile and --set, never pruned.
func ConfigPerNameMaps() []string {
	return []string{"lenses", "fans"}
}

// ConfigSpec returns lenses's layered-load spec.
// Both maps are per-name maps and guard keys: a lens's prose and a fan's lens set are what every review fork is given, so no local override may hold them.
// lens_set is a guard key too, since the language layer it picks is the prose every review fork is given.
// The module is per-worktree, not hub-wide.
// The language layers are the template overlay lens_set picks, read from the base layer only.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:        "lenses",
		Template:    []byte(ConfigTemplate()),
		PerNameMaps: ConfigPerNameMaps(),
		Overlay:     configengine.Overlay{Selector: "lens_set", Documents: lensSets()},
		Categories: map[string]configengine.Category{
			"lens_set": configengine.CategoryGuard,
			"lenses":   configengine.CategoryGuard,
			"fans":     configengine.CategoryGuard,
		},
	}
}

// Load reads lenses.yaml through the layer stack under dirs and decodes the resolved document strictly.
// An absent base file resolves the template, and a lens or fan the stack lacks resolves to its template value, so the returned Config holds every template entry.
// Every error carries the "lenses: " prefix.
func Load(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, fmt.Errorf("lenses: %w", err)
	}
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(resolved))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("lenses: parse the resolved config: %w", err))
	}
	return cfg, nil
}

// ResolveFan resolves a fan name into its ordered lenses, preserving order and repeats.
// It refuses an unknown fan, an empty fan, a fan longer than MaxFanLenses and a fan naming an undefined lens, each naming the fan and carrying the "lenses: " prefix.
// cfg is read alone, so a Config that did not come from Load resolves only the names it holds.
func ResolveFan(cfg Config, name string) ([]Lens, error) {
	entries, ok := cfg.Fans[name]
	if !ok {
		return nil, fmt.Errorf("lenses: unknown fan %q (known fans: %s)", name, strings.Join(fanNames(cfg.Fans), ", "))
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("lenses: fan %q is empty", name)
	}
	if len(entries) > MaxFanLenses {
		return nil, fmt.Errorf("lenses: fan %q has %d entries, exceeding the maximum of %d", name, len(entries), MaxFanLenses)
	}

	lenses := make([]Lens, 0, len(entries))
	for _, lensName := range entries {
		text, ok := cfg.Lenses[lensName]
		if !ok {
			return nil, fmt.Errorf("lenses: fan %q names undefined lens %q", name, lensName)
		}
		lenses = append(lenses, Lens{Name: lensName, Text: text})
	}
	return lenses, nil
}

// CheckLoad loads lenses.yaml under dirs and resolves every fan of the loaded config in name order.
// It returns every failure joined in that order, so the text is deterministic, or only the load's error when the load fails.
func CheckLoad(dirs configengine.Dirs) error {
	cfg, err := Load(dirs)
	if err != nil {
		return err
	}
	var failures []error
	for _, name := range fanNames(cfg.Fans) {
		if _, err := ResolveFan(cfg, name); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// fanNames returns the sorted fan names in fans.
func fanNames(fans map[string][]string) []string {
	names := make([]string, 0, len(fans))
	for name := range fans {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
