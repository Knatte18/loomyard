// command_test.go table-tests the pane-shell command composition helpers.
// It covers quoting, flag presence, the resume shape, resolveModelID's version composition, and the fork env prefix.
// Every produced command must hold the no-newline invariant, since it is typed via a single send-keys call.

package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// Each launch and resume line starts with the per-dialect env assignments:
// CLAUDE_ENV_FILE, BASH_MAX_TIMEOUT_MS, and the default 5m CLAUDE_CODE_PROMPT_CACHE_TTL.
// The env file paths are the ones the tests below thread through.
const (
	pwshEnvFilePath  = `C:\run\bash-env.sh`
	posixEnvFilePath = "/run/bash-env.sh"
	pwshEnvLead      = `$env:CLAUDE_ENV_FILE = 'C:\run\bash-env.sh'; $env:BASH_MAX_TIMEOUT_MS = '3600000'; $env:CLAUDE_CODE_PROMPT_CACHE_TTL = '5m'; `
	posixEnvLead     = `CLAUDE_ENV_FILE='/run/bash-env.sh' BASH_MAX_TIMEOUT_MS='3600000' CLAUDE_CODE_PROMPT_CACHE_TTL='5m' `
)

func TestClaudeBinary(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		claude Config
		want   string
	}{
		{"configured", Config{Binary: `C:\tools\claude.exe`}, `C:\tools\claude.exe`},
		{"default", Config{}, "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := claudeBinary(tt.claude)
			if got != tt.want {
				t.Errorf("claudeBinary(%+v) = %q; want %q", tt.claude, got, tt.want)
			}
		})
	}
}

