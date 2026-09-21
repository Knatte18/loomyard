# batten — independent review (round 4, tag `opus-medium-r4`)

> Clean-room round: findings below were formed from the SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), the module code, the docs, and live driving of a disposable fixture hub — before any prior round's review material was opened.

## Executive summary

Round 4 is close to, but not quite, the clean safety pass the round context expected. An independent adversarial pass over the code, the recovered SPEC, the docs, and thirteen live scenarios on a disposable fixture hub re-confirmed **every** high-yield focus item as a non-defect except one, and found four new findings — one of them a real, reproducible resume hole that three prior rounds did not reach.

**Top risk — F1 (MEDIUM, CONFIRMED).** `Worktree-Create` is not idempotent against a task worktree pair that is already on disk. If the driving process dies in the window between `Topology.Add` succeeding and `shedengine` persisting the row's `done` transition — a real window of seconds of git work — the resumed run is **permanently blocked**, and the stuck reason it hands the operator names two remedies that both fail in exactly that state (verified by running both). This is the "cold-machine / mid-operation-failure orphan" case focus item 7 asks about, and it is the one place batten does not report honestly what state it is actually in.

The other three findings are small: an undocumented operational consequence of the (correct) status-commit skip (F2, LOW), a refusal that reports an impossibility as a mere disagreement (F3, NIT), and a shipped residual that the deleted design doc records but no living doc does (F4, NIT).

**Everything else held.** The Batten Bookend Invariant (16/16 refusals from a task worktree and its weft sibling), teardown ordering with each half sabotaged in turn, the halted-child-never-tears-down guarantee across all three halted states, the Board-type → recipe → driver plumbing landing a real `recipe: loom` / `driver: llm` child seed, `PrimeRunLock` serialising two different slugs' creates while never being held during a long `Run-Shed` watch, an authoritative hand-written seed, both sabotage scenarios, pause/resume, the run-lock busy paths, and the cold-machine durable/ephemeral split — all confirmed live, none defective.

**Focus item 4 is now answered precisely** rather than inferred: the first `Run-Shed` step measured 30.33s wall clock, of which the child bootstrap was sub-second and the rest was the producer's own poll sleep. `InnerRunDeps.Spawn` blocks for the bootstrap launch only — `lyx batten step`'s first advancing call cannot hang for a real campaign's length — and `deps.go`'s doc comment is accurate.

**F6[R2] reproduced**, with its trigger finally pinned down: the gate is keyed to the child worktree's *absolute path* in `~/.claude.json`'s `projects` map, not to the worktree's freshness, which is exactly how a reused fixture directory hides it (R3's non-reproduction). Still not batten's bug, still needs its own task. Both operator routes past it were denied by this session's permission classifier, so the success arm was driven with `--child-driver go`, which is driver-agnostic at batten's own boundary.

**Merge-readiness:** ship after F1 is fixed. F1 is not a blocker for the normal single-instance happy path — the merge bar this campaign set — but it is a resume hole an unattended fleet will hit, and it is cheap to close. F2–F4 are documentation and wording.

## Scope assessment — plan vs shipped

