// command_test.go table-tests the pane-shell command composition helpers:
// quoting of paths with spaces and embedded single quotes, model/flag
// presence per the interactive toggle, the exact resume-command shape, the
// resolveModelID bare-word-plus-version composition rule, the
// forkSubagents env-prefix wrapping on both launch and resume lines, and a
// no-newline invariant every produced command must hold (they are typed
// into a pane via a single send-keys call). The pwsh-quote cases formerly
// asserted here now live in internal/shell's shell_test.go, since quoting
// itself moved there.

package claudeengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

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
		name          string
		sh            shell.Shell // nil defaults to shell.Pwsh(), the pre-existing coverage's shell
		bin           string
		promptPath    string
		settingsPath  string
		sessionID     string
		model         string
		effort        string
		interactive   bool
		forkSubagents bool
		want          string
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
		{
			// Proves the seam is shell-agnostic: the same builder produces the
			// posix form when handed shell.Posix() instead of the default
			// shell.Pwsh() every other row above exercises.
			name:         "posix_shell",
			sh:           shell.Posix(),
			bin:          "claude",
			promptPath:   "/run/prompt.md",
			settingsPath: "/run/settings.json",
			sessionID:    "abc-123",
			interactive:  false,
			want:         `'claude' "$(cat '/run/prompt.md')" --session-id 'abc-123' --settings '/run/settings.json' --dangerously-skip-permissions`,
		},
		{
			// Fork mode on (pwsh): the fully composed line is wrapped in the
			// pwsh $env: assignment prefix — asserted as the exact composed
			// prefix, not just a substring, so a future WithEnv shape change
			// cannot silently drift undetected here.
			name:          "fork_mode_on_pwsh",
			bin:           "claude",
			promptPath:    `C:\run\prompt.md`,
			settingsPath:  `C:\run\settings.json`,
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: true,
			want:          `$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; & 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --dangerously-skip-permissions`,
		},
		{
			// Fork mode on (posix): the fully composed line is wrapped in the
			// posix command-scoped assignment prefix.
			name:          "fork_mode_on_posix",
			sh:            shell.Posix(),
			bin:           "claude",
			promptPath:    "/run/prompt.md",
			settingsPath:  "/run/settings.json",
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: true,
			want:          `CLAUDE_CODE_FORK_SUBAGENT='1' 'claude' "$(cat '/run/prompt.md')" --session-id 'abc-123' --settings '/run/settings.json' --dangerously-skip-permissions`,
		},
		{
			// Fork mode off: the line is unchanged from today's shape — no
			// env prefix at all.
			name:          "fork_mode_off",
			bin:           "claude",
			promptPath:    `C:\run\prompt.md`,
			settingsPath:  `C:\run\settings.json`,
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: false,
			want:          `& 'claude' (Get-Content -Raw 'C:\run\prompt.md') --session-id 'abc-123' --settings 'C:\run\settings.json' --dangerously-skip-permissions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh := tt.sh
			if sh == nil {
				sh = shell.Pwsh()
			}
			got := buildLaunchCmd(sh, tt.bin, tt.promptPath, tt.settingsPath, tt.sessionID, tt.model, tt.effort, tt.interactive, tt.forkSubagents)
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

// TestResolveModelID covers resolveModelID's full input space: an empty
// version passes the model through unchanged (including an empty model), a
// dotted and a dotless version both compose against a bare model, an empty
// model with a non-empty version errors (nothing to compose against), and a
// dashed (full id) model with a non-empty version errors (the id already
// pins its own version — combining it with version is a contradiction).
func TestResolveModelID(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		version string
		want    string
		wantErr bool
	}{
		{"empty_version_passthrough_with_model", "sonnet", "", "sonnet", false},
		{"empty_version_passthrough_empty_model", "", "", "", false},
		{"dotted_version", "sonnet", "4.5", "claude-sonnet-4-5", false},
		{"dotless_version", "fable", "5", "claude-fable-5", false},
		{"empty_model_with_version_errors", "", "4.5", "", true},
		{"dashed_model_with_version_errors", "claude-sonnet-4-5", "4.5", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveModelID(tt.model, tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveModelID(%q, %q) = nil error; want an error", tt.model, tt.version)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveModelID(%q, %q) error: %v; want nil", tt.model, tt.version, err)
			}
			if got != tt.want {
				t.Errorf("resolveModelID(%q, %q) = %q; want %q", tt.model, tt.version, got, tt.want)
			}
		})
	}
}

func TestBuildResumeCmd(t *testing.T) {
	tests := []struct {
		name          string
		forkSubagents bool
		want          string
	}{
		{"fork_mode_off", false, `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json'`},
		{
			// A resumed fork-mode session must keep the fork-subagent
			// capability it launched with.
			"fork_mode_on", true,
			`$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; & 'claude' --resume 'abc-123' --settings 'C:\run\settings.json'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResumeCmd(shell.Pwsh(), "claude", `C:\run\settings.json`, "abc-123", tt.forkSubagents)
			if got != tt.want {
				t.Errorf("buildResumeCmd(...) = %q; want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("buildResumeCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
			}
		})
	}
}
