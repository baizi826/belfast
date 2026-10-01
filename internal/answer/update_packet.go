package answer

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/consts"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/misc"
	"github.com/ggmolly/belfast/internal/region"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

var (
	platformMap = map[string]string{
		"0": "Android", // Possibly means 'Play Store'
		"1": "iOS",     // Possibly means 'App Store'
		// maybe more?
	}
	versions []string
)

// Answer to a CS_10800 packet with a SC_10801 packet + hashes
func HandleUpdateCheck(buffer *[]byte, client *connection.Client) (int, int, error) {
	return buildUpdateCheckResponse(buffer, client, misc.GetGameHashesWithUpdate)
}

func HandleGatewayUpdateCheck(buffer *[]byte, client *connection.Client) (int, int, error) {
	return buildUpdateCheckResponse(buffer, client, misc.GetGameHashes)
}

func buildUpdateCheckResponse(buffer *[]byte, client *connection.Client, hashesFn func() misc.HashMap) (int, int, error) {
	const packetId = 10801
	var updateCheck protobuf.CS_10800
	err := proto.Unmarshal(*buffer, &updateCheck)
	if err != nil {
		return 0, packetId, err
	}

	// BELFAST_SC10801_FILE：直接回放抓到的官服 SC_10801 应答，跳过下面“去官方网关拉哈希”的流程。
	//
	// 为什么需要：下面 getGameHashes() 会 Dial **真官方网关**（consts.RegionGateways[region]:80）
	// 拿**当前**版本的哈希（而且伪造请求 Platform="1"=iOS、CN 还把 State 改成 56），
	// 而客户端本地资源可能比官服旧（我们做过资源复用，本地是九游的 hashes*.csv）
	// ⇒ 客户端报「哈希校验失败」或要求下载。
	// 回放「官服当时对这台客户端的原话」版本天然一致。
	// 详情见 notes/2026-09-30.md 的 U/V 节。
	if override := strings.TrimSpace(os.Getenv("BELFAST_SC10801_FILE")); override != "" {
		raw, readErr := os.ReadFile(override)
		if readErr != nil {
			logger.LogEvent("GameData", "SC10801Override", "cannot read "+override+": "+readErr.Error(), logger.LOG_LEVEL_ERROR)
		} else {
			var captured protobuf.SC_10801
			if unmarshalErr := proto.Unmarshal(raw, &captured); unmarshalErr != nil {
				logger.LogEvent("GameData", "SC10801Override", "bad captured payload: "+unmarshalErr.Error(), logger.LOG_LEVEL_ERROR)
			} else {
				logger.LogEvent("GameData", "SC10801Override", "serving captured SC_10801 from "+override, logger.LOG_LEVEL_INFO)
				return client.SendMessage(packetId, &captured)
			}
		}
	}

	updateVersions(hashesFn)
	belfastRegion := region.Current()

	// It seems like the game kind of ignore anything but the versions, timestamp & Monday_0OclockTimestamp
	response := protobuf.SC_10801{
		GatewayIp:               proto.String(consts.RegionGateways[belfastRegion]),
		GatewayPort:             proto.Uint32(80),
		Url:                     proto.String(""),
		Version:                 versions,
		ProxyIp:                 proto.String(consts.RegionProxies[belfastRegion]),
		ProxyPort:               proto.Uint32(20000),
		IsTs:                    proto.Uint32(0),
		Timestamp:               proto.Uint32(uint32(time.Now().Unix())),
		Monday_0OclockTimestamp: proto.Uint32(consts.Monday_0OclockTimestamps[belfastRegion]),

		// wtf is this i don't even understand what monday_0oclock_timestamp is
		// who would even do such a thing
	}

	resolvedPlatform, ok := platformMap[updateCheck.GetPlatform()]
	if !ok {
		resolvedPlatform = "Unknown"
	}

	url, ok := consts.GamePlatformUrl[belfastRegion][updateCheck.GetPlatform()]
	if !ok {
		return 0, 10801, fmt.Errorf("unknown platform '%s' (id='%s')", resolvedPlatform, updateCheck.GetPlatform())
	} else {
		response.Url = proto.String(url)
	}

	return client.SendMessage(packetId, &response)
}

func updateVersions(hashesFn func() misc.HashMap) []string {
	if len(versions) != 0 {
		return versions
	}
	hashes := hashesFn()
	for _, hash := range hashes {
		versions = append(versions, hash.Hash)
	}
	versions = append(versions, "dTag-1")
	return versions
}
