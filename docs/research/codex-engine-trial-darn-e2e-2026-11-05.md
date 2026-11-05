# Codex engine trial: a darn run with a Codex writer, 2026-11-05

The run `codex-greet` in the sandbox hub `/home/knatte/Code/lym-test-LYMHUB` stopped at Darn-Write: the Codex writer finished the change but could not commit it.
The sandbox refuses the commit every time, so the run cannot land with a Codex writer, and it was not resumed.

## Setup

- Hub: `lym-test-LYMHUB`, repositories `Knatte18/lym-test` and `Knatte18/lym-test-weft`.
- Entry: task `codex-greet`, recipe `darn`, profile `light`.
  The entry itself sets neither engine nor model.
- Writer seat: the hub's `darn.yaml` sets `writer: luna[medium]`, and the hub's `models.yaml` maps `luna` to engine `codex`, model `gpt-6-luna`, effort medium.
- Codex CLI 0.164.0, rollout `~/.codex/sessions/2026/11/05/rollout-2026-11-05T10-11-18-01a1aafa-15e4-7721-9a74-5f03119fc17b.jsonl`.

## Timeline

Times are UTC.

| Time | Event |
| --- | --- |
| 09:10 | Entry released; run starts, steps before Darn-Write pass or skip (Drift-Await skipped). |
| 09:11:18 | Codex session starts in the `codex-greet` worktree; its workspace roots are the worktree and its weft. |
| 09:14:02 | Go checks failed on the read-only default build cache; the writer switched to a cache under `/tmp`, after which test, build and `go run` pass. |
| 09:14:12 | First `git add … && git commit`: refused, `Unable to create '…/lym-test/.git/worktrees/codex-greet/index.lock': Read-only file system`. |
| 09:15:15 | Turn 1 ends with the change uncommitted; `lym loom validate-description` passed. |
| 09:15:41 | Re-prompt by the commit gate; commit refused again; turn 2 ends. |
| 09:16:07 | Third re-prompt; commit refused again; turn 3 ends. |
| 09:16:27 | The writer posts to its parent with `lym creel post --parent --expect-reply`; the post is accepted, id 4 to `lyt:orch`. |
| 09:17:50 | Run state `blocked`: `gate did not pass after 3 attempts`. |

## Checks against the engine docs

Matched against `internal/shuttleengine/codexengine/doc.go` and `internal/creelengine/doc.go`:

| Check | Result |
| --- | --- |
| Launch under the lym Codex home, session in the worktree | Matched: session started, `originator` codex-tui. |
| Folder trust | Matched: no trust prompt stalled the run. |
| `-a never -s workspace-write`, no approval prompt | Matched: no approval prompt appeared. |
| The sandbox lets the session write what a Claude seat writes | Not matched: the worktree's git directory, under the main checkout's `.git/worktrees`, is read-only, so a commit is refused (bug note B2). The default Go build cache is read-only too. |
| Network access reaching creel | Matched: `lym creel post --parent` from the session succeeded. |
| Creel post from the Codex strand | Matched: the post was journaled. |
| Mail and notices reaching the Codex strand | Not exercised: no mail arrived during the watch. The engine has no creel hooks, so the pane-mail path applies. |
| Hooks reaching the run's events, idle and turn-end readings | Not verified: the run's events file was not located in this watch. The rollout carries five `task_started` and five `task_complete`, and the run's re-prompts followed turn ends, which fits Stop-hook turn-end readings. |
| Writer commits | Not matched: none landed. |
| Hand-over to reviewers and Finalize | Not reached. |

## Cost and turns

Five turns in the rollout.
Token usage at the last reading: 1,586,014 total, of which 1,467,520 cached input, 14,021 output and 8,409 reasoning output.
The 258,400-token context window was never near full.

## Faults

- B2, `codex-writer-cannot-commit`: a Codex writer in a pair worktree cannot commit, because its git directory lies outside the sandbox's writable roots.
  Filed in project `codex-engine-trial`.

## Open

- The run's events file, hook stamps and trace logs were not read.
- Whether the run lands after B2 is fixed is untested.
