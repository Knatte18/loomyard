// entries_command_test.go covers the Command entry: its build-time refusals, its defaults and the timeout it hands the producer's runner.

package shedrecipe

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedadapters"
)

const commandTestRow = "Open-Window"

// commandTestConfig returns the valid Config of a Command row.
func commandTestConfig(*testing.T, Env) Config {
	return Config{"switch": "batten.open_window", "command": "batten.open_window_command"}
}

// commandTestSettings returns the setting values the helper's lookups answer: the switch on and a command with no {task_dir} placeholder.
func commandTestSettings() map[string]any {
	return map[string]any{
		"batten.open_window":         true,
		"batten.open_window_command": "echo hi",
	}
}

// newRecordedCommandTestEnv returns newTestEnv(t) with the Command seams filled for a Command row,
// and a pointer to the timeout the filled Run seam last received.
// ExecutableSetting is left nil, as the entry takes the lookup from the Env field at build time only.
func newRecordedCommandTestEnv(t *testing.T) (Env, *time.Duration) {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	var gotTimeout time.Duration
	env.Setting = fakeSetting(commandTestSettings())
	env.Command.Dir = dir
	env.Command.Lyx = "lyx"
	env.Command.Slug = "task"
	env.Command.Rerun = "lyx batten run task"
	env.Command.LogPath = func(row string) string { return filepath.Join(dir, row+".log") }
	env.Command.Run = func(_ context.Context, _, _, _ string, timeout time.Duration) (shedadapters.CommandResult, error) {
		gotTimeout = timeout
		return shedadapters.CommandResult{}, nil
	}
	return env, &gotTimeout
}

// newCommandTestEnv is newRecordedCommandTestEnv for a caller that reads no timeout.
func newCommandTestEnv(t *testing.T) Env {
	t.Helper()
	env, _ := newRecordedCommandTestEnv(t)
	return env
}

func TestCommandEntry(t *testing.T) {
	t.Parallel()

	const refWayForward = "way forward: set switch and command to module.key setting references in the row's config"
	const timeoutWayForward = "way forward: set timeout_s to a positive integer of seconds or remove it"
	refusals := []struct {
		name string
		edit func(Config)
		want []string
	}{
		{"MissingSwitch", func(c Config) { delete(c, "switch") }, []string{commandTestRow, "switch", refWayForward}},
		{"NonStringSwitch", func(c Config) { c["switch"] = 3 }, []string{commandTestRow, "switch", "must be a string", refWayForward}},
		{"MissingCommand", func(c Config) { delete(c, "command") }, []string{commandTestRow, "command", refWayForward}},
		{"NonStringCommand", func(c Config) { c["command"] = 3 }, []string{commandTestRow, "command", "must be a string", refWayForward}},
		{"ReferenceWithNoDot", func(c Config) { c["switch"] = "open_window" }, []string{commandTestRow, "switch", "way forward:"}},
		{"ZeroTimeout", func(c Config) { c["timeout_s"] = 0 }, []string{commandTestRow, "timeout_s", timeoutWayForward}},
		{"NonIntTimeout", func(c Config) { c["timeout_s"] = "5" }, []string{commandTestRow, "timeout_s", timeoutWayForward}},
		{"UnknownKey", func(c Config) { c["bogus_key"] = "x" }, []string{commandTestRow, "bogus_key", "way forward: remove the key; a Command row takes switch, command and timeout_s"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newCommandTestEnv(t)
			cfg := commandTestConfig(t, env)
			tt.edit(cfg)
			_, err := commandEntry(RowIdentity{Name: commandTestRow}, cfg, env)
			for _, want := range tt.want {
				assertErrContains(t, err, want)
			}
		})
	}

	t.Run("NilExecutableSettingStillBuilds", func(t *testing.T) {
		t.Parallel()
		env := newCommandTestEnv(t)
		producer, err := commandEntry(RowIdentity{Name: commandTestRow}, commandTestConfig(t, env), env)
		if err != nil || producer == nil {
			t.Fatalf("commandEntry() = (%v, %v); want a producer and nil", producer, err)
		}
	})

	timeouts := []struct {
		name string
		cfg  func(Config)
		want time.Duration
	}{
		{"DefaultsTo30Seconds", func(Config) {}, 30 * time.Second},
		{"HonoursTimeoutS", func(c Config) { c["timeout_s"] = 5 }, 5 * time.Second},
	}
	for _, tt := range timeouts {
		t.Run("Call_"+tt.name, func(t *testing.T) {
			t.Parallel()
			env, gotTimeout := newRecordedCommandTestEnv(t)
			env.ExecutableSetting = fakeSetting(commandTestSettings())
			cfg := commandTestConfig(t, env)
			tt.cfg(cfg)
			producer, err := commandEntry(RowIdentity{Name: commandTestRow}, cfg, env)
			if err != nil {
				t.Fatalf("commandEntry() error = %v; want nil", err)
			}
			if _, _, err := producer.Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if *gotTimeout != tt.want {
				t.Errorf("Run timeout = %v; want %v", *gotTimeout, tt.want)
			}
		})
	}
}
