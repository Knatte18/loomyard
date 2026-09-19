MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
duration_s: 145.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class model (self-assessment; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Who resolves cwd/args on the `lyx shed` path
**Section:** `generic-package-resolves-nothing` vs `flags-and-args-belong-to-the-arming-module`
**Issue:** The first says `shed`'s `PersistentPreRunE` delegates to the arming function so "each recipe keeps its own resolution"; the second says each module's existing `PersistentPreRunE` stays "the sole reader of its own arguments" — cobra runs only the nearest non-nil `PersistentPreRunE`, so `lifecyclecli.resolvePersistentPreRun` (which does `lyxcwd.Resolve` → `PrimeName` → non-prime refusal → `slug = args[0]`, and short-circuits on `cmd.Name() == "lifecycle"`) never runs under `lyx shed`.
**Fix:** State explicitly that resolution+arg-reading is factored out of each module's `PersistentPreRunE` into a single reachable entry point the arming function calls, and that its command-name guard is replaced by something name-independent.

### [BLOCKING:design] `friction` key presence vs reflection gating
**Section:** `envelope-contracts-move-with-the-verbs`
**Issue:** `run.go` initialises `frictionStatus := frictionengine.StatusSkipped` and always emits the `friction` key; only the *reflection call* is gated by `shouldReflectFriction`. The discussion's "carrying its present success-only gating" reads as gating the key, which would drop it on `RunPaused` — a silent envelope break.
**Fix:** State that `PostRun` returns `friction` unconditionally on the success path, with `skipped` as the value when reflection does not fire.

### [NIT:scope] Watch path has a second hardcoded `loom` literal
**Demoted-from:** BLOCKING
**Section:** `pause-and-watch-generalize`
**Issue:** Only `renderStatusLine`'s prefix is made a told label, but `statusUnavailableLine` = `"loom status unavailable (status file transiently unreadable)"` is a second loom literal in the same tail, and its byte-stability is load-bearing (the tail dedupes on printed text).
**Fix:** Give the disposition of `statusUnavailableLine` explicitly — told label composed in, with the dedupe-stability property preserved.

### [NIT:decision] loom `status`'s third error path has no stated home
**Demoted-from:** BLOCKING
**Section:** `envelope-contracts-move-with-the-verbs` (status halves)
**Issue:** `status.go` has a second error envelope, `loom: decode status file <path>'s product payload: <err>`, raised inside the `st.Product` unmarshal — the discussion covers only the `ReadJSONStrict` decode failure, and never says whether a `StatusExtras` error is reported verbatim or re-prefixed with the told decode prefix (which would double-prefix this one).
**Fix:** State that `StatusExtras` errors are reported verbatim (hook owns the full string) and name this message as the hook's own.

### [NIT:decision] `pause`'s absent-file refusal is undecided for lifecycle
**Demoted-from:** BLOCKING
**Section:** `pause-and-watch-generalize`
**Issue:** The Decision calls `pause` "identical for any Shed", but loom's absent-file refusal is loom-specific text naming `lyx loom start`; Testing then says "refuses with the told message", with no Decision establishing that told field and no wording decided for lifecycle.
**Fix:** Add the absent-file pause message to the arming spec's told strings and fix lifecycle's wording here, as `status`'s disposition already is.

### [BLOCKING:design] ly-drive enumeration rules do not reach the frontmatter
**Section:** `ly-drive-drives-any-recipe`
**Issue:** Rules (a)–(e) all scope body claims; the skill's frontmatter `description` ("Drive a loom task…") and its missing `argument-hint` fall outside all five, and the discussion never says how the recipe name is passed (other plugin skills declare `argument-hint`).
**Fix:** Add a rule covering the skill's own identity metadata, and state the argument mechanism (`argument-hint: "[recipe]"`, defaulting to `loom`).

### [NIT:consistency] `shedcli` naming-rule conformance claim unverified
**Section:** Constraints — CLI / Cobra Invariant
**Issue:** The discussion asserts `shedcli` → `internal/shedengine` conforms to `<module>cli` imports `<module>engine`, but the stated import set for `shedcli` is `shedverbs`, `loomcli`, `lifecyclecli` only.
**Fix:** Either state that `shedcli` genuinely imports `shedengine`, or accept a deviations-list entry.

### [NIT:design] `lifecycle status --watch` on a never-run slug
**Section:** `pause-and-watch-generalize`
**Issue:** With the absent-file disposition short-circuiting before the tail, `lyx lifecycle status --watch` on an unrun slug exits immediately instead of waiting — plausible but never stated.
**Fix:** Say so in one line, so the new verb's behaviour is decided rather than inferred.

## Verdict

REQUEST_CHANGES
Six envelope/ownership questions must be settled before the extraction can be planned safely.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
