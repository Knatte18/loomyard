// config_test.go pins batten.yaml's load: the tackler override keys it carries over shed.yaml, and the refusal of a driver_kind key.

package battenshed

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
)

// writeBattenLayer writes document to path, creating its directory.
func writeBattenLayer(t *testing.T, path, document string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestLoadConfig verifies the template's tackler keys inherit shed.yaml, set values are read, a negative spawn budget is refused naming batten, and a driver_kind key is refused in the local layer and in the base layer, since batten's own driver is always go.
func TestLoadConfig(t *testing.T) {
	t.Parallel()
	spawnsOff := 0

	t.Run("the template's tackler keys inherit", func(t *testing.T) {
		t.Parallel()
		cfg, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if got := cfg.ShedOverride(); !reflect.DeepEqual(got, sheddriver.Override{}) {
			t.Errorf("ShedOverride() = %+v; want every key inheriting", got)
		}
	})

	t.Run("set values are read, a spawn budget of 0 as a set 0", func(t *testing.T) {
		t.Parallel()
		board := t.TempDir()
		writeBattenLayer(t, configengine.ConfigFile(board, "batten"), strings.NewReplacer(`tackler: ""`, "tackler: sonnet[low]", "tackler_timeout_min: 0", "tackler_timeout_min: 5", "tackler_max_spawns: ~", "tackler_max_spawns: 0").Replace(ConfigTemplate()))
		cfg, err := LoadConfig(configengine.Dirs{Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		want := sheddriver.Override{Tackler: "sonnet[low]", TacklerTimeoutMin: 5, TacklerMaxSpawns: &spawnsOff}
		if got := cfg.ShedOverride(); !reflect.DeepEqual(got, want) {
			t.Errorf("ShedOverride() = %+v; want %+v", got, want)
		}
	})

	t.Run("a negative tackler_max_spawns is refused naming batten", func(t *testing.T) {
		t.Parallel()
		board := t.TempDir()
		writeBattenLayer(t, configengine.ConfigFile(board, "batten"), strings.Replace(ConfigTemplate(), "tackler_max_spawns: ~", "tackler_max_spawns: -1", 1))
		_, err := LoadConfig(configengine.Dirs{Board: board})
		if err == nil || !strings.Contains(err.Error(), "batten config key") || !strings.Contains(err.Error(), "tackler_max_spawns") {
			t.Errorf("LoadConfig() = _, %v; want a refusal naming %q and the key", err, "batten config key")
		}
	})

	t.Run("a local driver_kind is refused naming the key", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		writeBattenLayer(t, configengine.ConfigFile(board, "batten"), ConfigTemplate())
		if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(anchor), err)
		}
		writeBattenLayer(t, configengine.LocalFile(anchor, "batten"), "driver_kind: go\n")
		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err == nil || !strings.Contains(err.Error(), "driver_kind") {
			t.Errorf("LoadConfig() = _, %v; want a refusal naming driver_kind", err)
		}
	})

	t.Run("a base-layer driver_kind is refused naming the key and the way forward", func(t *testing.T) {
		t.Parallel()
		board := t.TempDir()
		writeBattenLayer(t, configengine.ConfigFile(board, "batten"), ConfigTemplate()+"driver_kind: go\n")
		_, err := LoadConfig(configengine.Dirs{Board: board})
		if err == nil {
			t.Fatal("LoadConfig() = _, nil; want a refusal")
		}
		for _, want := range []string{`batten config key "driver_kind"`, "always go", "remove the key from batten.yaml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}
