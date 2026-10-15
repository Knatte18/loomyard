// engineset_test.go verifies EngineSet's name lookup, its default, its sorted names and its isolation from the map it was built over.

package shuttleengine

import (
	"errors"
	"reflect"
	"testing"
)

// singleEngineSet wraps one engine as the default of a one-engine set, for tests that exercise a consumer of the set.
func singleEngineSet(engine Engine) EngineSet {
	return NewEngineSet("fake", map[string]Engine{"fake": engine})
}

// requireEngineSkipEpisode asserts the log holds one engine-skip episode that names runDir: one Warn, two Debug lines and one recovery.
func requireEngineSkipEpisode(t *testing.T, log, runDir string) {
	t.Helper()
	requireFailureEpisode(t, log, "skipping a run whose engine is not registered", 2, "run engine registered again")
	if got := countLevelLines(log, "WARN", runDir); got != 1 {
		t.Errorf("WARN lines naming %s = %d, want 1; log: %s", runDir, got, log)
	}
}

// TestEngineSet_Lookup pins how a name resolves: the empty name to the default, a registered name to its own engine,
// and an unknown name or an unregistered default to ErrEngineNotRegistered naming the sorted registered names.
func TestEngineSet_Lookup(t *testing.T) {
	t.Parallel()

	alpha := &fakeEngine{}
	beta := &fakeEngine{}
	engines := map[string]Engine{"beta": beta, "alpha": alpha}

	tests := []struct {
		name        string
		defaultName string
		lookup      string
		want        Engine
		wantErr     string
	}{
		{name: "EmptyNameResolvesDefault", defaultName: "beta", lookup: "", want: beta},
		{name: "RegisteredNameResolvesItself", defaultName: "beta", lookup: "alpha", want: alpha},
		{
			name: "UnknownName", defaultName: "beta", lookup: "gamma",
			wantErr: `shuttle: the engine is not registered: "gamma"; registered engines: alpha, beta`,
		},
		{
			name: "UnregisteredDefault", defaultName: "gamma", lookup: "",
			wantErr: `shuttle: the engine is not registered: "gamma"; registered engines: alpha, beta`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set := NewEngineSet(tt.defaultName, engines)
			got, err := set.Lookup(tt.lookup)
			if tt.wantErr != "" {
				if !errors.Is(err, ErrEngineNotRegistered) {
					t.Fatalf("Lookup(%q) error = %v; want ErrEngineNotRegistered", tt.lookup, err)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("Lookup(%q) error = %q; want %q", tt.lookup, err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Lookup(%q) error = %v", tt.lookup, err)
			}
			if got != tt.want {
				t.Errorf("Lookup(%q) = %p; want %p", tt.lookup, got, tt.want)
			}
		})
	}

	t.Run("NamesSortedAndDefaultTold", func(t *testing.T) {
		t.Parallel()
		set := NewEngineSet("beta", engines)
		if got, want := set.Names(), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Names() = %v; want %v", got, want)
		}
		if got := set.DefaultName(); got != "beta" {
			t.Errorf("DefaultName() = %q; want %q", got, "beta")
		}
	})

	t.Run("MutatingTheMapLeavesTheSetUnchanged", func(t *testing.T) {
		t.Parallel()
		source := map[string]Engine{"alpha": alpha}
		set := NewEngineSet("alpha", source)
		source["beta"] = beta
		delete(source, "alpha")
		if got, want := set.Names(), []string{"alpha"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Names() after mutating the source map = %v; want %v", got, want)
		}
		if got, err := set.Lookup("alpha"); err != nil || got != alpha {
			t.Errorf("Lookup(alpha) after mutating the source map = %v, %v; want the original engine", got, err)
		}
	})
}
