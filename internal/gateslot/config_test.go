//go:build tier0

// config_test.go covers loading the hub-wide gate config over a bare temp directory seeded with just a gate.yaml, with no hub and no git.

package gateslot

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

var templateDefaults = Config{
	Slots: 2, BusyQuitSec: 120,
	Gates: map[string]GateKey{
		"card":            {Tiers: []int{0}, Scope: "card"},
		"batch":           {Tiers: []int{0}, Scope: "card"},
		"webster":         {Tiers: []int{0}, Scope: "all"},
		"parent-align":    {Tiers: []int{0}, Scope: "all"},
		"finalize":        {Tiers: []int{0}, Scope: "all"},
		"webster-fix":     {Tiers: []int{0}, Scope: "diff"},
		"darn-write":      {Tiers: []int{0}, Scope: "diff"},
		"code-burler-fix": {Tiers: []int{0}, Scope: "diff"},
	},
	Scoper: "none",
}

// equateEmpty makes a nil and an empty map or slice equal, since a Config holds either for an empty gate.yaml collection.
var equateEmpty = cmpopts.EquateEmpty()

// hubWith returns the template defaults with mutate applied to a copy.
func hubWith(mutate func(*Config)) Config {
	cfg := templateDefaults
	cfg.Tiers = map[string]Tier{}
	cfg.TierParallel = map[string]int{}
	mutate(&cfg)
	return cfg
}

// fixIt is the way forward every key refusal of a load carries.
const fixIt = `; fix it with "lyx config gate" from the prime` + roleClause

// baseKeys is a hub file holding the slot keys alone, so a row states only the keys it tests.
const baseKeys = "slots: 2\n"

// hubTier0 is a hub file declaring tier0, so a machine tier_parallel entry for it takes part in the load.
const hubTier0 = baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest}}`

// machineBadParallel is a machine file whose tier_parallel entry the load refuses.
const machineBadParallel = "tier_parallel:\n  tier0: 0\n"

// badParallelKey is the key a refusal of machineBadParallel names.
const badParallelKey = `gate config key "tier_parallel.tier0": "0"`

// tier0Flow is a tier written whole in flow form with parallel and slots absent.
const tier0Flow = `tiers:
  tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}
`

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
		{"template loads to its defaults", ConfigTemplate(), hubWith(func(*Config) {}), ""},
		{"zero slots", "slots: 0\n", Config{}, `gate config key "slots": 0; want at least 1; fix it with "lyx config gate" from the prime` + roleClause},
		{"a retired go_parallel still loads, whatever its value", "slots: 2\ngo_parallel: 0\n", hubWith(func(*Config) {}), ""},
		{"a retired cli_wait_sec still loads", "slots: 3\ncli_wait_sec: 180\n", hubWith(func(c *Config) { c.Slots = 3 }), ""},
		{"a retired guard_tier still loads", "slots: 3\nguard_tier: 1\n", hubWith(func(c *Config) { c.Slots = 3 }), ""},
		{"busy_quit_sec zero turns the watchdog off", "slots: 2\nbusy_quit_sec: 0\n", hubWith(func(c *Config) { c.BusyQuitSec = 0 }), ""},
		{
			"a tier in flow form reads parallel and slots as 1", baseKeys + tier0Flow,
			hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "go test -json {targets} {args}", Dir: ".", AllTargets: "./...", Parallel: 1, TimeoutMin: 60, Counter: "go-json", Slots: 1}
			}), "",
		},
		{
			"a tier with slots all, covers and every key loads", baseKeys + `tiers:
  tier0: {command: "go test", timeout_min: 5, counter: vstest}
  tier1: {about: integration, command: "go test -tags integration {targets}", all_targets: ./..., dir: sub, env: {GOFLAGS: "-p={parallel}"}, parallel: 3, timeout_min: 30, counter: go-json, covers: [0], slots: all, prebuilt_lyx: true}
