// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// Config is the resolved gate.yaml.
type Config struct {
	// Slots is how many gate runs the hub executes at once.
	Slots int `yaml:"slots"`
	// GoParallel is the `-p` cap every slotted run gets.
	GoParallel int `yaml:"go_parallel"`
	// CLIWaitSec is how many seconds `lyx gate test` waits for a slot.
	CLIWaitSec int `yaml:"cli_wait_sec"`
}

// LoadConfig loads gate.yaml from baseDir strictly: an absent file is an error that names `lyx fabric reconcile`, and a value below 1 fails naming its key.
func LoadConfig(baseDir string) (Config, error) {
	resolved, err := configengine.Load(baseDir, "gate", []byte(ConfigTemplate()))
	if err != nil {
		if errors.Is(err, configengine.ErrNotInitialized) || strings.Contains(err.Error(), "not found") {
			return Config{}, fmt.Errorf("gate config absent; run \"lyx fabric reconcile\": %w", err)
		}
		return Config{}, err
	}
	return decodeConfig(resolved)
}

// TemplateConfig decodes the embedded template alone, the config of a run outside every hub.
func TemplateConfig() (Config, error) {
	return decodeConfig([]byte(ConfigTemplate()))
}

func decodeConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal gate config: %w", err)
	}
	for _, key := range []struct {
		name  string
		value int
	}{{"slots", cfg.Slots}, {"go_parallel", cfg.GoParallel}, {"cli_wait_sec", cfg.CLIWaitSec}} {
		if key.value < 1 {
			return Config{}, fmt.Errorf("gate config key %q: %d; want at least 1", key.name, key.value)
		}
	}
	return cfg, nil
}

// Limits converts the config to the pool limits, cli_wait_sec as a duration.
func (c Config) Limits() Limits {
	return Limits{Slots: c.Slots, GoParallel: c.GoParallel, CLIWait: time.Duration(c.CLIWaitSec) * time.Second}
}
