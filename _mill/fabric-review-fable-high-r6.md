# fabric — independent review (round 6, `fable-high-r6`)

Clean-room independent review of `internal/fabricengine` (+ `fabriccli`, `gitexec`, `gitrepo`),
per `_mill/fabric-review-prompt.md`. Model/effort: Fable/high.

Job 1 (this file) is complete and committed BEFORE any production/test file is touched (sequencing rule).

## Executive summary

The round's primary target — round 5's own fix for the create-side symlink-directed-write — is
**CONFIRMED to leave a live false-success escape**. `containedWorktreeAdd` stages `git worktree add`
at `container/.fabric-wt-staging-<random>/<slug>`: a **random PARENT** but a **predictable LEAF** (the
slug). An observer who can write into the hub sees the staging parent appear, then plants a symlink at
the predictable `<parent>/<slug>` leaf before git's subprocess writes there; `git worktree add`
follows that symlink and writes a whole worktree OUTSIDE the container. The killing detail is that
R5's `os.Root.Rename(<parent>/<slug>, <slug>)` then renames **the swapped leaf symlink itself** onto
the target (os.Root.Rename refuses a symlink at the *destination* — R5's tested case — and refuses a
symlink at an *intermediate source* component, but does NOT refuse a symlink at the *source's final
component*: it renames the link), so `container/<slug>` ends up a symlink pointing outside, the
worktree tree is outside, and the call returns `nil` — M3's exact false-success shape. Reproduced
**12/12** with a same-process observing planter.

Root cause: R5 made the staging PARENT unguessable but left the git-write LEAF predictable (= slug),
purely so git's internal admin dir (`.git/worktrees/<name>`) would be slug-named; and it never
verified, after the move, that the placed target is a real directory rather than a followed symlink.

Corrected analysis (my first draft of this report proposed a single-level unguessable-staging rewrite
"Approach A"; deeper testing showed that draft was WRONG — see "Rejected fix directions" below). The
current TWO-LEVEL design is actually necessary: os.Root.Rename's intermediate-source-symlink refusal
is what makes a *parent*-swap fail closed, and the 0700/0755 parent is what denies a *different-UID*
planter the leaf write. Single-level staging removes that protective parent and REGRESSES the
different-UID posture (a direct rmdir+swap of the single staging dir escapes: reproduced 121/600).

Fix (verified): keep the two-level `container/<random-parent 0700>/<slug>` staging, and make
`containedWorktreeAdd` **fail closed** by verifying containment is bound to the act at BOTH the staging
write and the placement:
- PRE-rename: after `git worktree add`, `os.Root.Lstat` the staging leaf; a symlink there means git
  followed a planted link and wrote outside — abort, remove the escaped worktree via git, clean up.
- POST-rename: after `os.Root.Rename`, `os.Root.Lstat` the target; a symlink there means the leaf was
  swapped in the pre-check→rename TOCTOU and the link got renamed onto the target — abort, unlink the
  symlink (never its target), remove the escaped worktree, clean up.

Verified against a realistic single-plant observing attacker: **0/400 false-success, hub stays clean
(no staging debris, target never a dangling symlink), 399 clean fail-closed aborts, 1 legitimate
success**; and 0 false-success under a relentless leaf+parent toggling attacker. Different-UID
protection is unchanged (parent-swap → os.Root intermediate-source refusal → abort; leaf-swap →
denied by the 0700 parent). Slug-named admin dir is preserved, so there is **zero blast radius** on
`prune.go`'s admin probe or the live-state mutation oracle.

Honest residual: a same-UID (or root) planter actively racing the add can still cause git to
transiently write a worktree checkout into a directory *it already controls* — this is fundamentally
undefendable (a same-UID process can substitute any path fabric writes to, anywhere, /tmp included,
and can read the repo directly regardless). What the fix guarantees is that fabric never REPORTS such
an add as success, never leaves `container/<slug>` a dangling out-of-hub symlink, and leaves the hub
consistent. That is the achievable integrity bar; perfect prevention against a same-UID racer is not
achievable by any staging location.

