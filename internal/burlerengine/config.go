// config.go defines the lens/fan configuration for burler cluster review: the
// Config decode shape, the embedded seed template, the direct-read loader,
// and the fan-to-lens resolver.
//
// burler.yaml is registered in configreg as a SEED-ONLY module (see
// configreg.go): its lens/fan key set is open-ended and operator-owned, the
// same posture models.yaml has for its aliases. Reconciling this file like an
// ordinary module would route it through yamlengine.Reconcile, whose default
// merge reports every operator-added lens or fan as "removed" (they are not
// present in the template) and deletes them on the next --apply — seed-only
// materialization (configsync.ReconcileAll) instead writes the template
// VERBATIM once, only when the file is absent, and never rewrites a present
// file again. Consequently LoadConfig follows modelspec.LoadRegistry's
// direct-read pattern — os.ReadFile plus a strict top-level decode — rather
// than configengine.Load, whose MissingKeys gate would also wrongly reject an
// operator who deliberately removed a standard lens they don't want.

package burlerengine

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// maxClusterN caps the number of lenses a single fan may resolve to — the
// upper bound on how many parallel fork reviewers one cluster round spawns.
const maxClusterN = 16

// Config is the decode shape of burler.yaml: a name->prose map of lenses and
// a name->lens-list map of fans. Both maps are open-ended and operator-owned
// — LoadConfig's strict decode rejects any OTHER top-level field, but places
// no closed vocabulary on the map keys or list entries themselves, mirroring
// modelspec.Registry's models.yaml precedent.
type Config struct {
	// Lenses maps a lens name to its emphasis-steering review prose.
	Lenses map[string]string `yaml:"lenses"`
	// Fans maps a fan name to an ordered list of lens names. A lens name may
	// repeat within one fan; ResolveFan preserves both the order and the
	// repeats.
	Fans map[string][]string `yaml:"fans"`
}

// Lens is one resolved fan entry: the lens name and the review prose it
// contributes to a fork's prompt.
type Lens struct {
	Name string
	Text string
}

// ConfigTemplate returns the seed content for burler.yaml: the standard lens
// library and the standard/full fans, embedded verbatim from template.yaml.
// configreg consumes this as the module's Template function; per the
// seed-only reconcile decision (configsync.ReconcileAll), it is written to
// disk ONCE, when burler.yaml is absent, and never rewritten afterward — every
// lens or fan an operator adds, edits, or removes survives untouched.
func ConfigTemplate() string {
	return configTemplate
}

// LoadConfig reads burler.yaml at hubgeometry.ConfigFile(baseDir, "burler")
// directly — never through configengine.Load, per this file's seed-only
// rationale above. A file that does not exist (or exists but decodes to no
// entries at all, e.g. a comments-only file) is NOT an error: LoadConfig
// returns the zero Config, mirroring modelspec.LoadRegistry's optional-file
// posture. A fresh hub with no burler.yaml at all thus loads cleanly; the
// caller only hits a fail-loud error once it asks ResolveFan for a fan that
// isn't there, at which point the message names `lyx config reconcile` as the
// way to seed the file.
//
// When the file is present, it is decoded with a strict yaml.Decoder —
// KnownFields(true) — against the two-field Config shape: any OTHER
// top-level key is a loud error naming the file. The map keys and values
// under lenses/fans are never validated for a closed vocabulary here; that is
// ResolveFan's job at resolution time, once a specific fan is requested.
func LoadConfig(baseDir string) (Config, error) {
	path := hubgeometry.ConfigFile(baseDir, "burler")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// burler.yaml is optional — no file means "nothing seeded yet",
			// not a failure. Clustering fails later, at fan resolution, with
			// a message naming lyx config reconcile.
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("burler: read %s: %w", path, err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		// An empty or comments-only file yields io.EOF from Decode with no
		// fields set — that is a valid "nothing configured" file, not
		// malformed YAML.
		if errors.Is(err, io.EOF) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("burler: parse %s: %w", path, err)
	}

	return cfg, nil
}

// ResolveFan resolves a fan name against cfg into its ordered []Lens, the
// input a cluster round fans its forks out from. It is fail-loud,
// burler-prefixed, and never degrades to a shorter or reordered fan:
//
//   - Unknown fan name: the error lists every fan name cfg does define. When
//     cfg.Fans is empty entirely (the common case for a hub that never ran
//     `lyx config reconcile`), the message names that command as how to seed
//     the standard library instead of listing an empty set.
//   - A fan entry naming a lens not present in cfg.Lenses.
//   - An empty fan (a fan name that resolves to zero entries).
//   - A fan longer than maxClusterN entries.
//
// On success, the returned slice preserves the fan's entry order exactly,
// including repeats — a lens name listed twice in the fan yields two separate
// Lens entries with identical Name/Text, not a deduplicated one.
func ResolveFan(cfg Config, name string) ([]Lens, error) {
	entries, ok := cfg.Fans[name]
	if !ok {
		if len(cfg.Fans) == 0 {
			return nil, fmt.Errorf("burler: no fans are configured (requested fan %q) — seed the standard lens/fan library with `lyx config reconcile`", name)
		}
		return nil, fmt.Errorf("burler: unknown fan %q (known fans: %s)", name, strings.Join(fanNames(cfg.Fans), ", "))
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("burler: fan %q is empty", name)
	}
	if len(entries) > maxClusterN {
		return nil, fmt.Errorf("burler: fan %q has %d entries, exceeding the maximum of %d", name, len(entries), maxClusterN)
	}

	lenses := make([]Lens, 0, len(entries))
	for _, lensName := range entries {
		text, ok := cfg.Lenses[lensName]
		if !ok {
			return nil, fmt.Errorf("burler: fan %q names undefined lens %q", name, lensName)
		}
		lenses = append(lenses, Lens{Name: lensName, Text: text})
	}
	return lenses, nil
}

// fanNames returns the sorted list of fan names in fans, used to render a
// deterministic "known fans" list in ResolveFan's unknown-fan error.
func fanNames(fans map[string][]string) []string {
	names := make([]string, 0, len(fans))
	for name := range fans {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
