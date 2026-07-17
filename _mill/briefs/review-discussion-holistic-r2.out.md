MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Server hang / timeout failure mode unaddressed
**Section:** Decisions → server-provisioning; lsp-client-surface; Technical context
**Issue:** Every failure mode is a typed error for a *fast* fault (ErrNoLanguage, ErrServerNotFound, ErrSymbolNotFound, ErrAmbiguousSymbol), but a server that launches yet hangs on `initialize`/references (rust-analyzer indexing, csharp-ls loading a solution — #008 already saw multi-second warm-up) has no deadline/cancellation contract, so the CLI blocks forever.
**Fix:** Decide a per-call timeout/context policy (e.g. a `--timeout` flag and a typed `ErrServerTimeout`) so the engine's `refs` signature and failure contract are fixed before mill-plan.

### [NOTE] pylsp alt-server mechanism left undecided
**Section:** Decisions → language-server-registry (Note)
**Issue:** Whether the registry gains an alt-server field or the benchmark points the client at pylsp directly is explicitly deferred to mill-plan; harmless for the registry contract but an open TBD that touches the registry schema.
**Fix:** Fine to delegate; flag that if an alt-server field is chosen it must ride the same overlay/validation path (KnownFields, install-hint) as primary entries.

### [NOTE] workspace/symbol capability variance not stated
**Section:** Decisions → cli-verb (name-resolution contract)
**Issue:** The name→position resolver assumes every server implements `workspace/symbol`; a server that omits or under-populates that capability yields zero candidates, surfacing as ErrSymbolNotFound and masquerading as "symbol absent."
**Fix:** Note that a missing/unsupported `workspace/symbol` capability should map to a distinct signal (or a documented caveat) so a resolver gap is not conflated with a genuine no-match.

## Verdict
GAPS_FOUND
One failure-mode gap (server hang/timeout); resolver-capability and pylsp notes non-blocking.
MILL_REVIEW_END
