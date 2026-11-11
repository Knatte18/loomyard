// wiring.go fills a slackCLI's seams with their production values over a real hub.
// The seams cover the hub's geometry, the creel mailbox, and the beam and pause readers.
// They also cover the relay's dependencies and the hidden read of the token on a terminal.

package slackcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// relayReexecLockWait is how long a relay that re-executed onto a replaced binary waits for the lock its old image released.
const relayReexecLockWait = 10 * time.Second

// rootProjectHandle is the handle of the board's root project, which no board project takes.
const rootProjectHandle = "P0"

// productionCLI returns the slackCLI whose seams PersistentPreRunE fills over a real hub.
func productionCLI() *slackCLI {
	return &slackCLI{
		slackBaseURL:    slackengine.DefaultBaseURL,
		clock:           slackengine.SystemClock{},
		resolveLocation: lymcwd.Resolve,
		wire:            wireProduction,
		readHiddenLine:  readHiddenTerminalLine,
	}
}

// readHiddenTerminalLine prints prompt to standard error and reads one line with echo off when in is a terminal.
// Any other input is left unread and reported as no terminal.
// An interrupt or termination signal during the read restores the terminal's echo and ends the read with an error,
// so the operator's shell is never left with echo off.
func readHiddenTerminalLine(in io.Reader, prompt string) (string, bool, error) {
	file, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", false, nil
	}
	fd := int(file.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return "", true, err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	type readResult struct {
		line []byte
		err  error
	}
	// Buffered, so the read's goroutine never blocks once an interrupt has ended the wait for it.
	read := make(chan readResult, 1)
	fmt.Fprint(os.Stderr, prompt)
	go func() {
		line, err := term.ReadPassword(fd)
		read <- readResult{line, err}
	}()
	// Echo off swallows the operator's Enter, so the newline is printed here to end the prompt's line.
	defer fmt.Fprintln(os.Stderr)
	select {
	case result := <-read:
		return string(result.line), true, result.err
	case <-signals:
		_ = term.Restore(fd, state)
		return "", true, errors.New("interrupted")
	}
}

