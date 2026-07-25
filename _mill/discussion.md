# Discussion: webster: rewrite for flat card list

```yaml
task: 'webster: rewrite for flat card list'
slug: webster-rewrite
status: discussing
parent: main
```

## Problem

The shipped `webster` module (`internal/websterengine` + `internal/webstercli`) is a
fork-based orchestrator: one long-lived Master LLM session forks one implementer per unit of
work, driven through fat Go "bracket verbs" (`begin/await/record/recover-batch`). Today it
consumes **plan-format v2** — a batch-based plan with per-batch `## Scope`, `NN.C` compound card
numbering, oversized batches, and deferred-verify chains. Critically, webster has **no plan
parser of its own**: it imports `builderengine.ParsePlan`/`Validate`/`ParseReport`/`Distill` (the
v2 parser and drift model) directly from `builderengine`.

The plan format has moved on. `docs/reference/plan-format-v3.md` pins a **flat card-list** format:
no batches, no `## Scope`, flat `N` numbering, five typed file-op fields
(`Context/Edits/Creates/Deletes/Moves`), a required `Depends-on:` field, plan-level `root:`/`//`
path resolution, and a plan-level `## verify:` integration suite. `builder` — the older
cold-start-per-batch consumer — is obsolete as a plan consumer and slated for deletion (a
separate later task). This task rewrites webster's **plan-consumption layer** to consume the flat
card-list format, and cuts webster's dependency on `builder`. The core orchestration loop (Master
+ warm-fork + digest + state/resume + recovery) is largely reusable — this is a rewrite of *what
webster reads and how it schedules*, not a from-scratch rebuild.

**Why now:** `plan-format-v3` is done (the loom-planner already produces the flat card list via
`internal/loomengine`), so webster is the last consumer still stuck on the dying v2 format. Until
webster consumes the new format, the planner's output has no implementer.

**Naming note:** the reference doc calls the format "v3", but **no Go symbol, package, or module in
this task carries a version suffix**. The old format dies with builder, so the new one is simply
*the* plan format. "v3" appears only in prose references to the pinned spec doc.

## Scope

**In:**

- New package **`internal/planparser`** — parses `_lyx/plan/` (`00-overview.md` +
  `NN-<card-slug>.md`) into `Plan`/`Card` Go structs, normalizes card paths (`root:`/`//`
  resolution → plain worktree-relative), and runs the format's **14 validation checks** (listed in
  `plan-format-v3.md` §"Validation checks"). Replaces `builderengine.ParsePlan`/`Validate`.
- New package **`internal/batcher`** — a *library* of batchifiers behind a `Batcher` interface
  (`[]Card → []Batch`), a name-keyed registry, and a **config-selected** active batcher. Ships the
  **identity** batcher (one card → one batch) as the first registered implementation. Grouping
  batchifiers (future) use each card's `Depends-on:` + declared file-ops to group cards touching
  the same files into appropriately-sized batches; the identity batcher ignores them and emits one
  batch per card.
- Rewire `internal/websterengine` + `internal/webstercli` to: parse via `planparser`, batch via
  `batcher`, fork **one implementer per batch**, and verify commits via `internal/gitrepo`
  directly.
- New minimal **fork return contract**: `OK`/`FAILED` + resulting head Git SHA + list of files the
  fork changed that fall outside the batch's declared file-ops (deviation list). Always
  informational, never blocking on its own.
- **Per-card commit** inside a batch's fork; Master captures per-card SHAs for the resume trail and
  SHA-bisect.
- **Integration suite** as one dedicated final fork running the plan-level `## verify:` once, with
  SHA-bisect over captured per-card SHAs on failure, then human escalation.
- **Cut webster's import dependency on `builder`** (re-point/extract mechanism helpers; write
  webster-local replacements for the builder-specific plan bits).
- Delete v2-only machinery from webster: `## Scope`-based drift, oversized / `master_oversized`
  role, deferred-verify chains (`chain.go`, `--restart-chain`, `ChainStartSHAs`).
- Docs updated in the same commit (see **Constraints** → Documentation Lifecycle).

**Out:**

