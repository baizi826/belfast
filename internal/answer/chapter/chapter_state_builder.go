package chapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

const (
	chapterAttachBorn   = 1
	chapterAttachBox    = 2
	chapterAttachSupply = 3
	// chapterSupplyAmmoAmount 补给格带的弹药量：官服抓包 2-4 是 type=3 id=3（模板 ammo_total=5 ✗），
	// ALAS pick_up_ammo 也是「最多补 3」⇒ 固定 3。
	chapterSupplyAmmoAmount = 3
	chapterAttachBornSub    = 16
	// chapterStartTimeOffset: official sends start_time = time - 43200.
	// Verified on two captured samples (seq115: time=1790746481, startTime=1790703281 => delta exactly 43200).
	chapterStartTimeOffset    = 43200
	chapterAttachBoss         = 8
	chapterAttachElite        = 4
	chapterAttachAmbush       = 5
	chapterAttachEnemy        = 6
	chapterAttachTorpedoEnemy = 7
	chapterAttachChampion     = 12
	chapterAttachAreaBoss     = 11
	chapterAttachTransport    = 17
	chapterAttachTransportDst = 18
	chapterAttachBombEnemy    = 24
	chapterAttachLandbase     = 100
	chapterCellActive         = 0
	chapterCellDisabled       = 1
	chapterCellAmbush         = 2
	// chapterFlagBossPoint 官服 2-4 抓包在 BOSS 候选格上带的 flag（cell_flag_list → flag_list = [1]）。
	chapterFlagBossPoint = 1
)

type chapterGrid struct {
	Row        uint32
	Column     uint32
	Walkable   bool
	Attachment uint32
}

type chapterPos struct {
	Row    uint32
	Column uint32
}

func buildCurrentChapterInfo(template *chapterTemplate, payload *protobuf.CS_13101, operationBuffID uint32) (*protobuf.CURRENTCHAPTERINFO, uint32, error) {
	grids, err := parseChapterGrids(template.Grids)
	if err != nil {
		return nil, 0, err
	}
	mainSpawns := selectSpawnPositions(grids, chapterAttachBorn)
	subSpawns := selectSpawnPositions(grids, chapterAttachBornSub)
	cellList := buildChapterCells(grids, template)
	mainGroups, mainCount := buildGroupsFromTeams(payload.GetFleet().GetMainTeam(), mainSpawns, template.AmmoTotal)
	subGroups, subCount := buildGroupsFromTeams(payload.GetFleet().GetSubmarineTeam(), subSpawns, template.AmmoSubmarine)
	supportGroups, supportCount := buildGroupsFromTeams(payload.GetFleet().GetSupportTeam(), mainSpawns, template.AmmoTotal)
	escortList, err := buildEscortList(grids, template)
	if err != nil {
		return nil, 0, err
	}
	strategies := buildChapterStrategies(template.ChapterStrategy)
	initShipCount := mainCount + subCount + supportCount
	current := &protobuf.CURRENTCHAPTERINFO{
		Id:            proto.Uint32(payload.GetId()),
		Time:          proto.Uint32(uint32(time.Now().Unix()) + template.Time),
		CellList:      cellList,
		MainGroupList: mainGroups,
		AiList:        []*protobuf.CHAPTERCELLINFO_P13{},
		EscortList:    escortList,
		Round:         proto.Uint32(0),
		// 官服实测 is_submarine_auto_attack=1（audit §2 line41 / §3 line64）。
		IsSubmarineAutoAttack: proto.Uint32(1),
		OperationBuff:         buildOperationBuffList(operationBuffID),
		ModelActCount:         proto.Uint32(0),
		BuffList:              []uint32{},
		LoopFlag:              proto.Uint32(payload.GetLoopFlag()),
		ExtraFlagList:         []uint32{},
		CellFlagList:          buildChapterCellFlags(grids),
		ChapterHp:             proto.Uint32(0),
		ChapterStrategyList:   strategies,
		KillCount:             proto.Uint32(0),
		InitShipCount:         proto.Uint32(initShipCount),
		ContinuousKillCount:   proto.Uint32(0),
		// 9.7 新增 required：不进则 proto.Marshal 失败、连接被 reset。
		// 官服实测 start_time = time - 43200（两份样本：seq115 time=1790746481 / startTime=1790703281，差恰好 12h）。
		StartTime:          proto.Uint32(uint32(time.Now().Unix()) + template.Time - chapterStartTimeOffset),
		BattleStatistics:   []*protobuf.STRATEGYINFO_P13{},
		FleetDuties:        payload.GetFleetDuties(),
		MoveStepCount:      proto.Uint32(0),
		SubmarineGroupList: subGroups,
		SupportGroupList:   supportGroups,
	}
	return current, initShipCount, nil
}

