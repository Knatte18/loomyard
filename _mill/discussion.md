# Discussion: self-report Tier 1: Go-detected structural anomalies

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
slug: self-report-tier1
status: discussing
parent: main
```

## Problem

`lyx selfreport create` ships today as a manual-only verb: a human (or an agent a human is watching) decides that something went wrong and types the issue.
That means the class of friction lyx can detect *without* anyone watching — a driver process that died mid-producer and got resumed, a producer that escalated to a human because it had no bounce target, a review segment that burned its whole bounce budget, a review finding that keeps coming back round after round — is never filed at all unless somebody happens to be present and happens to notice.

loom already writes an exact record of every one of those events.
`_lyx/loom/status.json` (`shedengine.Status` + loom's `product` payload, pinned by `contracts/specs/loom-status-spec.md`) carries an append-only `history[]` of one entry per producer call, a `state` enum, and an `error` string naming the halt reason verbatim.
The bouncer segments additionally write a per-round *finding-identity ledger* (`round-<N>-ledger.md`, with `key`/`rounds`/`status` per finding) whose path each Bouncer publishes as its own history entry's `output` field.
Go can therefore file these reports directly off its own history trail — deterministic, no LLM call, and strictly more complete than an LLM's approximate recall of its own session, since it reads an exact record instead of remembering one.

**Why now:** the design was settled with the operator on 2026-09-12 and split out of the former combined `designs/self-report.md` precisely so Tier 1 could build independently of Tier 2 and of `lyx loom step` — no code dependency in any direction, all three buildable in parallel.
Tier 1 is the cheap half: it costs no tokens, needs no session watching, and works identically under plain `lyx loom run` and under a future step-loop supervisor.

## Scope

**In:**

- A new pure, no-I/O anomaly detector in `internal/loomengine` (`anomaly.go`), told a `shedengine.Status`, an entry-time observation, and already-parsed ledger models, returning a slice of detected anomalies.
- Four detection triggers: crash-resume, escalation-to-human, bounce-budget exhaustion, and a review finding still open after N rounds.
- An exported ledger-read accessor on `internal/shedadapters`, so the ledger's sole parser stays its sole parser while another package can consume the parsed model.
- Wiring in `internal/loomcli`'s `drive` verb: read the status file at entry, run the phase machine, then detect and file.
- A filing path that renders each anomaly into a deterministic title plus a body and calls `selfreportengine.CreateIssue`.
- A machine-local filed-marker under `.lyx/loom/`, so a resumed run does not re-file an anomaly it already filed.
- A `selfreport` boolean key in `loom.yaml` (default `true`) plus its template entry, so filing can be switched off.
- Docs in the same commit: `manifest/designs/self-report-tier1.md` promoted from Planned to shipped shape, `docs/overview.md`'s selfreport bullet, `manifest/roadmap.md`'s Planned item moved to Done, and `contracts/specs/loom-status-spec.md` if the status schema is touched (it is not — see Decisions).

**Out:**

- Any change to `internal/shedengine`.
  Its Producer-Seam Invariant pins its imports to stdlib, `state`, and `lock`; it gains no anomaly concept, no injected anomaly closure, and no new `Status` field.
- Any change to the `_lyx/loom/status.json` schema.
  No new field, no `schema_version` — the spec deliberately omits versioning and this task does not reopen that.
- Tier 2 (per-agent friction notes) and `lyx loom step`.
  Independent items with no code dependency either direction.
- LLM judgment of any kind in the detection or filing path.
  Tier 1 is deterministic Go end to end.
- A new `lyx selfreport scan` verb, or any geometry wiring on `internal/selfreportcli`.
  That module is today the one seam module without `RunCLIIn` precisely because it references `lyxcwd` nowhere, and this task does not change that.
- Detection during a run.
  Filing happens once per `drive` invocation, after `shed.Run` returns.
- Closing or updating an already-filed issue.
  Filing is create-only.
- Any change to `lyx loom run`'s bootstrap, `lyx loom status`, or `lyx loom pause`.

## Decisions

### detection-site

- Decision: detection runs in `internal/loomcli`'s `drive` verb — a status read at drive's *entry* (before `shed.Run`), then a post-`Run` scan over the final status plus the ledgers, then filing.
  `drive` is the only production process that ever calls `Shed.Run` (`lyx loom run` spawns `lyx loom drive` detached; see `internal/loomcli/run.go` step 3).
- Rationale: `shedengine` cannot host detection without breaking its Producer-Seam Invariant, and a purely post-hoc scan cannot see a crash-resume at all — `Run`'s step 3b unconditionally overwrites a non-`running` state to `running` before calling the first producer, and a crashed driver leaves `state: running` behind, so the crash signal exists only at the instant the file is first read.
  Taking the entry observation in `drive`, where the file is already read for `loomengine.VerifySeedOwnership`, costs one extra decode and no new architecture.
- Rejected: an injected `OnAnomaly func(...)` closure on `Shed` (precedent exists — `Shed.CommitStatus` is exactly that shape) fired in-lock at each transition.
  It would be exact and would fire mid-run, but it puts a network call inside the run's lifetime, hands the generic engine a product-flavoured concept, and buys nothing Tier 1 needs, since nobody is watching for a mid-run issue anyway.
  Also rejected: a standalone `lyx selfreport scan` verb as the only mechanism — it reintroduces the "somebody has to be present" problem the task exists to remove, and would force geometry wiring onto the deliberately geometry-free `selfreportcli`.

### detector-package-and-purity

- Decision: the detector is `anomaly.go` in `internal/loomengine`, a pure function over told inputs with no I/O and no spawns — `DetectAnomalies(entry EntryObservation, final shedengine.Status, ledgers []LedgerObservation) []Anomaly` (exact signature is the plan's call).
  Every ledger file is read by the caller (`loomcli`) and handed in already parsed.
- Rationale: `loomengine` already imports `shedengine` and already houses exactly this shape — `coherence.go`'s `checkCoherence` is a pure, no-I/O, exhaustively table-tested Tier-1 validator over a decoded `shedengine.Status`, and says so in its own file comment.
  Keeping the detector pure is also what keeps `loomengine` free of a `shedadapters` import: the ledger models cross the boundary as data, not as a read call.
- Rejected: a new `internal/loomanomaly` package — a second home for the same validator shape with no separation gained.
  Also rejected: putting the detector in `internal/selfreportengine` — that would make the geometry-free, loom-agnostic selfreport module depend on loom's status schema and the bouncer's file contracts, inverting the dependency direction the module was built with.

### ledger-access

- Decision: `internal/shedadapters` gains an exported read accessor over the bouncer ledger file (returning an exported model carrying the round number and each entry's key, rounds, and status).
  `loomcli` calls it; the detector consumes the model and parses nothing.
- Rationale: `internal/shedadapters/bouncerfiles.go` already declares itself the owner of the three bouncer file contracts and their strict, fail-loud parsers.
  An exported accessor keeps that ownership intact — the same posture the repo already pins for other formats (Planparser Sole-Parser, Discussionparser Sole-Parser, Summaryparser Sole-Parser).
  No import cycle: `shedadapters` imports `burlerengine`, `logger`, `shedengine`, `shuttleengine`, `stencil`, `stencilstore`, `summaryparser`, `websterengine` — never `loomengine` — and `loomcli` already imports `shedadapters` today.
- Rejected: duplicating a minimal ledger parser in the consuming package — a second reader of a format with a documented owner, guaranteed to diverge.
  Also rejected: extracting the ledger format into its own sole-parser package — real churn across `shedadapters`' four bouncer files for a format with exactly one producer and, after this task, two consumers.

### trigger-list-and-thresholds

- Decision: four triggers.
  1. **crash-resume** — the entry-time read observed `state: "running"` with a non-empty `history`, meaning a previous driver process died between two persists.
  2. **escalation-to-human** — final `state: "blocked"` with `error == "stuck with no OnStuck target"`.
     Eight rows escalate this way (`Preflight`, `Loom-Preflight`, `Discussion-Write`, `Plan-Write`, `Batchifier`, `Webster`, `Publish`, `Finalize`).
  3. **bounce-budget exhausted** — final `state: "blocked"` with `error == "bounce budget exhausted"`.
  4. **recurring review finding** — a ledger entry whose `status` is `open` and whose `rounds` list has three or more entries.
- Rationale: the first three are read straight off fields `Shed` writes verbatim, with the two blocked reasons being exact string literals `internal/shedengine/run.go` sets (`reason := "stuck with no OnStuck target"` and `reason := "bounce budget exhausted"`).
  The fourth is the design doc's "N repeated review rounds on the same finding", and the bouncer ledger is the only place finding *identity* exists — `status.json`'s `history[]` records producer and outcome, never a finding.
  Threshold three, because every review segment carries `max_bounces: 5` and the Bouncer's own seed call permanently consumes one unit: firing at three reports the recurrence while the segment is still alive, rather than only after it has already degenerated into trigger 3.
- Rejected: dropping the ledger read and reporting a per-segment round *count* from `history[]` alone.
  It is cheaper but answers a different question — "this segment took many rounds" rather than "this specific finding survived many rounds" — and the design doc names the latter.
  Also rejected: a minimal two-trigger set (the two blocked reasons only), which files nothing for the silent-crash case that motivates the whole tier.

### ledger-discovery

- Decision: ledger files are discovered by walking the final status's `history[]` and collecting every non-empty `output` value belonging to a Bouncer row, then reading each distinct path through the `shedadapters` accessor.
  An empty `output` is skipped.
- Rationale: `Bouncer.settle` sets `shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}` and returns it on **both** the `APPROVED`/`Done` and the `BLOCKING`/`Stuck` branch, and `Shed.Run` persists `output.Path` into the history entry — so the trail from `status.json` to finding identity already exists on disk and needs no geometry derivation and no knowledge of each segment's `run_subdir` recipe key.
  The skip is required, not defensive: `Bouncer.seedCall` returns `Stuck` with an explicitly empty pointer, so every segment has at least one history entry with no ledger behind it.
- Rejected: deriving ledger paths from `loomengine.LoomReviewsDir` joined with each segment's `run_subdir` and globbing rounds.
  That duplicates recipe knowledge (`run_subdir` is a `contracts/recipes/loom-recipe.yaml` config value) in a second place and would silently go wrong on a recipe edit.

### one-issue-per-anomaly

- Decision: one GitHub issue per distinct anomaly, with a deterministic title of the shape `loom anomaly: <kind> — <slug>` plus, for the recurring-finding trigger, the ledger entry's key appended.
  Labels: the module's existing `bug` default only.
- Rationale: the title must be deterministic because it is the dedupe key (see below), and one anomaly per issue is what lets each be closed on its own merits.
  Sticking to the `bug` label avoids depending on a repo label that may not exist — the `loom anomaly:` title prefix already makes the class greppable and filterable.
- Rejected: one aggregated issue per `drive` invocation listing every anomaly found.
  It mixes unrelated causes into one thread that can never be cleanly closed, and its title cannot be deterministic without becoming meaningless.

### dedupe-marker

- Decision: a machine-local filed-marker JSON file at `.lyx/loom/selfreport-filed.json`, written through `internal/state`'s locked/atomic typed primitive with its own sibling lock file, holding the set of already-filed anomaly titles.
  A new `loomengine` accessor exposes the path (and the lock path), beside the existing `LoomScratchDir`.
- Rationale: without a marker, every resume of a blocked run re-files the same escalation issue, which would make the feature worse than useless.
  `.lyx` is the repo's declared home for never-tracked state at the mirrored subpath of the `_lyx` content it relates to (Durable-vs-Ephemeral State Invariant), and `loomengine` already exposes four accessors under exactly that directory (`LoomStatusLock`, `LoomRunLock`, `LoomDriverLog`, `LoomBootstrapLock`).
  Losing the marker — a fresh clone, a `fabric` re-wire — costs at most one duplicate issue, which is the cheapest failure mode available.
- Rejected: a durable marker under `_lyx/loom/`, which would need a weft commit at a point in `drive` that owns no commit boundary, and would drag the Fabric Git Invariant into a diagnostics path.
  Also rejected: network dedupe (search GitHub for an open issue with the same title before filing) — a second fallible API call per anomaly per run, non-deterministic, and it silently stops working the moment someone closes the issue without fixing the cause.

### on-by-default-with-a-knob

- Decision: on by default, disabled by setting a new `selfreport: false` key in `loom.yaml`.
  The key is added to `internal/loomengine/template.yaml` in the same change, and the default in the template is `true`.
- Rationale: the design doc's own argument is that Tier 1 costs nothing and needs no session watching, so there is no reason to gate it behind opt-in.
  A knob is still required, because a run in CI, or against a fork, must be able to stop filing issues into the hardcoded `Knatte18/loomyard` target.
  `loom.yaml` is where loom's run-wide knobs already live, and `loomengine.LoadConfig` uses `configengine.Load` (strict) — so the key must exist in the template or every existing config file fails to load.
- Rejected: default-off opt-in, which leaves the common case — an unattended run nobody is watching — filing nothing.
  Also rejected: no knob at all, which makes any fork or CI run a spammer of the upstream issue tracker.

### failure-posture

- Decision: every failure in the detect-and-file path degrades to `logger.Warn` and is never allowed to change `drive`'s outcome, its exit code, or its envelope.
  A failed `CreateIssue` leaves that anomaly's title *out* of the marker, so the next `drive` invocation retries it.
  A failed marker read is treated as an empty marker; a failed marker write is warned and nothing else.
- Rationale: this is a diagnostics side-channel on a long autonomous run.
  A GitHub outage, an unresolvable token, or a rate limit must never turn a completed loom run into a reported failure.
  The repo already has this posture written down in the adjacent code: `Bouncer.runSeedSpawn` "degrades every failure to a warning", and `selfreportengine.CreateIssue` already classifies token-unresolvable, network, and API-rejection cases distinctly for exactly this kind of caller.
  Not writing the marker on a filing failure is what makes the retry happen without any retry loop.
- Rejected: surfacing the failure on `drive`'s envelope, which would make an unrelated network problem indistinguishable from a real orchestration failure in the driver log an operator reads to diagnose a halt.

### issue-body-content

- Decision: a Go-rendered markdown body carrying: the anomaly kind and a one-line statement of what was detected; the task slug and parent branch from `product`; the final `state`, `current_producer`, and `error` verbatim; the relevant `history[]` slice rendered as producer/outcome/at rows; and, for the recurring-finding trigger, the ledger entry's key, its `rounds` list, and its `status`.
- Rationale: the issue has to be actionable by someone reading it cold, with no access to the worktree — the ledger and status files are under `.lyx`/`_lyx` in a worktree that may already be torn down.
  Everything named here is already in hand at filing time and needs no extra read.
- Rejected: title-only issues, which would require the reader to still have the worktree.
  Also rejected: embedding the whole `status.json`, which buries the signal and can be large.

## Technical context

**The status file.** `_lyx/loom/status.json` is `shedengine.Status` (`internal/shedengine/status.go`) plus loom's `product` payload (`internal/loomengine/status.go`), pinned by `contracts/specs/loom-status-spec.md`.
Fields that matter here:

- `state` — the five-value `shedengine.State` enum (`running`, `paused`, `done`, `blocked`, `failed`).
- `error` — the halt reason, set verbatim by `Shed.Run`.
- `current_producer` — which row the run is at.
- `history[]` — `[]shedengine.HistoryEntry{Producer, Outcome, Output, At}`, one entry per producer call.
  It is **budget-bearing, not only a log**: `episodeStuckCount` derives every producer's per-producer, episode-scoped bounce budget by counting its own `stuck` entries since its own most recent `done`.
  It must never be truncated or compacted.
- `product` — `{slug, parent, start_sha}`.

Reads go through `state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)`, which uses `DisallowUnknownFields` at the shell level; `product` is decoded separately afterwards.
`internal/loomengine/seed.go` and `VerifySeedOwnership` are the existing in-repo examples of that two-step read, and `drive` already calls `VerifySeedOwnership`.

**The two blocked reasons are exact literals.** In `internal/shedengine/run.go`'s `outcome == Stuck` arm:

- `def.OnStuck == ""` → `reason := "stuck with no OnStuck target"`, persisted as `StateBlocked`.
- `episodeStuckCount(st.History, def.Name) >= effectiveMaxBounces(def, s.MaxBounces)` → `reason := "bounce budget exhausted"`, persisted as `StateBlocked`.

Both strings also land in `Result.Reason`, so the detector can equally be told `Result.Reason` rather than re-reading `error` — the plan should pick one and pin it, not read both.

**Why crash-resume is not post-hoc detectable.** `Run`'s step 3b: when step 1's read finds `st.State != StateRunning`, it persists `StateRunning` with an empty error *before* calling the producer, explicitly to stop the status file and the status strand describing a paused/blocked/failed run that is in fact already spawning.
A crashed driver, by contrast, leaves `state: running` behind, because it died between two persists.
So `state == "running"` at the *first* read of a fresh `drive` process, with non-empty `history`, is the crash signal — and it is gone the moment `Run` proceeds.

**Concurrency.** Two concurrent drivers cannot both proceed: `Shed.Run` holds `LockPath` (`loomengine.LoomRunLock`) non-blocking for the whole run and returns `shedengine.ErrShedBusy` otherwise, and `drive` treats that as an ordinary error envelope.
`lyx loom run`'s `mustSpawnDriver` uses that same held lock as its liveness probe.
Consequence for this task: the entry-time crash observation must only be *acted on* once `Run` has actually returned without `ErrShedBusy` — otherwise a second `drive` racing a healthy live driver would read `state: running` and file a false crash-resume.
Detect at entry, file after `Run` returns cleanly.

**The bouncer ledger.** `internal/shedadapters/bouncerfiles.go` owns three file contracts: verdict, finding-identity ledger, focus.
The ledger's parsed model is `ledgerFile{Round int, Entries []ledgerEntry, Prose string}` with `ledgerEntry{Key string, Rounds []int, Status string}`; `parseLedger` is fail-loud and pins `status` to exactly `open` or `resolved`, `round` to a positive int, and every `rounds` element to a positive int.
Paths come from `ledgerPath(runDir, round)` in `internal/shedadapters/round.go`.
Carry-forward of entries across rounds is stated in the judge prompt and deliberately **not** enforced by `parseLedger` — so a `rounds` list of length ≥ 3 is a claim the judge made, not something the parser guarantees, and the detector must treat a short or missing list as "no anomaly" rather than as corruption.

**Where ledgers live.** `runDir` resolves under `loomengine.LoomReviewsDir` = `.lyx/loom/reviews/`, joined with each segment's `run_subdir` (`discussion`, `plan`, `webster`).
They are ephemeral by construction — `internal/loomengine/config.go`'s `LoomReviewsDir` doc says so explicitly, and the recipe's `Webster-Bouncer` comment repeats it.
The detector must therefore tolerate a ledger path in `history[].output` that no longer exists on disk.

**The three review segments.** `contracts/recipes/loom-recipe.yaml` has seventeen rows.
`Discussion-Bouncer`/`Discussion-Burler` (segment `Discussion-Review`), `Plan-Bouncer`/`Plan-Burler` (`Plan-Review`), `Webster-Bouncer`/`Webster-Burler` (`Webster-Review`) — each pair with `max_bounces: 5` and a mutual `on_stuck`.
The Bouncer is the row that publishes the ledger pointer; the Burler row never returns `Done` at all.

**The filing primitive.** `selfreportengine.CreateIssue(title string, body *string, labels []string) (url string, number int, err error)`.
`targetRepo` is the hardcoded `"Knatte18/loomyard"` constant; `NewGitHubClient` is an exported, swappable package variable (`var NewGitHubClient = githubclient.New`) — that is the seam the tests use, and it is what makes the filing path testable with no network.
The whole call is bounded by a 30s `createIssueTimeout`, and errors are already classified three ways (token-unresolvable via `githubclient.ErrTokenUnresolvable`, API rejection via `*github.ErrorResponse`, everything else as unreachable).
`internal/selfreportcli/cli.go` shows the existing caller shape, including the `nil` body pointer convention: `nil` means "omit the body field entirely", which this task will not use — every anomaly issue gets a body.

**The drive verb.** `internal/loomcli/drive.go` (147 lines) is the whole insertion site.
Current order: `ShouldAbort` → stat the status path → `VerifySeedOwnership` → `reed.Up()` → `fabricengine.Open` + branch/origin/parent resolution → build `c.env.Landing` → `loomrecipe.New` → `shed.Run(ctx)` → `output.Ok` envelope with `outcome`, `halted_producer`, `reason`, `history_length`.
The entry observation goes next to `VerifySeedOwnership` (the file is already being read there); the detect-and-file step goes after `shed.Run` returns and before the envelope, and must not alter the envelope's keys.

**Config.** `internal/loomengine/config.go`'s `Config` mirrors `loom.yaml`'s seven keys today; `LoadConfig` uses `configengine.Load` (**strict**, per the Config Strictness Invariant — loom is in the strict set) with `ConfigTemplate()`, and validates the three model-specs plus rejects negative timeouts.
The new `selfreport` key must be added to both `Config` and `internal/loomengine/template.yaml`, with a comment in the template matching the existing style.
Note the boolean-default trap: a `bool` field's zero value is `false`, so "default true" must come from the template's own `selfreport: true` line (which strict load always supplies), exactly as `discussion_interactive: false` does today.

**Two guard tests every new `.lyx` accessor must be registered in.** `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go` both walk loom's path constructors by name (`loomengine.LoomStatusLock`, `LoomDriverLog`, `LoomBootstrapLock`, …).
`LoomDriverLog`'s own doc comment states why it exists as an accessor at all: "cmd/lyx's transient guard walks constructors, not call sites."
The new filed-marker accessor (and its lock accessor) must be added to both tests in the same change or the guard will not cover them.

## Constraints

From `CONSTRAINTS.md`, the ones this task is bounded by:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`.
  This is the invariant that forces detection out of the engine.
  Not to be relaxed.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own; no direct `internal/lyxcwd` import.
  `loomengine` is not in the bound-package list (it owns path accessors taking a `*lyxcwd.Location`), but the detector itself must stay told-input-only: no path derivation, no reads.
