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
  `Preflight(l *hubgeometry.Layout) (Report, error)` that validates four preconditions over
  git/filesystem state only (one of them — check 4 — absorbing the no-half-finished condition as
  an internal coherence rule rather than a separate check).
- The canonical Go type for the `_lyx/status.json` schema (the pinned
  [status-schema.md](../docs/reference/status-schema.md)) and a coherence validator implementing
  the schema doc's validation checklist (with the presence nuance in
  field-presence-and-nullability — absent nullable/bool/slice fields satisfy "present" via
  zero/null semantics). This type is the one the later phase-machine skeleton reuses.
- Wording updates to `status-schema.md` (made in the implementation commit per the
  Documentation Lifecycle): validation-checklist item 1 (absent nullable/bool/slice fields
  satisfy "present") and the "Parse discipline" paragraph (realign `KnownFields(true)` →
  `json.Decoder.DisallowUnknownFields()` for the JSON seed).
- A **new shared strict-read primitive `state.ReadJSONStrict[T]`** in `internal/state` (beside
  the existing `ReadJSON`/`WriteJSON`), using `json.Decoder` + `DisallowUnknownFields()` while
  keeping the same shared read-lock and atomic-read behaviour, and exposing `state.ErrRead` /
  `state.ErrDecode` sentinels so callers can tell an I/O read error from a parse error. It is a
  reusable module function, not local to `loomengine` — builder, perch, and the phase-machine
  skeleton can all adopt it. `loomengine` uses it to parse the seed strictly.