func buildCurrentChapterInfoKR(template *chapterTemplate, payload *protobuf.CS_13101_KR, operationBuffID uint32) (*protobuf.CURRENTCHAPTERINFO, uint32, error) {
	grids, err := parseChapterGrids(template.Grids)
	if err != nil {
		return nil, 0, err
	}
	mainSpawns := selectSpawnPositions(grids, chapterAttachBorn)
	cellList := buildChapterCells(grids, template)
	mainGroups, mainCount := buildGroupsFromElite(payload.GetGroupIdList(), payload.GetEliteFleetList(), mainSpawns, template.AmmoTotal)
	strategies := buildChapterStrategies(template.ChapterStrategy)
	current := &protobuf.CURRENTCHAPTERINFO{
		Id:            proto.Uint32(payload.GetId()),
		Time:          proto.Uint32(uint32(time.Now().Unix()) + template.Time),
		CellList:      cellList,
		MainGroupList: mainGroups,
		AiList:        []*protobuf.CHAPTERCELLINFO_P13{},
		EscortList:    []*protobuf.CHAPTERCELLINFO_P13{},
		StartTime:     proto.Uint32(uint32(time.Now().Unix()) + template.Time - chapterStartTimeOffset),
		Round:         proto.Uint32(0),
		// 官服实测 is_submarine_auto_attack=1（KR 分支同源，保持一致）。
		IsSubmarineAutoAttack: proto.Uint32(1),
		OperationBuff:         buildOperationBuffList(operationBuffID),
		ModelActCount:         proto.Uint32(0),
		BuffList:              []uint32{},
		LoopFlag:              proto.Uint32(payload.GetLoopFlag()),
		ExtraFlagList:         []uint32{},
		CellFlagList:          buildChapterCellFlags(grids),
		ChapterHp:             proto.Uint32(0),
		ChapterStrategyList:   strategies,
		KillCount:             proto.Uint32(0),
		InitShipCount:         proto.Uint32(mainCount),
		ContinuousKillCount:   proto.Uint32(0),
		BattleStatistics:      []*protobuf.STRATEGYINFO_P13{},
		FleetDuties:           payload.GetFleetDuties(),
		MoveStepCount:         proto.Uint32(0),
		SubmarineGroupList:    []*protobuf.GROUPINCHAPTER_P13{},
		SupportGroupList:      []*protobuf.GROUPINCHAPTER_P13{},
	}
	return current, mainCount, nil
}

func buildOperationBuffList(buffID uint32) []uint32 {
	if buffID == 0 {
		return []uint32{}
	}
	return []uint32{buffID}
}

// buildChapterCellFlags 对应官服 SC_13102 的 cell_flag_list：
// 官服抓包 2-4 里带着 1 条 —— flag 落在 BOSS 候选格 (3,7) 上，flag_list = [1]（章节状态效果 1）。
// ⚠️ flag 1 的确切语义没在客户端里查实（weather_data_template 只认 101~103，
// chapter_status_effect[1] = {strategy: 90}），但既然官服就是在这个格子上发 1，
// 而且客户端只按天气/中毒/空袭来查询 flag，多带这一个 flag 不影响其它逻辑。
func buildChapterCellFlags(grids []chapterGrid) []*protobuf.CELLFLAG {
	flags := make([]*protobuf.CELLFLAG, 0, 1)
	for _, pos := range chapterGridPool(grids, chapterAttachBoss) {
		flags = append(flags, &protobuf.CELLFLAG{
			Pos:      buildPos(pos),
			FlagList: []uint32{chapterFlagBossPoint},
		})
	}
	return flags
}

