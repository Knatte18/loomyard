// cli.go exposes the cobra command tree for the codeintel module. It is the sole
// consumer of internal/output within the codeintel surface: internal/codeintelengine
// returns typed Go errors and results with no io.Writer/exit-code machinery (per the
// plan's engine/CLI layering Shared Decision), so this file is where every engine
// result and typed error gets mapped to the internal/output JSON envelope.

// Package codeintelcli wires internal/codeintelengine into the lyx cobra tree as the
// "codeintel" module, currently exposing a single "refs" verb that looks up every
// reference to a symbol name or an explicit source position across the languages
// internal/codeintelengine supports.
package codeintelcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/codeintelengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
)

// Command returns the cobra command tree for the codeintel module: a parent
// "codeintel" group command and its "refs" subcommand.
//
// The parent carries RunE: clihelp.GroupRunE so a bare "lyx codeintel" lists
// subcommands and an unknown subcommand emits a JSON error, matching every other
// module group in this repo (see internal/fabriccli.Command).
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codeintel",
		Short: "code intelligence lookups (references) across supported languages",
		RunE:  clihelp.GroupRunE,
	}

	cmd.AddCommand(refsCommand())
	return cmd
}

// refsCommand builds the "refs" subcommand: it resolves the target directory and
// language-server registry, parses the single positional argument into a
// codeintelengine.Query, calls codeintelengine.References, and maps the result or
// error to the internal/output JSON envelope.
func refsCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration

	refs := &cobra.Command{
		Use:   "refs <symbol|file:line:col>",
		Short: "list every reference to a symbol or source position",
		Long: `refs finds every reference to a symbol name or an explicit source position,
using the LSP "textDocument/references" request against the language server
detected for --target-dir (or --lang, to override detection).

The single positional argument is either:
  - a symbol name, resolved via the language server's workspace/symbol search:
      lyx codeintel refs MyFunction
  - an explicit "file:line:col" position (1-based line and column), bypassing
    name resolution entirely:
      lyx codeintel refs internal/foo/bar.go:42:8`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			// hubgeometry.Getwd() is the only permitted os.Getwd call outside
			// cmd/lyx/main.go; it anchors both the default target directory and
			// the overlay-base resolution below.
			cwd, err := hubgeometry.Getwd()
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			dir := targetDir
			if dir == "" {
				dir = cwd
			}

			query, err := parseQuery(args[0])
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Resolve the servers.yaml overlay base: when cwd is inside a lyx hub,
			// load the registry rooted at layout.Cwd (never layout.Hub — ConfigFile
			// resolves <baseDir>/_lyx/config/servers.yaml, so passing Hub would
			// silently miss every overlay, exactly as internal/buildercli/cli.go
			// anchors every config load at layout.Cwd). Outside a lyx hub, degrade
			// to the pinned built-in registry rather than failing the lookup.
			registry := codeintelengine.BuiltinRegistry()
			if layout, resolveErr := hubgeometry.Resolve(cwd); resolveErr == nil {
				loaded, loadErr := codeintelengine.LoadRegistry(layout.Cwd)
				if loadErr != nil {
					clihelp.SetExit(ctx, output.Err(out, loadErr.Error()))
					return nil
				}
				registry = loaded
			}

			opts := codeintelengine.Options{
				Registry:  registry,
				TargetDir: dir,
				Lang:      lang,
				Query:     query,
				Timeout:   timeout,
			}

			results, err := codeintelengine.References(ctx, opts)
			emitLookupResult(ctx, out, "references", results, err)
			return nil
		},
	}

	refs.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	refs.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	refs.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, references)")

	return refs
}

