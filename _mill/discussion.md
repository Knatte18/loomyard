# Discussion: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift

```yaml
task: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift
slug: config-set-key-loss
status: discussing
parent: main
```

## Problem

`lyx config <module> --set key=value` silently destroys any pre-existing config key
that isn't in the command's current template-derived known-key set, with zero warning.
Reproduced from the 2026-07-02 sandbox run (S4): `lyx config board --set
proposal_prefix=s4test-` (touching only `proposal_prefix`) caused the unrelated `path`
key to vanish entirely from `_lyx/config/board.yaml` — confirmed as real data loss (not
a stale-worktree artifact) against a sibling, untouched host↔weft pair on the same
template version that still has `path` present.

This is not hypothetical drift: `path` was deliberately removed from
`internal/boardengine/template.yaml` in commit `bac0e54` ("Harden the Path Invariant:
close enforcement hole + fix geometry leaks", 2026-06-30) — three days before this bug
was filed — as part of making the board directory structural
(`hubgeometry.BoardDir`/`--board-path`) rather than config-stored. Any `_lyx/config/board.yaml`
scaffolded before that commit still carries the legacy `path:` key on disk, and never
gets cleaned up unless `lyx config reconcile --apply` is run. `--set` hits this stale
key and drops it with no signal at all — the CLI's own success message
(`"edited and synced _lyx/config/board.yaml"`) gives no indication anything was lost.

**Empirical finding that reframes the original bug report:** the report also claimed
`lyx config reconcile` fails to detect this drift, both before and after `path`
vanished. This was verified **false** for the "before" case via a direct unit-level
repro against `yamlengine.Reconcile` and `boardengine.ConfigTemplate()`: given
`existing = "path: ../_board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"`,
`Reconcile` correctly returns `removed: ["path"]`, and `configcli.runReconcile` already
wires `Result.Removed` into its JSON envelope (`"removed": [...]`). Reconcile's
detection logic is correct. The likely explanation for the "after" observation: by the
time reconcile was run, `--set` had already silently stripped `path` from disk, so
there was nothing left to detect — reconcile wasn't blind, it was too late. See
Decisions → `fix-scope`.

A second, unrelated but adjacent defect was found by inspection while reading the
affected file: `configcli.go`'s `editOne` (the interactive `lyx config <module>` path,
no flags) and `setModule` (`--set` path) both emit their success message as bare plain
text (`fmt.Fprintf(out, "edited and synced _lyx/config/%s.yaml\n", module)`) instead of
through the `internal/output` JSON envelope. This violates CONSTRAINTS.md's CLI/Cobra
invariant ("Results and errors go through the internal/output envelope... one JSON
object per line") and is inconsistent with the rest of the CLI (`boardcli`, `warpcli`,
and `configcli`'s own `runReconcile` all use `output.Ok`/`output.Err` exclusively for
command results). Confirmed as real via `git log`/`grep`, not present in any other
command path checked. Bundled into this task's scope per operator decision (see Q&A
log).

## Scope

**In:**
- Fix `yamlengine.SetValues` so it never silently discards an existing config key
  whose leaf path isn't in the template: preserve the key verbatim in the written
  output and report it back to the caller.
- Thread that report up through `configengine.Set` and `configcli.setModule` so the
  CLI surfaces a warning naming the preserved key(s).
- Convert `setModule`'s success output to the `internal/output` JSON envelope
  (`output.Ok`), including the preserved-keys report as a field.
- Convert `editOne`'s success output to the `internal/output` JSON envelope as well
  (bundled fix for the identical, independently-discovered plain-text violation).
- Update `buildConfigLong()`'s `--set` help text and the relevant module doc(s) under
  `docs/shared-libs/` (`configengine.md`, `yamlengine.md`) to describe the new
  preserve-and-warn behavior, per this repo's docs-in-same-commit rule.
- Tests at the `yamlengine`, `configengine`, and `configcli` layers covering the new
  preserve behavior and the new JSON success envelopes.

**Out:**
- `internal/yamlengine.Reconcile` / `internal/configsync.ReconcileAll` — logic is
  already correct (verified). No changes.
- Restoring `path` (or any other removed key) to any module's template — `path` was
  deliberately excluded from `boardengine`'s template as part of the Hub Geometry
  Invariant (`bac0e54`); reintroducing it as a template key would violate
  CONSTRAINTS.md's "Geometry is structural, never config/env-overridable" rule.
