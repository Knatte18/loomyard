// config.go holds fabric's configuration: the Config type, its layered-load spec and LoadConfig.
// Config carries the keys of fabric.yaml.
// They are the warp branch prefix, the weft-sync pathspec, the warp exclude list and the hub's secret-file patterns.
// The push timing is among them too.
// LoadConfig resolves the fabric config file strictly through internal/configengine.Load and ConfigTemplate().
// The fabric module never reads config files or knows their layout itself.

package fabricengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/secretpath"
	"gopkg.in/yaml.v3"
)

// ExcludeListKey is fabric.yaml's key holding the warp exclude list.
const ExcludeListKey = "warp_exclude"

// SecretPathsKey is fabric.yaml's key holding the secret-file patterns a hub adds to the ones lym ships.
const SecretPathsKey = "secret_paths"

// PushIntervalKey is fabric.yaml's key holding the seconds between two pushes of one branch.
const PushIntervalKey = "push_interval_s"

// PushCeilingKey is fabric.yaml's key holding the most seconds a retry of a failed push or network call waits.
const PushCeilingKey = "push_ceiling_s"

// PushNoticeAfterKey is fabric.yaml's key holding the minutes a push stays owed before a notice reports it.
const PushNoticeAfterKey = "push_notice_after_min"

// Config holds fabric's settings:
// the warp branch prefix, the weft-sync pathspec, the warp exclude list and the hub's secret-file patterns.
// It also holds the push timing.
type Config struct {
	BranchPrefix       string `yaml:"branch_prefix"`
	Pathspec           string `yaml:"pathspec"`
	WarpExclude        string `yaml:"warp_exclude"`
	SecretPaths        string `yaml:"secret_paths"`
	PushIntervalS      int    `yaml:"push_interval_s"`
	PushCeilingS       int    `yaml:"push_ceiling_s"`
	PushNoticeAfterMin int    `yaml:"push_notice_after_min"`
}

// PushTiming is how often lym pushes and retries.
type PushTiming struct {
	// Interval is the time between two pushes of one branch while pushing works.
	Interval time.Duration
	// Ceiling is the longest wait before a retry, never shorter than Interval.
	Ceiling time.Duration
	// NoticeAfter is how long a push stays owed before a notice reports it.
	NoticeAfter time.Duration
}

// PushTiming returns the configured push timing, with no value below one second and Ceiling at least Interval.
// The second result holds one warning line for each value it raised, naming the key;
// the caller logs each line once.
func (c Config) PushTiming() (PushTiming, []string) {
	var warnings []string
	floored := func(key string, value int, unit time.Duration) time.Duration {
		d := time.Duration(value) * unit
		if d >= time.Second {
			return d
		}
		warnings = append(warnings, fmt.Sprintf("%s is %d, below one second; using 1s", configengine.KeyName("fabric", key), value))
		return time.Second
	}
	timing := PushTiming{
		Interval:    floored(PushIntervalKey, c.PushIntervalS, time.Second),
		Ceiling:     floored(PushCeilingKey, c.PushCeilingS, time.Second),
		NoticeAfter: floored(PushNoticeAfterKey, c.PushNoticeAfterMin, time.Minute),
	}
	if timing.Ceiling < timing.Interval {
		warnings = append(warnings, fmt.Sprintf("%s is %s, below %s; using %s",
			configengine.KeyName("fabric", PushCeilingKey), timing.Ceiling, configengine.KeyName("fabric", PushIntervalKey), timing.Interval))
		timing.Ceiling = timing.Interval
	}
	return timing, warnings
}

// SecretPathMatcher returns the matcher over lym's shipped secret-file patterns and the ones SecretPaths adds.
// SecretPaths is split on whitespace.
// A malformed added pattern is an error wrapping configengine.ErrInvalid that names the key.
func (c Config) SecretPathMatcher() (secretpath.Matcher, error) {
	m, err := secretpath.New(strings.Fields(c.SecretPaths))
	if err != nil {
		return secretpath.Matcher{}, fmt.Errorf("%w: fabric.yaml's %s: %w", configengine.ErrInvalid, SecretPathsKey, err)
	}
	return m, nil
}

// LoadSecretPathMatcher loads fabric configuration under dirs and returns its SecretPathMatcher.
// It serves a module that refuses a secret file in commits fabric does not make.
func LoadSecretPathMatcher(dirs configengine.Dirs) (secretpath.Matcher, error) {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return secretpath.Matcher{}, err
	}
	return cfg.SecretPathMatcher()
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// WarpExcludePaths returns the warp exclude list as paths relative to a warp worktree's anchor, split on whitespace.
// They are excluded through the warp repository's info/exclude and are never wired as junctions.
func (c Config) WarpExcludePaths() []string {
	return strings.Fields(c.WarpExclude)
}

// ConfigSpec returns fabric's layered-load spec.
// The pathspec and the warp exclude list reach every worktree of the hub, so every pair converges to one value of each.
// The secret-file key configures the guard on every commit, so every pair converges to one value of it too.
// The branch prefix is the convention of the person who makes pairs on a machine,
// so a machine's override may set it over the hub file;
// a pair keeps the branch name it was made with, whatever prefix another machine uses later.
// The push timing follows the machine's network, so a machine's override may set each of its keys.
// No key may be set in the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "fabric",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"branch_prefix":    configengine.CategoryMachine,
			"pathspec":         configengine.CategoryCrossPair,
			ExcludeListKey:     configengine.CategoryCrossPair,
			SecretPathsKey:     configengine.CategoryGuard,
			PushIntervalKey:    configengine.CategoryMachine,
			PushCeilingKey:     configengine.CategoryMachine,
			PushNoticeAfterKey: configengine.CategoryMachine,
		},
	}
}

// LoadConfig loads and unmarshals fabric configuration strictly through the hub-wide layer stack under dirs.
// It returns an error if the hub is not initialized, that is, if there is no _lym/ directory.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal fabric config: %w", err)
	}

	return cfg, nil
}
