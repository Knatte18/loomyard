// socket.go is the Slack Socket Mode client: the Socket seam the relay is told, and the WebSocket connection behind it.

package slackengine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const (
	// HelloTimeout is how long a dial waits for the `hello` Slack sends first on a new connection.
	HelloTimeout = 10 * time.Second
	// socketReadLimit bounds one Socket Mode message, which carries a whole event payload.
	socketReadLimit = 1 << 20
	// socketQueueLength is how many decoded envelopes wait for Read before the reader stops taking messages.
	socketQueueLength = 16
	// helloType is the type of the envelope Slack sends first on a new connection.
	helloType = "hello"
)

// Socket is one Socket Mode connection.
// Its methods are safe to call from different goroutines.
type Socket interface {
	// Read returns the next envelope, and returns the connection's error once it failed or closed.
	Read(ctx context.Context) (Envelope, error)
	// Ack acknowledges the envelope with the id, which Slack sends again when it goes unacknowledged.
	Ack(ctx context.Context, envelopeID string) error
	// Ping sends a WebSocket ping and returns once the pong arrives or ctx ends.
	Ping(ctx context.Context) error
	// Close closes the connection.
	Close() error
}

// Envelope is one message Slack sends on a Socket Mode connection.
type Envelope struct {
	// Type is `hello`, `events_api` or `disconnect`.
	Type string
	// EnvelopeID is the id to acknowledge, empty for a `hello` and a `disconnect`.
	EnvelopeID string
	// Reason is why Slack is closing the connection, set for a `disconnect`.
	Reason string
	// Channel is the channel of an `events_api` envelope's event.
	Channel string
	// TS is the timestamp of that event's message.
	TS string
	// ThreadTS is the timestamp of the thread that message belongs to, empty outside a thread.
	ThreadTS string
	// Subtype is that event's subtype, empty for a plain message.
	Subtype string
}

// Dialer opens a Socket Mode connection and returns it with the id of the Slack app Slack's `hello` names.
// A refused apps.connections.open is an *APIError, so a caller tells a refused app from a transport failure.
type Dialer func(ctx context.Context) (Socket, string, error)

// wireEnvelope is the part of a Socket Mode message the client reads.
type wireEnvelope struct {
	Type           string `json:"type"`
	EnvelopeID     string `json:"envelope_id"`
	Reason         string `json:"reason"`
	ConnectionInfo struct {
		AppID string `json:"app_id"`
	} `json:"connection_info"`
	Payload struct {
		Event struct {
			Channel  string `json:"channel"`
			TS       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
			Subtype  string `json:"subtype"`
		} `json:"event"`
	} `json:"payload"`
}

// NewDialer returns a Dialer that asks Slack for a connection URL with the app-level token over client.
// It dials that URL and reads the `hello` Slack sends first within helloTimeout.
// A connection that sends no `hello` in time, or a first message that is not one, is closed and the dial fails.
func NewDialer(baseURL, appToken string, client *http.Client, helloTimeout time.Duration) Dialer {
	api := NewAPI(baseURL, appToken, client)
	return func(ctx context.Context) (Socket, string, error) {
		connectionURL, err := api.OpenConnection(ctx)
		if err != nil {
			return nil, "", err
		}
		conn, _, err := websocket.Dial(ctx, connectionURL, &websocket.DialOptions{HTTPClient: client})
		if err != nil {
			return nil, "", fmt.Errorf("slack: dial the Socket Mode connection: %w", err)
		}
		conn.SetReadLimit(socketReadLimit)
		hello, err := readHello(ctx, conn, helloTimeout)
		if err != nil {
			_ = conn.CloseNow()
			return nil, "", err
		}
		return newWebSocket(conn), hello.ConnectionInfo.AppID, nil
	}
}

