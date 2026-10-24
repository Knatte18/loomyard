//go:build tier0

// layers_test.go — tests for the layer stack: the layered loaders, the local-layer rule, the layer reader and the unexpanded stack merge.

package configengine_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const (
	layerModule   = "mod"
	layerTemplate = "name: tpl\nport: 80\nnested:\n  deep: 1\nlist: [a, b]\nlabels:\n  bug: a bug\nrun: echo\n"
	// fullDefault holds every template key, so a load over it fills nothing.
	fullDefault = "name: d\nport: 1\nnested:\n  deep: 2\nlist: [x, y]\nlabels:\n  a: A\n  b: B\nrun: sh\n"
	// perNameTail is the per-name map a row with perNameMaps appends to the template.
	perNameTail = "lenses:\n  a: A\n"
)

// layerSpec is a per-worktree spec with one open map and a classification of some keys.
func layerSpec() configengine.Spec {
	return configengine.Spec{
		Name:     layerModule,
		Template: []byte(layerTemplate),
		OpenMaps: []string{"labels"},
		Categories: map[string]configengine.Category{
			"list":        configengine.CategoryGuard,
			"run":         configengine.CategoryExecutable,
			"nested":      configengine.CategoryCrossPair,
			"nested.deep": configengine.CategoryNone,
		},
	}
}

// overlaySpec is layerSpec with a selector key, pick, over a per-name map, lenses, and one overlay document, x, that replaces lens a and adds lens b.
// pick and lenses are guard keys.
func overlaySpec() configengine.Spec {
	spec := layerSpec()
	spec.Template = []byte(layerTemplate + "pick: \"\"\n" + perNameTail)
	spec.PerNameMaps = []string{"lenses"}
	spec.Categories["pick"] = configengine.CategoryGuard
	spec.Categories["lenses"] = configengine.CategoryGuard
	spec.Overlay = configengine.Overlay{
		Selector:  "pick",
		Documents: map[string][]byte{"x": []byte("lenses:\n  a: X\n  b: B\n")},
	}
	return spec
}

// machineSpec is a hub-wide spec classifying port as the machine key, name as free and run as executable.
func machineSpec() configengine.Spec {
	spec := layerSpec()
	spec.HubWide = true
	spec.Categories = map[string]configengine.Category{
		"port": configengine.CategoryMachine,
		"name": configengine.CategoryNone,
		"run":  configengine.CategoryExecutable,
	}
	return spec
}

// geometry is how a case lays out its anchor and board dir.
type geometry int

const (
	// geometryHub tells both dirs, each with its _lyx/config/.
	geometryHub geometry = iota
	// geometryStandalone tells the anchor alone.
	geometryStandalone
	// geometryAnchorWithoutLyx tells an initialised board dir and an anchor with no _lyx/.
	geometryAnchorWithoutLyx
	// geometryBoardOnly tells an initialised board dir and no anchor.
	geometryBoardOnly
)

// layerFixture is a temp anchor and board dir laid out by a geometry, with the files each layer holds.
type layerFixture struct {
	dirs   configengine.Dirs
	anchor string
	board  string
}

