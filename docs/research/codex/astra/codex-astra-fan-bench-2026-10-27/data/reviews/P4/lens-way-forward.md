| Refusal or recovery | Named next step | Progresses? |
|---|---|---|
| Empty hostname | Give the machine a hostname | Yes |
| Board payload changes ownership | Adopt from the prime | Yes, through adopt’s explicit choices |
| Owner setter finds another owner | Adopt | Yes |
| Owner write encounters dirty/diverged Bolt | Follow the pull’s recovery | Yes |
| Owner push fails | Follow its classified recovery | Yes |
| Owner push remains deferred | `lyx board sync`, then retry | Yes |
| Reconcile or weft push finds another owner | Adopt | Yes, possibly through `--keep-local`/`--drop-local` |
| Batten start/resume finds another owner | Adopt, then start if necessary | Yes |
| Batten start or owner-writing resume cannot reach origin | Retry once origin answers | Yes |
| Worktree-Create owner claim fails | Retry the same batten verb | Yes; the no-pair start check is specified |
| Offline resume reads another owner locally | Retry once origin answers | Yes |
| Loom owner refusal | Adopt from the prime | Yes |
| Loom verdict error | Retry the requested verb | Yes for transient failures |
| InnerRun owner check fails or origin is unreachable | Automatic later check | Yes |
| InnerRun owner pause fails | Automatic next notice probe | Yes |
| InnerRun spawn is refused for ownership | Pause, adopt, then recheck ownership | Yes |
| Finalize retains its lock after a read failure | Retry the read next check | Yes |
| Completed old-machine landing remains paused | `lyx resume <slug>` | Yes |
| Finalize cannot read ownership or reach origin | `lyx loom resume` after recovery | Yes |
| Finalize rejects run/parent ownership | Specified single or combined adopt commands | Yes |
| Adopter-first parent push fails | Existing push recovery **plus** reset/rebase repair | **No: conflicting recovery instructions** |
| Rebase repair conflicts | Abort, then drop the squash manually | Yes |
| Wake/orch owner pause fails | Next wake or `lyx orch start` | Yes for a transient pause failure |
| Adopt runs outside the prime | Rerun from the prime | Yes |
| Adopt receives unknown slug / note | Board list / promote | Yes |
| Adopt receives a done task | Resume its completed local landing | Yes for the stated local-landing case |
| Adopt receives both local-choice flags | None specified | **No** |
| Adopt receives no selection | None specified | **No** |
| Adopt cannot reconcile | Resolve the reported cause, then retry | Yes |
| Adopt stops on local commits/live pair | Keep or drop local state | Yes; live pairs require keep |
| Drop-local encounters tracked changes | Existing dirty-state recovery | Yes |
| Pair creation encounters a leftover branch | Delete the leftover branch, retry adopt | Yes |
| Adopt’s owner push fails or loses | Push recovery / retry adopt | Yes |
| Adopt leaves another pause standing | Slug or hub resume, as specified | Yes |
| Adopt’s run resume fails | Underlying resume error and recovery | Yes |

- **MEDIUM · consistency · `_lyx/plan/12-finalize-owner-checks.md:45` — Appending the adopter-first repair retains a conflicting instruction to merge and push the duplicate landing.** Existing `Finalize.pushParent` embeds the classified push error in its reason (`internal/landingshed/finalize.go:328`), whose non-fast-forward recovery explicitly says to merge origin and push (`internal/fabricengine/failedpush.go:217`, `:249`). Card 12 appends the reset/rebase repair without replacing that instruction. Following the original next step preserves the duplicate squash rather than repairing it. The adopter-first branch must replace the incompatible recovery guidance.

- **LOW · design · `_lyx/plan/16-adopt-module.md:54` — Two argument refusals specify no next step.** The card explicitly refuses simultaneous `--keep-local`/`--drop-local` and an invocation without slugs, `--prime`, or `--all`, but names no correction for either. Specify rerunning with one local-state choice and rerunning with an explicit selection, respectively. Evidence: `_lyx/plan/16-adopt-module.md:54`; the required actionable recovery contract is in `contracts/specs/refusal-spec.md:9`.