## Constraints — do this before you judge anything

- These are the repo's rules; read them in full before forming any judgment:

# PATTERN

Structural invariants, one line per entry: when it applies and what it requires.

## Paths and geometry

- `PATTERN-cwd-resolution` — Resolving a cwd or worktree root: only `internal/lyxcwd`; nothing else calls `os.Getwd` or `git rev-parse`. — [background](pattern/PATTERN-cwd-resolution.md)
- `PATTERN-told-geometry` — Writing an engine: handed its absolute paths, it derives none and never imports `internal/lyxcwd`. — [background](pattern/PATTERN-told-geometry.md)
- `PATTERN-lyxdirs-single-declarer` — Naming `_lyx` or `.lyx` in a path: only `internal/lyxdirs` declares them, bar one variable block atop a `tools/` script.
- `PATTERN-lyx-temp-root` — Creating a temp file or dir: under `internal/lyxtemp`'s run dir, never `os.TempDir()`. (test) — [background](pattern/PATTERN-lyx-temp-root.md)
- `PATTERN-durable-vs-ephemeral-state` — Adding a file: tracked under `_lyx`, never-tracked under `.lyx`, same subpath. — [background](pattern/PATTERN-durable-vs-ephemeral-state.md)
- `PATTERN-hub-containment` — Linking hub-level containers or writing the hub workspace file: from the hub only. — [background](pattern/PATTERN-hub-containment.md)
- `PATTERN-hub-wide-config` — A hub-level config module: `HubWide` in `configreg`, its file on the board only. — [background](pattern/PATTERN-hub-wide-config.md)
- `PATTERN-sparse-hub-base` — Writing a hub's base file: only the units the hub tuned, never a template fill; only `hubreconcile`'s one-time walk strip drops a unit the template holds. — [background](pattern/PATTERN-sparse-hub-base.md)
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
- `PATTERN-push-both-sides` — Pushing run records from Go: the code branch goes along. — [background](pattern/PATTERN-push-both-sides.md)
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
- `PATTERN-recipe-setting-lookup` — Giving a recipe row a value: never its own `Env` field. — [background](pattern/PATTERN-recipe-setting-lookup.md)
- `PATTERN-command-row` — Adding a side-action row to a recipe: a `Command` row over its config keys, its command read from the template and board default layer only, never a new engine.
- `PATTERN-shed-verb-set` — Adding a run, step, status, pause or goto verb: generic bodies only in `internal/shedverbs`. (test) — [background](pattern/PATTERN-shed-verb-set.md)
- `PATTERN-shed-driver` — Driving a shed run in Go, deciding its stops or spawning a tackler: only `internal/sheddriver`. (test) — [background](pattern/PATTERN-shed-driver.md)
- `PATTERN-shed-run-directory` — Touching run directories or `seed.json`: `internal/shedrun` alone declares their paths, ids and `Seed`. — [background](pattern/PATTERN-shed-run-directory.md)
- `PATTERN-tolerant-records` — Go-written JSON records: never refuse an unknown key; rewrite via `internal/state`. (test) — [background](pattern/PATTERN-tolerant-records.md)
- `PATTERN-transient-stop` — Marking a failure transient: the `internal/shedengine` mark. — [background](pattern/PATTERN-transient-stop.md)
- `PATTERN-failure-stop-visible` — An `--until-stop` failure as sole lock holder: status is non-running before the waiter returns. (test) — [background](pattern/PATTERN-failure-stop-visible.md)
- `PATTERN-driver-choice-single-site` — Reading a seed driver: once per recipe, in its bootstrap verbs. (test) — [background](pattern/PATTERN-driver-choice-single-site.md)
- `PATTERN-treadle-runner-seam` — Importing into `internal/treadleengine`: only its allowlist, never `burlerengine`, `lyxcwd` or an `internal/*cli` package.
- `PATTERN-plan-generation` — Archiving a plan: `_lyx/plan/`'s top-level files hold one generation. — [background](pattern/PATTERN-plan-generation.md)
- `PATTERN-ref-shape-registry` — Classifying a plan ref or `plan:` handle: `internal/planparser` alone declares and decides. (test) — [background](pattern/PATTERN-ref-shape-registry.md)
- `PATTERN-glyph-conversion-chokepoint` — Converting glyphs and paths: only via `glyph`. — [background](pattern/PATTERN-glyph-conversion-chokepoint.md)
- `PATTERN-gate-self-check-parity` — Adding a mechanical gate: its closure and CLI self-check call one package function. — [background](pattern/PATTERN-gate-self-check-parity.md)
- `PATTERN-verified-tree` — Running a verify command: only via `verifytree.Verify`. (test) — [background](pattern/PATTERN-verified-tree.md)
- `PATTERN-gate-slot` — Running a gate build or test: only in held `internal/gateslot` slots. — [background](pattern/PATTERN-gate-slot.md)
- `PATTERN-trimpath-safe-root` — Locating the module root in code a gate run builds: never from a `runtime.Caller` path alone, which `-trimpath` makes module-relative.
- `PATTERN-batcher-registry` — Choosing webster's execution unit: the batch the active `batcher.yaml` profile derives. — [background](pattern/PATTERN-batcher-registry.md)
- `PATTERN-review-round` — Review and fix round: review accepted on disk before any edit, each finding fixed or disputed, end on a verdict. — [background](pattern/PATTERN-review-round.md)
- `PATTERN-sole-parsers` — Parsing plan, discussion, summary, recipe or halt note: only its sole parser. (test) — [background](pattern/PATTERN-sole-parsers.md)

