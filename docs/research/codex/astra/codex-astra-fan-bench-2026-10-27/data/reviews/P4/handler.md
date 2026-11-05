---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: handler
    location: _lyx/plan/13-wake-and-orch-owner-pause.md:36
    summary: Orch captures the owner-pause seam before the root installs it
    fix: In card 13, resolve the installed pause function during orch's PersistentPreRunE, after the root pre-run, rather than copying it during Command construction. Keep the nil warning for genuinely unwired receivers. Extend the planned TestStart_ReconcilesTheHubFirst coverage with construction before installation followed by execution, asserting that the installed pause runs before strand startup.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: handler
    location: _lyx/plan/03-board-owner-writes.md:52
    summary: Board skip settings turn an unpushed owner write into a successful claim or adoption
    fix: Change card 3 so SetOwners always performs its pull, commit, push and post-push read regardless of BOARD_SKIP_GIT or BOARD_SKIP_PUSH. Replace its skip-success test row with rows proving both settings cannot report ownership recorded without the required push. Update card 8's planned real-setter claim test to use a board with a bare remote instead of relying on BOARD_SKIP_GIT; its fake-setter rows can remain isolated.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/03-board-owner-writes.md:47
    summary: A first-act prime claim can overwrite an owner discovered by the setter's pull
    fix: In card 3, apply the non-Takeover protection to Prime inside the post-pull write callback too. A nonempty prime owner different from Machine must remain unchanged and be reported as prime in Lost; only Takeover may replace it. Extend TestSetOwners with a locally unowned board whose remote already names another prime owner, asserting that a non-takeover prime claim preserves and reports that owner.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/12-finalize-owner-checks.md:42
    summary: Finalize's earlier parent fetch intercepts both prescribed fully offline branches
    fix: Revise card 12's Finalize.Call ordering so it takes the parent lock, evaluates the local past-merge predicate and runs the owner checks before the parent-moved check can fetch. For a proven past-merge continuation, preserve the lock and avoid the remote parent-tip precondition that would stop it before its push; for a pre-merge run, retain the parent-moved protection after the owner checks. Extend the already planned Finalize owner tests with a wired parent guard whose fetch fails, covering both the owner-prefixed pre-merge halt and the past-merge attempt to push.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:affected-set
    location: _lyx/plan/05-reconcile-owner-outcome.md:42
    summary: Pull can replay foreign-owned records before a push bypasses the guarded retry
    fix: Remove card 5's claim that every push following Fabric.Pull meets retryWeftPush. Insert a follow-up card after owner-reader wiring, using lyx loom insert-card, to guard Fabric.settleUnmovedWeft before replay with the installed owner reader over fetched Bolt bytes. An owned-elsewhere pair must retain its local commits without replay and name adopt as the way forward. Extend the existing TestPull_WeftDivergedWarpAdvancesCleanly scenario to cover that owner and a subsequent push, asserting that the local records never become a replayed fast-forward onto the adopter's tip. Keep the owned-here pull behavior.
  - id: F6
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/12-finalize-owner-checks.md:45
    summary: The adopter-first repair appends recovery advice that contradicts the existing push error
    fix: Change card 12 so a positively identified adopter-first failure replaces the normal push recovery clause with the reset-or-rebase repair and subsequent fabric removal, retaining diagnostic facts but not the instruction to merge and push. Keep today's recovery only when the adopter-first predicate is false or unreadable. In the planned failed-push rows, assert that the adopter-first halt contains no merge-origin or retry-push recommendation.
  - id: F7
    severity: LOW
    class: consistency
    origin: lens:affected-set
    location: _lyx/plan/10-innerrun-owner-check.md:40
    summary: SleepWatch does not report the exact one-minute sleep that card 10 requires
    fix: In card 10, derive the owner-check trigger from consecutive wall-minus-awake deltas with an inclusive comparison to SleepTolerance, storing those readings in childWait, rather than relying solely on new intervals from SleepWatch. Leave the shared watch's existing idle-accounting semantics unchanged. Make the planned TestInnerRun_OwnerCheck rows explicitly exercise exactly one minute, just below it and just above it.
  - id: F8
    severity: LOW
    class: scope
    origin: lens:way-forward
    location: _lyx/plan/16-adopt-module.md:54
    summary: Two adopt argument refusals have no specified correction
    fix: In card 16, specify that simultaneous local-choice flags name a rerun with exactly one of --keep-local and --drop-local, and that an empty selection names lyx adopt <slug>, lyx adopt --prime or lyx adopt --all. Require the already planned TestAdopt argument rows to assert those corrections and that no reconciliation or mutation ran.
---

### [BLOCKING] Owner pause is installed too late for orch's receiver

**Location:** _lyx/plan/13-wake-and-orch-owner-pause.md:36

**Issue:** `cmd/lyx/main.go` constructs `orchcli.Command()` in `newRoot` before any `PersistentPreRunE` executes. Card 13 copies the process seam at construction but installs it in that later hook. A fresh CLI process therefore captures nil, warns and starts without performing D4's required owner pauses. Repeating the recommended command repeats the same wiring failure. The injected-receiver tests described in the card would miss it.

### [BLOCKING] Local-only ownership is treated as published

**Location:** _lyx/plan/03-board-owner-writes.md:52

**Issue:** `ApplySkipEnv` reads ordinary process environment variables, not a test-only facility. The card explicitly lets those variables skip publication and still return the owners as recorded. Cards 8 and 16 then permit a claim/resume or adoption to proceed without origin recording the owner. D3 requires an owner write's push, and D5 requires publication before any resume. An existing convenience for other board writes does not authorize this exception to the answer key.