func buildChapterStrategies(ids []uint32) []*protobuf.STRATEGYINFO_P13 {
	if len(ids) == 0 {
		return []*protobuf.STRATEGYINFO_P13{}
	}
	strategies := make([]*protobuf.STRATEGYINFO_P13, 0, len(ids))
	for _, id := range ids {
		strategies = append(strategies, &protobuf.STRATEGYINFO_P13{
			Id:    proto.Uint32(id),
			Count: proto.Uint32(0),
		})
	}
	return strategies
}

func buildGroupsFromTeams(teams []*protobuf.TEAM_INFO, spawns []chapterPos, ammo uint32) ([]*protobuf.GROUPINCHAPTER_P13, uint32) {
	groups := make([]*protobuf.GROUPINCHAPTER_P13, 0, len(teams))
	var shipCount uint32
	for index, team := range teams {
		spawn := chooseSpawn(spawns, index)
		ships := make([]*protobuf.SHIPINCHAPTER_P13, 0, len(team.GetShipList()))
		for _, shipID := range team.GetShipList() {
			ships = append(ships, &protobuf.SHIPINCHAPTER_P13{
				Id:     proto.Uint32(shipID),
				HpRant: proto.Uint32(10000),
			})
			shipCount++
		}
		commanders := buildCommanderList(team.GetCommanderMain(), team.GetCommanderSub())
		groupID := team.GetId()
		if groupID == 0 {
			groupID = uint32(index + 1)
		}
		groups = append(groups, &protobuf.GROUPINCHAPTER_P13{
			Id:               proto.Uint32(groupID),
			ShipList:         ships,
			Pos:              buildPos(spawn),
			StepCount:        proto.Uint32(0),
			BoxStrategyList:  []*protobuf.STRATEGYINFO_P13{},
			ShipStrategyList: []*protobuf.STRATEGYINFO_P13{},
			StrategyIds:      []uint32{},
			Bullet:           proto.Uint32(ammo),
			// 官服抓包 2-4：start_pos = (0,0)。服务器不填它（required 子消息，所以得给个空值）。
			StartPos:      buildPos(chapterPos{}),
			CommanderList: commanders,
			MoveStepDown:  proto.Uint32(0),
			KillCount:     proto.Uint32(0),
			FleetId:       proto.Uint32(groupID),
			VisionLv:      proto.Uint32(0),
		})
	}
	return groups, shipCount
}

