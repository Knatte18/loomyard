// export_test.go exposes the pull request close of a Teardown to package pairteardown_test.

package pairteardown

// ClosePairPullRequestForTest runs td's pull request close for slug with its task worktree read as gone, which warns naming the task branch read from what is left of the pair.
func ClosePairPullRequestForTest(td *Teardown, slug string) {
	td.gone = func(string) (bool, error) { return true, nil }
	td.closePairPullRequest(slug)
}
