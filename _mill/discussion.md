# Discussion: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
slug: self-report-tier2
status: discussing
parent: main
```

## Problem

Millhouse's `mill-self-report` works because in Millhouse the LLM *is* the orchestrator: one session holds the whole run's conversation and can retrospectively notice friction across everything it just did.
`loom` inverts this on purpose — "No agent knows about rounds, gates, N-caps, finalize, or the others. Each phase becomes a pure function over files."
There is structurally no single LLM session with full-run context to reflect from, so porting Millhouse's self-report model directly does not work here.

Today `lyx selfreport create` exists as a shipped primitive (`internal/selfreportcli` + `internal/selfreportengine`) but has exactly one trigger: a human typing the command.
A task driven by plain `lyx loom run` with nobody watching live therefore produces zero self-reports, no matter how much friction the agents hit along the way — every observation any of them made dies with its pane.
Tier 2 closes that gap by adding a second, automatic trigger: each spawned agent may drop a short friction note about its own scoped task, Go collects them, and one dedicated fresh-context reflection agent decides at the end of the run whether anything is worth filing.

**Why now:** `lyx selfreport create` is shipped and unused in the autonomous path, and `manifest/roadmap.md` has this as a Planned item independent of Tier 1 and of `lyx loom step` — no code dependency in any direction, so all three can build in parallel.

## Scope

**In:**

- A new leaf package `internal/friction`, mirroring `internal/pattern`'s shape: it owns the friction-note directive text (read from stencils at call time, one variant per agent role) and the note-path composition given a told directory.
- Four new directive stencils under `contracts/stencils/friction/`, one per role, registered in `contracts/stencils/stencils.go`.
- A new `loomengine.LoomFrictionDir(l)` accessor, built on `LoomScratchDir`, mirroring `LoomReviewsDir`.
- Injection of an optional `{{.friction_directive}}` marker into **seven** existing agent prompts and their seven composers: Discussion-Write (`internal/loomengine/prompt.go`'s `composePrompt`), Plan-Write (`internal/loomengine/plan.go`'s `composePlanPrompt`), the Burler review+fix round (`internal/burlerengine/engine.go:104`), and all four `internal/websterengine/render.go` composers — `RenderForkPrompt` (`:145`), `RenderRecoveryPrompt` (`:177`), `RenderIntegrationPrompt` (`:210`), and `RenderMasterPrompt` (`:264`).
- Three `stencil.Fill` → `stencil.FillOptional` conversions, at the three of those seven composers not already using `FillOptional`: `internal/loomengine/prompt.go:30`, `internal/websterengine/render.go:162` (fork), and `internal/websterengine/render.go:225` (integration). `RenderForkPrompt` additionally gains an `anchorRoot`-equivalent told parameter, which it does not take today.
- A single `os.MkdirAll` of the friction directory in `internal/loomcli/run.go`, beside the seed-time clear.
- A new engine package `internal/frictionengine` exposing `Reflect(Deps) (Report, error)`: it scans the friction directory, skips out when empty, otherwise spawns one autonomous reflection agent over the aggregated notes via the told `Shuttle` seam, and archives the consumed notes.
- One new stencil for the reflection agent's own prompt.
- A single call site in `internal/loomcli/drive.go`, immediately after `shed.Run` returns, firing on `RunDone` and `RunBlocked`.
- A once-per-task clear of the friction directory in `internal/loomcli/run.go`, beside the existing `loomshed.Seed` call, on the genuine-first-seed branch only.
- Two new `loom.yaml` keys — `friction` (a model-spec that doubles as the on/off switch) and `friction_timeout_min` — plus their `internal/loomengine/template.yaml` defaults and `Config` fields.
- A new **Friction Leaf Invariant** in `CONSTRAINTS.md`.
- Doc updates in the same commit: `manifest/designs/self-report-tier2.md` (close its three Open questions), `docs/overview.md` (two new modules in the module table), `manifest/roadmap.md` (Planned → Done for this item).

**Out:**

- Tier 1 (Go-detected structural anomalies from loom's status file). Separate roadmap item, separate design doc, no shared code.
- `lyx loom step` and its supervisor skill. Separate roadmap item; the scoping relationship is documentation only.
- Cross-phase semantic friction — a pattern visible only across several phases but not Tier-1-detectable. The design doc defers this explicitly; a Tier 2 note can only ever describe friction inside its own narrow task, and that limitation is accepted, not worked around.
- Any change to `internal/selfreportengine` or `internal/selfreportcli`. Tier 2 adds a trigger on top of the shipped primitive; the primitive itself is untouched.
- Deduplication against already-open GitHub issues.
- A new CLI verb. There is no `lyx friction …` subtree and no `--no-friction` flag; the config key is the whole control surface.
- Per-producer / per-profile opt-in. One global key, not a recipe key on five different engine rows.
- The Bouncer's seed and judge agents, and the `mergeresolve` conflict-resolution agent. See Decisions.
- Making the reflection step a `shedengine` producer row in `contracts/recipes/loom-recipe.yaml`. See Decisions.

## Decisions

### Notes live under the ephemeral `.lyx` tree, never `_lyx`

- **Decision:** friction notes are written to `.lyx/loom/friction/`, reached through a new `loomengine.LoomFrictionDir(l *lyxcwd.Location) string` built on `LoomScratchDir(l)`, exactly as `LoomReviewsDir` already is.
- **Rationale:** the Durable-vs-Ephemeral State Invariant says every never-tracked file lives under `.lyx` at the mirrored subpath of the `_lyx` content it relates to.
  A friction note is consumed within the run and then discarded — it is never a reviewable artifact, never a merge participant, and nothing downstream reads it after the reflection step.
  Putting it under `_lyx` would additionally drag in the Fabric Git Invariant: weft content must be committed by Go at a loop-owner boundary with a positive-only `fabricengine.ScopedPathspec`, which would mean a new commit seam per producer for a file that is deleted minutes later.
  `.lyx/loom/reviews/` already proves agents can write freely into this tree — every Bouncer verdict, Burler round report, and focus file lands there.
- **Rejected:** `_lyx/friction/` (drags in the commit-seam machinery for a throwaway file); appending notes into `_lyx/loom/status.json` (that file is `internal/state`'s serialized Shed status with a fixed schema, and agents must never write it).

### `internal/loomcli/run.go` owns creating the directory, once, before any producer runs

- **Decision:** the friction directory is created by a single `os.MkdirAll` in `internal/loomcli/run.go`, immediately beside the seed-time clear.
  The **clear** is first-seed-only (nil-error `Seed` branch); the **create** is unconditional, on both the first-seed and the `loomshed.ErrSeedExists` re-entry branches, so a resumed run whose `.lyx` tree was swept still gets a directory.
  No composer and no agent creates it.
  A failed `MkdirAll` logs at `Warn` via `internal/logger` and the run continues — it never fails `lyx loom run`.
- **Rationale:** without an assigned owner, every note write depends on provider-specific parent-directory creation, and a write that silently fails produces exactly the zero-notes-with-no-signal state the missing-marker warn decision exists to prevent.
  `internal/burlerengine/engine.go:112` is the in-tree precedent: it `MkdirAll`s its own `.lyx/burler` directory before writing into it.
  One create at one call site beats seven composer-side creates racing each other, and `run.go` is already the once-per-task hook holding the clear.
  The failure is a `Warn` rather than an error for the same reason every other Tier 2 failure is: optional bookkeeping must never fail a task's run.
- **Rejected:** composer-side creation per injection (seven call sites racing, and the webster fork path runs concurrently); relying on the agent's own tooling to create parents (provider-specific and unverifiable); hard-erroring on a failed create (fails a real run over optional bookkeeping); creating it inside `Reflect` (far too late — every note is written long before the reflection step).

### Filename uniqueness comes from a caller-supplied id, never invented by the agent

- **Decision:** `friction.NotePath(frictionDir, id string) string` returns `filepath.Join(frictionDir, id+".md")`, where `id` is a caller-supplied identity string the caller already has.
  The composing caller passes the resulting absolute path into the directive text, so the agent is told exactly one path and never invents a filename.
  `NotePath` sanitizes `id` (reject empty; reject any entry containing a path separator or `..`) rather than trusting it.
- **Rationale:** every spawn site already holds a unique identity — the producer row name for Discussion-Write and Plan-Write, the round number plus run subdir for a Burler round, the batch identity for a webster fork.
  Deriving a name from a clock instead would need an injected `func() time.Time` in five packages and would still collide between two forks started inside the same second.
- **Rejected:** letting the agent choose its own filename (an agent that picks a colliding name silently overwrites another agent's note); a timestamp-derived name (needs a clock seam everywhere and still collides).

### The directive is injected the way `internal/pattern` already does it

- **Decision:** a new leaf package `internal/friction` exposes `Directive(frictionDir, notePath, stencilsDir string, role Role) (string, error)`, returning role-worded text read from a stencil at call time.
  Each of the seven composers fills a `{{.friction_directive}}` marker via `stencil.FillOptional`, so the marker renders as nothing when Tier 2 is off.
  Roles mirror `pattern`'s three: `RoleImplementer` (webster fork, loom plan), `RoleReviewFix` (burler round), `RoleOrchestrator` (webster Master), plus `RoleInterview` for the Discussion-Write interview agent, whose prompt is neither editing nor reviewing.
- **Rationale:** `internal/pattern` is the exact architectural precedent in this tree for "inject an optional, role-worded directive block into several agent prompts".
  Four of the seven composers this task touches are already `pattern`-wired and already call `FillOptional` — `RenderRecoveryPrompt` (`render.go:183`), `RenderMasterPrompt` (`render.go:270`), the Burler round (`engine.go:104`), and Plan-Write (`plan.go:71`) — so the injection mechanism is proven at those four.
  The remaining three — Discussion-Write, the webster fork, and the webster integration prompt — are new injection points, and are exactly the three needing the `Fill` → `FillOptional` conversion.
  The Stencil Ownership Invariant requires the prompt text be read at call time from a told absolute stencils directory, never embedded bytes, which the `stencilstore.Read` path satisfies.
- **Rejected:** a `shuttleengine.Spec` field that shuttle renders into the prompt (`shuttleengine`'s own `Spec.Prompt` doc states shuttle never templates prompt content — dumb transport, like reed); pasting the same paragraph inline into each of the seven stencils (seven copies to keep in sync, and no way to switch it off).

### `internal/friction` is a leaf; the reflection step is a separate `internal/frictionengine`

- **Decision:** two packages.
  `internal/friction` imports stdlib, `internal/logger`, `internal/stencil`, and `internal/stencilstore` only, and is imported by `loomengine`, `burlerengine`, and `websterengine`.
  `internal/frictionengine` holds the reflection step, imports `shuttleengine` for the `Shuttle` seam, and is imported by `loomcli` alone.
  A new **Friction Leaf Invariant** goes into `CONSTRAINTS.md` in the same commit, pinned by a `leaf_enforcement_test.go` modelled on `internal/pattern/leaf_enforcement_test.go`.
- **Rationale:** this is exactly the `pattern` (leaf, four importers) / `selfreportengine` (engine, one importer) split the tree already uses, and it keeps the prompt-injection half out of the dependency cone of `shuttleengine`.
  Collapsing them would put a `shuttleengine` dependency into the package three prompt composers import for one string.
- **Rejected:** one combined `internal/friction` package (widens the leaf's import set for no gain); putting `Directive` on `loomengine` (`burlerengine` and `websterengine` would then import `loomengine`, which they do not today and must not start doing).

### Five spawn sites get the directive; Bouncer and mergeresolve do not

- **Decision:** the directive is injected at Discussion-Write, Plan-Write, the Burler review+fix round, the webster implementer fork, and the webster Master.
  The Bouncer's seed and judge agents and the `mergeresolve` conflict-resolution agent are deliberately excluded.
- **Rationale:** the five included sites are the agents that do substantive work over the codebase and therefore have something to be frustrated by.
  The Bouncer's seed and judge passes are narrow judgment calls against a strict rubric whose output is parsed by `shedadapters`; adding a side-file instruction to a prompt whose contract is "produce exactly this verdict shape" risks polluting the verdict for a low-value note.
  `mergeresolve` runs inside Finalize, which is the very boundary the reflection step fires after — a note written there would land after aggregation has already read the directory, so it would silently never be seen.
- **Rejected:** all nine spawn sites (adds an ordering hazard at `mergeresolve` and a verdict-contract hazard at the Bouncer); only the four sites `pattern.Directive` already covers (leaves the discussion interview, the one agent that talks to a human about the task's shape, with no way to record friction when running autonomously).

### The reflection step fires from `loomcli/drive.go`, not from a recipe row

- **Decision:** one call site, in `internal/loomcli/drive.go`, immediately after `shed.Run(cmd.Context())` returns and before the `output.Ok` envelope is written.
  It fires on `shedengine.RunDone` and `shedengine.RunBlocked`, and on nothing else — never `RunPaused`, never a non-nil `err`.
- **Rationale:** `shedengine.Run` returns immediately on `RunBlocked` without calling any further producer (`internal/shedengine/run.go`, the `def.OnStuck == ""` and bounce-budget-exhausted branches both `return` rather than `continue`), so a recipe row can structurally never cover the `stuck` half of the design's two trigger points.
  A row placed after Finalize would cover only the `RunDone` half while still needing a second implementation for `stuck` — one site covers both.
  `RunPaused` is an operator stopping the run mid-flight; the run is not over, and re-filing on every pause would be noise.
  A non-nil `err` is an engine-level fault where the run's own bookkeeping is untrustworthy.
- **Rationale (ordering):** on `RunDone` this means the reflection agent runs after Finalize has already merged and published.
  That is correct — self-report filing is out-of-band bookkeeping, never a gate on landing the work.
- **Rejected:** a `Reflect` row appended after Finalize in `contracts/recipes/loom-recipe.yaml` (covers half the triggers, doubles the implementation, and adds a row to a durable on-disk identity list whose renames break resume).

### One `loom.yaml` key is both the model spec and the kill switch

- **Decision:** `loom.yaml` gains `friction: <model-spec>` and `friction_timeout_min: <int>`, parsed into new `loomengine.Config` fields and resolved through `modelspec.Registry` exactly as `discussion`, `plan`, and `review` already are.
  An absent or empty `friction` value means Tier 2 is off: no directive is injected at any of the five sites, and no reflection step runs.
  `internal/loomengine/template.yaml` ships it populated, so the default is on.
- **Rationale:** the design doc's open question "whether every phase gets this by default, or it's opt-in per producer/profile" resolves to default-on with one global switch.
  Per-row opt-in would mean a Tier 2 config key on five different recipe engines, which is a lot of surface for a feature whose entire value is breadth of coverage — and `loom.yaml` is already where every other run-wide agent knob lives.
  Folding the switch into the model key avoids a second boolean that can disagree with it.
- **Rejected:** a separate `friction_enabled: bool` alongside the model key (two keys that can contradict each other); per-producer `config:` keys in the recipe (five engines to teach, and the recipe row names are durable on-disk identities); a `--no-friction` CLI flag (`loom run` is the unattended path — a flag nobody is present to type is not a control surface).

### The new keys ship in the template, and `lyx config reconcile` is the migration

- **Decision:** both keys go into `internal/loomengine/template.yaml` with the shipped defaults, and an already-seeded worktree migrates by running `lyx config reconcile`.
  Until that is run, `lyx loom` hard-errors with `config file <path>: missing keys: friction, friction_timeout_min; run "lyx config reconcile"` — the existing message, which already names its own remedy.
  "Tier 2 off" is therefore **present-but-empty** (`friction: ""`), never a missing key: a missing key cannot reach the parse at all.
- **Rationale:** `loomengine.LoadConfig` goes through `configengine.Load` (`internal/loomengine/config.go:176`), whose `load` calls `yamlengine.MissingKeys(template, fileBytes)` and returns a hard error listing every template key the file lacks (`internal/configengine/config.go:110–123`).
  This is the standard, already-shipped cost of adding any `loom.yaml` key, and the failure is loud and self-remedying rather than silent.
  Keeping the keys out of the template to avoid it would make the feature permanently off with no way to switch it on, since `Load` resolves the file against the template.
- **Rejected:** omitting the keys from the template (feature can never be enabled); moving `loomengine` to `LoadOrTemplate` (the Config Strictness Invariant says a caller adopts exactly one side, and silently degrading loom's whole config to defaults to accommodate one optional feature is far worse than one migration error); a bespoke pre-parse that tolerates the two keys' absence (duplicates `configengine`'s job and diverges from every other module).
- **Correction this supersedes:** an earlier draft of this discussion claimed an un-migrated worktree would "degrade quietly rather than failing to load". That was wrong — it fails loudly, by design.

### Zero notes means no agent is spawned at all

- **Decision:** `frictionengine.Reflect` reads the friction directory first.
  A missing directory or zero `*.md` entries is the normal, expected case: it logs at `Info` via `internal/logger`, returns a `Report` saying so, and spawns nothing.
- **Rationale:** a clean run produces no notes, and that is the common case.
  Spawning a full LLM session to conclude "nothing to report" on every clean run is pure cost.
- **Rejected:** always spawning (wastes a session on every clean run).

### The reflection agent files the issue itself, via `lyx selfreport create`

- **Decision:** the reflection agent is told to read every note, make the self-report judgment call (worth filing at all? one issue or several? title and body?), and invoke `lyx selfreport create` itself for each issue it decides to file.
  It must additionally write one mandatory output file — its `shuttleengine.Spec.OutputFiles` entry, and therefore its file-contract return value — recording what it filed (issue URLs) or why it filed nothing.
  That file lands beside the notes in `.lyx/loom/friction/`.
- **Rationale:** this is what `manifest/designs/self-report-tier2.md` settled with the operator: "That agent makes the actual self-report judgment call … and invokes the shipped `lyx selfreport create` primitive to do the actual filing."
  There is precedent for an agent driving a `lyx` verb — `contracts/stencils/loom/loom-template-plan.md` instructs the plan agent to use `lyx quarry` as its only source of glyph spellings.
  The mandatory output file is what makes the run's completion detectable at all: `shuttleengine.Spec.OutputFiles` is the completion signal, so a spec with an empty set could never report Done.
- **Noted alternative, deliberately not taken:** having the agent write a structured decision file that Go then feeds to `selfreportengine.CreateIssue` would keep GitHub auth entirely in Go and be easier to test, at the cost of a new parser (and a Sole-Parser obligation) and a departure from a design already settled with the operator.
  If the shelling-out path proves unreliable in practice, this is the fallback to reach for — but it is not this task's scope.
- **Rejected:** Go calling `selfreportengine.CreateIssue` from a parsed agent file (see above); the agent filing with no output file at all (no completion signal, and no audit trail of what was filed).

### Notes are freeform markdown with no parser

- **Decision:** a friction note is freeform markdown.
  The directive asks for a title line naming what the agent was doing and one or two paragraphs of body, and states plainly that the file should be written **only** when something actually went wrong — an absent file is the normal outcome and is never an error.
  Nothing in Go parses a note's contents; `frictionengine` only enumerates `*.md` filenames and hands the reflection agent the directory path.
- **Rationale:** the reflection agent reads them as prose, which is the whole point of using an agent.
  Imposing a schema would create a sole-parser obligation (`internal/planparser`, `internal/discussionparser`, `internal/summaryparser` are each the sole parser of their format, and each carries a CONSTRAINTS.md invariant) for zero benefit.
- **Rejected:** a YAML front-matter schema (a parser, an invariant, and a validation failure mode, all for a file one LLM writes and another LLM reads).

### Consumed notes are archived, not deleted; the clear lands beside the `Seed` call in `run.go`

- **Decision:** after the reflection agent returns **cleanly**, `Reflect` renames `.lyx/loom/friction/` to a timestamped sibling `.lyx/loom/friction-<compact-ts>/` using its injected clock, so a second `Reflect` in the same task cannot re-file the same notes.
  The once-per-task clear is an `os.RemoveAll` of the friction directory in `internal/loomcli/run.go`, placed immediately beside the existing `loomshed.Seed` call and fired **only when `Seed` returns a nil error** — never on the `loomshed.ErrSeedExists` re-entry branch.
  `loomshed.Seed` itself is not touched: its signature (`Seed(statusPath, statusLockPath, slug, parent string) error`, `internal/loomshed/seed.go:37`) gains no friction parameter.
  `lyx loom drive` deliberately never clears.
- **Rationale:** a `loom` run legitimately resumes across process restarts and crash-resumes, so clearing on every drive invocation would destroy notes written before the crash — exactly the notes most worth reading.
  The clear lives at the call site rather than inside `Seed` because `Seed`'s told-parameter list is about status seeding, and widening it for an unrelated directory is a worse seam than one `os.RemoveAll` at the one call site (`internal/loomcli/run.go:101`) that already distinguishes a genuine first seed from a re-entry.
  Drive not clearing is correct rather than a gap: `internal/loomcli/drive.go:61` calls `loomengine.VerifySeedOwnership`, so `drive` requires an already-seeded task and `run` is always what creates one — a drive-only invocation is by definition a resume, which is exactly the case that must keep its notes.
  Archiving rather than deleting keeps the evidence on disk for an operator debugging a filed issue; the whole `.lyx` tree is never tracked and is swept with the worktree.
  The `blocked → operator fixes it → run again → done` path files twice, once per trigger, and the archive is what keeps the second filing about the second run's notes only.
- **Rejected:** deleting consumed notes (destroys the evidence behind a freshly filed issue); clearing at every `lyx loom run` or in `drive` (loses crash-resume notes); threading a friction path into `loomshed.Seed` (widens a status-seeding signature for an unrelated concern); not archiving at all (a `blocked` run followed by a successful one re-files everything from the first half).

### A failed reflection leaves the notes in place, and the report file is never counted as a note

- **Decision:** two halves, both pinned here rather than left to the plan writer.
  **(1)** On any reflection failure — a `Shuttle` error, `OutcomeDied`, `OutcomeTimeout` — `Reflect` does **not** archive.
  The notes stay exactly where they are, so the next trigger in the same task reflects on them.
  Only a clean agent return archives.
  **(2)** The reflection agent's own mandatory output file is named by an exported constant in `internal/friction` (`friction.ReportFileName`, `"reflection-report.md"`), it lands inside the friction directory, and `frictionengine`'s `*.md` note scan **excludes that exact filename**.
  Both the scanner and the reflection `Spec`'s `OutputFiles` entry read the one constant, so they cannot drift.
  Because `shuttleengine.Spec.validate` rejects an `OutputFiles` entry that already exists, `Reflect` deletes any stale report file at that path before composing the spec.
- **Rationale:** the notes are the run's un-acted-on signal — a failure means nothing was reflected on and nothing was filed, so archiving them would silently discard the evidence while creating no duplicate-filing risk to avoid.
  The exclusion is what stops a half-written report from a timed-out run being counted as a note on the next scan, which would otherwise make a failed run look like it produced new friction.
  The stale-file delete is not optional: without it a timed-out run leaves a file that makes every subsequent `Reflect` fail at spec validation rather than at the agent.
- **Rejected:** archiving on failure too (discards un-reflected notes and hides the failure from the next trigger); putting the report file outside the friction directory (a second told path for one file, and the archive would then no longer capture the report beside the notes it was written from); scanning by a glob that happens to miss the report name (an implicit coupling that breaks the first time either name changes).

### A stencil that never got the marker warns, rather than degrading silently

- **Decision:** each of the seven composers calls one exported helper in `internal/friction` that checks the template bytes for the literal `{{.friction_directive}}` marker before filling.
  **The helper itself emits the `Warn`**: it takes the stencil name as a parameter and logs via `internal/logger`, naming the stencil and pointing at `lyx stencil diff` / `lyx stencil sync`.
  The composers do not log; they call the helper and continue.
  It fires only on the enabled-but-marker-absent combination.
  This is why the Friction Leaf Invariant admits `internal/logger` — the leaf's import set follows from the helper logging rather than returning a bool for the caller to log on, and `internal/friction` already pulls `logger` transitively through `internal/stencilstore` (`reconcile.go` logs), so the admission widens nothing in practice.
- **Rationale:** `internal/stencilstore/reconcile.go` never refreshes a `StateEdited` stencil (`:124`, warn-only) and, in dev mode, does not refresh even a `StateUntouched` one (`:98`, warn-and-keep-the-older-copy).
  An existing worktree with operator-edited stencils, or any dev build, therefore keeps templates with no `{{.friction_directive}}` marker.
  `stencil.FillOptional`'s guarantee is one-directional — `unfilledTopLevelMarkers` checks markers in the template that have no value, never a value with no marker — so a directive computed but never rendered is dropped with no error at all, and Tier 2 produces zero notes forever with no signal anywhere.
  `stencilstore`'s own warnings fire at reconcile time, not at prompt-compose time, so they do not cover this.
  The check is on the template bytes rather than the rendered output because the marker is an exact literal there, whereas a rendered-output substring test would depend on how `Fill` transformed the inserted block.
- **Rejected:** accepting silent degradation (a feature that is off with no signal is indistinguishable from a feature that found nothing, which is the exact failure mode Tier 2 exists to fix); hard-erroring on an absent marker (turns an operator's legitimate stencil edit into a failed `loom` run over optional bookkeeping); relying on `lyx stencil sync` being run (nothing makes an operator run it, and a dev build refuses to refresh regardless).

### The reflection step can never change the run's outcome

- **Decision:** every failure inside `Reflect` — a `Shuttle` error, `OutcomeDied`, `OutcomeTimeout`, an unwritable directory — is logged at `Warn` via `internal/logger` and reported in the `lyx loom run` success envelope under a new `friction` key (`"skipped"`, `"reflected"`, or `"failed"`).
  It never sets a non-zero exit, never converts `RunDone` into an error, and never replaces the existing `outcome`/`halted_producer`/`reason`/`history_length` keys.
  The envelope in question is **`lyx loom drive`'s**, not `lyx loom run`'s: `internal/loomcli/run.go:230–232` gives the detached `loom drive` process a driver-log file for stdout and stderr before handing the operator a tmux session, so drive's envelope reaches `loomengine.LoomDriverLog(l)` and nothing else.
  Driver-log-only is **not** accepted as the sole surface: `Reflect` additionally emits a `logger.Info` naming the outcome and, on `"reflected"`, the report file's path.
  The status file is deliberately not a third surface — `_lyx/loom/status.json` is `internal/state`'s serialized Shed status with a fixed schema.
  The three values are exactly what Go can observe for itself: `"skipped"` (no notes found, no agent spawned), `"reflected"` (the agent ran and returned cleanly), `"failed"` (a `Shuttle` error, `OutcomeDied`, `OutcomeTimeout`, or an unreadable directory).
  There is deliberately no `"filed"` value: nothing in Go parses the agent's report file, so whether an issue was actually created is not something this envelope can honestly assert — the agent's report file and the GitHub repo are where that answer lives.
- **Rationale:** the run's outcome is about the task's work.
  Failing a successful, already-merged run because an optional bookkeeping agent timed out would be strictly worse than filing nothing, and the `RunBlocked` case is worse still — an operator staring at a blocked run does not need a second, unrelated failure layered on top.
- **Rejected:** surfacing a reflection failure as a command error (turns a bookkeeping miss into a run failure); swallowing it silently with no envelope key (the operator cannot tell "clean run, nothing to report" from "the reflection agent died").

### The reflection agent runs autonomous, with tools, without forks

- **Decision:** the reflection `shuttleengine.Spec` sets `Interactive: false`, `ForkSubagents: false`, `Model`/`Effort`/`Version` from the resolved `friction` model spec, `Timeout` from `friction_timeout_min`, and `Role: "friction"`.
- **Rationale:** it must read files and run `lyx selfreport create`, which the default tool access covers; it has nothing to fan out over, so forks are unnecessary authorization; and `loom run` is by definition the unattended path, so an interactive spec would hang waiting for a human who is not there.
- **Rejected:** `ForkSubagents: true` (authorization with no user); `Interactive: true` (hangs an unattended run).

## Technical context

**Geometry and path ownership.**
`internal/lyxcwd` owns cwd resolution alone, and a module's own durable subdirectory is its own constant joined onto `AnchorPath()`.
`internal/loomengine/config.go` is where loom's path accessors live: `LoomScratchDir(l)` returns `<anchor>/.lyx/loom`, and `LoomReviewsDir(l)` is built *on* `LoomScratchDir` rather than re-joining the `.lyx` literal — because the Lyxdirs Single-Declarer Invariant forbids naming that literal twice in production path construction.
`LoomFrictionDir` must follow the same rule: build on `LoomScratchDir`, never re-join.

**The `internal/pattern` precedent, in detail.**
`pattern.Directive(anchorPath, stencilsDir string, role Role) (string, error)` reads one of three stencils under `contracts/stencils/pattern/` and returns its text, or `""` when PATTERN is inactive.
Its four call sites are `internal/websterengine/render.go:183` — which is `RenderRecoveryPrompt`, **not** the implementer fork (`RoleImplementer`) — plus `internal/websterengine/render.go:270` (`RenderMasterPrompt`, `RoleOrchestrator`), `internal/burlerengine/engine.go:104` (`RoleReviewFix`), and `internal/loomengine/plan.go:71` (`RoleImplementer`).
Each site fills a `pattern_directive` key and calls `stencil.FillOptional(template, values, []string{"pattern_directive"})` — `FillOptional` is what lets the marker render as nothing.
`internal/pattern/leaf_enforcement_test.go` is the model for the new leaf test.
**The full composer inventory, enumerated against source.**
`internal/websterengine/render.go` carries **four** composers, not two:

| Composer | Line | `pattern` today | `Fill` form today | Friction role |
| --- | --- | --- | --- | --- |
| `RenderForkPrompt` | `:145` | none | `stencil.Fill` (`:162`) | `RoleImplementer` |
| `RenderRecoveryPrompt` | `:177` | `:183` | `stencil.FillOptional` (`:200`) | `RoleImplementer` |
| `RenderIntegrationPrompt` | `:210` | none | `stencil.Fill` (`:225`) | `RoleImplementer` |
| `RenderMasterPrompt` | `:264` | `:270` | `stencil.FillOptional` (`:291`) | `RoleOrchestrator` |

`RenderForkPrompt` is the per-batch implementer fork and takes no `anchorRoot` parameter at all today, so it needs one told path added alongside the `FillOptional` conversion.
`RenderIntegrationPrompt` is a real spawn — the plan's single dedicated integration-suite fork, rendered and written at `internal/websterengine/runlevel.go:548–560` — and gets the directive for the same reason the other implementer-class spawns do.
`RenderRecoveryPrompt` is the cold-start recovery strand and likewise gets it: it is an implementer-class spawn that runs precisely when something has already gone wrong, which is the highest-yield place in the whole run for a friction note.

The remaining two new injection points are outside webster.
Discussion-Write has no `pattern` injection today: `internal/loomengine/discussion.go:37` calls `composePrompt` (`internal/loomengine/prompt.go:17`), which fills `contracts/stencils/loom/loom-template-discussion.md` via plain `stencil.Fill` (`internal/loomengine/prompt.go:30`).
Plan-Write and the Burler round are already `pattern`-wired and already on `FillOptional`, so both take the new marker with no conversion.
Three composers total need the `Fill` → `FillOptional` conversion: `prompt.go:30`, `render.go:162`, `render.go:225`.
`stencil.Fill` is literally `FillOptional(template, values, nil)` (`internal/stencil/stencil.go:20–22`), so each conversion is a one-line change plus the optional-names argument.

**Stencil registration.**
`contracts/stencils/stencils.go` is the single place a stencil's on-disk path and its Go identifier are both named: a `//go:embed <family>/<name>.md` var plus an `entries` row `{"<name>", &Var}`.
`contracts/stencils/registry_test.go` walks the package directory and fails if a `*.md` file has no matching row, or vice versa — so a new stencil file without a registry row fails the build, in both directions.
Seeding, hashing, reading, and validation all belong to `internal/stencilstore`; a hash-mismatched file on disk is never overwritten.

