// cli.go exposes the cobra command tree for the trace module. It is the sole
// consumer of internal/output within the trace surface: internal/traceengine
// returns typed Go errors and results with no io.Writer/exit-code machinery (per the
// plan's engine/CLI layering Shared Decision), so this file is where every engine
// result and typed error gets mapped to the internal/output JSON envelope.

// Package tracecli wires internal/traceengine into the lyx cobra tree as the
// "trace" module, exposing four verbs — "refs" (every reference to a symbol or
// position), "definition" (a symbol or position's definition), "symbol" (a
// workspace/symbol name search), and "assert-no-callers" (a CI-shaped gate: fail if
// a symbol has any caller outside its declaration and an allowed list) — across the
// languages internal/traceengine supports.
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
package tracecli

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
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/traceengine"
)

// Command returns the cobra command tree for the trace module: a parent
// "trace" group command and its "refs", "definition", and "symbol"
// subcommands.
//
// The parent carries RunE: clihelp.GroupRunE so a bare "lyx trace" lists
// subcommands and an unknown subcommand emits a JSON error, matching every other
// module group in this repo (see internal/fabriccli.Command).
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "code intelligence lookups (references, definitions, symbol search) across supported languages",
		RunE:  clihelp.GroupRunE,
	}

	cmd.AddCommand(refsCommand())
	cmd.AddCommand(definitionCommand())
	cmd.AddCommand(symbolCommand())
	cmd.AddCommand(assertNoCallersCommand())
	return cmd
}

// refsCommand builds the "refs" subcommand: it resolves the target directory and
// language-server registry, parses the single positional argument into a
// traceengine.Query, calls traceengine.References, and maps the result or
// error to the internal/output JSON envelope.
func refsCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration
	var inFile string
	var within string

	refs := &cobra.Command{
		Use:   "refs <symbol|file:line:col>",
		Short: "list every reference to a symbol or source position",
		Long: `refs finds every reference to a symbol name or an explicit source position,
using the LSP "textDocument/references" request against the language server
detected for --target-dir (or --lang, to override detection).

The single positional argument is either:
  - a symbol name, resolved via the language server's workspace/symbol search:
      lyx trace refs MyFunction
  - an explicit "file:line:col" position (1-based line and column), bypassing
    name resolution entirely:
      lyx trace refs internal/foo/bar.go:42:8

--in-file <path> resolves each positional argument as a bare symbol name
within exactly that one file, via an exhaustive textDocument/documentSymbol
search rather than a project-wide workspace/symbol search — the positional
is always treated as a bare name, never position-parsed, even if it happens
to look like "file:line:col":
    lyx trace refs --in-file internal/foo/bar.go MyFunc

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"ambiguous"|"error",...}, ...]}
The process exit code is set to the worst status present across the batch
(0 < 1 < 2 < 3). Example:
    lyx trace refs Foo Bar Baz
--in-file composes with batch mode too, resolving every positional against
the same file:
    lyx trace refs --in-file internal/foo/bar.go Open Close

The result set is complete and semantically resolved by the language server
(including calls reached only through an interface, which no amount of
grepping can prove) — a caller does not need to cross-check it with grep or
re-verify individual candidates. A successful single-arg lookup carries a
machine-readable "resolution":"complete" field as this trust marker; batch
mode carries the same field on each per-entry "found" result.

Known limitation, and what --within is for: gopls' references for an
interface method conservatively include every method matching that
name+signature across every structurally-compatible interface anywhere in
the workspace — not just calls through the specific interface value you
queried. This is documented gopls behavior, not a bug in this wrapper, but
it means two unrelated, identically-shaped interfaces in different packages
(e.g. two local "type clock interface { Now() time.Time }" declarations)
conflate their results by default. "resolution":"complete" still means
"every result for the query as given" — it is not false — but for an
unscoped interface-method query, "complete" can include out-of-scope noise
a caller must still filter by hand. --within <dir> restricts the result set
to references whose file lies within <dir> (relative to --target-dir, or
absolute), discarding everything else, so a query already known to be
scoped to one package comes back both complete and precise:
    lyx trace refs --within internal/builderengine SomeMethod`,
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
			registry := traceengine.BuiltinRegistry()
			if layout, resolveErr := hubgeometry.Resolve(cwd); resolveErr == nil {
				loaded, loadErr := traceengine.LoadRegistry(layout.Cwd)
				if loadErr != nil {
					clihelp.SetExit(ctx, output.Err(out, loadErr.Error()))
					return nil
				}
				registry = loaded
			}

			// buildQuery is the one seam both the single-arg and batch-mode
			// paths below call to turn a positional argument into a Query:
			// --in-file routes every argument through inFileQuery instead of
			// parseQuery, so a positional is always a bare name against that
			// one file, never position-parsed — even when --in-file is
			// combined with batch mode.
			buildQuery := func(arg string) (traceengine.Query, error) {
				if inFile != "" {
					return inFileQuery(inFile, arg)
				}
				return parseQuery(arg)
			}

			if len(args) == 1 {
				query, err := buildQuery(args[0])
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}

				opts := buildOptions(registry, dir, worktreeRoot, lang, query, timeout)

				results, err := traceengine.References(ctx, opts)
				if err == nil && within != "" {
					results = filterWithin(results, within, dir)
				}
				emitLookupResult(ctx, out, "references", results, err)
				return nil
			}

			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				query, err := buildQuery(symbol)
				if err != nil {
					return statusError, map[string]any{"error": err.Error()}
				}
				results, err := traceengine.References(ctx, buildOptions(registry, dir, worktreeRoot, lang, query, timeout))
				if err == nil && within != "" {
					results = filterWithin(results, within, dir)
				}
				return classifyLookupError(err, "references", results)
			})
			return nil
		},
	}

	refs.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	refs.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	refs.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, references)")
	refs.Flags().StringVar(&inFile, "in-file", "", "resolve each positional argument as a bare symbol name within this one file, instead of a project-wide workspace/symbol search")
	refs.Flags().StringVar(&within, "within", "", "restrict results to references whose file lies within this directory (relative to --target-dir, or absolute) — see the interface-method conflation note above")

	return refs
}

