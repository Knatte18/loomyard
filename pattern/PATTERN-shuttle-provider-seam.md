# PATTERN-shuttle-provider-seam

Provider specifics live only under `internal/shuttleengine/<engine>`, and only `internal/shuttleengines` constructs or imports a concrete engine.

- `shuttleengine` and `reedengine` never reference a concrete engine's specifics, and `shuttleengine` never imports a concrete engine.
- Only `internal/shuttleengines` imports or constructs a concrete engine; the scan in `internal/shuttleengine`'s `seam_enforcement_test.go` covers production files.
- An engine's name, a registry key such as `shuttle.yaml`'s `engine: claude` or a `modelspec` engine name, is not a provider specific and may appear outside its directory.
- A `*Run` issued by `Start` or `StartGated` has already resolved its provider's startup probe.
  No caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys.