## Agents and prompts

- `PATTERN-agent-name` — Forming an agent name: only `internal/agentname`, once by reed. — [background](pattern/PATTERN-agent-name.md)
- `PATTERN-creel-store` — Writing the creel journal or reaching a pane: only `internal/creelengine`. (test) — [background](pattern/PATTERN-creel-store.md)
- `PATTERN-stencil-ownership` — Reading a prompt or spec: at call time via `internal/stencilstore`, never embedded. — [background](pattern/PATTERN-stencil-ownership.md)
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
- `PATTERN-spawn-observability` — Starting an OS process from `lyx`: log the spawn via `internal/logger`. — [background](pattern/PATTERN-spawn-observability.md)

## Daemons and waits

- `PATTERN-daemon-wakeups` — An unbounded wait wakes at most once a second and backs off while idle. (test) — [background](pattern/PATTERN-daemon-wakeups.md)
- `PATTERN-log-levels` — Logging in a loop or poll: follow `internal/logger`'s Level policy.

## Packages

- `PATTERN-leaf-packages` — Importing into a leaf package: only its closed import set. — [background](pattern/PATTERN-leaf-packages.md)

## Build and tooling

- `PATTERN-sandbox-coverage` — Registering a lyx module: the sandbox suite exercises it or excludes it with a reason.
- `PATTERN-dev-prod-binary-separation` — Resolving `lyx` in sandbox tooling: `resolveLyx` (`.dev-bin` first, then PATH), never bare PATH.
- `PATTERN-quarry-cgo` — Building this module: `CGO_ENABLED=1` and a C compiler, for tree-sitter grammars. — [background](pattern/PATTERN-quarry-cgo.md)

## Testing

- `PATTERN-test-tiers` — Running a hub's tests: only through `lyx gate test`'s tiers. — [background](pattern/PATTERN-test-tiers.md)
- `PATTERN-test-economy` — Writing a test: it pins public-surface behavior no test covers yet, else it extends one. — [background](pattern/PATTERN-test-economy.md)
- `PATTERN-test-speed` — Writing a test: lowest fitting tier, injected clocks, `t.Parallel` by default. (test) — [background](pattern/PATTERN-test-speed.md)
- `PATTERN-test-isolation` — Testing a package: git spawners use `gitkit.HermeticGitEnv()`. (test) — [background](pattern/PATTERN-test-isolation.md)
- `PATTERN-testkit` — Sharing test support across packages: one kit under `internal/testkit/<kit>/`, imported only from tests, never duplicated. (test) — [background](pattern/PATTERN-testkit.md)
- `PATTERN-hubforge-fixtures` — A hub fixture no fake can test: `hubforge` via `fabriccli.CloneAndWire`, never hand-assembled or wrapped. — [background](pattern/PATTERN-hubforge-fixtures.md)

