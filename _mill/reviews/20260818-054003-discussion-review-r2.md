MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
duration_s: 188.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact release ID not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Wired gate is per-worktree, not per-hub
**Section:** `### seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report`
**Issue:** `fabricengine.Ready(l)` stats `WeftWorktree(l)` = `weftname.SiblingPath(hub, base(worktree))` (`fabric.go:115`), a *per-worktree pairing* probe, not "is there a hub here"; `<hub>/_board` is itself a second weft worktree with no `-weft` sibling (`clone.go` step 7), so `Wired` returns false there and stencil seeding — whose target is `<hub>/_board/_lyx/stencils` — is silently skipped in a real, healthy hub whenever cwd is the board (or any weft sibling, or a worktree whose pair was removed). The discussion frames the gate as closing only the fictional-hub hole and never states this real-hub narrowing.
**Fix:** State the disposition explicitly — either accept and document the board/unpaired-worktree skip as intended, or define `Wired` over a hub-level probe (e.g. `<hub>/_board/_lyx` existence via a `fabricengine` accessor, still inside `internal/preflight`), and add the board-cwd case to the `cmd/lyx` test list.

### [NIT:consistency] Two contradictory targets for the shared-libs doc edit
**Demoted-from:** BLOCKING
**Section:** `## Scope` (In, last bullet) vs `## Technical context` ("Docs to touch")
**Issue:** Scope says the three bullets go under `docs/shared-libs/README.md`'s `## Implementation-only libraries`, "Not `## Libraries`" with a stated rationale; Technical context says "`docs/shared-libs/README.md`'s `## Libraries` section". The file has both sections (`README.md:14` and `:23`), so a plan writer can act on either and one is wrong. Technical context also names `docs/overview.md`'s line-315 shared-infrastructure sentence, which Scope's docs bullet omits.
**Fix:** Delete the `## Libraries` wording from Technical context and add the overview.md:315 sentence to the Scope docs bullet, so one list is authoritative.

### [NIT:design] `derive` seam's env inputs: unset vs set-but-empty undefined
**Section:** `### standalonestate-is-pure-derivation-with-an-injectable-seam`
**Issue:** The three error cases are stated in terms of "unset", but the seam takes `localAppData`, `xdgStateHome`, `home` as plain strings, where unset and set-to-empty are indistinguishable; the discussion also never says what `Derive` passes for `home` on the Windows branch.
**Fix:** State that empty string means unset at the seam boundary, and that `Derive` resolves `os.UserHomeDir` only on the POSIX branch (or unconditionally, ignoring failure on Windows).

## Verdict

REQUEST_CHANGES
Seed gate's real-hub narrowing unstated; docs target contradicts itself.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