func buildGroupsFromElite(groupIDs []uint32, elite []*protobuf.ELITEFLEETINFO, spawns []chapterPos, ammo uint32) ([]*protobuf.GROUPINCHAPTER_P13, uint32) {
	groups := make([]*protobuf.GROUPINCHAPTER_P13, 0, len(groupIDs))
	var shipCount uint32
	for index, groupID := range groupIDs {
		spawn := chooseSpawn(spawns, index)
		var eliteFleet *protobuf.ELITEFLEETINFO
		if index < len(elite) {
			eliteFleet = elite[index]
		}
		ships := []*protobuf.SHIPINCHAPTER_P13{}
		if eliteFleet != nil {
			ships = make([]*protobuf.SHIPINCHAPTER_P13, 0, len(eliteFleet.GetShipIdList()))
			for _, shipID := range eliteFleet.GetShipIdList() {
				ships = append(ships, &protobuf.SHIPINCHAPTER_P13{
					Id:     proto.Uint32(shipID),
					HpRant: proto.Uint32(10000),
				})
				shipCount++
			}
		}
		commanders := []*protobuf.COMMANDERSINFO{}
		if eliteFleet != nil {
			commanders = make([]*protobuf.COMMANDERSINFO, 0, len(eliteFleet.GetCommanders()))
			for _, commander := range eliteFleet.GetCommanders() {
				commanders = append(commanders, &protobuf.COMMANDERSINFO{
					Pos: proto.Uint32(commander.GetPos()),
					Id:  proto.Uint32(commander.GetId()),
				})
			}
		}
		if groupID == 0 {
			groupID = uint32(index + 1)
		}
		groups = append(groups, &protobuf.GROUPINCHAPTER_P13{
			Id:               proto.Uint32(groupID),
			ShipList:         ships,
			Pos:              buildPos(spawn),
			StepCount:        proto.Uint32(0),
			BoxStrategyList:  []*protobuf.STRATEGYINFO_P13{},
			ShipStrategyList: []*protobuf.STRATEGYINFO_P13{},
			StrategyIds:      []uint32{},
			Bullet:           proto.Uint32(ammo),
			StartPos:         buildPos(chapterPos{}),
			CommanderList:    commanders,
			MoveStepDown:     proto.Uint32(0),
			KillCount:        proto.Uint32(0),
			FleetId:          proto.Uint32(groupID),
			VisionLv:         proto.Uint32(0),
		})
	}
	return groups, shipCount
}

func buildCommanderList(mainID uint32, subID uint32) []*protobuf.COMMANDERSINFO {
	commanders := []*protobuf.COMMANDERSINFO{}
	if mainID != 0 {
		commanders = append(commanders, &protobuf.COMMANDERSINFO{
			Pos: proto.Uint32(1),
			Id:  proto.Uint32(mainID),
		})
	}
	if subID != 0 {
		commanders = append(commanders, &protobuf.COMMANDERSINFO{
			Pos: proto.Uint32(2),
			Id:  proto.Uint32(subID),
		})
	}
	return commanders
}

func buildChapterCells(grids []chapterGrid, template *chapterTemplate) []*protobuf.CHAPTERCELLINFO_P13 {
	return buildChapterCellsAtProgress(grids, template, 0)
}

// buildChapterCellsAtProgress 按官服刷怪节奏构建格子表。官服对照（抓包 2-4）：
//   - 只下发可行走格（含空格 item_type=0；模板 28 格中恰好下发 22 格，与官服一致）；
//   - 怪点位按 enemy_refresh 波次刷新，进图只出现第一波（2-4 = 2 只），清完一波补下一波；
//   - BOSS 在累计击杀达到 boss_refresh（官服 2-4 = 第 3 杀时出现 BOSS 节点）后出现；
//     9 章之后 progress_boss < 100，要反复击破 BOSS，但**同一时刻只亮一个 BOSS 节点**，
//     其余 boss 格是「BOSS 重刷位置」，先当空格下发。
func buildChapterCellsAtProgress(grids []chapterGrid, template *chapterTemplate, kills uint32) []*protobuf.CHAPTERCELLINFO_P13 {
	return buildChapterCellsWithBossPlan(grids, template, kills, chapterBossPlan{})
}

// chapterBossPlan BOSS 刷怪计划。
// 1-15 章：整张图只有 1 个 BOSS，击破即整图结算（同级 boss 格是「随机刷新点位」的候选）。
// 16 章：多 BOSS —— 按 boss_expedition_id 序列一个个刷，每个可在随机点位出现。
// Phase = 本局已击破的 BOSS 数（orm.ChapterProgress.KillBossCount）。
type chapterBossPlan struct {
	ChapterID  uint32
	IDs        []uint32
	Phase      uint32
	MultiPhase bool
}

// next 返回当前应刷的 BOSS 远征 id；ok=false 表示这张图的 BOSS 已全部击破。
func (plan chapterBossPlan) next(fallback uint32) (uint32, bool) {
	limit := uint32(1)
	if plan.MultiPhase {
		if len(plan.IDs) == 0 {
			return fallback, true
		}
		limit = uint32(len(plan.IDs))
	}
	if plan.Phase >= limit {
		return 0, false
	}
	if len(plan.IDs) == 0 {
		return fallback, true
	}
	return plan.IDs[plan.Phase], true
}

