package languageconfig

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is the language module's own configreg name, passed to configengine.LoadOrTemplate.
const moduleName = "language"

// skillsKey is the module's one key.
const skillsKey = "skills"

// wayForward is the clause every content failure ends with.
const wayForward = `; way forward: fix it with "lyx config language" from the prime`

// ErrUnusable marks a content failure of language.yaml whose error already carries its own way forward.
var ErrUnusable = errors.New("language config unusable")

// config mirrors language.yaml's one key as a raw node, so Load can check its YAML shape.
type config struct {
	Skills yaml.Node `yaml:"skills"`
}

// ConfigSpec returns the language module's layered-load spec.
// The skills key is Guard-classified, so a local override holding it is refused.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     moduleName,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			skillsKey: configengine.CategoryGuard,
		},
	}
}

// Load resolves the hub's language skills, in file order and repeats kept, from language.yaml through the layer stack under dirs.
// An empty dirs.Board resolves the empty list without reading any file;
// an absent hub language.yaml resolves the template's empty list.
// A content failure returns an error marked configengine.ErrInvalid that wraps ErrUnusable, names skills and ends with the way forward.
// Any other failure, such as an unreadable file, is returned unwrapped.
func Load(dirs configengine.Dirs) ([]string, error) {
	if dirs.Board == "" {
		return []string{}, nil
	}
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		if errors.Is(err, configengine.ErrInvalid) {
			return nil, unusable(err.Error())
		}
		return nil, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return nil, unusable(err.Error())
	}
	if cfg.Skills.Kind != yaml.SequenceNode {
		return nil, unusable("must be a list of strings")
	}
	skills := make([]string, 0, len(cfg.Skills.Content))
	for _, item := range cfg.Skills.Content {
		if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" {
			return nil, unusable("must be a list of strings")
		}
		if item.Value == "" {
			return nil, unusable("holds an empty entry")
		}
		if item.Value != strings.TrimSpace(item.Value) {
			return nil, unusable(fmt.Sprintf("entry %q carries leading or trailing whitespace", item.Value))
		}
		skills = append(skills, item.Value)
	}
	return skills, nil
}

// unusable returns the marked content-failure error naming the skills key, with detail and the way forward.
func unusable(detail string) error {
	return configengine.MarkInvalid(fmt.Errorf("language config: %s: %s (%w)%s", skillsKey, detail, ErrUnusable, wayForward))
}

// AppendSkills returns a fresh slice holding own in order, then each language entry not already present, in order.
// A skill repeated in language, or already in own, appears once at its first position.
// own's backing array is never written.
func AppendSkills(own, language []string) []string {
	merged := make([]string, len(own), len(own)+len(language))
	copy(merged, own)
	seen := make(map[string]bool, len(own)+len(language))
	for _, skill := range own {
		seen[skill] = true
	}
	for _, skill := range language {
		if seen[skill] {
			continue
		}
		seen[skill] = true
		merged = append(merged, skill)
	}
	return merged
}
