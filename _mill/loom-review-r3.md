# `loom` (loom-step + self-report Tier 1 + Tier 2) — independent review round 3 (SAFETY PASS)

> Filled per `_mill/loom-review-prompt.md`. Clean-room round: formed independently, with no prior
> round's review/fixer-report/handoff material read until this round's own findings list below was
> complete. Agent: `sonnet5-xhigh-r3` (crucible-reviewer-xhigh, model Sonnet 5).

## Status

COMPLETE. Job 1 (review) found one CONFIRMED BLOCKING finding, F-R3-1. Job 2 fixed it, verified hermetically (sabotage-proved) and live against the real, genuinely-merged PR the round itself opened. See `_mill/loom-review-r3-fixer-report.md` for the fix details.

## Severity ranking

1. F-R3-1 — BLOCKING, CONFIRMED, FIXED. See below.

No MEDIUM/LOW/NIT findings this round — a genuine safety-pass outcome for everything except the one path (`Publish`'s merged-PR resume) neither prior round could reach.

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

### `Publish`/`Finalize` continuation, RunDone friction reflection, and the spontaneous-note question (real, real, third-null-result)

After confirming F-R3-1 above, continued exercising the untested surface on the SAME real task rather than abandoning it: overrode `landing.yaml`'s `require_pr_to_base` to `[]` for `greet-lib` specifically (a real, honest config change — not a status-file fabrication) so `Publish` could return `Done` immediately and the run could keep walking forward through `Finalize` to a genuine `RunDone`, since the confirmed bug above would otherwise wedge every further test behind it. This is a deliberate, disclosed methodological substitution, not a way of hiding the bug's impact — F-R3-1 above is reported exactly as found, on the unmodified default config.

- **`Finalize` real merge-back:** `lyx loom step` on `Finalize` returned `outcome: done`, `state: done` — a genuine terminal `RunDone` reached through the real phase machine. Checked the prime worktree's own `git log`: local `main` now carries the exact commit SHA GitHub's own PR-merge produced earlier (`0448182a7ad66a698886479e1288b4c234fca161`) with the squash-merge commit message `"1: greet-package (#1)"`. This matches `manifest/designs/loom.md`'s own documented claim verbatim: *"after a remote-side merge the visible side reports already-up-to-date while the other genuinely merges"* — `Finalize`'s merge-in step correctly recognized the already-landed content from the earlier real GitHub merge and still completed cleanly. No defect found in `Finalize` itself; this also gives strong indirect confidence that once F-R3-1 is fixed, `Finalize` will behave correctly immediately afterward on the now-corrected `Publish` -> `Finalize` sequence, since this is exactly the post-merge state that sequence will hand it.
- **RunDone friction reflection on a REAL walked run (closes genuinely-open item 4):** the friction directory was empty at this point (see the spontaneous-note point below), so to exercise the reflection AGENT itself (not just the trigger condition) I hand-placed one clearly-labeled test note (`manual-test-note.md`, explicit about being hand-placed, never claiming to be agent-authored) into `.lyx/loom/friction/` — deliberately NOT touching the status file or faking any state, exactly the distinction the round's mission draws (round 2's gap was a hand-BUILT DONE-STATE fixture, not a hand-placed note under an otherwise-fully-real terminal state). Ran `lyx loom drive` against the already-`done` task: `shed.Run` hit its own already-done short-circuit (`internal/shedengine/run.go:107-108`, `Next: st.CurrentProducer, State: StateDone`) with `Outcome: RunDone`, and `driveCmd`'s RunE correctly called `reflectFriction()` — envelope: `{"friction":"reflected","halted_producer":"Finalize","outcome":"done",...}`. Confirmed a REAL reflection agent spawned, read the note, and wrote a genuine, sensible judgment call in the archived `reflection-report.md`: *"This is test data, not a real friction issue worth filing... No issues filed."* The friction directory was correctly archived to `friction-20260913-165526/` and recreated empty. This is CONFIRMED, on a real `RunDone` from a real walked task, closing the "never driven on a task that actually walked all the way to Finalize" gap the round context named.
- **Spontaneous friction note (third independent null result):** across the WHOLE real `greet-lib` run — Discussion-Write, Plan-Write, the Webster fork implementer, and all three Burler review-fix rounds, every one of which had `{{.friction_directive}}` available in its own prompt — not one agent wrote a friction note of its own before I hand-placed the test one above. Consistent with the round context's framing: three independent models across three rounds now agree the bar for "worth a note" sits above ordinary smooth-task friction. Reporting this as the informative null result it is, not as an open defect.
- **No third GitHub issue filed:** `selfreport: false` throughout: confirmed no `selfreportengine.CreateIssue`/`gh`-style call was ever made by loom itself during this whole run (the only real GitHub API calls made all session were mine directly: opening/listing/merging the PR). The reflection agent's own judgment also independently declined to file anything.