- **Deleting `builder`/`buildercli`/`builderengine`.** Explicitly a *separate later task*. Builder
  stays frozen and functional in-tree; this task only cuts webster's *import edge* to it. Do not
  remove the `lyx builder` command, do not touch builder's help-tree tests.
- **The DAG / symbol-field scheduler.** Cards run in strictly declared order in v0. The
  `if card.HasSymbolFields() { …DAG… } else { …declared order… }` seam is written, but only the
  else-branch is live. Symbol fields (`creates/edits/reads-symbols`) are absent from the format in
  v0 and depend on LSP/codeintel being up — future. Do not build Mechanism 1/2, SCC merging, or
  continuous DAG update.
- **`fabric`.** Unbuilt design-only module. Webster calls `internal/gitrepo` directly; when
  `fabric` lands later it becomes a transparent swap.
- **Grouping batchifiers.** v0 ships only the identity batcher. The interface, registry, and config
  selection exist so grouping batchifiers drop in later without a webster code change.
- **Proactive long-plan checkpoint/respawn** (token-threshold-triggered fresh Master session).
  Rely on the existing `state.json` crash/resume. Revisit once 40+-card runs actually hit
  auto-compact.
- **Auto-retry-with-stronger-model on fork failure.** v0 stops the plan and escalates to a human.
- **Parallel / worktree-per-card execution** (the retired `websterv2.md` design).

## Decisions

### plan-consumption-only-rewrite

- Decision: Rewrite only webster's plan-consumption + scheduling layer. Reuse the existing
  orchestration substrate (fork/bracket-verb mechanism, `state.json` persistence + resume,
  model-injection, fork-audit, recovery spawn, CLI `Command()`/`RunCLI` seam, and all mux/engine/
  starter test fakes) **as-is** — these are plan-format-agnostic.
- Rationale: The mechanism already ships and is tested hermetically. Re-deriving it would be waste
  and risk regressions. The delta is genuinely just parser + batcher + report/verify + removal of
  v2-only concepts.
- Rejected: From-scratch webster rewrite (throws away working, tested machinery).

### planparser-package

- Decision: A new `internal/planparser` package owns all plan parsing, path normalization, and the
  14 validation checks. Named `planparser` (no version suffix).
- Rationale: Parsing + validating the on-disk plan is one cohesive, independently-testable concern,
  and today it lives (as v2) in `builderengine` — the module being decoupled and eventually
  deleted. A standalone package keeps it reusable by a future non-webster consumer and lets the 14
  checks be table-tested in isolation.
- Rejected: Putting the parser inside `websterengine` (less reusable, harder to test standalone);
  extending `builderengine`'s v2 parser (builder is being decoupled and deleted; its parser is
  v2-only).

### batcher-library-config-selected

- Decision: `internal/batcher` is a **library** of batchifier implementations behind a `Batcher`
  interface (`[]Card → []Batch`), registered by name, with the **active batcher chosen via config**
  (a key in `webster.yaml`). v0 registers and defaults to the **identity** batcher (one card → one
  batch). Unknown batcher name in config is a load-time error.
- Rationale: Webster's execution unit is a *batch* = an ordered group of ≥1 cards, and "how to
  batch" is 100% webster's own execution-policy decision — never the plan's, never an LLM's
  (token-heavy). Making batching a config-selected strategy lets us add grouping batchifiers later
  and **A/B-test batchifiers back-to-back** without a code change. The identity batcher is not a
  "v0 version" — it is simply one entry in the library.
- Rationale (reconciliation of the two design docs): `webster-rewrite.md` says "fork per card,
  batch dropped"; `plan-format-v3.md` says "batching is a webster-internal execution-policy
  optimization… a later, measured, entirely internal decision." Both hold: the **plan** never has
  batches; webster **forks per batch**; in v0 the identity batcher makes batch ≡ card, so v0
  behavior *is* "fork per card." The seam for real grouping exists from day one — same philosophy
  as the dead `HasSymbolFields()` branch.
- Rejected: A single hard-coded identity mapping (no seam for future grouping / A/B testing);
  LLM-driven batching (token-heavy, non-deterministic); expressing batches in the plan format
  (violates plan-vs-schedule separation).

