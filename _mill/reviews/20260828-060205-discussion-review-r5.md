MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
duration_s: 159.1
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] websterengine test plan rests on a false premise
**Section:** Testing (`runlevel.go` / `runVerifyCommand` paragraphs)
**Issue:** "Both are outcome-switch sites … reachable by constructing a `shuttleengine.Result`" does not hold for `runlevel.go`: the switch is inline in `Run` (runlevel.go:567, after `SaveState`, the mutation lease release, and `handle.Wait()`), and the only existing driver is `newRunFixture` in `runlevel_test.go`, which is `//go:build integration`, `package websterengine_test`, and builds real scratch git repos — so an untagged unit test either cannot reach the branches or violates Test Tier Purity; separately `runVerifyCommand` is unexported, while that package's tagged test files are all in the external `websterengine_test` package, so "follow `integration_test.go`" puts the test where the function is unreachable.
**Fix:** State the tier and package placement for both websterengine tests (tagged vs untagged, internal `package websterengine` vs external), and say whether the four `runlevel.go` branches are covered through the existing tagged `Run` fixture or not directly tested at all.

### [NIT:consistency] `mergeresolve` listed as structurally exempt
**Section:** `constraints-md-prose-only`, paragraph after the replacement text
**Issue:** "so `gitexec`, `gitkit`, `githubclient`, and `mergeresolve` are covered by the rule" contradicts `mergeresolve-allowlist` (mergeresolve *gains* the `logger` import) and the spawn table (mergeresolve is not a spawn site at all — verified: no `exec.Command*` in the package).
**Fix:** Drop `mergeresolve` from that sentence.

### [NIT:consistency] Superseded r2 Q&A answers left unmarked
**Section:** Q&A log, entries at lines 280-281
**Issue:** They still assert the replacement wording is "production" (superseded by "reachable from a `lyx` command", line 288) and that the hard-error selector is a `shuttleengine.Outcome[A-Z]` grep minus `doc.go` (superseded by the AST selector, line 289).
**Fix:** Annotate both entries as superseded by the r4 answers, or restate them to match the current decisions.

### [NIT:design] Hard-error selector's switch half is under-specified
**Section:** `error-universe`
**Issue:** The spawn selector is fully mechanical (import name + `Sel.Name`), but "an `*ast.SwitchStmt` whose tag is an outcome-valued expression" needs type information or an unstated name heuristic — judgment reintroduced into the one decision that exists to remove it.
**Fix:** Give the switch half a syntactic rule of the same precision (e.g. a switch any of whose case expressions is a `shuttleengine.Outcome*` selector).

### [NIT:decision] Hard-error half of the audit has no enforcement disposition
**Section:** `enforcement-guard`
**Issue:** The guard covers `exec.Command*` sites only; the outcome-switch table gets no enforcement, yet the decision's own rationale ("a document is a snapshot that rots") applies to it identically, and the asymmetry is never stated.
**Fix:** State explicitly that the hard-error table is document-only and why (or that a second guard is deferred).

## Verdict

REQUEST_CHANGES
websterengine testing strategy contradicts the source's reachability and tier reality.
MILL_REVIEW_END
