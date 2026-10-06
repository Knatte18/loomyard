// panebin_test.go hermetically covers panebin.go's composition: paneBinPrelude and
// composePaneLaunchLine, driven directly against injected inputs with no tmux and no process
// spawned. Every case injects the executable path by overriding executablePath and restoring it via
// t.Cleanup -- under go test the live os.Executable() value is the test binary's path, which
// PATTERN-spawn-observability bars re-exec'ing, so no case here reads
// it.
// The stageLaunchScript cases at the end cover the per-strand launch script file.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// withInjectedExecutablePath overrides executablePath for the duration of t, restoring the previous
// value on cleanup.
func withInjectedExecutablePath(t *testing.T, fn func() (string, error)) {
	t.Helper()
	prev := executablePath
	executablePath = fn
	t.Cleanup(func() { executablePath = prev })
}

// TestPaneBinPrelude_ComposesPrependThenExport drives paneBinPrelude directly, once per dialect.
//
//testtiming:keep pins the prelude's own shape per dialect: one line, the PATH prepend of the executable's directory before the LYX_BIN export of the full path, joined by "; "; its covering tests run this code without asserting it
func TestPaneBinPrelude_ComposesPrependThenExport(t *testing.T) {
	dialects := []struct {
		name string
		sh   shell.Shell
	}{
		{"posix", shell.Posix()},
		{"pwsh", shell.Pwsh()},
	}
	const exe = "/opt/lyx/bin/lyx"
	dir := filepath.Dir(exe)

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			got := paneBinPrelude(d.sh, exe)

			if strings.Contains(got, "\n") {
				t.Errorf("paneBinPrelude(%s, %q) = %q, want a single line with no newline", d.name, exe, got)
			}

			prependIdx := strings.Index(got, dir)
			if prependIdx < 0 {
				t.Fatalf("paneBinPrelude(%s, %q) = %q, want it to name the executable's parent directory %q", d.name, exe, got, dir)
			}
			exportIdx := strings.Index(got, exe)
			if exportIdx < 0 {
				t.Fatalf("paneBinPrelude(%s, %q) = %q, want it to name the full executable path %q", d.name, exe, got, exe)
			}
			// The directory is a prefix of exe, so the first occurrence of dir inside got may be
			// the one inside the export statement rather than the prepend. Locate the export
			// statement itself via lyxBinEnvKey, which is unique to it, and compare positions
			// against that.
			exportStmtIdx := strings.Index(got, lyxBinEnvKey)
			if exportStmtIdx < 0 {
				t.Fatalf("paneBinPrelude(%s, %q) = %q, want it to carry the %s export", d.name, exe, got, lyxBinEnvKey)
			}
			if prependIdx >= exportStmtIdx {
				t.Errorf("paneBinPrelude(%s, %q) = %q, want the PATH prepend to appear before the %s export", d.name, exe, got, lyxBinEnvKey)
			}

			wantJoiner := "; "
			if !strings.Contains(got, wantJoiner) {
				t.Errorf("paneBinPrelude(%s, %q) = %q, want the two statements joined by %q", d.name, exe, got, wantJoiner)
			}
			wantPrepend := d.sh.PrependPathEntry(dir)
			wantExport := d.sh.ExportEnv(lyxBinEnvKey, exe)
			want := wantPrepend + wantJoiner + wantExport
			if got != want {
				t.Errorf("paneBinPrelude(%s, %q) = %q, want %q", d.name, exe, got, want)
			}
		})
	}
}

