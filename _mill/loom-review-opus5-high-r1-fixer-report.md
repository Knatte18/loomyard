# `loom` — fixer report, round 1 (`opus5-high-r1`)

Job B for `_mill/loom-review-opus5-high-r1.md`. One commit per finding, on `loom-crucible-hardening`, never pushed.

## Summary

**17 findings. 14 fixed and committed. 3 recorded NOT-FIXED-THIS-ROUND** — two because the fix lives outside this campaign's
module, and one (F7) because it is a genuine feature addition rather than a hardening change.

F9 was in the out-of-module group until the operator decided it mid-round ("mouse should be on by default"), which is the one thing
that resolves a trade a reviewer should not settle alone; it is fixed.

| Finding | Severity | Disposition | Commit |
|---|---|---|---|
| F1 — fast-halting driver reports a false bootstrap failure | BLOCKING | **Fixed** | `loom: fix F1 …` |
| F2 — every mechanical gate discards its findings | MEDIUM | **Fixed** | `loom: fix F2 …` |
| F3 — a fresh pair has no module config | MEDIUM | **NOT-FIXED-THIS-ROUND** (fabric) | — |
| F4 — Plan-Write's rotation archives a live agent's output | BLOCKING | **Fixed** | `loom: fix F4 …` |
| F5 — pause/status die on any config fault | MEDIUM | **Fixed** | `loom: fix F5 …` |
| F6 — the Bouncer's focus directive never reaches the Burler | BLOCKING | **Fixed** | `loom: fix F6 …` |
| F7 — the pipeline cannot pass `Plan-Validate` | BLOCKING | **NOT-FIXED-THIS-ROUND** (feature) | — |
| F8 — the Discussion agent can rewrite loom's own config | MEDIUM | **Fixed** | `loom: fix F8 …` |
| F9 — scrolling an attached session injects arrow keys | MEDIUM | **Fixed** (operator decision) | `reed: fix F9 …` |
| F10 — `status --watch` reprints every second | MEDIUM | **Fixed** | `loom: fix F10 …` |
| F11 — `drive` needs the reed substrate it disclaims | LOW | **Fixed** | `loom: fix F11 …` |
| F12 — attach leaves pre-attach terminal content on screen | LOW | **NOT-FIXED-THIS-ROUND** (reed) | — |
| F13 — recipe header miscounts the `on_stuck`-less rows | NIT | **Fixed** | `loom: fix F13/F14/F15/F16 …` |
| F14 — `LoadConfig` does not validate the timeouts | NIT | **Fixed** | same |
| F15 — burler's pre-attempt archive skips `cancelErr` | NIT | **Fixed** | same |
| F16 — the `AwaitOperator` log-location claim is wrong | NIT | **Fixed** | same |
| F17 — the status strand never returns after a reed restart | MEDIUM | **Fixed** | `loom: fix F17 …` |

Two of the three BLOCKING findings are fixed. The third, F7, is the one that keeps the module from working, and its disposition is
argued in full below rather than waved at.

## What was implemented

Every fix carries its own commit with the full argument in the message; this is the shape, not a restatement.

**F1** — `dispositionForHandshake` in `internal/loomcli/bootstrap.go` turns `awaitRunLock`'s deliberate three-way result into a
two-way decision the verb assembles over, which is where that file already says its judgment belongs. `awaitRunLockChildDied`
proceeds to the terminal handover; only a genuine deadline refuses.

**F2** — all four mechanical gates (`preflightshed`'s Preflight, `loomshed`'s Loom-Preflight, Discussion-Validate and
Plan-Validate/Plan-Revalidate) log their determined findings before returning `Stuck`. Outcomes, pointers and routing are
untouched — this adds an account, not a behaviour. `internal/logger` joins `loomshed`'s Told-Geometry allowlist as a genuine new
dependency that resolves no geometry.

**F4** — `SingleLLMProducer` gains a `prepareFreshSpawn` seam that runs on the respawn branch only, between the failed attach probe
and the archive. `loomshed.NewPlanDirRotator` supplies Plan-Write's rotation through it, and `planWrite` keeps only its commit half
and now touches no files at all. It stays a distinct type from `discussionWrite` despite the two carrying identical logic, because
`loomrecipe`'s `shape_test` asserts the concrete producer type per recipe row and merging them would retire that guard.

**F5** — `wireStatusPathsOnly` gives `status` and `pause` the location and the two status paths and nothing else. `run` and `drive`
keep the full `wire()` and its early config refusal.