- A new **WorktreeRoot-anchored** `hubgeometry` accessor `LoomStatusFile()` returning the
  host-side `<WorktreeRoot>/_lyx/status.json` path (required by the Hub Geometry Invariant —
  `_lyx` paths resolve only through `internal/hubgeometry`). Anchored at `WorktreeRoot`, not
  `Cwd`, so invocation from a worktree subdirectory does not misread the seed (see seed-read-path).
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
  where `CheckID` is a small closed set of string constants (e.g. `geometry`, `worktree-root`,
  `worktree-clean`, `weft-pairing`, `weft-sync`, `junction`, `seed-missing`, `seed-unreadable`,
  `seed-incoherent`, `half-finished`). `Reason` is the human-readable message (often passed
  through from the underlying helper, e.g. `PairInSync`'s `"host on X, weft on Y"`).
- **Rationale:** Machine-consumable (`CheckID`) for the future phase machine / status narration,
  plus a human string for the operator. `OK == (len(Failures) == 0)`.
- **Rejected:** Plain `[]string` reasons — not machine-classifiable.

### the-four-checks

Preflight validates exactly these four, in this order (see check-ordering-and-collection); the
former "fifth" (no half-finished prior run) is folded into check 4 as an internal coherence rule,
not a standalone check:

1. **Geometry / cwd** — `hubgeometry.Getwd()` then `hubgeometry.Resolve(cwd)` succeeds and
   yields a usable `*Layout` (non-empty `Prime`), **and `l.RelPath == "."` (Preflight runs at
   the worktree root)**. Foundational: on failure, short-circuit — nothing else is meaningful
   without a `Layout` that agrees on one `_lyx`. `ErrNotAGitRepo` → a `geometry` failure;
   `RelPath != "."` → a `worktree-root` failure ("run from the worktree root, not <RelPath>").
   See at-worktree-root for why this requirement exists.
2. **Clean host worktree** — `warpengine.HostClean(l)` reports no changes. **Untracked files
   count as dirty** (see host-clean-untracked-is-dirty).
3. **Weft paired and in sync** — `os.Stat(l.WeftWorktree())` first (a missing weft yields a
   clean `weft-pairing` "not paired" failure); if present, `warpengine.PairInSync(l)` validates
   host-branch == weft-branch **and** the `_lyx` junction is valid and points at the weft
   `_lyx` (see weft-pairing-composition). `PairInSync`'s single opaque `reason` string is
   classified into a `CheckID` by prefix (see weft-pairing-composition → reason-classification):
   branch divergence (`"host on …"`) → `weft-sync`; junction reasons (`"junction …"`) →
   `junction`.
4. **Seed exists and is coherent** — the `_lyx/status.json` handoff seed exists, parses
   strictly, and is internally coherent (see status-json-typed-and-strict and
   no-half-finished-prior-run). Read via the WorktreeRoot-anchored `LoomStatusFile()` accessor
   (see seed-read-path).
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
- **The caller contract MUST be carried into godoc (not left implicit).** This invariant —
  "only ever call Preflight at the fresh/preflight stage" — is what makes "non-empty history ⇒
  half-finished" correct rather than a false positive on legitimate cross-machine resume. It is
  owned by a *different, not-yet-built* module (the phase machine, the phase-machine skeleton /
  build-piece #5), so a future implementer wiring the call in will be reading `internal/loomengine`'s
  package/`Preflight` godoc, **not** this discussion doc (which is deleted once loom-preflight
  lands, per the Documentation Lifecycle). Therefore the **`Preflight` godoc comment (and the
  `loomengine` package doc) MUST state the caller precondition explicitly**, in words to the
  effect of: *"Callers MUST NOT invoke Preflight except when the task is at the fresh/preflight
  stage. Invoking it on an already-advanced task (non-empty `history`, set `start_sha`, etc.) is
  a caller error that will be reported as a `half-finished` precondition failure, not diagnosed
  as misuse — because Preflight is a stateless validator and cannot distinguish the two."* This
  is a required deliverable of the implementation commit, not optional prose.

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
  `start_sha`, `pause_requested`, `next_action`). It is read via the **new
  `state.ReadJSONStrict[T]`** (see Scope → In and strict-read-mechanism below), which uses
  `json.Decoder` + `DisallowUnknownFields()` — the JSON-accurate analogue of the
  **DisallowUnknownFields discipline** (this is *not* `KnownFields(true)`, which is a
  `yaml.Decoder` method; the seed is JSON, so the strict-unknown-field guard is
  `json.Decoder.DisallowUnknownFields()`). Unknown/malformed fields are a hard
  `seed-incoherent` failure, never silently ignored.
- **Rationale:** Preflight is the first code consumer of the pinned schema; defining the type
  here (rather than a throwaway existence check) enforces the contract at its first consumer
  and gives the later phase-machine skeleton the type to reuse. No `schema_version` field
  (the schema doc omits it deliberately — single writer, no version pressure).
- **Rejected:** A minimal "does it parse as *some* JSON" check — leaves the pinned contract
  unenforced at its first consumer and forces the type to be defined later anyway.

#### strict-read-mechanism

- **Decision:** `internal/state.ReadJSON` does **not** reject unknown fields today
  (`state.go:71` uses plain `json.Unmarshal`), so the strict parse cannot ride on it as-is.
  Add a **new sibling `state.ReadJSONStrict[T](path, lockPath) (T, bool, error)`** in the
  `internal/state` package that is identical to `ReadJSON` (same shared read-lock, same atomic
  read, same `(zero, false, nil)` on `os.IsNotExist`) except it decodes via
  `json.NewDecoder(...).DisallowUnknownFields()` instead of `json.Unmarshal`. **It also wraps
  its two failure modes with exported sentinels — `state.ErrRead` (the `os.ReadFile` failure)
  and `state.ErrDecode` (the decode failure)** — so callers can tell an I/O read error from a
  parse error via `errors.Is`. `loomengine` calls `ReadJSONStrict`. **Read-only: it does NOT
  `os.MkdirAll` the parent** (unlike `ReadJSON`, whose `state.go:52` `MkdirAll`-on-read is
  nonsensical) — a read must never create directories, so the only filesystem touch is the
  advisory read-lock file (see side-effects).
- **Side effects vs "Preflight never mutates".** "Never mutates" (Problem/Scope) means
  Preflight never mutates **git-tracked / observable repo state** — no worktree content, no git
  ops. The advisory read-lock `internal/lock` takes is an ephemeral `*.lock` file in the weft
  overlay (the same benign lock builder's `state.json` reads take); it is not host-tracked repo
  state and, being weft-side, is invisible to Preflight's host-only clean check (see lock-path
  for why it trips no check and why weft-commit exclusion is a future command's concern). With
  `MkdirAll` dropped, that lock file is the sole filesystem side effect.
- **Rationale:** Keeps one strict primitive in the shared `state` module (reusable by builder,
  perch, and the phase-machine skeleton — a general helper, not local to `loomengine`), with
  **zero blast radius** on existing `ReadJSON` callers (builder's `state.json`), and preserves
  the lock/atomic guarantees. The sentinels are what let Preflight honour result-error-contract
  (a genuine I/O failure must escalate as `error`, not be reported as a determined precondition).
- **Rejected:** (a) Changing `ReadJSON` itself to `DisallowUnknownFields()` — wide blast radius;
  every existing caller suddenly rejects any unknown/forward-compat field. (b) `loomengine`
  reading the bytes with a local decoder — bypasses the shared read-lock and duplicates the
  read primitive. (c) A single opaque `err` (as `ReadJSON` returns) with no read/decode
  sentinels — forces callers to string-match wrap messages to classify, which is fragile.
- **Error classification (read vs decode vs missing).** The missing/unreadable/incoherent
  split (Report `CheckID`s) is derived as:
  - `os.Stat` the seed path **first** (see seed-read-path), and classify its result **gated on
    check 3's outcome**, not on the stat error's shape alone:
    - **If check 3 produced a `junction` *or* `weft-pairing` failure** (junction missing /
      points elsewhere, or the weft worktree is missing so the junction target doesn't exist):
      the `_lyx` junction can't be trusted to reach the seed, and a broken junction / missing
      target commonly makes the seed stat return `IsNotExist` too (the `_lyx` component doesn't
      resolve, or the wrong target has no `status.json`), so *any* stat failure here —
      `IsNotExist` included — is classified `seed-unreadable` (Report, "see check 3"), **never**
      `seed-missing`. This is the case the "see check 3" attribution actually exists for.
      (`weft-sync` is deliberately excluded from this gate: a mere branch divergence leaves the
      junction healthy, so the seed is genuinely readable and normal shape-based classification
      applies.)
    - **Else (check 3's junction/pairing is healthy):** classify by stat shape — `os.IsNotExist`
      → `seed-missing` (Report); any other stat error → `seed-unreadable` (Report).
  - Then, only when the stat succeeded, call `ReadJSONStrict` and classify a non-nil `err` by a
    single rule: **`errors.Is(err, state.ErrDecode)` (parse / unknown-field / type-mismatch) →
    `seed-incoherent` (Report); *every other* non-nil error → escalate as `error`** ("broke
    while checking", per result-error-contract). "Every other" explicitly covers
    `state.ErrRead` (a rare TOCTOU/I/O read failure after a good stat) **and** a
    `lock.AcquireReadLock` failure (a third mode that is neither `ErrRead` nor `ErrDecode`) —
    both are infrastructure failures, not determined preconditions, so only `ErrDecode` ever
    maps to a Report failure.
  - A `found == false` after a successful stat (should not happen) is treated defensively as
    the escalate `error` path (the file was there a moment ago).
- **lock-path.** `ReadJSONStrict` takes a `lockPath`. Follow builder's precedent — builder puts
  `state.json.lock` beside its state file under `_lyx/builder/` — so loom's lock is a `*.lock`
  beside the seed (e.g. `<WorktreeRoot>/_lyx/status.json.lock`). **This lock poses no risk to any
  of *this task's* checks:** Preflight's clean check (check 2) is **host-only** (`git status` on
  the host worktree) and does not see weft-tree files through the `_lyx` junction, so a lock file
  living in the weft overlay never appears in the host `git status` and cannot trip
  `worktree-clean`. Weft-side commit exclusion is **not** a `.gitignore` or `ScopedPathspec`
  concern (there is no `*.lock` gitignore entry, and `ScopedPathspec` does not exclude) — exclusion
  is a per-command `:(exclude)*.lock` pathspec token owned by whichever command *commits* the weft
  (e.g. builder's, `weft.go:36`). Preflight **commits nothing**, so ensuring the lock isn't
  weft-committed belongs to the future seed/loom-commit command, **out of scope here**. The plan
  need only pin the exact lock-path (matching builder's convention); the weft-exclusion obligation
  travels with the future committing command.

#### field-presence-and-nullability

- **Decision:** `DisallowUnknownFields()` rejects *extra* fields but silently zero-fills
  *absent* ones, so presence of required fields is validated **explicitly after decode**, scoped
  by field semantics — not by making every field a pointer:
  - **Mandatory strings** `slug`, `parent`, `phase`, `stage`, `narration` — plain `string` in
    the struct, validated by an explicit **non-empty check** after decode. This catches both a
    literally-absent field (zero-fills to `""`) and a present-but-empty field; either → a
    `seed-incoherent` failure identifying the missing/empty field.
  - **Nullable fields** `start_sha`, `next_action` — `*string` (nil ⇔ JSON `null` ⇔ absent).
    Presence is **not** enforced: an omitted `start_sha` is semantically identical to the
    required `null`, and the fresh-start invariant already pins both to `null`. Modelling them
    as `*string` (rather than `string`) is what lets the coherence rules distinguish `null` from
    a real value (e.g. "`start_sha` must be `null` unless `phase ≥ builder`").
  - **Bool** `pause_requested` — plain `bool`; absent ⇒ `false`, which is the valid fresh value,
    so presence is not enforced.
  - **`history`** — a slice; absent (`nil`) and present-empty (`[]`) are behaviourally identical
    and both satisfy the fresh-start "empty history" invariant, so no absence detection is
    needed. Each entry's **`bounced_to`** is `*string`, presence-conditional on
    `outcome == "stuck"` (coherence rule).
- **Rationale:** Enforcing structural presence only where the zero value is ambiguous/invalid
  (the mandatory strings) gives real safety; enforcing it on fields whose zero value *is* a
  valid state (null/false/empty-slice) would flag semantically-valid seeds and add pointer-noise
  for no benefit. The `*string` choice for the nullable fields is driven by the coherence rules,
  not by presence detection.
- **Rejected:** (a) All-pointer / `json.RawMessage` structural presence for all nine fields —
  flags valid seeds (absent ≡ valid null/false), noisy. (b) Non-empty strings only, with
  `start_sha`/`next_action`/`bounced_to` as plain `string` — cannot represent JSON `null` vs
  absent, weakening the coherence rules.
- **Test-plan wording.** "Missing required field → `seed-incoherent`" means a missing/empty
  **mandatory string** (`slug`/`parent`/`phase`/`stage`/`narration`); the nullable/bool fields
  are not presence-tested (their absence is a valid state).
- **Reconciliation with the pinned schema doc.** `status-schema.md`'s validation-checklist
  item 1 says *all nine* fields must be "present". This decision **intentionally** treats an
  absent `history` / `start_sha` / `pause_requested` / `next_action` as *satisfying* "present"
  through zero/null semantics — an omitted field decodes to exactly the value the checklist's
  other items already require (empty slice / `null` / `false`), so there is no observable
  difference between "absent" and "present with the required zero value". This is a
  clarification of the checklist, not a contradiction of it. **The implementation must update
  `status-schema.md` in two places** (a doc change made in the mill-go commit per the
  Documentation Lifecycle — this discussion does not edit the pinned contract): (1) the
  validation-checklist **item 1** wording, to state that absent nullable/bool/slice fields
  satisfy "present" via zero/null semantics; and (2) the **"Parse discipline"** paragraph (which
  currently describes the JSON status.json strict parse as "the same `KnownFields(true)`
  discipline" — a `yaml.Decoder` API), realigning it to `json.Decoder.DisallowUnknownFields()`
  for the JSON seed. Only the five mandatory strings are structurally presence-enforced.

### seed-read-path

- **Decision:** Read `status.json` via the **normal host path**, resolved by a **new
  WorktreeRoot-anchored `hubgeometry` accessor `l.LoomStatusFile()` = `filepath.Join(l.WorktreeRoot,
  LyxDirName, "status.json")`** (i.e. through the worktree-root `_lyx` junction), **not** via
  `l.WeftLyxDir()`, and **not** via `l.LyxDir()` (which is `Cwd`-anchored, hubgeometry.go:319).
  Because check 1 requires `RelPath == "."` (see at-worktree-root), `Cwd == WorktreeRoot`, so
  `LoomStatusFile()` (WorktreeRoot-anchored) resolves to the **same `_lyx`** that check 3's
  junction validation (`PairInSync` → `HostLyxLinkHere()` = `WorktreeRoot/RelPath/_lyx`) checks
  and that `HostClean`/the branch check use — all four checks agree on one `_lyx`, and the
  junction check 3 validated is the very junction the seed read traverses. WorktreeRoot-anchoring
  is the canonical choice and is defence-in-depth even though `RelPath == "."` already makes it
  equal to `Cwd`. Error classification is **gated on check 3's outcome, not on the stat error's
  shape** (see strict-read-mechanism → Error classification for the full rule): a `os.Stat` that
  fails when check 3 has already reported a `junction` or `weft-pairing` failure is classified
  `seed-unreadable` "see check 3" — *including* `IsNotExist`, because a broken junction / missing
  weft target commonly surfaces as `IsNotExist` at the seed path, and that is exactly the case
  the attribution exists for. Only when check 3's junction/pairing is healthy does a stat failure
  classify by shape (`IsNotExist` → `seed-missing`; other → `seed-unreadable`). After a successful
  `os.Stat`, the parse goes through `state.ReadJSONStrict` (see strict-read-mechanism), and its
  error follows the single rule there: `state.ErrDecode` → `seed-incoherent`; **every other**
  non-nil error (`state.ErrRead`, a `lock.AcquireReadLock` failure, anything else) → escalate as
  `error`, not a Report failure.
- **Rationale:** The junction model exists precisely so `_lyx/` reads as part of the host repo;
  code reading `_lyx/status.json` must not need to know it goes via a junction to the weft.
  Reading via `WeftLyxDir()` in Preflight would break that abstraction in the one place that
  should validate the abstraction holds. A broken junction is an **ordering** concern, not a
  path concern: check 3 (junction/pairing health) runs before check 4 and produces the
  authoritative junction-broken/weft-missing report; check 4 then, seeing that outcome, attributes
  *its own* read failure (`IsNotExist` included) to it — "seed-unreadable, see check 3" — instead
  of lying about a missing seed. The gate is check 3's **outcome**, not the stat error's shape,
  because the realistic broken-junction failure surfaces as `IsNotExist` at the seed path, so a
  shape-based rule would mislabel the common case `seed-missing`.
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
- **reason-classification.** `PairInSync` returns a single opaque `reason` string, not a typed
  `CheckID`. `loomengine` maps it to a `CheckID` by **prefix match on the known reason strings**
  the function emits (`drift.go`): a reason beginning `"host on "` (branch divergence,
  `"host on X, weft on Y"`) → `weft-sync`; a reason beginning `"junction "` (`"junction missing"`,
  `"junction points elsewhere"`) → `junction`. Any **unrecognized** reason → `weft-sync` as the
  safe default (it still surfaces the raw reason string to the operator, just under the more
  general sync `CheckID`). The mapping lives in `loomengine` and is unit-testable against the
  three literal reason strings. A cleaner long-term alternative — promoting `PairInSync` to
  return a typed reason enum — is deliberately **out of scope** here: it is a warp change with
  a wider blast radius (`warp pairs`/`reconcile` consume `PairInSync`), and prefix-matching the
  three pinned strings is sufficient for Preflight. If warp later adds a fourth reason, the
  unknown-reason default keeps Preflight correct (reported, never dropped).

### run-in-existing-or-prime-worktree

- **Decision:** Preflight does **not** special-case or reject the Prime/main worktree, and
  does **not** enforce that the worktree directory name matches the seed's `slug`. All four
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

### at-worktree-root

- **Decision:** Preflight requires `l.RelPath == "."` — it must be run from the **worktree
  root**, not a subdirectory. A `RelPath != "."` is a foundational `worktree-root` failure
  (checked in check 1, short-circuits like the rest of geometry).
- **Rationale:** loom's geometry is only self-consistent at the root. `l.LyxDir()` and the
  junction `HostLyxLinkHere()` are `Cwd`/`RelPath`-anchored (`Cwd/_lyx`), while the seed's
  canonical location and `PairInSync`'s branch check are worktree-root things. If Preflight ran
  from a subdirectory (`RelPath != "."`), check 3 would validate the *subdir's* `_lyx` junction
  while check 4 read the *root's* `status.json` — two different `_lyx` dirs, breaking both the
  "all checks agree on one worktree" guarantee and the "seed-unreadable → see check 3"
  attribution. Requiring `RelPath == "."` collapses `Cwd == WorktreeRoot`, so every `_lyx`-path
  in every check is the same directory. This matches how loom is actually launched (the
  `.lyx/lyxrun.cmd` run-launcher does `cd <worktree>` then `lyx loom run`), so the requirement
  costs real usage nothing and turns a silent wrong-directory mistake into a clear, early
  failure.
- **Not in tension with run-in-existing-or-prime-worktree.** "Which worktree" (Prime or a task
  worktree — supported uniformly) is orthogonal to "at that worktree's root" (required). Prime's
  own root has `RelPath == "."` too.
- **Rejected:** Silently walking up to the worktree root from a subdirectory — masks a
  wrong-directory invocation; a loud `worktree-root` failure is safer and matches the launcher's
  guarantee.

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

- **Decision:** Geometry (check 1) is foundational: if `Resolve` fails **or `RelPath != "."`**,
  return immediately with a single `geometry` / `worktree-root` failure (no usable, root-anchored
  `Layout` ⇒ no other check is meaningful — the `_lyx` paths would not agree; see
  at-worktree-root). All remaining checks (2 clean, 3 pairing/sync/junction, 4 seed/coherence)
  **run and collect all their failures** into `Report.Failures`. The one interpretation
  dependency: check 3's **outcome** informs check 4's error attribution — when check 3 reported a
  `junction` or `weft-pairing` failure, check 4 classifies *any* resulting seed-read failure
  (`IsNotExist` included) as `seed-unreadable` "see check 3", never `seed-missing`
  (see strict-read-mechanism → Error classification). The gate is check 3's outcome, not the
  stat error's shape, because the realistic broken-junction/missing-weft case surfaces as
  `IsNotExist` at the seed path. Check 4 still runs regardless; it just does not lie about *why*
  the read failed.
- **Rationale:** Maximal actionable information per run (fix everything in one pass), with only
  the one unavoidable short-circuit (geometry) and honest cross-check attribution.
- **Rejected:** Fully sequential short-circuit — forces fix-rerun-fix cycles.

## Technical context

What the plan needs about the codebase:

- **`internal/hubgeometry`** ([hubgeometry.go](../internal/hubgeometry/hubgeometry.go)) —
  `Getwd()`, `Resolve(cwd) (*Layout, error)` (returns `ErrNotAGitRepo`), `Layout` with
  `Cwd/WorktreeRoot/Hub/RelPath/Prime/Repo`. Existing weft-geometry methods to reuse:
  `LyxDir()` (host `_lyx`, **`Cwd`-anchored** — `filepath.Join(l.Cwd, LyxDirName)`,
  hubgeometry.go:319), `WeftWorktree()`, `WeftLyxDir()`, `HostLyxLinkHere()`
  (`WorktreeRoot/RelPath/_lyx`), `WeftRepoRoot()`, `PrimeName()`. **This task adds** a
  **`WorktreeRoot`-anchored** `LoomStatusFile()` accessor (`filepath.Join(l.WorktreeRoot,
  LyxDirName, "status.json")`) — deliberately not built on `LyxDir()`, whose `Cwd`-anchoring
  would misread the seed from a subdirectory (Hub Geometry Invariant). Note: the Layout
  construction spawns `git rev-parse` — cheap but a spawn (matters for the Test Tier Purity
  Invariant).
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
  (T, bool, error)` and `WriteJSON[T]`: locked, atomic, typed. **Note `ReadJSON` at `state.go:71`
  uses plain `json.Unmarshal` — it does NOT reject unknown fields**, and returns `(zero, false,
  nil)` on `os.IsNotExist`. This task adds a strict sibling `ReadJSONStrict[T]` (see Decisions →
  strict-read-mechanism) using `json.Decoder.DisallowUnknownFields()`; `loomengine` uses that,
  not `ReadJSON`. The `internal/state` primitive is still the schema doc's mandated read
  mechanism (same package as builder's `state.json`).
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
  `Preflight` contract, `Report`/`Failure`/`CheckID`, and the status.json type — and **must**
  carry the caller precondition from preflight-invocation-model ("only call at the fresh/preflight
  stage; calling on an advanced task is reported as `half-finished`, not diagnosed as misuse"),
  since that invariant is owned by the not-yet-built phase machine and would otherwise vanish when
  this discussion doc is deleted.

## Testing

Hybrid tiering (decision the-testing-tiers):

- **Untagged (Tier 1, fast, no spawn) — pure status.json coherence/parse:**
  - Valid seed → no failure.
  - Unknown field / malformed JSON → `seed-incoherent` (strict `DisallowUnknownFields`).
  - Missing/empty **mandatory string** (`slug`/`parent`/`phase`/`stage`/`narration`) →
    `seed-incoherent` (non-empty check; see field-presence-and-nullability). Absent
    nullable/bool fields (`start_sha`/`next_action`/`pause_requested`) are NOT flagged —
    absence ≡ their valid null/false value.
  - Bad enum (`phase`/`stage`/`outcome`) → `seed-incoherent`.
  - `state.ErrDecode` vs `state.ErrRead` classification: a decode error → `seed-incoherent`;
    a post-stat I/O read error → escalate as `error` (TDD candidate for `ReadJSONStrict`'s
    sentinel behaviour, testable in `internal/state`).
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
  - Invoked from a **subdirectory** of an otherwise-healthy worktree (`RelPath != "."`) →
    `worktree-root`, short-circuit (no other failures), proving check 1's root requirement
    (see at-worktree-root).
  - Host worktree dirty: tracked-modified, staged, **and untracked-only** → `worktree-clean`
    (untracked-only is the deliberate strict-policy case — TDD candidate).
  - Weft worktree missing entirely → `weft-pairing` ("not paired"), distinct from sync.
  - Host and weft on different branches → `weft-sync` ("host on X, weft on Y").
  - Junction missing / points elsewhere → `junction`.
  - Seed missing when the **junction is healthy** (`os.IsNotExist`, check 3 passed) →
    `seed-missing` (hard failure).
  - **Broken junction → seed read `IsNotExist` (the common case):** with check 3 reporting a
    `junction` failure, the seed stat also returns `IsNotExist`; assert check 4 classifies it
    `seed-unreadable` "see check 3", **not** `seed-missing` — the outcome-gated attribution, and
    the realistic broken-junction shape on Linux/Windows. Both the `junction` and `seed-unreadable`
    failures appear in the same `Report`.
  - **Missing weft → seed read `IsNotExist`:** with check 3 reporting `weft-pairing`, the seed
    stat `IsNotExist` is likewise classified `seed-unreadable` "see check 3", not `seed-missing`.
  - Seed unreadable via a non-`IsNotExist` stat error (permission-denied, `_lyx` is a plain file)
    while check 3 is healthy → `seed-unreadable`.
  - Multiple simultaneous failures (e.g. dirty + out-of-sync + incoherent seed) all appear in
    `Report.Failures` (collection behaviour).
  - Running in the **Prime** worktree with name ≠ slug but a healthy pair+seed → `Report.OK`
    (run-in-existing-or-prime-worktree).
- **`warpengine.HostClean`** gets its own focused integration tests (clean, tracked-dirty,
  untracked-only-dirty) if implemented there.

Avoid prescribing exact assertion shapes beyond the `CheckID` each scenario must yield — the
plan pins the concrete table-test structure.

## Q&A log

- **Q (external review r1):** Does the "seed-unreadable, see check 3" attribution actually fire
  for a real broken junction? **A:** Not as originally written — a broken junction / missing weft
  commonly makes the seed stat return `IsNotExist`, so shape-based classification mislabels it
  `seed-missing`. Fixed: check 4's classification is **gated on check 3's outcome** — if check 3
  reported `junction` or `weft-pairing`, any seed-read failure (`IsNotExist` included) →
  `seed-unreadable` "see check 3". See strict-read-mechanism → Error classification.
- **Q (external review r1):** "Five" vs "four" checks? **A:** Four — the no-half-finished
  condition is folded into check 4 as an internal coherence rule, not a standalone check. Renamed
  `the-four-checks` and reworded Scope; godoc must not say "five".
- **Q (external review r1):** Is the "only call at the fresh stage" invariant visible where #5
  (phase machine) wires the call? **A:** Made it a required `Preflight`/`loomengine` godoc
  deliverable (preflight-invocation-model + Documentation Lifecycle), since the discussion doc is
  deleted on landing and the invariant is owned by the not-yet-built phase machine.
- **Q (review round 4 gap):** The WorktreeRoot-anchored seed read disagrees with check 3's
  `Cwd`-anchored junction check under subdir invocation — how is that reconciled? **A:** Require
  `RelPath == "."` in check 1 (a `worktree-root` failure otherwise), so `Cwd == WorktreeRoot` and
  every `_lyx` path across all checks is the same directory. Matches the run-launcher's
  `cd <worktree>`. See at-worktree-root.
- **Q (review round 2 gap):** How does Preflight detect a *missing* required field, since
  `DisallowUnknownFields` won't? **A:** Scope presence-detection by field semantics: an explicit
  non-empty check on the mandatory strings (`slug`/`parent`/`phase`/`stage`/`narration`);
  nullable/bool fields (`start_sha`/`next_action` as `*string`, `pause_requested` as `bool`) are
  not presence-enforced because their zero value is a valid state the fresh-start invariant
  already pins. See field-presence-and-nullability.
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
  canonical Go type + strict-unknown-field parse implementing the schema doc's full
  validation checklist — Preflight is the schema's first code consumer.
- **Q (review round 1 gap):** How is the strict, fail-loud parse implemented, given
  `state.ReadJSON` does not reject unknown fields today? **A:** Add a new shared
  `state.ReadJSONStrict[T]` in `internal/state` (a reusable module helper, not local to
  `loomengine`) using `json.Decoder.DisallowUnknownFields()`; keep the lock/atomic behaviour;
  zero blast radius on existing `ReadJSON` callers. Classify errors via a preceding `os.Stat`
  (missing/unreadable) + strict-parse error (incoherent).
- **Q:** Should untracked files count as dirty? **A:** Yes — deliberately stricter than
  Millhouse; everything must be committed at spawn. New `warpengine.HostClean` uses bare
  `--porcelain`.
- **Q:** Read status.json via the weft path or the host junction? **A:** Host junction path via
  a new **WorktreeRoot-anchored** `LoomStatusFile()` accessor (not `Cwd`-anchored `LyxDir()`,
  which would misread from a subdirectory) — the junction abstraction must hold. Classify
  `os.IsNotExist` (seed missing) vs other errors (unreadable → "see check 3"); check 3 validates
  the junction first.
- **Q:** How to distinguish "weft not paired" from "weft out of sync"? **A:** `os.Stat` the
  weft worktree first for a clean "not paired"; only then call `PairInSync` for the sync/junction
  verdicts.
- **Q:** Is a missing seed a failure or an acceptable state? **A:** Hard failure — no seed means
  no task. Seeding is a separate future "claim/seed" command (the mill-claim analogue).
- **Q:** Reject running in Prime / require worktree name == slug? **A:** No. Support running a
  new task in an existing worktree, Prime included; the slug is a logical pointer, pairing is
  name-based (and equals `WeftRepoRoot()` at Prime), so this is the simpler design, not the
  harder one.