### fork-return-contract

- Decision: Each batch's fork implements its cards in declared order, runs unit tests
  (`go build ./...` + unit tests) after **each card**, **commits per card**, and returns a minimal
  report to Master: `OK` or `FAILED`, the **resulting head Git SHA**, and the **list of files it
  changed that were not in the batch's declared file-ops** (the deviation list — only the file
  paths, no commentary, no success narrative).
- Rationale: Minimal returns keep Master's context lean (Master "ingests deviation deltas, never
  success narratives"). The head SHA matches the manifest's `OK, SHA <x>` contract and anchors
  Master's SHA capture. A file-list deviation is **always informational, never blocking** —
  production experience shows plan-predicted impact is frequently incomplete; treating deviation as
  failure would make the system impractically brittle.
- Rejected: Fork reports a rich success narrative (context bloat); deviation treated as failure
  (brittle); fork omits the SHA and Master captures it purely from git (loses the manifest's stated
  contract and the fork's own cross-check — see next decision).

### per-card-commit-and-sha-capture

- Decision: Commit **per card** even when a batch holds several cards. Master captures each card's
  SHA by reading `git log` from the batch's start SHA to the fork-reported head SHA (via
  `internal/gitrepo`: `CurrentSHA`/`ChangedFilesSince`). In v0 (batch ≡ card) the head SHA is that
  card's SHA directly.
- Rationale: `plan-format-v3.md` makes "commit-per-card is THE resume mechanism" — a fresh session
  reads `git log` to see exactly which card the previous run reached. Per-card commits also keep
  SHA-bisect at card granularity even when batched. On `FAILED`, how far `git` advanced tells
  Master which card broke, so the minimal return stays sufficient.
- Rejected: One commit per batch (collapses the per-card resume/bisect trail v3 is built around).

### gitrepo-verification-substrate

- Decision: Post-commit contract verification calls `internal/gitrepo` directly
  (`ChangedFilesSince`, `CurrentSHA`, `SHAExists`, `SnapshotSHA`) — all already implemented and
  tested.
- Rationale: Every primitive exists today; no need to pull in the unbuilt, separately-designed
  `fabric` module. `fabric` is a future consolidation wrapper over exactly these `gitrepo`
  primitives, so a later swap is transparent.
- Rejected: Building a `fabric` wrapper as part of this task (drags an unbuilt, out-of-scope module
  into scope prematurely).

### declared-order-scheduler-with-dead-dag-seam

- Decision: Cards run in strictly declared order in v0. Write the scheduler with the conditional
  seam `if card.HasSymbolFields() { …DAG (dead) } else { …declared order (live) }`. `Depends-on:`
  is consumed only by the `depends-on-order` validation check, **not** for execution ordering.
- Rationale: Symbol fields don't exist in the format in v0 (they need planner-side LSP/codeintel),
  so `HasSymbolFields()` is always false and the DAG branch is dead code — intended. Writing the
  seam now turns the eventual codeintel rollout into "planner starts populating fields," not a
  future webster code change. `Depends-on:` is hallucination-safe (references only same-plan cards)
  so it powers the cheap pre-review `depends-on-order` gate, but v0 does not schedule from it.
- Rejected: Deriving a real DAG from `Depends-on:` in v0 (deviates from "strictly declared order";
  adds a scheduler + cycle handling ahead of need); omitting the seam (forces a future webster
  rewrite when codeintel lands).

### integration-suite-fork-with-bisect

- Decision: After all batches land, one dedicated fork runs the plan-level `## verify:` suite
  **once** (sequential; webster waits for it; no commit from this fork). On failure, **SHA-bisect**
  over the captured per-card SHAs (logarithmic re-runs) to localize the offending card, then
  escalate to a human. Webster writes a summary document (from accumulated OK/deviation notices)
  that becomes loom's merge-commit message.
- Rationale: Integration tests are the expensive, LLM/heavy tests — run once at the end, not
  per-card. Bisect over already-available per-card SHAs is cheap and localizes the failure without
  a linear rescan.