- Any mechanism to let `--set` "adopt" or rename an orphaned key back into the
  template's known-key set. Once a key is orphaned, `lyx config reconcile --apply` is
  the only sanctioned way to remove it; `--set <orphaned-key>=...` continues to be
  rejected as `unknown config key(s)` exactly as today (this is correct, existing
  behavior tied to template membership, not something this task touches).
- Full leaf-path-level (dotted/indexed) reconstruction for orphaned config
  structure — preservation operates at root-key (top-level) granularity instead,
  which handles any shape (scalar, nested mapping, sequence) uniformly without
  needing leaf-path-specific logic. See Decisions → `root-key-preservation`.
- Any change to `internal/configengine.Edit`'s core edit/validate/abort loop — it
  never routes through `SetValues`/`Reconcile` (opens the raw on-disk file directly in
  the editor) and was confirmed unaffected by the key-loss bug. Only its final
  success-message line changes (JSON envelope), not its control flow.
- `internal/configcli/menu.go`'s interactive numbered-picker prompt text — that's
  human-facing REPL output, not a command result/error path, and is outside the CLI
  invariant's "one JSON object per line" contract.
- Any change to the "unknown config key(s)... (known: ...)" rejection error for
  user-*requested* `--set` keys not in the template — that error path is correct and
  untouched; this task is only about existing keys already on disk that the user did
  *not* ask to touch.

## Decisions

### fix-scope

- Decision: scope the fix to the `--set` write path (`yamlengine.SetValues` →
  `configengine.Set` → `configcli.setModule`) only. Treat `Reconcile`/`ReconcileAll` as
  already correct.
- Rationale: direct unit-level repro (see Problem) proves `Reconcile` returns
  `removed: ["path"]` correctly when the stale key is still on disk, and
  `configcli.runReconcile` already surfaces `Result.Removed` in its JSON output. The
  destructive behavior is fully explained by `SetValues` always marshalling from a
  `templateNode`-only working tree (a deliberate prior design from "Card 1", the fix
  that made partial-existing-file merges correct for *requested* keys — its side effect
  is that any existing key with no template counterpart became structurally unreachable
  in the output).
- Rejected: "reconcile detection gap" as originally reported — investigated and
  disproven; re-scoping to reconcile would fix a problem that doesn't exist and miss
  the real one.

### set-preserve-and-warn

- Decision: `--set` never silently drops an existing key. `yamlengine.SetValues`
  compares `existingNode`'s root mapping's **direct (top-level) keys** against
  `templateNode`'s root mapping's direct keys, and for any existing top-level key
  absent from the template, grafts that key's **entire value subtree verbatim**
  (whatever it is — scalar, mapping, or sequence) onto `templateNode`'s root mapping —
  appended after all template keys, **sorted by key name**. The grafted key node's
  `HeadComment` is **unconditionally set (never appended/concatenated)** to a fixed
  marker string (e.g. `# preserved (not in current template)`), replacing whatever
  comment the key carried on disk. Add `SetResult.Preserved []string` (sorted,
  top-level key names) reporting which keys were carried through untouched, populated
  whenever this grafting occurs (nil/empty otherwise). `configengine.Set`'s signature
  changes from `(baseDir, module, template string, pairs []yamlengine.KV) error` to
  `(...) ([]string, error)`, threading `SetResult.Preserved` up to the caller.
  `configcli.setModule` includes the preserved list in its JSON success envelope (see
  `output-format-json-envelope`) whenever non-empty.
- Idempotency guarantee: a preserving `--set` call is idempotent — running it again
  (with the preserved key still on disk, untouched) produces byte-identical output.
  This holds because (a) the graft always operates on the *current* on-disk top-level
  value subtree, so repeated runs graft the same content, and (b) the marker
  `HeadComment` is *set*, not appended, every time — it cannot stack or grow across
  repeated runs regardless of what comment (marker or original) the key carried
  going in. See Testing for the regression case this backs.
- Rationale: matches the project's "no silent data loss" bar. The user is informed
  every time (not just once) until they resolve the drift via `lyx config reconcile`,
  which is the explicit, correct tool for that job — reusing it rather than growing a
  second, partial reconciliation mechanism inside `--set`. Operating at root-key
  (rather than flattened-leaf) granularity also means a nested/structured orphan key
  (see `root-key-preservation`) is preserved as a whole, correct subtree instead of
  being decomposed and partially reconstructed.
