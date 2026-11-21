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
// A `between` error or a release error leaves a draft entry, which is closed and holds no start.
// A record write that fails after the release is logged, and `File` returns nil; `Settle` records the started entry.
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
// `Remove` drops the record and the entry under the record's lock, which `Settle` holds across its read and writes.
// A settle running beside a removal therefore never runs the action of an entry the removal is deleting.
// A failing step stops the removal with a `*RemoveError` that names the hand removal.
//
// # Settling
//
// A dobby calls `Settle` each tick for its worktree's project.
// When this machine does not own the worktree's subject, it drops the project's records and runs nothing.
// It records each open front entry that has no record, so a filing that died before its record is healed.
// It runs the action of each record whose entry is gone or done, once, and drops the record.
// A failed action keeps its record for the next settle and is returned with the hand step from the table.
// An entry that is draft, halted or in any other status keeps its record and runs nothing.
//
// The action table is closed: a record naming a key outside it runs nothing, so no board edit makes dobby run a command.
package fronttask
