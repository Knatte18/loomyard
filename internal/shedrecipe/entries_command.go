// entries_command.go implements the Command registry entry: it reads a row's command and timeout_s keys and builds a shedadapters.CommandProducer over Env.Command.

package shedrecipe

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// commandEngine is the registry name and error label of the Command entry.
	commandEngine = "Command"
	// commandDefaultTimeout is the time a Command row's command may run when the row sets no timeout_s.
	commandDefaultTimeout = 30 * time.Second

	commandRefWayForward     = "way forward: set command to a module.key setting reference in the row's config"
	commandTimeoutWayForward = "way forward: set timeout_s to a positive integer of seconds or remove it"
	commandKeyWayForward     = "way forward: remove the key; a Command row takes command and timeout_s, and a switch moves to the row's entry_gate"
)

// commandEntry is the Constructor for the "Command" registry row.
// The row's command is a required module.key setting reference and timeout_s is an optional positive number of seconds.
// Env.Command is handed to the producer with its two setting lookups taken from Env.Setting and Env.ExecutableSetting;
// a nil Env.ExecutableSetting builds, and the producer refuses when the row is reached.
func commandEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "command", "timeout_s"); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, commandKeyWayForward)
	}
	commandRef, err := commandSettingRef(row, cfg, "command", env)
	if err != nil {
		return nil, err
	}
	timeout, err := commandTimeout(row, cfg)
	if err != nil {
		return nil, err
	}

	deps := env.Command
	deps.Setting = env.Setting
	deps.ExecutableSetting = env.ExecutableSetting
	return shedadapters.NewCommandProducer(row.Name, shedadapters.CommandRow{
		CommandModule: commandRef.Module,
		CommandKey:    commandRef.Key,
		Timeout:       timeout,
	}, deps), nil
}

// commandSettingRef reads the required module.key reference cfg holds under key.
func commandSettingRef(row RowIdentity, cfg Config, key string, env Env) (settingReference, error) {
	if _, err := configString(cfg, key, false); err != nil {
		return settingReference{}, fmt.Errorf("shedrecipe: %s: row %q: %w; %s", commandEngine, row.Name, err, commandRefWayForward)
	}
	ref, named, err := settingRef(commandEngine, row, cfg, key, env)
	if err != nil {
		return settingReference{}, err
	}
	if !named {
		return settingReference{}, fmt.Errorf("shedrecipe: %s: row %q: config key %q is required; %s", commandEngine, row.Name, key, commandRefWayForward)
	}
	return ref, nil
}

// commandTimeout reads the row's optional timeout_s, defaulting to commandDefaultTimeout when it is absent.
func commandTimeout(row RowIdentity, cfg Config) (time.Duration, error) {
	seconds, err := configInt(cfg, "timeout_s", false)
	if err != nil {
		return 0, fmt.Errorf("shedrecipe: %s: row %q: %w; %s", commandEngine, row.Name, err, commandTimeoutWayForward)
	}
	if _, present := cfg["timeout_s"]; !present {
		return commandDefaultTimeout, nil
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("shedrecipe: %s: row %q: config key %q must be a positive integer, got %d; %s", commandEngine, row.Name, "timeout_s", seconds, commandTimeoutWayForward)
	}
	return time.Duration(seconds) * time.Second, nil
}