## Docs

- `PATTERN-markdown-link-integrity` — Linking in a `.md` file under `docs/`: every inline link resolves. (test) — [background](pattern/PATTERN-markdown-link-integrity.md)
- `PATTERN-comment-line-breaks` — Writing a Go comment: one sentence per line. (test) — [background](pattern/PATTERN-comment-line-breaks.md)
- `PATTERN-documentation-lifecycle` — Design docs: none for unbuilt work; a built one lives in its package's `doc.go`. — [background](pattern/PATTERN-documentation-lifecycle.md)
### Declared PATTERN changes

This task declares these changes to PATTERN entries, and its own diff makes them:

- `PATTERN-owner-machine` — adds: Starting, resuming or landing a run, bar the rest of a Finalize already past its parent-side merge, or acting on an entry after a wake: only on a machine that does not read the entry as owned by another, where a run-held entry with no owner recorded, and every entry of a hub with no prime owner recorded, reads as owned by no other machine, and any other entry with no owner recorded reads as owned by its parent's owner; its owner written only by `boardengine`'s owner setter; the owner verdict that gates a resume, a wake or a landing judged against origin inside `ReconcileOrigin`, a start's read from the Bolt it just pulled, the re-read after an owner write's push from the Bolt it just pushed, the landing's parent-owner read from the Bolt its reconcile left, and a resume with origin unreachable reading the local Bolt; this machine's name only from `internal/machinename`.

- An entry listed here is followed and judged by its listed new text, never by the overview's current text.
- An entry listed as `removes` no longer binds.
- A departure from a listed entry cites the listed text.
- Only the entries listed here are judged by their listed text; every other entry is judged by the overview's current text.

- Read every background file under pattern/ that the entries above link to and that touches what you are about to judge.
- Every violation of a listed constraint, an entry the declared changes list judged by its declared text, is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
## Focus directive for this round

Read the file at `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/instr/focus.md` before forming any finding.
It names where to look and which question to settle this round.

How the directive relates to the rubric:

1. The rubric binds over the focus directive.
   A directive never licenses a finding the rubric's `Do not flag` list or symmetry rule forbids, and never suppresses a finding the rubric requires.
2. A directive steers attention and order, not verdicts.
   "The concrete instance to judge" means reach it and state a verdict on it, and "not a defect" is a valid verdict.
3. Severity comes from the rubric's mapping and the evidence you find.
   A severity cap in a directive is advisory and yields to evidence the round turns up.
4. `exclude_lenses` keeps its mechanical meaning: it trims the cluster fan, and this rule does not govern it.

The directive never licenses reading a prior round's files before your review is saved.
A directive that points into one is followed only as far as its own text goes, and you record the gap under `## Focus departures`.

If you depart from a directive, record it in the review file's `## Focus departures` section, as the review-file format describes.
## What to review (the target)

- `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/_lyx/plan` (a directory — everything under it)

## What to judge it against (the fasit)

- `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/_lyx/discussion/decision-record.md`

The decision record named above is the answer key: the plan's job is to implement the decisions and constraints it settles. With no rework generation, the decision record alone is the answer key. The format authority is /home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md, and the mechanical checks over that format are already enforced by this round's own gate over this round's own output, so re-deriving them here is duplicated work. This round may never write the approved: key in the plan's overview file.

The fasit is the source of truth the target is judged AGAINST.
A review that ignores the fasit degenerates into a pure internal-consistency check — always read it and hold the target to it.

## Rubric

# Plan-Review rubric

The subject under review is the current plan: `_lyx/plan/00-overview.md` and the card files its Card Index names.
The plan directory may also hold `archive-*/` subdirectories, which are rotations of superseded plans;
they are out of scope, and a finding raised against one is never legitimate.

