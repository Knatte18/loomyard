// driver_test.go — untagged Tier-1 unit tests for ResolveDriver.
// Mirrors review_test.go's shape: pure Go over an in-memory Config and a temp-dir modelspec
// registry, no live hub, reed, or network involved.

package loomengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestResolveDriver verifies ResolveDriver resolves an alias to its provider model id, lifting
// effort and version out of the resolved value's Params -- the load-bearing case, since a test
// asserting only that Model is non-empty would also pass against a raw copy of cfg.Driver, which is
// the exact bug reg.Resolve exists to prevent.
func TestResolveDriver(t *testing.T) {
	cfg := Config{Driver: "opus[effort=high,v=1]"}

	reg, err := modelspec.LoadRegistry(t.TempDir())
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
}

// TestResolveDriver_UnknownAlias verifies an alias absent from the registry returns an error naming
// the driver role, rather than being silently carried into the driver session's spawn site.
func TestResolveDriver_UnknownAlias(t *testing.T) {
	cfg := Config{Driver: "not-a-real-alias"}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	_, err = ResolveDriver(cfg, reg)
	if err == nil {
		t.Fatal("ResolveDriver(...) = _, nil; want non-nil error for an unknown driver alias")
	}
	if !strings.Contains(err.Error(), "driver") {
		t.Errorf("ResolveDriver(...) error = %q; want it to name the driver role", err.Error())
	}
	if !strings.Contains(err.Error(), "not-a-real-alias") {
		t.Errorf("ResolveDriver(...) error = %q; want it to name the unknown alias %q", err.Error(), "not-a-real-alias")
	}
}

// TestResolveDriver_MalformedSpec verifies an ungrammatical driver model-spec returns an error
// naming the driver role, rather than being silently carried into the driver session's spawn site.
func TestResolveDriver_MalformedSpec(t *testing.T) {
	cfg := Config{Driver: "opus[effort"}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	_, err = ResolveDriver(cfg, reg)
	if err == nil {
		t.Fatal("ResolveDriver(...) = _, nil; want non-nil error for malformed driver spec")
	}
	if !strings.Contains(err.Error(), "driver") {
		t.Errorf("ResolveDriver(...) error = %q; want it to name the driver role", err.Error())
	}
}

// TestResolveDriver_EmptyDriverIsOffDefault verifies an empty cfg.Driver -- "defer to the provider
// default" -- resolves to a zero DriverSettings with a nil error, the same "off/default" arm
// Friction already has, rather than being forced through modelspec.Parse.
func TestResolveDriver_EmptyDriverIsOffDefault(t *testing.T) {
	cfg := Config{Driver: ""}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveDriver(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveDriver(...) = _, %v; want nil error for an empty cfg.Driver", err)
	}
	if settings != (DriverSettings{}) {
		t.Errorf("ResolveDriver(...) = %+v; want the zero DriverSettings for an empty cfg.Driver", settings)
	}
}
