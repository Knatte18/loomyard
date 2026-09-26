# Review: Loom persists done only after post-run friction reflection

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [NIT:decision] Step-driven runs would start filing public issues without the operator gate ly-drive deliberately keeps
**Demoted-from:** BLOCKING
**Section:** Decisions → "Mechanism: a terminal recipe row, not an engine hook" ("A step-driven run (`lyx loom step` / the `llm` driver) reaches the row like any other, which closes the no-reflection-under-step gap"); Problem, paragraph 4.
**Issue:** The discussion treats "step-driven runs never reflect" as a gap to close, but it was a deliberate design choice.
The reflection agent files public GitHub issues on its own: `contracts/stencils/friction/friction-template-reflection.md` Step 3 tells it to "invoke `lyx selfreport create` yourself".
`plugins/ly/skills/ly-drive/SKILL.md` § Self-report says "Nothing files automatically while this loop is driving `loom`", calls that "the trade this design makes: a live supervisor with an operator in the loop instead of a primitive filing public issues on its own", and allows `lyx selfreport create` only on explicit operator approval.
With a `Friction-Reflect` row, every `llm`-driven run would file issues autonomously, reversing that gate without saying so.
It also falsifies ly-drive's instruction to list `.lyx/loom/friction/` at stop because "`step` never spawns the reflection pass that `run` runs": `frictionengine.Reflect` archives the directory, so ly-drive would find it empty.
Neither `plugins/ly/skills/ly-drive/SKILL.md` nor the reversal appears in Scope or Decisions, and the CLAUDE.md docs-in-the-same-commit rule makes the skill text part of this change.
**Suggested fix:** Make an explicit decision, with rationale and a rejected alternative, between:
(a) the row runs reflection only for the `go` driver and returns `Done` with `StatusSkipped` under the `llm` driver, leaving the notes for ly-drive's operator-gated flow; loomcli already knows the seeded driver, so this fits the same closure;
or (b) reflection under step is intended, and ly-drive's Self-report section is rewritten in this task to say the row reflects and files, and to drop the list-the-directory instruction.
Keep the ly-drive edit minimal: task `shed-llm-driver` (#28, `manifest/designs/shed-llm-driver.md`) is removing all loom-specific friction knowledge from ly-drive, so this task should change only what its own behaviour change makes false.

### [NIT:decision] Re-invoke rationale overstates idempotence
**Section:** Decisions → "Row naming and interrupt policy" ("a re-run after an interrupt either finishes the job or no-ops").
**Issue:** The reflection agent files issues before it writes its report, and `Reflect` archives only after that.
An interrupt between a `lyx selfreport create` call and the archive leaves the notes in place, so a re-invoked row reflects again and can file duplicate public issues.
The same risk exists today on the `PostRun` path, so this does not change the policy choice, but the rationale should not claim a no-op.
**Suggested fix:** Reword the rationale to name the duplicate-filing window and state why `reinvoke` is still preferred over `handback`.

### [NIT:consistency] Out-of-scope item names a recipe that does not exist
**Section:** Scope → Out ("The Hardener recipe: it shares `Finalize` by reference but gets no reflection row.").
**Issue:** `contracts/recipes/` holds only `loom-recipe.yaml` and `batten-recipe.yaml`; Hardener is a Someday design (`manifest/designs/hardener.md`), and routing (`on_done`) is per-recipe yaml, so no shared `Finalize` routing can be affected.
**Suggested fix:** Drop the bullet, or reword it to say `Finalize`'s producer is shared through `internal/landingshed` while its routing lives in each recipe's own yaml.

## Verdict

APPROVE
The mechanism is sound and well grounded, but it silently makes step-driven runs file public issues without the operator gate ly-drive was designed around; that needs an explicit decision and a matching ly-drive doc change.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
