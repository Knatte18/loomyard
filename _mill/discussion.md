# Discussion: Build internal/stencil: fill markdown prompt templates

```yaml
task: 'Build internal/stencil: fill markdown prompt templates'
slug: internal-stencil
status: discussing
parent: main
```

## Problem

Every prompt-building call-site in loomyard needs the same mechanical step: take a
markdown template with marker fields, fill the markers from a set of values, and hand
the rendered markdown to `shuttle.Run` as a prompt. Today no shared helper exists, so
each future consumer (`burler`, `perch`, `loom`, `hardener`) would hand-roll its own
substitution — and each hand-rolled version would carry the same latent bug: a marker
left unfilled renders as a silent blank.

That blank is not cosmetic. `fasit` is the load-bearing field of a review profile
(`{fasit, target} → verdict`, not `target → verdict`). A template whose `fasit` marker
rendered empty would quietly neuter a review while still looking like a valid prompt.
`internal/stencil` exists to turn that whole class of bug into a **loud, early failure**:
one shared renderer that refuses to emit a prompt with a hole in it. The substitution
itself is trivial; centralizing the *guard* is the reason the leaf is worth building.

**Why now:** the review-engine design (burler/perch/loom/hardener) pinned stencil's
contract as a shared dependency of all four. stencil is domain-blind plumbing with no
dependency on any of them, so it can and should be built first, ahead of its consumers.

## Scope

**In:**

- A new leaf package `internal/stencil` with a single exported function:
  `Fill(template []byte, values map[string]string) ([]byte, error)`.
- Backing: Go stdlib `text/template`.
- Marker substitution (`{{.X}}`) and equality-based conditional sections
  (`{{if eq .Type "Cluster"}}…{{end}}`).
- The load-bearing guard: an unfilled marker (reached-but-absent, or top-level-empty) is
  a hard error listing every offender, never a silent blank.
- Stripping a leading `<!-- … -->` comment block before parsing.
- Deep, pure, table-driven tests (`internal/stencil/stencil_test.go`).
- Update `docs/shared-libs/stencil.md` to match the built contract (signature
  `map[string]any` → `map[string]string`; pin the chosen `text/template` grammar).

**Out:**

- **No file I/O.** The caller reads the `.md` asset; stencil takes bytes and returns
  bytes (like `yamlengine.Resolve`).
- **No CLI, no cobra command, no engine split.** stencil is a leaf, not a module. The
  CLI/Cobra Invariant and the Sandbox Suite Coverage Invariant do not apply (both
  enumerate registered cobra modules; stencil is never registered).
- **No domain knowledge.** stencil never learns the words "review", "phase", "cluster",
  "bulk", or what a `Type` value may be. The `Type`-discriminator idiom is a *caller*
  convention; stencil only knows `text/template`'s generic `{{if eq .X "y"}}`.
- **No template assets.** The `.md` prompt templates live with their consumers
  (burler/perch/loom/hardener), not in stencil. stencil ships no templates of its own.
- **No custom template funcs, no `html/template` escaping, no partial/nested-template
  loading.** Plain `text/template` execution only.
- The consumers themselves (burler/perch/loom/hardener) are out of scope — they are
  separate, later tasks. stencil is built against its contract, not against a live caller.

## Decisions

### backing-engine — stdlib `text/template`, not `<PLACEHOLDER>` regex

- Decision: back stencil with Go stdlib `text/template` (the `text/` variant, **not**
  `html/template` — markdown prompts must not be HTML-escaped).
- Rationale: the design needs conditional sections (a cluster-reviewer prompt carries a
  section a single-target prompt does not). `text/template` gives `{{if}}`/`eq` for free.
  Bolting conditionals onto mill-style `<TOKEN>` regex substitution would mean
  reinventing a worse template engine.
- Rejected: **plain `<PLACEHOLDER>`/`<TOKEN>` substitution** (mill's `_render.py` style,
  `<[A-Z][A-Z0-9_]*>` regex). It is simpler and makes the missing-marker guard trivial,
  but it has no conditionals — variants would each need a separate template file. We keep
  its good ideas (collect-all-missing-then-fail, sorted error, leading-comment strip) but
  choose the engine that supports conditionals.

