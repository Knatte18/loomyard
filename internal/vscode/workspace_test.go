// workspace_test.go covers BuildWorkspace's exact output shape and its settings splicing.

package vscode

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuildWorkspace(t *testing.T) {
	t.Parallel()

	folders := []WorkspaceFolder{{Name: "prime", Path: "../prime"}, {Name: "board", Path: "../_board"}}
	const head = "{\n  \"folders\": [\n    {\n      \"name\": \"prime\",\n      \"path\": \"../prime\"\n    },\n    {\n      \"name\": \"board\",\n      \"path\": \"../_board\"\n    }\n  ],\n  \"settings\": "

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
			got, err := BuildWorkspace(tt.folders, tt.settings)
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

func TestRelativeSettingKeys(t *testing.T) {
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
