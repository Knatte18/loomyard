// fill_test.go — tests that configengine fills a present config file's missing keys from each real module template.

package configreg_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// fillModules are the registry modules the fill is exercised against.
var fillModules = []string{"fabric", "board", "landing", "loom", "reed"}

// dropKey returns template's YAML with the dotted key-path removed from its mapping nodes.
func dropKey(t *testing.T, template, keyPath string) []byte {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(template), &doc); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	mapping := doc.Content[0]
	parts := strings.Split(keyPath, ".")
	for i, part := range parts {
		idx := -1
		for j := 0; j < len(mapping.Content); j += 2 {
			if mapping.Content[j].Value == part {
				idx = j
				break
			}
		}
		if idx < 0 {
			t.Fatalf("key-path %q not in template", keyPath)
		}
		if i == len(parts)-1 {
			mapping.Content = append(mapping.Content[:idx], mapping.Content[idx+2:]...)
			break
		}
		mapping = mapping.Content[idx+1]
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	return out
}

// firstKey returns the first top-level key of template.
func firstKey(t *testing.T, template string) string {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(template), &doc); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	return doc.Content[0].Content[0].Value
}

// loadFilled seeds module's config file with content and loads it through configengine.Load, returning the loaded value, the captured log and the file's bytes afterwards.
func loadFilled(t *testing.T, module, template string, content []byte) (loaded map[string]interface{}, log string, after []byte) {
	t.Helper()
	baseDir := t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := configengine.ConfigFile(baseDir, module)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})

	resolved, err := configengine.Load(baseDir, module, []byte(template))
	if err != nil {
		t.Fatalf("Load(%s): %v", module, err)
	}
	if err := yaml.Unmarshal(resolved, &loaded); err != nil {
		t.Fatalf("unmarshal resolved %s: %v", module, err)
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read config file: %v", err)
	}
	return loaded, buf.String(), after
}

// wantDefault returns the template's own resolved value, the baseline a fill must reproduce.
func wantDefault(t *testing.T, module, template string) map[string]interface{} {
	t.Helper()
	got, _, _ := loadFilled(t, module, template, []byte(template))
	return got
}

// assertFilled loads module's template with keyPath dropped and checks the fill reproduces the template default, leaves the file untouched and logs one fill line naming module and keyPath.
func assertFilled(t *testing.T, module, template, keyPath string) {
	t.Helper()
	content := dropKey(t, template, keyPath)
	loaded, log, after := loadFilled(t, module, template, content)

	if want := wantDefault(t, module, template); !reflect.DeepEqual(loaded, want) {
		t.Errorf("%s loaded %v; want the template default %v", module, loaded, want)
	}
	if !bytes.Equal(after, content) {
		t.Errorf("%s config file was rewritten", module)
	}
	if n := strings.Count(log, "filled missing keys from template"); n != 1 {
		t.Fatalf("%s: expected one fill line, got %d in: %s", module, n, log)
	}
	if !strings.Contains(log, "module="+module) || !strings.Contains(log, "keys="+keyPath) {
		t.Errorf("%s fill line does not name the module and %q: %s", module, keyPath, log)
	}
}

// moduleTemplate returns module's registered template, failing the test when the registry lacks it.
func moduleTemplate(t *testing.T, module string) string {
	t.Helper()
	fn, ok := configreg.Template(module)
	if !ok {
		t.Fatalf("module %q not in the registry", module)
	}
	return fn()
}

// TestFill loads each module's real template with part of it missing and checks the load fills the gap at the template default, leaves the file untouched and logs one fill line naming the module and key-path; an empty but present file loads as the template.
// It serializes its rows because they swap the process-global logger output.
func TestFill(t *testing.T) {
	type fillRow struct {
		name, module string
		// keyPath is the dotted key-path dropped from the template; empty drops its first top-level key.
		keyPath string
		// emptyFile seeds an empty file instead of a template with a key dropped.
		emptyFile bool
	}
	var rows []fillRow
	for _, module := range fillModules {
		rows = append(rows,
			fillRow{name: module + " dropped top-level key", module: module},
			fillRow{name: module + " empty file", module: module, emptyFile: true},
		)
	}
	rows = append(rows,
		fillRow{name: "reed dropped nested key", module: "reed", keyPath: "selvage.height_rows"},
		fillRow{name: "reed dropped whole mapping", module: "reed", keyPath: "status_line"},
	)

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			template := moduleTemplate(t, row.module)
			if row.emptyFile {
				loaded, _, after := loadFilled(t, row.module, template, nil)
				if want := wantDefault(t, row.module, template); !reflect.DeepEqual(loaded, want) {
					t.Errorf("%s loaded %v; want the template default %v", row.module, loaded, want)
				}
				if len(after) != 0 {
					t.Errorf("%s empty config file was rewritten", row.module)
				}
				return
			}
			keyPath := row.keyPath
			if keyPath == "" {
				keyPath = firstKey(t, template)
			}
			assertFilled(t, row.module, template, keyPath)
		})
	}
}
