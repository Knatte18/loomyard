// shuttleengines_test.go verifies the registry builds a set from a shuttle config, refuses an unregistered engine from any layer, and stays equal to modelspec's closed engine names.

package shuttleengines

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// loadWithLocalEngine loads a shuttle config over a temp anchor whose local layer file sets engine: bogus.
func loadWithLocalEngine(t *testing.T) shuttleengine.Config {
	t.Helper()
	anchor := t.TempDir()
	if err := os.Mkdir(filepath.Join(anchor, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	localFile := configengine.LocalFile(anchor, "shuttle")
	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatalf("mkdir local dir: %v", err)
	}
	if err := os.WriteFile(localFile, []byte("engine: bogus\n"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// TestNew pins the set the template config builds and the refusal of an engine name no layer may smuggle in.
func TestNew(t *testing.T) {
	t.Parallel()

	templateCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig over the template: %v", err)
	}

	tests := []struct {
		name    string
		cfg     shuttleengine.Config
		wantErr bool
	}{
		{name: "TemplateConfig", cfg: templateCfg},
		{name: "UnregisteredEngine", cfg: shuttleengine.Config{Engine: "bogus"}, wantErr: true},
		{name: "UnregisteredEngineFromLocalLayer", cfg: loadWithLocalEngine(t), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := New(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New(%q) accepted an unregistered engine", tt.cfg.Engine)
				}
				for _, want := range []string{"bogus", "registered engines: claude", "engine: in shuttle.yaml"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("New error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			engine, err := set.Lookup("")
			if err != nil {
				t.Fatalf("Lookup of the default: %v", err)
			}
			if _, ok := engine.(*claudeengine.Claude); !ok {
				t.Errorf("default engine = %T; want *claudeengine.Claude", engine)
			}
			if got, want := set.Names(), []string{"claude"}; !reflect.DeepEqual(got, want) {
				t.Errorf("Names() = %v; want %v", got, want)
			}
		})
	}
}

// TestNames_EqualModelspecEngineNames pins that the registry and modelspec's closed set name the same engines, and that modelspec hands out a copy.
func TestNames_EqualModelspecEngineNames(t *testing.T) {
	t.Parallel()

	if got, want := Names(), modelspec.EngineNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v; want modelspec.EngineNames() = %v", got, want)
	}

	// A caller overwriting the slice it was handed must not change the closed set.
	modelspec.EngineNames()[0] = "overwritten"
	if got, want := Names(), modelspec.EngineNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after overwriting a returned slice, Names() = %v; want modelspec.EngineNames() = %v", got, want)
	}
}