**F6** — `readRoundFocus` reads `focusPath` through `parseFocus`: one path builder, one parser, shared with the writer. The focus
file is hydrated into the round's prior-review context whenever it carries a directive; an APPROVED judge's mandatory-but-empty file
is not. `doc.go`'s pinned `focus.json` spelling moved with it. Fixtures now go through the real writer rather than hand-built
strings, which is what let the divergence hide in the first place.

**F8** — the Discussion stencil gains a write fence naming its two permitted files by marker (so the set cannot drift from Step 5),
fencing `_lyx/config/`, `_lyx/loom/`, `_lyx/plan/`, source and mutating git, and forbidding the agent from repairing a broken
environment instead of reporting it.

**F10** — `printStatusLinesOnChange` suppresses a line identical to the one last printed, with the poll/sleep seams and a bounded
poll count in the same shape `awaitRunLock` already uses.

**F11** — `drive` calls the same idempotent `reed.Up()` `run` does, and its help text stops claiming an independence from tmux the
verb never had.

**F17** — `resolveStatusStrandAction` makes liveness part of the add-or-keep question, replacing a tracked-but-dead entry rather
than leaving it to suppress the re-add forever.

## Not fixed this round, and why

### F7 — the pipeline cannot pass `Plan-Validate` (BLOCKING)

**This is the most important thing in the round and it is deliberately left open.** Recording the reasoning in full, because
"the reviewer found the blocking bug and did not fix it" deserves better than a shrug.

The minimal correct fix is not a fix, it is a feature. Both halves are needed and neither alone helps:

1. **Approval has to be produced by something.** `Plan-Bouncer`'s APPROVED settle is the only row that knows the plan passed
   review, so it must set `approved: true` in `00-overview.md`. That needs: a writer in `internal/planparser` (the Planparser
   Sole-Parser Invariant reserves plan-format writes to that package, so it cannot live anywhere else), a new `Approve func() error`
   field on `shedadapters.BouncerConfig` called on the approved branch before `Commit`, a new `approve_seam` recipe key with its own
   validation in `shedrecipe.bouncerEntry`, a new `Env.ApprovePlan` closure, and its fill in `loomcli.wire`.
2. **The pre-review gate must stop demanding it.** `Plan-Validate` runs *before* review and must skip `plan-unapproved`;
   `Plan-Revalidate` runs *after* the segment settles and must keep enforcing it. They share one `PlanValidate` engine, so that
   engine needs a recipe-authorable `require_approved` key — and `lyx loom validate-plan` has to pick a mode without breaking the
   **Gate Self-Check Parity Invariant**, which is a contract question, not an implementation one.

That is seven packages, two new recipe keys, a new `Env` field, and a new durable contract on a shipped recipe. `crucible/README.md`
is explicit that this class does not belong in a commit-per-fix hardening loop: *"a finding whose fix is genuinely LARGE (a
subsystem/feature addition, a cross-cutting refactor reaching outside the module) … record it fully, mark it NOT-FIXED-THIS-ROUND,
and the orchestrator spins it into its own mill-wiki task instead of letting a hardening round grow into feature work."*

There is also a design question a hardening round should not answer alone: whether approval belongs on the Bouncer at all, or on a
new row between `Plan-Bouncer` and `Plan-Revalidate`. Baking the wrong answer into the recipe is worse than leaving the gap
documented.

**What the orchestrator should know when spinning the task:**

- Everything downstream of row 8 is unverifiable until this lands, including the campaign's own "Webster chained from a real
  `Plan-Write` output" item. This is the highest-leverage single change in the module.
- The three contracts that contradict each other are `contracts/stencils/loom/loom-template-plan.md:79`
  ("Always write `approved: false` — you never self-approve"), `internal/planparser/validate.go:88-93` (`plan-unapproved`), and
  `contracts/recipes/loom-recipe.yaml`'s `Plan-Write → Plan-Validate → Plan-Write` edge.
- `contracts/specs/loom-plan-spec.md:206` frames `plan-unapproved` as "else refuse to **run**" — a consumer guard for Webster, not
  a format gate. `manifest/designs/loom.md` row 8 scopes `Plan-Validate` to the format checks. Both support splitting the check.
- F2's fix means the bounce loop now says why it is bouncing, so whoever picks this up will see
  `findings="plan-unapproved: plan frontmatter approved: is not true"` in the driver log immediately rather than having to
  reconstruct it out of band as this round did.

