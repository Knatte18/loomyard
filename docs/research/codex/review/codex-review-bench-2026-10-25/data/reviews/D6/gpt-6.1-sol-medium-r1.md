---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:91
    summary: The exception for a run-lock holder waiting at the network gate has no observable predicate
    fix: In D5, settle and record how the actual run-lock holder and its current gate-wait state are identified. Record the evidence from the existing lock and pause mechanisms, decide any additional observable state this requires, and require a matching live process identity and current wait state before ending the holder. Keep refusal for unknown or local-working holders and add acceptance coverage distinguishing those cases.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:59
    summary: Ending a recorded group leader misses its children in separate process groups
    fix: Change D3 to snapshot descendants with start times for recorded group leaders as well as non-leaders before signalling. End the identified descendants and their separate groups within the shared grace period, retaining the caller and ancestor exclusions and identity checks. Record the separate-group behavior of gitexec remote commands and StartTied as evidence, and add an acceptance scenario for an owned detached push with a live remote git child in another group.
  - id: F3
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:124
    summary: The unregistered orphan fallback does not enforce the protections promised by the reap bound
    fix: In D8, require the unregistered-process fallback to establish a task owner and apply the same absent-pair and non-running-batten checks as the registered path. Define how that owner is recovered from the deleted working-directory path and exclude hub-level or unclassifiable processes from automatic reaping. Align the Bound and acceptance criteria with that predicate.
  - id: F4
    severity: MEDIUM
    class: decision
    location: _lyx/discussion/decision-record.md:75
    summary: The reed down authorization and run-lock policy are ambiguous
    fix: Replace D4's shorthand reference to D5 with an explicit reed-down policy. Define the source and meaning of not running, whether reed down performs origin reconciliation and how stopped or unreachable-origin outcomes are handled, and whether D5's run-lock protection must precede End on this path. Preserve the local-working-step protection explicitly and add acceptance criteria for a paused step doing local work and Finalize Done while batten is still running.
  - id: F5
    severity: LOW
    class: design
    location: _lyx/discussion/decision-record.md:145
    summary: The empirical justification for the sixty-minute fork bound has no recorded evidence
    fix: Research the fork-duration observation D10 relies on and record the observed duration and its identifiable source in D10 or Constraints. If sixty minutes is instead an autonomous policy choice, state that choice and its rationale explicitly and remove the unsupported claim that it covers the longest fork seen.
  - id: F6
    severity: LOW
    class: consistency
    location: _lyx/discussion/support-log.md:77
    summary: Two rejected alternatives cite decision numbers from before renumbering
    fix: In support-log.md Rejected alternatives, change the process-table fallback reference from D7 to D8 and the shuttle arrival-rule reference from D10 to D11.
---

### [BLOCKING] The gate-wait exception cannot yet be evaluated safely

**Location:** _lyx/discussion/decision-record.md:91

**Issue:** D5 permits ending a held run lock only when its holder is a step waiting at the network gate. This distinction protects a paused merge or other local work, so it is a fact the design rests on. The record does not establish how it is read. In the actual code, `pairteardown/loop.go` infers a possible holder from the newest unfinished step record, otherwise reporting an unknown process. `pauserecord/hold.go` writes its network-operation marker only after `WaitUncovered` returns; that marker describes an operation in flight, not a process waiting at the gate. Pause coverage alone cannot distinguish the protected local-working case. Leaving this detection to Plan would defer the research that determines whether the exception is safe.

### [BLOCKING] Separate child groups escape End

**Location:** _lyx/discussion/decision-record.md:59

**Issue:** D3 snapshots descendants only for a non-leader record and signals only the recorded group for a leader. D2 explicitly records detached pushes, which lead their own groups. However, `gitexec/gitexec.go` configures remote git commands through `proc.ConfigureGroupKill`, and `proc/procgroup_linux.go` gives that child a separate group with `Setpgid`. `proc.StartTied` also creates a separate group in `proc/tied_linux.go`. Thus ending the recorded parent's group does not reach these children. D4 states the parent exits immediately on SIGTERM, and D1 then prunes its record; re-reading owner records cannot recover an unregistered git child. A push can continue after pair teardown despite the central ownership guarantee.

### [MEDIUM] The fallback can reap a process whose pair still exists

**Location:** _lyx/discussion/decision-record.md:124

**Issue:** The registered orphan rule requires an absent pair and no running batten record, and the Bound promises these protections for reap generally. The separate unregistered rule requires only a lyx executable and a deleted working directory inside the hub. A process can retain a deleted subdirectory of an existing pair, including one whose batten still runs; a deleted hub subdirectory also establishes no task ownership. The proposed fresh reading and start-time match establish process identity but do not establish orphanhood. The fallback therefore admits processes the Bound says reap never ends.

### [MEDIUM] Reed down does not have an actionable equivalent of D5

**Location:** _lyx/discussion/decision-record.md:75

**Issue:** D4 says reed down uses the same readings as D5 but also introduces not running without defining it. D5 obtains authoritative ended and owned-elsewhere verdicts by reconciling first, handles stopped, distinguishes Finalize Done from a still-running batten, and protects a local-working run-lock holder before ending processes. D4 does not say which of those operations and protections reed down inherits, while saying it ends paused processes at once. D6 expressly allows local work to continue under a pause. One plausible implementation therefore kills that work immediately, while another waits or refuses. These are materially different authorization policies, not missing signatures or call-site enumeration.

### [LOW] The fork-duration premise is unsupported

**Location:** _lyx/discussion/decision-record.md:145

**Issue:** D10 justifies sixty minutes by saying it covers the longest fork seen, without identifying that observation or recording its duration. The operator-view date and commit cited in the same rationale support the no-interruption and next-batch rules, not that empirical maximum. The support log repeats the chosen sixty-minute bound without supplying the evidence. By contrast, D11's claim that the driver never enters Wait is supported directly by the named `internal/loomcli/driverspec.go`, which explicitly documents that behavior; it does not need additional interface research.

### [LOW] Rejected-alternative references point to different decisions

**Location:** _lyx/discussion/support-log.md:77

**Issue:** The process-table fallback points to D7, which now concerns the owned-elsewhere pause; the fallback is in D8. The stopped-background-shell alternative at line 86 points to D10's arrival rule, but D10 now concerns hibernate confirmation and the arrival rule is in D11. The remaining decision-number references in the two reviewed files match their named subjects.

## Rejected

- D5's remove gate exemption, D6's hidden no-wait marker, and D9's pane-descendant kill each state what they bypass or kill and their process or session limits. No additional unbounded-edge finding is warranted for those decisions.
- D8's registered-worktree predicate and deleted-directory observation have existing substrates in `pairteardown.taskWorktreeGone` and `proc.ListProcesses`; D6's ahead-of-upstream predicate also has `gitrepo.HasUnpushed`. Requiring exact signatures or a complete call-site inventory would cross the writer/reviewer symmetry fence. F1 differs because the existing gate does not expose the required waiting state.
- D2 and D6 state departures from the board entry, rather than requesting edits to its body or scope. The Auto-mode assumptions likewise describe the interpretation chosen. No own-board-entry-edit finding applies.
- D2's departure rationale, D3's signalling rationale, and D6's parked-driver and records-replay rationale explain requirements and bounds the plan writer needs. D3 does not carry the named incident-history narrative, and moving these compact rationales would not improve the decision record. The e610f9250 scope exclusion and D5's #741 explanation also identify a settled boundary and the reason local removal must avoid stale remote writes.
- The record explicitly bounds the Windows implementation, the pane-snapshot race and the possibility of already-reparented descendants. These are stated design limits, not facts handed downstream for later discovery.
