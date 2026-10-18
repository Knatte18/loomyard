// server.go runs the per-hub daemon: it takes the hub's lock, binds the socket, serves one request per connection and delivers what the pool holds until its binary is replaced.

package creelengine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// pollInterval is how often the daemon asks whether its binary was replaced and whether the UTC day changed.
	pollInterval = time.Second
	// requestReadTimeout bounds how long a connection may take to send its request line.
	requestReadTimeout = 30 * time.Second
	// ackReadTimeout bounds how long a reading client may take to acknowledge the messages it was handed.
	ackReadTimeout = 30 * time.Second
)

// Options is what Run is told.
type Options struct {
	Geometry Geometry
	Delivery Delivery
	Clock    Clock
	// ParentPID reads a process's parent for the peer's ancestry walk.
	ParentPID func(pid int) (parent int, ok bool)
	// GOOS is the platform the daemon runs on; only "linux" resolves identity from process ancestry.
	GOOS string
	// PollClock, when set, times the daemon's once-a-second poll in place of Clock, so a test steps the poll apart from the delivery's waits.
	PollClock Clock
	// Replaced reports whether the daemon's binary was replaced; nil never reports it.
	Replaced func() bool
	// Ready, when set, is called once the daemon listens on its socket.
	Ready func()
}

// Run serves the hub until ctx ends or its binary is replaced, then returns nil.
// It returns ErrDaemonRunning when another daemon holds the hub's lock.
// On a replaced binary it starts no new send, waits for the sends in flight and journals their outcomes, closes its listener and releases the lock.
func Run(ctx context.Context, opts Options) error {
	if opts.ParentPID == nil {
		opts.ParentPID = func(int) (int, bool) { return 0, false }
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.PollClock == nil {
		opts.PollClock = opts.Clock
	}
	geom := opts.Geometry
	if err := os.MkdirAll(geom.StateDir, 0o755); err != nil {
		return fmt.Errorf("creel: create %s: %w", geom.StateDir, err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
	if err != nil {
		return fmt.Errorf("creel: take the daemon lock: %w", err)
	}
	if !acquired {
		return ErrDaemonRunning
	}
	defer func() { _ = held.Release() }()

	if err := removeUnansweredSocket(geom.SocketPath); err != nil {
		return err
	}
	store, err := OpenStore(geom, opts.Delivery, opts.Clock)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: geom.SocketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("creel: listen on %s: %w", geom.SocketPath, err)
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	scheduler := NewScheduler(store, opts.Delivery, opts.Clock)
	scheduler.Start()
	srv := &daemon{opts: opts, store: store, done: make(chan struct{})}
	srv.wg.Add(2)
	go srv.accept(listener)
	go srv.poll(runCtx, stop, journalDay(opts.Clock.Now()), opts.PollClock.After(pollInterval))
	if opts.Ready != nil {
		opts.Ready()
	}

	<-runCtx.Done()
	scheduler.Stop()
	_ = listener.Close()
	close(srv.done)
	srv.wg.Wait()
	return nil
}

// removeUnansweredSocket removes a socket file no listener answers.
// It dials, and only on a refusal re-stats and removes the same file, so a daemon that just bound is never unlinked.
func removeUnansweredSocket(path string) error {
	first, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creel: stat socket %s: %w", path, err)
	}
	if first.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("creel: %s exists and is not a socket", path)
	}
	if conn, err := net.Dial("unix", path); err == nil {
		_ = conn.Close()
		return fmt.Errorf("creel: a daemon already answers on %s", path)
	}
	second, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creel: restat socket %s: %w", path, err)
	}
	if !os.SameFile(first, second) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("creel: remove the stale socket %s: %w", path, err)
	}
	return nil
}

// daemon is one running Run.
type daemon struct {
	opts  Options
	store *Store
	// done is closed when the daemon shuts down, which ends every waiting connection.
	done chan struct{}
	wg   sync.WaitGroup
}

// poll asks once a second whether the binary was replaced, which ends the daemon, and whether the UTC day changed, which runs retention.
func (d *daemon) poll(ctx context.Context, stop context.CancelFunc, lastDay string, timer <-chan time.Time) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer:
		}
		if d.opts.Replaced != nil && d.opts.Replaced() {
			logger.Info("creel daemon: the binary was replaced, shutting down")
			stop()
			return
		}
		if day := journalDay(d.opts.Clock.Now()); day != lastDay {
			lastDay = day
			if err := d.store.RollDay(); err != nil {
				logger.Warn("creel daemon: could not roll the journal to the new day", "error", err)
			}
		}
		timer = d.opts.PollClock.After(pollInterval)
	}
}

