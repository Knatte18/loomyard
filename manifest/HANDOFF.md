Load skills `mill:conversation` and `mill:prose` before reading the rest of this document.

# Handoff

## Where you are

`/home/hanf/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `708b72775`.

`wts/webster-master-gate` is still on disk at `5e588effb` although its task landed — `mill:mill-cleanup` removes it.
No Monitor waits armed, no forks running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.

## The live thread: a baseline producer and a Webster gate

Agreed in discussion, written down nowhere else, and not yet a `manifest/designs/` doc.
Motivated by the user's drilling-simulator repo, where numerical drift is the thing to catch — not by lyx's own development.

The shape agreed:

- A `Capture-Baseline` producer row, shared by reference the way `preflightshed` and `landingshed` share theirs, placed **right after `Preflight`** so the planner can write against the current numerical signature. The cheaper late placement was rejected for that reason.
- The artifact goes in **`.lyx/baseline/`**, never `_lyx`. `.lyx` is the ephemeral, machine-bound, never-tracked sibling — and a numerical baseline is machine-bound by nature (floating point, BLAS, compiler flags), so committing one would make it cross-machine, which is wrong rather than merely expensive. Capture before the first edit, recompute after, compare, discard.
- **Two seams, not one**: `capture` (tree → artifact) and `compare` (before, after → verdict + findings, with per-channel tolerance). `plan.Verify` today is a command line whose exit code is the verdict; a numerical baseline can never be that. Modelling only "run a program, store output" ends in diffing floats as text.
- Both seams belong to the target repo, never to loomyard. Loomyard owns the row, the seam shapes, and the artifact location.
- The gate rides `shuttleengine.GateSpec`: on a failed verdict `wait.go` withholds finalization and re-prompts the live session with the drifted channels as findings, so the producer never hands control on.

Open within the thread: whether the compare runs once at Webster's exit or **per batch**.
Per batch localizes drift by construction and would make `websterengine`'s own `bisect` unnecessary for this class of failure, at the cost of recomputing the signature after every batch.

`#020 webster-master-gate` landed the seam this needs (`708b72775`); no validator name is registered, by design.

## Unclaimed

Read each brief through the wiki daemon client rather than trusting a summary here.

- **`#018 llm-driver-trust-dialog-hang`** — the brief now carries a `## Decided approach` section, so the approach is settled and the task is ready to spawn. Reproduction is the hard part and the brief says why.
- **`#019 crucible-batten-followup`** — the campaign's safety pass, never achieved. Its own lesson, per the brief: the last two rounds' yield came from looking OUTSIDE batten's packages.

## Open findings

**The "added forms, never widened signatures" decision has no definition site.**
It is cited six times — `burlerengine/engine.go:26`, `shedadapters/singlellm.go:40` and `:104`, `shuttleengine/attach.go:42`, `shuttleengine/run.go:260` and `:390` — and defined in none of `CONSTRAINTS.md`, `docs/`, or `manifest/`.
Only `singlellm.go:40` states the *condition* (widen unless the seam is shared by callers that will never use the parameter); the other five give the conclusion alone, which is the half a reader would copy where the condition does not hold.
The user rejected simply appending a section to `CONSTRAINTS.md` — accretion makes it long and badly written.
Unresolved: an alternative is to designate one existing site as the definition and have the other five name it, adding no new prose.

**Three `#017` review notes** (PR #261) were judged non-blocking at merge and nothing else records them:

- The PR summary claims the prelude also resolves the agent binary (`claude`). It does not — only `lyx`'s own directory is prepended; the `claude` half is a manual pre-condition check in `SANDBOX-SHUTTLE-SUITE.md`.
- The `Pane Binary Resolution` clause pins the dialect to `shell.ForGOOS()`, which is not the pane's real shell when an operator sets `LYX_REED_SHELL`. Pre-existing, and `Chain` joins with `;` so a rejected prelude never kills the launch line — but the assumption is now normative and the clause does not name the override.
- `Shell Mechanics Seam` was softened in passing (its method list is now "illustrative, not exhaustive") to make room for `ExportEnv`/`PrependPathEntry`/`Chain`.

## Rules in force

`manifest/` holds only unbuilt work: a design doc whose work has shipped is deleted, and a doc pinning a format shared between modules moves to `contracts/specs/`.
No tombstones — a dropped idea is removed, not recorded as dropped.

`plugins/scribe/skills/prose/SKILL.md` carries a **Never pin a count** section. It governs every doc and comment you write.

## Machine note (hanf/WSL2)

`GOPROXY=direct` is set machine-locally and should stay.
The route to `proxy.golang.org` stalls partway through any large object from this network — the `.zip` dies at exactly 147 456 B under HTTP/2 and hangs under HTTP/1.1, while github.com serves the same 803 268 bytes fine.
MTU was not the cause: identical truncation at 1400 and 1280.
`GOSUMDB` is untouched; `GOPRIVATE` is deliberately empty, since quarry is public.

A build failing on `github.com/Knatte18/quarry@v0.2.0` is this environment, never a code defect.

## Needs a decision

- The roadmap's two `shuttle Spec` items declare themselves unmotivated (*"stays unmotivated rather than blocked on anything"*, *"meaningless until a second engine lands"*). Under the rule above they belong in GitHub issues, not `manifest/`.
- A spec deployed by an earlier version stays on disk in target repos — `stencilstore` seeds and reconciles registered names and has no removal path, so a stale `loom-plan-card-format.md` may linger in one.
- [millhouse#1127](https://github.com/Knatte18/millhouse/issues/1127) asks `mill-setup` Phase 4.8 to retire the operator's target-blind `Bash(rm -rf:*)` deny rule for target-scoped ones. Until it lands, that rule still blocks scratch and fixture cleanup on every machine.
- [millhouse#1130](https://github.com/Knatte18/millhouse/issues/1130) is the ticket that started the baseline thread above. Nothing in it is loomyard's to fix: loomyard already localizes regressions inside the task's own commit range (`websterengine/integration.go`'s `bisect`) rather than checking out a parent. Its gap is the opposite one — loomyard captures no before-state at all, so it cannot tell a pre-existing failure from one the task caused.

## Suggested skills

- `mill:prose` + `mill:conversation` — load before writing anything.
- `mill:mill-status` / `mill:mill-inspect` — confirm task state before acting.
- `mill:mill-cleanup` — the stale `webster-master-gate` worktree.
- `mill:mill-spawn` — both unclaimed tasks need one; `CLAUDE.md` requires the user's explicit say-so before creating a worktree.
- `mill:mill-quick` — fits a task that is mechanical and compiler-checked, and `mill-config.yaml` has the `done_gate` it requires. It runs no reviewer at all, so verify the suite yourself and read the doc comments it wrote.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
