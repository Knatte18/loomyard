MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Anthropic Claude, Opus-class (session reports model ID claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Existing worktrees break until reconcile is run
**Section:** Decisions → no-migration-path + absent-config-is-a-hard-error
**Issue:** The two decisions combine into an unstated observable break: any worktree that has a `_lyx/config/` today has no `batcher.yaml`, so after this lands `lyx webster <verb>` aborts in `PersistentPreRunE` with `configengine.Load`'s "config file …/batcher.yaml not found" (`internal/configengine/config.go:60`) until `lyx config reconcile --apply` is run; the no-migration rationale covers only the orphan `batcher:` key, never the newly *required* file, and "no pre-split worktrees exist" is not reconciled with the live sandbox Hub that `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` drives.
**Fix:** State the required operator step (reconcile before the first post-split `lyx webster`) and record it as accepted, or say explicitly that no worktree with a materialized `_lyx/config/` exists.

### [BLOCKING:design] Test seeding method for `Active` is unspecified
**Section:** Testing, TDD candidates 1–2
**Issue:** "Seed a worktree with `_lyx/config/batcher.yaml`" does not say whether that is a plain-filesystem write or `lyxtest.SeedConfig`; `internal/batcher` has no `TestMain` today, so choosing the lyxtest path silently pulls the package under the Hermetic Git Test Environment Invariant (needs `TestMain` + `lyxtest.HermeticGitEnv()`) and adds a git spawn to an untagged Tier-1 package — neither invariant is named in `## Constraints`, though `configengine.FindBaseDir` needs only a plain `_lyx/` directory and `internal/websterengine/config_test.go:23` already documents a plain-filesystem stand-in for exactly this reason.
**Fix:** Name the seeding mechanism (plain `os.MkdirAll`/`os.WriteFile`, no git) and add Test Tier Purity + Hermetic Git to the Constraints section with that disposition.

### [NIT:scope] Seventh doc site missed: master-template.md
**Section:** Decisions → doc-amendments
**Issue:** `internal/websterengine/master-template.md:37` says batches are grouped "via the plan's configured batchifier" — an embedded agent prompt, wrong about the config owner both before and after this task, and absent from the six-site list.
**Fix:** Either add it as a seventh site or state why an already-inaccurate prompt line is left standing.

### [NIT:decision] `verbs_test.go:221–223` disposition is non-committal
**Section:** Scope → In (line 36) / Technical context (line 137)
**Issue:** "is revisited" and "needs a light re-read for accuracy" name no outcome; the plan writer cannot tell whether the comment is rewritten, the `batcher.Select("")` call is kept, or both.
**Fix:** State the disposition: keep the `Select("")` call, rewrite the comment to name `Active`-resolved `c.batcher`.

### [NIT:decision] PersistentPreRunE ordering left as an open option
**Section:** Technical context, "The two live call sites"
**Issue:** "It can also move earlier in `PersistentPreRunE`" offers an option without choosing; the position decides which module's not-found error a half-reconciled worktree sees first (`cli.go:137–163` currently orders shuttle → reed → webster → batcher).
**Fix:** Pick one — leaving it in place preserves today's error precedence and needs no test rework.

## Verdict

REQUEST_CHANGES
Two unstated consequences: the required reconcile step, and the test-seeding invariants.
MILL_REVIEW_END
