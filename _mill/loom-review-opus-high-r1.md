# `loom` — crucible round 2 review (tag `opus-high-r1`)

> Independent, clean-room review + fix round against `loom`, per `_mill/loom-review-prompt.md`.
> Worktree `/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2`, branch `loom-crucible-hardening-round2`.
> This file is written incrementally during Job 1 and committed after each meaningful append.

## Executive summary

_(written last)_

## Did a full live pipeline run complete this round?

_(answered when the live driving concludes — see "What was tested")_

## Scope assessment

**Design intent vs shipped: the producer list matches, and the two counts diverge exactly as
documented.** `contracts/recipes/loom-recipe.yaml` carries seventeen rows;
`manifest/designs/loom.md`'s table carries fifteen entries; the doc's own "The table and the
shipped recipe diverge deliberately" paragraph accounts for the delta precisely — the table's
`Plan-Sweep` row (absent from the recipe) against the recipe's three collapsed segment pairs.
I checked row-for-row: `Preflight`, `Loom-Preflight`, `Discussion-Write`,
`Discussion-Validate`, `Discussion-Bouncer`/`Discussion-Burler`, `Plan-Write`,
`Plan-Validate`, `Plan-Bouncer`/`Plan-Burler`, `Plan-Revalidate`, `Batchifier`, `Webster`,
`Webster-Bouncer`/`Webster-Burler`, `Publish`, `Finalize`. No row is stubbed: every one
resolves to a real producer through `internal/shedrecipe`'s registry, and the live run below
exercised them in order.

**Deliberate absences, correctly absent.** `Plan-Sweep` is a documented Someday stub with its
own build-order note, and `Plan-Write`'s stencil names its absence as the normal degraded
state rather than an error — the design doc, the recipe, and the stencil all agree.

**Routing matches the doc's stated rules.** Eight rows carry no `on_stuck` and therefore
escalate rather than bounce — the recipe header names five gates (`Preflight`,
`Loom-Preflight`, `Batchifier`, `Publish`, `Finalize`) plus three producers
(`Discussion-Write`, `Plan-Write`, `Webster`). I verified all eight against the file. Every
review segment carries a non-empty shared `segment:` label on both of its rows, which
`shedengine.validate` (`validate.go:83`) requires for the mutual `OnStuck` edges to build at
all.

**F7's mechanism is shipped as designed, and its four halves agree.** `approve_seam: plan` is
set on `Plan-Bouncer` and nowhere else; `bouncerEntry` (`entries_bouncer.go:106`) accepts only
`"plan"` and guards the Env closure with `requireSeam`; `Bouncer.settle`
(`bouncer.go:344-354`) calls `Approve` strictly before `Commit` and skips `Commit` entirely
when `Approve` fails; `Plan-Validate` carries no `require_approved` key (format-only mode,
`planparser.ValidateFormat`) while `Plan-Revalidate` carries `require_approved: true`
(`planparser.Validate`, including `checkApproved`). The `Plan-Burler` profile's `fasit`
instructions explicitly forbid the fixer from writing the `approved:` key itself. Nothing here
is a stub or a partial landing.

**Shipped-beyond-scope: none found.** No row, verb, or config key exists that the design docs
do not license.

**Deferred-that-should-ship: none found for loom itself.** The two things this round would
most like to see shipped are neither loom's scope nor loom's layer — an attach/reclaim probe
for the review-segment producers (F0, fixed here) and same-vintage `lyx` on a spawned agent's
PATH (F2, recorded as NOT-FIXED-THIS-ROUND).

## Findings

Twelve findings. Recorded provisionally as they were spotted (so the ids are in discovery
order, not severity order); this index is the severity ranking, and each entry links what the
finding is to how it was established.

| Severity | Id | One line | Evidence |
|---|---|---|---|
| **BLOCKING** | F0 | A driver crash mid review-segment round spawns a DUPLICATE agent on the same artifacts | reproduced live twice, on two rows, plus a counted sweep of all spawn sites |
| MEDIUM | F1 | `require_pr_to_base: []` is unrepresentable, and a longer list is silently truncated by `config reconcile --apply` | reproduced live; second half reproduced on an independent hub |
| MEDIUM | F2 | An agent loom spawns resolves `lyx` from PATH and can silently rewrite the hub's stencils mid-run | observed unprompted in this round's own run, then reproduced under control on a third hub |
| MEDIUM | F10 | A resumed run reports `paused`/`blocked`/`failed` for the whole first producer call | reproduced live, with a real judge session in the pane at the same instant |
| LOW | F3 | `BurlerProducer`'s doc claims the Bouncer shares its round-completion predicate; it does not | traced |
| LOW | F4 | `Batchifier` and the `Webster` row swallow `batcher.Active`'s error with no log line | traced, backed by a 16-site sweep |
| LOW | F6 | `lyx loom status --watch`'s help states the exact behaviour the code exists to prevent | confirmed against live `--help` output |
| LOW | F7 | `docs/overview.md`'s loom.yaml key list omits `review`/`review_timeout_min` | confirmed against the shipped template and `Config` |
| LOW | F8 | The Bouncer's clear-and-re-seed discards an approved review generation with no log line | traced, with the existing test that pins the path |
| LOW | F9 | `lyx loom --help` claims the shed engine "already drives Hardener" and omits every review segment | confirmed against live `--help` output |
| LOW | F11 | The card shape the plan stencil mandates is forced onto `Custom`, disabling `path-missing` on its edit targets | observed in the live run's own plan — **NOT-FIXED-THIS-ROUND** |
| NIT | F5 | `burlerRoundFileSet`'s `entry`/`field` parameters are dead, making two config errors indistinguishable | traced |

Two further items are recorded inside their parent findings as **NOT-FIXED-THIS-ROUND**
because each fix reaches well outside this module: F1's `Reconcile` truncation half (the
sequence merge model, shared by all ten module configs) and F2's PATH half (the shuttle/reed
spawn layer).

### F0 — a driver crash mid review-segment round spawns a DUPLICATE agent on the same artifacts — BLOCKING — CONFIRMED (reproduced live, twice-observed processes)

`internal/shedadapters/burler.go:238` (`BurlerProducer.Call`), and the same structural gap at
`internal/shedadapters/bouncer.go:399` (`runSeedSpawn`), `internal/shedadapters/bouncer.go:532`
(`judgeCall`), and `internal/shedadapters/webster.go:74` (`WebsterProducer.Call`).

`manifest/designs/loom.md`'s "Crash recovery — resume on output files, not live processes"
section states the ladder as loom's own invariant:

> 2. **Else, is the agent's session still alive?** … A match: re-attach, just wait on its
>    `Stop` hook (do **not** respawn — that would duplicate).

That ladder is implemented in exactly ONE producer: `SingleLLMProducer`, whose `Call` probes
`p.shuttle.Attach(spec)` before archiving anything (`singlellm.go:118`). **Every other
LLM-spawning producer in loom's list has no probe at all.** `BurlerProducer.Call` goes
straight from `highestCompleteRound` to `archiveStaleOutputs` to `p.runner.Run`; `Bouncer`'s
seed and judge calls go straight to `b.cfg.Shuttle.Run`; `WebsterProducer.Call` goes straight
to `p.run(...)`.

**Reproduction, live, with process-level evidence.**

State before the crash — one real Discussion-Burler round in flight:

```
$ .dev-bin/lyx loom status
{"activity":{"now":"Discussion-Burler","last":"Discussion-Bouncer → stuck",...},
 "current_producer":"Discussion-Burler","state":"running","history_length":5,...}
$ .dev-bin/lyx reed status
 strands: loom-status (%0), burler:1:3255aa42 (%4, live)
driver pid 2086870 = ".dev-bin/lyx loom drive"
burler agent pid 2089235 = "claude ... round-3829329960 ... --dangerously-skip-permissions"
```

