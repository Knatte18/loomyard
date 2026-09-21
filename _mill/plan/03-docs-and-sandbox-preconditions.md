# Batch: docs-and-sandbox-preconditions

```yaml
task: "Spawned agent panes resolve the spawning lyx binary"
batch: "docs-and-sandbox-preconditions"
number: 3
cards: 2
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks && go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded
depends-on: [2]
```

## Batch Scope

This batch carries the operator-facing half of the change: the crucible and sandbox-howto prose describing the landed mechanism, and the live-substrate check lines the sandbox suites need because the property this task guarantees — a pane spawned by one binary resolving `lyx` to that same binary — is a live fact no hermetic test can prove.
It is one batch because every card edits only `.md` prose against the same two guard tests, sharing no code context with batches 1 and 2.
It depends on batch 2 because the prose describes a mechanism that must already exist;
per the `docs-describe-the-landed-mechanism` decision no interim "prerequisite until the mechanism lands" warning is written, since it would be stale in the commit that added it.
The batch-local decision that differs from the overview is none;
`claude-resolution-check-is-shuttle-only` and `markdown-semantic-line-breaks` are the two Shared Decisions that bind it most directly.

## Cards

### Card 8: Describe the landed mechanism in the crucible and sandbox guides

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/reedengine/panebin.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `crucible/README.md`
  - `docs/sandbox-howto.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `crucible/README.md`, add one bullet to the "Reusable rules that bit us and are worth carrying to any module's live driving" list, immediately after the existing **Deploy-first footgun** bullet, keeping the list's bold-lead-in-then-explanation shape.
  The bullet states that spawned agent panes run the binary that spawned them: reed prepends the spawning binary's directory to every strand pane's `PATH` and exports `LYX_BIN` to its absolute path, so an agent pane's bare `lyx …` resolves to the same build driving the round — no PATH setup needed.
  It then states the consequence for a round agent: the deploy-first footgun above is about the binary the round is *started* from, and the panes underneath it inherit that choice rather than falling back to the operator's production install.
  Name the concrete failure this closes, since it is the reason the bullet exists: crucible round `fable-high-r2` of batten ran the dev binary in batten and loom while the Discussion/Plan/Webster agents inside the child worktree ran a newer `PATH`-resolved build, which reconciled and committed a rewrite of the child's reed and loom config that the dev binary could then no longer load — and nothing reported the mismatch.

  In `docs/sandbox-howto.md`, extend the "What the suite does" section.
  The paragraph that currently explains that the suite prepends `.dev-bin` to the agent's own child-process PATH when the resolved binary is the dev build gains a following sentence stating that panes the agent itself spawns inherit the same resolution from reed rather than from the launcher: reed prepends the spawning binary's directory to each strand pane's `PATH` and exports `LYX_BIN`, so a nested `lyx …` inside a spawned pane resolves to the binary under test too.
  State plainly that the two mechanisms are complementary and neither replaces the other — the launcher composes a Go child process's environment, reed composes a pane shell statement — because `tools/sandbox/resolve.go`'s own `prependPath` is unchanged by this task and the `Dev/Prod Binary Separation` invariant still names it the sole `.dev-bin`-first resolution site for sandbox tooling.
  Do not change the prerequisites list's statement that the dev binary in `.dev-bin` does not need to be on PATH — it remains true, and reed's prelude is what extends it to nested panes.

  Both files use semantic line breaks, one sentence per line with an extra break at an internal independent-clause boundary, per the `markdown-semantic-line-breaks` Shared Decision.
  Add no markdown link whose target does not already resolve — `docs/sandbox-howto.md` is inside the `Markdown Link Integrity` invariant's scan scope.
- **Commit:** `docs(crucible,sandbox): record that spawned panes resolve the spawning binary`

### Card 9: Add the live binary-resolution checks to the sandbox suite pre-conditions

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/reedengine/panebin.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both suite files open with a `## Pre-conditions` numbered list whose item 1 is **Deploy a fresh dev binary**, already ending in the phrase "no PATH setup needed, and production `lyx` stays untouched".
  Extend that item in each file, in place, with a short nested check list an agent driving the suite runs once a pane exists.
  Keep the existing numbering and the existing wording of every other item;
  add no new top-level numbered item, so the two files' pre-condition structure stays aligned with each other and with the main suite.

  In `tools/sandbox/SANDBOX-REED-SUITE.md`, item 1 gains two checks:
  - `command -v lyx` (`where lyx` on Windows) run inside a spawned strand pane names the spawning binary, not the production install.
  - `LYX_BIN` is set inside that pane and names the same binary.
  Follow them with one sentence stating that these are live-substrate facts no `go test` can prove, which is why they are pre-condition checks rather than assertions, and one sentence pointing at `SANDBOX-SHUTTLE-SUITE.md` for the agent-pane half — this suite's strands run operator-supplied commands and spawn no agent pane, so a `claude`-resolution check has nothing to exercise here.

  In `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`, item 1 gains the same two checks, run inside a spawned agent pane, plus a third: `claude` still resolves inside that pane.
  Give the third check its own sentence of rationale rather than listing it bare — `claude` is looked up by bare name from the pane's `PATH`, so it is the property most exposed to any future change in how a pane's shell is started, and it is the one property no hermetic test can prove.

  Do not add a `**Covers:**` tag, and do not move or reword any existing one — `cmd/lyx/sandbox_coverage_test.go` scans these files for those tags, and this card changes coverage for no module.
  Leave `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md` alone: it carries the same boilerplate PATH pre-condition line, but it drives the resize watch loop and spawns no agent pane, so these checks have nothing to exercise there.
  Both files use semantic line breaks, one sentence per line, per the `markdown-semantic-line-breaks` Shared Decision.
- **Commit:** `docs(sandbox): add pane binary-resolution checks to the reed and shuttle suites`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks && go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded` runs the only two automated guards that can see this batch's edits.

`TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`) enforces the `Markdown Link Integrity` invariant over `manifest/` and `docs/`, which is card 8's `docs/sandbox-howto.md` edit — every inline link's file part and `#anchor` must resolve.
`TestSandboxCoverage_AllModulesCoveredOrExcluded` (`cmd/lyx/sandbox_coverage_test.go`) enforces the `Sandbox Suite Coverage` invariant by scanning `tools/sandbox/*SUITE.md` for `**Covers:**` tags, which is the guard card 9's two suite-file edits could break by disturbing a tag line.

Both are named with `-run` rather than run as whole packages: `./cmd/lyx/` unscoped includes `TestCrossCompileLinux`, which builds the full cgo binary for another platform and is minutes of work this batch cannot affect.
`crucible/README.md` and the two `tools/sandbox/*SUITE.md` files are outside the link-integrity scan's `manifest/`-and-`docs/` roots, so their prose has no automated assertion;
that is a property of the existing guard's scope, not a gap this batch introduces, and the task-wide `pipeline.done_gate` re-runs both guards alongside everything else before the task is marked done.