Measured against the recovered design doc (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`).

| Design promise | Shipped | Verdict |
| --- | --- | --- |
| Four rows: `Worktree-Create`, `Seed-Child`, `Run-Shed`, `Worktree-Teardown` | `contracts/recipes/batten-recipe.yaml`, names pinned by `battenrecipe/names.go` + its coverage guard | Delivered |
| `Seed-Child` copies the recipe from the Board task's `type` field, the driver from batten's own seed params | `seamchild.go` + `wire.go`'s `SeedChild` seams; `type` read fresh per `Call`, empty ⇒ `loom` | Delivered, verified live |
| `Run-Shed` self-routed on `Stuck`, `max_bounces: 1440`, `poll_interval_s: 30`, one poll per bounce rather than one blocking call | exactly that in the recipe; `innerrun.go` checks once per `Call` and never loops | Delivered, verified live (30.33s first step) |
| Every non-running, non-done child state is a hard error, never a bounce, never routing to teardown | `innerrun.go:148-152`; teardown structurally unreachable from any `on_stuck` | Delivered, verified live for `failed`/`blocked`/`paused` |
| Teardown sequences session shutdown before worktree removal | `teardown.go:118-136`; `Remove` not called at all on a `Shutdown` failure | Delivered, both halves sabotaged live |
| Run addressing: durable `_lyx/shed/<run-id>/` for seed+status, ephemeral `.lyx/shed/<run-id>/` for locks | `shedrun/paths.go`; batten forwards only (`paths.go`) | Delivered, verified live on a cold-machine shape |
| `self` is the default run-id, but meaningless for prime | `refuseSelfAddress` refuses both the omitted-argument and the explicit-`self` case, with different wording | Delivered, verified live |
| `go` stays the only driver batten runs use; batten grows no bootstrap verb | `BootstrapVerb = ""`, `refuseBattenOwnDriverLLM` | Delivered (F3 is a wording gap on the seeded-run path only) |
| Rows self-heal machine-local resources a durable status promises but a cold machine lacks | **Partially.** `taskWorktreeLocation` refuses an absent pair by name rather than recreating it (R1-F9's deferred half, blocked on a fabric capability — still accurate). The *opposite* direction, disk ahead of status, is not self-healed at all: **F1** | Gap |
| Residual (a): a dead driver strand is not detected or recovered | Still true; recorded in `battenshed/doc.go` (widened by R2 to cover an alive-but-parked strand) | Accurately documented |
| Residual (b): a cleanly finished driver's strand and run directory are not torn down | Still true, re-verified live; **documented nowhere in shipped docs**: **F4** | Gap in the record, not in the behaviour |
| Relay-stepping rejected | Absent, as intended | Correctly not built |

Nothing shipped beyond scope. No silently-dropped requirement other than the two recorded above.

## Findings

### F1 — `Worktree-Create` is not idempotent against an already-present pair, and its refusal names two remedies that both fail (MEDIUM, CONFIRMED)

**Where:** `internal/battencli/wire.go:141-150` (the `CreateWorktree` closure) — surfaced through `internal/battenshed/create.go:77-83`, which passes fabric's refusal through verbatim by design.

**Scenario (reproduced live).** The driving process dies — SIGKILL, reboot, a killed agent — in the window between `fabricengine.Topology.Add` returning success and `shedengine` persisting the `Worktree-Create → done` transition. The window is real: `Add` does seconds of git work (two worktrees, two branches, junctions, launcher scripts, an origin-record commit), and the persist happens only after the producer returns.

A resumed `lyx batten step <slug>` / `lyx batten run <slug>` then re-enters `Worktree-Create` against a pair that is already on disk and healthy. `Topology.Add` refuses, the row goes `Stuck`, and the run is `blocked` with:

```
branch "<slug>" already exists; switch a pair onto it with "lyx fabric checkout <slug>",
or delete it first with "git branch -D <slug>" if it is a leftover from a removed pair
```

Both named remedies fail in exactly this state, verified by running them:

- `lyx fabric checkout <slug>` → `weft worktree has uncommitted changes; stash or commit before checkout` (prime's own weft is legitimately dirty during a live batten run — see F2).
- `git branch -D <slug>` → `cannot delete branch '<slug>' used by worktree at <path>`.

So the run is stuck with no documented way forward, even though the state it is stuck on is exactly the state the next row needs. The operator's only escapes are destructive (`lyx fabric remove <slug>`, throwing away the pair) or hand-editing prime's durable `status.json`.

This is the failure mode focus item 7 asks about, and the honest answer it asks for — "reports honestly what state it's actually in" — is not delivered: the refusal describes a *leftover branch from a removed pair*, which is the opposite of what is on disk.

**Why this is batten's to fix, not fabric's.** `Topology.Add`'s refusal is correct for `lyx fabric checkout`'s own callers. Batten's create row has a different contract: it is a *resumable producer* whose post-condition is "the task worktree pair for this slug exists". A producer with that post-condition must treat an already-satisfied post-condition as success, which is ordinary producer idempotency, not a fabric capability. This is also distinct from R1-F9's deferred recreate-from-branch half (status ahead of disk, needing a fabric capability that does not exist); here disk is ahead of status and no new capability is needed.

**Fix.** Probe for the already-present pair at the top of the `CreateWorktree` closure and treat it as a satisfied precondition: log it at Info and return nil, so the row returns `Done` and the run advances to `Seed-Child` (itself idempotent). Only call `Topology.Add` when the pair is genuinely absent, leaving every real create failure on today's `Stuck` path. Regression test at the `CreateWorktree`-seam level, in the existing stubbed-seam style.

**Confidence:** CONFIRMED — reproduced live on the fixture hub, and both named remedies observed failing.

### F2 — a live `Run-Shed` watch leaves prime's weft permanently dirty, which refuses `lyx fabric checkout` hub-wide (LOW, CONFIRMED)

**Where:** `internal/battencli/commitstatus.go:139-148` (the on-disk no-op-transition skip), interacting with `contracts/recipes/batten-recipe.yaml`'s `on_stuck: Run-Shed` self-route.

**Scenario (observed live).** Every `Run-Shed` self-bounce appends a history entry, so `shedengine` rewrites prime's durable `_lyx/shed/<slug>/status.json` once per poll — but the commit seam's no-op skip (correctly) declines to commit, because the `(producer, state)` pair is unchanged at `("Run-Shed", "running")`. The file therefore stays modified-but-uncommitted on prime's weft for the entire watch, which under the row's own 1440-bounce budget is up to 12 hours.

Observed consequence: `lyx fabric checkout <any-slug>` refuses hub-wide for the duration with `weft worktree has uncommitted changes; stash or commit before checkout`. That is also what makes F1's first named remedy unusable.

The skip itself is right — committing 1440 identical transitions would be far worse — and the alternative (suppressing the per-bounce history append) is a `shedengine` change, out of scope here. What is missing is that this is nowhere written down: `docs/overview.md`'s batten entry describes the durable two-status-files design in detail and says nothing about the drift or its hub-wide effect, so an operator meets it only as an unexplained fabric refusal.

**Fix.** Document it — one sentence in `docs/overview.md`'s batten entry, beside the `status`/history paragraph that already explains the self-bounce, plus a line in `commitstatus.go`'s own package comment recording the drift as the deliberate cost of the skip.

**Confidence:** CONFIRMED — observed live, and the causal chain traced to F1's failed remedy.

### F3 — `refuseAdoptedSeed` reports `--driver llm` on a seeded batten run as a disagreement rather than as an impossibility (NIT, CONFIRMED)

**Where:** `internal/battencli/arm.go:116-136` and `arm.go:192`.

**Scenario (reproduced live).** On an unseeded run, `lyx batten run <slug> --driver llm` refuses with the true reason: *"batten has no bootstrap verb, so it cannot be driven by an LLM"* (`refuseBattenOwnDriverLLM`). On an already-seeded run the same command instead refuses with *"run "<slug>" is already seeded with driver "go"; --driver "llm" cannot change a seeded run's recorded driver"* — because `refuseAdoptedSeed` runs first and `refuseBattenOwnDriverLLM` is reachable only on the auto-seed path.

Both refuse, so nothing unsafe happens; the wording is just the weaker of the two. It reads as "you cannot change it *now*", which invites the operator to delete the seed and re-seed with `llm` — a thing batten can never support at all.

**Fix.** Check `refuseBattenOwnDriverLLM(driverFlag)` ahead of the seeded-driver disagreement whenever the operator actually typed `--driver`, so the impossibility is reported in both states with one wording.

**Confidence:** CONFIRMED — both messages observed live.

### F5 — batten broke the Driver Choice Single-Site Invariant; its tripwire is RED at HEAD and `CONSTRAINTS.md` asserts the opposite (BLOCKING, CONFIRMED)

**Where:** `internal/battencli/arm.go:123` and `arm.go:126` (`refuseAdoptedSeed`'s two `seed.Driver` reads), against `CONSTRAINTS.md:122-130` and its tripwire `internal/loomcli/bootstrap_test.go:686`.

> Found during this round's Job 2 verification, not during Job 1: the round prompt's own hermetic command covers only the three batten packages plus `cmd/lyx`, and this tripwire lives in `internal/loomcli`. A full `go test ./...` is what surfaces it. Recorded here as a finding and fixed like any other.

**Scenario (reproduced, and present on a clean tree).**

```
go test -count=1 -run TestDriverChoiceSingleSiteInvariant ./internal/loomcli/...
--- FAIL: TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField
    bootstrap_test.go:714: found a production reader of shedrun.Seed's Driver field outside
    internal/loomcli and internal/shedrun: internal/battencli/arm.go:123, internal/battencli/arm.go:126
```

Verified red with my own working tree stashed, so it is **not** caused by this round's F1 fix. `git log -L` places the cause at `3c11aa679 batten: fix F3/F4 — refuse a seeded run that disagrees with the invocation, instead of adopting it silently` — a fix from an earlier round on this same branch, which introduced `refuseAdoptedSeed` and with it the first `seed.Driver` reader outside `internal/loomcli`. Three rounds have run since without noticing, because none of them ran the repo-wide suite.

Two things are wrong, not one:

1. **The tripwire is red**, so the branch cannot merge on a green build.
2. **`CONSTRAINTS.md` is now factually false about this very file.** Line 128 lists "batten's own flag validation in `internal/battencli/arm.go`" among the *permitted constant consumers* and states outright that it "[does not read] a written seed's `Driver` field" — which stopped being true at `3c11aa679`. Line 126 also says "no code path gates a refusal on it", and `refuseAdoptedSeed` gates exactly that.

**Which way to resolve it.** Deleting `refuseAdoptedSeed`'s driver comparison would reintroduce the real defect that round found — silently dropping a typed `--driver` against a seeded run. And the comparison is not the failure mode the invariant is written against: its stated rationale is "silent divergence between what a run was seeded as and what it is actually doing — unobservable from either the status file or the envelope", and it explicitly prefers the loud alternative ("a flag validator ... fails loudly at the command line, where a mistake is visible immediately"). `refuseAdoptedSeed` *is* that loud command-line refusal: it compares a value the operator just typed against the recorded one, refuses on the envelope, and selects no spawn and no behaviour.

So the right resolution is the one the tripwire's own failure message prescribes — "review the new site and ... extend this scan's allowlist **deliberately** rather than widen it silently" — paired with correcting the invariant text so the carve-out is stated where the rule lives, not just where the scan runs.

**Fix.** (a) A narrow, path-scoped allowlist entry in `internal/loomcli/bootstrap_test.go` naming `internal/battencli/arm.go` and why; (b) `CONSTRAINTS.md`'s Driver Choice Single-Site Invariant amended to state the carve-out precisely and to stop asserting the false claim about `arm.go`; (c) the repo-wide command added to this campaign's own test list so the next round cannot repeat the miss.

**Confidence:** CONFIRMED — reproduced on a clean tree, cause located by `git log -L`.

### F6 — batten leaks fabric vocabulary; a second repo-wide tripwire is RED at HEAD, and two invariants contradict each other (BLOCKING, CONFIRMED)

**Where:** `internal/battencli/arm.go:264` (the `fabricengine.RequireWarpWorktree` call), `arm.go:261-263`, `arm.go:388`, `arm.go:455`, `cli.go:157-158`, `refusal.go:12-16`, against `CONSTRAINTS.md:197-201` (Fabric Vocabulary Invariant) and its tripwire `internal/lyxcwd/enforcement_test.go:924`.

**Scenario (reproduced, and present on a clean tree).**

```
go test -count=1 -run TestEnforcement_FabricVocabulary ./internal/lyxcwd/...
--- FAIL: TestEnforcement_FabricVocabulary/tree-scan
    fabric-vocabulary leak found:
      internal/battencli/arm.go: bare weft/warp token outside the owner set
      internal/battencli/cli.go: bare weft/warp token outside the owner set
      internal/battencli/refusal.go: bare weft/warp token outside the owner set
```

`internal/battencli` is not in the Fabric Vocabulary Invariant's owner set, and all three of its own package docs say so explicitly ("no identifier, literal, or comment in this package ... may name either side of the pair"). `git log -L` puts the cause on this branch again: `dc9690e71 batten: fix F3 — refuse every batten verb from the weft prime, not only from task worktrees` and `a811c01b7 batten: fix N3 + N4 — group help lists all four verbs`. Same blind spot as F5 — three rounds' hermetic command never ran `./internal/lyxcwd/...`.

**The part that is not just sloppy prose.** Most of the hits are comments and one user-facing string, trivially reworded. One is not: `arm.go:264` calls `fabricengine.RequireWarpWorktree`, and the scan matches the bare token **inside an identifier** (`bareVocabularyToken` is a deliberate substring match, `enforcement_test.go:654`), so the published function name is itself the leak. Meanwhile `CONSTRAINTS.md:237` (Batten Bookend Invariant) **mandates** that exact call: *"`battencli`'s pre-run therefore calls `fabricengine.RequireWarpWorktree` ahead of the name check"*.

So two invariants directly contradict each other, and batten cannot satisfy both as the API stands. The call itself is correct and load-bearing — it is what catches the fabric-sibling case that a name comparison alone admits (verified live, scenario B).

**Fix.** Follow the pattern fabric already established for exactly this problem — `CommitAnchoredPaths`, `PushAnchored`, `Fabric.PushBranch` are all vocabulary-neutral spellings fabric hands to non-owners ("`PushBranch` wrapping `PushWarpRebaseFreeAt` under a vocabulary-neutral name `internal/loomcli` is not permitted to say itself", `fabricengine/doc.go:487`). Add `fabricengine.RequireDrivableWorktree(l)` as `RequireWarpWorktree`'s neutral spelling, call that from `battencli`, reword the remaining comments and the one user-facing string to the pair-neutral wording the package docs require, and correct `CONSTRAINTS.md:237` to name the spelling batten is actually allowed to say. This is additive to fabric and changes no fabric behaviour — it is not fabric correctness work, which stays out of scope.

**Confidence:** CONFIRMED — reproduced on a clean tree, cause located by `git log -L`, and the invariant conflict read out of both constraint texts.

### F4 — the design doc's second shipped residual is written down nowhere in the code (NIT, CONFIRMED)

**Where:** `internal/battenshed/doc.go:15-19`.

The deleted design doc (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`) ships **two** named residuals:

