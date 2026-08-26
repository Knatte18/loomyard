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

