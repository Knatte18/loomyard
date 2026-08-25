All 3 declared cards (7, 8, 9) have matching commits. 3 of 3 cards committed — all complete.

{"status":"success","commit_sha":"6403ce74f387961caf9595f4bdd026d6eb1a7a09","session_id":"3625e886-9141-4f66-8a87-a820a3f1c0d5","cards_done":[7,8,9]}

Summary: implemented all 3 cards of batch `tiered-adapter` (3 of 3 cards committed).

- Card 7: `plugins/prowler/fetcher.go`, `plugins/prowler/headers.go`, `plugins/prowler/main.go`, `plugins/prowler/fetch_test.go` — added `doNoRedirect` transport field and `noRedirectHTTPClient`.
- Card 8: `plugins/prowler/fetch.go`, `plugins/prowler/reddit.go`, `plugins/prowler/fetch_test.go`, `plugins/prowler/reddit_test.go` — `fetchOldRedditHTML` now returns `(string, error)`, detects the login redirect via `f.doNoRedirect`, and runs `looksLikeBlockPage` after decode.
- Card 9: `plugins/prowler/reddit.go`, `plugins/prowler/reddit_test.go`, `plugins/prowler/fetch_test.go`, `plugins/prowler/README.md` — `redditAdapter.Fetch` now runs the OAuth tier then the old.reddit.com tier then an error tier, always `handled=true`, never calling `f.browser`.

Verify command (`go -C plugins/prowler test -run 'TestReddit|TestFetchOldRedditHTML|TestFetchPage|TestRunAll|TestFormatRedditThread|TestFetchRedditOAuthThread' .`) passes, both plain and with `-race`. Working tree is clean of tracked in-scope modifications; commits are pushed to `prowler-fix-reddit-block`.

{"status":"success","commit_sha":"6403ce74f387961caf9595f4bdd026d6eb1a7a09","session_id":"3625e886-9141-4f66-8a87-a820a3f1c0d5","cards_done":[7,8,9]}
