// set_test.go contains table-driven tests for SetValues, covering unknown-key rejection, byte-for-byte round-tripping of tricky values, comment/order preservation, orphan-key preservation, list keys and open maps, and the partial-existing regression case that motivated Card 1's always-mutate-the-template-tree design.

package yamlengine

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	openMapSetTemplate = "name: tmpl\nlabels: {}\n"
	preservedMarker    = "# preserved (not in current template)"
	listKeyTemplate    = "require_pr_to_base: [\"main\"] # bases\nsquash: true\n"
	nullBudgetTemplate = "plan_max_bounces: ~\nreview_max_bounces: 1\n"
	roundListTemplate  = "plan_review: \"\" # rounds\n"
	perNameSetTemplate = "names:\n  a: default a\nlists:\n  l: [x]\n"

	// perNameMappingTemplate holds a per-name map whose entries are mappings, the second holding keys the first lacks.
	perNameMappingTemplate = "profiles:\n  plain:\n    kind: identity\n  costed:\n    kind: cost\n    budget: 200\n    tags: [a]\n"
)

var (
	perNameSetMaps     = []string{"lists", "names"}
	perNameMappingMaps = []string{"profiles"}
)

func intPointer(v int) *int { return &v }

// assertDecodesAsInts fails unless merged decodes each key of want into an *int equal to want's entry, nil included.
func assertDecodesAsInts(t *testing.T, merged []byte, want map[string]*int) {
	t.Helper()
	if want == nil {
		return
	}
	var got map[string]*int
	if err := yaml.Unmarshal(merged, &got); err != nil {
		t.Fatalf("decode %q: %v", merged, err)
	}
	for key, wantValue := range want {
		gotValue, present := got[key]
		switch {
		case !present:
			t.Errorf("%q does not hold %q", merged, key)
		case (gotValue == nil) != (wantValue == nil), gotValue != nil && *gotValue != *wantValue:
			t.Errorf("%s decodes as %v; want %v", key, gotValue, wantValue)
		}
	}
}

