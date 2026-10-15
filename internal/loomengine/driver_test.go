// driver_test.go — untagged Tier-1 unit tests for ResolveDriver.
// Mirrors review_test.go's shape: pure Go over an in-memory Config and a temp-dir modelspec
// registry, no live hub, reed, or network involved.

package loomengine

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestResolveDriver verifies ResolveDriver resolves an alias to its provider model id, lifting
// effort and version out of the resolved value's Params -- the load-bearing case, since a test
// asserting only that Model is non-empty would also pass against a raw copy of cfg.Driver, which is
// the exact bug reg.Resolve exists to prevent.
func TestResolveDriver(t *testing.T) {
	cfg := Config{Driver: "opus[effort=high,v=1]"}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := modelspec.Parse(cfg.Driver)
	if err != nil {
		t.Fatalf("modelspec.Parse(%q) = _, %v; want nil error", cfg.Driver, err)
	}
	wantResolved, err := reg.Resolve(spec)
	if err != nil {
		t.Fatalf("reg.Resolve(...) = _, %v; want nil error", err)
	}

	settings, err := ResolveDriver(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveDriver(...) = _, %v; want nil error", err)
	}
	if settings.Model != wantResolved.Model {
		t.Errorf("ResolveDriver(...).Model = %q; want %q (the registry's resolved provider model id, not the raw alias string)", settings.Model, wantResolved.Model)
	}
	if settings.Model == cfg.Driver {
		t.Errorf("ResolveDriver(...).Model = %q; must not equal the raw cfg.Driver string %q -- resolution must go through the registry", settings.Model, cfg.Driver)
	}
	if settings.Effort != "high" {
		t.Errorf("ResolveDriver(...).Effort = %q; want %q", settings.Effort, "high")
	}
	if settings.Version != "1" {
		t.Errorf("ResolveDriver(...).Version = %q; want %q", settings.Version, "1")
	}
	if settings.Engine != "claude" {
		t.Errorf("ResolveDriver(...).Engine = %q; want %q (the alias's engine)", settings.Engine, "claude")
	}
}

// TestResolveArbiter verifies ResolveArbiter resolves the spec through the registry, takes opus[high] for an empty spec and 30 minutes for a zero timeout, passes a set timeout and the spawn budget through, and names the arbiter key when the registry lacks the alias.
func TestResolveArbiter(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	resolve := func(text string) (ArbiterSettings, error) {
		t.Helper()
		spec, err := modelspec.Parse(text)
		if err != nil {
			t.Fatalf("modelspec.Parse(%q) = _, %v; want nil error", text, err)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			t.Fatalf("reg.Resolve(%q) = _, %v; want nil error", text, err)
		}
		return ArbiterSettings{ModelChoice: shuttleengine.NewModelChoice(resolved)}, nil
	}

	tests := []struct {
		name        string
		cfg         Config
		wantSpec    string
		wantTimeout time.Duration
		wantSpawns  int
		wantErrIn   []string
	}{
		{name: "empty spec takes opus[high]", cfg: Config{ArbiterMaxSpawns: 2}, wantSpec: "opus[high]", wantTimeout: 30 * time.Minute, wantSpawns: 2},
		{name: "set spec resolves to itself", cfg: Config{Arbiter: "sonnet[low]"}, wantSpec: "sonnet[low]", wantTimeout: 30 * time.Minute},
		{name: "set timeout passes through", cfg: Config{ArbiterTimeoutMin: 7, ArbiterMaxSpawns: 1}, wantSpec: "opus[high]", wantTimeout: 7 * time.Minute, wantSpawns: 1},
		{name: "alias the registry lacks", cfg: Config{Arbiter: "not-a-real-alias"}, wantErrIn: []string{"arbiter", "not-a-real-alias"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveArbiter(tt.cfg, reg)
			if tt.wantErrIn != nil {
				if err == nil {
					t.Fatalf("ResolveArbiter(%+v) = _, nil; want an error", tt.cfg)
				}
				for _, want := range tt.wantErrIn {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ResolveArbiter(%+v) error = %q; want it to contain %q", tt.cfg, err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveArbiter(%+v) = _, %v; want nil error", tt.cfg, err)
			}
			want, _ := resolve(tt.wantSpec)
			want.Timeout, want.MaxSpawns = tt.wantTimeout, tt.wantSpawns
			if got != want {
				t.Errorf("ResolveArbiter(%+v) = %+v; want %+v", tt.cfg, got, want)
			}
			if got.Engine != "claude" {
				t.Errorf("ResolveArbiter(%+v).Engine = %q; want %q (the alias's engine)", tt.cfg, got.Engine, "claude")
			}
		})
	}
}

