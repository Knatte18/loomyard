# Model-spec — provider/model/parameter notation

> **Status: Contract — pinned.** The notation every agent-spawning config in the stack
> uses to say *which* LLM runs a role: builder's roles, perch/burler reviewers and
> judges, loom's producers. Pinned alongside [plan-format v2](plan-format.md)
> because the plan is model-agnostic — so the config side needs a precise notation.
> The registry loader and spec parser land with the first consumer (`builder`); this doc
> is the spec they implement against.

## Grammar

```
<alias>[key=value,key=value,...]
```

- The alias is one word, resolved via the [registry](#the-registry--modelsyaml).
- The bracket part is optional; each `key=value` overrides that parameter for this spec
  only.

```yaml
implementer: sonnet                    # registry defaults apply
implementer: sonnet[effort=high]       # override one param
reviewer:    opus[effort=max]
```

**Escape form** for models not (yet) in the registry — no registry edit needed to try a
new model id:

```
<provider>:<model-id>[key=value,...]
```

```yaml
implementer: claude:claude-sonnet-4-5[effort=high]
```

## The registry — `models.yaml`

A dedicated config file (resolved via `hubgeometry.ConfigFile`, like all module config
under `_lyx/config/`), readable and editable on its own. Each entry maps an alias to:

- **engine** — which shuttle provider engine the alias requires (e.g. `claude`),
- **model** — the model string passed to that engine,
- **defaults** — values for optional parameters (effort, …) used when a spec's bracket
  doesn't set them.

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

**Built-in fallback:** Go ships a small built-in default set (`sonnet` / `opus` /
`haiku` / `fable` → claude engine) so everything works with **no file present**;
built-ins carry **no parameter defaults** — operator defaults (e.g. `effort`) live only
in the seeded `models.yaml`, never baked into Go. `models.yaml` overrides and extends
the built-in set when it exists.

## Newest by default; pinning is deliberate

An alias tracks the **newest** model: the registry passes the provider-side alias
through (the Claude CLI resolves `--model sonnet` to the newest Sonnet itself), so there
is no version-number maintenance treadmill. Pinning is always an active choice:

- **In the registry** — set an explicit model id (e.g. steer away from a fresh release
  you don't trust yet: `model: claude-sonnet-5`). One line in one file.
- **Per spec** — `sonnet[version=4.5]`. The provider engine translates the generic
  `version` param to its own id scheme. claudeengine's rule is generic over any
  bare single-word model value — not a closed alias list, so an operator-added alias
  translates on an old binary with no recompile (`sonnet` + `4.5` → `claude-sonnet-4-5`,
  `fable` + `5` → `claude-fable-5`). Combining `version=` with a full model id (one
  containing a dash, e.g. the escape form) is a hard error: the id already pins its own
  version, so a second pin is a contradiction. Provider naming conventions live in the
  provider engine only — see [Provider seam](#provider-seam).

**Reproducibility trade-off, signed off:** the same plan run a month apart may hit
different models. Mitigation: engines record the **resolved model id** in the run
artifacts (RunDir), so any historical run can be reconstructed and, if needed, re-pinned.

## Precedence — whole-spec replacement

The most specific config layer that sets a role wins **as a unit**:

```
loom's config section  >  the module's own config (e.g. builder.yaml)  >  (unset)
```

There is **no per-parameter merging across layers** — a losing spec contributes nothing.
Within the winning spec: **bracket param > registry default**.

Example — which effort does builder's implementer run at?

```yaml
# models.yaml:    sonnet defaults effort=medium
# builder.yaml:   implementer: sonnet[effort=high]
# loom config:    builder: { implementer: sonnet }
```

Loom set the role, so loom's spec wins whole: `sonnet`, empty bracket → effort comes from
the registry default → **medium**. builder.yaml's `effort=high` is irrelevant because its
entire spec lost. (Per-param merge was rejected: you'd read three files to know one
param's value, and a stale bracket in an old layer leaks in invisibly.)

Sharp edge, deliberate: overriding a role in a higher layer **silently discards** the
lower layer's bracket params — as above, where builder.yaml's `effort=high` vanished. If
a param must survive your override, restate it in the winning spec.

## Fail loud

Unknown alias, unknown param key, unrecognized provider → loud rejection, never silent
ignoring. Same discipline as the plan `format:` check; claudeengine already hard-errors
on an invalid `--effort` for exactly this reason.

## What is *not* a parameter

- **`context`** — context-window size is not tunable for Claude models. A role that
  needs a large window (builder's `implementer_oversized`) points at a model/variant
  that *has* one; how that variant is realized is the provider engine's business.
  Ollama-style `num_ctx` tuning is out of scope until a non-Claude engine exists.

## Provider seam

Registry data is provider-invariant (alias → engine name + model string + param
defaults). Everything provider-*specific* — CLI flags, `version=` id translation,
large-window variant realization — lives in the provider engine
(`internal/shuttleengine/claudeengine`) per the Shuttle Provider-Seam Invariant in
`CONSTRAINTS.md`.

## Roles that use this notation

builder.yaml holds four roles, each a model-spec: `orchestrator`, `implementer` (Sonnet
default), `implementer_oversized`, `recovery`. There is no builder `evaluator` — the LLM
orchestrator judges digests itself. Stack-wide roles elsewhere (perch/burler reviewers
and judges, loom producers) use the same notation in their own config sections; loom's
config section overrides per role when loom drives the module.
