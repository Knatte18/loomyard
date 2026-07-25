# Batch: suite-docs-sweep

```yaml
task: dev/test lyx.exe separated from production deploy
batch: suite-docs-sweep
number: 6
cards: 3
verify: null
depends-on: [4]
```

## Batch Scope

Retargets the remaining deploy-instruction surface: the seven embedded `SANDBOX-*-SUITE.md`
deploy lines and the three `docs/` sandbox references. All are consumed alongside the sandbox
suite, which resolves `.dev-bin` and prepends it to the agent's PATH — so the operator-facing
instruction is simply `deploy-dev` first, with the fingerprint `Source: dev` marker as
confirmation. Pure Markdown edits — `verify: null`. Depends on batch 4 (`deploy-dev` exists) and
runs in parallel with batch 5 (disjoint files).

## Cards

### Card 19: Retarget the SANDBOX-*-SUITE.md deploy lines

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-MUX-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `tools/sandbox/SANDBOX-PERCH-SUITE.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the identical prerequisite line `1. **Deploy a fresh
  binary.** Run `deploy.cmd` so `lyx.exe` on PATH is current source.` to run `deploy-dev`
  instead, and reword to reflect that the dev binary lives in `.dev-bin` and the suite resolves
  it and puts it on the agent's PATH (the fingerprint header's `Source: dev` line confirms the
  dev build under test). Do NOT change the black-box contract lines ("tests `lyx.exe` as a black
  box — exactly as a real user with only the binary on PATH") or the scenario `lyx <subcommand>`
  command bodies — from the agent's perspective bare `lyx` still resolves (to the dev build, via
  the suite's PATH prepend). Only the operator-facing deploy prerequisite line changes.
- **Commit:** `docs(sandbox): retarget SUITE.md deploy lines to deploy-dev`

### Card 20: Rewrite the sandbox-howto deploy flow

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/sandbox-howto.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the operator runbook for the dev flow. The "Deploy a fresh
  `lyx.exe`" step (~54) runs `deploy-dev` (building into `.dev-bin`) rather than `deploy.cmd`;
  explain that this leaves the production `lyx` untouched and that the suite fingerprints and
  runs the `.dev-bin` binary. Update the launcher list (~8–13) to include `deploy-dev`. Update
  the prerequisite about `C:\Code\tools\bin` being on PATH (~41) to clarify that PATH placement
  is for the production binary only — the suite resolves `.dev-bin` itself and does not require
  it on PATH. Update the "What the suite does" narrative (~24–33) and the troubleshooting row
  for a stale/`lyx`-not-found binary (~201) to reference `deploy-dev` and the `Source: dev`
  fingerprint marker as the way to confirm the correct build. Keep the verification-of-deployed-
  binary guidance, retargeted to the dev build.
- **Commit:** `docs(sandbox-howto): retarget runbook to deploy-dev flow`

### Card 21: Retarget sandbox-hub and overview references

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/sandbox-hub.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/sandbox-hub.md`, retarget the deploy references (~8, 44, 81, 86,
  97, 179): `lyx` is deployed via `deploy-dev` into `.dev-bin` (not `deploy.cmd`), the sandbox
  tool resolves that dev binary rather than requiring `lyx` on the system PATH, and the
  fingerprint records the `Source: dev` marker. In `docs/overview.md`, update the sandbox Hub
  paragraph (~439) so its "exercises the real deployed `lyx` binary … Build it via
  `sandbox-build.cmd` once `lyx` is deployed" wording reflects the dev split (`deploy-dev` →
  `.dev-bin`, resolved by the suite). Do not restructure the module table or unrelated sections.
- **Commit:** `docs(sandbox): retarget hub+overview deploy references to dev flow`

## Batch Tests

`verify: null` — Markdown-only edits with no runnable surface. Correctness is by review: the
seven SUITE.md deploy lines and all three `docs/` sandbox references target `deploy-dev` /
`.dev-bin`, with the `Source: dev` fingerprint marker cited as the confirmation mechanism.
