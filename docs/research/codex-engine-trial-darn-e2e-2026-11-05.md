# Codex engine trial: a darn run with a Codex writer, 2026-11-05

The run `codex-greet` in the sandbox hub `/home/knatte/Code/lym-test-LYMHUB` stopped at Darn-Write: the Codex writer finished the change but could not commit it.
The sandbox refused the commit every time, so the run could not land with a Codex writer; the section after `## Open` reports its resumption, which landed.

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
- Whether the run lands after B2 is fixed: it does, see the resumed run below.

# The resumed run: landing with the fixed binary

The installed `lym` was built from `fe2663d3` (`go version -m` on the binary: `vcs.revision`, `vcs.modified=false`), the commit that adds the Codex writable roots.
`lym loom resume` in the `codex-greet` worktree re-spawned Darn-Write; the run was neither cancelled nor re-created.
It reached the landing and ended `done` at Friction-Reflect.

## Timeline of the resumed run

Times are UTC.

| Time | Event |
| --- | --- |
| 12:06:19 | `lym loom resume`; a Go driver is spawned, state `running` at Darn-Write. |
| 12:06:20 | New Codex session `01a1ab9a-…` in the worktree. Its `runtime_workspace_roots` are the worktree, its weft, the worktree's git dir, the common `.git` and `~/.cache/go-build`. |
| 12:06:27 | Turn 1 ends: the writer read its skills, the prompt, the decision record and the gate findings. |
| 12:07:36 | Commit `ba26f56`, `feat(services/api): add greeting helper`; the first `git add` named `_lym/landing/summary.md`, which sits beyond a symlink, so git refused that pathspec, and the next command committed the five source files. |
| 12:07:41 | Turn 2 ends; `lym loom validate-description` had passed. No gate re-prompt followed. |
| 12:07:43 | Darn-Write done; Drift-Verify done at 12:07:45. |
| 12:08:08–12:10:42 | Code-Review round 1 with Sonnet lens reviewers: one NIT, F1 (subtest naming), fixed by Code-Burler-Fix in commit `89f0e67` at 12:09:50; the bouncer verdict is `CONVERGED`. |
| 12:11:06 | Describe done; PR #3 on `Knatte18/lym-test` opened at 12:11:17. |
| 12:11–14:13:23 | PR-Review waits on a human review. The pr-reviewer strand sat idle at its prompt. |
| ~12:37 | Parent asked; the operator decided no review is ever wanted in the sandbox hub. |
| 13:38 | The skip arrived at the pr-reviewer as creel #9 from `lyt:EXTERNAL` and the reviewer refused it, since only `lyt:orch`, a `lyt` orch strand or a lym program may instruct it. |
| 14:13:23 | The orch set the continue flag with `lym loom pr-continue`; PR-Review done. |
| 14:13:29 | Finalize: post-merge landing gate passed; `main` carries `47e693a`, the landed commit, and origin has it. PR #3 ended closed, not merged, since `pull_request` had been turned off. |
| 14:13:50 | Friction-Reflect files nothing and the run is `done`. |

## Checks against the engine docs, resumed run

| Check | Result |
| --- | --- |
| Writable roots for the worktree's git dir and the common git dir | Matched: both are in the session's roots, and the commit succeeded with no `Read-only file system` or `Permission denied` anywhere in the rollout. |
| Writable `GOCACHE` | Roots matched, use not exercised: `~/.cache/go-build` is a root, but the resumed session ran no Go command, since the change was already written. |
| Launch under the lym Codex home, folder trust, `-a never -s workspace-write` | Matched as in the first attempt: no trust or approval prompt. |
| Hooks: turn end read from Stop | Fits, not read directly: the Codex events file left with the torn-down worktree and the archive does not keep it. The rollout has two `task_started` and two `task_complete`, and the run advanced two seconds after the last turn end with no re-prompt. |
| Creel post, mail and notices to the Codex strand | Not exercised: the writer posted nothing and no mail went to it. |
| Writer commits | Matched: one commit, the change landed. |
| Hand-over to the Claude reviewers, Describe, Finalize | Matched: every step ran to done, and the landing commit message carries the change description. |
| Credit block | Observed: the landing commit credits LoomYard and Claude only; `credit_engines` in the `landing.yaml` template lists `claude` alone, though `engines-run.json` names both engines. |

## Cost and turns of the resumed run

The resumed Codex session had two turns, 457,756 tokens at the last reading: 407,680 cached input, 3,427 output, 1,261 reasoning output.
The 258,400-token window was never near full.
The first attempt used five turns and 1,586,014 tokens, so the resumed session spent about 29% of that.
The Code-Review fan ran on Sonnet: the review used 86,720 fresh and 1,077,240 cache-read tokens in 75 s, and the fix 73,266 fresh and 1,167,299 cache-read tokens in 54 s.
The only dollar figure in the archive's `_lym/usage` is Friction-Reflect, $0.2175; the writer and reviewer sessions have no session record there, and `lym report cost` reports no data below its minimum of runs.
The landed change is 44 lines added and 2 deleted over 5 files.

## Faults of the resumed run

No new lym fault.
The two-hour wait in PR-Review, with nothing in `lym loom status` naming it, is the existing note `pr-review-wait-silent`.
The refused creel #9 followed creel's sender rule: mail from outside the hub reads as `lyt:EXTERNAL`.
