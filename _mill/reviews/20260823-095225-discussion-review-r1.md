MILL_REVIEW_BEGIN
# Review: loom: self-checkable mechanical gates

```yaml
duration_s: 210.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude, Opus-class (self-reports as claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [BLOCKING:design] Findings/error ordering breaks "contract unchanged"
**Section:** `findings-not-bool` + `producer-contract-unchanged`
**Issue:** `discussionvalidate.go:68-81` short-circuits: a *missing* support log returns `Stuck` before the decision record is ever read, so a decision-record I/O fault behind a missing support log yields `Stuck` today; a `Validate` that accumulates findings across both files would surface that fault as an error, flipping `Stuck` → returned error (persist-blocked → persist-failed, run aborts). "The two are never both non-zero" does not say which wins.
**Fix:** State whether `Validate` short-circuits in the current stat-then-read order or accumulates, and which of a finding-on-A vs an error-on-B wins, so "outward contract exactly unchanged" is a checkable claim.

### [BLOCKING:design] CLI-verb tests cannot be tier 1 as described
**Section:** `## Testing` → `internal/loomcli` (new) + Parity tests; `## Constraints` → Test Tier Purity
**Issue:** The claim that verb tests reach the tree via `RunCLIIn` "the pattern `cli_test.go` already uses" is false for a real verb: the only two existing `RunCLIIn` calls (`cli_test.go:76,90`) are the `cmd.Name()=="loom"` group-guard path that skips resolution entirely. A real verb runs `lyxcwd.Resolve` (spawns `git rev-parse`) then `wire`, whose `loomengine.LoadConfig` is strict — so end-to-end envelope tests and the CLI half of the parity tests need a real repo/hub fixture and a build tag, not tier 1.
**Fix:** Name the actual mechanism — hub fixture + `//go:build integration`, or drive the leaf `*cobra.Command` against a hand-populated `loomCLI` receiver as `TestVerbRefusals` already does — and say which half of each parity test runs where.

### [NIT:consistency] Producer Pointer-Rule Invariant mis-cited
**Demoted-from:** BLOCKING
**Section:** `producer-contract-unchanged` rationale; `## Constraints`; Q&A #4
**Issue:** That invariant binds *instruction files* (prompts/skills) against paraphrasing another producer's format contract, and explicitly "not Go source". It has nothing to do with `shedengine.OutputPointer`, so "widening the pointer would change a contract governed by the Producer Pointer-Rule Invariant" is a false premise under a decision; the `requiredDiscussionSections`-comment citation is the same misread. (`Never Force-Add Invariant` in the same list is likewise unengaged — no fixture here touches git.)
**Fix:** Drop both citations and rest `producer-contract-unchanged` on its true grounds (no consumer asks for it; the roadmap item is about callability), and drop the two unengaged constraint entries.

### [NIT:scope] cmd/lyx test-update list is overstated
**Section:** `## Technical context` → Registration tests; `## Testing` → `cmd/lyx`
**Issue:** `longlist_test.go` derives from `root.Commands()` at module granularity and `registration_test.go` is an AST scan for packages exposing `Command()` — neither is touched by two new loom verbs; `drift_test.go` walks the live tree and needs no edit either. Only `helptree_test.go`'s pinned per-module `wantSubs` list for `loom` would gain them (and its superset check would not fail without it).
**Fix:** Narrow the list to `helptree_test.go` and say the addition there is deliberate coverage, not a forced update.

### [NIT:consistency] New invariant omits the leaf import cap
**Section:** `new-constraints-invariant`
**Issue:** The proposed Discussionparser Sole-Parser Invariant states sole-parser, no-path-declaration, and same-function-for-gate-and-verb — but not the stdlib-only import cap that `leaf_enforcement_test.go` will enforce. Every one of the repo's eight `leaf_enforcement_test.go` files is backed by a CONSTRAINTS section naming its allowlist.
**Fix:** Add the stdlib-only cap and its enforcing test name to the new section, so the machine check has a stated rule.

## Verdict

REQUEST_CHANGES
Ordering semantics, test feasibility, and a mis-cited invariant need resolving before plan writing.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