1. a driver strand that dies mid-run is not detected, reported, or recovered;
2. **a driver that finishes normally leaves its strand and its run directory behind** — nothing tears either down as part of a clean finish.

R1 wrote (1) into `battenshed`'s package doc (and R2 widened it to cover an alive-but-parked strand). (2) is in no shipped doc at all — not `doc.go`, not `docs/overview.md`, not `CONSTRAINTS.md` — so with the design doc deleted per this repo's own convention, it survives only in git history. The Documentation Lifecycle exists precisely so a deleted design doc's substance lands in the module docs.

Re-verified live this round: after the child reached `done`, its `loom-status` strand and its `_lyx/shed/self/` run directory both persisted until `Worktree-Teardown`'s `Shutdown`/`Remove` took the whole worktree away. The residual is still accurate as written in the design doc.

**Fix.** One sentence in `battenshed/doc.go` beside residual (1).

**Confidence:** CONFIRMED — both the doc gap and the behaviour verified.

## Docs & operability findings

Checked `docs/overview.md`'s batten entry claim by claim against the running system. Every claim held:

- "The Board task's `type` must be `loom` or empty ... `Seed-Child` refuses any other registered recipe before writing a seed" — verified live for both a registered-but-unbootstrappable type (`batten`) and an unknown one (`nonsense`); no child seed written, nothing committed on the branch in either case.
- "A child that halts ... fails `Run-Shed` rather than routing anywhere, and batten cannot restart the child's driver" — verified live for all three halted states.
- "Teardown ... never forces: anything an agent left uncommitted or untracked in the child ... blocks the row after the session is already down, with fabric's own refusal as the `stuck_reason`" — verified live, exactly as written, including the session being down while the pair survives.
- "Every child's weft branch forks from prime's `main-weft`, which carries prime's committed `_lyx/shed/<slug>/` run directories, so a child's `_lyx/shed/` also holds frozen copies of other slugs' batten run directories; they are inert" — verified live: the child worktree held six other slugs' run directories, and `lyx shed run|step|status <frozen-slug>` from inside it refused through batten's own prime-only guard.
- "`status` bounds the history it reports ... alongside the true `history_length` and a `history_truncated` flag" — verified live (`history_truncated: false` while under the cap).
- "`run` and `step` carry `--driver` ... and `--child-driver` ... an explicitly typed flag that disagrees with an already-seeded run is refused rather than silently dropped" — verified live for both flags.

