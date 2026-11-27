# PATTERN-engine-parity

Launching a role on an engine: the session gets every skill, standing passage, instruction file, guard, hook and directive
a session of that role gets on another engine, through its own channel;
each engine's launch manifest is compared by test.

## The launch manifest

Each engine's `Prepare` writes a record of what its launch gives the session, through `shuttleengine.WriteManifest`.
It reads the items back from what the launch wrote and passes,
so an item exists only where its delivery reached the launch.

- An item is a channel and an engine-neutral key.
  Two engines word and deliver the same item each their own way, under the same key.
- The channels are the `shuttleengine.ManifestChannel` constants.
  The edit directive is read off the rendered prompt by `internal/shuttleparity`, since an engine is told no stencils directory.
- An item the provider loads itself, such as the user's global instruction file, is listed as native,
  from the engine's resolution of it.
- An item from a shared heddle target (`agents-file`, `agents-prompt`) is compared,
  though it reaches each engine through that engine's own file or prompt.
- An item stored under a target aimed at one engine (`claude-file`, `claude-prompt`, `codex-file`, `codex-prompt`)
  is keyed to that engine and not compared.
- Engine settings are listed and not compared.
- The comparison is by channel and key alone, with Claude's manifest as the reference.
- The `lym webster` fork deny belongs to fork support:
  it is compared only when the Codex engine answers the fork requirement.
- No other item is exempt.
  A seat an engine cannot start is prepared and compared like any other, since only a start refuses it.

## The run record

- The record's verdict holds the items the launch goes without, such as a plugin session-start context that was omitted.
- A launch keeps a copy of the record, named by the run id, in the `manifests` directory under the run-directory root.
  A done run's cleanup leaves the copy, and nothing removes it.
- The orphan sweep and the run lookup by strand skip that directory.

## The parity test

`internal/shuttleparity` runs both engines' `Prepare` on fixtures, with no plugin hook or sandbox probe run,
and fails when the Codex manifest lacks a compared item of Claude's.

- It covers every seat of the seat catalog, one entry per production seat-table builder.
- It covers every other launch-spec producer through the producer table, one entry per function holding a launch-spec literal.
- A scan fails on an unlisted launch-spec literal, seat table or seat literal.
  A launch-spec literal is a `shuttleengine.Spec` literal that sets `Prompt`.
  The scan skips the orch packages, the shuttle CLI and the test-support packages under `internal/testkit`.
- Bound: a seat table an operator writes in a recipe row holds no seat in code, so the catalog does not list it.
- Bound: the scan is syntactic, so it does not see a value filled field by field.

## `lym shuttle manifest`

`lym shuttle manifest <seat>` prints each engine's manifest for a seat in the current worktree and the missing items.
It exits non-zero on a missing item, and names an engine's unmet requirements without failing on them.

- It starts no session and runs no plugin hook or sandbox probe.
- It writes nothing outside a temporary directory it removes, but the Codex home a Codex `Prepare` readies.
  It keeps no manifest copy.
- Plugin contexts are listed unrun, and only a run's record says whether one was delivered.
- Its size check of Codex's developer instructions leaves plugin contexts out,
  so a seat it passes can still be refused at a real launch.

## Enforcement

Tests in `internal/shuttleparity`:

- `TestSeats_CodexManifestHoldsClaudes`
- `TestProducers_CodexManifestHoldsClaudes`
- `TestLaunchSpecLiterals_AreListed`
