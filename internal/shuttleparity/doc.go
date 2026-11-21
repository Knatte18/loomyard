// Package shuttleparity compares what each engine gives a launched session, seat by seat.
// It prepares one spec per engine with that engine's Prepare.
// It reads back the launch manifest each writes, and reports every compared item the Codex manifest lacks of Claude's.
//
// A manifest-only launch runs no hook and no Codex,
// so the manifest lists the configured sandbox mode and each plugin context as unrun.
// The Webster fork deny belongs to fork support.
// Compare asks for it only when the Codex engine meets the fork requirement.
// A spec an engine cannot start is prepared and compared like any other, since Prepare refuses no requirement.
//
// The seat catalog lists every production seat table builder with the functions that hold its table and seat literals.
// A run's own inputs are stand-ins, written under a directory the caller removes.
// A builder that fails costs only its own seats, and the failure names the stand-in inputs to fix.
// The catalog covers the tables built from code.
// A recipe row whose seats an operator writes in config holds none in code.
//
// Report backs `lym shuttle manifest`.
package shuttleparity
