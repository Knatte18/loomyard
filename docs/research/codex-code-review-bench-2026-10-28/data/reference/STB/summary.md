# A loop child start survives a deploy replacing the lym binary

A loom run no longer ends `failed` because a deploy was rewriting the `lym` binary when the loop started a child step.
A child start that does fail now leaves a halt note that explains itself.

Closes #893

## What changes

- `internal/proc` gains `IsSpawnBusy`, the one predicate for a start refused with ETXTBSY because the executable was open for writing.
- `internal/shedverbs` loop:
  - A child start refused with ETXTBSY is tried again, five tries in all, waiting 100 ms before the first retry and doubling after.
    The waits go through the existing `LoopSpec.Sleep` seam, and each retry is logged at Warn.
    Any other start error fails at once, as before.
  - A start that still fails keeps the `start-failed: <error>` cause, and for ETXTBSY adds the number of tries and that the executable stayed open for writing, as while a deploy rewrites it in place.
  - The new `LoopSpec.LoopTraceFile` names the loop process's own trace file, which holds the start warnings.
    A child that never started takes it as its interrupted stop's trace file when there is no child trace and no non-empty stderr, so the halt note's `trace_file` is no longer empty.
    `internal/loomcli` and `internal/battencli` arm it with `logger.TraceFile`.
- `tools/deploy`, which `update-plugins.sh`, `deploy-dev` and the hub's Land-row `deploy` command all run, builds `lym` into a hidden temporary file in the install directory and renames it over the target.
  A failed build or rename removes the temporary file and leaves the installed binary as it was.

The `proc` and `shedverbs` package docs and the `tools/deploy` command doc are updated to match.

## Design decisions

- `go build -o` copies into an existing target in place when its work directory sits on another filesystem, as with a tmpfs `/tmp`, and that window is where exec returns ETXTBSY.
  A rename within the install directory replaces the directory entry at once, and a running process keeps the old file.
  So the rename removes the cause, and the start retry covers a binary written by other means.
- The retry lives at the loop's child start only, the site the failure occurred.
  ETXTBSY is refused before the child runs, so a retry never runs a step twice.
- A failed start keeps the `interrupted-stop` anomaly kind and gets no transient class.
  The cause and trace file now carry what the classification hid, and a re-step after the retry bound would only repeat the refusal.
- The temporary binary sits in the install directory, not in `lymtemp`'s run directory, because a rename across filesystems is no atomic replace.
- On Windows a rename over a running `lym.exe` fails, as `go build -o` over it did before; the change keeps that behaviour.

## Check by hand

- No real ETXTBSY was provoked: the loop test stubs the runner and records the waits.
- Run `./deploy-dev` while a loop step is starting, and confirm `.dev-bin` holds only `lym` afterwards.
