MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Fable 5 (claude-fable-5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:consistency] Stale nine-state count survives round 5's tenth state
**Demoted-from:** BLOCKING
**Section:** Scope (In), package-file-layout suggested files, Q&A log **Issue:** The tranche-1-state-matrix decision says **Ten** states and the cell arithmetic uses 10 (8×10×2=160), but the Scope bullet still says "(9 states)", the suggested-files list says "`states.go` (the 9 states)", the Q&A entry says "nine total", and the verb-table rejected bullet says "uniform nine-state product ... ~18 vacuous cells". **Fix:** Update all four stale sites to ten states (and ~20 for the rejected-alternative count, or mark it as the pre-round-5 figure).

### [BLOCKING:design] RefusedBefore cannot pin the layer for Remove's dirty refusal
**Section:** refused-before-the-gate-vs-by-the-gate **Issue:** The gate's dirtiness reason at `destroy.go:564` is byte-identical to `Remove`'s pre-flight message (`remove.go:74`), so a `RefusedBefore(substring)` match over the full error string also matches a gate refusal (`...dirtiness check failed for <target>: worktree has uncommitted changes; use --force`) — the stated property "fails when a future refactor moves a check across that boundary in either direction" is false in the pre-flight→gate direction, and sabotage row 3's neuter-the-pre-flight proof can stay green on the refusal half (the manifest diff catching it via the junction sweep is incidental and vanishes if the moved check runs earlier). **Fix:** Spec `RefusedBefore` to additionally require the absence of `"check failed"` in the error string (mirroring the exclusion TDD item 4 already demands of `RefusedByGate`'s negative), and note it in the sabotage row-3 mechanism.

### [NIT:consistency] "Same checkout" claim is inaccurate for the dirtyWeftUntracked pair
**Section:** tranche-1-state-matrix (`dirtyWeftUntracked` bullet) **Issue:** `Checkout` probes the prime weft sibling (`WeftWorktree(l)`, `checkout.go:39`) while `Remove` probes the pair's weft (`WeftWorktreePath(l, slug)`, `remove.go:79`), and dirty-what-per-cell dirties each verb's own target — so the two cells dirty *different* checkouts, not "same state, same checkout". **Fix:** Reword to "same state kind, each planted at the verb's own weft target"; the scope-divergence argument itself stands.

## Verdict

REQUEST_CHANGES
Two blockers: a stale nine-state count, and RefusedBefore's layer-pinning failing on the byte-identical dirty message.
MILL_REVIEW_END
