MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
duration_s: 238.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Nobody creates the joined per-segment RunDir
**Section:** "Per-segment run directories: `Env.RunRoot` plus a `Config` `run_subdir`"
**Issue:** `Bouncer.Call` runs `ResolveRound` first, which `os.Stat`s `RunDir` and returns a hard error when it does not exist (`internal/shedadapters/round.go:24-31`); only `BurlerProducer.Call` creates it (`burler.go:232`), and the Bouncer is the segment's entry point, so it runs first — meanwhile the joined `RunRoot/<run_subdir>` path is known only inside the entry, so the caller filling `Env` cannot pre-create it either.
**Fix:** State who creates the segment directory (the `Bouncer` entry at construction, `Bouncer.Call` before `ResolveRound`, or an explicit caller obligation) and record it as a decision with a test scenario.

### [BLOCKING:design] Required-vs-optional `Config` keys are never pinned
**Section:** "`Config` is strict about unknown keys" / per-entry key-set decisions
**Issue:** Every entry's *recognised* key set is enumerated, but not which keys are required; an absent `run_subdir` silently resolves `RunDir` to bare `Env.RunRoot`, reinstating the exact cross-segment overwrite the `run_subdir` decision exists to prevent, with no error anywhere.
**Fix:** Pin required vs optional per entry (at minimum `run_subdir`, `stencil`, `output_files`, `profile`), with an empty-string value treated as absent, and add the missing-required-key cases to testing items 6 and 7.

### [NIT:scope] Scope sentence's engine arithmetic does not reach twelve
**Section:** Scope → In, bullet 3
**Issue:** "every `ShedProducer` type reachable from loom's current list plus the two shipped review adapters" enumerates nine plus two; `SingleLLM` is neither in loom's current list nor a review adapter, yet is one of the twelve.
**Fix:** Reword to name `SingleLLM` explicitly as the third addition (it has no production caller today).

### [NIT:design] `SingleLLM` stencil existence is checked per-Call, not at construction
**Section:** "The `SingleLLM` entry builds its `SpecSource` from a stencil"
**Issue:** The stencil read sits inside the closure, so a mistyped `stencil` name fails at first `Call`, unlike `NewBouncer`, which probes its rubric at construction precisely to avoid that (`bouncer.go:124-133`); the asymmetry is never stated as a decision.
**Fix:** Record the asymmetry (or a construction-time probe) explicitly so testing item 4's "a missing stencil errors" is assertable at a known point.

### [NIT:consistency] `stencil.Fill`'s error condition is stated too narrowly
**Section:** Q&A "[review r4] Does `SingleLLM` read `Env.AnchorPath` unconditionally"
**Issue:** The rationale says `Fill` errors only on a marker "the map lacks"; it also errors on a marker whose value is present but empty or whitespace-only (`internal/stencil/stencil.go:169-181`, `presentButEmptyBranchMarkers`), which reaches an empty `Config.tokens` value.
**Fix:** Restate as "absent **or** empty", and say whether an empty `tokens` value is rejected at construction.

## Verdict

REQUEST_CHANGES
Run-directory creation and required-key semantics are unresolved; both cause silent or mid-run failures.
MILL_REVIEW_END
