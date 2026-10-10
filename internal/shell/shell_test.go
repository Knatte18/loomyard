// shell_test.go table-tests both pane-shell implementations: argument quoting across plain, space-containing, and quote-containing inputs, and the exact Invoke/WithEnv/ExportEnv/ PrependPathEntry/Chain/Source/EnvRef/Touch/ScriptExt output each impl composes.
// One test per Shell method; each table holds both dialects' rows, named "<dialect>/<case>".
// The pwsh quoting cases are migrated verbatim from claudeengine's former TestPwshSingleQuote so the coverage moves with the logic it tests.

package shell

import (
	"testing"
)

// Compile-time assertions that both dialects satisfy the Shell interface.
var (
	_ Shell = Posix()
	_ Shell = Pwsh()
)

func TestShell_Quote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		in   string
		want string
	}{
		{"pwsh/plain", Pwsh(), "claude", "'claude'"},
		{"pwsh/space", Pwsh(), `C:\a b\c`, `'C:\a b\c'`},
		{"pwsh/single_quote", Pwsh(), "it's", "'it''s'"},
		{"pwsh/multiple_quotes", Pwsh(), "'a'b'", "'''a''b'''"},
		{"posix/plain", Posix(), "claude", "'claude'"},
		{"posix/space", Posix(), "/a b/c", "'/a b/c'"},
		{"posix/single_quote", Posix(), "it's", `'it'\''s'`},
		{"posix/multiple_quotes", Posix(), "'a'b'", `''\''a'\''b'\'''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.Quote(tt.in); got != tt.want {
				t.Errorf("%s Quote(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestShell_Invoke(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		want string
	}{
		{"pwsh", Pwsh(), "& 'claude'"},
		{"posix", Posix(), "'claude'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.Invoke("claude"); got != tt.want {
				t.Errorf("%s Invoke(%q) = %q; want %q", tt.name, "claude", got, tt.want)
			}
		})
	}
}

func TestShell_WithEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		sh    Shell
		value string
		want  string
	}{
		{"pwsh/plain", Pwsh(), "1", "$env:CLAUDE_CODE_FORK_SUBAGENT = '1'; claude"},
		{"pwsh/space", Pwsh(), "a b", "$env:CLAUDE_CODE_FORK_SUBAGENT = 'a b'; claude"},
		{"pwsh/quote", Pwsh(), "it's", "$env:CLAUDE_CODE_FORK_SUBAGENT = 'it''s'; claude"},
		{"posix/plain", Posix(), "1", "CLAUDE_CODE_FORK_SUBAGENT='1' claude"},
		{"posix/space", Posix(), "a b", "CLAUDE_CODE_FORK_SUBAGENT='a b' claude"},
		{"posix/quote", Posix(), "it's", `CLAUDE_CODE_FORK_SUBAGENT='it'\''s' claude`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.WithEnv("CLAUDE_CODE_FORK_SUBAGENT", tt.value, "claude"); got != tt.want {
				t.Errorf("%s WithEnv(value %q) = %q; want %q", tt.name, tt.value, got, tt.want)
			}
		})
	}
}

func TestShell_Touch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		in   string
		want string
	}{
		{"pwsh/plain", Pwsh(), `C:\a\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a\\reed-resize.signal' | Out-Null"},
		{"pwsh/space", Pwsh(), `C:\a b\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a b\\reed-resize.signal' | Out-Null"},
		{"pwsh/single_quote", Pwsh(), `C:\a'b\reed-resize.signal`, "New-Item -ItemType File -Force -Path 'C:\\a''b\\reed-resize.signal' | Out-Null"},
		{"posix/plain", Posix(), "/a/reed-resize.signal", ": > '/a/reed-resize.signal'"},
		{"posix/space", Posix(), "/a b/reed-resize.signal", ": > '/a b/reed-resize.signal'"},
		{"posix/single_quote", Posix(), "/a'b/reed-resize.signal", `: > '/a'\''b/reed-resize.signal'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.Touch(tt.in); got != tt.want {
				t.Errorf("%s Touch(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestShell_ChangeDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		in   string
		want string
	}{
		{"pwsh/plain", Pwsh(), `C:\a\prime`, `Set-Location -LiteralPath 'C:\a\prime'`},
		{"pwsh/space", Pwsh(), `C:\a b\prime`, `Set-Location -LiteralPath 'C:\a b\prime'`},
		{"pwsh/single_quote", Pwsh(), `C:\a'b\prime`, `Set-Location -LiteralPath 'C:\a''b\prime'`},
		{"posix/plain", Posix(), "/a/prime", "cd '/a/prime'"},
		{"posix/space", Posix(), "/a b/prime", "cd '/a b/prime'"},
		{"posix/single_quote", Posix(), "/a'b/prime", `cd '/a'\''b/prime'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.ChangeDir(tt.in); got != tt.want {
				t.Errorf("%s ChangeDir(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

// TestShell_ExportEnv pins that each dialect's export stands alone: a bare assignment with no trailing command fragment.
func TestShell_ExportEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		sh    Shell
		value string
		want  string
	}{
		{"pwsh/plain", Pwsh(), `C:\bin\lyx.exe`, `$env:LYX_BIN = 'C:\bin\lyx.exe'`},
		{"pwsh/space_and_path_separator", Pwsh(), `C:\my bin;more`, `$env:LYX_BIN = 'C:\my bin;more'`},
		{"pwsh/quote", Pwsh(), "it's", "$env:LYX_BIN = 'it''s'"},
		{"posix/plain", Posix(), "/usr/local/bin/lyx", "export LYX_BIN='/usr/local/bin/lyx'"},
		{"posix/space_and_path_separator", Posix(), "/usr/local/my bin:more", "export LYX_BIN='/usr/local/my bin:more'"},
		{"posix/quote", Posix(), "it's", `export LYX_BIN='it'\''s'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.ExportEnv("LYX_BIN", tt.value); got != tt.want {
				t.Errorf("%s ExportEnv(%q) = %q; want %q", tt.name, tt.value, got, tt.want)
			}
		})
	}
}

