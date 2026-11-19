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
		want     string
		valid    bool
	}{
		{"plain json", folders, []byte("{\n  \"a\": 1\n}\n"), head + "{\n  \"a\": 1\n}\n}\n", true},
		{"jsonc verbatim", folders, []byte("{\n  // c\n  /* b */ \"a\": [1,],\n}"), head + "{\n  // c\n  /* b */ \"a\": [1,],\n}\n}\n", false},
		{"bom stripped", folders, append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\"a\":1}")...), head + "{\"a\":1}\n}\n", true},
		{"nil", folders, nil, head + "{}\n}\n", true},
		{"empty", folders, []byte{}, head + "{}\n}\n", true},
		{"whitespace only", folders, []byte(" \n\t\r\n"), head + "{}\n}\n", true},
		{"comments only", folders, []byte("// a\n/* b\n c */\n// d"), head + "{}\n}\n", true},
		{"ends in line comment", folders, []byte("{\"a\":1} // tail"), head + "{\"a\":1} // tail\n}\n", false},
		{"folder name and path escaped", escapedFolders, nil, escapedHead + "{}\n}\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildWorkspace(tt.folders, tt.settings, nil)
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

	memberTests := []struct {
		name    string
		members map[string]json.RawMessage
		want    string
		wantErr string
	}{
		{
			name: "spliced after settings in name order, trailing whitespace trimmed",
			members: map[string]json.RawMessage{
				"tasks":  json.RawMessage("{\"version\":\"2.0.0\"}\n"),
				"extras": json.RawMessage("[1]"),
			},
			want: head + "{},\n  \"extras\": [1],\n  \"tasks\": {\"version\":\"2.0.0\"}\n}\n",
		},
		{
			name:    "folders refused",
			members: map[string]json.RawMessage{"folders": json.RawMessage("[]")},
			wantErr: `workspace member "folders" is a key the workspace builder writes itself`,
		},
		{
			name:    "settings refused",
			members: map[string]json.RawMessage{"settings": json.RawMessage("{}")},
			wantErr: `workspace member "settings" is a key the workspace builder writes itself`,
		},
	}
	for _, tt := range memberTests {
		t.Run("Members_"+tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildWorkspace(folders, []byte("{}"), tt.members)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("BuildWorkspace error = %v; want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			if !json.Valid(got) {
				t.Errorf("output is not valid JSON:\n%s", got)
			}
		})
	}
}

func TestWorkspaceTasks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name, lymPath, wantCommand string
	}{
		{"GivenLymPath", "/opt/lym/bin/lym", "/opt/lym/bin/lym"},
		{"EmptyLymPathBecomesBareLym", "", "lym"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, value := WorkspaceTasks(tt.lymPath, "some-beam", "/hub/some-beam")
			if name != "tasks" {
				t.Fatalf("member name = %q; want tasks", name)
			}
			var got struct {
				Version string           `json:"version"`
				Tasks   []map[string]any `json:"tasks"`
			}
			if err := json.Unmarshal(value, &got); err != nil {
				t.Fatalf("member is not valid JSON: %v\n%s", err, value)
			}
			want := map[string]any{
				"label":        "beam start",
				"type":         "process",
				"command":      tt.wantCommand,
				"args":         []any{"beam", "start", "some-beam"},
				"options":      map[string]any{"cwd": "/hub/some-beam"},
				"presentation": map[string]any{"reveal": "always", "panel": "new", "echo": true, "focus": true},
			}
			if got.Version != "2.0.0" || len(got.Tasks) != 1 || !reflect.DeepEqual(got.Tasks[0], want) {
				t.Errorf("WorkspaceTasks = %+v; want one task %v", got, want)
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
