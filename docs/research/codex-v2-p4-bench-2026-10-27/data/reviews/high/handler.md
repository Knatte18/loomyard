---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    location: _lyx/plan/05-reconcile-owner-outcome.md:36
    summary: Direct loom and darn runs without prime-side batten records escape the owner verdict.
    fix: Amend card 5's ReconcileOrigin and classifyEntries Intent so owner candidates include the addressed existing run and local task pairs independently of the batten-record filter used for teardown candidates; preserve the existing teardown eligibility rule. Extend its already-targeted reconcile tests with an owned-elsewhere pair holding a child status but no prime-side batten status, and require card 9's integration scenario to reach the refusal for that pair.
    origin: lens:guard-reach
  - id: F2
    severity: BLOCKING
    class: scope
    location: _lyx/plan/08-batten-owner-check.md:42
    summary: The Arm and ArmAt production paths still lack the reconcile seam needed by the resume guard.
    fix: Amend card 8's armAt Intent to initialize a missing reconcile seam to reconcileRun over the armed location, alongside the owner seams, while preserving explicitly supplied test seams. Extend the planned TestRefuseStartOwnedElsewhere with an ArmAt resume of an existing batten status beyond Worktree-Create whose origin owner is elsewhere, asserting refusal before the returned run or step hook drives a row.
    origin: handler
  - id: F3
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:46
    summary: The direct owner setter cannot resolve pair-parent ownership as its guard requires.
    fix: Add an optional ParentOf resolver to the already-declared OwnerRequest and require SetOwners to install it on the Store it loads inside PullThenCommitWritten. Amend card 8's hubOwnerSeams Intent to supply its real ParentSlug resolver on setter calls, and extend TestSetOwners with an idle unowned child of a pair owned by B while the prime is owned by A, testing both B's accepted claim and A's rejected claim without Takeover.
    origin: lens:guard-reach
  - id: F4
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:51
    summary: The deferred-push integration row waits through a production retry with no injectable timing seam.
    fix: Amend card 3's setter and TestSetOwners Intent to provide a per-call retry wait or interval seam on the already-created request or setter implementation, defaulting to one second in production. Have the held-push-lock row inject the wait and assert the retry and final refusal without sleeping for the production second; do not use a package-global clock override.
    origin: handler
  - id: F5
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/05-reconcile-owner-outcome.md:42
    summary: An explicit fabric pull can replay old-owner commits before a push that never reaches the retry guard.
    fix: Use insert-card after the owner reader and its wiring to cover Fabric.settleUnmovedWeft's replay decision with the same fresh-origin owner verdict, preserving permitted fast-forwards. Include public pull-then-push integration coverage showing that an old owner's diverged weft is not replayed and subsequently published, and remove card 5's assertion that every later push reaches retryWeftPush.
    origin: lens:guard-reach
  - id: F6
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/10-innerrun-owner-check.md:48
    summary: Lifting the completed old machine's owner pause cannot produce the promised continuation to Land.
    fix: Amend cards 8 and 10's existing preflight and owner-check Intents with the D6 completion case: after the old machine successfully finishes its already-merged Finalize and origin records completion, lifting the pause may finish InnerRun and drive Land without restarting the child or allowing earlier work. Carry that condition through both the coverage-clear check and the next batten step preflight. Extend the already-targeted owner-check and step tests with an unchanged foreign owner, completed child, successful landing and lifted pause; the adopter-first failed landing must remain held.
    origin: lens:guard-reach
  - id: F7
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/12-finalize-owner-checks.md:42
    summary: Finalize's earlier parent-tip fetch preempts the specified offline halt and past-merge continuation.
    fix: Amend card 12's Finalize.Call Intent to account for step 1b2 as well as step 1b3: retain parent-lock acquisition, arrange the local past-merge and owner judgments before a failing remote parent-tip read can preempt them, and retain moved-parent validation before any new parent merge. Extend TestFinalize_OwnerChecks with a populated parent guard whose fetching tip read fails, covering both an unmerged owner-prefixed halt and a proven already-merged in-process re-entry reaching the push attempt.
    origin: lens:guard-reach
  - id: F8
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:47
    summary: A stale first-act prime claim overwrites a freshly pulled prime owner without Takeover.
    fix: Amend SetOwners' existing Intent so the post-pull no-Takeover guard applies to Prime as well as Slugs, retaining a foreign prime owner and reporting the lost prime claim in OwnerWrite. Extend TestSetOwners with another clone claiming the prime after the caller's preliminary read but before this setter's pull, then assert no overwrite and no successful recorded claim for the losing machine.
    origin: lens:guard-reach
  - id: F9
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/12-finalize-owner-checks.md:45
    summary: Appending the adopter-first repair leaves incompatible merge-and-push instructions in the same halt note.
    fix: Amend card 12's pushParent Intent so a confirmed adopter-first repair replaces the ordinary recovery instructions while preserving the diagnostic cause. Extend the already-targeted failed-push rows with a real classified code-side non-fast-forward PushFailedError, asserting the squash-drop instructions and absence of the ordinary merge-and-push advice; retain the ordinary advice when the freshly fetched parent does not prove another landing or the read fails.
    origin: lens:way-forward
  - id: F10
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/16-adopt-module.md:55
    summary: Adoption under a preserved hub pause blocks in the network gate before its promised result.
    fix: Amend card 16's hubDeps and adoption transaction Intent to provide a narrowly scoped network exemption for the exact existing hub pause while reconciling, adding pairs and pushing the owner write, following the existing entry-identity exemption mechanism. Restore the exemption before any resume and preserve the pause record, so a new or different pause is not bypassed. Extend the already-created adoption integration test with the real installed network gate and a live hub pause, asserting completion of the owner transaction and the standing-pause report.
    origin: lens:way-forward
  - id: F11
    severity: MEDIUM
    class: design
    location: _lyx/plan/10-innerrun-owner-check.md:38
    summary: The optional seam additions omit the shared reflective nil-policy contract and its consumers.
    fix: Use insert-card before card 10 to update internal/testkit/envkit's nilLegal contract for InnerRun.OwnerCheck, InnerRun.PauseOwned, InnerRun.ParentHoldsTaskWork, InnerRun.Restepped, Landing.ParentHoldsTaskWork, Landing.ParentOwner, Landing.OriginParentHoldsWork and Landing.ParentCommitsOriginLacks, documenting the nil defaults the plan already chooses. Keep production-population assertions in cards 11 and 12, and use the existing envkit and CLI wiring tests to verify the optional-field contract at each intervening card boundary.
    origin: lens:affected-set
  - id: F12
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/09-loom-owner-check.md:16
    summary: The external integration test cannot invoke the private owner helper and injected machine seam it is prescribed to test.
    fix: Keep the declared external test target, but rewrite card 9's TestRunOwnerVerdict Intent to exercise the public RunCLIIn resume boundary over hubforge fixtures, deriving the local owner from machinename.Name instead of injecting a private helper argument. Use a valid terminal child status for owned-here cases so the downstream done response proves passage through the guard without spawning a driver; assert the owner refusal for foreign-owner cases and preserve the origin and offline-local cases.
    origin: lens:affected-set
  - id: F13
    severity: MEDIUM
    class: scope
    location: _lyx/plan/04-board-owner-views.md:21
    summary: The existing compact board listing still hides the effective owner.
    fix: Use insert-card immediately after card 4 for the independently reviewable compact-listing surface, editing internal/boardcli's RenderCompact and extending its existing TestRenderCompact table to display the effective BriefTask.OwnerMachine. Cover an owned task, an empty owner and a note, retaining the existing alignment and empty-input behavior; both list --text and find --text should receive the owner through their shared renderer.
    origin: lens:affected-set
  - id: F14
    severity: MEDIUM
    class: design
    location: _lyx/plan/16-adopt-module.md:16
    summary: Consumer cards bundle independently reviewable public APIs from other modules.
    fix: Use insert-card to separate fabricengine.ParentSlug from card 3, Fabric.LocalOnlyCommits from card 12, and battencli.ResumeInWindow, fabricengine.DropLocal with DropLocalResult, and hubforge.CloneSecond from card 16, placing each public surface and its supporting tests and documentation before its consumers. Keep each surface's private helpers and inseparable result types with that surface, and keep the private adopt workflow together in the adopt card.
    origin: handler
