// Package beamsetting names the module, keys and values of beam.yaml's per-task settings.
// Those settings are beam.pull_request, beam.land_notice, beam.autonomy, beam.run_watch and beam.window_placement.
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
	// AutonomyKey decides how far the orch settles the halts of a task's runs without the operator.
	AutonomyKey = "autonomy"
	// RunWatchKey decides which changes of a task's run dobby mails the orch as a watch line.
	RunWatchKey = "run_watch"
	// WindowPlacementKey decides which desktop a task's terminal window is placed on.
	WindowPlacementKey = "window_placement"
)

// The values of beam.window_placement.
// All but WindowPlacementInherit are also the modes of the machine's desktop.window_placement.
const (
	// WindowPlacementInherit hands the decision over to the machine's desktop.window_placement.
	WindowPlacementInherit = ""
	// WindowPlacementOff leaves the window where the window manager opens it.
	WindowPlacementOff = "off"
	// WindowPlacementSame places the window from the desktop of its parent orch's window.
	WindowPlacementSame = "same"
	// WindowPlacementNext places the window from the desktop after its parent orch's window.
	WindowPlacementNext = "next"
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

// The values of beam.autonomy.
// false and true are YAML booleans in a layer, and unattended is text.
const (
	// AutonomyOff relays to the operator every halt the orch does not settle.
	AutonomyOff = "false"
	// AutonomyOn has the orch settle every halt.
	// It reaches the operator only for a severe halt, a merge-ready beam or a raddle split's approval.
	AutonomyOn = "true"
	// AutonomyUnattended has the orch settle every halt and never ask the operator; only a severe halt reaches them.
	AutonomyUnattended = "unattended"
)

// AutonomyValues returns every value beam.autonomy takes.
func AutonomyValues() []string {
	return []string{AutonomyOff, AutonomyOn, AutonomyUnattended}
}

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

// WindowPlacementValues returns every value beam.window_placement takes.
func WindowPlacementValues() []string {
	return append([]string{WindowPlacementInherit}, WindowPlacementModes()...)
}

// WindowPlacementModes returns the values that name a way of placing a window.
// They are the ones the machine's desktop.window_placement takes.
func WindowPlacementModes() []string {
	return []string{WindowPlacementOff, WindowPlacementSame, WindowPlacementNext}
}