The format contract, and the Card model it implements, are both `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md`.
This rubric points at both and restates neither.
The mechanical checks over that contract are already enforced by this round's own gate over this round's own output — every check but `plan-unapproved` and `delete-target-gone` — while `plan-unapproved` is enforced at no row at all, resting on the review segment's own approve seam failing loudly if ever wired nil, and `delete-target-gone` fires only at Webster's dispatch, after this round.

`Plan-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
Sitting behind a mechanical gate over this round's own output makes this gate's over-flagging surface larger than that of a gate with no validator ahead of it, not smaller.

**`support-log.md` is outside this review entirely.**
It appears in neither the artifact list nor the answer key, and it must not be read or reasoned from.
`Plan-Write` provably never reads it, so a finding grounded in its content cannot be satisfied except by inventing the missing link.

## Write surface

none

`none` means webster has begun no batch, and no card is frozen.
Otherwise a done card is frozen: a fix never edits its file.
A fix to its work is a follow-up card placed after the last begun batch, with its Card Index line in `00-overview.md`, in the format `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md` defines.
A fix inserts or removes a card only with `lyx loom insert-card` or `lyx loom remove-card`, which renumber the later cards and whose `--help` says what they do, never by renaming, rewriting under a new name or deleting a card file by hand.

## Do not flag

Do not flag any of the following as a finding:

- **Anything this round's own gate already checks.**
  Every check ID `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md`'s own validation-checks section lists is enforced deterministically by this round's own gate over this round's own output, except `plan-unapproved`, which is enforced at no row at all, resting on the review segment's own approve seam failing loudly if ever wired nil, and `delete-target-gone`.
  `delete-target-gone` fires only at Webster's dispatch, after this round, so no plan this round reviews can carry it.
  Re-deriving any of them here is duplicated work whose only possible outcome is disagreement with the gate.
- **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.**
  Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
  Plan-time completeness of that intersection is explicitly not provable;
  the real gate is the post-merge build and test.
- **A `Rename`, `Move`, `Prosa`, or `Custom` card carrying no `ImpactSummary`.**
  It is required for `Edit` and `Delete` only, per the per-type table in `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md`.
  For `Rename` the reason is specific: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

## Also flag

- **Granularity.**
  One card per independently reviewable unit, not one card per literal symbol.
  A private supporting type, or a constructor inseparable from its type, belongs in the other symbol's card;
  a symbol that is part of a module's public surface, or reused across packages, gets its own card even when one card is its only consumer, and a helper serving one surface is bundled into that surface's card.
- **Test economy.**
  A card that plans a new test function or file for behavior an existing test already covers, or one test per symbol, is a finding; so is a card that plans an assertion on a stencil's static prose.
  The finding names the existing test that covers the behavior, or, for a per-symbol test, the public-surface test that covers the symbol;
  a vague "seems redundant" is not a finding.
- **Member glyphs on an `Edit` card.**
  An `Edit` card whose targets are only Go file paths of non-test files while its `Intent` names specific symbols is a finding; the finding names the member glyphs to list instead.
- **The re-sign arrow.**
  An `Edit` member glyph without the re-sign arrow, while the card's `Intent` changes that member's parameters, results, type parameters or receiver, is a finding; the finding names the arrow to add.
  It never fires for an interface method or a struct field, which take no arrow.
- **`ImpactSummary` carries a real conclusion.**
  A one-line blast-radius conclusion — "3 callers, all local to the billing package, no cross-module effects" — never a restatement of `Intent`.
- **`Custom` is a last resort.**
  Used only where none of `Create`, `Edit`, `Delete`, `Rename`, `Move`, or `Prosa` genuinely fits, never as a shortcut around correct typing.
  A `Custom` card is exempt from `path-missing` on its own targets and from `prosa-symbol-target` — which under the glyph alphabet means a `Prosa` group may only target file and unit self glyphs, with a member glyph (or anything else that fails to parse as a self glyph) the finding — so a mistyped `Custom` card silently escapes two checks the rest of the plan is held to.
  It escapes only those two: `bare-symbol-target` and `directory-target` bind a card's flat `Targets`/`Uses` with no group scoping at all, so this round's own gate already blocks a `Custom` card carrying either — do not hunt for one here.
  A `Custom` card whose targets could instead be expressed as a multi-label combination of the other six is a finding — the format's one-or-more-labels grammar means `Custom` is never the only way to name a mixed target list.
- **PATTERN fit.**
  When the repository has a `PATTERN.md`, each card is checked against its entries and against the `pattern/` background files the plan names as relevant to the change;
  a card that breaks one of their rules is a finding, and the finding names the entry.
  In a repository without a `PATTERN.md` this check does not apply.
- **Fidelity to the answer key.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope the answer key does not license.
  A Decision step no card carries is accepted only when the step changes this task's own board entry's body or scope and the decision record carries an `Added after Discussion` entry recording its drop;
  any other uncarried Decision stays a finding.
  That path is anchor-relative: it resolves from this session's own working directory, and it is deliberately not the absolute form the artifact list uses.
  The answer key is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record or a findings file.
  An entry between `<!-- lyx:carry-over … -->` marker lines in the decision record's `## Open risks` is not part of the answer key, and it licenses no card's scope.
  In a rework generation the live generation's `findings.md` joins the decision record as the answer key:
  every finding in it is covered by some card, and every card's scope is licensed by the decision record or by a finding.
  A finding in `findings.md` is a reviewer's claim the rework plan answers, never a ruling;
  it licenses a card's scope unless it overturns a Decision of the decision record, and a card carrying a finding that overturns a Decision with no `Added after Discussion` entry recording it is a finding.
  The live generation's round is, among `_lyx/loom/rework/round-<N>/` directories, the highest `N` whose `record.json` carries a `class` and whose `first_card` equals the `first_card` in `_lyx/plan/00-overview.md`'s frontmatter (absent means `1`).
  With no such round (generation 0, or a fresh Plan-Write plan after an operator `goto Plan-Write`), the decision record alone is the answer key.
  The round's `prior-generation/` archive is context only, and a finding raised against anything inside it is never legitimate.
