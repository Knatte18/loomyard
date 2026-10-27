Guard reachability list:

- Card 3, owner payload mismatch, empty machine, conflicting owner, pull failure, push failure, and deferred push: reachable through the specified setter/upsert inputs.
- Card 5, owned-elsewhere classification and replay suppression: reachable after reading origin’s Bolt; ended precedence and nil-reader behavior are explicit.
- Card 7, owner pause labeling and already-paused handling: reachable; existing `Pause` already checks live entries by scope.
- Card 8, start, claim, resume, step, and window refusals: reachable for an otherwise valid run; the offline legacy-hub resume case conflicts with the preceding prime claim.
- Card 9, loom start/resume ownership refusal: reachable for an existing, uncovered run; an existing pause intentionally reaches `refuseCovered` first.
- Card 10, sleep, hold-end, spawn refusal, and owner-lock release: reachable through the listed clock, coverage, child-status, and spawn-error inputs.
- Card 12, Finalize ownership checks: reachable after summary parsing, status commit, and parent-lock validation; the tests must supply those successful prerequisites.
- Card 13, wake owner pause: reachable; production orch owner pause is not reachable with the prescribed installation/copy timing.
- Card 16, adopt validation, divergent/local-commit stop, live-pair drop refusal, lost write, and remaining-pause branches: reachable with the named inputs, including the hub-only pause seam.

Findings:

1. **BLOCKING — consistency — `_lyx/plan/13-wake-and-orch-owner-pause.md:36`**
   **The orch receiver copies its owner-pause function before the root pre-run installs it.**
   Card 13 says `newRoot`’s pre-run installs `InstallOwnerPause`, while `Command` copies the installed function into `orchCLI.pauseOwned`.
   Actual `newRoot` constructs `orchcli.Command()` when assembling the command tree (`cmd/lyx/main.go:161`); its pre-run executes later (`cmd/lyx/main.go:118`).
   `orchcli.Command` creates the receiver during construction (`internal/orchcli/cli.go:111`).
   Consequently a fresh process copies nil, then the installation updates only the global slot.
   Every production `orch start` takes the unwired-warning arm instead of pausing an owned-elsewhere run, violating D4 and declared `PATTERN-owner-machine`.
   The planned fake-receiver test does not exercise this construction sequence.
   Proposed correction: resolve the installed function during orch’s runtime wiring or invoke it through a runtime lookup; exercise construction followed by pre-run installation and start.

2. **MEDIUM — consistency — `_lyx/plan/08-batten-owner-check.md:50`**
   **The unconditional missing-prime claim can reject the offline legacy-run resume that the next paragraph promises to allow.**
   Card 8 first makes `reconcileResume` claim the prime whenever none is recorded, but line 52 requires an unreachable reconcile on a run-held unowned entry to warn and resume with no owner write.
   On an older hub with neither prime nor task ownership, the preceding `SetOwners` call instead encounters Card 3’s `fetch_failed` refusal (`_lyx/plan/03-board-owner-writes.md:49`) before that offline fallback can succeed.
   D3 explicitly permits an offline resume of a locally run-held unowned task (`_lyx/discussion/decision-record.md:68-72`).
   Existing `reconcileResume` obtains the reconcile report before handling `Unreachable` (`internal/battencli/resumereconcile.go:39-54`).
   Proposed correction: specify that the unreachable-local-owner fallback precedes any missing-prime or task-owner write, and extend the named resume table with an unreachable report plus no prime owner and an unowned run-held entry.

Tool-call manifest:

1. Located actual entry-point and InnerRun functions; read Finalize’s upstream guards and the pause/spawn hold, which the inherited searches had not shown.
2. Read loom start/resume, batten arming/preflight/window, and reconcile bodies to trace real guard order.
3. Read D3’s exact scope and InnerRun state transitions; located run-scope target construction, which was absent from inherited context.
4. Read run-scope candidate construction, recorded-pair filtering, tender pause creation, and wake recovery branches to check whether the proposed inputs reach their guards.
5. Obtained plan line references and read reconciliation verdict mapping, parent guard prerequisites, and coverage logic to ground the two findings and distinguish reachable guards from upstream refusals.
