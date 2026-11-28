// Package beamsetting names the module, keys and values of beam.yaml's per-task settings.
// Those settings are beam.pull_request, beam.land_notice, beam.autonomy and beam.run_watch.
// Every package that reads, writes or refuses one of them takes its name from here, so each is declared once.
package beamsetting

// Module is the name of the config module that holds the per-task settings.
const Module = "beam"

// The keys of beam.yaml's per-task settings.
const (
	// PullRequestKey decides whether a task's landing goes through a GitHub pull request.
	PullRequestKey = "pull_request"
	// LandNoticeKey decides whether dobby posts a line to the orch once a task's run lands or ends without landing.
	// A landed motif run's line goes whatever it reads.
	LandNoticeKey = "land_notice"
	// AutonomyKey decides whether the orch settles every halt of a task's runs.
	// Under it the orch reaches the operator only for a severe halt, a merge-ready beam or a raddle split's approval.
	AutonomyKey = "autonomy"
	// RunWatchKey decides which changes of a task's run dobby mails the orch as a watch line.
	RunWatchKey = "run_watch"
)

// The values of beam.run_watch.
const (
	// RunWatchStops mails a line for each stop of the run.
	RunWatchStops = "stops"
	// RunWatchRows mails a line for every change of the run.
	RunWatchRows = "rows"
	// RunWatchOff mails none.
	RunWatchOff = "off"
)

// The values of beam.pull_request.
const (
	// PullRequestReview lands through a pull request when the origin allows one.
	// Its review ends on the operator's word, given on GitHub or through `lym loom pr-continue`.
	// A PR-Review session sits in the task's window meanwhile.
	PullRequestReview = "review"
	// PullRequestOff lands with no pull request unless GitHub's rules for the parent branch require one.
	PullRequestOff = "off"
	// PullRequestTrail lands through a pull request squash-merged on GitHub at once, with no review,
	// so the parent branch keeps a trail of pull requests to backtrack through.
	PullRequestTrail = "trail"
	// PullRequestMergeIn lands a task that merges its parent's parent branch into its parent with no pull request.
	// It is a true merge that keeps the merged commit as a parent.
	PullRequestMergeIn = "merge-in"
)

// The values of beam.land_notice.
const (
	// LandNoticeOn sends the landing notice.
	LandNoticeOn = "on"
	// LandNoticeOff sends none.
	LandNoticeOff = "off"
)

// PullRequestValues returns every value beam.pull_request takes.
func PullRequestValues() []string {
	return []string{PullRequestReview, PullRequestOff, PullRequestTrail, PullRequestMergeIn}
}

// LandNoticeValues returns every value beam.land_notice takes.
func LandNoticeValues() []string {
	return []string{LandNoticeOn, LandNoticeOff}
}

// RunWatchValues returns every value beam.run_watch takes.
func RunWatchValues() []string {
	return []string{RunWatchStops, RunWatchRows, RunWatchOff}
}
