// set_test.go — unit tests for the non-interactive Set entry point (set.go).
//
// Tests cover: scaffold-then-set when the config file is missing, rollback of a freshly-scaffolded file on an unknown key, byte-for-byte preservation of a pre-existing file on a refused key, preservation of untouched keys when setting one key on an existing multi-key file, open-map entries, and end-to-end reporting of Set's returned preserved-keys list for an orphaned key.

package configengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestSet pins what Set writes: a missing file is scaffolded from the template and the pairs applied in one call, other keys keep their values, an orphaned top-level key survives and is reported, and a refused pair removes a file Set scaffolded or leaves a pre-existing one byte for byte unchanged, a machine write included.
func TestSet(t *testing.T) {
	t.Parallel()
	const labelsTemplate = "name: x\nlabels:\n  bug: a bug\n"

	tests := []struct {
		name     string
		state    baseState
		existing string
		template string
		pairs    []yamlengine.KV
		openMaps []string
		// perNameMaps is the spec's per-name maps.
		perNameMaps []string
		// wantErr is a substring of the refusal; empty means Set must succeed.
		wantErr       string
		wantPreserved []string
		contains      []string
		// layer is the layer written; empty means the worktree layer.
		layer configengine.Layer
		// board tells the call a board dir, which makes the worktree layer sparse.
		board bool
		// categories classify the template for a local or machine write.
		categories map[string]configengine.Category
		// localExisting is the local or machine file before the call; empty means it is absent.
		localExisting string
		// hubWide marks the spec hub-wide and gives the board dir its _lyx/config/, as a machine write needs; it needs board.
		hubWide bool
		// boardWithoutLyx leaves the board dir with no _lyx/ beside the anchor's, which hubWide would otherwise create.
		boardWithoutLyx bool
		// boardOnly tells the call the board dir alone, with no anchor.
		boardOnly bool
		// jsonEntries marks the spec JSONEntries.
		jsonEntries bool
		// wantExact is the written file's whole content; empty skips the check.
		wantExact string
		// seedOnly marks the spec seed-only, so a hub's default layer keeps its full scaffold.
		seedOnly bool
	}{
		{
			name:      "hub default layer write into an absent file writes exactly the key set",
			state:     baseLyx,
			template:  "key1: value1\nkey2: value2\n",
			pairs:     []yamlengine.KV{{Key: "key1", Value: "set1"}},
			layer:     configengine.LayerDefault,
			board:     true,
			wantExact: "key1: set1\n",
		},
		{
			name:          "hub default layer write into a present sparse file keeps its own units and adds no template key",
			state:         baseLyx,
			template:      "key1: value1\nkey2: value2\nkey3: value3\n",
			pairs:         []yamlengine.KV{{Key: "key1", Value: "set1"}},
			layer:         configengine.LayerDefault,
			board:         true,
			localExisting: "key2: mine\n",
			wantExact:     "key2: mine\nkey1: set1\n",
		},
		{
			name:      "seed-only hub default write into an absent file still scaffolds the template",
			state:     baseLyx,
			template:  "key1: value1\nkey2: value2\n",
			pairs:     []yamlengine.KV{{Key: "key1", Value: "set1"}},
			layer:     configengine.LayerDefault,
			board:     true,
			seedOnly:  true,
			wantExact: "key1: set1\nkey2: value2\n",
		},
		{
			name:     "missing file is scaffolded then set",
			state:    baseLyx,
			template: "key1: value1\nkey2: value2\n",
			pairs:    []yamlengine.KV{{Key: "key1", Value: "set1"}},
			contains: []string{"key1: set1", "key2: value2"},
		},
		{
			name:     "other keys keep their values",
			state:    baseWithFile,
			existing: "key1: original_value1\nkey2: original_value2\n",
			template: "key1: default1\nkey2: default2\n",
			pairs:    []yamlengine.KV{{Key: "key1", Value: "new_value1"}},
			contains: []string{"key1: new_value1", "key2: original_value2"},
		},
		{
			name:          "key absent from the template is preserved and reported",
			state:         baseWithFile,
			existing:      "key1: original_value1\nlegacy: keepme\n",
			template:      "key1: default1\n",
			pairs:         []yamlengine.KV{{Key: "key1", Value: "new_value1"}},
			wantPreserved: []string{"legacy"},
			contains:      []string{"key1: new_value1", "legacy: keepme"},
		},
		{
			name:     "declared open map gains an entry beside the existing ones",
			state:    baseWithFile,
			existing: labelsTemplate,
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "labels.x", Value: "an x"}},
			openMaps: []string{"labels"},
			contains: []string{"x: an x", "bug: a bug"},
		},
		{
			name:        "per-name map gains a name the template lacks on the base layer and sheds a template-equal one",
			state:       baseWithFile,
			existing:    "name: x\nlenses:\n  a: A\n",
			template:    "name: x\nlenses:\n  a: A\n",
			pairs:       []yamlengine.KV{{Key: "lenses.new", Value: "a new lens"}},
			perNameMaps: []string{"lenses"},
			wantExact:   "name: x\nlenses:\n    new: a new lens\n",
		},
		{
			name:        "per-name map gains a name the template lacks on a sparse layer",
			state:       baseLyx,
			template:    "name: x\nlenses:\n  a: A\n",
			pairs:       []yamlengine.KV{{Key: "lenses.new", Value: "a new lens"}},
			perNameMaps: []string{"lenses"},
			board:       true,
			wantExact:   "lenses:\n    new: a new lens\n",
		},
		{
			name:     "unknown key removes the scaffolded file",
			state:    baseLyx,
			template: "key1: value1\n",
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			wantErr:  "bogus",
		},
		{
			name:     "unknown key leaves the existing file unchanged",
			state:    baseWithFile,
			existing: "key1: original_value\n",
			template: "key1: default1\n",
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			wantErr:  "unknown config key(s)",
		},
		{
			name:     "undeclared key beside an open map is still refused",
			state:    baseWithFile,
			existing: labelsTemplate,
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			openMaps: []string{"labels"},
			wantErr:  "unknown config key(s)",
		},
		{
			name:     "open-map entry on a list-shaped file is refused",
			state:    baseWithFile,
			existing: "name: x\nlabels:\n  - bug\n",
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "labels.x", Value: "an x"}},
			openMaps: []string{"labels"},
			wantErr:  "map of name to description",
		},
		{
			name:      "worktree file under a board is sparse and holds only the key set",
			state:     baseLyx,
			template:  "key1: value1\nkey2: value2\n",
			pairs:     []yamlengine.KV{{Key: "key1", Value: "set1"}},
			board:     true,
			wantExact: "key1: set1\n",
		},
		{
			name:      "sparse worktree file keeps its own keys beside the key set",
			state:     baseWithFile,
			existing:  "key2: mine\n",
			template:  "key1: value1\nkey2: value2\n",
			pairs:     []yamlengine.KV{{Key: "key1", Value: "set1"}},
			board:     true,
			wantExact: "key2: mine\nkey1: set1\n",
		},
		{
			name:       "local write of an open key creates the local file",
			state:      baseLyx,
			template:   "name: x\nrun: y\n",
			pairs:      []yamlengine.KV{{Key: "name", Value: "z"}},
			layer:      configengine.LayerLocal,
			categories: map[string]configengine.Category{"run": configengine.CategoryExecutable},
			wantExact:  "name: z\n",
		},
		{
			name:       "refused local write creates no file",
			state:      baseLyx,
			template:   "name: x\nrun: y\n",
			pairs:      []yamlengine.KV{{Key: "run", Value: "evil"}},
			layer:      configengine.LayerLocal,
			categories: map[string]configengine.Category{"run": configengine.CategoryExecutable},
			wantErr:    "run",
		},
		{
			name:          "refused local write leaves the previous local file byte for byte",
			state:         baseLyx,
			template:      "name: x\nrun: y\n",
			pairs:         []yamlengine.KV{{Key: "run", Value: "evil"}},
			layer:         configengine.LayerLocal,
			categories:    map[string]configengine.Category{"run": configengine.CategoryExecutable},
			localExisting: "name: mine\n",
			wantErr:       "run",
		},
		{
			name:       "machine write of the machine key creates the file holding only that key",
			state:      baseLyx,
			template:   "name: x\nport: 1\n",
			pairs:      []yamlengine.KV{{Key: "port", Value: "8"}},
			layer:      configengine.LayerMachine,
			board:      true,
			hubWide:    true,
			categories: map[string]configengine.Category{"port": configengine.CategoryMachine, "name": configengine.CategoryNone},
			wantExact:  "port: 8\n",
		},
		{
			name:       "machine write of a non-machine key is refused and leaves no file",
			state:      baseLyx,
			template:   "name: x\nport: 1\n",
			pairs:      []yamlengine.KV{{Key: "name", Value: "z"}},
			layer:      configengine.LayerMachine,
			board:      true,
			hubWide:    true,
			categories: map[string]configengine.Category{"port": configengine.CategoryMachine, "name": configengine.CategoryNone},
			wantErr:    "name",
		},
		{
			name:       "machine write under the board dir alone does not apply and leaves no file",
			state:      baseLyx,
			template:   "name: x\nport: 1\n",
			pairs:      []yamlengine.KV{{Key: "port", Value: "8"}},
			layer:      configengine.LayerMachine,
			board:      true,
			boardOnly:  true,
			hubWide:    true,
			categories: map[string]configengine.Category{"port": configengine.CategoryMachine},
			wantErr:    "does not apply here",
		},
		{
			name:            "machine write beside a board dir with no _lyx is refused and leaves no file, though the anchor has one",
			state:           baseLyx,
			template:        "name: x\nport: 1\n",
			pairs:           []yamlengine.KV{{Key: "port", Value: "8"}},
			layer:           configengine.LayerMachine,
			board:           true,
			hubWide:         true,
			boardWithoutLyx: true,
			categories:      map[string]configengine.Category{"port": configengine.CategoryMachine},
			wantErr:         "directory not found",
		},
		{
			name:        "machine entry of a non-machine key in a JSON-entries spec is refused and leaves no file",
			state:       baseLyx,
			template:    "name: x\nport: 1\n",
			pairs:       []yamlengine.KV{{Key: "name", Value: `{"a": 1}`}},
			layer:       configengine.LayerMachine,
			board:       true,
			hubWide:     true,
			jsonEntries: true,
			categories:  map[string]configengine.Category{"port": configengine.CategoryMachine, "name": configengine.CategoryNone},
			wantErr:     "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, path := newBase(t, tt.state, "testmod", tt.existing)
			dirs := configengine.Dirs{Anchor: baseDir}
			if tt.board {
				dirs.Board = t.TempDir()
			}
			if (tt.hubWide || tt.layer == configengine.LayerDefault) && !tt.boardWithoutLyx {
				mustMkdirAll(t, configengine.ConfigDir(dirs.Board))
			}
			if tt.boardOnly {
				dirs.Anchor = ""
			}
			layer := tt.layer
			if layer == "" {
				layer = configengine.LayerWorktree
			}
			previous, scaffolded := tt.existing, tt.state == baseLyx
			if layer == configengine.LayerLocal || layer == configengine.LayerMachine || layer == configengine.LayerDefault {
				path = configengine.LocalFile(baseDir, "testmod")
				if layer == configengine.LayerMachine {
					path = configengine.MachineFile(dirs.Board, "testmod")
				}
				if layer == configengine.LayerDefault {
					path = configengine.ConfigFile(dirs.Board, "testmod")
				}
				previous, scaffolded = tt.localExisting, tt.localExisting == ""
				if tt.localExisting != "" {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(path, []byte(tt.localExisting), 0o644); err != nil {
						t.Fatalf("write local file: %v", err)
					}
				}
			}
			spec := configengine.Spec{Name: "testmod", Template: []byte(tt.template), OpenMaps: tt.openMaps, PerNameMaps: tt.perNameMaps, Categories: tt.categories, HubWide: tt.hubWide, JSONEntries: tt.jsonEntries, SeedOnly: tt.seedOnly}

			preserved, err := configengine.Set(dirs, spec, layer, tt.pairs)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Set() = %v; want an error containing %q", err, tt.wantErr)
				}
				if scaffolded {
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Errorf("config file still exists after the refusal; want the scaffold removed")
					}
					return
				}
				assertFileUnchanged(t, path, previous)
				return
			}
			if err != nil {
				t.Fatalf("Set() = %v; want nil", err)
			}
			if !slices.Equal(preserved, tt.wantPreserved) {
				t.Errorf("Set() preserved = %v; want %v", preserved, tt.wantPreserved)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("config file not found at %s: %v", path, readErr)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(data), want) {
					t.Errorf("Set() file = %q; want it to contain %q", data, want)
				}
			}
			if tt.wantExact != "" && string(data) != tt.wantExact {
				t.Errorf("Set() file = %q; want exactly %q", data, tt.wantExact)
			}
		})
	}
}
