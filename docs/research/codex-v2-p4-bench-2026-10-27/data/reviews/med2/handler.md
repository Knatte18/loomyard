---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    origin: lens:guard-reach
    location: _lyx/plan/05-reconcile-owner-outcome.md:36
    summary: Filtering ownership through batten records leaves owned pairs unchecked
    fix: In card 5's Intent, compute ownership and owned_elsewhere outcomes for all relevant local pairs, including the requested run, independently of the batten-record filter used for ended-run handling. Extend its already-targeted TestReconcileOrigin_Moves with a foreign-owned pair lacking a batten status, covering both judge-only run scope and moving hub scope with diverged records. Require an owned_elsewhere outcome and no replay.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: lens:affected-set
    location: _lyx/plan/16-adopt-module.md:52
    summary: The engine-less adopt module is missing its required PATTERN exception
    fix: Resolve the missing adoptcli exception in PATTERN-cli-cobra's package-naming declarations while preserving D5's decision to create no adoptengine. A concrete target amendment is withheld because this round forbids executing the required overlay validation; see Review constraints below.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/10-innerrun-owner-check.md:38
    summary: New seams omit reflective fixture readers and direct wiring callers
    fix: Complete the fixture-consumer inventory for cards 8, 10 and 12 before their respective gates, including envkit's nil-default policy and direct wire callers. In card 8's existing wire target, initialize a zero owner seam before creating the claim closure so direct wiring is safe. Concrete additional fixture targets are withheld pending the required overlay validation, which this round's no-execution rule prohibits.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/09-loom-owner-check.md:16
    summary: The external integration test cannot access its private verdict function
    fix: Make card 9's integration test package and access route consistent with its private runOwnerVerdict subject and fixed-machine argument. The concrete Create-target correction is withheld pending overlay validation under the conflicting no-execution rule; do not leave the external test dependent on an undeclared export.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/03-board-owner-writes.md:47
    summary: A stale first-act prime claim can overwrite an established owner
    fix: In card 3's Intent, apply the post-pull non-Takeover guard to Prime as well as Slugs. Preserve another machine's prime owner and report it in Lost. Extend the already-planned TestSetOwners with an ownerless local Bolt whose origin already records another prime owner, asserting no takeover without Takeover.
  - id: F6
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/08-batten-owner-check.md:50
    summary: The first-act prime claim blocks the promised offline legacy-run resume
    fix: In card 8's resume Intent, determine the reconcile's unreachable outcome before requiring a prime-owner write. On the prescribed offline legacy-run path, read local ownership, warn and resume without writing either owner. Extend TestRunCmd_ResumeDispositions with the combined no-prime-owner, unowned run-held entry and unreachable-origin case, asserting no Set call.
  - id: F7
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/12-finalize-owner-checks.md:45
    summary: Appending the adopter-first repair retains an unsafe merge-and-push remedy
    fix: Change card 12's pushParent Intent so successful adopter-first detection replaces the ordinary recovery instructions, retaining diagnostic details without their merge-and-push remedy. Extend its already-targeted TestFinalize_MergeMarkPushOrderAndVerdict repair rows with a classified non-fast-forward error and assert that only the reset or squash-removal recovery is offered. Preserve the original remedy when origin lacks the work or its fresh read fails.
---

### [BLOCKING] Ownership candidates must not depend on batten bookkeeping

**Location:** _lyx/plan/05-reconcile-owner-outcome.md:36

**Issue:** Card 5 asks the owner reader only about recorded pairs.
`recordedPairs` includes a local pair only when a prime-side batten status exists (`internal/fabricengine/originreconcilemoves.go:435`).
However, `originTargets` includes other local pairs and their branches (`internal/fabricengine/originreconcile.go:563`), and direct `loom start` seeds its own run independently (`internal/loomcli/start.go:510`).
An existing loom run with a foreign owner and no batten status therefore receives no owner outcome, allowing card 9's resume check and card 12's run-owner check to pass.
An adopted idle task with no run record also receives no owner outcome: its diverged records can be replayed in adopt's first reconcile before card 16's conditional local-commit stop can run.
This violates D3–D5 and the declared PATTERN-owner-machine.
This consolidates the guard-reach and affected-set findings about the same candidate filter.

### [BLOCKING] Register the decided engine-less module exception

**Location:** _lyx/plan/16-adopt-module.md:52

**Issue:** Card 16 explicitly creates adoptcli without an engine, as D5 requires.
PATTERN-cli-cobra's background requires a module CLI to import its engine and lists the exceptions individually (`pattern/PATTERN-cli-cobra.md:27`).
Cleanup is listed; adopt is not, and no card updates that declaration.
The defect is the missing architectural declaration, not the absence of an adoptengine.

