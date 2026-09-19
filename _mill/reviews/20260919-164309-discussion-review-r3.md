MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
duration_s: 157.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-1 (best-effort; presented to me as "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] No-op skip is in-process, so step mode still pushes
**Section:** `run-shed-re-entrancy-via-self-pointing-on-stuck`; `prime-status-commit-uses-the-loom-shaped-seam-not-bolt`
**Issue:** The skip "holds the last `(producer, state)` pair it committed in its own closure" — in-memory — but each `lyx shed step` / `lyx batten step` is a fresh process, so the closure starts empty and the step's single persist always commits and pushes; the "~4 commits per whole run" claim and the "contention is rare rather than per-30-seconds" serialisation argument hold for `run` mode only, and step mode is precisely the mode the re-entrancy decision exists to enable. `loomcli`'s seam pushes unconditionally after commit (`wiring.go:145-152`), so even a no-change `CommitAnchoredPaths` is followed by a network push.
**Fix:** State the skip's scope explicitly and decide the step-mode case — either an on-disk/last-committed-pair comparison, or accept one commit+push per step with the cost written out.

### [BLOCKING:design] Who resolves the Location for the pre-run seed read
**Section:** Technical context, `internal/shedcli`
**Issue:** The stated sequence ("resolve cwd → resolve run-id → `shedrun.ReadSeed` → `lookup` → verb gate → `Arm`") requires a `*lyxcwd.Location` before arming, but `resolvePersistentPreRun` today holds only a cwd string and deliberately never resolves one (`cli.go:159`, "each module's Arm owns its own resolution" — `lifecyclecli/arm.go:33`). Consequence left unstated: the seed read and its run-id-listing refusal now fire *before* each module's own refusals, so `lyx shed <verb> <slug>` from a task worktree reads that worktree's anchor and refuses with a run-id listing instead of `battencli`'s non-prime refusal.
**Fix:** Decide where `lyxcwd.Resolve` is called on the `lyx shed` path and state the new refusal precedence against the non-prime and `lyxcwd.Resolve` refusals.

### [BLOCKING:design] Batten auto-seed fires after arming, and only on `run`
**Section:** `seed-verb-plus-auto-seed-on-the-batten-entry-path`
**Issue:** `lifecyclePreRun` is a `shedverbs.Spec` hook invoked inside run's `RunE` (`shedverbs/run.go:30`), i.e. after `arm`→`wire` has already built the Env — so anything reading the seed at wire time (the `Seed-Child` driver, taken "from batten's own seed params") sees no seed on a first `lyx batten run <slug>`. It is also `run`-only: `status`/`pause`/`step` never auto-seed, which is unstated.
**Fix:** State that every seed-param read is lazy inside its closure body (as the path reads already are), or move the auto-seed ahead of `wire`; and state the disposition for the non-`run` verbs.

### [NIT:consistency] `poll_attempts: 8640` is a Go default, not a recipe key
**Section:** `run-shed-re-entrancy…`, budget derivation
**Issue:** `contracts/recipes/lifecycle-recipe.yaml` carries no `config:` block at all; 8640/5s are `defaultInnerRunPollAttempts`/`defaultInnerRunPollIntervalS` in `shedrecipe/entries_lifecycle.go:21-24`. Also `max_bounces` is a `shedbuild` producer-row field (`recipe.go:30`), not a `config` key, so "the `poll_attempts` config key is retired in favour of that budget" conflates two different surfaces.
**Fix:** Say that the recipe row gains an explicit `config: poll_interval_s: 30` plus a row-level `max_bounces: 1440`, and that the two default constants and `configRejectUnknown`'s allowlist move with the retired key.

### [NIT:design] `self` unreserved in prime's shared run namespace
**Section:** `one-package-for-addressing-and-seed`; `both-status-files-move`
**Issue:** Prime's `_lyx/shed/` holds one directory per batten slug and would also hold `self` if prime ever addresses its own run; the run-id vocabulary validates segment shape but never reserves the literal `self` against a Board slug.
**Fix:** State whether `self` is a reserved run-id and where that refusal lives.

## Verdict

REQUEST_CHANGES
Commit-skip scope, pre-run resolution ownership, and auto-seed ordering each need a stated decision.
MILL_REVIEW_END