// readHello reads the first message of conn, which must be a `hello`, within timeout.
func readHello(ctx context.Context, conn *websocket.Conn, timeout time.Duration) (wireEnvelope, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		return wireEnvelope{}, fmt.Errorf("slack: read the Socket Mode hello: %w", err)
	}
	hello, err := decodeWireEnvelope(data)
	if err != nil {
		return wireEnvelope{}, err
	}
	if hello.Type != helloType {
		return wireEnvelope{}, fmt.Errorf("slack: the Socket Mode connection began with a %q message, not a hello", hello.Type)
	}
	return hello, nil
}

// decodeWireEnvelope decodes one Socket Mode message.
func decodeWireEnvelope(data []byte) (wireEnvelope, error) {
	var wire wireEnvelope
	if err := json.Unmarshal(data, &wire); err != nil {
		return wireEnvelope{}, fmt.Errorf("slack: decode a Socket Mode message: %w", err)
	}
	return wire, nil
}

// wsSocket is a Socket over a coder/websocket connection.
// One reader goroutine takes every message off the connection.
// The library answers a ping's pong only while a read is in progress.
// Read takes the decoded envelopes from that goroutine.
type wsSocket struct {
	conn     *websocket.Conn
	incoming chan Envelope
	stop     context.CancelFunc
	// err is why the reader ended; it is written before incoming is closed and read only after.
	err error
}

// newWebSocket returns a wsSocket over conn and starts its reader.
func newWebSocket(conn *websocket.Conn) *wsSocket {
	ctx, stop := context.WithCancel(context.Background())
	s := &wsSocket{conn: conn, incoming: make(chan Envelope, socketQueueLength), stop: stop}
	go s.readLoop(ctx)
	return s
}

// readLoop decodes each message of the connection into incoming until the connection fails or ctx ends.
func (s *wsSocket) readLoop(ctx context.Context) {
	defer close(s.incoming)
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			s.err = fmt.Errorf("slack: read the Socket Mode connection: %w", err)
			return
		}
		wire, err := decodeWireEnvelope(data)
		if err != nil {
			s.err = err
			_ = s.conn.CloseNow()
			return
		}
		envelope := Envelope{
			Type:       wire.Type,
			EnvelopeID: wire.EnvelopeID,
			Reason:     wire.Reason,
			Channel:    wire.Payload.Event.Channel,
			TS:         wire.Payload.Event.TS,
			ThreadTS:   wire.Payload.Event.ThreadTS,
			Subtype:    wire.Payload.Event.Subtype,
		}
		select {
		case s.incoming <- envelope:
		case <-ctx.Done():
			s.err = fmt.Errorf("slack: read the Socket Mode connection: %w", ctx.Err())
			return
		}
	}
}

// Read returns the next envelope, and the connection's error once it failed or closed.
func (s *wsSocket) Read(ctx context.Context) (Envelope, error) {
	select {
	case envelope, open := <-s.incoming:
		if !open {
			return Envelope{}, s.err
		}
		return envelope, nil
	case <-ctx.Done():
		return Envelope{}, ctx.Err()
	}
}

// Ack writes `{"envelope_id": …}` for the envelope with the id.
func (s *wsSocket) Ack(ctx context.Context, envelopeID string) error {
	data, err := json.Marshal(map[string]string{"envelope_id": envelopeID})
	if err != nil {
		return fmt.Errorf("slack: encode a Socket Mode acknowledgement: %w", err)
	}
	if err := s.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("slack: acknowledge a Socket Mode envelope: %w", err)
	}
	return nil
}

// Ping sends a WebSocket ping and returns once the pong arrives or ctx ends.
func (s *wsSocket) Ping(ctx context.Context) error {
	if err := s.conn.Ping(ctx); err != nil {
		return fmt.Errorf("slack: ping the Socket Mode connection: %w", err)
	}
	return nil
}

// Close closes the connection and ends the reader.
func (s *wsSocket) Close() error {
	defer s.stop()
	return s.conn.Close(websocket.StatusNormalClosure, "")
}