// writeLayerFile writes content at path, creating its directories.
func writeLayerFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newLayerFixture lays out the geometry and writes each layer's file; a layer absent from files gets none.
func newLayerFixture(t *testing.T, geo geometry, files map[configengine.Layer]string) layerFixture {
	t.Helper()
	fx := layerFixture{anchor: t.TempDir(), board: t.TempDir()}
	switch geo {
	case geometryHub:
		fx.dirs = configengine.Dirs{Anchor: fx.anchor, Board: fx.board}
		mustMkdirAll(t, configengine.ConfigDir(fx.anchor))
		mustMkdirAll(t, configengine.ConfigDir(fx.board))
	case geometryStandalone:
		fx.dirs = configengine.Dirs{Anchor: fx.anchor}
		mustMkdirAll(t, configengine.ConfigDir(fx.anchor))
	case geometryAnchorWithoutLyx:
		fx.dirs = configengine.Dirs{Anchor: fx.anchor, Board: fx.board}
		mustMkdirAll(t, configengine.ConfigDir(fx.board))
	case geometryBoardOnly:
		fx.dirs = configengine.Dirs{Board: fx.board}
		mustMkdirAll(t, configengine.ConfigDir(fx.board))
	}
	paths := map[configengine.Layer]string{
		configengine.LayerDefault:  configengine.ConfigFile(fx.board, layerModule),
		configengine.LayerWorktree: configengine.ConfigFile(fx.anchor, layerModule),
		configengine.LayerMachine:  configengine.MachineFile(fx.board, layerModule),
		configengine.LayerLocal:    configengine.LocalFile(fx.anchor, layerModule),
	}
	for layer, content := range files {
		writeLayerFile(t, paths[layer], content)
	}
	return fx
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// decodeTopLevel returns each top-level key of a resolved document as its printed value.
func decodeTopLevel(t *testing.T, resolved []byte) map[string]string {
	t.Helper()
	var raw map[string]any
	if err := yaml.Unmarshal(resolved, &raw); err != nil {
		t.Fatalf("unmarshal resolved: %v\n%s", err, resolved)
	}
	got := make(map[string]string, len(raw))
	for key, value := range raw {
		got[key] = fmt.Sprint(value)
	}
	return got
}

// TestLoad_LayerStack pins how the two loaders resolve a module across its layers: template fallback, the highest layer winning key by key, whole replacement of lists, scalars and open maps, each geometry, strict and degrading absent bases, empty upper layers, the staging copy never read as the local layer, one expansion against the anchor's environment, and which fills are logged.
// It serializes its rows because they swap the process-global logger output.
func TestLoad_LayerStack(t *testing.T) {
	tests := []struct {
		name string
		geo  geometry
		// hubWide marks the spec hub-wide; machine also gives it a machine key, port.
		hubWide bool
		machine bool
		// seedOnly marks the spec seed-only, so a hub base stays full and logs its fill.
		seedOnly bool
		files    map[configengine.Layer]string
		staging  string
		// anchorEnv and boardEnv are the .env files of the anchor and the board dir.
		anchorEnv, boardEnv string
		// strictErr is the text the strict loader's error holds, when it refuses; the degrading loader still resolves.
		strictErr string
		// wantFill is the keys of the one template fill line; empty means no fill line.
		wantFill string
		// retired is the spec's retired keys, and wantAbsent the top-level keys the resolved document must lack.
		retired    map[string]configengine.Category
		wantAbsent []string
		// wantUnknown maps each layer whose file holds keys the template lacks to the keys of its one warning, over both loaders; empty means no warning.
		wantUnknown map[configengine.Layer]string
		// perNameMaps is the spec's per-name maps; a row that sets it loads over the template with perNameTail appended.
		perNameMaps []string
		// overlay swaps in overlaySpec, which keeps its own template and per-name maps whatever perNameMaps holds.
		overlay bool
		// refusal is the substrings both loaders must refuse with, marked ErrInvalid, and refusalLacks those the refusal must not hold; a refusal row asserts nothing else.
		refusal      []string
		refusalLacks []string
		want         map[string]string
	}{
		{
			name:    "a hub default picking a layer resolves its names over the base",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "pick: x\n"},
			overlay: true,
			want:    map[string]string{"lenses": "map[a:X b:B]"},
		},
		{
			name:     "a standalone worktree file picking a layer resolves its names and fills the one missing map",
			geo:      geometryStandalone,
			files:    map[configengine.Layer]string{configengine.LayerWorktree: fullDefault + "pick: x\n"},
			overlay:  true,
			wantFill: "lenses",
			want:     map[string]string{"lenses": "map[a:X b:B]"},
		},
		{
			name:    "a name the file holds beats the picked layer",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "pick: x\nlenses:\n  a: mine\n"},
			overlay: true,
			want:    map[string]string{"lenses": "map[a:mine b:B]"},
		},
		{
			name: "a worktree name beats the picked layer and the layer fills the rest",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "pick: x\n",
				configengine.LayerWorktree: "lenses:\n  b: wt\n",
			},
			overlay: true,
			want:    map[string]string{"lenses": "map[a:X b:wt]"},
		},
		{
			name:    "a default picking no layer resolves the base",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "name: d\n"},
			overlay: true,
			want:    map[string]string{"lenses": "map[a:A]"},
		},
		{
			name: "a hub worktree file holding the base's value loads",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "name: d\n",
				configengine.LayerWorktree: "pick: \"\"\n",
			},
			overlay: true,
			want:    map[string]string{"lenses": "map[a:A]"},
		},
		{
			name: "a hub worktree file picking a layer the base does not is refused",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "name: d\n",
				configengine.LayerWorktree: "pick: x\n",
			},
			overlay: true,
			refusal: []string{"mod.yaml holds pick, which only the base file", "delete the pick line from", "--layer default", "the orch runs it"},
		},
		{
			name:    "a hub default picking an unknown layer is refused naming the shipped ones",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "pick: nope\n"},
			overlay: true,
			refusal: []string{"names no shipped layer (shipped: x)", "--layer default --set pick=<name>", "the orch runs it"},
		},
		{
			name:         "a standalone worktree file picking an unknown layer is refused with a way forward that needs no prime",
			geo:          geometryStandalone,
			files:        map[configengine.Layer]string{configengine.LayerWorktree: "pick: nope\n"},
			overlay:      true,
			refusal:      []string{`"lyx config mod --set pick=<name>"`},
			refusalLacks: []string{"the orch runs it"},
		},
		{
			name:    "a hub default holding a list for the selector is refused",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "pick: [x]\n"},
			overlay: true,
			refusal: []string{"pick", "--layer default --set pick=<name>"},
		},
		{
			name: "a hub worktree file holding a list for the selector is refused",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "name: d\n",
				configengine.LayerWorktree: "pick: [x]\n",
			},
			overlay: true,
			refusal: []string{"mod.yaml holds pick", "delete the pick line from"},
		},
		{
			name:    "a machine value wins over the hub file",
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "d", "port": "8"},
		},
		{
			name:    "a machine value survives an anchor holding no _lyx",
			geo:     geometryAnchorWithoutLyx,
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "d", "port": "8"},
		},
		{
			name:    "a hub-file check telling the board dir alone stacks no machine layer",
			geo:     geometryBoardOnly,
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "d", "port": "1"},
		},
		{
			name:    "a standalone load never reads a machine layer",
			geo:     geometryStandalone,
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerWorktree: fullDefault, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "d", "port": "1"},
		},
		{
			name:    "a hub-wide spec with no machine key ignores a machine file",
			hubWide: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "name: m\nport: 8\n"},
			want:    map[string]string{"name": "d", "port": "1"},
		},
		{
			name:    "a comment-only machine file adds nothing",
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "# nothing\n"},
			want:    map[string]string{"name": "d", "port": "1"},
		},
		{
			name:       "a retired machine key in the machine file resolves as absent",
			machine:    true,
			files:      map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerMachine: "gone_machine: 1\n"},
			retired:    map[string]configengine.Category{"gone_machine": configengine.CategoryMachine},
			wantAbsent: []string{"gone_machine"},
			want:       map[string]string{"name": "d", "port": "1"},
		},
		{
			name: "a name the template lacks under a per-name map never warns while a key outside it still does",
			files: map[configengine.Layer]string{
				configengine.LayerDefault: fullDefault + perNameTail + "  extra: E\nold_name: d\n",
			},
			perNameMaps: []string{"lenses"},
			wantUnknown: map[configengine.Layer]string{configengine.LayerDefault: "old_name"},
			want:        map[string]string{"name": "d", "lenses": "map[a:A extra:E]"},
		},
		{
			name: "keys the template lacks warn once per file over both loaders, naming the reconcile that removes them",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault + "old_name: d\nold_map:\n  a: 1\n",
				configengine.LayerWorktree: "port: 5\nold_port: 6\n",
			},
			wantUnknown: map[configengine.Layer]string{
				configengine.LayerDefault:  "old_map.a,old_name",
				configengine.LayerWorktree: "old_port",
			},
			want: map[string]string{"name": "d", "port": "5"},
		},
		{
			name: "retired keys of the worktree and local layers resolve as absent",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault,
				configengine.LayerWorktree: "gone: 1\nport: 5\n",
				configengine.LayerLocal:    "gone_map:\n  a: 1\nname: l\n",
			},
			retired:    map[string]configengine.Category{"gone": configengine.CategoryNone, "gone_map": configengine.CategoryNone},
			wantAbsent: []string{"gone", "gone_map"},
			want:       map[string]string{"name": "l", "port": "5", "nested": "map[deep:2]", "list": "[x y]", "run": "sh"},
		},
		{
			name: "a dotted retired key drops only its own entry",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault,
				configengine.LayerWorktree: "nested:\n  deep: 3\n  gone: 1\n",
			},
			retired: map[string]configengine.Category{"nested.gone": configengine.CategoryNone},
			want:    map[string]string{"nested": "map[deep:3]"},
		},
		{
			name:  "template fills every depth the layers lack and a hub's sparse base logs nothing",
			files: map[configengine.Layer]string{configengine.LayerDefault: "name: d\n"},
			want:  map[string]string{"name": "d", "port": "80", "nested": "map[deep:1]", "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"},
		},
		{
			name:     "a seed-only hub base keeps its fill line",
			seedOnly: true,
			files:    map[configengine.Layer]string{configengine.LayerDefault: "name: d\n"},
			wantFill: "labels,list,nested,port,run",
			want:     map[string]string{"name": "d", "port": "80", "nested": "map[deep:1]", "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"},
		},
		{
			name:     "a standalone worktree base keeps its fill line",
			geo:      geometryStandalone,
			files:    map[configengine.Layer]string{configengine.LayerWorktree: "name: d\n"},
			wantFill: "labels,list,nested,port,run",
			want:     map[string]string{"name": "d", "port": "80", "nested": "map[deep:1]", "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"},
		},
		{
			name:  "a header-only hub base is a present base under both loaders and every value comes from the template",
			files: map[configengine.Layer]string{configengine.LayerDefault: configengine.SparseBaseHeader},
			want:  map[string]string{"name": "tpl", "port": "80", "nested": "map[deep:1]", "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"},
		},
		{
			name: "highest layer wins key by key and an inherited key logs nothing",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault,
				configengine.LayerWorktree: "name: w\nport: 5\n",
				configengine.LayerLocal:    "name: l\n",
			},
			want: map[string]string{"name": "l", "port": "5", "nested": "map[deep:2]", "list": "[x y]", "run": "sh"},
		},
		{
			name: "list and scalar are replaced whole",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault,
				configengine.LayerWorktree: "list: [q]\nport: 7\n",
			},
			want: map[string]string{"list": "[q]", "port": "7"},
		},
		{
			name: "open map comes whole from the highest layer holding it",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  fullDefault,
				configengine.LayerWorktree: "labels:\n  c: C\n",
				configengine.LayerLocal:    "labels:\n  d: D\n",
			},
			want: map[string]string{"labels": "map[d:D]"},
		},
		{
			name: "standalone stacks the worktree file and the local file",
			geo:  geometryStandalone,
			files: map[configengine.Layer]string{
				configengine.LayerWorktree: fullDefault,
				configengine.LayerLocal:    "name: l\n",
			},
			want: map[string]string{"name": "l", "port": "1"},
		},
		{
			name:      "absent base is refused strictly and replaced by the template when degrading",
			files:     map[configengine.Layer]string{configengine.LayerWorktree: "port: 9\n"},
			strictErr: "lyx loom resume",
			want:      map[string]string{"port": "9", "name": "tpl"},
		},
		{
			name:  "absent upper layers add nothing",
			files: map[configengine.Layer]string{configengine.LayerDefault: fullDefault},
			want:  map[string]string{"name": "d"},
		},
		{
			name:  "empty local file adds nothing",
			files: map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerLocal: ""},
			want:  map[string]string{"name": "d"},
		},
		{
			name:  "comment-only local file adds nothing",
			files: map[configengine.Layer]string{configengine.LayerDefault: fullDefault, configengine.LayerLocal: "# nothing\n"},
			want:  map[string]string{"name": "d"},
		},
		{
			name:    "staging copy is never the local layer",
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault},
			staging: "name: staged\n",
			want:    map[string]string{"name": "d"},
		},
		{
			name: "marker inherited from the default expands against the anchor's environment",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  strings.Replace(fullDefault, "name: d", "name: ${env:LAYER_TEST_MARKER}", 1),
				configengine.LayerWorktree: "port: 2\n",
			},
			anchorEnv: "LAYER_TEST_MARKER=from-anchor\n",
			boardEnv:  "LAYER_TEST_MARKER=from-board\n",
			want:      map[string]string{"name": "from-anchor", "port": "2"},
		},
		{
			name:     "template fill is logged with the layer template",
			seedOnly: true,
			files:    map[configengine.Layer]string{configengine.LayerDefault: "name: d\nport: 1\n"},
			wantFill: "labels,list,nested,run",
			want:     map[string]string{"name": "d", "port": "1", "run": "echo"},
		},
		{
			name:    "hub-wide hub file loads with an anchor holding no _lyx",
			geo:     geometryAnchorWithoutLyx,
			hubWide: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: fullDefault},
			want:    map[string]string{"name": "d", "port": "1"},
		},
		{
			name:  "repo default resolves with an anchor holding no _lyx",
			geo:   geometryAnchorWithoutLyx,
			files: map[configengine.Layer]string{configengine.LayerDefault: fullDefault},
			want:  map[string]string{"name": "d", "port": "1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			// The OS environment beats .env, so the marker must be unset; Setenv registers the restore.
			t.Setenv("LAYER_TEST_MARKER", "")
			os.Unsetenv("LAYER_TEST_MARKER")
			fx := newLayerFixture(t, tc.geo, tc.files)
			if tc.staging != "" {
				writeLayerFile(t, configengine.StagingFile(fx.anchor, layerModule), tc.staging)
			}
			if tc.anchorEnv != "" {
				writeLayerFile(t, filepath.Join(fx.anchor, ".env"), tc.anchorEnv)
			}
			if tc.boardEnv != "" {
				writeLayerFile(t, filepath.Join(fx.board, ".env"), tc.boardEnv)
			}
			spec := layerSpec()
			if tc.overlay {
				spec = overlaySpec()
			}
			if tc.machine {
				spec = machineSpec()
			}
			spec.HubWide = tc.hubWide || tc.machine
			spec.SeedOnly = tc.seedOnly
			spec.RetiredKeys = tc.retired
			if !tc.overlay {
				spec.PerNameMaps = tc.perNameMaps
				if len(tc.perNameMaps) > 0 {
					spec.Template = append(spec.Template, perNameTail...)
				}
			}

			if tc.refusal != nil {
				for loader, load := range map[string]func(configengine.Dirs, configengine.Spec) ([]byte, error){"Load": configengine.Load, "LoadOrTemplate": configengine.LoadOrTemplate} {
					_, err := load(fx.dirs, spec)
					if err == nil || !errors.Is(err, configengine.ErrInvalid) {
						t.Fatalf("%s error = %v; want one marked ErrInvalid", loader, err)
					}
					for _, want := range tc.refusal {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("%s error = %q; want it to hold %q", loader, err, want)
						}
					}
					for _, lacks := range tc.refusalLacks {
						if strings.Contains(err.Error(), lacks) {
							t.Errorf("%s error = %q; want it to lack %q", loader, err, lacks)
						}
					}
				}
				return
			}

			var resolved []byte
			if tc.strictErr != "" {
				if _, err := configengine.Load(fx.dirs, spec); err == nil || !strings.Contains(err.Error(), tc.strictErr) {
					t.Fatalf("Load error = %v; want one containing %q", err, tc.strictErr)
				}
				var err error
				if resolved, err = configengine.LoadOrTemplate(fx.dirs, spec); err != nil {
					t.Fatalf("LoadOrTemplate: %v", err)
				}
			} else {
				strict, err := configengine.Load(fx.dirs, spec)
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				log := buf.String()
				if tc.wantFill == "" {
					if strings.Contains(log, "filled missing keys") {
						t.Errorf("unexpected fill line: %s", log)
					}
				} else {
					assertOneFillLine(t, buf, layerModule, tc.wantFill)
					if !strings.Contains(log, "layer=template") {
						t.Errorf("fill line does not name the layer template: %s", log)
					}
				}
				if resolved, err = configengine.LoadOrTemplate(fx.dirs, spec); err != nil {
					t.Fatalf("LoadOrTemplate: %v", err)
				}
				if string(resolved) != string(strict) {
					t.Errorf("LoadOrTemplate = %q; want Load's %q", resolved, strict)
				}
			}

			log := buf.String()
			if n := strings.Count(log, "keys the template lacks"); n != len(tc.wantUnknown) {
				t.Errorf("unknown-keys warnings = %d; want %d\nlog:\n%s", n, len(tc.wantUnknown), log)
			}
			wayForward := map[configengine.Layer]string{
				configengine.LayerDefault:  `lyx config reconcile --layer default --apply\" from the hub's prime worktree`,
				configengine.LayerWorktree: `lyx config reconcile --apply\" in this worktree`,
			}
			for layer, keys := range tc.wantUnknown {
				for _, want := range []string{"layer=" + string(layer) + " ", "keys=" + keys + " ", wayForward[layer]} {
					if !strings.Contains(log, want) {
						t.Errorf("unknown-keys warning for layer %s lacks %q\nlog:\n%s", layer, want, log)
					}
				}
			}

			got := decodeTopLevel(t, resolved)
			for key, want := range tc.want {
				if got[key] != want {
					t.Errorf("%s = %q; want %q\nresolved:\n%s", key, got[key], want, resolved)
				}
			}
			for _, key := range tc.wantAbsent {
				if _, held := got[key]; held {
					t.Errorf("resolved document holds retired key %q\nresolved:\n%s", key, resolved)
				}
			}
		})
	}
}

