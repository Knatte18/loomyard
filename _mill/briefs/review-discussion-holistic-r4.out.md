MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (Anthropic); reported ID claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] Excluding .lyx from routing routes it to warp
**Section:** `never-committed-is-structural-not-configurable`; Testing/`fabricengine`
**Issue:** `classify.go:14-26` is a two-way split — anything not under a wired prefix goes to `warp`, and `commit.go:96-99` hands `warpFiles` to a warp `StageAndCommit`; dropping `.lyx` from the commit-routing set therefore sends a stray `.lyx` path into the **user's own repo**, where the warp `.git/info/exclude` entry makes `git add` fail the whole invocation (the exact exit-1 failure the decision names) and `commitBothSides` treats a warp failure as hard.
**Fix:** State what `classifyPaths` does with a `.lyx` path — a third "dropped" bucket, or an explicit error — since the decision already rejects silent dropping; the test bullet "never routes to weft" is not sufficient.

### [GAP] `(dir string)` seam cannot split state.json from its lock
**Section:** `explicit-scratch-dir-seam-per-module`
**Issue:** "Signatures keep their `(dir string)` shape; it is the caller that switches" does not hold for the largest artifact class: `websterengine.LoadState/SaveState` (`state.go:193-217`) and `builderengine.LoadState/SaveState` (`state.go:133-156`) derive **both** `state.json` and `state.json.lock` from one `dir` argument, and the task requires the file to stay in `_lyx` while the lock moves to `.lyx` — a single directory argument cannot express that.
**Fix:** Name the chosen signature change for these four functions (second scratch-dir parameter, or a lock-path parameter) and list their call sites, rather than asserting signatures are unchanged.

### [GAP] Engine-internal transient calls pass the durable dir
**Section:** `explicit-scratch-dir-seam-per-module`
**Issue:** The update list names only out-of-engine call sites, but `websterengine/runlevel.go:347,423,616,791` and `builderengine/runlevel.go:340,445,525` call `AcquireStateMutation`/`ClearPause` with `deps.WebsterDir`/`deps.BuilderDir`, and `RunDeps` (`runlevel.go:99-111`) carries no scratch field — the same silent-pause-breakage the decision cites, inside the engine.
**Fix:** State that webster's and builder's `RunDeps`/deps structs gain a scratch-dir field, and that every deps-constructing site (`webstercli/run.go:69`, `beginbatch.go:104`, buildercli equivalents, tests) supplies it.

### [GAP] Logger sink: claimed-contained change is understated
**Section:** `logger-sink-holds-no-persistent-handle`
**Issue:** Two load-bearing claims are wrong against `internal/logger/sink.go`: the trace **filename is not a package global** (it is a local inside `ensureDurableSink`'s `sync.Once` closure, line 108-114, reachable only via `sinkWriter`), and "nothing else about the sink changes" conflicts with `ensureDurableSink() (io.Writer, bool)` returning the handle — `sink_test.go:59-64` asserts that writer is non-nil.
**Fix:** Record that a new package-level path global is needed and that `ensureDurableSink`'s return contract plus its existing test change with it.

### [GAP] Unwire result vocabulary after the removals is unspecified
**Section:** `unwire-never-deletes-weft-content`; Technical context
**Issue:** `UnwireVerbResult.WeftContent` is said to "lose `cleared`" but no replacement value is named, and the `Gitignore` field is not mentioned at all even though `gitignore.Remove` (`unwire.go:113`) is deleted — both are CLI-observable JSON keys (`fabriccli/unwire.go:26-31`) under the output-envelope invariant.
**Fix:** Name the post-change value set for `weft_content` and state whether the `Gitignore` field and its `gitignore` output key are dropped or retained as a constant.

### [NOTE] loom has no `Dir(l)` for `ScratchDir(l)` to mirror
**Section:** `explicit-scratch-dir-seam-per-module`
**Issue:** `loomengine` exposes `PlanDir`/`DiscussionDir` only, and `status.json`/`status.json.lock` sit at the `_lyx` root (`config.go:71-80`), so "`ScratchDir(l)` as the sibling of the existing `Dir(l)`" has no referent there and the mirrored path is `.lyx/status.json.lock`.
**Fix:** State loom's scratch accessor shape explicitly instead of by analogy.

## Verdict

GAPS_FOUND
Routing, seam-signature, deps and result-vocabulary details must be settled before planning.
MILL_REVIEW_END
