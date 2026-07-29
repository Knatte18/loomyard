// registry_test.go table-drives builtins() against the pinned defaults from
// the language-server-registry Shared Decision, and validateEntry against
// each closed-vocabulary violation it must reject.

package codeintelengine

import (
	"strings"
	"testing"
)

func TestBuiltins(t *testing.T) {
	b := builtins()
	want := map[string]Entry{
		"go": {
			Markers:     []string{"go.mod"},
			Match:       "any",
			Command:     []string{"gopls"},
			InstallHint: "go install golang.org/x/tools/gopls@latest",
		},
		"python": {
			Markers:     []string{"pyproject.toml", "setup.py", "setup.cfg"},
			Match:       "any",
			Command:     []string{"pyright-langserver", "--stdio"},
			InstallHint: "npm install -g pyright",
		},
		"csharp": {
			Markers:     []string{".sln", ".csproj"},
			Match:       "any",
			Command:     []string{"csharp-ls"},
			InstallHint: "dotnet tool install --global csharp-ls",
		},
		"typescript": {
			Markers:     []string{"package.json", "tsconfig.json"},
			Match:       "all",
			Command:     []string{"typescript-language-server", "--stdio"},
			InstallHint: "npm install -g typescript-language-server typescript",
		},
		"rust": {
			Markers:     []string{"Cargo.toml"},
			Match:       "any",
			Command:     []string{"rust-analyzer"},
			InstallHint: "install via rustup component add rust-analyzer",
		},
	}

	if len(b) != len(want) {
		t.Fatalf("builtins() has %d entries; want %d", len(b), len(want))
	}
	for lang, wantEntry := range want {
		gotEntry, ok := b[lang]
		if !ok {
			t.Errorf("builtins() missing language %q", lang)
			continue
		}
		if !stringSlicesEqual(gotEntry.Markers, wantEntry.Markers) {
			t.Errorf("builtins()[%q].Markers = %v; want %v", lang, gotEntry.Markers, wantEntry.Markers)
		}
		if gotEntry.Match != wantEntry.Match {
			t.Errorf("builtins()[%q].Match = %q; want %q", lang, gotEntry.Match, wantEntry.Match)
		}
		if !stringSlicesEqual(gotEntry.Command, wantEntry.Command) {
			t.Errorf("builtins()[%q].Command = %v; want %v", lang, gotEntry.Command, wantEntry.Command)
		}
		if gotEntry.InstallHint != wantEntry.InstallHint {
			t.Errorf("builtins()[%q].InstallHint = %q; want %q", lang, gotEntry.InstallHint, wantEntry.InstallHint)
		}
	}
}

func TestBuiltins_GoHasPinnedNativeDaemon(t *testing.T) {
	b := builtins()
	goEntry := b["go"]
	if goEntry.PinnedVersion != "v0.23.0" {
		t.Errorf("builtins()[%q].PinnedVersion = %q; want %q", "go", goEntry.PinnedVersion, "v0.23.0")
	}
	if !goEntry.HasNativeDaemon {
		t.Errorf("builtins()[%q].HasNativeDaemon = %v; want %v", "go", goEntry.HasNativeDaemon, true)
	}
}

func TestBuiltins_NonGoLanguagesHaveNoDaemonStrategy(t *testing.T) {
	// Every non-Go builtin must leave PinnedVersion/HasNativeDaemon at their
	// Go zero values -- this is the "third, implicit no-daemon-strategy
	// mode" the registry-scope decision requires, so a caller checking
	// entry.HasNativeDaemon for these languages always gets false and never
	// invokes ensureServer.
	tests := []string{"python", "csharp", "typescript", "rust"}
	b := builtins()
	for _, lang := range tests {
		t.Run(lang, func(t *testing.T) {
			entry, ok := b[lang]
			if !ok {
				t.Fatalf("builtins() missing language %q", lang)
			}
			if entry.PinnedVersion != "" {
				t.Errorf("builtins()[%q].PinnedVersion = %q; want %q", lang, entry.PinnedVersion, "")
			}
			if entry.HasNativeDaemon {
				t.Errorf("builtins()[%q].HasNativeDaemon = %v; want %v", lang, entry.HasNativeDaemon, false)
			}
		})
	}
}

func TestValidateEntry_AcceptsBuiltinsZeroValuedNewFields(t *testing.T) {
	// Regression guard: validateEntry must keep accepting every builtin
	// entry unchanged now that Entry carries PinnedVersion/HasNativeDaemon --
	// the four non-Go entries leave both fields at their zero values, and
	// validateEntry must not start rejecting them.
	for lang, entry := range builtins() {
		t.Run(lang, func(t *testing.T) {
			if err := validateEntry(lang, entry); err != nil {
				t.Errorf("validateEntry(%q, builtins()[%q]) returned unexpected error: %v", lang, lang, err)
			}
		})
	}
}

func TestBuiltinRegistry_MatchesBuiltins(t *testing.T) {
	got := BuiltinRegistry()
	want := builtins()
	if len(got) != len(want) {
		t.Fatalf("BuiltinRegistry() has %d entries; want %d", len(got), len(want))
	}
	for lang := range want {
		if _, ok := got[lang]; !ok {
			t.Errorf("BuiltinRegistry() missing language %q present in builtins()", lang)
		}
	}
}

func TestPrecedence_PinnedOrder(t *testing.T) {
	want := []string{"go", "rust", "csharp", "typescript", "python"}
	if !stringSlicesEqual(precedence, want) {
		t.Errorf("precedence = %v; want %v", precedence, want)
	}
}

func TestValidateEntry(t *testing.T) {
	valid := Entry{
		Markers:     []string{"go.mod"},
		Match:       "any",
		Command:     []string{"gopls"},
		InstallHint: "go install gopls",
	}

	tests := []struct {
		name       string
		entry      Entry
		wantErr    bool
		wantSubstr string
	}{
		{
			name:    "valid entry passes",
			entry:   valid,
			wantErr: false,
		},
		{
			name: "empty markers",
			entry: Entry{
				Match:       "any",
				Command:     []string{"gopls"},
				InstallHint: "go install gopls",
			},
			wantErr:    true,
			wantSubstr: "no markers",
		},
		{
			name: "out-of-vocab match",
			entry: Entry{
				Markers:     []string{"go.mod"},
				Match:       "some",
				Command:     []string{"gopls"},
				InstallHint: "go install gopls",
			},
			wantErr:    true,
			wantSubstr: "invalid match",
		},
		{
			name: "empty command",
			entry: Entry{
				Markers:     []string{"go.mod"},
				Match:       "any",
				InstallHint: "go install gopls",
			},
			wantErr:    true,
			wantSubstr: "no command",
		},
		{
			name: "empty install hint",
			entry: Entry{
				Markers: []string{"go.mod"},
				Match:   "any",
				Command: []string{"gopls"},
			},
			wantErr:    true,
			wantSubstr: "no install hint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEntry("go", tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateEntry(%q, %+v) returned nil error; want error containing %q", "go", tt.entry, tt.wantSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("validateEntry error = %q; want substring %q", err.Error(), tt.wantSubstr)
				}
				if !strings.Contains(err.Error(), `"go"`) {
					t.Errorf("validateEntry error = %q; want it to name the entry %q", err.Error(), "go")
				}
			} else if err != nil {
				t.Errorf("validateEntry(%+v) returned unexpected error: %v", tt.entry, err)
			}
		})
	}
}

// stringSlicesEqual reports whether a and b contain the same elements in the
// same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
