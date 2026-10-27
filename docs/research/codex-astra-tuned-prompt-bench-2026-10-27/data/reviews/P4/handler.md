---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    location: _lyx/plan/05-reconcile-owner-outcome.md:36
    summary: Existing runs without a local batten record receive no owner judgment
    fix: Amend card 5 so ReconcileOrigin includes the requested run in owner judgment and entry classification even when recordedPairs omits it, while retaining ended precedence and judge-only behavior. Extend its already-targeted TestReconcileOrigin_Moves and card 9's TestRunOwnerVerdict with an existing child run in a newly added local pair, no prime-side batten status, and an origin owner on another machine; require the owned_elsewhere verdict and resume refusal.
    origin: lens:guard-reach
  - id: F2
    severity: BLOCKING
    class: scope
    location: _lyx/plan/09-loom-owner-check.md:35
    summary: Foreground loom run and step can still execute another machine's existing run
    fix: Use lyx loom insert-card to insert a companion card after card 9 that applies its owner judgment to loomCLI.loomPreRun and loomCLI.loomPreStepUnmarked in internal/loomcli/arm.go before bootstrap or execution of an existing run. Extend TestVerbRefusals with seeded, unlocked run and step cases whose owner is elsewhere, asserting refusal before reed or producer work. Cover the generic shed routes through these hooks and preserve the separate in-process Finalize continuation exception.
    origin: lens:guard-reach
  - id: F3
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:52
    summary: Skip flags certify an owner claim that was never published
    fix: Remove the owner setter's successful local-only paths from card 3. Require a successful pull, commit when needed, push and reread before reporting an owner write as Recorded; refuse owner-writing requests when skip settings prevent that protocol. Change the skip-setting rows in TestSetOwners and card 8's claim test to assert refusal or use explicit injected test seams, rather than treating BOARD_SKIP_GIT or BOARD_SKIP_PUSH as confirmation of ownership.
    origin: handler
  - id: F4
    severity: BLOCKING
    class: scope
    location: _lyx/plan/16-adopt-module.md:52
    summary: The engine-free adopt module is missing from the CLI pattern's exceptions
    fix: Use lyx loom insert-card to insert a Prosa companion before the adopt module that updates pattern/PATTERN-cli-cobra.md's Package naming deviations with adoptcli, naming the owning modules it calls and its lack of an adoptengine. Keep D5's engine-free design; do not invent an engine to satisfy the default naming rule.
    origin: lens:affected-set
  - id: F5
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:58
    summary: The deferred-push test uses a blocking lock and leaves its retry delay uninjected
    fix: Rewrite card 3's deferred-push test around the actual board-write-lock deferral after pull and commit have completed, with an origin move causing push rejection. Specify deterministic phase control so the test acquires that lock only at the push boundary. Make the retry wait injectable through a field on the already-created OwnerRequest, with SetOwners supplying the production one-second default, and have TestSetOwners use a fake wait while asserting one retry and the still-ahead refusal.
    origin: handler
  - id: F6
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/12-finalize-owner-checks.md:42
    summary: Existing Finalize guards prevent the promised past-merge continuation and repair
    fix: Amend card 12's already-targeted Finalize.Call to distinguish a locally completed parent-side merge under the parent lock before the network-dependent moved-parent check. For that state, continue only the remaining board-done mark, parent push and post-push closure, without repeating merge-in or parentHandle.Merge. Extend TestFinalize_OwnerChecks and TestFinalize_MergeMarkPushOrderAndVerdict to exercise the real ordering with a failing parent-tip fetch and with a divergent adopter landing, requiring the push failure path and adopterFirstRepair instead of an earlier merge refusal.
    origin: lens:way-forward
  - id: F7
    severity: MEDIUM
    class: scope
    location: _lyx/plan/08-batten-owner-check.md:48
    summary: Direct wire callers reach the new claim callback with zero owner seams
    fix: Amend card 8 so the already-targeted battenCLI.wire initializes the claim's owner seams from its supplied hub location when they are zero, preserving explicitly injected seams. Extend TestWire_CreateWorktreeClaimsTheBoardEntry with an unarmed receiver that calls wire and then the real CreateWorktree closure. Keep omission of preflight checks for deliberately unarmed tests separate from the callback's requirement for a usable owner setter.
    origin: lens:affected-set
  - id: F8
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/03-board-owner-writes.md:53
    summary: The Lost calculation turns an absent-entry no-op into a claim refusal
    fix: Amend card 3's result contract and reread rule so absent and done entries skipped by the setter are excluded from Lost as well as Recorded. Restrict Lost to eligible entries whose claim was refused or whose owner write lost. Add the absent and done cases to TestSetOwners, and make card 8's absent-entry claim row exercise the real setter result rather than a fake success.
    origin: handler
  - id: F9
    severity: MEDIUM
    class: design
    location: _lyx/plan/16-adopt-module.md:16
    summary: The adoption card bundles three independent cross-package public APIs
    fix: Use lyx loom insert-card to create prerequisite cards for battencli.ResumeInWindow, fabricengine.DropLocal with its inseparable result type and destruction registrations, and hubforge.CloneSecond. Assign each surface its relevant tests and documentation, remove those units from the adoption card through the card-edit workflow, and leave adoption as their consumer. Keep private adoption helpers with the adoption surface.
    origin: handler
  - id: F10
    severity: LOW
    class: consistency
    location: _lyx/plan/10-innerrun-owner-check.md:40
    summary: An exactly one-minute sleep does not trigger the planned owner check
    fix: Amend card 10's check and childWait design to set ownerDue from an inclusive wall-minus-awake gap of at least SleepTolerance, using the existing clock seams and per-wait sample state. Do not assume SleepWatch.Sleeps includes equality. Add exactly one minute and just below one minute to the planned TestInnerRun_OwnerCheck table, leaving SleepWatch's existing accounting contract unchanged.
    origin: lens:guard-reach