**Spawning an agent from `loomcli`.**
`internal/loomcli/wiring.go:258` constructs the `*shuttleengine.Runner` and `wiring.go:416` stores it as `c.runner`.
`shuttleengine.Runner.Run(spec Spec) (Result, error)` is the blocking one-shot form — this is what `frictionengine.Reflect` needs, told through a narrow package-local `Shuttle` seam interface rather than the concrete type, the way `internal/burlerengine`, `internal/mergeresolve`, and `internal/treadleengine` each declare their own.
The Told-Geometry Invariant means `frictionengine` is handed the absolute friction directory, stencils directory, and note-archive target, and derives none of them — it must not import `internal/lyxcwd`.

**The `OutputFiles` completion contract.**
`shuttleengine.Spec.OutputFiles` names the files the agent is instructed to write, and the run is not "done" until every entry exists — the run's output file *is* its return value.
Entries must NOT already exist when the run starts; `validate` rejects a pre-existing entry, because a stale file would satisfy the contract on the first turn end.
This is why a friction note can never be an `OutputFiles` entry: the note is *optional*, and `OutputFiles` is a hard requirement.
It is also why the reflection agent's own report file is mandatory rather than optional.
The Completion Signal Invariant governs the negative answers on the shuttle side and needs no change here.

**Where `stuck` surfaces.**
`internal/shedengine/run.go`'s `outcome == Stuck` branch returns `Result{Outcome: RunBlocked, …}` in both the `OnStuck == ""` and bounce-budget-exhausted cases, without calling another producer.
`internal/loomcli/drive.go:132` is the only caller of `shed.Run`, and lines 138–142 build the success envelope — the new `friction` key goes into that same map.

