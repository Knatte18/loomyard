// config_test.go verifies env.yaml's layer stack, its name refusals and the machine-layer write.
// It also verifies the two forms the variables are handed on in.
// Layers are seeded with plain file writes under t.TempDir(), so the test stays spawn-free.

package envconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// hubDirs returns the told directories of a hub whose board dir and anchor both sit under a fresh temp dir.
// The anchor holds a _lym/, since an anchor without one has no local layer.
func hubDirs(t *testing.T) configengine.Dirs {
	t.Helper()
	root := t.TempDir()
	dirs := configengine.Dirs{Board: filepath.Join(root, "board"), Anchor: filepath.Join(root, "anchor")}
	if err := os.MkdirAll(configengine.ConfigDir(dirs.Anchor), 0o755); err != nil {
		t.Fatal(err)
	}
	return dirs
}

func TestLoadConfig_StacksHubAndMachineName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// hub, machine and local are the layer files' content; empty writes no file.
		hub, machine, local string
		want                map[string]string
		wantInvalid         string
	}{
		{
			name: "NoFileSetsNothing",
			want: map[string]string{},
		},
		{
			name: "HubFileAlone",
			hub:  "vars:\n  COMMONLIB_REPO: /hub/commonlib\n  MODE: ci\n",
			want: map[string]string{"COMMONLIB_REPO": "/hub/commonlib", "MODE": "ci"},
		},
		{
			name:    "MachineValueWinsByNameAndKeepsTheRest",
			hub:     "vars:\n  COMMONLIB_REPO: /hub/commonlib\n  MODE: ci\n",
			machine: "vars:\n  COMMONLIB_REPO: /home/me/commonlib\n  EXTRA: \"1\"\n",
			want:    map[string]string{"COMMONLIB_REPO": "/home/me/commonlib", "MODE": "ci", "EXTRA": "1"},
		},
		{
			name: "NumberValueReadsAsText",
			hub:  "vars:\n  PORT: 8080\n",
			want: map[string]string{"PORT": "8080"},
		},
		{
			name:        "NameWithADashIsRefused",
			machine:     "vars:\n  BAD-NAME: x\n",
			wantInvalid: `"vars.BAD-NAME"`,
		},
		{
			name:        "NameStartingWithADigitIsRefused",
			hub:         "vars:\n  1ST: x\n",
			wantInvalid: `"vars.1ST"`,
		},
		{
			name:        "LymOwnNameIsRefused",
			hub:         "vars:\n  LYM_STRAND_NAME: x\n",
			wantInvalid: `"vars.LYM_STRAND_NAME"`,
		},
		{
			name:        "LowercaseLymNameIsRefused",
			hub:         "vars:\n  lym_parent: x\n",
			wantInvalid: `"vars.lym_parent"`,
		},
		{
			name:        "PathIsRefused",
			machine:     "vars:\n  PATH: /x\n",
			wantInvalid: `"vars.PATH"`,
		},
		{
			name:        "PathInAnotherCaseIsRefused",
			hub:         "vars:\n  Path: /x\n",
			wantInvalid: `"vars.Path"`,
		},
		{
			name:        "LocalLayerIsRefused",
			local:       "vars:\n  COMMONLIB_REPO: /x\n",
			wantInvalid: "machine and cannot be set in a local override",
		},
		{
			name:        "MachineValueThatIsAMappingIsRefused",
			machine:     "vars:\n  NESTED:\n    a: b\n",
			wantInvalid: `"vars.NESTED.a" is not in the env template`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := hubDirs(t)
			if err := os.MkdirAll(dirs.Anchor, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.hub != "" {
				writeFile(t, configengine.ConfigFile(dirs.Board, "env"), tt.hub)
			}
			if tt.machine != "" {
				writeFile(t, configengine.MachineFile(dirs.Board, "env"), tt.machine)
			}
			if tt.local != "" {
				writeFile(t, configengine.LocalFile(dirs.Anchor, "env"), tt.local)
			}

			got, err := envconfig.LoadConfig(dirs)

			if tt.wantInvalid != "" {
				if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), tt.wantInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error holding %s", err, tt.wantInvalid)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(got.Vars, tt.want) {
				t.Errorf("LoadConfig().Vars = %v; want %v", got.Vars, tt.want)
			}
		})
	}
}

// TestSet_MachineLayerTakesAVariable pins a per-machine value set through the machine layer,
// and the local layer still refuses it.
func TestSet_MachineLayerTakesAVariable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dirs := hubDirs(t)
	if err := os.MkdirAll(dirs.Anchor, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, configengine.ConfigFile(dirs.Board, "env"), "vars:\n  MODE: ci\n")
	pair := []yamlengine.KV{{Key: "vars.COMMONLIB_REPO", Value: "/home/me/commonlib"}}

	if _, err := configengine.Set(dirs, envconfig.ConfigSpec(), configengine.LayerMachine, pair); err != nil {
		t.Fatalf("Set(machine, vars.COMMONLIB_REPO) error = %v; want nil", err)
	}
	if _, err := configengine.Set(dirs, envconfig.ConfigSpec(), configengine.LayerLocal, pair); !errors.Is(err, configengine.ErrInvalid) {
		t.Errorf("Set(local, vars.COMMONLIB_REPO) error = %v; want an ErrInvalid refusal", err)
	}
	if _, err := os.Stat(configengine.LocalFile(dirs.Anchor, "env")); !os.IsNotExist(err) {
		t.Errorf("Stat(local file) error = %v; want the refused write to leave no file", err)
	}

	got, err := envconfig.LoadConfig(dirs)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v; want nil", err)
	}
	want := map[string]string{"COMMONLIB_REPO": "/home/me/commonlib", "MODE": "ci"}
	if !reflect.DeepEqual(got.Vars, want) {
		t.Errorf("LoadConfig().Vars = %v; want %v", got.Vars, want)
	}
}

func TestConfig_EntriesAndBeneath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg := envconfig.Config{Vars: map[string]string{"B": "hub-b", "A": "hub-a"}}

	if got, want := cfg.Entries(), []string{"A=hub-a", "B=hub-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Entries() = %v; want %v", got, want)
	}

	own := map[string]string{"B": "own-b", "C": "own-c"}
	got := cfg.Beneath(own)
	want := map[string]string{"A": "hub-a", "B": "own-b", "C": "own-c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Beneath(%v) = %v; want %v", own, got, want)
	}
	if len(cfg.Vars) != 2 || cfg.Vars["B"] != "hub-b" || len(own) != 2 {
		t.Errorf("Beneath changed an input: Vars = %v, own = %v", cfg.Vars, own)
	}
	if got := (envconfig.Config{}).Beneath(nil); got == nil || len(got) != 0 {
		t.Errorf("Config{}.Beneath(nil) = %#v; want an empty non-nil map", got)
	}
}
