MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] plan-format.md link repair has no stated target
**Section:** Decisions → `loom-md-links-fixed-prose-deferred` (the general link-repair rule); Testing → grep pattern 4
**Issue:** For `builder-contract.md` the repair target is named (`webster-contract.md`), but for the deleted `plan-format.md` no target is ever given, and several inbound links name v2 *as distinct from* v3 — verified: `docs/reference/model-spec.md:3` ("Pinned alongside [plan-format v2](plan-format.md) and the emerging [v3](plan-format-v3.md)"), `plan-format-v3.md:5` ("[plan-format.md v2](plan-format.md) stays live and valid"), `status-schema.md:3`, `discussion-format.md:3`, `loom.md:29`, `roadmap.md:211`, `review-finding-classification.md:7,:47`. Retargeting these at `plan-format-v3.md` makes each sentence false or self-duplicating, so "repair the link, touch nothing else on the line" is not executable at those sites.
**Fix:** State the disposition per shape — retarget, unlink to plain text, or (for the ones whose prose asserts v2 exists) name which of A/B/C rewords it — so the rule is mechanical rather than per-site judgment.

### [BLOCKING:consistency] Link repair contradicts shed-followups' deliberate dangling window
**Section:** Decisions → `shed-followups-inventory-repair`; Scope → Out ("plan-format-v3.md:5 prose is C's")
**Issue:** `manifest/designs/shed-followups.md:183–184` explicitly records that `docs/reference/plan-format.md` "does not exist at all" between task A and B and that "links to `plan-format.md` dangle in between, by design and briefly" — the opposite of this task's link-repair rule. The discussion lists only two overrides to record in `shed-followups.md` (phase enum, `builder-contract.md` deletion) plus `:165`/`:235`; this third divergence is unrecorded, as is the `roadmap.md:72` phase-word edit that `shed-followups.md:388`/`:393` assigns to task E as "its remaining roadmap obligation".
**Fix:** Either adopt `:183–184`'s dangling window for `plan-format.md` links or record the override — and add `:183–184` plus the `roadmap.md:68`/`:72` ownership note to the `shed-followups.md` edit list.

### [BLOCKING:consistency] Q&A log contradicts the weftgit fixture disposition
**Section:** Q&A log (last-but-four Q) vs Technical Context → Go sites → `internal/fabricengine/weftgit_exclude_test.go`
**Issue:** The Q&A answers "delete the `builder` fixture dirs rather than renaming them", but the Technical Context requires `:285` (`_lyx/<rel>/builder/state.json`) to be **renamed** to webster together with its assertion at `:302` (`durable := lyxRel + "/builder/state.json"`) — verified in source; deleting it strips the test's only durable positive control and leaves `TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth` asserting a file it never writes.
**Fix:** Correct the Q&A answer to the two-disposition split (delete `:279`/`:280`, rename `:285` + `:302`) so a plan writer reading the log alone cannot pick the wrong one.

### [NIT:consistency] "The only compile blocker" heading is false as written
**Section:** Technical Context → "The only compile blocker outside the two deleted packages"
**Issue:** It says every other cross-package reference "is a comment", yet `cmd/lyx/main.go:23`, `internal/configreg/configreg.go:10`, `cmd/lyx/notransients_test.go:21` and `cmd/lyx/constructoranchoring_test.go:34` all import the deleted packages (verified) and are listed as compile-affecting two lines later.
**Fix:** Reword to "the only compile blocker invisible to an untagged `go test ./...`".

### [NIT:scope] Un-enumerated false positive for the `builder:` grep pattern
**Section:** Testing → acceptance grep, pattern 5 (`builder:` commit-subject prefix)
**Issue:** `sandbox/build.cmd:2` reads "Launcher for the lyx sandbox Hub builder: clones ..." — a live hit on that pattern, ordinary English, not on the named exclusion list (`master-builder`, `strings.Builder`, "fixture builders", "fluent builder method").
**Fix:** Add it to the by-pattern exclusion list so the gate stays mechanical.

## Verdict

REQUEST_CHANGES
Link-repair target undefined, spec-override unrecorded, and Q&A contradicts the fixture disposition.
MILL_REVIEW_END
