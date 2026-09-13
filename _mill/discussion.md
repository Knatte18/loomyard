# Discussion: modelspec: bare-effort shorthand and version-to-v rename

```yaml
task: 'modelspec: bare-effort shorthand and version-to-v rename'
slug: modelspec-effort-shorthand
status: discussing
parent: main
```

## Problem

Every model-spec that tunes reasoning effort has to spell out the parameter key: `opus[effort=high]`.
Effort is the overwhelmingly common bracket param — it appears in `internal/loomengine/template.yaml` (four roles), `internal/websterengine/template.yaml`, `internal/landingshed/template.yaml`, and across loom/webster/landingshed config tests — and `effort=` is pure ceremony in every one of them.

Two changes to the bracket grammar in `contracts/specs/llm-model-spec.md`, implemented in `internal/modelspec/parse.go`:

1. A bare bracket token with no `=` means `effort=<token>`, so `opus[high]` is exactly `opus[effort=high]`.
2. The bracket's version param key is renamed `version=` → `v=`, so a combined spec reads `opus[high,v=4.8]`.

**Why now:** the grammar is young and its only in-repo `version=` use is the spec doc's own example — nothing in production config or code writes a `version=` bracket, so the breaking rename is free today and gets progressively more expensive as operator `models.yaml` and role configs accumulate.

## Scope

**In:**

- `internal/modelspec/parse.go` — `parseBracket` gains bare-token handling and a bracket-key-spelling vocabulary; `Parse` is untouched.
- `internal/modelspec/modelspec.go` — new `bracketKeys` map beside `knownParams`; package doc's grammar line and the `spec.Version = resolved.Params["version"]` consumer example stay canonical but the grammar prose gains the shorthand.
- `internal/modelspec/parse_test.go` — new accept/reject cases; one existing reject case is deleted and two existing accept cases are rewritten off `version=` (see Testing).
- `contracts/specs/llm-model-spec.md` — grammar section documents the shorthand; the three `version=` mentions (lines 64, 66, 112) become `v=`.
- `docs/overview.md` — the shuttle bullet's "the model-spec notation's `version=` param" (line 302) becomes `v=`.

**Out:**