---

### [BLOCKING] Direct child runs are missing from the owner candidates

**Location:** `_lyx/plan/05-reconcile-owner-outcome.md:36`; `_lyx/plan/09-loom-owner-check.md:32`.
**Issue:** The plan equates recorded pairs with existing runs, but `recordedPairs` in `internal/fabricengine/originreconcilemoves.go:436` includes a pair only when `shedrun.StatusFile(prime, slug)` exists.
Direct loom bootstrap writes the child's SelfRunID seed and child status, without creating that prime-side batten status (`internal/loomcli/sharedbootstrap.go:103`).
With a child status present and origin's board naming another owner, card 9 runs its guard but receives no owned-elsewhere entry; card 12's run-owner check consumes the same incomplete report.
This violates the declared PATTERN-owner-machine rule for resuming and landing existing runs, as well as D3 and D7.
Changing ownership candidates need not broaden the set eligible for automatic teardown.

### [BLOCKING] The generic shed entry bypasses batten's resume judgment

**Location:** `_lyx/plan/08-batten-owner-check.md:42`.
**Issue:** The card fills the new owner seams in `armAt` and explicitly claims coverage of `Arm` and `ArmAt`, but existing-status preflight delegates to `reconcileResume`.
That method returns immediately when `c.reconcile` is nil (`internal/battencli/resumereconcile.go:39`).
The production assignment is in `battencli.Command` (`internal/battencli/cli.go:254`), whereas `Arm` and `ArmAt` construct fresh empty receivers (`internal/battencli/arm.go:477`, `:488`).
The generic shed recipe table calls `battencli.ArmAt` directly (`internal/shedcli/table.go:60`).
Thus filling `owner` alone does not make an existing run reached through `lyx shed run` or `step` judge origin ownership.
The prescribed absent-status arm test cannot catch this PATTERN-owner-machine and PATTERN-origin-reconcile gap.

