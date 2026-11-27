// Package landingshed owns landing's general producers: Parent-Align, the PR-Review row and Finalize.
// Any producer list may name them, and the engine that drives them special-cases none.
//
// ResolvePullRequest is the one decision of whether a landing goes through a pull request.
// PR-Review and Finalize therefore cannot disagree.
// It reads no file: its callers tell it the beam.yaml `pull_request` mode that the per-task lookup found for the landing.
// They also tell it the origin facts and whether the task branch has commits beyond its parent.
// A yes marked MergeAtOnce is opened and squash-merged at once with no review.
// A `merge-in` mode on a landing that is no beam's is a no marked TrueMerge.
// So the beam keeps the parent's commit as an ancestor.
// GitHub's rules that require a pull request count as required whatever beam.yaml says.
// A repository whose plan has no repository rules has none, so GitHub's 403 for it reads as no rule, never a failure.
// Bound: a rule the read misses ends in a direct push GitHub refuses.
// So does branch protection set outside repository rules.
//
// PR-Review takes no parent lock and never lands a run by a direct push.
// No agent takes part in it: when the pull request is made, the run's agents are done.
// Under beam.yaml's review the row opens the pull request and waits on it, whatever the run's autonomy.
// The wait ends only on the operator's own ending:
// an approval or a merge on GitHub, or `lym loom pr-continue`, which refuses a call from any lym agent session.
// A wait on the pull request has no timeout.
// A commit made in the task worktree while the row waits is gated and pushed;
// a gate failure, and a commit pushed to the pull request's branch on GitHub by hand, halt the row.
// A transient push or GitHub failure is an error the driver re-steps.
// Any other failure is Stuck with Halt, and each Halt names its way forward.
//
// Parent-Align takes no parent lock across the merge and writes nothing in the parent pair.
// It merges the parent's upstream tip into the task worktree and gates the merged tree with its own gate key.
// A failing gate or a merge that brought parent commits runs the merge fixer in parent-align mode.
// A merge or test failure at Parent-Align or Finalize is owned by the recipe:
// it ends in the row's fixer or a halt to the orch, and Finalize never sends the run back to an earlier row.
// The task worktree must be clean before and after each merge and after the verify;
// a dirty tree is Stuck naming the paths, so nothing uncommitted is merged over or reviewed.
//
// Finalize holds the parent lock, taken through the shared guard, from after the change description is parsed.
// It holds it until every way out of its Call, and it re-reads the lock record before each landing attempt.
// A record that does not name the run, or an unreadable one, is Stuck with Halt.
// Finalize merges the parent branch into the task worktree first and then merges the task branch into the parent pair.
// It never pushes or rewrites a parent that is ahead of or diverged from its upstream.
// It lands only on the machine that owns the run and its parent; any other owner halts behind OwnerHaltPrefix.
// A parent that already holds the task's work makes Finalize idempotent:
// it takes the already-landed path rather than merging again.
// The parent-side merge squashes by default, except a landing the resolver marks TrueMerge.
// Finalize.Call documents which of its steps is the merge critical section.
// When a pull request is wanted, Finalize merges it on GitHub under the lock instead of landing by a local push.
//
// Each row gates the merged tree through internal/verifytree under its own told gate key.
// A gate whose result is a runner crash or a memory pressure stop judged no code:
// the row halts with LandingStopPrefix, spawns no fixer and writes no landing failure record.
// A failing gate spawns the merge fixer, whose whole change is judged against the size and kind caps.
// A change over the caps lands only behind the fix review's approval or the operator's acceptance.
// Each of these halts with LandingStopPrefix for the orch:
// a spent fixer, a fix review out of rounds, a turn without a usable result and a tree left dirty.
// Bound: the fix review lets an over-cap change land behind one reviewer's approval of the fixer's own commits.
// A fix within the caps lands with no perch, so only the gate it passed stands behind it.
//
// HandsBack is the one predicate that names the halts the orch takes over:
// a blocked stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix.
// An escalate report halts under EscalateStopPrefix on a tapestry, darn or sampler run, which is no hand-back.
// On a pawl run it halts under LandingStopPrefix.
//
// Landing config is hub-wide.
// Parent-Align and Finalize read it from the board dir, never from a task worktree's `_lym/config/`.
// A task therefore cannot weaken `deploy`, or set `deploy_branches` or `squash`, through its own tree.
//
// The landing commit carries the change description, the regression check and the credit block.
// They are separated by blank lines; the credit block is plain lines, never git trailers.
// PR-Review sets the pull request's body to the same text, so the commit and its pull request carry one credit.
// The pull request's title and body are set again when GitHub holds other text than the change description yields.
// PR-Review does this at every poll.
// Finalize does it before it merges a recorded pull request that is not a beam's.
//
// DescribeTable builds the Describe session: the lead writes the change description and an assistant reviews it.
// The reviewer checks the text for the tooling's terms and for missing deliverables.
// It also checks for claims the diff does not bear out.
// The table's gate runs behind the row's own description gate.
// Its first call releases the reviewer, which waits for it before it reads the description.
// It holds the lead's turn end until the reviewer's end marker exists or the review's time runs out.
// It then hands the lead a review with findings once.
//
// DraftBeamDescription writes a starting point for a beam's change description and starts no agent.
// RecordBeamDescription validates the orch's text with the check the Describe gate runs, and commits it.
//
// This package takes told absolute paths.
// It has no direct production import of internal/lymcwd, per the Told-Geometry Invariant:
// every path it operates on is handed to it by its caller, and it derives none.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set.
// None of its identifiers, string literals, or comments may name either fabric-internal side.
// An AST walk over every identifier enforces that ban (internal/lymcwd's TestEnforcement_FabricVocabulary).
package landingshed