// definitionCommand builds the "definition" subcommand: structurally
// identical to refsCommand (same flags, cwd/registry-resolution preamble,
// and parseQuery call), differing only in which traceengine entry point
// it calls (Definition instead of References) and the JSON key its results
// are reported under ("definitions").
func definitionCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration
	var inFile string
	var within string

	definition := &cobra.Command{
		Use:   "definition <symbol|file:line:col>",
		Short: "show the definition of a symbol or source position",
		Long: `definition shows the definition of a symbol name or an explicit source
position, using the LSP "textDocument/definition" request against the
language server detected for --target-dir (or --lang, to override
detection).

The single positional argument is either:
  - a symbol name, resolved via the language server's workspace/symbol search:
      lyx trace definition MyFunction
  - an explicit "file:line:col" position (1-based line and column), bypassing
    name resolution entirely:
      lyx trace definition internal/foo/bar.go:42:8

--in-file <path> resolves each positional argument as a bare symbol name
within exactly that one file, via an exhaustive textDocument/documentSymbol
search rather than a project-wide workspace/symbol search — the positional
is always treated as a bare name, never position-parsed, even if it happens
to look like "file:line:col":
    lyx trace definition --in-file internal/foo/bar.go MyFunc

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"ambiguous"|"error",...}, ...]}
The process exit code is set to the worst status present across the batch
(0 < 1 < 2 < 3). definition has no other shape difference from refs in batch
mode. Example:
    lyx trace definition Foo Bar Baz
--in-file composes with batch mode too, resolving every positional against
the same file:
    lyx trace definition --in-file internal/foo/bar.go Open Close

The result is semantically resolved by the language server, not text-matched
— a caller does not need to cross-check it with grep. A successful single-arg
lookup carries a machine-readable "resolution":"complete" field as this trust
marker; batch mode carries the same field on each per-entry "found" result.

--within <dir> restricts the result set to definitions whose file lies
within <dir> (relative to --target-dir, or absolute) — see "refs --help"
for why this exists (interface-method reference conflation across
structurally-identical interfaces in different packages).`,
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
			registry := traceengine.BuiltinRegistry()
			if layout, resolveErr := hubgeometry.Resolve(cwd); resolveErr == nil {
				loaded, loadErr := traceengine.LoadRegistry(layout.Cwd)
				if loadErr != nil {
					clihelp.SetExit(ctx, output.Err(out, loadErr.Error()))
					return nil
				}
				registry = loaded
			}

			// buildQuery is the one seam both the single-arg and batch-mode
			// paths below call to turn a positional argument into a Query:
			// --in-file routes every argument through inFileQuery instead of
			// parseQuery, so a positional is always a bare name against that
			// one file, never position-parsed — even when --in-file is
			// combined with batch mode.
			buildQuery := func(arg string) (traceengine.Query, error) {
				if inFile != "" {
					return inFileQuery(inFile, arg)
				}
				return parseQuery(arg)
			}

			if len(args) == 1 {
				query, err := buildQuery(args[0])
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}

				opts := buildOptions(registry, dir, worktreeRoot, lang, query, timeout)

				results, err := traceengine.Definition(ctx, opts)
				if err == nil && within != "" {
					results = filterWithin(results, within, dir)
				}
				emitLookupResult(ctx, out, "definitions", results, err)
				return nil
			}

			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				query, err := buildQuery(symbol)
				if err != nil {
					return statusError, map[string]any{"error": err.Error()}
				}
				results, err := traceengine.Definition(ctx, buildOptions(registry, dir, worktreeRoot, lang, query, timeout))
				if err == nil && within != "" {
					results = filterWithin(results, within, dir)
				}
				return classifyLookupError(err, "definitions", results)
			})
			return nil
		},
	}

	definition.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	definition.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	definition.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, definition)")
	definition.Flags().StringVar(&inFile, "in-file", "", "resolve each positional argument as a bare symbol name within this one file, instead of a project-wide workspace/symbol search")
	definition.Flags().StringVar(&within, "within", "", "restrict results to definitions whose file lies within this directory (relative to --target-dir, or absolute)")

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
    lyx trace symbol MyFunction