// TestComposePaneLaunchLine drives composePaneLaunchLine in both dialects with an injected executable path:
// the prelude, then the name export (always when a name is told), the parent export (only when a parent is told), then the command, all on one line;
// an empty command emits the prelude alone with no trailing separator;
// and a failed executable lookup drops only the prelude, passing the command and exports through with a warning naming the strand.
//
//testtiming:keep pins the composed pane launch line per dialect: prelude, name and parent exports then the command on one line, the prelude alone for an empty command, and only the prelude dropped with a warning naming the strand when the executable cannot be resolved; its covering tests run this code without asserting it
func TestComposePaneLaunchLine(t *testing.T) {
	const (
		exe       = "/opt/lyx/bin/lyx"
		launchCmd = "claude --continue"
		name      = "tst:wt:driver"
		parent    = "tst:wt:orch"
		guid      = "strand-guid-1"
	)
	noExecutable := errors.New("executable path unresolvable")
	nameExport := func(sh shell.Shell) string { return sh.ExportEnv(agentname.StrandNameEnv, name) }
	parentExport := func(sh shell.Shell) string { return sh.ExportEnv(agentname.ParentEnv, parent) }

	tests := []struct {
		name       string
		executable error
		cmd        string
		strand     string
		parent     string
		want       func(sh shell.Shell) string
	}{
		{
			name: "PreludeThenCommand",
			cmd:  launchCmd,
			want: func(sh shell.Shell) string { return sh.Chain(paneBinPrelude(sh, exe), launchCmd) },
		},
		{
			// The shape a strand added with no command produces (for example `lyx reed add` without `--cmd`).
			name: "EmptyCmdEmitsThePreludeAlone",
			want: func(sh shell.Shell) string { return paneBinPrelude(sh, exe) },
		},
		{
			name:   "NameExportWithoutParent",
			cmd:    launchCmd,
			strand: name,
			want:   func(sh shell.Shell) string { return sh.Chain(paneBinPrelude(sh, exe), nameExport(sh), launchCmd) },
		},
		{
			name:   "NameAndParentExports",
			cmd:    launchCmd,
			strand: name,
			parent: parent,
			want: func(sh shell.Shell) string {
				return sh.Chain(paneBinPrelude(sh, exe), nameExport(sh), parentExport(sh), launchCmd)
			},
		},
		{
			name:       "ExecutableErrorPassesTheCommandThrough",
			executable: noExecutable,
			cmd:        launchCmd,
			want:       func(sh shell.Shell) string { return launchCmd },
		},
		{
			name:       "ExecutableErrorWithEmptyCommandIsEmpty",
			executable: noExecutable,
			want:       func(sh shell.Shell) string { return "" },
		},
		{
			name:       "ExportsSurviveAnExecutableError",
			executable: noExecutable,
			cmd:        launchCmd,
			strand:     name,
			parent:     parent,
			want:       func(sh shell.Shell) string { return sh.Chain(nameExport(sh), parentExport(sh), launchCmd) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withInjectedExecutablePath(t, func() (string, error) {
				if tt.executable != nil {
					return "", tt.executable
				}
				return exe, nil
			})
			for _, sh := range launchScriptDialects() {
				buf := logcapture.CaptureVerbose(t)

				got := composePaneLaunchLine(sh, tt.cmd, guid, tt.strand, tt.parent)

				if want := tt.want(sh); got != want {
					t.Errorf("composePaneLaunchLine(...) = %q, want %q", got, want)
				}
				if strings.Contains(got, "\n") {
					t.Errorf("composePaneLaunchLine(...) = %q, want a single line with no newline", got)
				}
				if tt.cmd == "" && strings.HasSuffix(got, "; ") {
					t.Errorf("composePaneLaunchLine(...) = %q, want no trailing separator", got)
				}
				if tt.executable != nil {
					if strings.Contains(got, lyxBinEnvKey) {
						t.Errorf("composePaneLaunchLine(...) = %q, want no %s prelude", got, lyxBinEnvKey)
					}
					if !strings.Contains(buf.String(), guid) {
						t.Errorf("captured log output = %q, want it to name the strand %q", buf.String(), guid)
					}
				}
			}
		})
	}
}

// launchScriptDialects lists both dialects for the composePaneLaunchLine and stageLaunchScript cases.
func launchScriptDialects() []shell.Shell {
	return []shell.Shell{shell.Posix(), shell.Pwsh()}
}

// TestStageLaunchScript pins per dialect that the payload is the dialect's source command for the script path,
// the file holds the last staged line and a newline (an empty line still writes a newline-only script),
// regenerating replaces the content without leftovers, and the script carries 0o644 on non-Windows hosts.
func TestStageLaunchScript(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"WritesLineAndReturnsSource", []string{"echo hi"}},
		{"EmptyLineWritesANewlineOnlyScript", []string{""}},
		{"RegenerateReplacesContent", []string{"one", "two"}},
	}
	for _, tt := range tests {
		for _, sh := range launchScriptDialects() {
			t.Run(tt.name, func(t *testing.T) {
				stateDir := t.TempDir()
				path := launchScriptPath(sh, stateDir, "g")

				var got string
				for _, line := range tt.lines {
					got = stageLaunchScript(sh, stateDir, "g", line)
				}

				if want := sh.Source(path); got != want {
					t.Errorf("payload = %q, want %q", got, want)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if want := tt.lines[len(tt.lines)-1] + "\n"; string(data) != want {
					t.Errorf("content = %q, want %q", data, want)
				}
				entries, err := os.ReadDir(launchScriptDir(stateDir))
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 1 {
					t.Errorf("launch dir holds %d entries, want 1", len(entries))
				}
				if runtime.GOOS != "windows" {
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if info.Mode().Perm() != 0o644 {
						t.Errorf("mode = %o, want 644", info.Mode().Perm())
					}
				}
			})
		}
	}
}

// TestStageLaunchScript_WriteFailureDegrades checks an unwritable directory returns the line and warns.
//
//testtiming:keep pins an unwritable launch directory returning the composed line unchanged and logging the strand and script path; its covering tests run this code without asserting it
func TestStageLaunchScript_WriteFailureDegrades(t *testing.T) {
	sh := shell.Posix()
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "reed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := logcapture.CaptureVerbose(t)
	got := stageLaunchScript(sh, stateDir, "strand-x", "echo hi")
	if got != "echo hi" {
		t.Errorf("payload = %q, want the composed line", got)
	}
	out := buf.String()
	if !strings.Contains(out, "strand-x") || !strings.Contains(out, launchScriptPath(sh, stateDir, "strand-x")) {
		t.Errorf("log %q does not name the strand and path", out)
	}
}
