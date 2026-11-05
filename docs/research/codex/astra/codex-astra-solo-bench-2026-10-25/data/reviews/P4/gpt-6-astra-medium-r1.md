---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:52
    summary: Skip environment variables turn an unpublished owner write into a successful claim or adoption
    fix: In card 3's Intent, require SetOwners to complete the owner pull and push regardless of ordinary board-write skip settings, and report Recorded only after that succeeds. Replace the planned TestSetOwners skip-success row with rows proving these settings cannot return an unpublished owner as recorded. In card 8's existing claim-test instructions, use the fake Set seam for the isolated claim test instead of requiring a successful real owner write under BOARD_SKIP_GIT.
  - id: F2
    severity: BLOCKING
    class: scope
    location: _lyx/plan/03-board-owner-writes.md:47
    summary: The non-takeover guard protects task slugs but leaves the prime claim able to replace another owner
    fix: Extend card 3's Intent so a non-Takeover prime claim checks prime_owner inside the pull-first locked write and preserves any existing other owner, reporting it under Lost for prime. Only Takeover may replace an existing prime owner. Extend the already-planned TestSetOwners table with a stale local unowned prime whose pull discovers another machine's claim, asserting that claim remains and no takeover is committed.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/08-batten-owner-check.md:50
    summary: Claiming an unowned prime before reconcile prevents the promised offline legacy-run resume
    fix: In card 8's Intent, perform the resume reconcile before attempting a first-act prime claim. On an unreachable report, apply the stated local-owner warning/refusal rule without any prime or task owner write; on a reachable report, perform the required claims and re-reads before driving the run. Extend the existing TestRunCmd_ResumeDispositions instructions with the combined case of no prime owner, an unowned run-held task, and unreachable origin, asserting warning and resume with no setter call.
  - id: F4
    severity: MEDIUM
    class: design
    location: _lyx/plan/16-adopt-module.md:1
    summary: The adopt card combines independently reviewable public APIs from multiple packages
    fix: Use lyx loom insert-card to put separate prerequisite cards before adopt for fabricengine.DropLocal with its result type, destruction registration and TestDropLocal, for hubforge.CloneSecond, and for battencli.ResumeInWindow. Keep their inseparable supporting implementation and coverage with each surface, and retain the adopt CLI, private orchestration helpers and adoption tests together in the adopt card. Update numbering and references through the insertion command.
---

### [BLOCKING] Owner publication can be bypassed by ordinary board skip settings

**Location:** _lyx/plan/03-board-owner-writes.md:52

**Issue:** Card 3 explicitly treats locally written owners as recorded under both BOARD_SKIP_GIT and BOARD_SKIP_PUSH, despite performing no push. Cards 8 and 16 consume that successful setter result to create or resume runs. Thus, for example, an adoption with BOARD_SKIP_PUSH records this machine locally and resumes while origin still names the old owner. D3 requires an owner write's push before starting or resuming, and D5 step 3 requires publication before any adoption resume. Neither licenses this bypass. The existing ApplySkipEnv implementation reads ordinary process environment variables (internal/boardengine/skipenv.go), and Bolt.PushRecorded deliberately returns without pushing under either skip option (internal/fabricengine/bolt.go); these are not intrinsically test-only settings. The new owner setter needs to preserve the stronger decision-record contract.

### [BLOCKING] A first-act prime claim can silently take over an existing prime

**Location:** _lyx/plan/03-board-owner-writes.md:47

**Issue:** The setter unconditionally sets the prime owner when Prime is requested; its next sentence restricts only a slug owned elsewhere when Takeover is false. Cards 8 and 13 decide whether to claim from their local board before the setter's pull. A machine with stale, unowned local prime data can therefore request a first-act claim, pull a previously published claim by another machine, and overwrite it in a fresh commit. This does not require simultaneous drivers. Because the new commit is based on the freshly pulled owner, the origin-wins replay rule does not undo that overwrite. D2's first-claim behavior and D5's explicit prime adoption require preserving that owner unless adoption requested takeover.

### [MEDIUM] The offline resume exception is unreachable for an unowned legacy hub

**Location:** _lyx/plan/08-batten-owner-check.md:50

**Issue:** The card says reconcileResume claims the prime when none is recorded, but the claim uses the setter that refuses an unreachable origin. A legacy hub with neither a prime owner nor a task owner therefore fails at this initial claim before reaching the card's later unreachable-reconcile branch, which promises Warn and resume with no owner write for a run-held unowned task. D3 explicitly preserves that behavior. The planned separate tests for first-act prime claiming and offline unowned-task resume do not settle their intersection.

### [MEDIUM] Adoption bundles separate public surfaces into one commit

**Location:** _lyx/plan/16-adopt-module.md:1

**Issue:** Besides the new adopt CLI, this card creates fabricengine.DropLocal and its public result, hubforge.CloneSecond, and battencli.ResumeInWindow. DropLocal has its own destructive-operation contract, registrations and integration coverage; CloneSecond is shared fixture infrastructure; ResumeInWindow exposes batten's existing resume behavior. These are separate package surfaces, not private helpers inseparable from adopt's implementation. The rubric explicitly requires a public or cross-package surface to have its own card even when only one card consumes it. Their current placement makes all of those surfaces and the complete adoption workflow one review and rollback unit.

## Rejected

- Card-order concern: the owner records and effective reader are introduced in card 2, OwnerRequest and ParentSlug in card 3, fabric's reader type in card 5, its production builder in card 6, the owner label in card 7, the shed refusal kind in card 9, InnerRun's owner types in card 10, and PauseOwnedElsewhere in card 11 before card 13 consumes it. InstallOwnerPause is introduced and used on card 13 itself. Later wiring described by earlier cards does not itself require those earlier commits to reference a later symbol.
- The Takeover escape hatch is explicitly limited to adoption; the prime omission is F2, rather than a claim that the flag has no stated purpose or bound. Cards 9, 10 and 12 describe absent seams and production wiring, and cards 10–12 describe the Finalize carve-out's status and parent-work checks. Card 13 explicitly warns and continues on an unwired or failed pause, consistent with D4's non-refusing orch start.
- Refusal reachability: cards 8, 9, 10, 12 and 13 name owner verdicts, errors, pause states or spawn errors and tests that drive the corresponding paths. No demonstrated upstream rejection invalidates those inputs; F3 identifies the concrete ordering conflict for the combined offline legacy-hub input. Raw read errors are not separately flagged.
- Cards 15, 16 and 17 are licensed by D8, D5 and D9 respectively. The per-machine window rule includes same-machine detach intentionally. The second-clone fixture supports the required adoption scenario. Card 17's short PATTERN entry, verbatim background text and compensating wording cuts are expressly authorized by the Added after Discussion entry; the trailer does not by itself introduce additional scope.
- Test economy: card 7 extends TestCoverageOf and TestPause rather than adding per-symbol tests; card 14 extends TestStatus for the report seam. The owner behavior planned for InnerRun and the adopt workflow is new, so overlap in existing harnesses alone is not evidence of redundant behavior coverage. No static-prose assertion is prescribed.
- Re-sign/member concern: the inspected cards predominantly name members; card 5 expressly re-signs classifyEntries. The changes to existing receiver structs and dependency fields do not require re-sign arrows, and the reviewed Intents do not prescribe another demonstrated parameter/result/receiver change to an existing method. Mechanical target checks were not re-derived.

Review was read-only apart from this file. No build, tests, quarry commands, or plan validator were run under the round's read-files-only evidence rule. The proposed nonstructural fixes change Intent and already-planned test cases, not target lists or verify commands; F4 uses the explicitly exempt card-insertion route.
