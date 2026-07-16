## Consolidated Review — `internal/modelspec`

*(One process note: the eighth fork, tasked with the TEST GAPS lens, hit a "fork not available inside a forked worker" restriction and ran all eight lenses solo instead of just its own. Its overlapping findings were cross-checked against the seven dedicated-lens reports below and folded in only where they added something new; its lens-appropriate test-gap findings are included normally.)*

### HIGH

**1. [registry.go, `Resolve`] `Resolve` never enforces the "exactly one shape" invariant `modelspec.go` documents for `Spec` — a hand-built or zero-value `Spec{}` silently resolves instead of erroring.** `Resolve` routes purely on `s.Alias == ""`. A zero-value `Spec{}` (Alias, Engine, Model all empty — neither documented shape) falls into the escape-form branch and returns `Resolved{Engine:"", Model:"", Params:{}}` with a **nil error**. A `Spec` with both `Alias` and `Engine`/`Model` set (also invalid) silently takes the alias branch and drops `Engine`/`Model` with no complaint either. Every field on `Spec` is exported, so nothing stops a future caller from building one directly and bypassing `Parse`'s discipline — the one place that invariant is actually enforced. This directly contradicts the package's stated "fail loud, never tolerate malformed input" philosophy (which `Parse` embodies rigorously) and defers failure to a far-away, harder-to-diagnose call site instead of the point where bad data originates.
— Origin: handler (holistic), correctness, error-handling, api-design, test-gaps *(five independent lenses converged on this)*

### MEDIUM

**2. [load.go, `validateAlias`] `Entry.Model` and `Entry.Defaults` values loaded from `models.yaml` are checked for non-emptiness only — never charset-validated — unlike every other value-bearing path in the package.** `Parse` gates escape-form model-ids and bracket param values through `isModelIDChar`; `validateAlias` (load.go:83-105) does neither for `entry.Model` or `entry.Defaults` values. An operator-authored `models.yaml` can therefore inject arbitrary bytes (spaces, control characters, shell metacharacters) into `Resolved.Model`/`Resolved.Params` that no spec string could ever produce — and the package's own doc comment shows these flowing straight into `shuttleengine.Spec` fields (`spec.Model = resolved.Model`) with no further sanitization implied.
— Origin: error-handling, security (two independent findings), test-gaps

**3. [load.go, `validateAlias`] An empty-string alias key in `models.yaml` passes validation silently, contradicting both the file's own docstring and `Parse`'s stricter twin.** `validateAlias` calls `validateCharset(alias, "alias", isIdentChar)` with no separate emptiness check; `validateCharset`'s `for range` loop over `""` never executes and returns `nil`. A `models.yaml` block keyed `"":` (valid YAML) is accepted into the registry — even though `LoadRegistry`'s own doc comment states "an alias key must match `[a-z0-9-]+`" (a `+`-quantified, non-empty pattern) and `Parse` explicitly rejects an empty alias in a spec string (`parse.go:91-93`). Practically inert (no spec string can ever address `""`), but it's a real validation/doc mismatch in the one place meant to mirror `Parse`'s strictness.
— Origin: handler (holistic), docs-consistency, test-gaps

**4. [modelspec.go/registry.go, `Registry`] `Registry` is exposed as a bare `map[string]Entry` with no validation at the `Resolve` boundary.** Closed-vocabulary enforcement (known engine, non-empty model) happens only inside `LoadRegistry`'s `validateAlias`. Any `Registry` assembled another way — by a future consumer building one programmatically, or in a test — reaches `Resolve` with zero checking and can carry an unknown-engine or empty-model entry straight into a `Resolved`.
— Origin: api-design, test-gaps

### LOW

