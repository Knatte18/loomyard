Fixed the stale pattern-attribution NIT in holistic review round 1.

- Reviewed `internal/reedengine/config_test.go:1-3` — confirmed via VERIFY: finding accurate. `shuttleengine` (card 4, precedes reedengine's card 5) already carries the template-fallback pattern; `fabricengine` was deliberately untouched (per `no-strings-contains-migration`) and only has the old strict not-initialized pattern. No harm from the fix — pure comment change.
- Checked for a systemic sweep: `shuttleengine`'s and `perchengine`'s attribution chains are correct (both point backward to packages that do have the fallback pattern) — only `reedengine`'s link was stale. Single-file fix, no sweep needed.
- Edit: repointed the attribution from `fabricengine's` to `shuttleengine's` in `internal/reedengine/config_test.go`.
- Lint/vet/build clean on the change (pre-existing unrelated `errcheck` finding in `lock.go` confirmed untouched via `git diff --stat` and `--new-from-rev`).
- Codeguide not initialized for this repo (`found: false`) — sync skipped.
- Committed and pushed: `04cc00c56c053b2184426ae038e0bde40e198cd7` — "mill-go2: fix stale pattern-attribution in reedengine test header".
- All three batch verify commands pass (`internal/configengine`, the four producer packages, `internal/lyxcwd`).

{"status":"success","commit_sha":"04cc00c56c053b2184426ae038e0bde40e198cd7","session_id":"431c8623-6763-4c52-a9ff-c796cf01f06e"}
