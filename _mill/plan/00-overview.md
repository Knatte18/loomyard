# Plan: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()

```yaml
task: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()
slug: quarry-bump-v0-2-0-status-helpers
approved: true
started: 20260909-083542
parent: main
root: ""
verify: null
skip_checks: ["verify-full-suite"]
discussion_sha: 5a26144662b3691e12085cafa157cb7516b2b32b
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: quarry-bump-and-adoption
    file: 01-quarry-bump-and-adoption.md
    depends-on: []
    verify: CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: one-batch-scope

- **Decision:** the whole task is a single batch of four cards — the dependency bump, the `Known()` adoption, the `Rejected()` adoption, and the drift-guard test plus tripwire prose.
- **Rationale:** every card touches `internal/planglyph` (plus `go.mod`/`go.sum`), the package is ~1500 lines of production code, and the cards share almost their whole `Context:` set.
  Splitting them would push a builder to re-read the same four files across batches for no isolation benefit, and the `Known()` swap is not compilable until the bump lands.
- **Applies to:** all batches

### Decision: behaviour-preserving-swaps

- **Decision:** cards 2 and 3 must not change what findings `internal/planglyph` emits for any of the four readable statuses, for the zero value, or for an out-of-vocabulary status string.
  The existing assertions in `internal/planglyph/donecheck_test.go` are the proof and must pass unedited.
- **Rationale:** `switch r.Status { case <the four>: default: ... }` and `r.Status.Known()` have identical truth sets today, as do `r.Status == ""` and `r.Rejected()` — `Rejected()`'s own body is literally `return r.Status == ""`.
  A card that needs to edit an existing assertion has changed behaviour and is wrong.
- **Applies to:** all batches

### Decision: no-doc-changes

- **Decision:** no changes to `manifest/roadmap.md`, `CONSTRAINTS.md`, `docs/overview.md`, or anything under `manifest/designs/`.
- **Rationale:** per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item, and this is a hardening pass covered by git history.
  No module table, execution-stack, or observable CLI behaviour changes, and no new cross-cutting invariant appears — the `.Status` tripwire already exists and is unchanged in kind.
- **Applies to:** all batches

### Decision: follow-up-lives-in-a-commit-message

- **Decision:** the `internal/quarrycli` follow-up this task deliberately does not fix is recorded as a paragraph in card 3's own commit-message **body**, spelled out verbatim in that card's `Requirements:`.
  The `Commit:` field carries only the subject line, because the template defines it as a one-liner.
- **Rationale:** `_mill/` is a task-branch-only tree — `origin/main` carries no `_mill/` path — so neither `discussion.md` nor this plan survives `mill-merge` and neither can be the durable record.
  A paragraph in the landing commit's body is durable on `main`, costs no diff, and does not force the roadmap move `no-doc-changes` rules out.
  It rides on card 3 because card 3 is the `Rejected()` adoption and the follow-up is the same pattern left unadopted one package over.
- **Applies to:** quarry-bump-and-adoption

### Decision: statuses-is-read-only

- **Decision:** nothing in `lyx` may assign into `quarry.Statuses`, range it to build a runtime predicate, or size a loop off `len(quarry.Statuses)` as an assertion.
  It is read for exactly one purpose: the card 4 coverage assertion's two-directional set comparison.
- **Rationale:** `quarry.Statuses` is `engine.Statuses` itself, not a copy — quarry's own doc comment says so, and quarry rejected implementing `Known()` by ranging it for precisely this reason.
  A `len()` comparison would also pass a same-size add-one/remove-one swap, which is the drift the card 4 test exists to catch.
- **Applies to:** quarry-bump-and-adoption

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `go.mod`
- `go.sum`
- `internal/planglyph/donecheck.go`
- `internal/planglyph/resolve.go`
- `internal/planglyph/status_completeness_test.go`
- `internal/planglyph/status_enforcement_test.go`
