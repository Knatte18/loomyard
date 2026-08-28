MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize

```yaml
duration_s: 185.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic); reported model id claude-opus-5 — best-effort self-assessment
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:consistency] Link-integrity claim overstates the invariant's reach
**Section:** Constraints (Markdown Link Integrity bullet) **Issue:** The bullet requires links in the new `contracts/specs/final-summary-spec.md` to resolve, but `internal/lyxcwd/docslink_test.go` and CONSTRAINTS.md scan only `manifest/` and `docs/` — `contracts/specs` is a target, never a scan source. **Fix:** Restate the bullet as applying to `docs/overview.md` (enforced) with correct links in the spec files as discipline, not invariant coverage.

### [NIT:consistency] Retained webster-spec sentence names the wrong consumer
**Section:** spec-file-split **Issue:** `contracts/specs/webster-spec.md:43` — kept as a writer-side addition — says "because Finalize dumps `summary.md` verbatim into the PR body"; the PR body is `Publish`'s (`publish.go:169-182`, title and body as separate fields), and this task exists to pin the Publish/Finalize split. **Fix:** Say the retained sentence is corrected to name `Publish` and drop "verbatim" when the section is reduced.

### [NIT:scope] Kept-specs enumeration at overview.md:98 is already stale
**Section:** Scope (docs/overview.md bullet) **Issue:** The task adds `final-summary-spec.md` to the `docs/overview.md:98` list, but `contracts/specs/` also holds `loom-plan-spec.md`, absent from that list and linked at `docs/overview.md:304`. **Fix:** State whether the edit also repairs the omission or deliberately leaves the pre-existing drift alone.

## Verdict

APPROVE
Decisions complete and source-accurate; three documentation-level NITs, none blocking plan writing.
MILL_REVIEW_END
