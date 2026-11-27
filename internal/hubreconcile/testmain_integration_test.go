// testmain_integration_test.go wires this package's test binary into the hermetic git test environment.
// It installs, once for the binary, a detached board sync and an orch notifier.
// Both record each call under its board dir.
// Tests that walk different hubs therefore run in parallel, and each reads only the calls of its own board.

package hubreconcile_test

import (
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// boardSyncs records the detached board syncs every walk of the binary asks for.
var boardSyncs = &boardSyncRecorder{calls: map[string]int{}}

// orchMail records the orch mail every walk of the binary sends, and answers it as the test of its board set.
var orchMail = &mailRecorder{mailed: map[string][]string{}, answers: map[string]mailAnswer{}}

// uninstallOrchMail reinstates the notifier the process held before the recorder, which is none.
var uninstallOrchMail func()

// TestMain runs HermeticGitEnv before any test spawns git.
// It sets FABRIC_SKIP_PUSH so a commit's detached push child never re-executes the test binary.
// It installs the recording board sync and notifier, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Setenv("FABRIC_SKIP_PUSH", "1")
	hubreconcile.SetSpawnBoardSyncForTest(boardSyncs.record)
	uninstallOrchMail = hubreconcile.InstallNotifier(orchMail.send)
	os.Exit(tmuxkit.Main(m))
}

// boardSyncRecorder counts the detached board syncs asked for, per board dir.
// It is safe for concurrent use.
type boardSyncRecorder struct {
	mu    sync.Mutex
	calls map[string]int
}

// record counts one sync of boardPath and passes it to the real detached sync.
func (r *boardSyncRecorder) record(boardPath string) error {
	r.mu.Lock()
	r.calls[boardPath]++
	r.mu.Unlock()
	return boardengine.SpawnSync(boardPath)
}

// count returns how many detached syncs of boardDir were asked for.
func (r *boardSyncRecorder) count(boardDir string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[boardDir]
}

// mailAnswer is how the notifier answers a mail: posted unless unposted is set, failing with err when it is non-nil.
type mailAnswer struct {
	unposted bool
	err      error
}

// mailRecorder records the lines mailed to each board's orch.
// It is safe for concurrent use.
type mailRecorder struct {
	mu      sync.Mutex
	mailed  map[string][]string
	answers map[string]mailAnswer
}

// send records line under boardDir and answers as answer set for boardDir, posted by default.
func (r *mailRecorder) send(boardDir, line string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mailed[boardDir] = append(r.mailed[boardDir], line)
	answer := r.answers[boardDir]
	return !answer.unposted, answer.err
}

// answer makes the notifier answer every later mail to boardDir's orch with answer.
func (r *mailRecorder) answer(boardDir string, answer mailAnswer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.answers[boardDir] = answer
}

// lines returns the lines mailed to boardDir's orch, in the order they were sent.
func (r *mailRecorder) lines(boardDir string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.mailed[boardDir])
}
