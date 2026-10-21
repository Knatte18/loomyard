# PATTERN

Structural invariants, one line per entry: when it applies and what it requires.

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd or worktree root: only `internal/lyxcwd`; nothing else calls `os.Getwd` or `git rev-parse`. — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-told-geometry` — Writing an engine: handed its absolute paths, it derives none and never imports `internal/lyxcwd`. — [background](pattern/PATTERN-told-geometry.md)
- `PATTERN-lyxdirs-single-declarer` — Naming `_lyx` or `.lyx` in a path: only `internal/lyxdirs` declares them, bar one variable block atop a `tools/` script.
- `PATTERN-lyx-temp-root` — Creating a temp file or dir: under `internal/lyxtemp`'s run dir, never `os.TempDir()`. (test) — [background](pattern/PATTERN-lyx-temp-root.md)
- `PATTERN-durable-vs-ephemeral-state` — Adding a file: tracked under `_lyx`, never-tracked under `.lyx`, same subpath. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-hub-containment` — Linking hub-level containers or writing the hub workspace file: from the hub only. — [background](pattern/PATTERN-hub-containment.md)
- `PATTERN-hub-wide-config` — A hub-level config module: `HubWide` in `configreg`, its file on the board only, its machine override beside it. — [background](pattern/PATTERN-hub-wide-config.md)
- `PATTERN-sparse-hub-base` — Writing a hub's base file: only the units the hub tuned, never a template fill. — [background](pattern/PATTERN-sparse-hub-base.md)
- `PATTERN-hub-suffix` — Touching hub naming: `-LYXHUB` is the sole hub container suffix. (test) — [background](pattern/PATTERN-hub-suffix.md)

## CLI

- `PATTERN-cli-cobra` — Adding a CLI module: a cobra subtree under one root in `cmd/lyx/main.go`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-cliwire-sole-wiring` — Wiring a standalone-capable CLI: `internal/cliwire` alone resolves target dir, repo root and state. (test) — [background](pattern/PATTERN-cliwire-sole-wiring.md)
- `PATTERN-config-strictness` — Loading config: a caller adopts exactly one of strict `Load` or `LoadOrTemplate`, bar `runprofile`. — [background](pattern/PATTERN-config-strictness.md)
- `PATTERN-config-layers` — Resolving a module's config: only `configengine` reads, stacks and locates layer files, machine layer included. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-binary-change-reconcile` — Reconciling a hub's config after a binary change: only via `hubreconcile.Ensure`, owner of the build stamp and hub lock.
- `PATTERN-refusal-way-forward` — Adding a webster, shed or loom refusal: its message names the way forward; a test reaches it. — [background](pattern/PATTERN-refusal-way-forward.md)
- `PATTERN-no-denied-recovery` — Naming a step in a refusal, stencil or spec: never a command the agents' settings deny. (test) — [background](pattern/PATTERN-no-denied-recovery.md)

## Fabric and git