// TestValidateLocal pins the machine-layer rule, and that the local layer refuses a machine key, besides the local-layer rule: a key the template lacks, a shape mismatch, a category other than none and, for a hub-wide module, an unclassified key are refused naming the key; an open-map entry, a whole-entry alias, a per-name key some template entry holds under any name, the longest covering entry and an empty document pass.
func TestValidateLocal(t *testing.T) {
	t.Parallel()
	hubWide := layerSpec()
	hubWide.HubWide = true
	hubWide.Categories = map[string]configengine.Category{"name": configengine.CategoryNone, "run": configengine.CategoryExecutable}
	models := configengine.Spec{
		Name:         "models",
		Template:     []byte("sonnet: m1\nopus: m2\n"),
		WholeEntries: true,
		Categories:   map[string]configengine.Category{"sonnet": configengine.CategoryNone},
	}

	retired := layerSpec()
	retired.RetiredKeys = map[string]configengine.Category{
		"gone_scalar": configengine.CategoryNone,
		"gone_map":    configengine.CategoryNone,
		"gone_guard":  configengine.CategoryGuard,
		"gone_run":    configengine.CategoryExecutable,
		"old":         configengine.CategoryGuard,
		"old.open":    configengine.CategoryNone,
	}

	machine := machineSpec()
	machine.RetiredKeys = map[string]configengine.Category{
		"gone_machine": configengine.CategoryMachine,
		"gone_guard":   configengine.CategoryGuard,
	}
	const removal = "remove it from the machine override"

	profiles := configengine.Spec{
		Name:        "profiles",
		Template:    []byte("profiles:\n  plain:\n    kind: identity\n  costed:\n    kind: cost\n    budget: 200\n"),
		PerNameMaps: []string{"profiles"},
		Categories:  map[string]configengine.Category{"profiles": configengine.CategoryNone},
	}

	tests := []struct {
		name string
		spec configengine.Spec
		// validator is the rule the row runs, local when empty.
		validator string
		doc       string
		wantErr   string
		// wantMore are further substrings the error must hold.
		wantMore []string
	}{
		{name: "machine key", spec: machine, validator: "machine", doc: "port: 8\n"},
		{name: "machine empty document", spec: machine, validator: "machine", doc: ""},
		{name: "machine refuses a none key and points at the hub file and the local layer", spec: machine, validator: "machine", doc: "name: x\n", wantErr: "name", wantMore: []string{"not a machine key", removal, `"lyx config mod"`, "--layer local"}},
		{name: "machine refuses an executable key and points at the hub file", spec: machine, validator: "machine", doc: "run: evil\n", wantErr: "run", wantMore: []string{`"lyx config mod"`}},
		{name: "machine refuses a key the template lacks", spec: machine, validator: "machine", doc: "nope: 1\n", wantErr: "nope", wantMore: []string{removal}},
		{name: "machine refuses an unclassified key", spec: machine, validator: "machine", doc: "list: [x]\n", wantErr: "list", wantMore: []string{"not classified", removal}},
		{name: "machine refuses a mapping where the template holds a scalar", spec: machine, validator: "machine", doc: "port:\n  a: 1\n", wantErr: "port", wantMore: []string{removal}},
		{name: "machine refuses an unparseable document", spec: machine, validator: "machine", doc: "port: [unclosed\n", wantErr: "parse", wantMore: []string{removal}},
		{name: "machine admits a retired machine key", spec: machine, validator: "machine", doc: "gone_machine: 1\n"},
		{name: "machine refuses a retired guard key", spec: machine, validator: "machine", doc: "gone_guard: 1\n", wantErr: "retired", wantMore: []string{removal}},
		{name: "local still refuses a machine key", spec: machine, doc: "port: 8\n", wantErr: "port"},
		{name: "retired none scalar", spec: retired, doc: "gone_scalar: 1\n"},
		{name: "units under a retired none map", spec: retired, doc: "gone_map:\n  a: 1\n  b: 2\n"},
		{name: "retired none key holding a mapping the template never held", spec: retired, doc: "gone_scalar:\n  a: 1\n"},
		{name: "retired guard key", spec: retired, doc: "gone_guard: x\n", wantErr: "gone_guard"},
		{name: "retired executable key", spec: retired, doc: "gone_run: x\n", wantErr: "gone_run"},
		{name: "longest retired key admits its none entry", spec: retired, doc: "old:\n  open:\n    a: 1\n"},
		{name: "longest retired key refuses the guard remainder", spec: retired, doc: "old:\n  shut: 1\n", wantErr: "old"},
		{name: "empty document", spec: layerSpec(), doc: ""},
		{name: "comment-only document", spec: layerSpec(), doc: "# nothing\n"},
		{name: "unclassified keys of a per-worktree module", spec: layerSpec(), doc: "name: x\nport: 81\n"},
		{name: "open-map entry", spec: layerSpec(), doc: "labels:\n  docs: documentation\n"},
		{name: "longest covering entry is none", spec: layerSpec(), doc: "nested:\n  deep: 5\n"},
		{name: "whole-entry alias", spec: models, doc: "sonnet: other\nmyalias: m3\n"},
		{name: "unknown key", spec: layerSpec(), doc: "nope: 1\n", wantErr: "nope"},
		{name: "unknown nested key", spec: layerSpec(), doc: "nested:\n  other: 1\n", wantErr: "nested.other"},
		{name: "scalar where the template holds a mapping", spec: layerSpec(), doc: "nested: 5\n", wantErr: "nested"},
		{name: "mapping where the template holds a scalar", spec: layerSpec(), doc: "port:\n  a: 1\n", wantErr: "port"},
		{name: "guard category", spec: layerSpec(), doc: "list: [x]\n", wantErr: "list"},
		{name: "executable category", spec: layerSpec(), doc: "run: evil\n", wantErr: "run"},
		{name: "hub-wide classified none", spec: hubWide, doc: "name: x\n"},
		{name: "hub-wide unclassified key", spec: hubWide, doc: "port: 81\n", wantErr: "port"},
		{name: "hub-wide executable key", spec: hubWide, doc: "run: evil\n", wantErr: "run"},
		{name: "unparseable document", spec: layerSpec(), doc: "name: [unclosed\n", wantErr: "parse"},
		{name: "per-name keys are judged against the template entry holding them whatever the name", spec: profiles, doc: "profiles:\n  wide:\n    kind: cost\n    budget: 5\n  plain:\n    budget: 9\n"},
		{name: "per-name key no template entry holds", spec: profiles, doc: "profiles:\n  wide:\n    nope: 1\n", wantErr: "profiles.wide.nope"},
		{name: "per-name key in another shape than the template entry's", spec: profiles, doc: "profiles:\n  wide:\n    budget:\n      a: 1\n", wantErr: "profiles.wide.budget"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validate := configengine.ValidateLocal
			if tc.validator == "machine" {
				validate = configengine.ValidateMachine
			}
			err := validate(tc.spec, []byte(tc.doc))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate error = %v; want one containing %q", err, tc.wantErr)
			}
			for _, more := range tc.wantMore {
				if !strings.Contains(err.Error(), more) {
					t.Errorf("validate error = %v; want it to contain %q", err, more)
				}
			}
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Errorf("error is not marked ErrInvalid: %v", err)
			}
		})
	}
}

