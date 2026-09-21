// panebin_test.go hermetically covers panebin.go's composition: paneBinPrelude and
// composePaneLaunchLine, driven directly against injected inputs with no tmux and no process
// spawned. Every case injects the executable path by overriding executablePath and restoring it via
// t.Cleanup -- under go test the live os.Executable() value is the test binary's path, which
// CONSTRAINTS.md's Live-Substrate Spawn Observability clause bars re-exec'ing, so no case here reads
// it.

package reedengine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
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

// TestComposePaneLaunchLine_PreludeThenCommand drives composePaneLaunchLine with a non-empty launch
// command and an injected executable path.
func TestComposePaneLaunchLine_PreludeThenCommand(t *testing.T) {
	const exe = "/opt/lyx/bin/lyx"
	withInjectedExecutablePath(t, func() (string, error) { return exe, nil })

	const launchCmd = "claude --continue"
	sh := shell.Posix()
	got := composePaneLaunchLine(sh, launchCmd, "strand-guid")

	if strings.Contains(got, "\n") {
		t.Errorf("composePaneLaunchLine(...) = %q, want a single line with no newline", got)
	}
	if !strings.HasSuffix(got, launchCmd) {
		t.Errorf("composePaneLaunchLine(...) = %q, want it to end with the unchanged launch command %q", got, launchCmd)
	}

	prelude := paneBinPrelude(sh, exe)
	want := prelude + "; " + launchCmd
	if got != want {
		t.Errorf("composePaneLaunchLine(...) = %q, want %q", got, want)
	}

	prependIdx := strings.Index(got, "PATH")
	exportIdx := strings.Index(got, lyxBinEnvKey)
	cmdIdx := strings.Index(got, launchCmd)
	if !(prependIdx < exportIdx && exportIdx < cmdIdx) {
		t.Errorf("composePaneLaunchLine(...) = %q, want the PATH prepend, the %s export and the command in that order", got, lyxBinEnvKey)
	}
}

// TestComposePaneLaunchLine_EmptyCmdEmitsThePreludeAlone drives the empty-command case: the shape the
// interactive operator strand produces, since that strand is added with no command at all.
func TestComposePaneLaunchLine_EmptyCmdEmitsThePreludeAlone(t *testing.T) {
	const exe = "/opt/lyx/bin/lyx"
	withInjectedExecutablePath(t, func() (string, error) { return exe, nil })

	sh := shell.Posix()
	got := composePaneLaunchLine(sh, "", "strand-guid")
	want := paneBinPrelude(sh, exe)
	if got != want {
		t.Errorf("composePaneLaunchLine(sh, \"\", ...) = %q, want %q (the prelude alone, no trailing separator, no empty trailing fragment)", got, want)
	}
	if strings.HasSuffix(got, "; ") {
		t.Errorf("composePaneLaunchLine(sh, \"\", ...) = %q, want no trailing separator", got)
	}
}

// TestComposePaneLaunchLine_ExecutableErrorWarnsAndPassesTheCommandThrough overrides executablePath
// with a function returning an error, captures logs via captureLogOutput, and asserts the launch
// command passes through unchanged with a named warning logged.
func TestComposePaneLaunchLine_ExecutableErrorWarnsAndPassesTheCommandThrough(t *testing.T) {
	wantErr := errors.New("executable path unresolvable")
	withInjectedExecutablePath(t, func() (string, error) { return "", wantErr })
	buf := captureLogOutput(t)

	const launchCmd = "claude --continue"
	got := composePaneLaunchLine(shell.Posix(), launchCmd, "strand-guid-1")
	if got != launchCmd {
		t.Errorf("composePaneLaunchLine(...) = %q, want the launch command %q byte-for-byte unchanged", got, launchCmd)
	}
	if !strings.Contains(buf.String(), "strand-guid-1") {
		t.Errorf("captured log output = %q, want it to name the strand %q", buf.String(), "strand-guid-1")
	}

	t.Run("empty command", func(t *testing.T) {
		buf.Reset()
		got := composePaneLaunchLine(shell.Posix(), "", "strand-guid-2")
		if got != "" {
			t.Errorf("composePaneLaunchLine(sh, \"\", ...) with an executable-path error = %q, want the empty string", got)
		}
	})
}

// TestComposePaneLaunchLine_UsesTheSameDialectAsTheLaunchCommand asserts that
// composePaneLaunchLine(shell.ForGOOS(), ...) produces the dialect shell.ForGOOS() itself produces on
// the running host, so the prelude and the ForGOOS()-built launch command can never diverge. It must
// not branch on runtime.GOOS.
func TestComposePaneLaunchLine_UsesTheSameDialectAsTheLaunchCommand(t *testing.T) {
	const exe = "/opt/lyx/bin/lyx"
	withInjectedExecutablePath(t, func() (string, error) { return exe, nil })

	sh := shell.ForGOOS()
	got := composePaneLaunchLine(sh, "claude --continue", "strand-guid")
	want := sh.Chain(paneBinPrelude(sh, exe), "claude --continue")
	if got != want {
		t.Errorf("composePaneLaunchLine(shell.ForGOOS(), ...) = %q, want %q (built from the same shell.ForGOOS() dialect)", got, want)
	}
}

// TestComposePaneLaunchLine_DashLeadingLineStillRoundTripsThroughSendKeysLiteralArg asserts the
// composed string is opaque to sendKeysLiteralArg's dash guard: nothing downstream may assume the
// payload starts with the strand's own command.
func TestComposePaneLaunchLine_DashLeadingLineStillRoundTripsThroughSendKeysLiteralArg(t *testing.T) {
	// A synthetic dash-leading composed string, since neither dialect's real prelude begins with
	// '-' -- the property under test is sendKeysLiteralArg's own guard, which must not assume
	// anything about how the composed line was built.
	const composed = "-join('a','b'); echo hi"
	got := sendKeysLiteralArg(composed)
	want := " " + composed
	if got != want {
		t.Errorf("sendKeysLiteralArg(%q) = %q, want %q (a single leading space, since tmux parses a '-'-leading literal argument as flags)", composed, got, want)
	}
}
