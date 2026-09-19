MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
duration_s: 175.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Run-Shed re-entry says nothing about deps.Spawn
**Section:** `### run-shed-re-entrancy-via-self-pointing-on-stuck`
**Issue:** `innerRunProducer.Call` calls `deps.Spawn(ctx)` unconditionally on every invocation (`internal/lifecycleshed/innerrun.go:83`), and the wired Spawn runs `exec.Command(exe, "loom", "start", "--no-attach")` (`internal/lifecyclecli/wire.go:119`); the decision lists only "re-resolve worktree, one status read" per entry, so with up to 1440 re-entries the spawn's idempotency is undefined, and the existing spawn-error→`Stuck` arm is absent from the new three-disposition table — a `Stuck` that now self-bounces with no sleep.
**Fix:** State whether Spawn fires on first entry only (and what makes that decidable from durable state) or every entry, and give spawn failure an explicit disposition in the same table.

### [BLOCKING:design] `lyx shed seed` cannot survive the subtree's own pre-run
**Section:** `### seed-verb-plus-auto-seed-on-the-batten-entry-path`
**Issue:** `shedcli.resolvePersistentPreRun` short-circuits only on `cmd.Name() == "shed"`, so it fires for every subcommand — under the new scheme it would resolve a run-id, `ReadSeed`, gate the verb against the entry's `Verbs` set, and `Arm`, all before `seed` runs; `seed` is by definition invoked when no seed exists and belongs to no recipe's verb set.
**Fix:** Decide where `seed`'s body lives (shedcli-only vs a shedverbs verb) and how it is exempted from the seed-read/verb-gate/arming pre-run.

### [BLOCKING:design] Prime's status commit onto weft:main left as a question
**Section:** `## Technical context`, "The prime worktree is a worktree"
**Issue:** The discussion explicitly defers ("Check whether the ordinary `CommitStatus` seam shape is sufficient from prime before assuming it is"), while the Fabric Git Invariant's Board carve-out routes every `weft:main` write through `Bolt`; `newCommitStatusSeam`'s commit is a path-scoped `CommitAnchoredPaths` + `PushAnchored` with no serialisation against concurrent `boardengine` writes to the same weft branch.
**Fix:** Decide now whether batten's CommitStatus uses the loom-shaped seam or Bolt, and state how it serialises against Board writes to `weft:main`.

### [NIT:scope] Enumeration method misses non-Go artefacts naming the old path/flag
**Demoted-from:** BLOCKING
**Section:** `## Scope` (doc list) and `## Technical context` (`grep for LoomStatusRel callers`)
**Issue:** Grepping Go callers sees no prose: `contracts/specs/loom-status-spec.md` pins `_lyx/loom/status.json` as a deployed normative spec (hash-pinned by `stencilstore`, never force-overwritten), `contracts/stencils` pins the literal "`_lyx/loom/`" (`discussiontemplate_test.go:32`), `contracts/recipes/loom-recipe.yaml:293` names it in prompt text, `plugins/ly/skills/ly-drive/SKILL.md:22,96` instructs on `--recipe lifecycle`, and `tools/sandbox/SANDBOX-CORE-SUITE.md:245` hand-writes the old path.
**Fix:** Name a literal-based enumeration (`_lyx/loom`, `.lyx/lifecycle`, `--recipe`, `lyx lifecycle`) across `contracts/`, `plugins/`, `tools/`, and state the disposition of already-deployed spec copies whose hash will now mismatch.

### [NIT:consistency] Step mode still blocks a full poll interval
**Demoted-from:** BLOCKING
**Section:** `### run-shed-re-entrancy...` vs `## Testing` (integration bullet)
**Issue:** The still-running arm sleeps `poll_interval_s` inside `Call`, so each `lyx shed step`/`lyx batten step` blocks 30s; the testing section claims the step variant "returns control rather than blocking", and `wait_mode` was rejected precisely because the row cannot know which verb drove it.
**Fix:** Either accept and state the 30s-per-step block (and fix the testing bullet's wording), or decide a mechanism by which the sleep is skipped under `Step`.

### [NIT:decision] `argsFor()` disposition unresolved
**Section:** `## Technical context`, `internal/shedcli`
**Issue:** "needs rethinking" is the only statement about the per-entry `Args` closure once the positional is a universal run-id; whether `table.Entry.Args` survives, is ignored, or is deleted is undecided.
**Fix:** State the disposition of `Entry.Args` explicitly.

### [NIT:design] Absent-status disposition under the unified layout
**Section:** `### both-status-files-move`
**Issue:** `shedverbs.AbsentDisposition` today differs per module (loom refuses; lifecycle reports `found:false`); with one layout and a seed-driven arming the case "seed present, `status.json` absent" has no stated answer.
**Fix:** Say which disposition the unified layout carries, or that it stays per-recipe told.

## Verdict

REQUEST_CHANGES
Spawn re-entry, seed-verb pre-run, prime commit seam, and artefact enumeration all undecided.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
