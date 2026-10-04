// Package loggerconfig owns _lyx/config/logger.yaml, the trace-retention bounds the exit sweep reads.
//
// It lives outside internal/logger because internal/configengine imports internal/logger, so the logger cannot load its own config without an import cycle.
// Load never resolves cwd itself: the caller hands it the anchor (see PATTERN-cwd-resolution).
package loggerconfig

import (
	"fmt"
	"math"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// moduleName is the logger's own configreg module name, passed to configengine.LoadOrTemplate.
const moduleName = "logger"

const (
	countKey = "trace_retention_count"
	daysKey  = "trace_retention_days"
)

const day = 24 * time.Hour

// maxRetentionDays is the largest trace_retention_days a time.Duration can hold.
// A larger value would overflow MaxAge into a negative duration,
// and the sweep would then delete every non-live trace.
const maxRetentionDays = int(math.MaxInt64 / int64(day))

// config mirrors logger.yaml's two keys as raw nodes, so Load can check each one's YAML tag.
type config struct {
	Count yaml.Node `yaml:"trace_retention_count"`
	Days  yaml.Node `yaml:"trace_retention_days"`
}

// ConfigPath returns the path of logger.yaml under anchorPath,
// so a caller can name the file in a diagnostic without knowing the module name.
func ConfigPath(anchorPath string) string {
	return configengine.ConfigFile(anchorPath, moduleName)
}

// Load resolves the trace-retention bounds from logger.yaml under anchorPath.
// An absent _lyx/ directory or an absent logger.yaml resolves the embedded ConfigTemplate() instead of erroring;
// a file that exists but is invalid, or carries a non-positive or non-integer value, errors naming the key.
// A trace_retention_days value above maxRetentionDays also errors naming the key.
func Load(anchorPath string) (logger.RetentionBounds, error) {
	resolved, err := configengine.LoadOrTemplate(anchorPath, moduleName, []byte(ConfigTemplate()))
	if err != nil {
		return logger.RetentionBounds{}, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return logger.RetentionBounds{}, fmt.Errorf("unmarshal logger config: %w", err)
	}

	count, err := positiveInt(countKey, &cfg.Count)
	if err != nil {
		return logger.RetentionBounds{}, err
	}
	days, err := positiveInt(daysKey, &cfg.Days)
	if err != nil {
		return logger.RetentionBounds{}, err
	}
	if days > maxRetentionDays {
		return logger.RetentionBounds{}, fmt.Errorf("logger config: %s must be at most %d, got %d", daysKey, maxRetentionDays, days)
	}

	return logger.RetentionBounds{Count: count, MaxAge: time.Duration(days) * day}, nil
}

// positiveInt decodes node as a positive integer.
// The tag check rejects a float: yaml.v3 decodes a !!float such as 1.5 or 14.0 into an int by truncation without an error.
func positiveInt(key string, node *yaml.Node) (int, error) {
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!int" {
		return 0, fmt.Errorf("logger config: %s must be a positive integer", key)
	}
	var v int
	if err := node.Decode(&v); err != nil {
		return 0, fmt.Errorf("logger config: %s must be a positive integer: %w", key, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("logger config: %s must be a positive integer, got %d", key, v)
	}
	return v, nil
}
