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
// paths tmux/shell spawn, the window dimensions, the height-policy knobs
// render.Params carries, and the strand-name template.
type Config struct {
	Tmux string `yaml:"tmux"`
	// Shell is the path to the shell command lyx launches inside a fresh
	// pane (bash on POSIX, pwsh on Windows by default).
	Shell              string `yaml:"shell"`
	Width              int    `yaml:"width"`
	Height             int    `yaml:"height"`
	CollapsedStripRows int    `yaml:"collapsed_strip_rows"`
	MinFullRows        int    `yaml:"min_full_rows"`
	StrandName         string `yaml:"strand_name"`

	// DebugLog is the opt-in verbosity level for the server-spawning tmux
	// invocation: "0" (default) for no extra flags, "1" for -v, "2" for -vv.
	// It is deliberately a string, not an int, so yaml.Unmarshal never fails
	// on a non-numeric ${env:LYX_MUX_DEBUG} override — validating and
	// mapping it to actual tmux args is debugLogArgs' job (serverlog.go),
	// not this struct's. It takes effect only on the boot that spawns the
	// shared per-hub server (see hub-logs-dir/debug-log-key-semantics in the
	// plan's Shared Decisions); a hub whose mux.yaml predates this field
	// needs "lyx config reconcile" to adopt it.
	DebugLog string `yaml:"debug_log"`

	// Mouse is the tmux mouse-mode default: "off" (default) preserves
	// native terminal text selection/copy, "on" enables click-to-switch-pane.
	// It is deliberately a string, not a bool, so yaml.Unmarshal never fails
	// on a non-boolean ${env:LYX_MUX_MOUSE} override — validating and mapping
	// it to the tmux "on"/"off" option value is mouseOption's job
	// (mouse.go), not this struct's. It takes effect only on the boot that
	// spawns the shared per-hub server (see mouse-value-contract and
	// explicit-set-both-ways-at-boot in the plan's Shared Decisions); a hub
	// whose mux.yaml predates this field needs "lyx config reconcile" to
	// adopt it.
	Mouse string `yaml:"mouse"`

	// Header configures the always-on operator console pane's text. A hub
	// whose mux.yaml predates this field needs "lyx config reconcile" to
	// adopt it, matching the DebugLog/Mouse precedent above.
	Header HeaderConfig `yaml:"header"`
}

// HeaderConfig configures the header pane's rendered text: which template to
// render and how many rows it occupies. Template empty means "use the
// embedded default" (see internal/muxengine.HeaderTemplate); HeightRows
// defaults to 1.
type HeaderConfig struct {
	// Template is the raw header-text template, filled via
	// tokenvocab.Render. Empty means "use the embedded default template"
	// (headertemplate.go) rather than an empty rendered header.
	//
	// Live-change semantics (mirroring DebugLog/Mouse's boot-scoped notes
	// above): the pane renders its text ONCE, at header-pane launch, so a
	// template edit takes effect only when the header pane is next actually
	// (re)built — a server rebirth, a dead-header heal, or a down+up cycle.
	// An "up" that finds the header alive is an idempotent no-op and
	// deliberately leaves the running pane's text unchanged; "lyx mux
	// header" (the plain verb) previews the new rendering immediately.
	Template string `yaml:"template"`
	// HeightRows is the header pane's fixed row count; it defaults to 1.
	// Unlike Template it re-applies on the NEXT layout apply (any mutating
	// verb re-runs select-layout), no header rebuild needed.
	HeightRows int `yaml:"height_rows"`
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
