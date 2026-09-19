# Discussion: Seeded Shed core: run addressing, seed contract, batten

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
slug: seeded-shed-core
status: discussing
parent: main
```

## Problem

Shed is already the only execution machinery in lyx, and the previous task (`shed-generic-watchdog`) finished generalizing `run`/`step`/`status`/`pause` onto `internal/shedverbs` behind the `lyx shed` subtree.
What is still product-shaped is everything *around* the engine: a run has no address, so `lyx shed run --recipe loom` names a recipe rather than a run;
loom's status lives at `_lyx/loom/status.json` while lifecycle's lives at `.lyx/lifecycle/<slug>/status.json`, two unrelated layouts with opposite durability;
and the recipe a run executes is a CLI flag the operator retypes rather than a recorded property of the run.

Why now: the roadmap's next item after this one (`seeded driver choice: ly-drive strand as the child's driver`) binds directly to a seed field that does not exist yet, and `manifest/designs/seeded-shed.md` is the settled direction it is waiting on.
The durable-placement half also has a deadline of its own: the lifecycle recipe's `Loom-Run` row name becomes a durable on-disk identity the moment its status file moves under `_lyx`, so the rename to `Run-Shed` must land no later than the move — which is this task.

## Scope

**In:**

- A new leaf package `internal/shedrun`: sole declarer of the `shed` durable-directory segment and the run-id vocabulary (including the literal default `self`), and sole parser/writer of `seed.json`.
- The durable run directory `_lyx/shed/<run-id>/` holding `seed.json` and `status.json`, with the mirrored ephemeral `.lyx/shed/<run-id>/` holding `run.lock` and `status.json.lock`.
- Relocation of **both** existing status files onto that layout: loom's `_lyx/loom/status.json` → `_lyx/shed/self/status.json`, and lifecycle's `.lyx/lifecycle/<slug>/status.json` → `_lyx/shed/<slug>/status.json` (durable, in prime).
- Run addressing on the generic verbs: `lyx shed run|step|status|pause [<run-id>]`, defaulting to `self`, resolving the recipe from the addressed run's seed.
- Removal of the `--recipe` flag from the `lyx shed` subtree; `internal/shedcli`'s named-recipe table stays, now keyed by the seed's `recipe` value instead of a flag.
- A new seeding verb `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]`, idempotent and refusing to overwrite a disagreeing seed.
- A `type` field on the Board task record, empty meaning `loom`, plus the read seam that lets a producer resolve it.
- The lifecycle → **batten** rename in full: packages, embedded recipe file, CLI verb, row-name constants, and the CONSTRAINTS invariant heading.
- A new `Seed-Child` row in batten, between `Worktree-Create` and the renamed `Run-Shed`, that writes and commits the child worktree's `_lyx/shed/self/seed.json`.
- A step-friendly, re-entrant waiting form for `Run-Shed`, so a step-driven outer run is not held inside one blocking `Run` call for the child's whole duration.
- The `--parent` flag's absorption into the seed's params for the loom recipe.
- Doc updates in the same commit: `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/seeded-shed.md`, `manifest/roadmap.md`.

**Out:**

- `driver: llm`. The seed carries and validates a `driver` field, but only `go` is an accepted value in this task; `llm` is refused with a message naming the roadmap item that will implement it. That item ("seeded driver choice") owns the reed-strand spawn.
- Relay-stepping (`Run-Shed` executing `lyx shed step` in the child as a subprocess). Explicitly rejected in the design as the primary mode and not built here as a secondary one.
- The optional comfort rows (launching/closing VS Code around the child), named as "can come later" in the design.
- Hardener: no recipe, no registry entry, no artefact of any kind. `manifest/designs/hardener.md` still carries its DRAFT banner.
- `shedengine`, `shedbuild`, `shedrecipe`'s construction machinery, the status contract, the envelope contract, and the perch segments. This task adds addressing and a seed contract *around* them, never new machinery *in* them. In particular the two-value `Outcome` vocabulary (`Done`/`Stuck`) is not extended.
- A compatibility alias for the retired `lyx lifecycle` verb, and any on-disk migration of legacy run state.
- Board rendering/display of the new `type` field. It is a stored value, not a rendered column.

## Decisions

### one-package-for-addressing-and-seed