---

### [BLOCKING] Missing batten records suppress owner judgment

**Location:** `_lyx/plan/05-reconcile-owner-outcome.md:36`, `_lyx/plan/09-loom-owner-check.md:32`.
**Issue:** Card 5 reads owners only over recorded pairs, and card 9 allows a resume when the report contains no owned-elsewhere entry.
`recordedPairs` in `internal/fabricengine/originreconcilemoves.go:436` includes a pair only when the prime has its batten status file.
A second machine can add an existing task pair and receive its child run records without having that prime-side record.
The start and resume paths in `internal/loomcli/start.go`, `resume.go` and `arm.go` require no batten record, and judge-only reconciliation does not materialize one.
An existing run can therefore pass the new guard despite an explicit foreign owner on origin, violating D3 and the declared `PATTERN-owner-machine`.

### [BLOCKING] Alternate execution verbs bypass the owner guard

**Location:** `_lyx/plan/09-loom-owner-check.md:35`.
**Issue:** The card guards startCmd and resumeCmd only.
`loomCLI.specFor` in `internal/loomcli/arm.go:283` also wires foreground run and step to `loomPreRun` and `loomPreStep`.
`loomPreRun` at line 385 explicitly treats an already-seeded run as a resume, verifies seed identity and brings up reed without consulting machine ownership.
`loomPreStepUnmarked` at line 483 proceeds through bootstrap after the local lock probe, likewise without an owner check.
A matching seed and free local lock reach these paths on the wrong machine; the generic shed routes reuse the same hooks.
The later Finalize check cannot prevent preceding producers from running, contrary to the declared `PATTERN-owner-machine`.

### [BLOCKING] Unpublished ownership is reported as confirmed

**Location:** `_lyx/plan/03-board-owner-writes.md:52`.
**Issue:** The card explicitly returns locally written owners as recorded under both skip flags, even though neither path publishes them.
These are production environment inputs in `internal/boardengine/skipenv.go`, and `Bolt.PushRecorded` in `internal/fabricengine/bolt.go:85` returns success immediately when either skip is set.
Cards 8 and 16 then consume the setter's success to drive or resume work.
D3 requires the owner write's push, D5 requires publication before resumption, and the declared `PATTERN-owner-machine` requires the post-write reread from the Bolt just pushed.
Ordinary board-write skip behavior cannot satisfy that protocol.

### [BLOCKING] The new CLI module lacks its required pattern exception

**Location:** `_lyx/plan/16-adopt-module.md:52`.
**Issue:** `pattern/PATTERN-cli-cobra.md:27` requires a module CLI to import its corresponding engine and then enumerates deviations, including cleanupcli.
Card 16 explicitly creates adoptcli without an engine, but neither it nor card 17 updates that exception list.
The cleanup analogy does not register another exception.
D5 licenses the engine-free architecture, so the missing companion pattern change is the defect; an adoptengine would contradict the chosen design.

### [BLOCKING] The retry test cannot exercise its stated lock scenario safely

**Location:** `_lyx/plan/03-board-owner-writes.md:51`, `_lyx/plan/03-board-owner-writes.md:58`.
**Issue:** The prescribed fixture holds a push lock across a retry intended to return a deferred-push refusal.
`Bolt.pullFirst` in `internal/fabricengine/bolt.go:201` blocks acquiring that lock before the write, while `retryBoltPush` in `internal/fabricengine/pushclassified.go:542` defers on the board write lock and blocks on the push lock.
Holding the named lock therefore does not produce the named deferral and can prevent the test from reaching the setter's retry at all.
The card additionally prescribes a one-second production retry and a real test of that branch without specifying an injectable wait.
`pattern/PATTERN-test-speed.md` requires such timing to be a parameter or struct field and forbids tests in every tier from waiting out production durations.

### [MEDIUM] Past-merge recovery stops before its promised push failure

