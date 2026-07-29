MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetmax
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] native strategy: post-call subprocess teardown + probe-failure recovery unspecified
**Section:** `ensure-server-call-site` / `native-strategy-wire-compatibility`
**Issue:** `ensure-server-call-site` says the caller must "NOT close the daemon-owned connection," but `native-strategy-wire-compatibility` says `native` respawns a fresh local `gopls -remote=auto` client subprocess on every call — that local subprocess isn't the daemon, so it's unclear whether/how lyx tears it down (calling today's `close()` sends `shutdown`/`exit` over the wire, which for `native` risks reaching the *shared* daemon other clients depend on). Separately, the design doc's step-3 probe exists precisely because a live PID can still be a "hung shared instance" — but no Decision states what `EnsureServer` does when the probe fails while PID/version look healthy, and for `native` lyx never owns the shared daemon's PID at all (confirmed empirically: only the short-lived local client's PID is ever lyx's own child), so it has no safe kill/restart path `supervised` has.
**Fix:** Add a Decision (or amend `ensure-server-call-site`) stating exactly what happens to the native strategy's per-call local subprocess after use, and what `EnsureServer` returns/does on a probe failure for each strategy (especially `native`, where killing the shared daemon isn't an option).

### [GAP] Batch mode has no status for an engine-level failure mid-batch
**Section:** `batch-mode-cli`
**Issue:** Per-symbol status is a closed set (`found`/`not_found`/`ambiguous`, `found`/`not_found` for `symbol`), and the envelope is "always" `output.Ok`. But `References`/`Definition` can fail a specific symbol with `ErrServerTimeout`, `ErrResolverUnsupported`, or a toolchain-install failure — none of which is "not found." Today's single-query CLI maps any such error to a whole-command `output.Err`; batch mode's contract doesn't say whether one symbol's infra failure aborts the whole batch (contradicting "always `output.Ok`" and the 0/1/2 worst-outcome ranking, which has no slot for it) or gets silently folded into `not_found`.
**Fix:** Define a 4th per-symbol outcome (e.g. `"error"`) with its own exit-code rank, or explicitly state that any non-not-found/ambiguous engine error aborts the whole batch call with `output.Err`.

### [GAP] `symbol` verb's stated implementation contradicts `batch-mode-cli`'s "never ambiguous" requirement
**Section:** Scope (`symbol` bullet) / `batch-mode-cli`
**Issue:** Scope describes `symbol` as "exposes the existing internal `workspace/symbol` resolver as its own verb." The only existing resolver is `resolvePosition` (`refs.go`), confirmed by reading the source to return `ErrAmbiguousSymbol` on >1 candidate and use the first hit only on exactly 1 — but `batch-mode-cli` requires `symbol` to *never* report ambiguous and instead return every candidate as its normal success shape (`"symbols": [...]`). `resolvePosition` cannot produce that shape as-is; unlike `definition`, `symbol` has no dedicated `### Decision:` describing its actual engine entry point.
**Fix:** Add a `symbol-semantics` Decision naming the new (or adapted) engine function `symbol` actually calls — distinct from `resolvePosition` — that returns the raw candidate list without ambiguity-collapsing.

### [GAP] docs/overview.md's module-table entry not in the same-commit doc list
**Section:** Constraints (Documentation Lifecycle)
**Issue:** The Documentation Lifecycle bullet names `codeintelengine/doc.go`, `codeintelcli/cli.go`, and `CONSTRAINTS.md`'s leaf-invariant entry for same-commit updates, but omits `docs/overview.md`'s codeintel module-table line (confirmed on disk: it currently says "v1 scope: references-only, no call hierarchy, no in-process Go arm," which this task falsifies). CLAUDE.md's own Task Completion rule requires updating `docs/overview.md` "if the module table or execution stack changes," and overview.md's own doc-lifecycle section names itself as owning "the module and shared-lib map."
**Fix:** Add `docs/overview.md`'s codeintel bullet to the same-commit doc-update list.

### [NOTE] manifest/roadmap.md Done/Planned status unreconciled against precedent
**Section:** Technical context (External design references)
**Issue:** The discussion asserts "do not edit [roadmap.md] as part of this task," but the Planned `codeintel` bullet's own title ("V1 Go-only, built for multi-language") and body appear to be fully satisfied by this task's Scope, and the repo has a precedent (`loom: contracts, Preflight, Discussion producer` in Done, alongside a still-open `loom` Planned item) for adding a Done sub-entry when a named slice of a larger Planned item ships.
**Fix:** Explicitly decide (with rationale) whether landing V1 warrants a new Done entry per that precedent, rather than asserting no-edit by default.

### [NOTE] CONSTRAINTS.md's Sandbox Suite Coverage allowlist is already stale, untouched by this discussion
**Section:** Constraints (not addressed anywhere in the discussion)
**Issue:** `cmd/lyx/sandbox_coverage_test.go`'s live `excludedModules` map already contains a `codeintel` entry (external-binary reason), but CONSTRAINTS.md's Sandbox Suite Coverage Allowlist bullet only documents `ide`/`selfreport` — pre-existing drift, not caused by this task, but the discussion never mentions this invariant even though it adds 2 CLI verbs (mechanically fine since exclusion is module-level).
**Fix:** Since CONSTRAINTS.md is already being touched for the Leaf Invariant amendment, fold in the missing `codeintel` allowlist line in the same commit.

## Verdict

GAPS_FOUND
Four unresolved technical/process gaps (native lifecycle, batch error taxonomy, symbol-verb semantics, doc coverage) plus two housekeeping notes.
MILL_REVIEW_END
