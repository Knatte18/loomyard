// config.go — configuration for the mux module.
//
// Defines the Config type mirroring mux.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the mux config file; mux never reads config files
// or knows their on-disk layout itself.

package muxengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved mux.yaml configuration: the machine tool
// paths psmux/pwsh spawn, the window dimensions, the height-policy knobs
// render.Params carries, and the strand-name template.
type Config struct {
	Psmux              string `yaml:"psmux"`
	Pwsh               string `yaml:"pwsh"`
	Width              int    `yaml:"width"`
	Height             int    `yaml:"height"`
	CollapsedStripRows int    `yaml:"collapsed_strip_rows"`
	MinFullRows        int    `yaml:"min_full_rows"`
	StrandName         string `yaml:"strand_name"`

	// DebugLog is the opt-in verbosity level for the server-spawning psmux
	// invocation: "0" (default) for no extra flags, "1" for -v, "2" for -vv.
	// It is deliberately a string, not an int, so yaml.Unmarshal never fails
	// on a non-numeric ${env:LYX_MUX_DEBUG} override — validating and
	// mapping it to actual psmux args is debugLogArgs' job (serverlog.go),
	// not this struct's. It takes effect only on the boot that spawns the
	// shared per-hub server (see hub-logs-dir/debug-log-key-semantics in the
	// plan's Shared Decisions); a hub whose mux.yaml predates this field
	// needs "lyx config reconcile" to adopt it.
	DebugLog string `yaml:"debug_log"`

	// Mouse is the tmux/psmux mouse-mode default: "off" (default) preserves
	// native terminal text selection/copy, "on" enables click-to-switch-pane.
	// It is deliberately a string, not a bool, so yaml.Unmarshal never fails
	// on a non-boolean ${env:LYX_MUX_MOUSE} override — validating and mapping
	// it to the psmux "on"/"off" option value is mouseOption's job
	// (mouse.go), not this struct's. It takes effect only on the boot that
	// spawns the shared per-hub server (see mouse-value-contract and
	// explicit-set-both-ways-at-boot in the plan's Shared Decisions); a hub
	// whose mux.yaml predates this field needs "lyx config reconcile" to
	// adopt it.
	Mouse string `yaml:"mouse"`
}

// LoadConfig loads and unmarshals configuration for the mux module.
//
// Calls configengine.Load with mux's ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config
// struct. The module name is threaded through by the caller (never
// hardcoded to "mux" here), mirroring warpengine.LoadConfig.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the mux-specific hint,
		// matching warpengine's shape so every module surfaces the same
		// recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal mux config: %w", err)
	}

	return cfg, nil
}