`,
			hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "go test", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1}
				c.Tiers["tier1"] = Tier{About: "integration", Command: "go test -tags integration {targets}", Dir: "sub", Env: map[string]string{"GOFLAGS": "-p={parallel}"}, AllTargets: "./...", Parallel: 3, TimeoutMin: 30, Counter: "go-json", Covers: []int{0}, Slots: 0, PrebuiltLyx: true}
			}), "",
		},
		{
			"gate keys naming an undeclared tier still load", baseKeys + "gates:\n  card: {tiers: [3], scope: card}\n  push: {tiers: [0, 1], scope: diff}\n",
			hubWith(func(c *Config) {
				c.Gates = maps.Clone(templateDefaults.Gates)
				c.Gates["card"] = GateKey{Tiers: []int{3}, Scope: "card"}
				c.Gates["push"] = GateKey{Tiers: []int{0, 1}, Scope: "diff"}
			}), "",
		},
		{"scope card on a key but card and batch", baseKeys + "gates:\n  parent-align: {tiers: [0], scope: card}\n", Config{}, `gate config key "gates.parent-align": scope card is only for the card and batch keys` + fixIt},
		{"an unknown scope", baseKeys + "gates:\n  parent-align: {tiers: [0], scope: some}\n", Config{}, `gate config key "gates.parent-align": scope "some"; want card, diff or all` + fixIt},
		{"a key in the retired list shape", baseKeys + "gates:\n  card: [0]\n", Config{}, `gate config key "gates.card": the retired list shape; fix it with "lyx fabric reconcile" (or "lyx config reconcile --layer default --apply") from the prime, which migrates it` + roleClause},
		{
			"a manual tier above tier0 loads", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest}, tier3: {command: y, timeout_min: 5, counter: vstest, manual: true}}`,
			hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "x", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1}
				c.Tiers["tier3"] = Tier{Command: "y", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1, Manual: true}
			}), "",
		},
		{"a manual tier0", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, manual: true}}`, Config{}, `gate config key "tiers.tier0.manual": true; want false, since tier0 is the tier a call with no tier flag runs` + fixIt},
		{"tier without command", baseKeys + `tiers: {tier0: {timeout_min: 5, counter: vstest}}`, Config{}, `gate config key "tiers.tier0.command": missing; want the shell command a run executes` + fixIt},
		{"tier without timeout_min", baseKeys + `tiers: {tier0: {command: x, counter: vstest}}`, Config{}, `gate config key "tiers.tier0.timeout_min": missing; want at least 1` + fixIt},
		{"tier with timeout_min 0", baseKeys + `tiers: {tier0: {command: x, timeout_min: 0, counter: vstest}}`, Config{}, `gate config key "tiers.tier0.timeout_min": 0; want at least 1` + fixIt},
		{"tier without counter", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5}}`, Config{}, `gate config key "tiers.tier0.counter": missing; want go-json or vstest` + fixIt},
		{"tier with an unknown counter", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: tap}}`, Config{}, `gate config key "tiers.tier0.counter": "tap"; want go-json or vstest` + fixIt},
		{"command with {targets} and no all_targets", baseKeys + `tiers: {tier0: {command: "go test {targets}", timeout_min: 5, counter: go-json}}`, Config{}, `gate config key "tiers.tier0.all_targets": missing; want the {targets} value of a whole run, since the command holds {targets}` + fixIt},
		{"tier with parallel 0", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, parallel: 0}}`, Config{}, `gate config key "tiers.tier0.parallel": 0; want at least 1` + fixIt},
		{"tier with slots 0", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, slots: 0}}`, Config{}, `gate config key "tiers.tier0.slots": "0"; want all or a count of at least 1` + fixIt},
		{"tier with slots everything", baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, slots: everything}}`, Config{}, `gate config key "tiers.tier0.slots": "everything"; want all or a count of at least 1` + fixIt},
		{"tier key whose number overflows int", baseKeys + `tiers: {tier99999999999999999999: {command: x, timeout_min: 5, counter: vstest}}`, Config{}, `gate config key "tier99999999999999999999" under "tiers": want a tier number that fits an int, such as tier0` + fixIt},
		{"covers a tier that is not lower", baseKeys + `tiers: {tier1: {command: x, timeout_min: 5, counter: vstest, covers: [1]}}`, Config{}, `gate config key "tiers.tier1.covers": 1; want tier numbers lower than 1` + fixIt},
		{"tier key not spelled tier<N>", baseKeys + `tiers: {fast: {command: x, timeout_min: 5, counter: vstest}}`, Config{}, `gate config key "fast" under "tiers": want a name spelled tier<N>, such as tier0` + fixIt},
		{"unknown scoper", baseKeys + "scoper: rust\n", Config{}, `gate config key "scoper": "rust"; want go, none or dotnet` + fixIt},
		{"scoper dotnet", baseKeys + "scoper: dotnet\n", hubWith(func(c *Config) { c.Scoper = "dotnet" }), ""},
		{
			"filter, shared_build and carry false beside a build and inert_paths", baseKeys + `tiers: {tier0: {command: "dotnet test {filter}", filter: "Category!=Slow", shared_build: true, carry: false, timeout_min: 5, counter: vstest}, tier1: {command: x, timeout_min: 5, counter: vstest, carry: true}}
build: {command: "dotnet build", timeout_min: 10}
inert_paths: ["*.md", "docs/*"]
`,
			hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "dotnet test {filter}", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1, Filter: "Category!=Slow", SharedBuild: true, NoCarry: true}
				c.Tiers["tier1"] = Tier{Command: "x", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1}
				c.Build = BuildStep{Command: "dotnet build", Dir: ".", TimeoutMin: 10}
				c.InertPaths = []string{"*.md", "docs/*"}
			}), "",
		},
		{"filter on a command with no {filter}", baseKeys + `tiers: {tier0: {command: x, filter: "A", timeout_min: 5, counter: vstest}}`, Config{}, `gate config key "tiers.tier0.filter": set, but the command holds no {filter}; want {filter} in the command or no filter` + fixIt},
		{"build without command", baseKeys + "build: {dir: x, timeout_min: 5}\n", Config{}, `gate config key "build.command": missing; want the shell command the build runs` + fixIt},
		{"build without timeout_min", baseKeys + "build: {command: b}\n", Config{}, `gate config key "build.timeout_min": missing; want at least 1` + fixIt},
		{"build with timeout_min 0", baseKeys + "build: {command: b, timeout_min: 0}\n", Config{}, `gate config key "build.timeout_min": 0; want at least 1` + fixIt},
		{"shared_build with no build step", baseKeys + `tiers: {tier0: {command: x, shared_build: true, timeout_min: 5, counter: vstest}}`, Config{}, `gate config key "tiers.tier0.shared_build": true, but the hub declares no build step; want a build step or shared_build false` + fixIt},
		{"refuse_raw entry with a double space", baseKeys + `refuse_raw: ["go  test"]`, Config{}, `gate config key "refuse_raw": "go  test"; want one or more words of letters, digits, '.', '_' and '-' separated by single spaces` + fixIt},
		{"covers a negative tier", baseKeys + `tiers: {tier1: {command: x, timeout_min: 5, counter: vstest, covers: [-1]}}`, Config{}, `gate config key "tiers.tier1.covers": -1; want tier numbers lower than 1` + fixIt},
		{"refuse_raw entry with a shell character", baseKeys + `refuse_raw: ["go;test"]`, Config{}, `gate config key "refuse_raw": "go;test"; want one or more words of letters, digits, '.', '_' and '-' separated by single spaces` + fixIt},
		{"absent busy_quit_sec takes the template value", "slots: 2\n", hubWith(func(*Config) {}), ""},
		{"negative busy_quit_sec", "slots: 2\nbusy_quit_sec: -1\n", Config{}, `gate config key "busy_quit_sec": -1; want at least 0; fix it with "lyx config gate" from the prime` + roleClause},
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
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want none", err)
			}
			if diff := cmp.Diff(tc.want, got, equateEmpty); diff != "" {
				t.Fatalf("LoadConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	for _, key := range []string{"slots", "busy_quit_sec"} {
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
		// wantMissing lists text a failure must not hold.
		wantMissing func(hub, machine string) []string
		wantAbsent  bool
		// local is the anchor's local override file contents; "" leaves it absent.
		local string
		// anchorWithoutLyx tells an anchor with no _lyx/, so the load never stacks the local file.
		anchorWithoutLyx bool
		// wantLocalClause requires the failure to name the local file for removal, ahead of the role clause.
		wantLocalClause bool
	}{
		{
			name: "a local file the load refuses is named for removal ahead of the role clause", hub: hubTier0, machine: machineBadParallel, local: "slots: 9\n",
			wantInvalid: true, wantLocalClause: true,
		},
		{
			name: "a local file under an anchor with no _lyx is left out of the way forward", hub: hubTier0, machine: machineBadParallel, local: "slots: 9\n", anchorWithoutLyx: true,
			wantContains: func(hub, machine string) []string { return []string{badParallelKey} },
			wantMissing:  func(hub, machine string) []string { return []string{"remove", "takes no gate key"} },
		},
		{
			name: "a comment-only local file is left out of the way forward", hub: hubTier0, machine: machineBadParallel, local: "# nothing set\n",
			wantContains: func(hub, machine string) []string { return []string{badParallelKey} },
			wantMissing:  func(hub, machine string) []string { return []string{"takes no gate key"} },
		},
		{
			name: "a machine go_parallel is dropped as retired", hub: baseKeys, machine: "go_parallel: 8\n",
			want: hubWith(func(*Config) {}),
		},
		{
			name: "a machine tier_parallel entry beats the tier's own parallel", hub: baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, parallel: 4}}`, machine: "tier_parallel:\n  tier0: 2\n",
			want: hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "x", Dir: ".", Parallel: 2, TimeoutMin: 5, Counter: "vstest", Slots: 1}
				c.TierParallel["tier0"] = 2
			}),
		},
		{
			name: "a machine tier_parallel entry written as the string a scalar set writes", hub: baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest}}`, machine: "tier_parallel:\n  tier0: \"3\"\n",
			want: hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "x", Dir: ".", Parallel: 3, TimeoutMin: 5, Counter: "vstest", Slots: 1}
				c.TierParallel["tier0"] = 3
			}),
		},
		{
			name: "a machine tier_parallel entry for an undeclared tier loads and is dropped", hub: baseKeys, machine: "tier_parallel:\n  tier7: 2\n",
			want: hubWith(func(*Config) {}),
		},
		{
			name: "a machine tier_parallel entry below 1 names both files and both ways forward", hub: baseKeys + `tiers: {tier0: {command: x, timeout_min: 5, counter: vstest}}`, machine: "tier_parallel:\n  tier0: 0\n",
			wantErr: func(hub, machine string) string {
				return `gate config key "tier_parallel.tier0": "0"; want an integer of at least 1` + twoFileWayForward(hub, machine)
			},
		},
		{
			name: "a machine tier_parallel entry that is no integer names both files", hub: baseKeys, machine: "tier_parallel:\n  tier0: many\n",
			wantErr: func(hub, machine string) string {
				return `gate config key "tier_parallel.tier0": "many"; want an integer of at least 1` + twoFileWayForward(hub, machine)
			},
		},
		{
			name: "a machine busy_quit_sec is refused as a cross-pair key", hub: "slots: 2\n", machine: "busy_quit_sec: 9\n",
			wantInvalid: true,
			wantErr: func(hub, machine string) string {
				return "machine config file " + machine + `: key "busy_quit_sec" is cross-pair and cannot be set in a machine override; way forward: remove it from the machine override and set it in the hub file through "lyx config gate" from the prime` + twoFileWayForward(hub, machine)
			},
		},
		{
			name: "a machine cli_wait_sec is refused as retired", hub: "slots: 2\n", machine: "cli_wait_sec: 180\n",
			wantInvalid:  true,
			wantContains: func(hub, machine string) []string { return []string{machine, `"cli_wait_sec"`, "retired"} },
		},
		{
			name: "a board-only load never stacks the machine file", hub: hubTier0, machine: machineBadParallel, boardOnly: true,
			want: hubWith(func(c *Config) {
				c.Tiers["tier0"] = Tier{Command: "x", Dir: ".", Parallel: 1, TimeoutMin: 5, Counter: "vstest", Slots: 1}
			}),
		},
		{
			name: "a machine file beside an absent hub file is still absent", machine: "tier_parallel:\n  tier0: 2\n",
			wantAbsent: true,
		},
	}
	for _, tc := range machineTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := seedGateConfig(t, tc.hub), seedGateConfig(t, "")
			if tc.anchorWithoutLyx {
				anchor = t.TempDir()
			}
			machinePath := configengine.MachineFile(board, "gate")
			if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(machinePath, []byte(tc.machine), 0o644); err != nil {
				t.Fatal(err)
			}
			localPath := configengine.LocalFile(anchor, "gate")
			if tc.local != "" {
				if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(localPath, []byte(tc.local), 0o644); err != nil {
					t.Fatal(err)
				}
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
			case tc.wantErr != nil || tc.wantContains != nil || tc.wantLocalClause:
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
				if tc.wantMissing != nil {
					for _, unwanted := range tc.wantMissing(hubPath, machinePath) {
						if strings.Contains(err.Error(), unwanted) {
							t.Errorf("LoadConfig() error = %q; want it without %q", err.Error(), unwanted)
						}
					}
				}
				if clause := "; remove " + localPath + ", which takes no gate key" + roleClause; tc.wantLocalClause && !strings.Contains(err.Error(), clause) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), clause)
				}
				if tc.wantInvalid && !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("LoadConfig() error = %v; want ErrInvalid", err)
				}
			default:
				if err != nil {
					t.Fatalf("LoadConfig() error = %v; want none", err)
				}
				if diff := cmp.Diff(tc.want, got, equateEmpty); diff != "" {
					t.Fatalf("LoadConfig() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

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
	if err != nil {
		t.Fatalf("TemplateConfig() error = %v; want none", err)
	}
	if diff := cmp.Diff(templateDefaults, cfg, equateEmpty); diff != "" {
		t.Fatalf("TemplateConfig() mismatch (-want +got):\n%s", diff)
	}
	want := Limits{Slots: 2, BusyQuit: 120 * time.Second}
	if got := cfg.Limits(); got != want {
		t.Errorf("Limits() = %+v; want %+v", got, want)
	}

	board, anchor := seedGateConfig(t, baseKeys+`tiers: {tier0: {command: x, timeout_min: 5, counter: vstest, parallel: 4}}`), seedGateConfig(t, "")
	machinePath := configengine.MachineFile(board, "gate")
	if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(machinePath, []byte("tier_parallel:\n  tier0: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	machineCfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v; want none", err)
	}
	if got := machineCfg.Limits().Tier0Parallel; got != 2 {
		t.Errorf("Limits().Tier0Parallel = %d; want the machine override 2", got)
	}
}

// TestLoadConfig_StandaloneAbsentNamesTheWorktreeWrite pins the way forward of a standalone load with no worktree gate.yaml.
func TestLoadConfig_StandaloneAbsentNamesTheWorktreeWrite(t *testing.T) {
	t.Parallel()

	_, err := LoadConfig(configengine.Dirs{Anchor: seedGateConfig(t, "")})
	if !errors.Is(err, ErrConfigAbsent) {
		t.Fatalf("LoadConfig() error = %v; want ErrConfigAbsent", err)
	}
	for _, want := range []string{`run "lyx config gate"`, "_lyx directory", "creating"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadConfig() error = %q; want it to contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "lyx fabric reconcile") {
		t.Errorf("LoadConfig() error = %q; want it without lyx fabric reconcile, which only a hub has", err)
	}
}

// The logger's output is process-global, so this test does not run in parallel.
func TestLoadConfig_DropsTierParallelOfUndeclaredTier(t *testing.T) {
	const machineStep = `lyx config gate --layer machine`
	tests := []struct {
		name string
		// machine is the machine file's contents; "" loads a standalone worktree whose own file holds the entry.
		machine string
		// wantMachineStep tells whether the warning names the machine-layer editor session.
		wantMachineStep bool
	}{
		{"hub with a machine layer names both steps", "tier_parallel:\n  tier7: 2\n", true},
		{"standalone worktree names plain lyx config gate alone", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			var dirs configengine.Dirs
			if tc.machine == "" {
				dirs = configengine.Dirs{Anchor: seedGateConfig(t, baseKeys+"tier_parallel:\n  tier7: 2\n")}
			} else {
				board, anchor := seedGateConfig(t, baseKeys), seedGateConfig(t, "")
				machinePath := configengine.MachineFile(board, "gate")
				if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(machinePath, []byte(tc.machine), 0o644); err != nil {
					t.Fatal(err)
				}
				dirs = configengine.Dirs{Anchor: anchor, Board: board}
			}

			cfg, err := LoadConfig(dirs)
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want the load to succeed", err)
			}
			if len(cfg.TierParallel) != 0 {
				t.Errorf("LoadConfig() TierParallel = %v; want the undeclared tier's entry dropped", cfg.TierParallel)
			}
			warning := logs.String()
			if !strings.Contains(warning, "tier7") || !strings.Contains(warning, "lyx config gate") {
				t.Errorf("warning = %q; want it to name tier7 and plain lyx config gate", warning)
			}
			if got := strings.Contains(warning, machineStep); got != tc.wantMachineStep {
				t.Errorf("warning = %q; names %q = %v, want %v", warning, machineStep, got, tc.wantMachineStep)
			}
		})
	}
}

