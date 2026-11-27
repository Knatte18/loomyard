// client.go is the Go client of the daemon:
// it starts the daemon when none answers and checks which hub answers;
// it resumes the waiting verbs across a dropped connection.

package creelengine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// ConnectAttempts caps the connects a client makes after it started the daemon.
	ConnectAttempts = 6
	// MaxReconnects caps how often a waiting call reconnects after its connection dropped.
	MaxReconnects = 4

	connectWaitBase    = time.Second
	connectWaitCeiling = 8 * time.Second
	reconnectWait      = time.Second
	// maxResponseBytes bounds one response line.
	maxResponseBytes = 64 << 20
)

// Spawn describes a daemon that a client's spawn function started.
type Spawn struct {
	// PID is the pid of the started daemon.
	PID int
	// Exited receives a value once when that process exits; it is nil when the spawn gives no exit signal.
	Exited <-chan SpawnExit
}

// SpawnExit describes the exit of a spawned daemon.
type SpawnExit struct {
	// LostRace is true when the daemon exited because another daemon holds the hub's lock.
	LostRace bool
}

// Client talks to the daemon of one hub.
// Its clock measures the waits between connects and a waiting call's timeout.
type Client struct {
	geom  Geometry
	claim Claim
	// spawn starts the daemon detached; the client calls it at most once per connect.
	spawn func() (Spawn, error)
	clock Clock
}

// NewClient returns a Client for the hub in geom.
// claim states who the caller says it is.
// spawn starts the daemon when none answers.
// It returns the pid it started, with the signal of that daemon's exit; a nil spawn starts nothing.
// A client that gives up connecting after a spawn quotes that pid's lines from geom.DaemonLogPath.
func NewClient(geom Geometry, claim Claim, spawn func() (Spawn, error), clock Clock) *Client {
	return &Client{geom: geom, claim: claim, spawn: spawn, clock: clock}
}

// ErrAckUnsent marks a failed write of the acknowledgement line:
// the daemon never received it, so it journaled nothing of what it handed out.
// A confirmation the daemon refused or that was lost never matches it, since the acknowledgement was written.
var ErrAckUnsent = errors.New("creel: the acknowledgement could not be sent, so the daemon journaled nothing")

// connLostError marks a connection that dropped, or never came up, as opposed to a refusal by the daemon.
type connLostError struct{ err error }

func (e *connLostError) Error() string { return e.err.Error() }

func (e *connLostError) Unwrap() error { return e.err }

// daemonRefusal marks a refusal the daemon answered, not a dropped connection or an unreadable answer.
// Its text is the daemon's message.
// A refusal that carried a code also matches that code's sentinel under errors.Is.
type daemonRefusal struct{ err error }

func (e *daemonRefusal) Error() string { return e.err.Error() }

func (e *daemonRefusal) Unwrap() error { return e.err }

// connect dials the daemon's socket.
// When nothing answers it starts the daemon once and retries up to ConnectAttempts times.
// It sleeps on the client's clock between tries, the wait starting at one second and doubling up to a ceiling.
// A spawned daemon that exits other than by a lost start race ends the wait after one last connect;
// a lost race keeps the backoff, since the winner may not answer yet.
// When it gives up, its error names the socket and the daemon log and quotes the log's lines from that start.
func (c *Client) connect() (net.Conn, error) {
	conn, err := net.Dial("unix", c.geom.SocketPath)
	if err == nil {
		return conn, nil
	}
	if c.spawn == nil {
		return nil, &connLostError{fmt.Errorf("creel: no daemon answers on %s: %w", c.geom.SocketPath, err)}
	}
	logger.Info("creel: no daemon answers, starting one", "socket", c.geom.SocketPath)
	spawned, spawnErr := c.spawn()
	if spawnErr != nil {
		return nil, fmt.Errorf("creel: start the daemon for %s: %w", c.geom.SocketPath, spawnErr)
	}
	exited := spawned.Exited
	wait := connectWaitBase
	connects := 0
	for connects < ConnectAttempts {
		connects++
		if conn, err = net.Dial("unix", c.geom.SocketPath); err == nil {
			return conn, nil
		}
		if connects == ConnectAttempts {
			break
		}
		timer := c.clock.After(wait)
		wait = min(wait*2, connectWaitCeiling)
		select {
		case <-timer:
		case exit := <-exited:
			if !exit.LostRace {
				connects++
				if conn, err = net.Dial("unix", c.geom.SocketPath); err == nil {
					return conn, nil
				}
				return nil, c.gaveUp(spawned.PID, connects, err)
			}
			exited = nil
			<-timer
		}
	}
	return nil, c.gaveUp(spawned.PID, connects, err)
}

