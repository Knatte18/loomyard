// Package pullrequest holds a run's pull request: its record on disk and the GitHub calls on it.
//
// Record is the run's one JSON file about its pull request.
// It lives in the run's records directory, tracked with the run's other records,
// so it reaches another machine with the pair's push.
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
// SetDraft uses GraphQL, since the REST API cannot change a pull request's draft state.
// An error GitHub lists in the GraphQL response is a failure.
// Status reads the state (merged wins over closed), the head SHA and whether the pull request is a draft.
// A reviewer's standing review is the latest of their approvals, change requests and dismissals,
// so a later comment changes nothing.
// ReadMergeState judges whether GitHub would merge a pull request now, naming each cause when it would not;
// a mergeable_state GitHub has not yet computed is read again until MergeRetryWindow has passed on the told clock.
// A merge GitHub refuses with HTTP 405 or 409 returns a MergeRefusal.
// Its kind is RefusalMethod, RefusalPending or RefusalOther.
// Every other failure is returned wrapped, so githubclient.IsTransient still judges it.
//
// The package files no issue: filing LoomYard bugs is selfreport's alone.
package pullrequest
