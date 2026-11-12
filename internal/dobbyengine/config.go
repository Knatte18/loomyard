// config.go declares the beam config module, beam.yaml: its template, its layered-load spec, LoadConfig,
// and the migration that carries the retired test_map_every off switch into test_map_upkeep.

package dobbyengine

import (
	"bytes"
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns beam.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// The values of beam.yaml's land_notice.
const (
	// LandNoticeAll sends a landing notice for every run the dobby drives.
	LandNoticeAll = "all"
	// LandNoticePerTask sends one only for a run whose board entry carried notify_on_land.
	LandNoticePerTask = "per-task"
	// LandNoticeOff sends none.
	LandNoticeOff = "off"
)

// The keys the upkeep migration reads and writes.
const (
	// testMapEveryKey is the retired landing count between test-map refreshes, whose 0 switched them off.
	testMapEveryKey = "test_map_every"
	// testMapBatchKey is the retired cap on the packages one refresh measured.
	testMapBatchKey = "test_map_batch"
	// testMapUpkeepKey switches the prime dobby's upkeep passes on.
	testMapUpkeepKey = "test_map_upkeep"
)

// Config is the resolved beam.yaml.
type Config struct {
	// MaxRuns is the most runs dobby drives at once; 0 starts none.
	MaxRuns int `yaml:"max_runs"`
	// BoardTidyEvery is the landings between board tidies; 0 never tidies.
	BoardTidyEvery int `yaml:"board_tidy_every"`
	// BoardTidyTimeoutMin is the minutes the board-tidy agent may run.
	BoardTidyTimeoutMin int `yaml:"board_tidy_timeout_min"`
	// TestMapUpkeep reports whether the prime's dobby starts coverage-map upkeep passes.
	TestMapUpkeep bool `yaml:"test_map_upkeep"`
	// TaskHeadroom is the fewest free tasks under the cgroup task limit for dobby to start a run; 0 never holds a start.
	TaskHeadroom int `yaml:"task_headroom"`
	// PullRequest is the pull-request mode: "on", "off", "required" or "merge" (squash-merged at once, no review).
	PullRequest string `yaml:"pull_request"`
	// LandNotice decides which runs the dobby sends a landing notice for: LandNoticeAll, LandNoticePerTask or LandNoticeOff.
	LandNotice string `yaml:"land_notice"`
	// AdoptFixTries is the most fix tasks a failed adopt check files for the worktree's subject between two hand adopts; 0 files none.
	AdoptFixTries int `yaml:"adopt_fix_tries"`
}

// ConfigSpec returns beam's layered-load spec: a per-worktree module whose keys are open to every layer but pull_request.
// pull_request decides what lands on the parent, so it is cross-pair and the local layer refuses it.
// test_map_every and test_map_batch are retired, and each layer passes through MigrateConfig before the merge,
// so a layer no reconcile has rewritten yet that holds test_map_every: 0 loads as test_map_upkeep: false.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "beam",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"max_runs":               configengine.CategoryNone,
			"board_tidy_every":       configengine.CategoryNone,
			"board_tidy_timeout_min": configengine.CategoryNone,
			testMapUpkeepKey:         configengine.CategoryNone,
			"task_headroom":          configengine.CategoryNone,
			"pull_request":           configengine.CategoryCrossPair,
			"land_notice":            configengine.CategoryNone,
			"adopt_fix_tries":        configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			testMapEveryKey: configengine.CategoryNone,
			testMapBatchKey: configengine.CategoryNone,
		},
		MigrateLayer: MigrateConfig,
	}
}

// LoadConfig resolves beam.yaml strictly from the layer stack under dirs, since dobby only ever runs inside a hub.
// A negative value is refused, naming its key.
// A pull_request other than on, off, required or merge is refused too.
// So is a land_notice other than all, per-task or off.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal beam config: %w", err))
	}
	for _, key := range []struct {
		name  string
		value int
	}{
		{"max_runs", cfg.MaxRuns},
		{"board_tidy_every", cfg.BoardTidyEvery},
		{"board_tidy_timeout_min", cfg.BoardTidyTimeoutMin},
		{"task_headroom", cfg.TaskHeadroom},
		{"adopt_fix_tries", cfg.AdoptFixTries},
	} {
		if key.value < 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %d: must not be negative; way forward: fix it with `lym config beam --set %s=<value>`", key.name, key.value, key.name))
		}
	}
	switch cfg.PullRequest {
	case "on", "off", "required", "merge":
	default:
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %q: must be on, off, required or merge; way forward: fix it with lym config beam --set pull_request=<on|off|required|merge>", "pull_request", cfg.PullRequest))
	}
	switch cfg.LandNotice {
	case LandNoticeAll, LandNoticePerTask, LandNoticeOff:
	default:
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %q: must be all, per-task or off; way forward: fix it with lym config beam --set land_notice=<all|per-task|off>", "land_notice", cfg.LandNotice))
	}
	return cfg, nil
}

// MigrateConfig carries a beam.yaml document's retired test_map_every into test_map_upkeep.
// It returns one line per rewrite.
// A test_map_every of 0 switched the test-map refreshes off, so it becomes test_map_upkeep: false in its place;
// a document that already sets test_map_upkeep keeps that value, and its test_map_every is removed.
// Any other test_map_every is removed.
// Every other key and comment is carried through, and a document holding no test_map_every is returned byte for byte.
// It serves both the reconcile walk, which writes and logs the rewrite, and every load, which applies it in memory.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate beam config: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate beam config: yaml root is not a mapping"))
	}
	at := mappingKeyIndex(root, testMapEveryKey)
	if at < 0 {
		return existing, nil, nil
	}
	var line string
	if value := root.Content[at+1]; value.Kind == yaml.ScalarNode && value.Value == "0" && mappingKeyIndex(root, testMapUpkeepKey) < 0 {
		// The retired key's line comment described the landing count, so the new value carries none.
		root.Content[at].Value, root.Content[at].LineComment = testMapUpkeepKey, ""
		root.Content[at+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
		line = testMapEveryKey + ": 0 became " + testMapUpkeepKey + ": false"
	} else {
		root.Content = append(root.Content[:at], root.Content[at+2:]...)
		line = testMapEveryKey + ": retired, removed"
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate beam config: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate beam config: encode yaml: %w", err))
	}
	return out.Bytes(), []string{line}, nil
}

// mappingKeyIndex returns the index in mapping's content of key's key node, or -1 when mapping holds no key.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}
