//go:build integration

// envfile_integration_test.go runs the env file content under `bash` in Claude Code's prelude shape.
// It pins that the content closes the shell's default stdin without touching a stdin a command supplies itself,
// and that it re-prepends LYX_BIN's directory to PATH.

package claudeengine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/shell"
)

// writeExecutable writes an executable file at path with the given script content.
func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestEnvFile_DefaultStdinIsDevNull runs each command as `bash -c '<env file content>eval <command>'` with the shell's stdin an open pipe nothing ever writes to or closes.
// It pins the default stdin and the PATH re-prepend: an operator rc that left a decoy `lyx` first on PATH is overridden by LYX_BIN's directory.
func TestEnvFile_DefaultStdinIsDevNull(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	decoyDir := filepath.Join(root, "decoy")
	realDir := filepath.Join(root, "real dir")
	stubDir := filepath.Join(root, "stub")
	decoyLyx := filepath.Join(decoyDir, "lyx")
	realLyx := filepath.Join(realDir, "lyx")
	writeExecutable(t, decoyLyx, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, realLyx, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(stubDir, "cygpath"), "#!/bin/sh\nfor arg; do last=$arg; done\nprintf %s \"$last\"\n")

	hostPath := os.Getenv("PATH")
	decoyPath := decoyDir + string(os.PathListSeparator) + hostPath
	stubPath := stubDir + string(os.PathListSeparator) + decoyPath

	var baseEnv []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PATH=") || strings.HasPrefix(entry, agentname.LyxBinEnv+"=") {
			continue
		}
		baseEnv = append(baseEnv, entry)
	}

	lyxBinSet := agentname.LyxBinEnv + "=" + realLyx
	lyxBinEmpty := agentname.LyxBinEnv + "="
	tests := []struct {
		name          string
		goos          string
		command       string
		path          string
		extraEnv      []string
		want          string
		skipOnWindows bool
	}{
		{"bare_cat_ends_at_once", "linux", "cat", hostPath, nil, "", false},
		{"heredoc_still_feeds_cat", "linux", "cat <<'EOF'\nheredoc body\nEOF", hostPath, nil, "heredoc body\n", false},
		{"pipe_still_feeds_cat", "linux", "printf data | bash -c 'cat'", hostPath, nil, "data", false},
		{"posix_lyx_bin_directory_shadows_decoy", "linux", "command -v lyx", decoyPath, []string{lyxBinSet}, realLyx + "\n", true},
		{"posix_lyx_bin_unset_leaves_path", "linux", `printf %s "$PATH"`, decoyPath, nil, decoyPath, true},
		{"posix_lyx_bin_empty_leaves_path", "linux", `printf %s "$PATH"`, decoyPath, []string{lyxBinEmpty}, decoyPath, true},
		{"windows_without_cygpath_leaves_path", "windows", `printf %s "$PATH"`, decoyPath, []string{lyxBinSet}, decoyPath, true},
		{"windows_lyx_bin_unset_leaves_path", "windows", `printf %s "$PATH"`, stubPath, nil, stubPath, true},
		{"windows_converts_and_unsets_scratch_variable", "windows", `command -v lyx; printf %s "${__lyx_bin_dir-unset}"`, stubPath, []string{lyxBinSet}, realLyx + "\nunset", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skip("the PATH rows build POSIX paths")
			}

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()

			// The bound fails a row whose cat still reads the open pipe, instead of hanging the suite.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", envFileContentFor(tt.goos)+"eval "+shell.Posix().Quote(tt.command))
			cmd.Env = append(append([]string{}, baseEnv...), append([]string{"PATH=" + tt.path}, tt.extraEnv...)...)
			cmd.Stdin = r
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("bash exited with %v (ctx: %v); stdout=%q", err, ctx.Err(), out.String())
			}
			if got := out.String(); got != tt.want {
				t.Errorf("output = %q; want %q", got, tt.want)
			}
		})
	}
}
