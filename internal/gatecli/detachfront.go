// detachfront.go implements the front of a capped `lym gate test` call: the choice of a capped call, the start or join of a detached runner, the registration of the runner, and the answer once the run ended or the caller's cap passed.

package gatecli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// The production values of a front's bounds.
const (
	// defaultClaimBound is how long a front waits for the runner it started to claim its run.
	defaultClaimBound = 30 * time.Second
	// defaultLockMargin is the part of its cap a front keeps back from waiting for the key directory's lock.
	defaultLockMargin = 30 * time.Second
	// defaultGrace is how long a run's lease outlasts the deadline of the latest call that wrote it.
	defaultGrace = 120 * time.Second
)

// detachRunMaxAge is how long a finished run directory is kept; it matches the age at which run records are pruned.
const detachRunMaxAge = 14 * 24 * time.Hour

// frontPollPeriod is how often a front tries the key directory's lock and looks for the runner's claim; it is the one-second floor of the wake rule.
const frontPollPeriod = time.Second

// The files a front keeps beside a run's other files.
const (
	// keyLockFile is the key directory's lock, held while a front chooses, starts or joins the key's run.
	keyLockFile = "lock"
	// currentFile names the key's current run id.
	currentFile = "current.json"
	// treeFile holds the treeRecord the run was started over.
	treeFile = "tree.json"
	// deliveredFile exists once a front printed the run's result.
	deliveredFile = "delivered"
	// runnerLogFile receives the runner process's own output.
	runnerLogFile = "runner.log"
)

// gateVerbName returns the command a request of verb names in a message: `gate test` or `gate card-verify`.
func gateVerbName(verb string) string {
	return "gate " + verb
}

// replayedBusyLine returns the line written to stderr when the exit recorded by a run of verb is the code that otherwise means a run still going.
func replayedBusyLine(verb string) string {
	return gateVerbName(verb) + ": the run exited 75, a test's own code here and no busy hub, so this call exits 1"
}

// detachSeams are the bounds and the launcher of a capped call's front, which a test replaces so it never waits out a production duration or re-executes its own binary.
type detachSeams struct {
	// keepWarm resolves the keep-warm cap of the strand named by its second argument.
	keepWarm func(configengine.Dirs, string) shuttleengines.KeepWarmAnswer
	// launch starts the detached runner over a run directory and returns its pid.
	launch func(runnerSpawn) (int, error)
	// claimBound is how long a front waits for the runner to claim its run.
	claimBound time.Duration
	// lockMargin is the part of its cap a front keeps back from waiting for the key directory's lock.
	lockMargin time.Duration
	// grace is how long a run's lease outlasts the deadline of the latest call that wrote it.
	grace time.Duration
}

// defaultDetachSeams returns the seams of a production call.
func defaultDetachSeams() detachSeams {
	return detachSeams{
		keepWarm:   shuttleengines.KeepWarm,
		launch:     launchRunner,
		claimBound: defaultClaimBound,
		lockMargin: defaultLockMargin,
		grace:      defaultGrace,
	}
}

// runnerSpawn is what a launcher needs to start a runner.
type runnerSpawn struct {
	// runDir is the run directory the runner runs.
	runDir string
}