**Location:** `_lyx/plan/12-finalize-owner-checks.md:42`.
**Issue:** Replacing step 1b3 leaves two surrounding obstacles intact.
First, `Finalize.Call` in `internal/landingshed/finalize.go:199` calls holdParent before ownerChecks; holdParent calls the moved-parent check, whose `readParentTips(true)` fails on an unreachable origin before the exemption is considered (`parentguard.go:123`, `internal/fabricengine/parenttips.go:46`).
Second, after the exemption, Call still repeats merge-in and parentHandle.Merge.
The latter checks upstream divergence before its already-up-to-date probe (`internal/fabricengine/merge.go:388` and `mergeguards.go:190`).
When the local old-owner squash and the adopter's landed squash diverge, that guard stops the retry before pushParent, the only place card 12 adds adopterFirstRepair.
D6 and D7 require the remaining push to fail visibly and, for the adopter-first case, name the squash repair.
Tests that fake only Reconcile or the merge result do not establish that recovery path.

### [MEDIUM] Claim wiring misses existing unarmed callers

**Location:** `_lyx/plan/08-batten-owner-check.md:48`.
**Issue:** Card 8 initializes ownerSeams only in Command and armAt, explicitly leaving an unarmed receiver zero, but makes wire's CreateWorktree callback call c.owner.Set.
`wireForHub` in `internal/battencli/lifecycle_integration_test.go:105` and the direct caller at line 237 construct receivers and call wire without either initializer.
`internal/battencli/recordleak_integration_test.go:118` does the same and then steps the real Worktree-Create row.
These callers reach the new callback with no setter, causing a nil call unless an additional fallback is specified.
Changing only TestWire_CreateWorktreeClaimsTheBoardEntry does not cover the existing callers.

### [MEDIUM] Skipped entries become lost claims

**Location:** `_lyx/plan/03-board-owner-writes.md:53`.
**Issue:** Line 47 skips both claim and owner writes for an absent or done entry, but line 53 reports each requested slug whose recorded owner differs from Machine as Lost.
An absent slug has no recorded owner and therefore satisfies that latter condition.
Card 8 refuses every Lost slug while explicitly requiring an absent board entry to pass with nothing written.
The setter and its consumer thus disagree on the result of the same supported input.

### [MEDIUM] Public support surfaces are bundled into adoption

**Location:** `_lyx/plan/16-adopt-module.md:16`.
**Issue:** The card creates ResumeInWindow, DropLocal and CloneSecond in three different packages alongside the adoption command.
These are independently reviewable public surfaces: an execution entry point, a destructive fabric operation with mutation registration, and a shared two-clone fixture.
The granularity rubric requires public or cross-package surfaces to have their own cards even when only one card consumes them.
DropLocalResult belongs with DropLocal; private adoption helpers belong with adoption.
This is not a request for one card per literal symbol.

### [LOW] The wake threshold excludes equality

**Location:** `_lyx/plan/10-innerrun-owner-check.md:40`.
**Issue:** D6 and the planned test at line 51 require a sleep of at least one minute to trigger the check.
`SleepWatch.Observe` in `internal/pauserecord/sleep.go:24` records a sleep only when the wall-minus-awake gap is strictly greater than SleepTolerance.
`TestSleepWatch` in `internal/pauserecord/sleep_test.go:28` explicitly pins equality as no sleep.
Consuming only newly recorded SleepWatch intervals cannot reach the card's exactly-one-minute case.

The focus checks found no additional forward-reference ordering defect in cards 2–6 or 8–13, and no additional signature-arrow defect among the named edits.
Takeover, the Finalize exemption and the nil seams have stated bounds; their concrete implementation-path gaps are recorded above.
Cards 7, 10, 14 and 16 did not establish a duplicate public-surface assertion warranting a separate test-economy finding.
Cards 15–17 are licensed by D8, D5 and D9 respectively, subject to F4 and F9.
Card 17's shortened entry, full background text, byte-neutral wording changes and Pattern-Departure trailer follow the explicit Added-after-Discussion authorization.

The board-entry lookup and executable validation were not run because this round restricts evidence gathering to file reads.
No finding relies on unqueried board content.
The fixes above change existing card intent or use insert-card for structural changes; the rubric expressly exempts card insertion and renumbering from overlay validation.
No target files were changed and no tests were run.

## Rejected

- The fork's orch callback construction-order finding assumes Command copies the callback when constructing the Cobra tree; Command already assigns runtime fields inside PersistentPreRunE, where the root-installed callback is available, and card 13 does not require an earlier snapshot.
- A general nil-seam bypass finding is unsupported because the planned production constructors install the seams; the concrete unarmed wire callers are covered by F7.
- A separate first-prime-claim race finding would disregard card 8's explicit lost-claim reread and test requirement; the setter must implement that requirement, but the current text does not establish another intentional takeover path.
- Private helper tests were not rejected solely for their names: fake surface seams do not necessarily cover the helper's real owner mapping, pause-record selection or origin integration.
- The shortened PATTERN overview line is expressly authorized by the decision record and retains the full declared invariant in the background file, so its abbreviation is not a separate violation.
- The accepted offline-resume and already-merged Finalize exceptions are not themselves defects; F3 concerns an unpublished owner write, and F6 concerns failure to carry out the specified continuation.
