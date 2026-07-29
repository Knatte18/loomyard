// refs.go implements References, the public orchestration entry point that
// ties detection (detect.go), the language-server registry (registry.go),
// and the generalized LSP client (lspclient.go) together: given a target
// directory and a query (a symbol name or an explicit file:line:col
// position), it launches the right language server, resolves the query to
// a position if needed, and returns the reference list. It also defines the
// shared lookup pipeline (acquireConnection, teardownConnection, lookup)
// that References wraps and that a later batch's Definition wraps too —
// both differ only in which single LSP call they make once a position is
// resolved. This is the external interface the CLI layer
// (internal/codeintelcli, batch 3) calls.

package codeintelengine

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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
// language in and root the launched server at, WorktreeRoot is the
// worktree root EnsureServer's supervised strategy would anchor its daemon
// singleton at; unused by every strategy actually reachable in V1 (native
// never reads it), but threaded through now so a future language selecting
// supervised needs no signature change. The CLI layer populates it from a
// resolved hubgeometry.Layout.WorktreeRoot when available, empty
// otherwise. Lang optionally overrides detection, Query selects the symbol
// or position to look up, and Timeout bounds every individual LSP request
// (initialize, the resolver call, and references) — not the call as a
// whole, so a slow-but-eventually-fed server only fails the specific phase
// that stalls.
type Options struct {
	Registry     Registry
	TargetDir    string
	WorktreeRoot string
	Lang         string
	Query        Query
	Timeout      time.Duration
}

// References resolves opts.Query against the language server for
// opts.TargetDir and returns every reference to it, sorted by
// file:line:character. It is a two-line wrapper over the shared lookup
// pipeline, passing client.references as the one LSP call the pipeline
// should make once a position is resolved — see lookup's own doc comment
// for the full step-by-step behavior.
func References(ctx context.Context, opts Options) ([]Reference, error) {
	return lookup(ctx, opts, func(ctx context.Context, client *lspClient, fileURI string, pos lspPosition) ([]lspLocation, error) {
		return client.references(ctx, fileURI, pos)
	})
}

// acquireConnection obtains a ready-to-use *lspClient for lang/entry,
// alongside the connKind teardownConnection needs to tear it down
// correctly. When entry.HasNativeDaemon is true, this delegates entirely to
// ensureServer, which resolves the toolchain, spawns or dials, and hands
// back an already-initialized connection. Otherwise it runs the legacy
// cold-spawn-per-call sequence every non-Go language still uses today:
// newLSPClient followed by a manual initialize, both owned and torn down by
// this call alone (ensureServer is never invoked for this branch).
func acquireConnection(ctx context.Context, lang string, entry Entry, opts Options) (*lspClient, connKind, error) {
	if entry.HasNativeDaemon {
		return ensureServer(ctx, lang, entry, opts.TargetDir, opts.WorktreeRoot, opts.Timeout)
	}

	client, err := newLSPClient(entry.Command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, connKindLegacy, &ErrServerNotFound{Language: lang, InstallHint: entry.InstallHint}
		}
		return nil, connKindLegacy, fmt.Errorf("codeintelengine: start language server for %q: %w", lang, err)
	}

	rootURI, err := rootURIFor(opts.TargetDir)
	if err != nil {
		client.kill()
		return nil, connKindLegacy, err
	}

	initCtx, initCancel := context.WithTimeout(ctx, opts.Timeout)
	defer initCancel()
	if err := client.initialize(initCtx, rootURI); err != nil {
		// Mirror the existing timedOut-branching teardown logic exactly,
		// just localized to this one failure instead of spanning the whole
		// call: a timed-out server could re-block on the graceful shutdown
		// handshake close() sends, so a stalled initialize is hard-killed
		// instead.
		if errors.Is(err, ErrServerTimeoutSentinel) {
			client.kill()
		} else {
			client.close()
		}
		return nil, connKindLegacy, err
	}

	return client, connKindLegacy, nil
}

// teardownConnection tears down client per the rule its connKind demands —
// the two EnsureServer strategies wrap fundamentally different process
// lifetimes and must not be torn down the same way (see the plan's connKind
// teardown Shared Decision).
func teardownConnection(client *lspClient, kind connKind, timedOut bool) {
	switch kind {
	case connKindSupervised:
		// A supervised connection is a dial into a daemon lyx spawned to
		// outlive this call. Never run the LSP shutdown handshake or kill
		// it — the daemon is meant to keep serving other callers, and this
		// process's exit reclaims the dialed socket's fd on its own.
		return
	default:
		// connKindNative and connKindLegacy share identical teardown: both
		// wrap a connection this call owns outright (native's disposable
		// proxy subprocess, or the legacy path's own directly-spawned
		// server), so hard-killing on a timeout and gracefully closing
		// otherwise is safe for both.
		if timedOut {
			client.kill()
		} else {
			client.close()
		}
	}
}

// lookup is the shared pipeline every public lookup entry point (References
// here; Definition, batch 8) runs: detect the language, acquire a
// connection, resolve the query to a position, issue exactly one LSP call
// at that position, and convert the results. lspCall is the one step that
// varies between callers — everything else is identical regardless of which
// LSP method is ultimately invoked.
//
// The steps: (1) detect the language and its registry Entry; (2) acquire a
// connection via acquireConnection; (3) resolve the query to an LSP
// position — Query.Pos converted directly if set, otherwise a
// workspace/symbol lookup for Query.Symbol; (4) issue lspCall at that
// position; (5) map and sort the results. Every LSP phase from step (3)
// onward is bounded by a fresh context.WithTimeout(ctx, opts.Timeout)
// deadline; a phase that times out returns ErrServerTimeout and tears the
// connection down via teardownConnection's timedOut branch rather than its
// normal-completion branch.
func lookup(ctx context.Context, opts Options, lspCall func(ctx context.Context, client *lspClient, fileURI string, pos lspPosition) ([]lspLocation, error)) ([]Reference, error) {
	lang, entry, err := DetectLanguage(opts.TargetDir, opts.Registry, opts.Lang)
	if err != nil {
		return nil, err
	}

	client, kind, err := acquireConnection(ctx, lang, entry, opts)
	if err != nil {
		// No deferred teardown needed here: acquireConnection already tears
		// down any partial connection itself on its own error path.
		return nil, err
	}

	// timedOut is captured by reference via the closure below, since
	// resolvePosition/lspCall may still set it to true after the defer is
	// registered.
	timedOut := false
	defer func() {
		teardownConnection(client, kind, timedOut)
	}()

	fileURI, lspPos, err := resolvePosition(ctx, client, opts, lang, entry)
	if err != nil {
		if errors.Is(err, ErrServerTimeoutSentinel) {
			timedOut = true
		}
		return nil, err
	}

	callCtx, callCancel := context.WithTimeout(ctx, opts.Timeout)
	defer callCancel()
	locations, err := lspCall(callCtx, client, fileURI, lspPos)
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
