package onboarding

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ggmolly/belfast/internal/config"
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

const (
	createPlayerNameMin = 4
	createPlayerNameMax = 14
)

var starterShipIDs = map[uint32]struct{}{
	101171: {}, // Laffey
	201211: {}, // Javelin
	401231: {}, // Z23
}

func CreateNewPlayer(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_10024
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 10025, err
	}

	response := protobuf.SC_10025{
		Result: proto.Uint32(0),
		UserId: proto.Uint32(0),
	}

	nickname := payload.GetNickName()
	deviceID := payload.GetDeviceId()
	nameLength := utf8.RuneCountInString(nickname)
	if nameLength < createPlayerNameMin {
		response.Result = proto.Uint32(2012)
		return client.SendMessage(10025, &response)
	}
	if nameLength > createPlayerNameMax {
		response.Result = proto.Uint32(2011)
		return client.SendMessage(10025, &response)
	}

	createConfig := config.Current().CreatePlayer
	if len(createConfig.NameBlacklist) > 0 {
		lowerName := strings.ToLower(nickname)
		for _, blocked := range createConfig.NameBlacklist {
			blocked = strings.TrimSpace(blocked)
			if blocked == "" {
				continue
			}
			if strings.Contains(lowerName, strings.ToLower(blocked)) {
				response.Result = proto.Uint32(2013)
				return client.SendMessage(10025, &response)
			}
		}
	}
	if createConfig.NameIllegalPattern != "" {
		matcher, err := regexp.Compile(createConfig.NameIllegalPattern)
		if err != nil {
			return 0, 10025, err
		}
		if matcher.MatchString(nickname) {
			response.Result = proto.Uint32(2014)
			return client.SendMessage(10025, &response)
		}
	}

	// CN 9.7.10 客户端发来的初始舰 id 跟下面这张三条的白名单对不上（实测命中 result=1，
	// 客户端就显示「无效操作」，建号流程直接卡死）。客户端的可选初始舰本来就只有那几艘，
	// 服务端不该比客户端更严：认识的照旧，不认识的**记下 id 并放行**。
	// 拿到真实 id 后把 starterShipIDs 补全再收紧。
	shipID := payload.GetShipId()
	if _, ok := starterShipIDs[shipID]; !ok {
		logger.LogEvent("Server/SC_10025", "UnknownStarterShip",
			fmt.Sprintf("unknown starter ship id=%d nickname=%q - accepting anyway", shipID, nickname),
			logger.LOG_LEVEL_WARN)
	}

	// CN 客户端的 CS_10022/CS_10024 里 device_id 是空的（实测抓包 6: msg[0]），
	// 所以不能把 device_id 当成硬性要求 —— 没有它就跳过设备绑定，别拦登录。
	if deviceID != "" {
		// allow account binding across different connections using device id
		if deviceMapping, err := orm.GetDeviceAuthMapByDeviceID(deviceID); err == nil {
			if deviceMapping.AccountID != 0 {
				response.Result = proto.Uint32(1011)
				return client.SendMessage(10025, &response)
			}
			if client.AuthArg2 == 0 {
				client.AuthArg2 = deviceMapping.Arg2
			}
		} else if !errors.Is(err, db.ErrNotFound) {
			logger.LogEvent("Server", "SC_10025", fmt.Sprintf("failed to fetch device mapping: %s", err.Error()), logger.LOG_LEVEL_ERROR)
			response.Result = proto.Uint32(18)
			return client.SendMessage(10025, &response)
		}
	}

	if client.AuthArg2 == 0 {
		response.Result = proto.Uint32(1)
		return client.SendMessage(10025, &response)
	}

	// 账号已经建过了 ⇒ 不要把"再建一次"当成失败。客户端的本地记录（选择过的服务器、
	// 账号缓存）跟服务器对不上时会重跑整套序章+建号，此时若回 1011（已注册），玩家就
	// 永久卡在建号环节。按「你已经是这个账号了」回成功，幂等处理。
	if mapping, err := orm.GetYostarusMapByArg2(client.AuthArg2); err == nil {
		logger.LogEvent("Server/SC_10025", "AccountAlreadyExists",
			fmt.Sprintf("arg2=%d already mapped to account=%d - replying success", client.AuthArg2, mapping.AccountID),
			logger.LOG_LEVEL_WARN)
		response.UserId = proto.Uint32(mapping.AccountID)
		return client.SendMessage(10025, &response)
	} else if !errors.Is(err, db.ErrNotFound) {
		logger.LogEvent("Server", "SC_10025", fmt.Sprintf("failed to fetch account mapping: %s", err.Error()), logger.LOG_LEVEL_ERROR)
		response.Result = proto.Uint32(18)
		return client.SendMessage(10025, &response)
	}

	if err := orm.CheckCommanderNameAvailability(nickname); err != nil {
		if errors.Is(err, orm.ErrCommanderNameExists) {
			response.Result = proto.Uint32(2015)
			return client.SendMessage(10025, &response)
		}
		logger.LogEvent("Server", "SC_10025", fmt.Sprintf("failed to check commander name: %s", err.Error()), logger.LOG_LEVEL_ERROR)
		response.Result = proto.Uint32(18)
		return client.SendMessage(10025, &response)
	}

	accountID, err := client.CreateCommanderWithStarter(client.AuthArg2, nickname, shipID)
	if err != nil {
		logger.LogEvent("Server", "SC_10025", fmt.Sprintf("failed to create commander: %s", err.Error()), logger.LOG_LEVEL_ERROR)
		response.Result = proto.Uint32(18)
		return client.SendMessage(10025, &response)
	}
	if deviceID != "" {
		if err := orm.UpsertDeviceAuthMap(deviceID, client.AuthArg2, accountID); err != nil {
			logger.LogEvent("Server", "SC_10025", fmt.Sprintf("failed to save device mapping: %s", err.Error()), logger.LOG_LEVEL_ERROR)
		}
	}

	response.UserId = proto.Uint32(accountID)
	return client.SendMessage(10025, &response)
}