Passing 2 or more positional arguments switches to batch mode: each argument
is looked up independently and the results are reported as one array, rather
than the single-symbol envelope above:
    {"ok":true,"results":[{"symbol":...,"status":"found"|"not_found"|"error",...}, ...]}
Unlike refs/definition, symbol's status set is only three-way — there is no
"ambiguous" status and no exit code 2, since symbol never collapses multiple
matches into an ambiguity failure. Example:
    lyx trace symbol Foo Bar Baz`,
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
			registry := traceengine.BuiltinRegistry()
			if layout, resolveErr := hubgeometry.Resolve(cwd); resolveErr == nil {
				loaded, loadErr := traceengine.LoadRegistry(layout.Cwd)
				if loadErr != nil {
					clihelp.SetExit(ctx, output.Err(out, loadErr.Error()))
					return nil
				}
				registry = loaded
			}

			if len(args) == 1 {
				opts := buildOptions(registry, dir, worktreeRoot, lang, symbolQuery(args[0]), timeout)

				results, err := traceengine.Symbol(ctx, opts)
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
			// "lyx trace symbol foo.go:1:1 bar.go:2:2" treats both
			// arguments as literal search strings, not positions, consistent
			// across both arg-count shapes.
			runBatch(ctx, out, args, func(symbol string) (batchStatus, map[string]any) {
				results, err := traceengine.Symbol(ctx, buildOptions(registry, dir, worktreeRoot, lang, traceengine.Query{Symbol: symbol}, timeout))
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

// resolveWorktreeRoot resolves the traceengine.Options.WorktreeRoot value
// a lookup rooted at targetDir should carry, given the process's cwd. Inside
// a lyx hub (hubgeometry.Resolve(cwd) succeeds), it is the resolved
// layout.WorktreeRoot — the git repository root — exactly as every verb
// already used before this helper existed. Outside a hub (the supported
// degrade-to-BuiltinRegistry path), it falls back to the absolute form of
// targetDir rather than leaving WorktreeRoot empty: see the plan's
// "Supervised daemon anchoring outside a lyx hub" Shared Decision — once a
// language flips to the supervised strategy, an empty WorktreeRoot would
// resolve EnsureServer's daemon state/lock/socket files at a cwd-relative
// ".lyx/trace/<lang>/" path, littering/colliding across cwds for the
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

// buildOptions constructs a traceengine.Options value from its component
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
func buildOptions(registry traceengine.Registry, targetDir, worktreeRoot, lang string, query traceengine.Query, timeout time.Duration) traceengine.Options {
	return traceengine.Options{
		Registry:     registry,
		TargetDir:    targetDir,
		WorktreeRoot: worktreeRoot,
		Lang:         lang,
		Query:        query,
		Timeout:      timeout,
	}
}

// symbolQuery builds a traceengine.Query for the "symbol" verb's single
// positional argument. Per the plan's symbol-semantics decision, arg is
// always a plain workspace/symbol search string — unlike parseQuery (which
// refs/definition use), symbolQuery never calls parsePosition, even when arg
// happens to have a "file:line:col" shape. This is a separate named function
// rather than an inline literal so the "never position-parsed" contract is
// independently unit-testable without a live language server.
func symbolQuery(arg string) traceengine.Query {
	return traceengine.Query{Symbol: arg}
}

// assertNoCallersCommand builds the "assert-no-callers" subcommand: a CI-shaped
// gate, not a lookup verb. It resolves the same symbol|file:line:col argument
// refs/definition accept, then fails (exit 1) if any reference to it survives
// after excluding its own declaration site and every --except path. Intended
// for a mill batch's "verify:" step, so a plan's Deletes:/Moves: safety claim
// becomes a deterministic CI check instead of a reviewer's manual judgment call.
func assertNoCallersCommand() *cobra.Command {
	var targetDir string
	var lang string
	var timeout time.Duration
	var except []string
	var within string

	cmd := &cobra.Command{
		Use:   "assert-no-callers <symbol|file:line:col>",
		Short: "fail if a symbol has any caller outside its declaration and --except paths",
		Long: `assert-no-callers finds every reference to a symbol name or an explicit
