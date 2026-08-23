# Plan: loom: self-checkable mechanical gates

```yaml
task: 'loom: self-checkable mechanical gates'
slug: 'loom-self-checkable-mechanical-gates'
approved: false
started: '20260823-100622'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: discussionparser leaf package
    file: 01-discussionparser-leaf.md
    depends-on: []
    verify: go test ./internal/discussionparser/... ./internal/lyxcwd/...
  - number: 2
    name: loomshed thin wrap
    file: 02-loomshed-thin-wrap.md
    depends-on: [1]
    verify: go test ./internal/loomshed/...
  - number: 3
    name: loom CLI validate verbs
    file: 03-loomcli-validate-verbs.md
    depends-on: [1]
    verify: go test ./internal/loomcli/... ./cmd/lyx/...
  - number: 4
    name: gate parity tests
    file: 04-gate-parity-tests.md
    depends-on: [2, 3]
    verify: go test ./internal/loomcli/... ./internal/lyxcwd/...
  - number: 5
    name: docs and roadmap
    file: 05-docs-and-roadmap.md
    depends-on: [4]
    verify: go test ./internal/lyxcwd/... ./cmd/lyx/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: shared-implementation-is-the-whole-point

- **Decision:** each mechanical gate's `ShedProducer` row and its new CLI verb call the identical package function — `discussionparser.Validate` for `Discussion-Validate`, `planparser.Validate` for `Plan-Validate`.
  No verb re-implements, re-derives, or re-parses anything the producer checks, and no producer keeps a private copy of a check the verb calls.
- **Rationale:** the roadmap item exists so the writer agent's self-check and the Shed-level gate can never disagree.
  Two implementations is the exact failure mode being eliminated.
- **Applies to:** all batches

### Decision: short-circuit-order-is-load-bearing

- **Decision:** `discussionparser.Validate` reproduces `internal/loomshed/discussionvalidate.go`'s current control flow step for step: `os.Stat` the support log first (not-exist returns immediately with exactly one finding and a nil error; any other stat error returns immediately as an error with a nil findings slice), then `os.ReadFile` the decision record (same two-way split), and only then the heading check, which is the one place findings accumulate.
  An error always wins over a finding a *later* check would have produced, because the later check never runs.
- **Rationale:** an accumulating implementation flips one real case — support log missing plus a decision record unreadable for a reason that is not not-exist — from `Stuck` (persist blocked, bounce to `Discussion-Write`) to a returned error (persist failed, abort the run).
  Pinning the order is what makes "the producer's outward contract is unchanged" a checkable claim rather than an aspiration.
- **Applies to:** all batches

### Decision: producer-outward-contract-unchanged

- **Decision:** `loomshed.discussionValidate.Call` keeps its exact outward contract: a non-empty findings slice maps to `shedengine.Stuck` with an empty `OutputPointer`; zero findings maps to `shedengine.Done` with the decision record's path as the pointer; an I/O fault that is not a not-exist maps to a returned error, never to `Stuck`.
  The existing `entryErr` / `cancelErr` cancellation discipline and the `nonDoneExit` helper stay as they are.
  Findings never travel on the producer's `OutputPointer`.
- **Rationale:** the roadmap item is about making the check callable standalone, not about changing what the gate does.
  The task that replaces the `Discussion-Write` stub reads findings from the CLI verb it calls itself, not from the pointer of a gate it bounced off.
- **Applies to:** all batches

### Decision: envelope-and-exit-contract

- **Decision:** both verbs emit exactly one `internal/output` envelope per invocation.
  A clean gate emits `output.Ok` and exits 0.
  A gate with findings emits `output.ErrFields(out, "<summary>", map[string]any{"findings": []string{...}})` and exits 1.
  An I/O fault (or, for the plan verb, a `planparser.ParsePlan` error) emits `output.Err` and exits 1, with no `findings` key at all.
  Every `RunE` checks `clihelp.ShouldAbort(cmd.Context())` first, ahead of its own work.
- **Rationale:** the CLI / Cobra Invariant requires one JSON envelope per invocation and forbids bare plain-text error paths, and a non-zero exit on a failed gate is what makes the verb usable as a shell-level self-check.
  The findings-vs-fault distinction is deliberately **structural** (the presence or absence of the `findings` key), never a matter of message wording, because the parity tests' three-way comparison keys off exactly that.
- **Applies to:** `loom CLI validate verbs`, `gate parity tests`

### Decision: one-non-zero-exit-code-confirmed

- **Decision:** confirmed for this task — "the artifact needs more work" and "something is broken" both exit 1, distinguished only by envelope content.
  The discussion flagged this collapse for the plan stage to confirm, and the anticipated consumer is still an LLM agent reading the JSON body: the two roadmap items that consume these verbs (`loom: Discussion-Write producer`, `loom: Plan-Write producer`) both consume them from a `SingleLLMProducer` prompt, and no shell script that branches on `$?` alone is planned anywhere.
  If such a consumer is ever written, this decision must be revisited **before** it is written, not after.
- **Rationale:** `internal/output` offers no natural third state — `Ok` returns 0, both `Err` and `ErrFields` return 1 — so a distinct exit code would mean a new helper in a shared package, built speculatively for a consumer that does not exist.
  The envelope already names which kind of failure it is, structurally.
- **Applies to:** `loom CLI validate verbs`, `gate parity tests`

### Decision: findings-render-as-error-strings

- **Decision:** `discussionparser.Finding` is a struct with `Check`, `Path`, and `Detail` string fields plus an `Error() string` method, mirroring `planparser.ValidationError`'s shape.
  Both verbs render their findings payload identically: a `[]string` built by calling `Error()` on each finding, placed under the envelope's `findings` key.
- **Rationale:** the discussion left the exact field set to the plan stage but pinned the constraint — the two verbs' payloads must read alike, and the plan verb already has `[]planparser.ValidationError` with an `Error()` method to render.
  A `[]string` of rendered findings is the one shape both sides can produce without either package growing a JSON-marshalling concern.
- **Applies to:** `discussionparser leaf package`, `loom CLI validate verbs`

### Decision: cli-tests-never-go-through-runcliin

- **Decision:** every new `internal/loomcli` test — verb behaviour and both parity tests — builds the leaf `*cobra.Command` from `(*loomCLI).validateDiscussionCmd` / `(*loomCLI).validatePlanCmd` against a hand-populated `*loomCLI` receiver and runs it via `clihelp.Execute(cmd, &out, nil)`.
  No new test calls `RunCLIIn`, and no new test file carries a build tag.
- **Rationale:** a real verb takes the non-group branch of `resolvePersistentPreRun`, where `lyxcwd.Resolve` spawns `git rev-parse` and `wire` then calls `loomengine.LoadConfig` strictly — that breaches the Test Tier Purity Invariant in an untagged suite.
  `TestVerbRefusals` in `internal/loomcli/cli_test.go` already establishes the hand-populated-receiver mechanism for `drive` and `pause`.
- **Applies to:** `loom CLI validate verbs`, `gate parity tests`

### Decision: go-conventions-and-verify-scope

- **Decision:** this is a Go repo, so no `verify:` command carries a `PYTHONPATH=` prefix; each batch's `verify:` is a `go test` over exactly the packages that batch touches.
  Batches whose cards edit a `.md` file under `manifest/` or `docs/` additionally run `go test ./internal/lyxcwd/...`, because `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` is the Markdown Link Integrity check over those two roots.
- **Rationale:** the batch-verify scope must match what the batch touches, and the markdown link check lives in a package no docs batch would otherwise compile.
- **Applies to:** all batches

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` edit in this task uses semantic line breaks — one sentence per line, with an additional break at an internal independent-clause boundary in a long sentence.
  Never a fixed-column hard-wrap, never trailing double-spaces or a backslash.
  Table cells and blockquotes stay on one line.
- **Rationale:** `CLAUDE.md` mandates this for every `.md` file in the repo, not only newly written ones.
- **Applies to:** `discussionparser leaf package`, `gate parity tests`, `docs and roadmap`

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `docs/overview.md`
- `internal/discussionparser/doc.go`
- `internal/discussionparser/leaf_enforcement_test.go`
- `internal/discussionparser/validate.go`
- `internal/discussionparser/validate_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/parity_test.go`
- `internal/loomcli/validate.go`
- `internal/loomcli/validate_test.go`
- `internal/loomshed/discussionvalidate.go`
- `internal/loomshed/discussionvalidate_test.go`
- `internal/loomshed/seam_enforcement_test.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
