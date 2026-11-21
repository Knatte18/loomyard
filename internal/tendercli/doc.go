// Package tendercli builds the `lym tender` group over internal/tenderengine.
// It also installs the network gate that every lym process in a hub runs under.
// The verbs are `lym tender pause [<slug>]`, `lym tender resume [<slug>]` and `lym tender revive [<slug>]`.
// Its other verbs are `lym tender status` and `lym tender reap`.
// `lym p` and `lym r` are shortcuts for the first two, so the operator spelling stays short.
//
// Every verb runs from any worktree of the hub and resolves it with lymcwd.Resolve.
// The hub's fixed dependency set bars cliwire from serving them, since it admits neither lymcwd nor fabricengine.
//
// # Callers
//
// The caller is read from the strand-name variable reed injects:
// empty is an external caller, the orch's name is the orch, and any other name is an agent of the slug it carries.
// The orch and an external caller may run `pause`, `resume` and `revive` with or without a slug.
// An agent may pause, resume or revive only its own pair.
// A hub-wide call, or one naming another slug, is refused with the way forward of asking its parent session.
// `status` takes no caller check, since it writes nothing; `reap` takes it with no slug.
// The check guards against a mistaken call:
// the agents' settings deny any command naming the variable, so an agent cannot clear it to read as external.
//
// # Run pause intent
//
// `pause <slug>` and `resume <slug>` also write and clear the run's pause intent in its batten record on the prime.
// Each commits the record and pushes the prime's records side, and waits for the push.
// Both verbs refuse a run another machine owns, naming that machine.
// Offline, the pause entry still stops the run's processes here, and the intent is committed with its push owed.
// An owed push writes one line to stderr and the verb succeeds;
// until it lands another machine reads the run unpaused.
// Any other failed push fails the verb.
// `PausePair` and every hub-wide pause record no intent.
//
// # Bolt hold
//
// While a hold stands, `lym tender resume <slug>` of a slug held only by a `bolt-hold` entry is refused,
// and a hub-wide resume leaves every `bolt-hold` entry standing.
//
// # Plain pair pause
//
// `PausePair` pauses a run with a plain pair entry, for a run lym stopped because origin moved what it works on;
// it ends nothing.
//
// # Revive
//
// `revive` types a fixed carry-on line only into a session that reads stalled on an API error,
// and gives the orch a checked creel inject.
// It sends no key and kills nothing.
// See the tenderengine documentation for the sequence and its bound.
//
// # Network gate
//
// InstallNetworkGate installs the process gate that holds network git and GitHub calls while a pause covers the process.
// A pause stops runs, so the gate holds only a run's own process:
// a run command, a lym process a run command started, a detached child, or an agent of a strand.
// A run command is one whose cobra command carries `clihelp.RunProcessAnnotation`;
// it sets `pauserecord.RunProcessEnv`, which every process it starts inherits.
// An orch's strand is the operator's delegate and is not held.
// Any other process runs an operator's verb from a terminal or a script, and its gate lets every live entry pass.
// Its network calls are still marked in flight, so a pause's grace still waits for them.
// The gate installs nothing outside a hub.
// A `Deferrable` child that is not `ParentLocked` installs a deferring gate.
// That gate exits the child under a pause instead of parking it.
// A `HubScope` child, the detached board sync, is held under the empty slug,
// so only a hub entry holds it, whichever worktree spawned it.
package tendercli
