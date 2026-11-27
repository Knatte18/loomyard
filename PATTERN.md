# PATTERN

## Code

- `PATTERN-code-conventions` — Writing code: a package's `doc.go` documents only that package; a name or path that can go stale is one named constant; no language, engine or code branch is hard-coded, and a branch lym owns in its records namespace is one named constant in fabric. — [background](pattern/PATTERN-code-conventions.md)

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd, worktree root or anchor: only `lymcwd`, from the worktree's weft-root `.lym-anchor`; only `fabricengine` writes or git-reads it; engines get absolute paths, never import lymcwd; hub containers and workspace files stay hub-reachable only. (test) — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-lym-files` — Placing a file, temp dir, lock or link: tracked under `_lym`, never-tracked under `.lym` as a cache a fresh machine rebuilds or starts without; named only by `lymdirs`; temp under `lymtemp`, lock paths via `lock`, links via `fslink`. (test) — [background](pattern/PATTERN-lym-files.md)
- `PATTERN-leaf-packages` — Importing into a leaf package: only closed import set; `fabriccore` and `fabriclayout` run no git and do no I/O. — [background](pattern/PATTERN-leaf-packages.md)

## CLI and config

- `PATTERN-cli-cobra` — Adding or wiring a CLI module: a cobra subtree under one root in `cmd/lym/main.go`; standalone wiring only through `internal/cliwire`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-config-layers` — Loading, writing or changing config: layers only through `configengine`; a hub base file holds only tuned units; a meaning change ships a reinterpretation, a key move a move table, both run by `hubreconcile`; a per-task beam setting is read only via `dobbyengine`'s one lookup. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-config-key-names` — Naming a config key in a message, comment, doc or stencil: in full, `<module>.<dotted path>`, rendered in Go by `configengine`'s one key-name helper from named constants.
- `PATTERN-refusal-way-forward` — Adding webster, shed or loom refusal: message names the way forward and a test reaches it; no refusal, stencil or spec names a step agents' settings deny. (test) — [background](pattern/PATTERN-refusal-way-forward.md)

## Fabric and git

