---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/plan/03-board-owner-writes.md:48
    summary: The Takeover guard covers named slugs only; a Prime request without Takeover overwrites a prime owner another machine recorded between the caller's read and SetOwners' pull, so batten's, orch's and adopt's first-act prime claim can take the prime with no lyx adopt, against D2.
    fix: In card 3's Intent, after the sentence opening "Without `Takeover` a slug whose effective owner", add that without `Takeover` a `Prime` request whose prime owner, read after the pull, is already recorded as another machine writes no prime owner and reports `prime` in `Lost` with that owner, so a first-act claim never replaces a recorded prime owner. Add a `TestSetOwners` row for it, a second clone that pushed a prime owner after this clone's last read, then a `Prime` write here without `Takeover` that leaves origin's prime owner and reports it in `Lost`. Card 8's `claimPrimeOwner`, card 13's `claimPrime` and card 16's step 3 already read `Lost`, so they need no change.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/plan/12-finalize-owner-checks.md:42
    summary: Card 12's past-merge carve-out skips both owner checks and the unreachable-origin halt, but its Intent never states the bound D6 and D7 give it (only the board-done mark, the parent push and, after a pushed landing, the PR close get through, and a re-entry through InnerRun's Spawn seam meets D3's refusal instead).
    fix: In card 12's Intent, after "skipping both owner checks and the unreachable halt.", add a Bound sentence. Past the parent-side merge the skip lets through only Finalize's board-done mark, its parent push and, after a pushed landing, its close of the pull request, nothing that merges. It is reached only by the shed driver's in-process re-step, because a re-entry through InnerRun's `Spawn` seam meets card 9's owner refusal first and is paused by card 10 with no past-merge exemption. A `ParentHoldsTaskWork` read error never takes the skip.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/17-orch-role-and-pattern.md:18
    summary: Card 17's short PATTERN.md line for PATTERN-owner-machine drops the declared exception for a Finalize already past its parent-side merge, and says the owner is judged against origin in ReconcileOrigin with no qualifier. The overview every spawned agent reads as binding therefore forbids the carve-out cards 10 and 12 build, and implies that the local-Bolt reads of cards 8, 12 and 14 break the rule.
    fix: Replace the entry line in card 17's fenced block with a short form that keeps the exception and limits the ReconcileOrigin clause to the gating verdict, staying under pattern.MaxEntryLineChars. For example, Starting, resuming or landing a run (bar a Finalize past its parent-side merge), or acting on it after a wake, only where no other machine owns its entry; owner set only by `boardengine`'s owner setter; the verdict gating a resume, wake or landing judged in `ReconcileOrigin`; machine name only from `internal/machinename`. — [background](pattern/PATTERN-owner-machine.md). Count any added bytes into the cuts card 17 already plans.
  - id: F4
    severity: LOW
    class: consistency
    location: _lyx/plan/17-orch-role-and-pattern.md:23
    summary: Card 17 plans a Pattern-Departure trailer for its own planned short form, but loom-plan-spec's trailer is only for a fork that departs from its card because a PATTERN entry forbids the card's way. Card 17 departs from nothing, so the trailer would put a false departure into the batch digest that Webster-Review reads. The overview's matching sentence also sits indented under the unrelated weft-push-retry bullet.
    fix: Delete the sentence at 17-orch-role-and-pattern.md:23, which opens "The commit carries a `Pattern-Departure:` trailer", and the continuation line at 00-overview.md:42, which opens "Card 17's commit carries a `Pattern-Departure:` trailer". The reason for the short form already stands in the Shared Decisions bullet at 00-overview.md:39 and in the decision record's Added-after-Discussion entry.
  - id: F5
    severity: LOW
    class: consistency
    location: _lyx/plan/06-owner-seam-wiring.md:25
    summary: "SeamOptions.ParentOf is func(slug string) (string, error), but card 6's OwnerReader builder takes parentOf as func(hubPath, slug string) (string, error), and Teardown.OriginSeams is said to pass the options' ParentOf straight to it, so the two types disagree and no adapter is named."
    fix: "In card 6's Intent, either declare `SeamOptions.ParentOf` as `func(hubPath, slug string) (string, error)` with `HubSeamOptions` filling it as `fabricengine.ParentSlug` over the location `hubPath` resolves to, or keep `func(slug string)` and say `Teardown.OriginSeams` wraps it as `func(_, slug string) (string, error) { return o.ParentOf(slug) }` when it builds `OwnerOf`. Then align the TestTeardown_OriginSeams fake ParentOf rows with the chosen type."
  - id: F6
    severity: LOW
    class: design
    location: _lyx/plan/16-adopt-module.md:18
    summary: "Several new exported symbols, each public surface of another package and reviewable on its own, are bundled into a consumer card instead of getting their own card: fabricengine.ParentSlug (card 3, used by cards 6, 8, 9, 12, 14 and 16), Fabric.LocalOnlyCommits (card 12), tendercli.PauseOwnedElsewhere (card 11, also used by card 13), reedengine.Engine.ClientAttached (card 15), and fabricengine.DropLocal with DropLocalResult, a new destruction primitive behind PATTERN-fabric-destruction-chokepoint (card 16), plus battencli.ResumeInWindow (card 16)."
    fix: "With `lyx loom insert-card`, give each of these its own card ahead of its first consumer, carrying its Create glyph, its test (TestParentSlug, TestLocalOnlyCommits, TestClientAttached, TestDropLocal) and the Intent sentences that describe it. Put ParentSlug before card 3, PauseOwnedElsewhere before card 11, LocalOnlyCommits before card 12, ClientAttached before card 15, and DropLocal with its destroy.go header and destruction-guard table edits, plus ResumeInWindow, before card 16. Then remove those glyphs and sentences from the consumer cards and leave a Uses line in their place. Put the destroy.go and cmd/lyx destructive-guard edits on the DropLocal card."