// pickCell 官服 BOSS 可在多个候选点位中随机一个刷新；这里用 (章节+已击破数) 做稳定取向，
// 保证同一阶段重建/重进地图时 BOSS 不会乱跳。
func (plan chapterBossPlan) pickCell(cells []chapterPos) (chapterPos, bool) {
	if len(cells) == 0 {
		return chapterPos{}, false
	}
	index := (uint32(plan.ChapterID) + plan.Phase) % uint32(len(cells))
	return cells[index], true
}

func buildChapterCellsWithBossPlan(grids []chapterGrid, template *chapterTemplate, kills uint32, plan chapterBossPlan) []*protobuf.CHAPTERCELLINFO_P13 {
	if len(grids) == 0 {
		return []*protobuf.CHAPTERCELLINFO_P13{}
	}
	step := chapterSpawnStepForBattle(template, kills)
	enemyPool := chapterGridPool(grids, chapterAttachEnemy)
	elitePool := chapterGridPool(grids, chapterAttachElite)
	boxPool := chapterGridPool(grids, chapterAttachBox)
	// 官服的怪点位是随机选的（抓包 2-4：进图两只在 (4,3)/(5,3)，不是点位表前两个）。
	enemySlots := chapterRandomSlots(len(enemyPool), int(step.Enemies))
	eliteSlots := chapterRandomSlots(len(elitePool), int(step.Elites))
	boxSlots := chapterRandomSlots(len(boxPool), int(step.Boxes))
	var fallbackBossID uint32
	if template != nil && len(template.BossExpeditionID) > 0 {
		fallbackBossID = template.BossExpeditionID[0]
	}
	bossID, bossAvailable := plan.next(fallbackBossID)
	bossPos, bossFound := plan.pickCell(chapterGridPool(grids, chapterAttachBoss))
	bossReady := bossFound && bossAvailable && step.Boss
	cells := make([]*protobuf.CHAPTERCELLINFO_P13, 0, len(grids))
	for _, grid := range grids {
		if !grid.Walkable {
			continue
		}
		pos := chapterPos{Row: grid.Row, Column: grid.Column}
		cell := &protobuf.CHAPTERCELLINFO_P13{
			Pos:      buildPos(pos),
			ItemType: proto.Uint32(grid.Attachment),
			ItemFlag: proto.Uint32(resolveCellFlag(grid.Attachment)),
			ItemData: proto.Uint32(0),
		}
		if grid.Attachment == chapterAttachEnemy {
			// 未刷出的怪点位 = 空格；刷出来的（按 ALAS spawn_data 表）才变成 type=6。
			if slot, ok := enemySlotIndex(enemyPool, pos); !ok || !enemySlots[slot] {
				cell.ItemType = proto.Uint32(0)
			} else if expeditionID := selectAttachmentForSlot(chapterAttachEnemy, template, slot); expeditionID != 0 {
				cell.ItemId = proto.Uint32(expeditionID)
			}
		} else if grid.Attachment == chapterAttachElite && template != nil {
			if slot, ok := enemySlotIndex(elitePool, pos); !ok || !eliteSlots[slot] {
				cell.ItemType = proto.Uint32(0)
			} else if eliteID := selectAttachmentForSlot(chapterAttachElite, template, slot); eliteID != 0 {
				cell.ItemId = proto.Uint32(eliteID)
			}
		} else if grid.Attachment == chapterAttachBoss {
			if !bossReady || pos != bossPos {
				cell.ItemType = proto.Uint32(0)
			} else if bossID != 0 {
				cell.ItemId = proto.Uint32(bossID)
			}
		} else if grid.Attachment == chapterAttachBorn || grid.Attachment == chapterAttachBornSub {
			// 官服抓包（2-4）22 格里没有 type=1：出生点当空格下发（点位由舰队 group 的 pos 表达）。
			cell.ItemType = proto.Uint32(0)
		} else if grid.Attachment == chapterAttachBox && template != nil {
			if slot, ok := enemySlotIndex(boxPool, pos); !ok || !boxSlots[slot] {
				cell.ItemType = proto.Uint32(0)
			} else if boxID := selectAttachmentForSlot(chapterAttachBox, template, slot); boxID != 0 {
				cell.ItemId = proto.Uint32(boxID)
			}
		} else if grid.Attachment == chapterAttachSupply && template != nil {
			cell.ItemId = proto.Uint32(selectSupplyAttachmentAmount(template))
		} else if grid.Attachment == chapterAttachLandbase && template != nil {
			if attachmentID := selectPositionAttachmentID(pos, template.LandBased); attachmentID != 0 {
				cell.ItemId = proto.Uint32(attachmentID)
			}
		} else if template != nil {
			attachmentID := selectAttachmentID(grid.Attachment, template)
			if attachmentID != 0 {
				cell.ItemId = proto.Uint32(attachmentID)
			}
		}
		cells = append(cells, cell)
	}
	return cells
}

