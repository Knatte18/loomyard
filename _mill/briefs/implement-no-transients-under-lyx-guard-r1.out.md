Both card commits present, matching both `Commit:` messages in the batch file. 2 of 2 cards committed -- full completion confirmed.

{"status":"success","commit_sha":"ecad655f34ab4487a03124009db5e290b810f488","session_id":"8ec8392c-f5cb-487a-8cb1-65789c838a4a","cards_done":[31,32]}

Both cards in batch `05-no-transients-under-lyx-guard.md` are committed: 2 of 2 cards committed (all complete, honestly verified via `git log <range-start>..HEAD --oneline` matching both `Commit:` messages).

Summary:
- Card 31: created `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/cmd/lyx/notransients_test.go` with `TestNoTransientsUnderLyx`, an untagged Tier 1 test asserting the durable/transient path split and mirrored-subpath equality for every module's exported path constructor, across both unanchored and subpath-anchored `*lyxcwd.Location` fixtures. Commit `ab03f771`.
- Card 32: edited `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/CONSTRAINTS.md`, adding `## Durable-vs-Ephemeral State Invariant` after the Lyxdirs Single-Declarer Invariant, and a cross-reference bullet in the Cwd Resolution Invariant. Commit `ecad655f`.

Verify (`go test ./cmd/lyx/...`) passes. `git status --porcelain --untracked-files=no` is clean of tracked modifications. Final HEAD: `ecad655f34ab4487a03124009db5e290b810f488`.

{"status":"success","commit_sha":"ecad655f34ab4487a03124009db5e290b810f488","session_id":"8ec8392c-f5cb-487a-8cb1-65789c838a4a","cards_done":[31,32]}
