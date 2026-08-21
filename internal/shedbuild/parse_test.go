// parse_test.go covers Parse's shape checks and decode errors, table-driven over inline YAML
// byte-slice literals with no filesystem access anywhere in this file.

package shedbuild

import (
	"strings"
	"testing"
)

func TestParse_ValueAndErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantRecipe  *Recipe
		wantErrSub  string
		wantLineNum bool
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
						Segment:    "seg",
						MaxBounces: 3,
					},
				},
			},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) = _, %v; want no error", tt.name, err)
			}
			if !recipeEqual(got, *tt.wantRecipe) {
				t.Errorf("Parse(%q) = %+v; want %+v", tt.name, got, *tt.wantRecipe)
			}
		})
	}
}

// recipeEqual reports whether two Recipe values are equal, including a nil-vs-empty distinction on
// each row's Config map.
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

// rowEqual reports whether two Row values are equal, including a nil-vs-empty distinction on
// Config.
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

// configEqual deep-compares two decoded config maps by recursive structural comparison, since
// map[string]any values may nest further maps and slices.
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

// TestParse_UnknownKeyDeterminism guards against map-iteration-order nondeterminism in the
// unknown-key rejection message: parsing the same unknown-key input twice must report the same
// offending key both times.
func TestParse_UnknownKeyDeterminism(t *testing.T) {
	input := []byte(`
version: 1
entry: start
terminals: [done]
producers:
  - name: row1
    engine: bouncer
zzz_unknown_key: nope
`)

	_, err1 := Parse(input)
	_, err2 := Parse(input)

	if err1 == nil || err2 == nil {
		t.Fatalf("Parse(unknown key input) = nil error on at least one call; want error both times (err1=%v, err2=%v)", err1, err2)
	}
	if err1.Error() != err2.Error() {
		t.Errorf("Parse(unknown key input) reported different errors across two calls: %q vs %q", err1.Error(), err2.Error())
	}
}
