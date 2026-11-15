//go:build !windows

package presence

import (
	"net"
	"time"
)

// DefaultDial connects to a Discord IPC unix socket.
func DefaultDial(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, time.Second)
}