- Decision: a single new leaf package `internal/shedrun` owns both halves — the `shed` path segment and its five path constructors (`RunDir`, `SeedFile`, `StatusFile`, `RunLock`, `StatusLock`), the run-id vocabulary (`SelfRunID = "self"`, validation, and `List` over existing run directories), and the `Seed` struct with its `ReadSeed`/`WriteSeed` pair.
- Rationale: a run directory *is* its seed plus its status; splitting paths from the seed contract would mean two packages that only ever appear together, and two invariants where one does. It mirrors `internal/lifecyclecli/paths.go` and `internal/loomengine/config.go` in taking a `*lyxcwd.Location` and doing a plain `filepath.Join` onto `AnchorPath()`, which is what the Cwd Resolution Invariant requires of a module's own durable subdirectory.
- Rejected: separate `internal/shedpaths` + `internal/shedseed` (two packages, one concern); folding the paths into `internal/shedcli` (a CLI package would then own a durable on-disk layout that producers in `internal/battenshed` also need to construct, forcing an engine→cli import).

### locks-mirror-under-dot-lyx

- Decision: `seed.json` and `status.json` are durable under `_lyx/shed/<run-id>/`; `run.lock` and `status.json.lock` are ephemeral under `.lyx/shed/<run-id>/`, at the mirrored subpath.
- Rationale: the design text says the run directory holds "seed + status and the run's locks", which reads as one directory, but the Durable-vs-Ephemeral State Invariant is unambiguous — every never-tracked file lives under `.lyx` at the mirrored subpath of the `_lyx` content it relates to. loom already splits exactly this way today (`loomengine.LoomStatusFile` under `_lyx`, `LoomStatusLock` under `.lyx`), so this is the existing pattern applied to the new segment, not a new one.
- Rejected: locks under `_lyx/shed/<run-id>/` (would commit a lock file, or require a per-file exclusion the invariant exists to prevent).

### both-status-files-move

- Decision: loom's status moves to `_lyx/shed/self/status.json` and lifecycle's to `_lyx/shed/<slug>/status.json` in this task, and the old `_lyx/loom/` and `.lyx/lifecycle/` layouts are removed rather than retained alongside.
- Rationale: "one placement, always durable, prime included" is the design's central claim, and a half-move leaves two addressing schemes plus a rule for which applies when — strictly worse than either scheme alone. The prime-side move is the load-bearing half: on a machine switch mid-run the Board says a task is active, but only a durable batten status says the worktree is already created, the child already seeded, `Run-Shed` already in flight.
- Rejected: moving only lifecycle's (loom keeps a second layout forever, and `self` addressing then means nothing for the recipe that will use it most); moving only loom's (leaves the durability gap the design was written to close).

### self-healing-is-batten-rows-obligation

- Decision: making batten's status durable means its rows must self-heal machine-local resources a durable status promises but a cold machine lacks. `Run-Shed` re-resolves the child worktree on every entry and recreates it from its branch when absent, before watching its status.
- Rationale: named as the explicit cost in the design, and `loom start` already applies the same pattern to a cold task worktree, so the shape is precedented rather than invented here.
- Rejected: refusing on a cold machine and telling the operator to re-create the worktree by hand (turns the durability win back into a manual step, which is the thing being removed).

### run-id-positional-replaces-recipe-flag

- Decision: the four generic verbs take an optional positional run-id defaulting to `self`; `--recipe` is removed from `lyx shed` entirely. The verb reads `_lyx/shed/<run-id>/seed.json`, takes its `recipe` value, and looks that up in `internal/shedcli`'s existing arming table.
- Rationale: "the verb never knows what it starts — it reads the seed and looks the recipe up in the compiled-in registry" is the design's headline. Keeping both a flag and a seed creates a disagreement case with no good answer. The table itself survives untouched in shape — it still maps a recipe *name* to an *arming function*, which remains a different question from `internal/shedrecipe`'s engine-name→constructor registry.
- Rejected: `--recipe` retained as an override that must agree with the seed (a second source of truth whose only behaviour is to error); `--recipe` retained as a seeding default (conflates addressing with seeding, which the new `seed` verb owns).

### run-id-absent-refuses-with-a-listing

- Decision: when the addressed run-id has no `seed.json`, the verb refuses and names every run-id that does exist (directories under `_lyx/shed/` containing a readable `seed.json`, sorted), and never falls back to scanning for a single candidate. Run-ids are validated as a single path segment — no separator, no `.`/`..` — before being joined.
- Rationale: stated outright in the design ("it never guesses by scanning, because guessing is how the wrong run gets resumed"). The segment validation is not in the design but is required by the same reasoning the Cwd Resolution Invariant applies elsewhere: a run-id reaches this code from a CLI argument and from a Board slug, and both get joined onto an anchor path.
- Rejected: auto-selecting when exactly one run exists (the failure mode is resuming the wrong run silently, which is worse than a refusal the operator reads).