---

# Review: machine-handover plan, round 1

I read every card and the overview, and held them against D1 to D9, the Added-after-Discussion entry and the Constraints.
On fidelity, every Decision step has a card.
That covers D1 on card 1, D2 on cards 2, 3, 8, 13 and 16, D3 on cards 8 and 9, D4 on cards 5, 6, 7 and 13, D5 on card 16, D6 on cards 10, 11 and 12, D7 on card 12, D8 on card 15, D9 on cards 4, 8, 14 and 17, and the PATTERN entry on card 17.
None of cards 15, 16 or 17 adds scope the record does not license.
Card 5's guard on `retryWeftPush` is new, but the record already says the old machine's pushes are refused and recorded as failed pushes, which only holds with that guard, so I count it as licensed.
None of these findings is BLOCKING.

### [MEDIUM] Takeover guard leaves the prime path open

**Location:** `_lyx/plan/03-board-owner-writes.md:48`

**Issue:** `SetOwners` pulls, then writes "the prime owner when `Prime` is set".
The guard against moving another machine's owner without `Takeover` is worded only for slugs.
`claimPrimeOwner` (card 8), `claimPrime` (card 13) and adopt's step 3 (card 16) each decide to claim from a local read made before `SetOwners`' own pull.
If another machine's prime claim reaches origin in that window, the pull brings it in, the commit overwrites it, and the push fast-forwards, so no replay reports anything `Lost`.
That breaks D2: "a machine whose write the replay reports lost reads the other's claim".
It also means the overview's Shared Decision, "An owner write never takes an entry another machine owns unless `lyx adopt` asks for it", does not hold for the prime.

### [MEDIUM] Finalize past-merge skip has no stated bound

**Location:** `_lyx/plan/12-finalize-owner-checks.md:42`

**Issue:** The focus asked whether this carve-out states what it lets through and what bounds it.
Card 12 states what it skips: both owner checks and the unreachable halt.
It does not state what it lets through, or that it is limited to the in-process re-step.
D6 lists exactly what gets through ("Finalize's board-done mark, its parent push and, after a pushed landing, its close of the pull request, and nothing else").
D7's Bound names the `Spawn` re-entry as the case the carve-out does not cover.
Card 10's Bound covers the InnerRun side, but the guard bypass is introduced in card 12, so its bound belongs there.

### [MEDIUM] Short PATTERN line drops the Finalize exception

**Location:** `_lyx/plan/17-orch-role-and-pattern.md:18`

**Issue:** The Added-after-Discussion entry allows a short form, but a short form must not state a different rule.
The declared text includes "bar the rest of a Finalize already past its parent-side merge", and the short line leaves it out.
The short line also says the owner is "judged against origin in `ReconcileOrigin`" for every case, while the declared text names local-Bolt reads for a start, the post-push re-reads, Finalize's parent-owner read and a resume with origin unreachable.
`PATTERN.md`'s overview is inlined into every spawned agent's prompt, and the implementer directive says its constraints win over the card.
So a Webster fork building card 10's `keepsLockForFinalize` or card 12's past-merge skip would read its own card as a violation.
The planned line is 358 characters, so a reword like the one in the fix is needed rather than an append.

### [LOW] Pattern-Departure trailer misused for a planned change

**Location:** `_lyx/plan/17-orch-role-and-pattern.md:23`

