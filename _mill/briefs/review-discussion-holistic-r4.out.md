MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic) — exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Acceptance grep is blind to bare-word provenance comments
**Section:** Testing → Acceptance commands (pattern list) + `sweep-everything` / `provenance-comments-rewritten-to-stand-alone`
**Issue:** All five patterns key on `builderengine|buildercli`, phase/gate tokens, `lyx builder`/`builder.yaml`/`_lyx/builder`, `builder-contract`, and `builder:`/`/builder/` — and the discussion explicitly rejects a bare `-i builder` scan; but the largest swept class writes plain "builder", so the gate cannot verify it. Verified misses, present in the tree and absent from the Go-sites inventory: `internal/websterengine/outcome.go:4–7` ("a builder mechanism … an in-tree builder caller … builder's Outcome/ParseOutcome … Shared Decision builder-is-frozen-copy-not-move"), `recordbatch.go:9` and `recoverbatch.go:20` ("mirroring builder's own fabric-commit-boundary discipline"), `roles.go:15` ("Unlike builder, only the cold recovery strand carries its own role"). None is caught by any pattern.
**Fix:** State a completion criterion that covers the bare-word provenance class — e.g. a bare `-i builder` scan with the ordinary-English exclusions enumerated as today — so `sweep-everything`'s "opinion → test" claim holds for the class it was written for.

### [BLOCKING:scope] S9 deletion span leaves two live S9 references
**Section:** Markdown sites → `tools/sandbox/SANDBOX-CORE-SUITE.md`
**Issue:** The stated span (`:224` heading through `**Verdict:**`, up to the `---` at `:287`) excludes `:99` ("`ref` is the scenario id (`S0`-`S6`, `S9`)") and `:306` ("`S9: <OK|WARN|FAIL> …`" in the session-log template). Both survive deletion, neither contains the word "builder", so no acceptance pattern and no test sees them — the suite would instruct operators to report a scenario that no longer exists.
**Fix:** Add `SANDBOX-CORE-SUITE.md:99` and `:306` to the S9 disposition (drop `S9` from the id list and the log template).

### [NIT:scope] "one named false positive" claim is inaccurate
**Section:** Go sites → `refscanner_test.go` exception; Testing → deliberate exceptions
**Issue:** `internal/websterengine/audit_test.go:25,149,165,171–176,202–203,257–286` carries the same unrelated `master-builder` worktree-name fixture, but is neither inventoried nor named as an exception, so "the acceptance grep's one named false positive" is false as written.
**Fix:** Name `websterengine/audit_test.go` alongside `refscanner_test.go` in the exception list, or drop the "one" claim.

### [NIT:decision] `model-spec.md:105` has no webster analogue
**Section:** Markdown sites → `docs/reference/model-spec.md`
**Issue:** `:105` is listed but sits outside the worked example ("What is *not* a parameter": "a role that needs a large window (builder's `implementer_oversized`)"); webster's two roles (`master`, `recovery`, per `internal/websterengine/template.yaml`) offer no substitute, and the rebuild instruction covers only the `builder.yaml` example.
**Fix:** State the disposition for `:105` — generic rewording versus a named webster role.

## Verdict

REQUEST_CHANGES
Sweep gate cannot check its largest class; S9 residue at two unlisted lines.
MILL_REVIEW_END
