// reconcile_test.go contains table-driven tests for Reconcile and MissingKeys.

package yamlengine

import (
	"slices"
	"strings"
	"testing"
)

const (
	openMapTemplate = "name: tmpl\nlabels:\n  a: default a\n  b: default b\n"
	perNameTemplate = "name: tmpl\nlenses:\n  a: default a\n  b: default b\nfans:\n  f: [a, b]\n"
)

// sortedCopy returns a sorted copy of keys, so a comparison ignores report order.
func sortedCopy(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return out
}

// TestReconcile pins the merge: added and removed report template-only and file-only key paths, user values, template comments and template key order survive in merged, lists, open maps and per-name map entries are carried whole, a per-name map holds only the names the file changed, a name only the file holds under a per-name map is kept and reported in kept, a template-equal name is left out and reported in dropped, and reconciling the merged output again changes nothing.
func TestReconcile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		template    string
		existing    string
		openMaps    []string
		perNameMaps []string
		wantAdded   []string
		wantRemoved []string
		wantKept    []string
		wantDropped []string
		wantErr     []string
		contains    []string
		notContains []string
		inOrder     []string
	}{
		{
			name:      "missing key is added",
			template:  "\nkey1: default_value\n",
			existing:  "",
			wantAdded: []string{"key1"},
			contains:  []string{"key1: default_value"},
		},
		{
			name:        "stale key is removed",
			template:    "\nkey1: default\n",
			existing:    "\nkey1: user_value\nkey2: stale_value\n",
			wantRemoved: []string{"key2"},
			contains:    []string{"key1: user_value"},
			notContains: []string{"stale_value"},
		},
		{
			name:        "user value beats the template default",
			template:    "\nkey1: template_default\n",
			existing:    "\nkey1: user_custom_value\n",
			contains:    []string{"key1: user_custom_value"},
			notContains: []string{"template_default"},
		},
		{
			name:        "nested add, remove and preserve",
			template:    "\nlevel1:\n  level2:\n    kept_key: template_val1\n    added_key: template_val2\n",
			existing:    "\nlevel1:\n  level2:\n    kept_key: user_val1\n    extra_key: extra_val\n",
			wantAdded:   []string{"level1.level2.added_key"},
			wantRemoved: []string{"level1.level2.extra_key"},
			contains:    []string{"kept_key: user_val1"},
		},
		{
			name:      "empty existing adds every template key",
			template:  "\nkey1: val1\nkey2: val2\n",
			existing:  "",
			wantAdded: []string{"key1", "key2"},
			contains:  []string{"key1: val1", "key2: val2"},
		},
		{
			name:      "comments-only existing counts as empty",
			template:  "\nkey1: val1\n",
			existing:  "\n# This is just a comment\n# No actual config\n",
			wantAdded: []string{"key1"},
		},
		{
			name:     "template comments and key order survive",
			template: "\n# Key 1 comment\nkey1: template_val1\n# Key 2 comment\nkey2: template_val2\n",
			existing: "\nkey2: user_val2\nkey1: user_val1\n",
			contains: []string{"# Key 1 comment", "# Key 2 comment", "key1: user_val1", "key2: user_val2"},
			inOrder:  []string{"key1: user_val1", "key2: user_val2"},
		},
		{
			name:      "partial existing keeps its value and adds the rest",
			template:  "\nkey1: value1\nkey2: value2\n",
			existing:  "\nkey1: custom_value\n",
			wantAdded: []string{"key2"},
			contains:  []string{"key1: custom_value", "key2: value2"},
		},
		{
			name:     "emptied list is carried whole",
			template: "require_pr_to_base: [\"main\"]\nsquash: true\n",
			existing: "require_pr_to_base: []\nsquash: true\n",
			contains: []string{"require_pr_to_base: []"},
		},
		{
			name:     "lengthened list is carried whole",
			template: "require_pr_to_base: [\"main\"]\nsquash: true\n",
			existing: "require_pr_to_base: [a, b]\nsquash: true\n",
			contains: []string{"require_pr_to_base: [a, b]"},
		},
		{
			name:        "open map keeps the file's keys and adds none",
			template:    openMapTemplate,
			existing:    "name: mine\nlabels:\n  x: my x\n  a: my a\n",
			openMaps:    []string{"labels"},
			contains:    []string{"name: mine", "x: my x", "a: my a"},
			notContains: []string{"default"},
		},
		{
			name:      "open map missing is filled whole and reported as its path",
			template:  openMapTemplate,
			existing:  "name: mine\n",
			openMaps:  []string{"labels"},
			wantAdded: []string{"labels"},
			contains:  []string{"a: default a", "b: default b"},
		},
		{
			name:      "open map carries a list whole",
			template:  openMapTemplate,
			existing:  "labels:\n  - x\n  - y\n",
			openMaps:  []string{"labels"},
			wantAdded: []string{"name"},
			contains:  []string{"- x", "- y"},
		},
		{
			name:        "open map carries a scalar whole beside a template list",
			template:    "advisors: [\"sonnet[high]\"]\nname: base\n",
			existing:    "advisors: sonnet[low]\nname: base\n",
			openMaps:    []string{"advisors"},
			contains:    []string{"advisors: sonnet[low]"},
			notContains: []string{"sonnet[high]"},
		},
		{
			name:        "open map carries an empty string whole beside a template list",
			template:    "advisors: [\"sonnet[high]\"]\nname: base\n",
			existing:    "advisors: \"\"\nname: base\n",
			openMaps:    []string{"advisors"},
			contains:    []string{"advisors: \"\""},
			notContains: []string{"sonnet[high]"},
		},
		{
			name:        "open map carries null whole beside a template list",
			template:    "advisors: [\"sonnet[high]\"]\nname: base\n",
			existing:    "advisors:\nname: base\n",
			openMaps:    []string{"advisors"},
			contains:    []string{"advisors:"},
			notContains: []string{"sonnet[high]"},
		},
		{
			name:        "undeclared mapping still reports its keys",
			template:    openMapTemplate,
			existing:    "name: mine\nlabels:\n  x: my x\n  a: my a\n",
			wantAdded:   []string{"labels.b"},
			wantRemoved: []string{"labels.x"},
		},
		{
			name:        "per-name map leaves out a template name the file lacks and drops a name equal to the template",
			template:    perNameTemplate,
			existing:    "name: mine\nlenses:\n  a: my a\nfans:\n  f: [a, b]\n",
			perNameMaps: []string{"fans", "lenses"},
			wantDropped: []string{"fans.f"},
			contains:    []string{"a: my a", "fans: {}"},
			notContains: []string{"default b", "[a, b]"},
		},
		{
			name:        "per-name map drops a name whose value equals the template's in another style",
			template:    perNameTemplate,
			existing:    "name: mine\nlenses:\n  a: 'default a'\n  b: >-\n    default b\nfans:\n  f:\n    - a\n    - b\n",
			perNameMaps: []string{"fans", "lenses"},
			wantDropped: []string{"fans.f", "lenses.a", "lenses.b"},
			contains:    []string{"lenses: {}", "fans: {}"},
			notContains: []string{"default"},
		},
		{
			name:        "per-name map keeps a file-only name with its value and reports it kept",
			template:    perNameTemplate,
			existing:    "name: mine\nlenses:\n  z: my z\n  b: my b\n  a: my a\nfans:\n  f: [a, b]\n  g: [x]\n",
			perNameMaps: []string{"fans", "lenses"},
			wantKept:    []string{"fans.g", "lenses.z"},
			wantDropped: []string{"fans.f"},
			contains:    []string{"z: my z", "g: [x]"},
			notContains: []string{"[a, b]"},
			inOrder:     []string{"a: my a", "b: my b", "z: my z"},
		},
		{
			name:        "per-name map keeps a tuned value and a shorter list",
			template:    perNameTemplate,
			existing:    "lenses:\n  a: tuned a\n  b: default b\nfans:\n  f: [a]\n",
			perNameMaps: []string{"fans", "lenses"},
			wantAdded:   []string{"name"},
			wantDropped: []string{"lenses.b"},
			contains:    []string{"a: tuned a", "f: [a]"},
			notContains: []string{"default a", "default b", "[a, b]"},
		},
		{
			name:        "per-name map the file lacks is written empty and reported added",
			template:    perNameTemplate,
			existing:    "name: mine\n",
			perNameMaps: []string{"fans", "lenses"},
			wantAdded:   []string{"fans", "lenses"},
			contains:    []string{"lenses: {}", "fans: {}"},
			notContains: []string{"default a", "[a, b]"},
		},
		{
			name:        "per-name map holding null is written empty and reported added",
			template:    perNameTemplate,
			existing:    "name: mine\nlenses:\nfans: {}\n",
			perNameMaps: []string{"fans", "lenses"},
			wantAdded:   []string{"lenses"},
			contains:    []string{"lenses: {}", "fans: {}"},
			notContains: []string{"default a"},
		},
		{
			name:        "per-name map emptied under a key carrying a line comment stays valid YAML",
			template:    "name: tmpl\nlenses: # the lenses\n  a: default a\n",
			existing:    "name: mine\nlenses:\n  a: default a\n",
			perNameMaps: []string{"lenses"},
			wantDropped: []string{"lenses.a"},
			contains:    []string{"lenses: {} # the lenses\n"},
		},
		{
			name:        "per-name map holding a list is refused with the way forward",
			template:    perNameTemplate,
			existing:    "name: mine\nlenses: [a]\n",
			perNameMaps: []string{"lenses"},
			wantErr:     []string{"lenses", "sequence", "rewrite lenses as a map of name to value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			merged, added, removed, kept, dropped, err := Reconcile([]byte(tt.template), []byte(tt.existing), tt.perNameMaps, tt.openMaps...)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Reconcile() = %q; want an error", merged)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Reconcile() error = %q; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Reconcile() unexpected error: %v", err)
			}
			if !slices.Equal(sortedCopy(added), tt.wantAdded) {
				t.Errorf("Reconcile() added = %v; want %v", added, tt.wantAdded)
			}
			if !slices.Equal(sortedCopy(removed), tt.wantRemoved) {
				t.Errorf("Reconcile() removed = %v; want %v", removed, tt.wantRemoved)
			}
			if !slices.Equal(kept, tt.wantKept) {
				t.Errorf("Reconcile() kept = %v; want %v", kept, tt.wantKept)
			}
			if !slices.Equal(dropped, tt.wantDropped) {
				t.Errorf("Reconcile() dropped = %v; want %v", dropped, tt.wantDropped)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(merged), want) {
					t.Errorf("Reconcile() merged = %q; want it to contain %q", merged, want)
				}
			}
			for _, unwanted := range tt.notContains {
				if strings.Contains(string(merged), unwanted) {
					t.Errorf("Reconcile() merged = %q; want it not to contain %q", merged, unwanted)
				}
			}
			previous := -1
			for _, want := range tt.inOrder {
				at := strings.Index(string(merged), want)
				if at < previous {
					t.Errorf("Reconcile() merged = %q; want %q after the entry before it", merged, want)
				}
				previous = at
			}

			again, addedAgain, removedAgain, keptAgain, droppedAgain, err := Reconcile([]byte(tt.template), merged, tt.perNameMaps, tt.openMaps...)
			if err != nil {
				t.Fatalf("Reconcile() second call unexpected error: %v", err)
			}
			if !slices.Equal(keptAgain, tt.wantKept) {
				t.Errorf("Reconcile() second call kept = %v; want %v", keptAgain, tt.wantKept)
			}
			if strings.TrimSpace(string(again)) != strings.TrimSpace(string(merged)) {
				t.Errorf("Reconcile() is not idempotent: second merged = %q; want %q", again, merged)
			}
			if len(addedAgain) != 0 || len(removedAgain) != 0 || len(droppedAgain) != 0 {
				t.Errorf("Reconcile() second call added = %v, removed = %v, dropped = %v; want all empty", addedAgain, removedAgain, droppedAgain)
			}
		})
	}
}

