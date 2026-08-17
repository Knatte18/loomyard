# Discussion: Make producer engines runnable without a lyx worktree

```yaml
task: Make producer engines runnable without a lyx worktree
slug: producer-standalone-capability
status: discussing
parent: standalone-producers
```

## Reviewer orientation — this file is a pointer, not the content

This is a **discovery/design task, not an implementation task**.
It ships no production Go.
Its entire deliverable is two files in this worktree, and the substance of the discussion lives in the first of them, not here:

- **[`manifest/designs/producers-standalone.md`](../manifest/designs/producers-standalone.md)** — the design document.
  Read this in full;
  it is the artefact under review.
- **[`manifest/roadmap.md`](../manifest/roadmap.md)** — four new Planned entries pointing at that doc.

Duplicating the design doc's content here would create exactly the drift the [Producer Pointer-Rule Invariant](../CONSTRAINTS.md#producer-pointer-rule-invariant) exists to prevent, so this file points and stops.
The sections below give only what is *not* in the design doc: what the operator decided during the session, and why the task's shape is what it is.

## Problem

`lyx burler run --profile p.yaml` cannot review an arbitrary directory — a "Models" folder, a downloaded repo — on a machine with no lyx hub, no Fabric, and no `_lyx/config/` seeding.
Every producer CLI dies in its `PersistentPreRunE` before reaching any producer logic.
The same blocks `lyx perch run` and, later, Webster.

**Why now:** `loom`'s Shed-based rebuild plugs Webster, Perch and Burler into a flat producer list as interchangeable `ShedProducer`s.
`internal/shedengine` and `internal/shedadapters` have both shipped, and the adapters already take fully-told absolute paths — but the engines *beneath* them still demand a `*lyxcwd.Location`, so nothing can yet wire an adapter without a real hub.
The producer layer is the last thing standing between the shipped adapter seam and a working producer list.

## Scope

**In:**

- A design document at `manifest/designs/producers-standalone.md` carrying the three-tier resolution model, the verified per-module evidence, the pinned decisions, and a ten-task decomposition in five waves — each task written extraction-ready.
- Four Planned entries in `manifest/roadmap.md`, granular enough that the parallel waves are visible from the roadmap alone.
- A full audit of `internal/scoutengine`/`internal/scoutcli`, which the operator added to scope.

**Out:**

- **Any production Go change.**
  Not one line.
  The ten tasks the design doc describes are spawned separately, later.
- **Any wiki write.**
  The design doc's task entries are written so that lifting one into the wiki is copy-paste, but that happens only when a task is about to be spawned — these decompositions shift during implementation, so committing them to the board early is waste.
- Changing what `internal/lyxcwd` owns, or relaxing the Cwd Resolution Invariant's gate.
- Any non-reed spawn path.

## Decisions

Every decision below is stated with full rationale and rejected alternatives in the design doc's own **Decisions** section;
what follows is the index, so a reviewer knows what to check there.

| Decision | Where |
|---|---|
| Engines take plain path strings, never a `*lyxcwd.Location`; the synthetic-`Location` alternative is rejected on the `reedengine` tmux-socket hazard | design doc, "told-geometry" |
| `configengine` gains a degrading `LoadOrTemplate` beside the strict `Load`, rather than one loader with a flag | design doc, "config" |
| Stencils are a told directory (`--stencils-dir`), bootstrapped via `stencilstore.Reconcile`, not a hub path and not embedded bytes | design doc, "stencils" |
| No additive twins — parallelism comes from wave scheduling against real file contention, not from a duplicated API | design doc, "no additive twins" |
| Scout is not migrated — it already works standalone, and its migration is optional task T9 | design doc, "scout is not migrated" |

Two decisions were made in-session and are worth restating here because they shaped the task's form rather than its content:

### deliverable-is-a-design-doc-not-a-plan

- **Decision:** produce the design doc and roadmap entries directly, and make this `discussion.md` a thin pointer at them.
- **Rationale:** the operator's call.
  The task was filed as discovery, and the natural home for a durable ten-task decomposition is `manifest/designs/`, which survives this worktree's teardown — not a `discussion.md` that exists to feed one `mill-plan` run.
  Making this file a pointer is also what puts the design doc under discussion-review, which is the point.
- **Rejected:** writing a full self-contained `discussion.md` and letting `mill-plan` derive a plan from it.
  That would produce one implementation plan for work that is explicitly ten separately-spawned tasks.

### roadmap-granularity-shows-parallelism