// TestShell_PrependPathEntry pins that each dialect references the live PATH and carries the unset-PATH guard.
func TestShell_PrependPathEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		dir  string
		want string
	}{
		{"pwsh/plain", Pwsh(), `C:\bin`, `$env:PATH = 'C:\bin' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
		{"pwsh/space", Pwsh(), `C:\my bin`, `$env:PATH = 'C:\my bin' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
		{"pwsh/quote", Pwsh(), `C:\it's`, `$env:PATH = 'C:\it''s' + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`},
		{"posix/plain", Posix(), "/usr/local/bin", `export PATH='/usr/local/bin'${PATH:+:$PATH}`},
		{"posix/space", Posix(), "/usr/local/my bin", `export PATH='/usr/local/my bin'${PATH:+:$PATH}`},
		{"posix/quote", Posix(), "/usr/local/it's", `export PATH='/usr/local/it'\''s'${PATH:+:$PATH}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.PrependPathEntry(tt.dir); got != tt.want {
				t.Errorf("%s PrependPathEntry(%q) = %q; want %q", tt.name, tt.dir, got, tt.want)
			}
		})
	}
}

// TestShell_Chain pins that both dialects join the non-empty parts with "; " on one line, so they agree on every input.
func TestShell_Chain(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			if got := Posix().Chain(tt.parts...); got != tt.want {
				t.Errorf("Posix().Chain(%q...) = %q; want %q", tt.parts, got, tt.want)
			}
			if got := Pwsh().Chain(tt.parts...); got != tt.want {
				t.Errorf("Pwsh().Chain(%q...) = %q; want %q", tt.parts, got, tt.want)
			}
		})
	}
}

func TestShell_Source(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		in   string
		want string
	}{
		{"pwsh/plain", Pwsh(), `C:\a\launch.ps1`, `. ([scriptblock]::Create((Get-Content -Raw 'C:\a\launch.ps1')))`},
		{"pwsh/space", Pwsh(), `C:\a b\launch.ps1`, `. ([scriptblock]::Create((Get-Content -Raw 'C:\a b\launch.ps1')))`},
		{"pwsh/single_quote", Pwsh(), `C:\a\it's.ps1`, `. ([scriptblock]::Create((Get-Content -Raw 'C:\a\it''s.ps1')))`},
		{"posix/plain", Posix(), "/a/launch.sh", ". '/a/launch.sh'"},
		{"posix/space", Posix(), "/a b/launch.sh", ". '/a b/launch.sh'"},
		{"posix/single_quote", Posix(), "/a/it's.sh", `. '/a/it'\''s.sh'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.Source(tt.in); got != tt.want {
				t.Errorf("%s Source(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestShell_EnvRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		in   string
		want string
	}{
		{"pwsh/strand_name", Pwsh(), "LYX_STRAND_NAME", `"$env:LYX_STRAND_NAME"`},
		{"pwsh/parent", Pwsh(), "LYX_PARENT", `"$env:LYX_PARENT"`},
		{"posix/strand_name", Posix(), "LYX_STRAND_NAME", `"${LYX_STRAND_NAME}"`},
		{"posix/parent", Posix(), "LYX_PARENT", `"${LYX_PARENT}"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.EnvRef(tt.in); got != tt.want {
				t.Errorf("%s EnvRef(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestShell_ScriptExt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sh   Shell
		want string
	}{
		{"pwsh", Pwsh(), ".ps1"},
		{"posix", Posix(), ".sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sh.ScriptExt(); got != tt.want {
				t.Errorf("%s ScriptExt() = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestForGOOS(t *testing.T) {
	t.Parallel()
	// ForGOOS must always return a usable Shell — assert it behaves like one of the
	// two known impls rather than asserting a specific runtime.GOOS branch, since this
	// test runs on whatever host CI happens to be.
	sh := ForGOOS()
	got := sh.Quote("it's")
	if got != Pwsh().Quote("it's") && got != Posix().Quote("it's") {
		t.Errorf("ForGOOS().Quote(%q) = %q; want either the pwsh or posix form", "it's", got)
	}
}
