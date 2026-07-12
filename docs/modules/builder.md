# Builder: the batch-implementation loop

> **Status: As-built.** Unlike most per-module docs, this one is **not** deleted on
> landing (per the [documentation lifecycle](../overview.md#documentation-lifecycle)):
> builder's digest/outcome/state contracts are consumed by a future `loom` the same way
> [plan-format.md](plan-format.md) is, so this doc stays as the durable reference for
> those contracts, alongside the package documentation in `internal/builderengine` and
> `internal/buildercli`.

## What it is

Builder is Loom's Builder phase, carved into its own standalone module: it drives a
pinned [plan-format v2](plan-format.md) plan through implementer sessions, batch by
batch, until the plan is built. The loop itself is held by a **long-lived LLM
orchestrator session** (model config-chosen, Sonnet default), spawned by `lyx builder
run`; Go (`internal/builderengine` + `internal/buildercli`) provides only the fat
`lyx builder` verbs the orchestrator drives, plus the distillation behind them — it
never iterates batches or makes orchestration decisions itself. This is the ADVANCE
half of a pair whose CONVERGE half is `internal/perchengine`: builder ends at
**batches-built**, never running a terminal review itself (see
[Holistic review is perch's job](#holistic-review-is-perchs-job-not-builders) below).

`builder` branches off `shuttle` directly (it spawns implementers as interactive
psmux strands over the file contract) and does not need `perch`; `loom` (not yet
built) needs both `perch` and `builder`.

## Verb surface

`lyx builder` has six subcommands, all riding the `internal/output` JSON envelope:

| Verb | Job |
|------|-----|
| `validate` | Lints the plan at `_lyx/plan` against the plan-format machine checks without running anything — the standalone pre-flight for a Planner or human. |
| `run [--fresh]` | The product verb: takes the run-level lock, clears any leftover pause flag, runs the automatic validation gate, reclaims a prior run's orphaned orchestrator (stops the recorded strand if the mux still reports it live — see [Crash/resume](#crashresume-semantics--re-drive-the-first-unreported-batch)), checks the plan fingerprint against `state.json` (`--fresh` archives stale state/reports and re-inits on a mismatch), archives any stale `outcome.yaml`, spawns a fresh orchestrator session via shuttle — recording its strand in `state.json` *before* blocking — and blocks until the run reaches a terminal outcome (`done`/`stuck`/`paused`) or the orchestrator spawn itself ends asking/died/timed-out. Performs the loop's exit-time backstop weft commit. Requires a live mux session (`lyx mux up` first). |
| `spawn-batch <NN> [--role recovery] [--restart-chain]` | Runs the same automatic validation gate, checks the pause flag, recomputes the plan fingerprint against `state.json`'s recorded one (a mid-run plan edit refuses loud, pointing at `run --fresh` — no `--fresh` escape exists here, re-initializing is `run`'s job), resolves the batch's role (oversized-driven, or `--role recovery` for the escalation path), optionally performs the `--restart-chain` reset, records the batch's start-SHA in `state.json`, and spawns one implementer via shuttle (non-blocking — returns as soon as the strand is registered). Weft-commits `state.json` on success. |
| `poll [--wait DURATION]` | Long-polls the in-flight batch for its terminal digest (see [poll's four-branch terminal classification](#polls-four-branch-terminal-classification)) and distills a terminal batch-report into the pinned [digest contract](#digest-contract). Weft-commits the batch report plus `state.json` on a terminal classification; a running snapshot touches neither git nor weft. |
| `status` | An instant, side-effect-free snapshot of `state.json` plus the reports dir — human- and loom-facing navigation. Never spawns, never weft-commits, never mutates `state.json`. A run that has never started prints `{"initialized": false}`. |
| `pause` | Writes the pause flag file `spawn-batch`'s batch-boundary check refuses against. Never interrupts a batch already in flight; resume with `run`, which clears the flag at its own entry. |

## Digest contract

`poll`'s terminal classification returns exactly this pinned, terse field set to the
orchestrator — no prose, no full file lists (the mill-go-bloat lesson made
structural). A **running** snapshot carries only `batch`, `status`, and `elapsed_s`;
every other field populates only once a batch reaches a terminal classification. This
table is the contract the orchestrator prompt template co-versions with (see
[Co-versioning rule](#co-versioning-rule-templates--parsers-move-together) below).

| Field | Type | When present | Meaning |
|-------|------|---------------|---------|
| `batch` | string | always | The batch's `NN-<batch-slug>` identifier. |
| `status` | `running \| done \| stuck \| dead` | always | The batch's classification. |
| `tests` | `green \| red \| skipped` | terminal, report-backed | The report's test verdict, verbatim. |
| `stuck_reason` | string | `status: stuck` | The report's `stuck_reason`, verbatim — must name both the blocker and what was attempted. |
| `out_of_scope` | `[{path, why}]` | terminal, report-backed, optional | The report's `out_of_scope` list, verbatim — never recomputed. |
| `drift_unreported` | `[string]` | terminal, report-backed, optional | Paths only, sorted: changed files outside every declared scope entry with no matching `out_of_scope` entry — the rot signal. |
| `files_changed` | int | terminal, report-backed | Count of files changed since the batch's start SHA — a count, never a list, so it never scales with batch size. |
| `dirty` | bool | terminal, report-backed | Whether the worktree had uncommitted/untracked changes at terminal classification — a half-done-work signal. |
| `dead_reason` | `asking \| timeout \| died` | `status: dead` | Which of the three dead branches fired (see below). |
| `elapsed_s` | int | `status: running` only | Seconds since spawn. |

## poll's four-branch terminal classification

Nobody holds the shuttle `Run` handle across a batch's lifetime — `spawn-batch` exits
right after `Start` — so `poll` re-derives the in-flight implementer's state from
files and a live mux query on every tick, in this pinned decision order:

1. **Report present** → terminal, `done` or `stuck` per the report itself (via
   `Distill`).
2. **No report, the implementer's turn has ended** (a `Stop` event observed in the
   run dir's `events.jsonl`) → terminal `dead`, `dead_reason: asking` — an
   implementer that stopped without ever satisfying the file contract is
   respawn/recover material, same as a crash.
3. **No report, elapsed since spawn > `batch_timeout_min`** → terminal `dead`,
   `dead_reason: timeout`.
4. **No report, turn still in progress, mux strand gone** → terminal `dead`,
   `dead_reason: died`.
5. **Otherwise** → non-terminal `running` snapshot; the orchestrator's next `poll`
   call re-polls from there.

On any `dead` classification the pane/run dir is kept for diagnosis (shuttle's own
died/timeout discipline). On the report-backed terminals poll itself releases the
substrate — nobody else ever holds the shuttle `Run` handle (spawn-batch exits right
after `Start`), so without this every finished batch would leak a live pane hosting
an idle agent process: `done` removes the strand and the run dir (shuttle-finalize
parity); `stuck` removes the strand but keeps the run dir (the raw session output is
the stuck trail a human may still inspect). Cleanup failures are logged, never
fatal — the classification already stands. A later **respawn of a dead-classified
batch re-claims that kept substrate**: `spawn-batch` stops the kept strand if the mux
still reports it live (a timed-out implementer may still be *working*, and left alive
it races the fresh session on the host repo and the report path) and archives — never
deletes, never refuses on — any late report the orphan managed to write after the
classification. A `done` batch's report keeps the loud pre-existing-report refusal: an
accidental respawn of finished work must never silently archive it away. `poll --wait
DURATION` blocks inside Go on this loop —
**the long-poll IS the notification**: a true push cannot reach a Claude session
(mid-turn only tool results arrive; turn-end without the outcome file is `asking`
under the file contract), so file-watching inside Go costs no orchestrator tokens and
returns the instant a batch terminates.

## Role selection + recovery ladder

`spawn-batch` resolves a batch's implementer role from the batch's own `oversized:`
frontmatter — Go always makes this call, never the orchestrator, so a typo or
forgotten flag cannot slip through:

- `oversized: false` (default) → `implementer` (Sonnet default).
- `oversized: true` → `implementer_oversized` (a large-context model variant).
- `--role recovery` → always wins regardless of the batch's own `oversized:` flag —
  the **only** override `spawn-batch` accepts; any other `--role` value is rejected
  loud before any spawn.

Recovery is an exception path, entered only after a batch reports `stuck`: the
orchestrator's own judgment call (never a Go branch, never a `/model` switch inside
the polluted session) to spawn a **fresh**, higher-capability `recovery`-role session
that reconstructs from the durable trail (batch file, code, the card-commit git log)
instead of continuing the stuck session's context. Because the stuck batch's own
report is the recovery spawn's sole shuttle output file, `spawn-batch --role recovery`
**archives that stale report** (renaming it `NN-<slug>-<UTC-compact-timestamp>.yaml`,
with the same `-1`/`-2` same-second collision suffix the other archivers use) before
spawning — archive-never-refuse, so the prior stuck report stays auditable in the weft
while its path is freed for the recovery session's own fresh report. Without this the
respawn would be refused outright (both builder's own pre-existing-report guard and
shuttle's `Spec.validate` reject a pre-existing output file), which would leave the
whole stuck→recovery ladder unreachable for a non-chain batch.

All four of builder.yaml's roles (`orchestrator`, `implementer`,
`implementer_oversized`, `recovery`) are resolved against the model-spec registry as a
pre-flight at both `run` and `spawn-batch` entry (`ResolveRoles`) — a well-formed but
unknown alias fails loud before any agent spawns, never hours into a run when that
role first spawns.

## Chain rollback

A deferred-verify chain's intermediates commit non-green (possibly non-compiling)
code, so normal one-batch recovery does not apply to them — the whole chain is the
recovery unit. `spawn-batch <NN> --restart-chain`:

1. Resolves `NN`'s chain-end batch (`ChainEndFor`) and every chain member
   (`ChainMembers`) — the recorded `verify: deferred` + `chain-end:` group — and stops
   every member's recorded strand the mux still reports live (a kept-alive member left
   running would commit on top of the rolled-back tree).
2. Requires a **recorded** chain-start SHA in `state.json`'s `ChainStartSHAs` map —
   there is no caller-supplied SHA anywhere in this path; an unrecorded chain can
   never be rolled back to a hallucinated one.
3. Resets the host repo hard to that SHA (`ResetHard`).
4. Deletes every chain member's stale batch-report file and clears their in-memory
   `BatchState` entries, then resets `state.CurrentBatch` to `0`.
5. **Re-points the spawn to the chain's lowest-numbered member**, regardless of which
   member `NN` named. The chain always restarts from its lowest member — the reset just
   rolled the tree back to before that member's first card commit — so Go spawns the
   lowest member itself rather than trusting the caller to name it. Naming the chain's
   **end** (the batch that runs the chain's real `verify:`, and thus the member most
   likely to go `stuck` and trigger a restart) therefore re-runs the chain from the
   bottom instead of spawning the end on a tree missing every earlier member's
   just-discarded work.

The chain-start SHA itself is recorded once, at whichever member spawns first — the
host `HEAD` immediately before that member's first card commit — and is never
overwritten by a later member's own spawn. The orchestrator decides **when** to
restart a chain (its pinned recovery judgment); Go performs the destructive act only
against the one SHA it recorded itself, and always re-runs from the chain's lowest
member — the orchestrator never has to identify that member, and can name any member of
the chain it wants restarted.

## Pause

`lyx builder pause` mirrors perch's flag-file discipline against the builder dir
instead of a perch run dir:

- `pause` creates a flag file (`RequestPause`); idempotent.
- `spawn-batch` checks it at the batch boundary (`PauseRequested`) and refuses with a
  `{"paused": true}` envelope — a batch already in flight always finishes normally,
  since pause never interrupts a running implementer.
- The orchestrator reads that refusal as an operational signal (`ErrPaused`), never a
  hard error: it writes its own `outcome.yaml` with `outcome: paused` and exits
  cleanly; `run` sees `paused` and exits `RunResult{Outcome: "paused"}`.
- The flag is cleared (`ClearPause`) at two points: `run`'s own entry (so a resumed
  run never instantly re-pauses on the flag that requested the very pause it is now
  resuming from) and at every non-`paused` terminal outcome (so a pause request that
  lost the race against the last batch settling on its own never lingers in a
  finished run's builder dir).

## Outcome contract + archiving

The orchestrator's final action, every run, is writing `_lyx/builder/outcome.yaml` —
the shuttle `OutputFiles` entry `run`'s spec names:

```yaml
outcome: done | stuck | paused
stuck_reason: null | "<one line>"   # required non-empty when outcome: stuck
batches_done: <int>
```

`ParseOutcome` decodes it strictly (`yaml.Decoder.KnownFields(true)`) and enforces the
vocabulary plus the stuck/`stuck_reason` cross-field rule — the burler verdict-parse
discipline: an unparseable or malformed outcome file is a hard error, never guessed.

**Stale-file handling is archive-never-refuse**, since resume is just re-running
`lyx builder run`: `run` calls `ArchiveStaleOutcome` before every spawn, renaming a
pre-existing `outcome.yaml` to `outcome-<UTC-compact-timestamp>.yaml` in place (a
same-second collision appends a numeric `-1`, `-2`, ... suffix rather than clobbering).
Refusing would block the decided crash/resume path; archiving keeps the prior run's
judgment auditable.

**Non-done shuttle outcomes** for the orchestrator's own spawn never reach the
outcome-file parse at all: `asking`/`died`/`timeout` each map to their own distinct
`*OrchestratorAskingError`/`*OrchestratorDiedError`/`*OrchestratorTimeoutError`
carrying `SessionID` + kept `RunDir` (plus `LastAssistantMessage` for `asking`). The
fail-loud parse error is reserved for a `done` outcome whose file is present but
malformed — the two failure classes are never conflated.

## Fingerprint + `--fresh`

`state.json` records a `PlanFingerprint` — a SHA-256 over every plan `*.md` file's
name and contents, sorted lexically (`Fingerprint`) — at first init. Every later `run`
entry recomputes it and compares:

- **Match** → resume as normal; `state.json` and the reports dir are trusted progress.
- **Mismatch, no `--fresh`** → hard refusal (`ErrFingerprintMismatch`) naming both
  fingerprints and pointing at `run --fresh` — stale reports from a superseded plan
  must never be misread as progress.
- **Mismatch, `--fresh`** → archives `state.json` (to `state-<timestamp>.json`) and
  the whole reports dir (to `<reports-dir>-<timestamp>`, then recreates an empty
  reports dir), mints a fresh `RunGUID`, and re-inits `state.json` with the new
  fingerprint. Never a silent wipe — the prior run's artifacts are always archived,
  never deleted.

## `run.lock`

`run` holds an exclusive, non-blocking OS file lock (`run.lock` inside the builder
dir, `lock.TryAcquireWriteLock`) for its **entire** duration — perchengine's
`ErrBlockBusy` pattern applied to builder's own run-level mutex. A resume attempted
while a prior `run` is still genuinely alive fails fast with `ErrRunBusy` instead of
two orchestrators driving the same `state.json` at once. `ErrRunBusy` is special-cased
at the CLI layer: the losing call touched **nothing** on disk, so `run` skips its own
exit-time weft-commit backstop entirely rather than committing the winner's in-flight
partial state under a misleading label.

## The three weft-commit points

`internal/builderengine` is weft-BLIND: every weft commit of a builder artifact
happens in `internal/buildercli`, mirroring `perchcli`'s block-exit
`weftengine.Commit` + `Push`. The commit pathspec excludes every machine-local
runtime artifact — `:(exclude)*.lock` (advisory OS locks) **and**
`:(exclude)*/builder/pause` (the pause flag, which is present on disk during
`poll`'s terminal commit whenever a pause raced the last in-flight batch) — so neither
leaks into durable weft history nor materializes on another machine's weft pull (a
committed pause flag could read as a spurious pause request elsewhere). "When it makes
sense" (the discussion's own
phrasing) resolved to exactly three batch-boundary points across the loop, never a
single end-of-run commit (which would lose every weft-synced batch on a crash
mid-run):

1. **`spawn-batch`** commits `state.json` immediately after a successful spawn — the
   just-recorded start-SHA and `BatchState` entry.
2. **`poll`** commits the batch report plus `state.json` once a batch reaches a
   terminal classification.
3. **`run`** performs one backstop commit at its own exit, regardless of outcome
   (success or error) — except on `ErrRunBusy` (see [`run.lock`](#runlock) above).

## Crash/resume semantics — re-drive the first unreported batch

Resume is just re-running `lyx builder run`: it always spawns a **fresh** orchestrator
(never `claude --resume`), hydrated from on-disk state. That orchestrator's `{{.progress}}`
lists only the batches whose reports already landed (each by its own `done`/`stuck`
status — see [poll](#polls-four-branch-terminal-classification) and the progress rule),
so it re-drives the **first unreported batch** from scratch: a fresh `spawn-batch` that
captures a new start-SHA and overwrites that batch's `BatchState`, not a resume of the
recorded in-flight strand. No progress is lost — every completed card is a host commit,
so a re-driven batch continues on top of its own prior card commits, and the per-batch
weft commits above keep `state.json`/reports durable across the crash.

The orphaned-live-ORCHESTRATOR edge is closed by the **entry-time reclaim**:
`run` records the orchestrator's strand GUID in `state.json` (`OrchestratorStrand`)
immediately after the spawn starts, *before* blocking on it — so a `run` process that
dies mid-wait (a killed process, a closed terminal) or an orchestrator that outlives its
own `orchestrator_timeout_min` while still working leaves a durable record of the pane
that may still be live and driving. The next `run`'s entry stops that strand if the mux
still reports it live (liveness-gated, never cleared — a cleanly-finished orchestrator's
strand was already removed by shuttle and reports not-live), so "always spawns a fresh
orchestrator" is true by construction rather than an assumption that the old one died.
Without this, a resume spawns a second live orchestrator over the first and the two
double-drive the same `state.json` — both writing the same `outcome.yaml`, with no way
to attribute the result (found live in round fable-r4).

The orphaned-live-implementer edge is closed by the **in-flight guard**
(`ErrBatchInFlight`): `spawn-batch` refuses when `state.json` records a non-terminal
in-flight batch whose strand the mux still reports live — the strictly-sequential loop
never legitimately spawns over a live implementer. The guard distinguishes the two cases
a blanket live-strand check could not: every intended respawn-on-top-of-a-kept-pane
(the `dead: timeout`/`dead: asking` ladder, recovery after `stuck`) passes through a
terminal `poll` first, which sets `BatchState.Terminal` and clears `CurrentBatch`, so
the ladder never trips it; only a genuinely orphaned live implementer (orchestrator died
mid-batch, or a stray manual `spawn-batch` during a run) is refused, with the resolution
spelled out — long-poll (`lyx builder poll`) until the in-flight batch classifies
terminal. A mux-status error skips the guard (a downed mux hosts no live strand; the
spawn's own `Start` surfaces real substrate failures), so resume on a cold machine is
unaffected.

## Holistic review is perch's job, not builder's

`builder run` terminates `done` when the last batch is green (or `stuck`/`paused`) —
**batches-built, full stop.** The terminal holistic review some earlier design drafts
described as part of Builder's own loop is not: it is the separate **Builder-review
gate**, driven by `loom` (or the operator running `lyx perch run` directly) *after*
`builder run` returns `done`. Keeping the two split lets an LLM orchestrator drive the
batch loop without ever touching perch's own block-exit weft-committing discipline —
an agent-run perch loop would reintroduce exactly the non-deterministic orchestration
the Weft Git Invariant exists to keep out of agent hands.

## Co-versioning rule: templates ↔ parsers move together

Both the orchestrator prompt (`orchestrator-template.md`) and the implementer prompt
(`implementer-template.md`) are `.md` assets `//go:embed`'d into
`internal/builderengine`, filled via `internal/stencil` at spawn time. Each prompt is
**half of a Go-parsed contract**: the orchestrator prompt names the exact verbs and
reads the exact digest fields `poll` emits; the implementer prompt names the exact
batch-report schema `poll`/`Distill` parse. **A change to the digest contract, the
outcome schema, or the batch-report schema MUST land in the same commit as the
corresponding template edit** — independent versioning is exactly the failure mode
this pins against: a prompt keying off a field Go no longer produces raises no
compile error, only a silently-broken orchestrator. Property tests pin each
template's contract statements (`template_test.go`), the same discipline
`burlerengine`'s `TestTemplate_StatesRoundDiscipline` established.

## Config — `builder.yaml`

| Key | Meaning | Default |
|-----|---------|---------|
| `orchestrator` | Model-spec for the long-lived orchestrator session. | — |
| `implementer` | Model-spec for a normal-sized batch's implementer spawn. | Sonnet |
| `implementer_oversized` | Model-spec for an `oversized: true` batch's implementer spawn. | — |
| `recovery` | Model-spec for the fresh escalated recovery spawn. | — |
| `self_fix_cap` | Max in-session self-fix attempts after a red `verify:` before reporting stuck. | 2 |
| `poll_wait_s` | Seconds `poll --wait` blocks by default when `--wait` is not given. | ~480 (8m) |
| `batch_timeout_min` | Minutes since spawn after which a report-less, strand-live batch classifies `dead`/`timeout`. | 60 |
| `orchestrator_timeout_min` | Minutes the orchestrator's own shuttle spawn is allowed to run. | 480 |
| `batch_context_cap_tokens` | Validation check 5's context-estimate cap. | 100000 |
| `batch_card_cap` | Validation check 5's card-count cap per batch. | 10 |

Every role string is validated for grammar via `modelspec.Parse` at `LoadConfig` time
and resolved against the model registry via `ResolveRoles` as a pre-flight at `run`
and `spawn-batch` entry (see [Role selection](#role-selection--recovery-ladder)
above). Scope-drift itself is deliberately **unconfigurable** — the digest always
flags `drift_unreported`, the orchestrator always judges; a `judge|block` policy knob
would move that judgment back into a Go branch, rejected as YAGNI.

## See also

- [plan-format.md](plan-format.md) — builder's pinned input contract (plan structure,
  validation checks, the batch-report schema builder's `Distill` reads).
- [docs/reference/model-spec.md](../reference/model-spec.md) — the model-spec notation
  and registry `ResolveRoles` resolves against.
- [loom.md](loom.md) — the phase machine that will drive `builder run` as one phase,
  gated by `perch` on either side.
- `internal/builderengine` and `internal/buildercli` package documentation — the
  as-built code this doc summarizes.
