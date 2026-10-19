// entries_implementer_test.go pins the Implementer entry: the engine its setting names builds the row, and every unusable setting refuses with a way forward.

package shedrecipe

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
)

// newImplementerTestEnv returns newTestEnv with a setting lookup answering loom.implementer with webster.
func newImplementerTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.Setting = fakeSetting(map[string]any{"loom.implementer": "webster"})
	return env
}

// implementerTestConfig is the Config of an Implementer row naming loom.implementer.
func implementerTestConfig(*testing.T, Env) Config {
	return Config{implementerSettingKey: "loom.implementer"}
}

func TestImplementerEntry_WebsterBuildsTheWebsterProducer(t *testing.T) {
	t.Parallel()
	env := newImplementerTestEnv(t)
	producer, err := implementerEntry(RowIdentity{Name: "Webster"}, implementerTestConfig(t, env), env)
	if err != nil {
		t.Fatalf("implementerEntry() error = %v; want nil", err)
	}
	want := reflect.TypeOf(loomshed.NewWebsterProducer("", env.ConfigDirs, nil, env.WebsterDeps, nil))
	if got := reflect.TypeOf(producer); got != want {
		t.Errorf("implementerEntry() producer type = %v; want %v", got, want)
	}
}

func TestImplementerEntry_ForwardsTheRestOfTheRowConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		extra   Config
		wantErr string
	}{
		{"GatesKeyBuilds", gatesCfg("plan", 4), ""},
		{"UnknownKeyIsRefusedByTheConstructor", Config{"bogus": true}, `"bogus"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := reworkTestEnv(t)
			env.Setting = fakeSetting(map[string]any{"loom.implementer": "webster"})
			cfg := Config{implementerSettingKey: "loom.implementer"}
			for key, value := range tt.extra {
				cfg[key] = value
			}
			producer, err := implementerEntry(RowIdentity{Name: "Webster"}, cfg, env)
			if tt.wantErr != "" {
				assertErrContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("implementerEntry() error = %v; want nil", err)
			}
			if producer == nil {
				t.Error("implementerEntry() producer = nil; want the webster producer")
			}
		})
	}
}

func TestImplementerEntry_RefusesAnUnusableSetting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     Config
		setting func(module, key string) (any, error)
		want    string
	}{
		{"AbsentKey", Config{}, fakeSetting(nil), `way forward: set implementer_setting to the module.key setting naming the implementer, such as "loom.implementer"`},
		{"EmptyKey", Config{implementerSettingKey: ""}, fakeSetting(nil), `config key "implementer_setting" is empty; way forward: set implementer_setting to the module.key setting`},
		{"NonStringKey", Config{implementerSettingKey: 3}, fakeSetting(nil), `holds 3, want a module.key string; way forward: set implementer_setting to a module.key string such as "loom.implementer"`},
		{"UnknownImplementer", Config{implementerSettingKey: "loom.implementer"}, fakeSetting(map[string]any{"loom.implementer": "weaver"}), `holds "weaver", want one of webster; way forward: run "lyx config loom --set implementer=<implementer>" with one of webster`},
		{"EmptyImplementer", Config{implementerSettingKey: "loom.implementer"}, fakeSetting(map[string]any{"loom.implementer": ""}), `holds "", want one of webster; way forward`},
		{"NonStringImplementer", Config{implementerSettingKey: "loom.implementer"}, fakeSetting(map[string]any{"loom.implementer": 3}), `holds 3, want one of webster; way forward`},
		{"NullImplementer", Config{implementerSettingKey: "loom.implementer"}, fakeSetting(map[string]any{"loom.implementer": nil}), `holds <nil>, want one of webster; way forward`},
		{"LookupError", Config{implementerSettingKey: "loom.implementer"}, func(string, string) (any, error) { return nil, errors.New("loom.yaml unreadable") }, `setting "loom.implementer": loom.yaml unreadable`},
		{"NilLookup", Config{implementerSettingKey: "loom.implementer"}, nil, "way forward: fill Env.Setting in the caller that builds this recipe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			env.Setting = tt.setting
			producer, err := implementerEntry(RowIdentity{Name: "Webster"}, tt.cfg, env)
			assertErrContains(t, err, tt.want)
			if producer != nil {
				t.Errorf("implementerEntry() producer = %v; want nil", producer)
			}
		})
	}
}