// launchRunner starts `<this binary> gate run-detached <run dir>` detached in the run directory, its output in the run's runner log and the strand name dropped from its environment, and returns its pid.
// It refuses under a test binary, since a test never re-executes its own binary.
func launchRunner(spawn runnerSpawn) (int, error) {
	if testing.Testing() {
		return 0, errors.New("gate: the detached runner is not started under go test")
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve this binary: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(spawn.runDir, runnerLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open the runner log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "gate", "run-detached", spawn.runDir)
	cmd.Dir = spawn.runDir
	cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") })
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start %s: %w", exe, err)
	}
	pid := cmd.Process.Pid
	logger.Info("gate: started the detached runner", "exe", exe, "run", spawn.runDir, "pid", pid)
	// The runner outlives its front, so the wait only reaps it while the front lives.
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// frontCall is a capped call after the choice: where it runs, what it asks and how long it waits.
type frontCall struct {
	// verb is the gate verb the runner runs.
	verb string
	// words are the call's words after the verb.
	words []string
	// cwd is the absolute directory the call resolves against.
	cwd string
	// dir is the directory the call runs in: cwd, or the -C directory.
	dir string
	// location is the worktree dir belongs to.
	location *lymcwd.Location
	// inHub is whether location is in a hub.
	inHub bool
	// pool is the hub's slot pool; nil outside a hub.
	pool *gateslot.Pool
	// cap is how long the call waits for its run before it answers that the run goes on.
	cap time.Duration
	// gateKey is the --gate value of the call, empty without one.
	gateKey string
	// longCap is the cap a call waits when the key's runs outlast its break-even.
	longCap time.Duration
	// maxReruns is the number of short waits one long wait is worth; 0 takes no long wait.
	maxReruns int
	// recordsDir is the directory the call's run records are in.
	recordsDir string
}

// treeRecord is the tree identity a run was started over: HEAD's tree and the sorted paths `git status` reports.
type treeRecord struct {
	Tree  string   `json:"tree"`
	Dirty []string `json:"dirty"`
}

// currentRecord is the content of a key directory's current file.
type currentRecord struct {
	RunID string `json:"run_id"`
}

// joinState is what a front reads of a key's current run to decide whether it joins it.
type joinState struct {
	// lease is the run's lease deadline, zero when it has none.
	lease time.Time
	// delivered is whether a front already printed the run's result.
	delivered bool
	// current is the tree identity the front sees now.
	current treeRecord
	// recorded is the tree identity the run was started over.
	recorded treeRecord
	// exit is the run's result, read only when hasExit.
	exit    detachExit
	hasExit bool
	// process is the runner's claim, read only when hasProcess.
	process    processRecord
	hasProcess bool
}

// String names the phase as its state followed by the tier and the line it is for, such as `running tier0`.
func (p runPhase) String() string {
	words := slices.DeleteFunc([]string{p.State, p.Tier, p.Line}, func(word string) bool { return word == "" })
	return strings.Join(words, " ")
}

// cappedCall decides whether request is a capped call and, when it is, returns the call with its verb, words and directory filled.
// A call whose flags are malformed is never capped, so the in-process run refuses it.
func cappedCall(ctx context.Context, request testRequest, wait bool, seams detachSeams) (frontCall, bool) {
	if flagShapeRefusal(request) != "" {
		return frontCall{}, false
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return frontCall{}, false
	}
	dir, err := absoluteDir(cwd, request.dir)
	if err != nil {
		return frontCall{}, false
	}
	call, capped := frontDecision(dir, wait, seams)
	if !capped {
		return frontCall{}, false
	}
	call.verb, call.words, call.cwd, call.gateKey = verbTest, request.words(), cwd, request.gate
	return call, true
}

// frontDecision decides whether a call running in dir is capped: it runs in a detached runner it starts or joins, and answers when the caller's keep-warm cap passes.
// A call is not capped when it gives wait, when no strand name is set, when dir is in no worktree, when it runs inside a slot an enclosing gate run holds, and when the resolver's cap is zero.
// A resolver fallback is logged and still caps.
func frontDecision(dir string, wait bool, seams detachSeams) (call frontCall, capped bool) {
	strand := os.Getenv(agentname.StrandNameEnv)
	if wait || strand == "" {
		return frontCall{}, false
	}
	location, inHub, err := resolveHub(dir)
	if err != nil {
		return frontCall{}, false
	}
	if !inHub {
		if location, err = lymcwd.ResolveWorktree(dir); err != nil {
			return frontCall{}, false
		}
	}
	var pool *gateslot.Pool
	if inHub {
		pool = hubgeom.GateSlots(location)
		if inherited := os.Getenv(gateslot.InheritEnv); inherited != "" && pool.Inherited(inherited) {
			return frontCall{}, false
		}
	}
	answer := seams.keepWarm(fabricengine.ConfigDirs(location), strand)
	if answer.Fallback != nil {
		logger.Warn("gate test: the keep-warm resolver fell back, so the call waits the default cap", "strand", strand, "cause", answer.Fallback)
	}
	if answer.Cap <= 0 {
		return frontCall{}, false
	}
	return frontCall{
		dir: dir, location: location, inHub: inHub, pool: pool,
		cap: answer.Cap, longCap: answer.LongCap, maxReruns: answer.MaxReruns, recordsDir: runLogDirOf(location, inHub),
	}, true
}

// words returns the call's words after the verb in a fixed order that the runner parses to the same call: flags as `--name=value`, the targets, and `--` with the arguments after it.
func (r testRequest) words() []string {
	var words []string
	add := func(flag, value string) { words = append(words, "--"+flag+"="+value) }
	if r.dir != "" {
		add("dir", r.dir)
	}
	if r.gate != "" {
		add("gate", r.gate)
	}
	for _, tier := range r.tiers {
		add("tier", strconv.Itoa(tier))
	}
	if r.uptoGiven {
		add("upto", strconv.Itoa(r.upto))
	}
	for _, card := range r.cards {
		add("card", card)
	}
	if r.base != "" {
		add("base", r.base)
	}
	if r.all {
		words = append(words, "--all")
	}
	if r.guards {
		words = append(words, "--guards")
	}
	words = append(words, r.targets...)
	if len(r.args) > 0 {
		words = append(words, "--")
		words = append(words, r.args...)
	}
	return words
}

// detachKeyDir returns the key directory of a call: a hash of the verb, the run directory and the words, under the detach directory of anchor's gate directory.
// Calls with the same key share one current run.
func detachKeyDir(anchor, verb, runDir string, words []string) string {
	hash := sha256.New()
	for _, part := range append([]string{verb, runDir}, words...) {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return filepath.Join(gateslot.WaitDir(anchor), detachDirName, hex.EncodeToString(hash.Sum(nil))[:16])
}

// joinable reports whether a front joins the run state describes.
// It joins only a run whose lease holds at now, whose result no front printed, and that started over the tree identity the front sees now;
// the run must have ended without being abandoned, or its runner's pid must be live with the start reading the runner recorded, so a reused pid is no runner.
func joinable(run joinState, now time.Time, live func(int) bool, reading func(int) (string, bool)) bool {
	switch {
	case !run.lease.After(now), run.delivered:
		return false
	case run.current.Tree == "", run.current.Tree != run.recorded.Tree, !slices.Equal(run.current.Dirty, run.recorded.Dirty):
		return false
	case run.hasExit:
		return !run.exit.Abandoned
	}
	return runnerLive(run.process, run.hasProcess, live, reading)
}

// runnerLive reports whether the runner a claimed process file names is live with the start reading it recorded, so a reused pid is no runner.
func runnerLive(process processRecord, claimed bool, live func(int) bool, reading func(int) (string, bool)) bool {
	if !claimed || !live(process.PID) {
		return false
	}
	start, ok := reading(process.PID)
	return ok && start == process.Start
}

// processReading is the start reading of pid that a run's process file records.
func processReading(pid int) (string, bool) {
	return startReading(runtime.GOOS, pid, proc.StartTime, proc.CreationTime)
}

// front is one capped call's wait on the run of its key.
type front struct {
	call   frontCall
	seams  detachSeams
	out    io.Writer
	stderr io.Writer
	keyDir string
	start  time.Time
	// deadline is when the call answers that the run goes on.
	deadline time.Time
}

// runFront starts or joins the run of a capped call and answers.
// It prints the run's result once the run ended before the call's cap, and answers ok with done false and SlotBusyExit when the cap passed first.
// A terminating signal ends the front alone, with 128 plus the signal number, and the runner goes on.
func runFront(ctx context.Context, out, stderr io.Writer, call frontCall, seams detachSeams) int {
	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()

	start := time.Now()
	f := &front{
		call: call, seams: seams, out: out, stderr: stderr,
		keyDir:   detachKeyDir(call.location.AnchorPath(), call.verb, call.dir, call.words),
		start:    start,
		deadline: start.Add(chooseCap(call, gateslot.ReadRunRecords)),
	}
	return exitAfterSignal(caught, f.run(runCtx))
}

// chooseCap returns how long the call waits for its run.
// It is the call's long cap when the call names a gate key, the resolver gave a break-even and this machine's earlier runs of the key outlast that break-even, and its cap otherwise;
// a read error keeps the cap, and so do no key and no break-even, without a read.
// Bound: an estimate from a few earlier calls can mis-pick, which costs cache money only, never a verdict.
func chooseCap(call frontCall, readRecords func(dir string) ([]gateslot.RunRecord, error)) time.Duration {
	if call.gateKey == "" || call.maxReruns <= 0 {
		return call.cap
	}
	records, err := readRecords(call.recordsDir)
	if err != nil {
		logger.Warn("gate test: could not read the run records, so the call waits its short cap", "dir", call.recordsDir, "cause", err)
		return call.cap
	}
	if longWait(records, call.gateKey, call.cap, call.maxReruns) {
		return call.longCap
	}
	return call.cap
}

// longWaitCalls is how many of a key's latest full calls longWait weighs.
const longWaitCalls = 5

// longWaitMinCalls is how many full calls longWait needs to answer true.
const longWaitMinCalls = 3

// longWait reports whether the runs of the gate key outlast the break-even of capWait waited maxReruns times, so one long wait costs less than as many short ones.
// It weighs the records of the key by call, leaving out records without a call id, and a call only when it ran every tier it listed to an outcome:
// each listed tier has a non-build record, and no record of the call is carried, abandoned or of an empty scope, a non-build, non-carried record with neither a command nor a blocking build.
// A tier a failed build blocked carries the build as its blocker, so a call whose build failed counts.
// A call's wall time runs from its earliest start, a record's wait start or its start where there is no wait start, to its latest end.
// It takes the latest five full calls by that start and answers true when there are at least three and their median wall time exceeds capWait times maxReruns.
func longWait(records []gateslot.RunRecord, key string, capWait time.Duration, maxReruns int) bool {
	byCall := map[string][]gateslot.RunRecord{}
	for _, record := range records {
		if record.GateKey == key && record.CallID != "" {
			byCall[record.CallID] = append(byCall[record.CallID], record)
		}
	}
	type fullCall struct {
		start time.Time
		wall  time.Duration
	}
	var calls []fullCall
	for _, call := range byCall {
		if !ranEveryListedTier(call) {
			continue
		}
		start, end := callSpan(call)
		calls = append(calls, fullCall{start: start, wall: end.Sub(start)})
	}
	if len(calls) < longWaitMinCalls {
		return false
	}
	slices.SortFunc(calls, func(a, b fullCall) int { return a.start.Compare(b.start) })
	calls = calls[max(0, len(calls)-longWaitCalls):]
	walls := make([]time.Duration, len(calls))
	for i, call := range calls {
		walls[i] = call.wall
	}
	slices.Sort(walls)
	median := walls[len(walls)/2]
	if len(walls)%2 == 0 {
		median = (walls[len(walls)/2-1] + walls[len(walls)/2]) / 2
	}
	return median > capWait*time.Duration(maxReruns)
}

// ranEveryListedTier reports whether the records of one call show every tier its tier list names ran to an outcome, as longWait counts a full call.
func ranEveryListedTier(call []gateslot.RunRecord) bool {
	ran := map[int]bool{}
	for _, record := range call {
		emptyScope := !record.Build && !record.Carried && record.Command == "" && record.BlockedBy == ""
		if record.Carried || record.Abandoned || emptyScope {
			return false
		}
		if !record.Build {
			ran[record.Tier] = true
		}
	}
	// Every record of one call carries the call's tier list.
	return !slices.ContainsFunc(call[0].TierList, func(tier int) bool { return !ran[tier] })
}

// callSpan returns the earliest start and the latest end of the records of one call.
func callSpan(call []gateslot.RunRecord) (start, end time.Time) {
	for i, record := range call {
		recordStart := record.WaitStart
		if recordStart.IsZero() {
			recordStart = record.Start
		}
		if i == 0 || recordStart.Before(start) {
			start = recordStart
		}
		if record.End.After(end) {
			end = record.End
		}
	}
	return start, end
}

// run takes the key's lock, starts or joins its run and waits for the run's result.
func (f *front) run(ctx context.Context) int {
	if err := os.MkdirAll(f.keyDir, 0o755); err != nil {
		return output.Err(f.out, fmt.Sprintf("%s: cannot create the run directory %s: %v; way forward: run the same command again, or run it with --wait, which runs the tiers in this process", gateVerbName(f.call.verb), f.keyDir, err))
	}
	run, code, done := f.startOrJoin(ctx)
	if done {
		return code
	}

	exitPath := run.path(exitFile)
	_, ended, err := shuttleengine.AwaitAny(ctx, []string{exitPath}, time.Until(f.deadline))
	if ctx.Err() != nil {
		return 1
	}
	if err != nil {
		return output.Err(f.out, fmt.Sprintf("%s: cannot wait for the run: %v; way forward: run the same command again", gateVerbName(f.call.verb), err))
	}
	if !ended {
		return f.answerGoing(run)
	}
	exit, err := run.readExit()
	if err != nil {
		return output.Err(f.out, fmt.Sprintf("%s: cannot read the run's result: %v; way forward: run the same command again", gateVerbName(f.call.verb), err))
	}
	if exit.Abandoned {
		return f.answerGoing(run)
	}
	return f.replay(run, exit)
}

// answerGoing answers that run goes on, with the phase it reports.
func (f *front) answerGoing(run detachRun) int {
	phase, found := readRunFile[runPhase](run.path(phaseFile))
	if !found {
		phase = runPhase{State: phaseStarting}
	}
	return answerRunGoing(f.out, f.call.verb, f.call.pool, phase, time.Since(f.start))
}

// replay prints the result of the ended run, marks it delivered and returns the call's exit code.
// A run whose output could not be copied in full is not marked delivered, so the next call joins it again.
// A recorded SlotBusyExit is a test's own code and exits 1, since the caller reads SlotBusyExit as a run still going.
func (f *front) replay(run detachRun, exit detachExit) int {
	copyErr := errors.Join(copyRunFile(f.out, run.path(stdoutFile)), copyRunFile(f.stderr, run.path(stderrFile)))
	if copyErr != nil {
		logger.Warn("gate test: could not copy the run's output, so the run stays undelivered", "dir", run.dir, "cause", copyErr)
	} else if err := os.WriteFile(run.path(deliveredFile), nil, 0o644); err != nil {
		logger.Warn("gate test: could not mark the run delivered", "dir", run.dir, "cause", err)
	}
	if exit.Code == SlotBusyExit {
		printLine(f.stderr, replayedBusyLine(f.call.verb))
		return 1
	}
	return exit.Code
}

// copyRunFile copies the run file at path to w and returns the error of a failed open or copy; a file that does not exist is no error.
func copyRunFile(w io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	_, err = io.Copy(w, file)
	return err
}

// startOrJoin returns the run the front waits on, under the key directory's lock: the key's current run when it is joinable, else a fresh one.
// done is set when the front ends here, with code.
func (f *front) startOrJoin(ctx context.Context) (run detachRun, code int, done bool) {
	held, code, done := f.lockKey(ctx)
	if done {
		return detachRun{}, code, true
	}
	defer func() {
		if err := held.Release(); err != nil {
			logger.Warn("gate test: could not release the run directory's lock", "dir", f.keyDir, "cause", err)
		}
	}()

	identity := f.treeIdentity()
	current, found := f.currentRun()
	if found {
		if joinable(f.readJoinState(current, identity), time.Now(), proc.IsAlive, processReading) {
			if err := current.writeLease(f.deadline.Add(f.seams.grace)); err != nil {
				logger.Warn("gate test: could not renew the joined run's lease", "dir", current.dir, "cause", err)
			}
			return current, 0, false
		}
		endStrayLine(current)
		// A runner of the superseded run ends at its next lease read.
		if err := current.expireLease(time.Now()); err != nil {
			logger.Warn("gate test: could not expire the superseded run's lease", "dir", current.dir, "cause", err)
		}
	}
	return f.startRun(ctx, identity)
}

// endStrayLine kills the process group a superseded card-verify run recorded in its line file when the run's runner is gone.
// A dead runner's line shell dies with it through the parent-death signal, but the shell's descendants, a `--wait` gate child among them, do not.
func endStrayLine(run detachRun) {
	request, err := run.readRequest()
	if err != nil || request.Verb != verbCardVerify {
		return
	}
	process, claimed := run.readProcessRecord()
	if runnerLive(process, claimed, proc.IsAlive, processReading) {
		return
	}
	tree, found := readRunFile[proc.TreeRecord](run.path(lineFile))
	if !found {
		return
	}
	killed, err := proc.KillTree(tree)
	if err != nil {
		logger.Warn("gate card-verify: could not end the dead runner's line", "dir", run.dir, "pgid", tree.PGID, "cause", err)
		return
	}
	if killed {
		logger.Info("gate card-verify: ended the dead runner's line", "dir", run.dir, "pgid", tree.PGID)
	}
}

// lockKey takes the key directory's lock, trying once a second while at least the lock margin of the call's cap remains.
// done is set when the front ends instead, with code: it answered that the run goes on in the phase `starting`, or the lock could not be taken.
func (f *front) lockKey(ctx context.Context) (file *lock.FileLock, code int, done bool) {
	lockPath := filepath.Join(f.keyDir, keyLockFile)
	for {
		held, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, output.Err(f.out, fmt.Sprintf("%s: cannot take the lock of the run directory %s: %v; way forward: run the same command again, or run it with --wait, which runs the tiers in this process", gateVerbName(f.call.verb), f.keyDir, err)), true
		}
		if acquired {
			return held, 0, false
		}
		if time.Until(f.deadline) < f.seams.lockMargin {
			return nil, answerRunGoing(f.out, f.call.verb, f.call.pool, runPhase{State: phaseStarting}, time.Since(f.start)), true
		}
		select {
		case <-ctx.Done():
			return nil, 1, true
		case <-time.After(frontPollPeriod):
		}
	}
}

// treeIdentity reads the identity of the tree the call runs over.
// A tree that cannot be read answers the zero identity, which no run is joined over.
func (f *front) treeIdentity() treeRecord {
	root := f.call.location.WorktreePath()
	tree, _, err := headIdentity(root)
	if err != nil {
		logger.Warn("gate test: could not read HEAD's tree, so the call joins no run", "root", root, "cause", err)
		return treeRecord{}
	}
	dirty, err := verifytree.DirtyPaths(root)
	if err != nil {
		logger.Warn("gate test: could not read the dirty paths, so the call joins no run", "root", root, "cause", err)
		return treeRecord{}
	}
	slices.Sort(dirty)
	return treeRecord{Tree: tree, Dirty: dirty}
}

// currentRun returns the run the key directory names as current, and whether there is one.
func (f *front) currentRun() (detachRun, bool) {
	path := filepath.Join(f.keyDir, currentFile)
	record, found, err := state.ReadJSON[currentRecord](path, path+".lock")
	if err != nil {
		logger.Warn("gate test: could not read the current run", "dir", f.keyDir, "cause", err)
		return detachRun{}, false
	}
	if !found || record.RunID == "" {
		return detachRun{}, false
	}
	return detachRun{dir: filepath.Join(f.keyDir, record.RunID)}, true
}

// readJoinState reads the files of run that joinable judges.
func (f *front) readJoinState(run detachRun, identity treeRecord) joinState {
	leasePath := run.path(leaseFile)
	lease, _, err := state.ReadJSON[leaseRecord](leasePath, leasePath+".lock")
	if err != nil {
		lease = leaseRecord{}
	}
	js := joinState{lease: lease.Deadline, current: identity}
	_, err = os.Stat(run.path(deliveredFile))
	js.delivered = err == nil
	if recorded, found := readRunFile[treeRecord](run.path(treeFile)); found {
		js.recorded = recorded
	}
	js.exit, js.hasExit = readRunFile[detachExit](run.path(exitFile))
	js.process, js.hasProcess = run.readProcessRecord()
	return js
}

// readRunFile decodes the JSON file at path, and reports false for a file that is absent or does not decode.
func readRunFile[T any](path string) (value T, found bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return value, false
	}
	if err := json.Unmarshal(data, &value); err != nil {
		var zero T
		return zero, false
	}
	return value, true
}