// TestMissingKeys pins which template key paths a file lacks: an empty value counts as present, a list is a default rather than a minimum length, and an open map satisfies every leaf under it.
func TestMissingKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		existing string
		openMaps []string
		want     []string
	}{
		{"empty existing", "key1: v1\nkey2: v2", "", nil, []string{"key1", "key2"}},
		{"partial overlap", "a: 1\nb: 2\nc: 3", "b: 20", nil, []string{"a", "c"}},
		{"all present", "a: 1\nb: 2", "a: 10\nb: 20", nil, nil},
		{"empty value counts as present", "key1: template_val\nkey2: default_val\n", "key1: \"\"\nkey2: user_val\n", nil, nil},
		{
			"nested",
			"level1:\n  level2:\n    key_a: val_a\n    key_b: val_b\n",
			"level1:\n  level2:\n    key_a: user_val_a\n",
			nil,
			[]string{"level1.level2.key_b"},
		},
		{
			"emptied list",
			"require_pr_to_base: [\"main\"]\nsquash: true\n",
			"require_pr_to_base: []\nsquash: true\n",
			nil,
			nil,
		},
		{"shortened list", "bases: [\"a\", \"b\", \"c\"]\n", "bases: [\"a\"]\n", nil, nil},
		{"lengthened list", "bases: [\"a\"]\n", "bases: [\"a\", \"b\", \"c\"]\n", nil, nil},
		{
			"absent list key is still missing",
			"require_pr_to_base: [\"main\"]\nsquash: true\n",
			"squash: true\n",
			nil,
			[]string{"require_pr_to_base[0]"},
		},
		{
			"absent scalar key is still missing",
			"require_pr_to_base: [\"main\"]\nsquash: true\n",
			"require_pr_to_base: []\n",
			nil,
			[]string{"squash"},
		},
		{"nested emptied list", "outer:\n  inner: [\"x\"]\n", "outer:\n  inner: []\n", nil, nil},
		{
			"nested absent list key is still missing",
			"outer:\n  inner: [\"x\"]\n  other: 1\n",
			"outer:\n  other: 1\n",
			nil,
			[]string{"outer.inner[0]"},
		},
		{"open map present satisfies the leaves under it", openMapTemplate, "name: mine\nlabels:\n  x: my x\n", []string{"labels"}, nil},
		{"open map absent reports its leaves", openMapTemplate, "name: mine\n", []string{"labels"}, []string{"labels.a", "labels.b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := MissingKeys([]byte(tt.template), []byte(tt.existing), tt.openMaps...)
			if err != nil {
				t.Fatalf("MissingKeys() unexpected error: %v", err)
			}
			if !slices.Equal(sortedCopy(got), tt.want) {
				t.Errorf("MissingKeys() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestSequenceBasePath covers the split MissingKeys keys its sequence-element rule off, including
// the shapes that must NOT be read as an element.
func TestSequenceBasePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		path        string
		wantBase    string
		wantElement bool
	}{
		{"TopLevelElement", "bases[0]", "bases", true},
		{"NestedElement", "outer.inner[12]", "outer.inner", true},
		{"PlainScalar", "squash", "", false},
		{"NestedScalar", "outer.inner", "", false},
		{"NoIndexDigits", "bases[]", "", false},
		{"NonNumericIndex", "bases[a]", "", false},
		{"NoOwningKey", "[0]", "", false},
		{"UnclosedBracket", "bases[0", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base, isElement := sequenceBasePath(tt.path)
			if base != tt.wantBase || isElement != tt.wantElement {
				t.Errorf("sequenceBasePath(%q) = (%q, %v); want (%q, %v)", tt.path, base, isElement, tt.wantBase, tt.wantElement)
			}
		})
	}
}
