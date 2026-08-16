# Plan: PATTERN directives: move from Go constants to stencil files

```yaml
task: 'PATTERN directives: move from Go constants to stencil files'
slug: pattern-directive-stencils
approved: false
started: '20260816-145612'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: stencil-files
    file: 01-stencil-files.md
    depends-on: []
    verify: go build ./... && go test ./...
  - number: 2
    name: directive-read-path
    file: 02-directive-read-path.md
    depends-on: [1]
    verify: go build ./... && go test ./...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [2]
    verify: go build ./... && go test ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the stencil body is the byte-exact contract, never the whole file

- **Decision:** each new stencil file's **body** — the bytes remaining after `stencil.StripLeadingComment` — is byte-identical to the Go constant it replaces, trailing newline included.
  Whole-file byte equality is never asserted anywhere: every stencil carries a leading `<!-- … -->` banner, and `stencilstore.Reconcile` writes a `lyx-stencil:` stamp line into that banner when it seeds the file.
- **Rationale:** `Directive`'s return value is what must not change, and that is the stripped body.
  Stating the contract as whole-file equality would make every assertion about it false the moment a real hub stamps the file.
- **Applies to:** all batches

### Decision: `Directive` strips the banner itself, because its value never passes through `stencil.Fill`

- **Decision:** `Directive` calls `stencil.StripLeadingComment` on the bytes `stencilstore.Read` returns.
  Strip only — no LF normalisation.
- **Rationale:** `stencilstore.Read` is a plain `os.ReadFile` and strips nothing.
  Every other stencil consumer survives that only because it hands the bytes to `stencil.Fill`/`FillOptional`, whose first act is the same strip.
  `Directive`'s return is a *value* in a producer template's `values` map, never a template, so nothing downstream would strip it — without this call a real hub would inject the stamp banner into all four producer prompts.
  Strip-only rather than strip-plus-normalise keeps `Directive` exactly in step with `Fill`, rather than making it the odd one out for a CRLF hazard the whole stencil system shares.
- **Applies to:** all batches

### Decision: fail loud on a read failure

- **Decision:** `Directive` returns `(string, error)`.
  An active PATTERN whose directive stencil cannot be read is an error, propagated to the caller, wrapping `stencilstore.Read`'s own error.
  This overrides step 3 of `manifest/designs/pattern-directive-stencils.md`, which specified `logger.Warn` + `""`.
- **Rationale:** `stencilstore.Read`'s doc comment states the missing-board-is-a-hard-error contract directly, every other stencil consumer in the repo already hard-errors on a read failure, and `internal/pattern`'s own documented posture on ambiguity is fail-loud.
  Returning `""` when PATTERN *is* active would strip the constraints from an agent prompt invisibly.
  It also avoids an `internal/logger` allowlist entry.
- **Applies to:** all batches

### Decision: the read is lazy — no read on any inactive path

- **Decision:** `Directive` keeps its existing guard order and reads only after them.
  The full matrix: `l == nil` → `("", nil)`, no read; PATTERN inactive → `("", nil)`, no read; active with an unknown or zero `Role` → `("", nil)`, no read; active with a known role and a successful read → `(stencil.StripLeadingComment(string(content)), nil)`; active with a known role and a failed read → `("", err)`.
- **Rationale:** preserves all five of today's behaviours bit-for-bit, and confines fixture churn to the tests that actually activate PATTERN.
  An eager read would make every inactive-PATTERN call site depend on a seeded stencils directory.
- **Applies to:** all batches

### Decision: the Pattern Leaf Invariant gains exactly two entries, with different justifications

- **Decision:** `internal/pattern`'s allowlist gains `github.com/Knatte18/loomyard/internal/stencilstore` and `github.com/Knatte18/loomyard/internal/stencil`, and nothing else.
  The two carry **different** justifications in `CONSTRAINTS.md`: `internal/stencil` is a zero-import leaf and so cannot participate in a cycle by construction (the same argument the existing text already makes for `internal/lyxdirs`);
  `internal/stencilstore` is **not** a leaf — it imports `internal/stencil` and `internal/logger` — and its justification is that it is shared infrastructure rather than a feature package, plus a verified-acyclic closure.
- **Rationale:** conflating the two would state something false about `internal/stencilstore`.
  The invariant's stated subject is feature packages;
  neither addition is one.
- **Applies to:** batch 2, batch 3

### Decision: `Role` stays an `int` enum and the parameter order is `(l, stencilsDir, role)`

- **Decision:** `type Role int` and its three `iota` members are unchanged;
  only the `switch` body changes, from yielding a directive constant to yielding a stencil *name*, with the read happening once after the switch.
  The new signature is `Directive(l *lyxcwd.Location, stencilsDir string, role Role) (string, error)`.
- **Rationale:** zero `Role`-related churn at the four call sites.
  The `(location, stencilsDir, …)` argument order matches `loomengine.PlanSpec(layout, stencilsDir, cfg, reg)`, the house shape for a function that is told its stencils directory.
- **Applies to:** batch 2

### Decision: every batch verifies with the full suite, not a scoped package list

- **Decision:** every batch's `verify:` is `go build ./... && go test ./...`.
  No `PYTHONPATH= ` prefix — this is a Go repo, not a Python one.
- **Rationale:** the load-bearing guards for this change live in at least four unrelated packages (`internal/pattern`'s leaf test, `stencils/registry_test.go`, `internal/lyxcwd`'s vocabulary and geometry walks, `cmd/lyx`'s tier-purity guard), so a hand-enumerated package list is one omission away from false confidence.
  Measured baseline: a warm `go build ./... && go test ./...` completes well inside a normal verify budget on this tree.
- **Applies to:** all batches

### Decision: the three new files must stay marker-free and vocabulary-clean

- **Decision:** neither the banner nor the body of any new stencil contains a `{{` sequence, and neither contains the substrings `weft` or `warp` or a fabric-sense `host <noun>` phrase.
- **Rationale:** `stencilstore.Validate` parses every registered file *and* its shipped default with `stencil.TopLevelMarkers` even though these three never reach `stencil.Fill`, so a stray `{{` becomes a `lyx stencil validate` error rather than a harmless literal.
  Separately, `stencils/**/*.md` is inside the Fabric Vocabulary Invariant's enforcement walk (`internal/lyxcwd/enforcement_test.go`), which fails on those tokens.
  Zero markers is a valid template and yields zero findings, so this costs nothing as long as it stays true.
- **Applies to:** batch 1

### Decision: `manifest/designs/pattern-directive-stencils.md` is corrected and kept, not deleted

- **Decision:** the design doc's status flips and its four false claims are corrected in batch 3;
  the file is not deleted.
- **Rationale:** this is the decision `_mill/discussion.md` records, and it governs.
  Recording the tension rather than papering over it: `docs/overview.md`'s Documentation lifecycle and `manifest/roadmap.md`'s own Maintenance note say a `manifest/designs/<name>.md` doc is *deleted* when its item ships, with the Done entry pointing at the module's package documentation instead.
  That rule is written for **module**-design docs, and this doc describes a feature change to an already-shipped package (`internal/pattern`), which is why the discussion's keep-and-correct choice is defensible rather than a straight violation.
  The Done roadmap entry still points at `internal/pattern`'s package documentation, per the Maintenance note.
  A reviewer who disagrees should raise it against the discussion's Decision, not against this plan.
- **Applies to:** batch 3

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `.gitattributes`
- `CONSTRAINTS.md`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/prompt_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/prompt_test.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/pattern/pattern.go`
- `internal/pattern/pattern_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/pattern-directive-stencils.md`
- `manifest/roadmap.md`
- `stencils/pattern/pattern-directive-implementer.md`
- `stencils/pattern/pattern-directive-orchestrator.md`
- `stencils/pattern/pattern-directive-review-fix.md`
- `stencils/stencils.go`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