Merge-readiness (pre-fix): **NOT mergeable** — the create-side false-success escape is live. After the
fix lands green and re-attack holds, mergeable subject to the standing Windows-path limit (never
executed from a Linux host), the accepted N4 dirtiness-probe TOCTOU residual, and the honest same-UID
residual above.

## Threat-model note (honest scoping)

Fabric creates the hub at mode 0755 (`createExclusiveDir` → `os.Root.Mkdir(_, 0o755)`), so a
**different-UID** process cannot write into the hub at all — meaning the only planter who can put a
symlink at a staging path is **same-UID or root**. Same-UID is a weak security boundary in the
abstract (a same-UID process can `git worktree add` anywhere, or edit files directly), but this is
really fabric's **integrity invariant**: "never REPORT success while a gated primitive acted outside
its container, and never leave the hub pointing outside itself" (the M3→R2→R3→R5 lineage). A symlink
at the staging path can equally arise from a buggy concurrent op or an odd filesystem state, not only
a malicious peer. The fix makes the operation fail closed for every planter; that is the bar, and it
is the bar the fix actually meets (as opposed to "prevent the transient write", which no staging
location can meet against a same-UID racer).

## Rejected fix directions (empirically, not by argument)

- **Single-level unguessable staging (`container/<unguessable>`, git creates fresh)** — my first
  draft. REGRESSES different-UID: the single staging dir sits directly under the attacker-writable
  hub, so a direct rmdir+swap of it (git creates it empty, then populates via path re-resolution)
  escapes. Reproduced **121/600**. Also breaks slug admin naming (blast radius on prune + oracle).
- **Approach F — create the slug leaf ourselves, hand git an existing empty dir** — window between our
  `mkdir` and git's `open` lets a same-UID rmdir+swap redirect git's write. Reproduced **163/400**.
- **Approach G — let git create a slug leaf inside an unguessable parent** — git follows a leaf
  symlink planted in git's own parent→leaf internal window; and git follows any pre-planted leaf
  symlink (proven directly).