// readExit reads the run's result.
func (d detachRun) readExit() (detachExit, error) {
	data, err := os.ReadFile(d.path(exitFile))
	if err != nil {
		return detachExit{}, err
	}
	var exit detachExit
	return exit, json.Unmarshal(data, &exit)
}

// startRun makes a new run over identity, launches its runner and waits until the runner claimed it.
// done is set when the front ends here, with code.
func (f *front) startRun(ctx context.Context, identity treeRecord) (run detachRun, code int, done bool) {
	run, err := f.writeNewRun(identity)
	if err != nil {
		return detachRun{}, output.Err(f.out, fmt.Sprintf("%s: cannot prepare a run under %s: %v; way forward: run the same command again, or run it with --wait, which runs the tiers in this process", gateVerbName(f.call.verb), f.keyDir, err)), true
	}
	pid, err := f.seams.launch(runnerSpawn{runDir: run.dir})
	if err != nil {
		logger.Warn("gate test: could not start the detached runner", "run", run.dir, "cause", err)
		if claimErr := claimByFront(run); claimErr != nil {
			logger.Warn("gate test: could not claim the run that never started", "run", run.dir, "cause", claimErr)
		}
		return detachRun{}, f.failStart(run, err.Error()), true
	}
	f.pruneOldRuns(run)
	f.registerRunner(pid, run)
	if !f.awaitClaim(ctx, run, pid) {
		return detachRun{}, f.failStart(run, runnerLogText(run)), true
	}
	return run, 0, false
}

