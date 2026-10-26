# PATTERN

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd, worktree root or anchor: only `internal/lyxcwd`, from the worktree's own weft-root `.lyx-anchor`; only `fabricengine` writes or git-reads it; engines get absolute paths, never import lyxcwd; hub containers and workspace files stay hub-reachable only. (test) — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-lyx-temp-root` — Creating temp file or dir: under `internal/lyxtemp`'s run dir, never `os.TempDir()`. (test) — [background](pattern/PATTERN-lyx-temp-root.md)
- `PATTERN-durable-vs-ephemeral-state` — Adding file: tracked under `_lyx`, never-tracked under `.lyx`, same subpath; only `internal/lyxdirs` names either, bar a `tools/` script's variable block. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-leaf-packages` — Importing into a leaf package: only closed import set. — [background](pattern/PATTERN-leaf-packages.md)

## CLI and config

- `PATTERN-cli-cobra` — Adding CLI module: a cobra subtree under one root in `cmd/lyx/main.go`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-cliwire-sole-wiring` — Wiring standalone CLI: `internal/cliwire` alone resolves target dir, repo root and state. (test) — [background](pattern/PATTERN-cliwire-sole-wiring.md)
- `PATTERN-config-layers` — Loading config: only `configengine` reads, stacks, locates and judges layers against the template; a caller adopts one of strict `Load` or `LoadOrTemplate`, bar `runprofile`; after a binary change, only `hubreconcile.Ensure` reconciles. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-sparse-hub-base` — Writing a hub base file, from any package: only the units the hub tuned, never a template fill; only `internal/hubreconcile`'s one-time strip drops a template-equal unit.
- `PATTERN-refusal-way-forward` — Adding webster, shed or loom refusal: message names the way forward and a test reaches it; no refusal, stencil or spec names a step agents' settings deny. (test) — [background](pattern/PATTERN-refusal-way-forward.md)

## Fabric and git

