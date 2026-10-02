package playerops

import (
	"fmt"
	"time"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/orm"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// Reimplementation of SC_11000
func LastLogin(buffer *[]byte, client *connection.Client) (int, int, error) {
	// 客户端重连时会跳过 10022 握手直接发 11001 —— 此时 Commander 还是 nil。
	// 以前这里直接解引用 ⇒ 整个服务进程 panic 退出（不只是断这一条连接）。
	// 回错误交给 Dispatch，它会 CloseWithError 干净地断连，客户端重走完整登录。
	if client.Commander == nil {
		return 0, 11000, fmt.Errorf("commander not loaded (reconnect without the 10022 handshake?)")
	}
	now := time.Now().UTC()
	nowUnix := uint32(now.Unix())
	if _, err := orm.ApplyCommanderMoraleRecovery(client.Commander.CommanderID, nowUnix); err != nil {
		return 0, 11000, err
	}
	sc11000 := protobuf.SC_11000{
		Timestamp:               proto.Uint32(nowUnix),
		Monday_0OclockTimestamp: proto.Uint32(1606114800), // 23/11/2020 08:00:00
	}
	client.PreviousLoginAt = client.Commander.LastLogin
	if err := applyNavalAcademyLoginCatchup(client, now); err != nil {
		return 0, 11000, err
	}
	if err := client.Commander.Load(); err != nil {
		return 0, 11000, err
	}
	client.Commander.BumpLastLogin()
	logger.LogEvent("Server", "SC_11000", "Updated last login of uid="+fmt.Sprint(client.Commander.CommanderID), logger.LOG_LEVEL_INFO)
	return client.SendMessage(11000, &sc11000)
}
