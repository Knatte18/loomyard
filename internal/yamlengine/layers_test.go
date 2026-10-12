// layers_test.go contains table-driven tests for the layer operations: Merge, Strip, SetSparse, Prune, LeafPaths and AnnotateLeaves.

package yamlengine

import (
	"slices"
	"testing"
)

// TestMerge pins the depth merge of upper over lower: scalars and lists replace whole, a mapping both hold merges, an open map or a whole entry is carried whole, comments and order come from lower, and an empty side yields the other.
func TestMerge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		lower, upper string
		openMaps     []string
		wholeEntries bool
		want         string
		wantErr      bool
	}{
		{name: "mapping merges key by key", lower: "a:\n    x: 1\n    y: 2\nb: 3\n", upper: "a:\n    y: 9\n    z: 4\nc: 5\n", want: "a:\n    x: 1\n    y: 9\n    z: 4\nb: 3\nc: 5\n"},
		{name: "scalar and list replace whole", lower: "l: [a, b, c]\ns: 1\nkeep: k\n", upper: "l: [x]\ns: 2\n", want: "l: [x]\ns: 2\nkeep: k\n"},
		{name: "open map is carried whole", lower: "labels:\n    a: x\n    b: y\n", upper: "labels:\n    c: z\n", openMaps: []string{"labels"}, want: "labels:\n    c: z\n"},
		{name: "whole entry is carried whole", lower: "m:\n    a: 1\n    b: 2\n", upper: "m:\n    a: 9\n", wholeEntries: true, want: "m:\n    a: 9\n"},
		{name: "comments and order come from lower", lower: "# head\na: 1 # note\n", upper: "b: 2\na: 3\n", want: "# head\na: 3 # note\nb: 2\n"},
		{name: "empty upper yields lower", lower: "a: 1\n", upper: "", want: "a: 1\n"},
		{name: "comment-only upper yields lower", lower: "a: 1\n", upper: "# nothing\n", want: "a: 1\n"},
		{name: "empty lower yields upper", lower: "", upper: "a: 1\n", want: "a: 1\n"},
		{name: "both empty yields empty", lower: "", upper: "# c\n", want: ""},
		{name: "empty flow mapping takes the new keys", lower: "a: {}\n", upper: "a:\n    x: 1\n", want: "a:\n    x: 1\n"},
		{name: "non-mapping root is an error", lower: "a: 1\n", upper: "- x\n", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Merge([]byte(tc.lower), []byte(tc.upper), tc.openMaps, tc.wholeEntries)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Merge error = %v, wantErr %v", err, tc.wantErr)
			}
			if string(got) != tc.want {
				t.Errorf("Merge = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStrip pins which upper units Strip removes: equal ones, with a marker equal only to the same marker text, an open map kept whole when any entry differs, emptied mappings removed with their key, and the all-stripped empty result.
func TestStrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		lower, upper string
		openMaps     []string
		wholeEntries bool
		want         string
		wantRemoved  []string
	}{
		{name: "equal scalar removed", lower: "a: 1\nb: 2\n", upper: "a: 1\nb: 3\n", want: "b: 3\n", wantRemoved: []string{"a"}},
		{name: "same marker removed", lower: "p: ${env:X}\n", upper: "p: ${env:X}\n", want: "", wantRemoved: []string{"p"}},
		{name: "marker over literal kept", lower: "p: /tmp\n", upper: "p: ${env:X}\n", want: "p: ${env:X}\n", wantRemoved: []string{}},
		{name: "literal over marker kept", lower: "p: ${env:X}\n", upper: "p: /tmp\n", want: "p: /tmp\n", wantRemoved: []string{}},
		{name: "same text with another tag kept", lower: "p: 1\n", upper: "p: \"1\"\n", want: "p: \"1\"\n", wantRemoved: []string{}},
		{name: "equal list removed and shorter list kept", lower: "a: [x, y]\nb: [x, y]\n", upper: "a: [x, y]\nb: [x]\n", want: "b: [x]\n", wantRemoved: []string{"a"}},
		{name: "open map equal except one entry is kept whole", lower: "labels:\n    a: x\n    b: y\n", upper: "labels:\n    a: x\n    b: z\n", openMaps: []string{"labels"}, want: "labels:\n    a: x\n    b: z\n", wantRemoved: []string{}},
		{name: "equal open map removed", lower: "labels:\n    a: x\n", upper: "labels:\n    a: x\n", openMaps: []string{"labels"}, want: "", wantRemoved: []string{"labels"}},
		{name: "emptied nested mapping removed", lower: "a:\n    b:\n        c: 1\nk: 2\n", upper: "a:\n    b:\n        c: 1\nk: 3\n", want: "k: 3\n", wantRemoved: []string{"a.b.c"}},
		{name: "partly equal mapping keeps the rest", lower: "a:\n    x: 1\n    y: 2\n", upper: "a:\n    x: 1\n    y: 9\n", want: "a:\n    y: 9\n", wantRemoved: []string{"a.x"}},
		{name: "all stripped yields empty", lower: "a: 1\nb:\n    c: 2\n", upper: "b:\n    c: 2\na: 1\n", want: "", wantRemoved: []string{"a", "b.c"}},
		{name: "whole entry compared whole", lower: "m:\n    a: 1\n    b: 2\n", upper: "m:\n    a: 1\n", wholeEntries: true, want: "m:\n    a: 1\n", wantRemoved: []string{}},
		{name: "equal whole entry removed", lower: "m:\n    a: 1\n", upper: "m:\n    a: 1\n", wholeEntries: true, want: "", wantRemoved: []string{"m"}},
		{name: "empty lower removes nothing", lower: "", upper: "a: 1\n", want: "a: 1\n", wantRemoved: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, removed, err := Strip([]byte(tc.lower), []byte(tc.upper), tc.openMaps, tc.wholeEntries)
			if err != nil {
				t.Fatalf("Strip: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Strip = %q, want %q", got, tc.want)
			}
			if !slices.Equal(removed, tc.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

const sparseTemplate = "name: tmpl\nport: 80\nnested:\n    deep: 1\nlabels: {}\nlist: [a]\n"

// TestSetSparse pins that SetSparse writes only the set keys: into empty bytes, beside an existing key, through intermediate mappings, as an open-map entry and as a flow list, with a template tag kept and an unknown or misshapen request refused.
func TestSetSparse(t *testing.T) {
	t.Parallel()
	openMaps := []string{"labels"}
	tests := []struct {
		name        string
		existing    string
		pairs       []KV
		want        string
		wantUnknown []string
		wantErr     bool
	}{
		{name: "set into empty bytes writes only that key", pairs: []KV{{Key: "name", Value: "x"}}, want: "name: x\n"},
		{name: "set into a comment-only file", existing: "# c\n", pairs: []KV{{Key: "name", Value: "x"}}, want: "name: x\n"},
		{name: "set beside an existing key", existing: "port: 90\n", pairs: []KV{{Key: "name", Value: "x"}}, want: "port: 90\nname: x\n"},
		{name: "set replaces an existing value", existing: "name: old\n", pairs: []KV{{Key: "name", Value: "x"}}, want: "name: x\n"},
		{name: "set creates the intermediate mapping", pairs: []KV{{Key: "nested.deep", Value: "5"}}, want: "nested:\n    deep: 5\n"},
		{name: "int tag is kept from the template", pairs: []KV{{Key: "port", Value: "81"}}, want: "port: 81\n"},
		{name: "string tag quotes a number-like value", pairs: []KV{{Key: "name", Value: "81"}}, want: "name: \"81\"\n"},
		{name: "open-map entry", pairs: []KV{{Key: "labels.bug", Value: "desc"}}, want: "labels:\n    bug: desc\n"},
		{name: "open-map entry beside another", existing: "labels:\n    a: x\n", pairs: []KV{{Key: "labels.bug", Value: "desc"}}, want: "labels:\n    a: x\n    bug: desc\n"},
		{name: "list value", pairs: []KV{{Key: "list", Value: "[x, y]"}}, want: "list: [x, y]\n"},
		{name: "empty list value", pairs: []KV{{Key: "list", Value: "[]"}}, want: "list: []\n"},
		{name: "unknown key is reported and nothing written", pairs: []KV{{Key: "name", Value: "x"}, {Key: "nope", Value: "1"}}, wantUnknown: []string{"nope"}},
		{name: "list key refuses a scalar", pairs: []KV{{Key: "list", Value: "foo"}}, wantErr: true},
		{name: "open map holding a list is refused", existing: "labels: [a]\n", pairs: []KV{{Key: "labels.bug", Value: "desc"}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SetSparse([]byte(sparseTemplate), []byte(tc.existing), tc.pairs, openMaps...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SetSparse error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !slices.Equal(got.Unknown, tc.wantUnknown) {
				t.Fatalf("Unknown = %v, want %v", got.Unknown, tc.wantUnknown)
			}
			for _, key := range []string{"labels.<name>", "list", "name", "nested.deep", "port"} {
				if !slices.Contains(got.Known, key) {
					t.Errorf("Known = %v, lacks %q", got.Known, key)
				}
			}
			if len(got.Preserved) != 0 {
				t.Errorf("Preserved = %v, want none", got.Preserved)
			}
			if tc.wantUnknown != nil {
				if got.Merged != nil {
					t.Errorf("Merged = %q, want nil with unknown keys", got.Merged)
				}
				return
			}
			if string(got.Merged) != tc.want {
				t.Errorf("Merged = %q, want %q", got.Merged, tc.want)
			}
		})
	}
}

// TestPrune pins that Prune removes only units the template does not know: a retired key, an unknown nested key, a retired mapping unit by unit, an emptied parent with its key; a key under an open map stays, no template key is added, and an untouched file comes back byte for byte.
func TestPrune(t *testing.T) {
	t.Parallel()
	const template = "name: tmpl\nnested:\n    deep: 1\nlabels: {}\nlist: [a]\n"
	tests := []struct {
		name        string
		existing    string
		want        string
		wantRemoved []string
	}{
		{name: "retired key removed", existing: "name: x\nold: 1\n", want: "name: x\n", wantRemoved: []string{"old"}},
		{name: "open-map entry kept", existing: "labels:\n    anything: q\n", want: "labels:\n    anything: q\n", wantRemoved: []string{}},
		{name: "no template key added", existing: "name: x\n", want: "name: x\n", wantRemoved: []string{}},
		{name: "unknown nested key removed", existing: "nested:\n    deep: 2\n    gone: 3\n", want: "nested:\n    deep: 2\n", wantRemoved: []string{"nested.gone"}},
		{name: "retired mapping removed unit by unit", existing: "name: x\nold:\n    a: 1\n    b: 2\n", want: "name: x\n", wantRemoved: []string{"old.a", "old.b"}},
		{name: "emptied parent removed with its key", existing: "nested:\n    gone: 1\n", want: "", wantRemoved: []string{"nested.gone"}},
		{name: "list kept", existing: "list: [x, y, z]\n", want: "list: [x, y, z]\n", wantRemoved: []string{}},
		{name: "empty existing stays empty", existing: "", want: "", wantRemoved: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, removed, err := Prune([]byte(template), []byte(tc.existing), "labels")
			if err != nil {
				t.Fatalf("Prune: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Prune = %q, want %q", got, tc.want)
			}
			if !slices.Equal(removed, tc.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

// TestLeafPaths pins the unit paths of a document under both flags: nested keys are descended, a list and an empty mapping are one unit, an open map is one unit, and wholeEntries makes every top-level key one unit.
func TestLeafPaths(t *testing.T) {
	t.Parallel()
	const doc = "z: 1\nb:\n    d: [x]\n    c: 2\nlabels:\n    k: v\ne: {}\n"
	tests := []struct {
		name         string
		doc          string
		openMaps     []string
		wholeEntries bool
		want         []string
	}{
		{name: "descends every mapping", doc: doc, want: []string{"b.c", "b.d", "e", "labels.k", "z"}},
		{name: "open map is one unit", doc: doc, openMaps: []string{"labels"}, want: []string{"b.c", "b.d", "e", "labels", "z"}},
		{name: "whole entries are top-level units", doc: doc, openMaps: []string{"labels"}, wholeEntries: true, want: []string{"b", "e", "labels", "z"}},
		{name: "empty document has no units", doc: "# c\n", want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := LeafPaths([]byte(tc.doc), tc.openMaps, tc.wholeEntries)
			if err != nil {
				t.Fatalf("LeafPaths: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("LeafPaths = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAnnotateLeaves pins that a note lands as the line comment of its unit under both flags, a mapping unit carrying it on its key, a missing or non-unit path ignored and a leading # kept as given.
func TestAnnotateLeaves(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		doc          string
		notes        map[string]string
		openMaps     []string
		wholeEntries bool
		want         string
	}{
		{name: "scalar and list units", doc: "a: 1\nb:\n    c: 2\n    d: [x]\n", notes: map[string]string{"a": "note a", "b.c": "note c", "b.d": "note d"}, want: "a: 1 # note a\nb:\n    c: 2 # note c\n    d: [x] # note d\n"},
		{name: "missing path ignored", doc: "a: 1\n", notes: map[string]string{"zz": "x"}, want: "a: 1\n"},
		{name: "non-unit mapping path ignored", doc: "b:\n    c: 2\n", notes: map[string]string{"b": "x"}, want: "b:\n    c: 2\n"},
		{name: "open map unit", doc: "labels:\n    k: v\n", notes: map[string]string{"labels": "note"}, openMaps: []string{"labels"}, want: "labels: # note\n    k: v\n"},
		{name: "whole entry unit", doc: "b:\n    c: 2\n", notes: map[string]string{"b": "note"}, wholeEntries: true, want: "b: # note\n    c: 2\n"},
		{name: "leading hash kept", doc: "a: 1\n", notes: map[string]string{"a": "# as is"}, want: "a: 1 # as is\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := AnnotateLeaves([]byte(tc.doc), tc.notes, tc.openMaps, tc.wholeEntries)
			if err != nil {
				t.Fatalf("AnnotateLeaves: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("AnnotateLeaves = %q, want %q", got, tc.want)
			}
		})
	}
}
