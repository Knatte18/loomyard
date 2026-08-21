MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Anthropic); environment reports model id claude-opus-5
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [BLOCKING:scope] shedengine's ProducerDef file in no Context list
**Location:** batch 1 card 3; batch 2 cards 6, 7; batch 3 card 12
**Issue:** `shedengine.ProducerDef` is declared in `internal/shedengine/producer.go`, which appears in no `Context:` in the plan (only `shedengine/shed.go` is ever listed), yet those cards' `Requirements:` name `ProducerDef`, its `Name`/`OnDone`/`OnStuck`/`Segment`/`MaxBounces`/`Producer` fields, and `shedengine.ShedProducer`; card 6 lists no `internal/shedengine` file at all while prescribing `shed.Producers[0].Producer = …` and a six-field per-row assertion, and names `shedcheck.Check` without `internal/shedcheck/check.go`.
**Fix:** add `internal/shedengine/producer.go` to cards 3, 6, 7, 12, and `internal/shedengine/shed.go` + `internal/shedcheck/check.go` to card 6.

### [BLOCKING:scope] Card 23 names shedbuild symbols with no shedbuild file in Context
**Location:** batch 6 card 23
**Issue:** the new Recipe-Format Sole-Parser Invariant text must assert the `shedbuild.Recipe` model returned by `Parse`/`Load`, that `internal/shedbuild` declares no on-disk location "which its own package doc already asserts", and that it reaches the registry only through `shedrecipe.Lookup`/`Names` — but `Context:` lists five `*_test.go` files and nothing else, so `internal/shedbuild/doc.go`, `parse.go`, `recipe.go` and `internal/shedrecipe/registry.go` are unreadable to the implementer.
**Fix:** add those four files to card 23's `Context:`.

### [BLOCKING:design] Split status-path duplication gets no guard or stated disposition
**Location:** batch 1 card 3; batch 4 card 15
**Issue:** today `loomshed.Deps` carries one `StatusPath`/`StatusLockPath` pair feeding both `NewLoomPreflight` and `shedengine.Shed`; the conversion makes them two independently-filled copies (`Env` and `ShedPaths`), and card 3 explicitly forbids `New` from validating anything, so a divergent fill silently yields a Shed persisting to one file while `Loom-Preflight` reads another. The plan documents the duplication but never states a disposition on detecting a mismatch; card 18's loomcli assertions pin only the one caller that exists today.
**Fix:** either have `loomrecipe.New` error when `env.StatusPath != paths.StatusPath` (same for the lock pair), with a case in `recipe_test.go`, or record in card 3 why an unguarded mismatch is accepted.

### [NIT:consistency] Card 5 cites the wrong card for the resume tests
**Location:** batch 2 card 5
**Issue:** `writeBatcherConfig` is described as "needed by the moved resume tests in card 8", but card 8 moves `sequence_test.go`; `resume_test.go` moves in card 9.
**Fix:** change the reference to card 9.

### [NIT:consistency] Batch 6's Batch Tests contradicts its own verify and card 26
**Location:** batch 6 (Batch Tests), card 26
**Issue:** Batch Tests says "the two `-run` names are combined in one regex rather than two invocations", but the batch `verify:` carries no `-run` at all; it also says "card 26's four edits are comment text only" while card 26 says "Five comment-only repairs" and lists five `Edits:` entries (and later says "sweep wider than these four").
**Fix:** drop the `-run` sentence and make the edit count five throughout.

### [NIT:consistency] Card 23's title says two edits, its body says three
**Location:** batch 6 card 23
**Issue:** the heading reads "The two `CONSTRAINTS.md` edits" while the Batch Scope and the card's own `Requirements:` both specify three (registry-guard repoint, told-geometry enumeration, new Recipe-Format Sole-Parser Invariant).
**Fix:** retitle the card to name three edits.

### [NIT:consistency] Shared Decision name says three packages, body names four
**Location:** overview, `### Decision: no-production-change-to-the-three-consumed-packages`
**Issue:** the decision body covers `internal/shedengine`, `internal/shedcheck`, `internal/shedbuild`, and `internal/shedrecipe` — four packages — and batch 6 cites the decision by its "three" name.
**Fix:** rename the decision (e.g. `no-production-change-to-the-consumed-packages`) and update the two citing references.

### [NIT:scope] A falsified smoke_test.go comment is outside every card's reach
**Location:** batch 6 card 26
**Issue:** `internal/loomcli/smoke_test.go`'s header says "loom's own producer table (`internal/loomshed`) backs five of its thirteen rows with stub producers"; the table becomes the recipe's, the file is in no `Edits:` list nor `## All Files Touched`, and card 26's sweep token is the trailing-slash form `internal/loomshed/`, which does not match `(internal/loomshed)` — so the stale claim is neither caught nor fixable without tripping the card's own "stop and report" rule.
**Fix:** add `internal/loomcli/smoke_test.go` to card 26's `Edits:` and to `## All Files Touched`, and add the slashless `internal/loomshed` spelling to the sweep tokens.

## Verdict

REQUEST_CHANGES
Three context/design gaps plus five self-contradictions; structure, DAG, and sequencing are otherwise sound.
MILL_REVIEW_END
