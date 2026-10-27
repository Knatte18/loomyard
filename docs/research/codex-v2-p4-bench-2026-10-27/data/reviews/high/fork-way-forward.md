Per-refusal recovery list:

- Card 1, invalid hostname → assign a usable hostname, then retry: progresses.
- Card 3, owner payload change → `lyx adopt <slug>`: progresses.
- Card 3, empty machine → correct the hostname source: progresses.
- Card 3, unreachable origin → retry when reachable: progresses.
- Card 3, dirty/diverged Bolt or deferred push → the underlying repair and `lyx board sync`, then retry: progresses.
- Card 8, start/resume owned elsewhere → adopt from the prime, then start when no run exists: progresses.
- Card 8, failed claim before pair creation → retry batten through the planned start-check path: progresses.
- Card 8, failed/lost owner write → repair push or adopt the reported owner: progresses.
- Card 9, owner refusal → adopt from the prime: progresses.
- Card 9, verdict/read error → repair the named cause and retry: progresses.
- Cards 10–11, owner pause or refused child spawn → adopt; failed pause is retried: progresses.
- Card 12, run/parent/both owner halts → the specified adopt forms: progress.
- Card 12, unreachable origin or unreadable ownership → retry loom resume after repairing the cause: progresses.
- Card 12, adopter-first parent-push failure → **the proposed appended repair retains incompatible merge advice; finding below.**
- Card 13, failed owner pause → another wake or `lyx orch start`: progresses.
- Card 16, unknown/note/done entry → list/promote/resume respectively: progresses.
- Card 16, local-commit/live-pair stop → `--keep-local`, or `--drop-local` for a non-live pair: progresses outside a hub pause.
- Card 16, dirty drop/add failure → repair the reported paths or leftover branch and retry: progresses.
- Card 16, push failure/lost owner → repair and repeat adopt: progresses.
- Card 16, standing slug pause → `lyx resume <slug>`: progresses.
- Card 16, standing hub pause → **its promised post-adoption report is unreachable because the earlier network calls wait on that pause; finding below.**

Findings:

1. **MEDIUM — consistency — `_lyx/plan/12-finalize-owner-checks.md:45` — Appending the adopter-first repair leaves the halt recommending the merge D6 expressly forbids.**

   Card 12 says `pushParent` “appends `adopterFirstRepair`.”
   Existing `Finalize.pushParent` interpolates the complete push error into its reason (`internal/landingshed/finalize.go:326`–334).
   `PushFailedError.Error` appends `FailedPushWayForward` (`internal/fabricengine/pushclassified.go:36`–39); a rejected code push recommends `git merge origin/<parent>` followed by a push (`internal/fabricengine/failedpush.go:217`–220, 249–255).
   Consequently the adopter-first halt would recommend both merging and dropping the duplicate squash.
   D6 explicitly requires dropping that commit, “never a merge, which would land the task twice” (`_lyx/discussion/decision-record.md:149`).
   Amend the already-targeted `pushParent` behavior so the confirmed adopter-first case replaces the ordinary recovery instructions with the squash-removal instructions, retaining the diagnostic cause.
   Extend the already-targeted failed-push rows to use a classified non-fast-forward error and assert that the halt contains no merge-and-push recovery.

2. **MEDIUM — consistency — `_lyx/plan/16-adopt-module.md:55,62,66` — Adoption under a hub pause waits indefinitely before reaching its promised standing-pause recovery.**

   Card 16 explicitly supports adoption while preserving a hub pause, describes `--drop-local` operating under it, and promises an envelope naming `lyx resume`.
   However, every production root invocation installs the network gate (`cmd/lyx/main.go:125`–129).
   `gitexec.runCoreWithin` waits on that gate before starting any network deadline (`internal/gitexec/gitexec.go:203`–208).
   The gate uses an unbounded `WaitUncovered(context.Background(), …)` (`internal/pauserecord/hold.go:249`–281), and even a parent-lock exemption exempts only slug entries, leaving a hub entry covering the prime.
   No card supplies adopt a network exemption.
   Its step-1 reconcile therefore cannot finish while the hub pause stands, so neither the stated drop behavior nor the step-4 recovery envelope occurs.
   Specify a bounded, entry-specific network exemption for adopt’s reconcile/add/owner-push transaction, preserving the pause and restoring the gate before resume work; use the existing `SetGateExempt` mechanism as the wake path does.
   Exercise the already-targeted adoption test through an installed gate with a live hub pause.

Tool-call manifest:

1. Read batten’s window body, InnerRun wiring/spawn, and attempted pause-gate path: inherited context lacked the concrete recovery execution wiring.
2. Searched shed status error persistence, network gating, and tender lift/promote entry points: needed to verify owner-halt recognition and recovery routing.
3. Read network waits, tender lift behavior, parent-tip/work checks, and card repair lines: needed to establish the hub-pause transaction and repair predicates.
4. Read fabric remove and teardown quiet/refusal paths: checked whether the prescribed final removal introduces a separate recovery defect.
5. Read persisted halt normalization, remove invocation, quiet detection, network gate implementation, and numbered card-16 lines: verified concrete recovery states and locations.
6. Read push classification/recovery text and card-12 repair wording: discovered that appending the new repair retains merge advice.
7. Read exact failed-push formatter, Finalize failure handling/tests, numbered gate sites, and decision-record recovery clauses: confirmed both findings against code and fasit.
