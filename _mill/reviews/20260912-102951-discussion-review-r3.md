MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill

```yaml
duration_s: 221.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Interrupted-step rule rests on a false double-spawn premise
**Section:** § Decisions → `interrupted-step-hands-back-and-is-never-auto-retried`
**Issue:** The rationale asserts a re-called `current_producer` would spawn "a **second** agent for a row whose first agent may still be running", but every spawning row already probes for a still-live run first — `shedadapters/singlellm.go` ("probes for a still-live matching run before archiving anything and only archives-then-spawns a fresh run when the probe finds nothing", `Shuttle.Attach` on the seam), the same probe on `shedadapters/bouncer.go` and `burler.go`, and `websterengine/recoverbatch.go`; `manifest/designs/loom.md:375` records the probe as the fix for exactly this hazard.
**Fix:** Re-derive this decision against the shipped attach probe — either keep hand-back for a different, true reason (the skill cannot tell a mid-persist death from a pre-call one) or allow continuation, and drop the orphaned-agent warning the skill would otherwise print.

### [NIT:consistency] Q&A log keeps the superseded 77-step ceiling
**Demoted-from:** BLOCKING
**Section:** § Q&A log ("Does the skill's loop need its own ceiling?") vs § Decisions → `skill-loop-has-a-hard-iteration-cap`
**Issue:** The Q&A answer says 40 is "deliberately under the ~77-step mechanical ceiling", while the Decisions block computes ~107 and explicitly names 77 as an earlier draft's error; verified against `contracts/recipes/loom-recipe.yaml` (six review rows at `max_bounces: 5`), `shedengine.defaultMaxBounces = 10`, and `loomcli/wiring.go:402` leaving `ShedPaths.MaxBounces` zero.
**Fix:** Update the Q&A entry to the ~107 figure so the artefact carries one ceiling number.

### [BLOCKING:design] Bootstrap-lock ownership of the extracted helpers unspecified
**Section:** § Technical context (`internal/loomcli/run.go`, last paragraph) and § Scope (in-bullet 3)
**Issue:** `run` acquires `loomengine.LoomBootstrapLock` at line 143 — *after* seed, ownership verify, and `CommitAnchoredPaths` (lines 101–133) — yet the discussion says `step` "takes the bootstrap lock across its own seed+strand block", so the same extracted code runs inside the lock for one verb and outside it for the other, and the discussion never says whether the helper or its caller acquires.
**Fix:** State explicitly that the helpers are lock-agnostic and that each verb wraps its own window, pinning `run`'s acquisition point as unchanged.

### [NIT:design] Error envelope carries no discriminator for the one-retry rule
**Section:** § Decisions → `crash-cleanup-is-one-retry-then-hand-back`
**Issue:** `output.Err` emits a bare message, so the skill cannot distinguish a producer hard error from a bootstrap refusal (`ErrShedBusy` against a live driver, `VerifySeedOwnership`) and will spend its one retry on refusals no retry can fix.
**Fix:** Say the retry applies to any `err` envelope and is accepted as one wasted invocation, or pin a machine-readable kind key on the envelope.

### [NIT:decision] `sandbox_coverage_test.go` conditional is already resolvable
**Section:** § Testing (`cmd/lyx`)
**Issue:** The discussion leaves "if `sandbox_coverage_test.go` enumerates verbs rather than modules" open; the file's own comment says coverage "is module-level … each entry excludes the whole module, not individual subcommands", so the answer is "no change".
**Fix:** Replace the conditional with the settled statement.

### [NIT:design] Skill's cwd precondition and `.scratch/` root unstated
**Section:** § Decisions → `step-invocation-is-backgrounded-not-a-blocking-foreground-call`
**Issue:** `lyx loom step` derives everything from cwd (Cwd Resolution Invariant), and `.scratch/ly-supervise/step-<n>.json` is a relative path, but the discussion never states that the supervising session's cwd must be the task worktree, nor whether the skill verifies it before looping.
**Fix:** State the cwd precondition and that `.scratch/` resolves under it, per `plugins/scribe/skills/conversation/SKILL.md`'s file-writing rule.

## Verdict

REQUEST_CHANGES
One false premise, one superseded number, one unpinned lock boundary.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