//testtiming:keep pins the exact launch line for every flag combination and both shell dialects, which its covering tests only substring-match
func TestBuildLaunchCmd(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name             string
		sh               shell.Shell // nil defaults to shell.Pwsh(), the pre-existing coverage's shell
		bin              string
		promptPath       string
		settingsPath     string
		sessionID        string
		model            string
		effort           string
		appendPromptPath string
		interactive      bool
		forkSubagents    bool
		// promptCacheTTL defaults to 5m when empty.
		promptCacheTTL string
		want           string
	}{
		{
			name:           "prompt_cache_ttl_one_hour",
			bin:            "claude",
			promptPath:     `C:\run\prompt.md`,
			settingsPath:   `C:\run\settings.json`,
			sessionID:      "abc-123",
			promptCacheTTL: "1h",
			want:           `$env:CLAUDE_ENV_FILE = 'C:\run\bash-env.sh'; $env:BASH_MAX_TIMEOUT_MS = '3600000'; $env:CLAUDE_CODE_PROMPT_CACHE_TTL = '1h'; & 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
		{
			name:         "autonomous_no_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_no_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  true,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME"`,
		},
		{
			name:         "autonomous_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --model 'claude-opus-4' --dangerously-skip-permissions`,
		},
		{
			name:         "interactive_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			interactive:  true,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --model 'claude-opus-4'`,
		},
		{
			// A model value with a space or embedded quote must not corrupt the single launch line.
			// --model is single-quoted, like every other argument on the line.
			name:         "model_with_space_and_quote",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "my model's name",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --model 'my model''s name' --dangerously-skip-permissions`,
		},
		{
			name:         "paths_with_spaces_and_quotes",
			bin:          `C:\tools\it's claude.exe`,
			promptPath:   `C:\run dir\prompt.md`,
			settingsPath: `C:\run dir\settings.json`,
			sessionID:    "abc-123",
			model:        "",
			interactive:  false,
			want:         pwshEnvLead + `& 'C:\tools\it''s claude.exe' 'Read C:\run dir\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run dir\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
		{
			name:         "no_effort",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
		{
			name:         "effort_low",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "low",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'low' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_medium",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "medium",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'medium' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_high",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "high",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'high' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_xhigh",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "xhigh",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'xhigh' --dangerously-skip-permissions`,
		},
		{
			name:         "effort_max",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "max",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'max' --dangerously-skip-permissions`,
		},
		{
			// --model and --effort must both appear, in that order, when both are set.
			name:         "effort_with_model",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			model:        "claude-opus-4",
			effort:       "high",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --model 'claude-opus-4' --effort 'high' --dangerously-skip-permissions`,
		},
		{
			// An effort value with a space or embedded quote must not corrupt the single launch line.
			// --effort is single-quoted, like every other argument on the line.
			name:         "effort_with_space_and_quote",
			bin:          "claude",
			promptPath:   `C:\run\prompt.md`,
			settingsPath: `C:\run\settings.json`,
			sessionID:    "abc-123",
			effort:       "my effort's name",
			interactive:  false,
			want:         pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --effort 'my effort''s name' --dangerously-skip-permissions`,
		},
		{
			name:             "notice_pwsh",
			bin:              "claude",
			promptPath:       `C:\run\prompt.md`,
			settingsPath:     `C:\run\settings.json`,
			sessionID:        "abc-123",
			appendPromptPath: `C:\run\append-system-prompt.txt`,
			interactive:      false,
			want:             pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions --append-system-prompt-file 'C:\run\append-system-prompt.txt'`,
		},
		{
			name:             "notice_posix",
			sh:               shell.Posix(),
			bin:              "claude",
			promptPath:       "/run/prompt.md",
			settingsPath:     "/run/settings.json",
			sessionID:        "abc-123",
			appendPromptPath: "/run/append-system-prompt.txt",
			interactive:      true,
			want:             posixEnvLead + `'claude' 'Read /run/prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings '/run/settings.json' --name "${LYM_STRAND_NAME}" --append-system-prompt-file '/run/append-system-prompt.txt'`,
		},
		{
			// Proves the seam is shell-agnostic: the same builder produces the posix form when handed shell.Posix().
			// Every other row uses the default shell.Pwsh().
			name:         "posix_shell",
			sh:           shell.Posix(),
			bin:          "claude",
			promptPath:   "/run/prompt.md",
			settingsPath: "/run/settings.json",
			sessionID:    "abc-123",
			interactive:  false,
			want:         posixEnvLead + `'claude' 'Read /run/prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings '/run/settings.json' --name "${LYM_STRAND_NAME}" --dangerously-skip-permissions`,
		},
		{
			// Fork mode on (pwsh): the fully composed line is wrapped in the pwsh $env: assignment prefix.
			// The row asserts the whole prefix, not a substring, so a change to what WithEnv composes fails this row.
			name:          "fork_mode_on_pwsh",
			bin:           "claude",
			promptPath:    `C:\run\prompt.md`,
			settingsPath:  `C:\run\settings.json`,
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: true,
			want:          pwshEnvLead + `$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; & 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
		{
			// Fork mode on (posix): the fully composed line is wrapped in the posix command-scoped assignment prefix.
			name:          "fork_mode_on_posix",
			sh:            shell.Posix(),
			bin:           "claude",
			promptPath:    "/run/prompt.md",
			settingsPath:  "/run/settings.json",
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: true,
			want:          posixEnvLead + `CLAUDE_CODE_FORK_SUBAGENT='1' 'claude' 'Read /run/prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings '/run/settings.json' --name "${LYM_STRAND_NAME}" --dangerously-skip-permissions`,
		},
		{
			// The prompt file flag rides inside the fork-mode env wrap rather than after it.
			name:             "fork_mode_prompt_file_pwsh",
			bin:              "claude",
			promptPath:       `C:\run\prompt.md`,
			settingsPath:     `C:\run\settings.json`,
			sessionID:        "abc-123",
			appendPromptPath: `C:\run\append-system-prompt.txt`,
			interactive:      false,
			forkSubagents:    true,
			want:             pwshEnvLead + `$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; & 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions --append-system-prompt-file 'C:\run\append-system-prompt.txt'`,
		},
		{
			name:             "fork_mode_prompt_file_posix",
			sh:               shell.Posix(),
			bin:              "claude",
			promptPath:       "/run/prompt.md",
			settingsPath:     "/run/settings.json",
			sessionID:        "abc-123",
			appendPromptPath: "/run/append-system-prompt.txt",
			interactive:      false,
			forkSubagents:    true,
			want:             posixEnvLead + `CLAUDE_CODE_FORK_SUBAGENT='1' 'claude' 'Read /run/prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings '/run/settings.json' --name "${LYM_STRAND_NAME}" --dangerously-skip-permissions --append-system-prompt-file '/run/append-system-prompt.txt'`,
		},
		{
			// Fork mode off: the line carries no fork-subagent env assignment.
			name:          "fork_mode_off",
			bin:           "claude",
			promptPath:    `C:\run\prompt.md`,
			settingsPath:  `C:\run\settings.json`,
			sessionID:     "abc-123",
			interactive:   false,
			forkSubagents: false,
			want:          pwshEnvLead + `& 'claude' 'Read C:\run\prompt.md in full first; it is your complete, authoritative instructions.' --session-id 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh, envFilePath := tt.sh, pwshEnvFilePath
			if sh == nil {
				sh = shell.Pwsh()
			} else {
				envFilePath = posixEnvFilePath
			}
			promptCacheTTL := tt.promptCacheTTL
			if promptCacheTTL == "" {
				promptCacheTTL = "5m"
			}
			got := buildLaunchCmd(sh, tt.bin, launchPointer(tt.promptPath), tt.settingsPath, tt.sessionID, tt.model, tt.effort, tt.appendPromptPath, envFilePath, promptCacheTTL, false, !tt.interactive, tt.forkSubagents)
			if got != tt.want {
				t.Errorf("buildLaunchCmd(...) = %q; want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("buildLaunchCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
			}
		})
	}
}

// openLines returns an open launch's fresh and resume lines with their leading export of the prompt-cache TTL stripped.
// It fails the test unless both lines open with that export at ttl.
func openLines(t *testing.T, sh shell.Shell, launch shuttleengine.Launch, ttl string) (fresh, resume string) {
	t.Helper()
	const marker = "|"
	lead := strings.TrimSuffix(sh.Chain(sh.ExportEnv(promptCacheTTLEnvKey, ttl), marker), marker)
	fresh, freshFound := strings.CutPrefix(launch.Cmd, lead)
	resume, resumeFound := strings.CutPrefix(launch.ResumeCmd, lead)
	if !freshFound || !resumeFound {
		t.Fatalf("launch = %+v; want both lines to open with %q", launch, lead)
	}
	return fresh, resume
}

// TestOpenLaunch pins the open launch: the fresh line with the name reference and optional model flags;
// the resume line that falls back to the fresh one; the export of the role's prompt-cache TTL ahead of both;
// and the refusals made before a session id is minted.
func TestOpenLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	sh := shell.ForGOOS()
	nameRef := "--name " + sh.EnvRef("LYM_STRAND_NAME")
	tests := []struct {
		name string
		spec shuttleengine.OpenSpec
		// defaultTTL and ttlRoles replace the template's prompt-cache TTL fields when set.
		defaultTTL string
		ttlRoles   map[string]string
		// wantTTL is the TTL both lines export; empty means the template's default.
		wantTTL string
		// invalidConfig makes the engine load a claude.yaml the loader refuses.
		invalidConfig bool
		// board gives the engine a hub board directory, so the launch writes creel's files into the open directory.
		board bool
		// obstructAppendPrompt makes a directory at the append-prompt file's path,
		// so the write of that file fails after the directory and settings.json exist.
		obstructAppendPrompt bool
		wantErr              string
		// withAgents hands the launch the agents.yaml template, which gives a spec with no effort one.
		withAgents bool
		// check gets the launch and its two lines with the TTL export stripped.
		check func(t *testing.T, launch shuttleengine.Launch, fresh, resume string)
	}{
		{
			name:       "the role's mapped TTL is exported ahead of both lines",
			spec:       shuttleengine.OpenSpec{Role: "orch"},
			defaultTTL: "1h",
			ttlRoles:   map[string]string{"orch": "5m"},
			wantTTL:    "5m",
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				resumeLine := strings.Replace(fresh, "--session-id", "--resume", 1)
				if want := sh.OrElse(resumeLine, fresh); resume != want {
					t.Errorf("resume line = %q; want %q, both under the one export", resume, want)
				}
			},
		},
		{name: "an unmapped role exports the default TTL", spec: shuttleengine.OpenSpec{Role: "explore"}, defaultTTL: "1h", wantTTL: "1h"},
		{name: "an invalid TTL is refused", spec: shuttleengine.OpenSpec{Role: "explore"}, defaultTTL: "1H", wantErr: `prompt_cache_ttl "1H"`},
		{name: "a failed append-prompt write removes the directory", spec: shuttleengine.OpenSpec{Role: "explore"}, board: true, obstructAppendPrompt: true, wantErr: "write open append system prompt"},
		{name: "a board writes the creel settings and the append prompt", spec: shuttleengine.OpenSpec{Role: "explore"}, board: true},
		{name: "a board refusal creates nothing", spec: shuttleengine.OpenSpec{Role: "explore", PermissionMode: "auto"}, board: true, wantErr: "invalid permission mode"},
		{
			name: "fresh line",
			spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Model: "opus", Effort: "high"}},
			check: func(t *testing.T, launch shuttleengine.Launch, fresh, resume string) {
				for _, want := range []string{"--session-id " + sh.Quote(launch.SessionID), nameRef, "--model " + sh.Quote("opus"), "--effort " + sh.Quote("high")} {
					if !strings.Contains(fresh, want) {
						t.Errorf("fresh line = %q; want it to hold %q", fresh, want)
					}
				}
				if !strings.HasPrefix(fresh, sh.Invoke("claude")) {
					t.Errorf("fresh line = %q; want it to start with the invoked binary", fresh)
				}
				for _, banned := range []string{"--settings", "Read "} {
					if strings.Contains(fresh, banned) {
						t.Errorf("fresh line = %q; want no %q", fresh, banned)
					}
				}
				if got := strings.Count(resume, "--dangerously-skip-permissions"); got != 2 {
					t.Errorf("resume line = %q holds --dangerously-skip-permissions %d times; want 2, the empty mode's bypass default", resume, got)
				}
			},
		},
		{
			name: "bypass skips permissions on every line",
			spec: shuttleengine.OpenSpec{Role: "explore", PermissionMode: "bypass"},
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				const flag = "--dangerously-skip-permissions"
				if !strings.Contains(fresh, flag) {
					t.Errorf("fresh line = %q; want it to hold %q", fresh, flag)
				}
				resumeLine := strings.Replace(fresh, "--session-id", "--resume", 1)
				if want := sh.OrElse(resumeLine, fresh); resume != want {
					t.Errorf("resume line = %q; want %q", resume, want)
				}
				if got := strings.Count(resume, flag); got != 2 {
					t.Errorf("resume line = %q holds %q %d times; want 2", resume, flag, got)
				}
			},
		},
		{
			name: "prompt adds no permission flag",
			spec: shuttleengine.OpenSpec{Role: "explore", PermissionMode: "prompt"},
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				for _, line := range []string{fresh, resume} {
					if strings.Contains(line, "--dangerously-skip-permissions") {
						t.Errorf("line = %q; want no --dangerously-skip-permissions", line)
					}
				}
				resumeLine := strings.Replace(fresh, "--session-id", "--resume", 1)
				if want := sh.OrElse(resumeLine, fresh); resume != want {
					t.Errorf("resume line = %q; want %q", resume, want)
				}
			},
		},
		{
			name: "resume falls back to a fresh start",
			spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Model: "opus"}},
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				resumeLine := strings.Replace(fresh, "--session-id", "--resume", 1)
				if want := sh.OrElse(resumeLine, fresh); resume != want {
					t.Errorf("resume line = %q; want %q", resume, want)
				}
			},
		},
		{
			name: "no model or effort adds neither flag",
			spec: shuttleengine.OpenSpec{Role: "explore"},
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, _ string) {
				if strings.Contains(fresh, "--model") || strings.Contains(fresh, "--effort") {
					t.Errorf("fresh line = %q; want neither --model nor --effort", fresh)
				}
			},
		},
		{
			name:       "a spec with no effort takes the agents.yaml default",
			spec:       shuttleengine.OpenSpec{Role: "explore"},
			withAgents: true,
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				for _, line := range []string{fresh, resume} {
					if want := "--effort " + sh.Quote("medium"); !strings.Contains(line, want) {
						t.Errorf("line = %q; want it to hold %q", line, want)
					}
				}
			},
		},
		{
			name:       "a full model id takes the effort of its alias",
			spec:       shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Model: "claude-fable-5-1"}},
			withAgents: true,
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				for _, line := range []string{fresh, resume} {
					if want := "--effort " + sh.Quote("high"); !strings.Contains(line, want) {
						t.Errorf("line = %q; want it to hold %q", line, want)
					}
				}
			},
		},
		{
			name: "skills ride the fresh line as the first turn and no resume line",
			spec: shuttleengine.OpenSpec{Role: "board", Skills: []string{"ly:board-policy", "ly:board"}},
			check: func(t *testing.T, _ shuttleengine.Launch, fresh, resume string) {
				message := " " + sh.Quote(new(Claude).SkillLoadMessage([]string{"ly:board-policy", "ly:board"}))
				if want := sh.Invoke("claude") + message + " --session-id "; !strings.HasPrefix(fresh, want) {
					t.Errorf("fresh line = %q; want it to start with %q", fresh, want)
				}
				resumeLine := strings.Replace(strings.Replace(fresh, message, "", 1), "--session-id", "--resume", 1)
				if want := sh.OrElse(resumeLine, fresh); resume != want {
					t.Errorf("resume line = %q; want %q", resume, want)
				}
			},
		},
		{name: "a skill-load message past the launch limit is refused", spec: shuttleengine.OpenSpec{Role: "board", Skills: []string{strings.Repeat("x", maxLaunchPromptBytes)}}, board: true, wantErr: "skill-load message is"},
		{name: "an invalid effort is refused", spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Effort: "turbo"}}, wantErr: "invalid effort"},
		{name: "a version without a model is refused", spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Version: "4.5"}}, wantErr: "no model"},
		{name: "an unknown mode is refused", spec: shuttleengine.OpenSpec{Role: "explore", PermissionMode: "auto"}, wantErr: "invalid permission mode"},
		{name: "an invalid claude config is refused", spec: shuttleengine.OpenSpec{Role: "explore"}, invalidConfig: true, wantErr: "claude.local.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := engineWith(t, ttlConfig(t, tt.defaultTTL, tt.ttlRoles))
			if tt.invalidConfig {
				engine = engineWithInvalidConfig(t)
			}
			boardDir := ""
			if tt.board {
				boardDir = t.TempDir()
				engine.dirs.Board = boardDir
			}
			dir := filepath.Join(t.TempDir(), "open")
			if tt.obstructAppendPrompt {
				if err := os.MkdirAll(filepath.Join(dir, appendPromptFileName), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
			}
			launchConfig := shuttleengine.Config{}
			if tt.withAgents {
				launchConfig = templateAgentsConfig(t)
			}
			launch, err := engine.OpenLaunch(dir, tt.spec, launchConfig)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("OpenLaunch() error = %v; want it to contain %q", err, tt.wantErr)
				}
				if tt.invalidConfig {
					for _, want := range invalidConfigWant {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("OpenLaunch() error = %q; want it to name %q", err, want)
						}
					}
				}
				if launch.SessionID != "" {
					t.Errorf("SessionID = %q; want none from a refused open", launch.SessionID)
				}
				if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
					t.Errorf("Stat(%s) error = %v; want the directory uncreated by a refused open", dir, statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenLaunch() error = %v", err)
			}
			if strings.ContainsAny(launch.Cmd+launch.ResumeCmd, "\r\n") {
				t.Errorf("launch = %+v; want single-line commands", launch)
			}
			wantTTL := tt.wantTTL
			if wantTTL == "" {
				wantTTL = templateConfig(t).PromptCacheTTL
			}
			fresh, resume := openLines(t, sh, launch, wantTTL)
			if tt.board {
				checkOpenCreelFiles(t, launch, dir, boardDir, sh)
				return
			}
			if launch.CreelHooks {
				t.Errorf("Launch.CreelHooks = true; want false for an engine told no board directory")
			}
			if strings.Contains(launch.Cmd+launch.ResumeCmd, "--append-system-prompt-file") {
				t.Errorf("launch = %+v; want no --append-system-prompt-file on an open session without a board", launch)
			}
			if strings.Contains(launch.Cmd+launch.ResumeCmd, "--settings") {
				t.Errorf("launch = %+v; want no --settings on an open session without a board", launch)
			}
			if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
				t.Errorf("Stat(%s) error = %v; want the directory uncreated without a board", dir, statErr)
			}
			if tt.check != nil {
				tt.check(t, launch, fresh, resume)
			}
		})
	}
}

// checkOpenCreelFiles asserts what an open launch with a board directory leaves:
// CreelHooks is set, and both launch lines name the settings and append-prompt files in dir.
// The settings document holds creel's hooks and the session scope alone,
// and the append prompt holds the trust sentence alone.
func checkOpenCreelFiles(t *testing.T, launch shuttleengine.Launch, dir, boardDir string, sh shell.Shell) {
	t.Helper()
	if !launch.CreelHooks {
		t.Errorf("Launch.CreelHooks = false; want true for an engine told a board directory")
	}
	settingsPath := filepath.Join(dir, "settings.json")
	appendPath := filepath.Join(dir, "append-system-prompt.txt")
	for _, line := range []string{launch.Cmd, launch.ResumeCmd} {
		for _, want := range []string{"--settings " + sh.Quote(settingsPath), "--append-system-prompt-file " + sh.Quote(appendPath)} {
			if !strings.Contains(line, want) {
				t.Errorf("line = %q; want it to hold %q", line, want)
			}
		}
	}
	flagDir := shuttleengine.CreelFlagDir(boardDir)
	if runtime.GOOS == "windows" {
		posixFlagDir, err := shuttleengine.PosixPath(flagDir)
		if err != nil {
			t.Fatalf("PosixPath(%q) error: %v", flagDir, err)
		}
		flagDir = posixFlagDir
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command       string `json:"command"`
				AsyncRewake   bool   `json:"asyncRewake"`
				Timeout       int    `json:"timeout"`
				RewakeMessage string `json:"rewakeMessage"`
				RewakeSummary string `json:"rewakeSummary"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse settings.json %q: %v", data, err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("parse settings.json %q: %v", data, err)
	}
	for key := range keys {
		if key != "hooks" && !slices.Contains(scopeSettingsKeys, key) {
			t.Errorf("settings.json holds %q; want the hooks and the session scope alone (data: %s)", key, data)
		}
	}
	if len(doc.Hooks) != 2 {
		t.Fatalf("settings.json = %s; want a hooks object holding only Stop and PostToolUse", data)
	}
	stop, post := doc.Hooks["Stop"], doc.Hooks["PostToolUse"]
	if len(stop) != 2 || len(stop[0].Hooks) != 1 || len(stop[1].Hooks) != 1 || len(post) != 1 || len(post[0].Hooks) != 1 {
		t.Fatalf("settings.json = %s; want two Stop entries and one PostToolUse entry of one hook each", data)
	}
	quotedFlagDir := shQuote(flagDir)
	if got, want := stop[0].Hooks[0].Command, creelOpenStopHookCommand(quotedFlagDir); got != want {
		t.Errorf("Stop delivery command = %q; want %q", got, want)
	}
	waiter := stop[1].Hooks[0]
	if waiter.Command != creelWaitHookCommand() || !waiter.AsyncRewake || waiter.Timeout != creelWaitTimeout || waiter.RewakeMessage != creelRewakeMessage || waiter.RewakeSummary != creelRewakeSummary {
		t.Errorf("Stop waiter = %+v; want the idle waiter with asyncRewake", waiter)
	}
	if got, want := post[0].Hooks[0].Command, creelMailHookCommand(quotedFlagDir); got != want {
		t.Errorf("PostToolUse command = %q; want %q", got, want)
	}
	appendBytes, err := os.ReadFile(appendPath)
	if err != nil {
		t.Fatalf("read append-system-prompt.txt: %v", err)
	}
	if got, want := string(appendBytes), appendPromptText("", nil); got != want {
		t.Errorf("append-system-prompt.txt = %q; want %q", got, want)
	}
}