- **Decision:** four Planned roadmap entries rather than one umbrella entry, each naming its wave's task count and parallel-safety.
- **Rationale:** the operator's stated reason is throughput — running one task at a time is slow, and most of these tasks barely overlap.
  Making the waves visible from the roadmap is what lets that be acted on without opening the design doc.
- **Rejected:** one Planned entry (loses the parallelism signal);
  one entry per task (ten entries would violate `roadmap.md`'s own Maintenance rule that entries stay short and detail lives in the linked design doc).

## Technical context

The design doc carries the full evidence, including exact `file:line` citations for every claim.
The one thing a reviewer should know before reading it: **five of the originating discovery task's findings are stale or wrong against the current tree**, and the design doc has a dedicated "Corrections" section documenting each with evidence.
The two that would have misdirected the work are that `lyxcwd.Resolve` validates far less than claimed (it succeeds in any ordinary git repo run from its root), and that scout — suspected of the same problem — is in fact the repo's existing working precedent for standalone degradation.

The originating task's one surviving concrete finding, the `webstercli` → `loomengine` plan-directory coupling, is preserved as task T1.

## Constraints

Bound by, and in several cases requiring rewording in the tasks that touch them: the Cwd Resolution, Stencil Ownership, Pattern Leaf, Tokenvocab Leaf, Planparser Sole-Parser, Treadle Runner-Seam, Shed Producer-Seam, CLI/Cobra, Fabric Git, and Live-Substrate Spawn Observability invariants in [`CONSTRAINTS.md`](../CONSTRAINTS.md).
Each is named at the specific task that engages it in the design doc, rather than listed abstractly here.

Two repo-level rules bind this task itself: `manifest/roadmap.md` moves only on adding or completing a planned item (adding, here), and `roadmap.md`'s own Maintenance section caps entry length at a name plus one or two sentences.

## Testing

This task ships no production code, so there is nothing to unit-test.
Verification is:

- `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` — the [Markdown Link Integrity](../CONSTRAINTS.md#markdown-link-integrity) invariant resolves both the file part and the `#anchor` of every inline link.
  Its scan roots are `manifest/` and `docs/` only, so it covers the two changed `manifest/` files and **not** this file: `_mill/` is outside the walk entirely, exactly as `README.md`, `CLAUDE.md` and `internal/**/*.md` are.
  This file's own three relative links were hand-verified instead, which is the same review obligation every other unscanned `.md` in the repo carries.
- `go test ./...` — nothing should move, and a diff that touches only `manifest/` and `_mill/` proves it.

Each of the ten downstream tasks carries its own verification command in the design doc, and every one of them additionally rests on `go test ./...` from the worktree root.

## Q&A log

- **Q:** Is this task the "mapping" task the originating doc's process decision describes, and does it implement anything? **A:** Mapping only — no production Go. Produce the design doc(s) in `manifest/` plus a roadmap entry directly, and make `discussion.md` a thin pointer at them so the discussion-review agents review the design.
- **Q:** Which invocation gets traced end to end? **A:** `lyx burler run` from a bare non-git directory, as the full trace, with perch/webster/scout as deltas on top — it already bottoms out in `reedengine` and `fabricengine`, the deepest leaves, so the others add layers above rather than below it.
- **Q:** Is `scout` in scope? **A:** Yes, fully. Outcome: audited at full rigor and found already standalone; migration is optional task T9.
- **Q:** Does `lyxcwd.Resolve()` check that the repo is lyx-initialized with Fabric-wired worktrees? **A:** No — that was the operator's model of it, and it is wrong. `Resolve` succeeds in any git repo run from its root. The gate the operator described exists as `loomengine.Preflight`, which is what motivated the three-tier model and task T8.
- **Q:** How should the three hard-failing `LoadConfig`s degrade? **A:** Operator delegated the call. Chosen: `configengine.LoadOrTemplate`, after confirming `envsource.Build` has no `_lyx` requirement of its own.
- **Q:** Does a standalone stencils path just take one file in and one file out? **A:** A told *directory*, not one file — burler alone reads four named stencils, `pattern` a fifth, treadle four more. The operator's underlying point holds though: `stencilstore.Read` is a bare `os.ReadFile` with no parsing or validation, and all the hub-shaped machinery lives in `Reconcile`, which standalone skips.
- **Q:** Additive twins to unlock parallelism? **A:** Operator objects to two near-identical signatures side by side, accepting only if strictly necessary. On analysis they are not necessary — the real constraint is file contention concentrated in `burlercli/cli.go` and `perchcli/cli.go`, and wave scheduling gets 3/2/2/2/1-wide parallelism with no duplicated API. Twins recorded as a named escape hatch only.
- **Q:** Given scout already ships the synthetic-`Location` shape and it works, should told-geometry still be the target? **A:** Operator delegated the call. Chosen: yes — the deciding evidence is `reedengine/lock.go:43`, where a faked `HubPath` silently names the tmux socket, a failure no compiler or test would catch.
- **Q:** (review r1 gap) What names reed's tmux session and where does reed write logs in standalone mode, given neither a hub nor a worktree exists? **A:** A user-scoped state directory — `$XDG_STATE_HOME/lyx/<hash8-of-target-abspath>/`, `~/.local/state/lyx/` fallback — holds reed logs, burler scratch, and perch's runs/scratch; the session name is `<target basename>-<hash8>`. The target directory receives only what the profile explicitly names. Pinned as a table in the design doc's T6. This also settles where producer ephemeral state goes, and makes the Durable-vs-Ephemeral Invariant genuinely not engage (there is no `_lyx` for a `.lyx` sibling to mirror) rather than bent.
- **Q:** (review r1 gap) How is the standalone pre-run branch pinned against regression, given T6 named only a manual acceptance run? **A:** Two tiers, both required — an untagged unit test over an extracted "build the stack from told values" function, plus a `//go:build integration` test driving `RunCLIIn` from a non-repo temp dir. The extraction is what makes tier 1 possible at all: driving the real pre-run reaches `lyxcwd.Resolve`, which spawns git and would breach the Test Tier Purity Invariant.
- **Q:** (review r2 gap) Pinning `anchorRoot` to the target directory would still push `.lyx` into the reviewed folder via reed's `stateDir`, shuttle's `runDirRoot`, burler's scratch and perch's run dirs — contradicting the r1 answer. **A:** Split the two roots instead of enumerating directories: `worktreeRoot` = the target (the base caller-named profile paths resolve against, per `burlerengine/profile.go:59-66`), `anchorRoot` = `<state>`. Every `.lyx`/`_lyx`-derived path then relocates automatically, and the Durable-vs-Ephemeral Invariant is satisfied rather than dodged — the pair are ordinary siblings under `<state>`.
- **Q:** (review r2 gap) A per-invocation reed `socketKey` makes `ReedState`'s persisted `Socket`/`Session` unresumable, contradicting the promise that a re-run resumes its own state. **A:** Derive `socketKey` deterministically from the same `hash8` as the session name and state dir — one tmux server per target directory, resumable, no sibling-folder collision. Standalone reuses reed's existing server lifecycle with `<state>` playing the hub's role; `lyx reed down` is the existing teardown verb and T6 adds no new lifecycle concept.
- **Q:** (review r3 gap) `reedengine.New`'s proposed parameter list carries neither `RepoName` nor `HubPath`, but `Engine.HeaderText` renders the header pane from exactly those two `tokenvocab` tokens at every boot. **A:** Introduce `reedengine.Geometry`, the told-geometry struct the decision's own name promises — a positional list would reach five strings anyway. It carries `SocketKey`, `SessionName`, `AnchorRoot`, `LogsDir`, `RepoName`, `HubPath`, each with a pinned standalone value. `repo` = the target's basename, `hub` = `<state>`; both literally true rather than fictional.
- **Q:** (review r3 gap) Which directory do the three config loaders read from in standalone mode, and can an operator set machine-specific keys like reed's `tmux`/`shell` there? **A:** `baseDir` = `anchorRoot` = `<state>`, so operator config lives at `<state>/_lyx/config/` — supported, not unavailable. With T2's template fallback the directory is optional, and the command prints `<state>` so the path is findable rather than guessed.
- **Q:** (review r3) Telling `HubLogsDir` a hub path would yield `<state>/_board/.lyx/logs`, not the pinned `<state>/logs`. **A:** Tell reed its logs *directory* directly. `fabricengine.HubScratchDir` is `reedengine`'s only `internal/fabricengine` reference, so this removes that import outright — and with it the `treadleengine` → `shuttleengine` → `reedengine` → `fabricengine` transitive path the Treadle Runner-Seam Invariant currently has to acknowledge.
- **Q:** Should this task create the wiki task entries? **A:** No. Write each design-doc task entry extraction-ready instead, and create a wiki task only when it is certain that task is about to be spawned — experience is that these decompositions change during implementation.
