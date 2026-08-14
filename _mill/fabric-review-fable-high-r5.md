# fabric — independent review, round 5 (`fable-high-r5`)

Reviewer-fixer round agent, Fable/high.
Clean-room review per `_mill/fabric-review-prompt.md`: no prior `fabric-review-*` material read before this findings list was complete.
Primary target: the create-side containment gap in `createExclusiveDir`/`createGitWorktree` (seeded, orchestrator-confirmed residual).

## Executive summary

Round 5's primary target — a live, reproducible symlink-directed-write escape in the CREATE-side
executor `createGitWorktree` (reached via `Topology.Add`) — is **CONFIRMED and reproduced** against
the real substrate: a full git worktree gets written OUTSIDE the hub through a symlink toggled at the
Add target during the wide check-then-act window.
Root-caused, fixed with a staging-then-rooted-rename-then-repair approach that genuinely closes the
window (git's worktree WRITE only ever targets an unpredictable fabric-chosen staging name inside the
container; the only adversary-controllable path is touched solely by `os.Root.Rename`, which refuses
symlink-following), and re-attacked.
Two sibling gaps of the same class were found and fixed: `createExclusiveDir`'s intermediate-symlink
escape (closed via `os.Root`), and the non-gated weft/board/reconcile worktree-add sites (closed via
the same shared helper).
One NIT: round 4's F2 WARN-log regression test does not sabotage-prove the log line.

Merge-readiness: MERGEABLE after fixes (see fixer report). Windows path behavior remains a permanent
never-executed gap (Linux host); N4's dirtiness-probe TOCTOU stays an accepted documented residual.

## Scope assessment (plan vs shipped)

Scope is right and matches `doc.go`'s intent. The create-side of the destruction chokepoint's
containment property was the one place the "resolved-and-bound-to-the-act" guarantee the delete side
already has (`removeContainedPath`) had NOT been extended to. This round closes that asymmetry for the
create side. No over-reach, no silently-dropped v1 requirement observed beyond this.

## Code findings (severity-ranked)

### F1 — createGitWorktree symlink-directed-write escape via Topology.Add (MEDIUM, CONFIRMED)

- `internal/fabricengine/destroy.go:900` (`createGitWorktree`), reached from
  `internal/fabricengine/add.go:135`.
- Scenario: an external adversary toggles a symlink on/off at the Add target path
  (`WorktreePath(l, slug)` = `<hub>/<slug>`) during the check-then-act window between Add's
  `os.Stat(target)` guard (add.go:83) and the eventual `git worktree add -b <branch> <target>`
  (add.go:135) — a WIDE window spanning `git remote`, the weft-target stat, `weftBranchExists`, and
  `rev-parse HEAD`. When the "on" state (symlink → an outside directory) coincides with the git call,
  `git worktree add` follows the symlink and writes a full real worktree (`.git`, checked-out tree)
  OUTSIDE the hub, registered at the resolved outside path; exit 0. Fabric's subsequent steps then
  operate on the nominal in-hub path, fail, and roll back — but the rollback is keyed on the nominal
  target, so the escaped worktree is orphaned outside the hub.
- Blast radius: content written to an empty/creatable outside location; `git worktree add` refuses a
  NON-empty outside target (`fatal: '<path>' already exists`), so nothing pre-existing outside is
  overwritten and nothing INSIDE the hub is destroyed — a notch below M1's delete-side severity, but
  the same escape CLASS (nominal path and actual write location diverge under adversarial timing).
- CONFIRMED. Reproduced live: real hub built from local bare warp+weft remotes, `./deploy-dev`
  binary, a bash toggler flipping a symlink at `<hub>/racewt` on/off in a tight loop while
  `lyx fabric add racewt` ran repeatedly — escape hit on attempt 12 (`.git` written into the outside
  dir). See "What was tested".
