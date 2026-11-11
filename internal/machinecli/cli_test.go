// cli_test.go covers `lym machine name` through the command built over hand-made seams.

package machinecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runName runs `lym machine name` with args over d in cwd and returns the exit code and the decoded envelope.
func runName(t *testing.T, d deps, cwd string, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(newCommand(d), cwd, &out, append([]string{"name"}, args...))
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", out.String(), err)
	}
	return code, envelope
}

// fileState describes what stands at path: absent, a directory, or a file with its bytes.
func fileState(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "absent"
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		return "dir"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return "file:" + string(data)
}

// hostNamed returns a hostName seam that yields name.
func hostNamed(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func TestName_Show(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// content is the machine file's content; "" writes no file.
		content string
		// dirAtPath makes the machine file's path a directory.
		dirAtPath bool
		// noDirectory makes the locate seam resolve no directory.
		noDirectory bool
		locateErr   error
		hostErr     error
		wantOK      bool
		wantName    string
		wantSource  string
		wantFormer  []string
		// wantErr is a substring the refusal's message must carry, with each of wantErrAlso.
		wantErr     string
		wantErrAlso []string
	}{
		{name: "NoFileReadsTheHostnameRule", wantOK: true, wantName: "osl-1046", wantSource: "hostname", wantFormer: []string{}},
		{name: "NoDirectoryReadsTheHostnameRule", noDirectory: true, wantOK: true, wantName: "osl-1046", wantSource: "hostname", wantFormer: []string{}},
		{
			name: "FileNameWithFormerNames", content: `{"name":"osl46","names":["neve","","osl46"]}`,
			wantOK: true, wantName: "osl46", wantSource: "file", wantFormer: []string{"neve"},
		},
		{
			name: "UndecodableFileShowsItsErrorAndPath", content: "not json",
			wantErr: "does not decode", wantErrAlso: []string{"machine.json", "lym machine name <name>"},
		},
		{
			name: "InvalidNameShowsItsErrorAndPath", content: `{"name":"BAD","names":["BAD"]}`,
			wantErr: "holds no valid name", wantErrAlso: []string{"machine.json", "lym machine name <name>"},
		},
		{
			name: "UnreadableFileShowsItsPathAndRemoval", dirAtPath: true,
			wantErr: "does not read", wantErrAlso: []string{"machine.json", "remove the file", "lym machine name <name>"},
		},
		{name: "RelativeVariableShowsItsError", locateErr: errors.New("XDG_CONFIG_HOME is relative"), wantErr: "XDG_CONFIG_HOME is relative"},
		{name: "UnreadableHostnameShowsItsError", hostErr: errors.New("hostname unreadable"), wantErr: "hostname unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "machine.json")
			switch {
			case tt.dirAtPath:
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", path, err)
				}
			case tt.content != "":
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			before := fileState(t, path)
			d := deps{
				locate: func() (string, string, bool, error) {
					if tt.locateErr != nil {
						return "", "", false, tt.locateErr
					}
					if tt.noDirectory {
						return "", "", false, nil
					}
					return path, path + ".lock", true, nil
				},
				hostName: func() (string, error) {
					if tt.hostErr != nil {
						return "", tt.hostErr
					}
					return "osl-1046", nil
				},
			}

			code, envelope := runName(t, d, dir)

			if after := fileState(t, path); after != before {
				t.Errorf("machine file after the show = %q; want it unchanged from %q", after, before)
			}
			if !tt.wantOK {
				if code == 0 || envelope["ok"] != false {
					t.Fatalf("show exit %d, envelope %v; want a non-zero refusal", code, envelope)
				}
				message, _ := envelope["error"].(string)
				for _, want := range append([]string{tt.wantErr}, tt.wantErrAlso...) {
					if !strings.Contains(message, want) {
						t.Errorf("refusal %q; want it to name %q", message, want)
					}
				}
				if _, named := envelope["machine"]; named {
					t.Errorf("refusal carries machine %v; want no name", envelope["machine"])
				}
				return
			}
			if code != 0 || envelope["ok"] != true {
				t.Fatalf("show exit %d, envelope %v; want ok", code, envelope)
			}
			machine, _ := envelope["machine"].(map[string]any)
			if machine["name"] != tt.wantName || machine["source"] != tt.wantSource {
				t.Errorf("show name %v from %v; want %q from %q", machine["name"], machine["source"], tt.wantName, tt.wantSource)
			}
			wantFile := path
			if tt.noDirectory {
				wantFile = ""
			}
			if machine["file"] != wantFile {
				t.Errorf("show file = %v; want %q", machine["file"], wantFile)
			}
			var former []string
			for _, name := range machine["former"].([]any) {
				former = append(former, name.(string))
			}
			if !slices.Equal(former, tt.wantFormer) {
				t.Errorf("show former = %v; want %v", former, tt.wantFormer)
			}
		})
	}
}
