MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model; exact release designation not self-verifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] Deep links into the moved section: "both" is three too few
**Section:** §follow-up-task-set A, *Doc retirement*; §webster-sibling-section-moves-to-websterengine
**Issue:** A says "Re-point **both** deep links" (`finalize.md:36`, `:50`), but `builder-contract.md#webster-the-fork-based-sibling` is also deep-linked from `docs/overview.md:268` and `docs/reference/plan-format-v3.md:343` — the latter is B's renamed file, so the dangling link outlives A.
**Fix:** Replace "both" with the verified four-site list and assign `plan-format-v3.md:343` explicitly (A or B), since B's grep acceptance check (`plan-format-v3` hits) cannot catch it.

### [GAP] A's sandbox inventory misses SANDBOX-CORE-SUITE.md S9
**Section:** §follow-up-task-set A, `tools/sandbox` bullet
**Issue:** A enumerates only `SANDBOX-BUILDER-SUITE.md`, `suite.go`, `main.go`, but `tools/sandbox/SANDBOX-CORE-SUITE.md:224–232` is scenario S9 "Builder plan validate/status" with `**Covers:** builder` at `:229`; `sandbox_coverage_test.go`'s Assert 2 drift guard hard-fails on a `**Covers:**` token naming an unregistered module.
**Fix:** Add S9's removal (scenario body plus its Covers tag) to A's inventory.

### [GAP] loom.md's builder residue has no owner
**Section:** §follow-up-task-set (A, C, E scopes)
**Issue:** A's inventory does not list `loom.md`; C owns table rows 2–7; E owns `:15–17` and `:75`. That leaves `loom.md:29` (links the v2 `plan-format.md` A deletes), `:91–94` (the naming note calling `internal/builderengine`/`buildercli` "a real, separate, already-shipped sibling"), and `:187`'s module-decomposition row (same claim plus a `builder-contract.md` link) — all false or dangling after A.
**Fix:** Assign these three sites to a named task (E, given it already owns `loom.md` last) and state their line anchors in its body.

### [GAP] loom.md table row 8 pins the config key F moves
**Section:** §follow-up-task-set C and F
**Issue:** `loom.md:56` (row 8, `Batchifier`) names "`webster.yaml`'s `batcher:` key" and quotes `internal/batcher/doc.go`'s "never an LLM's decision". C is scoped to rows 2–7; F's file list excludes `loom.md`; and F `depends_on: B` only, so F may run concurrently with C/E, which own `loom.md`.
**Fix:** Either extend C's scope to row 8 or add `loom.md` to F and re-wire F's `depends_on` to include C/E; state which.

### [GAP] F leaves the batcher.yaml → websterengine wiring undecided
**Section:** §batcher-extracts-standalone-now-absorbed-by-shed-later; §follow-up-task-set F
**Issue:** F removes `webster.yaml`'s `batcher:` key but retains `websterengine.Config.Batcher` (`config.go:30–34`, yaml-tagged; `runlevel.go:332` is the sole reader) without saying how the field is populated from `batcher.yaml`. Having webster read another module's config is the same cross-module coupling the decision rejected for `loom.yaml`.
**Fix:** Decide and record the resolution path (webster reads `batcher.yaml` directly / batcher exposes a resolver webster calls / the field is set by the caller), and note that `config_test.go:125`'s `Batcher == "identity"` assertion moves with it.

### [GAP] F's sandbox-coverage obligation rests on a false premise
**Section:** §follow-up-task-set F, last bullet; §Constraints (Sandbox Suite Coverage)
**Issue:** "registering a new module triggers that guard" is wrong — `cmd/lyx/sandbox_coverage_test.go:39–47` enumerates `newRoot().Commands()`, i.e. cobra registration, not `internal/configreg`. A configreg-only `batcher` module trips nothing, and adding a `**Covers:** batcher` tag would instead fail Assert 2.
**Fix:** Decide whether "standalone module" means a cobra `lyx batcher` subtree (then CLI/Cobra Invariant and coverage both apply) or configreg-only (then drop the sandbox bullet).

### [NOTE] CONSTRAINTS.md:205 attributed to the wrong invariant, twice
**Section:** §follow-up-task-set A (`CONSTRAINTS.md` bullet); §Constraints (Review Round Invariant bullet)
**Issue:** `CONSTRAINTS.md:205` is inside the **Fabric Git Invariant**'s Enforced-by block; the Review Round Invariant starts at `:209` and names no `rawgitmutation_test.go` clause.
**Fix:** Re-attribute both mentions to the Fabric Git Invariant so A edits the right section.

### [NOTE] finalize.md residue beyond D's enumerated lines
**Section:** §follow-up-task-set D
**Issue:** D names `finalize.md:3, 11, 26, 36, 50, 52`, but `:45–46` still calls Finalize "`Shed`'s literally-shared code ... both share this exact code" and `:48` asserts "`Shed` hasn't been extracted from it yet (see that doc's own naming note)" — false once E fixes `loom.md:15–17`, which D runs parallel to.
**Fix:** Add `:45–48` to D's list (and note `:9`'s Builder-escalation reference), or state that D re-reads the whole file rather than fixed lines.

### [NOTE] Two stale self-references not assigned
**Section:** §follow-up-task-set C; §Surfaced open questions
**Issue:** `discussion-format.md:1`'s title still reads "the `discussion.md` ↔ Plan contract" (C's file, not called out), and `roadmap.md:31`'s "A dedicated scoping task should run first ... not yet broken down into buildable units" goes stale the moment this task lands, with no owner.
**Fix:** Name both explicitly — the title under C, the roadmap sentence under E alongside its precondition edit.

## Verdict

GAPS_FOUND
Six unowned or mis-scoped residue sites; F's module shape and config wiring undecided.
MILL_REVIEW_END
