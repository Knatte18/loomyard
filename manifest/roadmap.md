# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as self-contained modules landed one at a time. See [docs/overview.md](../docs/overview.md#principles) for the design principles. This file is a numbered list of what's planned, what's committed-to- but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its doc under [designs/](designs/). See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next.

1. **fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo** — a major expansion of the former Someday item (promoted): `fabric` becomes the sole, deliberately-simplified git portal for everything LoomYard does, giving callers the illusion of one flat repo over the two underlying histories. Folds the separate `lyx init` phase into `lyx fabric clone` (create-or-adopt: the first clone records the lyx-anchor subpath into `weft` and wires all junctions; every later clone reads it — no cd-and-init step), makes the junction set config-driven (a template new weft-backed modules append to, replacing today's hardcoded list), unifies snapshot-tracking into the `Warp-SHA` trailer mechanism (`Fabric.Commit([files], msg, [snapshot-tags])` as the centerpiece), and takes on warp-rebase / remote-reconcile recovery (fabric detects + precomputes; an orchestrator spawns the LLM for genuine conflicts — the hardest part, but bounded because most of `weft` regenerates). Keeps warp ordinary git for humans and the "an LLM never decides weft-commit timing" invariant intact — now a deliberate policy rather than an accident of `git add`. Depends on the `board: move storage to weft:main` item (which removed `board-url` from clone; see Done below); see `fabric` in Done below for its current status. See [designs/fabric-unified-view.md](designs/fabric-unified-view.md).
1. **Shed: shared outer phase-FSM, combined with the Finalize step** — generalizes the phase-sequencing engine `loom.md` already specifies (sequencing, resume, crash-recovery, pause, status-file contract) into a shared skeleton with two swappable slots (Preflight, producer), reused by the Someday `Hardener` module, **built together with Finalize** (see [designs/finalize.md](designs/finalize.md) — merge-back, incl. the warp/weft split and the Raddle-only-forward pathspec) since Finalize is Shed's own literally-shared code, not a per-instance slot — one task, not two, same reasoning as the combined `Treadle`+`perch` item. **Testable cheaply:** plug a quick, throwaway producer into the producer-slot to exercise the skeleton + Finalize end-to-end before any real producer (Discussion/Plan/Webster, or the Someday `Tenter`) needs to exist — the same "fake phases before real producers" approach `loom.md` already specifies for its own skeleton. Does not rewrite `loom.md`'s existing design — records the shared-engine name and scope only. Independent of the landed `Treadle` engine (see the `internal/treadleengine` package documentation) — a different engine, was never blocked on it. See [designs/shed.md](designs/shed.md).

1. **loom: phase-machine skeleton + session bootstrap** — the status-file-driven engine (sequencing, resume, crash-recovery, pause), testable against fake phases before real producers are wired in, plus the `lyx loom run` entry point. Builds on `Shed` above. See [designs/loom.md](designs/loom.md).

1. **`PATTERN.md` — loomyard's own invariants mechanism, wired into every agent** — a from-scratch equivalent of Millhouse's `CONSTRAINTS.md`, owned by loomyard (which has no such mechanism today; the root `CONSTRAINTS.md` is Millhouse's, present only because mill develops loomyard). A weft-backed `_pattern/` folder whose invariants are injected as a pointer into every code-touching agent prompt. **The wiring has landed**: the `hubgeometry`/`fabricengine`/`initengine` junction plumbing, the `internal/pattern` active-check leaf, the `stencil` optional-marker extension, and the `{{.pattern_directive}}` marker in all five code-touching templates (builder implementer, burler round, webster fork, webster Master, loom plan) are all built and merged. **The content migration** out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs remains outstanding and still happens only at loomyard-init-via-lyx — `CONSTRAINTS.md` stays the single live invariants doc until that cutover. Also supersedes the constraints-hiding half of Someday's `host-visibility`. See [designs/pattern.md](designs/pattern.md).

1. **codeintel: LSP-backed code intelligence — V1 Go-only, built for multi-language** (promoted from Someday) — gives planner/implementer/reviewer fast, deterministic "where is this defined / used" lookups so they stop grepping blindly and stop paying an LLM round per false-positive hit; also what makes plan-format-v3's symbol fields trustworthy. lyx is an LSP **client**, never a server — it drives published language-server binaries (`gopls` first). Two consumer entry points on one engine: an in-process **Go API** (webster's DAG-derivation) and a **`lyx codeintel references|definition|symbol` CLI** for agents (**no MCP** — the fixed 2–3 query surface doesn't justify it, and a CLI is one code path + engine-neutral + fits the CLI/Cobra invariant). The lifecycle is one `EnsureServer(lang, worktree)` seam with two swappable spawn strategies behind it — `native` (`gopls -remote=auto`, gopls owns supervision) and `supervised` (our own state-file/auto-spawn/staleness/detached-spawn daemon, for `ty`/OmniSharp which have no native shared-daemon). **Independent of the rest of the Planned queue** (no dependency on board / native-clients / fabric / loom) — buildable now, in parallel. V1 populates the registry for Go only but locks its format for all three planned languages, and proves the `supervised` strategy by running it against a plain `gopls` so layer 2 is validated before any C#/Python dependency exists. See [designs/codeintel-redesign.md](designs/codeintel-redesign.md).

## Someday

Committed to eventually — will be done — but not scheduled next. No build order is implied between these items.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: cross-worktree columns** — all worktrees in one window, a column per worktree.

1. **reed: daemon → Slack relay** — standalone watchdog + bidirectional Slack relay per worktree.

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own switchable tmux window instead of a pane.

1. **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on a real Linux box (built and cross-compiled so far, never executed there).

1. **raddle** — codeguide's woven-in successor; parallel-regeneration design exists; deferred phase slot between Builder and Finalize. See [designs/raddle.md](designs/raddle.md).

1. **webster: parallel card execution** — worktree-per-card concurrent forking with a DAG; explored twice (pre- and during vacation discussion), rejected both times for git-index-race and mid-flight-visibility hazards. See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).

1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving real tmux) in a sandbox repo, on-demand and post-loom, off the `shuttle → burler → perch → loom` spine. Concept still being figured out. `Tenter` is the review-loop (`Treadle` configured for behavior-review, `perch`'s direct sibling); `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back, the same lifecycle `loom` uses). Both stay Someday — neither is needed to get `loom` running, unlike the Planned `Treadle`/`Shed`/perch-rewrite work they build on once scheduled. See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

1. **host-visibility: CLAUDE.local.md invisible in host's git history** — `CLAUDE.local.md` via symlink (with a Windows-Developer-Mode note and a copy fallback), so nothing lyx-related shows up in host's own git history. The `CONSTRAINTS.md`-equivalent half is **superseded by the Planned `PATTERN.md`** — it lives in `weft`, already invisible to the host repo, so no junction-to-hide-a-constraints-dir is needed; only `CLAUDE.local.md` remains. See [designs/host-visibility.md](designs/host-visibility.md).

1. **reed daemon: foreign-pane self-heal** — extends the **reed: daemon → Slack relay** item. Today reed is one-shot, so an operator-split or stray "faux" pane is only reaped on the *next* reed verb; the daemon could reconcile on its own. Prefer event-driven tmux hooks (`after-split-window`/`window-layout-changed`) over polling; gate behind a policy that distinguishes a bug-induced faux pane from an operator's intentional scratch pane. Prerequisite: make the reap probe cheaper first (it currently spawns a fresh pwsh + full `Win32_Process` WMI enumeration per poll).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B agent; cluster reviewers turned out to be fork subagents inside the handler's own session (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — today "provider" means whichever engine is wired into the `Runner`; a selector field is only needed once a second engine lands (non-Claude engines are not a current priority, per `CLAUDE.md`). Scope, if this is ever picked up: almost everything lyx spawns (Discussion, Planner, Webster, Burler rounds, the progress-judge) is markdown-instruction + file-contract driven — no skill/slash-command/plugin dependency baked into task content, unlike Millhouse's Claude-Code-specific skill layer — which is what makes a second engine a real, not aspirational, swap: it only has to solve spawn/completion-detection/ resume, not rewrite prompts. The one real trade-off to weigh, not just a gap to patch: Burler's cluster-review fan-out (N reviewers as cheap, context-sharing forks via Claude Code's own Agent tool) is a genuine strength, including token cost — a non-Claude engine has no equivalent to fork into, so cluster mode on a second engine would mean N full separate sessions instead, costlier by construction, not a like-for-like swap. Only Burler's default single-reviewer round (no clustering) is unaffected either way.

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob). Bulk is what makes provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts + embeddings + temporal decay), to find code by concept rather than literal keyword. The "deferred idea" `codeintel-redesign.md` already refers to. Genuinely speculative, not yet designed in depth. See [designs/semantic-index.md](designs/semantic-index.md).

1. **self-report: two-tier friction capture** — loom's per-phase file-contract design means no single LLM session has full-run context the way Millhouse's self-report assumes. Splits into Go-detected structural anomalies (crash-resumes, stuck escalations, repeated review rounds — off loom's own status/history, no LLM needed) plus a narrow per-phase friction note any spawned agent may write about its own scoped task, aggregated by Go and fed to one dedicated reflection agent at natural end points (Finalize/stuck) — mirroring the `Raddle` pattern. See [designs/self-report.md](designs/self-report.md).

1. **board: curation/triage automation** — the GitHub-issue-intake and periodic-triage workflow originally scoped in `designs/board-weft-storage.md`'s Curation flow section, deferred out of `board: move storage to weft:main`: an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via `promote-note` (which already ships as a plain mechanical CLI primitive — this item is the automation layer on top, not the primitive itself). See [designs/curation-triage.md](designs/curation-triage.md).

## Done

1. **fabric** — unified host↔weft git-coordination module replacing warp/weft; cut over and old modules deleted.

1. **git-native-library: feasibility spike** — empirical spike evaluating a native Go git library (`go-git`) as a replacement for `internal/gitexec`'s shell-out plumbing, across the full surface `gitrepo` uses (reads and writes, including the `Push` rebase-retry path). Recommendation: ADOPT-PARTIAL — the read surface, both commit methods, and `SetSnapshotSHA` migrate cleanly; the rebase-retry recovery on a rejected push stays CLI-BOUND because go-git ships no rebase implementation. This verdict was reached on Linux and explicitly provisional on a Windows run that had never happened; the `native clients` migration below ran that Windows evidence and narrowed it — the two commit methods turned out CLI-bound too, on a measured autocrlf blob divergence. The prototype this spike produced, `internal/gitnativepoc`, was deleted once the migration landed; its findings live on in `internal/gitrepo`'s package documentation instead.

1. **native clients: migrate `gitrepo` to `go-git` + `selfreportengine`'s `gh`-CLI transport to `go-github`** — landed a narrower scope than the `git-native-library` spike's ADOPT-PARTIAL recommendation: `internal/gitrepo`'s read surface (`CurrentSHA`, `SHAExists`, `ChangedFilesSince`, `CurrentBranch`, `remoteName`, `isStrictDescendant`, `SnapshotSHA`'s ref read, and `SetSnapshotSHA`'s two inline local reads) migrated to go-git; the two commit methods (`StageAndCommit`, `StageAllAndCommit`) and `SetSnapshotSHA`'s write stayed on the git CLI, on measured Windows evidence the spike never gathered — go-git performs no CRLF conversion at all, so a file it commits under `core.autocrlf=true` (the Git-for-Windows default) is thereafter permanently "modified" to CLI git. `Push`'s rebase-retry stays CLI-bound as the spike predicted, since go-git ships no rebase implementation. `hasUnpushed` was migrated to a go-git ancestry walk and then measured and reverted back to the CLI `rev-list` spawn, per card 21's reversal criterion. Separately, `selfreportengine`'s `CreateIssue` swapped its `gh`-CLI shell-out for `google/go-github`, authenticated through the new `internal/githubclient` leaf (token resolution, a 12-hour on-disk cache, and the sole owner of the Authorization header); `gh` survives only as a bounded, non-blocking fallback token source. `CreateIssue`'s signature and behavior, and every `gitrepo` caller (`fabric` included), are unaffected. Both new invariants — the GitHub Auth Invariant and the gitrepo Client Boundary Invariant — landed in `CONSTRAINTS.md` with guard tests in the same commits. See the `internal/gitrepo` and `internal/githubclient` package documentation for the full evidence record.

1. **board** — task tracker (storage model superseded by the `board: move storage to weft:main` item below).

1. **board: use `gitrepo` as its git operator** — board's detached sync (`internal/boardengine`) talks to git exclusively through a single `gitrepo.Repo` (`StageAllAndCommit` + `PushCoalesced`) under board's own write and push locks, replacing its former hand-rolled `gitexec` calls.

1. **board: move storage to `weft:main`** — replaces board's own separate remote repo with a reserved `weft:main` branch: a second weft worktree at `<hub>/_board` on the host's own unsuffixed default branch (never a separate clone, never `<branch>-weft`); board's git routes through `internal/fabricengine`'s `CommitWeftAt`/`PushWeftAt` (a new warp-untethered primitive) instead of a direct `gitrepo.Repo` handle, preserving board's existing detached-sync architecture unchanged; a new `notes.json` store for not-yet-claimable manifest entries, sharing `tasks.json`'s exact schema; a `promote-note` cross-store move command; a single generated `README.md` (replacing `Home.md`+`_Sidebar.md`) with Tasks/Done and Manifest sections; a `short_name` field and a 32-character slug length cap. See the `internal/boardengine` package documentation.

1. **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/state`.

1. **gitrepo** — generic, repo-agnostic git primitives (`StageAndCommit`, `Push`, `PushCoalesced`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/ `SetSnapshotSHA`), split across two backends — go-git for local object and ref reads, `internal/gitexec` for anything that authenticates to a remote or mutates the working tree (`internal/gitrepo`; consumed by the `fabric` module).

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
