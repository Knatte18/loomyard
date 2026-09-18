MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514 (best-effort; presented to me as Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Warm attach silently loses envelope failure modes
**Section:** `ensure-session-is-the-seam`, Scope "Out" ("The warm paths do not change")
**Issue:** Today's warm attach pre-flight `Status()` reaches `loadOrInitStateLocked` (`spawn.go:195`) → `LoadState` + `adoptPaneGenerationLocked` → `refuseLiveForeignSessionLocked` (`generation.go:142`), so a corrupt `reed.json` or a bindings-vs-live generation collision aborts on the JSON envelope; `EnsureSession`'s early return reads no state at all, and `AttachArgv`'s own `loadOrInitStateLocked` error only degrades to the bare argv (`attach.go:111-115`, "returns no error, by contract") — so warm attach now proceeds where it used to refuse.
**Fix:** Add a decision dispositioning warm-attach's dropped corrupt-state and live-foreign-generation refusals (accept the loosening, or keep a state read in the pre-flight), and correct the "warm paths do not change" claim, which is false for attach as written.

### [BLOCKING:scope] Regression surface scoped to two packages, but tier-1 tests outside them depend on AddStrand refusing
**Section:** Testing → "Regression surface"
**Issue:** The surface is scoped to `internal/reedengine`/`internal/reedcli`, yet `internal/burlercli/wiring_test.go:356` (untagged, tier 1) drives a real `reedengine.AddStrand` and states its premise explicitly — "It reaches no live reed session (none was ever started, so `requireSessionLocked` fails fast) and spawns no process". After the change that test boots a real tmux server in an untagged file, which is both a behaviour change and a Test Tier Purity Invariant breach.
**Fix:** Widen the regression surface to "every test, in any package, that relies on `AddStrand` failing fast without spawning", and name the enumeration method (grep for `AddStrand` in `_test.go` outside reed), plus the disposition for each hit.

### [NIT:consistency] Stale `Status()` → `Up()` wording in Constraints
**Section:** Constraints → CLI / Cobra Invariant
**Issue:** Reads "Swapping `Status()` for `Up()`", superseded in r3 by `EnsureSession()`.
**Fix:** Re-word to the `EnsureSession()` seam.

### [NIT:consistency] "Exactly two cheap tmux round trips more than today" is wrong for attach
**Section:** `ensure-session-is-the-seam` rationale
**Issue:** Warm `add` gains one round trip (`list-panes`; `requireSessionLocked` already does `has-session`, `lifecycle.go:1135`), and warm `attach` gets strictly *cheaper* than `Status()`, not more expensive.
**Fix:** State the cost per verb rather than one shared "+2".

### [NIT:design] Boot-attribution test has no stated observation mechanism
**Section:** Testing → "Boot attribution"
**Issue:** The test asserts an `Info` line appears/does not appear, but the discussion never says how a `smoke` test reads `internal/logger` output.
**Fix:** Name the sink/capture mechanism, or drop the assertion to the `booted` return value the CLI already sees.

## Verdict

REQUEST_CHANGES
Warm-attach refusals dropped undispositioned; regression surface misses a tier-1 test outside reed.
MILL_REVIEW_END
