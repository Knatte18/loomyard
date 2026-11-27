// Package shuttleparity compares what each engine gives a launched session, seat by seat.
// It prepares one spec per engine with that engine's Prepare.
// It reads back the launch manifest each writes, and reports every compared item the Codex manifest lacks of Claude's.
//
// UnrunHooks and ConfiguredSandbox are the plugin hook runner and the sandbox probe a manifest-only launch runs under:
// neither runs a hook or Codex, so the manifest lists the configured sandbox mode and each plugin context as unrun.
// PromptItems adds the prompt-side items an engine cannot see of its own launch.
// PrepareEach prepares each engine of a set, and Compare names the compared items the Codex manifest lacks.
// The Webster fork deny belongs to fork support.
// Compare asks for it only when the Codex engine meets the fork requirement.
// Unmet names the requirements of a spec an engine cannot meet.
// A spec an engine cannot start is prepared and compared like any other, since Prepare refuses no requirement.
//
// The seat catalog lists every production seat table builder with the functions that hold its table and seat literals.
// It builds each table on the worktree's configuration.
// A run's own inputs are stand-ins, written under a directory the caller removes.
// A builder that fails costs only its own seats, and the failure names the stand-in inputs to fix.
// The catalog covers the tables built from code.
// A recipe row whose seats an operator writes in config holds none in code.
//
// Report prepares one seat of the catalog on each engine of a set and returns both manifests, what the Codex one lacks,
// and the requirements each engine does not meet.
// It backs `lym shuttle manifest`.
package shuttleparity