Hard-kill the driver only, leaving its agent alive — the exact crash the doc's step 2 exists
for:

```
$ kill -9 2086870
$ ps -p 2086870      -> gone
$ ps -p 2089235      -> 2089235 Sl+      (agent still alive)
$ .dev-bin/lyx loom status
  current_producer "Discussion-Burler", state "running"   (unchanged, as expected)
```

Resume exactly as `loom.md` documents ("stop anywhere … the next `lyx run` continues where it
left off"):

```
$ lyx loom run
$ .dev-bin/lyx reed status
 strands: loom-status (%0),
          burler:1:3255aa42 (%4, live),      <-- the ORPHAN from the dead driver
          burler:1:c93847f8 (%5, live)       <-- the RESPAWN
$ tmux -L ... list-panes -a
 %4 2089153 claude
 %5 2093019 claude
```

Two live `claude` agents, both `--dangerously-skip-permissions`, and both told to write the
**same two files**:

```
$ grep -l 'reviews/discussion/round-1-review.md' <wt>/.lyx/burler/round-*/instruction-*.md
 .lyx/burler/round-3441524935/instruction-2-review.md   (respawn)
 .lyx/burler/round-3441524935/instruction-3-fix.md
 .lyx/burler/round-3829329960/instruction-2-review.md   (orphan)
 .lyx/burler/round-3829329960/instruction-3-fix.md
```

Both round directories point at
`<wt>/.lyx/loom/reviews/discussion/round-1-review.md` and its `round-1-fixer-report.md`
sibling.

**Why this is BLOCKING, not cosmetic.**
1. Two agents interleave writes to the same review and fixer-report files. Whichever finishes
   second wins a file the other is mid-write on, and the Bouncer then judges an artifact that
   is a splice of two independent reviews.
2. `archiveStaleOutputs` runs in the respawn before either has written, so it cannot
   deduplicate; the orphan's later write simply lands on top with nothing archived.
3. Both rounds are `fix-scope: overlay` here and write into `_lyx/discussion/` concurrently,
   so the *reviewed artifact itself* is edited by two agents at once. On the
   **`Webster-Burler`** row the same crash is worse: that row is `fix-scope: source` and
   commits per fix to the warp repo, so two agents would interleave commits.
4. It doubles the real LLM cost of every crashed round, silently.
5. Nothing anywhere reports it — no warning, no envelope field, no status-file entry. The only
   way an operator sees it is by looking at the pane list.

The run could not proceed correctly without manual intervention: the orphan had to be removed
by hand before the segment could settle.

**Counted, not asserted.** I enumerated every site in loom's producer list that starts a real
provider session, and classified each by what it does on a resume, rather than reporting only
the row I happened to crash. Method:

```
grep -rn "\.Run(spec\|Shuttle.Run\|runner.Run\|StartMaster\|Attach(" --include=*.go \
  internal/shedadapters internal/mergeresolve internal/burlerengine internal/websterengine \
  | grep -v _test
```

| Spawn site | Rows it serves | Resume behaviour |
|---|---|---|
| `shedadapters/singlellm.go:118,143` | `Discussion-Write`, `Plan-Write`, generic `SingleLLM` | **probe** — `Attach` before archive, attaches to a live run |
| `shedadapters/bouncer.go:436` (`runSeedSpawn`) | all three Bouncers' seed call | **nothing** — archives, then `Run` |
| `shedadapters/bouncer.go:532` (`judgeCall`) | all three Bouncers' judge call | **nothing** — archives, then `Run` |
| `shedadapters/burler.go:341` | all three Burlers' round | **nothing** — archives, then `runner.Run` |
| `shedadapters/webster.go:74` → `websterengine/runlevel.go:523` | `Webster` | **reclaim** — `reclaimEntryTimeStrands` (`runlevel.go:260`, called at `:393` before anything acts on the loaded state) stops a leftover live Master first |
| `mergeresolve/mergeresolve.go:90` | `Publish`, `Finalize` conflict resolver | **nothing**, but out of loom's scope and reached only on a merge conflict |

So the picture is sharp: of loom's own five spawn sites, **two solve this (by two different
mechanisms — attach, and reclaim) and three do not**, and the three that do not are exactly the
review segments' own — the rows a run spends most of its wall-clock inside.

What this enumeration cannot see: a spawn reached through a seam whose method is not named
`Run`/`StartMaster`/`Attach` (there is none in this tree today), and burlerengine's own
internal fork spawns under `cluster-fan`, which loom's recipe never configures on any row.

Fix landed this round: give `BurlerProducer` and the `Bouncer`'s two spawn paths the same
live-agent probe `SingleLLMProducer` already has, so a resume attaches to a still-live round
instead of respawning over it. See the fixer report.

### F10 — a resumed run reports `paused`/`blocked`/`failed` for the whole first producer call, while a real LLM session is already spawning — MEDIUM — CONFIRMED (reproduced live)

`internal/shedengine/run.go:67-124`. The loop reads the status file (step 1), short-circuits
only on `StateDone`, checks pause/cancellation (step 3), and then calls the producer (step 4)
— **without ever writing `StateRunning`.** The first persist happens only when that producer
call *returns*, which for every LLM row in loom's list is minutes later. Until then the status
file still carries the state the run was resumed FROM.

`run.go:94` documents the routing half of this deliberately — "StateBlocked and StateFailed
deliberately do not short-circuit -- the loop proceeds and re-calls current_producer, which is
how a human resumes" — which is right. What is missing is that the file is never updated to
say so.

Reproduced live, immediately after the graceful-pause resume in scenario 5:

```
$ lyx loom run                    # 17:37:5x — resumes from state "paused"
$ lyx reed status                 # 17:38:18
 strands: loom-status (%0),
          bouncer-judge:1:1be2c1ef (%10, LIVE)     <-- a real judge session is running
$ ps -eo pid,etime,args | grep 'loom drive'
 2110778  00:21  .dev-bin/lyx loom drive           <-- the driver is alive and working
$ lyx loom status                 # same moment
 {"current_producer":"Plan-Bouncer","state":"paused", ...}   <-- says PAUSED
```

So the module's only observability surface — and `lyx loom status --watch`'s one-line strand
pane, which `loom.md` says exists "so the operator sees what the Go driver is *doing*" — reads
`loom paused | now Plan-Bouncer | last Plan-Burler → stuck` while a real, cost-bearing LLM
session is live in the pane directly below it.

Why this matters more than a cosmetic lag: the window is exactly the one an operator watches
hardest. Having just typed `lyx loom run` to resume, they look at the strand to see whether the
resume took. It says `paused`. The honest reading of that display is "my resume did not take",
and the natural next action is to run `lyx loom run` again — which is harmless today only
because the run lock refuses the second driver, not because the display was right.

The `blocked` and `failed` variants are worse still: a resumed run also keeps displaying the
STALE `error` text (`Activity.Wait` is composed from it at `activity.go:24`), so the pane
asserts a specific failure reason that the run is at that moment already retrying past.

Fix: after step 3's pause/cancellation check passes and before step 4 calls the producer,
persist `StateRunning` when the state just read is not already `StateRunning` — clearing the
stale error in the same write. One extra status write per `Run` invocation, and only when the
file was not already `running`.

### F1 — `require_pr_to_base: []`, the only way to say "never open a PR", is rejected by the config loader — MEDIUM — CONFIRMED

