// cli.go exposes the cobra command tree for the codeintel module. It is the sole
// consumer of internal/output within the codeintel surface: internal/codeintelengine
// returns typed Go errors and results with no io.Writer/exit-code machinery (per the
// plan's engine/CLI layering Shared Decision), so this file is where every engine
// result and typed error gets mapped to the internal/output JSON envelope.

// Package codeintelcli wires internal/codeintelengine into the lyx cobra tree as the
// "codeintel" module, exposing three verbs — "refs" (every reference to a symbol or
// position), "definition" (a symbol or position's definition), and "symbol" (a
// workspace/symbol name search) — across the languages internal/codeintelengine
// supports.
//
// # The exit-code contract
//
// Every verb's single-argument call exits 0 (found), 1 (not found, or any other
// engine error), or 2 (ambiguous — the response body still carries "ok":true with a
// "candidates" field, since multiple valid answers is not a process error, just a
// result the caller must disambiguate). symbol never produces "ambiguous"/exit 2 in
// either shape: returning several workspace/symbol candidates is its ordinary
// successful answer, not an error state needing disambiguation, so its single-arg
// call only ever exits 0 or 1.
//
// A call with 2 or more positional arguments switches to batch mode instead of the
// single-symbol shape above: it returns one JSON entry per symbol under a top-level
// "results" array, each entry carrying a 4th per-entry status — "found", "not_found",
// "ambiguous" (refs/definition only), or "error" (a genuine infrastructure failure,
// distinct from a confirmed-absent "not_found") — and the process exit code is set to
// the worst status present across the whole batch, ranked found(0) < not_found(1) <
// ambiguous(2) < error(3).
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
// "codeintel" group command and its "refs", "definition", and "symbol"
// subcommands.
//
// The parent carries RunE: clihelp.GroupRunE so a bare "lyx codeintel" lists
// subcommands and an unknown subcommand emits a JSON error, matching every other
// module group in this repo (see internal/fabriccli.Command).
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codeintel",
		Short: "code intelligence lookups (references, definitions, symbol search) across supported languages",
		RunE:  clihelp.GroupRunE,
	}

	cmd.AddCommand(refsCommand())
	cmd.AddCommand(definitionCommand())
	cmd.AddCommand(symbolCommand())
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
      lyx codeintel refs internal/foo/bar.go:42:8

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"ambiguous"|"error",...}, ...]}
The process exit code is set to the worst status present across the batch
(0 < 1 < 2 < 3). Example:
    lyx codeintel refs Foo Bar Baz

The result set is complete and semantically resolved by the language server
(including calls reached only through an interface, which no amount of
grepping can prove) — a caller does not need to cross-check it with grep or
re-verify individual candidates. A successful single-arg lookup carries a
machine-readable "resolution":"complete" field as this trust marker; batch
mode carries the same field on each per-entry "found" result.`,
		Args: cobra.MinimumNArgs(1),
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

			// worktreeRoot is resolved before registry loading below so both
			// derive independently from the same cwd/dir inputs — see
			// resolveWorktreeRoot's doc comment for why it never leaves
			// WorktreeRoot empty outside a hub.
			worktreeRoot := resolveWorktreeRoot(cwd, dir)

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

			if len(args) == 1 {
				query, err := parseQuery(args[0])
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}

				opts := buildOptions(registry, dir, worktreeRoot, lang, query, timeout)

				results, err := codeintelengine.References(ctx, opts)
				emitLookupResult(ctx, out, "references", results, err)
				return nil
			}

			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				query, err := parseQuery(symbol)
				if err != nil {
					return statusError, map[string]any{"error": err.Error()}
				}
				results, err := codeintelengine.References(ctx, buildOptions(registry, dir, worktreeRoot, lang, query, timeout))
				return classifyLookupError(err, "references", results)
			})
			return nil
		},
	}

	refs.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	refs.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	refs.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, references)")

	return refs
}

// definitionCommand builds the "definition" subcommand: structurally
// identical to refsCommand (same flags, cwd/registry-resolution preamble,
// and parseQuery call), differing only in which codeintelengine entry point
// it calls (Definition instead of References) and the JSON key its results
// are reported under ("definitions").
func definitionCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration

	definition := &cobra.Command{
		Use:   "definition <symbol|file:line:col>",
		Short: "show the definition of a symbol or source position",
		Long: `definition shows the definition of a symbol name or an explicit source
position, using the LSP "textDocument/definition" request against the
language server detected for --target-dir (or --lang, to override
detection).

