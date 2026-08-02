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

// Config is the decode shape of burler.yaml.
type Config struct {
	// Lenses maps a lens name to its review prose.
	Lenses map[string]string `yaml:"lenses"`
	// Fans maps a fan name to an ordered list of lens names.
	Fans map[string][]string `yaml:"fans"`
}

// Lens is one resolved fan entry: the lens name and the review prose it
// contributes to a fork's prompt.
type Lens struct {
	Name string
	Text string
}

// ConfigTemplate returns the seed content for burler.yaml: the standard lens
// library and standard/full fans, embedded from template.yaml.
func ConfigTemplate() string {
	return configTemplate
}

// LoadConfig reads burler.yaml. A missing file returns zero Config.
// When present, it is decoded with strict yaml.Decoder.
func LoadConfig(baseDir string) (Config, error) {
	path := hubgeometry.ConfigFile(baseDir, "burler")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Optional file — no file means nothing seeded yet.
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("burler: read %s: %w", path, err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		// Empty or comments-only file yields io.EOF — valid "nothing configured".
		if errors.Is(err, io.EOF) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("burler: parse %s: %w", path, err)
	}

	return cfg, nil
}

// ResolveFan resolves a fan name against cfg into its ordered []Lens. It is
// fail-loud and never degrades. The returned slice preserves fan order exactly.
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

// fanNames returns the sorted list of fan names in fans.
func fanNames(fans map[string][]string) []string {
	names := make([]string, 0, len(fans))
	for name := range fans {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
