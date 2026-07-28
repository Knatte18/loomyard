# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time. See [docs/overview.md](../docs/overview.md#principles) for the design principles. This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/). See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next.

1. **board: move storage to `weft:main`** — replaces board's own separate remote repo with a reserved `weft:main` branch (README.md rendering, JSON-backed Proposals/Manifest/Tasks/Done). Depends on `fabric`'s branch-naming enforcement (`<slug>-weft` uniformly), which is now live (`fabric` shipped Done below, old warp/weft modules deleted). See [designs/board-weft-storage.md](designs/board-weft-storage.md).

1. **Shed: shared outer phase-FSM, combined with the Finalize step** — generalizes the phase-sequencing engine `loom.md` already specifies (sequencing, resume, crash-recovery, pause, status-file contract) into a shared skeleton with two swappable slots (Preflight, producer), reused by the Someday `Hardener` module, **built together with Finalize** (see [designs/finalize.md](designs/finalize.md) — merge-back, incl. the warp/weft split and the Raddle-only-forward pathspec) since Finalize is Shed's own literally-shared code, not a per-instance slot — one task, not two, same reasoning as the combined `Treadle`+`perch` item. **Testable cheaply:** plug a quick, throwaway producer into the producer-slot to exercise the skeleton + Finalize end-to-end before any real producer (Discussion/Plan/Webster, or the Someday `Tenter`) needs to exist — the same "fake phases before real producers" approach `loom.md` already specifies for its own skeleton. Does not rewrite `loom.md`'s existing design — records the shared-engine name and scope only. Independent of the landed `Treadle` engine (see the `internal/treadleengine` package documentation) — a different engine, was never blocked on it. See [designs/shed.md](designs/shed.md).

