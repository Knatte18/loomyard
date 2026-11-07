// parse_test.go covers Parse's shape checks and decode errors, table-driven over inline YAML byte-slice literals.
// The file has no filesystem access anywhere.

package shedbuild

import (
	"maps"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestParse_ValueAndErrorCases(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		yaml        string
		wantRecipe  *Recipe
		wantErrSub  string
		wantLineNum bool
		wantRetired map[string]string
	}{
		{
			name: "minimal well-formed document with one row setting every field",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    config:
      key: value
    on_done: row2
    on_stuck: row1
    on_reroute: row3
    segment: seg
    max_bounces: 3
`,
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "start",
				Terminals: []string{"done"},
				Producers: []Row{
					{
						Name:       "row1",
						Engine:     "bouncer",
						Config:     map[string]any{"key": "value"},
						OnDone:     "row2",
						OnStuck:    "row1",
						OnReroute:  "row3",
						Segment:    "seg",
						MaxBounces: 3,
					},
				},
			},
		},
		{
			name: "amendment_target decodes into the field",
			yaml: `
version: 1
entry: start
amendment_target: row1
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantRecipe: &Recipe{
				Version:         1,
				Entry:           "start",
				AmendmentTarget: "row1",
				Terminals:       []string{"done"},
				Producers: []Row{
					{Name: "row1", Engine: "bouncer"},
				},
			},
		},
		{
			name: "contract and verify_source decode into the fields",
			yaml: `
version: 1
entry: start
contract: loom
verify_source: tree_scope
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantRecipe: &Recipe{
				Version:      1,
				Entry:        "start",
				Contract:     "loom",
				VerifySource: "tree_scope",
				Terminals:    []string{"done"},
				Producers: []Row{
					{Name: "row1", Engine: "bouncer"},
				},
			},
		},
		{
			name: "a well-formed block row decodes block and block_gate and may leave on_done empty",
			yaml: blockRowYAML(""),
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "Motif",
				Terminals: []string{"done"},
				Producers: []Row{
					{Name: "Motif", Block: "motif", BlockGate: "always"},
				},
			},
		},
		{
			name:       "a block row setting engine is refused naming the key and the way forward",
			yaml:       blockRowYAML("engine: bouncer"),
			wantErrSub: `shedbuild: producer 0 "Motif": a block row carries only name, block, block_gate and on_done, and sets engine; way forward: remove engine, since the block's rows carry their own`,
		},
		{
			name:       "a block row setting config is refused",
			yaml:       blockRowYAML("config: {a: 1}"),
			wantErrSub: `a block row carries only name, block, block_gate and on_done, and sets config; way forward: remove config,`,
		},
		{
			name:       "a block row setting on_stuck is refused",
			yaml:       blockRowYAML("on_stuck: row1"),
			wantErrSub: `and sets on_stuck; way forward: remove on_stuck,`,
		},
		{
			name:       "a block row setting on_reroute is refused",
			yaml:       blockRowYAML("on_reroute: row1"),
			wantErrSub: `and sets on_reroute; way forward: remove on_reroute,`,
		},
		{
			name:       "a block row setting segment is refused",
			yaml:       blockRowYAML("segment: seg"),
			wantErrSub: `and sets segment; way forward: remove segment,`,
		},
		{
			name:       "a block row setting max_bounces is refused",
			yaml:       blockRowYAML("max_bounces: 2"),
			wantErrSub: `and sets max_bounces; way forward: remove max_bounces,`,
		},
		{
			name:       "a block row setting entry_gate is refused",
			yaml:       blockRowYAML("entry_gate: loom.a"),
			wantErrSub: `and sets entry_gate; way forward: remove entry_gate,`,
		},
		{
			name: "a block row missing block is refused",
			yaml: `version: 1
entry: Motif
terminals: [done]
producers:
  - name: Motif
    block_gate: always
`,
			wantErrSub: `shedbuild: producer 0 "Motif": a block row needs block; way forward: set block, block_gate naming one of the shipped checks`,
		},
		{
			name: "a block row missing block_gate is refused",
			yaml: `version: 1
entry: Motif
terminals: [done]
producers:
  - name: Motif
    block: motif
`,
			wantErrSub: `shedbuild: producer 0 "Motif": a block row needs block_gate; way forward: set block_gate, block_gate naming one of the shipped checks`,
		},
		{
			name: "a hand-written engine Block is reserved for block rows",
			yaml: `version: 1
entry: row1
terminals: [done]
producers:
  - name: row1
    engine: Block
`,
			wantErrSub: `shedbuild: producer 0 "row1": engine Block is reserved for block rows; way forward: write the row as a block row with block and block_gate`,
		},
		{
			name:       "block_path written in a file is refused as an unknown field",
			yaml:       blockRowYAML("block_path: Outer"),
			wantErrSub: "block_path",
		},
		{
			name: "unknown contract is refused naming the key, the legal value and the way forward",
			yaml: `
version: 1
entry: start
contract: weave
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: `shedbuild: contract "weave" is not a legal value (legal: loom); way forward: set it to one of them or remove it`,
		},
		{
			name: "unknown verify_source is refused naming the key, the legal values and the way forward",
			yaml: `
version: 1
entry: start
verify_source: elsewhere
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: `shedbuild: verify_source "elsewhere" is not a legal value (legal: plan, tree_scope); way forward: set it to one of them or remove it`,
		},
		{
			name: "row with no config block yields nil Config",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "start",
				Terminals: []string{"done"},
				Producers: []Row{
					{Name: "row1", Engine: "bouncer"},
				},
			},
		},
		{
			name: "row config survives untouched including nested mapping and list value",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    config:
      nested:
        inner: 1
      list_value:
        - a
        - b
`,
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "start",
				Terminals: []string{"done"},
				Producers: []Row{
					{
						Name:   "row1",
						Engine: "bouncer",
						Config: map[string]any{
							"nested":     map[string]any{"inner": 1},
							"list_value": []any{"a", "b"},
						},
					},
				},
			},
		},
		{
			name: "row order preserved exactly",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: rowB
    engine: bouncer
  - name: rowA
    engine: bouncer
  - name: rowC
    engine: bouncer
`,
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "start",
				Terminals: []string{"done"},
				Producers: []Row{
					{Name: "rowB", Engine: "bouncer"},
					{Name: "rowA", Engine: "bouncer"},
					{Name: "rowC", Engine: "bouncer"},
				},
			},
		},
		{
			name:       "malformed YAML: tab-indented line",
			yaml:       "version: 1\nentry: start\n\tterminals: [done]\n",
			wantErrSub: "shedbuild: ",
		},
		{
			name:       "malformed YAML: unclosed quote",
			yaml:       "version: 1\nentry: \"start\nterminals: [done]\n",
			wantErrSub: "shedbuild: ",
		},
		{
			name: "missing version key",
			yaml: `
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: unsupported recipe version 0",
		},
		{
			name: "literal version: 0",
			yaml: `
version: 0
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: unsupported recipe version 0",
		},
		{
			name: "version: 2 names the offending value",
			yaml: `
version: 2
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: unsupported recipe version 2",
		},
		{
			name: "version: -1 names the offending value",
			yaml: `
version: -1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: unsupported recipe version -1",
		},
		{
			name: "duplicate mapping key at document level",
			yaml: `
version: 1
entry: start
entry: again
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: ",
		},
		{
			name: "duplicate key inside one row",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    name: row2
    engine: bouncer
`,
			wantErrSub: "shedbuild: ",
		},
		{
			name:       "empty input yields recipe is empty",
			yaml:       "",
			wantErrSub: "shedbuild: recipe is empty",
		},
		{
			name:       "whitespace-only input yields recipe is empty",
			yaml:       "   \n  \n",
			wantErrSub: "shedbuild: recipe is empty",
		},
		{
			name:       "comments-only input yields recipe is empty",
			yaml:       "# just a comment\n# another comment\n",
			wantErrSub: "shedbuild: recipe is empty",
		},
		{
			name: "unknown document-level key",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
bogus_key: nope
`,
			wantErrSub: "bogus_key",
		},
		{
			name: "unknown row-level key",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    bogus_row_key: nope
`,
			wantErrSub: "bogus_row_key",
		},
		{
			name: "config holding a scalar",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    config: scalarvalue
`,
			wantErrSub:  "shedbuild: ",
			wantLineNum: true,
		},
		{
			name: "config holding a list",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    config:
      - a
      - b
`,
			wantErrSub:  "shedbuild: ",
			wantLineNum: true,
		},
		{
			name: "empty entry errors distinctly",
			yaml: `
version: 1
entry: ""
terminals: [done]
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: entry must not be empty",
		},
		{
			name: "empty terminals errors distinctly",
			yaml: `
version: 1
entry: start
terminals: []
producers:
  - name: row1
    engine: bouncer
`,
			wantErrSub: "shedbuild: terminals must not be empty",
		},
		{
			name: "empty producers errors distinctly",
			yaml: `
version: 1
entry: start
terminals: [done]
producers: []
`,
			wantErrSub: "shedbuild: producers must not be empty",
		},
		{
			name: "empty name in a row",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: ""
    engine: bouncer
`,
			wantErrSub: "shedbuild: producer 0: name must not be empty",
		},
		{
			name: "empty engine in a row",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: ""
`,
			wantErrSub: `shedbuild: producer 0 "row1": engine must not be empty`,
		},
		{
			name: "negative max_bounces in a row",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
    max_bounces: -2
`,
			wantErrSub: `shedbuild: producer 0 "row1": max_bounces must not be negative, got -2`,
		},
		{
			name: "duplicate name across two rows",
			yaml: `
version: 1
entry: start
terminals: [done]
producers:
  - name: dup
    engine: bouncer
  - name: dup
    engine: burler
`,
			wantErrSub: `shedbuild: producer 1 "dup": duplicate name, already defined by producer 0`,
		},
		{
			name: "an entry_gate check joining two operands is refused naming the row, the check and the way forward",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate: "loom.a && loom.b"
`,
			wantErrSub: `shedbuild: row "row1": entry_gate check "loom.a && loom.b": "loom.a && loom.b" is not an operand of the form module.key or budget.<Segment>; way forward: write each check as <operand>, <operand> == <literal> or <operand> != <literal>`,
		},
		{
			name: "an entry_gate list holding an operand with no dot is refused",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate: [loom.a, loom]
`,
			wantErrSub: `entry_gate check "loom": "loom" is not an operand`,
		},
		{
			name: "an empty entry_gate check is refused",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate: ""
`,
			wantErrSub: "the check is empty",
		},
		{
			name: "a comparison with no literal is refused",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate: "loom.a =="
`,
			wantErrSub: "the literal after the comparison is empty",
		},
		{
			name: "an unquoted literal holding whitespace is refused",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate: "loom.a == 1 != 2"
`,
			wantErrSub: `the literal "1 != 2" holds whitespace`,
		},
		{
			name: "an entry_gate mapping is refused",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
    entry_gate:
      loom.a: true
`,
			wantErrSub:  "entry_gate must be one check or a list of checks",
			wantLineNum: true,
		},
		{
			name: "a retired map decodes into the field",
			yaml: retiredYAML("Old-Row: row2"),
			wantRecipe: &Recipe{
				Version:   1,
				Entry:     "row1",
				Terminals: []string{"row2"},
				Producers: []Row{
					{Name: "row1", Engine: "bouncer", OnDone: "row2"},
					{Name: "row2", Engine: "bouncer"},
				},
			},
			wantRetired: map[string]string{"Old-Row": "row2"},
		},
		{
			name:       "a retired key that is also a row is refused naming the key",
			yaml:       retiredYAML("row1: row2"),
			wantErrSub: `retired row "row1" is also a row of the recipe`,
		},
		{
			name:       "a retired target naming no row is refused naming the key and the target",
			yaml:       retiredYAML("Old-Row: nowhere"),
			wantErrSub: `retired row "Old-Row" moves to "nowhere", which names no row of the recipe`,
		},
		{
			// Parse refuses a stray "---" mid-recipe.
			// Decoding only the first document would run a self-consistent prefix as a truncated pipeline.
			name: "second YAML document is refused, not dropped",
			yaml: `version: 1
entry: row1
terminals: [row1]
producers:
  - name: row1
    engine: bouncer
---
version: 1
entry: the-replacement-graph
terminals: [the-replacement-graph]
producers:
  - name: the-replacement-graph
    engine: bouncer
`,
			wantErrSub: "more than one YAML document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tt.yaml))

			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, nil; want error containing %q", tt.name, got, tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("Parse(%q) error = %q; want substring %q", tt.name, err.Error(), tt.wantErrSub)
				}
				if tt.wantLineNum && !strings.Contains(err.Error(), "line ") {
					t.Errorf("Parse(%q) error = %q; want a yaml line-number position", tt.name, err.Error())
				}
				// Map-iteration order must not change which offending key an error names.
				if _, again := Parse([]byte(tt.yaml)); again == nil || again.Error() != err.Error() {
					t.Errorf("Parse(%q) second call error = %v; want the identical error %q", tt.name, again, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) = _, %v; want no error", tt.name, err)
			}
			if !recipeEqual(got, *tt.wantRecipe) {
				t.Errorf("Parse(%q) = %+v; want %+v", tt.name, got, *tt.wantRecipe)
			}
			if !maps.Equal(got.Retired, tt.wantRetired) {
				t.Errorf("Parse(%q) Retired = %v; want %v", tt.name, got.Retired, tt.wantRetired)
			}
			for i, row := range got.Producers {
				if want := tt.wantRecipe.Producers[i]; row.Block != want.Block || row.BlockGate != want.BlockGate {
					t.Errorf("Parse(%q) row %d Block, BlockGate = %q, %q; want %q, %q", tt.name, i, row.Block, row.BlockGate, want.Block, want.BlockGate)
				}
			}
		})
	}
}