- Rejected: fail-closed (refuse `--set` entirely when any unrecognized key exists) —
  adds friction to unrelated, otherwise-valid edits and was not the operator's pick.
  Silent preservation with no warning — stops the data loss but leaves the user unaware
  their config carries deprecated fields indefinitely; rejected in favor of surfacing
  it every time. Leaf-path-level grafting (matching `applyExistingOverrides`'s
  granularity) — rejected after discussion-review found it leaves non-flat orphans
  unhandled (see `root-key-preservation`); root-key granularity is simpler and covers
  every shape uniformly.

### output-format-json-envelope

- Decision: `setModule`'s and `editOne`'s success paths both switch from bare
  `fmt.Fprintf(out, "edited and synced _lyx/config/%s.yaml\n", module)` to
  `output.Ok(out, map[string]any{"module": module, "message": "edited and synced
  _lyx/config/" + module + ".yaml", ...})`. For `setModule`, add a `"preserved":
  [...]` field when `SetResult.Preserved`/the threaded-up preserved list is non-empty;
  omit the field entirely when empty. `editOne` gets no `"preserved"` field (concept
  doesn't apply there — it never touches `SetValues`).
- Rationale: CONSTRAINTS.md's CLI/Cobra invariant mandates results go through the
  `internal/output` envelope ("one JSON object per line... No bare plain-text error
  paths"); `boardcli`, `warpcli`, and `configcli`'s own `runReconcile` already follow
  this. `editOne`/`setModule`'s plain text was pre-existing debt from before (or missed
  by) the `7817b67` "CLI help & error ergonomics" sweep that introduced the JSON error
  envelope. Fixing `setModule` was already required to surface `Preserved`
  structurally rather than string-munging a warning into a plain-text line; fixing
  `editOne` in the same commit was an explicit operator call once the identical
  pre-existing violation was pointed out (see Q&A log) — same file, same pattern, same
  fix shape, avoids leaving one of the two nearly-identical functions inconsistent with
  the other after this commit.
- Non-breaking note: embedding the message text as a JSON string value means the
  literal substring `"edited and synced"` still appears in the raw output line, so
  existing `strings.Contains(outStr, "edited and synced")` assertions in
  `configcli_test.go` and `configcli_integration_test.go` continue to pass unchanged;
  only assertions (if any) checking the *exact* full line need updating.
- Rejected: leaving the warning as an appended plain-text line alongside the existing
  plain-text success message — was the original recommendation, superseded once the
  operator pointed at CONSTRAINTS.md's JSON-envelope requirement and the `boardcli`/
  `warpcli`/`runReconcile` precedent.

### root-key-preservation

- Decision: preservation operates at **root-key (top-level) granularity**, not
  flattened-leaf granularity. This means it uniformly handles every existing
  top-level key the template doesn't recognize — including a **non-flat orphan**
  (a top-level key whose value is itself a nested mapping or sequence) — by grafting
  the whole subtree verbatim. There is no special-case detection needed for
  "nested vs. flat": the graft always operates on whatever value node the orphaned
  top-level key has, at any depth.
- Rationale: discussion-review (round 1) found the earlier `flat-keys-only` framing
  left an unguarded gap — `configengine.Edit`'s interactive path validates only YAML
  *syntax* (`yaml.Unmarshal` into `map[string]any`), not shape, so a user *can*
  hand-edit a nested structure into `_lyx/config/<module>.yaml` today even though no
  current template itself is nested. Under the old leaf-path-level design, such a
  key would have silently vanished on the next `--set` — reintroducing the exact
  data-loss class this task exists to fix. Root-key granularity closes this
  unconditionally: every top-level orphan is preserved whole, regardless of its
  internal shape, with no code path that silently drops one.
- Rejected: leaf-path-level grafting scoped to flat keys only (original framing) —
  correct for the three current templates but leaves a real, reachable gap for
  hand-edited nested config. Failing loudly on a non-flat orphan instead of
  preserving it — rejected because it would make `--set` refuse to work on an
  otherwise-unrelated key edit just because some other, untouched key happens to
  carry nested structure; preserving it whole causes no harm and requires no extra
  user action.

### reconcile-unchanged

- Decision: no changes to `internal/yamlengine.Reconcile`, `internal/configsync.ReconcileAll`,
  or `configcli.runReconcile`.
- Rationale: verified correct (see `fix-scope`). `reconcile --apply` intentionally
  drops removed keys — that is its documented job ("reporting... removed keys (deleted
  from template)") — and it already reports `removed` accurately in both dry-run and
  apply output, giving the operator full visibility before any destructive write.
- Rejected: adding a confirmation/flag gate before `--apply` drops keys — operator
  explicitly declined; reconcile's dry-run-first, accurately-reported behavior is
  already sufficient safety.
- Interaction note for implementer: after a preserving `--set` run, `lyx config
  reconcile --apply` will now (correctly, per existing behavior) detect the preserved
  orphaned key and remove it on apply — this is intended, not a bug to guard against.

## Technical context

Files this task touches:

- `internal/yamlengine/set.go` — `SetValues` (the mutation entry point for `--set`),
  `SetResult` struct (add `Preserved []string`). Shares `applyExistingOverrides` and
  `collectLeafPaths` with `internal/yamlengine/reconcile.go` (both untouched).
- `internal/yamlengine/reconcile.go` — read-only reference for this task; confirms
  `Reconcile`'s `removed` detection already works (do not modify).
- `internal/configengine/set.go` — `Set` (signature change: add `[]string` preserved-keys
  return value alongside the existing `error`). Shares `scaffoldIfMissing` with
  `internal/configengine/edit.go` (unaffected — `Edit`'s scaffold path never carries
  extra keys since it writes the template verbatim when missing).
- `internal/configcli/configcli.go` — `setModule` (lines ~155–192: wire the new
  `Set` return value into the JSON envelope), `editOne` (lines ~94–134: convert its
  success line to `output.Ok`), `buildConfigLong` (update `--set` help prose).
- `internal/output/output.go` — `Ok`/`Err` JSON envelope helpers; reuse as-is, no
  changes needed to this package.
- `internal/boardengine/config.go` — reference only: `Config.Path` is `yaml:"-"`,
  deliberately never populated from the config file; confirms why `path` is correctly
  absent from the template and why `--set path=...` correctly stays rejected.
- `internal/boardengine/template.yaml`, `internal/warpengine/template.yaml`,
  `internal/weftengine/template.yaml` — confirmed flat, single-level templates today;
  the `root-key-preservation` design is not scoped to this shape, but it's the shape
  every current regression test will exercise.

Root-cause chain for the implementer to internalize: `configengine.Set` reads
`existingBytes` from disk, then calls `yamlengine.SetValues(template, existingBytes,
pairs)`. `SetValues` parses `template` into `templateNode` (the only tree that ever
gets marshalled), collects `templateLeaves`, then layers `existingLeaves`' values onto
*matching* `templateLeaves` via `applyExistingOverrides` — but leaves in `existing`
that have no template counterpart are never copied anywhere, so they vanish from the
final `Merged` output with no trace. The fix is a fourth step after
`applyExistingOverrides`, operating independently of the leaf-path machinery: walk
`existingNode`'s root `MappingNode.Content` in key/value pairs (same iteration shape
`collectLeafPathsHelper` already uses for `MappingNode`), and for every top-level key
not present as a top-level key in `templateNode`'s root mapping, append that key node
and its full value node (clone or reuse — implementer's call, but never mutate the
source `existingNode`) to `templateNode`'s root `MappingNode.Content`, set the fixed
marker string as the appended pair's comment (`HeadComment` on the key node, per
yaml.v3 convention for a comment line above a mapping entry — verify against yaml.v3
behavior during implementation), and record the key name in `Preserved`.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — "Errors are JSON... Results and errors go through the
  internal/output envelope (output.Ok / output.Err)... No bare plain-text error
  paths." Directly motivates `output-format-json-envelope`. Also: "Help accuracy is a
  review obligation... When a change alters observable behaviour, the reviewer must
  re-read every affected Short/Long and confirm it matches the code as changed" —
  `buildConfigLong()`'s `--set` description must be updated in this commit since this
  task changes `--set`'s observable output shape and adds new preserve-and-warn
  behavior.
