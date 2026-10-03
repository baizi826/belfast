package answer

import (
	"fmt"
	"strconv"

	"github.com/ggmolly/belfast/internal/config"
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

func HandleGatewayAuthConfirm(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_10020
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 10021, err
	}
	if payload.GetLoginType() == 2 {
		return HandleLocalLogin(&payload, client)
	}
	if arg2, err := strconv.Atoi(payload.GetArg2()); err == nil {
		client.AuthArg2 = uint32(arg2)
	}
	// AccountId 必须真的解析出来：以前这里写死 0，客户端于是永远把自己当新号。
	// 但网关是**前门**：解析失败（库没接上、库报错）也不能让客户端连服务器列表都拿不到
	// —— 退化成 0（客户端重跑新手流程）并大声记日志，服务器列表照发。
	accountID, err := resolveAccountID(client, client.AuthArg2)
	if err != nil {
		logger.LogEvent("Gateway", "SC_10021", fmt.Sprintf("failed to resolve account for arg2 %d: %s (replying AccountId=0)", client.AuthArg2, err.Error()), logger.LOG_LEVEL_ERROR)
		accountID = 0
	}
	response := protobuf.SC_10021{
		Result:       proto.Uint32(0),
		AccountId:    proto.Uint32(accountID),
		ServerTicket: proto.String(formatServerTicket(client.AuthArg2)),
		Device:       proto.Uint32(0),
	}
	servers := config.Current().Servers
	statuses := getServerStatusCache(servers)
	Servers = buildServerInfo(servers, statuses)
	response.Serverlist = Servers
	logger.LogEvent("Server", "SC_10021", fmt.Sprintf("sending %d servers", len(response.Serverlist)), logger.LOG_LEVEL_WARN)
	return client.SendMessage(10021, &response)
}
