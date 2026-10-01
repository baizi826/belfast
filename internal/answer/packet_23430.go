package answer

import (
	"fmt"
	"os"
	"sync"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/logger"
)

// CS_23430 has no protobuf definition anywhere in this repository - nothing in the
// 23000-23999 range is registered - so the reply is replayed byte for byte from a
// captured official SC_23431. The payload can only come from a real client session
// against an official gateway, which is why it lives in a data file and not in source.
//
// ponytail: static replay, not an implementation. Ceiling: the captured message
// carries per-account values frozen at capture time. Upgrade path: recover the
// SC_23431 message name and field semantics from the client, define it in
// internal/protobuf, and build the reply from orm instead of replaying.
const legacy23430ReplyPathEnv = "BELFAST_SC23431_FILE"

var (
	legacy23430Once  sync.Once
	legacy23430Bytes []byte
)

// loadLegacy23430Reply reads the captured SC_23431 payload once. An absent or
// unreadable file is not fatal: the reply degrades to an empty SC_23431, which the
// client parses as an all-defaults message.
func loadLegacy23430Reply() []byte {
	legacy23430Once.Do(func() {
		path := os.Getenv(legacy23430ReplyPathEnv)
		if path == "" {
			logger.LogEvent("Handler", "23430", fmt.Sprintf("%s is not set, replying with an empty SC_23431", legacy23430ReplyPathEnv), logger.LOG_LEVEL_WARN)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			logger.LogEvent("Handler", "23430", fmt.Sprintf("cannot read %s: %v, replying with an empty SC_23431", path, err), logger.LOG_LEVEL_ERROR)
			return
		}
		legacy23430Bytes = data
		logger.LogEvent("Handler", "23430", fmt.Sprintf("replaying %d bytes of captured SC_23431 from %s", len(data), path), logger.LOG_LEVEL_INFO)
	})
	return legacy23430Bytes
}

// HandleLegacy23430 answers CS_23430 with the captured SC_23431 payload. The client
// retries this command every few seconds while it goes unanswered, which blocks the
// rest of the login handshake.
func HandleLegacy23430(buffer *[]byte, client *connection.Client) (int, int, error) {
	out := make([]byte, len(loadLegacy23430Reply()))
	copy(out, loadLegacy23430Reply())
	connection.InjectPacketHeader(23431, &out, client.PacketIndex)
	if _, err := client.Buffer.Write(out); err != nil {
		return 0, 23431, err
	}
	return len(out), 23431, nil
}
