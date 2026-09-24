# batten — review round opus-medium-r4

Round 4 of the batten follow-up crucible campaign.
Worktree `crucible-batten-followup`, seeded at `b1e5ab9af`.
Clean-room: no `_mill/**/batten-review-*` file other than the prompt was opened before this report's findings were complete.

## Executive summary

(pending)

## Scope assessment

(pending)

## Code findings

(provisional, severity ordering pending)

### F1 (provisional) — Worktree-Create reports done over a pair whose provenance record was never written; the child's bootstrap then refuses forever

- Where: `internal/fabricengine/fabric.go` `PairComplete` (checks sibling + junction health only); consumed by `internal/battencli/wire.go` `taskWorktreeComplete` and `childSeedParams`.
- Scenario (CONFIRMED live, windows A6/A7/A8): SIGKILL of `lyx batten step <slug>` anywhere after `seedLyxJunction` and before `WriteOrigin` in `Topology.Add`.
  Resume: create row reports done (PairComplete passes), Seed-Child writes `{recipe: loom, driver: go}` with NO `params.parent` (no origin record), commits and pushes it.
  Run-Shed's spawn then fails on every resume: `loom: no recorded parent branch for this worktree pair; pass --parent once to record it`.
  Following that text from inside the child (`lyx loom start --parent main --no-attach`) is refused: `shedrun: disagreeing seed ... {Recipe:loom Driver:go Params:map[]}; refusing to overwrite with {... Params:map[parent:main]}`.
  The run cannot reach done without hand-editing the child's committed seed.

## Focus-1 enumeration (SIGKILL windows in Topology.Add / Topology.Remove)

(pending)

## Focus-2 enumeration (youngest fixes)

(pending)

## Docs & operability findings

(pending)

## What was tested

### Fixture hub A (`$S/r4fx-a7q`, `$S` = this session's scratchpad under /tmp)

- `mkfx.sh`: bare warp (go.mod + main.go + .gitignore, `-b main`), empty bare weft (`-b main`), `lyx fabric clone --into <hubs> weft.git warp.git`.
  First attempt without `-b main` on the bare warp: clone refused with its own "remote HEAD names a ref that does not exist" remedy — environment setup, not a defect.
- `fxover.sh`: committed `selfreport: false`, `friction: ""` (loom.yaml) and `require_pr_to_base: []` (landing.yaml) on prime's weft BEFORE any Board task or create; verified all three with grep against `git show HEAD:`.
- Board tasks `ka1`..`ka12` (`type: loom`), `floor` (type empty) via `lyx board upsert`.

### Instrumented kill build (focus 1)

- Throwaway kill points added to `add.go` (A1–A5, A8–A12), `junction.go` (A6, A7), `remove.go` (R1–R4, R8), `weftwiring.go` (R5–R7): each `r4kp("Xn")` self-SIGKILLs when `LYX_R4_KP=Xn`.
  Built to `$S/bin/lyx-kp`, then `git checkout -- internal/fabricengine/ && rm r4kp_tmp.go`; `git diff --quiet` clean.
  `.dev-bin/lyx` (deployed from `b1e5ab9af` before instrumentation) contains 0 occurrences of `LYX_R4_KP`; the kill binary 1.
  Only the killed step runs the kill binary; every resume runs `.dev-bin/lyx`.

### Add windows (driven from prime, `LYX_R4_KP=An lyx-kp batten step ka<n>` then `lyx batten step ka<n>`)

- A1 (after warp `worktree add -b`): exit 137; resume blocked at Worktree-Create with the incomplete-pair remedy naming the task worktree and `git branch -D ka1`.
  Remedy followed verbatim from prime: both commands succeed; resume creates the pair (done), Seed-Child done.
- A2 (after hook install): identical to A1.
- A3 (after sibling created): remedy names `git worktree remove --force <hub>/ka3` AND `git worktree remove --force <hub>/ka3-weft`.
  From prime the second command fails: `fatal: '<hub>/ka3-weft' is not a working tree` (rc 128) — it is a worktree of the sibling repository, not prime's.
  Resume after the rest of the remedy: `weft worktree directory already exists: <hub>/ka3-weft` (fabric's own vocabulary, no remedy).
  `git -C <hub>/ka3-weft worktree remove --force <hub>/ka3-weft` works; resume then creates the pair (adopting the leftover `ka3-weft` branch) and Seed-Child is done.
- A4 (after portal) / A5 (after launchers): with the corrected sibling command, resume is refused `link already exists — remove it first: <hub>/_portals/ka4` — the remedy never names the portal or launchers.
  That refused Add rolls back (removing the stale portal) but keeps the warp branch (known `fabric-rollback-keeps-warp-branch` item); the next resume gets createRefusal's leftover-branch text.
  Following that verbatim (`git branch -D ka4`; `git push origin --delete ka4` fails harmlessly: remote ref does not exist; `lyx fabric cleanup --apply --remote` deleted `ka4-weft` and `ka5-weft`) then resume: create done, Seed-Child done, clean both halves.
- A6 (after junction links, before sibling artifact excludes) / A7 (before `.git/info/exclude` seeding): create reports done; both halves `git status --porcelain` clean (info/exclude is shared by every worktree of a repository, so prime's clone-time entries already cover the junctions).
  Seed-Child writes a seed with no `params.parent` — see F1.
- A8 (after wiring, before `WriteOrigin`): same as A6/A7. Run-Shed: `loom: no recorded parent branch ... pass --parent once`; `lyx loom start --parent main --no-attach` from the child refused as a disagreeing seed — F1 CONFIRMED.
- A9 (origin written, not committed): create done; sibling shows `?? _lyx/fabric/`; Seed-Child writes `params.parent: main` (ReadOrigin reads the uncommitted record).
  loom's bootstrap commits the origin record unconditionally (`bootstrapCommitPaths`), so this state self-heals at Run-Shed (traced, `sharedbootstrap.go` step 3).
- A10 (committed, neither branch pushed) / A11 (warp pushed, sibling not): create done; Seed-Child's `PushSeed` pushed the sibling branch (weft remote shows it after Seed-Child).
- A12 (both pushed, before return): create done — a healthy re-entered create reports done.

### Hermetic baseline (start of Job 1)

- `go build ./... && go vet ./... && go test -count=1 ./...` at `b1e5ab9af`: exit 0, every package `ok`.
- Third self-report path sweep: `ls -d internal/*selfreport* internal/*friction*` plus a repo-wide grep for `CreateIssue`/issue-filing call sites.
  Only Tier 1 (`internal/loomcli/selfreport.go`, wired at `internal/loomcli/arm.go:326`) and Tier 2 (`internal/frictionengine` via `lyx selfreport create` in `internal/selfreportcli`).
  No third path.
- Environment: `which claude tmux gh` all resolve.
  `--child-driver llm`: the `ly@loomyard` plugin is absent on this host, an operator-accepted environment gap (focus 4); not driven this round.
- Windows path behaviour: not touched, unreachable from this Linux host.

## Teardown

(pending)
