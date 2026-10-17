// config.go — configuration for the landing module.
//
// Defines the Config type mirroring landing.yaml's keys and LoadConfig, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve landing's
// config file -- the strict entry point, where an absent config file is an error. Strict is
// required by the membership rule rather than chosen: neither producer in this package has a
// standalone entry point, both are reachable only inside a hub, and there an absent config means
// the hub is broken. It then validates the conflict key's model-spec grammar at load time via
// modelspec.Parse, so a typo fails loudly at load rather than hours into a run when the conflict
// session first spawns.

package landingshed

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved landing.yaml configuration.
type Config struct {
	// RequirePRToBase is the list of base branch names Publish requires a pull request against.
	RequirePRToBase []string `yaml:"require_pr_to_base"`
	// Squash selects whether Finalize's parent-side merge squashes.
	Squash bool `yaml:"squash"`
	// Conflict is the model-spec string selecting the model both producers' conflict-resolution
	// session runs under.
	Conflict string `yaml:"conflict"`
	// ConflictTimeoutMin is the conflict-resolution session's wall-clock budget in minutes.
	ConflictTimeoutMin int `yaml:"conflict_timeout_min"`
	// Describe is the model-spec string selecting the model the Describe row's session runs under.
	Describe string `yaml:"describe"`
	// DescribeTimeoutMin is the Describe session's wall-clock budget in minutes.
	DescribeTimeoutMin int `yaml:"describe_timeout_min"`
	// TrailerText is the landing commit's fixed trailer lines, copied verbatim; empty adds none.
	TrailerText string `yaml:"trailer_text"`
	// EngineTrailers maps an engine name to the trailer text the landing commit carries when that engine ran a session in the task.
	// An engine left out, or mapped to empty text, adds nothing.
	EngineTrailers map[string]string `yaml:"engine_trailers"`
	// PublishVerify is the shell command Publish runs in the task worktree after the plan's verify passes.
	// Empty runs nothing; Finalize never runs it.
	PublishVerify string `yaml:"publish_verify"`
	// Deploy is the shell command batten's Land row runs in the prime after a landing into the prime's branch.
	// Empty runs nothing.
	Deploy string `yaml:"deploy"`
	// ParentLockIdleReleaseMin is the minutes a waiting run, or an undecided PR-Gate hand-off, keeps its parent lock before the lock is released.
	// A value below 1 reads as the default.
	ParentLockIdleReleaseMin int `yaml:"parent_lock_idle_release_min"`
}

// defaultIdleReleaseMin is the idle bound in minutes that a missing or non-positive parent_lock_idle_release_min reads as.
const defaultIdleReleaseMin = 10

// IdleRelease returns the parent lock's idle bound.
// A parent_lock_idle_release_min below 1, an absent key included, returns the default and logs one Warn naming the key.
func (c Config) IdleRelease() time.Duration {
	if c.ParentLockIdleReleaseMin < 1 {
		logger.Warn("landingshed: parent_lock_idle_release_min is below 1 or absent, using the default", "key", "parent_lock_idle_release_min", "value", c.ParentLockIdleReleaseMin, "defaultMin", defaultIdleReleaseMin)
		return defaultIdleReleaseMin * time.Minute
	}
	return time.Duration(c.ParentLockIdleReleaseMin) * time.Minute
}