// TestSetValues pins what a successful SetValues merges: the requested values land byte-for-byte, every other key keeps its existing value or its template default, template comments and key order survive, an orphan top-level key is preserved whole and reported, list keys and open maps are set whole, a per-name map keeps only the names that differ from the template, and a second call on the merged output reproduces it.
func TestSetValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		template      string
		existing      string
		pairs         []KV
		openMaps      []string
		perNameMaps   []string
		wantValues    map[string]string
		wantPreserved []string
		contains      []string
		notContains   []string
		inOrder       []string
		wantInts      map[string]*int
	}{
		{
			name:       "value with equals signs round-trips",
			template:   "key1: default\n",
			pairs:      []KV{{Key: "key1", Value: "a=b=c"}},
			wantValues: map[string]string{"key1": "a=b=c"},
		},
		{
			name:       "value with spaces round-trips",
			template:   "key1: default\n",
			pairs:      []KV{{Key: "key1", Value: "hello there world"}},
			wantValues: map[string]string{"key1": "hello there world"},
		},
		{
			name:       "multiple pairs are all applied",
			template:   "key1: default1\nkey2: default2\nkey3: default3\n",
			pairs:      []KV{{Key: "key1", Value: "set1"}, {Key: "key3", Value: "set3"}},
			wantValues: map[string]string{"key1": "set1", "key3": "set3", "key2": "default2"},
		},
		{
			name:       "empty existing behaves like the template",
			template:   "key1: default1\nkey2: default2\n",
			pairs:      []KV{{Key: "key1", Value: "set1"}},
			wantValues: map[string]string{"key1": "set1", "key2": "default2"},
		},
		{
			name:       "template comments and order survive, only the requested key changes",
			template:   "# Key 1 comment\nkey1: template_val1\n# Key 2 comment\nkey2: template_val2\n",
			existing:   "key2: user_val2\nkey1: user_val1\n",
			pairs:      []KV{{Key: "key1", Value: "new_val1"}},
			wantValues: map[string]string{"key1": "new_val1", "key2": "user_val2"},
			contains:   []string{"# Key 1 comment", "# Key 2 comment"},
			inOrder:    []string{"key1: ", "key2: "},
		},
		{
			// The plan-review round-1 regression: a key in the template but absent from a non-empty, partial existing must still be applied rather than silently dropped.
			name:       "partial existing does not suppress the set",
			template:   "key1: default1\nkey2: default2\nkey3: default3\n",
			existing:   "key1: user_val1\n",
			pairs:      []KV{{Key: "key2", Value: "newly_set"}},
			wantValues: map[string]string{"key1": "user_val1", "key2": "newly_set", "key3": "default3"},
		},
		{
			name:          "unknown existing key is preserved after every template key",
			template:      "key1: default1\nkey2: default2\n",
			existing:      "key1: user_val1\nkey2: user_val2\npath: ../_board\n",
			pairs:         []KV{{Key: "key1", Value: "new_val1"}},
			wantValues:    map[string]string{"path": "../_board"},
			wantPreserved: []string{"path"},
			inOrder:       []string{"key1: ", "key2: ", "path: "},
		},
		{
			name:          "unknown existing keys are preserved and reported sorted",
			template:      "key1: default1\n",
			existing:      "key1: user_val1\nzebra: z_val\napple: a_val\nmango: m_val\n",
			wantValues:    map[string]string{"apple": "a_val", "mango": "m_val", "zebra": "z_val"},
			wantPreserved: []string{"apple", "mango", "zebra"},
		},
		{
			name:          "non-flat orphan is preserved whole under its top-level key",
			template:      "key1: default1\n",
			existing:      "key1: user_val1\nextra:\n  nested: value\n",
			wantPreserved: []string{"extra"},
			contains:      []string{"extra:", "nested: value"},
		},
		{
			name:     "emptied list survives an unrelated set",
			template: listKeyTemplate,
			existing: "require_pr_to_base: []\nsquash: true\n",
			pairs:    []KV{{Key: "squash", Value: "false"}},
			contains: []string{"require_pr_to_base: []"},
		},
		{
			name:     "lengthened list survives an unrelated set",
			template: listKeyTemplate,
			existing: "require_pr_to_base: [a, b]\nsquash: true\n",
			pairs:    []KV{{Key: "squash", Value: "false"}},
			contains: []string{"require_pr_to_base: [a, b]"},
		},
		{
			name:     "list key can be set to the empty list",
			template: listKeyTemplate,
			pairs:    []KV{{Key: "require_pr_to_base", Value: "[]"}},
			contains: []string{"require_pr_to_base: []"},
		},
		{
			name:     "list key can be set to two elements",
			template: listKeyTemplate,
			pairs:    []KV{{Key: "require_pr_to_base", Value: "[main, develop]"}},
			contains: []string{"require_pr_to_base: [main, develop]"},
		},
		{
			name:     "open map adds an entry and rewrites another",
			template: openMapSetTemplate,
			existing: "name: mine\nlabels:\n  a: old\n",
			pairs:    []KV{{Key: "labels.x", Value: "new x"}, {Key: "labels.a", Value: "new a"}},
			openMaps: []string{"labels"},
			contains: []string{"a: new a", "x: new x", "name: mine"},
		},
		{
			name:        "open map entry appended as a flow mapping keeps its scalar tags",
			template:    openMapSetTemplate,
			pairs:       []KV{{Key: "labels.t0", Value: `{command: "go test {targets}", timeout_min: 60, counter: go-json}`}},
			openMaps:    []string{"labels"},
			contains:    []string{`command: "go test {targets}"`, "timeout_min: 60", "counter: go-json"},
			notContains: []string{"'60'", `"60"`},
		},
		{
			name:        "open map flow mapping replaces an existing scalar entry whole",
			template:    openMapSetTemplate,
			existing:    "labels:\n  t0: old\n  keep: me\n",
			pairs:       []KV{{Key: "labels.t0", Value: "{parallel: 4}"}},
			openMaps:    []string{"labels"},
			contains:    []string{"parallel: 4", "keep: me"},
			notContains: []string{"old"},
		},
		{
			name:     "open map entry set to a flow list",
			template: openMapSetTemplate,
			pairs:    []KV{{Key: "labels.push", Value: "[0, 1, 2]"}},
			openMaps: []string{"labels"},
			contains: []string{"push: [0, 1, 2]"},
		},
		{
			name:     "open map flow mapping holds a quoted value with a comma",
			template: openMapSetTemplate,
			pairs:    []KV{{Key: "labels.t0", Value: `{about: "fast, offline"}`}},
			openMaps: []string{"labels"},
			contains: []string{`about: "fast, offline"`},
		},
		{
			name:     "scalar-or-list key takes a quoted flow list",
			template: roundListTemplate,
			pairs:    []KV{{Key: "plan_review", Value: `["opus[medium]", "sonnet[high]"]`}},
			openMaps: []string{"plan_review"},
			contains: []string{`plan_review: ["opus[medium]", "sonnet[high]"] # rounds`},
		},
		{
			name:       "scalar-or-list key holding a list takes a scalar",
			template:   roundListTemplate,
			existing:   "plan_review: [\"opus[high]\"]\n",
			pairs:      []KV{{Key: "plan_review", Value: "sonnet[high]"}},
			openMaps:   []string{"plan_review"},
			wantValues: map[string]string{"plan_review": "sonnet[high]"},
		},
		{
			name:     "scalar-or-list key takes a flow list of name and model mappings",
			template: "advisors: []\n",
			pairs:    []KV{{Key: "advisors", Value: `[{name: critic, model: "sonnet[high]"}]`}},
			openMaps: []string{"advisors"},
			contains: []string{`advisors: [{name: critic, model: "sonnet[high]"}]`},
		},
		{
			name:     "scalar-or-list key takes the empty list",
			template: roundListTemplate,
			pairs:    []KV{{Key: "plan_review", Value: "[]"}},
			openMaps: []string{"plan_review"},
			contains: []string{"plan_review: [] # rounds"},
		},
		{
			name:        "open map on an empty template writes a block mapping",
			template:    openMapSetTemplate,
			pairs:       []KV{{Key: "labels.x", Value: "new x"}},
			openMaps:    []string{"labels"},
			contains:    []string{"  x: new x"},
			notContains: []string{"{"},
		},
		{
			name:        "an integer set over a null leaf reads back as the integer",
			template:    nullBudgetTemplate,
			pairs:       []KV{{Key: "plan_max_bounces", Value: "2"}},
			contains:    []string{"plan_max_bounces: 2"},
			notContains: []string{"!!null"},
			wantInts:    map[string]*int{"plan_max_bounces": intPointer(2)},
		},
		{
			name:        "a spelled null set over an integer leaf reads back as null",
			template:    nullBudgetTemplate,
			existing:    "plan_max_bounces: 2\n",
			pairs:       []KV{{Key: "plan_max_bounces", Value: "~"}},
			contains:    []string{"plan_max_bounces: ~"},
			notContains: []string{"!!int"},
			wantInts:    map[string]*int{"plan_max_bounces": nil},
		},
		{
			name:        "per-name maps append a string name and a list name",
			template:    perNameSetTemplate,
			pairs:       []KV{{Key: "names.x", Value: "new x"}, {Key: "lists.m", Value: "[a, b]"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{"x: new x", "m: [a, b]"},
			notContains: []string{"default a", "l: [x]"},
		},
		{
			name:        "per-name scalar entry starting with a bracket is written as a string",
			template:    perNameSetTemplate,
			pairs:       []KV{{Key: "names.x", Value: "[not a list] prose"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{`x: '[not a list] prose'`},
		},
		{
			name:        "per-name entry the template holds is rewritten",
			template:    perNameSetTemplate,
			existing:    "names:\n  a: tuned a\nlists:\n  l: [x]\n",
			pairs:       []KV{{Key: "lists.l", Value: "[]"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{"a: tuned a", "l: []"},
		},
		{
			name:        "per-name name only the file holds survives an unrelated set",
			template:    perNameSetTemplate,
			existing:    "names:\n  a: default a\n  hub: hub only\nlists:\n  l: [x]\n  hubl: [q]\n",
			pairs:       []KV{{Key: "names.a", Value: "set a"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{"a: set a", "hub: hub only", "hubl: [q]"},
			notContains: []string{"l: [x]"},
		},
		{
			name:        "per-name entry set to its template value is left out",
			template:    perNameSetTemplate,
			existing:    "names:\n  a: tuned a\nlists:\n  l: [y]\n",
			pairs:       []KV{{Key: "names.a", Value: "default a"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{"names: {}", "l: [y]"},
			notContains: []string{"default a", "tuned a"},
		},
		{
			name:        "per-name map holding null becomes a mapping holding the new name",
			template:    perNameSetTemplate,
			existing:    "names:\nlists:\n  l: [x]\n",
			pairs:       []KV{{Key: "names.x", Value: "prose"}},
			perNameMaps: perNameSetMaps,
			contains:    []string{"names:\n    x: prose"},
			notContains: []string{"default a"},
		},
		{
			name:        "per-name mapping entry the template holds takes one key and keeps the rest",
			template:    perNameMappingTemplate,
			pairs:       []KV{{Key: "profiles.costed.budget", Value: "300"}},
			perNameMaps: perNameMappingMaps,
			contains:    []string{"costed:\n        kind: cost\n        budget: 300\n        tags: [a]\n"},
			notContains: []string{"plain"},
		},
		{
			name:        "per-name mapping entry keys absent from the named entry take the template entry's shape",
			template:    perNameMappingTemplate,
			existing:    "profiles:\n  wide:\n    kind: cost\n",
			pairs:       []KV{{Key: "profiles.wide.budget", Value: "250"}, {Key: "profiles.plain.tags", Value: "[b]"}},
			perNameMaps: perNameMappingMaps,
			contains:    []string{"plain:\n        kind: identity\n        tags: [b]\n", "wide:\n        kind: cost\n        budget: 250\n"},
			notContains: []string{"costed"},
		},
		{
			name:        "per-name mapping entry is set whole from a flow mapping",
			template:    perNameMappingTemplate,
			pairs:       []KV{{Key: "profiles.wide", Value: "{kind: identity, budget: 5}"}},
			perNameMaps: perNameMappingMaps,
			contains:    []string{"wide:\n        kind: identity\n        budget: 5\n"},
			notContains: []string{"costed", "plain"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := SetValues([]byte(tt.template), []byte(tt.existing), tt.pairs, tt.perNameMaps, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() unexpected error: %v", err)
			}
			if len(result.Unknown) != 0 {
				t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
			}
			merged := string(result.Merged)
			for key, want := range tt.wantValues {
				assertMergedKeyValue(t, result, key, want)
			}
			if !slices.Equal(result.Preserved, tt.wantPreserved) {
				t.Errorf("SetValues() Preserved = %v; want %v", result.Preserved, tt.wantPreserved)
			}
			if hasMarker := strings.Contains(merged, preservedMarker); hasMarker != (len(tt.wantPreserved) > 0) {
				t.Errorf("SetValues() merged has preserved marker = %v with Preserved = %v; merged = %q", hasMarker, result.Preserved, merged)
			}
			for _, want := range tt.contains {
				if !strings.Contains(merged, want) {
					t.Errorf("SetValues() merged = %q; want it to contain %q", merged, want)
				}
			}
			for _, unwanted := range tt.notContains {
				if strings.Contains(merged, unwanted) {
					t.Errorf("SetValues() merged = %q; want it not to contain %q", merged, unwanted)
				}
			}
			assertDecodesAsInts(t, result.Merged, tt.wantInts)
			previous := -1
			for _, want := range tt.inOrder {
				at := strings.Index(merged, want)
				if at < previous {
					t.Errorf("SetValues() merged = %q; want %q after the entry before it", merged, want)
				}
				previous = at
			}

			again, err := SetValues([]byte(tt.template), result.Merged, tt.pairs, tt.perNameMaps, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() second call unexpected error: %v", err)
			}
			if string(again.Merged) != merged {
				t.Errorf("SetValues() second call Merged = %q; want identical to first call Merged %q", again.Merged, merged)
			}
			if !slices.Equal(again.Preserved, result.Preserved) {
				t.Errorf("SetValues() second call Preserved = %v; want %v", again.Preserved, result.Preserved)
			}
		})
	}
}

// TestSetValues_UnknownKeys pins that an unknown key rejects the whole call: Unknown names it, Merged is nil so no partial mutation is observable, and Known lists what was settable.
func TestSetValues_UnknownKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		template    string
		pairs       []KV
		openMaps    []string
		perNameMaps []string
		wantUnknown []string
		wantKnown   string
	}{
		{
			name:     "valid pair next to an unknown one",
			template: "key1: default1\nkey2: default2\n",
			pairs:    []KV{{Key: "key1", Value: "new1"}, {Key: "bogus", Value: "irrelevant"}},
		},
		{
			name:      "undeclared key next to an open map",
			template:  openMapSetTemplate,
			pairs:     []KV{{Key: "bogus", Value: "v"}},
			openMaps:  []string{"labels"},
			wantKnown: "labels.<name>",
		},
		{
			name:        "per-name map lists its name placeholder as known",
			template:    perNameSetTemplate,
			pairs:       []KV{{Key: "bogus", Value: "v"}},
			perNameMaps: perNameSetMaps,
			wantKnown:   "names.<name>",
		},
		{
			name:        "per-name map refuses a deeper key and itself but knows one name",
			template:    perNameSetTemplate,
			pairs:       []KV{{Key: "names.x", Value: "v"}, {Key: "names.x.y", Value: "v"}, {Key: "lists", Value: "[a]"}, {Key: "lists.m.n", Value: "v"}},
			perNameMaps: perNameSetMaps,
			wantUnknown: []string{"lists", "lists.m.n", "names.x.y"},
		},
		{
			name:        "per-name map of mappings knows an entry key for any name and refuses one no entry holds",
			template:    perNameMappingTemplate,
			pairs:       []KV{{Key: "profiles.wide.budget", Value: "1"}, {Key: "profiles.wide.nope", Value: "1"}},
			perNameMaps: perNameMappingMaps,
			wantUnknown: []string{"profiles.wide.nope"},
			wantKnown:   "profiles.<name>.budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := SetValues([]byte(tt.template), nil, tt.pairs, tt.perNameMaps, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() unexpected error: %v", err)
			}
			wantUnknown := tt.wantUnknown
			if wantUnknown == nil {
				wantUnknown = []string{"bogus"}
			}
			if !slices.Equal(result.Unknown, wantUnknown) {
				t.Errorf("SetValues() Unknown = %v; want %v", result.Unknown, wantUnknown)
			}
			if result.Merged != nil {
				t.Errorf("SetValues() Merged = %q; want nil (no partial mutation)", result.Merged)
			}
			if tt.wantKnown != "" && !slices.Contains(result.Known, tt.wantKnown) {
				t.Errorf("SetValues() Known = %v; want it to contain %q", result.Known, tt.wantKnown)
			}
		})
	}
}

// TestSetValues_Refusals pins the errors SetValues returns for a value that does not fit its key.
func TestSetValues_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		template    string
		existing    string
		pair        KV
		openMaps    []string
		perNameMaps []string
		want        []string
	}{
		{
			name:     "scalar for a list key",
			template: "require_pr_to_base: [\"main\"]\n",
			pair:     KV{Key: "require_pr_to_base", Value: "main"},
			want:     []string{"list key require_pr_to_base"},
		},
		{
			name:     "entry into an open map that holds a list",
			template: openMapSetTemplate,
			existing: "labels:\n  - a\n",
			pair:     KV{Key: "labels.x", Value: "v"},
			openMaps: []string{"labels"},
			want:     []string{"labels", "map of name to description"},
		},
		{
			name:     "unterminated flow mapping for an open map entry",
			template: openMapSetTemplate,
			pair:     KV{Key: "labels.t0", Value: "{a: 1"},
			openMaps: []string{"labels"},
			want:     []string{"labels.t0", "double-quote"},
		},
		{
			name:     "text after the closing brace for an open map entry",
			template: openMapSetTemplate,
			pair:     KV{Key: "labels.t0", Value: "{a: 1} x"},
			openMaps: []string{"labels"},
			want:     []string{"labels.t0", "double-quote"},
		},
		{
			name:     "unquoted bracketed entries for a scalar-or-list key",
			template: roundListTemplate,
			pair:     KV{Key: "plan_review", Value: "[opus[medium], sonnet[high]]"},
			openMaps: []string{"plan_review"},
			want:     []string{"plan_review", `["opus[medium]", "sonnet[high]"]`},
		},
		{
			name:     "nested list for a scalar-or-list key",
			template: roundListTemplate,
			pair:     KV{Key: "plan_review", Value: "[[a], b]"},
			openMaps: []string{"plan_review"},
			want:     []string{"plan_review", "flow list of scalars"},
		},
		{
			name:     "mapping holding a list for a scalar-or-list key",
			template: roundListTemplate,
			pair:     KV{Key: "plan_review", Value: "[{name: [a]}]"},
			openMaps: []string{"plan_review"},
			want:     []string{"plan_review", "mappings of scalars"},
		},
		{
			name:     "text after the closing bracket for a scalar-or-list key",
			template: roundListTemplate,
			pair:     KV{Key: "plan_review", Value: `["a"] trailing`},
			openMaps: []string{"plan_review"},
			want:     []string{"plan_review", "flow list of scalars"},
		},
		{
			name:     "scalar at a mapping open map whose file value is a list",
			template: openMapSetTemplate,
			existing: "labels:\n  - a\n",
			pair:     KV{Key: "labels", Value: "x"},
			openMaps: []string{"labels"},
			want:     []string{"labels"},
		},
		{
			name:        "unterminated flow list for a per-name list entry",
			template:    perNameSetTemplate,
			pair:        KV{Key: "lists.m", Value: "[unterminated"},
			perNameMaps: perNameSetMaps,
			want:        []string{"lists.m"},
		},
		{
			name:        "plain value for a per-name list entry",
			template:    perNameSetTemplate,
			pair:        KV{Key: "lists.m", Value: "plain"},
			perNameMaps: perNameSetMaps,
			want:        []string{"lists.m"},
		},
		{
			name:        "entry into a per-name map that holds a list",
			template:    perNameSetTemplate,
			existing:    "names:\n  - a\n",
			pair:        KV{Key: "names.x", Value: "prose"},
			perNameMaps: perNameSetMaps,
			want:        []string{"names", "sequence", "rewrite names as a map of name to value"},
		},
		{
			name:        "plain value for a per-name mapping entry",
			template:    perNameMappingTemplate,
			pair:        KV{Key: "profiles.wide", Value: "identity"},
			perNameMaps: perNameMappingMaps,
			want:        []string{"profiles.wide", "flow mapping"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := SetValues([]byte(tt.template), []byte(tt.existing), []KV{tt.pair}, tt.perNameMaps, tt.openMaps...)
			if err == nil {
				t.Fatal("SetValues() = nil error; want a refusal")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q; want it to contain %q", err, want)
				}
			}
		})
	}
}

// assertMergedKeyValue is a test helper that unmarshals result.Merged into a
// map and asserts the given top-level key holds want.
func assertMergedKeyValue(t *testing.T, result SetResult, key, want string) {
	t.Helper()
	got := extractYAMLValue(t, result.Merged, key)
	if got != want {
		t.Errorf("SetValues() merged[%q] = %q; want %q", key, got, want)
	}
}

// TestSetEntries pins whole-entry sets: a new entry is appended, an existing one replaced whole with its key's comment kept, the other entries and comments untouched, and a value that is not a JSON object or an empty name refused.
func TestSetEntries(t *testing.T) {
	t.Parallel()
	const existing = "# head\nlight:\n  loom: {plan_max_bounces: 1}\n# about docs\ndocs:\n  darn: {writer: sonnet}\n"
	tests := []struct {
		name     string
		existing string
		pairs    []KV
		want     string
		wantErr  []string
	}{
		{
			name:     "a new entry is appended",
			existing: existing,
			pairs:    []KV{{Key: "wide", Value: `{"darn":{"review_max_bounces":2}}`}},
			want:     "# head\nlight:\n    loom: {plan_max_bounces: 1}\n# about docs\ndocs:\n    darn: {writer: sonnet}\nwide:\n    darn: {review_max_bounces: 2}\n",
		},
		{
			name:     "an existing entry is replaced whole",
			existing: existing,
			pairs:    []KV{{Key: "docs", Value: `{"loom":{"merge_max_bounces":1}}`}},
			want:     "# head\nlight:\n    loom: {plan_max_bounces: 1}\n# about docs\ndocs:\n    loom: {merge_max_bounces: 1}\n",
		},
		{
			name:  "a string needing quotes stays a string",
			pairs: []KV{{Key: "docs", Value: `{"darn":{"writer":"sonnet[medium]","note":"2"}}`}},
			want:  "docs:\n    darn: {writer: 'sonnet[medium]', note: \"2\"}\n",
		},
		{
			name:  "an empty document takes the entry",
			pairs: []KV{{Key: "light", Value: `{}`}},
			want:  "light: {}\n",
		},
		{
			name:     "a value that is not a JSON object is refused",
			existing: existing,
			pairs:    []KV{{Key: "light", Value: `[1]`}},
			wantErr:  []string{"light", "JSON object"},
		},
		{
			name:     "an empty name is refused",
			existing: existing,
			pairs:    []KV{{Key: "", Value: `{}`}},
			wantErr:  []string{"entry name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SetEntries([]byte(tt.existing), tt.pairs)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("SetEntries() = %q; want an error", got)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("SetEntries() error = %q; want it to name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("SetEntries() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("SetEntries() = %q; want %q", got, tt.want)
			}
		})
	}
}

// extractYAMLValue is a minimal top-level-key extractor for asserting a
// single scalar value out of merged YAML bytes without pulling in a full
// YAML-to-map round-trip (which would normalize quoting and defeat the
// byte-for-byte assertions this test file makes).
func extractYAMLValue(t *testing.T, data []byte, key string) string {
	t.Helper()
	prefix := key + ": "
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("key %q not found in merged YAML: %q", key, string(data))
	return ""
}