- Rejected: Per-card integration runs (expensive); deferring bisect and escalating the whole plan
  (loses automatic localization); running the suite in a separate worktree (out of scope in v0).

### decouple-from-builder-not-delete

- Decision: Cut webster's import dependency on `builder` without deleting builder. Re-point
  mechanism helpers that are thin wrappers to their underlying modules
  (`shuttleengine`/`muxengine`/`gitrepo`); extract genuinely reusable orchestration into clean
  modules where logical; write webster-local replacements for the builder-specific plan bits
  (`ParsePlan`/`Validate`/`ParseReport`/`Distill`/chain helpers) — which the rewrite replaces
  anyway. Exact per-helper disposition is a plan-phase determination.
- Rationale: The user requires webster not to depend on builder, but builder deletion is a separate
  later task; builder stays frozen and functional meanwhile. Preferring extraction into reusable
  modules (over burying logic webster-internally) is deliberate: a future *different* Webster is
  plausible, and shared orchestration should be reusable by it.
- Rejected: Keeping the `builderengine` import (violates the independence requirement); deleting
  builder now (out of scope, explodes the task); a single "utils" grab-bag package (unprincipled;
  leaves a one-consumer package after builder dies).

### keep-batch-cli-verbs

- Decision: Keep webster's existing CLI verbs unchanged: `run / validate / status / pause /
  begin-batch / await-batch / record-batch / recover-batch`. "batch" now denotes the
  webster-internal (batchifier-derived) batch.
- Rationale: Webster forks per batch, so the verb names remain accurate under the new model — no
  renaming needed. Preserves the `Command()`/`RunCLI` seam and help-tree pins.
- Rejected: Renaming batch→card verbs (inaccurate — a fork owns a batch, which may hold several
  cards under a future grouping batchifier).

### remove-v2-only-machinery

- Decision: Delete the v2-only concepts from webster: `## Scope`-based drift (`Distill`'s
  scope-prefix model), oversized batches + the `master_oversized` role, and deferred-verify chains
  (`chain.go`, `RestartChain`, the `--restart-chain` flag, `ChainStartSHAs`). State becomes
  batch-based over batchifier-derived batches, each `BatchState` referencing its member card(s) and
  per-card SHAs.
- Rationale: These concepts have no meaning in the flat card-list format (no plan-supplied batches,
  no Scope, no oversized escape hatch, no chains). Keeping them as dead scaffolding rots.
- Rejected: Keeping the chain/oversized scaffolding (no v3 meaning, pure dead weight).

## Technical context

Current state established during exploration (see `manifest/designs/webster-rewrite.md`,
`docs/reference/plan-format-v3.md`):

