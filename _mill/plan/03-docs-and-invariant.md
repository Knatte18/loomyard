# Batch: docs-and-invariant

```yaml
task: "config degrades to embedded template"
batch: "docs-and-invariant"
number: 3
cards: 2
verify: go test ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

This batch lands the documentation half of the task in the same commit range as the code, per CLAUDE.md's same-commit docs rule: the `LoadOrTemplate` section and stale-claim corrections in `docs/shared-libs/configengine.md`, and the new Config Strictness Invariant in `CONSTRAINTS.md`.
It is one batch because both files are prose describing the same new two-policy split, and because both are scanned by the same guard — `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` — so they verify together.

It depends on batch 1 alone, not batch 2: everything documented here is determined by `configengine`'s new exported surface plus the caller sets the discussion already pins, and no file in this batch overlaps any file in batch 2.

Batch-local decision beyond `## Shared Decisions`: `docs/shared-libs/configengine.md` contains zero inline markdown links today, so every link this batch adds is newly exposed to `TestEnforcement_MarkdownLinks`, which resolves both the relative file path and the `#anchor`.
Any link added must resolve in both halves;
where an anchor cannot be made to resolve, a bare mention is acceptable instead.

## Cards

### Card 8: `docs/shared-libs/configengine.md` — `LoadOrTemplate` section and stale-claim corrections

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/configengine/config.go`
  - `internal/lyxcwd/docslink_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update this file's claims that the new degrading path makes stale, and fix three claims that are already false today, in one pass.
  Each item below names the claim by its current wording, so the implementer locates it by text rather than by a line number that shifts as edits land.

  Claims made stale by this task, which must be **updated in place**, not merely appended to:

  1. The Layout section's sentence "`_lyx/` presence is what makes a directory 'initialised'; if it is absent, `configengine` errors" — qualify it as true of `Load` only, and state that `LoadOrTemplate` resolves the caller's embedded template instead.
  2. The Resolution model section's six-step flow, presented as the package's only loading path — reframe it as the strict path, shared by both entry points whenever a config file is present, and describe the degrading path's divergence: it skips step 3 (`yamlengine.MissingKeys`) and enters at step 4 with the template bytes in place of the file bytes.
  3. The Key properties bullet "**Errors are strict**: missing template keys, absent files, or unset required env vars cause hard errors" — the *absent files* half stops being true under `LoadOrTemplate`;
     the missing-template-keys and unset-required-env-var halves stay true on both paths.
  4. The `FindBaseDir` section's error-message list and its following "Note on error rewrapping" block — document `ErrNotInitialized` here: state that the absent-`_lyx/` error wraps the exported sentinel, that `errors.Is(err, configengine.ErrNotInitialized)` is the supported way to detect absence, that a stat failure deliberately does not wrap it, and that the substring match remains supported for the callers still using it.
  5. The "What it returns" section's sentence "Typed wrappers (`board.LoadConfig`, `worktree.LoadConfig`, `weft.LoadConfig`) unmarshal this into their own config structs" — no such three wrappers exist;
     name the real callers instead.
  6. The `Load` section's phrase "Implements the five-step flow described in the Resolution model section above", which contradicts the six-step list in that same section — make the two counts agree.
  7. The `Load` section's per-error-case list — leave it describing `Load`, and add a sibling `### LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)` section rather than editing the `Load` cases in place.
     The new section states: the signature is identical to `Load`;
     a provably-absent `_lyx/` directory or a provably-absent config file resolves the caller's template through `envsource.Build` then `yamlengine.Resolve`;
     `yamlengine.MissingKeys` is skipped on that path because the bytes being resolved are the template itself;
     a config file that exists but is invalid still errors exactly as under `Load`;
     any non-absence failure — a stat error, a permission or IO read error — propagates unchanged;
     and fallback-path errors are keyed on the module as `<module> config template: <underlying error>`, never on a config-file path that does not exist.

  Claims already false today, fixed in the same pass because they sit inside or beside the ranges above:

  8. The `### LyxDirName (constant, "_lyx")` section, which asserts `internal/configengine` exports the token and is "the single declarer of this token".
     No such export exists — `internal/configengine/config.go` uses `lyxdirs.LyxDirName` — and the claim contradicts the live Lyxdirs Single-Declarer Invariant, which names `internal/lyxdirs` as sole declarer.
     Correct it to name `internal/lyxdirs`, and prefer an inline markdown link to `CONSTRAINTS.md`'s Lyxdirs Single-Declarer Invariant over a bare mention.
     The link's file part is `../../CONSTRAINTS.md` from this file's directory and its anchor is `#lyxdirs-single-declarer-invariant`, derived from the heading `## Lyxdirs Single-Declarer Invariant` by the slug rule `internal/lyxcwd/docslink_test.go` implements;
     use a bare mention instead if the anchor turns out not to resolve.
  9. The `FindBaseDir` error-message list's quoted text `not initialized: _lyx/ directory not found in <dir>` — the source emits no ` in <dir>` suffix.
     Quote the real message.
  10. The rewrap note's example remedy `run "lyx init"` — every caller emits `run "lyx fabric reconcile"`.

  Keep the existing Environment variable grammar, `.env` loading, Migration, `ConfigDir`, `ConfigFile`, and `Set` sections as they are;
  none of them changes.
  Markdown follows semantic line breaks: one sentence per line, plus a break at internal independent-clause boundaries, with table cells on one line.
