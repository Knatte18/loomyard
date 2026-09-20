# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/seeded-shed-core rm <file>`.

### From discussion.md

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
- Removal of the `--recipe` **persistent** flag from the `shed` parent command; `internal/shedcli`'s named-recipe table stays, now keyed by the seed's `recipe` value instead of a flag. `--recipe` survives only as a **local** flag on the new `seed` subcommand, where it names the recipe being written rather than the one being armed.
- A new seeding verb `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]`, idempotent and refusing to overwrite a disagreeing seed.
- A `type` field on the Board task record, empty meaning `loom`, with its `upsertAllowedKeys` entry (the write path) and the read seam that lets a producer resolve it.
- The lifecycle → **batten** rename in full: packages, embedded recipe file, CLI verb, row-name constants, and the CONSTRAINTS invariant heading.
- A new `Seed-Child` row in batten, between `Worktree-Create` and the renamed `Run-Shed`, that writes and commits the child worktree's `_lyx/shed/self/seed.json`.
- A step-friendly, re-entrant waiting form for `Run-Shed`, so a step-driven outer run is not held inside one blocking `Run` call for the child's whole duration.
- A non-nil `CommitStatus` seam for batten, now that its status is durable, with a no-op-transition skip — backed by an ephemeral on-disk last-committed marker so it works across process boundaries — that keeps the re-entrant row from committing and pushing on every bounce or every step.
- A `step` verb for batten, which its table entry excludes today; the re-entrant `Run-Shed` row exists to be stepped, so without it that row has no caller.
- The `--parent` flag's absorption into the seed's params for the loom recipe.
- Doc updates in the same commit: `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/seeded-shed.md`, `manifest/roadmap.md`, plus every prose artefact naming the retired paths or the retired flag — `contracts/specs/loom-status-spec.md`, the `contracts/stencils` and `contracts/recipes` prompt text, `plugins/ly/skills/ly-drive/SKILL.md`, and both sandbox suite documents. See "Enumerate by literal" under Technical context.

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

- Decision: a single new leaf package `internal/shedrun` owns both halves — the `shed` path segment and **every** path constructed from it, the run-id vocabulary (`SelfRunID = "self"`, `ValidateRunID`, `IsReserved`, and `List` over existing run directories), and the `Seed` struct with its `ReadSeed`/`WriteSeed` pair.
- The full constructor set, enumerated so the new invariant has nothing left over: durable under `_lyx/shed/<run-id>/` — `RunDir`, `SeedFile`, `StatusFile`; ephemeral under `.lyx/shed/<run-id>/` — `ScratchDir`, `RunLock`, `StatusLock`, `LastCommitMarker`; anchor-relative forms for `fabricengine.ScopedPathspec` — `SeedRel`, `StatusRel`, which is where `loomengine.LoomStatusRel`'s job lands (that function is deleted, not relocated); and one hub-scoped path one level above any run-id — `PrimeRunLock`, at `.lyx/shed/run.lock`, which is where `lifecyclecli.PrimeRunLock` lands. `PrimeRunLock` takes no run-id: the lock it names serialises every slug's create/teardown against one another, so it is per-hub, not per-run — but it names the `shed` segment, so `shedrun` must own it even though it is batten's to *use*.
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
- The new state the unified layout creates — **seed present, `status.json` absent** — means "this run has been seeded but never started", which today's two layouts could not express. `shedverbs.AbsentDisposition` stays a told per-`Spec` field rather than becoming a shared rule, and both recipes keep today's answer, now with a precise meaning: loom refuses, pointing at `lyx loom start`; batten reports `found: false` on the success envelope, because a seeded-but-unstarted run is a determined answer rather than an error. What changes is only the refusal text, which must now name the run-id.

### self-healing-is-batten-rows-obligation

- Decision: making batten's status durable means its rows must self-heal machine-local resources a durable status promises but a cold machine lacks. `Run-Shed` re-resolves the child worktree on every entry and recreates it from its branch when absent, before watching its status.
- Rationale: named as the explicit cost in the design, and `loom start` already applies the same pattern to a cold task worktree, so the shape is precedented rather than invented here.
- Rejected: refusing on a cold machine and telling the operator to re-create the worktree by hand (turns the durability win back into a manual step, which is the thing being removed).

### run-id-positional-replaces-recipe-flag

- Decision: the four generic verbs take an optional positional run-id defaulting to `self`; the `--recipe` persistent flag on the `shed` parent is deleted, surviving only as a local flag on `seed`. The verb reads `_lyx/shed/<run-id>/seed.json`, takes its `recipe` value, and looks that up in `internal/shedcli`'s existing arming table.
- Rationale: "the verb never knows what it starts — it reads the seed and looks the recipe up in the compiled-in registry" is the design's headline. Keeping both a flag and a seed creates a disagreement case with no good answer. The table itself survives untouched in shape — it still maps a recipe *name* to an *arming function*, which remains a different question from `internal/shedrecipe`'s engine-name→constructor registry.
- Rejected: `--recipe` retained as an override that must agree with the seed (a second source of truth whose only behaviour is to error); `--recipe` retained as a seeding default (conflates addressing with seeding, which the new `seed` verb owns).

### shedcli-resolves-the-location-and-arm-gains-a-location-taking-form

- Decision: `internal/shedcli`'s pre-run calls `lyxcwd.Resolve(cwd)` itself — it must, because it has to read a seed before it knows which recipe to arm — and `Arm` gains a `*lyxcwd.Location`-taking form so the resolution happens once rather than twice. Each module keeps its existing cwd-resolving `arm(cwd, verb, args)` worker for its own subtree's pre-run, unchanged; the `lyx shed` path enters through the new form with the Location and the run-id already in hand.
- Rationale: today `resolvePersistentPreRun` holds only a cwd string and deliberately never resolves a Location, on the documented ground that "each module's Arm owns its own resolution". That rule was affordable when the pre-run's only job was reading a flag. A seed read is a path read, and a path read needs an anchor, so something must resolve one first — and resolving it twice for every `lyx shed` invocation to preserve a comment would be worse than moving the comment.
- **Refusal precedence, which this reorders and which must be stated rather than discovered:** on the `lyx shed` path the order is now (1) `lyxcwd.Resolve`'s own not-a-git-repository sentinel, (2) the seed read's run-id listing refusal, (3) the verb gate, (4) `Arm`'s own refusals — batten's non-prime refusal among them. The visible consequence: `lyx shed run <some-slug>` typed from a *task* worktree now reads that worktree's own `_lyx/shed/` and refuses with a run-id listing, where today it would reach `lifecyclecli`'s "re-run this from prime" message. That is acceptable because the clearer message stays reachable by the command an operator would actually type — `lyx batten run <slug>` arms `battencli` directly, whose own pre-run is unchanged and still refuses non-prime by name.
- This also falsifies a `CONSTRAINTS.md` bullet, which is amended in the same commit rather than left to rot — see Constraints, item 5: the Shed Verb-Set Invariant currently justifies keeping `shedcli` out of Told-Geometry's bound list by its having no resolver, which stops being true here.
- Rejected: resolving twice (once in the pre-run for the seed, once inside `Arm`) to avoid touching `Arm`'s signature — it preserves a doc comment at the cost of a second `git rev-parse` on every invocation, and the comment is describing a rule the seed read has already broken; reading the seed inside `Arm` (circular — `shedcli` needs the seed's `recipe` value to know which `Arm` to call).

### run-id-absent-refuses-with-a-listing

- Decision: when the addressed run-id has no `seed.json`, the verb refuses and names every run-id that does exist (directories under `_lyx/shed/` containing a readable `seed.json`, sorted), and never falls back to scanning for a single candidate. Run-ids are validated as a single path segment — no separator, no `.`/`..` — before being joined.
- Rationale: stated outright in the design ("it never guesses by scanning, because guessing is how the wrong run gets resumed"). The segment validation is not in the design but is required by the same reasoning the Cwd Resolution Invariant applies elsewhere: a run-id reaches this code from a CLI argument and from a Board slug, and both get joined onto an anchor path.
- **`self` is a reserved run-id.** It means "this worktree's own primary run" everywhere, prime included, so prime's `_lyx/shed/` holds one directory per batten slug *plus* potentially its own `self`. A Board task whose slug were literally `self` would collide with that meaning. `shedrun.ValidateRunID` still accepts `self` — it is the default and must be addressable — and a separate `shedrun.IsReserved` is what the seeding path consults whenever the run-id derives from a Board slug, refusing with a message naming the reservation. The refusal lives at the seeding site rather than in validation, because addressing `self` is always legal and only *claiming* it for a task is not.
- Rejected: auto-selecting when exactly one run exists (the failure mode is resuming the wrong run silently, which is worse than a refusal the operator reads).

### seed-verb-plus-auto-seed-on-the-batten-entry-path