The single positional argument is either:
  - a symbol name, resolved via the language server's workspace/symbol search:
      lyx codeintel definition MyFunction
  - an explicit "file:line:col" position (1-based line and column), bypassing
    name resolution entirely:
      lyx codeintel definition internal/foo/bar.go:42:8

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"ambiguous"|"error",...}, ...]}
The process exit code is set to the worst status present across the batch
(0 < 1 < 2 < 3). definition has no other shape difference from refs in batch
mode. Example:
    lyx codeintel definition Foo Bar Baz

The result is semantically resolved by the language server, not text-matched
— a caller does not need to cross-check it with grep. A successful single-arg
lookup carries a machine-readable "resolution":"complete" field as this trust
marker; batch mode carries the same field on each per-entry "found" result.`,
		Args: cobra.MinimumNArgs(1),
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

			// worktreeRoot is resolved before registry loading below so both
			// derive independently from the same cwd/dir inputs — see
			// resolveWorktreeRoot's doc comment for why it never leaves
			// WorktreeRoot empty outside a hub.
			worktreeRoot := resolveWorktreeRoot(cwd, dir)

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

			if len(args) == 1 {
				query, err := parseQuery(args[0])
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}

				opts := buildOptions(registry, dir, worktreeRoot, lang, query, timeout)

				results, err := codeintelengine.Definition(ctx, opts)
				emitLookupResult(ctx, out, "definitions", results, err)
				return nil
			}

			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				query, err := parseQuery(symbol)
				if err != nil {
					return statusError, map[string]any{"error": err.Error()}
				}
				results, err := codeintelengine.Definition(ctx, buildOptions(registry, dir, worktreeRoot, lang, query, timeout))
				return classifyLookupError(err, "definitions", results)
			})
			return nil
		},
	}

	definition.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	definition.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	definition.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, definition)")

	return definition
}

// symbolCommand builds the "symbol" subcommand: it shares refsCommand's
// flags and cwd/registry-resolution preamble, but — unlike refs/definition —
// never calls parseQuery. Per the plan's symbol-semantics decision, the
// positional argument is always a plain workspace/symbol search string,
// never position-parsed, even when it happens to look like "file:line:col".
func symbolCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration

	symbol := &cobra.Command{
		Use:   "symbol <query>",
		Short: "search workspace symbols by name",
		Long: `symbol searches workspace symbols by name, using the LSP
"workspace/symbol" request against the language server detected for
--target-dir (or --lang, to override detection).

