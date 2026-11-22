// Package beamsetting names the module, keys and values of beam.yaml's per-task settings.
// Those settings are pull_request and land_notice.
// Every package that reads, writes or refuses one of them takes its name from here, so each is declared once.
package beamsetting

// Module is the name of the config module that holds the per-task settings.
const Module = "beam"

// The keys of beam.yaml's per-task settings.
const (
	// PullRequestKey decides whether a task's landing goes through a GitHub pull request.
	PullRequestKey = "pull_request"
	// LandNoticeKey decides whether dobby posts a line to the orch once a task's run lands or ends without landing.
	LandNoticeKey = "land_notice"
)

// The values of beam.yaml's pull_request.
const (
	// PullRequestOn lands through a pull request when the origin allows one.
	PullRequestOn = "on"
	// PullRequestOff lands with no pull request unless GitHub's rules for the parent branch require one.
	PullRequestOff = "off"
	// PullRequestMerge lands through a pull request squash-merged on GitHub at once, with no review.
	PullRequestMerge = "merge"
	// PullRequestMergeIn lands a task that merges its parent's parent branch into its parent with no pull request.
	// It is a true merge that keeps the merged commit as a parent.
	PullRequestMergeIn = "merge-in"
)

// The values of beam.yaml's land_notice.
const (
	// LandNoticeOn sends the landing notice.
	LandNoticeOn = "on"
	// LandNoticeOff sends none.
	LandNoticeOff = "off"
)

// PullRequestValues returns every value pull_request takes.
func PullRequestValues() []string {
	return []string{PullRequestOn, PullRequestOff, PullRequestMerge, PullRequestMergeIn}
}

// LandNoticeValues returns every value land_notice takes.
func LandNoticeValues() []string {
	return []string{LandNoticeOn, LandNoticeOff}
}
