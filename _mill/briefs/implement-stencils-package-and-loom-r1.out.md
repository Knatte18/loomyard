{"status":"success","commit_sha":"41e1ff0db67fa67866289ddf0b310980b20b9820","session_id":"1ff45c71-97e0-45f7-88fd-87d48304f8e9","cards_done":[7,8,9,10,11]}

All 5 of 5 cards committed and verified this turn — no partial completion.

Summary of work (all paths relative to `/home/knatte/Code/loomyard/wts/stencils-directory-reorg`):

- **Card 7** (`76842c9a`): `git mv` of `internal/loomengine/discussion-template.md` and `plan-template.md` into `stencils/loom/loom-template-discussion.md` and `loom-template-plan.md`; created `stencils/stencils.go` with the two `//go:embed` vars, the `entries`/`registry` type, and `Registry()`.
- **Card 8** (`a1ebfe50`): rewrote both moved files' leading banners to name their new paths and describe the stencils package/board seeding, body/markers untouched.
- **Card 9** (`0ccd52f2`, `30699b59` — combined, one card, two commits due to a staging slip on my part): deleted `internal/loomengine/prompttemplate.go` and `plantemplate.go`; `composePrompt`/`composePlanPrompt` and `PlanSpec`/`DiscussionSpec` gained a leading `stencilsDir` parameter and now call `stencilstore.Read` at call time, error returned unwrapped.
- **Card 10** (`52e19b14`): added `newTestStencilsDir` test helper in `internal/loomengine/prompt_test.go`, threaded through all `PlanSpec`/`DiscussionSpec`/`composePrompt`/`composePlanPrompt` call sites, and added `TestDiscussionSpec_ReadsStencilAtCallTime` and `TestDiscussionSpec_MissingStencilsDirIsHardError` in `discussion_test.go`.
- **Card 11** (`41e1ff0d`): widened `internal/lyxcwd/enforcement_test.go`'s `.md` vocabulary walk to `["internal", "stencils"]` with a non-zero visit-count guard; added `stencils/registry_test.go` pinning registry/on-disk parity plus `Default`/`RelPath` checks; pinned both relocated loom prompts to LF in `.gitattributes`.

Verify (`go build ./... && go test ./stencils/... ./internal/loomengine/... ./internal/lyxcwd/...`) passed cleanly on the final state. Working tree is clean (`git status --porcelain --untracked-files=no` empty).

{"status":"success","commit_sha":"41e1ff0db67fa67866289ddf0b310980b20b9820","session_id":"1ff45c71-97e0-45f7-88fd-87d48304f8e9","cards_done":[7,8,9,10,11]}
