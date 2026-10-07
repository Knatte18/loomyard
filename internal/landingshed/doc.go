// Package landingshed owns landing's three general producers, Publish, PR-Gate and Finalize, which any producer list may name -- none is special-cased by the engine that drives them.
//
// Publish checks the configured require_pr_to_base list (see LoadConfig) against the task's own
// parent branch. When the parent is absent from that list, Publish is a no-op: Done immediately, no
// merge-in, no push, no GitHub call.
// When the parent is present, Publish syncs the task branch against the parent through internal/mergeresolve, verifies the merged tree, pushes it, and opens the pull request, or refreshes an open one's title and body, from the change description (the file the Describe row writes, one source shared with the landing commit), then returns Done.
// The PR-Gate producer in this package then owns approval, rejection and the wait for the reviewer;
// PRGate.Call documents its decision table.
//
// Publish checks the task worktree is clean at three points: before the merge-in (after the status commit), after the merge-in, and after the verify.
// A dirty tree at any of them is Stuck naming the paths, before anything is pushed;
// a verify that dirties the tree is a non-hermetic test and halts rather than ships.
//
// After the merge-in, the told verify command runs through internal/verifytree before the push, unless its verified-tree record already names the tree and the command.
// The record is keyed on the tree, so the verify runs after a resolved conflict, after new parent commits and after a crash between a merge and its verify, and a no-op merge onto a verified tree skips it.
// A failure is Stuck with the merge commit kept for the operator to fix forward,
// and a missing command logs a warning and proceeds.
//
// The push that follows is never retried when the remote rejects it, because the remote task branch moved and a repeat would be rejected again.
// Publish reads the remote task branch's tip and the commits on it that the local branch lacks through Deps.RemoteOnlyCommits,
// and stops Stuck with a reason stating that the merge-in already ran, the tip, the commit count, and the way forward:
// merge `origin/<task-branch>` in the task worktree, then resume the run with `lyx loom start`, which re-runs Publish over the merged branch.
// A failed read keeps the rejection and the way forward and names the cause.
// A remote task branch that holds no commit the local branch lacks is not cleared by a merge, so the reason names a remote rule as the cause and the way forward is to clear it, then resume.
// Every other push failure keeps its own reason, and a transient one is returned as an error so the driver re-steps.
//
// require_pr_to_base is a list of base-branch names, not a bool, because whether a pull request is
// needed depends on which parent branch a task targets -- a per-task runtime fact no static profile
// setting can encode as one flag. A bool would force (or skip) a pull request on every task-to-task
// merge alike, which a stacked branch whose own parent is not the trunk branch already disproves.
//
// Finalize always syncs the task branch against the parent through internal/mergeresolve, regardless
// of which branch Publish took -- the only sync in the no-pull-request case, a second one catching
// whatever landed in the parent while a pull request sat out for review in the other case -- and then
// merges the task branch into the parent pair itself.
// Each catch-up merge-in, the retry after the parent moved included, runs the same three clean-tree checks and the same post-merge verify gate as Publish before the parent-side merge,
// and a dirty tree or a failure is Stuck with the parent branch untouched.
// The landing commit carries the change description and exactly one Co-Authored-By trailer, appended from landing.yaml's co_authored_by.
// After the parent-side merge and before the push, Finalize marks the board task done; after a
// successful push it closes the open pull request with a comment naming the landing commit. A parent
// that already holds the task's work makes Finalize idempotent: it takes the already-landed path
// rather than merging again. The parent branch is pushed to its upstream, so a landing reaches the
// remote rather than only the hub's own parent worktree. That parent-side merge is this producer's own
// merge critical section: the one span a future regeneration step folds into rather than running as a
// separate step before or after it, because splitting the two would let the parent advance in between
// the read and the write. Finalize.Call documents which of its own steps is that section; nothing here
// reserves a hook or a scaffolded span for the fold ahead of that future work landing, since an
// interface with a permanently-nil implementation is exactly the hypothetical-requirement design this
// codebase avoids.
//
// Finalize's parent-side merge squashes by default (landing.yaml's squash key, defaulting to true):
// one commit per merged task branch was the goal, weighed against the accepted cost that a squashed
// branch carries no merge commit linking it to its target, so "was this branch merged?" becomes
// unanswerable from git history alone afterward. The commit noise an ordinary merge would leave behind
// -- one surviving commit per card the task landed -- was judged the worse trade.
//
// The Describe session loads `scribe:prose` before its prompt.
// Its stencil, like the conflict session's, carries the parent directive that internal/parentdirective renders from the told name:
// DescribeInputs.ParentName for Describe, and Deps.ParentName passed through to the resolver for Publish and Finalize.
//
// It takes told absolute paths and has no direct production import of internal/lyxcwd, per the
// Told-Geometry Invariant: every path this package operates on is handed to it by its caller, and it
// derives none of its own.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary Invariant's
// owner set, so none of its identifiers, string literals, or comments may name either fabric-internal
// side, and that ban is machine-enforced by an AST walk over every identifier
// (internal/lyxcwd's TestEnforcement_FabricVocabulary).
package landingshed