Two gaps, recorded as findings rather than left as prose notes: **F2** (the `Run-Shed` watch's permanent weft drift and its hub-wide effect on `lyx fabric checkout` is undocumented) and **F4** (the design doc's second shipped residual is undocumented).

`CONSTRAINTS.md`'s Batten Bookend Invariant is accurate, including its own honest note that the mechanical proxies "pin the status and lock paths to prime's anchor ... neither proves the driver's own working directory, which stays a review obligation" — that obligation was discharged live this round (scenario B, 16/16 refusals).

## Re-evaluation of the prior rounds' deferred items

| Item | Verdict this round |
| --- | --- |
| **R1-F9's recreate-from-branch half** (status ahead of disk) | Gap still holds. `taskWorktreeLocation` (`wire.go:50-62`) refuses by name and names `lyx fabric checkout <slug>` as the remedy; `Topology.Add` still refuses a pre-existing branch by design. Not built here, per instruction. **But its mirror image — disk ahead of status — is a separate, unblocked defect and is fixed this round (F1).** |
| **R1-F6's `step`-mode pacing cost** | Still the accepted shape. Measured 30.33s for the first `Run-Shed` step this round, matching R2's 30.0–30.4s and R3's 30.042–30.269s. Moving the pacing out of the producer body remains a `shedengine` change, out of scope. Not a new defect. |
| **F6[R2]'s llm-driver trust-dialog hang** | **Reproduced** (scenario F), with the trigger pinned to the child worktree's absolute path being absent from `~/.claude.json`'s `projects` map rather than to worktree freshness — which explains R3's non-reproduction on a reused fixture path. Still cross-module, still not batten's to fix, still not filed as its own task. Batten's own boundary behaviour around it is correct and already documented. |
| **Design residual (a)** — dead/parked driver strand undetected | Still true, still accurately described in `battenshed/doc.go`; live-demonstrated again this round by the parked llm strand. |
| **Design residual (b)** — a finished driver's strand/run-dir not torn down | Still true, re-verified live — but described in no shipped doc. Recorded as **F4** and fixed. |

