# happy-path review — opus-medium-r1

Status: Job 1 (review) COMPLETE — written before any production or test file was touched.

## Executive summary

- **go driver: landed — locally only.** On a hub whose weft bare's HEAD was pre-pointed at `main` (workaround for F1), task `add-sub` ran Preflight → Discussion → Plan → Webster → Publish (no-op) → Finalize → Friction-Reflect to `done` in ~8 minutes with no intervention.
  The squash commit `Add Sub to calc` landed on the prime worktree's local `main`, but **not on the bare warp's `main`**: Finalize never pushes the parent (F3).
- **llm driver: landed — locally only, after one operator repair.** Task `add-mul` on the same hub: the `ly-drive` skill is not installed for Claude sessions and nothing in `lyx` provides it (F4); the session found the skill by `find /` over the dev machine.
  Its first step then dirtied the task worktree with the skill's own `.scratch/` step-output directory, so Preflight's clean-tree gate blocked the run (F5).
  After an operator repair (exclude `.scratch/`, remove the driver strand, re-run `lyx loom start --no-attach`) the second driver session drove the run to `done`; `Add Mul to calc` landed on local `main` only (F3 again).
- Hub reuse: after both runs, `lyx fabric status` is clean, all three pairs `in_sync`/`junction_healthy`, and the second task started and landed on the hub the first one used.
- Top blockers: **F1** (a fabric hub whose weft bare's unborn HEAD differs from the warp's branch cannot create any task pair), **F5** (the llm driver blocks every run at Preflight), **F3** (nothing reaches the remote), **F4** (the llm driver's skill is not resolvable).

## Operator sequence used

All commands use the dev binary (`L=.../.dev-bin/lyx`); `<hub>` is `<scratch>/warp-LYXHUB`.

1. `git -C weft.git symbolic-ref HEAD refs/heads/main` on a fresh empty weft bare (only needed until F1 is fixed; a GitHub-created empty repo already has `main`).
2. `$L fabric clone --into <scratch> <weft.git> <warp.git>`.
3. From `<hub>/warp`: `$L config loom --set selfreport=false --set 'friction='` (and, for a no-PR landing, `require_pr_to_base: []` in `landing.yaml` via `VISUAL=<script> $L config landing`, since `--set` cannot empty a list — F6).
4. From `<hub>/warp`: `$L board upsert '{"slug":"<slug>","title":...,"brief":...,"body":...}'`.
5. From `<hub>/warp`: `$L fabric add <slug>` (records `parent_branch: main` in the pair's `origin.json`).
6. From `<hub>/<slug>`: `$L shed seed self --recipe loom --driver go|llm --param parent=main` (optional for `go`, which `loom start` seeds by default; the `--param parent=<parent>` must match what `loom start` writes or the seed write refuses).
7. From `<hub>/<slug>`: `$L loom start --no-attach`, then `$L loom status` until `state: done`.
8. After `done`: `git -C <hub>/warp push origin main` by hand (F3).

## Findings

Severity: BLOCKING = run cannot land; MEDIUM = lands only after intervention, or corrupts; LOW/NIT = friction.

### F1 — BLOCKING — clone names the weft primary branch after the weft bare's unborn HEAD, not the warp's branch; every later `fabric add` fails (CONFIRMED)

- Where: `internal/fabricengine/clone.go:437` `suffixWeftPrimaryBranch` reads `git branch --show-current` in the freshly cloned **weft** and suffixes that; `clone.go:285-295` then also puts `_board` on it.
  `fabric add` (`internal/fabricengine/add.go`) forks the new weft branch from `WeftBranchName(<warp branch>)`.
- Run: hub1, weft bare created with `git init --bare` on a host whose `init.defaultBranch` is `master`, warp on `main`.
  Clone produced `master-weft` + `_board` on `master`; `lyx fabric add add-sub` → `git worktree add -b add-sub-weft ... main-weft: fatal: invalid reference: main-weft`.
  No task pair can ever be created on that hub; the only workaround is re-cloning with the weft bare's HEAD re-pointed.
- Fix: derive the pairing branch from the warp prime's checked-out branch (read at the cloned warp worktree), and pass it into `suffixWeftPrimaryBranch`, so the weft primary is always `WeftBranchName(<warp branch>)` and `_board` sits on the warp's branch, whatever the weft remote's HEAD says.

### F3 — MEDIUM — Finalize merges into the local parent pair and never pushes it; nothing lands on the remote (CONFIRMED, both drivers)

- Where: `internal/landingshed/finalize.go:154-160` (`parentHandle.Merge` then `Done`); `fabricengine.(*Fabric).Merge` (`internal/fabricengine/merge.go:327`) is documented local-only.
- Run: after both runs reached `done`, `git -C <hub>/warp status -sb` = `main...origin/main [ahead 2]` while `git -C warp.git log main` = `seed calc` only.
  The run reports `done`, the board has no signal, and the landed work exists only in the prime worktree of one machine.
- Fix: after a successful parent-side merge, push the parent's warp branch to its upstream (skip when it has none); a push failure after a completed merge must not re-run the merge on resume.

### F4 — MEDIUM — the `ly-drive` skill is not resolvable by the driver session, and `lyx` neither provides nor checks it (CONFIRMED)

- Where: `internal/loomcli/driverprompt.go:22` (the launch prompt only says "Run the ly-drive skill"); the skill ships in `plugins/ly`, which this machine's Claude has not installed (`installed_plugins.json` has no `ly@loomyard`).
  No `--help` text names the prerequisite.
- Run: both driver sessions announced the skill was missing and located it with `find / -iname '*ly-drive*'`; the first read the copy in this crucible worktree, i.e. whichever source tree happened to exist on the machine.
  On a machine without a loomyard checkout the driver has no instructions at all.
- Fix: make the prerequisite discoverable (the llm-driver `--help` texts name `/plugin install ly@loomyard`), and make the launch prompt point the session at an absolute copy of the skill `lyx` itself materializes, so resolution no longer depends on the plugin cache.

### F5 — BLOCKING (llm driver) — ly-drive writes its step envelopes under the drive directory, which dirties the task worktree and blocks Preflight (CONFIRMED)

- Where: `plugins/ly/skills/ly-drive/SKILL.md` "How to invoke a step": step output goes to `.scratch/ly-drive/<run-id>/step-<n>.json` "under the driving session's own cwd"; `lyx loom start` launches the driver with its cwd = the task worktree = the drive directory, so the clause "so the redirect never lands under the drive directory" is false by construction.
- Run: step 1 envelope `Preflight` `outcome: stuck`, `state: blocked`, trace `failures="worktree-clean: uncommitted code changes: ?? .scratch/"`; the driver handed back (a `blocked` state is never repaired).
  Operator repair used: `echo .scratch/ >> <hub>/warp/.git/info/exclude`, `lyx reed remove <driver guid>`, `lyx loom start --no-attach`; the fresh driver used its own temp dir and drove to `done`.
  Any later clean-tree gate (Finalize's merge guard) would hit the same dirt.
- Fix: the skill writes step output to a private temp directory outside the drive directory (e.g. created once with `mktemp -d`).

### F6 — LOW — `lyx config <module> --set` cannot set a list-valued key, so the no-PR landing needs an editor (CONFIRMED)

- Where: `internal/configengine/set.go:55` via `yamlengine.SetValues`, which only knows the flattened `require_pr_to_base[0]` leaf.
- Run: `lyx config landing --set 'require_pr_to_base=[]'` → `unknown config key(s): require_pr_to_base`.
  A hub that lands straight to `main` (this repo's own policy) must set this; the only way was `VISUAL=<script> lyx config landing`.
- Fix: accept a flow-sequence value (`[]`, `[a, b]`) for a key whose template value is a sequence.

### F7 — LOW — a blocked Preflight's status/envelope reason is the generic `stuck with no OnStuck target` (CONFIRMED) — NOT-FIXED-THIS-ROUND

- Where: `internal/shedengine/run.go:236`; `internal/preflightshed/preflight.go:70-76` puts the real cause only in a WARN log line.
- Run: `lyx loom status` after F5 showed `error: "stuck with no OnStuck target"`; the cause was only in the step's trace file.
- Fix: carry a producer-supplied reason on a Stuck outcome into `Status.Reason`; that changes the `ShedProducer` seam every producer implements, so it needs its own design step.

### F8 — NIT — ly-drive gives no concrete way to wait for a backgrounded step (CONFIRMED)

- Where: `plugins/ly/skills/ly-drive/SKILL.md` "How to invoke a step": "Wait for the process to exit".
- Run: the second driver waited with `while pgrep -f 'lyx shed step' ...`, which matches its own waiting shell's command line, so after the run reached `done` it sat in a 10-minute timeout before reporting.
- Fix: name the mechanism — wait on the background job's own completion notice or PID, never a command-line pattern match.

## Out-of-scope observations

- `fabric add` rollback leaves the new warp branch behind (hub1): `rollbackAdd`'s branch delete is refused by the ownership gate under an empty `branch_prefix` (documented in `internal/fabricengine/add.go:346-349`), and the retry is then refused `branch "add-sub" already exists`.
  Impact: after any `fabric add` failure the operator must `git branch -D <slug>` before retrying; only reachable after another failure (F1 here).
- Nothing marks the board task done after landing: `lyx board get add-sub` still shows no `status` after `state: done`.
  Impact: the board keeps listing landed work as open; an operator could claim it again.
- The llm driver's second session batched many `lyx shed step` calls in one shell loop without reading each step's trace in between, contrary to the skill's loop.
  Impact: a repair-worthy failure mid-batch is seen late; LLM behaviour, not a `lyx` defect.

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
- Operator repair for F5: `echo .scratch/ >> <hub>/warp/.git/info/exclude`; `$L reed remove bfef724d32568a345c16d5f0ae3e4485` (the idle driver strand); `$L loom start --no-attach` → new `loom-driver` strand.
  The second session also found the skill via `find /`, wrote step files under its own temp dir, and stepped Preflight → … → Webster-Bouncer → `done` at 15:36 (history_length 19).
- Landing check: prime `main` = `51b4088 Add Mul to calc` on top of `d9f5d85 Add Sub to calc`, `[ahead 2]` of `origin/main`; bare `warp.git` `main` still `seed calc` (F3).
- Hub after both runs: `$L fabric status` → no changes; `$L fabric pairs` → `main`, `add-sub`, `add-mul` all `in_sync`/`junction_healthy`; no dirty worktree on either side.
- The second driver session then idled for ~10 minutes in `while pgrep -f 'lyx shed step'` (self-matching) before reporting (F8).
- Provisional F2 (rollback strands the warp branch) was reclassified as an out-of-scope observation: it is only reachable after another `fabric add` failure and is a documented gate decision.
