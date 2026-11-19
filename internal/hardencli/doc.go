// Package hardencli is the `lym harden` command tree: the verbs over the throwaway sandbox hubs of a hardener run.
// It holds the commands alone; the sandbox's building, listing and reaping are `internal/hardenshed`'s.
//
// # The verbs
//
// Each verb resolves its cwd's worktree and reads the run's seed.
// It refuses outside a worktree whose run's recipe is `hardener`, a worktree with no seed included,
// and the refusal names the way forward.
// `clear` also runs in a worktree whose sandbox root exists, whatever its run record says,
// so a teardown can clear a root its run record no longer names; `new` and `list` get no such exception.
//
// `lym harden sandbox new` builds a throwaway hub under the worktree's sandbox root.
// The hub is cloned from two local bare repositories, with no GitHub remote and no wiki,
// and its prime records commit a `loom.yaml` that turns Tier 2 self-reporting off.
// It rebuilds the worktree's `.dev-bin` lym only when that binary is not built from the worktree's HEAD or is modified.
// It prints the hub's path and the absolute path of the `.dev-bin` lym, as JSON, and writes only under the root.
//
// `lym harden sandbox list` prints the root's hub list, as JSON.
//
// `lym harden sandbox clear` reaps the root.
// It ends what the sandbox hubs left running, stops their reed servers, and removes the root's contents.
// It prints what it ended.
//
// # Bounds
//
// `clear` touches only reed servers whose key derives from a hub path the root's list records.
// It ends only processes whose cwd, executable or command line names the root.
// A hub made outside the root is outside it.
package hardencli