### [MEDIUM] Enumerate fixture consumers when adding seams

**Location:** _lyx/plan/10-innerrun-owner-check.md:38; cards 8 and 12

**Issue:** Cards 10 and 12 add four optional function fields each but omit the reflective reader `envkit.nilLegal` (`internal/testkit/envkit/envkit.go:46`).
`NilSeams` reports every unexempted nil function (`:502`), so `TestFullEnv_FillsEverySeam` and the batten, loom and darn wiring checks see new failures.
Later production wiring does not repair FullEnv or another recipe's intentionally absent seams.
The same inventory gap appears in card 8: owner seams are initialized through Command or armAt, while its claim closure calls owner.Set.
Existing `wireForHub` and `TestBattenIntegration_RunReconcilesTheHubBeforeAndThePairAfterCreating` construct literal receivers and call wire directly (`internal/battencli/lifecycle_integration_test.go:105`, `:237`), reaching a nil setter.
These are existing consumers omitted from the plan, grouped as one affected-set enumeration finding.

### [MEDIUM] Give the integration test a valid access route

**Location:** _lyx/plan/09-loom-owner-check.md:16

**Issue:** The card declares TestRunOwnerVerdict in the external loomcli_test package, while declaring runOwnerVerdict and its result type privately in loomcli.
Its Intent prescribes exercising that function with a fixed machine argument (`:39`), but supplies no exported test bridge or public injection route.
The other test uses a private receiver inside loomcli and does not provide external access.
The proposed integration test cannot be implemented through the declared surface as written.

### [MEDIUM] Protect the prime inside the locked claim

**Location:** _lyx/plan/03-board-owner-writes.md:47

**Issue:** The card writes prime_owner whenever Prime is set, but states its post-pull non-takeover guard only for task slugs.
Card 8 can request the first prime claim from a stale local board before resume reconciliation.
`PullThenCommitWritten` pulls before invoking the write (`internal/fabricengine/bolt.go:153`), so it can first acquire another machine's established prime owner and then overwrite it.
No conflicting push or replay is needed: the new write is based on the fetched owner.
D2's first-act claim and D5's explicit takeover require this case to preserve the existing prime owner.

### [MEDIUM] Reach the offline fallback before claiming

**Location:** _lyx/plan/08-batten-owner-check.md:50

**Issue:** Resume first claims the prime when none is recorded, but card 3 refuses that write when its pull cannot reach origin.
For an older hub with neither a prime owner nor a recorded owner on its run-held task, this exits before card 8's unreachable-reconcile fallback.
D3 expressly permits that existing run to warn and resume from local Bolt without an owner write.
The named tests cover unowned runs and first prime claims separately, leaving their offline combination untested.

### [MEDIUM] Replace contradictory landing recovery instructions

**Location:** _lyx/plan/12-finalize-owner-checks.md:45

**Issue:** Card 12 appends the adopter-first repair to the existing push failure reason.
`Finalize.pushParent` already embeds the push error (`internal/landingshed/finalize.go:328`), and `PushFailedError.Error` includes its class's recovery instructions (`internal/fabricengine/pushclassified.go:39`).
A code-side non-fast-forward failure instructs merging origin and pushing (`internal/fabricengine/failedpush.go:217`, `:249`).
Appending the new reset/rebase instructions leaves both remedies in the halt note.
D6 explicitly requires dropping the duplicate squash and never merging it, because merging would land the task twice.

## Rejected

- The two owner-candidate findings are one defect and are consolidated as F1, not counted separately.
- Orch installation timing is not a definite defect: Command can copy the installed pause seam inside its existing PersistentPreRunE, after the root hook; the card does not require a construction-time snapshot.
- Card 17's shortened entry is licensed by the decision record's added-after-Discussion decision, and its full declared text remains in the background file.
  The trailer does not independently waive an invariant, but the licensed short form is not itself a finding.
- No forward symbol dependency was established across the focused cards: the owner records, request, reader, pause label and installer are created before their stated consumers or on the same card.
- The focused Takeover, nil-seam and past-merge cases state their bypasses and bounds; their presence alone is not a finding.
- No duplicate existing behavior test was established for the focused cards 7, 10, 14 and 16.
  The fixture and test-access defects above are separate from test redundancy.

## Review constraints

The step-1 instruction says: Read-only analysis in the explore and review steps: read files, run nothing.
That conflicts with executing board get and the required validate-plan --overlay checks for target-changing fixes.
I followed the no-execution constraint: no board command, build, test or plan validator was run, and no target-level correction is claimed as overlay-validated.
F2–F4 therefore identify the missing correction but withhold concrete target amendments; the remaining fixes amend Intent and coverage within targets the cards already carry.
Only this review file was written.
