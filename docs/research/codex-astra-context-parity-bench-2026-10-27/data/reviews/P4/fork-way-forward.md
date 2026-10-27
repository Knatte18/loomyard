| Refusal or recovery path | Named next step | Assessment |
|---|---|---|
| Card 1: empty/unreadable hostname | Give the machine a hostname; retry after the OS error clears | Progresses. |
| Card 3: changed owner in an upsert/merge payload | `lyx adopt <slug>` | Progresses through the dedicated setter. |
| Card 3: unreachable origin, dirty/diverged pull, rejected push | Retry once origin answers, or follow the classified repair | Progresses. |
| Card 3: push remains deferred | `lyx board sync`, then rerun | Progresses once the holding lock clears. |
| Cards 5–6: old owner’s records push refused | `lyx adopt <slug>` | Progresses, subject to adopt’s local-commit choice. |
| Card 8: start/resume/window/step owned elsewhere | `lyx adopt <slug>` from the prime | Progresses through takeover and the local-commit choice where necessary. |
| Card 8: failed claim before pair creation | Rerun batten | Progresses through the explicitly planned start-check path for Worktree-Create without a pair. |
| Card 8: owner-writing resume cannot push | Retry once origin answers, or follow the push repair | Progresses. |
| Card 9: shed resume owned elsewhere | Adopt from the prime | Progresses. |
| Card 9: owner verdict cannot be read | Rerun the named verb after resolving the cause | Progresses. |
| Cards 10–11: owner check/spawn refusal | Owner-labelled pause; adopt to resume | Progresses after ownership and pause are changed. |
| Card 12: parent/run/both ownership checks fail | The corresponding parent/task adopt forms, with `--keep-local` where needed | Progresses through the explicitly planned owner-halt resume path. |
| Card 12: origin or ownership inputs unreadable | Rerun `lyx loom resume` after the cause clears | Progresses. |
| Card 12: adopter-first parent push fails | Reset/rebase away the stale squash, then remove the pair | **Defective:** the appended message retains the existing merge-and-push advice, and the proposed squash identification can name a different commit. |
| Card 13: owner pause fails | Next wake or rerun `lyx orch start` | Progresses if the pause failure clears. |
| Card 16: invalid slug/note/done entry or incompatible options | Board listing/promotion, resume of a landed run, or corrected arguments | Progresses as applicable. |
| Card 16: unreachable origin/shared-branch stop | Retry or follow the report’s repair | Progresses. |
| Card 16: local commits/live pair stop takeover | `--keep-local` or `--drop-local`; live pair requires `--keep-local` | Progresses when no hub pause holds the network gate. |
| Card 16: missing pair cannot be added | Remove the leftover local branch, then rerun adopt | Progresses under the existing fabric refusal conditions. |
| Card 16: owner push fails or replay loses | Repair/retry the push or rerun adopt | Progresses. |
| Card 16: unlabelled slug/hub pause remains | `lyx resume <slug>` or `lyx resume` | Slug case progresses; **hub case cannot reach the promised adopt result while that pause stands.** |

Findings:

1. **MEDIUM — decision — `_lyx/plan/12-finalize-owner-checks.md:45`**  
   **Appending the adopter-first repair leaves the existing unsafe merge-and-push recovery in the same halt message.**  
   Card 12 explicitly appends `adopterFirstRepair` to the non-transient push failure. Existing `Finalize.pushParent` includes the classified error verbatim (`internal/landingshed/finalize.go:328–334`), `PushFailedError.Error` appends its recovery (`internal/fabricengine/pushclassified.go:40–41`), and the code-side non-fast-forward recovery says to merge origin and push (`internal/fabricengine/failedpush.go:213–219`, `249–255`). Thus the exact adopter-first divergence receives both “merge origin and push” and “drop the stale squash”; following the former preserves both landings. D6 explicitly requires dropping that squash and never merging (`_lyx/discussion/decision-record.md:149–155`). The plan should replace the ordinary recovery when the adopter-first predicate holds, retaining ordinary recovery only otherwise.

2. **MEDIUM — decision — `_lyx/plan/12-finalize-owner-checks.md:45`**  
   **The repair identifies the task’s stale squash as the current parent HEAD, which need not be that squash.**  
   Card 12 prescribes using the parent head’s SHA for the squash, even when other local commits exist. Finalize supports re-entry after a prior local merge, and `parentGuard.moved` accepts a changed parent tip whenever that tip still holds the task’s work (`internal/landingshed/parentguard.go:112–165`). `Finalize.Call` then treats the already-contained task as merged and proceeds to `pushParent` (`internal/landingshed/finalize.go:232–239`). If the local parent acquired another commit after the earlier failed landing, HEAD is that later commit: the generated rebase drops the later commit and retains the stale squash. D6 requires identifying and dropping the task’s actual squash while preserving the other commits (`_lyx/discussion/decision-record.md:150–152`). The plan should establish the actual landing commit’s identity across re-entry and test a later local commit above it, rather than assume HEAD.

3. **MEDIUM — decision — `_lyx/plan/16-adopt-module.md:55–66`**  
   **Adopt’s promised recovery under a standing hub pause blocks at its first network operation.**  
   Card 16 explicitly supports the hub-paused `--drop-local` case and promises to leave the hub pause standing while reporting its `lyx resume` recovery. But it plans an ordinary moving reconcile and owner push without a network exemption. Every production command installs `tendercli.InstallNetworkGate` in the root pre-run (`cmd/lyx/main.go:125–133`); remote git waits at `pauserecord.Network` before even starting its deadline (`internal/gitexec/gitexec.go:192–208`). The parent-lock exemption bypasses slug entries only; a hub entry still holds the wait (`internal/pauserecord/hold.go:62–76`, `259–281`). Consequently adopt cannot reach its local-choice handling, owner write, or standing-pause envelope until somebody lifts the hub pause, contrary to D5 and the card’s explicit bound. The plan needs a defined, bounded way to perform the authorized takeover operations while preserving the standing hub pause, with coverage through the real installed gate.

Tool-call manifest:
1. Read `parentguard.go`, pause override wiring and pause-related references because the inherited context lacked parent-tip re-entry behavior and network-gate locations.
2. Read `tendercli/gate.go`, network wait implementation, `Pause`’s body and status-error references because the inherited context lacked production pause gating and error persistence.
3. Read `WaitUncovered`, exemption references and adopt recovery line locations because the inherited context lacked the gate’s hub-pause semantics.
4. Read Finalize’s merge-to-push continuation, stuck mapping and parent-tip references because the inherited context lacked the re-entry path and repair-message composition.
5. Read classified push-error references and remote-git gating references because the inherited context lacked the recovery embedded in a failed push.
6. Read `FailedPushWayForward`, `codeDivergedWayForward` and the remote-git gate call because the inherited context lacked the exact existing merge-and-push recovery and deadline ordering.