// TestLayers pins the layer reader and the stack merge: the present files in stack order per geometry, absent files skipped, a comment-only machine or local file dropped, a refused machine or local file naming itself, and a Doc merged unexpanded with no template key filled and an Origins entry per top-level key.
func TestLayers(t *testing.T) {
	t.Parallel()
	all := map[configengine.Layer]string{
		configengine.LayerDefault:  "name: d\nport: 1\n",
		configengine.LayerWorktree: "name: w\n",
		configengine.LayerLocal:    "name: l\n",
	}
	layerNames := func(layers []configengine.LayerDoc) []configengine.Layer {
		names := make([]configengine.Layer, len(layers))
		for i, layer := range layers {
			names[i] = layer.Layer
		}
		return names
	}

	allFour := map[configengine.Layer]string{
		configengine.LayerDefault:  "name: d\nport: 1\n",
		configengine.LayerWorktree: "name: w\n",
		configengine.LayerMachine:  "port: 8\n",
		configengine.LayerLocal:    "name: l\n",
	}
	orderTests := []struct {
		name    string
		geo     geometry
		hubWide bool
		// machine gives the hub-wide spec the machine key port.
		machine bool
		files   map[configengine.Layer]string
		want    []configengine.Layer
	}{
		{name: "hub-wide machine spec stacks default, machine and local", geo: geometryHub, machine: true, files: allFour, want: []configengine.Layer{configengine.LayerDefault, configengine.LayerMachine, configengine.LayerLocal}},
		{name: "comment-only machine file is dropped", geo: geometryHub, machine: true, files: map[configengine.Layer]string{configengine.LayerDefault: "name: d\n", configengine.LayerMachine: "# nothing\n", configengine.LayerLocal: "name: l\n"}, want: []configengine.Layer{configengine.LayerDefault, configengine.LayerLocal}},
		{name: "hub per-worktree", geo: geometryHub, files: all, want: []configengine.Layer{configengine.LayerDefault, configengine.LayerWorktree, configengine.LayerLocal}},
		{name: "hub hub-wide skips the worktree file", geo: geometryHub, hubWide: true, files: all, want: []configengine.Layer{configengine.LayerDefault, configengine.LayerLocal}},
		{name: "standalone", geo: geometryStandalone, files: map[configengine.Layer]string{configengine.LayerWorktree: "name: w\n", configengine.LayerLocal: "name: l\n"}, want: []configengine.Layer{configengine.LayerWorktree, configengine.LayerLocal}},
		{name: "absent files are skipped", geo: geometryHub, files: map[configengine.Layer]string{configengine.LayerWorktree: "name: w\n"}, want: []configengine.Layer{configengine.LayerWorktree}},
		{name: "comment-only local file is dropped", geo: geometryHub, files: map[configengine.Layer]string{configengine.LayerDefault: "name: d\n", configengine.LayerLocal: "# nothing\n"}, want: []configengine.Layer{configengine.LayerDefault}},
	}
	for _, tc := range orderTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newLayerFixture(t, tc.geo, tc.files)
			spec := layerSpec()
			spec.HubWide = tc.hubWide
			if tc.hubWide {
				spec.Categories = map[string]configengine.Category{"name": configengine.CategoryNone}
			}
			if tc.machine {
				spec = machineSpec()
			}
			layers, err := configengine.Layers(fx.dirs, spec)
			if err != nil {
				t.Fatalf("Layers: %v", err)
			}
			if got := layerNames(layers); !slices.Equal(got, tc.want) {
				t.Errorf("layers = %v; want %v", got, tc.want)
			}
		})
	}

	t.Run("refused local file names the file and the key", func(t *testing.T) {
		t.Parallel()
		fx := newLayerFixture(t, geometryHub, map[configengine.Layer]string{configengine.LayerDefault: "name: d\n", configengine.LayerLocal: "run: evil\n"})
		_, err := configengine.Layers(fx.dirs, layerSpec())
		localPath := configengine.LocalFile(fx.anchor, layerModule)
		if err == nil || !strings.Contains(err.Error(), localPath) || !strings.Contains(err.Error(), "run") {
			t.Fatalf("Layers error = %v; want one naming %s and run", err, localPath)
		}
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("error is not marked ErrInvalid: %v", err)
		}
	})

	t.Run("refused machine file names the file and the key", func(t *testing.T) {
		t.Parallel()
		fx := newLayerFixture(t, geometryHub, map[configengine.Layer]string{configengine.LayerDefault: "name: d\n", configengine.LayerMachine: "name: m\n"})
		_, err := configengine.Layers(fx.dirs, machineSpec())
		machinePath := configengine.MachineFile(fx.board, layerModule)
		if err == nil || !strings.Contains(err.Error(), machinePath) || !strings.Contains(err.Error(), "name") {
			t.Fatalf("Layers error = %v; want one naming %s and name", err, machinePath)
		}
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("error is not marked ErrInvalid: %v", err)
		}
	})

	t.Run("BelowLayer merges the template under the layers below, unexpanded, and ignores the layer and those above", func(t *testing.T) {
		t.Parallel()
		fx := newLayerFixture(t, geometryHub, all)
		const templateRest = "map[deep:1]"
		tests := []struct {
			layer configengine.Layer
			want  map[string]string
		}{
			{configengine.LayerDefault, map[string]string{"name": "tpl", "port": "80", "nested": templateRest, "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"}},
			{configengine.LayerWorktree, map[string]string{"name": "d", "port": "1", "nested": templateRest, "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"}},
			{configengine.LayerLocal, map[string]string{"name": "w", "port": "1", "nested": templateRest, "list": "[a b]", "labels": "map[bug:a bug]", "run": "echo"}},
		}
		for _, tt := range tests {
			below, err := configengine.BelowLayer(fx.dirs, layerSpec(), tt.layer)
			if err != nil {
				t.Fatalf("BelowLayer(%s): %v", tt.layer, err)
			}
			if got := decodeTopLevel(t, below); !mapsEqual(got, tt.want) {
				t.Errorf("BelowLayer(%s) = %v; want %v", tt.layer, got, tt.want)
			}
		}

		overlayFx := newLayerFixture(t, geometryHub, map[configengine.Layer]string{configengine.LayerDefault: "pick: x\n"})
		below, err := configengine.BelowLayer(overlayFx.dirs, overlaySpec(), configengine.LayerWorktree)
		if err != nil {
			t.Fatalf("BelowLayer over the overlay spec: %v", err)
		}
		if got := decodeTopLevel(t, below)["lenses"]; got != "map[a:X b:B]" {
			t.Errorf("BelowLayer over the overlay spec holds lenses %q; want map[a:X b:B]", got)
		}
		under, err := configengine.TemplateUnder(overlayFx.dirs, overlaySpec())
		if err != nil {
			t.Fatalf("TemplateUnder over the overlay spec: %v", err)
		}
		if got := decodeTopLevel(t, under)["lenses"]; got != "map[a:X b:B]" {
			t.Errorf("TemplateUnder over the overlay spec holds lenses %q; want map[a:X b:B]", got)
		}
		noDefaultFx := newLayerFixture(t, geometryHub, nil)
		bare, err := configengine.TemplateUnder(noDefaultFx.dirs, overlaySpec())
		if err != nil {
			t.Fatalf("TemplateUnder with no default file: %v", err)
		}
		if string(bare) != string(overlaySpec().Template) {
			t.Errorf("TemplateUnder with no default file = %q; want the template %q", bare, overlaySpec().Template)
		}

		machineFx := newLayerFixture(t, geometryHub, allFour)
		for _, tt := range []struct {
			layer    configengine.Layer
			wantPort string
		}{
			{configengine.LayerLocal, "8"},
			{configengine.LayerMachine, "1"},
		} {
			below, err := configengine.BelowLayer(machineFx.dirs, machineSpec(), tt.layer)
			if err != nil {
				t.Fatalf("BelowLayer(%s) over the machine spec: %v", tt.layer, err)
			}
			if got := decodeTopLevel(t, below)["port"]; got != tt.wantPort {
				t.Errorf("BelowLayer(%s) over the machine spec holds port %q; want %q", tt.layer, got, tt.wantPort)
			}
		}
	})

	stackTests := []struct {
		name      string
		spec      func() configengine.Spec
		files     map[configengine.Layer]string
		wantDoc   map[string]string
		wantEmpty bool
		origins   map[string]configengine.Layer
	}{
		{
			name:    "no template key is filled",
			files:   map[configengine.Layer]string{configengine.LayerDefault: "name: d\n"},
			wantDoc: map[string]string{"name": "d"},
			origins: map[string]configengine.Layer{"name": configengine.LayerDefault},
		},
		{
			name: "mapping merges per entry while scalar and list are replaced whole",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "name: d\nlist: [a, b]\nnested:\n  deep: 1\n  other: 2\n",
				configengine.LayerWorktree: "name: w\nlist: [q]\nnested:\n  deep: 9\n",
			},
			wantDoc: map[string]string{"name": "w", "list": "[q]", "nested": "map[deep:9 other:2]"},
			origins: map[string]configengine.Layer{"name": configengine.LayerWorktree, "list": configengine.LayerWorktree, "nested": configengine.LayerWorktree},
		},
		{
			name: "whole entry is replaced whole",
			spec: func() configengine.Spec {
				return configengine.Spec{Name: layerModule, Template: []byte("sonnet:\n  a: 1\n"), WholeEntries: true}
			},
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "sonnet:\n  a: 1\n  b: 2\nopus:\n  c: 3\n",
				configengine.LayerWorktree: "sonnet:\n  a: 9\n",
			},
			wantDoc: map[string]string{"sonnet": "map[a:9]", "opus": "map[c:3]"},
			origins: map[string]configengine.Layer{"sonnet": configengine.LayerWorktree, "opus": configengine.LayerDefault},
		},
		{
			name: "origin names the highest layer holding each key",
			files: map[configengine.Layer]string{
				configengine.LayerDefault:  "name: d\nport: 1\n",
				configengine.LayerWorktree: "port: 2\n",
				configengine.LayerLocal:    "name: l\n",
			},
			wantDoc: map[string]string{"name": "l", "port": "2"},
			origins: map[string]configengine.Layer{"name": configengine.LayerLocal, "port": configengine.LayerWorktree},
		},
		{name: "no present layer gives an empty doc", wantEmpty: true},
	}
	for _, tc := range stackTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newLayerFixture(t, geometryHub, tc.files)
			spec := layerSpec()
			if tc.spec != nil {
				spec = tc.spec()
			}
			stack, err := configengine.StackLayers(fx.dirs, spec)
			if err != nil {
				t.Fatalf("StackLayers: %v", err)
			}
			if tc.wantEmpty {
				if len(stack.Doc) != 0 || len(stack.Layers) != 0 || len(stack.Origins) != 0 {
					t.Errorf("stack = %+v; want it empty", stack)
				}
				return
			}
			if got := decodeTopLevel(t, stack.Doc); !mapsEqual(got, tc.wantDoc) {
				t.Errorf("Doc = %v; want %v", got, tc.wantDoc)
			}
			if len(stack.Origins) != len(tc.origins) {
				t.Errorf("Origins holds %d keys; want %d", len(stack.Origins), len(tc.origins))
			}
			for key, want := range tc.origins {
				if got := stack.Origins[key].Layer; got != want {
					t.Errorf("origin of %s = %q; want %q", key, got, want)
				}
			}
		})
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