### Live smoke suite

`go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — all 13 smoke test functions PASS (17.5s), including `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` (the hermetic sibling of my own live Webster-Burler repro above) and `TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus`. No regression. This suite is providerless (per the cost declaration, confirmed by reading the file header again) and does not touch `landingshed`/`Publish`, so it could never have caught F-R3-1 — consistent with F-R3-1 needing a real GitHub remote to surface at all.

### Root-cause cross-check for F-R3-1

Grepped the whole repo for other occurrences of the same anti-pattern (`GetMerged()` after a `PullRequests.List` call): `publish.go:204` is the ONLY call site in the entire codebase (`internal/selfreportengine` never touches pull requests at all; `landingshed`'s own integration test, `publish_integration_test.go`, never exercises the merged-PR branch either — its scripted List handler only ever returns `"[]"` or a freshly-created open PR). The defect and its blast radius are fully isolated to this one call site and the one hermetic unit test whose mock diverges from the real API shape.

### Sandbox suite (S8) — no extension needed

Read `tools/sandbox/SANDBOX-CORE-SUITE.md`'s S8 in full. S8 is a cheap, fixture-based (hand-written status.json), no-real-LLM human-dogfooding checklist for `lyx loom status`/`pause` — a deliberately different testing shape from this round's real-PR/real-agent-kill scenarios, which are far too expensive for that suite's own intended cadence. F-R3-1's fix is a package-level Go unit test in `internal/landingshed`, not a sandbox scenario. No S8 extension made; `sandbox_coverage_test.go` was not touched and needs no attention here.

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

This is a genuine safety pass: two prior rounds converged the general envelope-fidelity/anomaly-detection/friction-directive machinery, and this round's independent clean-room static read found nothing new to add there — the code is unusually well self-documented about its own prior-round fixes. Where this round earns its keep is exactly where the round context predicted: the untested surface neither prior round's method could reach without a genuine, real, merged pull request against a real GitHub remote.

**One CONFIRMED BLOCKING finding (F-R3-1):** `internal/landingshed/publish.go`'s merged-PR resume check (`pr.GetMerged()`) reads a field GitHub's List Pull Requests REST endpoint never populates — confirmed with a live `gh api` call against a genuinely-merged PR the round itself opened and merged on a disposable fixture repo. Every task using the shipped default `require_pr_to_base: ["main"]` therefore permanently fails to self-advance past its own merged PR: `Publish` reports "the pull request was closed without being merged" and blocks the run for a human, forever, on what is actually a fully successful landing. On a real (non-fixture) task with `selfreport: true` this also mis-files as Tier 1's `AnomalyEscalation` on every single occurrence. Masked by a hermetic unit test whose mocked List response hand-sets a field shape the real API never produces. This is the single most consequential result of the round, precisely because it sits on the ordinary, most-used happy path (a task whose PR gets reviewed and merged) rather than an edge case.

Beyond that: `Finalize`'s live merge-back was independently confirmed correct on the exact post-real-PR-merge state F-R3-1's fix will hand it (already-up-to-date recognition matches `manifest/designs/loom.md`'s own documented claim verbatim). The `RunDone`-triggered friction reflection was independently confirmed to spawn a REAL reflection agent and make a REAL, sensible judgment call over a hand-placed note on a task that walked the WHOLE real phase machine to a genuine terminal `done` (not a hand-built status-file fixture) — closing that genuinely-open item. A second, independent interrupted-and-resumed repro was driven on `Webster-Burler` (a non-Discussion `*-Burler` row, killed genuinely mid-agent — the real `claude` process still alive in its own pane after the driving process was `kill -9`'d) and confirmed clean reattachment with zero double-spawn. The spontaneous-friction-note question produced a third independent null result. No third GitHub issue was filed; `selfreport` stayed `false` throughout.

**Merge-readiness verdict: merge-ready AFTER this round's fix, which is now implemented, hermetically sabotage-proved, and re-verified LIVE against the same real, genuinely-merged PR that exposed it** (`internal/landingshed/publish.go` now checks `!pr.GetMergedAt().IsZero()` instead of `pr.GetMerged()`; re-running the fixed binary against `Knatte18/lyx-crucible-r3#1` — still merged on GitHub throughout — now correctly returns `Done` and advances to `Finalize`). No other residual carried forward; this is otherwise the clean safety pass the round context hoped for.

