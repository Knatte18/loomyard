// shell_test.go table-tests both pane-shell implementations: argument quoting across plain,
// space-containing, and quote-containing inputs, and the exact Invoke/ReadFile/ WithEnv output each
// impl composes.
// The pwsh quoting cases are migrated verbatim from claudeengine's former TestPwshSingleQuote so
// the coverage moves with the logic it tests.

package shell

import "testing"

// Compile-time assertions that both dialects satisfy the widened Shell interface.
var (
	_ Shell = Posix()
	_ Shell = Pwsh()
)

func TestPwshShell_Quote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "claude", "'claude'"},
		{"space", `C:\a b\c`, `'C:\a b\c'`},
		{"single_quote", "it's", "'it''s'"},
		{"multiple_quotes", "'a'b'", "'''a''b'''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pwsh().Quote(tt.in)
			if got != tt.want {
				t.Errorf("Pwsh().Quote(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPosixShell_Quote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "claude", "'claude'"},
		{"space", "/a b/c", "'/a b/c'"},
		{"single_quote", "it's", `'it'\''s'`},
		{"multiple_quotes", "'a'b'", `''\''a'\''b'\'''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Posix().Quote(tt.in)
			if got != tt.want {
				t.Errorf("Posix().Quote(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPwshShell_InvokeAndReadFile(t *testing.T) {
	sh := Pwsh()
	if got, want := sh.Invoke("claude"), "& 'claude'"; got != want {
		t.Errorf("Pwsh().Invoke(%q) = %q; want %q", "claude", got, want)
	}
	if got, want := sh.ReadFile(`C:\run\prompt.md`), `(Get-Content -Raw 'C:\run\prompt.md')`; got != want {
		t.Errorf("Pwsh().ReadFile(%q) = %q; want %q", `C:\run\prompt.md`, got, want)
	}
}

func TestPosixShell_InvokeAndReadFile(t *testing.T) {
	sh := Posix()
	if got, want := sh.Invoke("claude"), "'claude'"; got != want {
		t.Errorf("Posix().Invoke(%q) = %q; want %q", "claude", got, want)
	}
	if got, want := sh.ReadFile("/run/prompt.md"), `"$(cat '/run/prompt.md')"`; got != want {
		t.Errorf("Posix().ReadFile(%q) = %q; want %q", "/run/prompt.md", got, want)
	}
}

func TestPwshShell_WithEnv(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		cmd   string
		want  string
	}{
		{"plain", "CLAUDE_CODE_FORK_SUBAGENT", "1", "claude", "$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; claude"},
		{"space", "CLAUDE_CODE_FORK_SUBAGENT", "a b", "claude", "$env:CLAUDE_CODE_FORK_SUBAGENT = 'a b'; claude"},
		{"quote", "CLAUDE_CODE_FORK_SUBAGENT", "it's", "claude", "$env:CLAUDE_CODE_FORK_SUBAGENT = 'it''s'; claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pwsh().WithEnv(tt.key, tt.value, tt.cmd)
			if got != tt.want {
				t.Errorf("Pwsh().WithEnv(%q, %q, %q) = %q; want %q", tt.key, tt.value, tt.cmd, got, tt.want)
			}
		})
	}
}

func TestPosixShell_WithEnv(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		cmd   string
		want  string
	}{
		{"plain", "CLAUDE_CODE_FORK_SUBAGENT", "1", "claude", "CLAUDE_CODE_FORK_SUBAGENT='1' claude"},
		{"space", "CLAUDE_CODE_FORK_SUBAGENT", "a b", "claude", "CLAUDE_CODE_FORK_SUBAGENT='a b' claude"},
		{"quote", "CLAUDE_CODE_FORK_SUBAGENT", "it's", "claude", `CLAUDE_CODE_FORK_SUBAGENT='it'\''s' claude`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Posix().WithEnv(tt.key, tt.value, tt.cmd)
			if got != tt.want {
				t.Errorf("Posix().WithEnv(%q, %q, %q) = %q; want %q", tt.key, tt.value, tt.cmd, got, tt.want)
			}
		})
	}
}

func TestPwshShell_Touch(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `C:\a\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a\\reed-resize.signal' | Out-Null"},
		{"space", `C:\a b\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a b\\reed-resize.signal' | Out-Null"},
		{"single_quote", `C:\a'b\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a''b\\reed-resize.signal' | Out-Null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pwsh().Touch(tt.in)
			if got != tt.want {
				t.Errorf("Pwsh().Touch(%q) = %q; want %q", tt.in, got, tt.want)
			}
			wantPrefix := "New-Item -ItemType File -Force -Path "
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("Pwsh().Touch(%q) = %q; want prefix %q", tt.in, got, wantPrefix)
			}
			wantSuffix := " | Out-Null"
			if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
				t.Errorf("Pwsh().Touch(%q) = %q; want suffix %q", tt.in, got, wantSuffix)
			}
		})
	}
}

func TestPosixShell_Touch(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "/a/reed-resize.signal", ": > '/a/reed-resize.signal'"},
		{"space", "/a b/reed-resize.signal", ": > '/a b/reed-resize.signal'"},
		{"single_quote", "/a'b/reed-resize.signal", `: > '/a'\''b/reed-resize.signal'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Posix().Touch(tt.in)
			if got != tt.want {
				t.Errorf("Posix().Touch(%q) = %q; want %q", tt.in, got, tt.want)
			}
			wantPrefix := ": > "
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("Posix().Touch(%q) = %q; want prefix %q", tt.in, got, wantPrefix)
			}
		})
	}
}

func TestForGOOS(t *testing.T) {
	// ForGOOS must always return a usable Shell — assert it behaves like one of the
	// two known impls rather than asserting a specific runtime.GOOS branch, since this
	// test runs on whatever host CI happens to be.
	sh := ForGOOS()
	got := sh.Quote("it's")
	if got != Pwsh().Quote("it's") && got != Posix().Quote("it's") {
		t.Errorf("ForGOOS().Quote(%q) = %q; want either the pwsh or posix form", "it's", got)
	}
}
