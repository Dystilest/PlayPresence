//go:build windows

package presence

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// DefaultDial connects to a Discord IPC named pipe.
func DefaultDial(path string) (net.Conn, error) {
	timeout := time.Second
	return winio.DialPipe(path, &timeout)
}