// emitLookupResult maps the result of a References/Definition call to the
// internal/output JSON envelope, implementing the plan's 0/1/2 exit-code
// contract: a nil error emits {resultsField: [...]} and exit 0; an
// *codeintelengine.ErrAmbiguousSymbol emits {"candidates": [...]} with exit 2
// (found, but the caller must disambiguate — distinct from both success and
// failure); every other non-nil error falls through to output.Err's plain
// error-string envelope and hardcoded exit 1, which already serves as the
// design's "not found" contract value (ErrSymbolNotFound included) with no
// special-casing needed. resultsField is the caller-supplied JSON key
// ("references" for refs, "definitions" for definition) so this one helper
// serves both verbs, which differ only in that key name.
func emitLookupResult(ctx context.Context, out io.Writer, resultsField string, results []codeintelengine.Reference, err error) {
	if err != nil {
		var ambiguous *codeintelengine.ErrAmbiguousSymbol
		if errors.As(err, &ambiguous) {
			// output.Ok always returns 0, which SetExit would treat as a no-op
			// anyway; the exit code must be forced to 2 via a separate
			// clihelp.SetExit call, exactly as the plan's exit-code-contract
			// decision specifies.
			output.Ok(out, map[string]any{"candidates": ambiguous.Candidates})
			clihelp.SetExit(ctx, 2)
			return
		}

		// No other engine error type gets special-cased: ErrSymbolNotFound and
		// everything else fall through to output.Err's hardcoded exit 1, which
		// is already the design's "not found" contract value.
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return
	}

	clihelp.SetExit(ctx, output.Ok(out, map[string]any{resultsField: referenceFields(results)}))
}

// referenceFields converts each codeintelengine.Reference into the
// {file,line,character} map shape the JSON envelope emits.
func referenceFields(refs []codeintelengine.Reference) []map[string]any {
	fields := make([]map[string]any, len(refs))
	for i, r := range refs {
		fields[i] = map[string]any{
			"file":      r.File,
			"line":      r.Line,
			"character": r.Character,
		}
	}
	return fields
}

// parseQuery converts the single "refs" positional argument into a
// codeintelengine.Query: an explicit "file:line:col" position when arg matches that
// shape (see parsePosition), otherwise a bare symbol name.
func parseQuery(arg string) (codeintelengine.Query, error) {
	pos, ok := parsePosition(arg)
	if !ok {
		return codeintelengine.Query{Symbol: arg}, nil
	}

	// codeintelengine.Query.Pos.File must be an absolute path — References turns
	// it into a file:// URI directly, with no further resolution — so a relative
	// "file:line:col" argument is resolved against the process cwd here, the one
	// point where the CLI, not the engine, owns path interpretation.
	absFile, err := filepath.Abs(pos.File)
	if err != nil {
		return codeintelengine.Query{}, fmt.Errorf("resolve absolute path for %s: %w", pos.File, err)
	}
	pos.File = absFile

	return codeintelengine.Query{Pos: &pos}, nil
}

// parsePosition reports whether arg has the "file:line:col" shape — a path
// followed by two colon-separated positive integers — and if so returns the
// parsed codeintelengine.Position. It scans from the right (the last two colons)
// rather than splitting on every colon, so a Windows drive-letter path such as
// "C:\foo\bar.go:42:8" still parses correctly: only the trailing two segments are
// required to be integers, and everything before them is taken as File verbatim.
func parsePosition(arg string) (codeintelengine.Position, bool) {
	lastColon := strings.LastIndex(arg, ":")
	if lastColon < 0 {
		return codeintelengine.Position{}, false
	}
	col, err := strconv.Atoi(arg[lastColon+1:])
	if err != nil {
		return codeintelengine.Position{}, false
	}

	rest := arg[:lastColon]
	secondColon := strings.LastIndex(rest, ":")
	if secondColon < 0 {
		return codeintelengine.Position{}, false
	}
	line, err := strconv.Atoi(rest[secondColon+1:])
	if err != nil {
		return codeintelengine.Position{}, false
	}

	file := rest[:secondColon]
	if file == "" {
		return codeintelengine.Position{}, false
	}

	return codeintelengine.Position{File: file, Line: line, Character: col}, true
}

// RunCLI is the public seam for the codeintel module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the
// capture writer for all output (including cobra's error text), matching every
// other module's RunCLI seam.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