// retiredYAML returns a two-row recipe whose retired map holds the one entry given.
func retiredYAML(entry string) string {
	return `version: 1
entry: row1
terminals: [row2]
producers:
  - name: row1
    engine: bouncer
    on_done: row2
  - name: row2
    engine: bouncer
retired:
  ` + entry + "\n"
}

// blockRowYAML returns a one-row recipe whose row is a well-formed block row.
// It appends extra to the row when extra is non-empty.
func blockRowYAML(extra string) string {
	yaml := `version: 1
entry: Motif
terminals: [done]
producers:
  - name: Motif
    block: motif
    block_gate: always
`
	if extra != "" {
		yaml += "    " + extra + "\n"
	}
	return yaml
}

// recipeEqual reports whether two Recipe values are equal, row for row.
func recipeEqual(a, b Recipe) bool {
	if a.Version != b.Version || a.Entry != b.Entry {
		return false
	}
	if !stringSlicesEqual(a.Terminals, b.Terminals) {
		return false
	}
	if len(a.Producers) != len(b.Producers) {
		return false
	}
	for i := range a.Producers {
		if !rowEqual(a.Producers[i], b.Producers[i]) {
			return false
		}
	}
	return true
}

// rowEqual reports whether two Row values are equal.
// A nil Config map is not equal to an empty one.
func rowEqual(a, b Row) bool {
	if a.Name != b.Name || a.Engine != b.Engine || a.OnDone != b.OnDone ||
		a.OnStuck != b.OnStuck || a.Segment != b.Segment || a.MaxBounces != b.MaxBounces {
		return false
	}
	if (a.Config == nil) != (b.Config == nil) {
		return false
	}
	return configEqual(a.Config, b.Config)
}

// configEqual reports whether two decoded config maps are deep-equal.
// Their values may nest further maps and slices, so the comparison recurses.
func configEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !anyEqual(av, bv) {
			return false
		}
	}
	return true
}

// anyEqual recursively compares two decoded YAML values of unknown concrete type.
func anyEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		return ok && configEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !anyEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
