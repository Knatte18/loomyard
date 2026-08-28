// watchdog_test.go pins watchdog.go's pure surface: watchdogOption's validate/normalize contract,
// both embedded templates' watchdog default, resizeHookCommand's exact hook string for both shell
// dialects, tmuxQuoteValue's escaping, and resizeSignalPath's stateDir-anchored derivation.
// Untagged: nothing here spawns a process or sleeps (Test Tier Purity Invariant).

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
)

func TestWatchdogOption(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{"on", "on", true, false},
		{"upper_ON", "ON", true, false},
		{"whitespace_on", " on ", true, false},
		{"off", "off", false, false},
		{"upper_OFF", "OFF", false, false},
		{"whitespace_off", " off ", false, false},
		{"invalid_empty", "", false, true},
		{"invalid_numeric", "1", false, true},
		{"invalid_true", "true", false, true},
		{"invalid_yes", "yes", false, true},
		{"invalid_onn", "onn", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := watchdogOption(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("watchdogOption(%q) = %v, nil; want error", tt.raw, got)
				}
				if !strings.Contains(err.Error(), tt.raw) {
					t.Errorf("watchdogOption(%q) error = %v; want it to contain the offending value", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("watchdogOption(%q) = %v, %v; want %v, nil", tt.raw, got, err, tt.want)
			}
			if got != tt.want {
				t.Errorf("watchdogOption(%q) = %v; want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestWatchdogTemplateDefault_BothGOOS reads both embedded template YAML files directly by relative
// path, since ConfigTemplate() only ever exposes the GOOS this build embedded (template_posix.go
// carries !windows, template_windows.go carries windows) — the same limit
// TestLoadConfig_UninitializedFallsBackToTemplate documents in config_test.go.
func TestWatchdogTemplateDefault_BothGOOS(t *testing.T) {
	for _, path := range []string{"template_posix.yaml", "template_windows.yaml"} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if !strings.Contains(string(raw), "watchdog: ${env:LYX_REED_WATCHDOG:-on}") {
				t.Errorf("%s does not declare the expected watchdog default line", path)
			}
		})
	}

	// The accessor this build actually ships must carry the same line.
	if !strings.Contains(ConfigTemplate(), "watchdog: ${env:LYX_REED_WATCHDOG:-on}") {
		t.Errorf("ConfigTemplate() does not declare the expected watchdog default line")
	}
}

func TestResizeHookCommand_Posix(t *testing.T) {
	const signalPath = "/tmp/wt/.lyx/reed-resize.signal"
	got := resizeHookCommand(shell.Posix(), signalPath)
	want := `run-shell -b ": > '/tmp/wt/.lyx/reed-resize.signal'"`
	if got != want {
		t.Errorf("resizeHookCommand(Posix(), %q) = %q; want %q", signalPath, got, want)
	}
	if !strings.HasPrefix(got, "run-shell -b ") {
		t.Errorf("resizeHookCommand(Posix(), %q) = %q; want prefix %q", signalPath, got, "run-shell -b ")
	}
	remainder := strings.TrimPrefix(got, "run-shell -b ")
	if !strings.HasPrefix(remainder, `"`) || !strings.HasSuffix(remainder, `"`) {
		t.Errorf("resizeHookCommand(Posix(), %q) remainder = %q; want double-quoted", signalPath, remainder)
	}
	if strings.Contains(got, "-a") {
		t.Errorf("resizeHookCommand(Posix(), %q) = %q; want no -a flag anywhere", signalPath, got)
	}
}

func TestResizeHookCommand_Pwsh(t *testing.T) {
	const signalPath = "/tmp/wt/.lyx/reed-resize.signal"
	got := resizeHookCommand(shell.Pwsh(), signalPath)
	pwshFragment := shell.Pwsh().Touch(signalPath)
	want := "run-shell -b " + tmuxQuoteValue(pwshFragment)
	if got != want {
		t.Errorf("resizeHookCommand(Pwsh(), %q) = %q; want %q", signalPath, got, want)
	}
	if !strings.Contains(got, pwshFragment) {
		t.Errorf("resizeHookCommand(Pwsh(), %q) = %q; want it to contain the pwsh fragment %q", signalPath, got, pwshFragment)
	}
}

func TestResizeHookCommand_PathWithSpace(t *testing.T) {
	const signalPath = "/tmp/wt space/.lyx/reed-resize.signal"

	posixGot := resizeHookCommand(shell.Posix(), signalPath)
	posixFragment := shell.Posix().Touch(signalPath)
	if !strings.Contains(posixGot, posixFragment) {
		t.Errorf("resizeHookCommand(Posix(), %q) = %q; want it to contain the intact fragment %q", signalPath, posixGot, posixFragment)
	}

	pwshGot := resizeHookCommand(shell.Pwsh(), signalPath)
	pwshFragment := shell.Pwsh().Touch(signalPath)
	if !strings.Contains(pwshGot, pwshFragment) {
		t.Errorf("resizeHookCommand(Pwsh(), %q) = %q; want it to contain the intact fragment %q", signalPath, pwshGot, pwshFragment)
	}
}

func TestTmuxQuoteValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"double_quote", `say "hi"`, `"say \"hi\""`},
		{"backslash", `a\b`, `"a\\b"`},
		{"dollar", `$HOME`, `"\$HOME"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmuxQuoteValue(tt.in)
			if got != tt.want {
				t.Errorf("tmuxQuoteValue(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResizeSignalPath(t *testing.T) {
	e := newTestEngine(t)
	anchor := t.TempDir()
	e.geom.AnchorPath = anchor

	got := e.resizeSignalPath()
	want := filepath.Join(anchor, ".lyx", "reed-resize.signal")
	if got != want {
		t.Errorf("resizeSignalPath() = %q; want %q", got, want)
	}
	if filepath.Dir(got) != e.stateDir() {
		t.Errorf("filepath.Dir(resizeSignalPath()) = %q; want it to equal stateDir() %q", filepath.Dir(got), e.stateDir())
	}
}
