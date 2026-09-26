# happy-path review — opus-medium-r1

Status: IN PROGRESS (Job 1, review).

## Executive summary

(pending)

## Operator sequence used

(pending)

## Findings

(pending — provisional findings appended as they are spotted)

## Out-of-scope observations

(none yet)

## What was tested

Baseline: worktree HEAD at first deploy `f28403aaf happy-path: crucible seed r1 — round prompt`; `./deploy-dev` → `.dev-bin/lyx @ f28403aaf`.
`L=/home/knatte/Code/loomyard/wts/crucible-happy-path/.dev-bin/lyx` below.

### Fixture hub 1 (`$HOME/crucible-happy-path/opus-medium-r1/hub1`)

- Seed repo: `go.mod` (`example.com/calc`), `calc/calc.go` (`Add`), `calc/calc_test.go`, `README.md`, one commit on `main`, pushed to `warp.git` (HEAD re-pointed to `main`); `weft.git` empty bare (`git init --bare`, host default branch `master`).
- `$L fabric clone --into hub1 hub1/weft.git hub1/warp.git` → ok, hub `hub1/warp-LYXHUB` (prime `warp`, weft `warp-weft`, `_board`).
  Observed: the weft side took the empty weft bare's own unborn default branch — weft prime on `master-weft`, `_board` on `master` — while the warp is on `main`.
- `(cd warp && $L config loom --set selfreport=false --set 'friction=')` → ok, committed and pushed on `master-weft`.
- `$L config landing --set 'require_pr_to_base=[]'` → refused: `unknown config key(s): require_pr_to_base (known: ..., require_pr_to_base[0], ...)`.
  Worked around with `VISUAL=<awk script> $L config landing` writing `require_pr_to_base: []` → ok, committed and pushed.
- `(cd warp && $L board upsert '{"slug":"add-sub","title":"Add Sub to calc","brief":...,"body":...}')` → ok; the detached board sync committed `board sync` on `_board` a few seconds later.
- `(cd warp && $L fabric add add-sub)` → **FAILED**: `create weft worktree ... git worktree add -b add-sub-weft ... main-weft: exit 128: fatal: invalid reference: main-weft`, `partial: true`, plus `WARN fabricengine: rollbackAdd's warp-branch deletion was refused by the destructive gate; the branch is left behind ... check=ownership`.
  Provisional finding F1 (weft primary branch derived from the weft bare's own unborn HEAD, not the warp's branch) and F2 (rollback strands the warp branch).
- Retry `$L fabric add add-sub` → refused `branch "add-sub" already exists; ... delete it first with "git branch -D add-sub"` — the stranded branch blocks the retry.
- Operator workaround for F1: abandon hub1, build hub2 with the weft bare's HEAD pre-pointed at `main` (`git -C weft.git symbolic-ref HEAD refs/heads/main`) before `fabric clone`.

### Fixture hub 2 — go-driver run (`$HOME/crucible-happy-path/opus-medium-r1/hub2`)

- Same seed; `weft.git` HEAD pre-pointed at `main`; `fabric clone` → weft prime `main-weft`, `_board` on `main`; loom override (`selfreport: false`, `friction: ""`) and landing override (`require_pr_to_base: []`) committed and pushed on `main-weft` before any task.
- `(cd warp && $L board upsert '{"slug":"add-sub",...}')` → ok.
- `(cd warp && $L fabric add add-sub)` → ok; pair `add-sub`/`add-sub-weft`, `origin.json` `parent_branch: main`, launcher `_launchers/add-sub/run.sh` = `lyx loom start`.
  Verified `add-sub-weft/_lyx/config/loom.yaml` carries `selfreport: false`, `friction: ""`, and `landing.yaml` carries `require_pr_to_base: []`.
- `(cd add-sub && $L shed seed self --recipe loom --driver go --param parent=main)` → `{"driver":"go","ok":true,"recipe":"loom","run_id":"self"}`.
- `(cd add-sub && $L loom start --no-attach)` → rc 0, no output; weft commits `loom: seed session bootstrap for add-sub`, `Loom-Preflight -> running`, `Discussion-Write -> running`.
