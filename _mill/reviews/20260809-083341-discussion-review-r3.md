MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Old-order two-arg clone silently builds a reversed hub
**Section:** clone-argument-surface / conflict-rule
**Issue:** After the flip, the pre-change invocation `lyx fabric clone <warp-url> <weft-url>` still has valid arity, so the probe reads the *warp* repo, finds no `.lyx-warp`, and the bootstrap row fires: the user's real repo is cloned as the weft, `_board` is materialized on it, and `internal/fabriccli/clone.go:60-66` commits `.lyx-anchor`/`fabric.yaml`/`.lyx-warp` and **pushes** them to the user's repo's default branch. The discussion declares the flip "a deliberate breaking change" but never gives this footgun a disposition.
**Fix:** State a decision — either an explicit pre-bootstrap sanity guard on the weft candidate (e.g. refuse to bootstrap a weft whose HEAD commit carries neither `.lyx-anchor` nor an empty/unborn history, or a confirm flag), or an explicit "accepted hazard, documented in `Long`" ruling.

### [NIT:scope] Second sandbox clone call site omitted from the inventory
**Demoted-from:** BLOCKING
**Section:** Technical context → "Call sites to update"
**Issue:** The list names only `tools/sandbox/main.go:67` (`fabricCloneRun`), but `tools/sandbox/main.go:48` (`cloneRun`, the shared `lyx-test-HUB` launcher, `exec.Command(lyxPath, "fabric", "clone", warpURL, weftURL)`) is a second warp-first call site. Argument-order flips are silent at compile time, so the inventory is the only safety net; missing this one leaves the core sandbox launcher cloning the warp URL as the weft — the exact scenario of the finding above, against `Knatte18/lyx-test`.
**Fix:** Note that both `cloneRun` and `fabricCloneRun` in `tools/sandbox/main.go` flip, and state the enumeration rule as "every string-literal `"fabric", "clone"` occurrence", since these are invisible to the compiler.

### [NIT:consistency] Slice 9's "slice 10 still pending" line goes stale
**Section:** Technical context → "Docs that must change"
**Issue:** The doc edit is scoped to `manifest/designs/fabric-unified-view.md` lines 149-169, but line 147 (inside the slice 9 section) reads "slice 10 is still pending and still collides on `runCloneWithReset`", which this task falsifies.
**Fix:** Add line 146-147 to the same-commit doc edit list.

## Verdict

REQUEST_CHANGES
Flip hazard needs a disposition; one sandbox clone call site is missing.
MILL_REVIEW_END