- **`internal/websterengine`** — has **no Go loop**; the Master LLM session drives fat Go "bracket
  verbs". Key files and roles:
  - `doc.go` — package overview (fork-per-batch model, bracket-verb discipline, builderengine reuse
    inventory, crash/resume). **Read first.** Durable parts of `webster-rewrite.md` fold in here on
    land.
  - `runlevel.go` — `Run` verb: run-level lease, validation gate, zero-batch refusal, plan-
    fingerprint crash/resume guard (`--fresh`), Master spawn (fork-authorized), fork audit. Calls
    `builderengine.ParsePlan`+`Validate` (`runlevel.go:326`/`:342`) → **retarget to `planparser`**.
    Defines `MasterHandle`/`MasterStarter` interfaces.
  - `beginbatch.go` — `BeginBatch` bracket verb: pause/fingerprint gates, start-SHA capture,
    per-batch model assert (only model-injection site), renders previous batch's digest into the
    fork prompt. Defines `Injector`.
  - `awaitbatch.go` — `AwaitBatch` bounded long-poll on the batch report path (forks are
    backgrounded on current Claude Code).
  - `recordbatch.go` — `RecordBatch` bracket verb: fork-audit + settle retry, batch-report parse
    (`builderengine.ParseReport` → **replace with the new minimal card/batch report parser**),
    digest persistence.
  - `recoverbatch.go` — cold-recovery strand (the only place webster spawns a *process*, reusing
    `builderengine.SpawnBatch` → retarget).
  - `chain.go` — `RestartChain` (v2 deferred-verify chains → **delete**).
  - `audit.go` — pure fail-loud fork-audit policy over `shuttleengine.ForkAudit`/`ForkReport`
    (**reuse as-is**).
  - `state.go` — durable `_lyx/webster/state.json`; `State`/`BatchState` (webster's own schema).
    Currently: `CurrentBatch`, `Batches map[int]*BatchState`, `ChainStartSHAs`, `SeenForkTranscripts`.
    `BatchState`: `Slug`, `StartSHA`, `Kind`, `SessionID`, `Terminal`, `Status`,
    `Digest *builderengine.Digest`, recovery fields. **Rework:** batches are batchifier-derived;
    add per-card SHA capture; drop `ChainStartSHAs`; replace `builderengine.Digest` with a
    webster-local digest type.
  - `config.go` / `roles.go` — `webster.yaml` config + roles (`master`, `master_oversized`,
    `recovery`). **Add** the batcher-selection config key; **drop** `master_oversized`.
  - `render.go` — `RenderForkPrompt`/`RenderMasterPrompt`/`RenderBatchIndex` + embedded
    `fork-template.md`/`master-template.md`. **Rewrite templates** for card list + per-batch cards +
    the new fork return contract; drop batch/scope language.
  - `summary.go` — `summary.md` prose-artifact contract (reuse; retarget the one `builderengine`
    helper).
- **`internal/webstercli`** — `Command()`/`RunCLI` seam (`cli.go:122`/`:267`). PersistentPreRunE
  wires `hubgeometry.Resolve` → shuttle/mux/webster cfg → roles → mux engine → claude engine →
  `shuttleengine.NewRunner`, and adapts three seams (`starter`, `injector`, `masterStarter`). Eight
  verb files. Paths anchored at `layout.Cwd` (`PlanDir`, `WebsterDir`, `WebsterReportsDir`,
  `WebsterPromptsDir`) via `internal/hubgeometry` — **all plan/webster/_lyx paths go through
  hubgeometry** (Hub Geometry Invariant).
- **`internal/gitrepo`** — verification substrate, all present & tested: `ChangedFilesSince`
  (`gitrepo.go:240`), `CurrentSHA` (`:73`), `SHAExists` (`:225`), `StageAndCommit` (`:110`),
  `SnapshotSHA`/`SetSnapshotSHA` (`snapshot.go`).
- **Fork mechanism** — normal path: Master forks in-session via the Agent tool
  (`subagent_type "fork"`), enabled by a fork-authorized spec at Master spawn; a fork-context
  `PreToolUse(Bash)` hook in `internal/shuttleengine/claudeengine` refuses `lyx webster` inside a
  fork. Recovery path: `internal/muxengine` (tmux strands) via `internal/shuttleengine` `Runner`.
  These are plan-format-agnostic — **reuse**.
- **v2 parser (reference for the rewrite)** — `internal/builderengine/plan.go`: `Plan`,
  `PlanBatch` (with `Scope`, `Oversized`, `Cards`), `PlanCard` (`NN.C`), `MovePair`,
  `overviewFrontmatter`, `batchFrontmatter` (`chain-end`). The v3 format removes `Scope`,
  `Oversized`, `chain-end`, compound numbering; adds the five typed file-op fields, `Depends-on:`,
  plan-level `root:`, plan-level `## verify:` / `## Rename mechanic` / `## Shared Decisions`.
- **loom producer** — `internal/loomengine` (`plan.go`/`plantemplate.go`/`plan-template.md`) already
  *writes* the flat card list (an LLM shuttle profile, not a Go parser). `planparser` is webster's
  *reader* of that same on-disk layout. `_lyx/plan/` is shared across format generations
  (`hubgeometry.go:226`).
- **Test fakes (reusable)** — `runlevel_test.go` defines `runFakeMux` (`shuttleengine.MuxOps`
  double), `runFakeEngine` (`shuttleengine.Engine` double), scripted `MasterStarter`/`MasterHandle`
  fakes; `Injector`/`Starter` faked per-verb; `webstercli` tests inject fakes into `websterCLI`
  fields directly. All mock at the mux/engine/starter seam, independent of plan format — **reuse for
  the rewrite**.

## Constraints

From `CONSTRAINTS.md` (hub root) — read and honor:

- **Hub Geometry Invariant** — `internal/hubgeometry` owns all cwd/geometry and `_lyx`/config
  paths. `planparser` must resolve `_lyx/plan/` via hubgeometry, never hard-code paths.
- **CLI / Cobra Invariant** — preserve the module `Command()`/`RunCLI` seam; every command keeps a
  `Short`; help-tree tests must pass. Keeping the existing verb names (decision
  *keep-batch-cli-verbs*) avoids help-tree churn.
- **lyxtest Leaf Invariant** — honor if tests touch it (verify against `CONSTRAINTS.md` when writing
  tests).
- **Documentation Lifecycle** (project CLAUDE.md `## Task completion`) — same-commit doc updates:
  fold the durable parts of `manifest/designs/webster-rewrite.md` into `internal/websterengine`'s
  `doc.go` and `docs/reference/builder-contract.md`; update `docs/overview.md` if the module table /
  execution stack changes; **delete `manifest/designs/webster-rewrite.md`** on land (it is a
  "Design — not built" doc that self-declares deletion once webster ships).
- **New cross-cutting invariants to record in `CONSTRAINTS.md` (same commit):**
  - `internal/planparser` is the **sole** parser of the on-disk plan format; no other package
    parses `_lyx/plan/`.
  - Batcher selection is **registry + config**-based (`internal/batcher`); webster's execution unit
    is the batchifier-derived batch, and no plan-supplied batching exists.
- **fslink** — not expected to be touched by this task; if any cross-OS link is needed, go through
  `internal/fslink` (directory links only).
- **Roadmap discipline** — this task completes the planned "webster: rewrite for flat card list"
  item; move it Planned→Done in `manifest/roadmap.md` with a pointer, and do **not** append bugfix/
  polish notes to the roadmap.

Discovered during discussion:

- Webster must not depend on `builder` after this task (import edge cut).
- `fabric`, `mux` (as a top-level module), `warp`, `weft` do not exist; `muxengine` and `gitrepo`
  do. Do not reference unbuilt modules.

## Testing

- **`internal/planparser`** (TDD candidate): table-driven tests for **each of the 14 validation
  checks** (`format-unrecognized`/`plan-unapproved`, `index-file-mismatch`, `card-path-malformed`,
  `move-format`, `move-redundant`, `move-source-missing`, `move-target-collision`,
  `move-mechanic-missing`, `card-missing-field`, `card-field-overlap`, `card-numbering`,
  `path-missing`, `commit-subject-mismatch`, `depends-on-order`), plus path normalization
  (`root:`/`//`/`root: "."`/malformed single-`/`/`..` escape), `none`-sentinel handling, and the
  `Moves:` grammar. Use the `plan-format-v3.md` worked example as a golden happy-path fixture.
  Hermetic, no LLM. Fixtures under `.scratch/` or a `testdata/` dir, never `/tmp`.
- **`internal/batcher`** (TDD candidate): the `Batcher` interface contract; the **identity** batcher
  (N cards → N single-card batches, order preserved); the registry (register/lookup, unknown-name
  error); config selection (valid name resolves, unknown name errors at load). Grouping batchifiers
  are out of scope — do not test what isn't built, but keep the interface test general enough that a
  future grouping batcher slots in.
- **`internal/websterengine`**: reuse the existing mux/engine/starter fakes. New/changed coverage:
  batches sourced from a fake batcher (identity); the new minimal card/batch **report parser**
  (`OK`/`FAILED` + head SHA + deviating-files list, including malformed/empty returns);
  **per-card SHA capture** from git log across a batch; **SHA-bisect** localization over a scripted
  set of per-card SHAs (fake git/report inputs, no live LLM); `state.json` round-trip with the
  reworked batch/card schema; deletion of chain/oversized paths (ensure removed code has no
  lingering test dependence). Fork-audit tests stay at the pure-fact level.
- **`internal/webstercli`**: verb smoke/behavior tests via direct fake injection into `websterCLI`
  fields (existing pattern); help-tree test stays green with the unchanged verb set.
- **Integration-suite fork**: unit-test the orchestration (trigger-once-after-all-batches, wait
  semantics, no-commit, bisect-on-failure, escalation) with fakes — do **not** run a live LLM
  integration fork in the hermetic suite. Physically separate any genuinely expensive/LLM tests
  from unit tests (different files/build tags) so Go's per-binary test cache does not force
  expensive re-runs on every change.
- Follow `golang:golang-testing` conventions.

## Q&A log

- **Q:** Delete `builder` in this task? **A:** No — cut webster's *import dependency* on builder
  only; builder stays frozen/functional in-tree. Deletion is a separate later task.
- **Q:** Should webster be made independent of builder now, given deletion is deferred? **A:** Yes —
  independence is required and forced (webster must not depend on builder); the only choice is
  *how*. Re-point thin wrappers to underlying modules and extract reusable orchestration into clean
  modules where logical (a future different Webster is plausible; keep shared functionality
  reusable).
- **Q:** Any "v3"/version suffix in module/symbol names? **A:** No — the old format dies with
  builder; the new one is simply *the* plan format. "v3" only in prose references to the spec doc.
- **Q:** Where does the plan parser live, and what's it named? **A:** New `internal/planparser` —
  it parses `_lyx/plan/` into `Plan`/`Card` and runs the 14 checks.
- **Q:** Verification substrate — `gitrepo` or build `fabric`? **A:** `gitrepo` directly; all
  primitives already exist and are tested. No `fabric` dependency.
- **Q:** Fork-per-card or fork-per-batch? **A:** Fork **per batch**. Webster's execution unit is a
  batch = ordered group of ≥1 cards. The plan itself never has batches.
- **Q:** How are batches produced? **A:** A `Batcher` interface + a **library** of batchifiers,
  registered by name, with the active one **chosen in config** — so more can be added and
  A/B-tested. v0 ships the **identity** batcher (one card → one batch); it is one library entry, not
  a "version".
- **Q:** Reconcile "batch dropped" (`webster-rewrite.md`) vs "batching is a later internal
  optimization" (`plan-format-v3.md`)? **A:** Plan has no batches (both agree); webster forks per
  batch; in v0 identity batcher makes batch ≡ card, so v0 *is* "fork per card". The grouping seam
  exists from day one — same as the dead `HasSymbolFields()` branch.
- **Q:** Commit granularity inside a multi-card batch? **A:** Commit **per card** (v3's "commit-per-
  card is THE resume mechanism"); Master reconstructs per-card SHAs from git log for the resume
  trail and bisect.
- **Q:** What does a fork return? **A:** Minimal: `OK`/`FAILED` + the resulting head Git SHA + the
  list of files it changed outside the card/batch's declared file-ops. Deviation is always
  informational, never blocking. (Refines the manifest's `OK, SHA <x>` + deviation note.)
- **Q:** Scheduler in v0? **A:** Strictly declared order; write the `HasSymbolFields()` seam with
  only the else-branch live. `Depends-on:` powers the `depends-on-order` validation check only, not
  execution ordering (symbol fields need LSP/codeintel — future).
- **Q:** Integration suite? **A:** One dedicated final fork runs the plan-level `## verify:` once;
  SHA-bisect over captured per-card SHAs on failure to localize the offending card, then escalate.
  No commit from that fork. Webster writes the summary doc → loom's merge-commit message.
- **Q:** Long-plan checkpoint/respawn? **A:** Deferred — rely on existing `state.json` resume.
- **Q:** CLI verb names? **A:** Keep `run/validate/status/pause/begin-batch/await-batch/record-batch/
  recover-batch` unchanged ("batch" = webster-internal batch).
- **Q:** Is most of the machinery already clarified because it exists in today's webster? **A:** Yes
  — the fork/bracket-verb mechanism, state/resume, model-injection, fork-audit, recovery, CLI seam,
  and test fakes are plan-format-agnostic and reused as-is; discussion.md scopes tightly to the
  delta.