- Decision: add `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]`, which creates the run directory and writes `seed.json`, is idempotent against a byte-identical existing seed, and refuses a disagreeing one. Separately, `lyx batten run <slug>` auto-seeds prime's own batten run when absent.
- **Prime's batten seed carries `recipe: batten`, never the Board task's `type`.** The two are different runs' recipes and must not be conflated: prime's `_lyx/shed/<slug>/seed.json` describes the *outer* run that prime executes, which is always batten; the Board's `type` describes the *child's* recipe, which only the child's own seed carries. Getting this wrong would make `lyx shed status <slug>` from prime look up `loom` in the table and arm `loomcli` against prime — the wrong recipe against the wrong worktree. Prime's seed is therefore `{recipe: "batten", driver: <batten's own driver>, params: {slug: <slug>, child_driver: <the driver the child will run under>}}`.
- Both driver values come from flags on the auto-seeding command, each defaulting to `go`: `lyx batten run|step <slug>` takes `--driver` for batten's own driver and `--child-driver` for the child's. `lyx shed seed`'s `--driver` sets only the seed's own `driver`; a batten seed written that way carries `child_driver` through `--param child_driver=<v>` like any other param. Both flags refuse `llm` for now, per the driver decision — the flags exist so the next roadmap item changes a default rather than a surface.
- **Which source `Seed-Child` consumes, for each of the child's two choices:** the child's `recipe` is read **fresh from the Board task's `type`** at `Call` time, not copied from prime's seed params — the Board is the durable truth, and reading it fresh means a `type` corrected after prime was seeded is still honoured. The child's `driver` comes from prime's own seed params (`child_driver`), because it is a per-run startup choice of the outer run rather than a property of the task. This is exactly the split the design states ("copying the recipe choice from the Board task's type field and the driver choice from batten's own seed params"); the param is named `child_driver` rather than `driver` so a reader of prime's seed cannot mistake it for batten's own.
- Decision on where `seed` sits in the subtree: `seed` is a **`shedcli`-only command, never a `shedverbs` verb**, and `resolvePersistentPreRun` short-circuits on `cmd.Name() == "seed"` exactly as it already short-circuits on `cmd.Name() == "shed"`. `seed` resolves cwd for itself and does nothing else the pre-run does — no run-id seed read, no table lookup, no verb gate, no `Arm`.
- Rationale for that exemption: the pre-run's whole sequence is predicated on a seed already existing, and `seed` is by definition the command invoked when one does not. It also belongs to no recipe's `Verbs` set, so the verb gate would refuse it for every recipe. Putting it in `shedverbs` would be worse still: that package is a leaf that derives no path and knows no recipe table, and seeding needs both.
- Rationale: something must write the first seed, and there are two distinct callers with different needs — an operator addressing an arbitrary run by hand, and batten's own entry path. `lyx loom start` likewise gains a seed write next to the status seed it already performs.
- **The seed-presence check runs on the `lyx batten` path too, inside `battencli`'s own `arm`.** `shedcli`'s pre-run reads a seed because it must — it cannot know which recipe to arm otherwise — but `battencli` knows its recipe without one, so nothing on that path would read a seed unless this task says so. It does: batten's `arm` resolves the run-id and then branches by verb, sharing one `shedrun`-level helper with `shedcli` so the listing refusal is worded identically on both paths. The order on the batten path is: `lyxcwd.Resolve` → the existing non-prime refusal → run-id resolution → **a `self`-address refusal** → **`run`/`step` auto-seed when absent; `status`/`pause` require a seed and refuse with the run-id listing when absent** → `wire`.
- **Batten refuses a `self` address outright**, before the auto-seed gate. Prime hosts many batten runs, one per task slug, so `self` — "this worktree's own primary run" — has no meaning there; a batten run is always slug-addressed. Without this refusal, an argument-less `lyx batten run` would take the `self` default, fall through the auto-seed gate, and write prime a `_lyx/shed/self/seed.json` with an empty `params.slug`. `IsReserved` does not catch it, since that check is scoped to run-ids derived from a Board slug and this one comes from the positional's default. The refusal names the missing slug rather than listing run-ids: the operator's mistake is an omitted argument, not a wrong one.
- **The `lyx loom` path gets the same check, in `loomcli.Arm`, but with no auto-seed at all.** Like batten, loom knows its recipe without reading a seed, so nothing there would read one unless this task says so — and the claim under `no-migration-legacy-layouts-refuse-loudly` that the new verbs "find no run and refuse with the run-id listing" is only true on this path if something checks. The branch differs from batten's in one way that matters: **`lyx loom run` and `lyx loom step` do not auto-seed.** Only `lyx loom start` may seed loom's run, which is loom's existing discipline, stated outright in `lifecyclePreRun`'s own doc comment ("loom refuses in exactly the situation lifecycle seeds here, because only `lyx loom start` may seed loom's own status file") and extended unchanged from the status file to the seed. So on the loom path all four generic verbs require a seed and refuse with the run-id listing when absent; `start` is the one site that writes one.
- A status file present with no seed beside it is an inconsistency, not a state to tolerate: the seed is the run's identity, and `start` writes both together. It takes the same listing refusal, whose remedy — re-run `lyx loom start` — is the right one. This is also what makes the no-migration decision coherent on this path: a worktree from before this task has no `_lyx/shed/` at all, so it refuses with an empty listing and the operator re-runs `start`.
- Two absences that must not be confused, since both now exist: **no seed at all** is the run-id listing refusal, from the check above. **Seed present, `status.json` absent** is the seeded-but-never-started case, which reaches `shedverbs`' own `AbsentDisposition` and gets batten's `found: false` or loom's refusal per that field. The check above runs first and only ever fires on the former.
- **Where batten's auto-seed runs: inside `arm`, ahead of `wire` — not in the `PreRun` hook that seeds the status file.** `lifecyclePreRun` is a `shedverbs.Spec` hook called inside `run`'s `RunE`, which is *after* `arm` has already called `wire` and built the whole `Env`. Seeding there would leave anything that reads the seed at wiring time looking at a seed that does not exist yet on a first `lyx batten run <slug>`. Seeding in `arm` instead puts the write ahead of every reader. It is gated to the verbs that start a run — `run` and `step` — because `status` and `pause` are read-only and must not create state as a side effect of being asked a question, the same reasoning `EnsureStatusLockDir: false` already encodes for lifecycle's status verb. Those two therefore take the seed-presence check's listing refusal instead, per the branch above.
- Belt-and-braces alongside that: every seed-param read stays lazy inside its own closure body, exactly as `internal/battencli/wire.go`'s file doc already requires of every path read. Ordering and laziness fix the same bug from two directions, and the file's existing discipline is the cheaper of the two to keep.
- **Consequence: batten gains a `step` verb.** Its table entry today excludes `step` on the ground that lifecycle "has no step analogue" — which was true only because its `StepBusyKind`/`PreStep` were unset and its seeding was `run`-only. The re-entrant `Run-Shed` row exists precisely so an outer run can be stepped, so a batten that cannot be stepped would leave that row without its motivating caller. The entry gains `"step"`, and four things are wired with it — the table entry alone is not enough, because `step`'s `RunE` shares none of `run`'s path:
  - **A `PreStep` hook.** Batten's status-file seeding lives in `lifecyclePreRun`, which is a `PreRun` hook that `stepCmd` never calls; without an equivalent on the step path, `stepLocked`'s read gate hits an absent status and hard-errors ("Shed never seeds one"), which `shedverbs/step.go` reports as `kind: "producer"` — the one kind `ly-drive` retries, so the failure would loop. Batten's `PreStep` performs the same work `lifecyclePreRun` does (decode the status, refuse a `done` slug, resume silently over every other state, seed when absent) preceded by a run-lock probe, modelled on `loomPreStep`'s.
  - **Its refusal-kind mapping, inside the closed five:** run lock already held → `KindBusy`; a status decode failure or a failed status seed → `KindUnseeded`; any other pre-producer failure → `KindBootstrap`. `KindOwnership` is not used — it exists for loom's seeded-status-belongs-to-another-slug check, which has no batten analogue. `KindProducer` is `shed.Step`'s to emit, never the hook's.
  - **A `BuildShed` arm for `verb == "step"`.** `specFor` fills `BuildShed` only for `"run"` today, so step would reach the "no BuildShed constructor configured" refusal. Batten's step arm is plain `battenrecipe.New(c.env, c.shedPaths)` — the same constructor `run` uses, with no `buildLoomShed`-style analogue, because that split exists in loom solely to avoid opening the fabric and reading origin twice and batten's `wire` does no such work.
  - **`Step` verb texts and `StepBusyMessage`.** Batten's `VerbTexts` gains a `Step` entry with a non-empty `Short`, which the CLI/Cobra Invariant requires of every command, plus the told busy text that travels beside `StepBusyKind`. This also removes the `shedcli` table's one documented reason for gating verbs per recipe, but the gate itself stays — a future recipe may still exclude a verb, and the gate is what keeps `step`'s refusal-kind vocabulary closed at five values.
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

