MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: APPROVE
reviewer_model: sonnet
reviewer_self_id: Anthropic Claude, Sonnet-class (session reports model ID claude-sonnet-5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Scope of this pass

Round 1's four findings (existing-worktree break, test-seeding method, seventh doc site, two non-committal dispositions) are all resolved in the current text — verified against the live decisions, not just re-read: `reconcile-required-for-pre-registry-wefts`, the `Testing` section's plain-filesystem seeding mechanism, the seven-site `doc-amendments` list, `verbs-test-select-helper`, and `persistentprerun-ordering` all state a concrete disposition with rationale. No new design gap found. Every load-bearing claim (call sites, invariant text, config-module pattern, `configreg`/`configreg_test.go` alphabetical order, `lyxtest.SeedConfig`'s signature, the sandbox hub's per-run `clone` step) was checked directly against the source, not taken on the document's word.

This pass is a citation-accuracy sweep, since the document explicitly promises one ("Re-derive by grep rather than trusting the manifest's numbers; the same applies to every line reference in this file") and several of its own fresh citations don't clear that bar.

## Findings

### [NIT:citation] `runlevel.go:332` should be `:327`, repeated four times
**Section:** Scope → In (line 32); Decisions → runlevel-call-site (rationale + rejected); Decisions → runlevel-call-site (Note on manifest fidelity); Technical context → "The two live call sites"
**Issue:** `batcher.Select(deps.Config.Batcher)` is at `internal/websterengine/runlevel.go:327` today, not `:332` (verified via `grep -n`). `:332` is the manifest's number (`shed-followups.md:584`), carried forward unchanged into all four of this document's own citations of the line — including the "Technical context" section, which presents itself as freshly re-derived, not a manifest quote. This is the single most load-bearing call site in the task (the one every alternative in `runlevel-call-site` argues about) and the document's own "Line-number drift to expect" paragraph names two other drifted citations it caught (`overview.md`, `websterengine/doc.go`) but not this one, so it reads as verified when it wasn't.
**Fix:** `:332` → `:327` in all four sites.

### [NIT:citation] "16 `TestRun_*` tests" should be "15", repeated three times
**Section:** Decisions → runlevel-call-site (rationale); Testing → scenarios; Q&A log
**Issue:** `grep -c "^func TestRun_" internal/websterengine/runlevel_test.go` returns 15, not 16. This count is invoked as the "concrete evidence" that reversed the runlevel-call-site decision — the document's own framing leans on its precision ("this is new, concrete evidence that changes the calculation, not a one-off fixture cost"). The qualitative conclusion is unaffected by the off-by-one (a call inside `Run` still breaks every one of them), but a document that argues from a verified count should carry the actually-verified one, especially given this repo's own precedent for exact counts in override records (`shed-followups.md`'s "A verified count, not a remembered one" note on `plan-format-v3.md` occurrences).
**Fix:** "16" → "15" in all three sites.

### [NIT:citation] Two supporting quotes in `entry-point-name` are mis-cited
**Section:** Decisions → entry-point-name (rationale)
**Issue:** `registry.go:31`'s quoted "resolves the active batcher by name" is actually at `:27` (`:31` is a closing brace inside `Select`). `doc.go:12`'s quoted "The active batcher is chosen via …" starts at `:11`. Both off by a small, checkable amount; neither changes the (correct) argument that "active" is existing package vocabulary.
**Fix:** `registry.go:31` → `:27`; `doc.go:12` → `:11`.

### [NIT:citation] `websterengine/doc.go:23–25` is one line short
**Section:** Decisions → doc-amendments, site 5
**Issue:** The quoted phrase "selected once at config-load time via webster.yaml's `batcher:` key" spans `doc.go:24–26`, not `:23–25` — `:23` is the preceding sentence ("Master actually forks by internal/batcher: a name-keyed registry of"), which doesn't contain the quoted words. Smallest of the four citation issues; noted for completeness since this is one of the two sites this document claims to have freshly re-derived past the manifest's stale numbers.
**Fix:** `:23–25` → `:24–26`.

## Verdict

APPROVE.
The design is sound and every substantive round-1 gap is closed with a stated, reasoned disposition — nothing here blocks implementation, since none of the four findings change a decision or an acceptance criterion, and every mis-cited line is one `grep` away from the real one. Fix the four citation drifts (all mechanical) in the same write, per standing instruction to resolve NITs without a round-trip.
MILL_REVIEW_END