- **Outside-the-hub private-perms temp staging (the seed's suggested direction)** — a cross-device
  `os.Root.Rename` cannot use a single `rename(2)` and fails EXDEV, so it is not correct when `/tmp`
  is a separate mount; and it does not help against the same-UID planter (who owns the temp dir too).
  Same-filesystem-but-outside-attacker-reach locations under the hub do not exist, since the hub is
  the attacker's write foothold and anything directly under it is rmdir+swap-able while empty.

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

Severity: MEDIUM (100% false-success under observation vs the ~1.7% of the round-4 seed; a worktree
written outside the container and a dangling out-of-hub symlink at `container/<slug>`, all reported as
`ok:true`/exit 0). Not BLOCKING: the realistic planter is same-UID/root (fabric's hub is 0755, so no
different-UID process can plant a symlink in it), and the current design already fails closed for a
different-UID planter.

Fix (verified — keeps the two-level design, adds fail-closed act-binding):
- Keep `container/.fabric-wt-staging-<random>/<slug>` staging (the two-level structure is load-bearing
  — see "Rejected fix directions"); tighten the parent to mode 0700 (defense-in-depth: denies a
  different-UID planter even LIST access to the staging leaf).
- PRE-rename check: after `git worktree add`, `os.Root.Lstat(stagingRel)`; if it is not a real
  directory (a symlink → git followed a planted link and wrote outside), abort: `git worktree remove
  --force` the escaped worktree (resolve it via `os.Readlink`), `root.RemoveAll(parent)`, `git
  worktree prune`, return a `*destructiveRefusal`-shaped error.
- POST-rename check: after `os.Root.Rename(stagingRel, rel)`, `os.Root.Lstat(rel)`; if the target is
  not a real directory (a symlink → the leaf was swapped in the pre-check→rename TOCTOU and the link
  was renamed onto the target), abort: unlink the symlink via `os.Root.Remove` (never its target),
  remove the escaped worktree, clean up, return an error.
- Only after both checks pass: `root.Remove(parent)` (now empty), `git worktree repair <target>`.

Verified: 0/400 false-success under a realistic single-plant observing attacker (hub clean, 399
fail-closed aborts, 1 legitimate success); 0 false-success under a relentless toggling attacker;
different-UID protection unchanged; normal (no attacker) path yields a valid worktree.

Zero blast radius: the git admin dir stays slug-named, so `prune.go:335`'s admin probe and the
live-state oracle's `worktreeAdminTarget` are untouched.

Docs to update in the same change:
- `internal/fabricengine/doc.go` "the two CREATE-side minters" paragraph and destroy.go's header
  comment: describe the pre/post fail-closed act-binding (currently they claim the random staging
  path + os.Root.Rename alone close the create-side window, which the false-success repro disproves).
- `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant: same correction to the
  `containedWorktreeAdd` sentence.
- `internal/fabricengine/create_containment_toctou_integration_test.go`: add a regression test that
  reproduces the staging-leaf observation swap and asserts fail-closed (no false success, target not a
  symlink, hub clean) — the existing tests only cover the final-target-symlink refusal.

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
- `os.Root.Rename` symlink semantics pinned directly (Go 1.26): source final-component symlink →
  **renamed as a link** (dst becomes a symlink, err=nil) — this is the escape carrier; source
  intermediate-component symlink → **refused** ("path escapes from parent"); dest final-component
  symlink → **refused** ("not a directory", R5's tested case). This is why the leaf-swap escapes and
  the parent-swap does not.
- Single-level unguessable staging + staging-dir-swap attacker, 600 trials → **121/600 escapes** →
  REGRESSES different-UID posture, REJECTED (my first-draft "Approach A" was wrong; a leaf-plant
  attacker gave a false 0/200 because it attacked the wrong shape).
- Approach F (os.Root-created slug leaf handed to git) + rmdir+swap attacker, 400 trials → **163/400
  escapes** → REJECTED.
- Two-level + PRE/POST fail-closed check + single-plant observing attacker, 400 trials → **0/400
  false-success, 0 hub staging debris, 399 fail-closed, 1 legitimate success**. Relentless
  toggling attacker → 0 false-success. Clean (no attacker) run → valid worktree, slug-named admin dir,
  removal by path works.

### Could NOT verify
- Windows directory-junction path behaviour — out of scope, unreachable from a Linux host (standing
  campaign limit).

## Secondary sweep

Focused sweep (the module has had five hard rounds + a CLOSED-AND-VERIFIED list; this is a targeted
pass, not a re-run of round 1's breadth):

- **All six worktree-add call sites route through the one `containedWorktreeAdd` helper** and every
  one passes a `container` that is the direct parent of `target` (add.go warp+weft-adopt →
  `l.HubPath`; weftwiring.go `createWeftWorktree` → `l.HubPath`; reconcile.go `adoptWeftWorktree` →
  `warpLayout.HubPath`; boardweft.go x2 → `filepath.Dir(boardPath)`). Confirmed by reading each. The
  single-helper fix covers all six; no per-site work needed. (F1)
- **NIT-F2 — CONFIRMED — `containedWorktreeAdd` repair-failure leaves a half-placed worktree.**
  `destroy.go:1008`: on `git worktree repair <target>` failure the function returns an error but the
  worktree is already renamed into `target` with a STALE git registration (still naming the staging
  path). The caller's rollback then calls `git worktree remove <target>`, which can itself fail on the
  broken registration. Pre-existing, minor. Fold into the F1 rewrite: on repair failure, `git worktree
  remove --force <target>` + `git worktree prune` so rollback sees a clean slate. Fixed as part of F1.
- **NIT-F3 — round-4 F2 follow-up — `rollbackAdd`'s WARN-on-refused-branch-deletion (add.go:316-323)
  has no test that sabotage-proves the log line.** Named as a minor open item in the seed. Low value
  (a log-line assertion), deferred unless the F1 test work makes it convenient — see fixer report.
- No new BLOCKING/MEDIUM found outside F1. The delete-side `removeContainedPath` act-binding, the
  ownership predicates, `createdToken` unforgeability, and the gitexec checked/raw split were read for
  context and match their CLOSED-AND-VERIFIED characterisation; not re-litigated per the seed.

## Job 1 status: COMPLETE

Findings: F1 (MEDIUM, CONFIRMED), NIT-F2 (CONFIRMED, folded into F1's rewrite), NIT-F3 (test-coverage,
low). Proceeding to Job 2.