- Decision: add `Type string \`json:"type,omitempty"\`` to `boardengine.Task` and its store record, **and `"type"` to `internal/boardengine/store.go`'s `upsertAllowedKeys`**. An empty value means `loom`. `boardengine` never validates the value against a recipe name; the validation happens where the seed is written, against `internal/shedcli`'s table.
- The allowlist entry is not optional bookkeeping — it *is* the write path. `upsertAllowedKeys` is a closed set enforced by `validateUpsertFields` for both `UpsertTask` and `UpsertTasksBatch`, so a `type` field added to the structs but not the allowlist could never be set by anything, and `Seed-Child` would read empty (`loom`) forever. With the entry added, no new flag is needed: `lyx board upsert` takes a JSON payload, so `lyx board upsert '{"slug":"x","type":"batten"}'` works the moment the key is accepted, and `lyx board merge`'s inner `upsert` object inherits it for free.
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

- Decision: `Run-Shed`'s `Call` **reads the child's status before it does anything else**, and the recipe routes `on_stuck: Run-Shed` — the row's own re-entry — with `poll_interval_s: 30` and `max_bounces: 1440`. The existing `poll_attempts` config key is retired in favour of that budget. The full disposition table, read top to bottom:
  - child status **absent** (`found == false`) and `deps.Spawn` has not yet run this entry → **spawn**, then re-read once. This inverts today's order (`innerRunProducer.Call` spawns unconditionally, then polls), and the inversion is the whole point: the child's own status file is the durable, re-entry-safe record of whether the spawn already happened, so no separate marker is needed and a resumed run never double-spawns.
  - `deps.Spawn` returns an error → **hard error**. Today it is `Stuck`; under a self-route a `Stuck` here would bounce with no sleep and burn the budget in a tight loop.
  - child status still absent **after** a successful spawn → **hard error**, naming the spawn that returned success without producing a status. Today this is `Stuck` with the reason "no status file found after the inner shed run's own handshake confirmed a driver took the run lock". Making it a hard error is what closes the respawn loop: the row can never take the spawn arm twice for one child.
  - child `done` → `Done`, routing forward to `Worktree-Teardown` as today.
  - child still `running` → sleep `poll_interval_s`, return `Stuck`, re-enter this same row.
  - child `blocked`/`paused`/`failed` → **hard error**, whose message carries the child's `state`, `error` and `current_producer`. Not `Stuck`.
  - any unrecognised state → hard error, as today.
- Rationale for the hard error, which is the load-bearing half: `ProducerDef.OnStuck` is a static per-producer value (`internal/shedengine/producer.go`), and `Call` returns only `(Outcome, OutputPointer, error)` — so once `on_stuck` is non-empty, **every** `Stuck` from this row routes back to it (`run.go`'s `default:` branch). There is no such thing as "Stuck with no self-route" for one case and a self-route for another; a blocked child returning `Stuck` would bounce, and — having nothing to sleep on, since the sleep is in the still-running arm alone — would burn the whole 1440-bounce budget in a tight loop before reaching `StateBlocked`. The hard error escalates immediately instead, landing the run in `StateFailed` at `Run-Shed` with the child's own diagnosis in the message. It matches `innerRunProducer`'s existing error-vs-verdict split, where a condition the outer run cannot act on is a returned error rather than a verdict.
- Rationale for the re-entry shape itself: it delivers the design's "one status check per step with a re-entrant still-running outcome" without touching `shedengine`'s two-value `Outcome` vocabulary, which the design puts explicitly out of scope. `run` mode is unaffected in wall-clock behaviour: it loops the same route internally with the same sleep inside `Call`.
- What a step actually costs, stated precisely rather than as "returns control": the sleep stays inside `Call`, so one `lyx shed step` against a still-running child **blocks for one `poll_interval_s` (30s)** and then returns. That is the property worth having — a step-driven outer run is no longer held for the child's whole multi-hour duration — but it is not a non-blocking step, and nothing here makes it one. The sleep cannot be skipped under `step` alone, for the same reason `wait_mode` was rejected: the row cannot see which verb drove it.
- Budget derivation, stated as wall-clock rather than inherited as a number: the intended watch window is **12 hours**. Today that window is expressed by two Go constants, not by anything in the recipe file — `defaultInnerRunPollAttempts = 8640` and `defaultInnerRunPollIntervalS = 5` in `internal/shedrecipe/entries_lifecycle.go`, and `contracts/recipes/lifecycle-recipe.yaml` carries no `config:` block at all. Preserving the 12 hours at a 30s interval gives 1440. Reusing the attempt *count* instead would silently stretch the window to 72 hours.
- These land on two different surfaces, which the plan must not conflate: `poll_interval_s` is a `config` key read by the registry entry, so the row gains an explicit `config: {poll_interval_s: 30}`; `max_bounces` is a `shedbuild.Row` field, not a config key, so it is a row-level `max_bounces: 1440`. `poll_attempts` is retired as a config key, which means deleting `defaultInnerRunPollAttempts`, its negative-value guard, and its entry in the row's `configRejectUnknown` allowlist. `defaultInnerRunPollIntervalS` stays but changes to 30, so an omitted key and the explicit key agree.
- Known costs, both of them:
  - **History growth.** Every re-entry appends a history entry and rewrites the status file, so a 12-hour child run accumulates ~1440 entries. Accepted; the interval change is what keeps it at 1440 rather than 8640.
  - **Commit and push per bounce — mitigated, not accepted.** `shedengine.persist` calls `CommitStatus` unconditionally on every transition, the self-bounce included, and the seam this task models batten's on (`internal/loomcli/wiring.go`'s `newCommitStatusSeam`) commits *and* pushes, the push unconditional after the commit. Left alone, a 12-hour child would mean ~1440 weft commits and pushes on prime's pair. So **batten's `CommitStatus` seam skips a no-op transition**: when the incoming `(producer, state)` pair equals the last pair it committed, it returns nil without committing or pushing. The self-bounce is exactly that case — `persist(def.OnStuck, StateRunning, …)` with `def.OnStuck == "Run-Shed"`. Nothing durable is lost: the already-committed pair is `("Run-Shed", "running")`, which is precisely what a resuming machine needs, and the accumulated history rides along on the next real transition's commit.
  - **The bounce budget becomes non-durable, and that is accepted.** `episodeStuckCount` derives the budget from the status file's trailing run of `Stuck` history entries. The skip means those ~1440 entries are written locally but not committed until the next real transition, so a machine switch mid-`Run-Shed` resumes from a status whose bounce run is empty and restarts the 12-hour window from zero. Accepted without mitigation: it *extends* the watch window rather than truncating it, so the failure mode is watching a dead child for longer than intended rather than abandoning a live one — and a machine switch mid-watch is precisely the moment a fresh window is the more useful default.
  - **The skip's memory is on disk, not in a closure.** Each `lyx shed step` / `lyx batten step` is a fresh process, so an in-memory last-committed variable would start empty every time and the skip would work for `run` mode alone — leaving step mode, the mode the whole re-entrancy decision exists to enable, committing and pushing once per step. The seam therefore records the pair it last committed in a small ephemeral marker file at `.lyx/shed/<run-id>/last-commit`, reads it before deciding, and rewrites it after a successful commit. Ephemeral is the correct class: it is a cache, machine-local, and never tracked, so it belongs under `.lyx` at the mirrored subpath like the locks. Losing it — a fresh machine, a cleared scratch tree — costs exactly one redundant commit and push, never correctness.
  - **`shedengine.persist`'s own doc comment must be qualified in the same commit.** It currently states the seam "is called on every persist invocation, never conditionally on `nextCurrentProducer` having changed: state, history, and error can all change without it" — which is exactly the conditional this skip introduces, one layer out. The qualification to write: the engine still calls the seam unconditionally, and a *caller's* seam may skip; batten's does, keyed on `(producer, state)` alone and therefore deliberately blind to `history` and `error` changes. That blindness is safe because the only transition it ever skips is the self-bounce, where `error` is empty by construction and the accumulated `history` is committed by the next real transition.
- Rejected: a third `Outcome` value such as `Continue` — re-weighed with the true cost above on the table, and still rejected, but on one ground only: the design doc scopes `shedengine` out (`"adds addressing and a seed contract around them, not new machinery in them"`). It is semantically the cleanest primitive and would remove both the hard-error workaround and the skip mechanism. **If the plan finds the no-op-skip seam unworkable, this is the alternative to reopen, not a fourth workaround.** Also rejected: a `wait_mode` config key selecting blocking-vs-single-poll (the same recipe serves both verbs, so the row cannot know which one drove it); two rows, spawn then watch (moves the same budget and history-growth problem one row over).

### prime-status-commit-uses-the-loom-shaped-seam-not-bolt