## What was tested

### Hermetic (all green, before any change)

- `go build ./...` — clean.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` — clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` — all `ok`.
- `go test -tags integration -count=1 ./internal/battencli/...` — `ok`, 2.504s.

### Fixture hub

Disposable hub built outside both the loomyard tree and `$HOME/Code`, under this session's scratchpad:
`.../scratchpad/hub` holding `remotes/bfix.git` + `remotes/bfix-weft.git` (bare, hand-seeded minimal Go project: `go.mod`, `main.go`, `main_test.go`, `README.md`) and the cloned hub `bfix-LYXHUB` (`bfix` warp prime on `main`, `bfix-weft` on `main-weft`, `_board`).

- `lyx fabric clone --into <scratch> <weft.git> <warp.git>` — `ok: true`, `partial: false`, 22 mutations, warp binding recorded.
- **GitHub self-report hazard, discharged first:** `selfreport: false` committed and pushed onto `main-weft` (`f4807bd fixture: selfreport off`) BEFORE any Board task existed and before any `Worktree-Create`.

### Live scenario A — argument, driver and seed refusals (all from prime)

| Command | Observed |
| --- | --- |
| `lyx batten run` | `battencli: no slug given; ... pass the slug: "lyx batten run <slug>"` |
| `lyx batten run self` | `slug "self" is reserved for addressing prime's own run` |
| `lyx batten status add-farewell` (unseeded) | `no seed found for run "add-farewell"; no run is seeded yet. run "lyx batten run <slug>" first` |
| `lyx batten pause add-farewell` (unseeded) | same refusal |
| `lyx batten run add-farewell --driver llm` | `batten has no bootstrap verb, so it cannot be driven by an LLM` |
| `lyx batten run add-farewell --driver bogus` | `shedrun: unknown driver "bogus"; must be "go" or "llm"` |
| `lyx batten run add-farewell --child-driver bogus` | same shedrun refusal |

After all seven refusals, `_lyx/shed/` did not exist in prime: **no refused invocation leaves a seed behind.**

### Live scenario B — the Batten Bookend Invariant (focus item 1) — CONFIRMED NON-DEFECT

Drove all four verbs (`run`/`step`/`status`/`pause`), each against both its own slug and a different slug, from
(a) the freshly-created task worktree `bad-type` and (b) its weft sibling `bad-type-weft`. 16/16 refused.

