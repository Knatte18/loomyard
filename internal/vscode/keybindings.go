// keybindings.go seeds the marked block of terminal key bindings into the user's VS Code keybindings.json,
// so the integrated terminal forwards the Alt keys reed's tmux bindings listen for.
// It holds VS Code format knowledge only: the user-file location per platform, a comment-aware JSONC merge of the block, and an atomic write.

package vscode

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	keybindingsBeginMarker = "lyx:begin"
	keybindingsEndMarker   = "lyx:end"
	keybindingsFileName    = "keybindings.json"
)

// keybindingsBlockEntries are the block's entries, one terminal sendSequence per Alt key.
// Each text is written as it appears in the JSON file, with its escape sequences unexpanded.
var keybindingsBlockEntries = []struct{ key, text string }{
	{"alt+up", `\u001b[1;3A`},
	{"alt+down", `\u001b[1;3B`},
	{"alt+right", `\u001b[1;3C`},
	{"alt+left", `\u001b[1;3D`},
	{"alt+z", `\u001bz`},
}

// KeybindingsOutcome is what SeedKeybindings did to the user's file.
type KeybindingsOutcome string

// The outcomes of SeedKeybindings.
const (
	// KeybindingsCreated means the file did not exist and was created holding the block.
	KeybindingsCreated KeybindingsOutcome = "created"
	// KeybindingsWritten means an existing file was changed to hold the current block.
	KeybindingsWritten KeybindingsOutcome = "written"
	// KeybindingsUnchanged means the file already held the current block and was not written.
	KeybindingsUnchanged KeybindingsOutcome = "unchanged"
	// KeybindingsSkipped means the file was left untouched, for the reason in KeybindingsResult.Reason.
	KeybindingsSkipped KeybindingsOutcome = "skipped"
)

// KeybindingsResult reports what SeedKeybindings did and, for a skip, why.
type KeybindingsResult struct {
	Outcome KeybindingsOutcome `json:"outcome"`
	Reason  string             `json:"reason"`
}

// UserKeybindingsPath returns the user-level VS Code keybindings.json for the running platform.
func UserKeybindingsPath() (string, error) {
	home, _ := os.UserHomeDir()
	return userKeybindingsPath(runtime.GOOS, os.Getenv, home)
}