- `PATTERN-fabric-vocabulary` — Naming the wired composite: "fabric"; warp and weft only to tell its sides apart. (test) — [background](pattern/PATTERN-fabric-vocabulary.md)
- `PATTERN-fabric-git` — Running git on warp or weft: only `internal/fabricengine`, in-process, never raw git. — [background](pattern/PATTERN-fabric-git.md)
- `PATTERN-fabric-destruction-chokepoint` — Destroying anything in fabric: only `internal/fabricengine/destroy.go`. — [background](pattern/PATTERN-fabric-destruction-chokepoint.md)
- `PATTERN-fabric-write-containment` — Writing `_launchers` or the workspace file in `fabricengine`: via a hub `os.Root`, never raw `os` or `fslink`.
- `PATTERN-push-both-sides` — Pushing run records from Go: the pair's code branch goes along, and a status push is detached. — [background](pattern/PATTERN-push-both-sides.md)
- `PATTERN-push-failure-visible` — A push refused after its retry: a failed-push record and a creel notice. (test)
- `PATTERN-mutation-record` — Adding a mutating fabric verb: it fills a `*Mutations` record; its result embeds `MutationRecord`. — [background](pattern/PATTERN-mutation-record.md)
- `PATTERN-pair-teardown` — Tearing down a pair: only via `internal/pairteardown`. (test) — [background](pattern/PATTERN-pair-teardown.md)
- `PATTERN-batten-bookend` — Creating or destroying a task worktree: its producer never runs inside it. — [background](pattern/PATTERN-batten-bookend.md)
- `PATTERN-github-auth` — Calling GitHub: auth only via `internal/githubclient`; no other production package runs `gh`.
- `PATTERN-agent-filed-issues` — Filing a GitHub issue: only `lyx selfreport create` calls `selfreportengine.CreateIssue` in production. (test)
- `PATTERN-hub-store-gc` — Hub stores run no automatic gc or maintenance: only `fabricengine/housekeeping.go` sets that or runs `gc`. — [background](pattern/PATTERN-hub-store-gc.md)
- `PATTERN-gogit-read-helper` — Reading via go-git in `internal/gitrepo`: only via `readGoGit`, sole caller of `goGit()`. (test) — [background](pattern/PATTERN-gogit-read-helper.md)
- `PATTERN-gitrepo-client-boundary` — Git in `internal/gitrepo`: local reads via go-git and `dotgit`, the rest via `gitexec`. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-gitexec-checked-call` — Running git: `gitexec.Run`/`runChecked`; raw `RunGit`/`r.run` only at pinned `//gitexec:raw` sites.
- `PATTERN-never-force-add` — Keeping transients unindexed: each repo's own `.git/info/exclude`; fabric and gitrepo never run `git add -f`.
- `PATTERN-origin-reconcile` — Acting on a run or the hub after an absence: first `fabricengine`'s origin reconcile, sole judge of branches.
- `PATTERN-parent-lock` — Merging a parent into a task branch, or landing on it: only under that parent's `internal/parentlock` lock; a moved parent reroutes to Merge-In.

## Shed and loom

