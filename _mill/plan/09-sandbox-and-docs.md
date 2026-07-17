# Batch: sandbox-and-docs

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'sandbox-and-docs'
number: 9
cards: 3
verify: go test ./...
depends-on: [8]
```

## Batch Scope

Close-out: wire the webster sandbox suite into the runner (the suite file
itself landed with registration in card 38), fill both scenarios to their full
specified shape, and land every documentation obligation — the
builder-contract.md contract deltas, the overview module row, the roadmap
milestone flip, and the Master Builder → webster rename sweep. Final verify is
the full tree: every enforcement guard in the repo passes with the finished
module.

## Cards

### Card 40: sandbox suite scenarios and runner wiring

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `sandbox-builder-suite.cmd`
  - `_mill/discussion.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/main.go`
- **Creates:**
  - `sandbox-webster-suite.cmd`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Bring `SANDBOX-WEBSTER-SUITE.md` to its full shape in the
  existing suites' grammar (`### W<N> -- <title>`, `**Covers:** webster`,
  `**Goal:**`, `**Watch:**`, `**Verdict:** OK / WARN / FAIL`, `---`
  separators, the standard `## Capturing findings` section):
  **W1 — happy path:** a tiny two-batch plan driven by `lyx webster run` to
  `"outcome":"done"`; watch: one fork per batch (no extra mux strands during
  batches), per-batch weft commits landing, digest envelopes from
  `record-batch`, and a valid `summary.md` (`# <title>` first line) at exit.
  **W2 — /model injection validation (the escalation-vs-fallback decider,
  discussion.md `oversized-model-escalation`):** an `oversized: true` batch
  whose `begin-batch` injects `/model` WHILE the begin-batch Bash subprocess
  itself is still the foreground tool call in the Master's pane; watch, as
  three separately-verdicted assertions: (a) the keys reach the Claude TUI
  input and the session's model switches for subsequent calls in the same
  turn (miss = benign → the documented fallback: `oversized:` keeps plan
  compatibility but has no spawn effect); (b) the injected keystrokes do NOT
  leak into the running subprocess's stdin/output or corrupt that tool
  call's result (corruption = the dangerous class → fallback
  unconditionally); (c) the fork's `subagents/<id>.jsonl` transcript exists
  on disk at the moment the Agent call returns (the incremental audit's
  flush-timing assumption). State in the scenario prose that W2's verdict
  decides whether the escalation feature stays enabled.
  Runner wiring: `//go:embed SANDBOX-WEBSTER-SUITE.md` + a `websterSuite`
  spec in `tools/sandbox/suite.go`, a `case "webster-suite"` in
  `tools/sandbox/main.go`, and `sandbox-webster-suite.cmd` at the repo root —
  byte-identical to `sandbox-builder-suite.cmd` except the subcommand token
  and header comment.
- **Commit:** `sandbox: webster suite scenarios and runner wiring`

### Card 41: builder-contract deltas and overview row

- **Context:**
  - `internal/websterengine/doc.go`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/modules/builder-contract.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `builder-contract.md` (a contract doc, exempt from the
  module-doc deletion rule) gains a `## Webster: the fork-based sibling`
  section pinning ONLY the cross-module contract facts a future `loom`
  consumes: webster consumes the same plan-format input via the same parser;
  emits the same batch-report schema (one parser: `ParseReport`); emits a
  compatible `outcome.yaml` (same schema, `ParseOutcome`); adds the
  webster-only `_lyx/webster/summary.md` (`# <title>` + narrative, required
  at `outcome: done`, archive-never-refuse) as `loom-finalize`'s PR-text
  source; state/artifacts live in `_lyx/webster/` so A/B runs on the same
  plan never collide; the digest contract is shared. Point at
  `internal/websterengine`'s package docs for webster's own design.
  `docs/overview.md`: add the `- **webster** — ...` bullet to `## Modules`
  (fork-based sibling of builder: one long-lived Master session, in-session
  Agent-tool forks per batch, bracket verbs, cold-strand recovery;
  `internal/websterengine` + `internal/webstercli`; ✅ Implemented).
- **Commit:** `docs: webster contract deltas in builder-contract and overview`

### Card 42: roadmap flip and rename sweep

- **Context:**
  - `docs/modules/builder-contract.md`
- **Edits:**
  - `docs/roadmap.md`
  - `docs/long-term-ideas.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The module's name is `webster`; "Master Builder" was the
  POC label — update every remaining doc mention (the user's explicit
  rename-sweep directive). `docs/roadmap.md`: milestone 26 — retitle to
  `webster — fork-based implementation module (né "Master Builder")`, mark
  ✅ **Done** with a pointer to builder-contract.md's webster section and the
  websterengine package docs (roadmap discipline: a completed planned
  milestone gets the flip, nothing else appended); update the other three
  "Master Builder" mentions (the milestone-76-area cross-reference and the
  in-milestone body text) to webster. `docs/long-term-ideas.md`: retitle the
  DAG section heading to
  `## Webster: parallel batches via a DAG (further-out, beyond the sequential fork model)`
  and update the body's "Master Builder" mentions to webster — content
  otherwise unchanged (the DAG extension explicitly stays speculative and
  out of this task's scope).
- **Commit:** `docs: mark webster milestone done and finish the rename sweep`

## Batch Tests

`go test ./...` — the whole tree, deliberately: this is the task's terminal
gate, proving every enforcement guard (geometry literals, CLI registration
quartet, sandbox coverage incl. the stale-tag assert, tier purity, hermetic
git, provider-seam import rule) holds with the finished module. The docs cards
have no runnable surface of their own; the full-tree run is the batch's
verification that nothing they touched (embed directives, suite specs)
regressed the build.