- Fix: route the worktree creation through a staging path (a random name created via `os.Root` rooted
  at the container), run `git worktree add` into that staging path, then `os.Root.Rename` staging →
  target (refuses to follow a planted symlink at target), then `git worktree repair <target>` to fix
  git's registration. git's WRITE never targets an adversary-nameable path, so the escape is closed
  rather than narrowed.

### F2 — createExclusiveDir intermediate-symlink escape (LOW, CONFIRMED)

- `internal/fabricengine/destroy.go:884` (`createExclusiveDir`), call site `clone.go:233` (hub
  create).
- `os.Mkdir(path)` never follows a symlink at the FINAL component (EEXIST — safe), but DOES create
  the leaf through a symlink planted at an INTERMEDIATE ancestor component, landing the new directory
  outside the intended container. Verified directly: `os.Mkdir` through an intermediate symlink → dir
  created in the outside dir; `os.Root.Mkdir` rooted at the parent → refused ("path escapes from
  parent").
- Exposure is lower than F1 (hubPath's parent is the operator cwd, far less toggleable mid-call than
  a slug sibling path), but it is the same class and the fix is clean.
- CONFIRMED (unit-level filesystem experiment). Fix: create the leaf via `os.Root` rooted at
  `filepath.Dir(path)`.

### F3 — non-gated weft/board/reconcile worktree-add sites share F1's escape class (LOW, CONFIRMED)

- `internal/fabricengine/weftwiring.go:122` (`createWeftWorktree`), `add.go:158` (weft adopt),
  `reconcile.go:513` (`adoptWeftWorktree`), `boardweft.go:41,50` (`ensureBoardWorktree`).
- Each runs `git worktree add <...> <target> [...]` at a fabric-derived target with a preceding
  check-then-act window, so each has the same symlink-follow-write exposure as F1 (weft targets are
  hub siblings; `_board` is a hub child). These sites deliberately do NOT route through the gate's
  `createGitWorktree` (creation, not destruction), but the containment property is identical.
- CONFIRMED that the staging+rename+repair helper handles all their arg forms (`-b <b> <t> <start>`,
  adopt `<t> <branch>`, `--orphan -b <b> <t>`) — tested directly against real git.
- Fix: route all of them through the same shared helper F1 introduces.

### F4 — round-4 F2 WARN-log regression test does not sabotage-prove the log line (NIT, CONFIRMED)

- `internal/fabricengine/add_rollback_adopt_test.go`
  (`TestAddRollback_WarpBranchLeftBehindUnderEmptyPrefix`) asserts the bare-slug warp branch is LEFT
  BEHIND after rollback, but does not assert the `logger.Warn` at add.go:320 actually fires —
  reverting that production WARN hunk leaves the test green (it only asserts pre-existing
  branch-left-behind behavior).
- CONFIRMED by reading the test. Fix: add a test that captures the logger output and asserts the WARN
  line fires on the refused bare-slug branch deletion.

## Docs & operability findings

(provisional)

## What was tested

Exact commands and observations, appended incrementally as each scenario returns.

### Hermetic gates (pre-review baseline, clean tree at 08520a1b + report skeleton)

- `go build ./...` — rc=0.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` — rc=0, no output.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — all ok.

### Substrate experiments (git behavior, pre-fix)

- `git worktree add -b wt <symlink→outside>` → follows the symlink, writes full worktree (`.git`,
  files) into the outside dir, registered at the resolved path, rc=0. CONFIRMS the escape mechanism.
- `git worktree add -b b <existing-empty-real-dir>` → succeeds, writes into it. `git worktree add`
  into a dir containing any file → `fatal: '...' already exists` (rc=128). So git needs empty/absent
  target → outside overwrite of pre-existing content is NOT possible (narrows blast radius).
- `git worktree move <staging> <symlink→outside>` → FOLLOWS the symlink (re-introduces escape) — so
  the move must NOT be done by git.
- `os.Root.Rename(staging, target)` with a planted symlink at target → refused ("not a directory"),
  nothing written outside; clean case (target absent) → succeeds, target is a real dir. This is the
  window-closer.
- Full sequence `git worktree add <staging>` → `os.Root.Rename` → `git worktree repair <target>`:
  registration corrected, `git worktree list` shows target, git status/rev-parse inside target work.
  Verified for all three arg forms (weft-create with start-point, adopt, `--orphan`).
- `os.Mkdir` through an intermediate symlink → escapes; `os.Root.Mkdir` → refused. (F2.)

### Primary-target live reproduction (pre-fix, CONFIRMED escape)

- Built real hub: bare `warp.git` (seeded, HEAD→main) + bare `weft.git`, then
  `lyx fabric clone <weft> <warp> --subpath backend` (dev binary from `./deploy-dev`).
- Toggler: `ln -sfn $OUT $target; rm -f $target` in a tight loop at `<hub>/racewt` while
  `lyx fabric add racewt` ran repeatedly (fresh branch cleanup between attempts).
- Result: ESCAPE at attempt 12 — `$OUT/.git` (a full worktree fragment) written through the symlink
  into the outside directory. Confirms F1 live against the real substrate.

### Post-fix verification

- Hermetic: `go build ./...`, `go vet` (fabric pkgs), `go test <fabric pkgs> ./cmd/lyx/... -count=5` — all green.
- Guards: `TestNoDestructiveBypass_FabricengineProductionSource`, `TestMutationRecord_...`,
  `TestEnforcement_MarkdownLinks`, `TestEnforcement_FabricVocabulary`,
  `TestHermeticGitEnv_...`, `TestTierPurity_...` — all green.
- Full live integration suite `-tags integration` across fabricengine/fabriccli/gitexec/gitrepo — green.
- 4× concurrent integration suites (compile once, run 4, parallel=8): all rc=0, all PASS; no
  corruption markers (the grep hits were false positives on the word "failed" inside expected
  refusal WARN logs), no `.fabric-wt-staging-*` debris, no stray git processes.
- New tests determinism: `TestContainedWorktreeAdd_*` + `TestAddRollback_RefusedWarpBranchDeletionLogsWarn`
  `-count=5` — green.
- Sabotage proofs: reverting containedWorktreeAdd to a direct `git worktree add target` writes
  `.git`/`README` OUTSIDE the container (F1 test fails); removing rollbackAdd's WARN hunk fails the
  F4 test.
- Live re-attack of F1 (post-fix binary): 3 batches × 400 = 1200 toggle-race trials against
  `lyx fabric add` — 0 escapes (vs. pre-fix escape at attempt 12).
- Live happy path (post-fix binary): fresh `lyx fabric clone` + `add` + `remove` + `reconcile` on a
  real local-remote hub — all ok:true, worktrees land at nominal paths, git admin dirs named after
  slugs (warp `livetask`, weft `_board`/`livetask-weft`), no staging debris.

### Teardown

All scratch hubs I created (repro9L3O, liveAwY7, lv2evws) torn down; test binary and run logs removed;
zero stray git processes. Pre-existing `.gitrepo-push.lock`/`exclude.lyx.lock` files remain under
unrelated earlier-session scratch dirs (`drive1`, `verify2`) — not created by this round, left
untouched.

## Docs & operability findings

No standalone docs defect found. Documentation updated in the SAME commits as the fixes:
`internal/fabricengine/doc.go` ("The destruction chokepoint" now covers the create-side twin),
`internal/fabricengine/destroy.go`'s header, and `CONSTRAINTS.md`'s Fabric Destruction Chokepoint
Invariant (containment bullet extended to the two create-side minters). `docs/overview.md` and
`manifest/roadmap.md` were correctly NOT touched (no module-table move, hardening not a roadmap item).

## Merge-readiness

MERGEABLE. The seeded primary residual (F1) is closed and independently re-attacked; two sibling gaps
(F2, F3) closed via the same containment machinery; the NIT (F4) test-coverage gap closed and
sabotage-proved. Permanent out-of-scope limits unchanged: Windows path behavior (never-executed on a
Linux host), and N4's dirtiness-probe TOCTOU (accepted, documented residual).