`internal/yamlengine/reconcile.go:78` (`MissingKeys`) + `internal/yamlengine/reconcile.go:137`
(`collectLeafPathsHelper`) + `internal/landingshed/template.yaml:1`.

`internal/landingshed/publish.go:98` gives `Publish` an early-`Done` branch taken whenever the
task's parent branch is **not** in `Config.RequirePRToBase`, and `landing.yaml`'s own comment
says the key is "a list rather than a bool because tasks branch off other tasks". The natural
and only general way to express "this hub never requires a pull request" — the correct
configuration for any repo whose remote is not GitHub — is the empty list.

`MissingKeys` collects **sequence elements** as leaf key-paths (`require_pr_to_base[0]`), so
shortening the shipped one-element list to `[]` is reported as a missing key and `Load` refuses.

Reproduced live, first attempt at the live pipeline run:

```
$ .dev-bin/lyx loom run          # in the fixture pair, landing.yaml carrying require_pr_to_base: []
{"error":"config file .../greet-suffix/_lyx/config/landing.yaml: missing keys:
 require_pr_to_base[0]; run \"lyx config reconcile\"","ok":false}
```

The remedy the message names makes it worse: `lyx config reconcile` re-adds the template's
`"main"` element, so the operator is told to undo the very edit they meant.

The same defect applies to every list-valued config key in the repo, not just this one — no
list can ever be shortened below the template's own length, and the shipped templates'
list-valued keys are exactly the ones an operator is most likely to want to empty.

Impact for loom specifically: with the empty list unreachable, a loom run in a
non-GitHub-remote hub cannot reach `Finalize` at all — `Publish` carries no `on_stuck`
(deliberately, per the recipe header), so its stuck verdict is `RunBlocked`, terminal.

Workaround used for this round's fixture, recorded honestly: a one-element list naming a
branch that is never the parent (`["__no_pr_required__"]`).

**The same root cause bites in the other direction too, and that half IS data loss.**
`collectLeafPathsHelper` (`reconcile.go:176-183`) models each sequence ELEMENT as its own
named config key (`require_pr_to_base[0]`, `[1]`, …), and `applyExistingOverrides`
(`reconcile.go:117`) copies only `Value`/`Tag`/`Style` onto matching template leaves — it
cannot resize the template's sequence. So a user list LONGER than the template's is truncated
to the template's length on `lyx config reconcile --apply`.

Reproduced on the independent probe hub:

```
# landing.yaml hand-edited to a three-entry list
require_pr_to_base: ["main", "develop", "release"]

$ lyx config reconcile
 ...{"module":"landing","removed":["require_pr_to_base[1]","require_pr_to_base[2]"]}...
$ lyx config reconcile --apply
 ...{"module":"landing","applied":true,"removed":["require_pr_to_base[1]","require_pr_to_base[2]"]}...
$ head -1 .../landing.yaml
require_pr_to_base: ["main"]          # "develop" and "release" are gone
```

Two operator-authored values destroyed. It is reported — under `removed`, as though they were
stale keys the template had dropped — which is precisely backwards: they are the operator's
configuration, and the template's own single entry is the default they replaced.

Suggested fix, split by blast radius:
- **Fixed this round (narrow, relaxes an error only):** `MissingKeys` requires the sequence
  KEY, not its individual elements — a template list is a default, not a minimum length. This
  is what makes `require_pr_to_base: []` loadable and therefore `Publish`'s documented
  no-pull-request mode reachable, which is the loom-visible half.
- **NOT-FIXED-THIS-ROUND:** the `Reconcile` truncation. Fixing it means changing the merge
  model for sequences across every module's config (replace the template's sequence node
  wholesale rather than overriding element leaves), in a shared leaf package all ten module
  configs load through. That is a cross-cutting change to shared config handling, not a
  loom hardening fix, and it deserves its own task with its own test matrix. Recorded here in
  full, with the reproduction above, for the orchestrator to spin out.

### F2 — an agent loom spawns resolves `lyx` from PATH, so it can silently rewrite the hub's stencils mid-run — MEDIUM — CONFIRMED

`cmd/lyx/stencilseed.go:29` (`seedStencils`, the root `PersistentPreRunE` pass) +
`internal/stencilstore/reconcile.go:94` (`StateUntouched` handling) +
`contracts/stencils/loom/loom-template-discussion.md:31` (`lyx board get`),
`contracts/stencils/loom/loom-template-plan.md:159` (`lyx loom validate-plan`),
`contracts/stencils/webster/webster-template-master.md:10` (`lyx webster ...`).

Every stencil loom's agents read tells them to run `lyx` verbs, and an agent resolves `lyx`
from **PATH**, never from the driver's own `os.Executable()`. `lyx`'s root pre-run seeds and
**commits** the hub's stencils from its own embedded registry on every invocation. So any
`lyx` on the agent's PATH whose vintage differs from the driver's silently rewrites the very
prompt files the run's later rows will read at call time (the Stencil Ownership Invariant
makes the on-disk copy authoritative), and commits the rewrite to the board repo.

**Observed unprompted, in this round's own first live run.** `lyx loom run` started at
17:11:39; nine seconds later the board repo gained:

```
$ git -C <hub>/_board log --format='%h %ad %s' --date=iso
41be510 2026-08-26 17:11:48 +0200 lyx: seed stencils     <-- during the run
...
$ git -C <hub>/_board show --stat --format='' 41be510
 _lyx/stencils/loom/loom-rubric-plan-review.md  | 10 +++++-----
 _lyx/stencils/loom/loom-template-discussion.md | 26 +-------------------------
 _lyx/stencils/loom/loom-template-plan.md       |  4 ++--
```

What it removed is exactly round 1's own work plus documented shipped behaviour:
- the 26-line **"## What you may write"** fence `manifest/designs/loom.md`'s
  "Discussion producer detail" section describes as shipped (`_lyx/config/` etc. off limits),
- `loom-template-plan.md`'s `` `Plan-Bouncer`'s approved settle writes it to `true` `` line,
  reverted to "a future review gate flips it to `true`" — F7's own stencil half,
- `loom-rubric-plan-review.md`'s Plan-Revalidate wording — F7's other stencil half.

**Controlled reproduction (the sabotage-style proof, on an independent third hub).**

```
$ .dev-bin/lyx fabric clone .../probe-weft.git .../probe.git --into .../hub3
$ cd <hub3>/probe && .dev-bin/lyx board list          # dev binary seeds
  -> all three loom stencils MATCH contracts/stencils/ byte-for-byte (stamp line aside)

$ cd <hub3>/probe && /home/knatte/.local/bin/lyx board list   # ONE stale-lyx invocation
  -> loom-template-discussion.md   30 diff lines vs source
  -> loom-template-plan.md          9 diff lines vs source
  -> loom-rubric-plan-review.md    17 diff lines vs source
  -> new board commit "lyx: seed stencils", same 3 files, 8 insertions / 32 deletions
```