- **Commit:** `docs(configengine): document LoadOrTemplate and correct stale loader claims`

### Card 9: `CONSTRAINTS.md` — Config Strictness Invariant

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `docs/shared-libs/configengine.md`
  - `internal/configengine/config.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Config Strictness Invariant` section to `CONSTRAINTS.md`, placed among the review-obligation invariants near `## Planparser Sole-Parser Invariant`, `## Batcher Registry+Config Invariant`, and `## Producer Pointer-Rule Invariant`, and matching their shape: a one-or-two-sentence rule statement, a short bullet list, and a closing `**Enforced by**` bullet.

  The section must record all of the following:

  - **The rule.** `internal/configengine` offers two loading policies and a caller adopts exactly one.
    `Load` is strict: an absent `_lyx/` directory or an absent config file is an error.
    `LoadOrTemplate` degrades: either absence resolves the caller's embedded template instead.
  - **The membership rule**, stated as a predicate a future caller can apply rather than a bare list: a module belongs on the degrading side when it has, or is slated to have, a **standalone entry point** — a way to be invoked outside a lyx hub — because a config-less invocation is then a supported mode.
    A module that only ever runs inside a hub stays strict, because there an absent config means the hub is broken.
  - **The two pinned sets** as they stand today: degrading is `{shuttleengine, reedengine, perchengine, websterengine}`;
    strict is `{fabricengine, boardengine, loomengine, batcher}`.
  - **A third class, explicitly outside this invariant's guard subject: own-loader modules.**
    These never call either entry point — they resolve the path with `configengine.ConfigFile` and read the file themselves with their own absent-file fallback.
    Name all three members and their behaviour: `internal/burlerengine` (`burler.yaml`, absent file returns a zero `Config`, bypassing `Load` because `MissingKeys` would misfire on its open-ended lenses/fans key set), `internal/modelspec` (`models.yaml`, absent file returns `builtins()`; it cannot call a logging `Load` at all, being capped by the Modelspec Leaf Invariant), and `internal/scoutengine` (`servers.yaml`, absent file returns `builtins()`).
    State that all three already have the degrading behaviour and are deliberately not repointed, and that a set-equality grep over the two entry-point tokens is structurally blind to them — without this clause the invariant would read as though the two pinned sets enumerate every module config in the repo, which they do not.
  - **Absence is typed, not textual.** `FindBaseDir` wraps the exported `configengine.ErrNotInitialized` sentinel on its absent-`_lyx/` branch and deliberately does not wrap it on a stat failure, so a degrading caller falls back only on `errors.Is(err, ErrNotInitialized)`.
    Note that the four strict callers still use the older `strings.Contains(err.Error(), "not initialized")` rewrap, that the sentinel makes migrating them possible, and that the migration is available rather than done.
  - **A watch item for T7/T10:** `batcher` sits on the strict side because it has no standalone entry of its own, but its config is read on webster's batching path.
    If a standalone Webster reaches `batcher.Active`, `batcher` moves to the degrading side and these pinned sets change with it.
  - **`Enforced by` line:** review obligation today, with a set-equality grep guard named as a candidate and T10 named as its home.
    Record the guard's shape so T10 inherits a specification rather than re-deriving one: following `cmd/lyx/gitrepoboundary_test.go`'s pinned-set style, walk non-test `*.go` files under the module root, collect every package directory containing a `configengine.Load(` call and every one containing a `configengine.LoadOrTemplate(` call, compare each collected set against its pinned set, exclude `internal/configengine` itself as the declaration site, skip `_test.go` files, and — since resolving the scan root through `go env GOMOD` spawns a process — allowlist the new guard in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map as its four siblings do.
  - **The guard's blind spot**, stated the way the existing guards state theirs: a substring scan cannot see a call reached through an alias or a function value.

  Do not create or edit any file under `cmd/lyx/` in this card;
  the guard itself is deferred to T10.
  Do not modify any other invariant section in this file.
  Markdown follows semantic line breaks: one sentence per line, plus a break at internal independent-clause boundaries.
- **Commit:** `docs(constraints): add the Config Strictness Invariant`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs the package that hosts both markdown guards this batch is exposed to: `docslink_test.go`'s `TestEnforcement_MarkdownLinks`, which resolves every inline link's file part and `#anchor` across `docs/` and the repo-root markdown it scans, and `enforcement_test.go`'s `TestEnforcement_GeometryLiterals`, which would catch a `"_lyx"` literal introduced in a path-construction context.
Card 8 adds this repo's first inline link inside `docs/shared-libs/configengine.md`, so the link guard is the gate that matters most here.

This batch is otherwise pure prose with no runnable surface — no Go file is edited, so nothing in `internal/configengine` or the four producer packages can regress from it.
Correctness of the prose itself is a review obligation, which is exactly the enforcement level the new invariant declares for itself.
