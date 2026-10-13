// stalesocket.go builds a stale unix socket file for tests of the code that removes dead tmux sockets.

package tmuxkit

import (
	"net"
	"testing"
)

// BindStaleSocket leaves a socket file at path that refuses every stream connection.
// It binds a datagram socket and closes it, and a datagram socket never listens, so a child forked by a parallel test that inherits the fd before its exec cannot make the path answer a `Dial` as a closed listener's inherited copy would.
// A unix socket path is limited to about 104 bytes, so callers keep path short.
// It fails the test on its own setup only.
func BindStaleSocket(t *testing.T, path string) {
	t.Helper()
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