### [MEDIUM] Prime first-claim protection is missing inside the transaction

**Location:** _lyx/plan/03-board-owner-writes.md:47

**Issue:** Card 8's resume can request a prime claim from stale local Bolt. `Bolt.PullThenCommitWritten` pulls before invoking the write callback (`internal/fabricengine/bolt.go:145`). The callback then unconditionally sets the prime owner, while card 3's Takeover guard covers slugs only. An owner discovered by that pull is overwritten on top of origin's tip, so replay's conflict rule cannot save it. D2 permits a first claim only when the prime is unowned; takeover belongs to adopt.

### [MEDIUM] Offline Finalize is stopped before its new checks

**Location:** _lyx/plan/12-finalize-owner-checks.md:42

**Issue:** `Finalize.Call` calls `holdParent` before the step 1b3 replacement. That calls `parentGuard.moved`, which invokes `ReadParentTips(true)` when pushing is enabled. A failed fetch returns immediately (`internal/landingshed/parentguard.go:126`, `internal/fabricengine/parenttips.go:44`). With the code origin unreachable, neither the owner-prefixed halt nor the already-merged continuation described by D7 is reached. A fake unreachable reconcile report with an unwired parent guard is insufficient evidence for either branch.

### [MEDIUM] Pull bypasses the replay protection

**Location:** _lyx/plan/05-reconcile-owner-outcome.md:42

**Issue:** `Fabric.settleUnmovedWeft` calls `replayWeft` on divergent records (`internal/fabricengine/pull.go:379`). After that local replay, the initial push can succeed; `pushClassified` calls its retry only after a rejected push (`internal/fabricengine/pushclassified.go:118`). Thus the card's stated reason for leaving Pull unchanged is false, and its promised prevention of replayed old-owner records reaching the adopter is incomplete. The affected-set and guard-reach reports describe the same defect and are consolidated here.

### [MEDIUM] Duplicate-landing repair retains the wrong alternative

**Location:** _lyx/plan/12-finalize-owner-checks.md:45

**Issue:** `Finalize.pushParent` embeds the push error in its halt reason. A classified non-fast-forward code push supplies `codeDivergedWayForward`, which instructs a merge of origin followed by a push (`internal/fabricengine/failedpush.go:249`). Appending the new repair leaves that instruction standing. D6 explicitly requires dropping the duplicate squash, never merging it. The note must select the applicable recovery rather than offer both contradictory actions.

### [LOW] Inclusive sleep threshold is not implemented by the named helper

**Location:** _lyx/plan/10-innerrun-owner-check.md:40

**Issue:** D6 and the card say at least a minute. `SleepWatch.Observe` uses `gap > SleepTolerance`, and `TestSleepWatch` explicitly asserts that equality records no sleep. Merely looking for a new interval cannot trigger the required exact-boundary owner check. This is a behavioral mismatch, not a missing dependency-list entry.

### [LOW] Argument errors need their runnable corrections

**Location:** _lyx/plan/16-adopt-module.md:54

**Issue:** The plan names recovery for unknown, note and done slugs but only says to refuse conflicting local-choice flags and an empty selection. The decision record's Constraints require the new refusals to name a way forward and have a reaching test. Both inputs are reachable before reconciliation, so their corrections can be specified directly in the existing argument table.

## Rejected

- Affected-set's CLI seam-signature inventory finding: card 16 already prescribes both required signatures. The existing static lists also omit existing modules; their completeness is not a stated acceptance condition. Adding this inventory coverage is not necessary to establish a violation of the requested behavior.
- Guard-reach's pull/retry finding: retained once as F5, with affected-set provenance.
- A card-order defect involving OwnerRequest, the owner reader, the owner label or InstallOwnerPause: their creation precedes their consumers, or shares the same card. Root runtime initialization order is a separate defect, F1.
- A blanket objection to nil owner seams: D4 explicitly permits a nil reader, and the cards describe test defaults and production wiring. The concrete production failure is F1, rather than the existence of test defaults.
- Card 15's same-machine window reopening and card 17's PATTERN shortening: D8 expressly licenses the former; the Added after Discussion decision expressly licenses the latter, including preserving the full text in the background. The declared-text trailer and no-growth requirement are carried.
- Test duplication on cards 7, 10, 14 and 16: the owner-label, ownership-transition and takeover cases add behavior; no existing public-surface test was established to already cover those owner-specific assertions. Card 7 extends existing tests rather than creating new functions.

## Review basis

The decision record is the answer key; its carry-over claims are not authority. No live rework directory was present. BLOCKING is used for a production path that systematically fails a core ownership requirement; MEDIUM for concrete transactional, branch-reachability or recovery contradictions; LOW for the threshold and argument-message gaps. No deterministic plan-format check is re-reported.

The fixes above change Intent and already-planned coverage, except F5's expressly inserted follow-up card. They do not propose direct target-list or Verify edits requiring an overlay run. No plan or source file was changed and no validation program or test was run.

Reading covered the current overview and cards 1–17, the decision record and focus, the writer stencil and relevant format sections, PATTERN and the relevant background rules, and definitions/call paths for root/orch construction, board skip settings and pull-first writes, batten resume, loom pause preflight, Finalize/parent guards, parent-tip reads, failed-push recovery, pull replay, initial push dispatch, SleepWatch and its test, CLI seam signatures, and InnerRun's lock harness. All three supplied lens reports were read and adjudicated. The task board entry was not queried because this round restricts evidence gathering to file reads; the source reading was targeted rather than an exhaustive expansion of every named symbol.
