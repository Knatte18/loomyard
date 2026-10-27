| Refusal or recovery | Named next step | Assessment |
|---|---|---|
| Card 1: empty/unreadable hostname | Give the machine a hostname; error identifies cause | Progresses after correction. |
| Card 3: changed owner in board payload | `lyx adopt <slug>` | Progresses for an existing eligible task; prescribed by D2. |
| Card 3: empty machine | Correct hostname source | Progresses after correction. |
| Card 3: unreachable origin | Retry once origin answers | Progresses. |
| Card 3: dirty/diverged pull or classified push failure | Existing fabric remedy, then retry | Progresses after repairing the reported condition. |
| Card 3: deferred push still outstanding | `lyx board sync`, then retry | Progresses after the competing lock releases. |
| Cards 3/8: absent-entry claim | Supposed to pass without an owner write | Contradictory `Lost` rule can instead produce an unrecoverable adopt recommendation; finding below. |
| Card 8: start/resume owned elsewhere or lost owner write | Adopt, then start when there is no run | Progresses for an existing eligible task. |
| Card 8: incomplete Worktree-Create with no pair | Repeat `lyx batten run` through start preflight | Progresses; avoids the missing-pair run reconcile. |
| Card 8: offline resume with another local owner | Retry once origin answers | Progresses to a fresh verdict; may then require adoption. |
| Card 9: shed resume owned elsewhere | Adopt from prime | Progresses. |
| Card 9: verdict error | Fix reported cause and repeat the verb | Progresses after correction. |
| Cards 10/11: owner pause, failed owner check or failed pause | Adopt; retry checks/pauses at later probes | Progresses with the stated retry and hold behavior. |
| Card 12: run, parent or both owned elsewhere | Specified adopt forms, including `--keep-local` | Progresses within D5’s local-commit/live-pair rules. |
| Card 12: unreachable origin or ownership-read failure | Repeat `lyx loom resume` after correction | Progresses when the cause clears. |
| Card 12: adopter-first parent push failure | Reset or drop the squash, then remove the old pair | Carries D6’s prescribed recovery and leaves its pause standing. |
| Card 13: failed wake pause | Next wake or `lyx orch start` | The orch alternative is broken by the installation order below. |
| Card 13: unwired orch pause | Repeat `lyx orch start` | Does not progress in a fresh CLI process; finding below. |
| Card 15: client-read or window-command failure | Warn and continue | No blocked recovery required. |
| Card 16: unknown/note/done task or invalid arguments | Board list/promote/resume, or correct arguments | Progresses as applicable. |
| Card 16: unreachable origin, reconcile error or shared-branch stop | Retry or repair report’s condition | Progresses. |
| Card 16: local commits/live pair | `--keep-local` or `--drop-local`; live pair permits only keep | Progresses within the stated bounds. |
| Card 16: pair-add refusal | Remove leftover branch and repeat adopt | Progresses after correction. |
| Card 16: refused push/lost owner | Repeat adopt after reported repair | Progresses. |
| Card 16: standing non-owner pause | Slug resume or hub resume | Correct lift forms. A standing hub pause also holds the earlier network reconcile until lifted. |
| Card 16: child owner halt | Spawn `lyx loom resume` after ownership/pause processing | Progresses under the stated owner-halt predicate. |

Finding 1 — **BLOCKING, design**  
**Location:** `_lyx/plan/13-wake-and-orch-owner-pause.md:36–38`  
**Summary:** The owner-pause callback is captured before installation, so the prescribed “re-run `lyx orch start`” remedy repeats an unwired pause indefinitely.

Card 13 says `Command` copies the installed callback onto the receiver, while the root pre-run installs it. `newRoot` constructs `orchcli.Command()` while assembling the command tree, before any pre-run executes. Each fresh CLI process therefore constructs its receiver with nil `pauseOwned`; the later installation does not update that copied field. `startCmd` warns and launches without pausing the foreign-owned run, and rerunning repeats the same construction order. This misses D4’s orch pause.

**Evidence:** `cmd/lyx/main.go:119` defines the deferred pre-run; `cmd/lyx/main.go:161` invokes `orchcli.Command()` during root construction; `internal/orchcli/cli.go:111–112` constructs the receiver. The plan explicitly specifies the copy and ineffective remedy at card 13 lines 36 and 38.

**Proposed correction:** Resolve the installed callback when the invocation runs, or install it before constructing `orchcli.Command()`. Exercise that installation/construction order in coverage rather than only populating a fake receiver.

Finding 2 — **MEDIUM, consistency**  
**Location:** `_lyx/plan/03-board-owner-writes.md:53`, with card 8’s claim handling  
**Summary:** The setter’s universal post-push `Lost` classification turns an intentionally skipped absent slug into a refusal recommending an adopt that cannot accept it.

Card 3 first says an absent entry receives no owner or claim, but then says every requested slug whose recorded owner differs from `Machine` goes into `Lost`. An absent slug necessarily has no matching recorded owner. Card 8 refuses any slug reported in `Lost` through `ownedElsewhereRefusal`, recommending `lyx adopt <slug>`. Card 16 refuses unknown slugs before doing anything. Repeating batten or following its adopt recommendation therefore cannot progress, contrary to D3 and card 8’s explicit absent-entry pass. Card 8’s fake-setter absent row does not resolve the real setter’s contradictory contract.

**Evidence:** `_lyx/plan/03-board-owner-writes.md:47,53`; `_lyx/plan/08-batten-owner-check.md:43,48,61`; card 16’s preflight unknown-slug refusal. Existing absent-claim behavior is explicitly pinned by `internal/boardengine/board_test.go:661–674`.

**Proposed correction:** State that deliberately skipped absent entries are excluded from `Lost`, and cover that distinction through the real `SetOwners` surface using its already planned test.

Tool-call manifest:

1. Located halt persistence, lift operations, board entry points and orch start lifecycle; inherited readings lacked these downstream recovery definitions.
2. Read tender’s exact lift behavior, Bolt push/defer contracts, landing verdict helpers and board sync declaration; needed to establish whether named retries can advance.
3. Read network-gate installation, board sync implementation, note promotion and child lock wiring; inherited context lacked these recovery dependencies.
4. Read network hold/exemption behavior and located precise plan/source lines for installation, absent-owner classification and hub-pause handling.
5. Read network execution semantics, the orch start harness and absent-claim test evidence; needed to distinguish real CLI lifecycle behavior from fake receiver coverage.