### [BLOCKING] Setter inheritance differs from preflight inheritance

**Location:** `_lyx/plan/03-board-owner-writes.md:43` through `:48`.
**Issue:** `Config → Board → Store` wiring covers facade-created stores, but the new `SetOwners(boardDir, req)` explicitly loads its own store under the Bolt lock.
Its enumerated request fields carry no parent resolver, and the card specifies that a nil resolver treats every parent as the prime.
For prime owner A, pair-parent owner B and an idle unowned child of that pair, B passes card 8's resolved effective-owner preflight and then loses the setter's no-takeover check against A.
Conversely, a direct no-takeover setter call by A can pass the wrong inherited-owner check.
The claim occurs before the existing-pair shortcut in `internal/battencli/wire.go:777`, so an already-created child pair does not avoid the defect.
This contradicts the declared PATTERN-owner-machine inheritance rule and the plan's stated protection against claims that skipped their preliminary check.

### [BLOCKING] The push-lock test has no way to avoid the production wait

**Location:** `_lyx/plan/03-board-owner-writes.md:51`, `:58`.
**Issue:** The setter must wait one second before its deferred-push retry, and the integration table must hold the push lock across that retry to reach the final refusal.
Neither the request contract nor the described setter test arrangement supplies an injectable wait or interval.
As prescribed, reaching that public behavior waits out the production duration.
PATTERN-test-speed's background explicitly forbids that in every tier and requires a struct field or parameter for production timing a test otherwise waits through.
The integration tag does not exempt this row.

