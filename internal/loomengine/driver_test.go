//go:build tier0

// driver_test.go — untagged Tier-1 unit tests for ResolveDriver.
// Mirrors review_test.go's shape: pure Go over an in-memory Config and a temp-dir modelspec
// registry, no live hub, reed, or network involved.

package loomengine

import (
	"strings"
	"testing"

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
