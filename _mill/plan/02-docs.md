# Batch: docs

```yaml
task: 'Sandbox suite: emit findings JSON on the shared analysis contract'
batch: docs
number: 2
cards: 2
verify: null
depends-on: [1]
```

## Batch Scope

This batch updates the two sandbox docs to describe the new emit→fetch→triage flow and to
fix the pre-existing `tools/sandbox/test-scheme.md` doc-rot (the real file is
`SANDBOX-SUITE.md`). It is a pure-documentation batch with no runnable surface, so `verify:
null`. It depends on batch 1 so the docs describe the behaviour exactly as shipped (the
`-loomyard` launcher flag, the `.scratch/sandbox-report-<fingerprint>.json` destination, the
removal of the `gh`/`selfreport` transport). No file overlap with batch 1.

## Cards

### Card 7: Rewrite docs/sandbox-howto.md for the JSON-report flow

- **Context:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Edits:**
  - `docs/sandbox-howto.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In "## What the suite does", replace "The agent files WARN/FAIL findings
  via `lyx selfreport create`." with prose that the agent writes WARN/FAIL findings to
  `sandbox-report.json` in the host repo and `sandbox.cmd suite` fetches a normalized copy to
  the loomyard repo's `.scratch/sandbox-report-<fingerprint>.json`. In "## Prerequisites
  (one-time)", delete item 1 (`gh` authenticated) and renumber the remaining items. Rewrite
  "### 5. Triage findings": the agent no longer files GitHub issues — describe the flow as
  suite emits `sandbox-report.json` → `suite.go` fetches it to
  `.scratch/sandbox-report-<fingerprint>.json` → run the report-to-tasks triage skill
  (millhouse#586) against that file. In "## Troubleshooting", delete the
  "agent cannot file findings / `gh` not authenticated / `gh auth login`" row. In "## See
  also", change the `tools/sandbox/test-scheme.md` link (URL and label) to
  `tools/sandbox/SANDBOX-SUITE.md` and update the description to "the embedded test scheme the
  agent follows". Leave the deploy/build steps and the exit-code caveat unchanged.
- **Commit:** `docs(sandbox): rewrite howto for the JSON-report flow`

### Card 8: Update docs/sandbox-hub.md for the JSON-report flow

- **Context:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Edits:**
  - `docs/sandbox-hub.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In "## Running the Suite Agent" → "### Prerequisites", delete the bullet
  "`gh` installed and authenticated (`gh auth status`). The `lyx selfreport create` command
  that the agent uses to file findings delegates to the `gh` CLI." In the same section's
  numbered "Usage" description and the paragraph that follows, replace the prose "Findings
  (WARN or FAIL verdicts) are filed directly from inside the host repo via `lyx selfreport
  create`." with prose that findings are written to `sandbox-report.json` and fetched by
  `suite.go` into the loomyard repo's `.scratch/` on the millhouse#586 contract; mention the
  `-loomyard` root the launcher supplies. In numbered step 3 of "### Usage" (and any other
  occurrence), change the `tools/sandbox/test-scheme.md` reference to
  `tools/sandbox/SANDBOX-SUITE.md` (the embedded template is `SANDBOX-SUITE.md` itself). Leave
  the Hub topology, build/reset, dedicated-use, and psmux-future sections unchanged.
- **Commit:** `docs(sandbox): update hub doc for the JSON-report flow`

## Batch Tests

`verify: null` — this batch edits only Markdown documentation (`docs/sandbox-howto.md`,
`docs/sandbox-hub.md`); there is no runnable surface to test. Correctness is a review concern
(prose matches the behaviour shipped in batch 1), enforced at plan/code review, not by a Go
test.
