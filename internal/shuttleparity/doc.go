// Package shuttleparity compares what each shuttle engine gives a session, seat by seat.
// It prepares each engine's launch for one spec and reads back the launch manifests.
// It holds the Codex manifest to every compared item of Claude's.
//
// # Shared seams
//
// A prepared launch runs no plugin hook and no sandbox probe.
// ConfiguredSandbox reports a sandbox that starts, so the manifest lists the configured mode.
// UnrunHooks lists each plugin hook as unrun, so a plugin context counts as present by its plugin and hook.
//
// # Prompt items
//
// An engine is told no stencils directory, so it cannot tell its edit-directive copy in a prompt.
// PromptItems reads that item off the rendered prompt.
//
// # Comparison
//
// Compare checks the Codex manifest against Claude's by channel and key alone.
// The fork deny belongs to fork support, so it is compared only when the Codex engine answers the fork requirement.
// A seat that an engine cannot start is prepared and compared like any other, since only a start refuses it.
// Unmet names the requirements such an engine does not meet.
//
// # Seat catalog
//
// The catalog holds one entry per production seat-table builder.
// Each entry builds its tables on the worktree's configuration, with stand-ins for a run's own inputs only.
// The stand-ins are written under a directory the caller removes.
// Bound: a table an operator writes in a recipe row holds no seat in code, so the catalog does not list it.
package shuttleparity