// TestLayerFile pins which layers apply and where their files live under each geometry, the base layer, and that the local file is never at the staging path.
func TestLayerFile(t *testing.T) {
	t.Parallel()
	spec := layerSpec()
	hubWide := layerSpec()
	hubWide.HubWide = true
	machine := machineSpec()
	hub := configengine.Dirs{Anchor: "/a", Board: "/b"}

	tests := []struct {
		name     string
		dirs     configengine.Dirs
		spec     configengine.Spec
		layer    configengine.Layer
		wantPath string
		wantOK   bool
		wantBase configengine.Layer
	}{
		{"default under the board dir", hub, spec, configengine.LayerDefault, configengine.ConfigFile("/b", layerModule), true, configengine.LayerDefault},
		{"worktree under the anchor", hub, spec, configengine.LayerWorktree, configengine.ConfigFile("/a", layerModule), true, configengine.LayerDefault},
		{"local under the anchor's .lyx", hub, spec, configengine.LayerLocal, filepath.Join("/a", lyxdirs.DotLyxDirName, "config", layerModule+".local.yaml"), true, configengine.LayerDefault},
		{"hub-wide has no worktree file beside a board dir", hub, hubWide, configengine.LayerWorktree, "", false, configengine.LayerDefault},
		{"standalone has no default", configengine.Dirs{Anchor: "/a"}, spec, configengine.LayerDefault, "", false, configengine.LayerWorktree},
		{"hub-wide standalone keeps the worktree file", configengine.Dirs{Anchor: "/a"}, hubWide, configengine.LayerWorktree, configengine.ConfigFile("/a", layerModule), true, configengine.LayerWorktree},
		{"no anchor means no local layer", configengine.Dirs{Board: "/b"}, spec, configengine.LayerLocal, "", false, configengine.LayerDefault},
		{"template has no file", hub, spec, configengine.LayerTemplate, "", false, configengine.LayerDefault},
		{"machine under the board dir's .lyx", hub, machine, configengine.LayerMachine, filepath.Join("/b", lyxdirs.DotLyxDirName, "config", layerModule+".machine.yaml"), true, configengine.LayerDefault},
		{"machine does not apply to a per-worktree module", hub, spec, configengine.LayerMachine, "", false, configengine.LayerDefault},
		{"machine does not apply to a hub-wide module with no machine key", hub, hubWide, configengine.LayerMachine, "", false, configengine.LayerDefault},
		{"machine does not apply with the board dir alone", configengine.Dirs{Board: "/b"}, machine, configengine.LayerMachine, "", false, configengine.LayerDefault},
		{"machine does not apply with the anchor alone", configengine.Dirs{Anchor: "/a"}, machine, configengine.LayerMachine, "", false, configengine.LayerWorktree},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, ok := configengine.LayerFile(tc.dirs, tc.spec, tc.layer)
			if path != tc.wantPath || ok != tc.wantOK {
				t.Errorf("LayerFile = (%q, %v); want (%q, %v)", path, ok, tc.wantPath, tc.wantOK)
			}
			if got := configengine.BaseLayer(tc.dirs, tc.spec); got != tc.wantBase {
				t.Errorf("BaseLayer = %q; want %q", got, tc.wantBase)
			}
			if tc.layer == configengine.LayerLocal && ok && path == configengine.StagingFile("/a", layerModule) {
				t.Errorf("local file is at the staging path %q", path)
			}
		})
	}

	if got, want := configengine.LocalKeys(layerSpec()), []string{"nested.deep"}; !slices.Equal(got, want) {
		t.Errorf("LocalKeys = %v; want %v", got, want)
	}
	if got, want := configengine.LocalDenied(layerSpec()), []string{"list", "nested", "run"}; !slices.Equal(got, want) {
		t.Errorf("LocalDenied = %v; want %v", got, want)
	}
	if got, want := configengine.MachineKeys(machine), []string{"port"}; !slices.Equal(got, want) {
		t.Errorf("MachineKeys = %v; want %v", got, want)
	}
	if got := configengine.MachineKeys(layerSpec()); len(got) != 0 {
		t.Errorf("MachineKeys of a spec with no machine key = %v; want none", got)
	}
}

