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

// skillsKey is the key holding the skills every code role loads.
const skillsKey = "skills"

// verifyTiersKey is the key holding the hub's verify-tier rules.
const verifyTiersKey = "verify_tiers"

// wayForward is the clause every content failure ends with.
const wayForward = `; way forward: fix it with "lyx config language" from the prime`

// ErrUnusable marks a content failure of language.yaml whose error already carries its own way forward.
var ErrUnusable = errors.New("language config unusable")

// config mirrors language.yaml's keys as raw nodes, so Load can check their YAML shape.
type config struct {
	Skills      yaml.Node `yaml:"skills"`
	VerifyTiers yaml.Node `yaml:"verify_tiers"`
}

// Language is the hub's resolved language.yaml.
// Both lists are in file order and never nil.
type Language struct {
	// Skills are the skills every code role loads after its own.
	Skills []string
	// VerifyTiers are the hub's verify-tier rules, one entry per rule.
	VerifyTiers []string
}

// ConfigSpec returns the language module's layered-load spec.
// Both keys are Guard-classified, so a local override holding either is refused.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     moduleName,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			skillsKey:      configengine.CategoryGuard,
			verifyTiersKey: configengine.CategoryGuard,
		},
	}
}

// Load resolves the hub's language.yaml through the layer stack under dirs, each list in file order and repeats kept.
// An empty dirs.Board resolves both lists empty without reading any file;
// an absent hub language.yaml resolves the template's empty lists.
// A content failure returns an error marked configengine.ErrInvalid that wraps ErrUnusable, names the failing key where there is one and ends with the way forward.
// Any other failure, such as an unreadable file, is returned unwrapped.
func Load(dirs configengine.Dirs) (Language, error) {
	if dirs.Board == "" {
		return Language{Skills: []string{}, VerifyTiers: []string{}}, nil
	}
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		if errors.Is(err, configengine.ErrInvalid) {
			return Language{}, unusable("", err.Error())
		}
		return Language{}, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Language{}, unusable("", err.Error())
	}
	skills, err := stringList(skillsKey, cfg.Skills)
	if err != nil {
		return Language{}, err
	}
	tiers, err := stringList(verifyTiersKey, cfg.VerifyTiers)
	if err != nil {
		return Language{}, err
	}
	// An entry renders as one bullet line, so a line break would split it.
	for _, tier := range tiers {
		if strings.ContainsAny(tier, "\r\n") {
			return Language{}, unusable(verifyTiersKey, fmt.Sprintf("entry %q holds a line break", tier))
		}
	}
	return Language{Skills: skills, VerifyTiers: tiers}, nil
}

// stringList returns node's entries in order when it is a list of non-empty strings without surrounding whitespace.
// A violation returns the marked content-failure error naming key.
func stringList(key string, node yaml.Node) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, unusable(key, "must be a list of strings")
	}
	list := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" {
			return nil, unusable(key, "must be a list of strings")
		}
		if item.Value == "" {
			return nil, unusable(key, "holds an empty entry")
		}
		if item.Value != strings.TrimSpace(item.Value) {
			return nil, unusable(key, fmt.Sprintf("entry %q carries leading or trailing whitespace", item.Value))
		}
		list = append(list, item.Value)
	}
	return list, nil
}

// VerifyTiersValue renders tiers as the value every verify_tiers marker takes:
// each entry verbatim and in order as a "- " bullet line, or "none" for an empty list.
func VerifyTiersValue(tiers []string) string {
	if len(tiers) == 0 {
		return "none"
	}
	lines := make([]string, len(tiers))
	for i, tier := range tiers {
		lines[i] = "- " + tier
	}
	return strings.Join(lines, "\n")
}

// unusable returns the marked content-failure error with detail and the way forward, naming key when it is not empty.
func unusable(key, detail string) error {
	if key == "" {
		return configengine.MarkInvalid(fmt.Errorf("language config: %s (%w)%s", detail, ErrUnusable, wayForward))
	}
	return configengine.MarkInvalid(fmt.Errorf("language config: %s: %s (%w)%s", key, detail, ErrUnusable, wayForward))
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
