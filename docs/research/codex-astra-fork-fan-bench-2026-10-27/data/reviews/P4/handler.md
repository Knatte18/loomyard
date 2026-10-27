---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/13-wake-and-orch-owner-pause.md:36
    summary: Orch captures the owner-pause callback before the root installs it
    fix: >-
      Change card 13's Intent so Command installs a closure that looks up the
      process's owner-pause callback when start executes, after the root pre-run,
      rather than copying its current value during command construction. Preserve
      the unwired warning. Extend the already targeted TestStart_ReconcilesTheHubFirst
      with construction-before-installation followed by invocation, asserting that
      the subsequently installed callback pauses the foreign-owned slug.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: handler
    location: _lyx/plan/03-board-owner-writes.md:52
    summary: Board skip settings let ownership changes succeed without publication to Bolt
    fix: >-
      Replace card 3's skip-settings paragraph with an explicit rule that SetOwners
      always performs its required pull, commit and successful push, irrespective
      of BOARD_SKIP_GIT and BOARD_SKIP_PUSH; ordinary board writes keep their
      existing settings. Change the already planned TestSetOwners skip row to
      assert publication with those settings present. In card 8, make the existing
      TestWire_CreateWorktreeClaimsTheBoardEntry use its fake Set to assert the
      machine, slug and claim request; leave real commit-and-push coverage with
      TestSetOwners instead of requiring a real setter to succeed over a non-git
      fixture. These are Intent/test-case changes to existing targets.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/03-board-owner-writes.md:47
    summary: A first-act prime claim can overwrite an owner discovered by the setter's pull
    fix: >-
      Amend card 3's SetOwners Intent to check prime ownership inside the locked
      write after PullThenCommitWritten has pulled. Without Takeover, set the prime
      only if it is still unowned; retain another machine's existing prime claim
      and return it as Lost for prime. Add a TestSetOwners row with an initially
      primeless local clone and an already claimed remote prime, asserting no
      overwrite and the retained owner. Card 8's claimPrimeOwner then reads and
      accepts that retained claim before judging the task.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/12-finalize-owner-checks.md:42
    summary: The parent-tip fetch prevents the past-merge offline carve-out from being reached
    fix: >-
      Revise card 12's Finalize.Call Intent so it still acquires the parent lock,
      but evaluates the local ParentHoldsTaskWork result before the
      network-dependent moved-parent check. For a confirmed completed local merge,
      bypass that fetch-dependent check and follow the prescribed reconcile and
      parent-push path; otherwise retain the moved-parent guard and owner checks.
      Keep local-read failures on the stated halt path. Extend the already planned
      TestFinalize_OwnerChecks with a valid merge record, a wired parent guard,
      pushes enabled and an unreachable parent-tip fetch, proving the completed
      merge reaches the push instead of failing at the upstream guard.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/16-adopt-module.md:55
    summary: A standing hub pause blocks adopt before its promised preserve-and-report behavior
    fix: >-
      Specify in card 16's hubDeps/adopt Intent a temporary network exemption for
      the exact standing hub entry, identified by scope and paused time, during
      adoption's reconcile, pair materialization and owner publication only.
      Restore it on every exit and before any run or child resume; leave the pause
      record unchanged and report its existing lyx resume remedy. State this
      boundary beside the hub-pause allowance. Extend the already planned adopt
      integration test to install the actual pause network gate and retain a hub
      entry, proving adoption publishes ownership and returns with that entry
      standing and ordinary subsequent network calls still held.
  - id: F6
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/03-board-owner-writes.md:53
    summary: An intentionally skipped absent entry is also classified as a lost owner write
    fix: >-
      Restrict card 3's post-push Lost classification to entries eligible for an
      owner write or explicitly refused because another owner stands. Deliberately
      skipped absent entries appear in neither Recorded nor Lost. Add an
      absent-slug row to the already planned TestSetOwners asserting no entry,
      claim or Lost result, so card 8's real setter path preserves its stated
      absent-entry pass.
  - id: F7
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/12-finalize-owner-checks.md:40
    summary: The new optional seams are missing from the shared nil-seam compatibility policy
    fix: >-
      Using lyx loom insert-card, insert one prerequisite compatibility card before
      the current innerrun-owner-check card, targeting internal/testkit/envkit#nilLegal.
      It adds exactly InnerRun.OwnerCheck, InnerRun.PauseOwned,
      InnerRun.ParentHoldsTaskWork, InnerRun.Restepped,
      Landing.ParentHoldsTaskWork, Landing.ParentOwner,
      Landing.OriginParentHoldsWork and Landing.ParentCommitsOriginLacks to the
      documented optional-seam policy. Its Intent states that these eight names
      alone may remain nil in unused/test environments, with defaults supplied by
      the subsequent seam cards; it does not exempt the whole InnerRun or Landing
      subtree. Existing TestFullEnv_FillsEverySeam and TestWire_EverySeamFilled
      remain the compatibility checks. Renumber through the insertion tool.
  - id: F8
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/16-adopt-module.md:16
    summary: The adopt card bundles three independently reviewable cross-package APIs
    fix: >-
      Split the existing adopt-module card with lyx loom insert-card: put
      battencli.ResumeInWindow, fabricengine.DropLocal and hubforge.CloneSecond in
      separate prerequisite cards before the adopt orchestration card. Keep
      DropLocalResult, TestDropLocal and the destruction registry/header changes
      with DropLocal; keep each other API's supporting implementation and coverage
      with its own card. Leave adopt's private request/result/dependency helpers
      bundled with adopt and retain its integration scenarios there. Update the
      Card Index and numbering through the insertion tool, preserving all scope.
  - id: F9
    severity: LOW
    class: consistency
    origin: handler
    location: _lyx/plan/16-adopt-module.md:64
    summary: The child-resume subprocess contradicts the decided in-process module boundary
    fix: >-
      Change card 16's ResumeChild wiring Intent to invoke loomcli.RunCLIIn with
      the task anchor, a captured output writer and the resume argument, translating
      its exit/envelope into the slug outcome. Remove the instruction to spawn a
      separate lyx loom resume process. Keep the already planned owner-halted-child
      cases through this in-process wiring, including its owner and pause checks.
