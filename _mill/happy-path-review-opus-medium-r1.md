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
- Polled `$L loom status` every 20 s: Discussion-Write 15:13 → Discussion-Bouncer/Burler → Plan-Write 15:16 → Plan-Bouncer/Burler → Webster 15:18 → Webster-Bouncer/Burler → `state: done` at Friction-Reflect 15:21 (history_length 18). Driver log tail: `{"friction":"skipped","halted_producer":"Friction-Reflect","outcome":"done"}`; only WARNs are informational plan-gate findings and one dropped cluster-exclude focus directive.
- Landing check: prime `warp` `main` has `d9f5d85 Add Sub to calc` (squash, `calc.Sub` + `TestSub`), `git status -sb` = `main...origin/main [ahead 1]`.
  **Bare `warp.git` `main` is still `0258fde seed calc`** — Finalize merged into the local parent pair only and pushed nothing. Provisional finding F3.
- `lyx fabric status` clean, `lyx fabric pairs` both pairs `in_sync`/`junction_healthy`; board task `add-sub` has no status (not marked done by anything in the loom path).
- Left running after `done`, by design: the worktree's reed tmux session (`tmux -L lyx-warp-LYXHUB-ccc46d49 ... -s add-sub`) and the per-hub `lyx reed watchdog`.

### Hub 2 — llm-driver run, second task `add-mul` (also the "hub usable for the next run" check)

- ly-drive resolvability: `~/.claude/plugins/installed_plugins.json` has no `ly@loomyard` (the marketplace lists `ly`, but only `prowler@loomyard` is installed), and `~/.claude/skills` carries no `ly-drive`.
  Nothing in `lyx` installs, checks or points at the skill.
- `(cd warp && $L board upsert '{"slug":"add-mul",...}')`, `$L fabric add add-mul` → ok (forked from prime's local `main`, which carries the unpushed Sub landing).
- `(cd add-mul && $L shed seed self --recipe loom --driver llm --param parent=main)` → ok; `(cd add-mul && $L loom start --no-attach)` → rc 0, strands `loom-status` + `loom-driver`.
- Driver session transcript (`~/.claude/projects/-home-knatte-crucible-happy-path-opus-medium-r1-hub2-warp-LYXHUB-add-mul/*.jsonl`): the session did not have the skill; its first tool call was `find / -type d -name 'ly-drive*'`, which happened to find this repo's own source trees on this dev machine, and it read `/home/knatte/Code/loomyard/wts/crucible-happy-path/plugins/ly/skills/ly-drive/SKILL.md` from disk.
  Provisional finding F4 (CONFIRMED).
- Step 1: the session wrote the step envelope to `add-mul/.scratch/ly-drive/self/step-1.json` — the skill's own prescribed location "under the driving session's own cwd", which for a loom-launched driver IS the drive directory.
  Envelope: `Preflight` `outcome: stuck`, `state: blocked`, trace `preflightshed: preconditions not met ... failures="worktree-clean: uncommitted code changes: ?? .scratch/"`.
  The driver handed back per the skill (`blocked` is never repaired) after 1 step.
  Provisional finding F5 (CONFIRMED, BLOCKING for the llm driver).
