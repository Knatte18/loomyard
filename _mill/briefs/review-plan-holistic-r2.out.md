MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:consistency] Card 15 keeps/rewrites a design doc the Documentation Lifecycle says to delete
**Location:** batch 2 / card 15 **Issue:** Card 15 rewrites `manifest/designs/reed-born-as-strand.md` to "describe what shipped" and has the new Done entry in `manifest/roadmap.md` keep a link to it, but `docs/overview.md`'s Documentation Lifecycle section (verified) states a `manifest/designs/<module>.md` doc "is deleted when their module lands — the implementation and tests become the source of truth," and `manifest/roadmap.md`'s own maintenance rules (verified, "Delete that doc once the module ships … a Done entry instead points at the module's own package documentation") say the same thing. Card 15 explicitly says to follow those maintenance rules while doing the opposite of what they require for the design doc. **Fix:** Either delete `reed-born-as-strand.md` and point the Done entry at the relevant package doc (`internal/reedengine`/`internal/loomcli`), or state explicitly why this item is an exception to the lifecycle rule.

### [BLOCKING:scope] docs/overview.md's second four-step restatement is left stale
**Location:** batch 2 / cards 10, 12 **Issue:** `docs/overview.md` line 330 (the "four-step sentence") is the only overview.md target either card edits, but line ~420's separate "the bootstrap —" bullet under "Execution stack (orchestration layers)" (verified) independently restates the same four steps (tmux session, status strand, detached driver, terminal attach) with no mention of the operator strand or watchdog. This is the identical defect class the plan's own `docs-land-with-the-behaviour-card-that-makes-them-true` Shared Decision says it already found once (in `start.go`'s `Long`, "found in a second file" at line 330) — but a third occurrence in that same second file was missed. **Fix:** Add the operator-strand and watchdog-spawn mentions to the "Execution stack" bullet too, in the same cards.

### [BLOCKING:design] The watchdog-fires-under-no-attach property is never actually proven
**Location:** batch 2 / card 13 (and 14/15) **Issue:** Card 13 asks a Tier-1 test to assert the watchdog call "fires on a `--no-attach` invocation" — the property the card itself calls "the single thing a later edit is most likely to get wrong." But `loomCLI.reed` is a concrete `*reedengine.Engine` (verified, `internal/loomcli/cli.go`), and `ensureStatusStrand()` (which must return nil before the watchdog call is reached) calls `c.reed.Up()`/`c.reed.Status()` (verified, `sharedbootstrap.go`), which cannot run offline. Card 13's own escape hatch ("assert over the smallest seam … say which half the smoke tier covers instead") is never backed by any smoke card: card 14's only `--no-attach` case covers the operator strand, not the watchdog. **Fix:** Add a smoke-tier assertion (card 14 or 15) that `loom start --no-attach` still spawns the watchdog daemon, or extract a positional seam the Tier-1 test can actually exercise.

### [BLOCKING:scope] Card 14's Selvage-pane-id assertion needs identifiers outside its Context
**Location:** batch 2 / card 14 **Issue:** The fifth assertion ("asserting reed's persisted Selvage pane id is unchanged") requires `reedengine.LoadState` and `ReedState.SelvagePaneID`, both declared in `internal/reedengine/state.go` (verified) — not listed in card 14's `Context:` (only `strand.go`). Neither the smoke fixtures in `smoke_test.go`/`smoke_bootstrapwiring_test.go` (Context, verified) demonstrate this construction either. **Fix:** Add `internal/reedengine/state.go` to Context, or name the exact accessor the implementer should call.

### [BLOCKING:consistency] "Permanent full share" misattributed to the status strand's Decision
**Location:** batch 2 / card 10 (`manifest/designs/loom.md` edit) **Issue:** Card 10 asks the doc to say the operator pane "takes a permanent full share of window rows exactly as the status strand does under that file's own childless-full-height-is-acceptable Decision." Verified against `render/types.go`/`policy.go`: the status strand has `ShrinkWhenWaitingOnChild:true` and explicitly collapses "once a forked child exists" (loom.md's own step-2 text), i.e. its full-height is conditional/temporary, while the operator strand's builder (card 8) sets `ShrinkWhenWaitingOnChild:false`, an unconditional/permanent property the cited Decision does not establish. **Fix:** Describe the operator's full-height as its own (stronger) property rather than "exactly as the status strand does."

## Verdict

REQUEST_CHANGES
Doc-lifecycle contradiction, an unproven load-bearing behavior, and two context/consistency gaps need a fix.
MILL_REVIEW_END
