I've read all 11 files in `internal/modelspec/` in full.

**Summary:** `modelspec` parses and resolves the model-spec grammar used across the stack (builder roles, perch/burler reviewers/judges, loom producers) to name which LLM runs a role. The grammar is one line: `<alias>[key=value,...]` (registry form) or `<engine>:<model-id>[key=value,...]` (escape form, bypasses registry).

**Public API:** `Spec` (parsed, exactly one of alias-form or escape-form), `Entry`/`Registry` (alias → engine+model+defaults map), `Resolved` (final engine/model/params), `Parse(s string) (Spec, error)`, `(Registry) Resolve(Spec) (Resolved, error)`, `LoadRegistry(baseDir string) (Registry, error)`, `ConfigTemplate() string`.

**Invariants:** A `Spec` is never both/neither alias and escape form. `knownParams` (`effort`, `version`) and `knownEngines` (`claude`) are closed vocabularies gating keys/engine names only — never model strings, so new models need no recompile. `Resolved.Params` is always non-nil. Bracket params always win over registry defaults per-key. `LoadRegistry` never errors on an absent file (returns `builtins()`); an empty/comments-only file also yields builtins. File entries replace built-ins as whole entries, never merged field-by-field. `models.yaml` is seeded once and never rewritten by reconcile (operator-owned after that).

**Error handling:** Fail-loud, no silent tolerance — whitespace, bad charset, empty tokens, duplicate/unknown keys, unknown engines/aliases all produce specific errors prefixed `modelspec: `, naming the offending token/position. Unknown-alias errors list all known aliases sorted.

**How pieces fit:** `modelspec.go` declares types/vocab only; `parse.go` does grammar-only checking; `registry.go` holds built-ins + `Resolve`'s alias-lookup/default-merge logic; `load.go` reads/validates/merges an optional `models.yaml` via `hubgeometry.ConfigFile`; `template.go`/`template.yaml` embed the seed file that `configreg` materializes. A strict leaf-import allowlist (stdlib + `hubgeometry` + `yaml.v3` only) is enforced by `leaf_enforcement_test.go`, keeping this package cycle-free for all future consumers.

NONCE=cormorant-58211
