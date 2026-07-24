# dev/test `lyx.exe` separated from production deploy

> **Status: Design — not built.** Not derived from the vacation design-discussion issues — this
> is a direct operational requirement. Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), once built this becomes a new
> invariant in `CONSTRAINTS.md` (matching the existing pattern of `Sandbox Suite Coverage` /
> `Test Tier Purity Invariant` / `Hermetic Git Test Environment Invariant`), and this file is
> deleted.

## The problem, confirmed against the current codebase

Today there is **exactly one** deploy target. `deploy.cmd` builds `lyx` from the current
checkout and installs it to `C:\Code\tools\bin` (on PATH) — this is both "the binary an operator
actually uses day to day" and "the binary every test/review flow exercises." There is no
separation.

- `tools/sandbox/suite.go`'s `runSuite` resolves `lyx` via `lookPath("lyx")` and explicitly
  documents: *"the fingerprint captures the exact binary the operator has deployed; the binary
  must be on PATH before running the suite."*
- Every prompt in `crucible/*.md` (`orchestrator-prompt.md`, `review-prompt-template.md`,
  `builder-review-prompt.md`, `webster-review-prompt.md`, `mux-review-prompt.md`) calls this out
  as a named footgun: *"live driving runs the DEPLOYED binary, not your working tree — re-run
  `deploy.cmd` after EVERY source change or you validate a stale binary and draw a false
  PASS/FAIL."*
- `docs/sandbox-howto.md` step 2 is literally "deploy a fresh `lyx.exe`," overwriting whatever
  was there.

**Consequence:** any review/sandbox/test run that deploys before it finishes validating can
clobber the stable, working production binary with an in-progress "test variant" — a test run
that never finishes, or fails partway, leaves production pointing at unvetted code.

## What needs to change

1. **A second, dev-only deploy target**, distinct from the production install location — e.g. a
   separate directory the review/sandbox tooling puts first on PATH (or resolves explicitly),
   never the same path `deploy.cmd` installs to for real use.
2. **`tools/sandbox/suite.go`'s binary resolution** must stop doing a bare `lookPath("lyx")`
   against whatever the ambient PATH happens to resolve first, and instead resolve the dev/test
   binary specifically — either a dedicated env var, a `-dest`-equivalent flag threaded through
   the suite launchers, or a fixed dev-bin directory checked before falling back to PATH.
3. **Every `crucible/*.md` prompt and `docs/sandbox-howto.md`** updated so their "deploy
   before testing" instructions target the dev location, not the production one.
4. **`deploy.cmd`/`tools/deploy`** likely needs a dev-mode flag or a sibling launcher (e.g.
   `deploy-dev.cmd`) rather than requiring every test flow to pass `-dest` by hand each time.

## Why this matters enough to be its own item

Every existing review/sandbox prompt already carries the "deploy-first footgun" warning as
documentation — meaning the current design is aware of the risk but mitigates it only by asking
operators to be careful and redeploy often, not by structurally preventing production and
test/review binaries from ever being the same file. A structural fix (two targets, tooling
resolves the dev one) removes the risk instead of relying on discipline.

## Open questions

- Exact dev install path/convention (a fixed directory, or derived from the same `-dest` flag
  `tools/deploy` already supports, just pointed elsewhere by default for test flows).
- Whether the CLAUDE.md-documented `deploy.cmd`/`tools/deploy` Linux equivalent
  (`go run ./tools/deploy -dest /home/knatte/.local/bin`, per `crucible/webster-review-prompt.md`)
  needs the same dev/prod split, or whether Linux review runs are lower-risk (no evidence either
  way yet — worth checking before assuming parity is required).
- Whether the sandbox suite's binary fingerprint (`binaryFingerprint(lyxPath)`) needs a visible
  marker distinguishing "dev build" from "prod build" in its output, beyond just resolving a
  different path.

## Related

- `tools/sandbox/suite.go` — `runSuite`'s current `lookPath("lyx")` resolution.
- `deploy.cmd` / `tools/deploy/main.go` — the current single-target deploy tool.
- `docs/sandbox-howto.md`, `crucible/*.md` — the prompts/runbooks that need updating once
  this lands.
