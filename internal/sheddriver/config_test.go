// config_test.go pins the shed config module: its load, the override checks and the two pure resolvers.

package sheddriver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// writeLayer writes document to path, creating its directory.
func writeLayer(t *testing.T, path, document string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// intPointer returns a pointer to value.
func intPointer(value int) *int {
	return &value
}

// TestLoadConfig verifies LoadConfig reads the template defaults when no layer holds shed.yaml, reads a base value and a local override, and refuses an invalid value naming the module.
func TestLoadConfig(t *testing.T) {
	t.Parallel()
	templateDefaults := Config{DriverKind: "llm", Tackler: "haiku", TacklerTimeoutMin: 10, TacklerMaxSpawns: 4}

	tests := []struct {
		name    string
		base    string
		local   string
		want    Config
		wantErr string
	}{
		{name: "no layer reads the template defaults", want: templateDefaults},
		{
			name: "a base value is read",
			base: "driver_kind: go\ntackler: sonnet[low]\ntackler_timeout_min: 7\ntackler_max_spawns: 0\n",
			want: Config{DriverKind: "go", Tackler: "sonnet[low]", TacklerTimeoutMin: 7, TacklerMaxSpawns: 0},
		},
		{
			name:  "a local override wins over the template",
			local: "tackler_max_spawns: 1\n",
			want:  Config{DriverKind: "llm", Tackler: "haiku", TacklerTimeoutMin: 10, TacklerMaxSpawns: 1},
		},
		{name: "an invalid value is refused", base: "driver_kind: fast\n", wantErr: "shed config key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Anchor: t.TempDir()}
			if tt.base != "" {
				dirs.Board = t.TempDir()
				writeLayer(t, configengine.ConfigFile(dirs.Board, "shed"), tt.base)
			}
			if tt.local != "" {
				writeLayer(t, configengine.LocalFile(dirs.Anchor, "shed"), tt.local)
			}
			got, err := LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadConfig() = _, %v; want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil error", err)
			}
			if got != tt.want {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestCheckOverride verifies CheckOverride accepts the inherit values and the valid ones, names the module it was told in each refusal, and that CheckLocalOverride marks a refusal ErrInvalid and lets a null through.
func TestCheckOverride(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		module    string
		override  Override
		wantErrIn []string
	}{
		{name: "all inherit", module: "loom"},
		{name: "empty driver kind", module: "loom", override: Override{DriverKind: ""}},
		{name: "llm driver kind", module: "loom", override: Override{DriverKind: "llm"}},
		{name: "go driver kind", module: "darn", override: Override{DriverKind: "go"}},
		{name: "nil spawn budget", module: "darn", override: Override{TacklerMaxSpawns: nil}},
		{name: "zero spawn budget", module: "darn", override: Override{TacklerMaxSpawns: intPointer(0)}},
		{name: "unknown driver kind", module: "loom", override: Override{DriverKind: "fast"}, wantErrIn: []string{`loom config key "driver_kind"`, "llm or go", "loom.yaml", "inherit shed.yaml"}},
		{name: "unparseable tackler", module: "batten", override: Override{Tackler: "not a spec["}, wantErrIn: []string{`batten config key "tackler"`, "batten.yaml"}},
		{name: "negative timeout", module: "darn", override: Override{TacklerTimeoutMin: -1}, wantErrIn: []string{`darn config key "tackler_timeout_min"`, "must not be negative", "0 for the 10 minute default"}},
		{name: "negative spawn budget", module: "shed", override: Override{TacklerMaxSpawns: intPointer(-1)}, wantErrIn: []string{`shed config key "tackler_max_spawns"`, "0 to turn the tackler off"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckOverride(tt.module, tt.override)
			if tt.wantErrIn == nil {
				if err != nil {
					t.Fatalf("CheckOverride(%q, %+v) = %v; want nil", tt.module, tt.override, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckOverride(%q, %+v) = nil; want an error", tt.module, tt.override)
			}
			for _, want := range tt.wantErrIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckOverride(%q, %+v) error = %q; want it to contain %q", tt.module, tt.override, err.Error(), want)
				}
			}
		})
	}

	err := CheckLocalOverride("shed", []byte("tackler_max_spawns: -1\n"))
	if !errors.Is(err, configengine.ErrInvalid) {
		t.Errorf("CheckLocalOverride(tackler_max_spawns: -1) = %v; want an error marked ErrInvalid", err)
	}
	if err := CheckLocalOverride("shed", []byte("tackler_max_spawns: null\n")); err != nil {
		t.Errorf("CheckLocalOverride(tackler_max_spawns: null) = %v; want nil", err)
	}
}

// TestResolveTackler verifies ResolveDriverKind and ResolveTackler take each key from the override, else shed.yaml, else the default, and that an unresolvable spec is refused naming the tackler key.
func TestResolveTackler(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	settingsOf := func(text string) TacklerSettings {
		t.Helper()
		spec, err := modelspec.Parse(text)
		if err != nil {
			t.Fatalf("modelspec.Parse(%q) = _, %v; want nil error", text, err)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			t.Fatalf("reg.Resolve(%q) = _, %v; want nil error", text, err)
		}
		return TacklerSettings{Model: resolved.Model, Effort: resolved.Params["effort"], Version: resolved.Params["version"]}
	}

	for _, tt := range []struct {
		name     string
		cfg      Config
		override Override
		want     string
	}{
		{name: "override set wins", cfg: Config{DriverKind: "llm"}, override: Override{DriverKind: "go"}, want: "go"},
		{name: "empty override takes shed.yaml's", cfg: Config{DriverKind: "go"}, want: "go"},
		{name: "both empty takes llm", want: "llm"},
	} {
		if got := ResolveDriverKind(tt.cfg, tt.override); got != tt.want {
			t.Errorf("%s: ResolveDriverKind(%+v, %+v) = %q; want %q", tt.name, tt.cfg, tt.override, got, tt.want)
		}
	}

	tests := []struct {
		name        string
		cfg         Config
		override    Override
		wantSpec    string
		wantTimeout time.Duration
		wantSpawns  int
		wantErrIn   string
	}{
		{
			name:     "empty override inherits every shed.yaml value",
			cfg:      Config{Tackler: "sonnet[low]", TacklerTimeoutMin: 7, TacklerMaxSpawns: 4},
			wantSpec: "sonnet[low]", wantTimeout: 7 * time.Minute, wantSpawns: 4,
		},
		{
			name:     "a set override wins on each key",
			cfg:      Config{Tackler: "sonnet[low]", TacklerTimeoutMin: 7, TacklerMaxSpawns: 4},
			override: Override{Tackler: "opus[high]", TacklerTimeoutMin: 3, TacklerMaxSpawns: intPointer(2)},
			wantSpec: "opus[high]", wantTimeout: 3 * time.Minute, wantSpawns: 2,
		},
		{
			name:     "an override of 0 spawns beats shed.yaml's 4",
			cfg:      Config{TacklerMaxSpawns: 4},
			override: Override{TacklerMaxSpawns: intPointer(0)},
			wantSpec: "haiku", wantTimeout: 10 * time.Minute, wantSpawns: 0,
		},
		{
			name:     "an override timeout of 0 takes shed.yaml's",
			cfg:      Config{TacklerTimeoutMin: 7},
			wantSpec: "haiku", wantTimeout: 7 * time.Minute,
		},
		{
			name:     "a shed.yaml timeout of 0 takes 10 minutes",
			wantSpec: "haiku", wantTimeout: 10 * time.Minute,
		},
		{name: "an unresolvable spec", cfg: Config{Tackler: "not a spec["}, wantErrIn: "tackler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTackler(tt.cfg, tt.override, reg)
			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("ResolveTackler(%+v, %+v) = _, %v; want an error containing %q", tt.cfg, tt.override, err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTackler(%+v, %+v) = _, %v; want nil error", tt.cfg, tt.override, err)
			}
			want := settingsOf(tt.wantSpec)
			want.Timeout, want.MaxSpawns = tt.wantTimeout, tt.wantSpawns
			if got != want {
				t.Errorf("ResolveTackler(%+v, %+v) = %+v; want %+v", tt.cfg, tt.override, got, want)
			}
		})
	}
}