## Scope assessment

Plan-vs-shipped: unchanged from rounds 1/2's own conclusions, independently re-confirmed by actually walking a real task end to end through all fifteen (of the doc's) / seventeen (of the recipe's) rows, including the two rows (`Publish`, `Finalize`) neither prior round drove live. `step`'s ten-key envelope, Tier 1's exemption on the `step` path, and Tier 2's friction-directory lifecycle all matched their design docs exactly, observed directly rather than inferred from source. No deferred-that-should-be-v1 and no shipped-beyond-scope found. `Plan-Sweep`'s absence is confirmed correct (never reached in the real recipe; `Plan-Write` ran directly after `Discussion-Bouncer`'s approval, matching the recipe's 17-row list, not the design table's 15-row display list).

The one place scope and correctness meet: `Publish`'s "merged PR" outcome is IN scope (the design doc and the shipped code both clearly intend it — `case pr.GetMerged(): return shedengine.Done`) but is UNREACHABLE as implemented, which is a correctness defect rather than a scope gap.

## Docs & operability findings

No doc/skill drift found against the code as directly observed live. Specifically re-checked, live, against the trio's own claims:
- `loom-step.md`'s ten-key envelope and `next_interrupt_policy` semantics — matched exactly across every one of the ~24 real step calls in this round's dummy run, including the `reinvoke`<->`handback` flip precisely at the `Webster` row and back.
- `self-report-tier1.md`'s "step is deliberately exempt" claim — confirmed: no anomaly-detection code path is reachable from `step` at all (traced in `step.go`; `detectAndFileAnomalies` is only ever called from `drive.go`).
- `self-report-tier2.md`'s friction-directory lifecycle (`.lyx/loom/friction/`, cleared on first seed, ensured on re-entry, archived-with-timestamp-then-recreated on a reflection) — matched exactly, including the archive naming shape (`friction-20260913-165526/`).
- `loom.md`'s claim about a merged-PR's "visible side reports already-up-to-date while the other genuinely merges" — independently confirmed live (see above), verbatim match to the actual observed `Finalize` behavior.
- `ly-supervise/SKILL.md` — its error-kind vocabulary, the `busy`/`producer`-retry-once rule, and its `.lyx/loom/friction/` directory reference all matched the real envelopes and paths observed. No drift.

**Process observation, NOT a loom code finding (not counted in severity ranking, not something Job 2 fixes):** this round's session scratchpad directory (`/tmp/claude-.../scratchpad/`) already contained a substantial set of files (`.go.bak` backups, `walk.sh`, `setup-bares.sh`, `steps/`, `live/`, timestamped several hours before this round started) that read as a PRIOR round's live-driving workspace, not cleaned up and evidently reachable by a later round's session under this crucible method's current scratchpad-naming scheme. Per the clean-room constraint I did not open or read any of those files' contents and worked in a freshly-named `r3fixture/` subdirectory instead, so no contamination of this round's own independent findings occurred — but the orchestrator may want to consider whether the crucible method's own hygiene needs a directive to tear down (or the harness to isolate) a round's scratchpad at round end, so a future round's clean-room guarantee does not depend on the next round's agent noticing stale timestamps and choosing not to look.