// gaveUp builds the error for a client whose spawned daemon never answered, after the given number of connects.
func (c *Client) gaveUp(pid, connects int, err error) error {
	return &connLostError{fmt.Errorf("creel: no daemon answers on %s after starting one and %d connects: %w; daemon log %s: %s",
		c.geom.SocketPath, connects, err, c.geom.DaemonLogPath, daemonLogExcerpt(c.geom.DaemonLogPath, pid))}
}

// waitState remembers what a waiting call learned, so a reconnect can resume it.
type waitState struct {
	id int64
	// handled is set once a response reached the caller's handler, after which a hook-wait is never resumed.
	handled bool
}

// Call sends the request as the client's claim and returns the final response.
// A refusal by the daemon comes back as an error carrying its message.
// One that carries a code also matches that code's sentinel under errors.Is.
// The timeout is measured on the client's clock over the whole request, waiting or not; zero means none.
// Its expiry returns ErrWaitTimeout.
// A waiting call whose connection drops reconnects up to MaxReconnects times.
// It starts the daemon if needed and resumes the same wait.
// A hook-wait resumes only until a ring reaches handle.
// A connection lost after that ends the call, since the daemon may already have journaled the mail the ring carried.
// A read's response is handed to handle and acknowledged only when handle returns nil.
// A handle that fails leaves the messages unread.
func (c *Client) Call(req Request, timeout time.Duration, handle func(Response) error) (Response, error) {
	return c.CallWatching(req, timeout, handle, nil)
}

// CallWatching sends the request as the client's claim and returns the final response.
// It hands every progress response of a waiting inject or post to progress, in arrival order.
// A nil progress ignores them.
func (c *Client) CallWatching(req Request, timeout time.Duration, handle func(Response) error, progress func(Response)) (Response, error) {
	req.Claim = c.claim
	var timer <-chan time.Time
	if timeout > 0 {
		timer = c.clock.After(timeout)
	}
	state := &waitState{}
	current := req
	established := false
	for reconnects := 0; ; reconnects++ {
		conn, err := c.connect()
		if err == nil {
			established = true
			var resp Response
			if resp, err = c.exchange(conn, current, timer, state, handle, progress); err == nil {
				return resp, nil
			}
		}
		var lost *connLostError
		if !errors.As(err, &lost) || !established || !req.Wait {
			return Response{}, err
		}
		if req.Verb == VerbHookWait && state.handled {
			return Response{}, err
		}
		if reconnects >= MaxReconnects {
			return Response{}, fmt.Errorf("creel: lost the connection to %s and gave up after %d reconnects: %w", c.geom.SocketPath, reconnects, err)
		}
		if current, err = c.resume(req, state); err != nil {
			return Response{}, err
		}
		select {
		case <-timer:
			return Response{}, ErrWaitTimeout
		case <-c.clock.After(reconnectWait):
		}
	}
}