// userKeybindingsPath resolves the user keybindings.json for goos from getenv and the home directory.
func userKeybindingsPath(goos string, getenv func(string) string, home string) (string, error) {
	switch goos {
	case "windows":
		appData := getenv("APPDATA")
		if appData == "" {
			return "", errors.New("APPDATA is not set")
		}
		return appData + `\Code\User\` + keybindingsFileName, nil
	case "darwin":
		if home == "" {
			return "", errors.New("home directory is unknown")
		}
		return filepath.Join(home, "Library", "Application Support", "Code", "User", keybindingsFileName), nil
	default:
		if configHome := getenv("XDG_CONFIG_HOME"); configHome != "" {
			return filepath.Join(configHome, "Code", "User", keybindingsFileName), nil
		}
		if home == "" {
			return "", errors.New("home directory is unknown")
		}
		return filepath.Join(home, ".config", "Code", "User", keybindingsFileName), nil
	}
}

// keybindingsBlock returns the block's lines, begin marker to end marker, joined by lineEnding and with no trailing line ending.
func keybindingsBlock(lineEnding string) string {
	lines := []string{"// " + keybindingsBeginMarker}
	for _, entry := range keybindingsBlockEntries {
		lines = append(lines, fmt.Sprintf(
			`  { "key": %q, "command": "workbench.action.terminal.sendSequence", "args": { "text": "%s" }, "when": "terminalFocus" },`,
			entry.key, entry.text))
	}
	lines = append(lines, "// "+keybindingsEndMarker)
	return strings.Join(lines, lineEnding)
}

// keybindingsMarker is one marker line comment found by the scan.
type keybindingsMarker struct {
	begin bool
	// start is the offset of the comment's leading slashes, end the offset just past its text.
	start, end int
	// directlyInArray is true when the marker sits at the top-level array's own depth.
	directlyInArray bool
}

// scanKeybindings walks the JSONC in content, skipping string contents and comments,
// and returns the top-level array's opening bracket offset (-1 when there is none) and every marker line comment.
func scanKeybindings(content []byte) (arrayOpen int, markers []keybindingsMarker) {
	arrayOpen = -1
	depth := 0
	arrayClosed := false
	for i := 0; i < len(content); i++ {
		switch c := content[i]; {
		case c == '"':
			for i++; i < len(content) && content[i] != '"'; i++ {
				if content[i] == '\\' {
					i++
				}
			}
		case c == '/' && i+1 < len(content) && content[i+1] == '*':
			closing := bytes.Index(content[i+2:], []byte("*/"))
			if closing < 0 {
				return arrayOpen, markers
			}
			i += 2 + closing + 1
		case c == '/' && i+1 < len(content) && content[i+1] == '/':
			lineEnd := bytes.IndexByte(content[i:], '\n')
			if lineEnd < 0 {
				lineEnd = len(content) - i
			}
			text := strings.TrimRight(string(content[i+2:i+lineEnd]), " \t\r")
			trimmed := strings.TrimLeft(text, " \t")
			if trimmed == keybindingsBeginMarker || trimmed == keybindingsEndMarker {
				markers = append(markers, keybindingsMarker{
					begin:           trimmed == keybindingsBeginMarker,
					start:           i,
					end:             i + 2 + len(text),
					directlyInArray: arrayOpen >= 0 && !arrayClosed && depth == 1,
				})
			}
			i += lineEnd - 1
		case c == '[' || c == '{':
			if arrayOpen < 0 {
				if c != '[' {
					return -1, markers
				}
				arrayOpen = i
			}
			depth++
		case c == ']' || c == '}':
			depth--
			if depth == 0 {
				arrayClosed = true
			}
		}
	}
	return arrayOpen, markers
}

// mergeKeybindings returns existing with the lyx block replaced in place, or inserted after the top-level array's opening bracket when there is none.
// A file whose markers or array do not have exactly the expected shape is an error naming the shape.
func mergeKeybindings(existing []byte) ([]byte, error) {
	arrayOpen, markers := scanKeybindings(existing)
	if arrayOpen < 0 {
		return nil, errors.New("no top-level array")
	}

	lineEnding := "\n"
	if bytes.Contains(existing, []byte("\r\n")) {
		lineEnding = "\r\n"
	}
	var begins, ends []keybindingsMarker
	for _, marker := range markers {
		if !marker.directlyInArray {
			return nil, errors.New("a lyx marker sits outside the top-level array")
		}
		if marker.begin {
			begins = append(begins, marker)
		} else {
			ends = append(ends, marker)
		}
	}
	switch {
	case len(begins) > 1 || len(ends) > 1:
		return nil, errors.New("a lyx marker is repeated")
	case len(begins) == 1 && len(ends) == 0:
		return nil, errors.New("a lyx:begin marker has no lyx:end")
	case len(begins) == 0 && len(ends) == 1:
		return nil, errors.New("a lyx:end marker has no lyx:begin")
	case len(begins) == 1 && ends[0].start < begins[0].start:
		return nil, errors.New("the lyx:end marker comes before the lyx:begin marker")
	case len(begins) == 1:
		var merged bytes.Buffer
		merged.Write(existing[:begins[0].start])
		merged.WriteString(keybindingsBlock(lineEnding))
		merged.Write(existing[ends[0].end:])
		return merged.Bytes(), nil
	}

	// Inserted on its own lines: right after a line break that already follows the bracket, or on a fresh line otherwise.
	insertAt, prefix := arrayOpen+1, lineEnding
	if bytes.HasPrefix(existing[insertAt:], []byte(lineEnding)) {
		insertAt, prefix = insertAt+len(lineEnding), ""
	}
	var merged bytes.Buffer
	merged.Write(existing[:insertAt])
	merged.WriteString(prefix + keybindingsBlock(lineEnding) + lineEnding)
	merged.Write(existing[insertAt:])
	return merged.Bytes(), nil
}

// SeedKeybindings makes the file at path hold the lyx block between its markers, creating the file when it is missing.
// It never fails the caller: any condition that stops the write is a KeybindingsSkipped result naming the reason, and the file is then untouched.
// A symlinked path is written at its target, and an unchanged result writes nothing.
func SeedKeybindings(path string) KeybindingsResult {
	return seedKeybindings(path, func() string {
		return fmt.Sprintf(".%s.lyx-%d-%d.tmp", filepath.Base(path), os.Getpid(), time.Now().UnixNano())
	})
}

// seedKeybindings is SeedKeybindings with the temporary file's name drawn from temporaryName.
func seedKeybindings(path string, temporaryName func() string) KeybindingsResult {
	skip := func(format string, args ...any) KeybindingsResult {
		return KeybindingsResult{Outcome: KeybindingsSkipped, Reason: fmt.Sprintf(format, args...)}
	}

	target := path
	if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return skip("resolve symlink %s: %v", path, err)
		}
		target = resolved
	}

	var content []byte
	outcome := KeybindingsWritten
	mode := fs.FileMode(0o644)
	existing, err := os.ReadFile(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		outcome = KeybindingsCreated
		lineEnding := "\n"
		content = []byte("[" + lineEnding + keybindingsBlock(lineEnding) + lineEnding + "]" + lineEnding)
	case err != nil:
		return skip("read %s: %v", target, err)
	default:
		merged, err := mergeKeybindings(existing)
		if err != nil {
			return skip("%s: %v", target, err)
		}
		if bytes.Equal(merged, existing) {
			return KeybindingsResult{Outcome: KeybindingsUnchanged}
		}
		info, err := os.Stat(target)
		if err != nil {
			return skip("stat %s: %v", target, err)
		}
		mode = info.Mode().Perm()
		if mode&0o200 == 0 {
			return skip("%s is read-only", target)
		}
		content = merged
	}

	temporaryPath := filepath.Join(filepath.Dir(target), temporaryName())
	temporary, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return skip("create temporary file beside %s: %v", target, err)
	}
	writeErr := writeAndClose(temporary, content, mode)
	if writeErr == nil {
		writeErr = os.Rename(temporaryPath, target)
	}
	if writeErr != nil {
		os.Remove(temporaryPath)
		return skip("write %s: %v", target, writeErr)
	}
	return KeybindingsResult{Outcome: outcome}
}

// writeAndClose writes content to file, gives it mode regardless of the umask, and closes it.
func writeAndClose(file *os.File, content []byte, mode fs.FileMode) error {
	_, writeErr := file.Write(content)
	chmodErr := file.Chmod(mode)
	closeErr := file.Close()
	return errors.Join(writeErr, chmodErr, closeErr)
}
