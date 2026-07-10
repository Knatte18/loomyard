// shell_test.go table-tests both pane-shell implementations: argument quoting across
// plain, space-containing, and quote-containing inputs, and the exact Invoke/ReadFile
// output each impl composes. The pwsh quoting cases are migrated verbatim from
// claudeengine's former TestPwshSingleQuote so the coverage moves with the logic it
// tests.

package shell

import "testing"

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
