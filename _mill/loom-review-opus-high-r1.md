# `loom` — crucible round 2 review (tag `opus-high-r1`)

> Independent, clean-room review + fix round against `loom`, per `_mill/loom-review-prompt.md`.
> Worktree `/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2`, branch `loom-crucible-hardening-round2`.
> This file is written incrementally during Job 1 and committed after each meaningful append.

## Executive summary

_(written last)_

## Did a full live pipeline run complete this round?

_(answered when the live driving concludes — see "What was tested")_

## Scope assessment

_(pending)_

## Findings

_(recorded provisionally as they are spotted; severity ordering finalized last)_

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

Suggested fix: make `MissingKeys` require the sequence key itself but not its individual
elements — a template list is a default, not a minimum length. See the fixer report for the
exact shape landed.

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

## Docs & operability findings

_(pending)_

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

