MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] reedengine → fabricengine edge is a test-build cycle
**Section:** `### hub-scratch-constructor` / Testing (`internal/fabricengine`)
**Issue:** The decision asserts "`fabricengine` does not import `reedengine`, and nothing in `fabricengine`'s dependency set does" — but `internal/fabricengine/clone_test.go` is `package fabricengine` (line 4) and imports `reedengine` (line 15, used at line 162); adding a production `reedengine → fabricengine` import makes the fabricengine test binary an import cycle, and the discussion explicitly says to KEEP that test (`clone_test.go:155`) rather than delete it.
**Fix:** State the disposition of `clone_test.go`'s in-package reedengine import (move the idempotency test to `package fabricengine_test`, or place `HubScratchDir` where no cycle arises) as part of the decision, not left to the build.

### [BLOCKING:scope] Sandbox suite scenarios encode the `_board` junction
**Section:** Testing → **Sandbox** / Scope **In**
**Issue:** `tools/sandbox/SANDBOX-FABRIC-SUITE.md:237` (F8) requires `_lyx`, `.lyx` and `_board` to "land as links inside `<warp>/<dir>/`", and `:364` (F15) requires `/_board` present "exactly once" in the warp `.git/info/exclude`; both become false after the deletion, yet the scope list names no suite-doc edit and Testing only says the suites "must pass".
**Fix:** Put the affected `tools/sandbox/SANDBOX-FABRIC-SUITE.md` scenarios (F8, F15, and the F13 wording at `:254`) explicitly in scope with their new expectations.

### [BLOCKING:scope] reed CLI help text still names `<hub>/.lyx/logs/`
**Section:** Scope **In** / Testing (`internal/reedengine`)
**Issue:** `internal/reedcli/up.go:33` is user-visible cobra help reading "logging to `<hub>/.lyx/logs/`"; the move makes it wrong, and nothing in scope, testing, or the constraints list (which cites the CLI/Cobra help-accuracy obligation only for the envelope key) covers it.
**Fix:** Add the reedcli help-string update to scope, and name any other prose naming the old hub path (`internal/reedengine/lifecycle.go:29-33` doc comment).

### [BLOCKING:decision] Stale `_board` line in existing warp `.git/info/exclude`
**Section:** `### no-migration` / `### board-junction-deleted`
**Issue:** `wireBoardLink` (`junction.go:441`) seeds `_board` into the warp `.git/info/exclude` unconditionally on clone/add/reconcile and `unwireBoardLink` (`unwire.go:168`) is the only unseeder; deleting both leaves that repo-wide line permanently in any hub already reconciled by a current binary, silently git-ignoring an operator's own future `_board` path. The no-migration decision disposes of the old directory and the junctions, never the exclude entry.
**Fix:** State a disposition for the leftover exclude line (accept and document, or hand-remove instruction), since the "verified zero junctions on disk" evidence does not cover it.

### [NIT:scope] "Complete" junction surface omits one CLI test
**Section:** `### The `_board` junction surface, complete` → Tests
**Issue:** `internal/fabriccli/cli_test.go:768-791` is a standalone test asserting the `board_junction_removed` key is present and true; the inventory names only `cli_test.go:887-896` and the envelope-contract test.
**Fix:** Add that case to the deletion inventory so the "complete" claim holds.

## Verdict

REQUEST_CHANGES
Import-cycle premise, sandbox/help surfaces, and stale exclude entry need resolution first.
MILL_REVIEW_END
