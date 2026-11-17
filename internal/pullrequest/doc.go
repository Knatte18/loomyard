// Package pullrequest holds a run's pull request: its record on disk and the GitHub calls on it.
//
// Record is the run's one JSON file about its pull request.
// It lives in the run's scratch directory, so it is never tracked and goes with the pair.
// It holds the pull request's number, URL, and base and head branches.
// It also holds the head the PR-Review row last gated and pushed and the head whose gate failure was mailed.
// It holds the interactive session's strand and session identity and engine, and the operator's continue flag.
// It holds whether the review ended.
// The row ends it on that flag.
// Finalize ends it on the record it opens itself, for a parent that takes its pull request with no review.
// ReadRecord, WriteRecord, UpdateRecord and RemoveRecord go through internal/state,
// so a key a newer binary wrote survives a rewrite.
// An absent file reads as not found.
// UpdateRecord on an absent record returns ErrNoRecord and writes nothing.
// RemoveRecord of an absent file is no error.
//
// A Client is bound to one repository.
// Open authenticates only through githubclient and calls go-github directly, never the gh command;
// NewWithClient binds a told client, which tests point at an httptest server.
// The API interface lists the calls a caller makes, so a test of the caller fakes GitHub behind it.
// Each call takes the caller's context, and githubclient's own per-request time bound applies as well.
// RequiresPullRequest reports whether a branch's active rules include a pull_request rule.
// GitHub refuses a direct push under such a rule.
// It reads `GET repos/{owner}/{repo}/rules/branches/{branch}`.
// SetDraft changes a pull request's draft state, since the REST API cannot.
// It runs the GraphQL `convertPullRequestToDraft` or `markPullRequestReadyForReview` on the node id.
// An error GitHub lists in the GraphQL response is a failure.
// Status reads the state (merged wins over closed), the head SHA and whether the pull request is a draft.
// It also reads GitHub's mergeable_state.
// It reads whether a standing review approves that head or requests changes;
// a reviewer's standing review is the latest of their approvals, change requests and dismissals,
// so a later comment changes nothing.
// RequiredChecks reads every page of a base branch's active rules for its required status-check contexts.
// It then reads each context's conclusion at a head:
// from the check run of that name, else from the commit status of that context, else CheckMissing;
// a pending commit status reads as unfinished.
// ReadMergeState reads a pull request's Status and the required checks of its base.
// It judges whether GitHub would merge it now, naming each cause when it would not;
// a mergeable_state GitHub has not yet computed is read again until MergeRetryWindow has passed on the told clock.
// A merge GitHub refuses with HTTP 405 or 409 returns a MergeRefusal.
// Its kind is RefusalMethod when the merge method is not allowed.
// Its kind is RefusalPending when the base branch was modified or mergeability is still being computed.
// Its kind is RefusalOther for any other refusal.
// Every other failure is returned wrapped, so githubclient.IsTransient still judges it.
//
// The package files no issue: filing LoomYard bugs is selfreport's alone.
package pullrequest