// chapterEnemyWaves 描述官服刷怪节奏：enemy_refresh 为每波刷出的敌人数（列表循环使用），
// 累计击杀达到 num_2（★2 击破数，例：2-4 = 12）后 BOSS 出现。
type chapterEnemyWaves struct {
	waves  []uint32
	target uint32
}

// chapterBossThreshold 官服 boss_refresh = 「累计击破多少队后 BOSS 节点出现」。
// 抓包验证：2-4 模板 boss_refresh=3，官服第 3 杀时推送 type=8 的 BOSS 格。
func chapterBossThreshold(template *chapterTemplate) uint32 {
	if template == nil {
		return 0
	}
	if template.BossRefresh > 0 {
		return template.BossRefresh
	}
	return template.Num2
}

func chapterWavesOf(template *chapterTemplate) chapterEnemyWaves {
	result := chapterEnemyWaves{}
	if template != nil {
		result.waves = append(result.waves, template.EnemyRefresh...)
		result.target = template.Num2
	}
	if len(result.waves) == 0 {
		result.waves = []uint32{1}
	}
	if result.target == 0 {
		result.target = result.waves[0]
	}
	return result
}

// summary 由累计击杀数推导刷怪状态：已累计刷出数、当前存活怪占用的池下标、BOSS 是否出现。
// 规则：一波全部被击破后补下一波（数量取 enemy_refresh 循环），刷出总数不超过 target。
func (waves chapterEnemyWaves) summary(kills uint32, poolSize uint32) (spawned uint32, alive map[uint32]bool, boss bool) {
	alive = map[uint32]bool{}
	if poolSize == 0 {
		return 0, alive, false
	}
	spawned = waves.waves[0]
	if spawned > waves.target {
		spawned = waves.target
	}
	var lastSpawned uint32
	index := 0
	for kills >= spawned && spawned < waves.target {
		next := waves.waves[(index+1)%len(waves.waves)]
		if spawned+next > waves.target {
			next = waves.target - spawned
		}
		if next == 0 {
			break
		}
		index++
		lastSpawned = spawned
		spawned += next
	}
	deaths := kills - lastSpawned
	for slot := lastSpawned; slot < spawned; slot++ {
		if slot-lastSpawned < deaths {
			continue
		}
		alive[slot%poolSize] = true
	}
	return spawned, alive, kills >= waves.target
}

func enemySlotIndex(pool []chapterPos, pos chapterPos) (uint32, bool) {
	for index, candidate := range pool {
		if candidate == pos {
			return uint32(index), true
		}
	}
	return 0, false
}

