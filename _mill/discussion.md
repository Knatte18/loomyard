# Discussion: loom: Preflight phase (precondition validation)

```yaml
task: 'loom: Preflight phase (precondition validation)'
slug: loom-preflight
status: discussing
parent: main
```

## Problem

Loom is the phased orchestrator (`lyx loom run`) that drives a task from intent to a
merged change through a fixed phase sequence — Preflight → Discussion → Plan → Builder →
Raddle → Finalize (see [docs/modules/loom.md](../docs/modules/loom.md)). **Preflight is
the first phase**: a pure precondition/validity check that answers one question — *"is this
worktree in a fit state for loom to run a task here?"* — before any LLM, any producer, any
gate runs.

It is deliberately **not** worktree creation (that is `warp`'s job, already built) and
**not** task seeding (a separate future "claim/seed" command — the `mill-claim` analogue).
Preflight only *validates*; it never mutates git or filesystem state.

**Why now:** the loom build order (roadmap milestone 12) pins *contracts first* (done — the
[status schema](../docs/reference/status-schema.md) and
[discussion format](../docs/reference/discussion-format.md)), then **Preflight** as build
piece #2, ahead of the producers and the phase-machine skeleton. Preflight is the first
piece of loom that touches code, and the first code consumer of the pinned `_lyx/status.json`
schema. Building it now (a) unblocks the rest of the loom build order and (b) gives loom a
correct, fail-loud gate so no downstream phase ever runs against a broken worktree.

## Scope

**In:**

- A new `internal/loomengine` Go package exposing a pure function
  `Preflight(l *hubgeometry.Layout) (Report, error)` that validates five preconditions over
  git/filesystem state only.
- The canonical Go type for the `_lyx/status.json` schema (the pinned
  [status-schema.md](../docs/reference/status-schema.md)), plus a strict reader
  (`internal/state.ReadJSON` with `KnownFields(true)`) and a coherence validator implementing
  the schema doc's validation checklist. This type is the one the later phase-machine
  skeleton reuses.
- A new `hubgeometry` accessor returning the host-side `_lyx/status.json` path (required by
  the Hub Geometry Invariant — `_lyx` paths resolve only through `internal/hubgeometry`).
- A new exported host-worktree cleanliness helper `warpengine.HostClean(l *hubgeometry.Layout)`
  (untracked files count as dirty), replacing the ad-hoc inlined `status --porcelain` pattern
  for loom's use.
- Tests: fast untagged unit tests for the pure status.json coherence/parse logic; and
  `integration`-tagged fixture tests (real paired host+weft, `HermeticGitEnv` `TestMain`) for
  the git/filesystem checks.
- Doc updates in the same commit (see Constraints → Documentation Lifecycle).

**Out:**

- **No cobra/CLI module.** No `lyx loom …` subtree is registered in this task. Preflight is
  engine-only; the `loom`/`loomcli` module and its CLI surface land with the phase-machine
  skeleton (build piece #5). This deliberately avoids paying the CLI/Cobra + Sandbox-coverage
  scaffolding before the machine that owns it exists.
- **No worktree creation** (warp owns it), **no task seeding / status.json *writing*** (a
  separate future "claim/seed" command writes the seed; Preflight only reads it).
- **No dispatch logic** (deciding *whether* Preflight runs based on the recorded phase is the
  phase-machine's job — see Decisions → preflight-invocation-model).
- **No auto-repair** of any failed precondition. Preflight reports; it never fixes.
- **No Raddle / Finalize / producers** — separate build pieces.

## Decisions

### package-placement-engine-only

- **Decision:** Implement Preflight as a pure function `Preflight(l *hubgeometry.Layout)
  (Report, error)` in a **new `internal/loomengine` package**. No `loomcli`, no cobra command,
  no registration in `cmd/lyx/main.go` in this task.
- **Rationale:** The task requires Preflight be "testable in complete isolation." A pure
  engine function over a `*Layout` is exactly that. The `loom` cobra module (with its Short,
  help-tree, registration, longlist, and Sandbox-coverage obligations) belongs with the
  phase-machine skeleton that will actually own the `lyx loom` subtree. `docs/modules/loom.md`'s
  module-decomposition table already lists Preflight under "uses existing modules
  (`warp`, `weft`, `board`)", i.e. not a module of its own.
- **Rejected:** (a) Ship a `lyx loom preflight` verb now — pays all CLI/sandbox scaffolding
  early and prematurely pins the `loom` module shape. (b) Fold into `warpcli` — Preflight is
  loom-owned orchestration validation, not warp topology.

### result-error-contract

- **Decision:** `Preflight` returns `(Report, error)` where:
  - `error != nil` means **"couldn't determine"** — an infrastructure failure (git subprocess
    failed to spawn/non-zero in an unexpected way, an I/O/stat error that isn't a clean
    precondition signal, un-resolvable geometry). The caller escalates.
  - `error == nil` with `Report{OK: false, Failures: [...]}` means the checks **ran and
    determined** one or more preconditions are unmet (dirty worktree, out of sync, missing
    seed, incoherent status). The caller blocks and shows the reasons.
  - `error == nil` with `Report{OK: true}` means the worktree is fit for loom to run.
- **Rationale:** Cleanly separates "the repo is not ready" (expected, actionable, listed) from
  "something broke while checking" (infra, escalate). The phase machine needs this distinction.
- **Rejected:** Collapsing every failed precondition into `error` — conflates "dirty repo"
  with "git crashed" and cannot carry a collected list.

### report-shape

- **Decision:** `Report{ OK bool; Failures []Failure }`, `Failure{ Check CheckID; Reason string }`,
  where `CheckID` is a small closed set of string constants (e.g. `geometry`, `worktree-clean`,
  `weft-pairing`, `weft-sync`, `junction`, `seed-missing`, `seed-unreadable`, `seed-incoherent`,
  `half-finished`). `Reason` is the human-readable message (often passed through from the
  underlying helper, e.g. `PairInSync`'s `"host on X, weft on Y"`).
- **Rationale:** Machine-consumable (`CheckID`) for the future phase machine / status narration,
  plus a human string for the operator. `OK == (len(Failures) == 0)`.
- **Rejected:** Plain `[]string` reasons — not machine-classifiable.

### the-five-checks

Preflight validates exactly these, in this order (see check-ordering-and-collection):

1. **Geometry / cwd** — `hubgeometry.Getwd()` then `hubgeometry.Resolve(cwd)` succeeds and
   yields a usable `*Layout` (non-empty `Prime`). Foundational: on failure, short-circuit —
   nothing else is meaningful without a `Layout`. `ErrNotAGitRepo` → a `geometry` failure.
2. **Clean host worktree** — `warpengine.HostClean(l)` reports no changes. **Untracked files
   count as dirty** (see host-clean-untracked-is-dirty).
3. **Weft paired and in sync** — `os.Stat(l.WeftWorktree())` first (a missing weft yields a
   clean `weft-pairing` "not paired" failure); if present, `warpengine.PairInSync(l)` validates
   host-branch == weft-branch **and** the `_lyx` junction is valid and points at the weft
   `_lyx` (see weft-pairing-composition). Sync divergence → `weft-sync`; junction problems →
   `junction`.
4. **Seed exists and is coherent** — the `_lyx/status.json` handoff seed exists, parses
   strictly, and is internally coherent (see status-json-typed-and-strict and
   no-half-finished-prior-run). Read via the host-junction path (see seed-read-path).
5. *(No fifth "no half-finished prior run" as a separate filesystem check — it is folded into
   the coherence half of check 4; see no-half-finished-prior-run.)*

### preflight-invocation-model

- **Decision:** Preflight is invoked **only when the task is at the preflight stage** — the
  phase machine (a later build piece) is responsible for *not* calling Preflight once the
  recorded phase has advanced. Preflight itself is a stateless validator; it does not decide
  whether it should run.
- **Rationale:** loom is re-entrant: work done on machine A (status.json committed via weft,
  pulled on machine B) shows an advanced `phase`; on machine B the phase machine resumes at
  that phase and **skips Preflight entirely**. Because Preflight is never invoked on an
  advanced task, it can safely treat its own invocation as implying a fresh start — which is
  what gives no-half-finished-prior-run its teeth without breaking resume.
- **Consequence for this task:** Preflight does **not** need (and must not add) any
  "am I past preflight?" gate — that would duplicate the phase machine and risk rejecting a
  legitimate resume.

### no-half-finished-prior-run

- **Decision:** "No half-finished prior run" is expressed as **phase-value-agnostic
  fresh-start invariants** plus internal coherence, not as a check for a specific phase
  literal:
  - Fresh-start invariants: `history` is empty, `start_sha` is `null`, `next_action` is
    `null`, `pause_requested` is `false`. Any violation → `half-finished` failure.
  - Internal coherence (from the schema doc's validation checklist): `phase` ∈
    `{preflight, discussion, plan, builder, raddle, finalize, done}`; `stage` ∈
    `{produce, gate}`; every `history[].outcome` ∈ `{approved, stuck}`; `bounced_to` present
    only when `outcome == stuck`; every timestamp is RFC3339 UTC. Any violation →
    `seed-incoherent`.
- **Rationale:** Because Preflight is only invoked at the fresh/preflight stage
  (preflight-invocation-model), a non-empty `history` or a set `start_sha` is direct
  evidence a prior run advanced and then this worktree ended up back at preflight — i.e. a
  half-finished/inconsistent state. Keeping the invariants phase-agnostic makes Preflight
  robust whether the spawn seed starts at `phase: preflight` or `phase: discussion` (the
  schema doc's worked-example seed uses `discussion`; the exact seed phase is pinned by the
  future seed-writing command, not by this task).
- **Rejected:** Hardcoding `phase == "preflight"` — couples Preflight to a seed-phase choice
  this task does not own. Rejecting merely because `phase` has advanced — would break
  legitimate cross-machine resume if Preflight were ever called on such a task.

### status-json-typed-and-strict

- **Decision:** This task defines the **canonical Go type** for the `_lyx/status.json`
  schema in `internal/loomengine` (fields per [status-schema.md](../docs/reference/status-schema.md):
  `slug`, `parent`, `phase`, `stage`, `narration`, `history[]{phase,outcome,bounced_to?,ts}`,
  `start_sha`, `pause_requested`, `next_action`). It is read via `internal/state.ReadJSON[T]`
  with a strict, fail-loud parse (`KnownFields(true)` discipline — the same as builder's
  `ParseOutcome` and the burler verdict parse). Unknown/malformed fields are a hard
  `seed-incoherent` failure, never silently ignored.
- **Rationale:** Preflight is the first code consumer of the pinned schema; defining the type
  here (rather than a throwaway existence check) enforces the contract at its first consumer
  and gives the later phase-machine skeleton the type to reuse. No `schema_version` field
  (the schema doc omits it deliberately — single writer, no version pressure).
- **Rejected:** A minimal "does it parse as *some* JSON" check — leaves the pinned contract
  unenforced at its first consumer and forces the type to be defined later anyway.
- **Open implementation note for the plan:** `state.ReadJSON` takes a `lockPath`; choose a
  lock path consistent with builder's `state.json` convention (e.g. a sibling
  `.status.json.lock` under `_lyx/`). The plan must pin the exact lock-path choice.

### seed-read-path

- **Decision:** Read `status.json` via the **normal host path** `l.LyxDir()/status.json`
  (i.e. through the `_lyx` junction), **not** via `l.WeftLyxDir()`. Error classification:
  `os.IsNotExist` → clean `seed-missing` failure; any other stat/read error → `seed-unreadable`
  failure with reason "unreadable, see check 3 (junction)". Never report a non-`IsNotExist`
  error as "missing".
- **Rationale:** The junction model exists precisely so `_lyx/` reads as part of the host repo;
  code reading `_lyx/status.json` must not need to know it goes via a junction to the weft.
  Reading via `WeftLyxDir()` in Preflight would break that abstraction in the one place that
  should validate the abstraction holds. A broken junction is an **ordering** concern, not a
  path concern: check 3 (junction health) runs before check 4 and produces the authoritative
  junction-broken report; check 4 then attributes any non-`IsNotExist` read error to it
  instead of lying about a missing seed.
- **Rejected:** A weft-path (`WeftLyxDir()`) read that bypasses the junction — decouples the
  two failures at the cost of breaking the junction abstraction in exactly the wrong place.

### host-clean-untracked-is-dirty

- **Decision:** The host-clean check treats **untracked files as dirty** (bare `git status
  --porcelain`, i.e. *not* `--untracked-files=no`). Implemented as a new exported
  `warpengine.HostClean(l *hubgeometry.Layout) (clean bool, reason string, err error)`.
- **Rationale:** A deliberate stance for lyx: everything should be committed at spawn/handoff
  time. Millhouse never gated on stray untracked files and that has been a recurring
  irritation; lyx enforces the pristine-worktree discipline. Existing warp call sites are
  inconsistent (`--untracked-files=no` in `add`/`checkout`, bare `--porcelain` in `remove`);
  Preflight picks the strict policy deliberately.
- **Rationale for a new helper (not inline):** there is no reusable "is this worktree clean"
  function today — the `status --porcelain` pattern is inlined at four warpengine sites. A
  fifth inline copy in `loomengine` is rot. The **host** repo is explicitly unrestricted by
  the Weft Git Invariant ("the host repo is an ordinary project repo"), so a host clean-check
  via `gitexec` is permitted; putting it in `warpengine` keeps host-topology git in the
  package that already owns the pattern.
- **Rejected:** (a) `--untracked-files=no` (ignore untracked) — the exact leniency being
  removed. (b) A 5th inline `status --porcelain` in loomengine — duplication.
- **Open note:** confirm `HostClean` lives in `warpengine` vs a lower `gitexec.IsClean(dir)`;
  the plan should pick one. Recommendation: `warpengine.HostClean(l)` (topology-aware, takes
  the `Layout`).

### weft-pairing-composition

- **Decision:** Check 3 composes two calls: `os.Stat(l.WeftWorktree())` first, then
  `warpengine.PairInSync(l)` only if the weft directory exists. A missing weft → `weft-pairing`
  "weft not paired". `PairInSync`'s results map to: branch divergence → `weft-sync`
  ("host on X, weft on Y"); junction missing/elsewhere → `junction`.
- **Rationale:** `PairInSync` returns an opaque git *error* (not a clean reason) when the weft
  worktree is missing entirely — it assumes the pair resolves. The `os.Stat` pre-check turns
  "no weft" into a clean, distinct precondition failure, keeping "not paired" and "out of sync"
  as separate, honest messages.
- **Note:** `l.WeftWorktree()` is name-based (`<worktree-base>-weft`), and *"at the main
  worktree this equals `WeftRepoRoot()`"* — so pairing resolves uniformly for prime and task
  worktrees alike (see run-in-existing-or-prime-worktree).

### run-in-existing-or-prime-worktree

- **Decision:** Preflight does **not** special-case or reject the Prime/main worktree, and
  does **not** enforce that the worktree directory name matches the seed's `slug`. All five
  checks run uniformly on whatever worktree is current.
- **Rationale:** lyx must support running a new task in an existing worktree. The seed's `slug`
  is a *logical task pointer*; pairing is *name-based* (`WeftWorktree()` = `<worktree-base>-weft`,
  equal to `WeftRepoRoot()` at Prime), so Preflight is inherently slug-agnostic for pairing and a
  worktree-name ≠ slug situation is a non-issue. This is the *simpler* design — dropping a
  prime-rejection gate, not adding one. Whether the worktree is actually paired/seeded is a
  warp/seed-command setup concern; Preflight merely validates it and reports a clean "not paired"
  / "seed missing" if not.
- **Not Prime-limited.** The uniform, name-based checks work on *any* worktree with a healthy
  `-weft` sibling and a seed — a Prime worktree (`<prime>-weft`) or a task worktree
  (`foo` → `foo-weft`) alike. Reusing an existing worktree is "especially relevant for Prime"
  only because Prime is the *durable* worktree that persists and is reused across many tasks,
  whereas task worktrees are normally created fresh per task by `warp` and torn down afterward.
  The mechanism is identical; Prime is merely the common reuse target, not a special case in the
  code.
- **Rejected:** Rejecting `WorktreeRoot == Prime` or enforcing name == slug — would block a
  supported workflow and couple Preflight to a naming convention the seed does not require.

### missing-seed-is-hard-failure

- **Decision:** A missing seed (`os.IsNotExist` on `status.json`) is a **hard precondition
  failure** (`seed-missing`), not an OK pass.
- **Rationale:** Without a seed, `lyx loom run` has no task — nothing to do. The seed's
  existence *is* the handoff signal (per the pinned schema doc). Seeding a worktree with a task
  is a **separate future command** (the `mill-claim` analogue — "seed this worktree with a
  task"), out of scope here. Preflight validates the handoff happened; it never performs it.
- **Rejected:** Treating a missing seed as "nothing started yet, OK" — contradicts the pinned
  schema doc and would let loom run with no task.

### check-ordering-and-collection

- **Decision:** Geometry (check 1) is foundational: if `Resolve` fails, return immediately
  with a single `geometry` failure (no `Layout` ⇒ no other check is meaningful). All remaining
  checks (2 clean, 3 pairing/sync/junction, 4 seed/coherence) **run and collect all their
  failures** into `Report.Failures`. The one interpretation dependency: junction health
  (check 3) informs check 4's error attribution (a non-`IsNotExist` seed read error is reported
  as `seed-unreadable` "see check 3", never `seed-missing`). Check 4 still runs even if the
  junction is broken; it just does not lie about *why* the read failed.
- **Rationale:** Maximal actionable information per run (fix everything in one pass), with only
  the one unavoidable short-circuit (geometry) and honest cross-check attribution.
- **Rejected:** Fully sequential short-circuit — forces fix-rerun-fix cycles.

## Technical context

What the plan needs about the codebase:

- **`internal/hubgeometry`** ([hubgeometry.go](../internal/hubgeometry/hubgeometry.go)) —
  `Getwd()`, `Resolve(cwd) (*Layout, error)` (returns `ErrNotAGitRepo`), `Layout` with
  `Cwd/WorktreeRoot/Hub/RelPath/Prime/Repo`. Existing weft-geometry methods to reuse:
  `LyxDir()` (host `_lyx`), `WeftWorktree()`, `WeftLyxDir()`, `HostLyxLinkHere()`,
  `WeftRepoRoot()`, `PrimeName()`. **This task adds** a `_lyx/status.json` host-path accessor
  here (Hub Geometry Invariant). Note: the Layout construction spawns `git rev-parse` — cheap
  but a spawn (matters for the Test Tier Purity Invariant).
- **`internal/warpengine`** — `PairInSync(l) (ok bool, reason string, err error)` in
  [drift.go](../internal/warpengine/drift.go) is the "drift detection" loom.md references:
  checks host-branch == weft-branch and junction validity; stateless; returns `(false, reason,
  nil)` for out-of-sync (`"host on X, weft on Y"`, `"junction missing"`, `"junction points
  elsewhere"`) and `(false, "", err)` only on system error. Does **not** stat the weft dir
  (missing weft surfaces as a git error). **This task adds** `HostClean(l)`. Existing inline
  `status --porcelain` sites for reference: `add.go:77`, `remove.go:64/79`, `checkout.go:52`.
- **`internal/weftengine`** — `Status(weftWorktree, pathspec) (map[string]any, error)` exists
  (weft- and pathspec-scoped; returns a `dirty` bool) but is **not** a host clean-check; not
  what check 2 needs.
- **`internal/state`** ([state.go](../internal/state/state.go)) — `ReadJSON[T](path, lockPath)
  (T, bool, error)` and `WriteJSON[T]`: locked, atomic, typed. Use `ReadJSON` for the seed with
  strict decoding. This is the schema doc's mandated mechanism (same as builder's `state.json`).
- **Pinned contract:** [status-schema.md](../docs/reference/status-schema.md) — the field set,
  the validation checklist (required fields, enum values, `bounced_to`-only-with-`stuck`,
  RFC3339 UTC timestamps), and the strict-parse discipline. Preflight's coherence validator
  implements this checklist.
- **`internal/lyxtest`** — fixture helpers for integration tests (`CopyPaired*`, `SeedConfig`,
  `MustRun`, `HermeticGitEnv`). A paired host+weft fixture is what checks 2/3/4 exercise.

## Constraints

From [CONSTRAINTS.md](../CONSTRAINTS.md):

- **Hub Geometry Invariant** — all `_lyx`/cwd/geometry path construction goes through
  `internal/hubgeometry`. The new `_lyx/status.json` path accessor **must** live in
  `hubgeometry`; `loomengine` must not construct the `_lyx`/`status.json` path itself. Raw
  `os.Getwd`/`git rev-parse --show-toplevel` are banned outside `hubgeometry`.
- **Weft Git Invariant** — no raw git against a weft worktree outside `weftengine`/`warpengine`.
  Preflight's weft-facing checks go through `warpengine.PairInSync` (and `os.Stat`, which is not
  git). The **host** repo is unrestricted, so `HostClean` (host `status --porcelain`) is
  permitted; it lives in `warpengine` to keep host-topology git in the owning package.
- **Test Tier Purity Invariant** — untagged test files must not spawn (no `gitexec.RunGit`,
  `exec.Command`, `lyxtest.Copy*`). Preflight's git/fixture tests **must** be `integration`-tagged;
  only the pure status.json coherence/parse tests may be untagged (Tier 1). Note even
  `Resolve` on an error path spawns one cheap `git rev-parse` — so any test that calls
  `Preflight`/`Resolve` end-to-end must be tagged.
- **Hermetic Git Test Environment Invariant** — any test package that spawns git (directly or
  via lyxtest fixtures) must have a `TestMain` calling `lyxtest.HermeticGitEnv()`. The
  `loomengine` (and `warpengine`, if `HostClean` tests are added there) test package needs this.
- **CLI / Cobra Invariant** — N/A this task (no cobra module added). When the phase-machine
  skeleton later registers `loom`, it takes on the Short/help-tree/registration/longlist and
  Sandbox-coverage obligations.
- **Documentation Lifecycle** — this task changes design/module surface, so update docs in the
  **same commit**: note Preflight as built in `docs/modules/loom.md` (and mark roadmap
  milestone 12 build-piece #2 done, with a pointer); if a new cross-cutting invariant emerges
  (none expected), record it in `CONSTRAINTS.md`. Package godoc on `loomengine` documents the
  `Preflight` contract, `Report`/`Failure`/`CheckID`, and the status.json type.

## Testing

Hybrid tiering (decision the-testing-tiers):

- **Untagged (Tier 1, fast, no spawn) — pure status.json coherence/parse:**
  - Valid seed → no failure.
  - Unknown field / malformed JSON → `seed-incoherent` (strict `KnownFields`).
  - Missing required field → `seed-incoherent`.
  - Bad enum (`phase`/`stage`/`outcome`) → `seed-incoherent`.
  - `bounced_to` present without `outcome: stuck` → `seed-incoherent`.
  - Non-RFC3339 / non-UTC timestamp → `seed-incoherent`.
  - Fresh-start violated: non-empty `history` / non-null `start_sha` / non-null `next_action` /
    `pause_requested: true` → `half-finished`.
  - These operate on in-memory bytes (or a `t.TempDir()` file that is *read*, no git), so they
    stay spawn-free. TDD candidate — the coherence validator is a pure function and should be
    driven test-first.

- **`integration`-tagged (Tier 2, real fixtures, `HermeticGitEnv` `TestMain`) — git/fs checks:**
  Build a healthy paired host+weft fixture (via `lyxtest.CopyPaired*`), assert `Report.OK`, then
  mutate to trip each check independently:
  - Not a git repo (run from a non-repo dir) → `geometry`, short-circuit (no other failures).
  - `Resolve` yields empty `Prime` (no main worktree) → `geometry`.
  - Host worktree dirty: tracked-modified, staged, **and untracked-only** → `worktree-clean`
    (untracked-only is the deliberate strict-policy case — TDD candidate).
  - Weft worktree missing entirely → `weft-pairing` ("not paired"), distinct from sync.
  - Host and weft on different branches → `weft-sync` ("host on X, weft on Y").
  - Junction missing / points elsewhere → `junction`.
  - Seed missing (`os.IsNotExist`) → `seed-missing` (hard failure).
  - Seed unreadable via broken junction (non-`IsNotExist`) → `seed-unreadable` "see check 3",
    **not** `seed-missing` — asserted alongside the `junction` failure in the same `Report`.
  - Multiple simultaneous failures (e.g. dirty + out-of-sync + incoherent seed) all appear in
    `Report.Failures` (collection behaviour).
  - Running in the **Prime** worktree with name ≠ slug but a healthy pair+seed → `Report.OK`
    (run-in-existing-or-prime-worktree).
- **`warpengine.HostClean`** gets its own focused integration tests (clean, tracked-dirty,
  untracked-only-dirty) if implemented there.

Avoid prescribing exact assertion shapes beyond the `CheckID` each scenario must yield — the
plan pins the concrete table-test structure.

## Q&A log

- **Q:** Is Preflight a loom module of its own, or part of loom? **A:** Part of loom —
  `internal/loomengine`, engine-only, no cobra module yet (the loom.md decomposition table
  already lists Preflight under "uses existing modules").
- **Q:** How is "no half-finished prior run" reconciled with cross-machine resume? **A:** The
  phase machine only *invokes* Preflight at the preflight stage; once status.json shows an
  advanced phase, Preflight is skipped entirely. So Preflight can treat its invocation as a
  fresh start and assert phase-agnostic fresh-start invariants (empty history, null start_sha,
  etc.) without breaking resume.
- **Q:** Fail-fast or collect all failures? **A:** Collect all, except geometry (foundational)
  short-circuits; junction health informs seed-error attribution.
- **Q:** Validate the status.json schema deeply, or just check existence? **A:** Define the
  canonical Go type + strict `KnownFields(true)` parse implementing the schema doc's full
  validation checklist — Preflight is the schema's first code consumer.
- **Q:** Should untracked files count as dirty? **A:** Yes — deliberately stricter than
  Millhouse; everything must be committed at spawn. New `warpengine.HostClean` uses bare
  `--porcelain`.
- **Q:** Read status.json via the weft path or the host junction? **A:** Host junction path
  (`LyxDir()/status.json`) — the junction abstraction must hold. Classify `os.IsNotExist`
  (seed missing) vs other errors (unreadable → "see check 3"); check 3 validates the junction
  first.
- **Q:** How to distinguish "weft not paired" from "weft out of sync"? **A:** `os.Stat` the
  weft worktree first for a clean "not paired"; only then call `PairInSync` for the sync/junction
  verdicts.
- **Q:** Is a missing seed a failure or an acceptable state? **A:** Hard failure — no seed means
  no task. Seeding is a separate future "claim/seed" command (the mill-claim analogue).
- **Q:** Reject running in Prime / require worktree name == slug? **A:** No. Support running a
  new task in an existing worktree, Prime included; the slug is a logical pointer, pairing is
  name-based (and equals `WeftRepoRoot()` at Prime), so this is the simpler design, not the
  harder one.
