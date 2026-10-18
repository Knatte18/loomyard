// Package tendercli builds the `lyx tender` group with `lyx tender pause [<slug>]`, `lyx tender resume [<slug>]`, `lyx tender revive [<slug>]` and `lyx tender status` over internal/tenderengine, and installs the network gate every lyx process in a hub runs under.
// `lyx pause` and `lyx resume` are shortcuts for the first two, so the operator spelling stays short.
//
// Every verb runs from any worktree of the hub and resolves it with lyxcwd.Resolve.
// The hub's fixed dependency set bars cliwire from serving them, since it admits neither lyxcwd nor fabricengine.
//
// # Callers
//
// The caller is read from the strand-name variable reed injects: empty is an external caller, the orch's name is the orch, and any other name is an agent of the slug it carries.
// The orch and an external caller may run `pause`, `resume` and `revive` with or without a slug.
// An agent may pause, resume or revive only its own pair, so a hub-wide call, or one naming another slug, is refused with the way forward of asking its parent session.
// `status` takes no caller check, since it writes nothing.
// The check guards against a mistaken call: the agents' settings deny any command naming the variable, so an agent cannot clear it to read as external.
//
// # Input
//
// `pause` refuses a slug with no pair, listing the slugs that have one, and a `--grace` outside 0 to 60 seconds.
// `resume` takes any slug, so an entry whose pair is gone can still be lifted; it succeeds with nothing resumed when nothing is paused, and the engine refuses a slug that only a hub entry covers.
// `revive` refuses a slug with no pair as `pause` does.
// The envelope carries the engine's report verbatim under `pause`, `status`, `resume` or `revive`.
//
// # Revive
//
// `revive` gives one fixed carry-on line to each session stalled on an API error, and the orch a checked creel inject, reported with its id.
// Every send runs under the pause record's write lock, one send at a time, so a `pause`, `resume` or orch watcher record write waits at most one send's hold, or one orch revive's creel lookup and inject.
// It types only into a session that reads stalled on an API error at the send's own reading, sends no key and kills nothing; see the tenderengine documentation for the sequence and its bound.
//
// # Network gate
//
// InstallNetworkGate installs the process gate that holds network git and GitHub calls while a pause covers the process.
// It resolves the working directory and installs nothing outside a hub.
// A git worktree whose parent holds no board-level `_lyx`, such as an agent worktree nested in the prime, is outside one.
// The gate's waits write one line to stderr and the trace.
package tendercli
