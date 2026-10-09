package answer

import (
	"fmt"
	"time"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/consts"
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

	// The version list is built locally from BELFAST_DATA_DIR (see misc.localHashes).
	// It used to be replayed from a captured official SC_10801, or fetched by dialing
	// the official gateway; both made us depend on something outside this deployment.
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

// versionTrailers are the two literal entries the official SC_10801 appends after the
// fingerprint list. They are part of the contract, so they are added here - the hash
// source only produces the `$...hash$...` entries.
var versionTrailers = []string{"count-2", "dTag-1"}

func updateVersions(hashesFn func() misc.HashMap) []string {
	if len(versions) != 0 {
		return versions
	}
	hashes := hashesFn()
	for _, hash := range hashes {
		versions = append(versions, hash.Hash)
	}
	versions = append(versions, versionTrailers...)
	return versions
}