// resume builds the request that continues a waiting call on a new connection.
// A read and a hook-wait resume by sending the same request again, since neither waits on an id.
// A hook-wait whose ring reached the handler is never resumed.
func (c *Client) resume(req Request, state *waitState) (Request, error) {
	resumed := Request{Claim: c.claim, ID: state.id, Wait: true}
	switch {
	case req.Verb == VerbRead, req.Verb == VerbHookWait:
		return req, nil
	case state.id == 0:
		return Request{}, fmt.Errorf("creel: the connection to %s dropped before the %s was acknowledged, so it cannot be resumed", c.geom.SocketPath, req.Verb)
	case req.Verb == VerbPost || req.Verb == VerbWaitReply:
		resumed.Verb = VerbWaitReply
	case req.Verb == VerbInject || req.Verb == VerbWaitInject:
		resumed.Verb = VerbWaitInject
	default:
		return Request{}, fmt.Errorf("creel: a %s request cannot be resumed", req.Verb)
	}
	return resumed, nil
}

type lineResult struct {
	data []byte
	err  error
}

// exchange sends req on conn and reads responses until the final one, handing each progress response to progress.
func (c *Client) exchange(conn net.Conn, req Request, timer <-chan time.Time, state *waitState, handle func(Response) error, progress func(Response)) (Response, error) {
	defer conn.Close()
	line, err := encodeLine(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(line); err != nil {
		return Response{}, &connLostError{err}
	}
	reader := bufio.NewReader(conn)
	lines := make(chan lineResult)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			data, err := readLine(reader, maxResponseBytes)
			select {
			case lines <- lineResult{data, err}:
			case <-stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-timer:
			return Response{}, ErrWaitTimeout
		case received := <-lines:
			if received.err != nil {
				return Response{}, &connLostError{fmt.Errorf("creel: the connection to %s closed before the answer: %w", c.geom.SocketPath, received.err)}
			}
			var resp Response
			if err := json.Unmarshal(received.data, &resp); err != nil {
				return Response{}, fmt.Errorf("creel: the daemon's answer is not valid JSON: %w", err)
			}
			if resp.HubPath != c.geom.HubPath {
				return Response{}, fmt.Errorf("creel: the daemon on %s answers for the hub %s, not %s", c.geom.SocketPath, resp.HubPath, c.geom.HubPath)
			}
			if !resp.OK {
				switch resp.Code {
				case CodeStrandClaimOutsidePane:
					return Response{}, &daemonRefusal{&codedRefusal{message: resp.Error, sentinel: ErrStrandClaimOutsidePane}}
				case CodeProgramClaimInStrandPane:
					return Response{}, &daemonRefusal{&codedRefusal{message: resp.Error, sentinel: ErrProgramClaimInStrandPane}}
				}
				return Response{}, &daemonRefusal{errors.New(resp.Error)}
			}
			if resp.Pending {
				state.id = resp.ID
				continue
			}
			if resp.Progress {
				if progress != nil {
					progress(resp)
				}
				continue
			}
			if resp.AckRequired {
				if handle != nil {
					state.handled = true
					if err := handle(resp); err != nil {
						return Response{}, err
					}
				}
				ack, _ := encodeLine(ackLine{Ack: true})
				if _, err := conn.Write(ack); err != nil {
					return Response{}, &connLostError{fmt.Errorf("%w: %w", ErrAckUnsent, err)}
				}
				if err := c.awaitAckConfirmation(lines, timer); err != nil {
					return Response{}, err
				}
			}
			return resp, nil
		}
	}
}

// awaitAckConfirmation waits for the daemon's line saying it journaled the acknowledged read.
func (c *Client) awaitAckConfirmation(lines <-chan lineResult, timer <-chan time.Time) error {
	select {
	case <-timer:
		return ErrWaitTimeout
	case received := <-lines:
		if received.err != nil {
			return &connLostError{fmt.Errorf("creel: the connection to %s closed before the daemon confirmed the read: %w", c.geom.SocketPath, received.err)}
		}
		var resp Response
		if err := json.Unmarshal(received.data, &resp); err != nil {
			return fmt.Errorf("creel: the daemon's confirmation is not valid JSON: %w", err)
		}
		if !resp.OK {
			return errors.New(resp.Error)
		}
		return nil
	}
}