// TrailerBlock returns the landing commit's trailer block: the trimmed TrailerText, then the trimmed EngineTrailers text of each name in enginesRun, in sorted name order, one per line.
// An empty or whitespace-only text adds nothing, and an empty block means the commit carries no trailer block.
func (c Config) TrailerBlock(enginesRun []string) string {
	names := slices.Sorted(slices.Values(enginesRun))
	var parts []string
	if text := strings.TrimSpace(c.TrailerText); text != "" {
		parts = append(parts, text)
	}
	for _, name := range slices.Compact(names) {
		if text := strings.TrimSpace(c.EngineTrailers[name]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// engineTrailersKey is the landing.yaml key whose entries map engine names to trailer text.
const engineTrailersKey = "engine_trailers"

// ConfigOpenMaps returns the landing.yaml keys whose entries belong to the operator, which configengine carries whole through reconcile and the layer stack.
func ConfigOpenMaps() []string {
	return []string{engineTrailersKey}
}

// ConfigSpec returns landing's layered-load spec.
// Keys that gate or shape what lands on the parent branch, and the shell commands Publish and Land run, are barred from the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "landing",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"require_pr_to_base":   configengine.CategoryCrossPair,
			"squash":               configengine.CategoryCrossPair,
			"conflict":             configengine.CategoryNone,
			"conflict_timeout_min": configengine.CategoryNone,
			"describe":             configengine.CategoryNone,
			"describe_timeout_min": configengine.CategoryNone,
			"trailer_text":         configengine.CategoryCrossPair,
			engineTrailersKey:      configengine.CategoryCrossPair,
			"publish_verify":       configengine.CategoryExecutable,
			"deploy":               configengine.CategoryExecutable,
			// The idle bound decides when one pair's parent lock frees for the next pair waiting on it.
			"parent_lock_idle_release_min": configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals configuration for the landing module strictly through the hub-wide layer stack under dirs.
// It validates the conflict and describe keys' model-spec grammar at load time, and refuses an engine_trailers entry whose key names no engine.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal landing config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Conflict); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "conflict", err)
	}

	if _, err := modelspec.Parse(cfg.Describe); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "describe", err)
	}
	engines := modelspec.EngineNames()
	for name := range cfg.EngineTrailers {
		if !slices.Contains(engines, name) {
			return Config{}, fmt.Errorf("landing config key %q: %q is no engine; use one of %v, or remove the entry", engineTrailersKey+"."+name, name, engines)
		}
	}

	return cfg, nil
}

// MigrateConfig rewrites a landing.yaml document for lyx config reconcile and returns the rewritten bytes with one line per rewrite.
// It moves a retired co_authored_by value into engine_trailers as the claude entry `Co-Authored-By: <value>`;
// a document already holding engine_trailers keeps it and drops co_authored_by,
// and an empty or non-scalar co_authored_by is dropped without an entry, as empty means off.
// A document without co_authored_by is returned unchanged, so a migrated file is unchanged on a second pass.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	migrated, line, err := migrateCoAuthoredBy(existing)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate landing config: %w", err))
	}
	if line == "" {
		return existing, nil, nil
	}
	return migrated, []string{line}, nil
}

// migrateCoAuthoredBy rewrites a top-level co_authored_by key in place into an engine_trailers mapping holding its claude line, or drops it beside an existing engine_trailers.
// It returns the rewritten document and one line describing the rewrite, or the document unchanged and an empty line when the key is absent.
func migrateCoAuthoredBy(existing []byte) ([]byte, string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, "", fmt.Errorf("unmarshal yaml: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, "", nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("yaml root is not a mapping")
	}

	legacyIndex, currentIndex := -1, -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "co_authored_by":
			legacyIndex = i
		case engineTrailersKey:
			currentIndex = i
		}
	}
	if legacyIndex < 0 {
		return existing, "", nil
	}

	value := root.Content[legacyIndex+1].Value
	var line string
	switch {
	case strings.TrimSpace(value) == "":
		line = "co_authored_by: empty, dropped"
		root.Content = append(root.Content[:legacyIndex], root.Content[legacyIndex+2:]...)
	case currentIndex >= 0:
		line = fmt.Sprintf("co_authored_by: %s dropped, %s already set", value, engineTrailersKey)
		root.Content = append(root.Content[:legacyIndex], root.Content[legacyIndex+2:]...)
	default:
		root.Content[legacyIndex] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: engineTrailersKey}
		root.Content[legacyIndex+1] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "claude"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "Co-Authored-By: " + value},
		}}
		line = fmt.Sprintf("co_authored_by: moved to %s.claude", engineTrailersKey)
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), line, nil
}
