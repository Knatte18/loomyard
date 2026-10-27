---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:52
    summary: Owner writes can report success without the push required by D3 and D5
    fix: >-
      Change card 3's Intent so SetOwners always performs its required pull, commit,
      push and post-push read regardless of BOARD_SKIP_GIT or BOARD_SKIP_PUSH;
      those settings must not turn an owner write into a successful local-only
      claim. Change the already-planned TestSetOwners skip-environment case to
      assert that both settings still require origin publication, including an
      unreachable-origin failure. In card 8, replace the real-setter claim row
      relying on BOARD_SKIP_GIT with a recording fake Set that asserts Machine,
      Slugs and Claim arrive together; card 3's existing integration test owns
      the actual atomic commit and push coverage. These are Intent changes to
      tests already targeted by their cards.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/plan/16-adopt-module.md:16
    summary: Independently reviewable public APIs are bundled into their consumers' cards
    fix: >-
      Use lyx loom insert-card to give the cross-package public surfaces their
      own prerequisite cards, retaining each surface's supporting types,
      implementation, documentation and behavioral coverage together: ParentSlug
      before the current card 3; ParentHoldsTaskWork before its consumers in
      cards 11 and 12; PauseOwnedElsewhere before the current card 11;
      Fabric.LocalOnlyCommits before the current card 12; Engine.ClientAttached
      before the current card 15; and ResumeInWindow, DropLocal and CloneSecond
      before the current card 16. Keep DropLocalResult, TestDropLocal and the
      destruction-guard registrations with DropLocal. Give CloneSecond's card
      fixture-construction coverage while keeping the adoption integration
      scenario with the adopt CLI card. Keep adopt's private
      request, outcome, deps and helpers with its CLI surface. Update the Card
      Index and prose card references through the insertion workflow, keeping
      every extracted API before its first consumer. This is a card insertion
      and renumbering fix, exempt from the overlay procedure.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:53
    summary: The post-push Lost rule turns a skipped absent entry into an owner refusal
    fix: >-
      In card 3's Intent, distinguish skipped absent or done entries from
      eligible owner-write candidates. Return skipped entries in neither
      Recorded nor Lost, and apply the post-push owner mismatch check only to
      eligible candidates. Extend the already-targeted TestSetOwners table
      with absent-entry and done-entry rows asserting no claim, no owner write
      and no Lost result. Preserve card 8's absent-entry success behavior.
---

### [BLOCKING] Owner writes can succeed without publishing ownership

**Location:** `_lyx/plan/03-board-owner-writes.md:52`; related claim test at `_lyx/plan/08-batten-owner-check.md:61`.

**Issue:** Card 3 explicitly makes `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` bypass the owner setter's pull/push and then report the locally written owner as recorded. These are real environment switches: `internal/boardengine/skipenv.go:13` reads them without a test-only restriction. D3 requires an owner-writing start/resume to publish its owner before driving the run, and D5 step 3 requires publication before adoption can resume anything. A successful local read is not that publication. For example, with `BOARD_SKIP_PUSH=1`, adoption can leave origin naming the previous owner while reporting the new owner recorded; a later origin reconcile consequently still judges the previous owner. Card 8 also deliberately depends on this bypass in its claim test. No decision licenses this exception, and the Intent supplies no production bound for it.

### [MEDIUM] Public prerequisites need their own cards

**Location:** `_lyx/plan/16-adopt-module.md:16`; also cards 3, 5, 11, 12 and 15.

**Issue:** The granularity rule expressly gives an independently meaningful public surface its own card even when its first consumer is unique. Card 16 combines an adoption CLI, a batten resume API, a destructive fabric primitive and a reusable hub fixture constructor. `DropLocal` has its own mutation contract, guard registrations and test; `ResumeInWindow` belongs to batten's public surface; `CloneSecond` belongs to shared test infrastructure. They are independent prerequisites, not private helpers of adopt. The same bundling recurs with `ParentSlug` in the board-write card, `ParentHoldsTaskWork` in the reconcile card, `PauseOwnedElsewhere` in batten wiring, `LocalOnlyCommits` in Finalize, and `ClientAttached` in the window-resume card. File count alone is not the issue: the planned commits combine separately reviewable contracts across their owning modules.

