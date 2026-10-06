// fill_test.go covers FillMissing.

package yamlengine

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fillTemplate = `name: tmpl
selvage:
  height_rows: 10
  width: 5
status_line:
  enabled: true
`

func decodeMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("decode: %v\n%s", err, data)
	}
	return m
}

// rootKeyOrder returns the top-level key names of data in document order.
func rootKeyOrder(t *testing.T, data []byte) []string {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	var order []string
	root := node.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		order = append(order, root.Content[i].Value)
	}
	return order
}

// TestFillMissing pins the merge: missing keys are inserted whole and reported by path, and every value, shape and byte the file already holds survives.
// A row sets wantBytes to pin the merged bytes exactly, wantMap to pin the decoded document, and wantOrder to pin the top-level key order.
func TestFillMissing(t *testing.T) {
	t.Parallel()
	noOpExisting := "# keep me\nname:   spaced   # trailing\nselvage:\n    height_rows: 1\n    width: 2\nstatus_line:\n  enabled: no\n"
	listElementExisting := "items:\n  - name: b\n"
	openMapListExisting := "name: x\nlabels:\n  - a\n"

	tests := []struct {
		name      string
		template  string
		existing  string
		openMaps  []string
		wantKeys  []string
		wantBytes string
		wantMap   map[string]any
		wantOrder []string
	}{
		{
			name:     "top-level key",
			template: "a: 1\nb: 2\n",
			existing: "a: 9\n",
			wantKeys: []string{"b"},
			wantMap:  map[string]any{"a": 9, "b": 2},
		},
		{
			name:     "nested key",
			template: fillTemplate,
			existing: "name: mine\nselvage:\n  width: 7\nstatus_line:\n  enabled: false\n",
			wantKeys: []string{"selvage.height_rows"},
			wantMap: map[string]any{
				"name":        "mine",
				"selvage":     map[string]any{"height_rows": 10, "width": 7},
				"status_line": map[string]any{"enabled": false},
			},
		},
		{
			name:     "whole mapping",
			template: fillTemplate,
			existing: "name: x\nselvage:\n  height_rows: 1\n  width: 2\n",
			wantKeys: []string{"status_line"},
			wantMap: map[string]any{
				"name":        "x",
				"selvage":     map[string]any{"height_rows": 1, "width": 2},
				"status_line": map[string]any{"enabled": true},
			},
		},
		{
			name:     "empty and null values are kept",
			template: "a: x\nb: y\nc: z\n",
			existing: "a: \"\"\nb:\n",
			wantKeys: []string{"c"},
			wantMap:  map[string]any{"a": "", "b": nil, "c": "z"},
		},
		{
			name:     "emptied list is kept",
			template: "items:\n  - a\n  - b\nother: 1\n",
			existing: "items: []\n",
			wantKeys: []string{"other"},
			wantMap:  map[string]any{"items": []any{}, "other": 1},
		},
		{
			name:      "file's extra key and order are kept",
			template:  "a: 1\nb: 2\nc: 3\n",
			existing:  "z: 0\nc: 30\n",
			wantKeys:  []string{"a", "b"},
			wantOrder: []string{"z", "c", "a", "b"},
		},
		{
			name:      "empty existing returns the template",
			template:  "b: 2\na: 1\n",
			existing:  "",
			wantKeys:  []string{"a", "b"},
			wantBytes: "b: 2\na: 1\n",
		},
		{
			name:      "whitespace-only existing returns the template",
			template:  "b: 2\na: 1\n",
			existing:  "   \n",
			wantKeys:  []string{"a", "b"},
			wantBytes: "b: 2\na: 1\n",
		},
		{
			name:      "comments-only existing returns the template",
			template:  "b: 2\na: 1\n",
			existing:  "# just a comment\n",
			wantKeys:  []string{"a", "b"},
			wantBytes: "b: 2\na: 1\n",
		},
		{
			name:      "nothing missing returns the file byte for byte",
			template:  fillTemplate,
			existing:  noOpExisting,
			wantBytes: noOpExisting,
		},
		{
			name:      "key missing inside a list element is left for MissingKeys",
			template:  "items:\n  - name: a\n    extra: 1\n",
			existing:  listElementExisting,
			wantBytes: listElementExisting,
		},
		{
			name:      "open map present in any shape is skipped",
			template:  "name: tmpl\nlabels:\n  a: default\n",
			existing:  openMapListExisting,
			openMaps:  []string{"labels"},
			wantBytes: openMapListExisting,
		},
		{
			name:     "open map missing is appended whole",
			template: "name: tmpl\nlabels:\n  a: default\n",
			existing: "name: x\n",
			openMaps: []string{"labels"},
			wantKeys: []string{"labels"},
			wantMap:  map[string]any{"name": "x", "labels": map[string]any{"a": "default"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filled, keys, err := FillMissing([]byte(tc.template), []byte(tc.existing), tc.openMaps...)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(keys, tc.wantKeys) {
				t.Errorf("keys = %v; want %v", keys, tc.wantKeys)
			}
			if tc.wantBytes != "" && string(filled) != tc.wantBytes {
				t.Errorf("filled = %q; want %q", filled, tc.wantBytes)
			}
			if tc.wantMap != nil {
				if got := decodeMap(t, filled); !reflect.DeepEqual(got, tc.wantMap) {
					t.Errorf("filled = %v; want %v", got, tc.wantMap)
				}
			}
			if tc.wantOrder != nil {
				if got := rootKeyOrder(t, filled); !slices.Equal(got, tc.wantOrder) {
					t.Errorf("order = %v; want %v", got, tc.wantOrder)
				}
			}
		})
	}
}

func TestFillMissing_ShapeMismatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, template, existing string
		want                     []string
	}{
		{"null where mapping", "status_line:\n  enabled: true\n", "status_line:\n", []string{`"status_line"`, "mapping", "null"}},
		{"scalar where mapping", "status_line:\n  enabled: true\n", "status_line: 3\n", []string{`"status_line"`}},
		{"mapping where scalar", "a: 1\n", "a:\n  b: 2\n", []string{`"a"`}},
		{"nested", "x:\n  y:\n    z: 1\n", "x:\n  y: 5\n", []string{`"x.y"`}},
		{"root", "a: 1\n", "- 1\n- 2\n", []string{`"<document root>"`}},
		{"list at an undeclared open map", "name: tmpl\nlabels:\n  a: default\n", "name: x\nlabels:\n  - a\n", []string{`"labels"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := FillMissing([]byte(tc.template), []byte(tc.existing))
			if err == nil {
				t.Fatal("want error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %s", err, want)
				}
			}
		})
	}
}

func TestFillMissing_ParseErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := FillMissing([]byte("a: [unclosed\n"), []byte("a: 1\n")); err == nil {
		t.Error("want template parse error")
	}
	if _, _, err := FillMissing([]byte("a: 1\n"), []byte("a: [unclosed\n")); err == nil {
		t.Error("want existing parse error")
	}
}
