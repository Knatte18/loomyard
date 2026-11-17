# PATTERN-engine-parity

A session launched on an engine gets every skill, standing passage, instruction file, guard, hook and directive that a session of the same role gets on another engine.
Each engine delivers them through its own channel, and a test compares the launch manifests the engines write.

## The launch manifest

Each engine's `Prepare` writes a manifest of what the session it prepares receives, through `shuttleengine.WriteManifest`.
The record is `manifest.json` in the run directory.

- An item has a channel and an engine-neutral key.
  The channels are the prompt engine, the edit directive, standing passages, instruction sources, skill loads, skill listings,
  plugin contexts, guards, the batching nudge, notices and the engine's settings.
  A key is a heddle slug, a shell-guard row name, a skill name, `<plugin>/<hook command>`, an instruction file's path or a fixed notice key.
- An item is compared between engines unless it is keyed to one engine or is a settings item.
  A heddle target or an engine's own settings document that only one engine reads is keyed to that engine.
  The settings are listed for the reader and never compared.
- An item's delivery is delivered, native or omitted.
  A native item is one the provider loads itself, listed from the engine's resolution of it.
  An omitted item carries its reason, and the record's verdict lists every omitted item.
- An item is read back from the artifacts the launch wrote: each engine lists every candidate with the exact text or hook command its delivery writes,
  and keeps it only when the artifact as written holds it.
  Dropping a delivery anywhere in `Prepare` therefore drops its item.
- The edit directive is read off the rendered prompt by `internal/shuttleparity`, since an engine is told no stencils directory.
- The guard item `webster-fork-deny` belongs to fork support.
  The comparison asks for it only of an engine that meets the fork requirement, so on an engine that does not, the item is listed and not compared.
  The day that engine reports fork support, the parity test fails until it delivers a fork deny.

A launch keeps a copy of its record at `manifests/<run id>.json` under the run-directory root, once the run record is saved.
Nothing removes the copy.
A comparison matches items by channel and key whatever their delivery, so an omitted plugin context counts as present.

## The parity test

`internal/shuttleparity` prepares one spec per engine on a fixture machine and fails for each compared item of the Claude manifest that the Codex manifest lacks.
Every seat and every launch-spec producer is compared, including those an engine cannot start, since `Prepare` refuses no requirement and only `Start` does.

- The seat catalog lists every seat table builder of production code with the functions that hold its table and seat literals.
  It builds each table on the worktree's configuration, with stand-ins only for a run's own inputs, written under a directory the caller removes.
  A recipe row whose seats an operator writes in config holds none in code and is not in the catalog.
- The producer table lists every function outside the seat tables that holds a launch-spec literal.
  Each entry renders the producer's prompt through the producer's own render function on the seeded stencils.
- A scan fails on a production launch-spec literal that sets a prompt, a non-empty seat table literal or a seat literal that sets a stencil,
  whose enclosing function neither the producer table nor the seat catalog lists.
  It skips `internal/orchcli` and `internal/orchengine`, whose spec names the Claude engine, `internal/shuttlecli` and `internal/testkit`.
  A literal that sets no prompt, such as an attach probe's, is skipped.
  The scan is syntactic: a spec filled field by field is outside its sight.
- A new seat table builder takes a catalog entry, and a new producer takes a producer table entry, so its seats reach the parity test.

## `lym shuttle manifest`

`lym shuttle manifest <seat>` prints both engines' manifests for one seat of the catalog and exits non-zero when the Codex manifest lacks a compared item.
It names an engine that cannot start the seat with its unmet requirements, and that never fails the verb.

- It starts no session and runs no plugin hook or sandbox probe, so each plugin context is listed unrun.
- It writes nothing outside its temporary directory but the Codex home a Codex `Prepare` readies, and keeps no manifest copy.
- The developer-instructions size check leaves out plugin contexts, so a seat it passes can still be refused at a real launch.

## Enforcement

`internal/shuttleparity`'s `TestSeats_CodexManifestHoldsClaudes` compares every seat of the catalog.
`TestProducers_CodexManifestHoldsClaudes` compares every producer of the table.
`TestLaunchSpecLiterals_AreListed` runs the scan over the module.

Bound: the comparison matches by channel and key, so it does not see an item delivered with other text than the reference engine's.
Bound: an item is compared only when the engine lists it, so a delivery no engine lists as a candidate is outside the comparison.