func TestConfig_Inert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		patterns  []string
		paths     []string
		wantInert []bool
		wantAll   bool
	}{
		{"a base-name pattern matches at any depth", []string{"*.md"}, []string{"docs/a.md", "a.md", "a.cs"}, []bool{true, true, false}, false},
		{"a slash pattern matches the whole path", []string{"docs/*"}, []string{"docs/a.md", "src/docs/a.md"}, []bool{true, false}, false},
		{"a malformed pattern matches nothing", []string{"[", "*.md"}, []string{"a.cs", "a.md"}, []bool{false, true}, false},
		{"every path inert", []string{"*.md", "docs/*"}, []string{"a.md", "docs/b.png"}, []bool{true, true}, true},
		{"one non-inert path among inert ones", []string{"*.md"}, []string{"a.md", "b.cs"}, []bool{true, false}, false},
		{"an empty list is never all inert", nil, []string{"a.md"}, []bool{false}, false},
		{"an empty path set is never all inert", []string{"*.md"}, nil, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{InertPaths: tc.patterns}
			for i, rel := range tc.paths {
				if got := cfg.Inert(rel); got != tc.wantInert[i] {
					t.Errorf("Inert(%q) = %v; want %v", rel, got, tc.wantInert[i])
				}
			}
			if got := cfg.AllInert(tc.paths); got != tc.wantAll {
				t.Errorf("AllInert(%q) = %v; want %v", tc.paths, got, tc.wantAll)
			}
		})
	}
}

