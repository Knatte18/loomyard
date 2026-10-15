# PATTERN-model-choice-mapping

A resolved model-spec reaches a spawn only as a `shuttleengine.ModelChoice` built by `shuttleengine.NewModelChoice`; no other production code reads a `modelspec.Resolved` field.
A later engine then takes a role by its model key alone, since the engine, model, effort and version travel together.

- Outside `internal/modelspec` and `NewModelChoice`, nothing reads `Resolved.Engine`, `Resolved.Model` or `Resolved.Params`.
- Every function that resolves a model key returns a `shuttleengine.ModelChoice`, or a list or map of them, built by `NewModelChoice`.
  The resolvers it binds are the seat resolvers, burler's per-round lists and the Bouncer pair, the driver, arbiter, alignment and tackler settings, treadle's profile and webster's roles.
- A timeout or spawn budget the same resolver also resolves stays beside the choice, never inside it.

## Enforcement

`TestModelChoiceMappingRule` in `internal/shuttleengine/modelchoicemapping_enforcement_test.go` walks the production files, skips `internal/modelspec/`, and parses each file that imports `internal/modelspec`.
It reports the file and line of a selector `x.Engine`, `x.Model` or `x.Params` where `x` is an identifier holding a `modelspec.Resolved`, bar inside `NewModelChoice` in `internal/shuttleengine/modelchoice.go`.
An identifier holds one when it is assigned from a call of a method named `Resolve`, is declared with the type `modelspec.Resolved`, or is the value of a range over a map or slice of it.

The scan is syntactic, so it is a tripwire and not a proof.
It cannot see a `Resolved` reached through a struct field or a call chain; review covers those.