- Decision: batten's `CommitStatus` is the loom-shaped seam — `fabricengine.CommitAnchoredPaths` scoped to the run's own status path through `fabricengine.ScopedPathspec`, then `fabricengine.PushAnchored`, wrapped in the no-op-transition skip decided above. It is **not** `fabricengine.Bolt`.
- Rationale: `Bolt.Commit` stages *every* change in its repo (`internal/fabricengine/bolt.go`), which the Fabric Git Invariant forbids for a weft-commit caller — "every weft-commit caller passes a positive-only file list via `fabricengine.ScopedPathspec`". `Bolt` is the Board's own carve-out, scoped to the Board directory and to writes the Board serialises with its own push lock; borrowing it for an unrelated file would widen the carve-out rather than use it.
- Serialisation against Board writes to the same `weft:main`, which is the real question: batten does **not** take the Board's push lock. It does not need to, because (a) the on-disk no-op skip reduces batten to roughly four commits per whole run — one per real row transition, in `run` and `step` mode alike, which is what putting the skip's memory on disk rather than in a closure buys — so contention is rare rather than per-30-seconds, and (b) a push rejected because a Board write advanced the branch takes the loom seam's existing disposition: warn and let the next transition catch the branch up. `newCommitStatusSeam` already treats every push error that way, `gitrepo.ErrPushRejected` included, deliberately and with its reasoning written out.
- One interaction to state rather than discover: if the Board directory and prime's weft checkout turn out to be the same working tree, a concurrent `lyx board sync` would sweep batten's status file into its own all-staging `Bolt.Commit`. That is benign — `state.UpdateJSON` writes the status under its own advisory lock, so any file another committer picks up is a complete one, never a half-written one — and it commits the status slightly early rather than wrongly. The plan should confirm the two checkouts' relationship, but no design outcome hangs on the answer.
- Rejected: `Bolt` (stages everything, Board-scoped); batten taking the Board's push lock (serialises four commits per run against a lock built for a different writer's coalescing loop, for contention that the skip mechanism has already made rare).

### seed-child-writes-and-commits

- Decision: `Seed-Child` is a new row and a new `SeedChild` registry entry in `internal/shedrecipe`. It reads the Board task's `type` through a new `Env` seam, takes the driver from batten's own seed params, writes the child's `_lyx/shed/self/seed.json`, commits it to the child's weft pair through `internal/fabricengine`, **and pushes**. Verdicts: `Done` on a successful write+commit; `Stuck` on an unknown recipe name, an unreadable Board, or a failed commit; a hard error on a path-resolution failure, matching `innerRunProducer`'s existing error-vs-verdict split. A failed *push* is neither — it warns and returns `Done`, exactly as the `CommitStatus` seam's push disposition does, for the same reason: an offline machine must not halt a run, and the next push on that pair catches the branch up.
- The push is not optional garnish. This row's whole rationale is the machine-switch case, and a local commit reaches no other machine — a seed committed but never pushed is lost to a fresh clone just as surely as one never committed, which is the alternative this decision rejects. The sibling `CommitStatus` seam already pushes for precisely this reason.
- Rationale: the seed is durable content under `_lyx`, so leaving it uncommitted would break the very machine-switch case the durability decision exists for. Per the Fabric Git Invariant the commit is Go calling `fabricengine` in-process at a row boundary, which is exactly what this is. The row runs after `Worktree-Create`, so the child worktree exists; like every other seam in `internal/battencli/wire.go` it resolves the child's paths lazily inside its own closure body, never at wiring time.
- Note the two distinct commits this task introduces on the batten side, so the plan does not conflate them: this row's commit is a one-off write of the **child's** seed onto the **child's** pair, while the `CommitStatus` seam decided under `run-shed-re-entrancy-via-self-pointing-on-stuck` commits **prime's own** batten status onto **prime's** pair on every non-no-op transition. Different file, different pair, different frequency.
- Rejected: writing the seed without committing (child seed lost on a fresh clone of the pair); folding the seed write into `Worktree-Create` (the design keeps `Worktree-Create` fabric-only, and a combined row would make a seeding failure indistinguishable from a creation failure).

### driver-field-present-but-only-go-accepted

