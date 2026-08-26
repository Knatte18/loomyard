MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge

```yaml
duration_s: 135.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:scope] Discussion stencil's `_lyx/loom/` fence is missed
**Demoted-from:** BLOCKING
**Section:** Scope (enumeration bullets) / Testing
**Issue:** `contracts/stencils/loom/loom-template-discussion.md:145` is live prompt text fencing the Discussion agent off `**_lyx/loom/** — the phase machine's status file`; after the move that directory no longer exists and the real status dir `.lyx/loom/` is left unfenced, and `contracts/stencils/discussiontemplate_test.go:32` pins the exact literal `` `_lyx/loom/` `` so editing the stencil fails a Tier-1 test the Testing section never names.
**Fix:** Add both the stencil line (retarget the fence to `.lyx/loom/`) and its pinning assertion in `contracts/stencils/discussiontemplate_test.go` to Scope and Testing, beside the webster-review rubric bullet that already handles the other behavioral stencil.

### [BLOCKING:design] Pass-1 grep literal is one segment too long
**Section:** Scope, "Enumerate the remaining references by full-text grep … for the literal `_lyx/loom/status.json`"
**Issue:** The stated literal cannot match a reference that names the directory only (`_lyx/loom/`), which is exactly how the missed discussion-stencil fence and its pinning test spell it, so the document's own "primary method" is provably not closed over the class it was introduced to cover.
**Fix:** State the pass as a tree-wide grep for the shorter literal `_lyx/loom` (directory and file forms both), which subsumes the current one and reaches both missed hits.

### [NIT:scope] `docs/overview.md:312` falsified and unreachable by all four passes
**Demoted-from:** BLOCKING
**Section:** Scope / "Docs that make claims this change falsifies"
**Issue:** `docs/overview.md:312` says `lyx loom run` will "seed+commit the status file weft-side when it is absent" — false after this change — and no stated pass reaches it: it carries no `_lyx/loom/status.json` literal, neither identifier, none of the `durable`/`tracked`/`weft-synced` words ("weft-side"), and pass 4's `_lyx` scan is scoped to `loom.md`, `shed.md`, and `internal/shedengine/*.go` only.
**Fix:** Add `docs/overview.md:312` to the falsified-docs/Scope list, and widen pass 4's file set beyond `manifest/` to include `docs/`, `contracts/`, and `tools/`.

## Verdict

REQUEST_CHANGES
Two live-prompt/doc references and one Tier-1 test escape the stated enumeration passes.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