**The shipped primitive.**
`internal/selfreportengine.CreateIssue(title string, body *string, labels []string) (url string, number int, err error)` files against the hardcoded `Knatte18/loomyard` with a 30s bound, via `internal/githubclient`'s authenticated client (the GitHub Auth Invariant makes `githubclient` the sole authenticator).
`lyx selfreport create <title> [-b -] [--label …]` is the CLI surface; `--body -` reads stdin, and the default label is `bug`.
Neither package changes in this task.

**Config plumbing.**
`internal/loomengine/template.yaml` carries the shipped defaults and an inline comment per key; `loomengine.Config` is the parsed struct; `loomengine.LoadConfig(baseDir, module)` validates model-spec grammar at load time.
`internal/loomcli/wiring.go` reads the config and fills `shedrecipe.Env`'s run-wide `Review*` fields — the new friction values follow the same route but land on the `frictionengine.Deps` the drive-time call site builds, not on `Env`, since no registry entry reads them.

## Constraints

From `CONSTRAINTS.md`, the ones this task must satisfy:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone; a module's own durable subdirectory is its own constant joined onto `AnchorPath()`, never a `lyxcwd` call. `LoomFrictionDir` is `loomengine`'s to declare, and no other package may construct that path.
- **Told-Geometry Invariant** — `internal/frictionengine` is handed absolute paths and derives none; it must not import `internal/lyxcwd`. If it needs to appear in the invariant's bound-packages list, it is added there in the same commit.
- **Lyxdirs Single-Declarer Invariant** — `internal/lyxdirs` is the sole declarer of `_lyx` and `.lyx`. `LoomFrictionDir` builds on `LoomScratchDir` rather than re-joining the literal.
- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx` at the mirrored subpath. Friction notes are never-tracked; they go under `.lyx/loom/`, and `loomengine` exposes the accessor beside its durable ones.
- **Stencil Ownership Invariant** — every producer prompt is read at call time from a told absolute stencils directory, never embedded bytes; `//go:embed` in `contracts/stencils` is seed defaults only; `internal/stencilstore` is the sole owner of seeding/hashing/reading/validation.
- **Producer Pointer-Rule Invariant** — an instruction file never duplicates or paraphrases another producer's format-contract content, only points at it. The four friction stencils must not restate the discussion/plan/review contracts.
- **Shuttle Provider-Seam Invariant** — provider specifics live only under `internal/shuttleengine/claudeengine`; `frictionengine` references no Claude specifics.
- **GitHub Auth Invariant** — all GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`. The reflection *agent* invoking `lyx selfreport create` is not a violation: that is an LLM running a shipped lyx verb, and the verb itself authenticates through `githubclient` in-process. No new Go code in this task touches GitHub.
- **Live-Substrate Spawn Observability** — the reflection agent's spawn is a real OS process start reachable from a `lyx` command, so it logs its spawn via `internal/logger` at `Info` and its teardown where it waits.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns: no `gitexec.Run`/`RunGit`, no `exec.Command`, no `gitkit.Copy*`, no `hubforge.NewHub` outside `integration`/`smoke`-tagged files, and no `time.Sleep` ≥ 1s.
- **Sandbox Suite Coverage** — every *registered lyx module* is exercised or explicitly excluded with a reason. Neither new package registers a CLI module, so neither adds a sandbox obligation; confirm this against the suite's own registry rather than assuming.
- **Config Strictness Invariant** — `loomengine` is on the strict `Load` side (`internal/loomengine/config.go:176`), and `configengine.load` runs `yamlengine.MissingKeys(template, fileBytes)` and hard-errors `missing keys: …; run "lyx config reconcile"` (`internal/configengine/config.go:110–123`). See the migration decision below; a genuinely absent key can never reach the parse, so "Tier 2 off" is expressed as present-but-empty.
- **Markdown Link Integrity** — every inline markdown link under `manifest/` or `docs/` must resolve, file part and `#anchor`. The doc updates must keep that true.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