- Decision: `seed.json` carries `driver`, defaulting to `go` when absent. `llm` parses but is refused at arming time with a message naming the roadmap item that implements it. `shedrun` declares both constants so the next task adds behaviour, not vocabulary.
- Rationale: the next roadmap item binds to this field, and shipping the field without its second value is what lets that item be a spawn-command change rather than a contract change. Accepting `llm` and silently running `go` would be worse than refusing.
- Rejected: omitting the field entirely (the next task then changes the seed format, which is the durable on-disk contract this task is establishing); implementing `llm` here (that is the next roadmap item's whole content, and it needs the reed-strand spawn seam).

## Technical context

**Where the pieces are today.**

- `internal/shedverbs` — the generic `run`/`step`/`status`/`pause` cobra bodies. Reads everything through a told `shedverbs.Spec` (`spec.go`): `StatusPath`, `LockPath`, `StatusLockPath`, `BuildShed`, six `Hooks`, and a set of told message strings. Derives no path and imports no resolver. Adding run addressing must not change that — the run-id is resolved by the *arming* side and reaches `shedverbs` as the same three told paths it reads today.
- `internal/shedcli` — `cli.go` builds the subtree and `resolvePersistentPreRun` resolves cwd, looks `--recipe` up in `table.go`'s `recipes` map, gates the verb against the entry's `Verbs` set, and calls `e.Arm(cwd, verb, args)`. This pre-run is where the seed read replaces the flag read: resolve cwd → resolve run-id from `args` (default `self`) → `shedrun.ReadSeed` → `lookup(seed.Recipe)` → verb gate → `Arm`. **`table.entry.Args` is deleted**, and the `argsFor()` closure with it: the positional is now one optional run-id for every recipe, so `cobra.MaximumNArgs(1)` is set statically on each of the four verbs and there is nothing left for a per-recipe argument contract to vary. This removes the closure's whole reason for existing (it deferred `Args` resolution because `--recipe` was readable at validation time but the table entry was not).

Deleting `entry.Args` changes the **`lyx shed` path alone**, so each module's own subtree needs its own arity decision — and **all three surfaces land on `cobra.MaximumNArgs(1)`**, uniformly:

- `lyx shed <verb> [<run-id>]` — the universal positional, set statically on each of the four verbs.
- `lyx batten <verb> [<run-id>]` — `internal/battencli` sets `cobra.ExactArgs(1)` directly on its `run`/`status`/`pause` verbs today (`internal/lifecyclecli/cli.go`); all three relax, and the new `step` verb takes the same.
- `lyx loom <verb> [<run-id>]` — `internal/loomcli`'s four verbs declare **no** `Args` validator at all today, so without a decision here a run-id typed at `lyx loom status <run-id>` would be silently swallowed and address `self`. They gain `MaximumNArgs(1)` too. Loom's runs are `self` by construction in a task worktree, so the positional will almost always be omitted — but "almost always" is not "never", and a silently ignored argument is the worst of the three options.

Uniformity is the point: one arity contract across all three surfaces means no rule to remember about which subtree accepts a run-id.

Two pinned assertions move with this. `internal/shedcli/table.go` gives loom `cobra.NoArgs` today, and `internal/shedcli/parity_test.go`'s `TestShed_LoomRunRefusesExtraArgByNoArgs` pins exactly that — it is **deleted**, not adapted, since the universal positional is what replaces it and its intent (a wrong argument count refuses) is carried by the new arity assertions. `TestParity_PositionalArgs` survives but its cases change: it exists to prove both paths refuse identically *because* both validate through the same `cobra.ExactArgs(1)` value, and with the shared value gone it must assert that each path independently carries `MaximumNArgs(1)`. Its `TwoSlugs` case stays an arity refusal on both sides; its `NoSlug` case stops being a refusal at all.
- `internal/loomcli/arm.go` and `internal/lifecyclecli/arm.go` — each exposes `Arm(cwd, verb, args) (shedverbs.Spec, error)` plus an unexported `arm` worker and a resolution-free `specFor(verb)`. Both fill `shedbuild.ShedPaths`. These are the two functions whose path-construction changes: they currently call `loomengine.LoomStatusFile(l)` / `StatusFile(l, slug)` and must call `shedrun.StatusFile(l, runID)` instead. `Arm`'s signature may need the run-id threaded explicitly rather than dug back out of `args`.
- `internal/lifecyclecli/paths.go` — the five prime-anchored constructors under `.lyx/lifecycle/<slug>/`, plus `PrimeRunLock`, the hub-scoped lock serialising every slug's create/teardown against one another. `PrimeRunLock` is *not* per-run, but it does move into `shedrun` — see that package's constructor list; batten remains its only caller.
- `internal/lifecyclecli/wire.go` — builds the `shedrecipe.Env` with four seam groups (`CreateWorktree`, `Teardown`, `InnerRun`, `PrimeLock`) and `shedbuild.ShedPaths` with `CommitStatus: nil`. Two changes land here: a fifth seam group for `Seed-Child`, and `CommitStatus` becoming non-nil now that batten's status is durable — `internal/loomcli/wiring.go`'s `newCommitStatusSeam`/`loomCommitStatusDeps` is the existing implementation to model it on (note its three-case error handling, which is not trivial).
- `internal/lifecycleshed/innerrun.go` — `innerRunProducer`, the row being renamed and made re-entrant. Its current shape is: resolve status path, log spawn, `deps.Spawn`, log wait complete, then poll `deps.ReadStatus` up to `pollAttempts` times with `deps.Sleep(pollInterval)` between. Its doc comment carries the full verdict table, which must be rewritten with the new outcomes. `deps.Now`/`deps.Sleep` nil-resolution to `time.Now`/`time.Sleep` happens once in the constructor and is what tests substitute.
- `internal/shedrecipe/entries_lifecycle.go` — `worktreeCreateEntry`, `innerRunEntry`, `worktreeTeardownEntry`. `innerRunEntry` reads `poll_interval_s`/`poll_attempts` via `configInt` and validates `Env.Slug`/`Env.ScratchDir`/`Env.InnerRun.*` via `requireNonEmpty`/`requireAbsRoot`/`requireSeam`. The new `SeedChild` entry belongs in this file (renamed `entries_batten.go`) — it shares the `Slug`/`ScratchDir` validation shape that is the stated reason these three are grouped apart from `entries_simple.go`.
- `contracts/recipes/lifecycle-recipe.yaml` — three rows. Its header comment explains that `Loom-Run`'s empty `on_stuck` is load-bearing (a stuck verdict escalates with the worktree intact, which is what makes the destructive teardown row unreachable from a failure path). The re-entrancy decision changes that `on_stuck` from empty to self-pointing, so **that comment's claim must be re-established a different way**: with `on_stuck: Run-Shed`, a failing child escalates through the row's own hard-error return, which `shedengine` turns into `StateFailed` at `Run-Shed` with the run halted — `Worktree-Teardown` is never routed to. The recipe header must say so explicitly, and must not claim the old routing-based mechanism; this is the single easiest thing to get wrong in the whole task.
- `internal/boardengine/task.go` and `store.go` — two near-duplicate structs (`Task` and the store's own record) that both need the new field. `template.yaml`, `render.go` and `layer.go` are the places to check for anything that enumerates fields.
- `internal/fabricengine/junctionnames.go` — `structuralCommittedDirs` is `{_lyx}` and `structuralNeverCommittedDirs` is `{.lyx}`. **No change needed**: `_lyx/shed/` and `.lyx/shed/` are subpaths of names already in those sets. Do not add entries here.
- `internal/loomengine/config.go` — `loomDirName = "loom"`, `LoomStatusRel`, `LoomStatusFile`, `LoomStatusLock`, all four deleted in favour of `shedrun`'s equivalents. `LoomStatusRel` is the one with a non-obvious caller: `internal/loomcli/wiring.go` passes it to `fabricengine.CommitAnchoredPaths` as the commit's positive-only pathspec, so `shedrun.StatusRel(runID)` must exist before that call site can move. Grep for every `LoomStatusRel` caller before deleting it.

**Enumerate by literal, not by Go caller.** Grepping Go symbols misses every prose artefact that hand-writes the old path or flag, and there are several. Sweep these seven literals across `contracts/`, `plugins/`, `tools/`, `docs/`, `manifest/` **and repo-root `CONSTRAINTS.md`**, not just `internal/`: `_lyx/loom`, `.lyx/lifecycle`, `--recipe`, `lyx lifecycle`, `lifecycleshed`, `lifecyclerecipe`, `lifecyclecli`. The three package-name literals and the repo-root path are what reach `CONSTRAINTS.md`, which the path list would otherwise exclude entirely — see the Constraints section for the five lines there that go stale. Known hits at the time of writing:

- `contracts/specs/loom-status-spec.md` — pins the `_lyx/loom/status.json` schema in four places, and is a **deployed normative spec**. This is the one with a wrinkle: per the Stencil Ownership Invariant, `internal/stencilstore` never overwrites a hash-mismatched file, and there is explicitly no force-sync carve-out for specs. So editing the in-repo copy does **not** update an already-deployed one — the deployed copy keeps the old text and is simply never refreshed. The task must state the operator step (delete the deployed copy so the next run re-seeds it) in the commit message and in `docs/overview.md`; it must not add a force-sync path, which the invariant forbids.
- `contracts/stencils/…` — the literal `_lyx/loom/` appears in producer prompt text and is pinned by `internal/loomcli`'s `discussiontemplate_test.go`.
- `contracts/recipes/loom-recipe.yaml` — names `_lyx/loom/status.json` in a round's prompt text.
- `plugins/ly/skills/ly-drive/SKILL.md` — instructs on `--recipe lifecycle` and on the flag generally; the flag is being removed, so this skill's whole invocation surface changes to the run-id positional.
- `tools/sandbox/SANDBOX-CORE-SUITE.md` — hand-writes `_lyx/loom/status.json` as a fixture.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — an entire scenario written against `lyx lifecycle` and `.lyx/lifecycle/<slug>/`, including the run-lock and per-slug-directory refusal texts, all of which move.

**Naming collision to be careful about.** `internal/loomengine` already uses "seed" for something else — `CheckSeed`, `CheckSeedMissing`/`Unreadable`/`Incoherent`, `seedSlug`, `resolveParentBranch`'s "seed the status file" language. That "seed" means *the initial status.json for a fresh run*. The new `seed.json` is a different artefact with the same word. Neither can be renamed cheaply (`CheckSeed` is in CONSTRAINTS' Told-Geometry Invariant as tier 3). There is a **third** sense too: `shedverbs.KindUnseeded`, one of the five closed refusal kinds, whose doc reads "the status file could not be seeded" — the same first sense, but in a vocabulary an operator reads at the moment `lyx shed seed` also exists, so "unseeded" becomes ambiguous about which artefact is missing. Resolution across all three: `internal/shedrun` uses `Seed`/`ReadSeed`/`WriteSeed`/`SeedFile` for the new artefact, and every doc comment touching more than one says which it means. `KindUnseeded`'s **value does not change** — the five-kind vocabulary is pinned closed by `internal/shedverbs/step_test.go` and by `ly-drive`'s own contract — only its doc comment is clarified to say it means the status file, not `seed.json`. Do not rename `loomengine.CheckSeed` either; it is named in CONSTRAINTS' Told-Geometry Invariant as tier 3.

**The prime worktree is a worktree.** `_lyx` exists at prime's anchor like any other worktree's, so the durable batten run directory needs no new structural plumbing. See the `prime-status-commit-uses-the-loom-shaped-seam-not-bolt` decision for how its status reaches `weft:main`.

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

**Five existing `CONSTRAINTS.md` lines the rename and the resolution change make stale, all amended in the same commit:**

1. **Told-Geometry Invariant**, bound-packages list — `lifecycleshed` and `lifecyclerecipe` become `battenshed` and `battenrecipe`.
2. **CLI / Cobra Invariant**, interactive-handoff exception — `lyx lifecycle status --watch` becomes `lyx batten status --watch`.
3. **CLI / Cobra Invariant**, package-naming deviations — `lifecyclecli` becomes `battencli`, and `shedcli`'s own deviation line (which names `internal/lifecyclecli` among its imports) moves with it.
4. **Lifecycle Bookend Invariant** — heading renamed to **Batten Bookend Invariant**, and its body's "prime's own ephemeral tree" clause becomes the durable tree, with both mechanical proxies re-pointed at the renamed packages.
5. **Shed Verb-Set Invariant**, the bullet stating that neither `shedverbs` nor `shedcli` sits in Told-Geometry's bound list because "their identical no-derived-paths obligation is carried by this invariant's own no-resolver clause". This task makes that false of `shedcli`, which now calls `lyxcwd.Resolve` and builds seed paths from the result. The amendment splits the two: `shedverbs` keeps the no-resolver clause verbatim, enforced by `internal/shedverbs/seam_enforcement_test.go` as today; `shedcli` is carved out by name as the one site that resolves, on the stated ground that it cannot know which recipe to arm without first reading a seed, with the narrower obligation that every path it touches comes from a `shedrun` constructor and none is derived locally.

**New invariant to record in `CONSTRAINTS.md` in the same commit** — *Shed Run-Directory Invariant*: `internal/shedrun` is the sole declarer of the `shed` path segment and the run-id vocabulary (including the literal `self`), and the sole parser and writer of `seed.json`. No other production file names the segment or the filename in path-construction context; no other package decodes or encodes the seed struct. Run-ids are validated as a single path segment before being joined onto any anchor.

## Testing

Tier 1 (untagged, offline, fast) unless stated otherwise.

- **`internal/shedrun`** — the strongest TDD candidate in the task, and the one to write first: it is a pure leaf over told paths with no I/O beyond read/write of one JSON file. Cover every path constructor against a synthetic `*lyxcwd.Location`: the three durable ones under `_lyx/shed/<run-id>/`, the four ephemeral ones under `.lyx/shed/<run-id>/`, the two anchor-relative forms, and `PrimeRunLock` sitting one level above any run-id. Assert `RunLock` and `StatusLock` never collide — `shedengine.Shed` rejects `LockPath == StatusLockPath` outright — and that `PrimeRunLock` collides with neither for any run-id. Then run-id validation, with the traversal cases (`..`, `a/b`, `/abs`, empty) as explicit table rows, and `IsReserved` answering true for `self` alone while `ValidateRunID` still accepts it; seed round-trip including the absent-`driver`-means-`go` default and the unknown-`driver` refusal; `List` over a directory containing a valid run, a directory with no `seed.json`, and an unreadable entry, asserting sorted output.
- **`internal/shedcli`** — `seed`'s pre-run exemption: `lyx shed seed <run-id> --recipe <name>` succeeds with no seed present and no recipe armed, which is the case the pre-run would otherwise refuse three different ways. Then seed-driven arming: the run-id positional defaulting to `self`; an absent seed refusing with a listing of existing run-ids; a seed naming an unknown recipe refusing with the table's available names; the verb gate still firing for a verb a recipe's entry excludes. Pin the **refusal precedence** as a table — not-a-git-repository, then run-id listing, then verb gate, then `Arm`'s own refusals — since that order changed in this task and a regression in it is invisible until an operator gets the wrong message from the wrong worktree. Pin explicitly that a missing-run refusal carries **no** `kind` field, so the five-value `step` vocabulary stays closed — this is the regression the Shed Verb-Set Invariant most invites. `table_test.go`'s existing arming-coverage shape is the model. `TestParity_PositionalArgs` needs rewriting rather than extending — see the `entry.Args` paragraph under Technical context for what its two cases become once the shared `Args` value is gone.
- **`internal/battenshed`** — two producers under test. `Seed-Child`: the happy path writing then committing, asserting the child's seed takes its `recipe` from the Board task's `type` read at `Call` time and its `driver` from prime's `params.child_driver` — the two-source split is the thing most likely to be collapsed into one by a plan writer; a Board `type` changed between prime's seeding and this row's `Call` being honoured, which is what reading fresh buys; an empty `type` defaulting to `loom`; `Stuck` on unknown recipe, unreadable Board, and failed commit; a **failed push warning and still returning `Done`**, which is a different disposition from the failed commit beside it and easy to collapse into one arm; a hard error on a path-resolution failure. Separately assert prime's own seed carries `recipe: "batten"`, since a seed carrying the child's recipe would arm the wrong module against prime. `Run-Shed`: the full disposition table — status absent then spawn then re-read; spawn error → hard error; status still absent after a successful spawn → hard error; done → `Done`; blocked/paused/failed → hard error naming the child's state, error and `current_producer`; running → exactly one `deps.Sleep` call then `Stuck`; unrecognised state → hard error; context cancellation surfacing as an error, never as `Stuck`. Two assertions carry more weight than the rest: that `Stuck` is returned for the still-running case **and no other**, since that is what makes the static self-route safe; and that a second `Call` against a child whose status now exists does **not** call `deps.Spawn` again, which is the re-entry-safety property the read-before-spawn ordering exists for. Fake `deps.Now`/`deps.Sleep` throughout.
- **`internal/battenrecipe`** — the coverage guard pinning row-name constants against `batten-recipe.yaml` must be extended to the new four-row shape and must assert the **value** `"Run-Shed"` explicitly, the same way `recipe_test.go` today pins `"Loom-Run"` against a symmetry-minded rename. Add a test asserting `Run-Shed`'s `on_stuck` self-route and its explicit `max_bounces: 1440`/`poll_interval_s: 30` pair, since a dropped `max_bounces` would silently inherit a default budget of a few bounces and turn every long child run into a spurious `StateBlocked` — the highest-consequence, lowest-visibility failure in this task. Pin the two values against the 12-hour wall-clock they encode, so a later interval change cannot leave the budget stale.
- **`internal/shedrecipe`** — a constructor test for the `SeedChild` entry covering `configRejectUnknown` and each `requireNonEmpty`/`requireAbsRoot`/`requireSeam` field, matching the existing entry tests. `coverage_guard_test.go`'s cross-consumer engine-set check needs the renamed package's `RecipeEngines()`.
- **`internal/battencli`** — path-derivation tests pinning status, seed, run-lock and status-lock paths to **prime's** anchor and under the new segment. These are one of the Batten Bookend Invariant's two mechanical proxies; they must not be weakened while being relocated. Also cover the auto-seed's placement and gating: a first `lyx batten run <slug>` and a first `lyx batten step <slug>` each seed before `wire` builds the `Env`, while `status` and `pause` against an unseeded run-id seed nothing and refuse with the run-id listing — the read-only-verbs-create-no-state rule, which is easy to lose in a refactor of `arm`. Assert the two absences stay distinct: no seed → listing refusal; seed present but no `status.json` → batten's `found: false`. Cover the new `step` path end to end, since none of `run`'s coverage touches it: `PreStep` seeding an absent status before `shed.Step` reads it, each refusal-kind mapping (`KindBusy` on a held run lock, `KindUnseeded` on a failed seed or decode, `KindBootstrap` otherwise), and `BuildShed` being non-nil for `verb == "step"`. A `step` against a fresh slug must never surface `kind: "producer"`, which is what an unseeded status would produce and what `ly-drive` would then retry in a loop. Conflating them is the likeliest regression, since both are "nothing here yet" to a casual reader. Plus tests on the `CommitStatus` seam: that it is non-nil now that the status is durable; that a repeated `(producer, state)` pair commits and pushes **once**, not twice; and — the one that actually matters — that the skip still holds when the seam is rebuilt from scratch between calls, which is what proves the decision's on-disk marker rather than an in-closure variable. A test that only exercises one long-lived seam instance would pass against the broken in-memory design, so it proves nothing on its own. Also cover a missing/corrupt marker falling back to committing once rather than erroring.
- **`internal/loomcli`** — the relocated status path, and `lyx loom start` writing a seed alongside its status seed, including `params.parent`. Then the seed-presence branch: all four generic verbs refuse with the run-id listing when no seed is present, **including `run` and `step`**, which must not auto-seed — that asymmetry against batten is loom's only-`start`-may-seed discipline and is the thing a plan writer is most likely to "fix" into symmetry. Also cover a status file present with no seed taking the same refusal. `resolveParentBranch`'s existing disagreement table stays as-is and its tests should not need touching — if they do, that is a signal the parent decision drifted.
- **`internal/boardengine`** — the new `type` field round-tripping through both structs, `omitempty` keeping existing records byte-identical, and an absent value reading back as the empty string (with the empty-means-`loom` rule tested at the seeding site, not here). The load-bearing one: an `UpsertTask` carrying `type` **succeeds**, which is what proves the `upsertAllowedKeys` entry landed — without it the field is unsettable and every other test here still passes.
- **Integration (`integration`-tagged)** — one end-to-end batten run over a `hubforge`-built fixture: create → seed child → watch a stubbed child status to `done` → teardown, asserting the child's `_lyx/shed/self/seed.json` exists and is committed. Plus a step-driven variant proving a still-running child yields a re-entrant step that returns after one poll interval instead of holding for the child's whole duration — that bounded return, not non-blocking behaviour, is what the re-entrancy decision buys.
- **Sandbox suite** — the Sandbox Suite Coverage invariant requires every registered module to be exercised or explicitly excluded with a reason; the `lifecycle`→`batten` rename and the new `lyx shed seed` verb both touch that registration.
- **Help-tree tests** — the CLI/Cobra Invariant's help-tree coverage needs the retired `lyx lifecycle` subtree removed and `lyx batten` (including its new `step`) plus `lyx shed seed` added, each with a non-empty `Short`.

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
- **Q:** How is `Run-Shed`'s re-entrant "still running" outcome expressed, given `Outcome` has only `Done` and `Stuck`? **A:** [auto-pick] One status read per `Call`; still-running sleeps and returns `Stuck` routed `on_stuck: Run-Shed`, bounded by an explicit `max_bounces` replacing `poll_attempts`, with the interval raised 5s→30s. **Why:** it delivers the design's step-friendly form without touching `shedengine`, which the design excludes.
- **Q:** Does `Seed-Child` commit the seed it writes, or only write it? **A:** [auto-pick] Write then commit, through `fabricengine`. **Why:** the seed is durable `_lyx` content, and leaving it uncommitted breaks the machine-switch case the durability decision exists for.
- **Q:** With `on_stuck: Run-Shed` set, how does a blocked/paused/failed child escalate? **A:** [auto-pick] A hard error carrying the child's state, error and `current_producer` — not `Stuck`. **Why:** review round 1 established that `ProducerDef.OnStuck` is static, so a self-route applies to every `Stuck` from the row; "Stuck with no self-route" cannot exist, and a bouncing blocked child has no sleep to pace it and would burn the entire budget in a tight loop.
- **Q:** What re-establishes the "destructive teardown is unreachable from a failure path" property, now that `Run-Shed`'s `on_stuck` is no longer empty? **A:** [auto-pick] The hard-error return, which halts the run at `StateFailed` on `Run-Shed` so `Worktree-Teardown` is never routed to; the recipe header comment is rewritten to say so. **Why:** the routing that carried the property is gone, so the property has to be re-stated against the mechanism that now carries it — the single easiest thing to get wrong in this task.
- **Q:** What is the real cost of a re-entrant row, given `persist` calls `CommitStatus` on every transition? **A:** [auto-pick] A commit *and* a push per bounce, ~1440 of each per 12-hour child — mitigated by making batten's `CommitStatus` seam skip a repeated `(producer, state)` pair, not accepted as-is. **Why:** review round 1 found the original cost statement counted only history entries and omitted the expensive half; the already-committed pair is exactly what a resuming machine needs, so skipping the no-op loses nothing durable.
- **Q:** Does `max_bounces` inherit today's attempt *count* (8640) or today's wall-clock (12h)? **A:** [auto-pick] The wall-clock: 12h at the new 30s interval, so `max_bounces: 1440`. **Why:** reusing the count would silently stretch the watch window to 72 hours.
- **Q:** `innerRunProducer` spawns unconditionally before polling — what stops a re-entrant row spawning the child 1440 times? **A:** [auto-pick] Invert the order: read the child's status first, spawn only when it is absent, and make "still absent after a successful spawn" a hard error. **Why:** review round 2 found the spawn's idempotency undefined under re-entry; the child's own status file is a durable, resume-safe record of whether the spawn happened, so no extra marker is needed and the hard error closes the respawn loop.
- **Q:** Does a step against a still-running child block? **A:** [auto-pick] Yes, for one 30s poll interval — stated plainly, with the testing bullet's "returns control rather than blocking" wording corrected. **Why:** the sleep stays inside `Call` and the row cannot see which verb drove it, which is the same reason `wait_mode` was rejected; the property actually bought is a bounded return, not a non-blocking one.
- **Q:** Where does `lyx shed seed`'s body live, given the subtree's pre-run reads a seed, gates the verb and arms a recipe before any subcommand runs? **A:** [auto-pick] A `shedcli`-only command, exempted from the pre-run by the same `cmd.Name()` short-circuit that already exempts the bare `shed` group. **Why:** `seed` is by definition invoked when no seed exists and belongs to no recipe's verb set, so all three pre-run steps would refuse it; `shedverbs` is the wrong home because it knows neither paths nor the recipe table.
- **Q:** Does batten's `CommitStatus` use the loom-shaped seam or the Board's `Bolt`, and how does it serialise against Board writes to the same `weft:main`? **A:** [auto-pick] The loom-shaped seam (`CommitAnchoredPaths` + `PushAnchored`, positive-only pathspec); no Board push lock, with a rejected push taking the seam's existing warn-and-catch-up disposition. **Why:** `Bolt.Commit` stages every change in its repo, which the Fabric Git Invariant forbids a weft-commit caller; and the no-op skip already reduces batten to ~4 commits per run, so contention is rare rather than per-30-seconds.
- **Q:** With one layout, what does "seed present but `status.json` absent" mean, and which `AbsentDisposition` applies? **A:** [auto-pick] It means seeded-but-never-started; the disposition stays told per-recipe, with loom refusing and batten reporting `found: false`, only the refusal text changing to name the run-id. **Why:** the unified layout creates a state the two old layouts could not express, but it does not make the two recipes want the same answer.
- **Q:** Does `table.entry.Args` survive the run-id positional? **A:** [auto-pick] Deleted, along with the `argsFor()` closure; `cobra.MaximumNArgs(1)` is set statically on each verb. **Why:** the positional is now universal, so a per-recipe arity contract has nothing to vary — with the one visible consequence that `lyx batten run` with no argument becomes a `self` address, which batten refuses by name (see its `self`-address refusal) rather than as an arity error.
- **Q:** The no-op commit skip remembers the last pair in a closure — what happens in step mode, where every invocation is a fresh process? **A:** [auto-pick] Move the skip's memory on disk, to an ephemeral `.lyx/shed/<run-id>/last-commit` marker. **Why:** review round 3 found an in-memory skip works for `run` mode only, leaving step mode — the mode the re-entrancy decision exists to enable — committing and pushing once per step; a lost marker costs one redundant commit, never correctness.
- **Q:** Who resolves the `*lyxcwd.Location` the pre-run's seed read needs, given the pre-run deliberately holds only a cwd string today? **A:** [auto-pick] `shedcli` resolves it, and `Arm` gains a Location-taking form so it happens once; refusal precedence is stated explicitly. **Why:** a seed read is a path read and needs an anchor, so the "each module's Arm owns its own resolution" rule cannot survive intact — and the reorder makes `lyx shed run <slug>` from a task worktree refuse with a run-id listing instead of the non-prime message, which is worth stating since `lyx batten run <slug>` still gives the clearer one.
- **Q:** Batten's auto-seed sits in a `PreRun` hook that fires after `wire` has already built the `Env` — does the seed exist when the `Env`'s readers need it? **A:** [auto-pick] No; move the auto-seed into `arm` ahead of `wire`, gated to `run` and `step`, and keep every seed-param read lazy inside its closure. **Why:** review round 3 found the hook runs inside `run`'s `RunE`, so a first `lyx batten run <slug>` would wire over a seed that does not exist yet; `status`/`pause` stay excluded because read-only verbs must not create state.
- **Q:** Does batten gain a `step` verb? **A:** [auto-pick] Yes — its table entry gains `"step"`, plus a `PreStep` hook with its refusal-kind mapping, a `BuildShed` arm for `"step"`, `Step` verb texts, and `StepBusyMessage`/`StepBusyKind`. **Why:** the whole point of the re-entrant `Run-Shed` row is that an outer run can be stepped, so a batten without `step` leaves that row with no caller; review round 7 then found the table entry alone insufficient — batten's status seeding lives in a `PreRun` hook `step` never calls, and `specFor` fills `BuildShed` for `"run"` only, so a stepped batten would hard-error on the read gate and report `kind: "producer"`, the one kind `ly-drive` retries.
- **Q:** Which existing `CONSTRAINTS.md` lines does this task invalidate? **A:** [auto-pick] Five, all amended in the same commit — enumerated under Constraints. **Why:** review round 7 found the doc sweep's literals and paths both excluded `CONSTRAINTS.md`, so the Told-Geometry bound list, the interactive-handoff exception, the CLI/Cobra deviations, the Bookend invariant and the Shed Verb-Set no-resolver bullet would all have gone stale unnoticed.
- **Q:** What `recipe` does prime's own batten seed carry — `batten`, or the Board task's `type`? **A:** [auto-pick] `batten`. The Board `type` is the *child's* recipe and travels only in the child's seed; `Seed-Child` reads it fresh from the Board, while the child's driver comes from prime's `params.child_driver`. **Why:** review round 4 caught the two conflated — a prime seed carrying `loom` would make `lyx shed status <slug>` arm `loomcli` against prime, the wrong recipe against the wrong worktree.
- **Q:** Who owns the `shed`-segment paths that are not per-run — the prime run lock, the last-commit marker, the anchor-relative status form? **A:** [auto-pick] `shedrun`, all of them; its constructor set is enumerated in full so the new invariant has no exceptions. **Why:** the invariant says no other production file names the segment in path-construction context, and three paths had been left assigned elsewhere or to nobody. `loomengine.LoomStatusRel` is deleted rather than relocated, and batten stays `PrimeRunLock`'s only caller while `shedrun` declares it.
- **Q:** What arity do `lyx loom`'s own verbs take, given they declare no `Args` validator at all today? **A:** [auto-pick] `MaximumNArgs(1)`, the same as the other two surfaces; `TestShed_LoomRunRefusesExtraArgByNoArgs` is deleted along with the table's `cobra.NoArgs` for loom. **Why:** review round 5 found that without a decision a run-id typed at `lyx loom status <run-id>` would be silently swallowed and address `self` — the worst of the options — and one uniform arity contract across all three surfaces leaves no rule to remember.
- **Q:** Which code performs the seed-presence check on the `lyx batten` path, given batten's arm knows its recipe without reading a seed? **A:** [auto-pick] `battencli`'s own `arm`, sharing one `shedrun`-level helper with `shedcli`; `run`/`step` auto-seed when absent, `status`/`pause` refuse with the listing. **Why:** review round 5 found three passages asserting batten lands on the listing refusal while nothing on that path was specified to read a seed — the claims were true of the intent and false of the design.
- **Q:** Does `Seed-Child` push after committing the child's seed? **A:** [auto-pick] Yes, with a failed push warning and still returning `Done`, matching the `CommitStatus` seam. **Why:** review round 6 noted the row's rationale is the machine-switch case, which a local commit never reaches — an unpushed seed is as lost to a fresh clone as an uncommitted one.
- **Q:** Which code performs the seed-presence check on the `lyx loom` path, and do `run`/`step` auto-seed there? **A:** [auto-pick] `loomcli.Arm`, and no — all four generic verbs refuse with the run-id listing when no seed exists; only `lyx loom start` seeds. **Why:** loom's existing discipline is that only `start` may seed, stated in `lifecyclePreRun`'s own doc comment and extended here from the status file to the seed; it is also what makes the no-migration claim true on this path, since a pre-task worktree has no `_lyx/shed/` and refuses with an empty listing.
- **Q:** What does an argument-less `lyx batten run` do, given the auto-seed gate would otherwise fire on the `self` default? **A:** [auto-pick] Batten refuses a `self` address by name, before the auto-seed gate. **Why:** review round 6 found the two passages contradicted each other, and under the stated order it would have written prime a `self` seed with an empty `params.slug`; prime hosts many slug-addressed batten runs, so `self` has no meaning there.
- **Q:** Can the Board's `type` field actually be written? **A:** [auto-pick] Only once `"type"` is added to `upsertAllowedKeys`; with that entry no new flag is needed, since `lyx board upsert` takes a JSON payload. **Why:** review round 5 found the allowlist is closed and enforced for both upsert paths, so the field would have been permanently unsettable and `Seed-Child` would have read empty forever.
- **Q:** Where does `child_driver`'s value come from at auto-seed time? **A:** [auto-pick] A `--child-driver` flag on `lyx batten run|step`, defaulting to `go`, alongside `--driver` for batten's own; both refuse `llm` for now. **Why:** the seed was specified as carrying the param with no source named for it; having the flags exist now means the next roadmap item changes a default rather than a surface.
- **Q:** Does the no-op skip contradict `shedengine.persist`'s documented contract? **A:** [auto-pick] Yes, and the doc comment is qualified in the same commit — the engine still calls unconditionally, a caller's seam may skip, and batten's key is deliberately blind to `history`/`error`. **Why:** the comment explicitly rules out conditioning on `nextCurrentProducer`, which is one layer in from what batten now does; leaving it would make the next reader trust a contract the code no longer keeps.
- **Q:** Does deleting `table.entry.Args` change `lyx batten run`'s arity? **A:** [auto-pick] No — that deletion touches the `lyx shed` path alone; `battencli`'s own `ExactArgs(1)` is a separate change, relaxed to `MaximumNArgs(1)` on `run`/`status`/`pause` and matched by the new `step`. **Why:** review round 4 caught the mis-attribution; the two paths share the arity contract by both carrying the same value, not by one reading the other's, so `TestParity_PositionalArgs` must be rewritten to assert them independently.
- **Q:** Does skipping the no-op commit break the bounce budget across a machine switch? **A:** [auto-pick] Yes, and it is accepted — an uncommitted history run means a resumed watch restarts its 12-hour window from zero. **Why:** it extends the window rather than truncating it, so the failure mode is watching a dead child too long rather than abandoning a live one, and a fresh window is the better default at exactly that moment.
- **Q:** Is `self` reserved against a Board slug? **A:** [auto-pick] Yes — `ValidateRunID` still accepts it (it must be addressable), and a separate `IsReserved` check refuses it at the seeding site when the run-id derives from a Board slug. **Why:** prime's namespace holds one directory per slug plus potentially its own `self`, so a task slugged `self` would collide with the reserved meaning; addressing `self` is always legal, only claiming it is not.
- **Q:** Are `8640`/`5s` recipe config keys? **A:** [auto-pick] No — they are Go constants in `entries_lifecycle.go`, and the recipe carries no `config:` block at all; the row gains `config: {poll_interval_s: 30}` plus a row-level `max_bounces: 1440`, which are two different surfaces. **Why:** `max_bounces` is a `shedbuild.Row` field rather than a config key, so "retiring `poll_attempts` in favour of that budget" was conflating them; retiring the key also means deleting its default constant, its negative guard, and its `configRejectUnknown` allowlist entry.
- **Q:** How are the retired paths and the retired flag enumerated for the doc sweep? **A:** [auto-pick] By literal (`_lyx/loom`, `.lyx/lifecycle`, `--recipe`, `lyx lifecycle`) across `contracts/`, `plugins/`, `tools/`, `docs/` and `manifest/`, not by Go caller. **Why:** review round 2 found six prose artefacts a symbol grep misses, including a deployed normative spec whose already-deployed copy `stencilstore` will never overwrite — so the task states the operator's delete-and-re-seed step rather than adding a force-sync path the Stencil Ownership Invariant forbids.


### From _mill/plan/00-overview.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
slug: seeded-shed-core
approved: true
started: '20260919-171012'
parent: main
root: ""
verify: go build ./...
discussion_sha: cbc91bd080b37b09198b7a76c5ece6b95ddd0904
```

### From _mill/plan/01-shedrun-leaf.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: shedrun-leaf
number: 1
cards: 4
verify: go test ./internal/shedrun/...
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/shedrun/doc.go`
  - `internal/shedrun/paths.go`
  - `internal/shedrun/paths_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedrun/runid.go`
  - `internal/shedrun/runid_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/seed_test.go`
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-board-type-field.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: board-type-field
number: 2
cards: 2
verify: go test ./internal/boardengine/...
depends-on: []
```



- **Edits:**
  - `internal/boardengine/task.go`
  - `internal/boardengine/store.go`
  - `internal/boardengine/task_test.go`
  - `internal/boardengine/store_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/boardengine/board.go`
  - `internal/boardengine/board_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-batten-rename.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-rename
number: 3
cards: 5
verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/battencli/... ./internal/shedrecipe/... ./internal/shedcli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
depends-on: [1]
```



- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `contracts/recipes/recipes.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/table_test.go`
  - `internal/shedcli/cli.go`
  - `internal/shedcli/cli_test.go`
  - `internal/shedcli/doc.go`
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/testmain_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/notransients_test.go`
  - `internal/loomrecipe/recipe_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/newshed_test.go`
  - `internal/shedrecipe/coverage_guard_test.go`
  - `internal/shedverbs/pause_test.go`
  - `internal/shedverbs/status_test.go`
  - `internal/shedverbs/seam_enforcement_test.go`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-loom-run-directory.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: loom-run-directory
number: 4
cards: 4
verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/...
depends-on: [1]
```



- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/loomstatus_test.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/battencli/wire.go`
  - `internal/battencli/paths.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/sharedbootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/arm.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/wiring_commitstatus_test.go`
- **Creates:**
  - `internal/loomcli/arm_seed_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-batten-producers.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-producers
number: 5
cards: 5
verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/shedrecipe/... ./internal/shedengine/...
depends-on: [3]
```



- **Edits:**
  - `internal/battenshed/innerrun.go`
  - `internal/battenshed/innerrun_test.go`
  - `internal/battenshed/deps.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/battenshed/deps.go`
- **Creates:**
  - `internal/battenshed/seamchild.go`
  - `internal/battenshed/seamchild_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedrecipe/entries_batten.go`
  - `internal/shedrecipe/entries_batten_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `contracts/recipes/batten-recipe.yaml`
  - `internal/battenrecipe/names.go`
  - `internal/battenrecipe/recipe_test.go`
  - `internal/battenrecipe/coverage_guard_test.go`
  - `internal/battenrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/06-batten-wiring.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-wiring
number: 6
cards: 6
verify: go build ./... && go test ./internal/battencli/...
depends-on: [1, 2, 3, 5]
```



- **Edits:**
  - `internal/battencli/paths.go`
  - `internal/battencli/paths_test.go`
  - `internal/battencli/wire.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/battencli/commitstatus.go`
  - `internal/battencli/commitstatus_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/battencli/wire.go`
  - `internal/battencli/wire_test.go`
  - `internal/battencli/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/battencli/arm.go`
- **Creates:**
  - `internal/battencli/arm_seed_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/battencli/arm.go`
  - `internal/battencli/cli.go`
  - `internal/battencli/cli_test.go`
- **Creates:**
  - `internal/battencli/step_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/battencli/cli.go`
  - `internal/battencli/cli_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/07-shed-addressing.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: shed-addressing
number: 7
cards: 5
verify: go build ./... && go test ./internal/shedcli/... ./internal/loomcli/... ./internal/battencli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
depends-on: [1, 3, 4, 6]
```



- **Edits:**
  - `internal/loomcli/arm.go`
  - `internal/battencli/arm.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/shedcli/cli.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/doc.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedcli/seed.go`
  - `internal/shedcli/seed_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedcli/table_test.go`
  - `internal/shedcli/cli_test.go`
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/helptree_test.go`
  - `internal/shedverbs/step.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/08-docs-and-integration.md


```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: docs-and-integration
number: 8
cards: 5
verify: go build ./... && go test ./cmd/lyx/... && go test -tags integration ./internal/battencli/...
depends-on: [4, 6, 7]
```



- **Edits:**
  - `contracts/specs/loom-status-spec.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/discussiontemplate_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
  - `plugins/ly/skills/INDEX.md`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/seeded-shed.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/battencli/lifecycle_integration_test.go`
  - `internal/battencli/arm.go`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/seeded-shed-core add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/seeded-shed-core rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/seeded-shed-core rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/seeded-shed-core` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/seeded-shed-core`.