### F3 — a freshly-added pair has no module config (MEDIUM)

The defect is in `fabricengine`'s clone: the nine module configs are written into the weft prime's working tree and never staged or
committed, so `lyx fabric add` branches from a commit that does not carry them. It surfaces on loom's bootstrap path — the shipped
`run.sh` launcher `fabric add` drops cannot work on a fresh pair — but the fix is a fabric change, outside this campaign's module.
Recorded for the orchestrator. Worth pairing with the observation that the error text names the bare `lyx config reconcile` when the
remedy is `--apply`.

### F12 — attach leaves pre-attach terminal content on screen (LOW)

reed pins a fixed `width`/`height` and computes its whole layout against that box, never sets `window-size`, and both attach paths
are a bare `attach-session` with no size reconciliation and no screen clear. The pinned geometry and the attach argv live in
`internal/reedengine`/`internal/reedcli`. `internal/loomcli/bootstrap.go`'s `attachArgv` is loom's own duplicate of the same argv and
would need the same treatment, so the fix is one change spanning both modules. Left for the orchestrator.

## How each fix was verified

Hermetic gates were run after every fix; the live column is what makes the difference.

| Finding | Live verification |
|---|---|
| F1 | Same fast-halting `Preflight` run that previously printed `driver did not take the run lock` now proceeds to the handover. Reproduced on **two** independent hubs before the fix. |
| F2 | The same `Plan-Validate` bounce now logs `findings="plan-unapproved: plan frontmatter approved: is not true"`. |
| F4 | A real Plan-Write agent killed mid-run with 3 card files + `00-overview.md` on disk: the resume logged `shuttle: run attached` against the **same** `strandGUID`, left every plan file in place, and the attached run classified `Done` off them. |
| F5 | With `loom.yaml` truncated to two of seven keys — the exact damage that surfaced it — `status` and `pause` both succeed and `pause_requested` lands, while `drive` still refuses. |
| F6 | The same segment now runs with **no** focus warning at all, against the pre-fix `focus file absent … round-1-focus.json`. |
| F8 | Stencil content, pinned by test; no live re-drive of the agent's behaviour (see limits). |
| F10 | Restored strand pane carries **one** line at `history=0`, against `history=434` in fifteen minutes before. |
| F11 | With no reed session at all, `drive` brings the session up and proceeds into real producer work instead of erroring several rows deep. |
| F17 | `[('loom-status','',False), …]` → `[…, ('loom-status','%6',True)]`, and the status pane is visibly back. |
| F9 | A fresh reed boot against a `reed.yaml` carrying the new default reports `mouse on` from `tmux show-options -g mouse`. |

**Sabotage-proved** (the mechanism was neutered and the new test confirmed to fail, per `crucible/README.md`'s refinement 5 —
a green run that never executed the code is indistinguishable from a fix):

