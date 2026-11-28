// Package batcher groups a plan's flat card list into the execution units an implementer runs.
// The Batcher interface fronts a library of batchifier implementations.
// A registry maps each batchifier kind to its constructor, and Named is the config entry point.
// Batching, meaning how many cards land in one fork and in what grouping, is a standalone step webster consumes.
// The plan does not decide it, since a plan-format card carries no batch-membership field.
// No LLM decides it either: grouping is pure orchestrator-side logic over the parsed Card list.
//
// batcher.yaml holds named profiles under profiles:.
// Each profile is a batchifier kind (identity or cost) with that kind's parameters.
// Which profile batches a run is the implementer's own config, not this module's:
// each implementer names one under the ChoiceKey key of its own file, webster.batch_profile and weaver.batch_profile.
// profiles: is a per-name map, keyed by names the operator chooses.
// A layer's profiles add to or override the template's by name, each profile merging key by key,
// so a layer holds only the profiles and keys it tunes.
// `lym config batcher --set profiles.<name>.<key>=<value>` sets one key of any profile.
// Named resolves the profile an implementer names at config-load time and builds it through the registry;
// LoadProfiles builds every profile, so a config check refuses a profile no implementer names yet.
// A load error names batcher.yaml and the offending profile or key.
// A cost profile's budget must be present and positive;
// max_cards must be at least 2;
// and every weights coefficient must be present, non-negative and known.
// The retired keys are the top-level active, a profile's alone_above, and master_base and startup_context in weights:.
// A retired-key error wraps ErrRetiredKey and names the highest layer file holding the key or its profile.
// The active refusal's way forward opens with the implementer keys that choose a profile now.
// It ends with the way forward that file calls for.
// For a worktree or local file the way forward is deleting the key by hand or running "lym config reconcile --apply".
// For the repo default, only a reconcile of the default layer rewrites it.
// The way forward there is deleting the key by hand.
// It is also running "lym config reconcile --layer default --apply" from the hub's prime worktree.
// A session lym refuses that verb and reports status: FAILED, so the orch runs it.
// Reconcile keeps a profile the template does not name whole.
// For such a profile the way forward is the hand delete alone.
// Any layer may hold the profiles: key, so a retired key in any layer reaches the retired-key refusal.
// A hub's base file loses its active key through RetireActive, which the hub-wide reconcile applies once.
// An empty profile name and identity both resolve to the identity batchifier, one card per batch.
// The template's wide profile is the cost batchifier at the budget a webster fork holds;
// its cautious profile is the same cost batchifier at a tighter budget, kept for a repo that chooses it.
//
// The identity batcher (identity.go), one card per batch, is a production library entry.
//
// The estimator (estimate.go) predicts how full a fork's context gets without running it,
// so a cost-model batchifier can keep every fork inside the model's window.
// A SizeSource supplies the worktree facts it weighs: a file's line count and a directory's test files.
// DiskSizes reads them from a given worktree root.
// A fork inherits the orchestrating session's context.
// That context is the start base, what the session loads at start, plus orientation when the first fork spawns,
// and it grows by batch_growth for every batch that finished before the fork, so a later batch starts deeper.
// The estimate predicts a fork's peak context, so a batch fits the model's window.
// A file two cards share counts once, and adding a card never lowers the peak.
// The coefficients are a Weights value, read by ProfileWeights from a profile's weights: map in batcher.yaml.
//
// The template's cost profiles share their coefficients.
// The start coefficients are measured, and the in-fork coefficients are not.
// orientation is the measured first-fork start minus the measured Merriam session start.
// It is the context Merriam gains reading the codebase and plan before its first fork.
// batch_growth is the mean measured growth per batch, taken over cautious runs with each fork's start measured.
// Growth counts batches, not cards: Merriam gains context per fork it spawns and records, whatever the fork's size.
// The in-fork coefficients are fitted under a constant start, not against the position model.
// tools/tokencount's calibration produces that fit.
// It compares each one-card estimate with the measured peak context of the fork that ran the card.
// The measured peak is the largest input plus cache tokens of any of the fork's messages.
// The card text is not in a run's base tree, so write_per_card_line and the card-text read are unfitted.
// Each batch records its estimate's components (Breakdown), which webster keeps in state.json.
//
// The cost-model batchifier (cost.go) is built by NewCost from CostParams.
// It splits the card sequence into the fewest contiguous batches that fit.
// Cards share a fork only inside a contiguous segment of the given order.
// Card order and every forward dependency are therefore kept.
// A one-card segment is always feasible.
// A segment of two or more cards is feasible only when all of these hold:
// it holds at most MaxCards cards;
// its PeakContext at its own batch position stays within Budget;
// and no card in it renames or deletes a target of an earlier card in it.
// A card over Budget at its position therefore runs alone, as under identity, and nothing halts or warns.
// No threshold value makes the split infeasible.
// The recorded Estimate and Position show it.
// Among the splits with the fewest batches it takes the one whose largest batch peaks lowest.
// Batches therefore come out balanced.
// A remaining tie leaves the last batch shortest.
// Batch takes the number of batches the run executes ahead of its cards,
// so the k-th batch of the result is at position before + k, and a segment's peak depends on the batch it forms.
// Each batch carries the profile name and its PeakContext at its own position as its estimate.
// The estimate's Breakdown records that position.
//
// A card that renames or deletes what an earlier, distinct card targets starts a new batch (boundary.go),
// so a batch's done-check and card gates read a tree that still holds each card's targets.
// A card's targets are its Create, Edit and Delete refs and the new side of each Rename pair;
// its covers are the old side of each Rename pair and its Delete refs;
// and planparser.RefCovers decides when a cover covers a target.
// A card that names a C# member an earlier card re-signs also starts a new batch, in addition to the cover rule.
// An earlier card re-signs a C# member with an Edit arrow whose target parses as a C# glyph,
// and the naming card names it with that target's exact string among its Targets or Uses.
// The re-sign is rewritten to the member's minted id only once its batch is recorded.
// The naming card therefore cannot share the batch.
// The rules only add boundaries: they never merge batches, reorder cards or move a card across a dependency,
// and every grouping batchifier applies them.
// The rules' bound is that a file-level old side over-splits from a package-unit target in that file's directory.
//
// An implementer's execution unit is always the batch its named profile derives,
// and this package's registry is the one place a batchifier kind is chosen.
// The partition is recorded in state.json at first init;
// a later run reads the recorded partition and does not derive it again, and only a rebaseline replaces it.
// A recorded partition is therefore never regrouped, since only a derivation applies the boundary rules.
package batcher
