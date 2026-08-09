MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] "sole `batcher.Select` caller" is factually false
**Section:** Decision `batcher-extracts-standalone-now-absorbed-by-shed-later`; Technical context; F
**Issue:** `internal/webstercli/cli.go:160` holds a second production call — `batcher.Select(websterCfg.Batcher)`, wired into `PersistentPreRunE` as a fail-fast gate per `verbs_test.go:697` — so removing `websterengine.Config.Batcher` breaks `webstercli`, which F's inventory never names.
**Fix:** Correct the "only/sole live caller" claim and add `internal/webstercli` (cli.go:160, verbs_test.go:221–223, :697) to F's inventory with a decision on what the CLI gate resolves from post-split.

### [GAP] A's deletion inventory misses a build-breaking guard
**Section:** A — `builder-retire`; Testing
**Issue:** `internal/configcli/configcli_test.go:311,327–328,455` asserts the config menu prints `builder (default)`; dropping `builder` from `configreg` fails that test, and it is named neither in A's inventory nor in the "four guards" list.
**Fix:** Add `internal/configcli/configcli_test.go` to A's inventory and to the guard list in the Testing section.

### [GAP] `status-schema.md` and `loomengine.validPhases` have no owner
**Section:** follow-up-task-set
**Issue:** `internal/loomengine/coherence.go:14–22` pins `validPhases` to `preflight|discussion|plan|builder|raddle|finalize|done`, and `docs/reference/status-schema.md:45,62,92` pins the same enum plus builder-specific prose and a `builder-contract.md` link — live code and a pinned contract encoding the retired phase model, unclaimed by A–F.
**Fix:** Assign `status-schema.md` and the `validPhases`/status enum to a task (or state explicitly that the enum is deferred until `Shed` is built, and why).

### [GAP] `loom.md` has three owners, not two
**Section:** "`loom.md` has exactly two owners, in sequence"; B; E
**Issue:** B's acceptance grep (`plan-format-v3` and `plan-format v3` return zero hits) necessarily rewrites `loom.md:29` and table rows 5–7 (`:53–55`), so B is a third owner — and `:29` is explicitly assigned to E. B's mechanical rewrite also leaves `:29` self-contradicting ("today's pinned plan-format.md v2 ... is being replaced by plan-format v3 (plan-format.md)") while its grep criterion passes.
**Fix:** State B as a mechanical third owner of `loom.md`, and either give B the `:29` prose repair or note that B leaves a knowingly incoherent sentence for E.

### [GAP] `D` is not disjoint — `roadmap.md` is shared
**Section:** follow-up-task-set; D
**Issue:** D fixes `manifest/roadmap.md:68` while A and E both edit `roadmap.md`, yet D is justified as parallel on the claim it owns only `finalize.md`/`raddle.md`/`self-report.md`. This is the same shared-file collision that forced E to be serialized.
**Fix:** Either scope `roadmap.md:68` to A or E, or state why concurrent `roadmap.md` edits are acceptable where concurrent `overview.md` edits were not.

### [GAP] `shed.md`'s producer enumeration not updated for the new gate
**Section:** C (Discussion-Review-Gate); E
**Issue:** C inserts `Discussion-Review-Gate` into `loom.md`'s table, but `shed.md:13` enumerates `loom`'s producer list verbatim and `shed.md:41` lists the mechanical Go-function producers — both would silently disagree, and E's `shed.md` scope names only `:7`, `:18`, `:19`.
**Fix:** Add `shed.md:13` and `:41` to E's inventory (or to C's, with the ordering consequence stated).

### [GAP] F's migration behaviour left undecided
**Section:** F — `batcher-standalone-split`
**Issue:** "an existing worktree's `webster.yaml` `batcher:` value must be honoured or explicitly reported once — F's body decides which" defers a user-visible behavioural choice to an autonomous session with no conversation history, which is exactly what this discussion refuses elsewhere.
**Fix:** Decide honour-vs-report here and record it, with the rejected alternative.

### [NOTE] `overview.md` builder sites outside the module table unowned
**Section:** A (doc retirement)
**Issue:** A's scope is "docs/overview.md's module table", leaving `:92` (lists both `plan-format.md` and `plan-format-v3.md` as kept reference docs), `:227`, and `:375` unassigned.
**Fix:** Widen A's `overview.md` bullet to "all builder/plan-format references", not the module table alone.

### [NOTE] Rationale doc has no named path
**Section:** Scope (In)
**Issue:** "Commit a short summary/rationale doc in this worktree" names no filename or directory, so placement is left to interpretation.
**Fix:** Name the path.

## Verdict

GAPS_FOUND
Ownership holes and one false source claim would break follow-up tasks A, B, D and F.
MILL_REVIEW_END