- **Cwd Resolution Invariant** — a module's own durable subdirectory is its own constant joined onto `AnchorPath()`.
  The new marker accessor follows `LoomDriverLog`'s exact shape: `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "<file>")`.
- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx`, at the mirrored subpath of its `_lyx` content, and no engine derives its own `.lyx` path (each module exposes a scratch accessor beside its durable one).
  The filed-marker is never-tracked, so `.lyx/loom/`, via a `loomengine` accessor.
- **Lyxdirs Single-Declarer Invariant** — no production file outside `internal/lyxdirs` names the `_lyx`/`.lyx` literals in path-construction context.
  Use `lyxdirs.DotLyxDirName`.
- **CLI / Cobra Invariant** — `<module>cli` imports `<module>engine`; engine never imports cli/cobra; non-empty `Short` on every command; errors are JSON via `internal/output`; every `RunE` checks `clihelp.ShouldAbort` first.
  This task adds no command and no flag, so no help-tree change — but note it also adds a cross-module edge, `loomcli` → `selfreportengine`.
  The invariant's stated deviations are `stencilcli` → `stencilstore` and `quarrycli` → `planglyph`; whether this new edge needs recording there is a judgement call for the plan, and it must be settled explicitly rather than left implicit.
- **Config Strictness Invariant** — loom is in the strict (`configengine.Load`) set.
  A new `loom.yaml` key without a matching template entry breaks every existing config.
- **Live-Substrate Spawn Observability** — not triggered: this task starts no OS process.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns; no `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, `hubforge.NewHub` outside `integration`/`smoke`-tagged files.
  Every test this task adds is Tier 1 and must stay so.
