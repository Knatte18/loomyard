// refs.go implements References, the public orchestration entry point that
// ties detection (detect.go), the language-server registry (registry.go),
// and the generalized LSP client (lspclient.go) together: given a target
// directory and a query (a symbol name or an explicit file:line:col
// position), it launches the right language server, resolves the query to
// a position if needed, and returns the reference list. This is the only
// file in the package that imports no other codeintelengine file's
// internals beyond what's already exported at package scope — it is the
// external interface the CLI layer (internal/codeintelcli, batch 3) calls.

package codeintelengine

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Reference is one result of a References call: the file and the 1-based
// line/character position within it where the queried symbol is
// referenced. Character is a UTF-16 code-unit offset converted back to
// 1-based for display, matching Position's convention.
type Reference struct {
	File      string
	Line      int
	Character int
}

// Query is exactly one of two forms: Symbol (a name to resolve via
// workspace/symbol) or Pos (an explicit file:line:col position, bypassing
// name resolution entirely). Callers must set exactly one; References does
// not validate that both are unset or both are set, since Pos != nil is
// itself the discriminant it checks. Pos.File, when set, must be an
// absolute path — References turns it into a file:// URI directly, the
// same way it derives rootURI from TargetDir.
type Query struct {
	Symbol string
	Pos    *Position
}

// Options configures one References call: Registry supplies the language
// servers to choose from, TargetDir is the project root to detect the
// language in and root the launched server at, Lang optionally overrides
// detection, Query selects the symbol or position to look up, and Timeout
// bounds every individual LSP request (initialize, the resolver call, and
// references) — not the call as a whole, so a slow-but-eventually-fed
// server only fails the specific phase that stalls.
type Options struct {
	Registry  Registry
	TargetDir string
	Lang      string
	Query     Query
	Timeout   time.Duration
}

// References resolves opts.Query against the language server for
// opts.TargetDir and returns every reference to it, sorted by
// file:line:character.
//
// The steps: (1) detect the language and its registry Entry; (2) launch
// the language server named by the entry's Command; (3) initialize it
// rooted at TargetDir; (4) resolve the query to an LSP position —
// Query.Pos converted directly if set, otherwise a workspace/symbol lookup
// for Query.Symbol; (5) issue textDocument/references at that position;
// (6) map and sort the results. Every LSP phase is bounded by a fresh
// context.WithTimeout(ctx, opts.Timeout) deadline; a phase that times out
// returns ErrServerTimeout and tears the server down with kill()
// (hard-kill, since a server that is already unresponsive could re-block
// on the graceful shutdown handshake) rather than close().
func References(ctx context.Context, opts Options) ([]Reference, error) {
	lang, entry, err := DetectLanguage(opts.TargetDir, opts.Registry, opts.Lang)
	if err != nil {
		return nil, err
	}

	client, err := newLSPClient(entry.Command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ErrServerNotFound{Language: lang, InstallHint: entry.InstallHint}
		}
		return nil, fmt.Errorf("codeintelengine: start language server for %q: %w", lang, err)
	}

	timedOut := false
	defer func() {
		if timedOut {
			client.kill()
		} else {
			client.close()
		}
	}()

	absTargetDir, err := filepath.Abs(opts.TargetDir)
	if err != nil {
		return nil, fmt.Errorf("codeintelengine: resolve absolute path for %s: %w", opts.TargetDir, err)
	}
	rootURI := "file://" + absTargetDir

	initCtx, initCancel := context.WithTimeout(ctx, opts.Timeout)
	defer initCancel()
	if err := client.initialize(initCtx, rootURI); err != nil {
		if errors.Is(err, ErrServerTimeoutSentinel) {
			timedOut = true
		}
		return nil, err
	}

	fileURI, lspPos, err := resolvePosition(ctx, client, opts, lang, entry)
	if err != nil {
		if errors.Is(err, ErrServerTimeoutSentinel) {
			timedOut = true
		}
		return nil, err
	}

	refsCtx, refsCancel := context.WithTimeout(ctx, opts.Timeout)
	defer refsCancel()
	locations, err := client.references(refsCtx, fileURI, lspPos)
	if err != nil {
		if errors.Is(err, ErrServerTimeoutSentinel) {
			timedOut = true
		}
		return nil, err
	}

	return toSortedReferences(locations), nil
}

// resolvePosition returns the file:// URI and LSP wire position
// textDocument/references should query. When opts.Query.Pos is set, it is
// converted directly via toLSPPosition. Otherwise resolvePosition resolves
// opts.Query.Symbol via workspace/symbol: zero candidates is
// ErrSymbolNotFound, more than one is ErrAmbiguousSymbol (each candidate
// formatted as file:line:col), and exactly one candidate's own location is
// used as-is — its Range.Start is already the 0-based-line/UTF-16-character
// LSP position the wire format needs, so no round trip through the
// byte-column Position type happens on this path (that round trip would
// misconvert the offset on any line with a multi-byte rune before the
// symbol, exactly the hazard toLSPPosition exists to avoid on the
// Query.Pos path). A server that does not advertise
// workspaceSymbolProvider yields ErrResolverUnsupported rather than
// attempting the call.
func resolvePosition(ctx context.Context, client *lspClient, opts Options, lang string, entry Entry) (fileURI string, pos lspPosition, err error) {
	if opts.Query.Pos != nil {
		lspPos, err := toLSPPosition(*opts.Query.Pos)
		if err != nil {
			return "", lspPosition{}, fmt.Errorf("codeintelengine: convert position %+v: %w", *opts.Query.Pos, err)
		}
		return "file://" + opts.Query.Pos.File, lspPos, nil
	}

	if !client.supportsWorkspaceSymbol() {
		return "", lspPosition{}, &ErrResolverUnsupported{Language: lang, Server: entry.Command[0]}
	}

	symbolCtx, symbolCancel := context.WithTimeout(ctx, opts.Timeout)
	defer symbolCancel()
	candidates, err := client.workspaceSymbol(symbolCtx, opts.Query.Symbol)
	if err != nil {
		return "", lspPosition{}, err
	}

	switch len(candidates) {
	case 0:
		return "", lspPosition{}, &ErrSymbolNotFound{Symbol: opts.Query.Symbol, TargetDir: opts.TargetDir}
	case 1:
		loc := candidates[0].Location
		return loc.URI, loc.Range.Start, nil
	default:
		formatted := make([]string, len(candidates))
		for i, c := range candidates {
			formatted[i] = formatLocation(c.Location)
		}
		return "", lspPosition{}, &ErrAmbiguousSymbol{Symbol: opts.Query.Symbol, Candidates: formatted}
	}
}

// toSortedReferences maps raw LSP locations to the public Reference type
// (file:// URIs trimmed back to plain paths, 0-based positions promoted to
// 1-based for display) and sorts them by file, then line, then character —
// a stable, portable display order independent of whatever order the
// server returned results in.
func toSortedReferences(locations []lspLocation) []Reference {
	refs := make([]Reference, len(locations))
	for i, loc := range locations {
		refs[i] = Reference{
			File:      trimFileURI(loc.URI),
			Line:      loc.Range.Start.Line + 1,
			Character: loc.Range.Start.Character + 1,
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		if refs[i].Line != refs[j].Line {
			return refs[i].Line < refs[j].Line
		}
		return refs[i].Character < refs[j].Character
	})
	return refs
}

// trimFileURI strips the "file://" scheme from an LSP document URI, the
// same conversion formatLocation (position.go) applies.
func trimFileURI(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}