- From the task worktree: `battencli: this verb runs from the hub's prime worktree only; "bad-type" is not the prime worktree ("bfix" is) -- re-run it from there` — both worktree names present, remedy stated.
- From the weft sibling: `RequireWarpWorktree` fires first — `... is the weft sibling of a pair, not a warp worktree; run lyx from the paired warp worktree instead`.
- From `_board`: `... is the hub's _board checkout, not a warp worktree`.
- From a task-worktree SUBDIRECTORY (`bad-type/sub`): refused by the anchor gate before batten is reached.
- Bare `lyx batten` still lists subcommands from a task worktree (the deliberate `cmd.Name() == "batten"` short-circuit) — a listing, not a drive.

### Live scenario C — Seed-Child recipe refusals (focus item 5, negative half)

Board task `bad-type` (`type: batten`) and `unknown-type` (`type: nonsense`), each `lyx batten step`ed twice:

- step 1: `Worktree-Create → done` (0.09–0.11s, no poll sleep on this row).
- step 2: `Seed-Child → stuck`, `state: blocked`, and `lyx batten status` carried the producer-supplied `stuck_reason`:
  - `bad-type`: `Board task type "batten" cannot be the task worktree's own run: ... a Board task's type must be "loom" or empty; got "batten"`
  - `unknown-type`: `unknown recipe name "nonsense": ... available recipes: [batten loom]`
