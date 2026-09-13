# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review round 3 (SAFETY PASS)

> Filled per `_mill/loom-review-prompt.md`. Clean-room round: formed independently, with no prior
> round's review/fixer-report/handoff material read until this round's own findings list below was
> complete. Agent: `sonnet5-xhigh-r3` (crucible-reviewer-xhigh, model Sonnet 5).

## Status

IN PROGRESS — Job 1 (review) underway. This file is being built incrementally per the
"Log as you go" rule; committed after each meaningful append.

## What was tested

### Hermetic baseline (before any live driving)

- `go build ./...` — clean, no output, exit 0.
- `go vet ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./internal/shedadapters/... ./internal/websterengine/...` — clean, no output.
- `go test -count=5 ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./internal/shedadapters/... ./internal/websterengine/... ./cmd/lyx/...` — all packages `ok`, 5x repeats, no flakes observed.
- `go test ./...` (whole repo) — all packages `ok` or `[no test files]`. No regressions.

### Pre-existing-state checks

- `gh pr view 2 -R Knatte18/lyx-test --json state,title,url` — still `OPEN` ("FormatGreeting helper added to services/api"). Round 2's leftover PR is unresolved by the operator; per the cost declaration I will build my OWN fresh disposable external repo for any live `Publish` PR-merge repro rather than touching `Knatte18/lyx-test`.

### Static read (clean-room, no prior review files opened)

Read in full: `manifest/designs/loom-step.md`, `self-report-tier1.md`, `self-report-tier2.md`, `manifest/designs/loom.md` (full, 564 lines), `internal/loomcli/step.go`, `run.go`, `bootstrap.go`, `drive.go`, `sharedbootstrap.go`, `selfreport.go`, `internal/loomshed/interruptpolicy.go` (+test), `internal/loomengine/anomaly.go`, `anomalybody.go`, `config.go`, `internal/friction/friction.go`, `doc.go`, `internal/frictionengine/reflect.go`, `spec.go`, `deps.go`, `internal/shedadapters/bouncer.go`, `burler.go`, `internal/websterengine/render.go`, `internal/selfreportengine/selfreport.go`, `internal/selfreportcli/cli.go`, `plugins/ly/skills/ly-supervise/SKILL.md`, `internal/loomengine/template.yaml`.

No static-read defect found yet in this pass beyond items noted under Findings below (see there). The code is heavily self-documenting about prior crucible-round fixes (F-0..F-7, R2-F1..F4) with explicit "reproduced live in crucible round N" comments at each hardened seam — consistent with two prior rounds' worth of hardening. Moving to live driving next.

### Live driving — fixture setup