Unlike refs/definition, the positional argument is always treated as a
literal search string — even one that happens to look like "file:line:col" —
never position-parsed:
    lyx codeintel symbol MyFunction

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"error",...}, ...]}
Unlike refs/definition, symbol's status set is only three-way — there is no
"ambiguous" status and no exit code 2, since symbol never collapses multiple
matches into an ambiguity failure. Example:
    lyx codeintel symbol Foo Bar Baz`,
		Args: cobra.MinimumNArgs(1),
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

			// worktreeRoot is resolved before registry loading below so both
			// derive independently from the same cwd/dir inputs — see
			// resolveWorktreeRoot's doc comment for why it never leaves
			// WorktreeRoot empty outside a hub.
			worktreeRoot := resolveWorktreeRoot(cwd, dir)

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

			if len(args) == 1 {
				opts := buildOptions(registry, dir, worktreeRoot, lang, symbolQuery(args[0]), timeout)

				results, err := codeintelengine.Symbol(ctx, opts)
				if err != nil {
					// Symbol never returns *ErrAmbiguousSymbol (per symbol-semantics,
					// it has no ambiguous state), so emitLookupResult's ambiguity
					// branch does not apply here — this is the simple, uniform
					// error-mapping shape refsCommand used before card 33's retrofit.
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}

				clihelp.SetExit(ctx, output.Ok(out, map[string]any{"symbols": symbolMatchFields(results)}))
				return nil
			}

			// Every batch entry is built directly from the raw arg string as
			// Query.Symbol, exactly like the single-arg path above — symbol's
			// batch mode never calls parseQuery/position-parsing either, so
			// "lyx codeintel symbol foo.go:1:1 bar.go:2:2" treats both
			// arguments as literal search strings, not positions, consistent
			// across both arg-count shapes.
			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				results, err := codeintelengine.Symbol(ctx, buildOptions(registry, dir, worktreeRoot, lang, codeintelengine.Query{Symbol: symbol}, timeout))
				return classifySymbolError(err, results)
			})
			return nil
		},
	}

	symbol.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	symbol.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	symbol.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for the workspace/symbol request phase")

	return symbol
}

// resolveWorktreeRoot resolves the codeintelengine.Options.WorktreeRoot value
// a lookup rooted at targetDir should carry, given the process's cwd. Inside
// a lyx hub (hubgeometry.Resolve(cwd) succeeds), it is the resolved
// layout.WorktreeRoot — the git repository root — exactly as every verb
// already used before this helper existed. Outside a hub (the supported
// degrade-to-BuiltinRegistry path), it falls back to the absolute form of
// targetDir rather than leaving WorktreeRoot empty: see the plan's
// "Supervised daemon anchoring outside a lyx hub" Shared Decision — once a
// language flips to the supervised strategy, an empty WorktreeRoot would
// resolve EnsureServer's daemon state/lock/socket files at a cwd-relative
// ".lyx/codeintel/<lang>/" path, littering/colliding across cwds for the
// same --target-dir. filepath.Abs falls back to targetDir itself on error
// (an Abs failure here means the process's own cwd is unresolvable, an
// already-degraded environment), so this helper never returns an empty
// string. It is a separate named function, rather than inlined into each
// verb's RunE, specifically so the outside-a-hub fallback is independently
// unit-testable without a live daemon.
func resolveWorktreeRoot(cwd, targetDir string) string {
	if layout, err := hubgeometry.Resolve(cwd); err == nil {
		return layout.WorktreeRoot
	}

	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return targetDir
	}
	return abs
}

// buildOptions constructs a codeintelengine.Options value from its component
// parts. It is the single call site every refs/definition/symbol RunE
// (single-arg and batch-mode alike) goes through: before this helper
// existed, each verb's batch-mode closure built its own Options{...} literal
// by hand, and three of the six literals (the batch closures) silently
// omitted WorktreeRoot while their single-arg sibling set it — a latent
// drift that only mattered once a language's daemon strategy flips to
// supervised (native never reads WorktreeRoot). Collapsing every
// construction site through this one function makes that omission
// structurally impossible: a batch closure can no longer forget a field its
// single-arg sibling sets, since both now call the same function with the
// same locals.
func buildOptions(registry codeintelengine.Registry, targetDir, worktreeRoot, lang string, query codeintelengine.Query, timeout time.Duration) codeintelengine.Options {
	return codeintelengine.Options{
		Registry:     registry,
		TargetDir:    targetDir,
		WorktreeRoot: worktreeRoot,
		Lang:         lang,
		Query:        query,
		Timeout:      timeout,
	}
}

// symbolQuery builds a codeintelengine.Query for the "symbol" verb's single
// positional argument. Per the plan's symbol-semantics decision, arg is
// always a plain workspace/symbol search string — unlike parseQuery (which
// refs/definition use), symbolQuery never calls parsePosition, even when arg
// happens to have a "file:line:col" shape. This is a separate named function
// rather than an inline literal so the "never position-parsed" contract is
// independently unit-testable without a live language server.
func symbolQuery(arg string) codeintelengine.Query {
	return codeintelengine.Query{Symbol: arg}
}

// symbolMatchFields converts each codeintelengine.SymbolMatch into the
// {name,kind,file,line,character} map shape the JSON envelope emits,
// mirroring referenceFields's exact shape/style for Symbol's richer result
// type.
func symbolMatchFields(matches []codeintelengine.SymbolMatch) []map[string]any {
	fields := make([]map[string]any, len(matches))
	for i, m := range matches {
		fields[i] = map[string]any{
			"name":      m.Name,
			"kind":      m.Kind,
			"file":      m.File,
			"line":      m.Line,
			"character": m.Character,
		}
	}
	return fields
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

	// "resolution":"complete" is the machine-readable trust marker a caller
	// can key on to skip a redundant grep/re-verify pass: the language server
	// already resolved the query exhaustively, unlike a text-matched result.
	clihelp.SetExit(ctx, output.Ok(out, map[string]any{resultsField: referenceFields(results), "resolution": "complete"}))
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

// batchStatus is the per-symbol outcome batch mode reports for each entry in
// a multi-argument refs/definition/symbol call. Its four values rank
// strictly worst-to-best via statusRank, per the plan's batch-mode-cli
// exit-code contract: found < not_found < ambiguous < error.
type batchStatus string

const (
	statusFound     batchStatus = "found"
	statusNotFound  batchStatus = "not_found"
	statusAmbiguous batchStatus = "ambiguous"
	statusError     batchStatus = "error"
)

// statusRank orders batchStatus values from best (0) to worst (3) outcome,
// so runBatch can pick the process exit code that reflects the worst status
// present across an entire batch — a batch that finds every symbol exits 0,
// one with any error anywhere exits 3, regardless of how many other symbols
// succeeded.
var statusRank = map[batchStatus]int{
	statusFound:     0,
	statusNotFound:  1,
	statusAmbiguous: 2,
	statusError:     3,
}

// classifyLookupError maps a References/Definition call's outcome to a
// batchStatus and its extra JSON fields, shared by refs and definition's
// batch-mode closures (card 39). A nil err is statusFound, carrying results
// under resultsField. An *codeintelengine.ErrAmbiguousSymbol is
// statusAmbiguous, carrying the candidate list — batch mode surfaces the
// same ambiguity emitLookupResult's exit-2 path already reports for
// single-arg mode, just per-entry instead of for the whole call.
// ErrSymbolNotFoundSentinel is statusNotFound with no extra fields — a
// confirmed absence needs nothing more reported. Every other error
// (ErrNoLanguage, ErrServerNotFound, ErrServerTimeout,
// ErrResolverUnsupported, a toolchain-install failure, ...) is statusError,
// since none of them mean "confirmed absent."
func classifyLookupError(err error, resultsField string, results []codeintelengine.Reference) (batchStatus, map[string]any) {
	if err == nil {
		// Mirror emitLookupResult's single-arg "resolution":"complete" marker
		// per batch entry, so a batch-mode caller gets the same trust signal
		// on each "found" result the single-arg envelope carries.
		return statusFound, map[string]any{resultsField: referenceFields(results), "resolution": "complete"}
	}

	var ambiguous *codeintelengine.ErrAmbiguousSymbol
	if errors.As(err, &ambiguous) {
		return statusAmbiguous, map[string]any{"candidates": ambiguous.Candidates}
	}

	if errors.Is(err, codeintelengine.ErrSymbolNotFoundSentinel) {
		return statusNotFound, nil
	}

	return statusError, map[string]any{"error": err.Error()}
}

// classifySymbolError is classifyLookupError's twin for Symbol's batch-mode
// closure (card 40). It shares the same nil/not-found/else structure but has
// no ambiguous branch at all — per the plan's symbol-semantics decision,
// Symbol never collapses multiple candidates into ambiguity, so
// classifySymbolError has no case that could ever produce statusAmbiguous.
func classifySymbolError(err error, results []codeintelengine.SymbolMatch) (batchStatus, map[string]any) {
	if err == nil {
		return statusFound, map[string]any{"symbols": symbolMatchFields(results)}
	}

	if errors.Is(err, codeintelengine.ErrSymbolNotFoundSentinel) {
		return statusNotFound, nil
	}

	return statusError, map[string]any{"error": err.Error()}
}

// runBatch drives batch mode for any of the three verbs: it calls lookupOne
// once per entry in args, builds one {"symbol":..., "status":..., ...}
// envelope entry per call (lookupOne's returned fields map merged in — a nil
// fields map, the not-found case, merges nothing extra), emits
// {"results": [...]} via output.Ok, and overrides the process exit code to
// the worst statusRank seen across the batch. output.Ok's return value is
// discarded — like emitLookupResult's own output.Ok call, it always returns
// 0 — and clihelp.SetExit is only called when the worst rank is non-zero:
// when every symbol is found, the rank is already 0 and SetExit(ctx, 0)
// would be a no-op anyway. runBatch has no opinion on how lookupOne resolves
// a symbol; that is each verb's own closure (cards 39, 40).
func runBatch(ctx context.Context, out io.Writer, args []string, lookupOne func(symbol string) (batchStatus, map[string]any)) {
	entries := make([]map[string]any, len(args))
	worst := statusFound
	for i, arg := range args {
		status, fields := lookupOne(arg)
		if statusRank[status] > statusRank[worst] {
			worst = status
		}

		entry := map[string]any{"symbol": arg, "status": string(status)}
		for k, v := range fields {
			entry[k] = v
		}
		entries[i] = entry
	}

	output.Ok(out, map[string]any{"results": entries})
	if statusRank[worst] != 0 {
		clihelp.SetExit(ctx, statusRank[worst])
	}
}

// RunCLI is the public seam for the codeintel module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the
// capture writer for all output (including cobra's error text), matching every
// other module's RunCLI seam.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