### seed-verb-plus-auto-seed-on-the-batten-entry-path

- Decision: add `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]`, which creates the run directory and writes `seed.json`, is idempotent against a byte-identical existing seed, and refuses a disagreeing one. Separately, `lyx batten run <slug>` auto-seeds prime's own batten run when absent, copying the recipe from the Board task's `type` and the driver from its flags.
- Rationale: something must write the first seed, and there are two distinct callers with different needs — an operator addressing an arbitrary run by hand, and batten's own entry path, which already seeds its status file when absent (`lifecyclePreRun`) and so has a precedented place to seed alongside it. `lyx loom start` likewise gains a seed write next to the status seed it already performs.
- Rejected: auto-seeding inside the generic verbs (would make "run" silently create a run, defeating the refusal decision above); a seed verb alone with no auto-seeding (every `lyx batten run` becomes two commands, and the Board's `type` field would then never be read by anything).

### params-are-string-keyed-and-recipe-validated

- Decision: `seed.json`'s `params` is a `map[string]string`. `internal/shedrun` validates only that keys are non-empty and the map decodes; which keys a recipe requires, and what they mean, is validated by that recipe's arming function.
- Rationale: keeps `shedrun` recipe-agnostic, which is the same split `internal/shedbuild` already makes between parsing a row's `config` map and letting each `internal/shedrecipe` entry validate the keys it reads. The params in play are branch names, slugs and driver names — all strings; a typed-per-recipe struct would put every recipe's vocabulary inside the one package that must not know them.
- Rejected: `map[string]any` (mirrors `shedbuild.Config`, but buys nothing here since no param is numeric or nested, and costs a type-assertion helper set); per-recipe typed structs in `shedrun` (inverts the dependency).

### parent-branch-moves-into-seed-params

- Decision: the loom seed carries `params.parent`, and `lyx loom start --parent <branch>` writes it there. The existing recorded-origin provenance (`fabricengine.Origin.ParentBranch`) stays the durable truth; the seed records what this run was started against, and `resolveParentBranch`'s existing disagreement refusal is reused unchanged when the two differ.
- Rationale: the design names `lyx loom start --parent` as one of the two loose ends of the verb extraction whose landing place is the seed's params. The seed is "the run's *recorded* startup choice, not the durable truth about the task" — which is exactly the relationship the recorded origin already has to the flag.
- Rejected: moving the durable parent record itself into the seed (the seed is per-run and the parent is per-pair; a second run in the same worktree would lose it).

### board-type-field-defaults-to-loom-and-is-validated-late

- Decision: add `Type string \`json:"type,omitempty"\`` to `boardengine.Task` and its store record. An empty value means `loom`. `boardengine` never validates the value against a recipe name; the validation happens where the seed is written, against `internal/shedcli`'s table.
- Rationale: `omitempty` plus an empty-means-`loom` rule makes every existing task record valid with no migration. `boardengine` knowing the set of recipe names would give the Board a dependency on the CLI arming table for a value it only stores — the Board's job is to carry the choice, and the seeder's job is to reject a choice that names nothing.
- Rejected: a non-pointer field with an explicit `"loom"` default written into every record (a migration for no gain); validating in `boardengine` (couples the Board to the recipe table).

### full-rename-no-alias

- Decision: rename in full and in one commit — `internal/lifecycleshed`→`internal/battenshed`, `internal/lifecyclerecipe`→`internal/battenrecipe`, `internal/lifecyclecli`→`internal/battencli`, `contracts/recipes/lifecycle-recipe.yaml`→`batten-recipe.yaml` (and its embed var), `lyx lifecycle`→`lyx batten`, `NameLoomRun`/`"Loom-Run"`→`NameRunShed`/`"Run-Shed"`, and CONSTRAINTS' "Lifecycle Bookend Invariant"→"Batten Bookend Invariant". No deprecated alias is kept.
- Rationale: the design fixes the timing — the rename lands no later than the move to durable state, and the move is in this task. lyx is pre-1.0 with a single operator and no external consumers, so an alias would only mean two names in the docs and two paths in the tests. The row-name constant's *value* is what matters for resume, and there are no in-flight lifecycle runs to break, because today's lifecycle state is per-machine and ephemeral — which is precisely the coordination point the design names.
- Rejected: hidden `lyx lifecycle` alias (two surfaces to document and test); renaming the recipe but not the packages (leaves the product name in exactly the places a reader looks first).

### no-migration-legacy-layouts-refuse-loudly

- Decision: no migration code. A leftover `.lyx/lifecycle/` or `_lyx/loom/status.json` is not read, not moved, and not deleted by lyx; the new verbs simply find no run and refuse with the run-id listing. The commit message and `docs/overview.md` state that landing requires no lifecycle or loom run in flight.
- Rationale: migration code for a state class that is per-machine, ephemeral, and recreated by the next `start` is code written to be deleted. The loom half is durable, but a task worktree mid-run at landing time is a coordination problem an operator resolves by finishing or discarding that run, not a problem code should guess its way through.
- Rejected: a one-shot migration that copies the old status into the new location (would have to guess the run-id, the recipe, and the driver — all three being exactly what the seed exists to record).

### run-shed-re-entrancy-via-self-pointing-on-stuck

- Decision: `Run-Shed`'s `Call` performs one status read per invocation. A terminal child state verdicts `Done` (child done) or `Stuck` with no self-route (child blocked/paused/failed — escalation, unchanged). A still-`running` child sleeps `poll_interval_s` and returns `Stuck`, and the recipe routes `on_stuck: Run-Shed` — the row's own re-entry — with `max_bounces` set to today's attempt cap. The existing `poll_attempts` config key is retired in favour of that budget. The default interval rises from 5s to 30s.
- Rationale: it delivers the design's "one status check per step with a re-entrant still-running outcome" without touching `shedengine`'s two-value `Outcome` vocabulary, which the design puts explicitly out of scope. The engine already treats `Stuck` with an `OnStuck` target as ordinary routing that persists `StateRunning`, and the perch pattern uses the same Stuck-to-offshoot shape deliberately. `run` mode is unaffected in wall-clock behaviour: it loops the same route internally with the same sleep inside `Call`.
- Known cost, stated rather than hidden: every re-entry appends a history entry and persists the status file, so a 12-hour child run accumulates ~1440 entries at a 30s interval. That is the reason for the interval change, and it is the weakest link in this decision.
- Rejected: a third `Outcome` value such as `Continue` (cleanest semantically, and the right answer if the reviewer disagrees with the cost above — but it changes `shedengine`, which the design excludes); a `wait_mode` config key selecting blocking-vs-single-poll (the same recipe serves both verbs, so the row cannot know which one drove it); two rows, spawn then watch (moves the same bounce-budget and history-growth problem one row over).

### seed-child-writes-and-commits

- Decision: `Seed-Child` is a new row and a new `SeedChild` registry entry in `internal/shedrecipe`. It reads the Board task's `type` through a new `Env` seam, takes the driver from batten's own seed params, writes the child's `_lyx/shed/self/seed.json`, and commits it to the child's weft pair through `internal/fabricengine`. Verdicts: `Done` on a successful write+commit; `Stuck` on an unknown recipe name, an unreadable Board, or a failed commit; a hard error on a path-resolution failure, matching `innerRunProducer`'s existing error-vs-verdict split.
- Rationale: the seed is durable content under `_lyx`, so leaving it uncommitted would break the very machine-switch case the durability decision exists for. Per the Fabric Git Invariant the commit is Go calling `fabricengine` in-process at a row boundary, which is exactly what this is. The row runs after `Worktree-Create`, so the child worktree exists; like every other seam in `internal/battencli/wire.go` it resolves the child's paths lazily inside its own closure body, never at wiring time.
- Rejected: writing the seed without committing (child seed lost on a fresh clone of the pair); folding the seed write into `Worktree-Create` (the design keeps `Worktree-Create` fabric-only, and a combined row would make a seeding failure indistinguishable from a creation failure).

### driver-field-present-but-only-go-accepted

- Decision: `seed.json` carries `driver`, defaulting to `go` when absent. `llm` parses but is refused at arming time with a message naming the roadmap item that implements it. `shedrun` declares both constants so the next task adds behaviour, not vocabulary.
- Rationale: the next roadmap item binds to this field, and shipping the field without its second value is what lets that item be a spawn-command change rather than a contract change. Accepting `llm` and silently running `go` would be worse than refusing.
- Rejected: omitting the field entirely (the next task then changes the seed format, which is the durable on-disk contract this task is establishing); implementing `llm` here (that is the next roadmap item's whole content, and it needs the reed-strand spawn seam).

## Technical context

**Where the pieces are today.**

- `internal/shedverbs` — the generic `run`/`step`/`status`/`pause` cobra bodies. Reads everything through a told `shedverbs.Spec` (`spec.go`): `StatusPath`, `LockPath`, `StatusLockPath`, `BuildShed`, six `Hooks`, and a set of told message strings. Derives no path and imports no resolver. Adding run addressing must not change that — the run-id is resolved by the *arming* side and reaches `shedverbs` as the same three told paths it reads today.
- `internal/shedcli` — `cli.go` builds the subtree and `resolvePersistentPreRun` resolves cwd, looks `--recipe` up in `table.go`'s `recipes` map, gates the verb against the entry's `Verbs` set, and calls `e.Arm(cwd, verb, args)`. This pre-run is where the seed read replaces the flag read: resolve cwd → resolve run-id from `args` (default `self`) → `shedrun.ReadSeed` → `lookup(seed.Recipe)` → verb gate → `Arm`. The `argsFor()` closure that today defers to the entry's `Args` needs rethinking, since the positional is now the run-id for every recipe rather than a per-recipe contract.
- `internal/loomcli/arm.go` and `internal/lifecyclecli/arm.go` — each exposes `Arm(cwd, verb, args) (shedverbs.Spec, error)` plus an unexported `arm` worker and a resolution-free `specFor(verb)`. Both fill `shedbuild.ShedPaths`. These are the two functions whose path-construction changes: they currently call `loomengine.LoomStatusFile(l)` / `StatusFile(l, slug)` and must call `shedrun.StatusFile(l, runID)` instead. `Arm`'s signature may need the run-id threaded explicitly rather than dug back out of `args`.
- `internal/lifecyclecli/paths.go` — the five prime-anchored constructors under `.lyx/lifecycle/<slug>/`, plus `PrimeRunLock`, the hub-scoped lock serialising every slug's create/teardown against one another. `PrimeRunLock` is *not* per-run and does not move into `shedrun`'s per-run-id layout; it stays a batten-owned path (under `.lyx/shed/` now, one level above any run-id, to keep the segment consistent).
- `internal/lifecyclecli/wire.go` — builds the `shedrecipe.Env` with four seam groups (`CreateWorktree`, `Teardown`, `InnerRun`, `PrimeLock`) and `shedbuild.ShedPaths` with `CommitStatus: nil`. Two changes land here: a fifth seam group for `Seed-Child`, and `CommitStatus` becoming non-nil now that batten's status is durable — `internal/loomcli/wiring.go`'s `newCommitStatusSeam`/`loomCommitStatusDeps` is the existing implementation to model it on (note its three-case error handling, which is not trivial).
- `internal/lifecycleshed/innerrun.go` — `innerRunProducer`, the row being renamed and made re-entrant. Its current shape is: resolve status path, log spawn, `deps.Spawn`, log wait complete, then poll `deps.ReadStatus` up to `pollAttempts` times with `deps.Sleep(pollInterval)` between. Its doc comment carries the full verdict table, which must be rewritten with the new outcomes. `deps.Now`/`deps.Sleep` nil-resolution to `time.Now`/`time.Sleep` happens once in the constructor and is what tests substitute.
- `internal/shedrecipe/entries_lifecycle.go` — `worktreeCreateEntry`, `innerRunEntry`, `worktreeTeardownEntry`. `innerRunEntry` reads `poll_interval_s`/`poll_attempts` via `configInt` and validates `Env.Slug`/`Env.ScratchDir`/`Env.InnerRun.*` via `requireNonEmpty`/`requireAbsRoot`/`requireSeam`. The new `SeedChild` entry belongs in this file (renamed `entries_batten.go`) — it shares the `Slug`/`ScratchDir` validation shape that is the stated reason these three are grouped apart from `entries_simple.go`.
- `contracts/recipes/lifecycle-recipe.yaml` — three rows. Its header comment explains that `Loom-Run`'s empty `on_stuck` is load-bearing (a stuck verdict escalates with the worktree intact, which is what makes the destructive teardown row unreachable from a failure path). The re-entrancy decision changes that `on_stuck` from empty to self-pointing, so **that comment's claim must be re-established a different way**: with `on_stuck: Run-Shed`, a genuinely stuck child no longer escalates by routing — it escalates by exhausting `max_bounces`, which `shedengine`'s `episodeStuckCount` path turns into `StateBlocked` at the same row. The recipe header must say so explicitly; this is the single easiest thing to get wrong in the whole task.
- `internal/boardengine/task.go` and `store.go` — two near-duplicate structs (`Task` and the store's own record) that both need the new field. `template.yaml`, `render.go` and `layer.go` are the places to check for anything that enumerates fields.
- `internal/fabricengine/junctionnames.go` — `structuralCommittedDirs` is `{_lyx}` and `structuralNeverCommittedDirs` is `{.lyx}`. **No change needed**: `_lyx/shed/` and `.lyx/shed/` are subpaths of names already in those sets. Do not add entries here.
- `internal/loomengine/config.go` — `loomDirName = "loom"`, `LoomStatusRel`, `LoomStatusFile`, `LoomStatusLock`. `LoomStatusRel` (the anchor-relative form) has callers that pass it to `fabricengine.ScopedPathspec`; grep for them before moving anything.

**Naming collision to be careful about.** `internal/loomengine` already uses "seed" for something else — `CheckSeed`, `CheckSeedMissing`/`Unreadable`/`Incoherent`, `seedSlug`, `resolveParentBranch`'s "seed the status file" language. That "seed" means *the initial status.json for a fresh run*. The new `seed.json` is a different artefact with the same word. Neither can be renamed cheaply (`CheckSeed` is in CONSTRAINTS' Told-Geometry Invariant as tier 3). Resolution: `internal/shedrun` uses `Seed`/`ReadSeed`/`WriteSeed`/`SeedFile` for the new artefact, and every doc comment touching both says which it means. Do not rename `loomengine.CheckSeed`.

**The prime worktree is a worktree.** `_lyx` exists at prime's anchor like any other worktree's, so the durable batten run directory needs no new structural plumbing — but prime's weft pair is `weft:main`, and the Fabric Git Invariant's Board carve-out (`boardengine` writes to `weft:main` from any worktree through `Bolt`) is the nearest precedent for what committing prime's own `_lyx/shed/<slug>/status.json` looks like. Check whether the ordinary `CommitStatus` seam shape is sufficient from prime before assuming it is.

## Constraints

From `CONSTRAINTS.md`, the ones this task is most likely to trip:

- **Cwd Resolution Invariant** — `internal/shedrun` must join its own `shed` constant onto a told `*lyxcwd.Location`'s `AnchorPath()`, never call `lyxcwd` to resolve one, and never call `os.Getwd`/`git rev-parse`.
- **Told-Geometry Invariant** — an engine is handed absolute paths and derives none. `internal/battenshed`'s new `Seed-Child` producer takes the child's seed path told, resolved lazily by the closure in `internal/battencli/wire.go`.
- **Durable-vs-Ephemeral State Invariant** — drives the locks-mirror decision above. `_lyx` holds tracked content only; every never-tracked file lives under `.lyx` at the mirrored subpath. Each module exposes a scratch accessor beside its durable one.
- **Lyxdirs Single-Declarer Invariant** — `internal/shedrun` takes `_lyx`/`.lyx` from `lyxdirs.LyxDirName`/`DotLyxDirName`, never as literals.
- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`. Nothing in this task may add an import there.
- **Shed Recipe Registry Invariant** — the new `SeedChild` entry goes in the one `map[string]Constructor` reached through `Lookup`/`Names`, with no `init()` self-registration and no runtime `Register`.
- **Shed Verb-Set Invariant** — `internal/shedverbs` derives no path and imports no resolver or `<module>cli`; the `lyx shed` recipe table lives in `internal/shedcli` alone as one map literal reached through accessors. The `step` refusal-kind vocabulary stays closed at five values — a run-id that does not exist must **not** become a sixth kind.
- **Lifecycle Bookend Invariant** (→ **Batten Bookend Invariant**) — a producer that creates or destroys a task worktree never runs from inside it. Batten is driven from prime; its status file and locks live under prime's own anchor. This invariant's *text* changes in this task (the "ephemeral tree" clause becomes "durable tree"), and its two mechanical proxies — `internal/battenshed`'s seam-enforcement scan barring a direct resolver import, and `internal/battencli`'s path-derivation tests pinning status and lock paths to prime's anchor — must both survive the rename and the relocation.
- **CLI / Cobra Invariant** — non-empty `Short` on every new command (`lyx shed seed`), JSON errors via `internal/output`, every `RunE` checks `clihelp.ShouldAbort` first, and the package-naming deviations list needs `lifecyclecli` replaced by `battencli` and `shedcli`'s own deviation line updated.
- **Fabric Git Invariant** — the child-seed commit and the batten status commit are both Go calling `fabricengine` in-process at a row/phase boundary, never raw git and never an agent. Every weft-commit caller passes a positive-only file list via `fabricengine.ScopedPathspec`.
- **Test Tier Purity Invariant** — no `gitexec.Run`, `exec.Command`, `gitkit.Copy*` or `hubforge.NewHub` outside `integration`/`smoke`-tagged files, and no `time.Sleep` ≥ 1s in an untagged file. The re-entrancy tests must substitute `deps.Sleep`.
- **Hermetic Git Test Environment Invariant** — any new test package that spawns git calls `gitkit.HermeticGitEnv()` in `TestMain`.
- **Markdown Link Integrity** — the rename touches link targets in `docs/overview.md` and `manifest/designs/`; the allowlist is keyed by `(file, target)`.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

**New invariant to record in `CONSTRAINTS.md` in the same commit** — *Shed Run-Directory Invariant*: `internal/shedrun` is the sole declarer of the `shed` path segment and the run-id vocabulary (including the literal `self`), and the sole parser and writer of `seed.json`. No other production file names the segment or the filename in path-construction context; no other package decodes or encodes the seed struct. Run-ids are validated as a single path segment before being joined onto any anchor.

## Testing

Tier 1 (untagged, offline, fast) unless stated otherwise.

- **`internal/shedrun`** — the strongest TDD candidate in the task, and the one to write first: it is a pure leaf over told paths with no I/O beyond read/write of one JSON file. Cover each path constructor against a synthetic `*lyxcwd.Location` (durable four under `_lyx/shed/<run-id>/`, ephemeral two under `.lyx/shed/<run-id>/`, and the two never colliding — `shedengine.Shed` rejects `LockPath == StatusLockPath` outright); run-id validation, with the traversal cases (`..`, `a/b`, `/abs`, empty) as explicit table rows; seed round-trip including the absent-`driver`-means-`go` default and the unknown-`driver` refusal; `List` over a directory containing a valid run, a directory with no `seed.json`, and an unreadable entry, asserting sorted output.
- **`internal/shedcli`** — seed-driven arming: the run-id positional defaulting to `self`; an absent seed refusing with a listing of existing run-ids; a seed naming an unknown recipe refusing with the table's available names; the verb gate still firing for a verb a recipe's entry excludes. Pin explicitly that a missing-run refusal carries **no** `kind` field, so the five-value `step` vocabulary stays closed — this is the regression the Shed Verb-Set Invariant most invites. `table_test.go`'s existing arming-coverage shape is the model. The existing `parity_test.go` needs extending to the new positional.
- **`internal/battenshed`** — two producers under test. `Seed-Child`: the happy path writing then committing; `Stuck` on unknown recipe, unreadable Board, and failed commit; a hard error on a path-resolution failure. `Run-Shed`: a table over child states — done → `Done`; blocked/paused/failed → `Stuck` with no self-route and the reason naming state, error and `current_producer`; running → one `deps.Sleep` call then `Stuck`; `found == false` → `Stuck`; an unrecognised state → hard error; and context cancellation surfacing as an error, never as `Stuck`. Fake `deps.Now`/`deps.Sleep` throughout.
- **`internal/battenrecipe`** — the coverage guard pinning row-name constants against `batten-recipe.yaml` must be extended to the new four-row shape and must assert the **value** `"Run-Shed"` explicitly, the same way `recipe_test.go` today pins `"Loom-Run"` against a symmetry-minded rename. Add a test asserting `Run-Shed`'s `on_stuck` self-route and its explicit `max_bounces`, since a dropped `max_bounces` would silently inherit a default budget of a few bounces and turn every long child run into a spurious `StateBlocked` — the highest-consequence, lowest-visibility failure in this task.
- **`internal/shedrecipe`** — a constructor test for the `SeedChild` entry covering `configRejectUnknown` and each `requireNonEmpty`/`requireAbsRoot`/`requireSeam` field, matching the existing entry tests. `coverage_guard_test.go`'s cross-consumer engine-set check needs the renamed package's `RecipeEngines()`.
- **`internal/battencli`** — path-derivation tests pinning status, seed, run-lock and status-lock paths to **prime's** anchor and under the new segment. These are one of the Batten Bookend Invariant's two mechanical proxies; they must not be weakened while being relocated. Plus a test that `CommitStatus` is non-nil now that the status is durable.
- **`internal/loomcli`** — the relocated status path, and `lyx loom start` writing a seed alongside its status seed, including `params.parent`. `resolveParentBranch`'s existing disagreement table stays as-is and its tests should not need touching — if they do, that is a signal the parent decision drifted.
- **`internal/boardengine`** — the new `type` field round-tripping through both structs, `omitempty` keeping existing records byte-identical, and an absent value reading back as the empty string (with the empty-means-`loom` rule tested at the seeding site, not here).
- **Integration (`integration`-tagged)** — one end-to-end batten run over a `hubforge`-built fixture: create → seed child → watch a stubbed child status to `done` → teardown, asserting the child's `_lyx/shed/self/seed.json` exists and is committed. Plus a step-driven variant proving a still-running child yields a re-entrant step that returns control rather than blocking, which is the behaviour the re-entrancy decision exists for.
- **Sandbox suite** — the Sandbox Suite Coverage invariant requires every registered module to be exercised or explicitly excluded with a reason; the `lifecycle`→`batten` rename and the new `lyx shed seed` verb both touch that registration.
- **Help-tree tests** — the CLI/Cobra Invariant's help-tree coverage needs the retired `lyx lifecycle` subtree removed and `lyx batten` plus `lyx shed seed` added.

## Q&A log

- **Q:** Does this task implement `driver: llm`, or only carry the field? **A:** [auto-pick] Carry and validate the field; accept only `go`, refuse `llm` with a pointer to the roadmap item. **Why:** the next roadmap item ("seeded driver choice") is exactly that work, and shipping the field now makes it a spawn-command change rather than a seed-format change.
- **Q:** One package for run addressing and the seed contract, or two? **A:** [auto-pick] One leaf package, `internal/shedrun`. **Why:** a run directory is its seed plus its status; two packages that only ever appear together would mean two invariants where one does.
- **Q:** Where do the run's locks live, given the design says "seed, status, and the run's locks" in one directory? **A:** [auto-pick] Mirrored under `.lyx/shed/<run-id>/`, not beside the durable pair. **Why:** the Durable-vs-Ephemeral State Invariant is unambiguous and loom already splits exactly this way.
- **Q:** Does loom's own status move to `_lyx/shed/self/status.json` in this task, or only lifecycle's? **A:** [auto-pick] Both move. **Why:** "one placement everywhere" is the design's central claim; a half-move leaves two schemes plus a rule for which applies.
- **Q:** What happens to `--recipe` on the `lyx shed` subtree? **A:** [auto-pick] Removed; the run-id positional addresses the run and the seed names the recipe. **Why:** "the verb never knows what it starts — it reads the seed" is the design's headline, and keeping both creates a disagreement case with no good answer.
- **Q:** Who writes the first seed? **A:** [auto-pick] A new `lyx shed seed <run-id>` verb, plus auto-seeding on batten's own entry path from the Board task's `type`. **Why:** two distinct callers with different needs, and batten's entry path already seeds its status file when absent.
- **Q:** How are the seed's `params` typed? **A:** [auto-pick] `map[string]string`, with per-recipe key validation in that recipe's arming function. **Why:** keeps `shedrun` recipe-agnostic, mirroring how `shedbuild` parses a row's `config` and lets each registry entry validate the keys it reads.
- **Q:** Where does the Board's `type` field get validated against the recipe names? **A:** [auto-pick] At the seeding site, never in `boardengine`. **Why:** the Board's job is to carry the choice; giving it the recipe table would couple it to the CLI for a value it only stores.
- **Q:** Full lifecycle→batten rename now, or recipe-only with a compatibility alias? **A:** [auto-pick] Full rename, one commit, no alias. **Why:** the design fixes the timing at "no later than the durable move", lyx is pre-1.0 with no external consumers, and today's ephemeral lifecycle state means there are no in-flight runs to break.
- **Q:** Is there migration code for the old `.lyx/lifecycle/` and `_lyx/loom/` layouts? **A:** [auto-pick] None; the new verbs find no run and refuse with the run-id listing. **Why:** a migration would have to guess the run-id, recipe and driver — the three things the seed exists to record.
- **Q:** How is `Run-Shed`'s re-entrant "still running" outcome expressed, given `Outcome` has only `Done` and `Stuck`? **A:** [auto-pick] One status read per `Call`; still-running sleeps and returns `Stuck` routed `on_stuck: Run-Shed`, bounded by an explicit `max_bounces` replacing `poll_attempts`, with the default interval raised 5s→30s. **Why:** it delivers the design's step-friendly form without touching `shedengine`, which the design excludes — accepting history growth as the named cost. **This is the weakest decision in the task**; a third `Outcome` value is the cleaner answer if the cost is judged unacceptable.
- **Q:** Does `Seed-Child` commit the seed it writes, or only write it? **A:** [auto-pick] Write then commit, through `fabricengine`. **Why:** the seed is durable `_lyx` content, and leaving it uncommitted breaks the machine-switch case the durability decision exists for.
- **Q:** What re-establishes the "destructive teardown is unreachable from a failure path" property, now that `Run-Shed`'s `on_stuck` is no longer empty? **A:** [auto-pick] Exhausting `max_bounces`, which `shedengine` turns into `StateBlocked` at the same row, with the recipe header comment rewritten to say so. **Why:** the routing that carried the property is gone, so the property has to be re-stated against the mechanism that now carries it — the single easiest thing to get wrong in this task.
