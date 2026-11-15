package presence

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := map[string]any{"v": float64(1), "client_id": "123"}

	raw, err := EncodeFrame(OpHandshake, payload)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	op, body, err := DecodeFrame(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}

	if op != OpHandshake {
		t.Errorf("op = %d, want %d", op, OpHandshake)
	}
	if !bytes.Contains(body, []byte(`"client_id":"123"`)) {
		t.Errorf("body = %s", body)
	}
}

func TestEncodeFrameHeaderIsLittleEndian(t *testing.T) {
	raw, err := EncodeFrame(OpFrame, map[string]string{"a": "b"})
	if err != nil {
		t.Fatal(err)
	}

	if got := binary.LittleEndian.Uint32(raw[0:4]); got != OpFrame {
		t.Errorf("op = %d", got)
	}
	if got := binary.LittleEndian.Uint32(raw[4:8]); int(got) != len(raw)-8 {
		t.Errorf("length = %d, want %d", got, len(raw)-8)
	}
}

func TestDecodeFrameRejectsOversizedPayload(t *testing.T) {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], OpFrame)
	binary.LittleEndian.PutUint32(header[4:8], MaxPayload+1)

	_, _, err := DecodeFrame(bytes.NewReader(header))

	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("err = %v, want ErrPayloadTooLarge", err)
	}
}

func TestDecodeFrameTruncatedInput(t *testing.T) {
	raw, _ := EncodeFrame(OpFrame, map[string]string{"a": "b"})

	if _, _, err := DecodeFrame(bytes.NewReader(raw[:len(raw)-2])); err == nil {
		t.Fatal("expected error for truncated payload")
	}
	if _, _, err := DecodeFrame(bytes.NewReader(raw[:3])); err == nil {
		t.Fatal("expected error for truncated header")
	}
}