- `PATTERN-fabric-boundary` — Changing fabric or running git on warp or weft: only `internal/fabricengine`, called "fabric", which learns nothing of IDEs, shuttle, the board or heddle. (test) — [background](pattern/PATTERN-fabric-boundary.md)
- `PATTERN-single-repo-hub` — Running a hub as one repository: a code store and a records store of one origin, records and the board under origin's `refs/lym/`, mapped by the records store's refspecs; only fabric reads the layout. — [background](pattern/PATTERN-single-repo-hub.md)
- `PATTERN-gitrepo-client-boundary` — Running git or writing remote-tracking refs: through `gitexec` and `gitrepo`, raw only at pinned sites; fetch, pull and push under the remote-refs lock. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-pair-remote` — Detached pushes of lym's branches: one background pusher per target, records before code, owed pushes retried quietly; a push whose result its caller reads stays synchronous; a pair's remote steps hold the slug's teardown lock; teardown only via `internal/pairteardown`. (test) — [background](pattern/PATTERN-pair-remote.md)
- `PATTERN-owner-machine` — Starting, resuming, pushing or landing a run, a Finalize past its merge included, or picking a pair's parent: only on the entry's owner machine, read via hubgeom's `ParentOf` and Bolt's machine registry; a loom pusher reconciles first; after absence, a catch-up from origin first. — [background](pattern/PATTERN-owner-machine.md)
- `PATTERN-landing` — Landing on a local parent branch or deciding on a pull request: only in a run's Finalize under `internal/parentlock`'s lock; landingshed's one resolver decides the pull request; a pull request's review ends only on the operator's word, whatever the run's autonomy. — [background](pattern/PATTERN-landing.md)
- `PATTERN-github-auth` — Calling GitHub: auth only via internal/githubclient; no other production package runs gh; only lym selfreport create files an issue; only PR-Review, Finalize, `lym beam pr` and pair teardown open, reopen, merge or close a pull request; `lym beam pr` never merges. (test)

## Shed and loom

- `PATTERN-shed-recipe-registry` — Adding a producer, recipe row, block gate or loom recipe: one `shedrecipe` map, row values by key; a loom recipe declares `contract: loom`. — [background](pattern/PATTERN-shed-recipe-registry.md)
- `PATTERN-shed-driver` — Driving or stopping a shed run: verbs in `shedverbs`, the loop in `sheddriver`, paths in `shedrun`, shared machinery in shed; one stop chain; transient marks only via `shedengine`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-run-fsm` — Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule; a transition commits and the run acts at once, a background pusher bringing every commit to origin in order; a run whose record holds another machine's takeover hands over, never replays. — [background](pattern/PATTERN-run-fsm.md)
- `PATTERN-dobby-runs` — Starting or holding a board task's run: only its parent's dobby, started with its orch bar `lym adopt --no-orch`; front tasks only via `fronttask`; an adopt holds the hub through Bolt's holds until an adopt's check is green. — [background](pattern/PATTERN-dobby-runs.md)
- `PATTERN-run-records` — Writing a run's friction note, usage record or project record: only its one writer, `internal/friction`, shed's usage writer or `lym loom decision add` or `replace`. (test) — [background](pattern/PATTERN-run-records.md)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-gate-slot` — Running or clearing a gate: in held `internal/gateslot` slots, verify via `verifytree.Verify`; a cold fixer only via `gatefixer`, judged by Go's re-run. (test) — [background](pattern/PATTERN-gate-slot.md)
- `PATTERN-sole-parsers` — Parsing an on-disk format its background lists, or classifying a plan ref: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-stencil-ownership` — Reading or writing a prompt, spec or stencil: read at call time via `stencilstore`; one engine's wording only in its engine blocks, rendered for the launch's engine; a spawned role's stencil renders the parent and edit directives. (test) — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-heddle-instructions` — Rendering or writing agent instructions: heddle entries only via `heddleengine`; lym's own instructions in its code, stencils, specs or heddle's shipped layer. — [background](pattern/PATTERN-heddle-instructions.md)
- `PATTERN-reed-panes` — Spawning an agent or touching its pane: an interactive tmux session through reed, never headless; text typed only via guarded paths; the wait mark is display only. (test) — [background](pattern/PATTERN-reed-panes.md)
- `PATTERN-agent-addressing` — Naming or messaging an agent: names only via `internal/agentname`; only through creel; across hubs with the sender confirmed, across machines orch to orch via the Slack relay; authority only for a trusted hub's or approved machine's orch. (test) — [background](pattern/PATTERN-agent-addressing.md)
- `PATTERN-shuttle-provider-seam` — Touching a provider or shuttle run: specifics only under `internal/shuttleengine/<engine>`, model-specs via `NewModelChoice`, a spec's prompt rendered for its launch's engine, a strand stopped via the stop verb, a negative finish after `allOutputFilesExist`. (test) — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-engine-parity` — Launching a role on an engine: the session gets every skill, standing passage, instruction file, guard, hook and directive a session of that role gets on another engine, through its own channel; each engine's launch manifest is compared by test. (test) — [background](pattern/PATTERN-engine-parity.md)
- `PATTERN-shell-guard-table` — Guarding a shell command or building a pane-shell string: only `internal/shellguard`'s table, every engine rendering every row, and `internal/shell`, importing only stdlib.

## Processes and waits

- `PATTERN-child-processes` — Starting or ending a process: log the spawn via `internal/logger`; its starter waits on it, bar a double fork's intermediate; a run's processes register in `internal/runproc` and end only via it, orphans via `EndProcess`; a pinned or spawned lym is named via `selfexec.LymBinary`. — [background](pattern/PATTERN-child-processes.md)
- `PATTERN-daemon-wakeups` — Waiting: an unbounded wait wakes at most once a second and backs off while idle; a wait an agent blocks on takes its cap from the keep-warm resolver. (test) — [background](pattern/PATTERN-daemon-wakeups.md)

## Testing

- `PATTERN-test-speed` — Writing or running a test: via `lym gate test` by key; one `tiergate.Require` tier, lowest that fits; it pins behavior no other test covers; skips, clocks, `t.Parallel`, `//lym:seam`, `//lym:watch` and `//lym:only` per the background. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-stand-ins` — A test's stand-in: never for a repository module's state, records, config or output; drive the real module or move the test there. For something outside: below the one module reaching it, one per boundary, answering only recordings, held to it by a contract test. — [background](pattern/PATTERN-test-stand-ins.md)
- `PATTERN-test-isolation` — Testing package: hermetic git and `tmuxkit.Main`; shared support in one `internal/testkit/<kit>/`; a read-only or unreadable directory via `permkit.Chmod`; hub fixtures via `hubforge`. (test) — [background](pattern/PATTERN-test-isolation.md)
- `PATTERN-test-only-code` — Writing a function only tests call: it lives in a `_test.go` file or a test-support package (`internal/testkit`, `gitkit`, `hubforge`), never a production file; every other function is reachable from a `main` package. (test) — [background](pattern/PATTERN-test-only-code.md)
