// workspace_test.go covers BuildWorkspace's exact output shape and its settings splicing.

package vscode

import (
	"encoding/json"
	"testing"
)

func TestBuildWorkspace(t *testing.T) {
	folders := []WorkspaceFolder{{Name: "prime", Path: "../prime"}, {Name: "board", Path: "../_board"}}
	const head = "{\n  \"folders\": [\n    {\n      \"name\": \"prime\",\n      \"path\": \"../prime\"\n    },\n    {\n      \"name\": \"board\",\n      \"path\": \"../_board\"\n    }\n  ],\n  \"settings\": "

	tests := []struct {
		name     string
		settings []byte
		want     string
		valid    bool
	}{
		{"plain json", []byte("{\n  \"a\": 1\n}\n"), head + "{\n  \"a\": 1\n}\n}\n", true},
		{"jsonc verbatim", []byte("{\n  // c\n  /* b */ \"a\": [1,],\n}"), head + "{\n  // c\n  /* b */ \"a\": [1,],\n}\n}\n", false},
		{"bom stripped", append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\"a\":1}")...), head + "{\"a\":1}\n}\n", true},
		{"nil", nil, head + "{}\n}\n", true},
		{"empty", []byte{}, head + "{}\n}\n", true},
		{"whitespace only", []byte(" \n\t\r\n"), head + "{}\n}\n", true},
		{"comments only", []byte("// a\n/* b\n c */\n// d"), head + "{}\n}\n", true},
		{"ends in line comment", []byte("{\"a\":1} // tail"), head + "{\"a\":1} // tail\n}\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildWorkspace(folders, tt.settings)
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

func TestBuildWorkspaceEscapesFolders(t *testing.T) {
	got, err := BuildWorkspace([]WorkspaceFolder{{Name: `a"b`, Path: `c\d`}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(got) {
		t.Errorf("output is not valid JSON:\n%s", got)
	}
}
