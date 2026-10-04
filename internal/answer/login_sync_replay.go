package answer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/logger"
)

// PlayerLoginSync answers CS_11001 - the "sync my whole account" request.
//
// The state sync is a static snapshot, and this server rebuilds it from its own
// database, which only holds the fraction the importers wrote. The client then
// receives near-empty packets - SC_11003 332B vs 15418B captured, SC_11300 empty,
// SC_13001 38B vs 10697B, SC_19001 26B vs 19603B, SC_12201 empty - and its Lua
// managers (ShipFlagMgr, TechnologyNationProxy, ChapterProxy, Task, BuffHelper,
// SeriesGuideMgr) blow up on the nil fields, leaving it stuck on LOADING.
//
// When BELFAST_OFFICIAL_DIR points at a directory of `req<CS>_rep<SC>_<NN>.bin`
// files dumped from an official session (tools/bhx-mapdump.py --out), CS_11001 is
// answered with those captured payloads byte for byte: correct field semantics we
// have not reverse engineered, with no import needed. Everything the player does
// afterwards (chapters, mail, builds...) still goes through the real handlers, so
// picking 1-1 answers with 1-1's data instead of a replayed 2-1.
//
// ponytail: the snapshot is frozen at capture time. Ceiling: live interactions run
// against this server's own (thin) account. Upgrade path: port the fields the
// client actually reads into orm and delete this file.
const officialLoginDirEnv = "BELFAST_OFFICIAL_DIR"

// The built-in sync, used when no capture directory is configured.
var loginSyncChain = []func(*[]byte, *connection.Client) (int, int, error){
	LastLogin,
	PlayerInfo,
	PlayerBuffs,
	GetMetaProgress,
	LastOnlineInfo,
	ResourcesInfo,
	EventData,
	Meowfficers,
	CommanderCollection,
	OngoingBuilds,
	PlayerDock,
	CommanderDock,
	CommanderFleet,
	CommanderOwnedSkins,
	TechnologyRefreshList,
	ShipyardData,
	TechnologyNationProxy,
	CommanderStoryProgress,
	EventCollectionInfo,
	CommanderCommissionsFleet,
	ShopData,
	WorldBaseInfo,
	ChapterBaseSync,
	EquipedSpecialWeapons,
	EquippedWeaponSkin,
	OwnedItems,
	CommanderMissions,
	WeeklyMissions,
	ActivityTaskStateSync,
	DormData,
	FleetEnergyRecoverTime,
	GameMailbox,
	CompensateNotification,
	CommanderFriendList,
	Activities,
	PermanentActivites,
	GameNotices,
	SendPlayerShipCount,
}

func PlayerLoginSync(buffer *[]byte, client *connection.Client) (int, int, error) {
	if dir := os.Getenv(officialLoginDirEnv); dir != "" {
		sent, err := replayOfficialLogin(dir, client)
		if err == nil && sent > 0 {
			logger.LogEvent("Handler", "11001",
				fmt.Sprintf("replayed %d captured SC packets from %s (built-in sync skipped)", sent, dir),
				logger.LOG_LEVEL_INFO)
			return 0, 11001, nil
		}
		if err != nil {
			logger.LogEvent("Handler", "11001",
				fmt.Sprintf("%s=%s replay failed: %v - falling back to the built-in sync", officialLoginDirEnv, dir, err),
				logger.LOG_LEVEL_WARN)
		}
	}
	var written, packetId int
	var err error
	for _, handler := range loginSyncChain {
		written, packetId, err = handler(buffer, client)
		if err != nil {
			return written, packetId, err
		}
	}
	return written, packetId, nil
}

// replayOfficialLogin sends every captured `req11001_rep<SC>_<NN>.bin` in capture
// order (the trailing number is the index the official server sent it at).
func replayOfficialLogin(dir string, client *connection.Client) (int, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "req11001_rep*_*.bin"))
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		return 0, nil
	}
	type entry struct {
		idx  int
		cmd  int
		path string
	}
	entries := make([]entry, 0, len(paths))
	for _, p := range paths {
		base := strings.TrimSuffix(filepath.Base(p), ".bin")
		parts := strings.Split(strings.TrimPrefix(base, "req11001_rep"), "_")
		if len(parts) != 2 {
			continue
		}
		cmd, errCmd := strconv.Atoi(parts[0])
		idx, errIdx := strconv.Atoi(parts[1])
		if errCmd != nil || errIdx != nil {
			continue
		}
		entries = append(entries, entry{idx: idx, cmd: cmd, path: p})
	}
	if len(entries) == 0 {
		return 0, nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].idx < entries[j].idx })

	sent := 0
	for _, e := range entries {
		payload, err := os.ReadFile(e.path)
		if err != nil {
			return sent, err
		}
		// The frame length field is 16 bits; a bigger payload would be truncated
		// and desync the stream, so refuse it instead (same guard as SendProtoMessage).
		if len(payload)+5 > 0xFFFF {
			return sent, fmt.Errorf("%s: SC_%d payload %d bytes exceeds the 16-bit length field", e.path, e.cmd, len(payload))
		}
		out := make([]byte, len(payload))
		copy(out, payload)
		connection.InjectPacketHeader(e.cmd, &out, client.PacketIndex)
		if _, err := client.Buffer.Write(out); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
