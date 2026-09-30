// workspace.go builds the content of a VS Code .code-workspace file.
// It owns VS Code format knowledge only: folder names and paths arrive as values,
// and no hub geometry is known here.

package vscode

import (
	"bytes"
	"encoding/json"
)

// WorkspaceFolder is one entry of a .code-workspace file's folders array.
type WorkspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// BuildWorkspace returns the bytes of a .code-workspace file listing folders in order,
// with settings spliced verbatim as the value of the "settings" key.
// A leading BOM is stripped and trailing whitespace trimmed from settings first;
// settings holding no JSON value (empty, whitespace, or only comments) become {}.
// The only error is the folder encoding's.
func BuildWorkspace(folders []WorkspaceFolder, settings []byte) ([]byte, error) {
	if folders == nil {
		folders = []WorkspaceFolder{}
	}
	var enc bytes.Buffer
	e := json.NewEncoder(&enc)
	e.SetEscapeHTML(false)
	e.SetIndent("  ", "  ")
	if err := e.Encode(folders); err != nil {
		return nil, err
	}
	value := bytes.TrimRight(bytes.TrimPrefix(settings, utf8BOM), " \t\r\n")
	if !holdsValue(value) {
		value = []byte("{}")
	}

	var out bytes.Buffer
	out.WriteString("{\n  \"folders\": ")
	out.Write(bytes.TrimRight(enc.Bytes(), "\n"))
	out.WriteString(",\n  \"settings\": ")
	out.Write(value)
	out.WriteString("\n}\n")
	return out.Bytes(), nil
}

// holdsValue reports whether b has any byte outside whitespace and JSONC comments.
// An unterminated block comment holds no value.
func holdsValue(b []byte) bool {
	for i := 0; i < len(b); {
		switch {
		case b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n':
			i++
		case bytes.HasPrefix(b[i:], []byte("//")):
			end := bytes.IndexByte(b[i:], '\n')
			if end < 0 {
				return false
			}
			i += end + 1
		case bytes.HasPrefix(b[i:], []byte("/*")):
			end := bytes.Index(b[i+2:], []byte("*/"))
			if end < 0 {
				return false
			}
			i += 2 + end + 2
		default:
			return true
		}
	}
	return false
}