// pruneOldRuns removes the key directory's run directories other than keep that are older than detachRunMaxAge and whose runner is not live.
// A run that cannot be removed is logged and left.
func (f *front) pruneOldRuns(keep detachRun) {
	entries, err := os.ReadDir(f.keyDir)
	if err != nil {
		logger.Warn("gate test: could not list the key directory for old runs", "dir", f.keyDir, "cause", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == filepath.Base(keep.dir) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < detachRunMaxAge {
			continue
		}
		old := detachRun{dir: filepath.Join(f.keyDir, entry.Name())}
		process, claimed := readRunFile[processRecord](old.path(processFile))
		if runnerLive(process, claimed, proc.IsAlive, processReading) {
			continue
		}
		if err := os.RemoveAll(old.dir); err != nil {
			logger.Warn("gate test: could not remove an old run directory", "dir", old.dir, "cause", err)
		}
	}
}

// claimByFront claims run for the calling front, which no runner can claim after.
func claimByFront(run detachRun) error {
	start, _ := startReading(runtime.GOOS, os.Getpid(), proc.StartTime, proc.CreationTime)
	return run.claim(os.Getpid(), start)
}

// writeNewRun creates a run directory under the key directory, writes its request, tree identity and lease, and points the key directory at it.
func (f *front) writeNewRun(identity treeRecord) (detachRun, error) {
	run := detachRun{dir: filepath.Join(f.keyDir, fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()))}
	if err := os.Mkdir(run.dir, 0o755); err != nil {
		return detachRun{}, err
	}
	request, err := json.Marshal(detachRequest{Verb: f.call.verb, Args: f.call.words, Cwd: f.call.cwd})
	if err != nil {
		return detachRun{}, err
	}
	recorded, err := json.Marshal(identity)
	if err != nil {
		return detachRun{}, err
	}
	if err := fsx.AtomicWriteBytes(run.path(requestFile), request); err != nil {
		return detachRun{}, err
	}
	if err := fsx.AtomicWriteBytes(run.path(treeFile), recorded); err != nil {
		return detachRun{}, err
	}
	if err := run.writeLease(f.deadline.Add(f.seams.grace)); err != nil {
		return detachRun{}, err
	}
	currentPath := filepath.Join(f.keyDir, currentFile)
	return run, state.WriteJSON(currentPath, currentPath+".lock", currentRecord{RunID: filepath.Base(run.dir)})
}