New invariant introduced by this task, to be written into `CONSTRAINTS.md` in the same commit:

- **Friction Leaf Invariant** — `internal/friction` imports only stdlib, `internal/logger`, `internal/stencil`, and `internal/stencilstore`. Reverse import never allowed. Pinned by a `leaf_enforcement_test.go` modelled on `internal/pattern/leaf_enforcement_test.go`.

Build prerequisite, unchanged: `lyx` links quarry's tree-sitter grammars through cgo, so `CGO_ENABLED=1` and a C compiler on `PATH` are required.

## Testing

All new tests are Tier 1 — untagged, offline, fast. No new `integration`/`smoke` files are needed, because nothing in this task spawns git or a hub.

**`internal/friction`** — the natural TDD candidate; it is pure functions over told strings and a stencil read.

- `Directive` returns the expected stencil's text for each of the four roles, and returns `""` (nil error) when the feature is off.
- An unknown `Role` value is a hard error, not a silent empty string.
- A missing or unreadable stencil surfaces as an error naming the stencil.
- The returned directive text contains the told note path verbatim — this is the assertion that catches a composer wiring the wrong path.
- `NotePath` rejects an empty `id`, an `id` containing a path separator, and an `id` containing `..`; a valid `id` yields the expected join.
- The marker-presence helper reports absent for template bytes with no `{{.friction_directive}}` literal and present for bytes carrying it, and the `Warn` fires only on the absent-and-enabled combination — never when Tier 2 is off.
- `ReportFileName` is a single exported constant, and the note-scan exclusion and the reflection `Spec`'s `OutputFiles` entry are both derived from it (assert they agree rather than asserting two literals).
- `leaf_enforcement_test.go` asserts the import set, mirroring `internal/pattern/leaf_enforcement_test.go`.

