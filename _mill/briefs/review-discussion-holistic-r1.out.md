MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:consistency] Batchifier row keeps `plan.md`, criterion 3 forbids it
**Section:** Decision `loom-table-row-insertion-and-renumbering` / Testing item 3
**Issue:** `loom.md:56`'s `Batchifier` Input cell reads ``| 8 | `Batchifier` | mechanical | `plan.md` (approved) + `webster.yaml`'s `batcher:` key |``, and the decision pins that rows 9–12 get "their number cell renumbered and **nothing else** — no cell content is touched", yet Testing item 3 demands "No surviving `discussion.md` or `plan.md` artifact references in `loom.md`'s producer table" over the whole table range.
**Fix:** Either extend the artifact-name fix to `Batchifier`'s Input cell (and say so in the pinned table), or narrow criterion 3 to rows 2–8 and record explicitly that `Batchifier`'s stale `plan.md` is handed to task E.

### [BLOCKING:scope] `Discussion-Review-Gate` occurrence set is wrong and unreachable
**Section:** Testing item 2 / Scope "Out"
**Issue:** Item 2 requires "Zero occurrences of `Discussion-Review-Gate`" while simultaneously saying `shed-followups.md:281,304` "will still carry it" — self-contradictory; and the enumeration is incomplete: the token occurs at `shed-followups.md:265,281,283,301,304,325,329,342,424` and at `roadmap.md:47`, none of which (except :304) are in the stated `Plan-Review-Gate`-token-only sweep. Item 1's grep is likewise unsatisfiable, since `_mill/discussion.md` and `_mill/status.md` themselves carry both tokens.
**Fix:** Decide and state one disposition for `Discussion-Review-Gate` in spec/roadmap prose (rewrite, leave as historical spec wording, or hand to E), scope the greps to the four edited files, and drop the contradictory "zero occurrences" phrasing.

### [BLOCKING:decision] No disposition for inserting the new producer into `shed.md:13,41`
**Section:** Scope "Out" / Decision `rename-sweep-crosses-task-ownership-boundaries`
**Issue:** `shed-followups.md:423–424` states `shed.md:13`'s producer enumeration and `:41`'s mechanical-producer list "must gain `Discussion-Review-Gate` once task C inserts it into `loom.md`'s table, or the two docs silently disagree"; the discussion touches both lines for the rename but never says whether C also inserts `Discussion-Validate` there or leaves it to E — and leaving it is exactly the interim self-contradiction the same decision rejects for the rename.
**Fix:** State explicitly whether C adds `Discussion-Validate` to `shed.md:13` and `:41`, with the same override rationale used for the rename sweep, or record a named hand-off to E.

### [NIT:consistency] Stale line references in the "already verified" facts
**Section:** Technical context, "Key source facts already verified"
**Issue:** The Planner's `NN-<card>.md` / `00-overview.md` statement is at `loom.md:186`, not `:188` (`:188` is the execution-stack row), and the webster naming rewrite sits at `:91–92` and `:185`, not `:187` — a list offered so the plan writer need not re-derive should not carry off-by-N refs, given the file's own `:15`→`:16` note.
**Fix:** Re-verify and correct the `loom.md` line numbers in that list.

### [NIT:consistency] `Plan-Review-Gate` count stated as 7, actual is 8
**Section:** Scope "In" / Decision `rename-gate-producers-to-validate` blast radius
**Issue:** Scope says "all 7 occurrences"; the enumerated sites are 8 lines with one occurrence each (`loom.md:54,75`; `shed.md:13,41`; `roadmap.md:45,46`; `shed-followups.md:304,306`), and the reconciliation note ("8 line-hits across 7 distinct sites") does not resolve the arithmetic.
**Fix:** State 8 occurrences across 8 lines in 4 files, or drop the count in favour of the zero-hit grep criterion.

### [NIT:consistency] Insertion widens `:75`'s second open question, unacknowledged
**Section:** Decision `repair-loom-75-open-question`
**Issue:** `:75`'s second clause enumerates `Preflight`/`Finalize` as the thin-Output (pass/fail, no artifact) producers; this task adds `Discussion-Validate` with the identical property on the same line it edits, while declaring the clause untouched.
**Fix:** Note in the hand-off to E that the thin-Output question now covers `Plan-Validate`/`Discussion-Validate` too, or say the enumeration is deliberately left as-is.

## Verdict

REQUEST_CHANGES
Three contradictions between pinned decisions and acceptance criteria must be resolved first.
MILL_REVIEW_END
