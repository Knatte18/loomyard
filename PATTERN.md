# PATTERN

## Code

- `PATTERN-code-conventions` — Writing code: a package's `doc.go` documents only that package; a name or path that can go stale is one named constant; no language, engine or branch is hard-coded. — [background](pattern/PATTERN-code-conventions.md)

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd, worktree root or anchor: only `lymcwd`, from the worktree's weft-root `.lym-anchor`; only `fabricengine` writes or git-reads it; engines get absolute paths, never import lymcwd; hub containers and workspace files stay hub-reachable only. (test) — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-lym-files` — Placing a file, temp dir, lock or link: tracked under `_lym`, never-tracked under `.lym`, named only by `lymdirs`; temp under `lymtemp`, lock paths via `lock`, links via `fslink`. (test) — [background](pattern/PATTERN-lym-files.md)
- `PATTERN-leaf-packages` — Importing into a leaf package: only closed import set. — [background](pattern/PATTERN-leaf-packages.md)

## CLI and config

- `PATTERN-cli-cobra` — Adding or wiring a CLI module: a cobra subtree under one root in `cmd/lym/main.go`; standalone wiring only through `internal/cliwire`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-config-layers` — Loading, writing or changing the meaning of config: layers only through `configengine`; a hub base file holds only tuned units; a meaning change ships a reinterpretation `hubreconcile` applies. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-refusal-way-forward` — Adding webster, shed or loom refusal: message names the way forward and a test reaches it; no refusal, stencil or spec names a step agents' settings deny. (test) — [background](pattern/PATTERN-refusal-way-forward.md)

## Fabric and git

- `PATTERN-fabric-boundary` — Changing fabric or running git on warp or weft: only `internal/fabricengine`, called "fabric", which learns nothing of IDEs, shuttle, the board or heddle. (test) — [background](pattern/PATTERN-fabric-boundary.md)
- `PATTERN-gitrepo-client-boundary` — Running git or writing remote-tracking refs: through `gitexec` and `gitrepo`, raw only at pinned sites; fetch, pull and push under the remote-refs lock. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-pair-remote` — Pushing run records or tearing down a pair: the code branch goes along; a pair's remote steps hold the slug's teardown lock; teardown only via `internal/pairteardown`. (test) — [background](pattern/PATTERN-pair-remote.md)
- `PATTERN-owner-machine` — Starting, resuming, pushing or landing a run, or picking a pair's parent: only on the entry's owner machine, read through hubgeom's one `ParentOf`. — [background](pattern/PATTERN-owner-machine.md)
- `PATTERN-landing` — Landing on a local parent branch or deciding on a pull request: only in a run's Finalize under `internal/parentlock`'s lock; landingshed's one resolver decides the pull request. — [background](pattern/PATTERN-landing.md)
- `PATTERN-github-auth` — Calling GitHub: auth only via internal/githubclient; no other production package runs gh; only lym selfreport create files an issue; only PR-Review, Finalize, `lym beam pr` and pair teardown open, reopen, merge or close a pull request; `lym beam pr` never merges. (test)

## Shed and loom

- `PATTERN-shed-recipe-registry` — Adding a producer, recipe row, block gate or loom recipe: one `shedrecipe` map, row values by key; a loom recipe declares `contract: loom`. — [background](pattern/PATTERN-shed-recipe-registry.md)
- `PATTERN-shed-driver` — Driving or stopping a shed run: verbs in `shedverbs`, the loop in `sheddriver`, paths in `shedrun`, shared machinery in shed; one stop chain; transient marks only via `shedengine`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-dobby-runs` — Starting or holding a board task's run: only its parent's dobby, started with its orch; front tasks only via `fronttask`; an adopt hold lifts only on a green adopt check. — [background](pattern/PATTERN-dobby-runs.md)
- `PATTERN-run-records` — Writing a run's friction note, usage record or project record: only its one writer, `internal/friction`, shed's usage writer or `lym loom decision add`. (test) — [background](pattern/PATTERN-run-records.md)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-gate-slot` — Running or clearing a gate: in held `internal/gateslot` slots, verify via `verifytree.Verify`; a cold fixer only via `gatefixer`, judged by Go's re-run. (test) — [background](pattern/PATTERN-gate-slot.md)
- `PATTERN-sole-parsers` — Parsing an on-disk format its background lists, or classifying a plan ref: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-stencil-ownership` — Reading or writing a prompt, spec or stencil: read at call time via `stencilstore`; engine text in engine blocks; a spawned role's stencil renders the parent and edit directives. (test) — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-heddle-instructions` — Rendering or writing agent instructions: heddle entries only via `heddleengine`; lym's own instructions in its code, stencils, specs or heddle's shipped layer. — [background](pattern/PATTERN-heddle-instructions.md)
- `PATTERN-reed-panes` — Spawning an agent or touching its pane: an interactive tmux session through reed, never headless; text typed only via guarded paths; the wait mark is display only. (test) — [background](pattern/PATTERN-reed-panes.md)
- `PATTERN-agent-addressing` — Naming or messaging an agent: names only via `internal/agentname`; messages only through creel; Slack only via `internal/slackengine`, its token only in its machine credential file; operator Slack text reaches agents only as creel mail from `<shortname>:SLACK`. (test) — [background](pattern/PATTERN-agent-addressing.md)
- `PATTERN-shuttle-provider-seam` — Touching a provider or shuttle run: specifics only under `internal/shuttleengine/<engine>`, model-specs via `NewModelChoice`, a strand stopped via the stop verb, a negative finish after `allOutputFilesExist`. (test) — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-shell-guard-table` — Guarding a shell command or building a pane-shell string: only `internal/shellguard`'s table, every engine rendering every row, and `internal/shell`, importing only stdlib.

## Processes and waits

- `PATTERN-child-processes` — Starting or ending a process: log the spawn via `internal/logger`; its starter waits on it, bar a double fork's intermediate, which exits at once; a run's processes register in `internal/runproc` and end only through it. — [background](pattern/PATTERN-child-processes.md)
- `PATTERN-daemon-wakeups` — Waiting: an unbounded wait wakes at most once a second and backs off while idle; a wait an agent blocks on takes its cap from the keep-warm resolver. (test) — [background](pattern/PATTERN-daemon-wakeups.md)

## Testing

- `PATTERN-test-speed` — Writing or running a test: via `lym gate test` by gate key; one `tiergate.Require` tier, lowest that fits; it pins behavior no test covers; skips, clocks, `t.Parallel` and `//lym:seam` per the background. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing package: hermetic git and `tmuxkit.Main`; shared support in one `internal/testkit/<kit>/`; hub fixtures via `hubforge`. (test) — [background](pattern/PATTERN-test-isolation.md)
