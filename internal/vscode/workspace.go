// workspace.go builds the content of a VS Code .code-workspace file.
// It owns VS Code format knowledge only: folder names and paths arrive as values,
// and no hub geometry is known here.

package vscode

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// WorkspaceFolder is one entry of a .code-workspace file's folders array.
type WorkspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// BuildWorkspace returns the bytes of a .code-workspace file listing folders in order, with settings spliced verbatim as the value of the "settings" key.
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

// RelativeSettingKeys returns, sorted, the top-level keys of the settings document whose string value starts with "./" or "../".
// Such a value resolved against the folder's own .vscode/ before and resolves against the workspace file's directory once spliced into one.
// Values of other types and keys nested inside an object are not examined.
// Settings that are empty, not a JSON object after comments and trailing commas are dropped, or cannot be decoded yield nil.
func RelativeSettingKeys(settings []byte) []string {
	value := bytes.TrimPrefix(settings, utf8BOM)
	if !holdsValue(value) {
		return nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(value), &doc); err != nil {
		return nil
	}
	var keys []string
	for k, raw := range doc {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// stripJSONC removes comments and trailing commas from a JSONC document, leaving string contents untouched.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		switch {
		case b[i] == '"':
			j := i + 1
			for j < len(b) && b[j] != '"' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(b) {
				j = len(b) - 1
			}
			out = append(out, b[i:j+1]...)
			i = j + 1
		case bytes.HasPrefix(b[i:], []byte("//")):
			end := bytes.IndexByte(b[i:], '\n')
			if end < 0 {
				return out
			}
			i += end
		case bytes.HasPrefix(b[i:], []byte("/*")):
			end := bytes.Index(b[i+2:], []byte("*/"))
			if end < 0 {
				return out
			}
			i += 2 + end + 2
		case b[i] == ',' && trailingComma(b[i+1:]):
			i++
		default:
			out = append(out, b[i])
			i++
		}
	}
	return out
}

// trailingComma reports whether the first significant byte of rest, skipping whitespace and comments, closes an object or array.
func trailingComma(rest []byte) bool {
	for i := 0; i < len(rest); {
		switch {
		case rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\r' || rest[i] == '\n':
			i++
		case bytes.HasPrefix(rest[i:], []byte("//")):
			end := bytes.IndexByte(rest[i:], '\n')
			if end < 0 {
				return false
			}
			i += end + 1
		case bytes.HasPrefix(rest[i:], []byte("/*")):
			end := bytes.Index(rest[i+2:], []byte("*/"))
			if end < 0 {
				return false
			}
			i += 2 + end + 2
		default:
			return rest[i] == '}' || rest[i] == ']'
		}
	}
	return false
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