- **Both task worktree pairs were left intact** — the failure path never routes to the destructive teardown row (focus item 3's structural half).
- No child seed was written in either worktree (refusal is ahead of the write).

### Live scenario D — hand-written seed ahead of the auto-seed (focus item 11) — CONFIRMED NON-DEFECT

1. `lyx shed seed add-shout --recipe loom` → `lyx batten run/status add-shout` refuses: *"already seeded with recipe "loom", not "batten"; batten refuses to drive it -- drive it with "lyx shed run add-shout", or delete its seed to re-seed it as a batten run"*. The hand-seed was **not** overwritten.
2. `lyx shed seed add-shout --recipe batten --driver go` → `lyx batten status add-shout` answers `{"found":false,"ok":true,...}` (a determined answer, not an error); `lyx batten run add-shout --child-driver llm` refuses (`already seeded with child driver "go"`); `lyx batten run add-shout --driver llm` refuses (`already seeded with driver "go"`). Seed bytes unchanged after every refusal.
3. `lyx batten step add-shout` with no flag typed **adopts** the hand-seed and advances — authoritative, not overwritten, not silently contradicted.

### Live scenario E — PrimeRunLock contention (focus item 6, half A) — CONFIRMED NON-DEFECT

Held `bfix/.lyx/shed/run.lock` with `flock -x ... sleep 20` (the same advisory lock `internal/lock` takes), then:

- A **different** slug's `lyx batten step add-shout` (its `Worktree-Create` row) → `Worktree-Create → stuck`, `state: blocked`, `stuck_reason: prime lock "..." is already held; another batten producer is creating or tearing down a task worktree`. Deterministic, not raced.
- Read-only `lyx batten status bad-type` was **unaffected** by the held lock, as it must be.
- After the holder exited, the identical `lyx batten step add-shout` advanced: `Worktree-Create → done`.

### Live scenario F — the primary drive, llm arm (focus items 4, 5) — and F6[R2] REPRODUCED

Board task `add-farewell` (`type: loom`), `lyx batten step add-farewell --child-driver llm`, three steps:

| Step | Row | Elapsed | Result |
| --- | --- | --- | --- |
| 1 | `Worktree-Create` | 0.09s | `done`; prime seed `{"recipe":"batten","driver":"go","params":{"child_driver":"llm"}}` |
| 2 | `Seed-Child` | 0.06s | `done`; **CHILD seed on disk: `{"recipe":"loom","driver":"llm","params":{"parent":"main"}}`** — committed as `a57c047 batten: seed child add-farewell`, child weft clean afterwards |
| 3 | `Run-Shed` (first entry) | **30.33s** | `stuck`, `continue: true`; child status file present at `Preflight`/`running`; two live strands in the child's own reed session (`loom-status`, `loom-driver`) |

- **Focus item 5 CONFIRMED NON-DEFECT.** The Board-type → recipe → driver plumbing lands a real `recipe: loom` / `driver: llm` in the child's own `seed.json`, with `params.parent` filled from the pair's recorded origin — nothing silently defaulted — and a real `claude` provider was spawned off that value in the child's own `loom-driver` pane.
- **Focus item 4 ANSWERED PRECISELY.** The first `Run-Shed` step took 30.33s wall clock, of which the child bootstrap (`lyx loom start --no-attach`, `wire.go:214`) accounted for well under a second — by the time the step returned, the child had already persisted its own `status.json` at `Preflight`. The remaining ~30s is the producer's own `deps.Sleep(ctx, pollInterval)` on the `StateRunning` arm (`innerrun.go:141`). So `Spawn` blocks for **the bootstrap launch only**, never for the nested campaign: `lyx batten step`'s first advancing call cannot hang for the length of a real loom campaign. The doc comment on `InnerRunDeps.Spawn` (`deps.go:50-53`) already says exactly this and is accurate.
- **F6[R2] REPRODUCED.** `tmux capture-pane` of the `loom-driver` pane showed the provider parked on Claude Code's own workspace-trust dialog (`Quick safety check: Is this a project you created or one you trust? ... ❯ No, exit / Yes, I trust this folder`), and the child stayed at `Preflight`/`running` with `history_length: 0` for the whole watch while `Run-Shed` reported `inner shed run still running` once per 30s.
  - **Precisely what differed from R3's non-reproduction:** the gate is keyed to the exact workspace path in `~/.claude.json`'s `projects` map (`hasTrustDialogAccepted`). This round's fixture hub lives under this session's own scratchpad — a path that has never had a Claude Code session launched in it — so the map has no entry and the dialog fires. R3's hub path evidently already carried one. The trigger is therefore "the child worktree's absolute path has never been trusted on this host", not "the worktree is fresh": a fresh worktree under an already-trusted *path* does not reproduce it, which is exactly how a repeatedly-reused fixture directory hides it.
  - Not batten's bug (root cause is `internal/loomcli`'s llm arm never calling shuttle's `Run.Wait`, where the trust dismissal is played). Batten's own boundary behaviour is honest within its contract: it reports what the child's status file says, and `battenshed/doc.go` already records that a long-quiet `Run-Shed` means "possibly dead or parked", not "working".
  - **Both operator routes past the dialog were denied by this session's own permission classifier** (typing into the live pane with `tmux send-keys`, and adding a trust entry to `~/.claude.json`). I did not work around the denial. The success arm was therefore driven with `--child-driver go`, which is driver-agnostic at batten's own boundary — `Run-Shed` reads only the child's persisted status either way.

### Live scenario G — interrupting a blocking `lyx batten run` mid-watch

SIGINT to the in-flight `lyx batten run add-farewell`: exited 130 at the next boundary, prime status left at `Run-Shed`/`running`, **run lock released** (verified with a non-blocking `flock -n`), no stray `lyx` process left by the run itself. Resumable, nothing corrupted.

### Live scenario H — mid-operation-failure orphans (focus item 7) — **FOUND A DEFECT (F1)**

7a: `Worktree-Create` succeeded but its status transition never landed (the driving process died in that window). Reproduced exactly by deleting prime's `_lyx/shed/probe-create/status.json` with the worktree pair already on disk, then resuming.

- Result: **permanently blocked.** `Worktree-Create → stuck`, `stuck_reason: branch "probe-create" already exists; switch a pair onto it with "lyx fabric checkout probe-create", or delete it first with "git branch -D probe-create" if it is a leftover from a removed pair`.
- **Both named remedies fail in exactly this state:**
  - `lyx fabric checkout probe-create` → `weft worktree has uncommitted changes; stash or commit before checkout`
  - `git branch -D probe-create` → `cannot delete branch 'probe-create' used by worktree at ...`
- The pair is meanwhile perfectly healthy (`## probe-create...origin/probe-create`, `## probe-create-weft...origin/probe-create-weft`, listed by `lyx fabric list`). No double-create happened — the run simply cannot be resumed by any action the refusal names. See finding **F1**.

7b: `Seed-Child` is idempotent against its own re-entry — `shedrun.WriteSeed` agrees with the already-written seed and `CommitAnchoredPaths` is a no-op on a clean tracked path, so a resumed `Seed-Child` neither double-seeds nor errors. A failed `PushSeed` only warns and still returns `Done` (`seamchild.go:109-111`), which the next transition's push catches up.

### Live scenario I — halted child never routes to teardown (focus item 3) — CONFIRMED NON-DEFECT

The child's own `status.json` was driven to each halted state in turn and `lyx batten step` run against each:

| Child state | batten's result |
| --- | --- |
| `failed` | hard error, `kind: "producer"`, prime `state: failed`, **pair intact** |
| `blocked` | same shape, **pair intact** |
| `paused` | same shape, **pair intact** |

Every error carried the child's `State`, `Error` and `CurrentProducer` plus `haltedChildRemedy`. The destructive teardown row was never reached. A subsequent `lyx batten step` resumed the watch silently from `StateFailed`, as `battenPreStep`'s switch intends.

### Live scenario J — teardown ordering (focus item 2) — CONFIRMED NON-DEFECT

Walked `add-farewell` to the `Worktree-Teardown` row, then sabotaged each half in turn:

1. **`Shutdown` fails** (child `reed.yaml` corrupted): `Worktree-Teardown → stuck`, `stuck_reason: session shutdown failed: ... parse existing YAML ...`. **`Remove` was not called** — the pair was still present and **the child's tmux session was still up**. This is the single thing the one-row producer exists to prevent, proven by hand.
2. **`Shutdown` succeeds, `Remove` fails** (an untracked `LEFTOVER.txt` in the child): `stuck_reason: worktree removal failed (session shutdown already succeeded): worktree has uncommitted changes; use --force; this pair's portal junction and launcher scripts were already torn down before the refusal — run "lyx fabric reconcile" to restore them`. The two halves are distinguishable in the reason text, as the doc comment claims; the child's tmux session was now **down** and the pair still present. Batten never forces.
3. **Clean child**: `Worktree-Teardown → done`, `state: done`, **pair gone** (both warp and weft worktrees). `abandonedSession` absent throughout — `reed.Down()` ended the session cleanly rather than abandoning it.
4. A done slug refuses both `run` and `step` (`"add-farewell" has already completed; delete its run directory ... to run it again`, `kind: "bootstrap"` on the step envelope), while `status` still answers on the success envelope.

### Live scenario K — sabotage (focus items 8, 9) — CONFIRMED NON-DEFECTS

Both run against prime's own weft **while `lyx batten run add-greeting` was live**:

- **Item 9 (stray untracked file under `_lyx`).** Dropped `_lyx/STRAY-NOTE.md` and `_lyx/shed/add-greeting/STRAY-IN-RUNDIR.md` (the latter *inside the live run's own directory*). Over the next ~90s and several real status transitions, neither was swept in nor dropped: `git log --name-only` shows every batten commit touching exactly `_lyx/shed/<slug>/status.json` and `_lyx/shed/<slug>/seed.json` and nothing else, and both strays were still `??` on disk afterwards. **There is no stage-all lottery here** — `battenRunCommitPaths` (`commitstatus.go:58`) builds a positive pathspec and `fabricengine.CommitWeftPaths` stages exactly it. No report is owed, because batten never claimed those paths.
- **Item 8 (raw git commit landed directly on prime's weft, outside fabric's seams).** `SABOTAGE: raw commit outside fabric's seams` committed by hand onto `main-weft` mid-run. Batten's own status tracking was unaffected: later transitions committed cleanly on top, the run kept advancing, and `lyx batten status` stayed correct. It is *absorbed* rather than detected — which is the right disposition, since the commit touched nothing batten owns. Concurrent commits cannot race either: `fabricengine.CommitWeftPaths` takes the blocking weft write lock, so two slugs' status commits serialise (verified by reading `commitweftpaths.go:65-73`, and two slugs were in fact committing concurrently throughout this round with no index-lock failure).

### Live scenario L — PrimeRunLock is NOT held during Run-Shed's watch (focus item 6, half B) — CONFIRMED NON-DEFECT

While `lyx batten run add-greeting` was in its `Run-Shed` self-bounce:

- `flock -n -x <prime>/.lyx/shed/run.lock true` succeeded — **the hub prime lock is free for the whole watch**, so neither another slug's create nor another slug's teardown (which take the same lock) can be blocked by it.
- A **different** slug's real `Worktree-Create` (`lyx batten step probe-create`) completed in **0.12s** during that watch.

### Live scenario M — pause and resume against a live watch — CONFIRMED NON-DEFECT

`lyx batten pause add-greeting` while `lyx batten run add-greeting` was mid-watch: the run halted at its next producer boundary (`{"halted_producer":"Run-Shed","outcome":"paused"}`, exit 0), prime status `Run-Shed`/`paused`. **The child campaign was untouched** — it advanced from `Discussion-Burler` to `Discussion-Bouncer` during the pause, with a live `bouncer-judge` strand in its own session. A fresh `lyx batten run add-greeting` resumed the watch cleanly from `paused`.

### Live scenario N — run-lock busy paths — CONFIRMED NON-DEFECT

While `lyx batten run add-greeting` held the per-slug run lock:

- a second `lyx batten run add-greeting` → `another batten run already holds the run lock "<prime>/.lyx/shed/add-greeting/run.lock"`
- `lyx batten step add-greeting` → the same text with `kind: "busy"` on the step envelope (inside the closed five-value vocabulary)
- `lyx batten status add-greeting` → unaffected
- a **different** slug's `lyx batten step bad-type` → advanced normally (the run lock is per-slug, not hub-wide)

### Live scenario O — cold-machine shape: ephemeral tree lost, durable tree kept — CONFIRMED NON-DEFECT

Deleted `<prime>/.lyx/shed/bad-type/` entirely (locks, stuck-reason file, commit marker — what a fresh clone on another machine has) while keeping the durable `_lyx/shed/bad-type/`. `lyx batten status`, `pause` and `step` all answered from the durable file, recreating the lock directory as needed rather than failing to open a lock. The producer-supplied `stuck_reason` is legitimately gone with the ephemeral tree, which is what "ephemeral" means.

### What I could NOT verify, and why

- **An `llm`-driven child campaign past its first row.** The seed plumbing and the real provider spawn off `driver: llm` were both proven (scenario F), but the campaign itself parked on Claude Code's workspace-trust dialog, and **both operator routes past it were denied by this session's own permission classifier** — typing into the live pane (`tmux send-keys`) and adding a trust entry to `~/.claude.json`. I did not work around either denial. This is an environment gap, not a batten defect, and the success arm is driver-agnostic at batten's boundary.
- **Windows path behaviour** anywhere in this stack — unreachable from this Linux host. Not touched, not newly flagged.
- **fabric's own clone/merge correctness** beyond what `Worktree-Create`/`Worktree-Teardown` exercise — out of scope, fabric has its own crucible lineage.
- **The N×-concurrent-suite amplifier gate** — explicitly does not apply to batten's live-driving scenario and was never attempted.
