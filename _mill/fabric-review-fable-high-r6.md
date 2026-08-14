# fabric — independent review (round 6, `fable-high-r6`)

Clean-room independent review of `internal/fabricengine` (+ `fabriccli`, `gitexec`, `gitrepo`),
per `_mill/fabric-review-prompt.md`. Model/effort: Fable/high.

Job 1 (this file) is complete and committed BEFORE any production/test file is touched (sequencing rule).

## Executive summary

The round's primary target — round 5's own fix for the create-side symlink-directed-write — is
**CONFIRMED broken exactly as the seed described**. `containedWorktreeAdd` stages `git worktree add`
at `container/.fabric-wt-staging-<random>/<slug>`: a **random PARENT** but a **predictable LEAF**
(the slug). An adversary who can observe the hub sees the staging parent appear, then plants a
symlink at the predictable `<parent>/<slug>` leaf before git's subprocess writes there; `git worktree
add` resolves and follows that symlink and writes a whole worktree OUTSIDE the container, while
`containedWorktreeAdd` returns `nil` (false success). Reproduced **12/12** with a same-process
observing watcher. The `os.Root.Rename` half of R5's design is sound (it refuses a symlink at the
final target) — the flaw is upstream, in what path git's own write is aimed at.

Root cause: R5 made the staging PARENT unguessable but left the git-write LEAF predictable (= slug),
purely so git's internal admin dir (`.git/worktrees/<name>`) would be slug-named. A predictable leaf
inside an observable parent is plantable the instant the parent is observed — unguessability of the
parent defends a *guessing* adversary, not an *observing* one.

Fix (verified, Approach A): git's write must target a path whose **basename is itself unguessable and
never materialised on disk until git creates the real worktree there**, so no observer can pre-plant
a symlink at it. Concretely: `git worktree add` writes to a single-level `container/<unguessable>`
that git creates fresh; then `os.Root.Rename(<unguessable>, <slug>)` moves it into place (still
refusing a symlink at the target), then `git worktree repair`. Verified **0/200 escapes** against the
observation attack, **0/200** against an aggressive target-toggle+repair-window attacker (61 benign
fail-closed aborts, 0 corrupt successes), and a clean run produces a fully valid worktree.

Cost of the fix: git's internal admin dir is then named after the random staging token, not the slug.
Two intentionally-unsafe alternatives that would preserve slug-named admin dirs were **empirically
rejected**: (F) create the leaf ourselves and hand git an existing empty dir — 163/400 escapes via an
rmdir+swap window; (G) let git create a slug leaf inside an unguessable parent — git follows a leaf
symlink planted in git's own parent→leaf window. Only an unguessable BASENAME is safe, and an
unguessable basename cannot equal the slug. The admin-naming change ripples to (a) `prune.go`'s
slug-named admin-dir probe and (b) the live-state mutation oracle's `worktreeAdminTarget`, both of
which must become admin-name-independent (match the admin dir by its `gitdir` pointer, not its name).

Merge-readiness (pre-fix): **NOT mergeable** — the primary create-side escape is live and reproduces
at 100% under observation. After the fix + ripples land green and re-attack holds, mergeable subject
to the standing Windows-path limit (never executed from a Linux host) and the accepted N4
dirtiness-probe TOCTOU residual.

## Threat-model note (honest scoping)

The current code already defends a **different-UID** adversary with write access to the hub: the
staging parent is created mode 0755, so a different-UID process cannot create the `<parent>/<slug>`
leaf inside it. The 12/12 reproduction is therefore a **same-UID** observing adversary (the same
model every prior fabric crucible round used when planting symlinks). Same-UID is a weak security
boundary in the abstract, but this is really fabric's **integrity invariant** — "never let a gated
primitive act outside its container when a symlink is present, regardless of who planted it" (the
M3→R2→R3→R5 lineage). A symlink at the staging path can equally come from a buggy concurrent op or an
odd filesystem state, not only a malicious peer. The fix closes it for all planters; that is the bar.

## Scope assessment (plan vs shipped)

No scope drift found in this pass. The create-side containment machinery (`createExclusiveDir`,
`createGitWorktree`, `containedWorktreeAdd`) is wired through the single shared helper at every
worktree-add call site (add.go warp + weft-adopt, weftwiring.go `createWeftWorktree`,
reconcile.go, boardweft.go x2) — confirmed by grep. Fixing the shared helper fixes all sites; no
per-site work is needed for the primary gap, but each site's `container` must remain a direct parent
of `target` for the single-component rename to land (to re-confirm during the fix).

## Findings

### F1 — CONFIRMED — MEDIUM — create-side staging-path observation escape

`internal/fabricengine/destroy.go:966` (`containedWorktreeAdd`) / `:1018` (`mkWorktreeStagingDir`).

Scenario (reproduced): a real warp repo + hub container. Staging = `container/.fabric-wt-staging-<8
random bytes>/<slug>`. A watcher polls `container`, and the instant the random staging parent appears
(before git's subprocess, which has real process-spawn latency, has written into it) plants a symlink
`container/.fabric-wt-staging-<rand>/<slug> -> <outside-the-container>`. `git worktree add
-b <branch> <that path>` follows the symlink and writes a full, populated worktree into
`<outside>`; `os.Root.Rename` then renames the symlink itself into place, leaving `container/<slug>`
a symlink pointing outside and the worktree tree outside the container. `containedWorktreeAdd`
returns `nil` — the exact false-success shape M3/R3 had (`ok:true`, exit 0).

Evidence: standalone harness replicating the exact current staging structure (random parent via
`os.Root.Mkdir(name,0o755)`, `git worktree add <parent>/<slug>`, `os.Root.Rename`) + same-process
observing watcher: **12/12 escapes, all with `err=nil`** (worktree tree landed in `outside`).
Underlying primitive confirmed directly: pre-planting `<parent>/<slug>` as a symlink and running real
`git worktree add` writes `.git`+tracked files into the outside dir.

Why R5 missed it: R5's re-attack only toggled a symlink at the FINAL target (which `os.Root.Rename`
correctly refuses), never at the new staging surface its own fix introduced. The regression test
`TestContainedWorktreeAdd_RefusesSymlinkedTarget` pins only the final-target refusal.

Severity: MEDIUM (100% under observation vs the ~1.7% of the round-4 seed it replaced; false success;
worktree written outside the container). Not BLOCKING because it requires an observing planter racing
the add and, in a single-tenant deployment, the same-UID caveat above.

Fix (Approach A, verified 0/200): rewrite `containedWorktreeAdd`/`mkWorktreeStagingDir` so git writes
to a single-level, crypto-random-named staging directory directly under `container` that **git creates
fresh** (name generated in memory, never pre-materialised on disk, so no observer can pre-plant a
symlink at it). Then `os.Root.Rename(<staging>, <slug>)` moves it into place (unchanged final-target
refusal), then `git worktree repair <target>`. On git-add failure: `root.RemoveAll(<staging>)`; on
rename failure: `git worktree remove --force <stagingPath>` then `root.RemoveAll(<staging>)`.

Required ripples (same change):
- `internal/fabricengine/prune.go:335` probes `.git/worktrees/<slug>` by name to decide whether to
  record `KindWorktreeRemoved` for the warp admin registration `git worktree prune` clears. The admin
  dir is now named after the staging token, not the slug, so this probe must match the admin dir by
  its `gitdir` pointer (`.git/worktrees/*/gitdir` == `<hubPath>/<slug>/.git`), not by name — else the
  mutation record silently under-reports (Mutation Record Invariant regression).
- `internal/fabricengine/livestate_mutationoracle_test.go` `worktreeAdminTarget` derives the expected
  admin path as `<prime>/.git/worktrees/<slug>` from the record target's slug basename. It must become
  admin-name-independent (discover the admin dir for a worktree-rooted record by `gitdir` match) so
  the honesty assertion still covers the real (randomly-named) admin dir without weakening precision.
- `internal/fabricengine/create_containment_toctou_integration_test.go:107` asserts the admin dir is
  slug-named; update it (add a regression test for the staging-observation escape while there).
- `internal/fabricengine/doc.go` "the two CREATE-side minters" paragraph and
  `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant both say the staging path is a
  "collision-free os.Root-created staging path"/"named after the slug"; update to describe the
  unguessable-basename, git-created staging and the admin-dir-naming consequence.

## What was tested (Job 1)

Commands and observations logged incrementally below.

- Read in full: `internal/fabricengine/doc.go`, `destroy.go`, `ancestors.go`, `add.go`; call sites in
  `weftwiring.go`/`reconcile.go`/`boardweft.go` (grep); `create_containment_toctou_integration_test.go`;
  `CONSTRAINTS.md`; `crucible/README.md`.
- `git worktree add <dir-with-planted-symlink-leaf>` → follows the symlink, writes worktree OUTSIDE
  (`.git` + `f` landed in `outside/`). CONFIRMED the primitive escape.
- `git worktree add <existing-empty-dir>` → accepted; admin dir = basename. `git worktree add
  <parent-that-does-not-exist>/<leaf>` → git creates the intermediate parent; admin = leaf basename.
- Race harness (current staging structure) + observing watcher, 12 trials → **12/12 escapes, err=nil**.
- Race harness (Approach A: single unguessable staging, git creates fresh) + observing watcher, 200
  trials → **0/200 escapes, 0 add-failures**; + aggressive target-toggle/repair-window attacker, 200
  trials → **0/200 escapes, 61 benign fail-closed aborts, 0 corrupt successes**.
- Race harness (Approach F: os.Root-created slug leaf handed to git) + rmdir+swap attacker, 400 trials
  → **163/400 escapes** → Approach F REJECTED.
- Clean (no attacker) Approach A run → `git worktree list` correct, branch checked out, files present,
  real dir, no staging debris, `git worktree remove <target>` works by path; admin dir = random token.
- `gitdir` pointer contents confirmed: `.git/worktrees/<name>/gitdir` == `<worktreePath>/.git` — the
  name-independent match key for the prune + oracle ripples.

### Could NOT verify
- Windows directory-junction path behaviour — out of scope, unreachable from a Linux host (standing
  campaign limit).

## Secondary sweep

Pending (see later commits to this file): a focused sweep of the rest of the module after the primary
fix, weighted per the merge bar (normal single-instance correctness is the gate).