// selectExpeditionForSlot 给怪点位固定一个远征 ID（模板权重表按池下标取），
// 保证同一地图重建、重进时怪 ID 稳定。
func selectExpeditionForSlot(template *chapterTemplate, slot uint32) uint32 {
	if template == nil || len(template.ExpeditionWeight) == 0 {
		return 0
	}
	entry := template.ExpeditionWeight[int(slot)%len(template.ExpeditionWeight)]
	if len(entry) == 0 {
		return 0
	}
	id, err := parseUint32(entry[0])
	if err != nil {
		return 0
	}
	return id
}

// diffChapterCells 找出两张格子表中内容变化的格（官服 13105 只推变化的格）。
func diffChapterCells(before []*protobuf.CHAPTERCELLINFO_P13, after []*protobuf.CHAPTERCELLINFO_P13) []*protobuf.CHAPTERCELLINFO_P13 {
	index := make(map[chapterCellKey]*protobuf.CHAPTERCELLINFO_P13, len(before))
	for _, cell := range before {
		index[chapterCellPositionKey(cell)] = cell
	}
	changed := make([]*protobuf.CHAPTERCELLINFO_P13, 0, len(after))
	for _, cell := range after {
		if old, ok := index[chapterCellPositionKey(cell)]; ok &&
			old.GetItemType() == cell.GetItemType() &&
			old.GetItemId() == cell.GetItemId() &&
			old.GetItemFlag() == cell.GetItemFlag() {
			continue
		}
		changed = append(changed, cell)
	}
	return changed
}

func chapterCellPositionKey(cell *protobuf.CHAPTERCELLINFO_P13) chapterCellKey {
	return chapterCellKey{Row: cell.GetPos().GetRow(), Column: cell.GetPos().GetColumn()}
}

func resolveCellFlag(attachment uint32) uint32 {
	if attachment == chapterAttachAmbush {
		return chapterCellAmbush
	}
	return chapterCellActive
}

func selectAttachmentID(attachment uint32, template *chapterTemplate) uint32 {
	switch attachment {
	case chapterAttachBox:
		return selectFirst(template.RandomBoxList)
	case chapterAttachSupply:
		return selectSupplyAttachmentAmount(template)
	case chapterAttachBoss:
		return 0
	case chapterAttachEnemy:
		return selectExpeditionFromWeights(template.ExpeditionWeight)
	case chapterAttachElite:
		return selectFirst(template.EliteExpeditions)
	case chapterAttachAmbush:
		if id := selectFirst(template.AmbushExpeditions); id != 0 {
			return id
		}
		return selectExpeditionFromWeights(template.ExpeditionWeight)
	case chapterAttachChampion, chapterAttachBombEnemy, chapterAttachTorpedoEnemy:
		if id := selectFirst(template.GuarderExpeditions); id != 0 {
			return id
		}
		return selectExpeditionFromWeights(template.ExpeditionWeight)
	default:
		return 0
	}
}

func selectSupplyAttachmentAmount(template *chapterTemplate) uint32 {
	// 官服按固定 3 发（见 chapterSupplyAmmoAmount 注释），不用 ammo_total（那是舰队弹量上限）。
	return chapterSupplyAmmoAmount
}

func selectBoxAttachmentID(pos chapterPos, template *chapterTemplate) uint32 {
	if ids := selectPositionAttachmentIDs(pos, template.BoxList); len(ids) > 0 {
		return ids[0]
	}
	return selectFirst(template.RandomBoxList)
}

