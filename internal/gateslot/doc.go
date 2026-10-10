// Package gateslot owns the hub-wide pool of gate slots that bounds how many gate builds and test runs the hub executes at once.
//
// The package is told every directory it works on and derives none beyond the Dir and WaitDir joins, which spell the ephemeral directory name only through internal/lyxdirs.
//
// # Slots
//
// A Pool holds one OS file lock per slot at `<Dir>/slot-<n>.lock`, n from 1 to the current Limits.Slots.
// Acquire takes the first free slot and writes its Holder record (worktree, site, pid, start time) as YAML at `<Dir>/slot-<n>.yaml`.
// While every slot is held it logs the wait start, polls every Pool.Poll and logs the wait end on acquiring.
// Limits is a closure read at every attempt, so a lowered count takes effect on the next acquire while a running holder of a higher slot finishes.
// Acquire imposes no deadline: a caller wanting a bound passes a deadline context.
// Polling gives no FIFO order among waiters.
//
// An OS lock is released by process death, so a crashed holder never leaks a slot; its holder record stays on disk but Holders trusts a record only while its lock is held.
// Holders reads every slot record in the directory, so a holder of a slot above a lowered count still shows.
//
// # Inheritance
//
// Lease.Env returns the environment for a child that runs inside the slot: GOFLAGS gets `-p=<GoParallel>` appended and InheritEnv names the held slot's lock path.
// A nested gate run checks Pool.Inherited on that variable and runs inside its parent's slot without acquiring.
//
// # Config
//
// The hub-wide `gate` config module keeps `slots`, `go_parallel` and `cli_wait_sec` at `<BoardDir>/_lyx/config/gate.yaml`, seeded from template.yaml by the hub-wide reconcile.
// LoadConfig reads it strictly and refuses any value below 1 naming its key; Config.Limits converts it to the Limits a Pool reads.
// TemplateConfig decodes the template alone, for a run outside every hub.
//
// # Wait records
//
// A waiter that has no verifytree marker keeps a Wait record, one uniquely named YAML file in WaitDir under its worktree's anchor root.
// WriteWait creates it and the writer removes it; ReadWaits returns only the records whose process is alive.
package gateslot
