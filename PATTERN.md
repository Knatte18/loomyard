# PATTERN

The structural invariants of the loomyard code, one line per entry: when it applies and what it requires.

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd or worktree root: only `internal/lyxcwd` does it, and nothing else calls `os.Getwd` or `git rev-parse --show-toplevel`. — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-told-geometry` — Writing an engine: it is handed the absolute paths it works on and derives none, so it never imports `internal/lyxcwd`. — [background](pattern/PATTERN-told-geometry.md)
- `PATTERN-lyxdirs-single-declarer` — Naming `_lyx` or `.lyx` in a path: only `internal/lyxdirs` declares those literals, and no other production file spells them.
- `PATTERN-lyx-temp-root` — Creating a temp file or directory: under `internal/lyxtemp`'s run directory; `os.MkdirTemp`/`os.CreateTemp` never take `""` or `os.TempDir()` elsewhere. (test) — [background](pattern/PATTERN-lyx-temp-root.md)
- `PATTERN-durable-vs-ephemeral-state` — Adding a file: tracked content lives under `_lyx`, anything never tracked under `.lyx` at the mirrored subpath. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-hub-containment` — Linking hub-level containers: `_board`, `_portals` and `_launchers` are reachable from the hub only, never junctioned into a worktree. — [background](pattern/PATTERN-hub-containment.md)
- `PATTERN-hub-wide-config` — A hub-level config module: `HubWide` in `configreg`, its file on the board only. — [background](pattern/PATTERN-hub-wide-config.md)
- `PATTERN-hub-suffix` — Touching hub naming: `-LYXHUB` is the sole hub container suffix, and no code parses, trims or recognises the retired `-HUB`. (test) — [background](pattern/PATTERN-hub-suffix.md)

## CLI

- `PATTERN-cli-cobra` — Adding a CLI module: a cobra subtree under one root in `cmd/lyx/main.go`. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-cliwire-sole-wiring` — Wiring a standalone-capable CLI: `internal/cliwire` alone resolves target dir, repo root and state. (test) — [background](pattern/PATTERN-cliwire-sole-wiring.md)
- `PATTERN-config-strictness` — Loading config: a caller adopts exactly one of `configengine.Load` (strict) or `LoadOrTemplate` (degrades to the embedded template). — [background](pattern/PATTERN-config-strictness.md)
- `PATTERN-config-layers` — Resolving a module's config: only `configengine` reads or stacks layer files and forms their paths; keys closed to the local layer are set through `configreg`. — [background](pattern/PATTERN-config-layers.md)
- `PATTERN-binary-change-reconcile` — Reconciling a hub's config after a binary change: only through `hubreconcile.Ensure`, which owns the build stamp and the hub lock.
- `PATTERN-refusal-way-forward` — Adding a webster, shed or loom refusal: name the way forward, land its spec row with it, add a test. — [background](pattern/PATTERN-refusal-way-forward.md)
- `PATTERN-no-denied-recovery` — Naming a step in a refusal, stencil or spec: never a command the agents' settings deny; lyx does it. (test) — [background](pattern/PATTERN-no-denied-recovery.md)

## Fabric and git

- `PATTERN-fabric-vocabulary` — Naming the wired composite: "fabric"; warp and weft only to tell its sides apart. (test) — [background](pattern/PATTERN-fabric-vocabulary.md)
- `PATTERN-fabric-git` — Running git on warp or weft: only `internal/fabricengine`, in-process, never raw git. — [background](pattern/PATTERN-fabric-git.md)
- `PATTERN-fabric-destruction-chokepoint` — Destroying anything in fabric: only `internal/fabricengine/destroy.go`. — [background](pattern/PATTERN-fabric-destruction-chokepoint.md)
- `PATTERN-fabric-write-containment` — Writing under `_launchers` or `_portals` from `fabricengine`: through an `os.Root` rooted at the hub, never raw `os.MkdirAll`, `os.WriteFile` or `fslink`.
- `PATTERN-push-both-sides` — Pushing run records from Go: the pair's code branch goes along; the prime sends records only. — [background](pattern/PATTERN-push-both-sides.md)
- `PATTERN-mutation-record` — Adding a mutating fabric verb: it accumulates a `*Mutations` record and its result embeds `MutationRecord` under a fixed envelope key set. — [background](pattern/PATTERN-mutation-record.md)
- `PATTERN-pair-teardown` — Tearing down a pair: only through `internal/pairteardown`, which ends the pair's reed session before any worktree is removed. (test) — [background](pattern/PATTERN-pair-teardown.md)
- `PATTERN-batten-bookend` — Creating or destroying a task worktree: the producer never runs from inside it, and session shutdown precedes removal in one row. — [background](pattern/PATTERN-batten-bookend.md)
- `PATTERN-github-auth` — Calling GitHub: all authentication goes through `internal/githubclient`, and no other production package shells out to `gh`.
- `PATTERN-agent-filed-issues` — Filing a GitHub issue from lyx: only through `lyx selfreport create`; no other production package calls `selfreportengine.CreateIssue`. (test)
- `PATTERN-hub-store-gc` — Hub stores carry `gc.auto=0` and `maintenance.auto=false`: only `fabricengine/housekeeping.go` sets them or runs `gc`. — [background](pattern/PATTERN-hub-store-gc.md)
- `PATTERN-gogit-read-helper` — Reading through go-git in `internal/gitrepo`: only through `readGoGit`, the sole caller of `goGit()`. (test) — [background](pattern/PATTERN-gogit-read-helper.md)
- `PATTERN-gitrepo-client-boundary` — Git in `internal/gitrepo`: local reads via go-git and `dotgit`, the rest via `gitexec`. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-gitexec-checked-call` — Running git: use `gitexec.Run`/`runChecked`; the raw `RunGit`/`r.run` forms survive only at pinned `//gitexec:raw` call sites.
- `PATTERN-never-force-add` — Keeping transients out of the index: each repo's own `.git/info/exclude`; fabric and gitrepo never run `git add -f`.
- `PATTERN-parent-lock` — Merging a parent into a task branch, or landing on it: only under that parent's lock through `internal/parentlock`; a moved parent reroutes to Merge-In.