func (d *daemon) accept(listener *net.UnixListener) {
	defer d.wg.Done()
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			defer conn.Close()
			d.serve(conn)
		}()
	}
}

// serve answers the one request on conn.
func (d *daemon) serve(conn *net.UnixConn) {
	reader := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(requestReadTimeout))
	line, err := readLine(reader, maxRequestBytes)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		d.fail(conn, fmt.Errorf("creel: could not read the request line: %w", err))
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		d.fail(conn, fmt.Errorf("creel: the request line is not valid JSON: %w", err))
		return
	}
	sender, err := d.resolve(conn, req)
	if err != nil {
		d.fail(conn, err)
		return
	}
	if err := d.dispatch(conn, reader, sender, req); err != nil {
		d.fail(conn, err)
	}
}

// resolve decides who sent the request from the connecting peer and its claim.
func (d *daemon) resolve(conn *net.UnixConn, req Request) (Sender, error) {
	pid, err := peerPID(conn)
	if err != nil {
		return Sender{}, err
	}
	strands, strandsErr := d.opts.Delivery.Strands()
	return resolveSender(pid, req.Claim, d.opts.Geometry.Shortname, strands, strandsErr, d.opts.ParentPID, d.opts.GOOS)
}

// write sends one response line carrying the hub path.
func (d *daemon) write(conn net.Conn, resp Response) error {
	resp.HubPath = d.opts.Geometry.HubPath
	line, err := encodeLine(resp)
	if err != nil {
		return err
	}
	_, err = conn.Write(line)
	return err
}

func (d *daemon) fail(conn net.Conn, err error) {
	_ = d.write(conn, Response{OK: false, Error: err.Error()})
}

// closeWatcher notices the client closing a connection while the daemon waits for an outcome.
type closeWatcher struct {
	conn   net.Conn
	reader *bufio.Reader
	closed chan struct{}
	exited chan struct{}
}

func watchClose(conn net.Conn, reader *bufio.Reader) *closeWatcher {
	watcher := &closeWatcher{conn: conn, reader: reader, closed: make(chan struct{}), exited: make(chan struct{})}
	go func() {
		defer close(watcher.exited)
		if _, err := reader.ReadByte(); err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			close(watcher.closed)
		}
	}()
	return watcher
}

// stop ends the watch and leaves the connection readable again.
func (w *closeWatcher) stop() {
	_ = w.conn.SetReadDeadline(time.Now())
	<-w.exited
	_ = w.conn.SetReadDeadline(time.Time{})
}

// dispatch runs the verb of the request as sender and writes its response.
func (d *daemon) dispatch(conn *net.UnixConn, reader *bufio.Reader, sender Sender, req Request) error {
	store := d.store
	ok := Response{OK: true, Sender: sender.Name}
	switch req.Verb {
	case VerbPost:
		if req.Wait {
			id, waiter, err := store.PostWait(sender, req.Receiver, req.Text)
			if err != nil {
				return err
			}
			return d.awaitOutcome(conn, reader, waiter, id, sender)
		}
		id, err := store.Post(sender, req.Receiver, req.Text)
		if err != nil {
			return err
		}
		ok.ID = id
	case VerbWaitReply:
		waiter, err := store.WaitReply(sender, req.ID)
		if err != nil {
			return err
		}
		return d.awaitOutcome(conn, reader, waiter, req.ID, sender)
	case VerbReply:
		id, err := store.Reply(sender, req.ID, req.Text)
		if err != nil {
			return err
		}
		ok.ID = id
	case VerbRead:
		return d.read(conn, reader, sender, req)
	case VerbInbox:
		inbox, err := store.Inbox(sender)
		if err != nil {
			return err
		}
		ok.Inbox = &inbox
	case VerbForward:
		if err := store.Forward(sender, req.ID, req.Receiver); err != nil {
			return err
		}
	case VerbLog:
		ok.Log = store.Log(LogFilter{ID: req.ID, From: req.From, To: req.Receiver, Since: req.Since})
	case VerbInject:
		ttl := time.Duration(req.TTLSeconds) * time.Second
		if req.Command != "" || req.Argument != "" {
			if req.Wait || req.Interrupt {
				return errors.New("creel: a command inject takes neither wait nor interrupt")
			}
			result, err := store.InjectCommand(sender, req.Receiver, Command{Name: req.Command, Argument: req.Argument}, ttl)
			if err != nil {
				return err
			}
			ok.ID = result.ID
			break
		}
		if req.Wait {
			result, waiter, err := store.InjectWait(sender, req.Receiver, req.Text, ttl, req.Interrupt)
			if err != nil {
				return err
			}
			return d.awaitOutcome(conn, reader, waiter, result.ID, sender)
		}
		result, err := store.Inject(sender, req.Receiver, req.Text, ttl, req.Interrupt)
		if err != nil {
			return err
		}
		ok.ID, ok.Interrupted = result.ID, result.Interrupted
	case VerbWaitInject:
		waiter, err := store.WaitInject(sender, req.ID)
		if err != nil {
			return err
		}
		return d.awaitOutcome(conn, reader, waiter, req.ID, sender)
	case VerbInterrupt:
		sent, err := store.Interrupt(sender, req.Receiver)
		if err != nil {
			return err
		}
		ok.Sent = sent
	case VerbHold:
		held, err := store.Hold(sender, req.Receiver, time.Duration(req.ForSeconds)*time.Second)
		if err != nil {
			return err
		}
		ok.Hold = &held
	case VerbRelease:
		released, err := store.Release(sender, req.Receiver)
		if err != nil {
			return err
		}
		ok.Released = released
	default:
		return fmt.Errorf("creel: unknown verb %q", req.Verb)
	}
	return d.write(conn, ok)
}