**Issue:** `loom-plan-spec.md`'s `## Pattern-Departure trailer` section says the trailer is for "A Webster fork, the recovery strand or the verify-fix fork that departs from its card because a PATTERN entry forbids the card's way".
Card 17 builds exactly what it prescribes, and the decision record licenses the short form, so nothing departs.
`record-batch` would still read the trailer into the digest's `departures`, which Webster-Review then reads as a real departure.
The overview's matching sentence (`00-overview.md:42`) is also indented as a continuation of the weft-push-retry bullet, which it has nothing to do with.

### [LOW] Two types for ParentOf in card 6

**Location:** `_lyx/plan/06-owner-seam-wiring.md:25`

**Issue:** The `OwnerReader` builder's signature takes `parentOf func(hubPath, slug string)`, while `SeamOptions.ParentOf` is `func(slug string)`.
Line 26 says `Teardown.OriginSeams` fills `OwnerOf` "with `OwnerReader` over the options' `Machine` and `ParentOf`", which does not type-check as written.

### [LOW] Public helpers bundled into consumer cards

**Location:** `_lyx/plan/16-adopt-module.md:18`

**Issue:** The rubric's granularity rule says a symbol that is part of a package's public surface, or reused across packages, gets its own card.
The symbols listed in the finding are each exported from a package other than the card's subject, and each has its own test.
`DropLocal` matters most here: it is a new destructive primitive in `destroy.go` behind the destruction chokepoint, and it sits inside the 77-line adopt card.
I raise this once as a method finding rather than once per symbol.
Exporting `ParentHoldsTaskWork` on card 5 is left out: it is a one-line export of an existing function, in the same package as that card's own work.

## Rejected

- Card order across cards 2 to 6 and 8 to 13: every planned symbol is created before its first user. The owner reader comes from cards 5 and 6, `OwnerRequest` and `SetOwners` from card 3, `OwnerCaller` from card 7, `StartOwnedElsewhereKind` from card 9, `ChildOwnedElsewhereError` from card 10, `PauseOwnedElsewhere` from card 11 (used by card 13), `ParentHoldsTaskWork` from card 5 (used by cards 11 and 12), and `OwnerHaltPrefix` from card 12 (used by card 16). Not a defect.
- `OwnerRequest.Takeover` on named slugs: card 3 states that without it a slug owned elsewhere is skipped and reported `Lost`, and that only adopt sets it. That is a stated bound. The prime gap is F1.
- The nil-seam-is-absent rules on cards 9, 12 and 13: each card states what nil does. `newLoomCLI` (`internal/loomcli/cli.go:179`) is the only production constructor of `loomCLI`, `loomcli.landingDeps` (`internal/loomcli/landingdeps.go:70`) is the only production builder of `landingshed.Deps` and is pinned by `TestLandingDeps_EveryFieldPopulated`, and orch's nil `pauseOwned` is reported under warnings. Not a defect.
- Reachability: every prescribed refusal or owner-pause branch on cards 8, 9, 10, 12 and 13 names an input and a test row. Loom's `refuseCovered` (`internal/loomcli/pausehold.go:40`) runs before the owner refusal, but InnerRun holds every spawn under coverage, so no reaching input is rejected first. Not a defect.
- Card 5 leaving only `needs_replay` unmoved: `originMover.needsMove` (`internal/fabricengine/originreconcilemoves.go:305`) moves only `fast_forward` and `needs_replay` branches, so `ahead`, `rewound`, `rewritten` and `diverged` already stay where they are. That covers D4.
- Member glyphs and re-sign arrows on cards 3, 5, 6, 10, 12 and 16: the one signature change, `classifyEntries`, carries its arrow. `resumeRefusal(slug, report)`, `reconcileVerdict`, `pushParent` and `childSpawnError` keep their signatures. Not a defect.
- Test economy on cards 7, 10, 14 and 16: cards 7 and 14 add rows to existing tests. `TestInnerRun_OwnerCheck`, `TestPairOwners` and `TestHubOnlyPaused` cover branches no existing test reaches. Not a defect.
- Card 1's sole-source scan: no production file calls `os.Hostname(` today, so the scan forces no other edit.

## Focus departures

- "PATTERN fit of card 17 and the cards touching `pattern/` files: settle whether the `Pattern-Departure:` trailer and the shortened entry line obey the PATTERN rules the plan names." I settled it against `loom-plan-spec.md`'s trailer section and the decision record rather than against a PATTERN.md entry, because no PATTERN.md entry or `pattern/` file governs the trailer (F3, F4).
- The rubric's step of checking target-changing fixes with `lyx loom validate-plan --overlay` was not run, because this round's tool-use rule allows reading only. F1 to F5 add, drop, move or pair no targets. F6 inserts cards, which the rubric excludes from the overlay, so no proposed fix needed one.