- **GitHub Auth Invariant** — all GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`.
  Satisfied by going through `selfreportengine.CreateIssue`, which is already the only door.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

From `CLAUDE.md`:

- Docs land in the same commit: the module doc in `manifest/designs/` (here `self-report-tier1.md`), `docs/overview.md` if the module table or execution stack changes, `CONSTRAINTS.md` for any new cross-cutting invariant.
- `manifest/roadmap.md` moves on completing a planned item — which this is, so the Planned entry moves.
- **Markdown Link Integrity** (`CONSTRAINTS.md`): every inline markdown link under `manifest/` or `docs/` must resolve, file part and `#anchor`.
  Editing `self-report-tier1.md` and `roadmap.md` puts this task inside that gate.
- Markdown: semantic line breaks, one sentence per line, no fixed-column hard-wrap.
- Build prerequisite: `CGO_ENABLED=1` and a C compiler on `PATH` (Quarry CGO Requirement Invariant).

Discovered during discussion, and load-bearing:

- **The entry observation must not be acted on before `Run` returns cleanly.**
  A second `drive` racing a healthy driver reads `state: running` and would file a false crash-resume; gating on `Run` not returning `shedengine.ErrShedBusy` is what closes it.
- **`history[]` must never be read destructively or compacted.**
  It stores every producer's bounce budget.