// awaitOutcome answers a waiting verb: an interim response with the id at once, a progress response for each stall note of a waiting inject, and the outcome when the waiter is answered.
// The interim and final responses carry the resolved sender's name.
// A client that closes its connection, or a daemon that shuts down, ends the wait with no answer.
func (d *daemon) awaitOutcome(conn net.Conn, reader *bufio.Reader, waiter *Waiter, id int64, sender Sender) error {
	defer waiter.Cancel()
	if err := d.write(conn, Response{OK: true, Pending: true, ID: id, Sender: sender.Name}); err != nil {
		return nil
	}
	watcher := watchClose(conn, reader)
	defer watcher.stop()
	for {
		select {
		case note := <-waiter.Progress:
			if err := d.write(conn, progressResponse(id, note)); err != nil {
				return nil
			}
		case outcome := <-waiter.C:
			// A note read before the outcome still goes out first, so the client sees the last stall in order.
			select {
			case note := <-waiter.Progress:
				if err := d.write(conn, progressResponse(id, note)); err != nil {
					return nil
				}
			default:
			}
			resp := Response{OK: true, ID: id, Sender: sender.Name, Outcome: outcome.Kind, Stall: outcome.Stall, StallAt: outcome.StallAt}
			if outcome.Kind == OutcomeReply || outcome.Kind == OutcomeReturned {
				resp.Message = &outcome.Message
			}
			_ = d.write(conn, resp)
			return nil
		case <-watcher.closed:
			return nil
		case <-d.done:
			return nil
		}
	}
}

// progressResponse is the interim response that carries a stall note of the inject id.
func progressResponse(id int64, note StallNote) Response {
	return Response{OK: true, Progress: true, ID: id, Stall: note.Stall, StallAt: note.At}
}

// read answers a read: the caller's messages, reserved until the client acknowledges them on the same connection.
// With Wait it first waits until the caller holds unread mail.
func (d *daemon) read(conn *net.UnixConn, reader *bufio.Reader, sender Sender, req Request) error {
	var reading *Reading
	for {
		var err error
		if reading, err = d.store.Read(sender, req.From, req.All); err != nil {
			return err
		}
		if !req.Wait || len(reading.ids) > 0 {
			break
		}
		reading.Release()
		waiter, err := d.store.WaitMail(sender)
		if err != nil {
			return err
		}
		if !d.waitFor(conn, reader, waiter) {
			return nil
		}
	}
	resp := Response{OK: true, Sender: sender.Name, Messages: reading.Messages, AckRequired: len(reading.ids) > 0}
	if err := d.write(conn, resp); err != nil {
		reading.Release()
		return nil
	}
	if !resp.AckRequired {
		return nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(ackReadTimeout))
	line, err := readLine(reader, maxRequestBytes)
	var ack ackLine
	if err != nil || json.Unmarshal(line, &ack) != nil || !ack.Ack {
		reading.Release()
		return nil
	}
	if err := reading.Ack(); err != nil {
		logger.Warn("creel daemon: could not journal an acknowledged read", "error", err)
		return d.write(conn, Response{OK: false, Error: err.Error()})
	}
	return d.write(conn, Response{OK: true, Sender: sender.Name})
}

// waitFor blocks until the waiter is answered, and reports false when the client left or the daemon is shutting down.
func (d *daemon) waitFor(conn net.Conn, reader *bufio.Reader, waiter *Waiter) bool {
	defer waiter.Cancel()
	watcher := watchClose(conn, reader)
	defer watcher.stop()
	select {
	case <-waiter.C:
		return true
	case <-watcher.closed:
	case <-d.done:
	}
	return false
}