// TestResolveAlignment verifies ResolveAlignment resolves the spec through the registry, takes opus[high] for an empty spec and 20 minutes for a zero timeout, passes a set timeout through, and names the alignment key when the spec does not resolve.
func TestResolveAlignment(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	resolve := func(text string) AlignmentSettings {
		t.Helper()
		spec, err := modelspec.Parse(text)
		if err != nil {
			t.Fatalf("modelspec.Parse(%q) = _, %v; want nil error", text, err)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			t.Fatalf("reg.Resolve(%q) = _, %v; want nil error", text, err)
		}
		return AlignmentSettings{ModelChoice: shuttleengine.NewModelChoice(resolved)}
	}

	tests := []struct {
		name        string
		cfg         Config
		wantSpec    string
		wantTimeout time.Duration
		wantErrIn   []string
	}{
		{name: "empty spec and zero timeout take the defaults", cfg: Config{}, wantSpec: "opus[high]", wantTimeout: 20 * time.Minute},
		{name: "set spec resolves to itself", cfg: Config{Alignment: "sonnet[low]"}, wantSpec: "sonnet[low]", wantTimeout: 20 * time.Minute},
		{name: "set timeout passes through", cfg: Config{AlignmentTimeoutMin: 7}, wantSpec: "opus[high]", wantTimeout: 7 * time.Minute},
		{name: "alias the registry lacks", cfg: Config{Alignment: "not-a-real-alias"}, wantErrIn: []string{"alignment", "not-a-real-alias"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveAlignment(tt.cfg, reg)
			if tt.wantErrIn != nil {
				if err == nil {
					t.Fatalf("ResolveAlignment(%+v) = _, nil; want an error", tt.cfg)
				}
				for _, want := range tt.wantErrIn {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ResolveAlignment(%+v) error = %q; want it to contain %q", tt.cfg, err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAlignment(%+v) = _, %v; want nil error", tt.cfg, err)
			}
			want := resolve(tt.wantSpec)
			want.Timeout = tt.wantTimeout
			if got != want {
				t.Errorf("ResolveAlignment(%+v) = %+v; want %+v", tt.cfg, got, want)
			}
			if got.Engine != "claude" {
				t.Errorf("ResolveAlignment(%+v).Engine = %q; want %q (the alias's engine)", tt.cfg, got.Engine, "claude")
			}
		})
	}
}

// TestResolveDriver_Refuses verifies an alias absent from the registry and an ungrammatical driver model-spec each return an error naming the driver role, rather than being silently carried into the driver session's spawn site.
func TestResolveDriver_Refuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		driver string
		wantIn []string
	}{
		{"unknown alias", "not-a-real-alias", []string{"driver", "not-a-real-alias"}},
		{"malformed spec", "opus[effort", []string{"driver"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			_, err = ResolveDriver(Config{Driver: tt.driver}, reg)
			if err == nil {
				t.Fatalf("ResolveDriver(%q) = _, nil; want non-nil error", tt.driver)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ResolveDriver(%q) error = %q; want it to contain %q", tt.driver, err.Error(), want)
				}
			}
		})
	}
}

// TestResolveDriver_EmptyDriverIsOffDefault verifies an empty cfg.Driver -- "defer to the provider
// default" -- resolves to the zero ModelChoice (an empty Engine reads as the default engine) with a nil error, the same "off/default" arm
// Friction already has, rather than being forced through modelspec.Parse.
func TestResolveDriver_EmptyDriverIsOffDefault(t *testing.T) {
	cfg := Config{Driver: ""}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveDriver(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveDriver(...) = _, %v; want nil error for an empty cfg.Driver", err)
	}
	if settings != (shuttleengine.ModelChoice{}) {
		t.Errorf("ResolveDriver(...) = %+v; want the zero ModelChoice for an empty cfg.Driver", settings)
	}
}
