MILL_REVIEW_BEGIN
# Review: config degrades to embedded template

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [BLOCKING:design] Membership rule misclassifies loomengine
**Section:** `### membership-rule-is-what-the-config-governs` + Scope/Out
**Issue:** The rule is "operator-tunable producer knobs (model specs, timeouts, poll intervals) degrade; hub state stays strict", but `internal/loomengine/config.go:90` is documented as exactly "role model-specs and timeout knobs" (`discussion`, `discussion_timeout_min`, `plan`, `plan_timeout_min`) — indistinguishable from `webster.yaml`, which the same rule places in the degrading set — while the pinned strict set keeps it. `internal/batcher`'s single `active:` key with a registry default (`config.go:22`) is the same problem, weaker.
**Fix:** Either state an additional discriminator that puts loom/batcher on the strict side (e.g. "consumed only by a loop that already requires a resolved hub location"), or move loomengine and re-pin the two sets T10's guard will inherit.

### [BLOCKING:design] Non-absence errors silently fall back
**Section:** `### shared-body-refactor`
**Issue:** The flag is described as consulted at "the `FindBaseDir` failure" (whole error) versus "the `os.IsNotExist` config-file read failure" — asymmetric. `FindBaseDir` (`internal/configengine/config.go:27-36`) returns a wrapped `stat _lyx: %w` for a permission/IO error with no sentinel, so on the degrading path a genuinely broken `_lyx` would resolve the template instead of erroring, and the fallback debug line would misreport the condition as "absent `_lyx/`".
**Fix:** Decide explicitly whether `LoadOrTemplate` falls back only on absence — and, if so, say how absence is distinguished (a `FindBaseDir` sentinel/`errors.Is` seam, or an `os.Stat`+`os.IsNotExist` check inside the shared body).

### [BLOCKING:design] Debug-observability rationale is false
**Section:** `### debug-level-observability`
**Issue:** Both rationale legs are wrong against source. `logger.SetVerbosity` (`internal/logger/logger.go:379-385`) maps count==1 to Info and count>=2 to Debug, so the fallback line is *not* "available under `-v`" — it needs `-vv`. And `durableHandler` is unconditional at Info and above only (`logger.go:281-285`), so a Debug record never reaches the durable trace file, while the rejected `logger.Info` would not print to stderr at default verbosity either — it would only land in the trace sink, which is the opposite of "prints on essentially every producer run".
**Fix:** Restate the decision against the real level semantics and re-choose: Debug (stderr at `-vv`, never in the trace file) versus Info (trace file always, stderr at `-v`).

### [NIT:decision] configengine's own package comment has no disposition
**Section:** Scope/In + Technical context ("plus its package comment")
**Issue:** `internal/configengine/config.go:1-6` says the file "implements strict YAML configuration loading" and names three stale wrappers; the In list covers only the four callers' doc comments and `docs/shared-libs/configengine.md`.
**Fix:** Name the configengine file-header/package comment as an in-scope edit in the same commit.

### [NIT:consistency] "all four templates" overstates env-marker coverage
**Section:** `### fallback-resolves-through-envsource`
**Issue:** Only `shuttleengine/template.yaml` and `reedengine/template_{posix,windows}.yaml` carry `${env:…}` markers; perch's and webster's templates carry none, so the six named vars cover two modules, not four.
**Fix:** Reword to "the two templates that use env markers use only the optional form; perch/webster use none".

## Verdict

REQUEST_CHANGES
Membership rule, fallback-error scope, and the logger-level rationale each need resolving.
MILL_REVIEW_END