// registerRunner records the runner in the run-process owner record of the call's pair, so the pair's teardown ends it.
// The prime and a worktree outside every hub have no pair, and register nothing.
func (f *front) registerRunner(pid int, run detachRun) {
	if !f.call.inHub {
		return
	}
	slug := originSlug(f.call.location)
	if slug == "" {
		return
	}
	argv := []string{os.Args[0], "gate", "run-detached", run.dir}
	if err := runproc.Register(fabricengine.RunProcDir(f.call.location.HubPath), slug, runproc.ChildRecord(pid, runproc.RoleGate, argv)); err != nil {
		logger.Warn("gate test: could not register the detached runner", "pid", pid, "slug", slug, "cause", err)
	}
}

// awaitClaim waits, checking once a second for at most the claim bound, until the runner claimed run, and reports whether it did.
// A runner that ended unclaimed, or none that claimed by the bound, is a failed start: the front claims the run itself so no runner can, and reports false.
// A cancelled ctx reports true, since the front is ending and the run stands as it is.
func (f *front) awaitClaim(ctx context.Context, run detachRun, pid int) bool {
	limit := time.Now().Add(f.seams.claimBound)
	for {
		select {
		case <-ctx.Done():
			return true
		case <-time.After(frontPollPeriod):
		}
		if _, err := os.Stat(run.path(processFile)); err == nil {
			return true
		}
		if !proc.IsAlive(pid) || !time.Now().Before(limit) {
			break
		}
	}
	return errors.Is(claimByFront(run), os.ErrExist)
}

