# PATTERN

The structural invariants of the loomyard code, one line per entry: when it applies and what it requires.

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd or worktree root: only `internal/lyxcwd` does it, and nothing else calls `os.Getwd` or `git rev-parse --show-toplevel`. — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-told-geometry` — Writing an engine: it is handed the absolute paths it works on and derives none, so it never imports `internal/lyxcwd`. — [background](pattern/PATTERN-told-geometry.md)
- `PATTERN-lyxdirs-single-declarer` — Naming `_lyx` or `.lyx` in a path: only `internal/lyxdirs` declares those literals, and no other production file spells them.
- `PATTERN-durable-vs-ephemeral-state` — Adding a file: tracked content lives under `_lyx`, anything never tracked under `.lyx` at the mirrored subpath. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-hub-containment` — Linking hub-level containers: `_board`, `_portals` and `_launchers` are reachable from the hub only, never junctioned into a worktree. — [background](pattern/PATTERN-hub-containment.md)
- `PATTERN-hub-suffix` — Touching hub naming: `-LYXHUB` is the sole hub container suffix, and no code parses, trims or recognises the retired `-HUB`. (test) — [background](pattern/PATTERN-hub-suffix.md)

## CLI

- `PATTERN-cli-cobra` — Adding a CLI module: a cobra subtree mounted under one root in `cmd/lyx/main.go`, with `Command`, `RunCLI`, a `Short` and JSON errors. (test) — [background](pattern/PATTERN-cli-cobra.md)
- `PATTERN-cliwire-sole-wiring` — Wiring a standalone-capable CLI: `internal/cliwire` alone resolves target dir, repository root and state; a module declares a descriptor. (test) — [background](pattern/PATTERN-cliwire-sole-wiring.md)
- `PATTERN-config-strictness` — Loading config: a caller adopts exactly one of `configengine.Load` (strict) or `LoadOrTemplate` (degrades to the embedded template). — [background](pattern/PATTERN-config-strictness.md)
- `PATTERN-refusal-way-forward` — Adding a refusal in webster, shed or loom: its message names the way forward, and its spec row and reaching test land in the same commit. — [background](pattern/PATTERN-refusal-way-forward.md)

## Fabric and git

- `PATTERN-fabric-vocabulary` — Naming the wired composite: "fabric"; warp and weft only where the two sides must be told apart, and `host` is retired. — [background](pattern/PATTERN-fabric-vocabulary.md)
- `PATTERN-fabric-git` — Running git on warp or weft: only `internal/fabricengine`, in-process, never raw git; the weft commit is Go with a scoped pathspec. — [background](pattern/PATTERN-fabric-git.md)
- `PATTERN-fabric-destruction-chokepoint` — Destroying anything in fabric: only `internal/fabricengine/destroy.go`, checking containment, ownership, dirtiness, force in that order. — [background](pattern/PATTERN-fabric-destruction-chokepoint.md)
- `PATTERN-fabric-write-containment` — Writing under `_launchers` or `_portals` from `fabricengine`: through an `os.Root` rooted at the hub, never raw `os.MkdirAll`, `os.WriteFile` or `fslink`.
- `PATTERN-mutation-record` — Adding a mutating fabric verb: it accumulates a `*Mutations` record and its result embeds `MutationRecord` under a fixed envelope key set. — [background](pattern/PATTERN-mutation-record.md)
- `PATTERN-pair-teardown` — Tearing down a pair: only through `internal/pairteardown`, which ends the pair's reed session before any worktree is removed. (test) — [background](pattern/PATTERN-pair-teardown.md)
- `PATTERN-batten-bookend` — Creating or destroying a task worktree: the producer never runs from inside it, and session shutdown precedes removal in one row. — [background](pattern/PATTERN-batten-bookend.md)
- `PATTERN-github-auth` — Calling GitHub: all authentication goes through `internal/githubclient`, and no other production package shells out to `gh`.
- `PATTERN-gitrepo-client-boundary` — Reading or mutating git state in `internal/gitrepo`: go-git owns local reads, `gitexec` owns remote-authenticating or tree-mutating work. (test) — [background](pattern/PATTERN-gitrepo-client-boundary.md)
- `PATTERN-gitexec-checked-call` — Running git: use `gitexec.Run`/`runChecked`; the raw `RunGit`/`r.run` forms survive only at pinned `//gitexec:raw` call sites.
- `PATTERN-never-force-add` — Keeping transients out of the index: each repo's own `.git/info/exclude`; fabric and gitrepo never run `git add -f`.