- `PATTERN-fabric-vocabulary` — Naming the wired composite: "fabric". (test) — [background](pattern/PATTERN-fabric-vocabulary.md)
- `PATTERN-fabric-git` — Running git on warp or weft: only `internal/fabricengine`, in-process; destroying only in its `destroy.go`, hub writes via a hub `os.Root`, store gc only in `housekeeping.go`. — [background](pattern/PATTERN-fabric-git.md)
- `PATTERN-gitrepo-client-boundary` — Running git: `gitexec.Run`/`runChecked`, raw only at pinned `//gitexec:raw` sites; in `internal/gitrepo`, local reads via `readGoGit` and `dotgit`; transients via `.git/info/exclude`, never `git add -f`. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-push-both-sides` — Pushing run records from Go: code branch goes along; a push refused after retry leaves a failed-push record and creel notice. (test) — [background](pattern/PATTERN-push-both-sides.md)
- `PATTERN-pair-teardown` — Tearing down a pair: only via `internal/pairteardown`; a producer creating or destroying a task worktree never runs inside it. (test) — [background](pattern/PATTERN-pair-teardown.md)
- `PATTERN-parent-lock` — Moving, merging into or landing on a local parent branch: only in a run's Finalize, under `internal/parentlock`'s lock for that parent.
- `PATTERN-owner-machine` — a continue pass dobby schedules into a run it already drives is not a resume and needs no origin verdict; the first pass of a run after a dobby start or a machine wake, every pass while a full pass is owed, and every hand pass, is a resume and gets one. — [background](pattern/PATTERN-owner-machine.md)
- `PATTERN-pair-parent` — Picking a new pair's parent for a board entry or reading an entry's owner through its parent: only `ParentBeam` and hubgeom's one `ParentOf`, never the acting worktree.
- `PATTERN-github-auth` — Calling GitHub: auth only via `internal/githubclient`, no other production package runs `gh`, and only `lyx selfreport create` files an issue. (test)

## Shed and loom

- `PATTERN-shed-recipe-registry` — Adding producer or recipe row: one `Constructor` map in `internal/shedrecipe`, bar `Implementer`'s; a row's value looked up by key, a side action a `Command` row, a config-off row its `entry_gate`. — [background](pattern/PATTERN-shed-recipe-registry.md)
- `PATTERN-shed-driver` — Driving a shed run: generic verb bodies only in `internal/shedverbs`, the step-until-stop loop only in `internal/sheddriver`, run paths, ids and `Seed` only in `internal/shedrun`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-shed-stop-chain` — A shed run's stop: rule table, tackler, arbiter, then parent, for every recipe; a blocked stop at a row the driver's arbiter seam marks as judged first goes rule table, arbiter, then parent; arbiter keys in shed.yaml with module overrides.
- `PATTERN-dobby-starts-runs` — Starting or driving a board task's run: its parent's dobby, from an empty status, in batten passes; a hand `lyx batten run` is one pass.
- `PATTERN-transient-stop` — Marking a failure transient: only `internal/shedengine`'s mark, translated by `internal/shedtransient` at a producer or step-bootstrap boundary; never on a gate verdict.
- `PATTERN-friction-capture` — Halting or crash-resuming a loom run, or refusing in webster: Go writes a note via `internal/friction`; none is archived before a reflection covers it. (test)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-gate-slot` — Running gate build, test or verify: in held `internal/gateslot` slots, verify via `verifytree.Verify`, a tier skipped only on `lyx gate test`'s green record; a gate and its self-check call one function. (test) — [background](pattern/PATTERN-gate-slot.md)
- `PATTERN-sole-parsers` — Parsing plan, discussion, summary, recipe or halt note, or classifying a plan ref: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-stencil-ownership` — Reading prompt or spec: at call time via `internal/stencilstore`, never embedded; an instruction file points at another producer's format contract, never restating it. — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-parent-directive` — Writing a spawned role's stencil: it renders the parent and edit directives; its skills and color go on the launch spec, never asked of the agent. (test) — [background](pattern/PATTERN-parent-directive.md)
- `PATTERN-shuttle-provider-seam` — Touching a provider or shuttle run: specifics only under `internal/shuttleengine/<engine>`, model-specs via `NewModelChoice`, a strand stopped via the stop verb, a negative finish after `allOutputFilesExist`. (test) — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-pane-text-injection` — Typing into an agent pane: reed's guarded paste-submit, an interrupt key only via shuttle's guards, a covered session held; the orch pane and creel journal only via creel. (test) — [background](pattern/PATTERN-pane-text-injection.md)
- `PATTERN-agent-name` — Forming, parsing or matching an agent name: only `internal/agentname`, a strand's full name formed once by reed; a spawned role is its module's constant.
- `PATTERN-wait-mark-display-only` — Marking a pane waiting: only reed's `SetWaitMark`; no Go decision reads `@lyx_wait` or `@lyx_wait_start`.
- `PATTERN-shell-guard-table` — Guarding a shell command or building a pane-shell string: only `internal/shellguard`'s table, every engine rendering every row, and `internal/shell`, importing only stdlib.

## Processes and waits

- `PATTERN-spawn-observability` — Starting OS process from `lyx`: log the spawn via `internal/logger`; logging in a loop or poll follows its Level policy. — [background](pattern/PATTERN-spawn-observability.md)
- `PATTERN-run-process-owner` — A run's processes: registered in `internal/runproc`, ended only via `runproc.End`. — [background](pattern/PATTERN-run-process-owner.md)
- `PATTERN-daemon-wakeups` — Unbounded wait wakes at most once a second and backs off while idle. (test) — [background](pattern/PATTERN-daemon-wakeups.md)

## Testing

- `PATTERN-test-speed` — Writing or running a test: via `lyx gate test` by gate key; one `tierN` tag, the lowest that fits; it pins behavior no test covers; injected clocks, `t.Parallel` by default. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing package: hermetic git and `tmuxkit.Main`; shared support in one `internal/testkit/<kit>/`; hub fixtures via `hubforge`. (test) — [background](pattern/PATTERN-test-isolation.md)
