MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:consistency] pattern/stencilstore import sites are misattributed
**Section:** `fabric-verdict-do-not-extract-and-say-why-precisely`; `fabric-recommendation-is-an-in-repo-seam-not-a-repo-boundary`
**Issue:** The claim that `pattern` and `stencilstore` enter via `stencilcommit.go`/`stencilhistory.go` is measurably false: `stencilcommit.go` imports neither (only `gitrepo`, `lock`, `lyxdirs`), and `internal/pattern` enters solely through `pull.go:23` (`pattern.PathspecFile`/`PathspecDir` at :423,:441,:464) — a file the discussion assigns to the **pair-kernel** side of the partition, which breaks the "the split isolates the stencil/pattern coupling [on the hub-layout side]" rationale.
**Fix:** State the two real import sites (`pull.go` → `pattern`, `stencilhistory.go` → `stencilstore`), and re-derive whether the pair kernel is `pattern`-free or must carry that edge; the split's rationale and the "a standalone paired-git-repos library cannot ship those" line both depend on the corrected answer.

### [BLOCKING:consistency] Logger prep item (a) collides with a shipped enforcement test
**Section:** `reed-preparation-work-is-worth-doing-regardless` (a); `reed-logger-decoupling-is-an-injected-slog-logger`
**Issue:** The slog rationale asserts the Live-Substrate Spawn Observability invariant "requires that spawns are logged, not which package logs them", but CONSTRAINTS.md names `internal/logger` explicitly and `cmd/lyx/spawnobservability_test.go` enforces it as file-level import presence (`spawnObservabilityLoggerImportPath`, :59; allowlist at :64 requires a written structural reason). `internal/reedengine/{lifecycle,overlay,attach}.go` all carry real `exec.Command` calls, so an *in-repo* `logger` decoupling — which is what (a) recommends — fails that test absent allowlist entries, while Scope bars CONSTRAINTS.md edits.
**Fix:** Say explicitly whether (a) is standalone-only (injection seam added, `internal/logger` retained in-repo) or an in-repo removal, and in the latter case record that it requires a CONSTRAINTS.md amendment plus allowlist entries as part of that follow-up item — do not present it as cost-free hygiene.

### [NIT:consistency] `render` import claim is incomplete
**Section:** `reed-standalone-layout-mirrors-quarry`; `other-modules-are-a-note-not-a-follow-up-task`
**Issue:** "imports only `fmt` and `strings`" is wrong — `internal/reedengine/render/policy.go:9` imports `sort`. The substance (stdlib-only, zero internal deps) holds, but the doc's stated value is that every number is reproducible.
**Fix:** State the import set as measured (`fmt`, `sort`, `strings`) or characterize it as stdlib-only with no internal dependency, with the producing command.

## Verdict

REQUEST_CHANGES
Two load-bearing measured claims are wrong; one recommendation conflicts with a shipped enforcement test.
MILL_REVIEW_END