- **A ledger path from `history[].output` may not exist on disk**, and the seed call's history entry has an empty `output`.
  Both are normal, neither is corruption.

## Testing

Tier 1 only.
No new `integration`- or `smoke`-tagged test, and nothing that spawns.

**`internal/loomengine` — the detector (TDD candidate, write the table first).**
`coherence_test.go` is the model to follow: a table over hand-built `shedengine.Status` values asserting the exact set of detected anomalies.
Cases that must be covered:

- Clean `state: done` run, non-empty history, no ledger anomalies → empty result.
- Entry observation `running` + non-empty history → crash-resume.
- Entry observation `running` + **empty** history → no crash-resume (that is a fresh seed at `Preflight`, exactly the shape `contracts/specs/loom-status-spec.md`'s worked seed example carries).
- Entry observation `paused`/`blocked`/`failed` → no crash-resume (an ordinary human resume, which is what step 3b's conditional write exists for).
- Final `blocked` with `error == "stuck with no OnStuck target"` → escalation.
- Final `blocked` with `error == "bounce budget exhausted"` → budget exhaustion.
- Final `blocked` with some other `error` → neither of the two blocked triggers (no over-matching on `state` alone).
- Final `failed` → not classified as either blocked trigger.
- Ledger entry `status: open` with `rounds` length 3 → recurring finding; length 2 → not; length 5 → one anomaly, not three.
- Ledger entry `status: resolved` with a long `rounds` list → no anomaly.
- Multiple independent anomalies in one status → all reported, deterministically ordered (pin the order; an unordered result cannot be table-asserted).
- Anomaly titles are deterministic: the same input twice yields byte-identical titles.

**`internal/loomengine` — the marker accessor.**
Assert the exact path shape (`.lyx/loom/<file>`) against a fixture `*lyxcwd.Location`, mirroring how `LoomDriverLog` is asserted today, and register the new accessor in `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go`.

**`internal/shedadapters` — the exported ledger accessor.**
The existing unexported `parseLedger` tests already cover the parse grammar; the new tests cover the exported surface only: a well-formed ledger file round-trips into the exported model, a malformed one reports failure rather than a half-filled model, and an absent file reports failure without panicking.
Use a `t.TempDir()` fixture file — file I/O is not an expensive spawn and stays Tier 1.

**The filing path (TDD candidate).**
Swap `selfreportengine.NewGitHubClient` — the package variable that exists for exactly this — and assert: one `Issues.Create` call per detected anomaly; the rendered title matches the deterministic shape; the body contains the slug, the halt reason, and (for a recurring finding) the ledger key and rounds; the label list is the `bug` default.
Then the posture cases: a `CreateIssue` error leaves that title out of the marker and does not propagate; an anomaly whose title is already in the marker is not filed again; a marker read failure is treated as an empty marker; a marker write failure does not propagate.

**Marker round-trip.**
Write, read back, confirm the set survives; confirm a second detect-and-file over the identical status files nothing.
This is the regression test for the "every resume re-files the same issue" failure the marker exists to prevent — it deserves its own named test, not a clause inside another.

**Config.**
`selfreport` absent from a `loom.yaml` → strict load supplies the template's `true`.
`selfreport: false` → detection and filing both skipped, and the skip is asserted by the swapped client seam recording zero calls, not by inspecting a log.
Follow `config_test.go`'s existing shape.

**`internal/loomcli` — the drive wiring.**
A white-box test asserting the entry observation is taken before `shed.Run` and that the detect-and-file step runs after it, with the envelope's four keys (`outcome`, `halted_producer`, `reason`, `history_length`) unchanged.
`internal/loomcli/wiring_test.go` and `wiring_commitstatus_test.go` are the precedent for testing a `drive`/`run` seam without a real run.
One case must assert that a `shedengine.ErrShedBusy` return files nothing at all.

## Q&A log

- **Q:** Where does detection run — post-hoc in `drive`, an injected `shedengine` closure, or a standalone scan verb? **A:** [auto-pick] Post-hoc scan in `drive`, plus an entry-time status read for crash-resume. **Why:** `shedengine`'s Producer-Seam Invariant pins its imports to stdlib/`state`/`lock`, and `drive` is the only production process that ever calls `Shed.Run`; crash-resume is the one signal a purely post-hoc read cannot see, because `Run`'s step 3b overwrites a non-`running` state before the first producer call.
- **Q:** Which package owns the detector? **A:** [auto-pick] `anomaly.go` in `internal/loomengine`, pure over told inputs. **Why:** `loomengine` already imports `shedengine` and already houses exactly this shape in `coherence.go` — a pure, no-I/O, exhaustively table-tested Tier-1 validator over a decoded `shedengine.Status`; keeping it pure also keeps `loomengine` free of a `shedadapters` import.
- **Q:** How does the detector see finding identity? **A:** [auto-pick] `shedadapters` gains an exported ledger accessor; `loomcli` reads and hands the parsed models in. **Why:** `bouncerfiles.go` already declares itself owner of the bouncer file contracts and their strict parsers, matching the repo's other sole-parser invariants; `loomcli` already imports `shedadapters`, and `shedadapters` never imports `loomengine`, so there is no cycle.
- **Q:** Which triggers, at which thresholds? **A:** [auto-pick] Four — crash-resume, escalation-to-human, bounce-budget exhausted, and a ledger entry still `open` after ≥3 rounds. **Why:** the first three read exact literals `Shed.Run` writes verbatim; the fourth is the design doc's "same finding" clause, which only the ledger can answer. Threshold 3 against `max_bounces: 5` (one unit of which the Bouncer's seed call permanently consumes) fires while the segment is still alive rather than only after it has degenerated into the exhaustion trigger.
- **Q:** How are ledger files found? **A:** [auto-pick] By walking `history[].output` for non-empty Bouncer pointers. **Why:** `Bouncer.settle` publishes the ledger path on both the `Done` and `Stuck` branches and `Shed.Run` persists it, so the trail already exists on disk — deriving paths from `LoomReviewsDir` + `run_subdir` instead would duplicate recipe config knowledge. The empty-pointer skip is required, not defensive: `seedCall` returns `Stuck` with no pointer.
- **Q:** One issue per anomaly, or one aggregate per run? **A:** [auto-pick] One per anomaly, deterministic title, `bug` label only. **Why:** the title is the dedupe key so it must be deterministic, one anomaly per issue is independently closable, and sticking to the existing default label avoids depending on a repo label that may not exist.
- **Q:** How is re-filing on every resume prevented? **A:** [auto-pick] A machine-local `.lyx/loom/selfreport-filed.json` marker via `internal/state`, keyed by title. **Why:** `.lyx` is the declared home for never-tracked state at the mirrored subpath, and `loomengine` already exposes four accessors in that exact directory. A durable marker would need a weft commit at a boundary `drive` does not own; network dedupe adds a fallible call per anomaly per run and breaks the moment someone closes an issue without fixing the cause. Marker loss costs at most one duplicate issue.
- **Q:** On by default, or opt-in? **A:** [auto-pick] On by default, with `selfreport: false` in `loom.yaml` to disable. **Why:** the tier's whole argument is that it costs nothing and needs no watcher, so opt-in would leave the unattended case filing nothing — but a fork or CI run must be able to stop filing into the hardcoded upstream target. Strict config means the key must ship in `template.yaml`, whose `selfreport: true` line is what makes the default true despite `bool`'s `false` zero value.
- **Q:** What happens when GitHub is unreachable? **A:** [auto-pick] `logger.Warn` and continue; the run's outcome, exit code, and envelope are untouched, and the title stays out of the marker so the next run retries. **Why:** a diagnostics side-channel must never turn a completed loom run into a reported failure; `Bouncer.runSeedSpawn` sets the same precedent, and omitting the marker write gives a retry with no retry loop.
- **Q:** What goes in the issue body? **A:** [auto-pick] Anomaly kind, slug and parent, final `state`/`current_producer`/`error` verbatim, the relevant history slice, and the ledger key/rounds/status for a recurring finding. **Why:** the reader will not have the worktree — the status file is under `_lyx` and the ledgers under `.lyx`, in a worktree that may already be torn down — and every field named is already in hand at filing time.
- **Q:** Does the new `loomcli` → `selfreportengine` edge need recording as a CLI/Cobra Invariant deviation? **A:** [auto-pick] Flagged for the plan to settle explicitly rather than silently introduced. **Why:** the invariant names two existing deviations (`stencilcli` → `stencilstore`, `quarrycli` → `planglyph`), so a third cross-module edge is exactly the kind of thing that should be either recorded or consciously judged out of scope — not left implicit. This is the one open item handed forward.
