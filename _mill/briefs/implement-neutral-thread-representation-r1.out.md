No dirty tracked files. All 3 cards committed, matching the batch's 3 declared cards. Card-count self-check: 3 of 3 cards committed — this is genuinely complete, verified by matching commit subjects against the batch file's Commit: messages.

Summary of work:

- `plugins/prowler/redditoauth_test.go` — added then relocated the golden-file test scaffolding; final state exercises credentials, User-Agent, token request/errors/caching, OAuth URL rewriting, and `fetchRedditOAuthThread` end to end.
- `plugins/prowler/testdata/reddit-thread-golden.md` — new golden file capturing the pre-refactor OAuth formatter output byte-for-byte.
- `plugins/prowler/redditformat.go` — new file holding the tier-neutral `redditPost`/`redditComment` types, the re-signatured `formatRedditThread(post redditPost, sourceURL string) string`, and the new `redditPostFromListings` OAuth mapping (untruncated) plus its `redditCommentFromChild` helper.
- `plugins/prowler/redditformat_test.go` — new file with the moved `TestFormatRedditThread`/golden test plus card 3's new coverage (nil-score rendering, heading discriminator, cap placement, link rendering, mapping kind-filtering).
- `plugins/prowler/redditoauth.go` — removed the old `formatRedditThread`, routed `fetchRedditOAuthThread` through `redditPostFromListings` + `formatRedditThread`, updated file doc comment.

Verify (`go -C plugins/prowler test .`) passes, including the golden regression, confirming the refactor changed nothing observable on the OAuth path.

{"status":"success","commit_sha":"e1c302abd27beada27a0f290ecf5825295eb866f","session_id":"6faf9e0e-9b65-40ae-a376-513fe1a369ef","cards_done":[1,2,3]}