### conditionals — single string `Type` discriminator, values stay `map[string]string`

- Decision: conditional sections switch on a single string discriminator via the built-in
  `eq`: `{{if eq .Type "Cluster"}}…{{end}}`. Values remain `map[string]string`.
- Rationale: a template is always *for* a known kind of prompt. Driving conditionals off
  one string `Type` field (rather than a scatter of independent boolean flags) is the
  clean idiom **and** keeps the value bag `map[string]string`: a string discriminator
  needs no `bool`, so nothing forces `map[string]any`. stencil stays domain-blind — it
  supports generic `{{if eq .X "y"}}`; the "one `Type` field per template" convention is
  the caller's, not baked into the leaf.
- Rejected: **boolean flags** (`{{if .Cluster}}` with `Cluster bool`). Works, but forces
  `map[string]any` and scatters N ad-hoc flags where one discriminator reads cleaner.
- Rejected: **no conditionals at all** (one `.md` file per variant, mill-style). Airtight
  and simplest, but pushes variant-selection onto every caller and forbids inline
  sections the design calls for.

### signature — `Fill(template []byte, values map[string]string) ([]byte, error)`

- Decision: `[]byte` in, `[]byte` out, `map[string]string` values, no I/O.
- Rationale: matches the leaf convention (`yamlengine.Resolve(src []byte, …) ([]byte,
  error)` does no I/O; the caller owns reading the asset). `map[string]string` is tighter
  than the design doc's original `map[string]any` and, with the string-`Type` conditional
  idiom, `any` buys nothing while forcing a stringification rule no caller needs.