---

The decision record is the answer key. In particular, D2 permits first-act prime
claims, D3 requires successful publication for an owner write, D4 requires orch to
pause foreign-owned local runs, and D5–D7 prescribe the adoption and landing
exceptions. The added-after-Discussion entry authorizes PATTERN compression.
BLOCKING findings below break a primary production ownership guarantee; MEDIUM
findings identify concrete execution, recovery or card-structure defects; LOW
identifies a narrower implementation-contract mismatch.

### [BLOCKING] Orch captures an uninstalled pause callback

**Location:** `_lyx/plan/13-wake-and-orch-owner-pause.md:36`

**Issue:** The plan says `Command` copies the installed callback, while the root
pre-run installs it. `cmd/lyx/main.go:119` defines that deferred pre-run, but
`main.go:161` calls `orchcli.Command()` while building the tree. The receiver is
constructed at `internal/orchcli/cli.go:112`. A fresh process consequently captures
nil, later warns about an unwired pause and launches without pausing the run. The
recommended rerun repeats the same ordering. This fails D4. Guard-reach and
way-forward independently reported the same defect; it is retained once.

### [BLOCKING] Unpublished ownership is reported as successful

**Location:** `_lyx/plan/03-board-owner-writes.md:52`

**Issue:** The plan explicitly makes both board skip settings return locally
written owners as recorded, despite omitting the push and even the pull. Card 8
trusts that result before driving a claimed run, and card 16 trusts it before
lifting owner pauses and resuming adopted runs. These environment variables are
ordinary production inputs (`internal/boardengine/skipenv.go:13`), not a test-only
seam. D3 says an owner write needs its push; D5 requires publication before resume.
An existing setting for ordinary board writes does not license bypassing those
new guarantees. The plan's real-setter unit fixture is the reason given for the
bypass, and can instead use the setter seam already prescribed on card 8.

### [MEDIUM] First-act prime claims are not checked after pulling

**Location:** `_lyx/plan/03-board-owner-writes.md:47`

**Issue:** Card 3 protects task slugs without `Takeover`, but unconditionally sets
the prime when `Prime` is requested. Card 8's resume claims a prime missing from
the local board before reconciling. `internal/fabricengine/bolt.go:145` pulls before
calling the write closure. Thus a stale clone can discover another machine's
prime claim during that pull and immediately overwrite it; there is no competing
push for replay to reject. D2's first-act claim is not permission to take an
already-owned prime. The post-pull condition must cover the prime as well as tasks.

### [MEDIUM] Finalize's earlier guard intercepts the offline exemption

**Location:** `_lyx/plan/12-finalize-owner-checks.md:42`

**Issue:** The new check replaces step 1b3, after `holdParent` in
`internal/landingshed/finalize.go:199`. That helper reaches `parentGuard.moved`,
which calls `readParentTips(!pushSkipped)` and returns its error
(`internal/landingshed/parentguard.go:126`). Production `ReadParentTips` fetches
first and returns a fetch failure before reading local tips
(`internal/fabricengine/parenttips.go:44`). With a valid merge record and origin
unreachable, an already-merged Finalize never reaches the new exemption or the
parent push. D7 expressly requires that re-step to reach its push and record the
failed push. A fake unreachable reconcile with no wired parent guard does not
exercise this path.

### [MEDIUM] Adopt waits behind the hub pause it promises to preserve

**Location:** `_lyx/plan/16-adopt-module.md:55`

