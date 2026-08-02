# Plan: webster: stop re-rendering already-inherited context into fork prompts

```yaml
task: 'webster: stop re-rendering already-inherited context into fork prompts'
slug: 'webster-fork-context-hygiene'
approved: true
started: '20260802-133803'
parent: 'main'
root: ""
verify: go build ./... && go test ./internal/websterengine/... ./internal/planparser/... ./internal/hubgeometry/...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: planparser-card-source-identity
    file: 01-planparser-card-source-identity.md
    depends-on: []
    verify: go build ./... && go test ./internal/planparser/... ./internal/hubgeometry/...
  - number: 2
    name: webster-prompt-split
    file: 02-webster-prompt-split.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/websterengine/... ./internal/hubgeometry/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: fork-context-hygiene (supersedes fork-prompt-plan-level-context)

- **Decision:** This task reverses the prior `fork-prompt-plan-level-context` Shared Decision (which had `RenderForkPrompt` inline plan-level Shared Decisions, the rename mechanic, the PATTERN directive, and every card's own fields into every fork prompt). The new decision, named **`fork-context-hygiene`**, is: a thin in-session fork prompt that injects nothing already inherited from Master; a full, honest cold-start recovery prompt for the separate recovery-strand process; a single shared implementer-job body both prompts compose; and card content delivered by a worktree-relative card-file pointer (`_lyx/plan/NN-<slug>.md`) rather than inlined fields. Every doc comment naming the old decision is rewritten to the new name.
- **Rationale:** `RenderForkPrompt`'s output feeds two callers with opposite context situations — `beginbatch.go`'s in-session fork (inherits Master's whole context) and `recoverbatch.go`'s cold recovery strand (a separate process that inherits nothing). One prompt cannot be honest for both. See `_mill/discussion.md` Decisions `split-fork-and-recovery-prompts`, `shared-implementer-body`, `thin-in-session-fork-prompt`, `full-cold-recovery-prompt`.
- **Applies to:** all batches.

### Decision: card pointer is a bare worktree-relative token owned by planparser

- **Decision:** The card-file pointer rendered into both prompts is the bare worktree-relative token `_lyx/plan/NN-<slug>.md`, stored on `planparser.Card`. planparser builds it by joining a **new `hubgeometry.PlanDirRel()` accessor** (which returns the relative `_lyx/plan` token, so the `_lyx/plan` path is constructed inside `hubgeometry` per `PlanDir`'s own "no other package may construct this path" doc) with planparser's own `NN-<slug>.md` filename (the plan-file naming that the Sole-Parser Invariant reserves to planparser). planparser must NOT hardcode the literal `"plan"` segment or a literal `_lyx`. `render.go` renders the stored token **verbatim** — it must NOT `filepath.Rel`/`filepath.Join`-compose the pointer against `Cwd`/`WorktreeRoot`, must NOT rebuild the `NN-<slug>.md` filename from `Card.Number`/`Slug`, and must NOT name a literal `_lyx`.
- **Rationale:** Hub Geometry Invariant reserves the `_lyx` token to `hubgeometry` and `PlanDir`'s doc reserves the `_lyx/plan` construction to `hubgeometry`; Planparser Sole-Parser Invariant makes the `NN-<slug>.md` filename planparser's domain — so the `_lyx/plan` segment comes from a hubgeometry accessor and the filename from planparser. A bare token (not a cwd-relative composition) matches every other bare `_lyx/...`/`_pattern/...`/`CONSTRAINTS.md` token already in the webster prompts, all of which resolve from the session cwd. See `_mill/discussion.md` Decisions `card-pointer-relative-via-hubgeometry`, `card-source-identity-in-planparser`.
- **Applies to:** all batches.

### Decision: empty-What falls back to the Card Index intent, in the prompt

- **Decision:** The shared implementer-body's card instruction tells the agent: read each card file; if a card's `What:` is empty, fall back to that card's one-line intent from the Card Index in `00-overview.md` (matched by NN/slug). This preserves today's `renderCard` What→Intent fallback under the pointer scheme. Empty `What` is NOT promoted to a `planparser.Validate` error.
- **Rationale:** `Card.Intent` lives only in the Card Index (`00-overview.md`), never in `NN-<slug>.md`; the pointer switch would otherwise silently drop the fallback. See `_mill/discussion.md` Decision `empty-what-index-fallback`.
- **Applies to:** webster-prompt-split.

### Decision: stencil has no include support — reuse is Go byte-composition, banner only on the prefix

- **Decision:** `internal/stencil` is single-pass `text/template` with no `{{template}}` include support and no recursive marker expansion inside a substituted value. So the shared body is reused by **Go byte-composition** of raw template text before `Fill`: `prefix-bytes + body-bytes`, then `stencil.Fill`/`FillOptional`. `stencil.stripLeadingComment` strips only a `<!-- -->` banner at the very TOP of the composed text, so **only the prefix asset may carry a leading `<!-- -->` banner; the shared body asset must carry none.**
- **Rationale:** Injecting a pre-rendered body as a `{{.marker}}` value would leave the body's own `{{.report_path}}` etc. unfilled (single-pass). Composing raw bytes keeps the body's markers in the template text where `Fill` resolves them. See `_mill/discussion.md` Decision `shared-implementer-body` and `internal/stencil/stencil.go`.
- **Applies to:** webster-prompt-split.

## All Files Touched

- `internal/hubgeometry/hubgeometry.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/fork-prefix.md`
- `internal/websterengine/implementer-body.md`
- `internal/websterengine/integration-template.md`
- `internal/websterengine/master-template.md`
- `internal/websterengine/recovery-prefix.md`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/render.go`
- `internal/websterengine/template_test.go`