- Rejected: **reading from a `Path`** (mill's `render(template_path, values)`). Convenient
  but pushes file I/O into the leaf, against the Go leaf pattern and the pinned signature.
- Rejected: **`map[string]any`** (the design doc's original). Needs a defined stringify
  rule for non-string values that, without boolean flags, no caller passes. This decision
  supersedes the doc; `docs/shared-libs/stencil.md` is updated in the same commit.

### the load-bearing guard — reached-but-absent OR top-level-empty → hard error

- Decision: `Fill` returns an error if any marker is unfilled. "Unfilled" means either:
  1. **reached-but-absent** — a field the executed template actually references is not
     present in `values`; enforced by `text/template`'s `Option("missingkey=error")`,
     which naturally scopes to *taken* branches (a `{{.ClusterName}}` inside an untaken
     `{{if eq .Type "Cluster"}}` branch is never reached, so it is not required when not
     clustering); **and**
  2. **top-level-empty** — a substitution `{{.X}}` at the top level of the template (not
     nested inside any `{{if}}/{{with}}/{{range}}`) resolves to an empty or
     whitespace-only value. Enforced by a parse-tree walk over depth-0 action nodes,
     checking `strings.TrimSpace(values[X]) != ""`.
- Rationale: (2) is the empty-`fasit` guard — the whole reason the leaf exists. `fasit`
  and `target` are always top-level substitutions, so they are fully guarded. Optional
  content lives inside conditional branches and is caller-owned (that is the point of the
  branch). (1) catches typo'd markers and a forgotten `Type` discriminator (the `{{if eq
  .Type …}}` is always reached, so an absent `Type` errors).
- Error shape: **collect all offenders, fail once, sorted alphabetically** (mill's
  collect-then-raise pattern), so one `Fill` call reports every hole deterministically
  rather than failing one-at-a-time. Plain `fmt.Errorf` — no sentinel/typed error (leaf
  convention: `yamlengine`/`output` use plain errors; no consumer needs to branch on it).
- Rejected: **`missingkey=error` alone** — does not catch a present-but-empty value, so an
  empty `fasit` would still render blank. Fails the stated guarantee.
- Rejected: **post-render scan for `<no value>`** — fragile (real content could contain
  the sentinel) and still misses empty values.

### leading-comment strip — port mill's behavior

- Decision: a `<!-- … -->` comment block at the very start of the template (after optional
  leading whitespace) is stripped **before** parsing/execution. Mid-template comments are
  preserved verbatim.
- Rationale: lets template authors annotate an asset (authoring notes, provenance) without
  the annotation's `{{…}}` being parsed or its markers checked. Proven in mill's
  `_render.py`; cheap to port. Stripping *before* parse matters — otherwise `text/template`
  would execute `{{…}}` inside the comment.
- Rejected: dropping it as YAGNI — small surface saved, but the annotation affordance is
  useful and the cost is ~10 lines.

## Technical context

- **Where it lives:** `internal/stencil/stencil.go` + `internal/stencil/stencil_test.go`.
  Single-file leaf, same shape as `internal/output/` (`output.go` + `output_test.go`) and
  `internal/state/`.
- **Package name:** `stencil`. The name deliberately avoids two collisions: `render`
  (already `muxengine/render`, strands→layout) and `template` (already
  `configreg.ConfigTemplate()`, the config default). See `docs/shared-libs/stencil.md`.
- **Pattern to follow:** `internal/yamlengine/resolve.go` is the closest sibling — an
  I/O-free `([]byte, map) → ([]byte, error)` transform with a hard error on an unset
  required marker and godoc explaining the empty-vs-absent distinction. Mirror its
  godoc density and error style (`fmt.Errorf("…: %w", err)` for wrap, plain
  `fmt.Errorf` for the marker error).
- **Reference implementation:** mill's
  `c:/Code/millhouse/wts/millhouse/plugins/mill/scripts/_render.py` does the same job in
  Python. Borrow its three good ideas — collect-all-missing-then-fail, sorted error
  message, leading-comment strip — and go **beyond** it on the empty-value guard (mill's
  `render` errors on an absent key but silently substitutes a present-but-empty value;
  stencil must reject the empty value too).
- **`text/template` mechanics the plan will use:**
  - Parse: `template.New("stencil").Option("missingkey=error").Parse(string(body))`.
  - The parse-tree walk for the top-level-empty check reads `tmpl.Tree.Root.Nodes` and
    inspects `*parse.ActionNode` whose pipe is a bare `*parse.FieldNode` (a `{{.X}}`
    substitution). Only depth-0 nodes are checked; nodes inside `*parse.IfNode`/`WithNode`
    /`RangeNode` bodies are skipped (caller-owned). The field name is
    `FieldNode.Ident[0]`.
  - Execute into a `bytes.Buffer`; a `missingkey=error` miss surfaces as an execution
    error whose message names the missing key — parse it (or pre-check presence against
    the collected reached-field set) to fold into the collected-offenders list so the
    single sorted error covers both absent and empty causes.
- **Consumers (not built here, for context only):** `burler` (handler + cluster-reviewer
  prompts; bulk blob passed as a *value*, Go-assembled, not read via tools), `perch`
  (progress-judge prompt), `loom` (discussion/plan producer prompts), `hardener` (DRAFT;
  round-agent prompt). All four go through `Fill`; templates are their `.md` assets.

## Constraints

From `CONSTRAINTS.md` (hub root) and the design:

- **Hub Geometry Invariant** — stencil constructs no paths and touches no cwd/geometry
  tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, `_lyx`). It
  is I/O-free, so this invariant is trivially satisfied; do not introduce any path work.
- **CLI / Cobra Invariant** — **does not apply.** stencil is a leaf, not a registered
  cobra module. It exposes no `Command()`/`RunCLI`, is not wired into `newRoot()`, and
  contributes nothing to `root.Long`. Do not add a CLI seam.
- **Sandbox Suite Coverage Invariant** — **does not apply.** The coverage test enumerates
  the live cobra root's commands; stencil is never registered, so it needs neither a
  sandbox scenario nor an `excludedModules` allowlist entry.
- **lyxtest Leaf Invariant** — stencil must remain a leaf: it imports only the standard
  library (`text/template`, `text/template/parse`, `bytes`, `fmt`, `strings`, `sort`). It
  imports no feature package and no `configreg`. Its own tests need no substrate.
- **Documentation Lifecycle** — this task changes a named lib's design surface (the
  signature and grammar), so `docs/shared-libs/stencil.md` **must** be updated in the same
  commit. `docs/overview.md`'s module/execution-stack tables change only if they list
  shared libs (check and update if so). `docs/roadmap.md`: if stencil is a listed planned
  milestone, mark it ✅ Done with a link to the shared-lib doc; otherwise leave the roadmap
  untouched (this is planned-milestone delivery, not a bugfix).

## Testing

Pure, table-driven, no substrate — stencil owns deep tests like every shared lib. TDD is
a strong fit here: the guarantee is the product, so write the guard tests first.

Scenarios that must be covered:

- **Happy path** — template with several `{{.X}}` markers, all values present and
  non-empty → correct substituted output, no error.
- **Missing-marker (reached-but-absent)** — a referenced top-level marker absent from
  `values` → error naming that marker.
- **Empty-value (the load-bearing case)** — a top-level marker present but `""` → error;
  and present but whitespace-only (`"   "`) → error. This is the empty-`fasit` guard.
- **Multiple offenders collected & sorted** — two+ unfilled markers → single error listing
  all of them in deterministic sorted order.
- **Conditional taken** — `{{if eq .Type "Cluster"}}…{{end}}` with `Type: "Cluster"` →
  section present; markers inside it (`Index`, `Total`) substituted.
- **Conditional not taken** — same template with `Type: "Single"` (or `Type` any
  non-matching value) → section absent, and markers that live *only* inside that branch
  (e.g. `ClusterName`) are **not** required — no error even though they are absent from
  `values`.
- **Forgotten discriminator** — template references `{{if eq .Type …}}` but `values` has
  no `Type` key → error (the `if` is always reached, so `missingkey=error` fires).
- **Unused values ignored** — `values` carries keys the template never references → no
  error, output unaffected.
- **Leading-comment strip** — leading `<!-- … -->` dropped before render; a marker/`{{…}}`
  inside that leading comment is neither substituted nor checked (no error); a
  mid-template comment is preserved verbatim; a comment-only template → empty output.
- **Empty / whitespace-only template** → empty output, no error.
- **Idempotence / determinism** — same template + values → byte-identical output across
  runs; the collected error message is sorted so it too is stable.
- **No HTML escaping** — a value containing `<`, `>`, `&`, or quotes passes through
  verbatim (confirms `text/template`, not `html/template`).

## Q&A log

- **Q:** Marker engine — `text/template` or mill-style `<PLACEHOLDER>`? **A:** `text/template`, because conditional sections are a real near-term need; keep mill's collect-all/sorted-error/leading-comment ideas but choose the engine with conditionals.
- **Q:** How are conditional (`if`) values set — scattered bools like `.Cluster`? **A:** No — a single string `Type` discriminator via `{{if eq .Type "Cluster"}}`. A template is always *for* a known type, and a string discriminator keeps values `map[string]string` (no bool → no `map[string]any`).
- **Q:** Empty value — match mill (only absent key errors) or go beyond? **A:** Go beyond. A top-level substitution that is empty or whitespace-only is a hard error; that empty-`fasit` guard is the reason the leaf exists.
- **Q:** How is the empty-guard scoped so a legitimately-off conditional isn't flagged? **A:** Non-empty is enforced only for **top-level** substitutions (not nested in an `if`/`with`/`range`); `missingkey=error` covers reached-but-absent and naturally skips untaken branches. `fasit`/`target` are top-level, so fully guarded.
- **Q:** Value type — `map[string]string` or the doc's `map[string]any`? **A:** `map[string]string`; the string-`Type` idiom means no non-string value is ever passed. Update the design doc's signature in the same commit.
- **Q:** Input — `[]byte` or read from a path like mill? **A:** `[]byte`, I/O-free, per the Go leaf convention (`yamlengine.Resolve`).
- **Q:** Port mill's leading-comment strip? **A:** Yes — strip a leading `<!-- … -->` before parsing so annotations with `{{…}}` aren't executed or checked.
- **Q:** Error type — sentinel/typed or plain? **A:** Plain `fmt.Errorf`, per leaf convention; no consumer needs to branch on it.
