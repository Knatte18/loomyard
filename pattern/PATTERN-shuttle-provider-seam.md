# PATTERN-shuttle-provider-seam

Provider specifics live only under `internal/shuttleengine/claudeengine`.

- `shuttleengine` and `reedengine` never reference Claude specifics, and `shuttleengine` never imports `claudeengine`.
- A `*Run` issued by `Start` or `StartGated` has already resolved its provider's startup probe.
  No caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys.