**`internal/frictionengine`** — the second TDD candidate, driven entirely through a fake `Shuttle` seam and a `t.TempDir()` friction directory, following the fake-seam pattern `internal/burlerengine`'s and `internal/shedadapters`' own tests already use.

- Missing directory → no spawn, `Report` says skipped, nil error.
- Directory present but empty, and directory containing only non-`.md` files → same skipped result.
- One note present → exactly one spawn; the composed `Spec` carries the configured model/effort/timeout, `Interactive: false`, `ForkSubagents: false`, and a non-empty `OutputFiles`.
- The spawn's prompt names the friction directory.
- After a successful spawn the directory is archived to the timestamped sibling and the original path no longer exists; a second `Reflect` against the same location then reports skipped rather than re-spawning.
- A directory containing only `ReportFileName` and no other `*.md` → skipped, no spawn: this is the stale-report-from-a-timed-out-run case, and it must not be mistaken for one note.
- Shuttle returns an error / `OutcomeDied` / `OutcomeTimeout` → `Reflect` returns a `Report` marking failure with a nil error (never a hard error), **and the friction directory is left exactly as it was** — assert the original path still exists and that no timestamped archive sibling was created.
- A stale `ReportFileName` present before `Reflect` runs is deleted before the spec is composed, so the composed `Spec.OutputFiles` entry does not already exist (this is what `shuttleengine.Spec.validate` would otherwise reject).
- Nil `Shuttle` seam and an empty stencils dir are rejected at construction, matching the `requireSeam`/`requireAbsRoot` discipline `internal/shedrecipe` uses.
- An injected clock is used for the archive timestamp so the test asserts an exact directory name.

