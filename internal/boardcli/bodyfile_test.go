// bodyfile_test.go covers applyBodyFile with temp files and in-memory stdin only.

package boardcli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeBodyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	return path
}

// TestApplyBodyFile asserts applyBodyFile sets the body from a file, from stdin and from an empty file, and refuses a missing file, a body already in the payload and stdin claimed for both payload and body.
func TestApplyBodyFile(t *testing.T) {
	t.Parallel()
	str := func(s string) *string { return &s }
	tests := []struct {
		name   string
		fields map[string]any
		// fileContent, when set, is written to a temp file whose path is the body-file argument.
		fileContent *string
		// bodyFile is the body-file argument when fileContent is nil; "missing" stands for a path that does not exist.
		bodyFile string
		// payloadSource is the source the payload was read from.
		payloadSource string
		stdin         string
		// wantErr is a substring of the error; "missing" stands for the missing path.
		wantErr string
		// wantFields is the fields map after the call, checked when set.
		wantFields map[string]any
	}{
		{
			name:          "file content becomes the body",
			fields:        map[string]any{"slug": "s"},
			fileContent:   str("# Body\n\n\"quoted\"\n"),
			payloadSource: "{}",
			wantFields:    map[string]any{"slug": "s", "body": "# Body\n\n\"quoted\"\n"},
		},
		{
			name:          "stdin becomes the body",
			fields:        map[string]any{"slug": "s"},
			bodyFile:      "-",
			payloadSource: "{}",
			stdin:         "from stdin",
			wantFields:    map[string]any{"slug": "s", "body": "from stdin"},
		},
		{
			name:          "an empty file sets an empty body",
			fields:        map[string]any{"slug": "s"},
			fileContent:   str(""),
			payloadSource: "{}",
			wantFields:    map[string]any{"slug": "s", "body": ""},
		},
		{
			name:          "a missing file is refused naming its path",
			fields:        map[string]any{},
			bodyFile:      "missing",
			payloadSource: "{}",
			wantErr:       "missing",
		},
		{
			name:          "a body already in the payload is refused naming the way forward",
			fields:        map[string]any{"slug": "s", "body": "x"},
			fileContent:   str("y"),
			payloadSource: "{}",
			wantErr:       "drop",
			wantFields:    map[string]any{"slug": "s", "body": "x"},
		},
		{
			name:          "stdin for both payload and body is refused",
			fields:        map[string]any{},
			bodyFile:      "-",
			payloadSource: "-",
			stdin:         "x",
			wantErr:       "stdin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bodyFile := tt.bodyFile
			if tt.fileContent != nil {
				bodyFile = writeBodyFile(t, *tt.fileContent)
			}
			if bodyFile == "missing" {
				bodyFile = filepath.Join(t.TempDir(), "absent.md")
			}

			err := applyBodyFile(tt.fields, bodyFile, tt.payloadSource, strings.NewReader(tt.stdin))

			if tt.wantErr != "" {
				want := tt.wantErr
				if want == "missing" {
					want = bodyFile
				}
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want one containing %q", err, want)
				}
			} else if err != nil {
				t.Fatalf("applyBodyFile: %v", err)
			}
			if tt.wantFields != nil && !reflect.DeepEqual(tt.fields, tt.wantFields) {
				t.Fatalf("fields = %#v, want %#v", tt.fields, tt.wantFields)
			}
		})
	}
}