func TestMigrateConfig(t *testing.T) {
	t.Parallel()

	withPublish := "gates:\n  card: {tiers: [0], scope: card}\n  webster-fix: {tiers: [0, 1], scope: diff}\n  publish: [2]\nscoper: none\n"
	withoutPublish := "gates:\n  card: {tiers: [0], scope: card}\n  webster-fix: {tiers: [0, 1], scope: diff}\nscoper: none\n"
	retiredLists := "gates:\n  # the lists\n  card: [0]\n  batch: [0, 1]\n  round: [0, 1]\n  final: [0, 1, 2]\n  landing: [0, 1]\nscoper: none\n"
	migratedLists := "gates:\n  # the lists\n  card: {tiers: [0], scope: card}\n  batch: {tiers: [0, 1], scope: card}\n  webster-fix: {tiers: [0, 1], scope: diff}\n  webster: {tiers: [0, 1, 2], scope: all}\n" +
		"  parent-align: {tiers: [0, 1], scope: all}\n  finalize: {tiers: [0, 1], scope: all}\n  darn-write: {tiers: [0, 1], scope: diff}\n  code-burler-fix: {tiers: [0, 1], scope: diff}\nscoper: none\n"

	tests := []struct {
		name      string
		input     string
		want      string
		wantLines []string
		wantErr   bool
	}{
		{"a publish list pruned and the round list kept", withPublish, withoutPublish, []string{"gates.publish: retired, removed"}, false},
		{"an empty publish list pruned", "gates:\n  card: {tiers: [0], scope: card}\n  publish: []\n", "gates:\n  card: {tiers: [0], scope: card}\n", []string{"gates.publish: retired, removed"}, false},
		{
			"the retired lists migrate to keys with scopes", retiredLists, migratedLists,
			[]string{
				"gates.card: list shape retired, migrated to tiers and scope card",
				"gates.batch: list shape retired, migrated to tiers and scope card",
				"gates.round: retired, migrated to webster-fix",
				"gates.final: retired, migrated to webster",
				"gates.landing: retired, migrated to parent-align, finalize, darn-write and code-burler-fix",
			},
			false,
		},
		{
			"a migrated key the file already holds keeps its value", "gates:\n  landing: [0, 1]\n  parent-align: {tiers: [0], scope: diff}\n",
			"gates:\n  finalize: {tiers: [0, 1], scope: all}\n  darn-write: {tiers: [0, 1], scope: diff}\n  code-burler-fix: {tiers: [0, 1], scope: diff}\n  parent-align: {tiers: [0], scope: diff}\n",
			[]string{"gates.landing: retired, migrated to finalize, darn-write and code-burler-fix"}, false,
		},
		{
			"a merge-in key is renamed to parent-align with its value", "gates:\n  card: {tiers: [0], scope: card}\n  merge-in: {tiers: [0, 1], scope: all}\n",
			"gates:\n  card: {tiers: [0], scope: card}\n  parent-align: {tiers: [0, 1], scope: all}\n",
			[]string{"gates.merge-in: renamed to gates.parent-align"}, false,
		},
		{
			"a merge-in key beside parent-align is dropped", "gates:\n  merge-in: {tiers: [0, 1], scope: all}\n  parent-align: {tiers: [0], scope: diff}\n",
			"gates:\n  parent-align: {tiers: [0], scope: diff}\n",
			[]string{"gates.merge-in: retired, removed"}, false,
		},
		{
			"a hub file with the retired lists and guard_tier reconciles to the keys with no guard_tier", "gates:\n  # the lists\n  card: [0]\n  batch: [0, 1]\n  round: [0, 1]\n  final: [0, 1, 2]\n  landing: [0, 1]\nscoper: go\nguard_tier: 1\nrefuse_raw: []\n",
			strings.Replace(migratedLists, "scoper: none\n", "scoper: go\nrefuse_raw: []\n", 1),
			[]string{
				"gates.card: list shape retired, migrated to tiers and scope card",
				"gates.batch: list shape retired, migrated to tiers and scope card",
				"gates.round: retired, migrated to webster-fix",
				"gates.final: retired, migrated to webster",
				"gates.landing: retired, migrated to parent-align, finalize, darn-write and code-burler-fix",
				"guard_tier: retired, removed",
			},
			false,
		},
		{"a guard_tier alone is pruned", "scoper: go\nguard_tier: 0\n", "scoper: go\n", []string{"guard_tier: retired, removed"}, false},
		{"a document without publish unchanged", withoutPublish, withoutPublish, nil, false},
		{"an empty document unchanged", "", "", nil, false},
		{"an unparseable document", "gates: [", "", nil, true},
		{"a root that is not a mapping", "- card\n", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.input))
			if tt.wantErr {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("MigrateConfig(%q) error = %v; want one marked configengine.ErrInvalid", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("MigrateConfig(%q) error = %v; want none", tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("MigrateConfig(%q) = %q; want %q", tt.input, got, tt.want)
			}
			if diff := cmp.Diff(tt.wantLines, lines); diff != "" {
				t.Errorf("MigrateConfig(%q) lines mismatch (-want +got):\n%s", tt.input, diff)
			}
			again, againLines, err := MigrateConfig(got)
			if err != nil || string(again) != string(got) || len(againLines) != 0 {
				t.Errorf("second MigrateConfig = (%q, %v, %v); want the document unchanged with no lines", again, againLines, err)
			}
		})
	}
}