1. **native clients: migrate `gitrepo` to `go-git` (ADOPT-PARTIAL) + `selfreportengine`'s internal `gh`-CLI transport to `go-github`** — executes the `git-native-library` spike's finding (read surface, both commit methods, and `SetSnapshotSHA` migrate cleanly; `Push`'s rebase-retry stays CLI-bound permanently — go-git has no rebase) into `internal/gitrepo`, and separately swaps only what's underneath `selfreportengine`'s public `CreateIssue` entry point — its `gh`-CLI shell-out — for `google/go-github`, for the same "stop parsing CLI output as an API" reason, on a much smaller, already-stable surface (no spike needed). `CreateIssue`'s signature/behavior and all its callers are unaffected. One task, since both are the same underlying cleanup. Sequenced ahead of `loom` even though `gitrepo`'s public surface is unchanged by the migration (callers, incl. `fabric`, are unaffected either way): building `loom`'s Finalize logic against the final, go-git-based `gitrepo` from the start avoids re-validating that logic later if the swap surfaces any subtle CLI-vs-library behavioral difference. See [designs/native-clients-migration.md](designs/native-clients-migration.md).

1. **loom: phase-machine skeleton + session bootstrap** — the status-file-driven engine (sequencing, resume, crash-recovery, pause), testable against fake phases before real producers are wired in, plus the `lyx loom run` entry point. Builds on `Shed` above. See [designs/loom.md](designs/loom.md).

1. **`PATTERN.md` — loomyard's own invariants mechanism, wired into every agent** — a from-scratch equivalent of Millhouse's `CONSTRAINTS.md`, owned by loomyard (which has no such mechanism today; the root `CONSTRAINTS.md` is Millhouse's, present only because mill develops loomyard). A weft-backed `_pattern/` folder whose invariants are injected as a pointer into every code-touching agent prompt. **The wiring has landed**: the `hubgeometry`/`fabricengine`/`initengine` junction plumbing, the `internal/pattern` active-check leaf, the `stencil` optional-marker extension, and the `{{.pattern_directive}}` marker in all five code-touching templates (builder implementer, burler round, webster fork, webster Master, loom plan) are all built and merged. **The content migration** out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs remains outstanding and still happens only at loomyard-init-via-lyx — `CONSTRAINTS.md` stays the single live invariants doc until that cutover. Also supersedes the constraints-hiding half of Someday's `host-visibility`. See [designs/pattern.md](designs/pattern.md).

## Someday

Committed to eventually — will be done — but not scheduled next. No build order is implied between these items.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: cross-worktree columns** — all worktrees in one window, a column per worktree.

1. **reed: daemon → Slack relay** — standalone watchdog + bidirectional Slack relay per worktree.

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.

1. **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on a real Linux box (built and cross-compiled so far, never executed there).

1. **codeintel** — full four-layer design (toolchain manager, daemon/supervisor, LSP client, language registry) exists; deprioritized until loom's first end-to-end run lands. See [designs/codeintel-redesign.md](designs/codeintel-redesign.md).

1. **raddle** — codeguide's woven-in successor; parallel-regeneration design exists; deferred phase slot between Builder and Finalize. See [designs/raddle.md](designs/raddle.md).

1. **fabric: unified-repo view** — extend the "junctions make weft look like part of the host repo" illusion all the way through `fabric`'s own API: a single auto-routing `Fabric.Commit`, a unified diff/status spanning both repos, and SHA-bookkeeping reuse — all while keeping the existing "an LLM never decides weft-commit timing" invariant intact, only enforced more consistently (a hard block, not a silent no-op). Several sub-questions still open. See [designs/fabric-unified-view.md](designs/fabric-unified-view.md).

1. **webster: parallel card execution** — worktree-per-card concurrent forking with a DAG; explored twice (pre- and during vacation discussion), rejected both times for git-index-race and mid-flight-visibility hazards. See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving real tmux) in a sandbox repo, on-demand and post-loom, off the `shuttle → burler → perch → loom` spine. Concept still being figured out. `Tenter` is the review-loop (`Treadle` configured for behavior-review, `perch`'s direct sibling); `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back, the same lifecycle `loom` uses). Both stay Someday — neither is needed to get `loom` running, unlike the Planned `Treadle`/`Shed`/perch-rewrite work they build on once scheduled. See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

1. **host-visibility: CLAUDE.local.md invisible in host's git history** — `CLAUDE.local.md` via symlink (with a Windows-Developer-Mode note and a copy fallback), so nothing lyx-related shows up in host's own git history. The `CONSTRAINTS.md`-equivalent half is **superseded by the Planned `PATTERN.md`** — it lives in `weft`, already invisible to the host repo, so no junction-to-hide-a-constraints-dir is needed; only `CLAUDE.local.md` remains. See [designs/host-visibility.md](designs/host-visibility.md).

1. **reed daemon: foreign-pane self-heal** — extends the **reed: daemon → Slack relay** item. Today reed is one-shot, so an operator-split or stray "faux" pane is only reaped on the *next* reed verb; the daemon could reconcile on its own. Prefer event-driven tmux hooks (`after-split-window`/`window-layout-changed`) over polling; gate behind a policy that distinguishes a bug-induced faux pane from an operator's intentional scratch pane. Prerequisite: make the reap probe cheaper first (it currently spawns a fresh pwsh + full `Win32_Process` WMI enumeration per poll).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B agent; cluster reviewers turned out to be fork subagents inside the handler's own session (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — today "provider" means whichever engine is wired into the `Runner`; a selector field is only needed once a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`). Scope, if this is ever picked up: almost everything lyx spawns (Discussion, Planner, Webster, Burler rounds, the progress-judge) is markdown-instruction + file-contract driven — no skill/slash-command/plugin dependency baked into task content, unlike Millhouse's Claude-Code-specific skill layer — which is what makes a second engine a real, not aspirational, swap: it only has to solve spawn/completion-detection/ resume, not rewrite prompts. The one real trade-off to weigh, not just a gap to patch: Burler's cluster-review fan-out (N reviewers as cheap, context-sharing forks via Claude Code's own Agent tool) is a genuine strength, including token cost — a non-Claude engine has no equivalent to fork into, so cluster mode on a second engine would mean N full separate sessions instead, costlier by construction, not a like-for-like swap. Only Burler's default single-reviewer round (no clustering) is unaffected either way.

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob). Bulk is what makes provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts + embeddings + temporal decay), to find code by concept rather than literal keyword. The "deferred idea" `codeintel-redesign.md` already refers to. Genuinely speculative, not yet designed in depth. See [designs/semantic-index.md](designs/semantic-index.md).

1. **self-report: two-tier friction capture** — loom's per-phase file-contract design means no single LLM session has full-run context the way Millhouse's self-report assumes. Splits into Go-detected structural anomalies (crash-resumes, stuck escalations, repeated review rounds — off loom's own status/history, no LLM needed) plus a narrow per-phase friction note any spawned agent may write about its own scoped task, aggregated by Go and fed to one dedicated reflection agent at natural end points (Finalize/stuck) — mirroring the `Raddle` pattern. See [designs/self-report.md](designs/self-report.md).

## Done

1. **fabric** — unified host↔weft git-coordination module replacing warp/weft; cut over and old modules deleted.

1. **git-native-library: feasibility spike** — empirical spike evaluating a native Go git library (`go-git`) as a replacement for `internal/gitexec`'s shell-out plumbing, across the full surface `gitrepo` uses (reads and writes, including the `Push` rebase-retry path). Recommendation: ADOPT-PARTIAL — the read surface, both commit methods, and `SetSnapshotSHA` migrate cleanly; the rebase-retry recovery on a rejected push stays CLI-BOUND because go-git ships no rebase implementation. The kept prototype and its findings write-up live in `internal/gitnativepoc/doc.go`; no consumer was migrated.

1. **board** — task tracker (storage model superseded by the Planned `board` item once it ships).

1. **board: use `gitrepo` as its git operator** — board's detached sync (`internal/boardengine`) talks to git exclusively through a single `gitrepo.Repo` (`StageAllAndCommit` + `PushCoalesced`) under board's own write and push locks, replacing its former hand-rolled `gitexec` calls.

1. **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/state`.

1. **gitrepo** — generic, repo-agnostic git primitives (`StageAndCommit`, `Push`, `PushCoalesced`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/ `SetSnapshotSHA`) built on `internal/gitexec` (`internal/gitrepo`; consumed by the `fabric` module).

1. **worktree + ide** — worktree/portal management, VS Code launcher (worktree itself superseded by `warp`).

1. **weft** — companion weft repo, paired host+weft spawn/teardown (superseded by the `fabric` module).

1. **config TUI** — `lyx config` interactive menu + `reconcile`.

1. **warp** — host↔weft-coordinated git topology (clone, add/remove, checkout, reconcile, cleanup) (superseded by the `fabric` module).

1. **proc** — cross-OS process spawn.

1. **reed** — tmux overlay + strand bookkeeping + render (renamed from `mux`, no behavior change).

1. **shuttle** — run one LLM agent as an interactive tmux strand over a swappable engine.

1. **burler** — one review+fix round (A-review → B-fix).

1. **perch** — the gate loop: run `burler` rounds until `APPROVED`/`STUCK`.

1. **builder** — batch-implementation loop over a pinned plan (sequential, one strand per batch) — superseded as an active plan-format consumer now that `webster`'s flat-card-list rewrite has shipped; stays frozen and functional in-tree, with deletion tracked as a separate later task.

1. **webster: rewrite for flat card list** — fork-per-card unchanged; no DAG/SCC in v0 (a dead `HasSymbolFields()` scheduler branch is reserved for later); consumes the flat card-list plan format via `internal/planparser` (sole parser) and `internal/batcher` (config-selected batchifier registry); integration suite runs as one final fork with SHA-bisect on failure. `builder` becomes obsolete as a plan-format consumer. See the `internal/websterengine` package documentation.

1. **plan-format v3: flat card list** — a card carries `What:`, the five typed file-op fields (`Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`), and `Depends-on:` only; symbol fields wait for `codeintel`. Coexists with the still-live [plan-format v2](../docs/reference/plan-format.md) — still used by the frozen `builder` — until `builder` is deleted. See [docs/reference/plan-format-v3.md](../docs/reference/plan-format-v3.md).

1. **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.

1. **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).

1. **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far (loom as a whole is not done — see the Planned `loom` item).

1. **loom: Planner producer** — reads the discussion decision-record and writes a plan-format-v3 flat-card plan; a prompt/profile fed to `shuttle.Run` (not a module), the `PlanSpec(...)` factory + `plan-template.md` in `internal/loomengine`. No review logic of its own.

1. **dev/test `lyx.exe` separated from production deploy** — a second deploy target (`deploy-dev`/`deploy-dev.cmd`) so review/sandbox tooling never overwrites the stable production binary with an in-progress test build. See CONSTRAINTS.md's Dev/Prod Binary Separation invariant.

1. **Treadle: shared round-loop engine, combined with the `perch` rewrite** — generalized `perch`'s existing judge/gate/round-spawn/cap/pause/lock loop into `internal/treadleengine`, a shared engine with a pluggable `RoundRunner` seam (`internal/perchengine`'s burler adapter is the reference consumer; a live-substrate agent for the Someday `Tenter` is a future second one) and a judge-maintained handoff that bounds the progress judge's read-set — an efficiency fix to `perch`'s own shipped behavior, not just a `Tenter` need. `perch` was rewritten onto it in the same task, behavior/CLI unchanged from the outside: `internal/perchengine` is now the thin configuration layer that resolves `perch.yaml`/profile data and adapts `burlerengine` onto treadle's `RoundRunner` seam. Renamed from the discussion-time placeholder `gorch`. See the `internal/treadleengine` package documentation.

## Maintenance

- **Numbering is automatic, not manual, and restarts at 1 in each section.** Every item is written literally as `1.` in the source — GitHub/CommonMark renders ordered-list items sequentially from the first item in a contiguous list block regardless of the literal digit on the rest, and a new `##` heading starts a new block. So Planned, Someday, and Done each render as their own 1, 2, 3, … with **zero number edits ever needed** — inserting, removing, or reordering items anywhere just works.
- **Numbers are not stable cross-reference IDs** (the same number exists in all three sections). Cross-reference by **bold item name** instead (e.g. "the Planned `board` item," "Someday's `codeintel` item") — every reference elsewhere in this file and in `designs/*.md` already does this.
- Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when it ships — no renumbering needed anywhere.
- Delete a module's doc under `designs/` once it ships (see the [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — that's why Done entries above don't link anywhere.
- Someday items get a `designs/<name>.md` doc when there's real design behind them (`codeintel`, `raddle`, `webster: parallel card execution`, `hardener`, `host-visibility`, `semantic-index` above do); trivial ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to (`codeintel`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no separate long-term-ideas file. Add new speculative ideas directly to Someday.