### [MEDIUM] A skipped absent entry becomes a lost owner write

**Location:** `_lyx/plan/03-board-owner-writes.md:53`; consumer at `_lyx/plan/08-batten-owner-check.md:48`.

**Issue:** Card 3 first says an absent or done entry receives neither a claim nor an owner, but its post-push rule then puts each slug whose recorded owner is not `Machine` into `Lost`. An absent entry necessarily has no recorded owner, so it meets that mismatch rule. Card 8 refuses any claim whose slug appears in `Lost`, yet expressly requires an absent entry to pass without a write. D3 also explicitly permits starting a slug with no board entry. The setter must distinguish an intentional skip from losing an eligible write; the proposed fake-Set absent-entry row in card 8 cannot detect this disagreement in the real setter.

## Rejected

- **Forward dependencies in cards 2–6 and 8–13:** No additional ordering defect established. OwnerRequest is created in card 3, the reconcile reader interface in card 5 and its production builder in card 6; the owner-pause label arrives in card 7, before its consumers. Card 11 creates PauseOwnedElsewhere before card 13 installs it. References to later production wiring describe staged activation rather than an earlier card needing a later symbol to build.
- **Unbounded Takeover, Finalize exemption or nil seams:** Takeover is expressly confined to adopt; non-takeover writes retain the existing owner's claim. Cards 10–12 bound the Finalize exemption by status, the transient restep and the parent's already-held task work. Cards 9, 12 and 13 describe their nil behavior and production wiring. These do not independently warrant an attack-surface finding; the distinct skip-environment bypass is F1.
- **Unreachable owner refusals:** Cards 8, 9, 10, 12 and 13 name relevant inputs and reaching tests for their new owner outcomes. Loom's existing refuseCovered check would precede an owner refusal for a paused fixture, but an existing unpaused run owned elsewhere reaches the new check; the plan does not require that fixture to be paused. Finalize's tests distinguish the before-merge check from its past-merge exemption. No additional refusal was shown to be impossible after upstream checks.
- **Unlicensed scope in cards 15–17:** Card 15 implements D8, including same-machine detach/resume. Card 16 implements D5 and the root registration/sandbox constraint. Card 17 implements D9 and the explicit Added after Discussion instruction. D1 is carried by card 1; D2 by cards 2, 3, 8, 13 and 16; D3 by cards 8–11; D4 by cards 5–7 and 11–13; D6 by cards 10–12; D7 by card 12; D9 by cards 4, 8, 14 and 17. The identified behavioral disagreements are F1 and F3, rather than an omitted entire decision.
- **Test duplication in the focused cards:** Card 7 extends TestCoverageOf and TestPause rather than creating parallel test functions. Existing InnerRun parent-lock tests do not cover an owner verdict, owner-labelled pause or post-merge ownership exemption. Existing tender status coverage has no owner-selection contract, and existing pause coverage has no adopt-specific hub-only liveness contract. No additional redundancy finding is supported merely by the new tests exercising related code. Private supporting types remain with their consuming surfaces; F2 concerns independent public APIs.
- **Missing member glyphs or re-sign arrows:** The focused cards identify the changed members. Card 5 supplies the classifyEntries re-sign arrow; the other focused changes add fields or behavior rather than prescribing changed parameters, results or receivers of existing functions. No additional re-sign defect established. Mechanical target checks were not re-derived.
- **Card 17 PATTERN short form and trailer:** The decision record expressly authorizes the shortened overview line and verbatim full text in its background file. The card preserves other entries' rules while cutting wording, and its trailer cites the declared new text as the plan contract requires. No extra stencil-prose test is required.

The review used file reads only. No target files were changed and no tests or validation programs were run. F1 and F3 propose Intent-only changes; F2 uses the specified card-insertion exemption from overlay validation.
