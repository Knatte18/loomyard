MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] `Finalize` classified `simple` on a premise D falsified
**Section:** `producer-typology-carve-out` + `shed-md-is-authoritative-loom-md-points` (Kind column: "the other eight are `simple`")
**Issue:** Post-task-D, `Finalize` owns an internal multi-spawn process: `finalize.md:37–38` has it run raddle's **parallel leaf forks** + a serial `Overview.md` step inside one merge-lock critical section, and `finalize.md:9` spawns a fresh higher-capability LLM on merge conflict — which is the stated discriminator for `bespoke, multi-spawn` (own loop, own crash-recovery need), not `simple, single-agent-spawn`. `roadmap.md:58` classifies neither way, so this is a new classification the discussion adds, not carry-forward.
**Fix:** State an explicit disposition for `Finalize`'s `Kind` with rationale — either `bespoke` (and say what its internal crash-recovery obligation is, given the merge lock), or `simple` with an argument for why the fold's forks do not count.

### [NIT:consistency] "Finalize has no output artifact" vs. `loom.md:60`
**Section:** `resolve-thin-output-over-four-producers`
**Issue:** The decision says all four producers "genuinely have no output artifact", but `loom.md:60`'s Output cell reads "merge-back, PR" and `finalize.md` describes a `SyncWeft` commit plus optional PR — the thin-Output framing fits `Preflight`/`*-Validate` cleanly and `Finalize` only loosely.
**Fix:** Either restrict the "no artifact" wording to the three gate producers and state `Finalize`'s case separately, or say explicitly that its merge commit/PR is not a contract-level output artifact.

### [NIT:scope] `loom.md:29` verify-only obligation not carried
**Section:** Scope / Exact edit sites (`manifest/designs/loom.md`)
**Issue:** `shed-followups.md:446–449` leaves E a verify-only obligation on `loom.md:29` (task B's in-place rewrite); the discussion's loom.md residue list (`:15–17`, `:57`, `:76–83`) never names it, so a plan writer inheriting E's residue could miss it.
**Fix:** Add `:29` to the loom.md site list marked verify-only, as was done for `:76–77` and the gate section.

### [NIT:consistency] `shed.md:59–63` section disposition unstated
**Section:** Exact edit sites (`manifest/designs/shed.md`)
**Issue:** The site list says the whole `## Why this doc doesn't rewrite loom.md's full detail` section's "premise changed once C and this task ran" (echoing `shed-followups.md:492`), but Scope names only `:63` as residue — leaving whether the section survives, is rewritten, or is trimmed to a reader's judgment.
**Fix:** Add an explicit one-line disposition (e.g. "section stays; only `:63` is retired") the way `roadmap.md:51` and `loom.md:82` already get one.

## Verdict

REQUEST_CHANGES
`Finalize`'s simple/bespoke classification needs an explicit, argued disposition before plan writing.
MILL_REVIEW_END