- **`models.yaml` per-alias defaults.** `Entry.Defaults` already does effort/version defaults per alias and version-pinning via an explicit `model:` id. Confirmed working as wanted; nothing to build. Its YAML key stays spelled `version`, not `v` — the terse form exists for inline brackets only.
- **Whitespace tolerance.** `Parse` rejects any whitespace rune anywhere in the spec string before structural parsing (`parse.go:47-51`). That stays. `opus[high, v=4.8]` remains a hard error.
- **Rewriting existing `[effort=...]` call sites.** `effort=` stays valid; `internal/loomengine/template.yaml` (`discussion`, `plan`, `review`, `friction`), `internal/websterengine/template.yaml`, `internal/landingshed/template.yaml`, and the loom/webster/landingshed config and wiring tests keep their current spelling. No `.yaml` outside `internal/modelspec` is touched by this task at all.
- **Effort value vocabulary.** modelspec gains no list of legal effort values.
- **`internal/shuttleengine` and `claudeengine`.** `Spec.Version` and the version→model-id translation are untouched; this task changes only the notation feeding them.
- **`docs/reference/model-spec.md`.** Referenced by `internal/modelspec/template.yaml` and `internal/landingshed/template.yaml`, but the file does not exist. Pre-existing dangling reference, unrelated to this change — noted, not fixed.
- **Registry, loader, resolution.** `registry.go`, `load.go`, `template.go` unchanged (`load.go`'s `validateAlias` keeps validating `Entry.Defaults` against `knownParams`).

## Decisions

### `v=` normalizes to the canonical param key `version`

- Decision: `v` is a bracket **spelling** only. `Parse("opus[v=4.8]")` yields `Spec{Alias: "opus", Params: {"version": "4.8"}}` — the map key is `version`, never `v`.
- Rationale: seven production sites read `resolved.Params["version"]` — `internal/websterengine/runlevel.go:592`, `internal/websterengine/recoverbatch.go:177`, `internal/loomengine/review.go:46`, `internal/loomengine/discussion.go:71`, `internal/loomengine/plan.go:106`, `internal/frictionengine/spec.go:62`, `internal/mergeresolve/spec.go:80`. `Entry.Defaults` also keeps the `version` key. `Registry.Resolve` (`registry.go`) implements bracket-over-default precedence by overlaying `s.Params` onto `entry.Defaults` key-by-key, so a literal `v` key would sit *beside* a default `version` key instead of overriding it — silently yielding both, with consumers reading the stale default.
- Rejected: literal `v` in `Params`. Costs a change to all seven consumers plus the defaults YAML key, contradicting the task's explicit "defaults map key stays `version`", and breaks precedence in the interim.

### Bare token means `effort`, with no value validation

- Decision: a bracket segment containing no `=` is `effort=<segment>`. The segment is validated against `isModelIDChar` (the same charset any `key=value` **value** gets), and against nothing else.
- Rationale: `knownParams`' own doc comment states it gates parameter KEYS ONLY, never values — that is what preserves the pinned "adopt a new model with no recompile" property. Effort values (`low|medium|high|xhigh|max`) are engine-validated; `claudeengine` hard-errors on an invalid `--effort`, which `contracts/specs/llm-model-spec.md`'s "Fail loud" section already cites as the intended enforcement point. Adding a second, closed effort vocabulary in modelspec would need a recompile every time a provider adds a tier.
- Rejected: validating the bare token against a closed effort set. Duplicates a vocabulary that lives in the provider engine and can drift from it.
- Rejected: `isIdentChar` for the bare token. Would make `opus[high]` legal but `opus[4.5]` a charset error while `opus[effort=4.5]` is accepted — two charsets for one field.

### Bare and explicit effort in one bracket is a duplicate-key error

- Decision: `opus[high,effort=max]` and `opus[high,max]` both fail with a duplicate-key error naming the canonical key **and quoting both offending segments as written** — e.g. `modelspec: duplicate param key "effort" in spec "opus[high,effort=max]" (segments "high" and "effort=max" both set it)`. The existing `duplicate param key` substring is preserved, so `parse_test.go`'s current assertion still holds.
- Rationale: normalizing each segment to its canonical key *before* the existing duplicate check makes the detection fall out of code already present in `parseBracket`. Silently letting one win would hide an operator's contradiction — the same reasoning the whitespace rule and the whole-spec-replacement precedence rule rest on. Quoting both segments answers the objection that a canonical-key-only message names a spelling the operator never wrote: `opus[v=4.8,v=4.5]` would otherwise report `duplicate param key "version"` against an input containing no `version`, the same defect that rules out folding empty segments into `empty param key` (below). Echoing *only* the as-written spelling is not an option either — for `opus[high,effort=max]` the two segments spell it differently, so there is no single as-written key to name. Naming the canonical key plus both segments is the one form that is complete for every collision.
- Rejected: last-wins and bare-loses. Both pick a winner for a spec that states two answers.
- Rejected: canonical key alone. Names a spelling absent from the input in the `v`/bare cases.
- Note: `opus[v=4.8,version=4.8]` cannot arise — `version=` is no longer a legal bracket key at all (below), so it fails as an unknown key before any duplicate check.

### `version=` is removed, not aliased, and its rejection carries a migration hint

- Decision: `opus[version=4.5]` fails with the existing unknown-param-key error, extended to name the replacement — e.g. `modelspec: unknown param key "version" in spec "opus[version=4.5]" (the bracket version param is spelled "v")`. The hint clause is **conditional on the offending key being exactly `version`**: every other unknown key keeps the unadorned message, so `sonnet[speed=fast]` (`parse_test.go:165-169`) still produces `modelspec: unknown param key "speed" in spec "sonnet[speed=fast]"` and its existing assertion passes untouched.
- Rationale: the task pins this as a rename, not an added alias. Nothing in the repo outside `contracts/specs/llm-model-spec.md`'s own example and `internal/modelspec`'s two test files writes a `version=` bracket, so no production config breaks — but an operator's `webster.yaml` might, and a bare "unknown param key" gives them nothing to act on. The hint is one clause in an error that already exists.
- Rejected: accepting `version=` as an alias for `v=`. Two spellings for one key is exactly the ambiguity the closed-vocabulary design avoids, and the task states `version=` stops being valid.

### Bracket key vocabulary is separate from the canonical param vocabulary

- Decision: add `bracketKeys = map[string]string{"effort": "effort", "v": "version"}` in `modelspec.go` — accepted bracket spelling → canonical `Params` key. `parseBracket` looks up `bracketKeys` instead of `knownParams`. `knownParams` keeps its current canonical membership (`effort`, `version`) and keeps being the vocabulary `load.go`'s `validateAlias` gates `Entry.Defaults` against.
- Rationale: the bracket surface and the `Params`/`Defaults` key space are now genuinely different vocabularies; one map cannot express both. Splitting them keeps `validateAlias` untouched and makes the defaults-key-stays-`version` requirement structural rather than incidental.
- Rejected: redefining `knownParams` to `{effort, v}`. Would force `models.yaml` defaults to `v:` too, contradicting the task.

### `opus[effort]` becomes valid, parsing to `effort=effort`

- Decision: the shorthand rule is uniform — a segment with no `=` is an effort value, even when that value happens to spell a known key. `parseBracket`'s `no '=' separator` error is deleted from the grammar entirely, and with it the `"param with no equals"` case at `internal/modelspec/parse_test.go:175-179`.
- Rationale: a special case rejecting bare tokens that collide with key names buys nothing — `effort=effort` is not a legal effort value and dies loudly at `claudeengine`, one layer down, exactly where every other bad effort value dies. A grammar with one rule is easier to document and to hold in your head than one with an exception.
- Rejected: special-casing bare `effort`/`v` as errors. Adds a rule to the pinned grammar to catch a typo the next layer already catches.

### Empty bracket segments get their own error

- Decision: `opus[high,]`, `opus[,high]`, and `opus[a,,b]` fail with a new `empty param in spec ...` error naming the full spec.
- Rationale: an empty segment used to be caught incidentally by the `no '=' separator` check, which is being deleted. Without a replacement it would parse as `effort=""` and hit the empty-value path with a confusing message, or slip through. `sonnet[=high]` (empty key, `=` present) keeps its distinct `empty param key` error, and `sonnet[effort=]` keeps `empty value for param key`.
- Rejected: folding empty segments into `empty param key`. That message names a key the input does not have.

### The shorthand applies to escape form too

- Decision: `claude:claude-opus-5[high]` parses identically to `claude:claude-opus-5[effort=high]`.
- Rationale: `Parse` splits the bracket off before deciding alias vs escape form and calls one `parseBracket` for both (`parse.go:60-70, 112-118`). There is one bracket grammar, and the spec doc presents it as one. Restricting the shorthand to alias form would mean forking `parseBracket` to gain an inconsistency.
- Rejected: alias form only.

## Technical context

**`internal/modelspec` layout.** `parse.go` (grammar), `registry.go` (`builtins`, `Resolve`), `load.go` (`LoadRegistry`, `validateAlias`), `modelspec.go` (types + `knownParams`/`knownEngines`, no logic), `template.go`/`template.yaml` (seeded `models.yaml`).

**The only function that changes is `parseBracket`** (`parse.go:122-155`). Its current loop, per comma-separated segment: find `=` (absent → error), split key/value, reject empty key, reject empty value, charset-check key as `param key`/`isIdentChar`, charset-check value as `param value`/`isModelIDChar`, reject duplicate, reject unknown key against `knownParams`, store. The new shape keeps every one of those checks for the `key=value` branch and adds a bare-token branch ahead of them; the duplicate check must see canonical keys from both branches.

**Charset helpers already exist** and need no change: `isIdentChar` (lowercase, digits, dash), `isModelIDChar` (that plus `.` and `_`), `validateCharset(s, kind, allowed)` which produces `invalid character %q at position %d in %s %q`. The `kind` string for a bare token should read as a value, not a key — `"param value"` keeps the existing message shape.

**Error style is pinned by test:** every error string starts with `modelspec: ` (asserted for every reject case in `TestParse_Rejects`) and names the offending token or character. `parse.go`'s header comment states each rejection is its own named error.

**No programmatic spec construction exists.** A repo-wide grep for `fmt.Sprintf`-style bracket assembly finds nothing — every model-spec string in the repo is a literal in YAML, a test, or a doc. This is why the blast radius stays inside the four named files: no Go code needs to learn the new spelling to keep emitting valid specs.

**`version=` occurrences repo-wide** (excluding `_mill/`): `contracts/specs/llm-model-spec.md:64,66,112`; `docs/overview.md:302`; `internal/modelspec/parse_test.go:31,46`; `internal/modelspec/registry_test.go:66,69,149,161`. The `registry_test.go` occurrences are `Spec{Params: {"version": ...}}` literals constructed directly, bypassing `Parse` — they test `Resolve`'s precedence against the **canonical** key and therefore stay correct and unchanged under this task's normalize-to-`version` decision. That is a useful signal for the plan: if a `registry_test.go` change looks necessary, the normalization was implemented wrong.

**Doc conventions.** Markdown in this repo uses semantic line breaks — one sentence per line, with breaks at internal independent-clause boundaries; never fixed-column hard wrap. `contracts/specs/llm-model-spec.md` is a pinned contract doc kept (not deleted) on landing per `docs/overview.md`'s Documentation Lifecycle.

## Constraints

- **Modelspec Leaf Invariant** (`CONSTRAINTS.md`): `internal/modelspec` imports only stdlib, `internal/configengine`, and `gopkg.in/yaml.v3`; reverse import never allowed. Enforced by `leaf_enforcement_test.go`'s `TestLeafInvariant_AllowlistOnly`. This change needs no new import at all — it is pure string handling on top of existing helpers.
- **Documentation Lifecycle** (`CONSTRAINTS.md` / `CLAUDE.md`): a change to observable behaviour updates its docs in the same commit. Here that means `contracts/specs/llm-model-spec.md` and `docs/overview.md` land with the code.
- **`manifest/roadmap.md` does not move** — this is a grammar change to a shipped module, not a planned-item completion.
- **Build prerequisite:** `CGO_ENABLED=1` plus a C compiler, since `lyx` links quarry's tree-sitter grammars. Affects running `go test ./...`, not this package's content.
- **Fail-loud discipline** (`contracts/specs/llm-model-spec.md`, "Fail loud"): unknown alias, unknown param key, unrecognized provider → loud rejection, never silent ignoring. Every new rejection path here follows it.
- **Closed vocabularies gate keys and engine names only, never model names, aliases, or param values** — the property that lets a new model be adopted with no recompile.

## Testing

All test work is in `internal/modelspec/parse_test.go`, extending the two existing tables. TDD candidate: write every case below first — the grammar is fully specified by these decisions, so the tests can be authored before `parseBracket` is touched.

**`TestParse_Accepts` — new cases:**

- Bare effort, alias form: `opus[high]` → `Params{"effort": "high"}`.
- Bare effort with `v`, the task's headline combined form: `opus[high,v=4.8]` → `Params{"effort": "high", "version": "4.8"}`. Note the `version` key — this case is the regression guard for the normalization decision.
- `v=` alone: `sonnet[v=4.5]` → `Params{"version": "4.5"}`.
- Bare effort, escape form: `claude:claude-opus-5[high]` → `Params{"effort": "high"}`.
- Bare token spelling a known key: `sonnet[effort]` → `Params{"effort": "effort"}`.
- Bare token with a dot, exercising the value charset: `sonnet[4.5]` → `Params{"effort": "4.5"}`.
- Order independence: `opus[v=4.8,high]` → same map as the headline case.

**`TestParse_Rejects` — new cases:**

- `sonnet[version=4.5]` → substring `unknown param key`, plus a second assertion (or a wider substring) covering the `v` migration hint.
- `opus[high,effort=max]` → `duplicate param key`.
- `opus[high,max]` → `duplicate param key`.
- `opus[high,]` → the new empty-segment substring.
- `opus[,high]` → same.
- `opus[high!]` → `invalid character` (bare token, value charset).
- `opus[high, v=4.8]` → `whitespace character`, pinning that the shorthand did not soften the whitespace rule.

**`TestParse_Accepts` — two existing cases must be rewritten off `version=`.** Both currently assert that a `version=` bracket parses successfully, which the rename makes false. Neither is deleted — each is re-spelled in place, keeping its name and its expected `Params` map (the canonical `version` key is unchanged; only the input string moves to `v=`):

- `"alias multiple params"` (`parse_test.go:29-33`): input `sonnet[effort=high,version=4.5]` → `sonnet[effort=high,v=4.5]`. Expected `Params{"effort": "high", "version": "4.5"}` is unchanged — that invariance is precisely the normalization decision under test.
- `"dotted version value"` (`parse_test.go:44-48`): input `sonnet[version=4.5]` → `sonnet[v=4.5]`. Expected `Params{"version": "4.5"}` unchanged.

These two rewrites plus the one deletion below are the **complete** set of edits to pre-existing cases. If implementation forces a change to any other existing case, something in these decisions was implemented wrong — treat it as a signal, not a test to adjust.

**`TestParse_Rejects` — deleted case:** `"param with no equals"` (`sonnet[effort]`, `internal/modelspec/parse_test.go:175-179`). It moves to the accept table per the uniform-rule decision. Deleting it is the intended, reviewed consequence of that decision, not an oversight.

**Unchanged and expected to stay green:** every case in both tables other than the two rewrites and the one deletion named above — including `"unknown param key"` (`sonnet[speed=fast]`, `parse_test.go:165-169`), whose message keeps its unadorned form because the `v` migration hint fires only for the key `version`. Also `registry_test.go` in full (its `version` literals are canonical-key constructions that never pass through `Parse`), `load_test.go` (`Entry.Defaults` still validates against `knownParams`, which still contains `version`), and `leaf_enforcement_test.go`.

**Verify command:** `go test ./internal/modelspec/...` for the unit work, then `go build ./... && go test ./...` before handoff to catch any consumer that turns out to depend on the deleted `no '=' separator` error string.

## Q&A log

- **Q:** With the bracket key renamed to `v`, what key lands in `Spec.Params` — `v` or the canonical `version`? **A:** [auto-pick] Normalize to canonical `version`. **Why:** seven production sites read `Params["version"]` and `Entry.Defaults` keeps the `version` key; a literal `v` would sit beside the default instead of overriding it, breaking bracket-over-default precedence.
- **Q:** Should the bare effort token be validated against a closed effort vocabulary? **A:** [auto-pick] No — value charset only, same as any `key=value` value. **Why:** `knownParams` gates keys only by design; effort values are engine-validated and `claudeengine` already hard-errors on a bad `--effort`.
- **Q:** What does `opus[high,effort=max]` do? **A:** [auto-pick] `duplicate param key "effort"` error. **Why:** normalizing before the existing duplicate check makes it fall out for free, and it also covers `opus[high,max]`; silently picking a winner would hide a stated contradiction.
- **Q:** Does the bare shorthand apply in escape form (`claude:claude-opus-5[high]`)? **A:** [auto-pick] Yes — one bracket grammar for both forms. **Why:** `Parse` already calls one `parseBracket` for both shapes; restricting it would mean forking the function to gain an inconsistency.
- **Q:** What happens to `opus[version=4.5]` after the rename? **A:** [auto-pick] Unknown-param-key error extended with a hint naming `v`. **Why:** it is a hard rename with no deprecation window, and a bare unknown-key error leaves an operator with a stale `webster.yaml` nothing to act on.
- **Q:** How are the two vocabularies structured? **A:** [auto-pick] New `bracketKeys` spelling→canonical map for `parseBracket`; `knownParams` stays canonical and keeps gating `Entry.Defaults`. **Why:** the bracket surface and the `Params`/`Defaults` key space are now different vocabularies; one map cannot express both without forcing `models.yaml` defaults to `v:`.
- **Q:** `sonnet[effort]` is currently rejected with `no '=' separator`. Now? **A:** [auto-pick] Valid, parsing to `effort=effort`; the `no '=' separator` error and its test case are deleted. **Why:** one uniform rule beats a special case, and the nonsense value dies loudly one layer down where every bad effort value dies.
- **Q:** What catches an empty bracket segment (`opus[high,]`) once `no '=' separator` is gone? **A:** [auto-pick] A dedicated `empty param in spec ...` error. **Why:** the old error caught this incidentally; folding it into `empty param key` would name a key the input does not have.
- **Q:** Rewrite existing `opus[effort=high]` template and config strings to the shorthand? **A:** [auto-pick] No. **Why:** `effort=` stays valid and the task pins a four-file blast radius; a sweep would balloon the diff for zero behaviour change.
- **Q:** Which charset validates the bare token? **A:** [auto-pick] `isModelIDChar`, the value charset. **Why:** otherwise `opus[4.5]` would be a charset error while `opus[effort=4.5]` is accepted — two charsets for one field.
- **Q:** Update docs beyond the four named files? **A:** [auto-pick] Yes — `docs/overview.md:302` and `contracts/specs/llm-model-spec.md:66,112` also mention `version=`. **Why:** leaving stale `version=` prose reintroduces the spelling the task removes.
- **Q:** `docs/reference/model-spec.md` is referenced by two `template.yaml` files but does not exist. Fix it? **A:** [auto-pick] No — note it, leave it. **Why:** pre-existing dangling reference, unrelated to this grammar change.