**The damage is sticky and one-way.** The newer binary runs in `ModeDev`, whose
`StateUntouched` branch refuses to refresh and only emits
`logger.Warn("stencilstore: dev build does not refresh an untouched stencil", ...)` — three
WARN lines before every single `lyx` envelope, forever, with **no remedy named**. The remedy
exists (`lyx stencil sync`, "Force-refresh every stencil against the shipped registry, even
from a -dev build") but nothing at the point of failure says so.

Consequences for loom specifically, in order of severity:
1. loom's LLM rows can execute under prompt text that does not match the binary driving them,
   with the only signal being a warning that reads as benign.
2. It is not only prompts: `lyx webster begin-batch/await-batch/record-batch` and
   `lyx loom validate-plan` are run BY the agents, so the Webster phase can be driven by a
   different lyx vintage than the driver, against the driver's own on-disk state.
3. Every such invocation writes a commit into the shared board repo, so two machines on one
   board remote flap the stencils against each other indefinitely.

Fixes, split by size:
- **Fixed this round (cheap, real):** the dev-mode refusal now names `lyx stencil sync` and
  says the run will use the older on-disk text, so the warning is actionable instead of
  decorative.
- **NOT-FIXED-THIS-ROUND (cross-cutting):** making a spawned agent inherit the driving
  binary's own directory at the front of its PATH belongs in the shuttle/reed spawn layer,
  outside this module and outside a hardening round's commit-per-fix loop. Recorded here for
  the orchestrator to spin into its own task.

### F11 — the card shape `Plan-Write`'s own stencil mandates is forced onto `Custom`, which silently disables `path-missing` on its edit targets — LOW — CONFIRMED (observed in the live plan) — NOT-FIXED-THIS-ROUND

`contracts/stencils/loom/loom-template-plan.md` (card rules) + `internal/planparser/validate.go:174`
(`checkCardTypeMissing`) + `internal/planparser/validate.go:545` (`checkPathMissing`).

Three shipped rules collide:

1. The plan stencil requires a card to **bundle its own test**: "implementation plus test for
   the same behaviour land together; only pure refactors/renames may rely on existing tests
   instead."
2. The same stencil requires **exactly one** bold type label per card, and
   `checkCardTypeMissing` enforces it: `card %d carries %d type labels; exactly one is required`.
3. `checkPathMissing` skips `Create` and `Custom` cards' targets entirely.

A card that edits an existing file AND creates a new test file therefore has no correct
single label — it is an `Edit` and a `Create` at once — so the only label that fits is
`Custom`. And `Custom` is exactly the label whose targets escape existence checking.

Observed, unprompted, in the live run's own plan — the very first card of the very first
real plan `Plan-Write` has ever produced end to end:

```
$ head -6 _lyx/plan/01-greet-trim.md
# Card 1 — greet-trim

**Custom:**
- `greet.go`
- `greet_test.go`
```

`greet.go` is an edit target that exists; `greet_test.go` is a create target that does not.
Because the card is `Custom`, neither was existence-checked. Had the plan writer typo'd
`greet.go`, `path-missing` would have stayed silent — the plan would have passed both
`Plan-Validate` and `Plan-Revalidate` and reached Webster with an unresolvable target.

This is not a mistyped card. `manifest/designs/loom.md`'s Plan-Review rubric calls `Custom`
"a last resort … never as a shortcut around correct typing", and warns that "a mistyped one
silently escapes two checks the rest of the plan is held to" — but here correct typing IS
`Custom`, for the single most common card shape the format asks authors to write. The round's
own `Plan-Bouncer` judge noticed the symptom and filed it as a NIT ("the `Custom` type label
carries no stated rationale"), which is the rubric working as designed and still not reaching
the real issue.

**NOT-FIXED-THIS-ROUND**, and deliberately so: every available fix is a contract change, not a
bug fix — allow a second type label, add an `EditAndCreate`-shaped type, or make
`checkPathMissing` classify a `Custom` card's targets individually by whether some other card
creates them. Each reaches `planparser`, the plan stencil, the Plan-Review rubric,
`plan-card-format.md`, and webster's own consumption of `Targets`, and each needs its own
check-matrix. That is a design task, not a hardening round's commit-per-fix loop. Recorded in
full for the orchestrator to spin out.

Severity is LOW rather than MEDIUM because the downstream failure is loud rather than silent:
a Webster fork handed a target that does not exist fails its batch visibly. The cost is a
wasted real batch, not a wrong result that ships.

### F3 — `BurlerProducer`'s doc comment claims the Bouncer uses the same round-completion predicate; it does not — LOW — CONFIRMED

`internal/shedadapters/burler.go:177-178` states: "The same pair predicate is what the
segment's Bouncer uses to tell its seed call from its judge call, so the two sides run the
same test."

It does not. `BurlerProducer` uses `roundComplete` (`burler.go:130`), which requires **both**
`round-N-review.md` and `round-N-fixer-report.md`. The Bouncer uses
`ResolveRound(cfg.RunDir, cfg.ReportName)` (`bouncer.go:176` → `round.go:24`), which stats
**only** `round-N-review.md` — `ReportName` is pinned to `round-%d-review.md` at
`internal/shedrecipe/entries_bouncer.go:161`.

Traced, not merely read: the divergence is currently unreachable in production, because a
process killed in the phase-A-written/phase-B-pending window leaves `current_producer` naming
the Burler row, so the Burler re-resolves the same round and archives the orphan before
re-running. But the comment is the load-bearing justification for the pair predicate existing
at all, and a future reader who trusts it will conclude the Bouncer is protected when it is
not.

Fix: state what each side actually tests and why the asymmetry is safe today.

### F4 — `Batchifier` and the `Webster` row swallow `batcher.Active`'s error with no log line — LOW — CONFIRMED (by trace)

`internal/loomshed/batchifier.go:44-49` and `internal/loomshed/webster.go:61-67` both map
every `batcher.Active` failure onto `shedengine.Stuck` while **discarding `err` entirely** —
no `logger.Warn`, nothing.

Failure scenario: an operator (or an agent) puts an unknown batchifier name in
`batcher.yaml`'s `active:` key. `Batchifier` carries no `on_stuck`, so `Shed` persists
`state: blocked`, `error: "stuck with no OnStuck target"` and returns `RunBlocked`. The
status file names the producer but nothing anywhere — status file, driver log, envelope —
names the cause, and `Active` conflates unknown-name, malformed YAML, and I/O failure into
one bare error with no sentinel. The operator is told "Batchifier is blocked" and must guess.

This is the same disposition `planValidate` deliberately gets right one file over
(`internal/loomshed/planvalidate.go:95` logs the findings precisely because "this line is the
only record of it anywhere"). The two rows sit beside each other and disagree.

Fix: log the discarded error at `logger.Warn` on both rows, matching `planValidate`'s shape.

**Counted, not asserted.** I enumerated every `return shedengine.Stuck` across loom's producer
packages rather than reporting the two sites I happened to notice. Method:

```
grep -rn "shedengine.Stuck" --include=*.go \
  internal/loomshed internal/shedadapters internal/landingshed internal/preflightshed \
  | grep -v _test | grep return
```

16 return sites. Classified, every row including the ones judged correct:

| Site | Reason reaches an operator? |
|---|---|
| `loomshed/planvalidate.go:96` | yes — `logger.Warn` with the findings list |
| `loomshed/loompreflight.go:89` | yes — `logger.Warn` with the seed failures |
| `loomshed/discussionvalidate.go:64` | yes — `logger.Warn` with the findings list |
| `preflightshed/preflight.go:76` | yes — `logger.Warn` with the precondition failures |
| `landingshed/publish.go:201` | yes — `reportStuck` writes a stuck-reason file |
| `landingshed/finalize.go:186` | yes — `reportStuck` writes a stuck-reason file |
| `shedadapters/bouncer.go:208` | yes — `logger.Warn`, the re-bounce line |
| `shedadapters/bouncer.go:277` | yes — `degrade` logs every caller's message |
| `shedadapters/bouncer.go:363` | yes — the BLOCKING verdict file itself is the record |
| `shedadapters/bouncer.go:391` | n/a — the seed Stuck is a routine hand-off, and `runSeedSpawn` logs each of its own failures |
| `shedadapters/burler.go:355` | n/a — a successful round's routine hand-off, not a fault |
| `shedadapters/singlellm.go:172` | yes — `logger.Warn` with the asking message and session id |
| `shedadapters/webster.go:86` | yes — `logger.Warn` with the master's question |
| `shedadapters/webster.go:107` | yes — `logger.Warn` with `stuckReason` and `batchesDone` |
| **`loomshed/batchifier.go:48`** | **NO** |
| **`loomshed/webster.go:66`** | **NO** |

Exactly two sites discard their reason, and both discard the *same* error —
`batcher.Active`'s. Fourteen of sixteen get it right, which is what makes these two a
divergence rather than a missing convention.

What this enumeration cannot see: a Stuck reached through a helper that returns
`(Outcome, OutputPointer, error)` without the literal token on the `return` line
(`degrade`, `nonDoneExit`, `stuckOrCancelled` are the three in this tree, and all three are
listed above via their own definition site rather than their call sites).

### F5 — `burlerRoundFileSet`'s `entry` and `field` parameters are dead, and their absence makes two distinct config errors indistinguishable — NIT — CONFIRMED

`internal/shedrecipe/entries_burler.go:101`:

```go
func burlerRoundFileSet(entry, field string, cfg Config) (burlerengine.FileSet, error) {
```

Neither `entry` nor `field` is referenced in the body. Both call sites
(`entries_burler.go:196` and `:200`) pass `"BurlerRound"` plus `"target"` / `"fasit"`, which
reads as though the errors below are qualified by them. They are not: every error comes from
`configString`/`configStringSlice`/`configRejectUnknown`, which render only the leaf key.

Consequence: a bad `profile.target.paths` and a bad `profile.fasit.paths` both produce the
byte-identical message `shedrecipe: config key "paths" must be a string list, got ...`, and a
stray key under either produces `shedrecipe: unrecognized config key "X"` with no path. A
recipe author with a typo in one of two sibling maps is told which key, never which map. The
two unused parameters are the signature of an intent that was never wired up — `go vet` cannot
see an unused function parameter, so nothing catches it.

Fix: use the two parameters — wrap the returned error so it names the entry and the field.

### F6 — `lyx loom status --watch`'s own help text states the exact behaviour the code deliberately does NOT have — LOW — CONFIRMED

`internal/loomcli/status.go:86` (the `Long` text) and `internal/loomcli/status.go:147` (the
flag help) both say the tail prints **"one line per poll"**:

```
With --watch, it performs the same read once as a pre-flight, then prints
one line per poll to the terminal and never exits
...
cmd.Flags().BoolVar(&watch, "watch", false, "tail the status file one line per poll instead of ...")
```

The shipped behaviour is the opposite, and deliberately so. `printStatusLinesOnChange`
(`status.go:57`) prints only when the composed line differs from the last printed one, and its
own doc comment ninety lines above the flag says why in detail ("measured at 434 lines in
fifteen minutes, against a 2000-line default history limit"). `manifest/designs/loom.md` pins
it as an invariant: "**The strand prints on change, never once per poll.**"

So the one user-facing description of this verb states the exact regression the code, its
comments, and the design doc all exist to prevent. An operator reading `--help` and then seeing
a quiet pane has every reason to think the strand has hung.

Fix: correct both strings to describe print-on-change.

### F7 — `docs/overview.md`'s loom.yaml key list omits the two review keys — LOW — CONFIRMED

`docs/overview.md:319`:

> loom's config module (`loom.yaml`, holding the `discussion`/`plan` role model-specs,
> `discussion_timeout_min`/`plan_timeout_min`, and `discussion_interactive`) exists …

`loomengine.Config` (`internal/loomengine/config.go:163`) carries seven keys, not five:
`Review` and `ReviewTimeoutMin` are missing from the list above.
`internal/loomengine/template.yaml:6-7` ships them, `LoadConfig` validates the review
model-spec and the review timeout alongside the other four, and
`manifest/designs/loom.md`'s "The review model's home is `loom.yaml`, not the recipe" section
makes their location a deliberate, documented design decision:

> `loom.yaml`'s `review:` and `review_timeout_min:` keys are the review segments' model and
> timeout, validated at load time exactly like the existing `discussion:` and `plan:` keys.

So the module table's enumeration is the one place that still describes the pre-review-segment
config. An operator looking for where to tune the review model is told, by the top-level
module reference, that the key does not exist.

Fix: add both keys to the enumeration.

### F9 — `lyx loom --help` claims the shed engine "already drives Hardener", and its phase list omits every review segment — LOW — CONFIRMED

`internal/loomcli/cli.go:149-151`:

```
Long: `loom drives one task's phase machine (Preflight, Discussion, Plan,
Batchifier, Webster, Publish, Finalize) over a per-worktree status.json,
the same shed engine that already drives Hardener. ...
```

Two separate inaccuracies in one sentence, both in the module's own user-facing help:

1. **"already drives Hardener" is false.** `manifest/designs/hardener.md`'s first line is
   "**DRAFT — concept not yet settled**", with "**Status: Someday, deprioritized**" and "Do
   not implement from this doc yet". `docs/overview.md:325` gets the tense right — "the
   generic outer phase-FSM `loom` and **the eventual** `Hardener` are each built on" — and so
   does every package doc. The CLI help is the only place in the tree that asserts Hardener
   exists, and it asserts it to the operator.
2. **The phase enumeration omits the three review segments and `Plan-Revalidate`.** It reads
   as an exhaustive list (seven names, no ellipsis) but names 7 of the 17 rows, dropping
   `Discussion-Validate`, `Plan-Validate`, `Plan-Revalidate`, `Loom-Preflight` and — most
   consequentially — `Discussion-Review`, `Plan-Review`, and `Webster-Review`. The review gate
   is the module's defining mechanism (`loom.md`: "each guarded by a uniform **review gate**"),
   and an operator reading this help would not know a review segment sits between Discussion
   and Plan at all, let alone that it spawns its own LLM sessions and can block the run.

Fix: drop the false Hardener claim and give the enumeration the review segments, keeping it
honest about being a summary rather than the full seventeen rows.

### F8 — the Bouncer's clear-and-re-seed throws away a whole approved review generation without a single log line — LOW — CONFIRMED (by trace)

`internal/shedadapters/bouncer.go:187-198`:

```go
if n > 0 {
    if verdict, ok := b.judgedVerdict(n); ok && verdict == verdictApproved {
        if err := archiveRunDir(b.cfg.RunDir, b.cfg.Now); err != nil {
            return b.degrade(ctx, "...failed to clear an already-approved run directory", ...)
        }
        n = 0
    }
}
```

The FAILURE branch logs. The SUCCESS branch — the one that actually fires — logs nothing at
all. Every other consequential event in this file carries a `logger.Warn` (the re-bounce at
`:204`, the synthesized focus file at `:309`, the BLOCKING replay at `:361`, every `degrade`),
and `Live-Substrate Spawn Observability` names exactly this class: "a cleanup that skipped are
exactly the events an operator goes looking for after the fact".

Why it matters concretely: this clear is not cheap. It discards a settled APPROVED generation
and re-seeds from round 1, which costs one fresh judge spawn plus one fresh `Burler` round —
real LLM sessions, real minutes — and, because the `Burler` row's bounce episode never resets
(documented on `BurlerProducer`'s own doc comment), it can spend the leftover budget that
halts the run. `TestBouncer_Clear_AfterCommitFailureSubsequentCallClears`
(`bouncer_clear_test.go:384`) pins that a commit-seam failure followed by a resume takes this
exact path, so a transient git fault silently converts into a whole extra review generation.

**This is not a re-litigation of the clear-and-re-seed design** — that design is closed and
verified (`8cac77aa`), and the finding accepts it entirely. The finding is only that the
branch is silent: an operator whose run suddenly costs a second generation has nothing in the
driver log, the status file, or the run directory naming why.

Fix: log the clear at `logger.Warn` before archiving, naming the producer, the round whose
APPROVED verdict triggered it, and the archive destination.

## Docs & operability findings

Severity-tagged findings live in the Findings section above; this section records what I
checked and what came back clean, so a later round knows what has already been walked.

**Checked and accurate** — `manifest/designs/loom.md` against the shipped tree:
- The seventeen-vs-fifteen row-count divergence and its two separate causes: accurate.
- The bootstrap's numbered steps 0a–4 against `internal/loomcli/run.go`'s actual step order
  (parent record, seed, seed commit, reed up, strand, detached spawn + handshake, attach):
  accurate, including the "already-seeded case is tolerated via its own sentinel" claim
  (`loomshed.ErrSeedExists`) and the "a driver that already ran and exited proceeds to step 4
  rather than refusing" clause, which is exactly what I observed twice live.
- The crash-recovery ladder's three steps, for `SingleLLMProducer` — verified live (scenario
  2). **But the doc states the ladder as loom's own invariant without saying it holds for only
  one of loom's four LLM-spawning producers — that gap is F0, and the doc half of F0 is fixed
  with it.**
- "The strand prints on change, never once per poll": the implementation honours it; only the
  verb's `--help` contradicts it (F6).
- `Plan-Sweep`'s "stays a stub, deferred to its own Someday roadmap item": accurate — no
  `Plan-Sweep` row exists in the recipe and `Plan-Write`'s stencil names its absence as the
  normal degraded state.
- The Plan-Validate detail section's parity claim ("the verb reaches every mode the row set
  uses"): verified live against a real plan, both modes (scenario 3).
- `manifest/designs/webster-parallel-execution.md`'s "strictly sequential, one card at a time"
  — consistent with `batcher.yaml`'s default identity batchifier (one card, one batch) in the
  fixture; nothing in loom's own rows implies or attempts parallelism.

**Checked and inaccurate** — F6 (`status --watch` help), F7 (`docs/overview.md`'s loom.yaml
key list), F3 (`BurlerProducer`'s round-predicate claim), plus F0's own doc half.

**Operability observations that are not defects**, recorded so a later round does not re-file
them:
- `lyx loom run` from a non-TTY context fails only at its final `tmux attach-session` hand-off
  ("open terminal failed: not a terminal") after every fallible step has already succeeded and
  the driver is already detached and running. That is the documented step-7 exception behaving
  correctly, not a bug — but it does mean the verb's exit status is not a usable success
  signal in a headless context. `lyx loom status` is.
- The weft worktree is legitimately dirty for the whole run (` M _lyx/loom/status.json`),
  because Shed persists on every transition and only the CLI's own seed commit and the
  segments' commit seams ever commit. This is already documented in
  `internal/loomcli/smoke_test.go`'s own comments; noted here because it looks alarming live.
- An agent that builds the project leaves its binary in the warp worktree (the fixture
  accumulated an untracked `tinytool`). Nothing in loom's list cleans it up, and `Preflight`
  only runs at row 1. Whether that can block `Finalize`'s merge guard is recorded under the
  live-run account below rather than as a finding, since a real repo gitignores its build
  output and this fixture deliberately does not.

## What was tested

### Environment check (first, per the prompt's "check for a genuine environment gap FIRST")

```
which lyx claude tmux git go
```
- `lyx` → `/home/knatte/.local/bin/lyx` (deployed dev binary present)
- `claude` → `/home/knatte/.local/bin/claude`, version `2.1.231 (Claude Code)`
- `tmux` → `/usr/bin/tmux`, version `3.6`
- `go` → `go1.26.0 linux/amd64`
- `ps aux | grep -i tmux` at session start: **zero** tmux processes — clean baseline.

No environment gap. Real substrate driving is available.

### Hermetic gates (baseline, before any edit)

```
go build ./...                       -> rc=0, 2.7s
go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... \
       ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... \
       ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...
                                     -> rc=0, no diagnostics
go test -count=5 <same nine packages> ./cmd/lyx/...
                                     -> rc=0, 10 packages `ok`, zero FAIL, zero panic
```
Baseline is green.

### Deployed-binary provenance (the deploy-first footgun, checked before any live driving)

`deploy-dev` deploys to `<worktree>/.dev-bin/lyx` and prints
`WARNING: .../.dev-bin is not on PATH`.
The `lyx` that IS on PATH is `/home/knatte/.local/bin/lyx`, an OLDER production install.

Proof they differ, observed live: `lyx fabric clone` run from PATH produced a mutation
record with **no** `commit_created` for the weft prime (module configs written but not
committed), while the freshly-deployed `.dev-bin/lyx` produced
`{"kind":"commit_created","target":"tinytool2-weft",...}` — i.e. round 1's F3 fix
(`a426ba48`) is present in the dev build and absent from the PATH install.

**Consequence for this round:** every live command below uses the absolute
`.dev-bin/lyx` path. Using bare `lyx` would have validated a stale binary and drawn a
false PASS/FAIL, exactly as `crucible/README.md`'s deploy-first footgun warns.

### Live fixture built (real hub, real pair, real board task)

All commands run with `LYX=/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2/.dev-bin/lyx`.

```
# a real Go project as the warp remote, an empty weft remote
git -C /home/knatte/Code/loomyard/live-r2/src/tinytool init -q -b main .   (+ commit)
git clone --bare .../src/tinytool .../tinytool2.git
git init --bare -b main .../tinytool2-weft.git

$LYX fabric clone .../tinytool2-weft.git .../tinytool2.git --into .../hub2
  -> ok:true, hub .../hub2/tinytool2-HUB, weft configs written AND committed
$LYX fabric add greet-suffix            (run from the hub's warp prime)
  -> ok:true, pair created, launchers written, both branches pushed
  -> stencils auto-seeded at <hub>/_board/_lyx/stencils on first in-hub lyx invocation
$LYX board upsert '{"slug":"greet-suffix", ...}'
  -> ok:true  (Discussion-Write's stencil STOPs without a board task for the slug)
```

Task chosen deliberately minimal, per the prompt's "Minimal real task" note: make
`Greet` trim whitespace around its `name` argument, plus a unit test. One symbol,
one file, one new test file — so `Plan-Write` plausibly yields a single card and
Webster's real cost stays bounded.

**One fixture-only config change, recorded honestly:** `landing.yaml`'s
`require_pr_to_base` was set from `["main"]` to `[]` and committed weft-side. The
remote here is a local bare repo, not GitHub, so `Publish` would block on
"origin URL unusable" — a genuine environment gap (no GitHub remote), not a loom
defect. With the empty list `Publish` takes its documented step-2 early-`Done`
branch. This is the honest configuration for a non-GitHub remote and is the only
config deviation from the shipped template in the whole fixture.

**Operator attach commands for every live run below** (run from
`/home/knatte/Code/loomyard/live-r2/hub2/tinytool2-HUB/greet-suffix`):

```
/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2/.dev-bin/lyx reed status
/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2/.dev-bin/lyx reed attach
/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2/.dev-bin/lyx loom status
```

### Live scenario 1 — bootstrap and the Discussion phase (rows 1–6), OBSERVED

```
$ .dev-bin/lyx loom run          # 17:11:39
  seeded + committed the status file, brought reed up, added the loom-status strand,
  spawned the detached driver, then failed only at the tmux handover
  ("open terminal failed: not a terminal") because this session has no TTY — the
  documented step-7 interactive-handoff tail, and everything fallible had already run.
$ .dev-bin/lyx reed status
 {"session":"greet-suffix","socket":"lyx-tinytool2-HUB-22bfdfd7",
  "strands":[{"name":"loom-status","paneId":"%0","live":true},
             {"name":"discussion::9f518386","paneId":"%2","live":true}]}
```

Observed transitions from `status.json` (a poller printing only on change):

```
17:11:58 | Discussion-Write   running  history=2     (Preflight, Loom-Preflight both done)
17:13:29 | Discussion-Bouncer running  history=4     (Discussion-Validate done)
17:14:19 | Discussion-Burler  running  history=5     (Bouncer seed call -> Stuck, as designed)
17:26:38 | Discussion-Bouncer running  history=6     (Burler round 1 complete -> Stuck)
17:28:08 | Plan-Write         running  history=7     (Bouncer judged APPROVED -> Done)
```

Rows 1–6 all executed for real. Confirmed alongside:
- `_lyx/discussion/{decision-record.md,support-log.md}` written by a real autonomous
  Discussion-Write session.
- A real `Discussion-Burler` round wrote `round-1-review.md` then `round-1-fixer-report.md`
  under `.lyx/loom/reviews/discussion/`, review-before-fix, matching the Review Round
  Invariant's A-before-B discipline (the pane showed "Review saved. Now job B.").
- `commit_seam: discussion` fired on the APPROVED settle — the weft carries
  `ac5e2d2 loom: discussion artifacts for greet-suffix` on top of `988afaf loom: seed
  session bootstrap for greet-suffix`, i.e. the seed commit first and the segment's
  approved-settle commit after.
- The `Plan-Write` prompt visible in the live pane carried the CURRENT stencil text
  ("`Plan-Bouncer`'s approved settle writes it to `true`"), confirming the `lyx stencil sync`
  performed after F2 took effect for the rows that matter to this round.

### Live scenario 2 — crash mid-`Plan-Write`, the round-1 deferred item — PASS

This is the scenario round 1 recorded as "owed to a later round" because F7 made `Plan-Write`
unreachable. Driven here for the first time.

```
state before:  current_producer Plan-Write, running, history=7
               driver pid 2092990 = ".dev-bin/lyx loom drive"
               plan agent pid 2100584 in pane %7, strand plan::c27f2d85
               _lyx/plan does not exist yet (the agent had not written it)

$ kill -9 2092990            # 17:28:42 — hard driver crash, agent left alive
$ ps -p 2100584              # 2100584 Sl+   (agent survives, as designed)

$ lyx loom run               # 17:28:49 — the documented resume
$ lyx reed status
 strands: loom-status (%0), plan::c27f2d85 (%7, live)      <-- SAME strand, no second one
$ tmux -L ... list-panes -a
 %0 ... lyx
 %7 2100502 claude                                          <-- SAME pane, SAME pid
$ ps -eo pid,ppid,etime,args | grep 'loom drive'
 2102151  1851  00:23  .dev-bin/lyx loom drive               <-- new driver alive, waiting
```

Exactly one plan agent. `SingleLLMProducer.Call`'s `Attach` probe (`singlellm.go:118`) found
the live run and attached instead of respawning, and — the second half of the same
invariant — `prepareFreshSpawn`'s plan-directory rotation did NOT run, so nothing was moved
out from under the live agent.

**This is the positive control for F0.** The identical crash, one row earlier, produced ONE
agent; at the `Discussion-Burler` row it produced TWO. The difference is not the environment,
the tmux server, or the resume path — all three were identical. It is solely that
`SingleLLMProducer` has the probe and `BurlerProducer`/`Bouncer` do not.

### Live scenario 3 — F7's `approve_seam`/`require_approved` mechanism, both modes — PASS

`Plan-Write` completed at 17:29:48 and the machine advanced through `Plan-Validate` to
`Plan-Bouncer` (history 7 -> 9) — **the row-8 deadlock round 1 could never get past is gone,
observed live.** The plan it produced is a genuine single-card plan against the minimal task:

```
$ ls _lyx/plan/
00-overview.md
01-greet-trim.md
$ head -4 _lyx/plan/00-overview.md
---
format: 4
approved: false
root: cmd/tinytool
```

Both parity modes driven against that real, freshly-written, not-yet-approved plan — the exact
state F7 was about:

```
$ lyx loom validate-plan                      # Plan-Validate's mode (no require_approved)
{"ok":true,"plan_dir":".../_lyx/plan"}

$ lyx loom validate-plan --require-approved   # Plan-Revalidate's mode
{"error":"loom: plan is not yet valid",
 "findings":["plan-unapproved: plan frontmatter approved: is not true"],"ok":false}
```

So the pre-review gate tolerates `approved: false` and the post-review gate genuinely enforces
it, on the same bytes, through the same `planparser` functions the two rows call (Gate
Self-Check Parity Invariant). That is F7's fix demonstrated end to end rather than argued.

**And then the seam itself fired, for real.** The Plan segment settled APPROVED at 17:40:53
(the judge's verdict file carries `verdict: APPROVED` with three findings — one MEDIUM, one
LOW, one NIT — all fixed by the round's own fixer phase, matching the Review Round Invariant's
fix-every-severity rule). The machine went `Plan-Bouncer → Plan-Revalidate → Batchifier →
Webster`, history 11 → 14, with no bounce.

The load-bearing claim in `loom.md` — "**Row 9's approval flag is written on the approved
settle, before the commit** … so the flag lands inside the same commit that captures the
segment's approved plan — never as working-tree dirt applied after the fact" — verified from
git, not from the file's current contents:

```
$ head -3 _lyx/plan/00-overview.md
---
format: 4
approved: true
$ git -C <weft> log --oneline -2
3bd8cb1 loom: plan artifacts for greet-suffix
6f6222f loom: seed session bootstrap for greet-suffix
$ git -C <weft> show HEAD -- _lyx/plan/00-overview.md | grep -E '^[+-].*approved'
-approved: false
+approved: true
$ git -C <weft> status --porcelain
 M _lyx/loom/status.json
?? _lyx/webster/
```

The flip is a hunk *inside* `3bd8cb1`, and the working tree carries no residual plan change
afterwards. Approve-then-Commit ordering confirmed against the durable record.

That `Plan-Revalidate` then returned `done` is the independent confirmation that the flag the
seam wrote is the flag the enforcing gate reads: with `require_approved: true` it runs
`planparser.Validate`, whose `checkApproved` is exactly the check that failed two minutes
earlier on the same file.

**Overlay fix-scope confinement held too.** `Plan-Burler` runs `fix-scope: overlay` with
`_lyx/plan` as its only target path, and after its round the weft showed only
`_lyx/plan/00-overview.md` and `_lyx/plan/01-greet-trim.md` modified — nothing outside the
target paths, and no agent-side commit (the Fabric Git Invariant reserves that to the loop
owner, and the two plan commits above are both the loop owner's).

### Live scenario 4 — the emergency brake survives a broken `loom.yaml` — PASS

`manifest/designs/loom.md`'s "pause and status depend on the status file and nothing else"
section exists because "an agent loom itself spawned can rewrite `loom.yaml` mid-run". Nothing
had ever driven it. Driven here, against the RUNNING pipeline, mid `Plan-Burler` round:

```
# sabotage: loom.yaml's review model-spec made unparseable
-review: opus[effort=high]
+review: opus[effort=high            <- unclosed bracket

$ lyx loom status
{"activity":{"now":"Plan-Burler",...},"state":"running","pause_requested":true,"ok":true}   # WORKS
$ lyx loom pause
{"ok":true,"status_file":".../_lyx/loom/status.json"}                                        # WORKS
$ lyx loom drive
{"error":"loom config key \"review\": modelspec: bracket part in \"opus[effort=high\" must
  end with ']' and have nothing after it","ok":false}                                        # REFUSES
```

Both halves hold: the read-out and the brake survive a config fault that legitimately refuses
the verbs that actually build producers, and the refusal names the offending key and the exact
grammar violation. `loom.yaml` was restored immediately afterwards and the run continued.

**Sabotage discipline note:** this is also the proof the scenario reached the code. The same
three commands against a HEALTHY config all succeed, so a green `status`/`pause` here is only
meaningful because `drive` — which goes through the full `wire()` — failed on the identical
file at the identical moment.

### Live scenario 5 — graceful pause at a producer boundary — PASS

Requested mid `Plan-Burler` round (a real LLM round in flight):

```
$ lyx loom pause                       # 17:35:59
{"ok":true,...}
$ lyx loom status
{"current_producer":"Plan-Burler","state":"running","pause_requested":true,...}
```

Nothing was killed: the flag went up, the state stayed `running`, and the burler agent kept
working — exactly `loom.md`'s "The leaf agent finishes its unit; nothing is killed."

The boundary landed at 17:37:31, and every clause of the documented contract held:

```
17:37:17  Plan-Burler running  history=10  pause_requested=true   round still in flight
17:37:26  burler strand gone from `lyx reed status` — the round finished its own unit,
          wrote round-1-review.md AND round-1-fixer-report.md, and its pane was reaped
17:37:31  Plan-Bouncer PAUSED   history=11  pause_requested=FALSE
          $ ps -eo args | grep 'loom drive'   -> nothing; the driver exited cleanly
```

### Live scenario 6 — Webster driven from a real `Plan-Write` output — PASS (never proven before)

Round 1 named this as blocked and "owed to a later round": Webster's own smoke tests drive its
fork machinery against an isolated fixture plan, never a plan `Plan-Write` itself produced.

```
17:40:53 | Webster running history=14        (Plan-Bouncer done, Plan-Revalidate done, Batchifier done)
$ lyx reed status
 strands: loom-status (%0), master::41d7a4af (%11, live)     <-- a real Webster Master session

$ lyx webster status
{"batches":[{"number":1,"slug":"greet-trim","kind":"fork","status":"done","terminal":true,
             "has_digest":true}],
 "current_batch":0,"paused":false,
 "plan_fingerprint":"1aeb8dcf80d9663affc679eb4ba4eddf4ec57f8b831fe0bf858aebd3ae13512e", ...}

$ ls _lyx/webster/reports/
01-greet-trim.yaml    integration.yaml

17:42:18 | Webster-Bouncer running history=15   (Webster returned done)
```

The batching matched `manifest/designs/webster-parallel-execution.md`'s documented strictly
sequential shape: the default identity batchifier produced exactly one batch for the one card,
and Master drove it as a single `kind: fork` batch — no parallelism attempted, none implied.

The implementation is real and correct, committed to the **warp** repo by the fork itself
(commit-per-card, the Fabric Git Invariant's one sanctioned agent commit):

```
$ git log --oneline -2
4f1bf6a 1: greet-trim
3e259ef tinytool: initial commit
$ cat cmd/tinytool/greet.go
package main

import "strings"

// Greet returns the greeting line tinytool prints for name, with surrounding
// whitespace trimmed before it is composed into the line. A whitespace-only
// name trims to empty, yielding "Hello, !".
func Greet(name string) string {
	return "Hello, " + strings.TrimSpace(name) + "!"
}
$ ls cmd/tinytool/
greet.go   greet_test.go   main.go
```

The card's own bundled test file landed with it, and the integration report ran. The commit
subject `1: greet-trim` matches the plan's `**Commit:**` contract.

**Not staged: a crash mid-Webster BATCH.** The single batch went from spawn to `done` in about
fifty seconds against a six-line function, and by the time I read `lyx webster status` it was
already terminal — there was no mid-batch window left to kill the driver inside. I state that
as a miss rather than a pass: the mission named it and I did not drive it. What I did instead
is the sibling half the same mission bullet names — a crash mid **`Webster-Burler` round** (see
scenario 7). For the batch half, what I can report is read, not driven:
`websterengine.reclaimEntryTimeStrands` (`runlevel.go:260`, called at `:393` before anything
acts on the loaded state) stops a leftover live Master before a new one is started, which is a
different mechanism from `SingleLLMProducer`'s attach but has the same no-duplicate property.
A round with a larger fixture task (enough cards that a batch runs for minutes) could stage
this properly.

### Live scenario 7 — crash mid-`Webster-Burler` round — F0 REPRODUCED AGAIN, on the source-scope row

The second half of the mission's "crash/resume ladder for the rows past Plan" bullet, and the
severe variant of F0: `Webster-Burler` is the one row in the whole recipe running
`fix-scope: source`, so its agent has warp write access and commit-per-fix authority.

```
17:43:49 | Webster-Burler running history=16
$ lyx reed status      -> burler:1:b4791233 (%13, live)
$ ps ... 'loom drive'  -> 2110778  06:33  .dev-bin/lyx loom drive
$ git log --oneline -1 -> 4f1bf6a 1: greet-trim

$ kill -9 2110778                       # 17:44:3x
$ lyx loom run
$ lyx reed status
 strands: loom-status (%0),
          burler:1:b4791233 (%13, live)      <-- ORPHAN, still alive
          burler:1:a0ff9cb9 (%14, live)      <-- RESPAWN
$ tmux -L ... list-panes -a
 %13 2116430 claude
 %14 2117592 claude
```

Both target the same artifacts, from two different burler round directories:

```
$ grep -l 'reviews/webster/round-1-fixer-report.md' <wt>/.lyx/burler/round-*/instruction-3-fix.md
 .lyx/burler/round-1197877629/instruction-3-fix.md
 .lyx/burler/round-861290293/instruction-3-fix.md
```

And both carry commit authority over the warp repo:

```
$ grep -n commit .lyx/burler/round-861290293/instruction-3-fix.md
14:You commit each fix individually, once green, before starting the next finding.
   Commit message format: `<module-or-target>: fix <finding-id> — <one-line what/why>`. Never push.
```

So the crash produced **two concurrent agents, each instructed to commit per fix to the same
git repository, each judging the same diff, each writing the same review and fixer-report
files.** Interleaved commits from two independent reviewers of the same diff is a materially
worse outcome than the overlay case in F0's first reproduction: the overlay case corrupts an
`_lyx` artifact a later row re-reads, this one corrupts the branch that `Publish` and
`Finalize` are about to land.

The orphan was removed by hand (`lyx reed remove b4791233…`) within about twenty seconds so the
pipeline could continue — which is itself the finding's operational shape: **the run cannot
recover from its own documented resume path without a human deleting a pane.**

Two independent reproductions, on two different rows, with two different `fix-scope` values,
against the identical crash-and-resume sequence that produced exactly one agent at
`Plan-Write`. F0 is not an anecdote.

### Live scenario 5 details

Four separate documented properties, all confirmed in one observation:
1. The pause is honoured at a **producer boundary**, never mid-operation — the in-flight
   `Burler` round completed and its artifacts landed.
2. The routing persist still happened first (`Plan-Burler → stuck`, history 10 → 11,
   `current_producer` advanced to `Plan-Bouncer`), so the resume point is the NEXT row, not a
   re-run of the finished one.
3. `pause_requested` was **cleared in the same persist** that recorded `paused` — `run.go:113`'s
   "the durable record of 'this run is paused' is state, not the flag" — so the resume below
   does not re-pause on the flag it is resuming from.
4. The driver process exited rather than idling.