- `PATTERN-shed-producer-seam` — Importing into `internal/shedengine`: only stdlib, `state` and `lock`; its status and lock paths are caller-supplied.
- `PATTERN-shed-recipe-registry` — Adding a producer: one `map[string]Constructor` in `internal/shedrecipe` behind `Lookup`, bar `Implementer`'s; no `init()`/`Register`/`lyxcwd`.
- `PATTERN-recipe-setting-lookup` — Giving a recipe row a value: looked up by key, never its own `Env` field. — [background](pattern/PATTERN-recipe-setting-lookup.md)
- `PATTERN-command-row` — Adding a side-action row to a recipe: a `Command` row over its config keys, its command read from the template and board default layer only, never a new engine.
- `PATTERN-shed-verb-set` — Adding a run, step, status, pause or goto verb: generic bodies only in `internal/shedverbs`. (test) — [background](pattern/PATTERN-shed-verb-set.md)
- `PATTERN-shed-driver` — Driving a shed run in Go, deciding its stops or spawning a tackler: only `internal/sheddriver`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-shed-run-directory` — Touching run directories or `seed.json`: `internal/shedrun` alone declares their paths, ids and `Seed`. — [background](pattern/PATTERN-shed-run-directory.md)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-transient-stop` — Marking a failure transient: the `internal/shedengine` mark, set only at a producer or step-bootstrap boundary. — [background](pattern/PATTERN-transient-stop.md)
- `PATTERN-failure-stop-visible` — An `--until-stop` failure as sole lock holder: status is non-running before the waiter returns. (test) — [background](pattern/PATTERN-failure-stop-visible.md)
- `PATTERN-driver-choice-single-site` — Reading a seed driver: once per recipe, in its bootstrap verbs, for the surface only. (test) — [background](pattern/PATTERN-driver-choice-single-site.md)
- `PATTERN-treadle-runner-seam` — Importing into `internal/treadleengine`: only its allowlist, never `burlerengine`, `lyxcwd` or an `internal/*cli` package.
- `PATTERN-plan-generation` — Archiving a plan: `_lyx/plan/`'s top-level files hold one generation. — [background](pattern/PATTERN-plan-generation.md)
- `PATTERN-ref-shape-registry` — Classifying a plan ref or `plan:` handle: `internal/planparser` alone declares and decides. (test) — [background](pattern/PATTERN-ref-shape-registry.md)
- `PATTERN-glyph-conversion-chokepoint` — Converting glyphs and paths: only via `glyph`. — [background](pattern/PATTERN-glyph-conversion-chokepoint.md)
- `PATTERN-gate-self-check-parity` — Adding a mechanical gate: its closure and CLI self-check call one package function. — [background](pattern/PATTERN-gate-self-check-parity.md)
- `PATTERN-verified-tree` — Running a verify command: only via `verifytree.Verify`. (test) — [background](pattern/PATTERN-verified-tree.md)
- `PATTERN-gate-slot` — Running a gate build or test: only in a `-p`-capped, `-trimpath` hub slot of `internal/gateslot`.
- `PATTERN-trimpath-safe-root` — Locating the module root in code a gate run builds: never from a `runtime.Caller` path alone, which `-trimpath` makes module-relative.
- `PATTERN-batcher-registry` — Choosing webster's execution unit: the batch the active `batcher.yaml` profile derives via `internal/batcher`. — [background](pattern/PATTERN-batcher-registry.md)
- `PATTERN-review-round` — Review and fix round: review accepted on disk before any edit, each finding fixed or disputed, end on a verdict. — [background](pattern/PATTERN-review-round.md)
- `PATTERN-sole-parsers` — Parsing plan, discussion, summary, recipe or halt note: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-agent-name` — Forming an agent name: only `internal/agentname`, once by reed. — [background](pattern/PATTERN-agent-name.md)
- `PATTERN-creel-store` — Writing the creel journal or reaching a pane: only `internal/creelengine`, a pane only via its `Delivery` seam. (test) — [background](pattern/PATTERN-creel-store.md)
- `PATTERN-stencil-ownership` — Reading a prompt or spec: at call time via `internal/stencilstore` from a told directory, never embedded. — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-producer-pointer-rule` — Writing an instruction file: it points at another producer's format contract, never restating it.
- `PATTERN-friction-capture` — Halting a loom run or refusing in webster: Go writes a note, reflected at the run's end, archived once covered. (test) — [background](pattern/PATTERN-friction-capture.md)
- `PATTERN-completion-signal` — Finding a run unfinished in `internal/shuttleengine`: consult `allOutputFilesExist` first. (test) — [background](pattern/PATTERN-completion-signal.md)
- `PATTERN-shuttle-provider-seam` — Referencing a provider: its specifics live only under `internal/shuttleengine/<engine>`. (test) — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-model-choice-mapping` — Mapping a resolved model-spec onto a spawn: only `shuttleengine.NewModelChoice` reads its fields. (test) — [background](pattern/PATTERN-model-choice-mapping.md)
- `PATTERN-shuttle-stop` — Stopping a shuttle strand from Go outside `internal/shuttleengine`: only via shuttle's stop verb. (test) — [background](pattern/PATTERN-shuttle-stop.md)
- `PATTERN-orch-pane-single-writer` — Typing into the orch session from Go: only creel, via its `Delivery` seam. — [background](pattern/PATTERN-orch-pane-single-writer.md)
- `PATTERN-shell-guard-table` — Guarding a shell command: only in `internal/shellguard`, every engine rendering every row; engine tool guards stay in their engine.
- `PATTERN-shell-mechanics-seam` — Building a pane-shell command string: only via `internal/shell`, which imports only stdlib.
- `PATTERN-pane-text-injection` — Typing text into an agent pane: only reed's guarded paste-submit. — [background](pattern/PATTERN-pane-text-injection.md)
- `PATTERN-guarded-interrupt` — Sending an agent pane the interrupt key: only shuttle's two pane guards. — [background](pattern/PATTERN-guarded-interrupt.md)
- `PATTERN-pane-binary-resolution` — Creating a strand pane or detached `lyx` window in reed: resolve `lyx` via `panebin.go` only. (test) — [background](pattern/PATTERN-pane-binary-resolution.md)
- `PATTERN-role-skills-typed` — Loading a spawned session's skill: its spawner names it on the launch spec, lyx delivers it on the launch line or typed; no stencil asks for one. (test)
- `PATTERN-parent-directive` — Writing a spawned role's top-level stencil: it renders the parent directive. (test) — [background](pattern/PATTERN-parent-directive.md)
- `PATTERN-edit-directive` — Writing a spawned role's opening stencil: it renders `internal/editdirective`'s directive. (test) — [background](pattern/PATTERN-edit-directive.md)
- `PATTERN-spawn-color` — Coloring a spawned session: its spawner names a segment on the launch spec. — [background](pattern/PATTERN-spawn-color.md)
- `PATTERN-wait-mark-display-only` — Marking a wait on screen: only reed's `SetWaitMark`; no Go decision reads the marks. — [background](pattern/PATTERN-wait-mark-display-only.md)
- `PATTERN-spawn-observability` — Starting an OS process from `lyx`: log the spawn, and a teardown it waits for, via `internal/logger`. — [background](pattern/PATTERN-spawn-observability.md)

## Daemons and waits

- `PATTERN-daemon-wakeups` — An unbounded wait wakes at most once a second and backs off while idle. (test) — [background](pattern/PATTERN-daemon-wakeups.md)
- `PATTERN-log-levels` — Logging in a loop or poll: follow `internal/logger`'s Level policy.

## Packages

- `PATTERN-leaf-packages` — Importing into a leaf package: only its closed import set, packages and sets named in the background. — [background](pattern/PATTERN-leaf-packages.md)

## Build and tooling

- `PATTERN-sandbox-coverage` — Registering a lyx module: the sandbox suite exercises it or excludes it with a reason.
- `PATTERN-dev-prod-binary-separation` — Resolving `lyx` in sandbox tooling: `resolveLyx` (`.dev-bin` first, then PATH), never bare PATH.
- `PATTERN-quarry-cgo` — Building this module: `CGO_ENABLED=1` and a C compiler, for tree-sitter grammars. — [background](pattern/PATTERN-quarry-cgo.md)

## Testing

- `PATTERN-test-economy` — Writing a test: it pins public-surface behavior no test covers yet, else it extends one. — [background](pattern/PATTERN-test-economy.md)
- `PATTERN-test-speed` — Writing a test: lowest fitting tier, injected clocks, `t.Parallel` by default. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing a package: git spawners use `gitkit.HermeticGitEnv()`; tagged packages run through `tmuxkit.Main`. (test) — [background](pattern/PATTERN-test-isolation.md)
- `PATTERN-testkit` — Sharing test support across packages: one kit under `internal/testkit/<kit>/`, imported only from tests, never duplicated. (test) — [background](pattern/PATTERN-testkit.md)
- `PATTERN-hubforge-fixtures` — A hub fixture no fake can test: `hubforge` via `fabriccli.CloneAndWire`, never hand-assembled or wrapped. — [background](pattern/PATTERN-hubforge-fixtures.md)

## Docs

- `PATTERN-markdown-link-integrity` — Linking in a `.md` file under `docs/`: every inline link resolves, file part and `#anchor`. (test) — [background](pattern/PATTERN-markdown-link-integrity.md)
- `PATTERN-comment-line-breaks` — Writing a Go comment: one sentence per line, checked by `lyx loom lint-comments` alone. (test) — [background](pattern/PATTERN-comment-line-breaks.md)
- `PATTERN-documentation-lifecycle` — Design docs: none for unbuilt work; a built one lives in its package's `doc.go`. — [background](pattern/PATTERN-documentation-lifecycle.md)