## Shed and loom

- `PATTERN-shed-producer-seam` — Importing into `internal/shedengine`: only stdlib, `state` and `lock`, and its status and lock paths are caller-supplied.
- `PATTERN-shed-recipe-registry` — Registering a shed producer: one `map[string]Constructor` in `internal/shedrecipe`, reached through `Lookup`/`Names`, with no `init()`, no `Register`, no `lyxcwd`.
- `PATTERN-shed-verb-set` — Adding a run, step, status, pause or goto verb: the generic bodies live in `internal/shedverbs` only, which derives no path. (test) — [background](pattern/PATTERN-shed-verb-set.md)
- `PATTERN-shed-run-directory` — Touching run directories or `seed.json`: `internal/shedrun` is the sole declarer of the `shed` segment, the run-id vocabulary and the `Seed` codec. — [background](pattern/PATTERN-shed-run-directory.md)
- `PATTERN-transient-stop` — Marking a failure transient: the mark is declared in `internal/shedengine` and set only at a producer or step-bootstrap boundary, never on a verdict. — [background](pattern/PATTERN-transient-stop.md)
- `PATTERN-driver-choice-single-site` — Reading a recorded seed driver: once per recipe, in its own bootstrap verb, selecting the driving surface and nothing else. (test) — [background](pattern/PATTERN-driver-choice-single-site.md)
- `PATTERN-treadle-runner-seam` — Importing into `internal/treadleengine`: never `burlerengine` or an `internal/*cli` package, and only its allowlist, not `lyxcwd`.
- `PATTERN-plan-generation` — Archiving a plan: `_lyx/plan/`'s top-level files hold exactly one generation, and a retired one lives under `round-<N>/prior-generation/`. — [background](pattern/PATTERN-plan-generation.md)
- `PATTERN-ref-shape-registry` — Classifying a plan ref or `plan:` handle: `internal/planparser` is the sole declarer, and every decision routes through its kind-policy ledger. (test) — [background](pattern/PATTERN-ref-shape-registry.md)
- `PATTERN-glyph-conversion-chokepoint` — Converting between glyphs and paths: only `glyph.Self`, `Glyph.UnitPath`, `glyph.Parse` and `Glyph.String`, never trimming, regex or disk reads. — [background](pattern/PATTERN-glyph-conversion-chokepoint.md)
- `PATTERN-gate-self-check-parity` — Adding a mechanical gate: its closure and its CLI self-check verb call the same package function, and both land in one task. — [background](pattern/PATTERN-gate-self-check-parity.md)
- `PATTERN-verified-tree` — Running a plan's verify command: through `verifytree.Verify` only, never on a dirty tree, skipping only on a record of HEAD's tree. (test) — [background](pattern/PATTERN-verified-tree.md)
- `PATTERN-batcher-registry` — Choosing webster's execution unit: the batchifier-derived batch, selected by `internal/batcher`'s registry and `batcher.yaml`'s `active:` key.
- `PATTERN-review-round` — Running a review and fix round: review on disk before any target is touched, every finding fixed, converged only on a judge verdict. — [background](pattern/PATTERN-review-round.md)
- `PATTERN-sole-parsers` — Reading or writing plan, discussion, summary or recipe files: only `planparser`, `discussionparser`, `summaryparser` and `shedbuild` parse them. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-agent-name` — Forming or parsing an agent name: only `internal/agentname`, as `<shortname>:<role>` or `<shortname>:<slug>:<role>`, formed once by reed. — [background](pattern/PATTERN-agent-name.md)
- `PATTERN-stencil-ownership` — Reading a producer prompt or normative spec: from a told absolute directory at call time through `internal/stencilstore`, never embedded bytes. — [background](pattern/PATTERN-stencil-ownership.md)
- `PATTERN-producer-pointer-rule` — Writing an instruction file: it points at another producer's format contract and never duplicates or paraphrases it.
- `PATTERN-friction-capture` — Halting a loom run or refusing in webster: a Go-authored friction note is written, and none is archived before a reflection covers it. (test) — [background](pattern/PATTERN-friction-capture.md)
- `PATTERN-completion-signal` — Finalizing a negative "did this run finish" answer in `internal/shuttleengine`: consult `allOutputFilesExist` first. (test) — [background](pattern/PATTERN-completion-signal.md)
- `PATTERN-shuttle-provider-seam` — Referencing a provider: its specifics live only under `internal/shuttleengine/claudeengine`, never in `shuttleengine` or `reedengine`. — [background](pattern/PATTERN-shuttle-provider-seam.md)
- `PATTERN-shell-mechanics-seam` — Building a pane-shell command string: only through `internal/shell`, which imports the standard library alone.
- `PATTERN-pane-binary-resolution` — Creating a strand pane in reed: it resolves `lyx` to the spawning binary through the one chokepoint in `panebin.go`. (test) — [background](pattern/PATTERN-pane-binary-resolution.md)
- `PATTERN-spawn-observability` — Starting a real OS process from a `lyx` command: log the spawn, and the teardown where it waits, via `internal/logger`. — [background](pattern/PATTERN-spawn-observability.md)

## Packages

- `PATTERN-leaf-packages` — Importing into `gitkit`, `modelspec`, `tokenvocab`, `buildinfo`, `standalonestate`, `pattern` or `friction`: each admits a closed import set. — [background](pattern/PATTERN-leaf-packages.md)

## Testing

- `PATTERN-testkit` — Sharing test support between packages: one kit under `internal/testkit/<kit>/`, imported only from tests, never duplicated or placed under the faked package. (test) — [background](pattern/PATTERN-testkit.md)
- `PATTERN-hubforge-fixtures` — Building a hub fixture: `internal/hubforge` through `fabriccli.CloneAndWire`, never hand-assembled and never wrapped in a test-local type. — [background](pattern/PATTERN-hubforge-fixtures.md)
- `PATTERN-test-tier-purity` — Writing an untagged test: no git, process, tmux or `lyxbin` spawn and no sleep of a second or more; those need `integration` or `smoke`. (test) — [background](pattern/PATTERN-test-tier-purity.md)
- `PATTERN-hermetic-git-tests` — Testing a package that spawns git: its `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted. — [background](pattern/PATTERN-hermetic-git-tests.md)
- `PATTERN-tmux-test-isolation` — Testing a package with an `integration` or `smoke` file: every `TestMain` calls `tmuxkit.Main`, so no test touches the default tmux socket. (test) — [background](pattern/PATTERN-tmux-test-isolation.md)
- `PATTERN-sandbox-coverage` — Registering a lyx module: the sandbox suite exercises it, or explicitly excludes it with a reason.
- `PATTERN-dev-prod-binary-separation` — Resolving `lyx` in sandbox tooling: `resolveLyx` (`.dev-bin` first, then PATH), never a bare-PATH lookup.
- `PATTERN-quarry-cgo` — Building this module: it needs `CGO_ENABLED=1` and a C compiler, because quarry links tree-sitter's C grammars and refuses a cgo-free build. — [background](pattern/PATTERN-quarry-cgo.md)

## Docs

- `PATTERN-markdown-link-integrity` — Linking in a `.md` file under `manifest/` or `docs/`: every inline link resolves, file part and `#anchor`. (test) — [background](pattern/PATTERN-markdown-link-integrity.md)
- `PATTERN-comment-line-breaks` — Writing or changing a Go comment: semantic line breaks, one sentence per line, with no column limit. — [background](pattern/PATTERN-comment-line-breaks.md)
- `PATTERN-documentation-lifecycle` — Deciding which docs are kept or deleted: see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).