Built a fresh disposable fixture hub (NOT the operator's `lyx-test-HUB`): two brand-new private GitHub repos under my own account, `Knatte18/lyx-crucible-r3` (warp, `--add-readme`) and `Knatte18/lyx-crucible-r3-weft` (weft, empty). Wired via:

```
lyx fabric clone https://github.com/Knatte18/lyx-crucible-r3-weft.git https://github.com/Knatte18/lyx-crucible-r3.git
```

into a fresh scratch directory (`r3fixture/`), separate from an apparent PRIOR round's leftover scratch files found already present in this same session's scratchpad path (timestamps ~15:07-16:12 today, several hours before this round started at 18:29 — `.go.bak` files, `walk.sh`, `setup-bares.sh`, `steps/`, `live/`, etc.). Per the clean-room constraint I did not open or read any of those files' contents; I used a freshly-named `r3fixture/` subdirectory instead and left the older files untouched (noting this scratchpad-reuse-across-rounds as a minor process observation, not a loom code defect, in Docs & operability findings below).

Deployed the dev binary (`./deploy-dev`) and overrode config for a cheap dummy task:
- `loom.yaml`: `discussion`/`plan`/`review` = `sonnet[effort=low]`, `friction` = `haiku[effort=low]` (Tier 2 left ON deliberately, to pursue the RunDone-reflection and spontaneous-note items), `selfreport: false` (mandatory per the cost declaration — never re-enabled).
- `webster.yaml`: `master`/`recovery` = `sonnet[effort=low]`.
- `landing.yaml`: `conflict` = `sonnet[effort=low]`.
- `require_pr_to_base: ["main"]` left at its shipped default so `Publish` genuinely opens a PR against the real GitHub-hosted warp repo.

Created board task `greet-lib` (a trivial "add a Go greet package + test" scope, chosen to keep every phase's real LLM session short) and `lyx fabric add greet-lib`.

Drove the WHOLE run via repeated `lyx loom step` invocations (never `lyx loom run`'s tmux-attach — that needs a real controlling TTY this Bash-tool session doesn't have; `step` needs no terminal handover and is itself one of the two verbs under review, so this doubles as `step`'s own primary exercise). Walked, one real step at a time: `Preflight` -> `Loom-Preflight` -> `Discussion-Write` -> `Discussion-Validate` -> `Discussion-Bouncer` (seed) -> `Discussion-Burler` (round 1) -> `Discussion-Bouncer` (judge, APPROVED round 1) -> `Plan-Write` -> `Plan-Validate` -> `Plan-Bouncer` (seed) -> `Plan-Burler` (round 1) -> `Plan-Bouncer` (judge — see interrupted-repro note below, APPROVED round 1) -> `Plan-Revalidate` -> `Batchifier` -> `Webster` (implementer black box, produced a working `greet/greet.go` + `greet/greet_test.go` + `go.mod`, `go test ./...` green) -> `Webster-Bouncer` (seed) -> `Webster-Burler` (round 1 — see interrupted-repro finding below) -> ...(continuing).

Every envelope observed so far carries exactly the documented ten keys, `continue` tracks `state == "running"` correctly at every step, `next_interrupt_policy` matches `internal/loomshed.InterruptPolicies` at every row including flipping to `"handback"` exactly at `Webster` and back to `"reinvoke"` immediately after. No scope/envelope defect found in this walk.

### Interrupted-and-resumed repro #1 — Plan-Bouncer judge-pass, killed AFTER round settle but BEFORE the outer status persist (independent, real kill)

Attempted a judge-pass mid-agent kill on `Plan-Bouncer`'s round-1 judge call. The judge call completed (verdict/ledger/focus files fully written) faster than my poll-and-kill loop could react — `kill -9` landed on the `lyx loom step` process AFTER the Bouncer's own `settle()` had written `round-1-bouncer-verdict.md`/`round-1-bouncer-ledger.md`/`round-2-focus.md` to disk, but evidently before `shedengine`'s own status-file persist/commit completed (`_lyx/loom/status.json` still read `current_producer: Plan-Bouncer` immediately after the kill, not yet advanced to `Plan-Revalidate`).

Re-invoking `lyx loom step` correctly re-entered `Plan-Bouncer`, found round 1 already `judged` with an APPROVED verdict on disk, and replayed `settle()` — envelope reported `outcome: done`, `next: Plan-Revalidate`. No re-spawn, no lost state, no duplicate side effect. (Initially misread `round-2-focus.md`'s presence as evidence of a BLOCKING verdict; re-checking `bouncer.go`'s `judgeOutputs`/prompt-fill code confirmed the judge's OWN three declared `OutputFiles` always include round n+1's focus path regardless of verdict, so its mere presence proves nothing about which way the verdict went — this was my own mis-read, not a code defect, corrected before drawing any conclusion.) This exercises the "crash between round-settle and outer status persist" path on the Plan segment specifically, a real, valid, independently-reproduced crash-resume, just a later window than a genuine mid-LLM-work kill.

### Interrupted-and-resumed repro #2 — Webster-Burler round 1, killed genuinely mid-agent (CONFIRMED, closes a genuinely-open item)

This is the one the "High-yield focus" list specifically asked for: a `*-Burler` round on a segment OTHER than Discussion, killed while the real agent was still actively working (not merely between its own completion and the outer persist).

Procedure: launched `lyx loom step` for `Webster-Burler` round 1 in the background; within ~2s confirmed via `ps` that a real `claude` subprocess had spawned in its own tmux pane (`--session-id 403fe93f-70c9-4fea-b339-ecd593f92383`, role burler round 1) while neither `round-1-review.md` nor `round-1-fixer-report.md` existed yet (only the pre-existing `round-1-focus.md` from the seed pass). `kill -9`'d the `lyx loom step` PID directly (confirmed gone via `ps -p`), while confirming the `claude` agent PID was still alive and running.

Re-invoked `lyx loom step`. Observed in the driver's own log:
```
shuttle: run attached ... strandGUID=d21431e... sessionID=403fe93f-70c9-4fea-b339-ecd593f92383
shuttle: run finished ... outcome=done cleanedUp=true
shedadapters: attached to a live burler round instead of respawning ... producer=Webster-Burler engine=burler round=1 sessionID=403fe93f-70c9-4fea-b339-ecd593f92383
```
Confirmed via `ps` immediately after re-invoking that only ONE `claude` process carried that session/role (no second Webster-Burler `claude` process ever appeared) — no double-spawn, no two agents racing to write one review, no two sessions holding commit authority over the branch (the concern `NewBurlerProducer`'s own doc comment names explicitly). Final envelope: `outcome: stuck` (routine Burler-to-Bouncer hand-off), `next: Webster-Bouncer`, `history_length: 17`. CONFIRMED, independently reproduced, on the exact code path (`BurlerProducer.probeLiveRound` / `Shuttle.Attach`) neither prior round exercised on a non-Discussion segment.

## Findings (provisional — recorded as spotted, ranked at the end)

### F-R3-1 (BLOCKING, CONFIRMED live against a real GitHub repo) — `Publish` can never detect its own merged PR; every merged-PR resume misreports "closed without being merged"

`internal/landingshed/publish.go:198-209` (the switch over `prs[0]` after `client.PullRequests.List(...)`):

```go
pr := prs[0]
switch {
case pr.GetState() == "open":
    ...
case pr.GetMerged():
    return shedengine.Done, shedengine.OutputPointer{}, nil
default:
    // Closed and not merged: ...
    return p.stuckOrCancelled(ctx, "the pull request was closed without being merged")
}
```

`Call` resolves the existing PR via `client.PullRequests.List(...)` (the LIST endpoint), then branches on `pr.GetMerged()`. GitHub's List Pull Requests REST endpoint (`GET /repos/{owner}/{repo}/pulls`) never populates the `merged` boolean field on its list items — only the single-PR Get endpoint (`GET /repos/{owner}/{repo}/pulls/{number}`) does. `merged` therefore comes back JSON `null` on every list item regardless of actual merge state, so `pr.GetMerged()` is unconditionally `false` for anything this call ever sees.

**Empirically confirmed against the real GitHub API**, not just reasoned from docs, using my disposable fixture's real merged PR (`Knatte18/lyx-crucible-r3#1`):

```
$ gh api "repos/Knatte18/lyx-crucible-r3/pulls?state=all&head=Knatte18:greet-lib&base=main" --jq '.[0] | {number, state, merged, merged_at, merge_commit_sha}'
{"merge_commit_sha":"0448182a7ad66a698886479e1288b4c234fca161","merged":null,"merged_at":"2026-09-13T16:49:32Z","number":1,"state":"closed"}

$ gh api "repos/Knatte18/lyx-crucible-r3/pulls/1" --jq '{number, state, merged, merged_at, merge_commit_sha}'
{"merge_commit_sha":"0448182a7ad66a698886479e1288b4c234fca161","merged":true,"merged_at":"2026-09-13T16:49:32Z","number":1,"state":"closed"}
```

**Live repro against loom itself:** drove the `greet-lib` dummy task through `Publish` for real (opened `Knatte18/lyx-crucible-r3#1` via a genuine `lyx loom step` call), merged the PR for real via `gh pr merge 1 --squash`, then re-invoked `lyx loom step`. Instead of advancing to `Finalize`, it returned:
```
level=WARN msg="landingshed: producer stuck" producer=Publish reason="the pull request was closed without being merged"
```
and the machine went permanently `blocked`/`"stuck with no OnStuck target"` — this is Tier 1's own `AnomalyEscalation` trigger shape, so on a real (non-fixture) task with `selfreport: true` this specific bug would also auto-file a GitHub issue for what is actually a fully successful, merged task, every single time the PR path is used to completion.

**Impact: every task using the shipped default `require_pr_to_base: ["main"]` — i.e. every ordinary PR-gated landing — permanently fails to self-advance past a merged PR.** The operator's own merge action never reaches Finalize on its own; the run needs manual intervention (edit the status file, or otherwise force it) every single time. This is not an edge case; it is the ordinary, designed-for happy path of the one deliberately-untested-until-now scenario the campaign's "genuinely open" list named.

**Root cause of why two prior rounds AND the hermetic suite missed it:** `TestPublish_ClosedAndMergedPR_Done` (`internal/landingshed/publish_test.go:544`) scripts its mock list response as `[{"number":7,"state":"closed","merged":true}]` — hand-setting `"merged":true` on a LIST response, a JSON shape the real GitHub API never actually produces from that endpoint (confirmed above: the real List response's `merged` key is always `null`). The hermetic test is internally consistent with the *code's* assumption rather than with GitHub's real contract, so it stays green while masking exactly this defect. Neither prior round could have caught this without actually merging a real PR against a real GitHub remote and resuming — which the round context explicitly says neither one did (round 1 stopped short, round 2 was blocked by an operator PR it could not merge).

**Suggested fix:** branch on `!pr.GetMergedAt().IsZero()` instead of (or in addition to) `pr.GetMerged()` — `merged_at` IS populated by the List endpoint (confirmed above) and is exactly the signal GitHub's own docs recommend for this. Also correct `TestPublish_ClosedAndMergedPR_Done`'s mock body to the real List shape (`"merged_at": "...timestamp...", "merged": null` — no `"merged": true` on a list item) so the test cannot silently regress back to trusting the wrong field, and add a companion test proving a genuinely-closed-not-merged PR (both `merged` and `merged_at` absent/null) still reports "closed without being merged".

CONFIRMED, not PLAUSIBLE — reproduced end-to-end against a real GitHub repository via loom's own real code path, independent of any prior round.

## Executive summary

(written last, once the full picture is in)

## Scope assessment

(written after the code read is complete)

## Docs & operability findings

(populated incrementally)
