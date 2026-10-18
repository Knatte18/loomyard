# PATTERN-push-both-sides

Go pushes a run's records side, and in a task pair the code branch goes with it.

- In a task pair the records push carries the code branch along, rebase-free, under fabric's absorbing push lock.
- The push is never left to an agent.
- A per-transition status push is spawned through fabric's detached push, never run by the seam that committed; only `commit-records` pushes synchronously.
- The prime's pushes stay records-only, its code branch being the operator's.
