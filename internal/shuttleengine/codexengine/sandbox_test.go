// sandbox_test.go pins the sandbox probe against a fake Codex executable it runs as a real process.

package codexengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunSandboxProbe pins the probe's command and its three answers.
// It runs `<bin> sandbox` with the workspace-write policy over the probe command, in the dir and under the home given.
// An exit 0 reads as a sandbox that starts;
// any other exit reads as one that cannot, with the executable's output as the cause.
// An executable that cannot be run, or that a signal ends, gives an error wrapping errSandboxProbeNotRun.
// It writes an executable and runs it, so it does not run in parallel:
// a fork in a concurrent test could hold the file open for writing and fail the run with "text file busy".
func TestRunSandboxProbe(t *testing.T) {
	tiergate.Require(t, 1)

	const cause = "bwrap: setting up uid map: Permission denied"
	tests := []struct {
		name string
		// exitCode is the fake executable's exit status; a non-zero one also prints the cause.
		exitCode int
		// wantErr holds the texts the probe's error must carry; empty means the probe passes.
		wantErr []string
		// missingBin points the probe at an executable that does not exist.
		missingBin bool
		// killed makes the fake executable kill itself with a signal instead of exiting.
		killed bool
		// wantNotRun reports whether the error must wrap errSandboxProbeNotRun.
		wantNotRun bool
	}{
		{name: "SandboxStarts"},
		{name: "SandboxCannotStart", exitCode: 1, wantErr: []string{"exit status 1", cause}},
		{name: "ExecutableMissing", missingBin: true, wantNotRun: true},
		{name: "ProbeSignalled", killed: true, wantErr: []string{"signal: killed"}, wantNotRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve temp dir: %v", err)
			}
			home := filepath.Join(dir, "home")
			argsFile := filepath.Join(dir, "args")
			bin := filepath.Join(dir, "codex")
			script := "#!/bin/sh\nprintf '%s\\n' \"$" + codexHomeEnvKey + "\" \"$(pwd -P)\" \"$@\" > " + shell.Posix().Quote(argsFile) + "\n"
			if tt.exitCode != 0 {
				script += "echo '" + cause + "' >&2\n"
			}
			if tt.killed {
				script += "kill -9 $$\n"
			} else {
				script += "exit " + strconv.Itoa(tt.exitCode) + "\n"
			}
			if tt.missingBin {
				err = runSandboxProbe(filepath.Join(dir, "absent"), home, dir)
				if !errors.Is(err, errSandboxProbeNotRun) {
					t.Errorf("runSandboxProbe() error = %v; want it to wrap errSandboxProbeNotRun", err)
				}
				return
			}
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatalf("write fake codex: %v", err)
			}

			err = runSandboxProbe(bin, home, dir)
			if got := errors.Is(err, errSandboxProbeNotRun); got != tt.wantNotRun {
				t.Errorf("runSandboxProbe() error = %v; wraps errSandboxProbeNotRun = %t; want %t", err, got, tt.wantNotRun)
			}

			if len(tt.wantErr) == 0 && err != nil {
				t.Errorf("runSandboxProbe() error: %v; want none", err)
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("runSandboxProbe() error = %v; want it to contain %q", err, want)
				}
			}
			raw, readErr := os.ReadFile(argsFile)
			if readErr != nil {
				t.Fatalf("read the fake's arguments: %v", readErr)
			}
			got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			want := []string{home, dir, "sandbox", "-c", `sandbox_mode="` + sandboxWorkspaceWrite + `"`, "--", sandboxProbeCommand}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("fake codex got CODEX_HOME, dir and arguments %q; want %q", got, want)
			}
		})
	}
}
