// Package batcher groups a plan's flat card list into the execution units webster forks each run.
// The Batcher interface fronts a library of batchifier implementations.
// A registry maps each batchifier kind to its constructor, and Active is the config entry point.
// Batching, meaning how many cards land in one fork and in what grouping, is a standalone step webster consumes.
// The plan does not decide it, since a plan-format card carries no batch-membership field.
// No LLM decides it either: grouping is pure orchestrator-side logic over the parsed Card list.
//
// batcher.yaml holds named profiles under profiles:.
// Each profile is a batchifier kind (identity or cost) with that kind's parameters.
// The active: key names the profile in force (see contracts/specs/loom-plan-spec.md).
// profiles: is a per-name map, keyed by names the operator chooses.
// A layer's profiles add to or override the template's by name, each profile merging key by key,
// so a layer holds only the profiles and keys it tunes.
// `lym config batcher --set profiles.<name>.<key>=<value>` sets one key of any profile.
// Active resolves the active profile at config-load time and builds it through the registry.
// An empty active: and active: identity resolve to the identity batcher.
// This holds even when no profile of that name is configured, so a batcher.yaml without profiles: keeps working.
// A configured profile named identity wins over that default.
// A load error names batcher.yaml and the offending profile or key.
// It arises from an active: naming no profile, an unknown batchifier kind, or a retired key.
// It also arises from a cost profile that breaks a validation rule.
// The budget must be present and positive;
// max_cards must be at least 2;
// and every weights coefficient must be present, non-negative and known.
// The retired keys are alone_above, and master_base and startup_context in weights:.
// A retired-key error wraps ErrRetiredKey and names the highest layer file holding the profile.
// It ends with the way forward that file calls for.
// For a worktree or local file the way forward is running "lym config reconcile --apply", which migrates the profile.
// For the repo default, which only the hub-wide reconcile rewrites, it is running "lym fabric reconcile".
// A session lym refuses that verb and reports status: FAILED, so the orch runs it.
// Active loads batcher.yaml through the layer stack under a configengine.Dirs.
// ConfigSpec classifies the active: and profiles: keys as meeting no category, so any layer may hold them,
// and a retired key in any layer reaches the retired-key refusal.
// Every load error is marked configengine.ErrInvalid, so a caller tells a file's content from an unreadable file.
// MigrateConfig is a pure rewrite of a batcher.yaml document that drops the retired keys from every profile.
// It adds weights.orientation at the template's value where a retired weights key stood and the profile had none.
// It touches only those keys and carries every other entry and comment through yaml node editing.
// The loader never calls it.
// The template ships active: wide, so a fresh repo groups cards with the cost batchifier without configuring it.
// Its cautious profile is the same cost batchifier at a tighter budget, kept for a repo that chooses it.
// The budget trial behind that default is recorded in docs/research/webster/batch-budget/batch-budget-trial-2026-11-01.
// It found wide cost no more per card than cautious and showed no quality loss.
// An operator who wants one card per batch sets active: to identity or to an empty value.
//
// The identity batcher (identity.go), one card per batch, is a production library entry.
// The Batcher interface lets a further grouping batchifier join the registry without changing webster's call sites.
//
// The estimator (estimate.go) predicts how full a fork's context gets without running it,
// so a cost-model batchifier can keep every fork inside the model's window.
// A SizeSource supplies the worktree facts it weighs: a file's line count and a directory's test files.
// DiskSizes reads them from a given worktree root.
// A card's read set holds one entry per target and Uses ref that planparser.RefFile maps to an existing file,
// and each such entry weighs its lines times context_per_line.
// It also holds a package entry and a tests entry for each directory the card's targets live in.
// Each of those weighs package_context.
// It holds the card's own text too, weighing its lines times context_per_line.
// A card's messages are the sum of these terms: target_messages per target (a Rename pair counts once);
// test_file_messages per test file in its target directories;
// and uses_messages per Uses entry.
// Its write allowance is its messages times message_context, which covers the tool calls and output they add.
// It also adds its card-text lines times write_per_card_line, the code the card carries that the fork writes back out.
// PeakContext of a segment at a 1-based batch position is the sum of these terms:
// the start base's context plus orientation;
// (position - 1) times batch_growth;
// fork_messages times message_context;
// the weight of the distinct read-set entries of all its cards;
// and every card's write allowance.
// The start base (StartBase) is what the orchestrating session loads at start.
// Webster computes it from the loaded text's line count and a fixed system-prompt-and-tools figure.
// Its context is its lines times context_per_line plus the fixed part, and the zero value adds nothing.
// A fork inherits the orchestrating session's context.
// That context is the start base plus orientation when the first fork spawns,
// and it grows by batch_growth for every batch that finished before the fork, so a later batch starts deeper.
// Context only grows within a fork, so the peak is the context after its last card.
// A file two cards share counts once, and adding a card never lowers the peak.
// A card's own estimate is the PeakContext of its one-card segment.
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
// The comparison covers the one-card forks of past runs.
// The measured peak is the largest input plus cache tokens of any of the fork's messages.
// The card text is not in a run's base tree, so write_per_card_line and the card-text read are unfitted.
// Each batch records its estimate's components (Breakdown), which webster keeps in state.json.
// Later runs can fit all the coefficients from them.
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
// An exact dynamic program over (batches so far, last card placed) finds the split.
// It takes O(cards² x MaxCards) segment evaluations.
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
// Webster's execution unit is always the batch the active profile derives,
// and this package's registry is the one place a batchifier kind is chosen.
// The partition is recorded in state.json at first init;
// a later run reads the recorded partition and does not derive it again, and only a rebaseline replaces it.
// A recorded partition is therefore never regrouped, since only a derivation applies the boundary rules.
package batcher
