// Package pullrequest holds a run's pull request: its record on disk and the GitHub calls on it.
//
// Record is the run's one JSON file about its pull request, in the run's scratch directory so it is never tracked and goes with the pair.
// It holds the pull request's number, URL, base and head branches, the head the PR-Review row last gated and pushed, the head whose gate failure was mailed, the interactive session's strand and session identity and engine, the operator's continue flag, and whether the review ended, by the row on that flag or by Finalize on the record it opens itself for a parent that takes its pull request with no review.
// ReadRecord, WriteRecord, UpdateRecord and RemoveRecord go through internal/state, so a key a newer binary wrote survives a rewrite;
// an absent file reads as not found, UpdateRecord on an absent record returns ErrNoRecord and writes nothing, and RemoveRecord of an absent file is no error.
//
// A Client is bound to one repository.
// Open authenticates only through githubclient and calls go-github directly, never the gh command; NewWithClient binds a told client, which tests point at an httptest server.
// The API interface lists the calls a caller makes, so a test of the caller fakes GitHub behind it:
// FindOpen, Create, Edit, Status, Close, Reopen, SetDraft, Comment, Merge, RequiresPullRequest and RequiredChecks, each taking the caller's context, with githubclient's own 30-second bound applying per request.
// RequiresPullRequest reads the repository rules active on a branch, `GET repos/{owner}/{repo}/rules/branches/{branch}`, and reports whether one is a pull_request rule, under which GitHub refuses a direct push.
// SetDraft runs GitHub's GraphQL `convertPullRequestToDraft` or `markPullRequestReadyForReview` on the pull request's node id, since the REST API sets neither, and an error GitHub lists in the GraphQL response is a failure.
// Status reads the state (merged wins over closed), the head SHA, whether the pull request is a draft, GitHub's mergeable_state, and whether a standing review approves that head or requests changes;
// a reviewer's standing review is the latest of their approvals, change requests and dismissals, so a later comment changes nothing.
// RequiredChecks reads every page of the rules active on a base branch for its required status-check contexts, then each context's conclusion at a head:
// from the check run of that name, else from the commit status of that context with a pending one read as unfinished, else CheckMissing.
// ReadMergeState reads a pull request's Status and the required checks of its base and judges whether GitHub would merge it now, naming each cause when it would not;
// a mergeable_state GitHub has not yet computed is read again until MergeRetryWindow has passed on the told clock.
// A merge GitHub refuses with HTTP 405 or 409 returns a MergeRefusal of kind RefusalMethod when the merge method is not allowed, RefusalPending when the base branch was modified or mergeability is still being computed, and RefusalOther for any other refusal;
// every other failure is returned wrapped, so githubclient.IsTransient still judges it.
//
// The package files no issue: filing LoomYard bugs is selfreport's alone.
package pullrequest