### [MEDIUM] Pull followed by a successful push avoids the replay guard

**Location:** `_lyx/plan/05-reconcile-owner-outcome.md:42`.
**Issue:** The assertion that every push after a local pull meets the guarded retry is false.
`Fabric.settleUnmovedWeft` reads the ended set and calls `replayWeft` for a diverged records branch (`internal/fabricengine/pull.go:369`, `:379`).
After replay onto the adopter's fetched tip, the old machine's next push can fast-forward.
`pushClassified` returns immediately on a successful push and calls the records retry only after rejection (`internal/fabricengine/pushclassified.go:118`).
The old-owner commits therefore reach origin without the new owner check, defeating the no-replay handover bound claimed in card 5 and D3/D6.
This is a distinct public route to the guarded operation, not a request to prevent all pushes after takeover.

### [MEDIUM] The successful old landing is paused again instead of reaching Land

**Location:** `_lyx/plan/10-innerrun-owner-check.md:40`, `:48`; `_lyx/plan/08-batten-owner-check.md:54`.
**Issue:** D6 explicitly requires an old machine that finishes its already-merged Finalize first to proceed to Land after `lyx resume <slug>` lifts its pause (`_lyx/discussion/decision-record.md:156`).
Card 10 instead sets `ownerDue` when coverage clears and immediately restores the pause on the unchanged foreign owner; its lock carve-out ends when the child becomes Done.
Even allowing InnerRun to finish would leave card 8's next step preflight refusing the same owner.
The driver invokes `batten step` through `RunUntilStop` (`internal/battencli/drive.go:139`), and the generic step executes `PreStep` before constructing the row (`internal/shedverbs/step.go:367`).
An origin done mark does not automatically solve this: `withoutFinalizeEntered` deliberately removes this machine's own landing from the ended set (`internal/fabricengine/replay.go:312`).
The planned done-child table row needs the foreign owner to remain foreign; changing that fake verdict to owned-here would test adoption back, not the required completion path.

### [MEDIUM] The offline policy is behind a fetch that already fails

**Location:** `_lyx/plan/12-finalize-owner-checks.md:42`, `:50`.
**Issue:** Card 12 replaces step 1b3, but step 1b2 first calls `holdParent` (`internal/landingshed/finalize.go:198`).
That calls the moved-parent guard, whose normal path fetches parent tips and returns an infrastructure error on failure (`internal/landingshed/stuck.go:115`, `internal/landingshed/parentguard.go:126`).
Production wires this to `Fabric.ReadParentTips`, which returns immediately when its fetch fails (`internal/loomcli/landingdeps.go:175`, `internal/fabricengine/parenttips.go:44`).
An unreachable code origin therefore prevents both the prescribed owner-prefixed halt before a fresh merge and the local past-merge exemption on an in-process retry from being evaluated.
A fake unreachable reconcile with an absent parent guard can pass the proposed table while the production path still fails earlier.

### [MEDIUM] The no-takeover bound omits the prime

**Location:** `_lyx/plan/03-board-owner-writes.md:47` through `:48`.
**Issue:** The setter writes the prime whenever `Prime` is set, while the post-pull no-takeover rule names only a slug.
A first-act caller can read no prime owner, then enter the setter after a second machine's claim has reached origin.
The setter pulls that claim and overwrites it in a normal descendant commit, so no replay conflict invokes the origin-wins rule.
Card 8's lost-prime-claim scenario and the overview's statement that only adopt moves an existing owner are therefore not implemented for this race.
The guard must apply to the fresh store state, not only the caller's earlier read.

### [MEDIUM] The repair note still offers the forbidden merge

