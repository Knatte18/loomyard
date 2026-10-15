//go:build !linux

// peer_other.go reports no peer process off Linux, where identity is the environment alone.

package creelengine

import "net"

// peerPID reports no peer: 0 means the platform cannot tell.
func peerPID(*net.UnixConn) (int, error) { return 0, nil }
