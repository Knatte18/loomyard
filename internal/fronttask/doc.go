// Package fronttask files, removes and settles a front task:
// a board task lym files ahead of its worktree's other starts, which runs a registered action once it lands.
// It knows nothing of the adopt check but the `adopt-check` row of its action table.
//
// # The task
//
// A front task is a task entry whose `front` field is true.
// It is open while its status is empty or a run status; done, draft or any other status is closed.
// Only `File` creates one, through the board's one write that sets `front` and `on_land`.
// No board edit can therefore make a task a front task or change its action.
// While one is open, the dobby of its worktree starts no other task there.
// A worktree holds at most one open front task: `File` refuses a second.
//
// # Filing
//
// `File` refuses an `OnLand` outside the action table and a project that already holds an open front task.
// Both refusals come before any write.
// It then creates the entry as a draft and runs the caller's `between` step.
// It releases the entry and records its action and project under its slug.
// A `between` error or a release error leaves the draft entry, which is closed and holds no start.
// A record write that fails after the release is logged, and `File` returns nil.
// The released entry starts, and `Settle` records it.
//
// # The record
//
// The record file `records.json` sits in the front-task directory of the hub's never-tracked scratch tree.
// It has its own lock.
// It maps a slug to the action to run and the project that holds the task.
// It outlives a dobby restart, so a front task that lands while its dobby is down gets its action at the next settle.
//
// # Removing
//
// `Remove` cancels the slug's run when one exists, drops its record and removes its entry.
// The last two steps run under the record's lock, which `Settle` holds across its read of the board and its writes.
// A settle running beside a removal therefore never runs the action of an entry the removal is deleting.
// A failing step stops the removal with a `*RemoveError` that names the hand removal.
//
// # Settling
//
// A dobby calls `Settle` each tick for its worktree's project.
// When this machine does not own the worktree's subject, it drops the project's records and runs nothing.
// Otherwise it records each open front entry that has no record,
// so a filing that died between its release and its record is healed before dobby starts the task.
// It then runs the action of each record whose entry is gone or done, once, and drops the record.
// A failed action keeps its record for the next settle and is returned with the hand step from the table.
// An entry that is draft, halted or in any other status keeps its record and runs nothing.
//
// The action table is closed: a record naming a key outside it runs nothing, so no board edit makes dobby run a command.
//
// # Hub wiring
//
// `HubDeps` and `HubSettleDeps` fill the seams over a hub; a test fakes them.
// The action runs as a detached spawn of the running `lym` executable in the prime's anchor.
// Its output is appended to a file in the front-task directory.
package fronttask