func TestConfig_TierUse(t *testing.T) {
	t.Parallel()

	twoTier := Config{
		Tiers: map[string]Tier{"tier0": {About: "unit tests"}, "tier1": {About: "integration tests"}},
		Gates: map[string]GateKey{
			"card": {Tiers: []int{0}, Scope: "card"}, "batch": {Tiers: []int{0, 1}, Scope: "card"},
			"webster-fix": {Tiers: []int{0, 1}, Scope: "diff"}, "webster": {Tiers: []int{0, 1}, Scope: "all"}, "finalize": {Tiers: []int{0, 1}, Scope: "all"},
			"push": {Tiers: []int{0, 1}, Scope: "diff"}, "dangling": {Tiers: []int{0, 2}, Scope: "diff"}, "empty": {Scope: "diff"},
		},
	}

	t.Run("Gate", func(t *testing.T) {
		t.Parallel()
		got, err := twoTier.Gate("push")
		if err != nil || !slices.Equal(got.Tiers, []int{0, 1}) || got.Scope != "diff" {
			t.Errorf("Gate(push) = (%+v, %v); want tiers [0 1] and scope diff", got, err)
		}
		if _, err := twoTier.Gate("nope"); err == nil || !strings.Contains(err.Error(), `gates holds no key "nope"`) {
			t.Errorf("Gate(nope) error = %v; want the no-such-key refusal", err)
		}
	})

	t.Run("TierList", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			cfg     Config
			gate    string
			want    []int
			wantErr string
		}{
			{"a declared list", twoTier, "batch", []int{0, 1}, ""},
			{"a gate the config holds no list for", twoTier, "nope", nil, `gates holds no key "nope"`},
			{"a list naming an undeclared tier", twoTier, "dangling", nil, "gates.dangling lists tier2, which the hub does not declare"},
			{"an empty list for any gate", twoTier, "empty", nil, "gates.empty is empty, and every gate verifies at least one tier"},
			{"an empty list for a gate lyx reads", Config{Tiers: twoTier.Tiers, Gates: map[string]GateKey{"webster-fix": {Scope: "diff"}}}, "webster-fix", nil, "gates.webster-fix is empty, and every gate verifies at least one tier"},
			{"a hub with no tier0", Config{Tiers: map[string]Tier{"tier1": {}}, Gates: twoTier.Gates}, "batch", nil, "the hub declares no tier0"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got, err := tt.cfg.TierList(tt.gate)
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.HasSuffix(err.Error(), `; fix it with "lyx config gate" from the prime`+roleClause) {
						t.Fatalf("TierList(%q) error = %v; want it to contain %q and end with the role clause", tt.gate, err, tt.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("TierList(%q) error = %v; want none", tt.gate, err)
				}
				if diff := cmp.Diff(tt.want, got, equateEmpty); diff != "" {
					t.Errorf("TierList(%q) mismatch (-want +got):\n%s", tt.gate, diff)
				}
			})
		}
	})

	t.Run("Tier", func(t *testing.T) {
		t.Parallel()
		if got, err := twoTier.Tier(1); err != nil || got.About != "integration tests" {
			t.Errorf("Tier(1) = (%+v, %v); want the declared tier", got, err)
		}
		_, err := twoTier.Tier(5)
		if err == nil || !strings.Contains(err.Error(), "tier5") || !strings.HasSuffix(err.Error(), `; fix it with "lyx config gate" from the prime`+roleClause) {
			t.Errorf("Tier(5) error = %v; want a refusal naming tier5 and ending with the role clause", err)
		}
	})

	t.Run("GateCall", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			gate  string
			cards []string
			want  string
		}{
			{"batch", []string{"01", "02"}, "lyx gate test --wait --gate batch --card 01 --card 02"},
			{"card", []string{"03"}, "lyx gate test --wait --gate card --card 03"},
			{"webster", nil, "lyx gate test --wait --gate webster"},
		}
		for _, tt := range tests {
			got, err := twoTier.GateCall(tt.gate, tt.cards...)
			if err != nil || got != tt.want {
				t.Errorf("GateCall(%q, %v) = (%q, %v); want (%q, nil)", tt.gate, tt.cards, got, err, tt.want)
			}
		}
		if _, err := twoTier.GateCall("nope"); err == nil {
			t.Error("GateCall(nope) error = nil; want the Gate refusal")
		}
	})

	t.Run("TiersValue", func(t *testing.T) {
		t.Parallel()
		want := "- tier0: unit tests\n- tier1: integration tests\n" +
			"- gate card: [0] card\n- gate batch: [0, 1] card\n- gate webster: [0, 1] all\n- gate finalize: [0, 1] all\n- gate webster-fix: [0, 1] diff"
		if got := twoTier.TiersValue(); got != want {
			t.Errorf("TiersValue() = %q; want %q", got, want)
		}
		template, err := TemplateConfig()
		if err != nil {
			t.Fatal(err)
		}
		if got := template.TiersValue(); got != "none" {
			t.Errorf("template TiersValue() = %q; want none, since the template declares no tier", got)
		}
	})
}
