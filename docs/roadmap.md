# Roadmap: Loomyard

Loomyard will, in time, **replace mill/millhouse (Python) entirely** — the Go
infrastructure becomes the orchestration layer, and the mill skills get reworked
and trimmed in the same move.

We get there by building **toolkits first**: small, self-contained modules with
deep internal tests, landed one at a time so the operator keeps full control at
every step. The toolkit layer (board, worktree, weft, ide, config) is largely
done. What remains splits into two tracks:

- **Setup track** — finish bootstrapping a hub: board-repo creation, `doctor`. (Config TUI,
  milestone 7, is done. `warp clone` handles the clone step — no standalone `ly-git-clone`.)
- **Orchestration stack** — the part that ties worktrees, the board, and tmux
  into a spawn→review→merge lifecycle. This used to be a single distant "endgame";
  it is now a **designed, layered path**: `proc → mux → shuttle → burler → perch → loom`
  (see the [execution stack](overview.md#execution-stack-orchestration-layers)
  and [modules/loom.md](modules/loom.md)). Each layer is its own shippable
  milestone; mill's existing Agent Dispatch handles orchestration until `loom`
  lands.

See [overview.md](overview.md#principles) for the design principles these
milestones follow.

## Build order

The dependency-ordered sequence — what is actually buildable next, respecting what each layer
needs below it. The numbered [Milestones](#milestones) below carry the detail; this is the
at-a-glance order.

**Done foundation:** board → shared infra (`configengine`/`git`/`lock`) → `state` → worktree + ide →
weft engine + producers → **`proc`** (cross-OS spawn). ✅

**Orchestration spine** — a strict chain, each layer needs the one before it:

```
proc ✅ ──▶ mux ✅ ──▶ shuttle ✅ ──┬▶ burler ✅ ──▶ perch ✅ ──┬▶ loom
                                   └▶ builder ✅ ───────────────┘
```

`builder` branched off `shuttle` (it spawns implementers directly, did **not** need `perch`);
`loom` joins the two — it needs both `perch` (review) and `builder` (implementation).

- **`mux`** is done — tmux overlay + **strand** bookkeeping + render sub-package
  (`internal/muxcli` + `internal/muxengine` + `internal/muxengine/render`; it absorbs what earlier
  drafts split into `shed`/`glance`). See [milestone 9](#orchestration-stack) / the
  `internal/muxengine` package documentation.
- **`shuttle`** is done — one LLM agent as an interactive tmux strand over the file contract,
  behind a swappable engine. See [milestone 10](#orchestration-stack) / the
  [overview module entry](overview.md#modules).
- **`burler` + `perch`** are both done — the former gate module `review` split into two:
  **`burler`** is one review+fix round (A-review + optional cluster → B-fix, no self-grading),
  and **`perch`** is the Go loop that runs burler rounds until `APPROVED`/`STUCK`
  (progress-judge + cap). See [milestone 11](#orchestration-stack) / the `internal/burlerengine`
  and `internal/perchengine` package documentation.
- **`builder`** (the batch-implementation module, carved out of loom's Builder phase) ✅ **Done.**
  See [builder-contract.md](reference/builder-contract.md): the six as-built verbs (`validate`/`run`/
  `spawn-batch`/`poll`/`status`/`pause`) over `internal/builderengine` + `internal/buildercli`,
  driven by a long-lived LLM orchestrator session against the pinned
  [plan format](reference/plan-format.md) (ordered batch list, **no DAG** — task-level parallelism
  via separate worktrees + `lyx run`, not intra-plan) and the accompanying
  [model-spec notation](reference/model-spec.md). It branched off `shuttle` and did not need
  `perch`. **`loom`** still needs both `perch` and `builder` and remains the **last** spine layer
  — the critical path to the orchestrator. `lyx loom status` (the 1-line view) ships as a loom
  subcommand, not a module.

**Setup track** — independent of the spine, interleave at any time: `init`/board-repo creation ·
`doctor`. (Config TUI, milestone 7, is done.)

**Deferred** — after `loom` works and only if wanted: mux daemon → Slack relay; session sync;
plugin packaging.

So the immediate front: **`loom`** (the phase machine, the last spine layer, now that
`builder` ✅ is done — see [builder-contract.md](reference/builder-contract.md)) — the last
remaining spine layer. The setup-track items (`init`/board-repo creation, `doctor`) remain
available to interleave at any time; neither blocks `loom`. **Milestone 26 (webster) can
proceed in parallel with the whole `loom-*` build order** (see milestone 12) — it's a new, separate
module, not a revision of the existing `builder`, so nothing about it blocks starting `loom-*` work;
only one `loom` piece (`loom-finalize`) depends on the prose summary artifact it adds.

## Milestones

Each milestone is independently shippable. Refactor milestones (2–4) are
**behaviour-preserving**: board's existing test suite is the guardrail, so nothing
observable changes until the new module that needs the extracted lib arrives.

1. **board** — the task tracker. ✅ **Done.** See the board module in
   [overview.md#modules](overview.md#modules).

2. **Extract shared infrastructure: `internal/configengine`, `internal/git`,
   `internal/lock`.** ✅ **Done.** See
   [shared-libs/](shared-libs/README.md).

3. **`internal/state`.** Generic locked typed JSON I/O primitive: `WriteJSON[T]` / `ReadJSON[T]` with
   exclusive/shared locking on `path + ".lock"` via `internal/lock` and atomic writes
   via atomic filesystem operations. No fixed schema — callers own the fields
   and file paths. Built test-first. ✅ **Done.** A generic locked-JSON helper, shipped as part
   of the shared infrastructure with no consumer yet.

4. **worktree module + portals, launchers, and ide module.** ✅ **Done.** Create / track / tear down
   git worktrees; manage container junctions and spawnable
   launchers; VS Code launcher with interactive menu; centralized path geometry
   in `internal/hubgeometry`. Consumes `internal/configengine` + `internal/git`; owns the **junction-aware teardown**
   sequence (the Windows locked-worktree hazard). The module is **stateless by design** — `lyx worktree list` is a thin
   `git worktree list` wrapper; there is no worktree registry. Introduces `internal/hubgeometry` as the sole geometry owner, banning
   raw `os.Getwd` and `git rev-parse --show-toplevel` outside `internal/hubgeometry` and `cmd/lyx/main.go`
   via `internal/hubgeometry/enforcement_test.go`. (Portals are present and working — a subdir-mirrored
   Hub view of each worktree's `_lyx/`; kept available, not slated for removal.)

5. **Task 006 — Weft engine.** ✅ **Done.** Path geometry for weft worktrees, paired host+weft spawn and teardown, `lyx weft` command (`status|commit|push|pull|sync`).
   Implements the canonical weft overlay model (host stays pristine, all lyx artifacts in companion weft repo).
   Weft directories are reached by direct sibling access; portals remain available as the cross-worktree status view.
   The weft **producers** (the `lyx worktree add` paired host+weft spawn) also landed.

6. **Hub-creator / clone.** ✅ **Done — absorbed by `warp` (milestone 20).** `lyx warp clone <host> <weft>` creates the Hub and clones host, weft, and board. The standalone `lyx git-clone` subcommand was never built; warp made it redundant.

7. **Task 008 — configuration TUI.** ✅ **Done.** `lyx config` and `lyx config <module>` — an
   interactive menu over the `_lyx/config/` YAML schema — plus `lyx config reconcile`
   (`internal/configcli` + `configengine` + `configreg` + `configsync`). Overlay-directory junction
   activation (`_raddle`), once mis-bundled into this task, is a **geometry** concern that belongs to
   the raddle nav-doc work — never part of the config TUI.

### Orchestration stack

The concrete path to the orchestrator, replacing the old single "endgame" milestone.
Each layer knows only the one below it; built bottom-up. See the
[execution stack](overview.md#execution-stack-orchestration-layers).

8. **`internal/proc`.** ✅ **Done.** Cross-OS windowless/detached process spawn — the OS
   base every higher layer launches through (build-tagged `proc_windows.go` / `proc_linux.go`;
   third member of the portability family after `fsx` and `fslink`).

9. **`internal/mux` — the window to the world.** ✅ **Done.** Three things in one, split across
   `internal/muxcli` + `internal/muxengine` + `internal/muxengine/render`: **overlay** over tmux
   (panes, send-keys/capture, env hygiene, native `--resume`, one named server per hub — orphan
   firewall — with one session per worktree); **strand bookkeeping** (each managed process is a
   strand — a `guid`-keyed record with name, worktree slug, parent, opaque `cmd`/`resumeCmd`,
   generic display spec — persisted to `.lyx/mux.json`); and a **render** sub-package
   (`internal/muxengine/render`, `Rules(strands, box) -> (layout, focus)` over a closed generic
   display vocabulary — a pure-function, golden-file test surface, heights fully derived). Callers
   hand it `{cmd, name, display}`; mux never learns a domain `type`. Scope: one terminal per
   worktree (cross-worktree columns deferred). It absorbs what earlier drafts split into
   `shed`/`glance`. CLI verbs: `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`.
   (see the `internal/muxengine` package documentation) **Built on what muxpoc proved** —
   clean-env boot, interactive claude, child-pane spawn, bottom-dominant layout, and resume after
   `kill-server`; muxpoc itself has since been deleted, its job done
   ([overview.md#modules](overview.md#modules)).

10. **`internal/shuttleengine` — one LLM agent via a swappable engine.** ✅ **Done.** Runs a single
    agent as an interactive tmux strand over the file contract; `Stop`-hook completion read off an
    events file classifies the run into `done`/`asking`/`died`/`timeout`; `PreToolUse` guardrails
    (deny in-process `Agent` always, `AskUserQuestion` too when autonomous). The **engine** seam
    isolates the provider (`internal/shuttleengine/claudeengine` is the only v1 engine; Gemini etc.
    later, not a priority) — `internal/shuttleengine` never imports it. Named `shuttle`, not
    `agent`, to avoid colliding with Claude's own agent vocabulary. Asks `mux.AddStrand` for its
    pane; CLI surface is `internal/shuttlecli` (`lyx shuttle run|interrupt|send`). The design doc
    was deleted on landing per the documentation lifecycle; durable parts live in the
    `internal/shuttleengine` package header and the [overview module entry](overview.md#modules).

11. **`burler` + `perch` — the review+fix round and the gate loop.** The former single `review`
    module, split in two. **`burler` (round worker):** ✅ **Done** — one review+fix round: A-review →
    B-fix, one agent, no self-grading, over the shuttle file contract; LLM-heavy, standalone,
    smoke-tested; builds on `shuttle`. See the `internal/burlerengine` package documentation. Cluster
    fan-out — ✅ **Done** via built-in fork subagents (`cluster-fan` names a fan from the seed-only
    `burler.yaml` lens/fan library; the handler explores, spawns N unnamed lens forks in one message
    + does its own holistic review, then consolidates — all inside step A) — see the
    `internal/burlerengine` package documentation for the as-built shape. **`perch` (`lyx perch run|pause`,
    gate loop):** ✅ **Done** — the Go loop that runs `burler` rounds until `APPROVED`/`STUCK` (plus an
    operational `PAUSED` exit) — loop-until-dry convergence, a milestone-capped `round_caps` ladder, a
    holistic verdict judge (superseding an earlier canonical-key/cycle-detection design — see the
    `internal/perchengine` package documentation), and a pluggable gate (`llm-verdict` | `command` |
    `both`); deterministic, fake-burler-tested; builds on `burler` (and `shuttle` directly for its own
    judge/triage calls). One engine serves discussion /
    plan / builder / ad-hoc review — the per-type difference is the profile (rubric + fasit), not the
    code; per-phase profiles live in `loom`, not in `perch.yaml`. **Independent of `loom`** (runs
    standalone); loom just uses perch between every phase. See the `internal/perchengine` package
    documentation.

12. **`loom` (`lyx loom run`, alias `lyx run`) — the phase machine.** The autonomous driver:
    Preflight → Discussion → Plan → Builder → Raddle → Finalize, each producing phase gated by a
    review, resume-from-disk via the `_lyx/` status file, yielding only at human boundaries (or
    never, in `--auto`). **This is the orchestrator that finally replaces mill/millhouse** — the
    top of the stack above, sitting on board + worktree + weft + the `mux → shuttle` layers.
    `lyx run` is the **session bootstrap**: ensure the worktree's tmux session, add the
    `lyx loom status` strand (1-line top pane), spawn the driver detached (`proc`), attach the
    terminal; a `.lyx/lyxrun.cmd` launcher makes it one click. The 1-line view ships as the
    `lyx loom status` subcommand (a strand), not a module. The **Builder** phase is carved into its
    own module (**`builder`**, `internal/builderengine`) — ✅ **Done**, see
    [builder-contract.md](reference/builder-contract.md) — a *sequential* batch-implementation
    loop (ordered batches, **no DAG**; parallelism is task-level via separate worktrees + `lyx run`,
    not intra-plan), driven by an LLM orchestrator over **distilled** batch-reports (never raw
    sub-agent prose — the mill-go bloat lesson). **Batches are bounded to fit an implementer's
    context window** (mill's 200k-Haiku/Sonnet pain; eased by Sonnet's 1M but not to be relied on):
    prefer many small batches over few large. Its plan-format contract is pinned in
    [plan-format.md](reference/plan-format.md). ([modules/loom.md](modules/loom.md))

    **Build order for the rest of loom** — too large for one task; decomposed into independently
    buildable/testable pieces, contracts before code:
    1. **Contracts first** ✅ **Done** (spec only, no code, review-gated like everything else — never
       hand-written outside the pipeline): the spawn/handover status schema
       ([status-schema.md](reference/status-schema.md) — the seed state of loom's own ongoing
       `_lyx/` status file — just a pointer, e.g. the board-task slug, plus loom's phase/round
       state; board already owns title/description durably, so nothing is duplicated) and
       [discussion-format.md](reference/discussion-format.md) (the `discussion.md` ↔ Plan contract;
       `plan-format.md` already exists as the precedent). `discussion.md` itself is expected to
       split into a distilled decision-record (what Plan reads) and a raw support log (what the
       Discussion-review gate reads, never Plan) — mirrors Builder's own "distilled digest, never
       raw prose" rule.
    2. **Preflight** (renamed from an earlier "Setup" label — it's a pure precondition/validity
       check, not worktree creation, which is `warp`'s job): geometry/cwd via
       `internal/hubgeometry`, clean worktree, weft paired and in sync, no half-finished prior run.
       No LLM involved; testable in complete isolation. ✅ **Done** — see `internal/loomengine`.
    3. **Discussion producer** — the one interactive phase, heavily reusing `mill-start`, auto-mode
       capable.
    4. **Plan producer** — autonomous, no inputs beyond `discussion.md`; mostly a well-instructed
       prompt/profile fed to `shuttle.Run`, heavily reusing `mill-plan`; no review logic of its own
       (that's `perch`/`burler`, entirely separate).
    5. **Phase-machine skeleton** — the status-file-driven engine itself (sequencing, resume,
       crash-recovery, pause); buildable/testable against fake phases before real producers are
       wired in.
    6. **Finalize** — vital, not deferred: the merge-back step after Builder-review is `APPROVED`,
       mostly wiring on top of the already-built `warp cleanup` mechanics.
    7. **Session bootstrap** — the `lyx loom run` entry point.

    **Raddle is the one deferred piece** — the phase machine reserves its slot (after Builder,
    before Finalize) from the start, but it isn't built in this first pass.

### Deferred mux enhancements

Layer in once the core stack works; not required for `loom` v1.

13. **Cross-worktree columns.** All worktrees in one window, a column per worktree — just a
    `worktree` strand field + a grouping rule on top of mux's strand model
    (see the `internal/muxengine` package documentation). Deferred only because
    one-terminal-per-worktree is the right starting scope.

14. **mux daemon.** Standalone watchdog process: detects a tmux crash via `cmd.Wait()`, recovers
    each strand by replaying its stored opaque `resumeCmd` (native `--resume` **works** for
    programmatically-driven Claude panes once the inherited Claude-Code parent-session env is
    stripped — see the `internal/muxengine` package documentation on resume; the
    capture journal is optional belt-and-suspenders, not the primary mechanism), mutual watchdog so
    both must die to go dark. See the `internal/muxengine` package documentation. **Proven in
    muxpoc, now built into mux's on-demand reconcile** ([overview.md#modules](overview.md#modules)).
    A possible further extension (foreign-pane self-heal) is not yet scoped — see
    [long-term-ideas.md](long-term-ideas.md).

15. **Slack relay.** Bidirectional, one channel per worktree, riding on the daemon.

    **See also milestone 24 (own-window strand anchoring)** — another deferred mux enhancement,
    numbered at the end to avoid renumbering the list; an independent mux enhancement, no longer a
    gate for `burler`'s cluster review (see milestone 24 below).

### Setup & supporting milestones

Independent of the orchestration stack; interleave as needed.

16. **`init` grows: create the board repo from scratch.** `warp clone` already handles the
    "clone existing board repo" case (it clones all three repos). What remains: when starting
    fresh with no existing board remote, `init` should create and initialise a board git repo
    locally (and optionally push it to a new remote). Today the board dir is auto-created on
    first write and made a git repo by hand.

17. **doctor.** A diagnostics command (`lyx doctor`): checks `_lyx/` is present, `*.yaml` parse and
    use known keys, the board repo is reachable, no stale lock files, the state file is readable —
    and prints remediation. Pure troubleshooting, no domain logic.

18. **session sync.** `lyx session push/pull` — copy Claude `.jsonl` transcripts across machines so
    `claude --resume` works elsewhere (sessions are not portable today). See the
    `internal/muxengine` package documentation on session files and portability.

19. **Claude Code plugin packaging.** Ship `lyx` as an installable Claude Code plugin, exactly as
    mill/millpy were, once the binary and module architecture are proven.

20. **`warp` — host↔weft-coordinated git topology.** ✅ **Done.** Consolidated the
    host↔weft mirror invariant into one module: coordinated checkout (switches host+weft
    together + re-points junctions — the correctness gap raw `git checkout` left),
    dual-worktree add/remove, clone, reconcile, and cleanup. **Replaced** `worktree`
    (milestone 4), **folded in** `git-clone` (milestone 6), and **renamed** `internal/git`
    → `internal/gitexec` (the thin leaf both `weft` and `warp` sit on). The config module
    `worktree` → `warp` (`_lyx/config/warp.yaml`). The design doc was deleted on landing
    per the documentation lifecycle; durable parts live in the `internal/warpengine` package
    header and the [warp module entry](overview.md#modules) in overview.md.

21. **Built-in CLI help — self-documenting modules & commands.** ✅ **Done.** Cobra refactor of
    `cmd/lyx` + every module's `RunCLI`: `lyx` lists all modules; `lyx <module>` lists subcommands;
    `lyx <module> <cmd> --help` gives per-command usage. Help text lives co-located with each
    command (no central stale table). Introduces `internal/clihelp` (exec + JSON help). A persistent
    `--json` flag on the root command offers machine-readable help output.

22. **`selfreport` — file LoomYard bugs as GitHub issues.** ✅ **Done.** `lyx selfreport create <title>`
    files a new issue on the `Knatte18/loomyard` GitHub repository via the `gh` CLI. The target repo
    is hardcoded (no config required), making the command trivially callable from any sandbox agent
    context. Supports `--body` (or `-` to read from stdin) and `--label`; defaults to the `bug`
    label when no label is supplied. Durable design lives in the `internal/selfreportengine` package header.

*(Milestone 23, `hardener`, moved to [long-term-ideas.md](long-term-ideas.md) — it's an unsettled
draft concept, not a scoped milestone; see [modules/hardener.md](modules/hardener.md) for the design
sketch.)*

24. **Own-window strand anchoring** *(deferred mux enhancement — grouped with 13–15 thematically,
    numbered here to avoid renumbering the list).* A new `display` anchor that spawns a strand into
    its **own switchable tmux window** rather than a pane in the worktree column — the piece mux is
    missing today (windows are not yet a display target; mux runs one window of panes per worktree).
    This **no longer gates `burler`'s cluster review**: cluster fan-out shipped via built-in fork
    subagents, which run in-session as background tasks inside the handler's own pane, needing no
    extra tmux window at all (see [milestone 11](#orchestration-stack) and
    the `internal/burlerengine` package documentation). This milestone's remaining cluster-adjacent
    value is narrower but still real: **live per-reviewer pane visibility** — today a fork's progress
    is only inspectable via its `subagents/*.jsonl` transcript on disk, not a watchable pane; an
    own-window anchor could still be picked up as a pure mux enhancement independent of the spine.
    Purely additive to mux's closed display vocabulary + render rules. Distinct from milestone 13
    (cross-worktree *columns*): that groups worktrees into columns of one window; this adds *windows*
    as a spawn target.

25. **Real-Linux validation for `lyx` on Linux.** 🚧 **Planned.** This task (Facilitate Linux
    support) built and cross-compiled the Linux seam from a Win11-only box with no Linux machine
    to execute against; batch 6's `TestCrossCompileLinux` gate proves the module compiles for
    Linux but nothing here has actually *run* on Linux. This milestone is that deferred execution
    pass, carried verbatim from the discussion's "Out" section:
    1. Run the sandbox smoke suite green on real Linux.
    2. Real tmux behavioral validation of every tmux edge-case assumption — silent split
       failure, dead-pane adoption, the `-l` leading-dash bug, empty-layout destruction, and
       async kill-server.
    3. Real `/proc` execution validation, including confirming the `serverProcessesOnSocket`
       `/proc/*/cmdline` match shape holds against a live tmux server (which may rewrite its
       title to `tmux: server` and drop the `-L` token from argv — load-bearing for Linux
       confirm-gone).

26. **webster — fork-based implementation module (né "Master Builder").** ✅ **Done.** See
    [builder-contract.md](reference/builder-contract.md#webster-the-fork-based-sibling) (the
    cross-module contract deltas) and the `internal/websterengine` package documentation (webster's
    own design). Graduated from
    [long-term-ideas.md](long-term-ideas.md). **Not a revision of the existing `builder`** — a new
    module built alongside it, so both can be A/B tested on the same plan before deciding anything.
    The spawn mechanism differs too much to revise in place: today's `builder` spawns each batch's
    implementer as its own separate mux/tmux strand (a new `shuttle.Run` process — see
    [builder-contract.md](reference/builder-contract.md)). **webster** instead reads
    the codebase and the overall implementation plan once in one long-lived session, then forks out
    one implementer per batch (sequential, same order as today) as **in-session Agent-tool forks** —
    the same mechanism `burler` validated for cluster review, applied to writing instead of
    reviewing; no new process, no new mux strand per batch. Note: `burler`'s existing fork-audit
    (`internal/burlerengine/cluster.go`) hard-bans any fork from writing/editing — the opposite of
    what an implementer fork must do — so webster needs its **own** audit policy (allow
    writes, still ban nested `Agent` calls), not a shared audit path with `burler`. Separates what's
    safe to inherit through every fork indefinitely (stable orientation: codebase structure,
    conventions, `CONSTRAINTS.md`, the plan itself) from what isn't (mutable file content — each
    fork does its own fresh read of the files its batch touches); cross-batch dependencies are
    bridged by a short distilled summary Master absorbs before forking a dependent batch, never a
    raw file re-read or raw sub-agent transcript — the same "mill-go bloat lesson" digest discipline
    used elsewhere. **Also includes a new prose summary artifact**: alongside the existing
    machine-readable `_lyx/builder/outcome.yaml` (`outcome`/`stuck_reason`/`batches_done`), the
    final action now also writes a human-readable summary (title + narrative of what was actually
    built, including deviations from the original task) — the future `loom-finalize` PR-text
    source, since a long-lived Builder session is the only party with full oversight of what
    actually shipped (often diverging from the original task description). **Kept
    contract-compatible with the existing `builder`** wherever possible (same `plan-format.md`
    input, compatible `outcome.yaml` + the new summary shape, same batch-report shape) so
    `loom-phase-machine`'s Builder-phase integration won't care which implementation runs
    underneath. Does **not** include the riskier parallel-batches-via-DAG extension — that remains
    speculative in [long-term-ideas.md](long-term-ideas.md) until this lands and looks worth the
    added complexity. Independent of the `loom-*` build order below — can proceed alongside all of
    it; only `loom-finalize` depends on the prose summary artifact this adds.

*(Burler's cluster fan-out — the once-deferred idea in this slot — shipped; see milestone 11 above.
Three other, still-unscheduled burler/shuttle ideas from this section moved to
[long-term-ideas.md](long-term-ideas.md).)*

## Explicitly out of scope

These stay in the Python/millpy domain and are **not** planned for `lyx`:

- millpy plumbing that a Go binary does not need: junctions/hardlinks/portals as a
  *config* concern, `PYTHONPATH`, venv setup, `MILL_PYTHON`. (Note: the worktree
  module *does* manage junctions as a teardown concern, but Loomyard never depends on them for
  its own layout.)
- The millpy wiki daemon and its socket/RPC infrastructure (Loomyard's board is
  one-shot and daemonless by design).
- Heuristic inference of home-file content shape and board-URL derivation. (Note: the deterministic weft→wiki URL rewrite (`.git`→`.wiki.git`) performed by `lyx warp clone` is **in** scope; only *heuristic* inference of board URLs stays out.)

## Maintenance

This roadmap is the shared reference for sequencing. When landing work:

- Move a milestone to ✅ **Done** with a link to its module doc when it ships.
- Add newly-discovered deferred work as a numbered milestone in the right place — but only once
  it's scoped enough to commit to. Unscheduled, speculative ideas that aren't there yet belong in
  [long-term-ideas.md](long-term-ideas.md), not here (see its Maintenance note for the reverse
  direction — graduating an idea into a milestone).
- Do **not** enumerate fine-grained sub-tasks here — those live in the board.
