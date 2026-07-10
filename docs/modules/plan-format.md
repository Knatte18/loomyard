# Plan format v1 — Builder's input contract

> **Status: Contract — pinned.** This doc pins **plan-format v1**: the artifact the
> (future) Planner phase produces and the `builder` module consumes. Neither consumer is
> built yet; the contract exists first so `builder` can be built and tested against a
> hand-written plan fixture before any Planner exists. Future format changes bump the
> version. Per the [documentation lifecycle](../overview.md#documentation-lifecycle) this
> is a durable design doc — it stays.

## Who consumes this, and how

The **Builder orchestrator is a long-lived LLM session** (model chosen via config — see
[Roles and models](#roles-and-models--none-of-them-live-here)) that holds the batch loop
and drives fat `lyx builder` verbs (`spawn-batch`, `poll`, `status`). Go
(`internal/builderengine`) provides only those verbs plus the distillation behind them —
it does not hold the loop, iterate batches, or make orchestration decisions.

Two consequences for this contract:

- The [batch-report](#batch-report--the-output-half-of-the-contract) is read by Go inside
  the `poll` verb, which returns a **distilled digest** to the orchestrator. The
  orchestrator never ingests raw session prose (the mill-go bloat lesson: context bloat
  came from an LLM orchestrator swallowing verbose sub-agent output). This doc pins **only
  the on-disk batch-report schema**; the digest shape is builder-design territory.
- Recovery on a stuck batch is the orchestrator's judgment, not a Go branch.

## Principle #0 — a batch must not be too long

The most important sizing rule, learned the hard way in mill: **a batch must fit
comfortably inside a standard implementer's context window.** A fat batch overflows the
window and makes the implementer thrash; Sonnet's 1M window eases this but is not to be
relied on.

- **Prefer many small batches over few large ones.** Granularity is cheap; a stuck fat
  batch is expensive, and redoing a failed small batch is cheap.
- A batch is a **stand-alone unit** one implementer session completes: read the batch +
  the relevant code + implement + run `verify:` + commit, all within budget.
- The plan is **extremely detailed** so a cheaper model can execute a small batch
  reliably.

Exceptions exist but are always explicit and always challenged — see
[Oversized batches and deferred-verify chains](#oversized-batches-and-deferred-verify-chains).

## Structure — ordered list, no DAG

A plan is an **ordered sequence of batches**, executed strictly in order. Batch N may
assume batches 1..N−1 are committed. There is **no DAG, no `depends_on`, no topological
sort**: mill's DAG existed only for intra-plan parallelism, which was never used.
Parallelism lives one level up — split work into separate **tasks**, each its own
worktree + `lyx run`.

```
_lyx/plan/
  00-overview.md       # ordered Batch Index + task framing
  01-<batch-slug>.md   # batch 1   (NN prefix = execution order)
  02-<batch-slug>.md   # batch 2
  ...
```

The `NN` prefix *is* the order. No separate ordering metadata.

**On-disk locations.** The plan lives at `_lyx/plan/`; batch-reports at
`_lyx/builder/reports/NN-<batch-slug>.yaml`. Both are weft overlay, like the status file:
agents write them via the junction, Go reads and commits them (Weft Git Invariant — see
`CONSTRAINTS.md`). When builder is implemented, these paths resolve through
`internal/hubgeometry` helpers like every other `_lyx` path (Hub Geometry Invariant);
no other package constructs them.

## `00-overview.md`

Frontmatter carries exactly two fields:

```yaml
format: 1          # plan-format version this plan is written against
approved: true     # Builder refuses to run an unapproved plan
```

Builder refuses a plan that is unapproved **or** whose `format` it does not recognize —
fail loud, never misread (the same discipline as the burler verdict-parse and the psmux
capability-probe). `format: 1` makes every plan self-identifying the day v2 arrives.

The body carries:

- **Batch Index** — an ordered list, not a graph: `NN — <batch-slug> — <one-line intent>`.
- **Task framing** — a short paragraph of what the whole task delivers. The implementer
  reads its own batch, not this; the framing orients the orchestrator and review.

Nothing else. Batch count is derivable from the index; everything runtime-relevant lives
per-batch or in config.

## Batch file — `NN-<batch-slug>.md`

One batch = one file = one implementer session. Contents:

- **Frontmatter** (only when needed):
  - `oversized: true` — see [Oversized batches](#oversized-batches-and-deferred-verify-chains).
  - `verify: deferred` + `chain-end: NN` — see [deferred-verify chains](#oversized-batches-and-deferred-verify-chains).
- **Title + intent** — what this batch delivers as a stand-alone unit. An oversized batch
  MUST justify its flag here.
- **Scope** — the files/areas this batch owns (see [Scope](#scope--declared-ownership-not-a-cage)).
- **Cards** — the ordered steps (see [Card](#card--the-smallest-implementable-unit--one-commit)).
- **`verify:`** — the command that proves the batch is done-right (see [verify](#verify)).

## Scope — declared ownership, not a cage

Scope is a plain **path list with prefix semantics**: files and/or directories; a
directory covers everything under it. No globs (nothing to dialect-pin, nothing for the
Planner to get subtly wrong), no prose (not mechanically checkable).

Purpose: the implementer knows where to work; batches don't step on each other; and the
`poll` verb computes the batch's actual changed files from git, compares against declared
scope, and flags drift in the digest.

**There is no blind auto-revert.** An implementer — especially when self-fixing against an
incomplete plan — may legitimately need to touch files the plan never listed. Every such
change MUST be justified in the batch-report's `out_of_scope` field; the orchestrator
judges — accept as a legitimate fix, or demand a revert. **Unreported drift** (changes
outside scope with no `out_of_scope` entry) is the rot signal.

Honest limitation: the lean digest-only orchestrator judges the **stated one-line
reason**, not the diff — a plausible-but-wrong justification passes this tier. That is
accepted defense-in-depth: the mechanical comparison catches *unreported* drift, and the
[holistic review](#red-tests-recovery-and-the-review-cadence) at the end audits the
actual changes.

## Card — the smallest implementable unit (≈ one commit)

Each card is one coherent change:

- **What** — the change to make, concretely. The plan is detailed enough for a cheap
  model to execute.
- **Where** — the file(s) touched.
- Optionally a per-card **`verify:`** — a cheap check (e.g. `go build ./...`) where the
  Planner sees value.

**"One coherent commit" is the planning rule for card sizing, not a runtime invariant.**
The implementer commits per card to the **host** repo (the agent commits its own code —
Weft Git Invariant asymmetry), with the commit subject referencing batch + card:

```
02.3: <short what>        # batch 02, card 3
```

Commit-per-card is the **resume mechanism**: a fresh session sees from `git log` exactly
which card the previous session reached, and a half-done card is resumed by discarding
uncommitted changes and restarting that card. Fix-commits after a red verify are
legitimate; **commit count is never used as a check** — the card-referencing log is the
authoritative trail.

Known gap, accepted: a card that is *committed but incomplete* (the mill #574 failure
mode) is not detected at commit time; it surfaces at the batch `verify:`. The batch gate
is the backstop, and per-card verify (where present) narrows the window.

## `verify:`

- **Per-batch `verify:` is mandatory** — it is the gate. Its value may be `deferred`
  (chain intermediates only — see below).
- **Per-card `verify:` is optional** — finer signal where it is cheap, without forcing N
  test runs per batch.
- `verify:` output must be **filtered to pass/fail + failures** — never raw build/test
  noise (the dotnet-warning lesson; language plugins own the filtering).

## Oversized batches and deferred-verify chains

Two explicit escape mechanisms for work that resists decomposition (e.g. a large atomic
refactor with no compiling intermediate state). The Planner picks per case; the format
supports both.

### `oversized: true`

The batch declares it needs a large-context implementer. The orchestrator then spawns the
`implementer_oversized` role's model-spec instead of the standard implementer role — a
model/variant that *has* a large window (for Claude today: the 1M-Sonnet variant,
realized however claudeengine realizes it). Context size is **not** a generic tunable
parameter.

Governance — the flag is never a silent default:

- Set at plan-authoring time (Planner, or the human hand-writing a plan), only when the
  batch demonstrably cannot be decomposed, **with justification in the intent section**.
- Plan review carries an explicit rubric point challenging unjustified flags.
- Plan validation (Go, when built) mechanically estimates each batch's context — sum of
  file sizes in bytes over the batch's referenced files (Scope list + card Where files),
  divided by 4 — plus a card-count cap. A batch over cap **without** `oversized: true`
  fails validation loudly, forcing the Planner to split or flag. (Precedent: millhouse's
  `_plan_validate.py` `batch-oversized` checks.)
- The estimate is a **coarse safety net, not a bound**: it ignores conversation overhead,
  card text, tool output, and files the implementer reads that are not listed in
  Scope/Where. A passing batch is not guaranteed to fit — the check catches egregious
  cases only.

**Floor:** a batch that cannot fit even the large-window variant has no third
escalation — the plan is invalid and MUST be decomposed differently (oversized and/or
chained).

### Deferred-verify chains

The Planner splits the refactor into consecutive **small** batches where the
intermediates deliberately don't verify:

- An intermediate declares `verify: deferred` + `chain-end: NN` in frontmatter, where
  `NN` is the batch that runs the real `verify:` for the whole chain. Chains are
  **explicit** — an implicit "consecutive deferred flags" chain was rejected as fragile.
- **Chain membership** = every batch whose `chain-end` names the same `NN`, plus batch
  `NN` itself. **Chain-start SHA** = the host commit immediately before the
  lowest-numbered member's first card commit.
- Renumbering batches MUST update `chain-end` in the same edit; validation fails loudly
  on a dangling `chain-end` (target missing, or itself declaring `verify: deferred`).
  Explicitness makes renumbering breakage *detectable*, not impossible.
- Intermediates report `tests: skipped`; the green invariant holds at **chain level**:
  the final batch must be green.

**The chain is the recovery unit.** Intermediates commit non-green (possibly
non-compiling) code, so normal one-batch recovery does not apply. If any chain batch goes
`stuck`, the orchestrator rolls the host repo back to the chain-start SHA and re-runs the
whole chain, possibly escalated. No recovery session ever operates inside a deliberately
broken intermediate state without a green anchor.

## Red tests, recovery, and the review cadence

- **Bounded self-fix.** After a red `verify:`, the implementer gets a small bounded
  number of in-session fix attempts (~2). Still red → batch-report `status: stuck`,
  `tests: red`. Unbounded self-fix is the observed thrashing mode (Haiku on Go);
  zero self-fix wastes the one-line-fix cases.
- **Recovery is an exception path** — entered only on `stuck`. The orchestrator spawns a
  **fresh** escalated recovery session that reads the durable artifacts (batch file,
  code, git log of card commits, batch-report) — never a `/model` switch inside the
  polluted session. So the failure trail survives, `stuck_reason` MUST name both the
  blocker and what was attempted.
- **Review cadence — holistic only (v1).** `verify:` gates correctness per batch; the
  perch/burler design review runs **once, after the last batch**, on
  green-by-construction code. Review is a quality/design gate, never a test-fixing
  mechanism. The trade-off (a bad design choice in batch 1 is caught only after later
  batches build on it) is accepted deliberately: batches are small, the plan is detailed
  and itself plan-reviewed, and per-batch review was a main cost/latency driver in mill.
  A per-batch review knob is a possible v-later config option.

## Batch-report — the output half of the contract

Written by the implementer **as its final action**, via the file contract, to
`_lyx/builder/reports/NN-<batch-slug>.yaml`:

```yaml
batch: 02-<batch-slug>
status: done | stuck
tests: green | red | skipped   # skipped = deferred-verify intermediate
stuck_reason: null | "<short>" # on stuck: one line naming blocker AND what was attempted
out_of_scope:                  # optional; present only when needed
  - path: <path>
    why: "<one line>"
```

Principle: the report carries **only decision fields plus what Go cannot cheaply compute
itself**. Deliberately absent:

- `commits` / `files_changed` — git is authoritative (`poll` computes the diff against
  the batch's start SHA; the card-referencing commit log is the trail).
- `duration` — Go owns spawn/exit times.

No prose. The raw session output stays in the RunDir; the orchestrator never ingests it.

## Roles and models — none of them live here

The plan is **model-agnostic**: it carries no provider, model, or effort fields. The only
model-adjacent thing a plan can say is `oversized: true`, which selects a *role*, not a
model. Models are configuration:

- builder.yaml holds a model-spec per builder role — `orchestrator`, `implementer`
  (Sonnet default), `implementer_oversized`, `fixer`. There is no builder `evaluator`:
  the LLM orchestrator itself judges digests.
- loom's config section overrides per role when loom drives builder.
- The notation, registry, and precedence rules are pinned in
  [docs/reference/model-spec.md](../reference/model-spec.md).

## Validation checks (spec for the future validator)

Machine checks this format is designed to support — they land with builder, not with this
doc:

1. `format` recognized; `approved: true` — else refuse to run.
2. Batch Index ↔ batch files consistent (numbering, slugs, no gaps).
3. Per-batch `verify:` present (or `deferred` with a valid `chain-end`).
4. No dangling `chain-end` (target exists and is not itself deferred).
5. Context estimate (bytes//4 over Scope + Where files) and card-count cap — over cap
   without `oversized: true` fails loudly.
6. Scope paths exist or are creatable (prefix list is well-formed).

## Worked example

A complete minimal plan for a fictional task ("add a `--json` flag to `lyx board list`"),
byte-consistent across index ↔ filenames ↔ report.

`_lyx/plan/00-overview.md`:

```markdown
---
format: 1
approved: true
---

# Plan: add --json to `lyx board list`

Add a `--json` output mode to `lyx board list`, emitting one JSON object per row via the
`internal/output` envelope, with tests and help text updated.

## Batch Index

- 01 — json-flag — add the `--json` flag and envelope emission to boardcli list
- 02 — list-tests — cover `--json` in boardcli list tests and update help-tree pins
```

`_lyx/plan/01-json-flag.md`:

```markdown
# 01 — json-flag: add the `--json` flag and envelope emission

## Intent

`lyx board list --json` emits one `output.Ok` envelope per row instead of the table.
Stand-alone: after this batch the flag works end-to-end; tests land in batch 02.

## Scope

- internal/boardcli/list.go
- internal/boardengine/rows.go

## Cards

### Card 1 — flag + row struct

**What:** Add a `--json` bool flag to the list command; define `RowJSON` with the
existing table's columns as fields.
**Where:** internal/boardcli/list.go, internal/boardengine/rows.go
**verify:** go build ./...

### Card 2 — emission path

**What:** When `--json` is set, marshal each row through `output.Ok` instead of the
table writer; keep the table path unchanged.
**Where:** internal/boardcli/list.go

## verify:

go test ./internal/boardcli/... ./internal/boardengine/...
```

`_lyx/builder/reports/01-json-flag.yaml` (written by the implementer):

```yaml
batch: 01-json-flag
status: done
tests: green
stuck_reason: null
```

The same report with a justified out-of-scope edit:

```yaml
batch: 01-json-flag
status: done
tests: green
stuck_reason: null
out_of_scope:
  - path: internal/output/envelope.go
    why: "Ok() lacked an io.Writer variant the list path needs; added OkTo()"
```
