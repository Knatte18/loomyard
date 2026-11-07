# LLM model-spec — provider/model/parameter notation

> **Status: Contract — pinned.** The notation every agent-spawning config in the stack uses to say *which* LLM runs a role: webster's roles, burler reviewers and Bouncer judges, loom's producers. The plan itself is model-agnostic (see `contracts/stencils/loom/loom-template-plan.md`), so the config side needs a precise notation of its own. The registry loader and spec parser implement this spec.

## Grammar

```
<alias>[item,item,...]        where item is  <effort>  |  key=value
```

- The alias is one word, resolved via the [registry](#the-registry--modelsyaml).
- The bracket part is optional;
  the canonical short form is `model[effort]`, where the bare token in the bracket names the effort.
- The long form `effort=<level>` stays parseable and means the same as the bare token.
- A parameter other than effort, such as `v=4.5`, keeps its `key=value` spelling, since the short form names effort alone.

```yaml
implementer: sonnet                    # registry defaults apply
implementer: sonnet[high]              # override effort
reviewer:    opus[max]
```

**Escape form** for models not (yet) in the registry — no registry edit needed to try a new model id:

```
<provider>:<model-id>[item,item,...]        where item is the same shape as above
```

```yaml
implementer: claude:claude-sonnet-4-5[high]
```

## The registry — `models.yaml`

A dedicated config file, readable and editable on its own.
It resolves via `configengine.ConfigFile`, like all module config under `_lym/config/`.
Each entry maps an alias to:

- **engine** — which shuttle provider engine the alias requires (e.g. `claude`),
- **model** — the model string passed to that engine,
- **defaults** — values for optional parameters (effort, …) used when a spec's bracket doesn't set them.

```yaml
# models.yaml
sonnet:
  engine: claude
  model: sonnet          # provider-side alias — resolves to newest (see below)
  defaults:
    effort: medium
opus:
  engine: claude
  model: opus
  defaults:
    effort: high
```

**Built-in fallback:** Go ships a built-in default set of aliases for the claude and codex engines,
so everything works with **no file present**,
and a hub whose operator-owned `models.yaml` predates an alias still resolves it.
The Claude built-ins carry **no parameter defaults**;
operator defaults (e.g. `effort`) live only in the seeded `models.yaml`.
The Codex built-ins carry `effort: medium`, so a Codex spec that names no effort never launches at the provider's own default.
`models.yaml` overrides and extends the built-in set when it exists.

## Newest by default; pinning is deliberate

An alias tracks the **newest** model.
The registry passes the provider-side alias through, so there is no version-number maintenance treadmill.
The Claude CLI resolves `--model sonnet` to the newest Sonnet itself.
Pinning is always an active choice:

- **In the registry** — set an explicit model id, such as `model: claude-sonnet-5`.
  Use it to steer away from a fresh release you don't trust yet.
  It is one line in one file.
- **Per spec** — `sonnet[v=4.5]`.
  The provider engine translates the generic `version` param to its own id scheme.
  claudeengine's rule is generic over any bare single-word model value, not a closed alias list.
  An operator-added alias therefore translates on an old binary with no recompile:
  `sonnet` + `4.5` → `claude-sonnet-4-5`, `fable` + `5` → `claude-fable-5`.
  Combining `v=` with a full model id (one containing a dash, e.g. the escape form) is a hard error:
  the id already pins its own version, so a second pin is a contradiction.
  Provider naming conventions live in the provider engine only — see [Provider seam](#provider-seam).

**Reproducibility trade-off:** the same plan run a month apart may hit different models.
Mitigation: engines record the **resolved model id** in the run artifacts (RunDir).
Any historical run can then be reconstructed and, if needed, re-pinned.

## Precedence — whole-spec replacement

The most specific config layer that sets a role wins **as a unit**:

```
loom's config section  >  the module's own config (e.g. webster.yaml)  >  (unset)
```

There is **no per-parameter merging across layers** — a losing spec contributes nothing.
Within the winning spec: **bracket param > registry default**.

Example — which effort does webster's `master` role run at?

```yaml
# models.yaml:    sonnet defaults effort=medium
# webster.yaml:   master: sonnet[high]
# loom config:    webster: { master: sonnet }
```

Loom set the role, so loom's spec wins whole.
`sonnet` has no bracket, so no effort is set and the effort comes from the registry default → **medium**.
webster.yaml's `[high]` is irrelevant because its entire spec lost.
Per-param merge is not done: a reader would need three files to know one param's value,
and a stale bracket in an old layer would leak in invisibly.

Sharp edge, by design: overriding a role in a higher layer **silently discards** the lower layer's bracket params.
In the example above, webster.yaml's `[high]` vanished.
If a param must survive your override, restate it in the winning spec.

## Fail loud

Unknown alias, unknown param key, unrecognized provider → loud rejection, never silent ignoring.
Same discipline as the plan `format:` check;
claudeengine hard-errors on an invalid `--effort` for the same reason.

## What is *not* a parameter

- **`context`** — context-window size is not tunable for Claude models.
  A role that needs a large window points at a model/variant that *has* one;
  how that variant is realized is the provider engine's business.
  Ollama-style `num_ctx` tuning is out of scope until a non-Claude engine exists.

## Provider seam

Registry data is provider-invariant (alias → engine name + model string + param defaults).
Everything provider-*specific* lives in the provider engine, per `PATTERN-shuttle-provider-seam`:
CLI flags, `version` id translation and large-window variant realization.
For Claude, that engine is `internal/shuttleengine/claudeengine`.

## Roles that use this notation

webster.yaml holds one model-spec per role.
`master` is the long-lived Master session.
Its in-session forks always inherit Master's current model, so there are no per-fork roles.
The cold plan fixer `fix-plan` spawns runs on `master` too.
`recovery` is the cold recovery strand `recover-batch` spawns, and the cold verify-gate fixer `fix-verify` spawns.
Stack-wide roles elsewhere (burler reviewers, Bouncer judges, loom producers) use the same notation.
Each lives in its own config section, and loom's config section overrides per role when loom drives the module.

loom.yaml gives the Bouncer judge and seed passes keys of their own, each a model-spec validated only when non-empty:

- A per-segment judge key (`discussion_judge`, `code_judge`) left empty takes `judge`.
- A per-segment seed key (`discussion_seed`, `code_seed`) left empty takes `seed`, and `seed` left empty takes that segment's resolved judge.

tapestry.yaml gives the plan segment's Bouncer passes `plan_judge` and `plan_seed`, each validated only when non-empty.
An empty `plan_judge` takes loom.yaml's `judge`.
An empty `plan_seed` takes loom.yaml's `seed`, and that left empty takes the segment's resolved judge.

landing.yaml's `fixer` is the model-spec of the merge fixer session that Parent-Align and Finalize run.
It also serves the fix review session Finalize runs over an over-cap fixer change, and defaults to `opus[high]`.
`fixer_timeout_min` bounds one of the fixer's turns or the fix review session.
