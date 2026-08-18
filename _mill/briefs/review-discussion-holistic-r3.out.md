MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] CONSTRAINTS.md contended with T4, denied in same doc
**Section:** § Scope "In" (CONSTRAINTS.md bullet) vs § Constraints, last design-doc bullet
**Issue:** Scope adds two new `CONSTRAINTS.md` sections, while the Constraints section asserts "T5 is parallel-safe with T4 and must not touch files T4 touches; it owns `cmd/lyx/stencilseed.go` and `tools/deploy/main.go` for this wave" — but `manifest/designs/producers-standalone.md:337` lists `CONSTRAINTS.md` in T4's own **Files**, so the two wave-2 tasks are scheduled to edit the same file with no stated sequencing or conflict disposition; `internal/stencilstore/stencilstore.go` (`ModeFor`) is likewise added to T5's footprint outside that ownership statement and outside the design doc's T5 file list (line 388-393).
**Fix:** State the disposition explicitly — either that the two CONSTRAINTS.md edits are disjoint appended sections to be sequenced on conflict (T4's is the Pattern Leaf Invariant reword, T5's are two new sections), and record `internal/stencilstore/stencilstore.go` as a third T5-owned file verified uncontended with T4, or narrow scope.

### [NIT:consistency] "three entry points" heading and stale Q&A row
**Section:** § Decisions `preflight-exposes-three-entry-points`; § Q&A log line 346
**Issue:** The decision heading says three while its body exports four (`Check`, `CheckResolved`, `Wired`, `HubPresent`), and the Q&A row enumerating the entry points still lists only three — a superseded statement carrying no supersession marker, unlike the adjacent gate row which does carry one.
**Fix:** Rename the decision to `preflight-exposes-four-entry-points` and append the same `(superseded by the round-2 gap below)` parenthetical to that Q&A row.

### [NIT:design] Injected `goos` does not make the Windows path row testable
**Section:** § Decisions `standalonestate-is-pure-derivation-with-an-injectable-seam`, "Rationale on the injectable seam"
**Issue:** `filepath.Join`/`Clean` select their separator from the compile-time host, not from the injected `goos`, so on Linux the `goos=="windows"` row yields `/localappdata/lyx/<hash8>`; the seam makes the *branch selection* and the *case fold* testable everywhere, not the `%LOCALAPPDATA%\lyx\<hash8>` form the Testing section says the test asserts.
**Fix:** State that the Windows-row assertion is built with the same `filepath.Join` rather than a literal backslash string, and scope the "both rows testable everywhere" claim to branch selection plus case folding.

### [NIT:decision] Gate-split deviation has no durable record
**Section:** § Decisions `seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report`, "Note for the reviewer"
**Issue:** The split from the brief's single "identical tier-1-AND-tier-2 check" (design doc line 384) is recorded only in `_mill/discussion.md`, which is worktree-local and vanishes on merge; the parallel `StencilMode()` deviation, by contrast, is explicitly routed into `internal/buildinfo/doc.go`, and § Scope forbids editing the design doc here.
**Fix:** Route the same way — require `internal/preflight/doc.go` to state why `Wired` and `HubPresent` both exist and which one the seed gate uses.

### [NIT:design] hash8 collision behaviour unstated
**Section:** § Decisions `standalonestate-is-pure-derivation-with-an-injectable-seam`
**Issue:** Eight hex characters is 32 bits, so two distinct targets can derive the same `stateDir`, socket and session — silently the exact failure the normalisation rationale exists to prevent — and the enumerated failure modes cover only the three error returns.
**Fix:** One sentence accepting the truncation's collision probability as out of scope (inherited from the design doc's `hash8`), or naming what a colliding pair is expected to do.

## Verdict

REQUEST_CHANGES
One blocking file-ownership contradiction against T4; the rest are nits.
MILL_REVIEW_END