source position (the same resolution refs/definition use), then fails if any
reference remains once its own declaration site and every --except path are
excluded.

Intended for a mill batch's "verify:" step: mechanically confirm a Deletes: or
Moves: batch's target symbol truly has no remaining external callers before
the batch is approved, turning a review-discipline judgment call into a
deterministic CI gate.

The single positional argument is either a symbol name (resolved via
workspace/symbol) or an explicit "file:line:col" position, exactly as for
refs/definition. --except may be repeated; each is a file path (relative to
--target-dir, or absolute) allowed to keep referencing the symbol without
failing the check — typically the symbol's own sanctioned wrapper.

Exit 0: no unexpected callers remain; the envelope carries an empty "callers"
list. Exit 1: either one or more unexpected callers were found (the envelope
carries "violation":true and the "callers" list), or the lookup itself failed
(not found, server error) — the "violation" field is the only way to tell
these two exit-1 cases apart. Exit 2: the symbol name was ambiguous (more than
one workspace/symbol candidate) — the envelope carries "candidates", exactly
as refs/definition already report ambiguity.

Use --within <dir> when checking an interface method. gopls' references for
an interface method conservatively include every method matching that
name+signature across every structurally-compatible interface anywhere in
the workspace — not just calls through the interface value you're actually
checking. Without --within, that means checking an interface method can
report an unrelated, structurally-identical interface elsewhere in the repo
as a false "violation" (e.g. two unrelated local "type clock interface {
Now() time.Time }" declarations in different packages). --within <dir>
restricts the caller search to references whose file lies within <dir>
(relative to --target-dir, or absolute) before --except is applied, so a
check already known to be scoped to one package's own interface stays
correct. This has no effect on plain functions/methods with no interface
involved — only interface methods are at risk of this conflation.`,
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

			registry := traceengine.BuiltinRegistry()
			var worktreeRoot string
			if layout, resolveErr := hubgeometry.Resolve(cwd); resolveErr == nil {
				loaded, loadErr := traceengine.LoadRegistry(layout.Cwd)
				if loadErr != nil {
					clihelp.SetExit(ctx, output.Err(out, loadErr.Error()))
					return nil
				}
				registry = loaded
				worktreeRoot = layout.WorktreeRoot
			}

			query, err := parseQuery(args[0])
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			baseOpts := traceengine.Options{
				Registry:     registry,
				TargetDir:    dir,
				WorktreeRoot: worktreeRoot,
				Lang:         lang,
				Timeout:      timeout,
			}

			// Resolve the declaration site(s) to exclude via Definition,
			// regardless of whether query is a bare symbol name or an explicit
			// position: Definition's Reference results are always in LSP's
			// UTF-16-column coordinate system (same as References's own
			// results below), so declRefs and refs stay directly comparable.
			// Building a Reference straight from a caller-supplied Position
			// instead would be wrong whenever query.Pos is set — Position's
			// Character is a 1-based byte column, which only coincides with
			// Reference's 1-based UTF-16 column on a pure-ASCII line; on any
			// other line the declaration would silently fail to match and be
			// misreported as an unexpected caller.
			defOpts := baseOpts
			defOpts.Query = query
			declRefs, defErr := traceengine.Definition(ctx, defOpts)
			if defErr != nil {
				emitAmbiguousOrError(ctx, out, defErr)
				return nil
			}

			refOpts := baseOpts
			refOpts.Query = query
			refs, refErr := traceengine.References(ctx, refOpts)
			if refErr != nil {
				emitAmbiguousOrError(ctx, out, refErr)
				return nil
			}
			if within != "" {
				// Scope the candidate set to the intended package before
				// --except even runs — see the Long help's --within
				// paragraph for why an unscoped interface-method check can
				// otherwise report a false "violation" from an unrelated,
				// structurally-identical interface elsewhere in the repo.
				refs = filterWithin(refs, within, dir)
			}

			exceptAbs := make(map[string]bool, len(except))
			for _, e := range except {
				p := e
				if !filepath.IsAbs(p) {
					p = filepath.Join(dir, p)
				}
				exceptAbs[filepath.Clean(p)] = true
			}

			violations := filterUnexpectedCallers(refs, declRefs, exceptAbs)
			if len(violations) == 0 {
				clihelp.SetExit(ctx, output.Ok(out, map[string]any{"callers": []map[string]any{}}))
				return nil
			}

			output.Ok(out, map[string]any{"violation": true, "callers": referenceFields(violations)})
			clihelp.SetExit(ctx, 1)
			return nil
		},
	}

	cmd.Flags().StringVar(&targetDir, "target-dir", "", "project directory to detect the language in and root the server at (default: cwd)")
	cmd.Flags().StringVar(&lang, "lang", "", "override language detection with this registry key")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "deadline for each LSP request phase (initialize, resolve, references/definition)")
	cmd.Flags().StringArrayVar(&except, "except", nil, "file path allowed to reference the symbol without failing the check (repeatable)")
	cmd.Flags().StringVar(&within, "within", "", "restrict the caller search to references whose file lies within this directory (relative to --target-dir, or absolute) — required for a correct check on an interface method, see above")

	return cmd
}

// emitAmbiguousOrError maps a References/Definition error to the envelope,
// identically to emitLookupResult's own error branch: *ErrAmbiguousSymbol
// becomes exit 2 with candidates, everything else becomes exit 1 via
// output.Err. It returns false in both cases (there is never a "handled but
// continue" outcome) — the bool return exists only so a call site can use it
// as a single-expression early-return guard without repeating the two-branch
// body inline.
func emitAmbiguousOrError(ctx context.Context, out io.Writer, err error) bool {
	var ambiguous *traceengine.ErrAmbiguousSymbol
	if errors.As(err, &ambiguous) {
		output.Ok(out, map[string]any{"candidates": ambiguous.Candidates})
		clihelp.SetExit(ctx, 2)
		return false
	}
	clihelp.SetExit(ctx, output.Err(out, err.Error()))
	return false
}

// filterUnexpectedCallers returns every entry in refs that is neither one of
// declRefs (compared by exact file+line+character — both refs and declRefs
// come from traceengine's Reference type, so this is a same-coordinate-
// system comparison, never a byte-column-vs-UTF-16 mismatch) nor whose File
// is a key in exceptAbs (already filepath.Clean'd, absolute paths).
func filterUnexpectedCallers(refs []traceengine.Reference, declRefs []traceengine.Reference, exceptAbs map[string]bool) []traceengine.Reference {
	declSet := make(map[traceengine.Reference]bool, len(declRefs))
	for _, d := range declRefs {
		declSet[d] = true
	}

	var violations []traceengine.Reference
	for _, r := range refs {
		if declSet[r] {
			continue
		}
		if exceptAbs[filepath.Clean(r.File)] {
			continue
		}
		violations = append(violations, r)
	}
	return violations
}

// filterWithin returns every entry in refs whose File lies within (or is
// exactly) within — both compared as absolute, filepath.Clean'd paths.
// within is resolved against baseDir first when it isn't already absolute,
// mirroring assertNoCallersCommand's own --except path-resolution rule.
// This is the mitigation for gopls' documented interface-method reference
// conflation (see refs/definition/assert-no-callers' own Long help text):
// a caller who already knows the query is scoped to one package can use
// this to discard every cross-package hit gopls conservatively included.
func filterWithin(refs []traceengine.Reference, within, baseDir string) []traceengine.Reference {
	w := within
	if !filepath.IsAbs(w) {
		w = filepath.Join(baseDir, w)
	}
	// baseDir itself may still be relative here (e.g. --target-dir "."
	// passed through verbatim, never resolved to absolute elsewhere in this
	// file) — filepath.Abs resolves whatever remains against the process's
	// actual working directory, the same convention parseQuery's own
	// "file:line:col" path resolution already uses. Reference.File (what
	// every entry in refs is compared against) is always absolute, so w
	// must be too, or every comparison below silently fails.
	if abs, err := filepath.Abs(w); err == nil {
		w = abs
	}
	w = filepath.Clean(w)

	var filtered []traceengine.Reference
	for _, r := range refs {
		if isWithinDir(w, filepath.Clean(r.File)) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// isWithinDir reports whether target is dir itself or lies somewhere inside
// it, computed via filepath.Rel rather than a plain string-prefix check —
// a prefix check would wrongly accept a sibling directory whose name merely
// starts with dir's name (e.g. dir "internal/builder" matching target
// "internal/builderengine/poll.go").
func isWithinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// symbolMatchFields converts each traceengine.SymbolMatch into the
// {name,kind,file,line,character} map shape the JSON envelope emits,
// mirroring referenceFields's exact shape/style for Symbol's richer result
// type.
func symbolMatchFields(matches []traceengine.SymbolMatch) []map[string]any {
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
// *traceengine.ErrAmbiguousSymbol emits {"candidates": [...]} with exit 2
// (found, but the caller must disambiguate — distinct from both success and
// failure); every other non-nil error falls through to output.Err's plain
// error-string envelope and hardcoded exit 1, which already serves as the
// design's "not found" contract value (ErrSymbolNotFound included) with no
// special-casing needed. resultsField is the caller-supplied JSON key
// ("references" for refs, "definitions" for definition) so this one helper
// serves both verbs, which differ only in that key name.
func emitLookupResult(ctx context.Context, out io.Writer, resultsField string, results []traceengine.Reference, err error) {
	if err != nil {
		var ambiguous *traceengine.ErrAmbiguousSymbol
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

// referenceFields converts each traceengine.Reference into the
// {file,line,character} map shape the JSON envelope emits.
func referenceFields(refs []traceengine.Reference) []map[string]any {
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
// traceengine.Query: an explicit "file:line:col" position when arg matches that
// shape (see parsePosition), otherwise a bare symbol name.
func parseQuery(arg string) (traceengine.Query, error) {
	pos, ok := parsePosition(arg)
	if !ok {
		return traceengine.Query{Symbol: arg}, nil
	}

	// traceengine.Query.Pos.File must be an absolute path — References turns
	// it into a file:// URI directly, with no further resolution — so a relative
	// "file:line:col" argument is resolved against the process cwd here, the one
	// point where the CLI, not the engine, owns path interpretation.
	absFile, err := filepath.Abs(pos.File)
	if err != nil {
		return traceengine.Query{}, fmt.Errorf("resolve absolute path for %s: %w", pos.File, err)
	}
	pos.File = absFile

	return traceengine.Query{Pos: &pos}, nil
}

// inFileQuery converts a "--in-file" positional argument into a
// traceengine.Query carrying InFile: name is always treated as a bare
// symbol name to search for exhaustively within inFilePath, never
// position-parsed — even when name happens to have a "file:line:col" shape —
// mirroring parseQuery's role for the flag-less form and symbolQuery's
// never-position-parsed discipline. This is a separate named function,
// rather than an inline literal at each of refsCommand/definitionCommand's
// two call sites, specifically so the "--in-file" query-construction
// contract is independently unit-testable without a live language server.
func inFileQuery(inFilePath, name string) (traceengine.Query, error) {
	// traceengine.InFileQuery.File must be an absolute path — References
	// turns it into a file:// URI directly, with no further resolution — so a
	// relative --in-file path is resolved against the process cwd here,
	// exactly like parseQuery resolves Pos.File: the CLI layer, not the
	// engine, owns path interpretation.
	absFile, err := filepath.Abs(inFilePath)
	if err != nil {
		return traceengine.Query{}, fmt.Errorf("resolve absolute path for %s: %w", inFilePath, err)
	}

	return traceengine.Query{InFile: &traceengine.InFileQuery{File: absFile, Name: name}}, nil
}

// parsePosition reports whether arg has the "file:line:col" shape — a path
// followed by two colon-separated positive integers — and if so returns the
// parsed traceengine.Position. It scans from the right (the last two colons)
// rather than splitting on every colon, so a Windows drive-letter path such as
// "C:\foo\bar.go:42:8" still parses correctly: only the trailing two segments are
// required to be integers, and everything before them is taken as File verbatim.
func parsePosition(arg string) (traceengine.Position, bool) {
	lastColon := strings.LastIndex(arg, ":")
	if lastColon < 0 {
		return traceengine.Position{}, false
	}
	col, err := strconv.Atoi(arg[lastColon+1:])
	if err != nil {
		return traceengine.Position{}, false
	}

	rest := arg[:lastColon]
	secondColon := strings.LastIndex(rest, ":")
	if secondColon < 0 {
		return traceengine.Position{}, false
	}
	line, err := strconv.Atoi(rest[secondColon+1:])
	if err != nil {
		return traceengine.Position{}, false
	}

	file := rest[:secondColon]
	if file == "" {
		return traceengine.Position{}, false
	}

	return traceengine.Position{File: file, Line: line, Character: col}, true
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
// under resultsField. An *traceengine.ErrAmbiguousSymbol is
// statusAmbiguous, carrying the candidate list — batch mode surfaces the
// same ambiguity emitLookupResult's exit-2 path already reports for
// single-arg mode, just per-entry instead of for the whole call.
// ErrSymbolNotFoundSentinel is statusNotFound with no extra fields — a
// confirmed absence needs nothing more reported. Every other error
// (ErrNoLanguage, ErrServerNotFound, ErrServerTimeout,
// ErrResolverUnsupported, a toolchain-install failure, ...) is statusError,
// since none of them mean "confirmed absent."
func classifyLookupError(err error, resultsField string, results []traceengine.Reference) (batchStatus, map[string]any) {
	if err == nil {
		// Mirror emitLookupResult's single-arg "resolution":"complete" marker
		// per batch entry, so a batch-mode caller gets the same trust signal
		// on each "found" result the single-arg envelope carries.
		return statusFound, map[string]any{resultsField: referenceFields(results), "resolution": "complete"}
	}

	var ambiguous *traceengine.ErrAmbiguousSymbol
	if errors.As(err, &ambiguous) {
		return statusAmbiguous, map[string]any{"candidates": ambiguous.Candidates}
	}

	if errors.Is(err, traceengine.ErrSymbolNotFoundSentinel) {
		return statusNotFound, nil
	}

	return statusError, map[string]any{"error": err.Error()}
}

// classifySymbolError is classifyLookupError's twin for Symbol's batch-mode
// closure (card 40). It shares the same nil/not-found/else structure but has
// no ambiguous branch at all — per the plan's symbol-semantics decision,
// Symbol never collapses multiple candidates into ambiguity, so
// classifySymbolError has no case that could ever produce statusAmbiguous.
func classifySymbolError(err error, results []traceengine.SymbolMatch) (batchStatus, map[string]any) {
	if err == nil {
		return statusFound, map[string]any{"symbols": symbolMatchFields(results)}
	}

	if errors.Is(err, traceengine.ErrSymbolNotFoundSentinel) {
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

// RunCLI is the public seam for the trace module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the
// capture writer for all output (including cobra's error text), matching every
// other module's RunCLI seam.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