**5. [parse.go, `Parse` bracket splitting] Multi-bracket input (e.g. `sonnet[a=1][b=2]`) is accepted structurally and only rejected later via a misattributed error.** The bracket boundary is found by the *first* `'['` and the remainder is only checked to end in `']'` — it doesn't verify a single well-formed bracket. `sonnet[a=1][b=2]` is correctly rejected, but via "invalid character '[' ... in param value" rather than a message naming the real problem (a second bracket group). Correct outcome, misleading diagnostic — breaks the file's own stated goal that "every rejection is its own named error explaining the real problem."
— Origin: correctness, error-handling (trailing-comma finding, same theme)

**6. [load_test.go] No test exercises `validateAlias`'s alias-charset rejection branch** (e.g. an uppercase or invalid-character alias key in `models.yaml`) — untested code path.
— Origin: test-gaps

**7. [load_test.go] No test covers `LoadRegistry`'s non-`ErrNotExist` `os.ReadFile` error branch** (e.g. the models.yaml path pointing at a directory) — the wrapped-read-error path (load.go:50-51) is untested.
— Origin: test-gaps

**8. [template.go, `ConfigTemplate` doc] Godoc overclaims uniform defaults.** `ConfigTemplate`'s doc comment says the seed entries come "with their operator-owned effort defaults," but `template.yaml`'s `haiku` block has no `defaults` key at all (confirmed by `template_test.go`'s `wantNoDef: true`) — the doc implies all four aliases carry an effort default when one doesn't.
— Origin: docs-consistency

**9. [registry.go/modelspec.go/load.go, package state] A few undocumented-but-currently-harmless concurrency invariants.** `knownParams`/`knownEngines` are mutable-typed package vars read without synchronization (never written after init today, but nothing marks them immutable); `LoadRegistry` mutates the map `builtins()` returns in place, which is only race-safe because `builtins()` always allocates fresh — an undocumented load-bearing assumption that a future "cache builtins()" optimization could silently break; `Registry` itself carries no documented copy-on-share contract for concurrent readers. No live race exists in the package today (confirmed: no shared mutable state is actually written after init), but these are worth a doc note before any hot-reload feature is added.
— Origin: concurrency

**10. [parse.go/registry.go/load.go, various] Minor avoidable allocations, all negligible at this input scale.** `copyParams` always allocates even for the empty-result case; `builtins()` reconstructs its 4-entry map on every `LoadRegistry` call; `Parse` makes several independent linear passes over the same short string. None matter given hand-typed, short spec strings and a tiny fixed-size registry — noted for completeness, not worth changing.
— Origin: performance

---

### Rejected

- **"Duplicate alias keys in `models.yaml` are silently resolved to the last occurrence" (error-handling lens).** Empirically disproven: I wrote a standalone repro decoding a YAML doc with two `sonnet:` blocks through `gopkg.in/yaml.v3` into a `map[string]...` — it returns `yaml: unmarshal errors: line 3: mapping key "sonnet" already defined at line 1`. `LoadRegistry` already surfaces this as a wrapped "parse" error, consistent with its other malformed-YAML handling. False positive.
- **"`builtins()`'s bare alias strings (`Model: "sonnet"` etc.) may not be valid provider-side model ids, breaking zero-config resolution" (correctness lens, rated HIGH by that fork).** Speculation about `claudeengine`/`shuttleengine` behavior, which is outside `internal/modelspec` and wasn't part of this review's Phase 1 scope. The claim is also undercut by strong internal consistency: the identical `sonnet`/`opus`/`haiku`/`fable` bare-string pattern is repeated verbatim in the operator-facing `template.yaml` seed, and the package's own docs are explicit that `Model` is a deliberately opaque, never-validated free-form string passed straight to the engine — exactly the shape you'd expect if the underlying claude engine accepts these as legitimate shorthand (which matches known real-world `claude --model sonnet`-style usage). Rejected as unverified and most likely intentional.
- **"`Resolve`'s returned `Params` map could later be mutated by a caller, corrupting future resolutions" (implicit in a couple of forks' framing).** Already disproven by the code and by `TestRegistry_Resolve_NeverMutatesInputs`: `copyParams` returns a fresh map on every call, and the registry entry's own `Defaults` is never shared with the caller. Not a real gap.
- **"`LoadRegistry` should lock `models.yaml` against a concurrent writer" (concurrency lens).** No concurrent-writer scenario exists anywhere in this package's design — it's an operator-edited, between-process-runs config file, not a live-reload target. Out of scope; rejected as speculative.
- **Leaf Invariant / Hub Geometry Invariant violations.** Checked directly against `CONSTRAINTS.md` and `leaf_enforcement_test.go`: every production file's non-stdlib imports are exactly `hubgeometry` + `yaml.v3`, and both `load.go` and its test file route every path through `hubgeometry.ConfigDir`/`ConfigFile`, never hand-joined. No violation found by any lens or the handler.