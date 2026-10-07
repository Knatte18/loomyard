//go:build integration

// envfile_integration_test.go runs the env file Prepare writes under `bash` in Claude Code's prelude shape,
// and pins that it closes the shell's default stdin without touching a stdin a command supplies itself.

package claudeengine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestEnvFile_DefaultStdinIsDevNull runs each command as `bash -c '<env file content>eval <command>'` with the shell's stdin an open pipe nothing ever writes to or closes.
func TestEnvFile_DefaultStdinIsDevNull(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if _, err := New().Prepare(runDir, shuttleengine.Spec{Prompt: "p"}, templateConfig(t)); err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	envFile, err := os.ReadFile(filepath.Join(runDir, envFileName))
	if err != nil {
		t.Fatalf("read %s: %v", envFileName, err)
	}

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"bare_cat_ends_at_once", "cat", ""},
		{"heredoc_still_feeds_cat", "cat <<'EOF'\nheredoc body\nEOF", "heredoc body\n"},
		{"pipe_still_feeds_cat", "printf data | bash -c 'cat'", "data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()

			// The bound fails a row whose cat still reads the open pipe, instead of hanging the suite.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", string(envFile)+"eval "+shell.Posix().Quote(tt.command))
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