**`internal/loomengine`** — `LoomFrictionDir` returns `<anchor>/.lyx/loom/friction`, asserted the same way `LoomReviewsDir`'s existing test does; the new `Config` fields parse from YAML; a **present-but-empty** `friction` key yields the zero value and means Tier 2 off; a malformed model spec fails at `LoadConfig`; and a `loom.yaml` genuinely lacking the keys fails `LoadConfig` with the `missing keys: …` error rather than defaulting — assert that explicitly, since it is the migration contract and the previous draft got it backwards.

**The seven composers** — each already has prompt-composition tests. Extend each with: directive present when enabled (assert the note path appears in the composed prompt), the marker rendering as nothing when disabled, and a template lacking the `{{.friction_directive}}` marker producing a successful compose plus the `Warn` (never an error). `internal/loomengine/prompt_test.go`, `internal/loomengine/plan_test.go`, `internal/burlerengine`'s template/engine tests, and `internal/websterengine`'s render tests are the files to extend — the last covering all four of that file's composers (`RenderForkPrompt`, `RenderRecoveryPrompt`, `RenderIntegrationPrompt`, `RenderMasterPrompt`), not just the fork.

**`internal/loomcli/run.go`** — the once-per-task clear and the create, which have deliberately different conditions:

- A nil-error `Seed` clears a pre-populated friction directory, then creates it.
- A `loomshed.ErrSeedExists` re-entry leaves existing notes untouched but still ensures the directory exists. This is the crash-resume guarantee and is the one that must not regress.
- A `MkdirAll` failure leaves the run's own outcome unchanged (assert no error is returned and the seed path still completes).

