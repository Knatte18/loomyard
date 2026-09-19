MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
duration_s: 145.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] step's busy text misattributed to shed.Step
**Demoted-from:** BLOCKING
**Section:** `busy-refusal-comes-from-the-arming-spec` **Issue:** The decision says `step` "keeps mapping it to `kind: busy` with its `lyx loom pause` remedy text", but `internal/loomcli/step.go:198` emits `err.Error()` (bare sentinel) for `shed.Step`'s `ErrShedBusy`; the `lyx loom pause` remedy at `step.go:155` belongs to the early run-lock probe, which this task assigns to loom's `PreStep`. **Fix:** State that `step`'s told busy message is empty (passthrough) and that the remedy wording stays inside loom's `PreStep` probe, or the move silently reworders a shipped envelope.

### [NIT:scope] status/pause lose ensureStatusLockDir
**Demoted-from:** BLOCKING
**Section:** `pause-and-watch-generalize` / Technical context **Issue:** Both `loomcli.statusCmd` and `pauseCmd` begin with `ensureStatusLockDir` (`sharedbootstrap.go:142`), a crucible-fixed MkdirAll of the ephemeral lock parent without which the "no status file … run lyx loom start" remedy is unreachable on a never-bootstrapped pair; no hook, told field, or generic step in the discussion covers it, and a generic unconditional MkdirAll would create a per-slug directory on a read-only `lyx lifecycle status`. **Fix:** Decide its home explicitly — a told pre-step/`EnsureLockDir` field on the spec, or an unconditional generic step with lifecycle's side effect accepted.

### [NIT:decision] Per-module status/run decode-error prefix undecided
**Demoted-from:** BLOCKING
**Section:** `envelope-contracts-move-with-the-verbs` **Issue:** The decode-failure text diverges per module (`"loom: decode status file …"` vs `"lifecyclecli: decode status file …"`); the discussion notes the prefix only in passing ("already recipe-agnostic apart from its `lifecyclecli:` error prefix") and never says how the generic body obtains it — the told status label is `loom`/`lifecycle`, not `loom:`/`lifecyclecli:`, so it is not derivable. **Fix:** Name the prefix (or the whole decode-error message) as a told field on the arming spec, like the busy message.

### [NIT:scope] Run/pause envelope enumeration incomplete
**Section:** `envelope-contracts-move-with-the-verbs` **Issue:** `run`'s envelope is listed as four keys plus `PostRun` extras, but loom's shipped success envelope also carries `friction` (`run.go:200`), and `pause`'s shipped envelope key `status_file` (`pause.go:51`) is never stated at all. **Fix:** Say plainly that `friction` arrives via `PostRun` extras (with its success-only gating) and pin `pause`'s `status_file` key.

### [NIT:design] PreRun's extraEnvelope return has no consumer
**Section:** `optional-hooks-for-module-specific-work` **Issue:** `PreRun` is specified as returning `(extraEnvelope map[string]any, err error)`, but no shipped filler returns keys and the discussion never says where that map lands or how it merges against `PostRun`'s. **Fix:** Either state its destination and precedence versus `PostRun` extras, or drop the return and keep `PreRun func(ctx) error`.

### [NIT:scope] ly-drive rules miss producer-semantics claims
**Section:** `ly-drive-drives-any-recipe` **Issue:** Rules (a)–(d) cover invocations, output shapes, substrate/filesystem, and graph numbers, but `SKILL.md:111` ("every spawning row's adapter probes for a live agent and waits on it") and `SKILL.md:105` ("kills the in-flight agent and restarts that row's work") are loom-recipe producer-behaviour claims that fall under none of the four. **Fix:** Add a fifth rule covering claims about a recipe's producer/adapter semantics.

## Verdict

APPROVE
Three verified gaps in status/pause and busy-message contracts; the rest is sound.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
