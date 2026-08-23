# Batch: loomshed thin wrap

```yaml
task: 'loom: self-checkable mechanical gates'
batch: 'loomshed thin wrap'
number: 2
cards: 2
verify: go test ./internal/loomshed/...
depends-on: [1]
```

## Batch Scope

This batch rewrites `loomshed.discussionValidate.Call` as a thin wrap over `discussionparser.Validate`, leaving the producer's outward `Done` / `Stuck` / returned-error contract byte-identical, and narrows `internal/loomshed/discussionvalidate_test.go` to the producer-level mapping and cancellation cases that remain this package's own.
It is one batch because all three edited files live in `internal/loomshed` and must move together: the allowlist entry is what keeps `seam_enforcement_test.go` passing once the new import appears, and the test narrowing is what keeps the package compiling once `requiredDiscussionSections` leaves it.

Card order matters and is deliberate: the test file is narrowed **first**, while the old implementation is still in place, because `internal/loomshed/discussionvalidate_test.go:112` currently ranges over `requiredDiscussionSections` and would stop compiling the moment that variable moved.
Narrowing first keeps every intermediate commit green.

The next batches consume nothing new from this batch — batch 3's verbs call `discussionparser` and `planparser` directly.
What batch 4 consumes is the *behaviour* pinned here: `NewDiscussionValidate` over the shared implementation, which is one half of the discussion parity test.

## Cards

### Card 3: narrow discussionvalidate_test.go to the producer's own mapping

- **Context:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/discussionparser/validate_test.go`
- **Edits:**
  - `internal/loomshed/discussionvalidate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Narrow `TestDiscussionValidate_Call` to the producer-level outcome mapping and the existing cancellation discipline, dropping every case whose subject is the check itself — those cases now live in `internal/discussionparser/validate_test.go` (batch 1).

  Remove the `EachRequiredSectionMissing`, `NotesForPlanWriterAbsentStillPasses`, `SectionsOutOfOrderStillPasses`, and `ExtraUnexpectedH2StillPasses` subtests.
  `EachRequiredSectionMissing` is the one case that ranges over `requiredDiscussionSections`, and removing it is what lets card 4 move that variable out of the package.
  Keep the `validDecisionRecord` constant and the `writeDiscussionFixture` helper: both remain in use, and `validDecisionRecord` stays a hardcoded seven-heading literal rather than becoming an export of the new package — per the discussion, no caller outside `internal/discussionparser` needs the list itself once the iteration case has moved.

  Retain and keep asserting: `BothFilesPresentAllSections` (zero findings maps to `shedengine.Done` with the decision record path as the pointer);
  `DecisionRecordMissing` and `SupportLogMissing` (a finding maps to `shedengine.Stuck`, and additionally assert the returned `OutputPointer` is the zero value, which today's cases do not check);
  and `CancelledContextReturnsErrorNotVerdict`.

  Add two new subtests, both of which pass against the current implementation as well as the rewritten one:

  - `HeadingMissingMapsToStuck` — a decision record built from `validDecisionRecord` with one required heading line removed, asserting `shedengine.Stuck`, an empty `OutputPointer`, and a nil error.
    This preserves heading-driven `Stuck` coverage at the producer level without ranging over the moved list.
  - `DecisionRecordUnreadableReturnsErrorNotStuck` — the decision record path created as a directory (via `os.MkdirAll`) while the support log exists, asserting a non-nil returned error and an outcome that is neither `shedengine.Done` nor `shedengine.Stuck`.
    This is the case the `short-circuit-order-is-load-bearing` Shared Decision exists to protect;
    without it, nothing in this package would notice an accumulating rewrite flipping it to `Stuck`.

  Add a cancellation case covering the `nonDoneExit` path taken on a finding, not only the entry path: a cancelled context together with a missing support log must still produce a non-nil error and no verdict.
  Update the file's header comment, if one is added, and every retained subtest's intent so it reads as the producer's own mapping suite rather than the check's.
  Do not touch `internal/loomshed/planvalidate_test.go`.
- **Commit:** `test(loomshed): narrow discussionValidate's suite to producer-level outcome mapping`

### Card 4: rewrite discussionValidate.Call over discussionparser

- **Context:**
  - `internal/discussionparser/validate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/discussionvalidate_test.go`
- **Edits:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomshed/discussionvalidate.go`, delete `requiredDiscussionSections` and `hasAllSections` outright — no copy of either stays in this package — and rewrite `(*discussionValidate).Call`'s body as a thin wrap over `discussionparser.Validate(p.decisionRecordPath, p.supportLogPath)`, in the shape `internal/loomshed/planvalidate.go` already models for `planparser`.

  The rewritten `Call` keeps `entryErr(ctx, p.name)` as its first statement, unchanged.
  It then calls `discussionparser.Validate` exactly once and maps its two return values per the `producer-outward-contract-unchanged` Shared Decision: a non-nil error routes through `p.nonDoneExit(ctx, "", err)`;
  a non-empty findings slice routes through `p.nonDoneExit(ctx, shedengine.Stuck, nil)`;
  and the clean case returns `shedengine.Done` with `shedengine.OutputPointer{Path: p.decisionRecordPath}`.
  Findings are discarded here rather than placed on the pointer — the pointer stays empty on `Stuck`.
  Keep `nonDoneExit` as it is;
  do not change its signature or its `cancelErr` consultation.

  The `bufio` and `strings` imports become unused and must go;
  `errors` and `os` also become unused, since the not-exist discrimination now lives entirely inside `discussionparser.Validate`.
  Add the `github.com/Knatte18/loomyard/internal/discussionparser` import.

  Update the file's top-of-file comment and `Call`'s godoc so they describe a thin wrap: state that the two checks and the seven required headings are now `internal/discussionparser`'s, that this file owns only the outcome mapping, and preserve the existing explanation of why a read failure that is not a not-exist is a returned error rather than a `Stuck` (a `Stuck` would bounce back to `Discussion-Write`, which cannot fix an I/O fault).
  Point at `discussionparser` for the three documented non-checks rather than restating them, matching how `planvalidate.go` points at `planparser` rather than restating plan-format rules.
  Keep the `discussionValidate` struct, its three fields, `NewDiscussionValidate`'s signature and `shedengine.ShedProducer` return type, and the `var _ shedengine.ShedProducer = (*discussionValidate)(nil)` assertion exactly as they are.

  In `internal/loomshed/seam_enforcement_test.go`, add `"github.com/Knatte18/loomyard/internal/discussionparser": true` to `loomshedAllowedImports`.
  Place it next to the existing `planparser` entry, since the two are the same kind of dependency.
  Without this the Told-Geometry allowlist test fails on the new import.
- **Commit:** `refactor(loomshed): make discussionValidate a thin wrap over discussionparser.Validate`

## Batch Tests

`verify: go test ./internal/loomshed/...` covers `internal/loomshed/discussionvalidate_test.go` (the narrowed producer-mapping suite, including the two added cases and the extra cancellation case), `internal/loomshed/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`, which must accept the new `discussionparser` import and would fail without the allowlist entry), and `internal/loomshed/planvalidate_test.go`, which this batch leaves untouched but which shares the package and so must keep passing.
The scope is one package because every file this batch edits lives in it;
no other package's behaviour changes, and the producer's outward contract is deliberately identical before and after.
Both cards are tier 1 — every fixture is a `t.TempDir()` and no case spawns a process.
