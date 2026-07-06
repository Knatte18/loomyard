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
		effort       string
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
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_no_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  true,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json'`,
		},
		{
			name:         "autonomous_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --model 'claude-opus-4' --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  true,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --model 'claude-opus-4'`,
		},
		{
			// A model value with a space or embedded quote must not corrupt
			// the single launch line — --model is single-quoted exactly
			// like every other argument on the line.
			name:         "model_with_space_and_quote",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "my model's name",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --model 'my model''s name' --dangerously-skip-permissions`,
		},
		{
			name:         "paths_with_spaces_and_quotes",
			bin:          `C:\tools\it's claude.exe`,
			promptPath:   `C:\run dir\prompt.md`,
			settingsPath: `C:\run dir\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  false,
			want:         `& 'C:\tools\it''s claude.exe' (Get-Content -Raw 'C:\run dir\prompt.md') --session-id 'abc-123' --settings 'C:\run dir\settings.json' --dangerously-skip-permissions`,
		},
		{
			name:         "no_effort",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_low",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "low",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'low' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_medium",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "medium",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'medium' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_high",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "high",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'high' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_xhigh",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "xhigh",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'xhigh' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_max",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "max",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'max' --dangerously-skip-permissions`,
		},
		{
			// --model and --effort must both appear, in that order, when
			// both are set.
			name:         "effort_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			effort:       "high",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --model 'claude-opus-4' --effort 'high' --dangerously-skip-permissions`,
		},
		{
			// An effort value with a space or embedded quote must not
			// corrupt the single launch line — --effort is single-quoted
			// exactly like every other argument on the line (mirrors the
			// model_with_space_and_quote row above).
			name:         "effort_with_space_and_quote",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "my effort's name",
			interactive:  false,
			want:         `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --effort 'my effort''s name' --dangerously-skip-permissions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLaunchCmd(tt.bin, tt.promptPath, tt.settingsPath, tt.sessionID, tt.model, tt.effort, tt.interactive)
			if got != tt.want {
				t.Errorf("buildLaunchCmd(...) = %q; want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("buildLaunchCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
			}
		})
	}
}

// TestValidateEffort covers validateEffort's full input space: the empty
// string (defers to claude's default), every exact-lowercase valid value,
// and both an unrecognized value and a wrong-case valid value (case
// sensitivity is load-bearing — claude only warns-and-ignores an
// unrecognized value rather than failing, so a silently-accepted "High"
// would defeat the whole hard-error guarantee).
func TestValidateEffort(t *testing.T) {
	tests := []struct {
		name    string
		effort  string
		wantErr bool
	}{
		{"empty", "", false},
		{"low", "low", false},
		{"medium", "medium", false},
		{"high", "high", false},
		{"xhigh", "xhigh", false},
		{"max", "max", false},
		{"bogus", "bogus", true},
		{"wrong_case", "High", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEffort(tt.effort)
			if tt.wantErr && err == nil {
				t.Errorf("validateEffort(%q) = nil; want an error", tt.effort)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateEffort(%q) = %v; want nil", tt.effort, err)
			}
		})
	}
}

func TestBuildResumeCmd(t *testing.T) {
	got := buildResumeCmd("claude", `C:\run\settings.json`, "abc-123")
	want := `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json'`
	if got != want {
		t.Errorf("buildResumeCmd(...) = %q; want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("buildResumeCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
	}
}