- **Hub Geometry Invariant** — "Geometry is structural, never config/env-overridable
  (the board dir is `--board-path` flag > `hubgeometry.BoardDir(l.Hub)`, not a config
  key)." Confirms `path` must never be reintroduced as a template key; directly
  informs the Scope → Out entry on this point.

Per this repo's `CLAUDE.md` "Task completion" section: this task changes observable CLI
behavior (`--set`'s output shape, plus new preserve/warn semantics), so
`docs/shared-libs/configengine.md` and `docs/shared-libs/yamlengine.md` must be updated
in the same commit to describe the new behavior. Check `docs/overview.md` for whether
its module table/execution-stack description needs a matching update (read it during
planning; update only if it references `--set`'s current behavior).

## Testing

- **`internal/yamlengine/set_test.go`** (TDD candidate): new test(s) verifying
  `SetValues` preserves an unrelated unknown existing top-level key verbatim into
  `Merged` and reports it via `SetResult.Preserved`; a multi-preserved-key case
  asserting sorted order; a zero-preserved regression case confirming existing
  byte-for-byte assertions (`TestSetValues_PartialExistingDoesNotSuppressSet` et al.)
  are unaffected when no unrecognized existing keys are present. Also verify the
  marker comment is present above preserved keys and that preserved keys are appended
  after all template keys.
  - **Idempotency case:** call `SetValues` twice in sequence, feeding the first call's
    `Merged` output back in as the second call's `existing` — assert the second call's
    `Merged` is byte-identical to the first's (no comment duplication/growth, no
    duplicate preserved key entries).
  - **Non-flat orphan case:** `existing` contains a top-level key absent from the
    template whose value is a nested mapping (or sequence) rather than a scalar —
    assert the whole subtree survives verbatim in `Merged` and the top-level key name
    appears once in `SetResult.Preserved`.
- **`internal/configengine/set_test.go`**: existing tests
  (`TestSet_ScaffoldWhenMissingThenSet`, `TestSet_UnknownKeyRemovesScaffoldedFile`,
  `TestSet_UnknownKeyLeavesExistingFileUnchanged`, `TestSet_PreservesOtherKeysOnExistingFile`)
  need updating for `Set`'s new `([]string, error)` signature. New end-to-end test:
  a real on-disk file with an extra key survives `Set()` unchanged and the key is
  reported back.
- **`internal/configcli/configcli_test.go`** / **`configcli_integration_test.go`**:
  update existing `--set`/edit success assertions for the new JSON envelope shape
  (verify the substring-based assertions noted in `output-format-json-envelope` still
  pass, or convert them to JSON-decode-and-check for stronger typing). New test:
  `--set` against a file with an unrecognized existing key produces a JSON success
  envelope containing a `"preserved"` field naming it; a clean file (no unrecognized
  keys) produces the same envelope shape with no `"preserved"` field.
- **`internal/configcli/reconcile_test.go`**: no changes expected (reconcile logic
  untouched) — re-run as a regression check only.
- Avoid prescribing exact JSON field names/ordering beyond what's specified in
  Decisions — assertion shape (JSON-decode vs substring) is mill-plan's call.

## Q&A log

- **Q:** Should the fix be scoped to `--set` only, or also address a possible reconcile
  detection gap? **A:** Scoped to `--set` only, after empirical verification (direct
  `yamlengine.Reconcile` repro) proved reconcile's detection is already correct.
- **Q:** How should `--set` handle existing keys not in the template? **A:** Preserve
  + warn (option 1/recommended) — never silently drop, always inform the user.
- **Q:** Should reconcile get an extra confirmation/flag gate before `--apply` drops
  keys? **A:** No — leave reconcile as-is (option 1/recommended); its existing
  dry-run-first, accurately-reported behavior is sufficient.
- **Q:** Should the preserve-warning be plain text or JSON? **A:** Initially
  recommended plain text (matching what looked like existing `--set` convention);
  operator pushed back, citing CONSTRAINTS.md's JSON-envelope requirement and a recent
  ("a few days ago") standardization sweep. Verified: `7817b67` ("CLI help & error
  ergonomics: JSON error envelope...") plus CONSTRAINTS.md's explicit "Results and
  errors go through the internal/output envelope" text confirm the JSON requirement;
  `boardcli`/`warpcli`/`runReconcile` already comply. `configcli.go`'s `editOne`/
  `setModule` plain-text success messages were found to be pre-existing debt that
  missed or predates that sweep. Revised decision: JSON via `output.Ok` for both.
- **Q:** Is key *removal* from a template common, or is *addition* the more typical
  case? **A:** Investigated via `git log --follow` on `internal/boardengine/template.yaml`:
  `path` was added once (initial extraction) and removed exactly once, in `bac0e54`
  ("Harden the Path Invariant"), three days before this bug was filed, as a deliberate
  invariant-hardening change. Addition (`added`, when a new lyx version introduces a
  config field) is the routine case; removal is rarer but this incident is the direct,
  reproducible fallout of exactly the kind of deliberate removal that does happen.
- **Q:** Once `editOne`'s plain-text-vs-JSON issue was surfaced (found by inspection,
  not part of the original bug report), should it be fixed in this same task? **A:**
  Yes (option 2) — bundle it into this commit since it's the identical pattern in the
  same file, discovered as a direct byproduct of this task's investigation.
