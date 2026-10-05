// fill_test.go covers FillMissing.

package yamlengine

import (
	"reflect"
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

func TestFillMissing_TopLevelKey(t *testing.T) {
	filled, keys, err := FillMissing([]byte("a: 1\nb: 2\n"), []byte("a: 9\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"b"}) {
		t.Errorf("keys = %v", keys)
	}
	m := decodeMap(t, filled)
	if m["a"] != 9 || m["b"] != 2 {
		t.Errorf("filled = %v", m)
	}
}

func TestFillMissing_NestedKey(t *testing.T) {
	existing := "name: mine\nselvage:\n  width: 7\nstatus_line:\n  enabled: false\n"
	filled, keys, err := FillMissing([]byte(fillTemplate), []byte(existing))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"selvage.height_rows"}) {
		t.Errorf("keys = %v", keys)
	}
	sel := decodeMap(t, filled)["selvage"].(map[string]any)
	if sel["height_rows"] != 10 || sel["width"] != 7 {
		t.Errorf("selvage = %v", sel)
	}
}

func TestFillMissing_WholeMapping(t *testing.T) {
	filled, keys, err := FillMissing([]byte(fillTemplate), []byte("name: x\nselvage:\n  height_rows: 1\n  width: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"status_line"}) {
		t.Errorf("keys = %v", keys)
	}
	sl := decodeMap(t, filled)["status_line"].(map[string]any)
	if sl["enabled"] != true {
		t.Errorf("status_line = %v", sl)
	}
}

func TestFillMissing_EmptyAndNullKept(t *testing.T) {
	template := "a: x\nb: y\nc: z\n"
	existing := "a: \"\"\nb:\n"
	filled, keys, err := FillMissing([]byte(template), []byte(existing))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"c"}) {
		t.Errorf("keys = %v", keys)
	}
	m := decodeMap(t, filled)
	if m["a"] != "" || m["b"] != nil || m["c"] != "z" {
		t.Errorf("filled = %v", m)
	}
}

func TestFillMissing_EmptiedListKept(t *testing.T) {
	filled, keys, err := FillMissing([]byte("items:\n  - a\n  - b\nother: 1\n"), []byte("items: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"other"}) {
		t.Errorf("keys = %v", keys)
	}
	if items := decodeMap(t, filled)["items"].([]any); len(items) != 0 {
		t.Errorf("items = %v", items)
	}
}

func TestFillMissing_ExtraFileKeyAndOrder(t *testing.T) {
	filled, keys, err := FillMissing([]byte("a: 1\nb: 2\nc: 3\n"), []byte("z: 0\nc: 30\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"a", "b"}) {
		t.Errorf("keys = %v", keys)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(filled, &node); err != nil {
		t.Fatal(err)
	}
	var order []string
	root := node.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		order = append(order, root.Content[i].Value)
	}
	if !reflect.DeepEqual(order, []string{"z", "c", "a", "b"}) {
		t.Errorf("order = %v", order)
	}
}

func TestFillMissing_EmptyAndCommentsOnly(t *testing.T) {
	for _, existing := range []string{"", "   \n", "# just a comment\n"} {
		filled, keys, err := FillMissing([]byte("b: 2\na: 1\n"), []byte(existing))
		if err != nil {
			t.Fatal(err)
		}
		if string(filled) != "b: 2\na: 1\n" {
			t.Errorf("filled = %q for %q", filled, existing)
		}
		if !reflect.DeepEqual(keys, []string{"a", "b"}) {
			t.Errorf("keys = %v for %q", keys, existing)
		}
	}
}

func TestFillMissing_NoOpByteIdentity(t *testing.T) {
	existing := "# keep me\nname:   spaced   # trailing\nselvage:\n    height_rows: 1\n    width: 2\nstatus_line:\n  enabled: no\n"
	filled, keys, err := FillMissing([]byte(fillTemplate), []byte(existing))
	if err != nil {
		t.Fatal(err)
	}
	if string(filled) != existing {
		t.Errorf("filled = %q", filled)
	}
	if len(keys) != 0 {
		t.Errorf("keys = %v", keys)
	}
}

func TestFillMissing_ShapeMismatch(t *testing.T) {
	cases := []struct {
		name, template, existing, wantPath string
	}{
		{"null where mapping", "status_line:\n  enabled: true\n", "status_line:\n", `"status_line"`},
		{"scalar where mapping", "status_line:\n  enabled: true\n", "status_line: 3\n", `"status_line"`},
		{"mapping where scalar", "a: 1\n", "a:\n  b: 2\n", `"a"`},
		{"nested", "x:\n  y:\n    z: 1\n", "x:\n  y: 5\n", `"x.y"`},
		{"root", "a: 1\n", "- 1\n- 2\n", `"<document root>"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := FillMissing([]byte(tc.template), []byte(tc.existing))
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q lacks %s", err, tc.wantPath)
			}
		})
	}
}

func TestFillMissing_ShapeMismatchNamesBothShapes(t *testing.T) {
	_, _, err := FillMissing([]byte("a:\n  b: 1\n"), []byte("a:\n"))
	if err == nil || !strings.Contains(err.Error(), "mapping") || !strings.Contains(err.Error(), "null") {
		t.Errorf("err = %v", err)
	}
}

func TestFillMissing_ParseErrors(t *testing.T) {
	if _, _, err := FillMissing([]byte("a: [unclosed\n"), []byte("a: 1\n")); err == nil {
		t.Error("want template parse error")
	}
	if _, _, err := FillMissing([]byte("a: 1\n"), []byte("a: [unclosed\n")); err == nil {
		t.Error("want existing parse error")
	}
}

func TestFillMissing_ListElementKeyLeftForMissingKeys(t *testing.T) {
	template := "items:\n  - name: a\n    extra: 1\n"
	existing := "items:\n  - name: b\n"
	filled, keys, err := FillMissing([]byte(template), []byte(existing))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 || string(filled) != existing {
		t.Errorf("keys = %v, filled = %q", keys, filled)
	}
}

func TestFillMissing_OpenMapSkipsPresentKeyWhateverItsShape(t *testing.T) {
	template := "name: tmpl\nlabels:\n  a: default\n"

	filled, keys, err := FillMissing([]byte(template), []byte("name: x\nlabels:\n  - a\n"), "labels")
	if err != nil {
		t.Fatalf("FillMissing() unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("keys = %v; want none", keys)
	}
	if !strings.Contains(string(filled), "- a") {
		t.Errorf("filled = %q; want the list untouched", filled)
	}

	if _, _, err := FillMissing([]byte(template), []byte("name: x\nlabels:\n  - a\n")); err == nil {
		t.Errorf("FillMissing() without the declaration: want a shape mismatch")
	}
}

func TestFillMissing_OpenMapMissingIsAppendedWhole(t *testing.T) {
	filled, keys, err := FillMissing([]byte("name: tmpl\nlabels:\n  a: default\n"), []byte("name: x\n"), "labels")
	if err != nil {
		t.Fatalf("FillMissing() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"labels"}) {
		t.Errorf("keys = %v; want [labels]", keys)
	}
	if !strings.Contains(string(filled), "a: default") {
		t.Errorf("filled = %q; want the template's labels", filled)
	}
}
