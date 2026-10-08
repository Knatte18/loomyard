// keybindings_test.go covers the keybindings block merge, the seeding of the user file and the per-platform path.

package vscode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeKeybindings(t *testing.T) {
	t.Parallel()

	block := keybindingsBlock("\n")
	crlfBlock := keybindingsBlock("\r\n")
	// staleBlock is a block from an older lyx, holding one entry the current block does not.
	staleBlock := "// lyx:begin\n  { \"key\": \"alt+q\" },\n// lyx:end"

	tests := []struct {
		name     string
		existing string
		want     string
		wantErr  string
	}{
		{
			name:     "inserted after the bracket when the entries follow on their own lines",
			existing: "[\n  { \"key\": \"ctrl+k\" }\n]\n",
			want:     "[\n" + block + "\n  { \"key\": \"ctrl+k\" }\n]\n",
		},
		{
			name:     "inserted ahead of a last entry that has a trailing comma",
			existing: "[\n  { \"key\": \"ctrl+k\" },\n]\n",
			want:     "[\n" + block + "\n  { \"key\": \"ctrl+k\" },\n]\n",
		},
		{
			name:     "inserted into an empty array on its own lines",
			existing: "[]",
			want:     "[\n" + block + "\n]",
		},
		{
			name:     "a bracket, a marker and a block comment inside strings and comments are not read",
			existing: "// [ lyx:begin\n[\n  /* ] // lyx:begin */\n  { \"key\": \"// lyx:begin ]\" }, // lyx:begin and more\n]\n",
			want:     "// [ lyx:begin\n[\n" + block + "\n  /* ] // lyx:begin */\n  { \"key\": \"// lyx:begin ]\" }, // lyx:begin and more\n]\n",
		},
		{
			name:     "an existing block is replaced in place, markers included",
			existing: "[\n  { \"key\": \"ctrl+k\" },\n  " + staleBlock + "\n  { \"key\": \"ctrl+j\" },\n]\n",
			want:     "[\n  { \"key\": \"ctrl+k\" },\n  " + block + "\n  { \"key\": \"ctrl+j\" },\n]\n",
		},
		{
			name:     "a file with the current block is returned unchanged",
			existing: "[\n" + block + "\n  { \"key\": \"ctrl+k\" }\n]\n",
			want:     "[\n" + block + "\n  { \"key\": \"ctrl+k\" }\n]\n",
		},
		{
			name:     "a CRLF file keeps CRLF",
			existing: "[\r\n  { \"key\": \"ctrl+k\" }\r\n]\r\n",
			want:     "[\r\n" + crlfBlock + "\r\n  { \"key\": \"ctrl+k\" }\r\n]\r\n",
		},
		{name: "no top-level array", existing: "{ \"key\": 1 }", wantErr: "no top-level array"},
		{name: "an empty file", existing: "", wantErr: "no top-level array"},
		{name: "a begin without an end", existing: "[\n// lyx:begin\n]", wantErr: "no lyx:end"},
		{name: "an end without a begin", existing: "[\n// lyx:end\n]", wantErr: "no lyx:begin"},
		{name: "a repeated begin", existing: "[\n// lyx:begin\n// lyx:begin\n// lyx:end\n]", wantErr: "repeated"},
		{name: "an end before its begin", existing: "[\n// lyx:end\n// lyx:begin\n]", wantErr: "before"},
		{name: "a marker outside the array", existing: "// lyx:begin\n[\n// lyx:end\n]", wantErr: "outside"},
		{name: "a marker inside an entry", existing: "[\n  { // lyx:begin\n  }\n// lyx:end\n]", wantErr: "outside"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mergeKeybindings([]byte(tt.existing))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("mergeKeybindings error = %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeKeybindings error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("mergeKeybindings =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSeedKeybindings(t *testing.T) {
	t.Parallel()

	block := keybindingsBlock("\n")
	read := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	write := func(t *testing.T, path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	assertNoLeftovers := func(t *testing.T, dir string, want ...string) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, entry := range entries {
			got = append(got, entry.Name())
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("directory holds %v; want %v", got, want)
		}
	}

	t.Run("a missing file is created holding the block, and a second run changes nothing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "keybindings.json")
		if got := SeedKeybindings(path); got.Outcome != KeybindingsCreated {
			t.Fatalf("first run = %+v; want created", got)
		}
		if want := "[\n" + block + "\n]\n"; read(t, path) != want {
			t.Fatalf("file = %q; want %q", read(t, path), want)
		}
		if got := SeedKeybindings(path); got.Outcome != KeybindingsUnchanged || got.Reason != "" {
			t.Fatalf("second run = %+v; want unchanged", got)
		}
		assertNoLeftovers(t, dir, "keybindings.json")
	})

	t.Run("an existing file is rewritten with its permission bits kept", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "keybindings.json")
		write(t, path, "[\n  { \"key\": \"ctrl+k\" }\n]\n", 0o600)
		if got := SeedKeybindings(path); got.Outcome != KeybindingsWritten {
			t.Fatalf("run = %+v; want written", got)
		}
		if want := "[\n" + block + "\n  { \"key\": \"ctrl+k\" }\n]\n"; read(t, path) != want {
			t.Errorf("file = %q; want %q", read(t, path), want)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, %v; want 0600", info.Mode().Perm(), err)
		}
		assertNoLeftovers(t, dir, "keybindings.json")
	})

	t.Run("a read-only file already holding the block is unchanged", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "keybindings.json")
		write(t, path, "[\n"+block+"\n]\n", 0o444)
		if got := SeedKeybindings(path); got.Outcome != KeybindingsUnchanged {
			t.Fatalf("run = %+v; want unchanged", got)
		}
	})

	t.Run("a read-only file needing a change is skipped and untouched", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "keybindings.json")
		const content = "[\n  { \"key\": \"ctrl+k\" }\n]\n"
		write(t, path, content, 0o444)
		got := SeedKeybindings(path)
		if got.Outcome != KeybindingsSkipped || !strings.Contains(got.Reason, "read-only") {
			t.Fatalf("run = %+v; want skipped as read-only", got)
		}
		if read(t, path) != content {
			t.Errorf("file = %q; want it untouched", read(t, path))
		}
		assertNoLeftovers(t, dir, "keybindings.json")
	})

	t.Run("a malformed file is skipped and untouched", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "keybindings.json")
		const content = "[\n// lyx:begin\n]\n"
		write(t, path, content, 0o644)
		got := SeedKeybindings(path)
		if got.Outcome != KeybindingsSkipped || !strings.Contains(got.Reason, "lyx:end") {
			t.Fatalf("run = %+v; want skipped naming the shape", got)
		}
		if read(t, path) != content {
			t.Errorf("file = %q; want it untouched", read(t, path))
		}
	})

	t.Run("an unwritable directory is skipped with no temporary file left", func(t *testing.T) {
		t.Parallel()
		if os.Geteuid() == 0 {
			t.Skip("a root user writes into a read-only directory")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "keybindings.json")
		const content = "[\n  { \"key\": \"ctrl+k\" }\n]\n"
		write(t, path, content, 0o644)
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
		if got := SeedKeybindings(path); got.Outcome != KeybindingsSkipped || got.Reason == "" {
			t.Fatalf("run = %+v; want skipped with a reason", got)
		}
		assertNoLeftovers(t, dir, "keybindings.json")
		if read(t, path) != content {
			t.Errorf("file = %q; want it untouched", read(t, path))
		}
	})

	t.Run("an existing file of the temporary name is skipped and left alone", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "keybindings.json")
		const content = "[\n  { \"key\": \"ctrl+k\" }\n]\n"
		write(t, path, content, 0o644)
		write(t, filepath.Join(dir, "taken.tmp"), "someone else's", 0o644)
		got := seedKeybindings(path, func() string { return "taken.tmp" })
		if got.Outcome != KeybindingsSkipped || got.Reason == "" {
			t.Fatalf("run = %+v; want skipped with a reason", got)
		}
		if read(t, path) != content || read(t, filepath.Join(dir, "taken.tmp")) != "someone else's" {
			t.Errorf("target or the taken temporary file changed")
		}
	})

	t.Run("a directory in the file's place is skipped", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "keybindings.json")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := SeedKeybindings(path); got.Outcome != KeybindingsSkipped || got.Reason == "" {
			t.Fatalf("run = %+v; want skipped with a reason", got)
		}
	})

	t.Run("a missing user directory is skipped", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "absent", "keybindings.json")
		if got := SeedKeybindings(path); got.Outcome != KeybindingsSkipped || got.Reason == "" {
			t.Fatalf("run = %+v; want skipped with a reason", got)
		}
	})

	t.Run("a symlinked path is written at its target", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		target := filepath.Join(dir, "dotfiles-keybindings.json")
		link := filepath.Join(dir, "keybindings.json")
		write(t, target, "[]", 0o644)
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if got := SeedKeybindings(link); got.Outcome != KeybindingsWritten {
			t.Fatalf("run = %+v; want written", got)
		}
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the link was replaced: %v, %v", info, err)
		}
		if want := "[\n" + block + "\n]"; read(t, target) != want {
			t.Errorf("target = %q; want %q", read(t, target), want)
		}
	})
}

func TestUserKeybindingsPath(t *testing.T) {
	t.Parallel()

	env := func(vars map[string]string) func(string) string {
		return func(name string) string { return vars[name] }
	}
	tests := []struct {
		name    string
		goos    string
		vars    map[string]string
		home    string
		want    string
		wantErr bool
	}{
		{"linux default", "linux", nil, "/home/u", filepath.Join("/home/u", ".config", "Code", "User", "keybindings.json"), false},
		{"linux with XDG_CONFIG_HOME", "linux", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "/home/u", filepath.Join("/xdg", "Code", "User", "keybindings.json"), false},
		{"linux with no home", "linux", nil, "", "", true},
		{"darwin", "darwin", nil, "/Users/u", filepath.Join("/Users/u", "Library", "Application Support", "Code", "User", "keybindings.json"), false},
		{"windows", "windows", map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, `C:\Users\u`, `C:\Users\u\AppData\Roaming\Code\User\keybindings.json`, false},
		{"windows with no APPDATA", "windows", nil, `C:\Users\u`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := userKeybindingsPath(tt.goos, env(tt.vars), tt.home)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("userKeybindingsPath = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