// failStart ends the front for a runner that never started, whose run the front claimed: it expires the lease so no call joins the run, prints detail to stderr and answers an error.
func (f *front) failStart(run detachRun, detail string) int {
	if err := run.expireLease(time.Now()); err != nil {
		logger.Warn("gate test: could not expire the lease of the run that never started", "dir", run.dir, "cause", err)
	}
	printLine(f.stderr, strings.TrimSpace(detail))
	return output.Err(f.out, gateVerbName(f.call.verb)+": the detached runner did not start; way forward: run the same command again, and when it fails again run it with --wait, which runs the tiers in this process and needs no runner")
}

// runnerLogText returns the text of the run's runner log, empty when it cannot be read.
func runnerLogText(run detachRun) string {
	data, err := os.ReadFile(run.path(runnerLogFile))
	if err != nil {
		return ""
	}
	return string(data)
}

// answerRunGoing answers ok with done false and the phase the run is in, and returns SlotBusyExit.
// A run waiting for its slots also answers slot_free false and names each holder's worktree, site and tier, read from pool.
// The message tells the caller to run the same command again.
func answerRunGoing(out io.Writer, verb string, pool *gateslot.Pool, phase runPhase, waited time.Duration) int {
	fields := map[string]any{"done": false, "phase": phase.String()}
	message := fmt.Sprintf("%s: the run is still going after %s (%s)", gateVerbName(verb), waited, phase)
	if phase.State == phaseSlots && pool != nil {
		holders, err := pool.Holders()
		if err != nil {
			logger.Warn("gate test: could not read the gate holders", "cause", err)
		}
		named := make([]map[string]string, 0, len(holders))
		described := make([]string, 0, len(holders))
		for _, holder := range holders {
			entry := map[string]string{"worktree": holder.Worktree, "site": holder.Site}
			if holder.Tier != "" {
				entry["tier"] = holder.Tier
			}
			named = append(named, entry)
			described = append(described, fmt.Sprintf("%s (%s)", holder.Worktree, holder.Site))
		}
		fields["slot_free"] = false
		fields["holders"] = named
		message = fmt.Sprintf("%s: no gate slot is free yet after %s; held by %s", gateVerbName(verb), waited, strings.Join(described, ", "))
	}
	fields["message"] = message + "; run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls"
	output.Ok(out, fields)
	return SlotBusyExit
}