- **A card that carries a research step.**
  A card that leaves a fact the design depends on to be found out while it runs, or carries a rule to halt when its work discovers such a fact, is a finding.
  The finding names the fact;
  its fix is the fact researched and recorded in the plan, or in the decision record through the parent when the answer changes the design.
- **A card that edits the task's own board entry.**
  A card that changes this task's own board entry's body or scope is a finding, because the run's board lock refuses such a change.
  A card writing another entry, the entry's status or its `run_settings`, or changing board-module code is not flagged by this bullet.
- **Card order.**
  A card that depends on something a later card does breaks the rule in `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md`'s "Plan vs. schedule" section; the finding names the card and the later card it depends on.
- **Verify lines.**
  A card's `**Verify:**` line naming a raw test command is a finding, because the gates run the hub's tier lists over scopes lyx computes and a line that tests is one `lyx gate test --wait` call with a tier list and a scope form.
  The hub's tiers and gate lists, as the plan writer was given them:
none
  A plan-level `## verify:` section is accepted and ignored, so it is no finding.
- **The attack-surface question.**
  For every new verb, flag, escape hatch or routing edge, and every guard that is removed, downgraded or bypassable, the card that introduces the change states in its `**Intent:**` what it can now skip or let through and what bounds it.
  A card that introduces one with no stated bound is a finding.
- **Reachability of a prescribed refusal.**
  A card that prescribes a refusal or guard branch with no named input that reaches it, or no named test that reaches it, is a finding.
  So is one whose named input an upstream check rejects; the finding names that upstream check.
  A propagated raw I/O error, which `refusal-spec.md` exempts as transient, is exempt, and so is a fail-loud wiring guard in that file's sense, which takes no reaching test.
  A test that calls only the refusal's error constructor does not reach the refusal.
