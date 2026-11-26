# PATTERN-landing

Landing on a local parent branch or deciding on a pull request: only in a run's Finalize under `internal/parentlock`'s lock; landingshed's one resolver decides the pull request.

## Parent lock

Rule: Moving, merging into or landing on a local parent branch, or merging a run's pull request: only in a run's Finalize, under `internal/parentlock`'s lock for that parent.

## Pull request mode

Rule: Deciding if a landing uses a pull request: only landingshed's one resolver, from the landing's `beam.pull_request` as the per-task lookup reads it from the landing run's worktree up, a beam's pull-request record, the parent's GitHub rules, origin and task commits; a yes merges on GitHub.
A task whose `beam.pull_request` is `merge-in` is a no under every rule.
Its parent-side merge never squashes.
