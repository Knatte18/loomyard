// settinglookup_test.go — tests for SettingLookup over temp hub fixtures.

package configreg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
)

// writeConfigFile writes content at path, creating the config directory above it.
func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// withTemplateValue returns template with the line opening "<key>: " rewritten to carry value, failing the test when the key is absent.
func withTemplateValue(t *testing.T, template, key, value string) string {
	t.Helper()
	prefix := key + ": "
	lines := strings.Split(template, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("template has no key %q", key)
	return ""
}

// TestSettingLookup pins SettingLookup's answers and refusals over a hub fixture each row builds for itself.
func TestSettingLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// arrange writes the row's files under the board and anchor dirs.
		arrange func(t *testing.T, dirs configengine.Dirs)
		// act reads through the lookup and returns the value and error asserted on.
		act       func(t *testing.T, dirs configengine.Dirs) (any, error)
		want      any
		wantError []string
	}{
		{
			name:    "a key no layer sets answers its template default",
			arrange: writeLoomDefault(""),
			act:     lookupOnce("loom", "plan_auto_accept"),
			want:    false,
		},
		{
			name:    "a key the default file sets answers the file's value",
			arrange: writeLoomDefault("5"),
			act:     lookupOnce("loom", "review_max_bounces"),
			want:    5,
		},
		{
			name: "a CategoryNone key in the local layer answers the local value",
			arrange: func(t *testing.T, dirs configengine.Dirs) {
				writeLoomDefault("5")(t, dirs)
				writeConfigFile(t, configengine.LocalFile(dirs.Anchor, "loom"), "review_max_bounces: 7\n")
			},
			act:  lookupOnce("loom", "review_max_bounces"),
			want: 7,
		},
		{
			name: "a guard key in the local layer fails the call",
			arrange: func(t *testing.T, dirs configengine.Dirs) {
				writeLoomDefault("5")(t, dirs)
				writeConfigFile(t, configengine.LocalFile(dirs.Anchor, "loom"), "plan_auto_accept: true\n")
			},
			act:       lookupOnce("loom", "plan_auto_accept"),
			wantError: []string{"loom", "plan_auto_accept"},
		},
		{
			name:    "a second key answers from the snapshot while a fresh lookup sees the rewrite",
			arrange: writeLoomDefault("5"),
			act: func(t *testing.T, dirs configengine.Dirs) (any, error) {
				lookup := SettingLookup(dirs)
				first, err := lookup("loom", "review_max_bounces")
				if err != nil || first != 5 {
					t.Fatalf("first read = %v, %v; want 5", first, err)
				}
				writeLoomDefault("9")(t, dirs)
				second, err := lookup("loom", "review_max_bounces")
				if err != nil || second != 5 {
					t.Fatalf("second read = %v, %v; want the snapshot's 5", second, err)
				}
				return SettingLookup(dirs)("loom", "review_max_bounces")
			},
			want: 9,
		},
		{
			name: "a hub-wide module answers from the board's file",
			arrange: func(t *testing.T, dirs configengine.Dirs) {
				m, _ := Lookup("darn")
				writeConfigFile(t, configengine.ConfigFile(dirs.Board, "darn"), withTemplateValue(t, m.Template(), "darn_fixer_fan", "fix-code"))
			},
			act:  lookupOnce("darn", "darn_fixer_fan"),
			want: "fix-code",
		},
		{
			name:      "an unregistered module is refused",
			arrange:   writeLoomDefault(""),
			act:       lookupOnce("lom", "review_max_bounces"),
			wantError: []string{"lom", "way forward"},
		},
		{
			name: "a key only the default file holds is refused",
			arrange: func(t *testing.T, dirs configengine.Dirs) {
				writeConfigFile(t, configengine.ConfigFile(dirs.Board, "loom"), loomengine.ConfigTemplate()+"\nextra_key: 1\n")
			},
			act:       lookupOnce("loom", "extra_key"),
			wantError: []string{"loom", "extra_key", "way forward"},
		},
		{
			name: "an unreadable default layer propagates naming the module",
			arrange: func(t *testing.T, dirs configengine.Dirs) {
				writeConfigFile(t, configengine.ConfigFile(dirs.Board, "loom"), "review_max_bounces: [unclosed\n")
			},
			act:       lookupOnce("loom", "review_max_bounces"),
			wantError: []string{"loom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir(), Anchor: t.TempDir()}
			// Both dirs need a config directory so the layer stack treats them as initialised.
			for _, dir := range []string{dirs.Board, dirs.Anchor} {
				if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			tt.arrange(t, dirs)
			got, err := tt.act(t, dirs)
			if len(tt.wantError) > 0 {
				if err == nil {
					t.Fatalf("lookup = %v; want an error containing %q", got, tt.wantError)
				}
				for _, fragment := range tt.wantError {
					if !strings.Contains(err.Error(), fragment) {
						t.Errorf("error %q lacks %q", err, fragment)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("lookup error: %v", err)
			}
			if got != tt.want {
				t.Errorf("lookup = %#v; want %#v", got, tt.want)
			}
		})
	}
}

// writeLoomDefault returns an arranger writing loom's default file from the template, with review_max_bounces set to bounces when it is not empty.
func writeLoomDefault(bounces string) func(t *testing.T, dirs configengine.Dirs) {
	return func(t *testing.T, dirs configengine.Dirs) {
		t.Helper()
		content := loomengine.ConfigTemplate()
		if bounces != "" {
			content = withTemplateValue(t, content, "review_max_bounces", bounces)
		}
		writeConfigFile(t, configengine.ConfigFile(dirs.Board, "loom"), content)
	}
}

// lookupOnce returns an action reading one key of one module through a fresh lookup.
func lookupOnce(module, key string) func(t *testing.T, dirs configengine.Dirs) (any, error) {
	return func(t *testing.T, dirs configengine.Dirs) (any, error) {
		t.Helper()
		return SettingLookup(dirs)(module, key)
	}
}
