# Plan format v2 — Builder's input contract

> **Status: Contract — pinned.** This doc pins **plan-format v2**: the artifact the
> (future) Planner phase produces and the `builder` module consumes. **v2 supersedes v1
> outright** (a version bump, not a dialect): `builder` refuses a `format: 1` plan via the
> `format-unrecognized` check exactly as it refuses any other unrecognized value — there is
> no dual-version support and no production v1 plans exist to migrate. Per the
> [documentation lifecycle](../overview.md#documentation-lifecycle) this is a durable
> design doc — it stays.

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
format: 2          # plan-format version this plan is written against
approved: true     # Builder refuses to run an unapproved plan
```

Builder refuses a plan that is unapproved **or** whose `format` it does not recognize —
fail loud, never misread (the same discipline as the burler verdict-parse and the psmux
capability-probe). `format: 2` makes every plan self-identifying the day v3 arrives.

The body carries:

- **Batch Index** — an ordered list, not a graph:
  `NN — <batch-slug> (C cards) — <one-line intent>`. The `(C cards)` segment is
  **mandatory** — the Planner's own count of that batch's cards, singular `(1 card)`
  accepted — and is mechanically cross-checked against the batch file's actual
  `### Card` heading count by the `card-count-mismatch` validation check: the index and
  the batch file are two independently-written places that must agree.
- **Task framing** — a short paragraph of what the whole task delivers. The implementer
  reads its own batch file **and** this overview (framing, Batch Index, Shared
  Decisions below) — never another batch's file.
- **`## Shared Decisions`** (optional) — cross-cutting decisions every batch inherits,
  so an implementer three batches in never has to re-derive a decision batch 1 already
  made. One `### Decision: <short-name>` subsection per decision:

  ```markdown
  ### Decision: <short-name>

  - **Decision:** <what was decided>
  - **Rationale:** <why>
  - **Applies to:** <which batches, or "all batches">
  ```

  This section is **prose for humans and LLM sessions only** — Go does not parse it and
  no validation check reads it. Zero-check is deliberate: a Planner needing to say
  "every batch that touches X must also do Y" has one place to say it once, without a
  machine-checkable "Applies to" that no consumer needs.

  **Not adopted: mill's "All Files Touched" overview section.** Every file any card
  touches is already fully derivable from the batch files' typed fields — a maintained,
  separately-checksummed union of that same data would be pure derivative bloat (worst
  exactly when a plan is biggest, on a large refactor) for a check that catches nothing
  a Planner writing correct cards wouldn't already get right. See the discussion's
  `no-all-files-touched` decision for the full investigation.

Nothing else. Batch count is derivable from the index; everything runtime-relevant lives
per-batch or in config.

## Batch file — `NN-<batch-slug>.md`

One batch = one file = one implementer session. Contents:

- **Frontmatter** (only when needed):
  - `oversized: true` — see [Oversized batches](#oversized-batches-and-deferred-verify-chains).
  - `verify: deferred` + `chain-end: NN` — see [deferred-verify chains](#oversized-batches-and-deferred-verify-chains).
  - `root: <worktree-relative-dir>` — an optional shared path prefix every card
    file-op path in this batch resolves against; see
    [Card path resolution: `root:` and `//`](#card-path-resolution-root-and-).
- **Title + intent** — what this batch delivers as a stand-alone unit. An oversized batch
  MUST justify its flag here.
- **Scope** — the files/areas this batch owns (see [Scope](#scope--declared-ownership-not-a-cage)).
- **Cards** — the ordered steps (see [Card](#card--the-smallest-implementable-unit--one-commit)).
- **`## Rename mechanic`** — required when any card declares a `Moves:` pair; see
  [Moves and the Rename mechanic](#moves-and-the-rename-mechanic).
- **`verify:`** — the command that proves the batch is done-right (see [verify](#verify)).

## Scope — declared ownership, not a cage

Scope is a plain **path list with prefix semantics**: files and/or directories; a
directory covers everything under it. No globs (nothing to dialect-pin, nothing for the
Planner to get subtly wrong), no prose (not mechanically checkable). Scope entries are
always **worktree-relative** — the batch's `root:` shorthand (see below) resolves card
file-op paths only, never Scope, so a batch's declared ownership reads the same
regardless of whether its cards happen to use `root:`.

Purpose: the implementer knows where to work; batches don't step on each other; and the
`poll` verb computes the batch's actual changed files from git, compares against declared
scope, and flags drift in the digest. A card's own typed file-op paths (`Edits:`,
`Creates:`, `Deletes:`, and both `Moves:` endpoints — `Context:` is exempt, reading
outside scope being legitimate) must additionally fall under one of the batch's Scope
prefixes, mechanically enforced by the `card-outside-scope` check.

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

Each card is one coherent change, and its markdown heading **is its global identifier**:

```
### Card NN.C — <short title>
```

`NN` is the batch's own zero-padded two-digit number (the same `NN` as the batch
filename's own prefix), and `C` restarts at 1 within each batch (e.g. `### Card 02.3 —
emission path`). ASCII `-`/`--` is accepted wherever the em dash `—` is, the same
tolerance the Batch Index separator gets. This is a deliberate divergence from mill,
which numbers cards globally sequentially across the whole plan: `NN.C` carries the same
global uniqueness plus the batch context for free, and it matches lyx's existing
commit-subject convention (`02.3: <short what>`) 1:1 — a card is citable unambiguously
("5.3") in review findings and discussion, and the heading matches the commit log
exactly. The `card-numbering` check enforces both halves mechanically: the heading's
`NN` must equal the batch's own number, and `C` must run 1..M sequentially with no gaps
or duplicates.

A card's fields, **in this order**:

1. **`What:`** — the change to make, concretely (prose, may span multiple lines until
   the next field label). The plan is detailed enough for a cheap model to execute; this
   plays the role mill calls `Requirements:` — lyx keeps its own established `What:`
   name.
2. **`Context:`** — files the card's implementer is expected to *read but not change*.
3. **`Edits:`** — existing files this card changes.
4. **`Creates:`** — new files this card creates.
5. **`Deletes:`** — files this card removes.
6. **`Moves:`** — rename pairs this card performs (see
   [Moves and the Rename mechanic](#moves-and-the-rename-mechanic)).

Then, optionally:

7. **`Commit:`** — pins the exact commit subject (see below).
8. **`verify:`** — a per-card cheap check, same semantics as v1 (unchanged).

**All five typed file-op fields (`Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`) are
required on every card** — never omitted. A field with nothing to declare carries the
literal `none` on its own label line:

```markdown
**Context:** none
```

A non-`none` field's value is one or more indented sub-bullets below the label line,
each a single backtick-wrapped path, no commentary, no line-range suffix, no
comma-separated inline list:

```markdown
**Edits:**
- `internal/boardcli/list.go`
```

A `Moves:` sub-bullet instead carries a two-path pair, ASCII ` -> ` arrow, both sides
backtick-wrapped:

```markdown
**Moves:**
- `internal/boardengine/rows.go` -> `internal/boardengine/rowsjson.go`
```

The `card-missing-field` check flags a card missing any of the five (or `What:`) —
`none` sentinels are silent-degradation-proof exactly because an omitted field is
mechanically indistinguishable from a forgotten one otherwise; a forgotten `Moves:` in
particular would silently degrade into an unstructured create+delete pair, the exact
failure this format exists to prevent.

**`Context:` is advisory, not an allowlist.** Unlike mill's strict "read ONLY these
files" posture, `Context:` here is "files the Planner expects the implementer to read" —
the implementer may read beyond it when the plan under-specifies something, consistent
with Scope's own "declared ownership, not a cage" philosophy above. A read restriction
is not mechanically enforceable anyway, and mill's stricter posture served a per-batch
review-bulking step lyx deliberately does not have (see
[Red tests, recovery, and the review cadence](#red-tests-recovery-and-the-review-cadence)).
`Context:` bytes still count toward the batch-oversized context estimate, and files in
`Edits:` are implicitly read — they are never repeated in `Context:` for the same card
(that repetition is itself a `card-field-overlap` finding, below).

**Fields are mutually exclusive within one card.** The same path appearing in two of a
single card's five fields (or as a `Moves:` endpoint alongside another field) is a
contradiction — is the file being edited, or moved, or deleted? — flagged by the
`card-field-overlap` check. This is strictly **per-card**: across two cards of the same
batch, `Creates:` in an earlier card followed by `Edits:` of the same path in a later
card is legitimate sequencing (a file the plan creates in one step and refines in the
next), and the same path repeated across multiple cards' `Edits:` is entirely normal.
Only a `Moves:` endpoint gets a batch-wide exclusivity rule (`move-redundant`, see
below) — everything else about cross-card overlap is left to the Planner's judgment.

**`Commit:` pins the exact commit subject**, backtick-wrapped:

```markdown
**Commit:** `02.3: add the --json flag`
```

When absent, the implementer derives the subject from the `NN.C: <short what>`
convention itself, unchanged from v1 practice. A *present* `Commit:` value must start
with the card's own `NN.C: ` prefix — the `commit-subject-mismatch` check enforces this,
because a pinned message that breaks the `NN.C` shape would corrupt the git-log resume
trail the whole numbering scheme exists to give.

**"One coherent commit" is the planning rule for card sizing, not a runtime
invariant.** The implementer commits per card to the **host** repo (the agent commits
its own code — Weft Git Invariant asymmetry), with the commit subject referencing batch
+ card exactly as the card's own heading numbers it:

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

### Card path resolution: `root:` and `//`

A batch's frontmatter may carry an optional `root: <worktree-relative-dir>`. When set,
every card file-op path in that batch — all five typed fields, both sides of every
`Moves:` pair — resolves as `<root>/<path>` **unless** the path starts with `//`, which
is *always* worktree-root-relative (root set or not — one rule, no special cases): that
is how a card names a file outside the shared root, e.g. `//cmd/lyx/main.go`. This is
purely a token-economy shorthand for a batch whose cards repeat the same directory
prefix over and over; it changes nothing about what gets validated or how a path
compares against Scope — Scope entries themselves are never root-resolved (see
[Scope](#scope--declared-ownership-not-a-cage) above). The degenerate `root: "."` case
(the worktree root itself) resolves a card path to the raw path unchanged, rather than
the unclean `"./<raw>"` a literal string join would produce.

The parser normalizes every card path to a plain worktree-relative, forward-slash path
exactly once, at parse time — the validator, the context estimate, and any future
consumer never see `root:` or `//` again, only normalized paths. A single-`/` prefix or
a `..` segment in a card path is malformed and is flagged by the `scope-malformed`
check (the same check that already polices Scope entries' well-formedness — card-path
well-formedness reuses it rather than minting a parallel check name).

## Moves and the Rename mechanic

A `Moves:` sub-bullet declares a rename: `` `old/path` -> `new/path` `` (backtick-wrapped
paths on both sides, ASCII ` -> ` arrow, exactly the same grammar as any other field's
path bullets, extended to a pair). A path appearing as a `Moves:` endpoint must not also
appear in the same batch's `Creates:`/`Deletes:` anywhere — that would be two
contradictory instructions for the same file, flagged by `move-redundant`.

**Rename-plus-extraction is one `Moves:` pair plus a separate `Creates:` entry**: when a
rename also splits new content out of the relocated file, the relocation itself is
still exactly one `Moves:` pair (the file that moved), and the newly-split-out file is
a plain `Creates:` entry in that same card or another — never folded into the `Moves:`
pair itself.

**Every batch with at least one non-empty `Moves:` field MUST carry its own
`## Rename mechanic` section** in the batch file body — the `move-mechanic-missing`
check flags a batch that declares a rename but omits it. The section's text is
CANONICAL — reproduce it verbatim (adjusted only for the specific paths involved),
because it encodes the repo's own rename convention as a mechanical instruction, not
free-form prose:

```markdown
## Rename mechanic

1. Run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. Use `Creates:` only for genuinely new files, never for the relocated file itself.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
```

This is the repo's own `git mv` + surgical-edits convention (see this repo's root
`CLAUDE.md`) made declarable in a plan and mechanically checkable, rather than an
unstated expectation an implementer might miss.

## `verify:`

- **Per-batch `verify:` is mandatory** — it is the gate. Its value may be `deferred`
  (chain intermediates only — see below).
- **Per-card `verify:` is optional** — finer signal where it is cheap, without forcing N
  test runs per batch.
- Note the two spellings: the frontmatter `verify: deferred` is a **sentinel** (this
  batch defers to its chain's end); the `## verify:` section in the body carries the
  actual **command**. A batch has one or the other, never both.
- `verify:` output must be **filtered to pass/fail + failures** — never raw build/test
  noise (the dotnet-warning lesson; language plugins own the filtering).

**Design constraint: per-batch `verify:` stays narrowly package-scoped, never a
full-suite run.** A batch's `verify:` command MUST scope to the packages that batch
actually touches (e.g. `go test ./internal/builderengine/...`), never
`go test ./...` — the exact per-batch full-suite slowdown lyx's small-batch discipline
exists to avoid; a fat batch-boundary test run defeats Principle #0 just as surely as a
fat batch does. This format deliberately does **not** port mill's optional module-wide
overview-level `verify:` key: if a module-wide gate is ever wanted, it must be
baseline-aware and boundary-gated (comparing against a recorded baseline rather than
re-running everything cold), a design this format does not yet specify — not a
per-batch shortcut.

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
- Plan validation (Go, when built) mechanically estimates each batch's context — bytes
  of the batch's Scope entries plus every card's five typed file-op path fields and both
  sides of every `Moves:` pair, resolved on disk, divided by 4 — plus a card-count cap. A
  batch over cap **without** `oversized: true` fails validation loudly, forcing the
  Planner to split or flag. (Precedent: millhouse's `_plan_validate.py`
  `batch-oversized` checks.) A path that does not yet exist on disk (a `Creates:` target,
  or a `Moves:` destination that hasn't landed) contributes zero bytes to the estimate —
  it cannot be a source of context bloat before it exists.
- The estimate is a **coarse safety net, not a bound**: it ignores conversation overhead,
  card text, tool output, and files the implementer reads that are not listed in
  Scope/the card's typed fields. A passing batch is not guaranteed to fit — the check
  catches egregious cases only.

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
  (Sonnet default), `implementer_oversized`, `recovery`. There is no builder `evaluator`:
  the LLM orchestrator itself judges digests.
- loom's config section overrides per role when loom drives builder.
- The notation, registry, and precedence rules are pinned in
  [docs/reference/model-spec.md](../reference/model-spec.md).

## Validation checks (spec for the future validator)

Machine checks this format is designed to support — they land with builder, not with this
doc, in this fixed order:

1. `format-unrecognized` / `plan-unapproved` — `format:` recognized, `approved: true`;
   else refuse to run.
2. `index-file-mismatch` — Batch Index ↔ batch files consistent (numbering, slugs, no
   gaps, no orphaned file on disk).
3. `verify-missing` — per-batch `verify:` present (or `deferred` with a valid
   `chain-end:`).
4. `chain-end-dangling` — no dangling `chain-end` (target exists, is not itself
   deferred, and is a later batch number).
5. `batch-oversized` — the context estimate (bytes of Scope + every card's typed
   file-op paths, divided by 4) and the card-count cap — over either cap without
   `oversized: true` fails loudly.
6. `scope-malformed` — every Scope entry, and every card's normalized file-op path
   (both `Moves:` sides included), is non-empty, relative, clean, and free of `..`
   escapes; the `root:`/`//` resolution rule is part of what "normalized" means here.
7. `move-format` — every non-`none` `Moves:` sub-bullet matches the
   `` `src` -> `dst` `` grammar.
8. `move-redundant` — a path is both a `Moves:` endpoint and in `Creates:`/`Deletes:`
   of the same batch.
9. `move-source-missing` — a `Moves:` source neither exists on disk nor is a
   `Creates:` target or `Moves:` destination of an earlier or later batch (plan-wide
   suppression, so a chained rename across batches never false-positives).
10. `move-target-collision` — a `Moves:` target already exists on disk, is targeted by
    more than one batch, or collides with a different batch's `Creates:` entry
    (same-batch overlap is `move-redundant`'s job).
11. `move-mechanic-missing` — a batch with at least one `Moves:` pair but no
    `## Rename mechanic` section.
12. `card-missing-field` — a card lacks one of `What:`/`Context:`/`Edits:`/`Creates:`/
    `Deletes:`/`Moves:`.
13. `card-field-overlap` — the same path appears in more than one of a single card's
    `Context:`/`Edits:`/`Creates:`/`Deletes:` fields or `Moves:` endpoints (per-card
    mutual exclusivity only — the legitimate cross-card `Creates:`-then-`Edits:`
    sequencing is never flagged).
14. `card-numbering` — a card heading's `NN` prefix must equal its batch's own number,
    and `C` must run 1..M sequentially with no gaps or duplicates.
15. `card-count-mismatch` — the Batch Index's `(C cards)` segment must equal the batch
    file's actual `### Card` heading count.
16. `path-missing` — an `Edits:`/`Deletes:`/`Context:` path (a `Moves:` source is
    check 9's job) that does not exist on disk and is not a `Creates:` target or
    `Moves:` destination of any batch.
17. `card-outside-scope` — an `Edits:`/`Creates:`/`Deletes:` path or `Moves:` endpoint
    that falls under none of the batch's own Scope prefixes (`Context:` is exempt).
18. `commit-subject-mismatch` — a present `Commit:` value that does not start with the
    card's own `NN.C: ` prefix.

## Worked example

A complete minimal plan for a fictional task ("add a `--json` flag to `lyx board list`"),
byte-consistent across index ↔ filenames ↔ report. Across its three batch files this
example demonstrates every plan-format v2 feature: all five typed file-op fields (with
`none` sentinels), `NN.C` card headings, `(C cards)` Batch Index segments, a
`## Shared Decisions` overview entry, a `root:` batch with a `//`-escaped path, a pinned
`Commit:` field, and a `Moves:` card with its `## Rename mechanic` section.

`_lyx/plan/00-overview.md`:

```markdown
---
format: 2
approved: true
---

# Plan: add --json to `lyx board list`

Add a `--json` output mode to `lyx board list`, emitting one JSON object per row via the
`internal/output` envelope, with tests and help text updated, and the row mapper
relocated ahead of a later extraction.

## Batch Index

- 01 — json-flag (2 cards) — add the `--json` flag and envelope emission to boardcli list
- 02 — list-tests (2 cards) — cover `--json` in boardcli list tests, update help-tree pins, and rename the row mapper

## Shared Decisions

### Decision: json-envelope-reuse

- **Decision:** `--json` marshals each row through the existing `internal/output.Ok`
  envelope — no new envelope type is introduced.
- **Rationale:** one JSON emission path for the whole CLI; a second envelope shape
  would fork behavior for no gain.
- **Applies to:** all batches
```

`_lyx/plan/01-json-flag.md`:

```markdown
---
root: internal/boardcli
---

# 01 — json-flag: add the --json flag and envelope emission

## Intent

`lyx board list --json` emits one `output.Ok` envelope per row instead of the table.
Stand-alone: after this batch the flag works end-to-end; tests land in batch 02.

## Scope

- internal/boardcli/list.go
- internal/boardengine/rows.go

## Cards

### Card 01.1 — flag + row struct

**What:** Add a `--json` bool flag to the list command; define `RowJSON` with the
existing table's columns as fields.
**Context:** none
**Edits:**
- `list.go`
- `//internal/boardengine/rows.go`
**Creates:** none
**Deletes:** none
**Moves:** none
**Commit:** `01.1: add the --json flag and row struct`
**verify:** go build ./...

### Card 01.2 — emission path

**What:** When `--json` is set, marshal each row through `output.Ok` instead of the
table writer; keep the table path unchanged.
**Context:**
- `//internal/output/envelope.go`
**Edits:**
- `list.go`
**Creates:** none
**Deletes:** none
**Moves:** none

## verify:

go test ./internal/boardcli/... ./internal/boardengine/...
```

`list.go` above resolves (per the batch's `root: internal/boardcli`) to
`internal/boardcli/list.go`; the `//`-prefixed `rows.go` and `envelope.go` entries stay
worktree-root-relative regardless of `root:`, escaping it for the one file each card
needs outside the shared prefix.

`_lyx/plan/02-list-tests.md`:

```markdown
# 02 — list-tests: cover --json in tests, update help pins, and rename the row mapper

## Intent

Tests prove the `--json` path end-to-end; help-tree pins reflect the new flag; the row
mapper is relocated ahead of a later extraction (not shown in this example).
Stand-alone: assumes batch 01 is committed.

## Scope

- internal/boardcli/list_test.go
- cmd/lyx/helptree_test.go
- internal/boardengine/rows.go
- internal/boardengine/rowsjson.go

## Rename mechanic

1. Run `git mv internal/boardengine/rows.go internal/boardengine/rowsjson.go` FIRST,
   before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. Use `Creates:` only for genuinely new files, never for the relocated file itself.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.

## Cards

### Card 02.1 — list --json tests

**What:** Add table-driven tests asserting one `output.Ok` envelope per row for
`list --json`, and that the table path is unchanged without the flag.
**Context:** none
**Edits:**
- `internal/boardcli/list_test.go`
**Creates:** none
**Deletes:** none
**Moves:** none

### Card 02.2 — help-tree pin + row-mapper rename

**What:** Update the pinned help-tree set with the new `--json` flag help text, and
relocate the row mapper via `git mv` per the Rename mechanic above (no behavior change
in this card).
**Context:** none
**Edits:**
- `cmd/lyx/helptree_test.go`
**Creates:** none
**Deletes:** none
**Moves:**
- `internal/boardengine/rows.go` -> `internal/boardengine/rowsjson.go`

## verify:

go test ./internal/boardcli/... ./cmd/lyx/...
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
