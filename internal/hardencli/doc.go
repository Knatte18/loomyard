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
// `lym harden sandbox new` builds a throwaway hub under the worktree's sandbox root, with no GitHub remote and no wiki.
// By default the hub is a two-repository hub cloned from two local bare repositories.
// With `--single-repo` it is a one-repository hub cloned from one local bare repository.
// That repository holds the records and the board under `refs/lym/`.
// Its prime records commit a `loom.yaml` that turns Tier 2 self-reporting off.
// It prints the hub's path, the absolute path of the `.dev-bin` lym and `single_repo`, as JSON.
// It writes only under the root.
//
// `lym harden sandbox list` prints the root's hub list, as JSON.
//
// `lym harden sandbox clear` reaps the root and prints what it ended.
//
// # Bounds
//
// `clear` touches only reed servers whose key derives from a hub path the root's list records.
// It ends only processes whose cwd, executable or command line names the root.
// A hub made outside the root is outside it.
package hardencli
