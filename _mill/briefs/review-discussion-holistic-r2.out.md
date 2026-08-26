MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Marker duplicates state the verdict already carries
**Section:** `### clearing-trigger-is-a-durable-settled-marker`
**Issue:** At `Call` entry, `judged(n)` plus a parsed `verdictApproved` is already durable, already-written on-disk evidence that the previous `Call` settled `Done` — including in the crash window — so the "APPROVED verdict on disk" trigger has the same fire/non-fire set as the new marker with no write step and no failure mode; the rejected-alternatives list considers only an in-memory flag and a digest-keyed marker, never this one. The marker's own accepted failure posture (write failure logged and swallowed, calling it "cosmetic") silently restores the exact replay defect the task exists to fix, with only a `logger.Warn` as evidence.
**Fix:** State why a separate marker file is required given the verdict file already encodes the same fact, or adopt the verdict-based trigger and drop the marker (and with it the swallow-vs-hard-error question).

### [BLOCKING:scope] Stale-comment inventory is incomplete and unidentifiable
**Section:** `## Scope` (In: `contracts/recipes/loom-recipe.yaml` comment-only edits)
**Issue:** "the two comments that document the wrong-root defect as deferred" names no identifiable pair — only `loom-recipe.yaml:118-121` (Plan-Bouncer) documents deferral; `loom-recipe.yaml:201-203` (Webster-Bouncer) states "every entry resolves to an absolute path under `Env.WorktreeRoot`" as fact and becomes false, and deleting it would drop unrelated rationale. Two files carrying comments that become false are outside Scope entirely: `internal/loomcli/wiring.go:87-91` explicitly justifies BurlerGeometry's `WorktreePath()` fill as intentional and non-interchangeable with WebsterGeometry, and `internal/shedrecipe/recipe.go:37-41` documents `Env.WorktreeRoot` as "the root every worktree-relative Config path resolves against" and omits Bouncer from `Env.AnchorPath`'s reader list.
**Fix:** Replace the "two comments" phrasing with a stated enumeration method for rationale comments asserting the old root, and name reword-vs-delete per site.

### [BLOCKING:scope] burlercli help/error text asserts the superseded root
**Section:** `### burlercli-hub-mode-changes-too`
**Issue:** The decision changes observable `lyx burler` hub-mode behaviour but Scope lists `internal/burlercli` only under the regression sweep; `internal/burlercli/cli.go:101,107,124` and `internal/burlercli/wiring.go:67` all tell the operator "the worktree is already the target" / "the worktree itself is structurally the target", which stops being the resolution root once `AnchorRel` is not ".". The CLI/Cobra Invariant makes help accuracy on an observable-behaviour change a review obligation.
**Fix:** Give the burlercli `Long`/flag-usage strings and the `--target-dir` refusal message an explicit disposition (reword to the anchor, or state why they remain correct).

### [NIT:design] Crash-window cost understated
**Section:** `### clearing-trigger-is-a-durable-settled-marker` (rejected: digest-keyed marker)
**Issue:** The accepted cost is stated as "re-reviews an already-approved artifact from round 1", but today that window replays a settled APPROVED verdict and returns `Done` deterministically; after the change the same window runs a fresh non-deterministic judgement that may come back BLOCKING on an already-committed, already-approved artifact.
**Fix:** State the verdict-flip possibility as part of the accepted cost.

### [NIT:decision] Marker filename left as a style, not a value
**Section:** `## Technical context` — "Naming caution"
**Issue:** The marker is described only as "a `settled.md`-style name at the run-directory root"; the exact filename is a durable on-disk contract that `internal/shedadapters/doc.go`'s round-artifact convention will pin, and no value is chosen.
**Fix:** Pin the literal filename (moot if the verdict-based trigger above is adopted).

## Verdict

REQUEST_CHANGES
Clearing trigger duplicates existing durable state; stale-comment and burlercli help scope incomplete.
MILL_REVIEW_END
