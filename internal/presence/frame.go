// Package presence talks to the local Discord client to publish activities.
package presence

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// IPC opcodes of the Discord local protocol.
const (
	OpHandshake = 0
	OpFrame     = 1
	OpClose     = 2
	OpPing      = 3
	OpPong      = 4
)

// MaxPayload is the largest payload accepted from Discord.
const MaxPayload = 1 << 20

// ErrPayloadTooLarge is returned when a frame announces more than MaxPayload bytes.
var ErrPayloadTooLarge = errors.New("presence: payload too large")

// EncodeFrame serialises payload as JSON behind the 8-byte little-endian
// header (opcode, length).
func EncodeFrame(op uint32, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("presence: encode payload: %w", err)
	}
	out := make([]byte, 8, 8+len(body))
	binary.LittleEndian.PutUint32(out[0:4], op)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(body))) //nolint:gosec // bounded by json size
	return append(out, body...), nil
}

// DecodeFrame reads one frame. It refuses payloads larger than MaxPayload
// before allocating anything for them.
func DecodeFrame(r io.Reader) (op uint32, payload []byte, err error) {
	var header [8]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		return 0, nil, fmt.Errorf("presence: read header: %w", err)
	}
	op = binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > MaxPayload {
		return 0, nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, length)
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, fmt.Errorf("presence: read payload: %w", err)
	}
	return op, payload, nil
}