**Issue:** Card 16 promises adoption while a hub pause remains standing, including
the D5 hub-paused `--drop-local` case and a returned standing-pause remedy. The
root installs a network gate (`cmd/lyx/main.go:129`;
`internal/tendercli/gate.go:29`). `pauserecord.Network` waits behind a covering hub
entry, using a background context, before doing remote work
(`internal/pauserecord/hold.go:243`). No adoption exemption is planned, so its first
reconcile cannot complete until someone lifts the pause. The fake-deps rows bypass
this upstream check. Wake reconciliation already supplies an explicit exact-entry
exemption at `internal/hibernatecli/deps.go:103`; adoption needs its own bounded
handling to implement the decided behavior.

### [MEDIUM] Absent-entry success conflicts with the Lost contract

**Location:** `_lyx/plan/03-board-owner-writes.md:53`

**Issue:** Card 3 deliberately writes no owner for an absent entry, then puts each
requested slug whose recorded owner differs from `Machine` in `Lost`. Card 8
refuses every such Lost slug and recommends adoption; card 16 refuses unknown
slugs. The real setter therefore contradicts card 8's absent-entry pass and D3's
explicit no-entry behavior. The existing skip behavior is pinned by
`internal/boardengine/board_test.go:661`, including the absent-slug row at line 674.
The proposed fake-setter row does not settle the real setter's contradictory rule.

### [MEDIUM] Optional-seam changes omit their reflection consumers

**Location:** `_lyx/plan/12-finalize-owner-checks.md:40`; also card 10's new seams

**Issue:** Nil defaults do not make the repository's seam scanner accept nil.
`internal/testkit/envkit/envkit.go:44` maintains an explicit `nilLegal` path map;
`NilSeams` and `walkNil` at lines 502 onward consult that map, not documentation.
The new InnerRun and Landing functions appear in neither that map nor any planned
update to it. `TestFullEnv_FillsEverySeam` (`envkit_test.go:14`) rejects the new
nils. Batten's `TestWire_EverySeamFilled`
(`internal/battencli/wire_seams_test.go:99`) also rejects new unused Landing seams,
and rejects the new InnerRun seams before card 11 wires them. The affected-set
finding is confirmed and expanded to the shared policy that handles both cards.
This is a missed source-inspection consumer, not a recheck of plan syntax.

### [MEDIUM] Adopt's prerequisites need their own cards

**Location:** `_lyx/plan/16-adopt-module.md:16`

**Issue:** This card creates a public batten resume facade, a destructive fabric
operation with its own result, tests and guard registrations, and a reusable hub
clone fixture, alongside the adoption CLI. These are independently reviewable
cross-package surfaces, not private helpers inseparable from `adopt`. The rubric's
granularity rule expressly gives such surfaces their own cards even when only one
card initially consumes them. The private adoption helpers should remain bundled.

### [LOW] Child resume crosses the wrong module boundary

**Location:** `_lyx/plan/16-adopt-module.md:64`

**Issue:** D5 and card 16's opening paragraph say adoption calls each owning
module's function in-process. The child-resume paragraph instead prescribes a new
`lyx loom resume` subprocess. An existing in-process entry point is available:
`internal/loomcli/cli.go:528`, `RunCLIIn`, executes the module with a supplied cwd
and captured output. The subprocess is a direct contradiction of the decided
boundary, independent of whether its logging is correct.

## Rejected

- Forward-symbol suspicion across cards 2–6 and 8–13: the owner record/read API,
  reconcile reader, owner-pause label and shared pause function are introduced
  before their consumers. `InstallOwnerPause` is introduced and consumed on card
  13; its defect is invocation ordering (F1), not a missing earlier card.
- Missing re-sign arrows on cards 3, 6, 10, 12 and 16: the described changes add
  fields or bodies, or declare new functions; they do not prescribe changed
  signatures for the listed existing functions. Card 5 supplies its
  `classifyEntries` arrow. No additional arrow finding is justified.
- Duplicate owner tests merely because they reuse pause/status/InnerRun harnesses:
  the existing assertions do not cover owner labels, owner-triggered lock release,
  foreign-owner status results or adoption. Shared setup is not duplicated behavior.
- Card 17's short PATTERN entry and wording cuts as unauthorized scope: the
  added-after-Discussion decision expressly authorizes the short form, full
  background text and rule-preserving cuts. The card also prescribes the required
  `Pattern-Departure:` trailer. Compression itself is not a finding.
- Nil owner seams as inherently unbounded production bypasses: the cards describe
  their compatibility use and production assembly. The concrete callback-order
  defect and missing compatibility registry are F1 and F7; nil defaults alone
  are not separate findings.
- The repeated orch callback report from way-forward is merged into F1 rather
  than counted twice.

Review was read-only apart from this file. No tests, live board command or plan
validator were executed under the instruction to read files and run nothing.
The decision record remained the sole answer key; no support log or prior-round
artifact was read. Proposed fixes change Intent on existing targets, or insert
cards through the required tool; insertion/renumbering proposals are explicitly
exempt from the overlay procedure.