**`contracts/stencils`** — `registry_test.go` covers all five new stencils' registration (the four directive stencils plus the reflection agent's own) automatically in both directions once the files and rows exist. Add content assertions in the `rubric_test.go` style (short distinctive substrings, not whole paragraphs) for the one property that matters: each directive stencil states that writing the note is optional and that an absent note is normal.

**`internal/loomcli`** — a drive-level test asserting the `friction` key appears in **`loom drive`'s** success envelope for `RunDone` and `RunBlocked`, that its value is one of `"skipped"`/`"reflected"`/`"failed"` and never `"filed"`, and that a reflection failure leaves `outcome` untouched. If `drive.go`'s existing tests cannot reach that path without a real Shed, assert the seam instead: that the `RunPaused` and non-nil-`err` branches never call the reflection seam at all. Do not add a `smoke`-tagged test for this.

**Whole-repo gates that must still pass** — `go build ./...`, `go test ./...`, the `contracts/stencils` registry test, the markdown-link-integrity test over `manifest/` and `docs/`, and the Test Tier Purity scan.

## Q&A log

- **Q:** Where do Tier 2 friction notes physically live? **A:** [auto-pick] `.lyx/loom/friction/`, via a new `loomengine.LoomFrictionDir` built on `LoomScratchDir`. **Why:** the Durable-vs-Ephemeral State Invariant puts never-tracked files under `.lyx`; `_lyx` would drag in the Fabric Git Invariant's commit-seam machinery for a file deleted minutes later, and `.lyx/loom/reviews/` already proves agents write freely into this tree.
- **Q:** Who names a note file, and how is uniqueness guaranteed? **A:** [auto-pick] `friction.NotePath(frictionDir, id)` with a caller-supplied, sanitized `id`; the caller passes the resulting absolute path into the directive. **Why:** every spawn site already holds a unique identity (row name, round number, batch id), so no clock seam is needed in five packages and two same-second forks cannot collide.
- **Q:** How do agents learn to write one? **A:** [auto-pick] mirror `internal/pattern` — a leaf package returning role-worded stencil text, injected as an optional `{{.friction_directive}}` marker via `stencil.FillOptional`. **Why:** it is the tree's existing precedent for this exact shape, already wired at four of the seven composers, and it satisfies the Stencil Ownership Invariant.
- **Q:** One package or two? **A:** [auto-pick] two — leaf `internal/friction` (stdlib + `stencil` + `stencilstore`) and `internal/frictionengine` (holds the `Shuttle` seam). **Why:** matches the `pattern`/`selfreportengine` split and keeps `shuttleengine` out of the dependency cone of the package three prompt composers import.
- **Q:** Which spawn sites get the directive? **A:** [auto-pick] five — Discussion-Write, Plan-Write, Burler round, webster fork, webster Master. **Why:** those five do substantive work. The Bouncer's judge/seed have a strictly-parsed verdict contract a side-file instruction could pollute, and `mergeresolve` runs inside Finalize, after aggregation would have read the directory.
- **Q:** Where is aggregation-and-reflection triggered? **A:** [auto-pick] one site in `internal/loomcli/drive.go` right after `shed.Run` returns, on `RunDone` and `RunBlocked` only. **Why:** `shedengine.Run` returns on `RunBlocked` without calling another producer, so a recipe row can never cover the `stuck` trigger; a post-Finalize row would cover half the triggers and need a second implementation anyway.
- **Q:** Default-on, or opt-in per producer/profile? **A:** [auto-pick] default-on with one global `loom.yaml` key that is both the model spec and the kill switch (absent/empty = off). **Why:** the feature's value is breadth; per-row opt-in means a Tier 2 key on five recipe engines whose row names are durable on-disk identities.
- **Q:** What happens when a run produces zero notes? **A:** [auto-pick] `Reflect` scans first and spawns nothing, logging at `Info`. **Why:** a clean run is the common case, and a session that concludes "nothing to report" is pure cost.
- **Q:** Who actually files the issue? **A:** [auto-pick] the reflection agent invokes `lyx selfreport create` itself, and additionally writes one mandatory output file recording what it filed. **Why:** it is what the design doc settled with the operator, there is precedent (`lyx quarry` in the plan stencil), and the mandatory file is required regardless — `OutputFiles` is the completion signal, so an empty set could never report Done. The Go-files-it alternative is recorded under Decisions as the fallback if shelling out proves unreliable.
- **Q:** Does a note have a schema? **A:** [auto-pick] no — freeform markdown, nothing in Go parses it. **Why:** a schema would create a sole-parser obligation and a CONSTRAINTS.md invariant for a file one LLM writes and another LLM reads.
- **Q:** What happens to notes after reflection? **A:** [auto-pick] archived to a timestamped sibling directory; the directory is cleared only at `loomshed.Seed` time, never on every drive invocation. **Why:** `loom` resumes across crashes, so clearing per-drive would destroy exactly the notes most worth reading; archiving stops a `blocked → fixed → done` task re-filing its first half's notes.
- **Q:** Can a reflection failure fail the run? **A:** [auto-pick] never — logged at `Warn`, reported under a new `friction` envelope key (`"skipped"`/`"reflected"`/`"failed"`), exit code untouched. **Why:** the run's outcome is about the task's work; failing an already-merged run over optional bookkeeping is strictly worse than filing nothing.
- **Q:** How is the reflection agent's session configured? **A:** [auto-pick] `Interactive: false`, `ForkSubagents: false`, model/effort/timeout from the new config keys, `Role: "friction"`. **Why:** `loom run` is the unattended path so interactive would hang; it has nothing to fan out over, so forks are authorization with no user.
- **Q:** Dedup against already-open GitHub issues? **A:** [auto-pick] out of scope. **Why:** it needs issue-search capability nothing in this path has, Tier 1 faces the identical question, and it is not in the design doc's scope.
- **Q:** A new CLI verb or flag? **A:** [auto-pick] neither — the config key is the whole control surface. **Why:** YAGNI, and a flag nobody is present to type is not a control surface on the unattended path.
- **Q:** Where does the once-per-task clear actually land, given `loomshed.Seed` is told no friction path and `lyx loom drive` never calls it? **A:** [auto-pick] beside the `Seed` call in `internal/loomcli/run.go`, on the nil-error branch only; `Seed`'s signature is untouched and `drive` deliberately never clears. **Why:** `Seed`'s told-parameter list is about status seeding, and `run.go:101` already distinguishes a first seed from an `ErrSeedExists` re-entry. A drive-only invocation is by definition a resume (`drive.go:61` requires an existing seed), which is exactly the case that must keep its notes.
- **Q:** What happens to the notes when the reflection agent fails? **A:** [auto-pick] they are left in place, not archived; only a clean return archives. **Why:** a failure means nothing was reflected on and nothing was filed, so archiving would discard un-acted-on signal while creating no duplicate-filing risk to avoid.
- **Q:** Can the agent's own report file be miscounted as a friction note? **A:** [auto-pick] no — it is named by one exported `friction.ReportFileName` constant that both the note scan's exclusion and the `Spec`'s `OutputFiles` entry read, and `Reflect` deletes a stale copy before composing the spec. **Why:** a half-written report from a timed-out run would otherwise look like new friction on the next scan, and `shuttleengine.Spec.validate` rejects an `OutputFiles` entry that already exists.
- **Q:** What happens when an already-seeded stencil never receives the `{{.friction_directive}}` marker? **A:** [auto-pick] the composer warns via `internal/logger`, naming the stencil and pointing at `lyx stencil diff`/`sync`. **Why:** `reconcile.go:124` never refreshes a `StateEdited` stencil and `:98` refuses in dev mode, while `FillOptional`'s guarantee is one-directional — so without a warning Tier 2 produces zero notes forever, indistinguishable from having found nothing.
- **Q:** Should the envelope claim `"filed"`? **A:** [auto-pick] no — the values are `"skipped"`/`"reflected"`/`"failed"`. **Why:** nothing in Go parses the agent's report, so whether an issue was created is not something this envelope can honestly assert; `"reflected"` is exactly what Go observed.
- **Q:** Do the new `loom.yaml` keys break an already-seeded worktree, given the strict `Load`? **A:** [auto-pick] yes, loudly — the keys ship in the template and `lyx config reconcile` is the migration. **Why:** `configengine`'s `load` hard-errors with a message that already names its own remedy (`config.go:110–123`); keeping the keys out of the template would make the feature permanently unswitchable-on. "Tier 2 off" is therefore present-but-empty, never a missing key.
- **Q:** Which webster composers actually get the directive, given `render.go:183` is the recovery prompt rather than the fork? **A:** [auto-pick] all four — `RenderForkPrompt`, `RenderRecoveryPrompt`, `RenderIntegrationPrompt`, `RenderMasterPrompt`. **Why:** all four are real spawns doing substantive work, and recovery in particular runs exactly when something has already gone wrong. This takes the composer count from five to seven and the `Fill` → `FillOptional` conversions from one to three; `RenderForkPrompt` also gains a told path parameter it does not have today.
- **Q:** Who creates `.lyx/loom/friction/`? **A:** [auto-pick] one `os.MkdirAll` in `internal/loomcli/run.go` beside the clear — clear on first seed only, create on both branches; a failed create is a `Warn`, never a run failure. **Why:** unowned creation means every note write depends on provider-specific parent creation, and a silent write failure is the exact zero-notes-no-signal state Tier 2 exists to prevent. `burlerengine/engine.go:112` is the in-tree precedent.
- **Q:** Does the operator ever see the `friction` envelope key? **A:** [auto-pick] not via `loom run` — it is `loom drive`'s envelope, which `run.go:230–232` redirects into the driver log. `Reflect` therefore also emits a `logger.Info`. **Why:** `loom run` detaches drive and hands over a tmux session, so the envelope alone would be invisible; the logger sink is the surface the operator actually has.
- **Q:** Does the missing-marker helper log, or return a bool? **A:** [auto-pick] it logs, taking the stencil name as a parameter. **Why:** one implementation instead of seven duplicated `Warn` lines, and it settles the leaf's import set — which is why the Friction Leaf Invariant admits `internal/logger` (already pulled transitively via `stencilstore`).
