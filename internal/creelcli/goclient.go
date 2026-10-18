// goclient.go is the Go-side caller of creel: the calls other modules make from a program or a strand, each building its request here.

package creelcli

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
)

// goCallTimeout bounds every request a GoClient makes, so a caller under a lock waits at most this long per call.
const goCallTimeout = 30 * time.Second

// GoClient calls the creel daemon of one hub as a program or as a strand.
// It connects on its first call, starting the daemon when none answers.
type GoClient struct {
	client    *creelengine.Client
	shortname string
}

// NewProgramClient returns a client that signs as the program named program, an upper-case name such as BATTEN, under the hub's shortname.
// Its claim carries no strand name, whatever $LYX_STRAND_NAME holds, so it never claims a strand.
// It refuses a location outside a hub, and a program name that is not well formed.
func NewProgramClient(location *lyxcwd.Location, program string) (*GoClient, error) {
	return newGoClient(location, func(shortname string) (creelengine.Claim, error) {
		full, err := agentname.FormatProgram(shortname, program)
		if err != nil {
			return creelengine.Claim{}, fmt.Errorf("creel: the program name %q is not usable: %w", program, err)
		}
		return creelengine.Claim{Program: full}, nil
	})
}

// NewStrandClient returns a client that claims the strand named strandName and no program.
// The daemon accepts the claim only from a process in that strand's pane.
// It refuses a location outside a hub.
func NewStrandClient(location *lyxcwd.Location, strandName string) (*GoClient, error) {
	return newGoClient(location, func(string) (creelengine.Claim, error) {
		return creelengine.Claim{StrandName: strandName}, nil
	})
}

// newGoClient resolves the creel geometry of the location's hub and builds a client signing with the claim that claim returns for the hub's shortname.
func newGoClient(location *lyxcwd.Location, claim func(shortname string) (creelengine.Claim, error)) (*GoClient, error) {
	if location == nil || location.HubPath == "" {
		return nil, errors.New("creel: a client needs a hub, and this location is not in one; way forward: call it from a worktree of a lyx hub")
	}
	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		return nil, fmt.Errorf("creel: open the lyxtemp root: %w", err)
	}
	geometry, err := hubgeom.CreelGeometry(location.HubPath, root.Path())
	if err != nil {
		return nil, err
	}
	signed, err := claim(geometry.Shortname)
	if err != nil {
		return nil, err
	}
	hubPath := location.HubPath
	client := creelengine.NewClient(geometry, signed, func() (creelengine.Spawn, error) { return seam.spawn(hubPath, geometry.DaemonLogPath) }, seam.clock)
	return &GoClient{client: client, shortname: geometry.Shortname}, nil
}

// Shortname returns the shortname of the client's hub.
func (c *GoClient) Shortname() string { return c.shortname }

// Post leaves body as a message for receiver and returns its id.
func (c *GoClient) Post(receiver, body string) (int64, error) {
	resp, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbPost, Receiver: receiver, Text: body}, goCallTimeout, nil)
	return resp.ID, err
}

// ReadAll reads every unread message of the caller without waiting and hands them to handle.
// The daemon marks them read only once handle returns nil, so a handler error leaves them unread.
// With nothing unread handle is not called.
func (c *GoClient) ReadAll(handle func([]creelengine.Message) error) error {
	_, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbRead}, goCallTimeout, func(resp creelengine.Response) error {
		return handle(resp.Messages)
	})
	return err
}

// Inject queues text to be typed into receiver's pane at its next idle and returns the inject's id; a zero ttl takes the default.
func (c *GoClient) Inject(receiver, text string, ttl time.Duration) (int64, error) {
	resp, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbInject, Receiver: receiver, Text: text, TTLSeconds: int64(ttl / time.Second)}, goCallTimeout, nil)
	return resp.ID, err
}

// InjectCommand queues command to be run in the orch strand receiver at its next idle and returns the inject's id; a zero ttl takes the default.
func (c *GoClient) InjectCommand(receiver string, command creelengine.Command, ttl time.Duration) (int64, error) {
	resp, err := c.client.Call(creelengine.Request{
		Verb: creelengine.VerbInject, Receiver: receiver, Command: command.Name, Argument: command.Argument, TTLSeconds: int64(ttl / time.Second),
	}, goCallTimeout, nil)
	return resp.ID, err
}

// Log returns the journal entries matching filter, oldest first.
func (c *GoClient) Log(filter creelengine.LogFilter) ([]creelengine.LogEntry, error) {
	resp, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbLog, ID: filter.ID, From: filter.From, Receiver: filter.To, Since: filter.Since}, goCallTimeout, nil)
	return resp.Log, err
}

// Hold holds receiver's deliveries to the caller's own for duration.
func (c *GoClient) Hold(receiver string, duration time.Duration) error {
	_, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbHold, Receiver: receiver, ForSeconds: int64(duration / time.Second)}, goCallTimeout, nil)
	return err
}

// Release ends the caller's hold on receiver and reports whether there was one.
func (c *GoClient) Release(receiver string) (bool, error) {
	resp, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbRelease, Receiver: receiver}, goCallTimeout, nil)
	return resp.Released, err
}

// Withdraw removes the caller's own queued injects to receiver and its own unread mail held by receiver, and answers what it removed.
func (c *GoClient) Withdraw(receiver string) (creelengine.WithdrawResult, error) {
	resp, err := c.client.Call(creelengine.Request{Verb: creelengine.VerbWithdraw, Receiver: receiver}, goCallTimeout, nil)
	if err != nil || resp.Withdrawn == nil {
		return creelengine.WithdrawResult{}, err
	}
	return *resp.Withdrawn, nil
}