- **The writer/reviewer symmetry note.**
  The plan writer's own stencil is `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/stencils/loom/loom-template-plan.md`.
  Whatever it says not to write, this rubric must not flag as missing.

## Proposed fixes

A finding whose fix adds, drops, moves or pairs targets, or changes a verify command, proposes only a fix that every check in the validation-checks section of `/home/knatte/Code/loomyard-LYMHUB/codex-astra-context-parity-bench/.scratch/bench/inputs/P4/work/.bench/specs/loom/loom-plan-spec.md` admits.
A fix that adds or pairs targets must pass `redundant-package-target` and `redundant-file-target` there, since a file or a package listed beside a member of it on one card is refused, and the spec says what counts and the way forward.
When the reviewer sees no admissible fix, it still reports the defect and names the check that constrains the fix.
A finding's `fix` key is a proposal: the fixer may apply any other fix the checks admit, and when none exists it defers the finding with the constraining check as the reason.
The rule narrows only which fix shapes a reviewer proposes; no defect becomes unreportable.

Before writing a finding whose `fix` adds, drops, moves or pairs targets, or changes a verify command, the agent writing the review file checks the fixes against the plan gate with `lyx loom validate-plan --overlay`, which checks proposed plan file contents in memory and writes no file.
`lyx loom validate-plan --help` shows the stdin format; this rubric does not restate it.
It first runs `validate-plan --overlay` with `{}` as a baseline, never the flag-less run, so that no run writes a file.
It then runs it once with every such fix applied together, with its Bash timeout raised to cover a type load, and proposes no fix whose run reports a blocking finding the baseline does not.
When the combined run reports a new blocking finding, it re-runs with each fix overlaid alone.
A fix whose solo run reports a new blocking finding is withheld, and its defect is reported with the constraining check named, as above for a defect with no admissible fix.
A new blocking finding of the combined run that no solo run reproduces is a joint conflict: the fixes that finding names by card are withheld, and their defects are reported together the same way.
A fix that inserts, removes or renumbers cards is not overlaid, since it goes through `insert-card` and `remove-card`.
The directive binds writing a finding's `fix`, not a Bouncer row's focus file.

A finding whose proposed glyph stands for a comment or a line it cites names the function or method whose body encloses that line, confirmed with `lyx quarry resolve` that the line lies inside the glyph's span, never the member after or near it.
When no member span holds the line, as for a free-standing comment between declarations, it names the file's self glyph.

## Amendment

none

When the note above is not `none`, this round reviews an amendment: review only the change it names, and report nothing about the settled remainder.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the plan's structure is wrong: batching, sequencing, sizing or `verify:` correctness.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.

The rubric tells you what counts as BLOCKING, MEDIUM, LOW, or NIT for THIS target.
It maps its own criteria onto that fixed four-value severity vocabulary;
it never introduces a new severity name, and neither do you.
Each finding also carries one of the four classes — `design`, `scope`, `decision`, `consistency` — and the rubric says what `design` means for this target.

## Tool-use rules — how you gather evidence

Read-only analysis in the explore and review steps: read files, run nothing.

## Reading for the fork reviewers

This round fans out: in step 2 you spawn fork reviewers, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what the review will need, once, instead of leaving each fork to read it again.

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- for each symbol the target changes, guards or removes: its direct callers and callees, and every path that reaches it from another process, such as a child the code spawns or a command that calls the guarded verb in-process;
- for each state, setting or file a decision introduces or changes: the existing code paths that also write, reset or reapply it, such as other verbs, hooks, resize or apply steps, and restart paths, and the code that reads it;
- for each output, key, string, file shape or behaviour the target changes: every test, caller, doc and fixture that reads or asserts it;
- the board entry of the task the target belongs to, through `lyx board get`;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- the failure, restart and resume paths of each change the target makes.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.

Close this step with a reading manifest: one line per file and symbol you read, grouped by the part of the target it serves.
The manifest names what you read and nothing about what you found.
Write it as text in the same turn and go straight on to step 2: the manifest never ends your turn.