// wireProduction fills the seams the verb about to run reads.
// A nil location is a worktree-free verb's, which needs only the machine's paths and the relay's three seams:
// each relay seam is told the hub and the prime by its caller.
func wireProduction(c *slackCLI, location *lymcwd.Location) error {
	c.machine = hubgeom.SlackMachine
	if location == nil {
		c.spawnIntermediate = spawnIntermediate
		c.awaitReparent = func(intermediate int) error { return awaitReparent(c.clock, intermediate) }
		c.runRelay = runRelay
		return nil
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return err
	}
	geom, err := hubgeom.SlackGeometry(location.HubPath)
	if err != nil {
		return err
	}
	prime, err := lymcwd.NamedWorktree(location.HubPath, primeName)
	if err != nil {
		return err
	}
	c.geom = geom
	c.worktreeOrch = func() (string, error) { return worktreeOrch(location, primeName, prime, geom.Shortname) }
	c.pane = func() paneCaller { return hubPaneCaller(location.HubPath) }
	c.newSlack = func(baseURL, token string) slackChecks { return slackengine.NewAPI(baseURL, token, &http.Client{}) }
	c.newDialer = func(baseURL, appToken string) slackengine.Dialer {
		return slackengine.NewDialer(baseURL, appToken, &http.Client{}, slackengine.HelloTimeout)
	}
	c.liveHubs = liveHubGeometries
	c.strandName = func() string { return os.Getenv(agentname.StrandNameEnv) }
	c.newStrandClient = func(strand string) (strandClient, error) {
		client, err := creelcli.NewStrandClient(location, strand)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	c.showToggle = reedengine.ShowToggleOutcome
	c.ensure = func(geom slackengine.Geometry) (slackengine.StartOutcome, error) {
		return slackengine.EnsureRelay(geom, primeName)
	}
	c.stop = slackengine.StopRelay
	c.awaitFirstRead = slackengine.AwaitFirstRead
	c.paused = func() (bool, error) { return orchPaused(prime) }
	return nil
}

// liveHubGeometries lists the Slack geometry of each hub the machine record of hubs lists as live.
// A failed read of the record keeps its own message in the chain, and a hub whose geometry fails is named by its path.
func liveHubGeometries() ([]slackengine.Geometry, error) {
	dir, err := standalonestate.UserStateDir()
	if err != nil {
		return nil, fmt.Errorf("slack: find the directory of the machine record of hubs: %w", err)
	}
	entries, err := hubregistry.Live(dir)
	if err != nil {
		return nil, fmt.Errorf("slack: read the machine record of hubs %s: %w", filepath.Join(dir, hubregistry.RecordFileName), err)
	}
	geometries := make([]slackengine.Geometry, 0, len(entries))
	for _, entry := range entries {
		geom, err := hubgeom.SlackGeometry(entry.HubPath)
		if err != nil {
			return nil, fmt.Errorf("slack: build the Slack geometry of the hub at %s: %w", entry.HubPath, err)
		}
		geometries = append(geometries, geom)
	}
	return geometries, nil
}

// hubPaneCaller describes this process for the pane check, against the strands of the hub at hubPath.
func hubPaneCaller(hubPath string) paneCaller {
	strands, err := creelcli.HubStrands(hubPath)
	return paneCaller{
		PID:        os.Getpid(),
		Strands:    strands,
		StrandsErr: err,
		ParentPID:  proc.ParentPID,
		GOOS:       runtime.GOOS,
		StrandName: os.Getenv(agentname.StrandNameEnv),
	}
}

// worktreeOrch returns the creel name of the orch that runs in the worktree at location and has a slack button.
// That is the hub's own orch in the prime, named primeName, and a beam's orch in a beam's worktree, named by its slug.
// Any other worktree, a pair's or a task's, gives the empty string.
func worktreeOrch(location *lymcwd.Location, primeName string, prime *lymcwd.Location, shortname string) (string, error) {
	if location.WorktreeName == primeName {
		return agentname.Format(shortname, "", agentname.RoleOrch)
	}
	beams, err := beamSlugs(prime)
	if err != nil {
		return "", err
	}
	if !slices.Contains(beams, location.WorktreeName) {
		return "", nil
	}
	return agentname.Format(shortname, location.WorktreeName, agentname.RoleOrch)
}

// orchProject names the board project the orch of the beam slug runs: its handle and its slug.
// They are read from the board of the hub at hubPath.
// An empty slug is the hub's own orch, which runs the root project.
// A beam the board holds no project entry for gives an empty handle and the slug itself.
func orchProject(hubPath, slug string) (string, string, error) {
	if slug == "" {
		return rootProjectHandle, boardengine.RootSubject, nil
	}
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(hubPath))
	if err != nil {
		return "", "", fmt.Errorf("slack: read the board for the project of the beam %s: %w", slug, err)
	}
	for _, task := range tasks {
		if task.Slug == slug && task.Kind == boardengine.KindProject {
			return task.Handle, task.Slug, nil
		}
	}
	return "", slug, nil
}

// beamSlugs lists the slugs of the local beams of the hub whose prime is prime.
func beamSlugs(prime *lymcwd.Location) ([]string, error) {
	pairs, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		slugs = append(slugs, pair.Slug)
	}
	return slugs, nil
}

// orchPaused reports whether a pause covers the orch of the worktree at location.
func orchPaused(location *lymcwd.Location) (bool, error) {
	pause, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return false, err
	}
	coverage, err := pauserecord.ReadCoverage(pause.RecordPath, pause.Slug, time.Now())
	return coverage.Covered, err
}

// programMailbox is the relay's creel mailbox, a program client signing as `<shortname>:SLACK`.
// The client is built on the first call and kept, so a creel that cannot be reached fails the call that needs it;
// it is safe for concurrent use.
type programMailbox struct {
	newClient func() (*creelcli.GoClient, error)
	once      sync.Once
	client    *creelcli.GoClient
	err       error
}

// newProgramMailbox returns the mailbox of the SLACK program of the hub whose prime is prime.
func newProgramMailbox(prime *lymcwd.Location) *programMailbox {
	return &programMailbox{newClient: func() (*creelcli.GoClient, error) {
		return creelcli.NewProgramClient(prime, slackengine.SlackProgram)
	}}
}