// TestValidateEffort covers validateEffort's full input space:
// the empty string, which defers to claude's default;
// every exact-lowercase valid value;
// and both an unrecognized value and a wrong-case valid value.
// Case sensitivity is load-bearing: claude only warns-and-ignores an unrecognized value rather than failing,
// so a silently accepted "High" would defeat the hard-error guarantee.
//
//testtiming:keep pins the accepted effort values and the wrong-case rejection, which its covering tests do not assert row by row
func TestValidateEffort(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestResolveModelID covers resolveModelID's input space:
// an empty version passes the model through unchanged, including an empty model;
// a dotted and a dotless version both compose against a bare model;
// an empty model with a non-empty version errors, since there is nothing to compose against;
// and a dashed (full id) model with a non-empty version errors, since the id already pins its own version.
func TestResolveModelID(t *testing.T) {
	tiergate.Require(t, 0)
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

//testtiming:keep pins the exact resume line for every mode-bearing flag, which its covering tests only substring-match
func TestBuildResumeCmd(t *testing.T) {
	tiergate.Require(t, 0)
	// The resumed process is a fresh claude that inherits nothing from the launch, and reed replays this string verbatim.
	// So every mode-bearing flag that buildLaunchCmd emits must appear here too.
	// Without them, a resumed autonomous run comes back permission-gated:
	// it stalls at its first tool dialog with no operator present, which shuttle can only classify as a timeout,
	// and it comes back on the provider default model rather than the pinned one.
	tests := []struct {
		name             string
		model            string
		effort           string
		appendPromptPath string
		interactive      bool
		forkSubagents    bool
		ttl              string
		want             string
	}{
		{"autonomous_bare", "", "", "", false, false, "5m", pwshEnvLead + `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions`},
		{"interactive_bare", "", "", "", true, false, "5m", pwshEnvLead + `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME"`},
		{"model_and_effort_pinned", "haiku", "low", "", false, false, "5m", pwshEnvLead + `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --model 'haiku' --effort 'low' --dangerously-skip-permissions`},
		{"prompt_file_on_resume", "", "", `C:\run\append-system-prompt.txt`, false, false, "5m", pwshEnvLead + `& 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME" --dangerously-skip-permissions --append-system-prompt-file 'C:\run\append-system-prompt.txt'`},
		{
			// A resumed fork-mode session must keep the fork-subagent capability it launched with.
			"fork_mode_on", "", "", "", true, true, "5m",
			pwshEnvLead + `$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; & 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME"`,
		},
		{"prompt_cache_ttl_one_hour", "", "", "", true, false, "1h", `$env:CLAUDE_ENV_FILE = 'C:\run\bash-env.sh'; $env:BASH_MAX_TIMEOUT_MS = '3600000'; $env:CLAUDE_CODE_PROMPT_CACHE_TTL = '1h'; & 'claude' --resume 'abc-123' --settings 'C:\run\settings.json' --name "$env:LYM_STRAND_NAME"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResumeCmd(shell.Pwsh(), "claude", `C:\run\settings.json`, "abc-123", tt.model, tt.effort, tt.appendPromptPath, pwshEnvFilePath, tt.ttl, !tt.interactive, tt.forkSubagents)
			if got != tt.want {
				t.Errorf("buildResumeCmd(...) = %q; want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("buildResumeCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
			}
		})
	}
}

// TestBuildResumeCmd_NoticePosix pins the posix resume line:
// it carries the append-system-prompt file flag inside the fork-mode env wrap.
//
//testtiming:keep pins the posix resume line with the prompt file flag inside the fork env wrap, which its covering test only substring-matches
func TestBuildResumeCmd_NoticePosix(t *testing.T) {
	tiergate.Require(t, 0)
	want := posixEnvLead + `CLAUDE_CODE_FORK_SUBAGENT='1' 'claude' --resume 'abc-123' --settings '/run/settings.json' --name "${LYM_STRAND_NAME}" --dangerously-skip-permissions --append-system-prompt-file '/run/append-system-prompt.txt'`

	got := buildResumeCmd(shell.Posix(), "claude", "/run/settings.json", "abc-123", "", "", "/run/append-system-prompt.txt", posixEnvFilePath, "5m", true, true)
	if got != want {
		t.Errorf("buildResumeCmd(...) = %q; want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("buildResumeCmd(...) = %q; contains a newline, but the command is typed via a single send-keys call", got)
	}
}

// TestValidatePermissionMode pins the mode resolution table.
//
//testtiming:keep pins every mode and interactivity row, including the wrong-case rejection, which its covering tests do not assert
func TestValidatePermissionMode(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		mode        string
		interactive bool
		wantSkip    bool
		wantErr     bool
	}{
		{"", false, true, false},
		{"", true, true, false},
		{"bypass", true, true, false},
		{"bypass", false, true, false},
		{"prompt", true, false, false},
		{"prompt", false, false, true},
		{"Bypass", true, false, true},
	}
	for _, tt := range tests {
		got, err := validatePermissionMode(tt.mode, tt.interactive)
		if (err != nil) != tt.wantErr || got != tt.wantSkip {
			t.Errorf("validatePermissionMode(%q, %v) = (%v, %v); want skip=%v err=%v", tt.mode, tt.interactive, got, err, tt.wantSkip, tt.wantErr)
		}
	}
}
