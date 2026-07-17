// registry.go defines the language-server registry shape (Entry, Registry),
// the pinned built-in fallback set for the five supported languages, the
// fixed detection precedence order, and entry validation. It mirrors
// internal/modelspec's registry.go: builtins() is the offline default every
// consumer gets with zero servers.yaml present, and BuiltinRegistry() is the
// one-line exported accessor the CLI layer uses when no lyx-hub overlay base
// is resolvable.

package codeintelengine

import "fmt"

// Entry describes how to detect and launch the language server for one
// language. Markers lists the marker files/dirs that identify a project as
// this language (checked relative to the target directory); Match selects
// whether all or any of them must be present. Command is the launch argv
// (the first element is the binary looked up on $PATH); InstallHint is the
// operator-facing command to install that binary when it's missing.
type Entry struct {
	Markers     []string
	Match       string
	Command     []string
	InstallHint string
}

// Registry maps a canonical language name ("go", "python", "csharp",
// "typescript", "rust") to its Entry. The zero value (a nil map) behaves
// like an empty registry: lookups fail cleanly rather than panicking.
type Registry map[string]Entry

// precedence is the fixed language-detection order used by DetectLanguage
// when no --lang override is given. It is pinned here (not derived from map
// iteration, which is unordered in Go) per the batch-local decision: earlier
// languages win when a target directory satisfies more than one language's
// markers (e.g. a Go module vendoring a TypeScript frontend still resolves
// to "go").
var precedence = []string{"go", "rust", "csharp", "typescript", "python"}

// builtins returns the pinned, default-free fallback registry for the five
// supported languages. This is what every consumer gets with zero
// servers.yaml present — operator overrides live only in the seeded
// servers.yaml (see ConfigTemplate), never baked into Go, so changing a
// default never needs a recompile.
func builtins() Registry {
	return Registry{
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
}

// BuiltinRegistry returns the pinned built-in registry (builtins()). It is
// the registry the CLI layer uses when no lyx-hub overlay base is
// resolvable — e.g. a command run outside any worktree — so codeintel
// lookup still works with zero configuration.
func BuiltinRegistry() Registry {
	return builtins()
}

// validateEntry enforces the closed shape every Entry (built-in or
// file-supplied) must satisfy: non-empty Markers, Match in {"all", "any"},
// non-empty Command, and non-empty InstallHint. Every failure names the
// offending entry's language/alias name so the operator can find it in
// servers.yaml.
func validateEntry(name string, e Entry) error {
	if len(e.Markers) == 0 {
		return fmt.Errorf("codeintelengine: entry %q has no markers", name)
	}
	if e.Match != "all" && e.Match != "any" {
		return fmt.Errorf("codeintelengine: entry %q has invalid match %q; want \"all\" or \"any\"", name, e.Match)
	}
	if len(e.Command) == 0 {
		return fmt.Errorf("codeintelengine: entry %q has no command", name)
	}
	if e.InstallHint == "" {
		return fmt.Errorf("codeintelengine: entry %q has no install hint", name)
	}
	return nil
}
