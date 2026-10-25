---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:55
    summary: The offline path promises ownership protection that a stale local board cannot provide
    fix: Amend D2 to state that an unreachable origin skips fresh ownership and stopped-branch judgments and can therefore cancel local remnants of a remotely owned or stopped run. Bound this explicitly to the named slug's local processes and artifacts, retain refusal for locally known foreign ownership and landed work, and qualify the absolute ownership guarantee accordingly. Add an acceptance scenario with stale local ownership and unavailable origin.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:68
    summary: An interruption after the claim clear makes the remaining cancel steps unreachable on retry
    fix: Revise D2 and D3 to permit completion of pending finalization after records, pair and claim are gone. Specify durable pending-finalization state that survives D6, is considered by the nothing-left refusal, and is cleared only after the pause lift and records-push scheduling have completed; define notice retry behavior. Add an acceptance case interrupted between D3 steps 7 and 8 with --remove and a live slug pause.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:171
    summary: The older-process recovery advice relies on reap handling processes it explicitly excludes
    fix: Replace the older-binary risk's generic tender reap remedy with the researched boundary from internal/tenderengine/doc.go. Record in D4 that unregistered surviving processes in a kept pair and replaced-binary processes require explicit inspection and manual termination through tender status and platform process tools, and qualify the no-process guarantee for these cases. Keep reap advice only for processes its orphan rules actually admit.
---

### [MEDIUM] Offline cancellation lacks an accurate bound

**Location:** _lyx/discussion/decision-record.md:55

**Issue:** D2 permits proceeding when origin is unreachable, using the local board for ownership, but concludes that the verb cannot end a run owned elsewhere. A local board can predate a remote ownership change; the stopped-branch refusal likewise depends on a reconcile verdict unavailable on this path. A warning and failed-push reporting bound neither missed judgment. The existing resume precedent acknowledges proceeding without judging against origin (`internal/battencli/resumereconcile.go`); it does not make local ownership fresh. The attack-surface rule requires this Decision to acknowledge what the fallback lets through and state its actual local bound.

### [MEDIUM] Finalization cannot always resume

**Location:** _lyx/discussion/decision-record.md:68

**Issue:** With `--remove`, after step 7 there can be no pair, records directory, run-form claim or registered live process. If interrupted there, step 8 has not lifted the slug pause or scheduled the records push. On retry, D2's nothing-left refusal fires before either action. This contradicts D3's anywhere-retry promise and the corresponding acceptance criterion; the stale pause can hold the next run at InnerRun, exactly as D8 explains. Merely making each operation idempotent does not make the remaining operations reachable.

### [MEDIUM] Reap does not cover the stated older-binary risk

**Location:** _lyx/discussion/decision-record.md:171

**Issue:** The risk says unregistered older processes not caught by driver/run locks remain for `lyx tender reap`. The current `internal/tenderengine/doc.go` states that reap never ends a process working inside a registered worktree or a pair that exists. Its unregistered-orphan listing also excludes processes whose executable is deleted, which are reported separately as replaced-binary processes. Thus a detached older process in the default kept pair is not repaired by the named remedy. `internal/runproc/doc.go` confirms that End reaches registered roots and their descendants, not every unrelated unregistered process. This is a false recovery premise, not a demand for exhaustive process enumeration.

## Rejected

- D3/D4 lock sequence: no self-deadlock established. D4 step 2 expressly takes batten's prime-side locks, whereas step 5 belongs to the pairteardown phase. The existing `awaitPairRunLock` in `internal/pairteardown/teardown.go` waits on the task location's self-run lock, distinct from the prime slug lock used by batten cleanup. D3 correctly retains the latter. The survivor rule refers to surviving external holders, not the cancel's own acquired locks.
- D1, D4 and D11 attack surface: the prime/slug restriction, explicit mid-turn termination, survivor refusal, preserved-work bound and typed-error-only hand-back state the relevant limits. Skipping a pre-end quiet wait is the chosen cancellation behavior, not by itself a defect.
- D5 snapshot and claim mode: current `RemovePair` snapshots only in Local mode and normally settles the claim. However, Scope explicitly adds a caller-owned-claim removal mode, and D5 explicitly requires snapshot preservation alongside ordinary origin steps. These are requirements for new behavior, not a claim that today's implementation already provides that combination. No missing API-shape finding is warranted.
- Deferred research: D10 names its tracked-only probe and source; D8 names Lift, whose current `sendResumeLine` checks liveness before sending. The index.lock risk is an acknowledged failure mode, not an instruction for Plan to discover whether the design works. The older-process recovery premise is addressed in F3 rather than demanding broad additional research or citations.
- Relocation: Scope, Constraints, Auto-mode assumptions and Open risks are compact exclusions, requirements, choice pointers and operational limitations. No surrounding deliberation warrants relocation at the cost of losing plan input.
- Support-log Turn 3: the historical unreachable-origin refusal is expressly revised in Turn 5, Q10 and Rejected alternatives. The chronology does not misrepresent the final D2 choice.
