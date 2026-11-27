// workspace_test.go covers BuildWorkspace's exact output shape and its settings splicing.
// It also covers the workspace look WorkspaceSettings sets.

package vscode

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBuildWorkspace(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	folders := []WorkspaceFolder{{Name: "prime", Path: "../prime"}, {Name: "board", Path: "../" + lymcwd.BoltDirName}}
	const head = "{\n  \"folders\": [\n    {\n      \"name\": \"prime\",\n      \"path\": \"../prime\"\n    },\n    {\n      \"name\": \"board\",\n      \"path\": \"../" + lymcwd.BoltDirName + "\"\n    }\n  ],\n  \"settings\": "

	escapedFolders := []WorkspaceFolder{{Name: `a"b`, Path: `c\d`}}
	const escapedHead = "{\n  \"folders\": [\n    {\n      \"name\": \"a\\\"b\",\n      \"path\": \"c\\\\d\"\n    }\n  ],\n  \"settings\": "

	tests := []struct {
		name     string
		folders  []WorkspaceFolder
		settings []byte
		members  map[string]json.RawMessage
		want     string
		valid    bool
		wantErr  string
	}{
		{"plain json", folders, []byte("{\n  \"a\": 1\n}\n"), nil, head + "{\n  \"a\": 1\n}\n}\n", true, ""},
		{"jsonc verbatim", folders, []byte("{\n  // c\n  /* b */ \"a\": [1,],\n}"), nil, head + "{\n  // c\n  /* b */ \"a\": [1,],\n}\n}\n", false, ""},
		{"bom stripped", folders, append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\"a\":1}")...), nil, head + "{\"a\":1}\n}\n", true, ""},
		{"nil", folders, nil, nil, head + "{}\n}\n", true, ""},
		{"empty", folders, []byte{}, nil, head + "{}\n}\n", true, ""},
		{"whitespace only", folders, []byte(" \n\t\r\n"), nil, head + "{}\n}\n", true, ""},
		{"comments only", folders, []byte("// a\n/* b\n c */\n// d"), nil, head + "{}\n}\n", true, ""},
		{"ends in line comment", folders, []byte("{\"a\":1} // tail"), nil, head + "{\"a\":1} // tail\n}\n", false, ""},
		{"folder name and path escaped", escapedFolders, nil, nil, escapedHead + "{}\n}\n", true, ""},
		{
			name:     "members follow settings in name order",
			folders:  folders,
			settings: []byte(`{"a":1}`),
			members:  map[string]json.RawMessage{"tasks": []byte("{\"x\": 1}\n"), "extensions": []byte("[]  ")},
			want:     head + "{\"a\":1},\n  \"extensions\": [],\n  \"tasks\": {\"x\": 1}\n}\n",
			valid:    true,
		},
		{name: "a member named folders", folders: folders, members: map[string]json.RawMessage{"folders": []byte("[]")}, wantErr: `workspace member "folders" is a key the workspace builder writes itself`},
		{name: "a member named settings", folders: folders, members: map[string]json.RawMessage{"settings": []byte("{}")}, wantErr: `workspace member "settings" is a key the workspace builder writes itself`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildWorkspace(tt.folders, tt.settings, tt.members)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("BuildWorkspace() error = %v; want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			if tt.valid && !json.Valid(got) {
				t.Errorf("output is not valid JSON:\n%s", got)
			}
		})
	}
}

// TestWorkspaceTasks pins the tasks member of a workspace file: its name and its one beam start task.
func TestWorkspaceTasks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		lymPath     string
		wantCommand string
	}{
		{"ResolvedPath", "/opt/lym/bin/lym", "/opt/lym/bin/lym"},
		{"EmptyPathRunsTheBareName", "", "lym"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, value := WorkspaceTasks(tt.lymPath, "my-beam", "/hub/my-beam")

			if name != "tasks" {
				t.Errorf("member name = %q; want tasks", name)
			}
			var got map[string]any
			if err := json.Unmarshal(value, &got); err != nil {
				t.Fatalf("member value is not JSON: %v\n%s", err, value)
			}
			want := map[string]any{
				"version": "2.0.0",
				"tasks": []any{map[string]any{
					"label":        "beam start",
					"type":         "process",
					"command":      tt.wantCommand,
					"args":         []any{"beam", "start", "my-beam"},
					"options":      map[string]any{"cwd": "/hub/my-beam"},
					"presentation": map[string]any{"reveal": "always", "panel": "new", "echo": true, "focus": true},
				}},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("WorkspaceTasks(%q) value = %v; want %v", tt.lymPath, got, want)
			}
		})
	}
}

func TestWorkspaceSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const beamColor = "#7d2d6b"
	lookColors := func(project string) map[string]any {
		return map[string]any{
			"titleBar.activeBackground":      project,
			"titleBar.inactiveBackground":    project,
			"titleBar.activeForeground":      "#ffffff",
			"titleBar.inactiveForeground":    "#ffffffaa",
			"activityBar.background":         WorkspaceColor,
			"activityBar.foreground":         "#ffffff",
			"activityBar.inactiveForeground": "#ffffffaa",
		}
	}
	withKey := func(colors map[string]any, key string, value any) map[string]any {
		colors[key] = value
		return colors
	}

	tests := []struct {
		name     string
		settings []byte
		want     map[string]any
	}{
		{
			"PrimeSettingsKeepTheirKeysAndGreen",
			[]byte(`{"editor.tabSize": 2, "window.title": "LY: prime", "workbench.colorCustomizations": {"titleBar.activeBackground": "#1b5e20", "activityBar.background": "#1b5e20", "editor.background": "#000000"}}`),
			map[string]any{
				"editor.tabSize":                float64(2),
				"window.title":                  "LY: hub workspace",
				"workbench.colorCustomizations": withKey(lookColors(PrimeColor), "editor.background", "#000000"),
			},
		},
		{
			"BeamSettingsKeepTheBeamColorOnTheTitleBar",
			[]byte(`{"workbench.colorCustomizations": {"titleBar.activeBackground": "` + beamColor + `", "activityBar.background": "#2d4f7d"}}`),
			map[string]any{"window.title": "LY: hub workspace", "workbench.colorCustomizations": lookColors(beamColor)},
		},
		{
			"JSONCWithBOMDecodes",
			append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\n  // c\n  \"a\": 1,\n}")...),
			map[string]any{"a": float64(1), "window.title": "LY: hub workspace", "workbench.colorCustomizations": lookColors(PrimeColor)},
		},
		{
			"NoSettingsTakeGreen",
			nil,
			map[string]any{"window.title": "LY: hub workspace", "workbench.colorCustomizations": lookColors(PrimeColor)},
		},
		{
			"CommentsOnlyTakeGreen",
			[]byte("// only a comment"),
			map[string]any{"window.title": "LY: hub workspace", "workbench.colorCustomizations": lookColors(PrimeColor)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := WorkspaceSettings(tt.settings, "LY: hub workspace")
			if err != nil {
				t.Fatalf("WorkspaceSettings: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("WorkspaceSettings output is not JSON: %v\n%s", err, got)
			}
			if !reflect.DeepEqual(decoded, tt.want) {
				t.Errorf("WorkspaceSettings() = %v; want %v", decoded, tt.want)
			}
		})
	}

	undecodable := []struct{ name, settings string }{
		{"Truncated", `{"a": `},
		{"Array", `["x"]`},
		{"Null", `null`},
	}
	for _, tt := range undecodable {
		t.Run("UndecodableReturnedUnchanged_"+tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := WorkspaceSettings([]byte(tt.settings), "LY: hub workspace")
			if err != nil || string(got) != tt.settings {
				t.Errorf("WorkspaceSettings(%q) = %q, %v; want the settings unchanged and no error", tt.settings, got, err)
			}
		})
	}
}

func TestRelativeSettingKeys(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name     string
		settings []byte
		want     []string
	}{
		{"dot slash", []byte(`{"a": "./x"}`), []string{"a"}},
		{"dot dot slash", []byte(`{"a": "../x"}`), []string{"a"}},
		{"sorted", []byte(`{"b": "./x", "a": "../y"}`), []string{"a", "b"}},
		{"absolute path", []byte(`{"a": "/abs/x", "b": "C:\\x"}`), nil},
		{"plain string", []byte(`{"a": "x", "b": ".hidden", "c": "."}`), nil},
		{"non-string values", []byte(`{"a": 1, "b": true, "c": null, "d": ["./x"]}`), nil},
		{"nested object not examined", []byte(`{"a": {"b": "./x"}}`), nil},
		{"jsonc comments and trailing comma", []byte("{\n  // c\n  \"a\": \"./x\", /* b */\n  \"u\": \"http://h/./y\",\n}"), []string{"a"}},
		{"comment marker inside string", []byte(`{"a": "// not a comment", "b": "./x"}`), []string{"b"}},
		{"bom", append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"a": "./x"}`)...), []string{"a"}},
		{"nil", nil, nil},
		{"empty", []byte{}, nil},
		{"comments only", []byte("// a\n/* b */"), nil},
		{"not an object", []byte(`["./x"]`), nil},
		{"undecodable", []byte(`{"a": `), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeSettingKeys(tt.settings)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