- **F10** — neutering the change predicate fails 3 of 5 table rows.
- **F6** — reintroducing the `.json` filename fails 13 assertions across the reader's own tests *and* the BurlerProducer integration test.
- **F4** — moving the preparation back above the probe reproduces the defect (the live agent's output file is moved away) and fails the `AttachFound` row.
- **F15** — removing the `cancelErr` consult reports the archive's `permission denied` where the test demands `context.Canceled`.

One test was **rejected and rewritten** for exactly this reason: F15's first version used an already-cancelled context, which Call's
entry check catches long before the archive step, so it passed without ever reaching the changed line. It now cancels from inside
the injected clock — which `archiveStaleOutputs` calls between the stat and the failing rename — and asserts the clock actually ran.

## Test commands and results

```
go build ./...                                                        PASS
go vet ./internal/... ./cmd/...                                        PASS
go vet <the nine review-prompt packages>                               PASS
go test -count=5 <the nine packages> ./cmd/lyx/...                     PASS (all ten ok)
go test -count=1 ./...                                                 PASS (whole repo)
go test -tags smoke ./internal/loomcli/... \
    -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -v -count=1  PASS (1.04s)
```

The EXECUTION BAN was respected throughout: `TestSmokeBurlerClusterCleanFan` and `TestSmokeBurlerClusterRogueFork` were never run,
and no bare `-run Smoke` pattern was ever used. Never more than one live-substrate invocation at a time.

## Changed files

Production:

- `internal/loomcli/` — `bootstrap.go`, `run.go`, `cli.go`, `wiring.go`, `drive.go`, `status.go`
- `internal/loomshed/` — `planwrite.go`, `planvalidate.go`, `discussionvalidate.go`, `loompreflight.go`
- `internal/preflightshed/preflight.go`
- `internal/shedadapters/` — `singlellm.go`, `focus.go`, `burler.go`, `doc.go`
- `internal/shedrecipe/` — `entries_planwrite.go`, `entries_discussionwrite.go`, `entries_singlellm.go`
- `internal/loomengine/config.go`
- `internal/shuttleengine/wait.go` (doc only)
- `contracts/recipes/loom-recipe.yaml` (comment only), `contracts/stencils/loom/loom-template-discussion.md`
- `internal/reedengine/` — `template_posix.yaml`, `template_windows.yaml`, `doc.go` (F9, on the operator's decision)

Tests:

- `internal/loomcli/` — `bootstrap_test.go`, `status_test.go`, `wiring_test.go`
- `internal/loomshed/` — `planwrite_test.go`, `gatefindings_test.go` (new), `seam_enforcement_test.go`
- `internal/preflightshed/findings_test.go` (new)
- `internal/shedadapters/` — `focus_test.go`, `singlellm_test.go`, `burler_test.go`
- `internal/loomengine/config_test.go`
- `internal/loomrecipe/shape_test.go`
- `contracts/stencils/discussiontemplate_test.go` (new)

Docs, in the same commits as the fixes they document:

- `manifest/designs/loom.md` — the strand's print-on-change rule, the bootstrap's handshake dispositions, ladder step 3's
  fresh-spawn-preparation rule, pause/status's config independence, and the Discussion producer's write fence.
- `internal/shedadapters/doc.go` — the focus-file contract and the `prepareFreshSpawn` ordering.
- `contracts/recipes/loom-recipe.yaml` — the full `on_stuck`-less row set.

`manifest/roadmap.md` is deliberately untouched: this is a hardening pass, and the project rule reserves roadmap moves for
completing or adding a planned item.

## Limits of this round, stated plainly

- **F7 is open, and with it everything past row 8.** `Plan-Revalidate`, `Batchifier`, `Webster`, `Webster-Review`, `Publish` and
  `Finalize` were never reached, by this round or any before it.
- **`Plan-Review` and `Webster-Review` were never driven end to end.** Only `Discussion-Review` completed a full
  seed → round → APPROVED → commit cycle. The other two are the same two adapters with different config, and their construction is
  exercised, but that is not the same as having been run.
- **F4's fix is verified live; F4's original failure was not.** The defect is CONFIRMED by trace and the fix is confirmed by a real
  crash-and-resume, but I did not stage the pre-fix timeout failure itself, because F7 makes `Plan-Write` bounce before that window
  is interesting.
- **F8 was fixed in the stencil and pinned by test, not re-driven.** Whether a live agent honours the fence is a question about the
  agent, and a single run would not settle it either way.
- **F12's visual half needs eyes I do not have.** The operator supplied it; the code-side mechanism is mine.
- **No concurrency stress.** Out of scope by the campaign's own merge bar and cost declaration.

## Substrate teardown

Both reed servers created during this round were killed. Verified:

```
$ pgrep -a -f 'tmux -L lyx-'              -> zero lyx tmux servers
$ pgrep -a claude | grep -v 'claude -c$'  -> zero stray claude agents
```

One orphan had to be killed by hand to reach that, and it is worth recording rather than glossing: after
`tmux kill-server`, a `Plan-Write` agent process (a `claude` launched into a pane) **survived its pane's death** and had to be
killed by pid. Whether reed's own `down`/`remove` reaping covers that path — it reaps pane children, and this was a
server-level kill, not a pane-level one — is a reed question this round did not chase. Flagging it for the orchestrator as a
possible follow-up rather than asserting it is a defect.

The surviving `claude -c` processes are the operator's own. The two scratch hubs and their bare remotes are left on disk under the
session scratchpad for the orchestrator to inspect; nothing outside it was touched, and no other worktree's git was used.

## Merge-readiness

**NOT READY**, and the reason is one finding: while F7 stands, `loom` cannot complete a task. Every other finding this round raised
is closed, and the module is materially better — the review segment's targeting channel works for the first time, a crash mid-plan
no longer destroys a finished plan, the emergency brake survives a config fault, and a run that halts now says why. None of that
changes the verdict. Fix F7 and the merge question becomes worth asking again.
