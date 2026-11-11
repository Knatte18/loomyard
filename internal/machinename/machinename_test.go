package machinename

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFromHostname(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		host    string
		want    string
		wantErr bool
	}{
		{"Lowercased", "Neve", "neve", false},
		{"CutAtFirstDot", "neve.lan", "neve", false},
		{"TrimmedAndLowercased", " Box ", "box", false},
		{"Empty", "", "", true},
		{"OnlyDomain", ".local", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := FromHostname(tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FromHostname(%q) error = %v; wantErr %v", tt.host, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("FromHostname(%q) = %q; want %q", tt.host, got, tt.want)
			}
		})
	}
}

// TestResolveNames pins how the name resolves from the machine file at the path the locator hands in.
// No file, or no directory, falls back to the hostname rule;
// a file that yields no valid name, and a relative variable, are errors and never the hostname's name.
func TestResolveNames(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hostname := func() (string, error) { return "Neve.lan", nil }
	tests := []struct {
		name string
		// content is the machine file's content; "" writes no file.
		content string
		// noDirectory makes the locator resolve no directory.
		noDirectory bool
		// locateErr is the locator's error, as for a relative variable.
		locateErr   error
		hostname    func() (string, error)
		wantName    string
		wantFormer  []string
		wantErrText []string
	}{
		{name: "NoDirectoryReadsTheHostnameRule", noDirectory: true, hostname: hostname, wantName: "neve"},
		{name: "NoFileReadsTheHostnameRule", hostname: hostname, wantName: "neve"},
		{name: "FileNameWinsOverTheHostname", content: `{"name":"osl46","names":["osl46"]}`, hostname: hostname, wantName: "osl46"},
		{name: "UnknownKeyIsIgnored", content: `{"name":"osl46","color":"red"}`, hostname: hostname, wantName: "osl46"},
		{
			name:       "FormerNamesDropEmptyEntriesAndKeepInvalidOnes",
			content:    `{"name":"osl46","names":["neve","","Old.Box","osl46"]}`,
			hostname:   hostname,
			wantName:   "osl46",
			wantFormer: []string{"neve", "Old.Box"},
		},
		{name: "UndecodableFileIsAnError", content: `not json`, hostname: hostname, wantErrText: []string{"machine.json", "lym machine name"}},
		{name: "EmptyNameIsAnError", content: `{"name":""}`, hostname: hostname, wantErrText: []string{"machine.json", "lym machine name"}},
		{name: "InvalidNameIsAnError", content: `{"name":"OSL46"}`, hostname: hostname, wantErrText: []string{"machine.json", "OSL46", "lym machine name"}},
		{name: "RelativeVariableIsItsError", locateErr: errors.New("HOME must be absolute"), hostname: hostname, wantErrText: []string{"HOME must be absolute"}},
		{
			name:        "UnreadableHostnameNamesItsWayForward",
			noDirectory: true,
			hostname:    func() (string, error) { return "", errors.New("no uname") },
			wantErrText: []string{"no uname", "give the machine a hostname", "lym machine name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "machine.json")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatalf("write the machine file: %v", err)
				}
			}
			locate := func() (string, bool, error) { return path, !tt.noDirectory, tt.locateErr }

			name, former, err := resolveNames(locate, tt.hostname)

			if tt.wantErrText != nil {
				if err == nil || name != "" {
					t.Fatalf("resolveNames() = %q, %v; want an error and no name", name, err)
				}
				for _, text := range tt.wantErrText {
					if !strings.Contains(err.Error(), text) {
						t.Errorf("resolveNames() error = %v; want it to name %q", err, text)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveNames() error = %v; want nil", err)
			}
			if name != tt.wantName || !reflect.DeepEqual(former, tt.wantFormer) {
				t.Errorf("resolveNames() = %q, %q; want %q, %q", name, former, tt.wantName, tt.wantFormer)
			}
		})
	}
}

// TestResolveNames_UnreadableFileIsAnError pins that a machine file that exists but does not read
// is an error naming its path.
// A directory at the file's path stands in for an unreadable file on every platform.
func TestResolveNames_UnreadableFileIsAnError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	path := filepath.Join(t.TempDir(), "machine.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make a directory at the machine file's path: %v", err)
	}

	name, _, err := resolveNames(func() (string, bool, error) { return path, true, nil }, func() (string, error) { return "neve", nil })

	if err == nil || name != "" || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "lym machine name") {
		t.Errorf("resolveNames() = %q, %v; want an error naming %s and `lym machine name`", name, err, path)
	}
}

func TestCheckName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"LettersAndDigits", "osl46", false},
		{"Letters", "neve", false},
		{"InnerHyphen", "osl-46", false},
		{"MaximumLength", strings.Repeat("a", 63), false},
		{"UpperCase", "OSL46", true},
		{"Dot", "osl.46", true},
		{"Colon", "osl:46", true},
		{"Underscore", "osl_46", true},
		{"LeadingHyphen", "-osl46", true},
		{"TrailingHyphen", "osl46-", true},
		{"OverMaximumLength", strings.Repeat("a", 64), true},
		{"Empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := CheckName(tt.input); (err != nil) != tt.wantErr {
				t.Errorf("CheckName(%q) = %v; wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestFilePath pins the machine file's and its lock's names in the per-user config directory.
// It sets both platforms' variables to one directory, so the answer is the same on every platform.
// It sets process-global environment, so it is not parallel.
func TestFilePath(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("XDG_CONFIG_HOME", base)

	file, lock, ok, err := FilePath()

	if err != nil || !ok {
		t.Fatalf("FilePath() ok, error = %v, %v; want true, nil", ok, err)
	}
	dir := filepath.Join(base, "lym")
	if file != filepath.Join(dir, "machine.json") || lock != filepath.Join(dir, "machine.lock") {
		t.Errorf("FilePath() = %q, %q; want machine.json and machine.lock under %s", file, lock, dir)
	}
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scankit.AssertImportAllowlist(t, "internal/machinename", "github.com/Knatte18/loomyard/internal/standalonestate")
}

func TestSoleSource_NoOtherProductionFileReadsTheHostname(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}, Filter: scankit.Prod}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/machinename/") {
			return
		}
		if strings.Contains(string(f.Data), "os.Hostname(") {
			t.Errorf("%s reads the hostname; only internal/machinename does", f.Rel)
		}
	})
	scankit.RequireFloor(t, scanned, 100, "production files under internal/ and cmd/")
}