// annotationsOf returns the line comment of every annotated line of a provenance document, keyed by the line's key.
func annotationsOf(provenance []byte) map[string]string {
	notes := map[string]string{}
	for _, line := range strings.Split(string(provenance), "\n") {
		code, comment, found := strings.Cut(line, " # ")
		if !found {
			continue
		}
		key, _, _ := strings.Cut(strings.TrimSpace(code), ":")
		notes[key] = comment
	}
	return notes
}

// TestProvenance pins that every unit of the resolved configuration is annotated with the highest layer holding it, the template layer when none does, and that the refusals of the loader apply unchanged.
func TestProvenance(t *testing.T) {
	const base = "name: d\nnested:\n  deep: 2\nlist: [x, y]\nlabels:\n  a: A\nrun: sh\n"
	tests := []struct {
		name string
		geo  geometry
		// machine swaps in the hub-wide spec with the machine key port.
		machine bool
		// overlay swaps in overlaySpec.
		overlay   bool
		files     map[configengine.Layer]string
		want      map[string]string
		wantError string
	}{
		{
			name:    "a unit only the picked overlay layer holds names it as the template layer's overlay",
			overlay: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: "pick: x\n"},
			want:    map[string]string{"pick": "default", "a": "template:x", "b": "template:x", "name": "template", "port": "template", "deep": "template", "list": "template", "labels": "template", "run": "template"},
		},
		{
			name:    "a machine-held unit names the machine layer",
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: base, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "default", "port": "machine", "deep": "default", "list": "default", "labels": "default", "run": "default"},
		},
		{
			name:    "a machine-held unit names the machine layer with an anchor holding no _lyx",
			geo:     geometryAnchorWithoutLyx,
			machine: true,
			files:   map[configengine.Layer]string{configengine.LayerDefault: base, configengine.LayerMachine: "port: 8\n"},
			want:    map[string]string{"name": "default", "port": "machine", "deep": "default", "list": "default", "labels": "default", "run": "default"},
		},
		{
			name:  "each unit names the highest layer holding it",
			files: map[configengine.Layer]string{configengine.LayerDefault: base, configengine.LayerWorktree: "name: w\nlabels:\n  b: B\n", configengine.LayerLocal: "nested:\n  deep: 5\n"},
			want:  map[string]string{"name": "worktree", "port": "template", "deep": "local", "list": "default", "labels": "worktree", "run": "default"},
		},
		{
			name:      "a refused local key errors naming the file",
			files:     map[configengine.Layer]string{configengine.LayerDefault: base, configengine.LayerLocal: "run: sh\n"},
			wantError: "local config file",
		},
		{
			name:      "an absent base layer errors rather than degrading",
			files:     map[configengine.Layer]string{configengine.LayerLocal: "nested:\n  deep: 5\n"},
			wantError: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newLayerFixture(t, tt.geo, tt.files)
			spec := layerSpec()
			if tt.overlay {
				spec = overlaySpec()
			}
			if tt.machine {
				spec = machineSpec()
			}
			provenance, err := configengine.Provenance(fixture.dirs, spec)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Provenance error = %v, want one containing %q", err, tt.wantError)
				}
				if strings.Contains(tt.wantError, "local") && !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("Provenance error = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Provenance: %v", err)
			}
			if got := annotationsOf(provenance); !maps.Equal(got, tt.want) {
				t.Fatalf("annotations = %v, want %v\n%s", got, tt.want, provenance)
			}
		})
	}
}
