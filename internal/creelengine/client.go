// client.go is the Go client of the daemon: it starts the daemon when none answers, checks which hub answers, and resumes the waiting verbs across a dropped connection.

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

// Client talks to the daemon of one hub.
// Its clock measures the waits between connects and a waiting call's timeout.
type Client struct {
	geom  Geometry
	claim Claim
	// spawn starts the daemon detached; the client calls it at most once per connect.
	spawn func() error
	clock Clock
}

// NewClient returns a Client for the hub in geom.
// claim states who the caller says it is, and spawn starts the daemon when none answers; nil spawns nothing.
func NewClient(geom Geometry, claim Claim, spawn func() error, clock Clock) *Client {
	return &Client{geom: geom, claim: claim, spawn: spawn, clock: clock}
}

// connLostError marks a connection that dropped, or never came up, as opposed to a refusal by the daemon.
type connLostError struct{ err error }

func (e *connLostError) Error() string { return e.err.Error() }

func (e *connLostError) Unwrap() error { return e.err }

// connect dials the daemon's socket.
// When nothing answers it starts the daemon once and retries up to ConnectAttempts times, sleeping on the told clock from one second between tries.
func (c *Client) connect() (net.Conn, error) {
	conn, err := net.Dial("unix", c.geom.SocketPath)
	if err == nil {
		return conn, nil
	}
	if c.spawn == nil {
		return nil, &connLostError{fmt.Errorf("creel: no daemon answers on %s: %w", c.geom.SocketPath, err)}
	}
	logger.Info("creel: no daemon answers, starting one", "socket", c.geom.SocketPath)
	if spawnErr := c.spawn(); spawnErr != nil {
		return nil, fmt.Errorf("creel: start the daemon for %s: %w", c.geom.SocketPath, spawnErr)
	}
	wait := connectWaitBase
	for attempt := 1; attempt <= ConnectAttempts; attempt++ {
		if conn, err = net.Dial("unix", c.geom.SocketPath); err == nil {
			return conn, nil
		}
		if attempt < ConnectAttempts {
			<-c.clock.After(wait)
			wait = min(wait*2, connectWaitCeiling)
		}
	}
	return nil, &connLostError{fmt.Errorf("creel: no daemon answers on %s after starting one and %d connects: %w", c.geom.SocketPath, ConnectAttempts, err)}
}

// waitState remembers what a waiting call learned, so a reconnect can resume it.
type waitState struct {
	id int64
}

// Call sends the request as the client's claim and returns the final response.
// A refusal by the daemon comes back as an error carrying its message.
// A waiting request (Wait set) takes a timeout measured on the client's clock, zero for none; its expiry returns ErrWaitTimeout.
// A waiting call whose connection drops reconnects up to MaxReconnects times, starting the daemon if needed, and resumes the same wait.
// A read's response is handed to handle, and acknowledged only when handle returns nil; a handle that fails leaves the messages unread.
func (c *Client) Call(req Request, timeout time.Duration, handle func(Response) error) (Response, error) {
	return c.CallWatching(req, timeout, handle, nil)
}

// CallWatching is Call that also hands every progress response of a waiting inject to progress, in the order they arrive; a nil progress ignores them.
func (c *Client) CallWatching(req Request, timeout time.Duration, handle func(Response) error, progress func(Response)) (Response, error) {
	req.Claim = c.claim
	var timer <-chan time.Time
	if req.Wait && timeout > 0 {
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
func (c *Client) resume(req Request, state *waitState) (Request, error) {
	resumed := Request{Claim: c.claim, ID: state.id, Wait: true}
	switch {
	case req.Verb == VerbRead:
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
				return Response{}, errors.New(resp.Error)
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
					if err := handle(resp); err != nil {
						return Response{}, err
					}
				}
				ack, _ := encodeLine(ackLine{Ack: true})
				if _, err := conn.Write(ack); err != nil {
					return Response{}, &connLostError{err}
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
