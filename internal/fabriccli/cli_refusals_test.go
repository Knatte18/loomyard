// cli_refusals_test.go covers the fabric CLI's refusals and help output that arise before any hub or git work:
// the --warp-path/--weft-path push-only gate, clone's positional arity and merge's usage surface.
// Every case runs against a bare temporary directory, spawning nothing.

package fabriccli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestRunCLI_RefusesBeforeAnyHubWork(t *testing.T) {
	t.Parallel()

	const worktreeContextRefusal = "subcommand requires a worktree context"

	tests := []struct {
		name string
		// args may hold the placeholder "<dir>", replaced with a fresh temporary directory.
		args []string
		// wantExit is 0 for help output and 1 for an error envelope.
		wantExit int
		// wantErrorContains is the substring the error envelope's message must hold.
		wantErrorContains string
		// wantErrorExact, when set, is the whole error message.
		wantErrorExact string
		// wantOutputContains are substrings the help output must hold.
		wantOutputContains []string
	}{
		{
			name:              "WeftPathWithNonPushVerb",
			args:              []string{"--weft-path", "<dir>", "status"},
			wantExit:          1,
			wantErrorContains: worktreeContextRefusal,
			wantErrorExact:    worktreeContextRefusal,
		},
		{
			name:              "WarpPathWithNonPushVerb",
			args:              []string{"--warp-path", "<dir>", "status"},
			wantExit:          1,
			wantErrorContains: worktreeContextRefusal,
			wantErrorExact:    worktreeContextRefusal,
		},
		{
			name:              "CloneWithZeroArgs",
			args:              []string{"clone"},
			wantExit:          1,
			wantErrorContains: "usage: lyx fabric clone",
		},
		{
			name:              "CloneWithThreeArgs",
			args:              []string{"clone", "https://example.com/weft", "https://example.com/warp", "https://example.com/board"},
			wantExit:          1,
			wantErrorContains: "usage: lyx fabric clone",
		},
		{
			name:              "MergeContinueRejectsExtraArg",
			args:              []string{"merge", "--continue", "extra-arg"},
			wantExit:          1,
			wantErrorContains: "usage:",
		},
		{
			name:               "MergeHelpNamesAllFourFlags",
			args:               []string{"merge", "--help"},
			wantExit:           0,
			wantOutputContains: []string{"--squash", "--continue", "--abort", "--message"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			args := make([]string, len(tt.args))
			for i, arg := range tt.args {
				if arg == "<dir>" {
					arg = dir
				}
				args[i] = arg
			}

			var out bytes.Buffer
			exitCode := fabriccli.RunCLIIn(dir, &out, args)
			if exitCode != tt.wantExit {
				t.Fatalf("RunCLI(%v) = %d; want %d\noutput: %s", tt.args, exitCode, tt.wantExit, out.String())
			}

			for _, want := range tt.wantOutputContains {
				if !strings.Contains(out.String(), want) {
					t.Errorf("RunCLI(%v) output missing %q; got:\n%s", tt.args, want, out.String())
				}
			}
			if tt.wantExit == 0 {
				return
			}
			result := envelope.RequireErr(t, out.String(), tt.wantErrorContains)
			if tt.wantErrorExact != "" && result.Error != tt.wantErrorExact {
				t.Errorf("error message = %q; want %q", result.Error, tt.wantErrorExact)
			}
		})
	}
}
