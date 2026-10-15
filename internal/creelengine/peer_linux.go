// peer_linux.go reads the process id of a unix-socket peer with SO_PEERCRED.

package creelengine

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerPID returns the process id of the process that connected to conn, as the kernel recorded it at connect time.
func peerPID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("creel: read the peer of the connection: %w", err)
	}
	var (
		cred    *unix.Ucred
		credErr error
	)
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("creel: read the peer of the connection: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("creel: read the peer credentials of the connection: %w", credErr)
	}
	return int(cred.Pid), nil
}
