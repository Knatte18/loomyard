MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-16
```

## Findings

### [BLOCKING] Keepalive regression test is unreachable by verify
**Location:** Batch 4, card 20 (and Shared Decision `go-verify-scoping-and-tiers`)
**Issue:** `internal/muxengine/contract_integration_test.go` begins `//go:build integration`, but the plan repeatedly calls it "untagged real-tmux" and batch 4's `verify: go test ./internal/muxengine/... ./internal/muxcli/...` omits `-tags integration`, so the flipped `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds` (the core header-keepalive regression) never compiles or runs under any batch verify.
**Fix:** Run the integration file in batch 4's verify (add `-tags integration`, ensuring the package's hermetic `TestMain` still applies) or name an explicit integration run; correct the "untagged" claim in the decision and Batch Tests to match the file's real build tag.

### [MEDIUM] Folding HeaderPaneID into boundPaneIDs flips reaping semantics
**Location:** Batch 4, card 18 (`planReconcile`, reconcile.go:93-113)
**Issue:** `boundPaneIDs` both gates `anyBoundPresent` and defines the reap-exemption; adding `st.HeaderPaneID` to it makes `anyBoundPresent` true whenever the header is live, so with zero strands + header + operator/foreign panes, reconcile now reaps those foreign panes it previously left untouched (while apply still self-skips via `anyPlacedStrand`), contradicting the documented "no bound content ⇒ foreign panes untouched" invariant.
**Fix:** Exempt the header via a separate set used only in the reap-skip test; keep `anyBoundPresent` computed from real strand bindings only.

### [LOW] Card 13 omits lock.go from Context
**Location:** Batch 3, card 13
**Issue:** Requirements build an `Engine` via `muxengine.New(cfg, layout)`, but `New`/`type Engine` live in `internal/muxengine/lock.go`, which is not in the card's `Context:` (card 10, which also touches the Engine, does list it).
**Fix:** Add `internal/muxengine/lock.go` to card 13's `Context:`.

### [LOW] Resume boot path skips eager header validation
**Location:** Batch 4, card 17
**Issue:** Header creation is placed in `ensureServerAndSessionLocked`, which both `Up` (lifecycle.go:390) and `Resume` (lifecycle.go:442) call, but `ValidateHeader()` is invoked only "at the start of `Up`"; a bad template on a resume-after-crash therefore is not caught loudly pre-boot as the `eager-header-validation` decision requires (it degrades to the print-and-block fallback).
**Fix:** Validate on the shared boot path (or add the same eager call in `Resume`).

### [NIT] Card 17 references clearAllPaneBindings without its file in Context
**Location:** Batch 4, card 17
**Issue:** Requirements name `clearAllPaneBindings` as the reboot landmark that must also clear `HeaderPaneID`, but that function is defined in `reconcile.go`, which is not in the card's `Context:`/`Edits:` (the actual edit lands in lifecycle.go's `if booted` block).
**Fix:** Add `internal/muxengine/reconcile.go` to Context, or state the clear happens in lifecycle.go's booted block.

## Verdict

REQUEST_CHANGES
The keepalive regression test is gated behind an unrun build tag; fix verify reachability plus the reconcile exemption.
MILL_REVIEW_END
