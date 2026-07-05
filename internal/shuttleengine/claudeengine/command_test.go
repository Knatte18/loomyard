// command_test.go table-tests the pwsh command composition helpers:
// quoting of paths with spaces and embedded single quotes, model/flag
// presence per the interactive toggle, the exact resume-command shape, and
// a no-newline invariant every produced command must hold (they are typed
// into a pane via a single send-keys call).

package claudeengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestPwshSingleQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "claude", "'claude'"},
		{"space", "C:\\a b\\c", "'C:\\a b\\c'"},
		{"single_quote", "it's", "'it''s'"},
		{"multiple_quotes", "'a'b'", "'''a''b'''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pwshSingleQuote(tt.in)
			if got != tt.want {
				t.Errorf("pwshSingleQuote(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestClaudeBinary(t *testing.T) {
	tests := []struct {
		name string
		cfg  shuttleengine.Config
		want string
	}{
		{"configured", shuttleengine.Config{Claude: `C:\tools\claude.exe`}, `C:\tools\claude.exe`},
		{"default", shuttleengine.Config{}, "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := claudeBinary(tt.cfg)
			if got != tt.want {
				t.Errorf("claudeBinary(%+v) = %q; want %q", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestBuildLaunchCmd(t *testing.T) {
	tests := []struct {
		name         string
		bin          string
		promptPath   string
		settingsPath string
		sessionID    string
		model        string
		interactive  bool
		want         string
	}{
		{
			name:         "autonomous_no_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id abc-123 --settings 'C:\run\settings.json' --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_no_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  true,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id abc-123 --settings 'C:\run\settings.json'`,
		},
		{
			name:         "autonomous_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id abc-123 --settings 'C:\run\settings.json' --model claude-opus-4 --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  true,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id abc-123 --settings 'C:\run\settings.json' --model claude-opus-4`,
		},
		{
			name:         "paths_with_spaces_and_quotes",
			bin:          `C:\tools\it's claude.exe`,
			promptPath:   `C:\run dir\prompt.md`,
			settingsPath: `C:\run dir\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  false,
			want:         `& 'C:\tools\it''s claude.exe' (Get-Content -Raw 'C:\run dir\prompt.md') --session-id abc-123 --settings 'C:\run dir\settings.json' --dangerously-skip-permissions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLaunchCmd(tt.bin, tt.promptPath, tt.settingsPath, tt.sessionID, tt.model, tt.interactive)
			if got != tt.want {
				t.Errorf("buildLaunchCmd(...) = %q; want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("buildLaunchCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
			}
		})
	}
}

func TestBuildResumeCmd(t *testing.T) {
	got := buildResumeCmd("claude", `C:\run\settings.json`, "abc-123")
	want := `& 'claude' --resume abc-123 --settings 'C:\run\settings.json'`
	if got != want {
		t.Errorf("buildResumeCmd(...) = %q; want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("buildResumeCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
	}
}