## Shed and loom

- `PATTERN-shed-producer-seam` — Importing into `internal/shedengine`: only stdlib, `state` and `lock`, and its status and lock paths are caller-supplied.
- `PATTERN-shed-recipe-registry` — Registering a shed producer: one `map[string]Constructor` in `internal/shedrecipe`, reached through `Lookup`/`Names`, with no `init()`, no `Register`, no `lyxcwd`.
- `PATTERN-shed-verb-set` — Adding a run, step, status, pause or goto verb: the generic bodies live in `internal/shedverbs` only. (test) — [background](pattern/PATTERN-shed-verb-set.md)
- `PATTERN-shed-run-directory` — Touching run directories or `seed.json`: `internal/shedrun` alone declares their paths, ids and `Seed`. — [background](pattern/PATTERN-shed-run-directory.md)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-transient-stop` — Marking a failure transient: the `internal/shedengine` mark, set only at a producer or step-bootstrap boundary. — [background](pattern/PATTERN-transient-stop.md)
- `PATTERN-failure-stop-visible` — An `--until-stop` failure as sole lock holder: status is non-running before the waiter returns. (test) — [background](pattern/PATTERN-failure-stop-visible.md)
- `PATTERN-driver-choice-single-site` — Reading a seed driver: once per recipe, in its bootstrap verbs (`start`/`resume`), for the surface only. (test) — [background](pattern/PATTERN-driver-choice-single-site.md)
- `PATTERN-treadle-runner-seam` — Importing into `internal/treadleengine`: never `burlerengine` or an `internal/*cli` package, and only its allowlist, not `lyxcwd`.
- `PATTERN-plan-generation` — Archiving a plan: `_lyx/plan/`'s top-level files hold exactly one generation, and a retired one lives under `round-<N>/prior-generation/`. — [background](pattern/PATTERN-plan-generation.md)
- `PATTERN-ref-shape-registry` — Classifying a plan ref or `plan:` handle: `internal/planparser` alone declares and decides. (test) — [background](pattern/PATTERN-ref-shape-registry.md)
- `PATTERN-glyph-conversion-chokepoint` — Converting glyphs and paths: only `glyph`'s `Self`, `UnitPath`, `Parse` and `String`, plus `planparser`'s one external-test unit mapping. — [background](pattern/PATTERN-glyph-conversion-chokepoint.md)
- `PATTERN-gate-self-check-parity` — Adding a mechanical gate: its closure and its CLI self-check verb call the same package function, and both land in one task. — [background](pattern/PATTERN-gate-self-check-parity.md)
- `PATTERN-verified-tree` — Running a verify command: through `verifytree.Verify` only, never on a dirty tree, skipping only on a record of HEAD's tree. (test) — [background](pattern/PATTERN-verified-tree.md)
- `PATTERN-gate-slot` — Running a gate build or test: only in a `-p`-capped hub slot of `internal/gateslot`.
- `PATTERN-batcher-registry` — Choosing webster's execution unit: the batch the active `batcher.yaml` profile derives via `internal/batcher`. — [background](pattern/PATTERN-batcher-registry.md)
- `PATTERN-review-round` — Review and fix round: review accepted on disk before any edit, each finding fixed or disputed, end on a verdict. — [background](pattern/PATTERN-review-round.md)
- `PATTERN-sole-parsers` — Parsing plan, discussion, summary, recipe or halt note: only `planparser`, `discussionparser`, `summaryparser`, `shedbuild`, `haltnote`. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-agent-name` — Forming or parsing an agent name: only `internal/agentname`, formed once by reed; the spawning module owns its role names. — [background](pattern/PATTERN-agent-name.md)
- `PATTERN-stencil-ownership` — Reading a prompt or spec: at call time via `internal/stencilstore` from a told directory, never embedded. — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-producer-pointer-rule` — Writing an instruction file: it points at another producer's format contract and never duplicates or paraphrases it.
- `PATTERN-friction-capture` — Halting a loom run or refusing in webster: a Go-authored friction note is written, and none is archived before a reflection covers it. (test) — [background](pattern/PATTERN-friction-capture.md)
- `PATTERN-completion-signal` — Finalizing a negative "did this run finish" answer in `internal/shuttleengine`: consult `allOutputFilesExist` first. (test) — [background](pattern/PATTERN-completion-signal.md)
- `PATTERN-shuttle-provider-seam` — Referencing a provider: its specifics live only under `internal/shuttleengine/claudeengine`. — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-shuttle-stop` — Stopping a shuttle run's strand from Go outside `internal/shuttleengine`: only through shuttle's stop verb. (test) — [background](pattern/PATTERN-shuttle-stop.md)
- `PATTERN-orch-pane-single-writer` — Typing into the orch session from Go: only the orch watcher does it, idle-gated except the interrupt a hub pause asks for; another module queues a notice through `orchengine` instead.
- `PATTERN-shell-mechanics-seam` — Building a pane-shell command string: only through `internal/shell`, which imports the standard library alone.
- `PATTERN-pane-text-injection` — Typing text into an agent pane: only reed's guarded paste-submit, never a blind Enter. — [background](pattern/PATTERN-pane-text-injection.md)
- `PATTERN-pane-binary-resolution` — Creating a strand pane or detached `lyx` window in reed: resolve `lyx` via `panebin.go` only. (test) — [background](pattern/PATTERN-pane-binary-resolution.md)
- `PATTERN-role-skills-typed` — Loading a skill into a spawned session: the spawning module names it on the launch spec and lyx types it; no stencil asks an agent to load a skill. (test)
- `PATTERN-parent-directive` — Writing a spawned role's top-level stencil: it renders the parent directive, and no stencil tells an agent to ask the operator. (test) — [background](pattern/PATTERN-parent-directive.md)
- `PATTERN-edit-directive` — Writing a spawned role's opening stencil: it renders the edit directive through `internal/editdirective`; the orch stencils are exempt. (test) — [background](pattern/PATTERN-edit-directive.md)
- `PATTERN-spawn-color` — Coloring a spawned session: its spawning module names a segment on the launch spec, and no stencil asks an agent to set a color. — [background](pattern/PATTERN-spawn-color.md)
- `PATTERN-wait-mark-display-only` — Marking a wait on screen: set `@lyx_wait` and `@lyx_wait_start` only through reed's `SetWaitMark`; no Go decision reads them. — [background](pattern/PATTERN-wait-mark-display-only.md)
- `PATTERN-spawn-observability` — Starting a real OS process from a `lyx` command: log the spawn, and the teardown where it waits, via `internal/logger`. — [background](pattern/PATTERN-spawn-observability.md)

## Daemons and waits

- `PATTERN-daemon-wakeups` — An unbounded wait wakes at most once a second and backs off while idle. (test) — [background](pattern/PATTERN-daemon-wakeups.md)

## Packages

- `PATTERN-leaf-packages` — Importing into a leaf package: each admits a closed import set, the packages and sets named in the background. — [background](pattern/PATTERN-leaf-packages.md)

## Build and tooling

- `PATTERN-sandbox-coverage` — Registering a lyx module: the sandbox suite exercises it, or explicitly excludes it with a reason.
- `PATTERN-dev-prod-binary-separation` — Resolving `lyx` in sandbox tooling: `resolveLyx` (`.dev-bin` first, then PATH), never bare PATH.
- `PATTERN-quarry-cgo` — Building this module: it needs `CGO_ENABLED=1` and a C compiler for tree-sitter grammars. — [background](pattern/PATTERN-quarry-cgo.md)

## Testing

- `PATTERN-test-economy` — Writing a test: it pins public-surface behavior no test covers yet, else it extends one. — [background](pattern/PATTERN-test-economy.md)
- `PATTERN-test-speed` — Writing a test: lowest fitting tier of four, `llm` files alone reach an LLM, inject clocks, `t.Parallel` by default. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing a package: git spawners use `gitkit.HermeticGitEnv()`; tagged packages run through `tmuxkit.Main`. (test) — [background](pattern/PATTERN-test-isolation.md)
- `PATTERN-testkit` — Sharing test support between packages: one kit under `internal/testkit/<kit>/`, imported only from tests, never duplicated. (test) — [background](pattern/PATTERN-testkit.md)
- `PATTERN-hubforge-fixtures` — A hub fixture no fake can test: `hubforge` via `fabriccli.CloneAndWire`, never hand-assembled or wrapped. — [background](pattern/PATTERN-hubforge-fixtures.md)

## Docs

- `PATTERN-markdown-link-integrity` — Linking in a `.md` file under `docs/`: every inline link resolves, file part and `#anchor`. (test) — [background](pattern/PATTERN-markdown-link-integrity.md)
- `PATTERN-comment-line-breaks` — Writing a Go comment: one sentence per line, checked by `lyx loom lint-comments` alone. (test) — [background](pattern/PATTERN-comment-line-breaks.md)
- `PATTERN-documentation-lifecycle` — Deciding which docs are kept: keep no design doc for unbuilt work; a built design lives in its package's `doc.go`. — [background](pattern/PATTERN-documentation-lifecycle.md)
