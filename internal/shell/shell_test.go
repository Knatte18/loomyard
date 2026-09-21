// shell_test.go table-tests both pane-shell implementations: argument quoting across plain,
// space-containing, and quote-containing inputs, and the exact Invoke/ReadFile/WithEnv/ExportEnv/
// PrependPathEntry/Chain output each impl composes.
// The pwsh quoting cases are migrated verbatim from claudeengine's former TestPwshSingleQuote so
// the coverage moves with the logic it tests.

package shell

import (
	"strings"
	"testing"
)

// Compile-time assertions that both dialects satisfy the Shell interface.
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

func TestPosixShell_ExportEnv(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"plain", "LYX_BIN", "/usr/local/bin/lyx", "export LYX_BIN='/usr/local/bin/lyx'"},
		{"space_and_path_separator", "LYX_BIN", "/usr/local/my bin:more", "export LYX_BIN='/usr/local/my bin:more'"},
		{"quote", "LYX_BIN", "it's", `export LYX_BIN='it'\''s'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Posix().ExportEnv(tt.key, tt.value)
			if got != tt.want {
				t.Errorf("Posix().ExportEnv(%q, %q) = %q; want %q", tt.key, tt.value, got, tt.want)
			}
			wantPrefix := "export " + tt.key + "="
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("Posix().ExportEnv(%q, %q) = %q; want it to stand alone with prefix %q and no trailing command fragment", tt.key, tt.value, got, wantPrefix)
			}
		})
	}
}

func TestPwshShell_ExportEnv(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"plain", "LYX_BIN", `C:\bin\lyx.exe`, `$env:LYX_BIN = 'C:\bin\lyx.exe'`},
		{"space_and_path_separator", "LYX_BIN", `C:\my bin;more`, `$env:LYX_BIN = 'C:\my bin;more'`},
		{"quote", "LYX_BIN", "it's", "$env:LYX_BIN = 'it''s'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pwsh().ExportEnv(tt.key, tt.value)
			if got != tt.want {
				t.Errorf("Pwsh().ExportEnv(%q, %q) = %q; want %q", tt.key, tt.value, got, tt.want)
			}
			wantPrefix := "$env:" + tt.key + " = "
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("Pwsh().ExportEnv(%q, %q) = %q; want it to stand alone with prefix %q and no trailing command fragment", tt.key, tt.value, got, wantPrefix)
			}
		})
	}
}

func TestPosixShell_PrependPathEntry(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"plain", "/usr/local/bin", `export PATH='/usr/local/bin'${PATH:+:$PATH}`},
		{"space", "/usr/local/my bin", `export PATH='/usr/local/my bin'${PATH:+:$PATH}`},
		{"quote", "/usr/local/it's", `export PATH='/usr/local/it'\''s'${PATH:+:$PATH}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Posix().PrependPathEntry(tt.dir)
			if got != tt.want {
				t.Errorf("Posix().PrependPathEntry(%q) = %q; want %q", tt.dir, got, tt.want)
			}
			if !strings.Contains(got, "$PATH") {
				t.Errorf("Posix().PrependPathEntry(%q) = %q; want it to reference the live $PATH variable", tt.dir, got)
			}
			if !strings.Contains(got, "${PATH:+") {
				t.Errorf("Posix().PrependPathEntry(%q) = %q; want it to carry the unset-PATH guard ${PATH:+", tt.dir, got)
			}
		})
	}
}

func TestPwshShell_PrependPathEntry(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"plain", `C:\bin`, `$env:PATH = 'C:\bin' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
		{"space", `C:\my bin`, `$env:PATH = 'C:\my bin' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
		{"quote", `C:\it's`, `$env:PATH = 'C:\it''s' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pwsh().PrependPathEntry(tt.dir)
			if got != tt.want {
				t.Errorf("Pwsh().PrependPathEntry(%q) = %q; want %q", tt.dir, got, tt.want)
			}
			if !strings.Contains(got, "$env:PATH") {
				t.Errorf("Pwsh().PrependPathEntry(%q) = %q; want it to reference the live $env:PATH variable", tt.dir, got)
			}
			if !strings.Contains(got, "if ($env:PATH)") {
				t.Errorf("Pwsh().PrependPathEntry(%q) = %q; want it to carry the unset-PATH guard if ($env:PATH)", tt.dir, got)
			}
		})
	}
}

func TestShellChain(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"zero_parts", []string{}, ""},
		{"one_part", []string{"export FOO='bar'"}, "export FOO='bar'"},
		{"several_parts", []string{"a", "b", "c"}, "a; b; c"},
		{"mixed_empty_and_nonempty", []string{"", "a", "", "b", ""}, "a; b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			posixGot := Posix().Chain(tt.parts...)
			if posixGot != tt.want {
				t.Errorf("Posix().Chain(%q...) = %q; want %q", tt.parts, posixGot, tt.want)
			}
			pwshGot := Pwsh().Chain(tt.parts...)
			if pwshGot != tt.want {
				t.Errorf("Pwsh().Chain(%q...) = %q; want %q", tt.parts, pwshGot, tt.want)
			}
			if strings.Contains(posixGot, "\n") {
				t.Errorf("Posix().Chain(%q...) = %q; want no newline", tt.parts, posixGot)
			}
			if strings.Contains(pwshGot, "\n") {
				t.Errorf("Pwsh().Chain(%q...) = %q; want no newline", tt.parts, pwshGot)
			}
			if posixGot != pwshGot {
				t.Errorf("Posix().Chain(%q...) = %q; Pwsh().Chain(%q...) = %q; want them to agree", tt.parts, posixGot, tt.parts, pwshGot)
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