**Location:** `_lyx/plan/12-finalize-owner-checks.md:45`.
**Issue:** Existing `Finalize.pushParent` formats the complete push error into its reason (`internal/landingshed/finalize.go:326`).
`PushFailedError.Error` includes its class's recovery instructions (`internal/fabricengine/pushclassified.go:36`), and a code-side non-fast-forward advises merging origin's parent and pushing (`internal/fabricengine/failedpush.go:217`, `:249`).
Appending the new squash-drop repair therefore leaves both contradictory instructions in the halt note.
D6 expressly requires dropping the duplicate squash and never merging it (`_lyx/discussion/decision-record.md:149`).
The test must use the classified error that production returns; an ordinary synthetic error does not reveal the inherited merge advice.

### [MEDIUM] A hub pause prevents adoption from reaching its pause report

**Location:** `_lyx/plan/16-adopt-module.md:55`, `:62`, `:66`.
**Issue:** The card promises adoption under a hub pause, including treating the interrupted pair as not live for drop-local and leaving the hub pause standing with a resume instruction.
The root installs the pause network gate before executing adopt (`cmd/lyx/main.go:125`).
Remote git operations enter that gate before starting their timeout (`internal/gitexec/gitexec.go:203`), and `Network` waits with a background context until coverage clears (`internal/pauserecord/hold.go:249`).
Changing `HubPaused` for the liveness seam does not exempt the command's network operations.
The initial reconcile therefore waits before the workflow can write owners or emit its standing-pause result.
The existing `SetGateExempt` mechanism identifies an exact entry by scope and pause time (`internal/pauserecord/hold.go:216`); the plan currently prescribes no equivalent bounded handling for adopt.

### [MEDIUM] The shared seam checker rejects the planned nil defaults

**Location:** `_lyx/plan/10-innerrun-owner-check.md:38`; `_lyx/plan/12-finalize-owner-checks.md:40`.
**Issue:** Both cards add exported function fields and explicitly allow nil defaults, but no card updates the shared reflective contract that interprets a nil field as missing wiring.
`envkit.NilSeams` recursively checks exported fields and skips only the paths listed in `nilLegal` (`internal/testkit/envkit/envkit.go:44`, `:502`).
The added paths are absent from that list, and existing `FullEnv` builders cannot initialize fields that do not yet exist.
Consequently `TestFullEnv_FillsEverySeam`, `TestNilSeams_ReportsExactPaths` and `TestNilSeams_SkipsNilLegalPaths` acquire unexpected paths (`internal/testkit/envkit/envkit_test.go:14`, `:57`, `:73`).
The ordinary loom and batten wiring assertions also inspect the complete result; loom leaves InnerRun unused, and Env.Landing is assembled later (`internal/loomcli/wiring_seams_test.go:13`, `:102`; `internal/battencli/wire_seams_test.go:114`).
Card 12's populated `landingDeps` test covers a different construction stage.
This is one shared affected-set omission across the two cards, including the interval before card 11 fills batten's new seams.

### [MEDIUM] The prescribed integration test crosses a private package boundary

**Location:** `_lyx/plan/09-loom-owner-check.md:16`, `:39`.
**Issue:** The card declares `runOwnerVerdict` and `ownerVerdict` in `internal/loomcli`, but declares `TestRunOwnerVerdict` in the external `internal/loomcli_test` package and prescribes a fixed injected machine argument.
That package cannot call the private helper or inspect its private result type, and no bridge is planned.
The existing internal landing integration tests demonstrate the distinction (`internal/loomcli/landingdeps_integration_test.go:8`).
A public resume integration test can cover the intended owner outcomes without this inaccessible seam: the existing done-state response returns before any driver spawn (`internal/loomcli/resume.go:71`).

### [MEDIUM] Text board listings never render the new field

**Location:** `_lyx/plan/04-board-owner-views.md:21`.
**Issue:** D9 requires owner visibility in board list, and its heading explicitly covers every status view (`_lyx/discussion/decision-record.md:206`).
The card covers `BriefTask`, JSON get/list and the README but omits the existing compact listing.
`writeListing` routes both text list and text find through `RenderCompact` (`internal/boardcli/cli.go:961`).
That renderer constructs explicit columns and appends only status; adding a struct field cannot make it appear (`internal/boardcli/text.go:20`, `:44`).
Its existing `TestRenderCompact` table pins the output and is the appropriate assertion to extend (`internal/boardcli/text_test.go:18`).

