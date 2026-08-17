MILL_REVIEW_BEGIN
# Review: shuttleengine + reedengine + tokenvocab told-geometry

```yaml
duration_s: 199.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build self-reported as claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] Scope says eleven test files, lists ten
**Section:** Scope → In, "Every out-of-package caller…"
**Issue:** The sentence claims "eleven test files" but the enumeration names ten (`shuttlecli/cli_test.go`, `shuttlecli/smoke_interrupt_test.go`, `webstercli/verbs_test.go`, `websterengine/recoverbatch_test.go`, `treadleengine/smoke_judge_test.go`, `burlerengine/smoke_cluster_test.go`, `burlerengine/smoke_round_test.go`, `fabricengine/hubscratch_test.go`, `reedcli/smoke_debuglog_test.go`, `cmd/lyx/constructoranchoring_test.go`); grep over the four symbols confirms ten is the complete set, so only the count word is wrong.
**Fix:** Change "eleven" to "ten", or say "the files listed below" and drop the count.

### [NIT:consistency] hubgeom filed under docs/shared-libs without a classification
**Section:** Scope → Docs; Q&A "Which docs land in this commit"
**Issue:** `docs/shared-libs/README.md` states its own admission line — "a shared lib does one mechanical thing … carries no domain logic" — and every current entry is a low-level leaf, whereas `hubgeom` sits *above* `reedengine`/`fabricengine` and imports both; `docs/overview.md:314`'s shared-infrastructure sentence is the same list.
**Fix:** State explicitly whether `hubgeom` is documented as shared infrastructure (and why that does not contradict the shared-libs line) or only as a package-tree entry in `docs/overview.md`.

### [NIT:design] Trust rule stated for SocketKey only, not the other six fields
**Section:** Decisions → "SocketKey is trusted, not validated"
**Issue:** The decision covers `SocketKey`; the same question for `LogsDir`, `AnchorPath` and `WorktreeRoot` is left implicit — an empty `LogsDir` reaches `os.MkdirAll("")` at `lifecycle.go:257` and an empty `AnchorPath` makes `stateDir()` relative, both from any non-`hubgeom` caller.
**Fix:** Restate the decision as "`Geometry` is trusted verbatim, all seven fields, validated by nobody", so T7/T8 inherit an explicit rule rather than an inference from one field.

## Verdict

APPROVE
Scope, call-site enumeration, and constraint claims verified accurate against source; three NITs only.
MILL_REVIEW_END
