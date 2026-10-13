// fmtverb_test.go covers the `fmt` loom verb over a temporary worktree, with no git repository behind it.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

const (
	misformattedGo = "package p\nfunc  F( ) {}\n"
	formattedGo    = "package p\n\nfunc F() {}\n"
)

// TestFmtCmd drives fmtCmd over a temporary worktree the way the pre-run leaves the receiver: only env.WorktreeRoot and cwd set.
func TestFmtCmd(t *testing.T) {
	t.Parallel()

	newWorktree := func(t *testing.T) string {
		t.Helper()
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		return dir
	}
	write := func(t *testing.T, path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		return string(got)
	}
	run := func(t *testing.T, root, cwd string, args ...string) (int, envelope.Envelope) {
		t.Helper()
		c := &loomCLI{env: shedrecipe.Env{WorktreeRoot: root}, cwd: cwd}
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.fmtCmd(), &out, args)
		return exitCode, envelope.Decode(t, out.String())
	}
	stringList := func(t *testing.T, env envelope.Envelope, key string) []string {
		t.Helper()
		raw, _ := env.Raw[key].([]any)
		got := make([]string, 0, len(raw))
		for _, item := range raw {
			got = append(got, item.(string))
		}
		return got
	}

	t.Run("a misformatted file is rewritten, keeps its mode and is the one changed entry", func(t *testing.T) {
		t.Parallel()
		dir := newWorktree(t)
		path := filepath.Join(dir, "a.go")
		write(t, path, misformattedGo, 0o600)

		exitCode, env := run(t, dir, dir, path)

		if exitCode != 0 || !env.OK {
			t.Fatalf("exit code = %d, ok = %v, error = %q; want 0, true", exitCode, env.OK, env.Error)
		}
		if got := stringList(t, env, "changed"); !slices.Equal(got, []string{path}) {
			t.Errorf("changed = %v; want [%s]", got, path)
		}
		if got := read(t, path); got != formattedGo {
			t.Errorf("file = %q; want %q", got, formattedGo)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v; want 0600", info.Mode().Perm())
			}
		}
	})

	t.Run("an already formatted file keeps its bytes and modification time and is not reported", func(t *testing.T) {
		t.Parallel()
		dir := newWorktree(t)
		path := filepath.Join(dir, "a.go")
		write(t, path, formattedGo, 0o644)
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}

		exitCode, env := run(t, dir, dir, path)

		if exitCode != 0 || !env.OK {
			t.Fatalf("exit code = %d, ok = %v, error = %q; want 0, true", exitCode, env.OK, env.Error)
		}
		if got := stringList(t, env, "changed"); len(got) != 0 {
			t.Errorf("changed = %v; want empty", got)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.ModTime().Equal(past) {
			t.Errorf("modification time = %v; want %v", info.ModTime(), past)
		}
		if got := read(t, path); got != formattedGo {
			t.Errorf("file = %q; want %q", got, formattedGo)
		}
	})

	t.Run("a relative path resolves against the cwd", func(t *testing.T) {
		t.Parallel()
		dir := newWorktree(t)
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		write(t, filepath.Join(sub, "a.go"), misformattedGo, 0o644)

		exitCode, env := run(t, dir, sub, "a.go")

		if exitCode != 0 || !env.OK {
			t.Fatalf("exit code = %d, ok = %v, error = %q; want 0, true", exitCode, env.OK, env.Error)
		}
		if got := stringList(t, env, "changed"); !slices.Equal(got, []string{"a.go"}) {
			t.Errorf("changed = %v; want [a.go]", got)
		}
		if got := read(t, filepath.Join(sub, "a.go")); got != formattedGo {
			t.Errorf("file = %q; want %q", got, formattedGo)
		}
	})

	t.Run("no file named refuses with its way forward", func(t *testing.T) {
		t.Parallel()
		dir := newWorktree(t)

		exitCode, env := run(t, dir, dir)

		if exitCode != 1 || env.OK {
			t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		if want := `way forward: run "lyx loom fmt <file>..." with the Go files to format`; !strings.HasSuffix(env.Error, want) {
			t.Errorf("error = %q; want it to end %q", env.Error, want)
		}
	})

	refusals := []struct {
		name string
		// setup creates the refused files under dir and returns the arguments naming them and the refused entries wanted, in order.
		setup func(t *testing.T, dir string) (args []string, wantRefused []string)
	}{
		{
			name: "a non-go path",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				write(t, filepath.Join(dir, "notes.txt"), "x", 0o644)
				return []string{"notes.txt"}, []string{"notes.txt: not a .go file"}
			},
		},
		{
			name: "a path outside the worktree",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				outside := filepath.Join(newWorktree(t), "o.go")
				write(t, outside, formattedGo, 0o644)
				return []string{outside}, []string{outside + ": outside this worktree"}
			},
		},
		{
			name: "a symlink resolving outside the worktree",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				outside := filepath.Join(newWorktree(t), "o.go")
				write(t, outside, formattedGo, 0o644)
				if err := os.Symlink(outside, filepath.Join(dir, "link.go")); err != nil {
					t.Skipf("symlink not permitted on this platform: %v", err)
				}
				return []string{"link.go"}, []string{"link.go: outside this worktree"}
			},
		},
		{
			name: "an unparsable file",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				write(t, filepath.Join(dir, "bad.go"), "package p\nfunc (", 0o644)
				return []string{"bad.go"}, []string{"bad.go: does not parse: "}
			},
		},
		{
			name: "a nonexistent go path",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				return []string{"missing.go"}, []string{"missing.go: no such file"}
			},
		},
		{
			name: "a directory named dir.go",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				if err := os.Mkdir(filepath.Join(dir, "dir.go"), 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				return []string{"dir.go"}, []string{"dir.go: "}
			},
		},
		{
			name: "a non-go path and an unparsable file together",
			setup: func(t *testing.T, dir string) ([]string, []string) {
				write(t, filepath.Join(dir, "notes.txt"), "x", 0o644)
				write(t, filepath.Join(dir, "bad.go"), "package p\nfunc (", 0o644)
				return []string{"notes.txt", "bad.go"}, []string{"notes.txt: not a .go file", "bad.go: does not parse: "}
			},
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name+" refuses the whole call and leaves a misformatted file unwritten", func(t *testing.T) {
			t.Parallel()
			dir := newWorktree(t)
			good := filepath.Join(dir, "good.go")
			write(t, good, misformattedGo, 0o644)
			args, wantRefused := tt.setup(t, dir)

			exitCode, env := run(t, dir, dir, append([]string{"good.go"}, args...)...)

			if exitCode != 1 || env.OK {
				t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
			}
			wantEnd := `way forward: name only .go files inside this worktree that parse, then re-run "lyx loom fmt"`
			if !strings.HasSuffix(env.Error, wantEnd) {
				t.Errorf("error = %q; want it to end %q", env.Error, wantEnd)
			}
			refused := stringList(t, env, "refused")
			if len(refused) != len(wantRefused) {
				t.Fatalf("refused = %v; want %d entries starting %v", refused, len(wantRefused), wantRefused)
			}
			for i, want := range wantRefused {
				if !strings.HasPrefix(refused[i], want) {
					t.Errorf("refused[%d] = %q; want prefix %q", i, refused[i], want)
				}
			}
			if got := read(t, good); got != misformattedGo {
				t.Errorf("good.go = %q; want it left as %q", got, misformattedGo)
			}
		})
	}

	t.Run("a write that fails partway reports the files written and the one that failed", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("a read-only file stops a write only for a non-root user off Windows")
		}
		dir := newWorktree(t)
		first := filepath.Join(dir, "first.go")
		second := filepath.Join(dir, "second.go")
		write(t, first, misformattedGo, 0o644)
		write(t, second, misformattedGo, 0o444)

		exitCode, env := run(t, dir, dir, "first.go", "second.go")

		if exitCode != 1 || env.OK {
			t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		wantEnd := `way forward: re-run "lyx loom fmt" with the same files once the write error is fixed; a formatted file is left untouched`
		if !strings.HasSuffix(env.Error, wantEnd) {
			t.Errorf("error = %q; want it to end %q", env.Error, wantEnd)
		}
		if got := stringList(t, env, "written"); !slices.Equal(got, []string{"first.go"}) {
			t.Errorf("written = %v; want [first.go]", got)
		}
		if failed, _ := env.Raw["failed"].(string); !strings.HasPrefix(failed, "second.go: ") {
			t.Errorf("failed = %q; want it to name second.go", failed)
		}
		if got := read(t, first); got != formattedGo {
			t.Errorf("first.go = %q; want %q", got, formattedGo)
		}
		if got := read(t, second); got != misformattedGo {
			t.Errorf("second.go = %q; want it left as %q", got, misformattedGo)
		}
	})
}
