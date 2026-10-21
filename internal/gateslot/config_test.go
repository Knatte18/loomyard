// config_test.go covers loading the hub-wide gate config over a bare temp directory seeded with just a gate.yaml, with no hub and no git.

package gateslot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
)

var templateDefaults = Config{Slots: 2, GoParallel: 4, BusyQuitSec: 120}

func seedGateConfig(t *testing.T, contents string) string {
	t.Helper()
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if contents != "" {
		if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return baseDir
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		want     Config
		wantErr  string
	}{
		{"template loads to its defaults", ConfigTemplate(), templateDefaults, ""},
		{"zero slots", "slots: 0\ngo_parallel: 4\n", Config{}, `gate config key "slots": 0; want at least 1; fix it with "lyx config gate" from the prime` + roleClause},
		{"zero go_parallel", "slots: 2\ngo_parallel: 0\n", Config{}, `gate config key "go_parallel": 0; want at least 1; fix it with "lyx config gate" from the prime` + roleClause},
		{"a retired cli_wait_sec still loads", "slots: 3\ngo_parallel: 4\ncli_wait_sec: 180\n", Config{Slots: 3, GoParallel: 4, BusyQuitSec: 120}, ""},
		{"busy_quit_sec zero turns the watchdog off", "slots: 2\ngo_parallel: 4\nbusy_quit_sec: 0\n", Config{Slots: 2, GoParallel: 4, BusyQuitSec: 0}, ""},
		{"absent busy_quit_sec takes the template value", "slots: 2\ngo_parallel: 4\n", templateDefaults, ""},
		{"negative busy_quit_sec", "slots: 2\ngo_parallel: 4\nbusy_quit_sec: -1\n", Config{}, `gate config key "busy_quit_sec": -1; want at least 0; fix it with "lyx config gate" from the prime` + roleClause},
		{"absent file names the one way forward", "", Config{}, `gate config absent; run "lyx fabric reconcile"; a session lyx refuses the verb from reports status: FAILED and the orch runs it`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadConfig(configengine.Dirs{Board: seedGateConfig(t, tc.contents)})
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("LoadConfig() error = %v; want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("LoadConfig() = (%+v, %v); want (%+v, nil)", got, err, tc.want)
			}
		})
	}

	for _, key := range []string{"slots", "go_parallel"} {
		t.Run("a local "+key+" is refused naming the local file and the key", func(t *testing.T) {
			t.Parallel()
			board, anchor := seedGateConfig(t, ConfigTemplate()), seedGateConfig(t, "")
			if err := os.MkdirAll(configengine.ScratchDir(anchor), 0o755); err != nil {
				t.Fatal(err)
			}
			localPath := configengine.LocalFile(anchor, "gate")
			if err := os.WriteFile(localPath, []byte(key+": 9\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
			}
			for _, want := range []string{localPath, key, "remove " + localPath + ", which takes no gate key"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

	machineTests := []struct {
		name string
		// hub is the hub file's contents; "" leaves it absent.
		hub     string
		machine string
		// boardOnly loads with no anchor, as a hub-file check does.
		boardOnly bool
		want      Config
		// wantErr is the exact message, built from the hub and machine file paths.
		wantErr func(hub, machine string) string
		// wantInvalid requires the error to be marked configengine.ErrInvalid.
		wantInvalid bool
		// wantContains lists text a failure must hold, for the paths and keys a row does not pin whole.
		wantContains func(hub, machine string) []string
		wantAbsent   bool
	}{
		{
			name: "a machine go_parallel beats the hub's", hub: "slots: 2\ngo_parallel: 4\n", machine: "go_parallel: 8\n",
			want: Config{Slots: 2, GoParallel: 8, BusyQuitSec: 120},
		},
		{
			name: "a machine go_parallel below 1 names both files and both ways forward", hub: "slots: 2\ngo_parallel: 4\n", machine: "go_parallel: 0\n",
			wantErr: func(hub, machine string) string {
				return `gate config key "go_parallel": 0; want at least 1` + twoFileWayForward(hub, machine)
			},
		},
		{
			name: "a machine busy_quit_sec is refused as a cross-pair key", hub: "slots: 2\ngo_parallel: 4\n", machine: "busy_quit_sec: 9\n",
			wantInvalid: true,
			wantErr: func(hub, machine string) string {
				return "machine config file " + machine + `: key "busy_quit_sec" is cross-pair and cannot be set in a machine override; way forward: remove it from the machine override and set it in the hub file through "lyx config gate" from the prime` + twoFileWayForward(hub, machine)
			},
		},
		{
			name: "a machine cli_wait_sec is refused as retired", hub: "slots: 2\ngo_parallel: 4\n", machine: "cli_wait_sec: 180\n",
			wantInvalid:  true,
			wantContains: func(hub, machine string) []string { return []string{machine, `"cli_wait_sec"`, "retired"} },
		},
		{
			name: "a board-only load never stacks the machine file", hub: "slots: 2\ngo_parallel: 4\n", machine: "go_parallel: 0\n", boardOnly: true,
			want: Config{Slots: 2, GoParallel: 4, BusyQuitSec: 120},
		},
		{
			name: "a machine file beside an absent hub file is still absent", machine: "go_parallel: 8\n",
			wantAbsent: true,
		},
	}
	for _, tc := range machineTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := seedGateConfig(t, tc.hub), seedGateConfig(t, "")
			machinePath := configengine.MachineFile(board, "gate")
			if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(machinePath, []byte(tc.machine), 0o644); err != nil {
				t.Fatal(err)
			}
			dirs := configengine.Dirs{Anchor: anchor, Board: board}
			if tc.boardOnly {
				dirs.Anchor = ""
			}

			got, err := LoadConfig(dirs)
			hubPath := configengine.ConfigFile(board, "gate")
			switch {
			case tc.wantAbsent:
				if !errors.Is(err, ErrConfigAbsent) {
					t.Fatalf("LoadConfig() error = %v; want ErrConfigAbsent", err)
				}
			case tc.wantErr != nil || tc.wantContains != nil:
				if err == nil {
					t.Fatalf("LoadConfig() = %+v, nil; want an error", got)
				}
				if tc.wantErr != nil && err.Error() != tc.wantErr(hubPath, machinePath) {
					t.Errorf("LoadConfig() error = %q; want %q", err.Error(), tc.wantErr(hubPath, machinePath))
				}
				if tc.wantContains != nil {
					for _, want := range tc.wantContains(hubPath, machinePath) {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
						}
					}
				}
				if tc.wantInvalid && !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("LoadConfig() error = %v; want ErrInvalid", err)
				}
			default:
				if err != nil || got != tc.want {
					t.Fatalf("LoadConfig() = (%+v, %v); want (%+v, nil)", got, err, tc.want)
				}
			}
		})
	}
}

// roleClause closes every gate load failure's way forward.
const roleClause = "; a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them"

// twoFileWayForward is the way forward of a failure with a machine file in the stack.
func twoFileWayForward(hub, machine string) string {
	return "; the hub file " + hub + ", the machine file " + machine + " or the file the error names holds the value: " +
		`fix the hub file with "lyx config gate" from the prime, and the machine file with "lyx config gate --layer machine" from the prime, ` +
		"or, when that refuses a key the machine file holds, by removing " + machine + "; " +
		`"lyx config gate --resolved" names the layer holding each value` + roleClause
}

func TestTemplateConfigAndLimits(t *testing.T) {
	t.Parallel()

	cfg, err := TemplateConfig()
	if err != nil || cfg != templateDefaults {
		t.Fatalf("TemplateConfig() = (%+v, %v); want (%+v, nil)", cfg, err, templateDefaults)
	}
	want := Limits{Slots: 2, GoParallel: 4, BusyQuit: 120 * time.Second}
	if got := cfg.Limits(); got != want {
		t.Errorf("Limits() = %+v; want %+v", got, want)
	}
}
