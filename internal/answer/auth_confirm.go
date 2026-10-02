package answer

import (
	"fmt"
	"strconv"

	"github.com/ggmolly/belfast/internal/config"
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

var protoValidAnswer protobuf.SC_10021

func updateServerList(servers []config.ServerConfig) {
	statuses := getServerStatusCache(servers)
	Servers = buildServerInfo(servers, statuses)
	protoValidAnswer.Serverlist = Servers
}

// resolveAccountID 按 SDK uid（arg2）解析账号：先查 yostarus_maps，查不到且开了
// skip_onboarding 就现建一个，否则回 0（表示交给 CS_10024 建号）。
//
// ⚠ 游戏服（HandleAuthConfirm）与网关（HandleGatewayAuthConfirm）**必须走这一份**。
// 以前网关那份把 AccountId 写死 0，客户端于是永远把自己当新号：跑完序章去建号，
// 而建号又因为账号已存在回 1011 ⇒ 永久卡在「建立角色」界面。
func resolveAccountID(client *connection.Client, arg2 uint32) (uint32, error) {
	yostarusAuth, err := orm.GetYostarusMapByArg2(arg2)
	if err == nil {
		return yostarusAuth.AccountID, nil
	}
	if !db.IsNotFound(err) {
		return 0, err
	}
	if !config.Current().CreatePlayer.SkipOnboarding {
		return 0, nil // 0 = CS_10024 handles account creation.
	}
	// skip onboarding by creating the account on auth
	accountID, err := client.CreateCommander(arg2)
	if err != nil {
		return 0, err
	}
	return accountID, nil
}

func HandleAuthConfirm(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_10020
	err := proto.Unmarshal(*buffer, &payload)
	if err != nil {
		return 0, 10021, fmt.Errorf("failed to unmarshal payload: %s", err.Error())
	}
	if payload.GetLoginType() == 2 {
		return HandleLocalLogin(&payload, client)
	}

	intArg2, err := strconv.Atoi(payload.GetArg2())
	if err != nil {
		return 0, 10021, fmt.Errorf("failed to convert arg2 to int: %s", err.Error())
	}
	client.AuthArg2 = uint32(intArg2)
	protoValidAnswer.ServerTicket = proto.String(formatServerTicket(client.AuthArg2))

	accountID, err := resolveAccountID(client, client.AuthArg2)
	if err != nil {
		logger.LogEvent("Server", "SC_10021", fmt.Sprintf("failed to resolve account for arg2 %d: %s", client.AuthArg2, err.Error()), logger.LOG_LEVEL_ERROR)
		return 0, 10021, err
	}
	protoValidAnswer.AccountId = proto.Uint32(accountID)

	// Update server list
	updateServerList(config.Current().Servers)
	logger.LogEvent("Server", "SC_10021", fmt.Sprintf("sending %d servers", len(protoValidAnswer.Serverlist)), logger.LOG_LEVEL_WARN)
	return client.SendMessage(10021, &protoValidAnswer)
}

func init() {
	protoValidAnswer.ServerTicket = proto.String("=*=*=*=BELFAST=*=*=*=")
	protoValidAnswer.Serverlist = Servers
	protoValidAnswer.Result = proto.Uint32(0)
	protoValidAnswer.Device = proto.Uint32(0)
}
