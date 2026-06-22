The enforcement test scans the whole tree and bans `os.Getwd` / `--show-toplevel`. The fslink design uses neither, so the path-invariant-clean decision holds. Good.

I have enough to produce the review. Let me summarize my findings:

1. **Card 1 Context gap** (minor): references `junction_windows.go` as a port source for `prepareLink` but it's not in Context. Mitigated because logic is identical to `junction_other.go` (which IS in Context) and fully reproduced in requirements. NIT.

2. **seedLyxJunction edge-case reordering** (card 7): plan checks `IsLink` before "missing target", original checks "missing target" first. Divergent only for the untested case (real dir + missing target). NIT.

The plan is otherwise sound: DAG valid, numbering sequential, all cards complete, decisions implemented, messages preserved, tests covered. This is an APPROVE with two NITs.

MILL_REVIEW_BEGIN
# Review: Extract internal/fslink cross-OS link primitive — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-22
```

## Findings

### [NIT] Card 1 Context omits junction_windows.go
**Location:** Batch 1, Card 1
**Issue:** Requirements line 51 says `prepareLink` is "ported from `junction_other.go`/`junction_windows.go`", but Card 1 Context lists only `junction_other.go`.
**Fix:** Add `internal/worktree/junction_windows.go` to Card 1 Context (low impact — the two files' guard logic is identical and the unified message is spelled out inline, so no cold-start exploration is actually forced).

### [NIT] seedLyxJunction edge-case check order changes
**Location:** Batch 2, Card 7
**Issue:** Original `seedLyxJunction` tests "missing target" (errTarget) before the not-a-link branch; the card checks `fslink.IsLink` first. For the untested case real-dir-link + absent-target this flips the message from "weft _lyx directory does not exist…" to "host repo already contains a real _lyx…".
**Fix:** None required for the tested paths (`TestWeftHostPristineEnforced` passes either order since its weft target exists); optionally mirror the original order to keep the edge message byte-identical per `preserve-behaviour-and-messages`.

## Verdict

APPROVE
Plan is complete, sequenced, decisions implemented; two cosmetic nits only.
MILL_REVIEW_END
