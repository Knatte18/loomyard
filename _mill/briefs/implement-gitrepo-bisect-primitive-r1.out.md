Both cards (13 and 14) are committed and match their declared `Commit:` messages exactly. Verify passes (`go test -tags integration ./internal/gitrepo/...` → ok). 2 of 2 cards committed — the full batch is complete.

Summary of work:
- `internal/gitrepo/gitrepo.go` — added `CurrentBranch`, `CheckoutDetached`, `RestoreBranch` methods on `*Repo`, all routed through the existing unexported `run` choke point, with `CheckoutDetached` validating input via `validSHA`/`ErrInvalidSHA`.
- `internal/gitrepo/doc.go` — added the three methods to the Repo API bullet list and narrowed the Scope boundaries section to admit the detached-checkout + branch-restore pair as the single in-place-bisect exception.
- `internal/gitrepo/gitrepo_test.go` — added `//go:build integration` tests: `TestBisectPrimitives_DetachRestoreCycle`, `TestCheckoutDetached_RejectsNonHexSHA`, `TestCurrentBranch_ErrorsOnDetachedHEAD`.

Commits: `373d0f08` (Card 13), `05fbfff4` (Card 14). Both pushed to `webster-rewrite`.

{"status":"success","commit_sha":"05fbfff4","session_id":"78d1c371-7f42-4974-95df-898c0a396b37","cards_done":[13,14]}
