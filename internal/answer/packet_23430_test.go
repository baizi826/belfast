package answer

import (
	"bytes"
	"testing"

	"github.com/ggmolly/belfast/internal/connection"
)

// The wire header is the only thing this handler builds itself, so pin it: a wrong
// length or command id makes the client drop the frame and retry CS_23430 forever.
func TestLegacy23430Framing(t *testing.T) {
	payload := []byte{0x08, 0x01, 0x10, 0x02}
	out := make([]byte, len(payload))
	copy(out, payload)

	connection.InjectPacketHeader(23431, &out, 7)

	if len(out) != 7+len(payload) {
		t.Fatalf("framed size = %d, want %d", len(out), 7+len(payload))
	}
	if got := int(out[0])<<8 | int(out[1]); got != 5+len(payload) {
		t.Errorf("header length field = %d, want %d", got, 5+len(payload))
	}
	if out[2] != 0 {
		t.Errorf("header flag = %d, want 0", out[2])
	}
	if got := int(out[3])<<8 | int(out[4]); got != 23431 {
		t.Errorf("header command = %d, want 23431", got)
	}
	if got := int(out[5])<<8 | int(out[6]); got != 7 {
		t.Errorf("header index = %d, want 7", got)
	}
	if !bytes.Equal(out[7:], payload) {
		t.Errorf("payload = %x, want %x", out[7:], payload)
	}
}
