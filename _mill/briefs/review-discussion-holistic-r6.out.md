MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class (best effort; brief names "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:consistency] Argument-less `lyx batten run` both auto-seeds and refuses
**Section:** seed-verb-plus-auto-seed-on-the-batten-entry-path / Q&A ("Does deleting `table.entry.Args`…")
**Issue:** The batten-path order says `run`/`step` **auto-seed when absent**, while the Q&A says argument-less `lyx batten run` "becomes a `self` address that lands on the run-id listing refusal"; under the stated order it would instead auto-seed `_lyx/shed/self/seed.json` in prime with an empty `params.slug`, and `IsReserved` is scoped to run-ids "derived from a Board slug" so it would not fire.
**Fix:** State whether the batten path refuses a `self` (or empty-slug) address outright before the auto-seed gate, and reconcile the two passages to one answer.

### [BLOCKING:design] `Seed-Child`'s commit has no push disposition
**Section:** seed-child-writes-and-commits
**Issue:** The row is specified as write + commit through `fabricengine` with no push, yet its rationale is the machine-switch case and its rejected alternative is "child seed lost on a fresh clone of the pair" — neither of which a local commit reaches; the sibling `CommitStatus` seam explicitly pushes (`internal/loomcli/wiring.go:61`, `fabricengine.PushAnchored`).
**Fix:** Decide and state whether `Seed-Child` pushes the child's pair after committing, and if not, say what carries the seed to another machine.

### [BLOCKING:design] Seed-presence behaviour unspecified on the `lyx loom` path
**Section:** shedcli-resolves-the-location… / no-migration-legacy-layouts-refuse-loudly / Technical context (`internal/loomcli/arm.go`)
**Issue:** The seed read is assigned to `shedcli`'s pre-run and (round 5) to `battencli`'s `arm`, but `loomcli.Arm` (`arm.go:118`) knows its recipe without a seed and nothing is specified to read one there; the no-migration decision nonetheless asserts the new verbs "find no run and refuse with the run-id listing", which on the loom path only `loom start` would ever make true.
**Fix:** State which code performs the seed-presence check on the `lyx loom` path, whether `lyx loom run|step` auto-seed when absent, and what `lyx loom status|pause` does with a status-but-no-seed worktree.

### [NIT:consistency] "`--recipe` removed from the subtree" vs `lyx shed seed --recipe`
**Section:** Scope (In) / run-id-positional-replaces-recipe-flag
**Issue:** `--recipe` is today a *persistent* flag on the `shed` parent (`internal/shedcli/cli.go:97`), so "removed from the `lyx shed` subtree" reads as contradicting the new `lyx shed seed --recipe <name>`.
**Fix:** Say the persistent flag is deleted and `--recipe` survives only as a local flag on `seed`.

### [NIT:scope] Third sense of "seed" left off the collision list
**Section:** Technical context — "Naming collision to be careful about"
**Issue:** The paragraph covers `loomengine.CheckSeed` vs `seed.json` but not `shedverbs.KindUnseeded`, whose doc reads "the status file could not be seeded" (`internal/shedverbs/step.go:26`) — a closed-vocabulary value that becomes misleading once `lyx shed seed` exists.
**Fix:** Add it to the collision list with the disposition (doc-comment clarification only; the constant's value stays).

## Verdict

REQUEST_CHANGES
Three unresolved behaviours: batten's self-address, the child seed's push, and loom's seed check.
MILL_REVIEW_END
