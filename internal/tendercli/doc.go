// Package tendercli builds the `lym tender` group with `lym tender pause [<slug>]`, `lym tender resume [<slug>]`, `lym tender revive [<slug>]`, `lym tender status` and `lym tender reap` over internal/tenderengine, and installs the network gate every lym process in a hub runs under.
// `lym p` and `lym r` are shortcuts for the first two, so the operator spelling stays short.
//
// Every verb runs from any worktree of the hub and resolves it with lymcwd.Resolve.
// The hub's fixed dependency set bars cliwire from serving them, since it admits neither lymcwd nor fabricengine.
//
// # Callers
//
// The caller is read from the strand-name variable reed injects: empty is an external caller, the orch's name is the orch, and any other name is an agent of the slug it carries.
// The orch and an external caller may run `pause`, `resume` and `revive` with or without a slug.
// An agent may pause, resume or revive only its own pair, so a hub-wide call, or one naming another slug, is refused with the way forward of asking its parent session.
// `status` takes no caller check, since it writes nothing.
// Its processes part reads the hub's owner record and every worktree git still registers, prunable ones included, so a pair whose worktree exists keeps its slug's records out of the orphans.
// `reap` takes the caller check with no slug: the orch and an external caller may run it, and an agent is refused with the way forward of asking its parent session, before any hub read.
// The check guards against a mistaken call: the agents' settings deny any command naming the variable, so an agent cannot clear it to read as external.
//
// # Input
//
// `pause` refuses a slug with no pair, listing the slugs that have one, and a `--grace` outside 0 to 60 seconds.
// `resume` takes any slug, so an entry whose pair is gone can still be lifted; it succeeds with nothing resumed when nothing is paused, and the engine refuses a slug that only a hub entry covers.
// `revive` refuses a slug with no pair as `pause` does.
// `reap` takes no argument and ends the orphans of one fresh reading of the processes part, which `HubDeps` fills with `runproc.End` over the hub's owner record and `runproc.EndProcess`.
// The envelope carries the engine's report verbatim under `pause`, `status`, `resume`, `revive` or `reap`; a reap that leaves survivors exits non-zero with its report beside the error.
//
// `HubDeps` fills the lift's spawn seams: a pair's detached push over the pair's location, the prime's records push, and the board sync, which `syncBoardIfOwed` spawns when Bolt is ahead of its upstream or holds uncommitted changes, the state a sync deferred at its start leaves.
// The resume report names them under `spawned` and the failed ones under `spawn_failed`; a pair a live entry still covers, one another machine owns and one that is gone is not pushed.
//
// # Owned elsewhere
//
// `PauseOwnedElsewhere` pauses a run another machine owns, and `EndOwnedElsewhere` then ends the run's owned processes on this machine, so a driver chain parked under the owner pause never outlives an adoption.
// The pause stands whatever the end does: a failed end or a survivor is logged at Warn, naming `lym fabric remove <slug>`, or `lym reed down` in the pair while it exists, and `lym tender reap` once it is gone.
// A run whose Finalize is past its parent-side merge keeps its processes, since it finishes its landing on either machine; its pair's removal ends them.
// A task worktree with no status file, or none at all, is ended by slug, since no Finalize can be running there.
// The end spares the calling process and its ancestors, so inside `lym batten run` batten survives its own probe's end, keeping its owner hold and probe, and the pair's removal ends it.
// The hibernate wake fills its own end seam with `EndOwnedElsewhere`.
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
// A git worktree whose parent holds no board-level `_lym`, such as an agent worktree nested in the prime, is outside one.
// A detached push child is told its pause slug by its hidden flag, empty included, and starts in the hub's board worktree, so a non-nil pause slug resolves the working directory as a worktree root and replaces the slug the directory would give.
// A `DetachedChild` carries the hidden flags a detached child was spawned with.
// A `Deferrable` child that is not `ParentLocked` installs a deferring gate, which exits the child under a pause instead of parking it; a `ParentLocked` child's gate lets its own slug's owner entry pass.
// A `HubScope` child, the detached board sync, is held under the empty slug, so only a hub entry holds it whichever worktree spawned it.
// The gate's waits write one line to stderr and the trace.
package tendercli
