MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (self-assessed; exact build unverifiable from inside the session)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] Task A's deletion inventory is build-breaking incomplete
**Section:** follow-up-task-set → A `builder-retire`
**Issue:** A names only `main.go:75/:107`, `helptree_test.go`, `notransients_test.go`, but `internal/configreg/configreg.go:10,:44` imports `builderengine` and registers its `ConfigTemplate` (with `configreg_test.go:17` asserting `"builder"` in the module list), `cmd/lyx/constructoranchoring_test.go:34,78-92,130-166` imports and asserts on it, `cmd/lyx/rawgitmutation_test.go:30,48` pins `internal/builderengine` paths, and `tools/sandbox` carries `SANDBOX-BUILDER-SUITE.md` plus a `builder-suite` case in `main.go:326` — deleting the packages fails to compile and trips the Sandbox Suite Coverage guard.
**Fix:** Enumerate configreg (+ its test), `constructoranchoring_test.go`, `rawgitmutation_test.go`, the sandbox suite file and `tools/sandbox/main.go`, plus the CONSTRAINTS.md entries that name `rawgitmutation_test.go`'s builder half, and state what happens to an existing worktree's now-orphaned `builder.yaml` under `lyx config reconcile`.

### [GAP] C and E collide on loom.md; A/B/E collide on overview.md
**Section:** follow-up-task-set (dependency table)
**Issue:** E's rationale claims "touches files disjoint from D", but E edits `loom.md:15-17` and resolves `loom.md:75` while C scoped-edits `loom.md`'s table rows and inserts `Discussion-Review-Gate` — and `loom.md:75` holds *both* open questions (Discussion pre-gate and thin-Output), one owned by C's decision and one by E's, with no dependency between them; `docs/overview.md` is likewise edited by A (module table), B (sweep) and E (`:272`) in parallel.
**Fix:** Either wire `depends_on` between the colliding tasks or partition loom.md/overview.md edits by explicit line-range ownership stated in each body.

### [GAP] Task F understates the batcher split's code surface
**Section:** batcher-config-key-moves-to-loom-yaml / Technical context
**Issue:** The claim that F touches "only" the package doc and the config-key location does not hold: `websterengine` is the live `batcher.Select` caller (`runlevel.go`, `beginbatch.go`, `render.go`, `recoverbatch.go`, `config.go:34`, plus `template.yaml`), so moving `batcher:` to `loom.yaml` forces webster to read another module's config or to be handed the batching — and `docs/overview.md:271` pins the key to `webster.yaml` in the module table, unnamed by F.
**Fix:** State in F who reads `batcher:` until `Shed` exists, whether `websterengine.Config.Batcher` is removed or retained, how `template.yaml`/`configreg`/`lyx config reconcile` migrate existing worktrees, and add `overview.md:271`.

### [GAP] Discussion-Review-Gate's third check is not runtime-mechanical
**Section:** discussion-review-gate-exists
**Issue:** Checks 1–2 (`discussion-format.md:80-82`) are filesystem/section checks, but check 3 — "the Plan producer's declared input set never names `support-log.md`" — is a property of the producer definition, not of a task's artifacts; there is nothing per-run for a gate producer to evaluate.
**Fix:** Decide whether check 3 is a build-time/test assertion over the producer definition or restate it as an artifact-observable condition, and say so in C's body.

### [GAP] Stale references A and B create are unowned
**Section:** A / B / D
**Issue:** `finalize.md:50` (Related) also deep-links `builder-contract.md#webster-the-fork-based-sibling` — only `:36` is named; `discussion-format.md:30` justifies plan-format's `approved:` by "`lyx builder run` can be invoked standalone", false once A lands; `discussion-format.md:3` and `:30` link `plan-format.md`, which A deletes and B re-creates.
**Fix:** Add `finalize.md:50` to A's re-point list and the two `discussion-format.md` sites to C (or A), and note the deliberate window where `plan-format.md` does not exist between A and B.

## NOTES

### [NOTE] Rename-sweep scale figures are overstated
**Section:** plan-format-v3-renamed-to-plan-format-mechanically
**Issue:** A repo grep for `plan-format-v3`/`plan_format_v3` returns 24 files excluding `_mill/discussion.md`, not 30; `README.md` has no match; `websterengine` has 1 file, not 4; `webstercli` 2, not 3.
**Fix:** Re-derive the file list in B's body, or drop the counts and keep only the zero-hit grep acceptance check.

### [NOTE] Webster's atomicity tension has no owning task
**Section:** Surfaced open questions #1
**Issue:** It is called "the single largest unresolved tension... should be decided before `Shed` is built", yet it is only recorded as text in E — no task, and no gate on the roadmap's `Shed` item, owns deciding it.
**Fix:** Name where the decision must land (e.g. a precondition noted on roadmap's `Shed` entry) rather than only recording it in E's output.

### [NOTE] finalize.md cites a non-existent invariant name
**Section:** D `raddle-finalize-fold-and-link-repair`
**Issue:** `finalize.md:26` cites "CONSTRAINTS.md's Weft Git Invariant"; the file's actual entry is "Fabric Git Invariant (warp + weft)". Not in D's repair list alongside the `fabric.md` links.
**Fix:** Add it to D's stale-reference list.

## Verdict

GAPS_FOUND
Task A and F inventories are incomplete; C/E file collisions and unowned stale references need resolving.
MILL_REVIEW_END