// resolve returns the program client, building it on the first call.
func (m *programMailbox) resolve() (*creelcli.GoClient, error) {
	m.once.Do(func() { m.client, m.err = m.newClient() })
	return m.client, m.err
}

// Post leaves body as a message for receiver and returns its id.
func (m *programMailbox) Post(receiver, body string) (int64, error) {
	client, err := m.resolve()
	if err != nil {
		return 0, err
	}
	return client.Post(receiver, body)
}

// ReadAll reads every unread message without waiting and hands them to handle.
func (m *programMailbox) ReadAll(handle func([]creelengine.Message) error) error {
	client, err := m.resolve()
	if err != nil {
		return err
	}
	return client.ReadAll(handle)
}

// ReadWait blocks until there is unread mail and hands it to handle, up to timeout.
func (m *programMailbox) ReadWait(timeout time.Duration, handle func([]creelengine.Message) error) error {
	client, err := m.resolve()
	if err != nil {
		return err
	}
	return client.ReadWait(timeout, handle)
}

// spawnIntermediate is the double fork's intermediate step for the hub at hubPath.
func spawnIntermediate(hubPath, primeName string) error {
	geom, err := hubgeom.SlackGeometry(hubPath)
	if err != nil {
		return err
	}
	return slackengine.SpawnRelayFromIntermediate(geom, primeName)
}

// awaitReparent waits until this process is no longer a child of intermediate.
// A parent that cannot be read reads as no parent, which is not the intermediate.
func awaitReparent(clock slackengine.Clock, intermediate int) error {
	parent := func() int {
		pid, _ := proc.ParentPID(os.Getpid())
		return pid
	}
	return slackengine.AwaitReparent(intermediate, parent, clock, slackengine.ReparentBound)
}

// runRelay runs the relay of the hub at hubPath, whose prime worktree is primeName, until ctx ends.
// It reads the credentials without refusing a missing token: the relay itself tells the operator which one is missing.
func runRelay(ctx context.Context, hubPath, primeName string) error {
	geom, err := hubgeom.SlackGeometry(hubPath)
	if err != nil {
		return err
	}
	credentials, err := slackengine.ReadCredentials(geom.TokenPath)
	if err != nil {
		return err
	}
	prime, err := lymcwd.NamedWorktree(hubPath, primeName)
	if err != nil {
		return err
	}
	client := &http.Client{}
	deps := slackengine.RelayDeps{
		Mailbox: newProgramMailbox(prime),
		Slack:   slackengine.NewAPI(slackengine.DefaultBaseURL, credentials.BotToken, client),
		Dial:    slackengine.NewDialer(slackengine.DefaultBaseURL, credentials.AppToken, client, slackengine.HelloTimeout),
		Hub: slackengine.Hub{
			Beams:   func() ([]string, error) { return beamSlugs(prime) },
			Paused:  func(receiver string) (bool, error) { return receiverPaused(hubPath, prime, receiver) },
			Project: func(slug string) (string, string, error) { return orchProject(hubPath, slug) },
		},
		LiveHubs:       liveHubGeometries,
		Clock:          slackengine.SystemClock{},
		PID:            os.Getpid,
		ReexecLockWait: relayReexecLockWait,
		CheckBinary:    selfexec.New().CheckWith,
		ReapInherited: func() error {
			_, err := proc.ReapInheritedChildren()
			return err
		},
		Register: func() error {
			return runproc.Register(geom.RunProcDir, runproc.HubOwner, runproc.SelfRecord(runproc.RoleSlackRelay))
		},
	}
	return slackengine.RunRelay(ctx, geom, deps)
}

// receiverPaused reports whether a pause covers the orch named receiver:
// the prime's for the hub's own orch, and the beam's own for a beam's.
func receiverPaused(hubPath string, prime *lymcwd.Location, receiver string) (bool, error) {
	name, err := agentname.Parse(receiver)
	if err != nil {
		return false, err
	}
	worktree := prime
	if name.Slug != "" {
		if worktree, err = lymcwd.NamedWorktree(hubPath, name.Slug); err != nil {
			return false, fmt.Errorf("locate the orch of %s: %w", receiver, err)
		}
	}
	return orchPaused(worktree)
}