### [MEDIUM] Several public surfaces are buried in their consumers' cards

**Location:** `_lyx/plan/16-adopt-module.md:16`; also `_lyx/plan/03-board-owner-writes.md:22` and `_lyx/plan/12-finalize-owner-checks.md:23`.
**Issue:** Card 16 combines the new adopt command with a batten resume API, a fabric destruction API and a reusable hub fixture API.
These have separate owning modules, public contracts and verification concerns; the card itself acknowledges all three exported additions in its impact summary.
The same batching occurs for the pair-parent resolver in card 3 and the local-commit reader in card 12.
The rubric requires independently reviewable public or cross-package surfaces to have their own cards, even when a surface has one consumer.
This does not justify splitting `DropLocalResult` from `DropLocal`, constructors from their inseparable types, or private workflow helpers from adopt.

## Focus conclusions

The focused owner symbols have valid declaration order: owner records precede OwnerRequest and ReadOwners, the reconcile reader precedes its wiring, the owner-pause label precedes its consumers, and PauseOwnedElsewhere precedes card 13's installer.
No finding is based on an incomplete Uses list or a deterministic plan-gate check.
The material intermediate-card issue is the reflective seam contract in F11.

Takeover, the Finalize carve-out and nil-is-absent behavior have stated bounds in their Intents.
The defects are the incomplete prime bound in F8, missing production wiring in F2, and reachable-path failures in F1, F6 and F7, rather than the mere existence of those escape hatches.
The other reviewed owner refusal and pause branches have named inputs and tests; their presence does not repair the missing paths above.

D1–D9 are represented by cards, but the specific completion, owner-write, recovery and visibility behavior identified above is incomplete.
Cards 15 and 16's intended same-machine window reopening and takeover workflow are licensed by D8 and D5.
Card 17's short overview entry is expressly licensed by the added parent decision, which also requires the full text in the background and meaning-preserving byte reductions elsewhere.
Its Pattern-Departure trailer records that authorized short form; it does not authorize weakening other invariants.
No additional scope finding is raised against those choices.

Card 7 extends existing coverage and pause tests.
The new owner-state rows in cards 10, 14 and 16 cover behavior the existing tests do not already pin; no duplicate-test finding is substantiated for those rows, and private helpers remain with their surfaces.
F14 concerns independent public APIs, not a demand for one card or test per private symbol.
The focused Edit cards use member glyphs for changed members, and card 5 supplies the classifyEntries re-sign arrow; struct-field additions require no arrow.
No additional member-glyph or re-sign finding is raised.

## Rejected

- The suspected card 13 constructor-time snapshot failure is not established: `orchcli.Command` already fills its operational fields inside its PersistentPreRunE, after the root hook (`internal/orchcli/cli.go:169`), so copying the installed pause there fits the card's wording.
- The affected-set report's claim that the darn-specific wiring assertion also fails on every new nil path is too broad: that test intersects the result with its `darnSeams` list (`internal/loomcli/wiring_darn_test.go:61`); F11 relies on the complete-result assertions instead.
- Treating card 17's shortened overview as an unauthorized departure would contradict the fasit, which explicitly authorizes that representation while retaining the full declared rule in the background.

## Review limits and proposed-fix shape

This review used source reads only, as the explore/review tool-use rule requires.
No tests, board command or plan validator were executed; the failures above are source-grounded predictions, not reported test runs.
That read-only rule conflicts with the separate instruction to retrieve the board through a command, so the supplied decision record and plan are the authority actually reviewed.
Existing-card proposals refine Intent and already-planned tests without changing target lists; proposals that introduce or separate target surfaces explicitly use insert-card, which the proposed-fixes rubric exempts from overlay validation.
No executable verification result is claimed.