func selectPositionAttachmentID(pos chapterPos, entries [][]any) uint32 {
	ids := selectPositionAttachmentIDs(pos, entries)
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func selectPositionAttachmentIDs(pos chapterPos, entries [][]any) []uint32 {
	for _, entry := range entries {
		if len(entry) < 3 {
			continue
		}
		row, err := parseUint32(entry[0])
		if err != nil || row != pos.Row {
			continue
		}
		column, err := parseUint32(entry[1])
		if err != nil || column != pos.Column {
			continue
		}
		return parseUint32List(entry[2])
	}
	return nil
}

func selectExpeditionFromWeights(weights [][]any) uint32 {
	for _, entry := range weights {
		if len(entry) == 0 {
			continue
		}
		id, err := parseUint32(entry[0])
		if err == nil && id != 0 {
			return id
		}
	}
	return 0
}

func selectFirst(values []uint32) uint32 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func selectSpawnPositions(grids []chapterGrid, attachment uint32) []chapterPos {
	positions := []chapterPos{}
	for _, grid := range grids {
		if grid.Attachment == attachment {
			positions = append(positions, chapterPos{Row: grid.Row, Column: grid.Column})
		}
	}
	if len(positions) == 0 && len(grids) > 0 {
		positions = append(positions, chapterPos{Row: grids[0].Row, Column: grids[0].Column})
	}
	return positions
}

func chooseSpawn(spawns []chapterPos, index int) chapterPos {
	if len(spawns) == 0 {
		return chapterPos{Row: 1, Column: 1}
	}
	if index < len(spawns) {
		return spawns[index]
	}
	return spawns[0]
}

func buildPos(pos chapterPos) *protobuf.CHAPTERCELLPOS_P13 {
	return &protobuf.CHAPTERCELLPOS_P13{
		Row:    proto.Uint32(pos.Row),
		Column: proto.Uint32(pos.Column),
	}
}

func parseChapterGrids(raw [][]any) ([]chapterGrid, error) {
	grids := make([]chapterGrid, 0, len(raw))
	for _, entry := range raw {
		if len(entry) < 4 {
			return nil, fmt.Errorf("invalid grid entry")
		}
		row, err := parseUint32(entry[0])
		if err != nil {
			return nil, err
		}
		column, err := parseUint32(entry[1])
		if err != nil {
			return nil, err
		}
		walkable, err := parseBool(entry[2])
		if err != nil {
			return nil, err
		}
		attachment, err := parseUint32(entry[3])
		if err != nil {
			return nil, err
		}
		grids = append(grids, chapterGrid{
			Row:        row,
			Column:     column,
			Walkable:   walkable,
			Attachment: attachment,
		})
	}
	return grids, nil
}

func parseUint32(value any) (uint32, error) {
	switch typed := value.(type) {
	case float64:
		return uint32(typed), nil
	case int:
		return uint32(typed), nil
	case int64:
		return uint32(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, err
		}
		return uint32(parsed), nil
	default:
		return 0, fmt.Errorf("unsupported number")
	}
}

func parseBool(value any) (bool, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case float64:
		return typed != 0, nil
	case int:
		return typed != 0, nil
	case int64:
		return typed != 0, nil
	default:
		return false, fmt.Errorf("unsupported bool")
	}
}

func parseUint32List(value any) []uint32 {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	parsed := make([]uint32, 0, len(values))
	for _, entry := range values {
		id, err := parseUint32(entry)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}
	return parsed
}

func buildEscortList(grids []chapterGrid, template *chapterTemplate) ([]*protobuf.CHAPTERCELLINFO_P13, error) {
	if template == nil {
		return []*protobuf.CHAPTERCELLINFO_P13{}, nil
	}
	if template.FriendlyID == 0 {
		return []*protobuf.CHAPTERCELLINFO_P13{}, nil
	}
	friendly, err := loadFriendlyData(template.FriendlyID)
	if err != nil {
		return nil, err
	}
	hp := uint32(0)
	if friendly != nil {
		hp = friendly.HP
	}
	escorts := []*protobuf.CHAPTERCELLINFO_P13{}
	for _, grid := range grids {
		if grid.Attachment != chapterAttachTransport {
			continue
		}
		escorts = append(escorts, &protobuf.CHAPTERCELLINFO_P13{
			Pos:      buildPos(chapterPos{Row: grid.Row, Column: grid.Column}),
			ItemType: proto.Uint32(chapterAttachTransport),
			ItemId:   proto.Uint32(template.FriendlyID),
			ItemFlag: proto.Uint32(chapterCellActive),
			ItemData: proto.Uint32(hp),
		})
	}
	return escorts, nil
}
