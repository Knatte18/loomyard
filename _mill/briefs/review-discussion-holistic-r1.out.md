MILL_REVIEW_BEGIN
# Review: planparser owns the plan-directory path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [BLOCKING:design] "cli.go:194 is untestable" rests on a false premise
**Section:** § Where anchoring is actually proven / § Testing → `internal/webstercli`
**Issue:** The discussion states twice that `PersistentPreRunE` "is not exercised by any test — the fixtures construct the CLI struct directly", but `internal/webstercli/verbs_test.go:721` and `:749` (`TestPersistentPreRunE_UnknownBatcherFailsFast`, `TestPersistentPreRunE_DefaultBatcherResolves`) drive the real `Command()` pre-run via `RunCLIIn(h.PrimeWorktree(), …)` on a real `hubforge.NewHub(t, ".")` hub, and `hubforge.NewHub` takes an anchor argument documented as `"." or "backend"` — so a subpath-anchored pre-run test asserting the resolved plan dir is available today.
**Fix:** Re-decide the anchor-always coverage question against those two tests: either add a `hubforge.NewHub(t, "backend")` pre-run case that pins `c.planDir` under `AnchorPath()`, or state explicitly why that case is deferred to T7 despite the fixture existing.

### [BLOCKING:consistency] Named verification commands never compile verbs_test.go
**Section:** § Testing (verification baseline) and § Testing → `internal/webstercli`
**Issue:** `internal/webstercli/verbs_test.go` is `//go:build integration` (line 1), so neither `go test ./...` nor the task-specific `go test ./internal/webstercli/...` compiles it — the `verbs_test.go:220` fixture flip, a named In-scope item, is unverified by every command the discussion names.
**Fix:** Name the tagged invocation (`go test -tags integration ./internal/webstercli/...`) alongside the untagged baseline, and say which listed test edits it covers.

### [BLOCKING:consistency] "Both files already import planparser" is false
**Section:** § The `cmd/lyx` guard tables, and what they still prove
**Issue:** `cmd/lyx/notransients_test.go`'s import block (lines 16-29) has no `planparser` import, so the rewrite is not import-churn-free there; its header comment (lines 6-8) also enumerates the owning modules the file may import at once, a touch-up the discussion flags only for `constructoranchoring_test.go`.
**Fix:** Correct the claim to cover `constructoranchoring_test.go` only, and add `notransients_test.go`'s import plus header-enumeration edit to scope.

### [NIT:decision] `fabric-unified-view.md` names `PlanDir` with no disposition
**Section:** § Scope (doc updates)
**Issue:** `manifest/designs/fabric-unified-view.md:68` lists `PlanDir` in the as-built `_lyx`-durable anchoring table (and `:49` in the constructor inventory); the doc scope covers only `doc.go`, `docs/overview.md`, and `CONSTRAINTS.md`.
**Fix:** State the disposition — leave as a historical as-built record, or annotate the relocation.

### [NIT:consistency] Two stale comments the scope does not mention
**Section:** § Technical context
**Issue:** `internal/webstercli/cli.go:57-58` describes `planDir` as one of "the lyxcwd-resolved `_lyx` dirs", and `verbs_test.go:12-13`'s header asserts tests always bypass `PersistentPreRunE` — already false, and the likely source of the premise in the first finding.
**Fix:** Name both comments as in-scope touch-ups, or state that they are deliberately left alone.

## Verdict

REQUEST_CHANGES
Coverage decision rests on a false premise; two verifiable claims about existing files are wrong.
MILL_REVIEW_END
