# One-repository fabric cases no hardener round drove

Live drive of three cases on a one-repository sandbox hub, run after `harden-fabric-undriven-cases` landed (`540e8c9b5`).
The hub was cloned from a local bare origin, so no GitHub repository was touched.

```yaml
date: 2026-11-23
lym_build: /home/knatte/go/bin/lym at 540e8c9b5
origin: a local bare repository under the task worktree's .scratch
hub: /home/knatte/Code/sbund-sandbox/undriven-origin-LYMHUB (shortname sbund)
result: one fault found (filed as a board note); the other behaviours matched their documentation
```

## Result

- A discard or remove killed mid-operation leaves a pending-teardown record that `lym fabric status` lists;
  re-running `lym batten cancel <slug> --remove` finishes the teardown and clears the record.
- A step that fails with no kill (origin rejects the branch delete) leaves a `discard` record carrying
  `rerun: lym batten cancel <slug> --remove`, the new field, and the re-run recovers.
- `lym adopt --archives root` lists the owner archive tags whose commit the slug's branch on origin lacks, on both sides,
  and skips a tag the branch contains.
- Fault: the way forward of a `remove` record names `lym fabric remove <slug> --remote`, which answers `pair not found` once the pair is gone.

## Method

The task session cannot run `lym fabric clone` or `lym fabric add`, so the orch cloned the hub and added the pairs;
everything else ran from the task session.
Origin hooks (a `pre-receive` script in the bare origin) stalled or rejected branch deletes;
a stalled run was killed with `kill -9`, the foreground command and its detached `lym fabric remote-teardown` children both.
Each case used a pair with one local commit origin lacked.

## 1. A discard killed mid-operation

`lym batten cancel <slug> --remove` with a hook that sleeps on `refs/heads/<slug>` deletes, then killed.

- The foreground command does not wait out the stall by design: the remote half runs in a detached `remote-teardown` child
  that outlives a kill of the foreground command and keeps the slug's teardown lock.
  While that child lives, `lym fabric status` lists no pending teardown; it lists the record once the child is killed.
- The record's kind is `remove`, not `discard`, in both killed runs (`disc1`, `disc3`), because the pair's removal ran first
  and the delete that stalled was its remote step.
  A discard-kind record was reached only through the rejected delete in case 2.
- Re-running `lym batten cancel <slug> --remove` after the kill, with the hook gone, archived the local tip
  (`archive/<slug>/<sha12>` pushed under `refs/lym/code-tags/`), deleted the branch on origin and locally,
  and cleared the record.
- Fault: the record's way forward ends in `lym fabric remove <slug> --remote`.
  Run on `disc1` after the pair was gone it answered `{"error":"pair not found: nothing of \"disc1\" remains",...}`;
  the command that finished the teardown was the batten re-run.

## 2. A fabric step that fails with no kill

A hook rejected every branch delete (`exit 1`) while `lym batten cancel fail1 --remove` ran.

- The foreground envelope answered `ok: true`, `pair_removed: true`, an empty `way_forward`;
  the failed delete showed only in `lym fabric status`, as a `discard` record with
  `rerun: "lym batten cancel fail1 --remove"` and a way forward naming the archive-tag push, then the re-run.
- The named archive-tag push was already done (the tag was on origin); only the re-run was needed.
- With the hook removed, the re-run recovered: the branch is gone from origin and no record remains.
  The first re-run overlapped the hook's removal, so the recovery under a still-rejecting hook was not isolated.

## 3. `lym adopt --archives` over owner archive tags

The owner archive tags were pushed by hand, since a real takeover needs a second machine:
one code tag and one records tag at commits origin's `arch1` branches lacked, and one code tag at the branch tip.

- `lym adopt --archives arch1` refused: `arch1` is not on the board (a task is not a subject).
- `lym adopt --archives root` listed the code tag and the records tag with their `look` command, and skipped the tag at the tip.
  The listed tag was readable locally with `git log` afterwards.
- With the `arch1` pair cancelled, the branch on origin was gone and the tag at the old tip was listed too,
  as the documented "a branch origin lacks contains no commit" rule says.

## Not driven

- A real takeover producing owner archive tags (needs a second machine).
- A discard-kind record from a kill, which `lym batten cancel` did not reach; only the cleanup path's discard remains untried.
