// socket_test.go covers the Socket Mode client against an httptest server that speaks Slack's side of the protocol.

package slackengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	helloMessage      = `{"type":"hello","num_connections":1,"connection_info":{"app_id":"A161CLERW"}}`
	eventsMessage     = `{"envelope_id":"E1","type":"events_api","accepts_response_payload":false,"payload":{"event":{"type":"message","channel":"C1","ts":"1.000001","thread_ts":"1.000000","subtype":"file_share"}}}`
	disconnectMessage = `{"type":"disconnect","reason":"refresh_requested"}`
)

// socketServer is an httptest server that answers apps.connections.open and serves the WebSocket it names.
type socketServer struct {
	server *httptest.Server
	// authorization receives the Authorization header of each apps.connections.open request.
	authorization chan string
}

// newSocketServer starts a server whose WebSocket runs serve once it accepted a connection.
// With openReply nil, apps.connections.open names the server's own WebSocket; otherwise it answers openReply.
func newSocketServer(t *testing.T, openReply *string, serve func(ctx context.Context, conn *websocket.Conn)) *socketServer {
	t.Helper()
	fake := &socketServer{authorization: make(chan string, 4)}
	mux := http.NewServeMux()
	mux.HandleFunc("/apps.connections.open", func(w http.ResponseWriter, r *http.Request) {
		fake.authorization <- r.Header.Get("Authorization")
		if openReply != nil {
			answerJSON(w, *openReply)
			return
		}
		answerJSON(w, `{"ok":true,"url":"ws://`+r.Host+`/socket"}`)
	})
	mux.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("Accept = %v", err)
			return
		}
		defer conn.CloseNow()
		serve(r.Context(), conn)
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// dialer returns a Dialer for the server with the app-level token xapp-secret and the given hello bound.
func (fake *socketServer) dialer(helloTimeout time.Duration) slackengine.Dialer {
	return slackengine.NewDialer(fake.server.URL+"/", "xapp-secret", fake.server.Client(), helloTimeout)
}

// drain reads from conn until it fails, which keeps the connection answering pings and close frames.
func drain(ctx context.Context, conn *websocket.Conn) {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func TestSocketClient(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	ctx := context.Background()

	t.Run("DialReadsTheHelloAndTheSocketReadsAcksAndPings", func(t *testing.T) {
		t.Parallel()
		acks := make(chan string, 1)
		fake := newSocketServer(t, nil, func(ctx context.Context, conn *websocket.Conn) {
			for _, message := range []string{helloMessage, eventsMessage} {
				if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
					return
				}
			}
			_, ack, err := conn.Read(ctx)
			if err != nil {
				return
			}
			acks <- string(ack)
			if err := conn.Write(ctx, websocket.MessageText, []byte(disconnectMessage)); err != nil {
				return
			}
			drain(ctx, conn)
		})
		socket, appID, err := fake.dialer(time.Second)(ctx)
		if err != nil {
			t.Fatalf("dial = %v", err)
		}
		t.Cleanup(func() { _ = socket.Close() })
		if appID != "A161CLERW" {
			t.Errorf("dial app id = %q; want A161CLERW", appID)
		}
		if got := <-fake.authorization; got != "Bearer xapp-secret" {
			t.Errorf("apps.connections.open authorization = %q; want the app-level token", got)
		}

		events, err := socket.Read(ctx)
		if err != nil {
			t.Fatalf("Read events = %v", err)
		}
		wantEvents := slackengine.Envelope{
			Type: "events_api", EnvelopeID: "E1", Channel: "C1", TS: "1.000001", ThreadTS: "1.000000", Subtype: "file_share",
		}
		if diff := cmp.Diff(wantEvents, events); diff != "" {
			t.Errorf("Read events mismatch (-want +got):\n%s", diff)
		}
		if err := socket.Ack(ctx, events.EnvelopeID); err != nil {
			t.Fatalf("Ack = %v", err)
		}
		var ack map[string]string
		if err := json.Unmarshal([]byte(<-acks), &ack); err != nil || !cmp.Equal(ack, map[string]string{"envelope_id": "E1"}) {
			t.Errorf("server received %v, %v; want the acknowledgement of E1", ack, err)
		}
		if err := socket.Ping(ctx); err != nil {
			t.Errorf("Ping = %v", err)
		}
		disconnect, err := socket.Read(ctx)
		if want := (slackengine.Envelope{Type: "disconnect", Reason: "refresh_requested"}); err != nil || disconnect != want {
			t.Errorf("Read disconnect = %+v, %v; want %+v", disconnect, err, want)
		}
	})

	t.Run("RefusedConnectionOpenIsAnAPIError", func(t *testing.T) {
		t.Parallel()
		reply := `{"ok":false,"error":"invalid_auth"}`
		fake := newSocketServer(t, &reply, drain)
		_, _, err := fake.dialer(time.Second)(ctx)
		var apiErr *slackengine.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "invalid_auth" {
			t.Errorf("dial error = %v; want an *APIError with code invalid_auth", err)
		}
	})

	t.Run("DialFailsWhenTheFirstMessageIsNoHello", func(t *testing.T) {
		t.Parallel()
		fake := newSocketServer(t, nil, func(ctx context.Context, conn *websocket.Conn) {
			if err := conn.Write(ctx, websocket.MessageText, []byte(eventsMessage)); err != nil {
				return
			}
			drain(ctx, conn)
		})
		if _, _, err := fake.dialer(time.Second)(ctx); err == nil || !strings.Contains(err.Error(), "not a hello") {
			t.Errorf("dial error = %v; want a refusal naming that the first message is not a hello", err)
		}
	})

	t.Run("DialFailsWhenNoHelloComesWithinTheBound", func(t *testing.T) {
		t.Parallel()
		fake := newSocketServer(t, nil, drain)
		if _, _, err := fake.dialer(50 * time.Millisecond)(ctx); err == nil {
			t.Error("dial = nil error; want a failure once the hello bound passed")
		}
	})
}
