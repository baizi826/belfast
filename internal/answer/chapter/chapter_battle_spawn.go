package chapter

import (
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"github.com/ggmolly/belfast/internal/rng"
	"google.golang.org/protobuf/proto"
)

// chapterSpawnRand 刷怪用的随机源：官服的怪点位与敌人 id 都是随机的
// （抓包 2-4：初始两只在 (4,3)/(5,3)、id 204072/204082，都在候选点位/权重表里）。
var chapterSpawnRand = rng.NewLockedRand()

// chapterRandomSlots 从 n 个候选里等概率选 k 个互不重复的下标。
func chapterRandomSlots(n int, k int) map[uint32]bool {
	chosen := make(map[uint32]bool, k)
	if n <= 0 || k <= 0 {
		return chosen
	}
	if k >= n {
		for i := 0; i < n; i++ {
			chosen[uint32(i)] = true
		}
		return chosen
	}
	for len(chosen) < k {
		chosen[uint32(chapterSpawnRand.Uint32N(uint32(n)))] = true
	}
	return chosen
}

// shuffleChapterPos 官服式随机取法：先洗牌再取前几个。
func shuffleChapterPos(pool []chapterPos) []chapterPos {
	shuffled := append([]chapterPos(nil), pool...)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := int(chapterSpawnRand.Uint32N(uint32(i + 1)))
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled
}

// 刷怪表直接照抄 ALAS 的提取逻辑（dev_tools/map_extractor.py: parse_spawn_data）：
//
//	spawn_data[i] = {enemy: enemy_refresh[i] + elite_refresh[i],
//	                 mystery: box_refresh[i],
//	                 boss: 1 if i == boss_refresh}
//
// ALAS 的 campaign/campaign_main/campaign_2_4.py 就是这张表：
//
//	[{'battle':0,'enemy':2},{'battle':1,'enemy':2},{'battle':2,'enemy':1},{'battle':3,'enemy':2,'boss':1}]
//
// 与官服抓包完全一致：进图 2 只；第 1 杀补 2、第 2 杀补 1、第 3 杀补 2 并出 BOSS、第 4 杀无增援。
// 表用完后不再刷怪；BOSS 只在那一步出现一次，同时在场只有一个（点位在多个候选里随机，见 pickCell）。

type chapterSpawnStep struct {
	Enemies uint32
	Elites  uint32
	Sirens  uint32
	Boxes   uint32
	Boss    bool
}

// chapterSpawnTable 每张图的刷怪表，下标 = 战斗次数（0 = 进图），长度同 ALAS：
// max(boss_refresh, 各 refresh 列表长度)。
func chapterSpawnTable(template *chapterTemplate) []chapterSpawnStep {
	if template == nil {
		return nil
	}
	length := int(template.BossRefresh) + 1
	for _, list := range [][]uint32{template.EnemyRefresh, template.EliteRefresh, template.AiRefresh, template.BoxRefresh} {
		if len(list) > length {
			length = len(list)
		}
	}
	if length <= 0 {
		return nil
	}
	table := make([]chapterSpawnStep, length)
	fill := func(list []uint32, add func(*chapterSpawnStep, uint32)) {
		for index, count := range list {
			if index < length {
				add(&table[index], count)
			}
		}
	}
	fill(template.EnemyRefresh, func(step *chapterSpawnStep, count uint32) { step.Enemies += count })
	fill(template.EliteRefresh, func(step *chapterSpawnStep, count uint32) { step.Elites += count })
	fill(template.AiRefresh, func(step *chapterSpawnStep, count uint32) { step.Sirens += count })
	fill(template.BoxRefresh, func(step *chapterSpawnStep, count uint32) { step.Boxes += count })
	if int(template.BossRefresh) < length {
		table[template.BossRefresh].Boss = true
	}
	return table
}

// chapterSpawnStepForBattle 第 battle 次战斗后的刷怪步（0 = 进图）；表用完后零值 = 不再刷怪。
func chapterSpawnStepForBattle(template *chapterTemplate, battle uint32) chapterSpawnStep {
	table := chapterSpawnTable(template)
	if int(battle) >= len(table) {
		return chapterSpawnStep{}
	}
	return table[battle]
}

// chapterGridPool 模板里某个附件类型的可行走格（刷怪候选点位）。
func chapterGridPool(grids []chapterGrid, attachment uint32) []chapterPos {
	pool := make([]chapterPos, 0, len(grids))
	for _, grid := range grids {
		if grid.Attachment == attachment && grid.Walkable {
			pool = append(pool, chapterPos{Row: grid.Row, Column: grid.Column})
		}
	}
	return pool
}

// selectAttachmentForSlot 选一个远征 id。官服是随机的：小怪按 expedition_id_weight_list 的权重
// 随机（抓包 2-4 的 204072/204082 就在表里），精英/箱子从各自的表里随机。
func selectAttachmentForSlot(attachment uint32, template *chapterTemplate, slot uint32) uint32 {
	_ = slot
	if template == nil {
		return 0
	}
	switch attachment {
	case chapterAttachEnemy:
		return selectEnemyExpeditionRandom(template)
	case chapterAttachElite:
		return chapterRandomFromList(template.EliteExpeditions)
	case chapterAttachBox:
		return chapterRandomFromList(template.RandomBoxList)
	default:
		return selectAttachmentID(attachment, template)
	}
}

func chapterRandomFromList(values []uint32) uint32 {
	if len(values) == 0 {
		return 0
	}
	return values[chapterSpawnRand.Uint32N(uint32(len(values)))]
}

// selectEnemyExpeditionRandom 按 expedition_id_weight_list 的权重随机选一个敌人远征 id
// （每项 = [id, weight, tier]，与 ALAS/官服一致）。
func selectEnemyExpeditionRandom(template *chapterTemplate) uint32 {
	type candidate struct {
		id     uint32
		weight uint32
	}
	candidates := make([]candidate, 0, len(template.ExpeditionWeight))
	total := uint32(0)
	for _, entry := range template.ExpeditionWeight {
		if len(entry) < 2 {
			continue
		}
		id, err := parseUint32(entry[0])
		if err != nil || id == 0 {
			continue
		}
		weight, err := parseUint32(entry[1])
		if err != nil || weight == 0 {
			weight = 1
		}
		candidates = append(candidates, candidate{id: id, weight: weight})
		total += weight
	}
	if len(candidates) == 0 {
		return 0
	}
	roll := chapterSpawnRand.Uint32N(total)
	for _, item := range candidates {
		if roll < item.weight {
			return item.id
		}
		roll -= item.weight
	}
	return candidates[0].id
}

// spawnPoolCells 把 count 只敌人放到「未刷新 / 已击沉」的点位上：优先没刷过的点位，
// 其次复用已击沉的（官服抓包 2-4 里 7 个点位各用一次，用完表也结束了）。
func spawnPoolCells(current *protobuf.CURRENTCHAPTERINFO, grids []chapterGrid, template *chapterTemplate, attachment uint32, count uint32) []*protobuf.CHAPTERCELLINFO_P13 {
	if current == nil || count == 0 {
		return nil
	}
	pool := chapterGridPool(grids, attachment)
	if len(pool) == 0 {
		return nil
	}
	fresh := make([]chapterPos, 0, len(pool))
	reusable := make([]chapterPos, 0, len(pool))
	for _, pos := range pool {
		_, cell := findChapterCellAt(current, pos)
		if cell == nil || cell.GetItemType() != attachment {
			fresh = append(fresh, pos)
			continue
		}
		if cell.GetItemFlag() != chapterCellActive {
			reusable = append(reusable, pos)
		}
	}
	changed := make([]*protobuf.CHAPTERCELLINFO_P13, 0, count)
	// 官服先在「没刷过」的点位里随机取，不够了再从「已击沉」的点位里随机取（抓包 2-4 就是 7 个点位各用一次）。
	ordered := append(shuffleChapterPos(fresh), shuffleChapterPos(reusable)...)
	for _, pos := range ordered {
		if uint32(len(changed)) >= count {
			break
		}
		slot, ok := enemySlotIndex(pool, pos)
		if !ok {
			continue
		}
		cell := &protobuf.CHAPTERCELLINFO_P13{
			Pos:      buildPos(pos),
			ItemType: proto.Uint32(attachment),
			ItemId:   proto.Uint32(selectAttachmentForSlot(attachment, template, slot)),
			ItemFlag: proto.Uint32(chapterCellActive),
			ItemData: proto.Uint32(0),
		}
		upsertChapterCell(current, cell)
		changed = append(changed, cell)
	}
	return changed
}

// applyChapterSpawnStep 应用一步刷怪：小怪/精英/补给箱各按自己的候选点位补位，BOSS 单独处理。
func applyChapterSpawnStep(current *protobuf.CURRENTCHAPTERINFO, grids []chapterGrid, template *chapterTemplate, step chapterSpawnStep, bossPlan chapterBossPlan) []*protobuf.CHAPTERCELLINFO_P13 {
	changed := make([]*protobuf.CHAPTERCELLINFO_P13, 0, step.Enemies+step.Elites+1)
	changed = append(changed, spawnPoolCells(current, grids, template, chapterAttachEnemy, step.Enemies)...)
	changed = append(changed, spawnPoolCells(current, grids, template, chapterAttachElite, step.Elites)...)
	changed = append(changed, spawnPoolCells(current, grids, template, chapterAttachBox, step.Boxes)...)
	if step.Boss {
		if boss := applyChapterBoss(current, grids, template, bossPlan); boss != nil {
			changed = append(changed, boss)
		}
	}
	return changed
}

// chapterBattleExpedition 战斗会话里正在打的节点远征 id（= 被击破那个格的 item_id）。
func chapterBattleExpedition(client *connection.Client) uint32 {
	session, err := orm.GetBattleSession(client.Commander.CommanderID)
	if err != nil || session == nil {
		return 0
	}
	return session.StageID
}

// isChapterEnemyAttachment 与客户端 chapterconst.lua 的 AttachEnemyTypes 对齐：
// {AttachEnemy(6), AttachAmbush(5), AttachElite(4), AttachBoss(8), AttachAreaBoss(11),
//  AttachBomb_Enemy(24), AttachChampion(12)}；额外算上 AttachTorpedo_Enemy(7)。
func isChapterEnemyAttachment(attachment uint32) bool {
	switch attachment {
	case chapterAttachBoss,
		chapterAttachElite,
		chapterAttachAmbush,
		chapterAttachEnemy,
		chapterAttachTorpedoEnemy,
		chapterAttachChampion,
		chapterAttachAreaBoss,
		chapterAttachBombEnemy:
		return true
	default:
		return false
	}
}

// markChapterCellSunk 把刚被击破的怪格切成「击沉」态：保留 type/id，flag 置 disabled。
func markChapterCellSunk(current *protobuf.CURRENTCHAPTERINFO, expeditionID uint32) *protobuf.CHAPTERCELLINFO_P13 {
	if current == nil || expeditionID == 0 {
		return nil
	}
	for _, cell := range current.GetCellList() {
		if cell.GetItemId() != expeditionID || !isChapterEnemyAttachment(cell.GetItemType()) {
			continue
		}
		if cell.GetItemFlag() != chapterCellActive {
			continue
		}
		cell.ItemFlag = proto.Uint32(chapterCellDisabled)
		return cell
	}
	return nil
}

// chapterEnemyPool 可刷怪的点位（等价于 chapterGridPool(grids, chapterAttachEnemy)）。
func chapterEnemyPool(grids []chapterGrid) []chapterPos {
	return chapterGridPool(grids, chapterAttachEnemy)
}

// chapterWaveSizeForKills 本次击杀后应补多少只小怪：spawn_data[kills].enemy。
func chapterWaveSizeForKills(template *chapterTemplate, kills uint32) uint32 {
	return chapterSpawnStepForBattle(template, kills).Enemies
}

// applyChapterBoss 刷 BOSS：点位在模板的多个 boss 候选格里按 (章节+已击破数) 稳定取一个；
// 16 章多 BOSS 时按 chapter_auto_statistics 的序列换下一个，1-15 章击破一次后不再刷。
func applyChapterBoss(current *protobuf.CURRENTCHAPTERINFO, grids []chapterGrid, template *chapterTemplate, plan chapterBossPlan) *protobuf.CHAPTERCELLINFO_P13 {
	if current == nil || template == nil {
		return nil
	}
	var fallback uint32
	if len(template.BossExpeditionID) > 0 {
		fallback = template.BossExpeditionID[0]
	}
	bossID, available := plan.next(fallback)
	if !available || bossID == 0 {
		return nil
	}
	pos, ok := plan.pickCell(chapterGridPool(grids, chapterAttachBoss))
	if !ok {
		return nil
	}
	_, cell := findChapterCellAt(current, pos)
	if cell == nil {
		return nil
	}
	if cell.GetItemType() == chapterAttachBoss && cell.GetItemFlag() == chapterCellActive && cell.GetItemId() == bossID {
		return nil // 这个 BOSS 已经在场
	}
	// ponytail: 只在新点位放新 BOSS，不清理上一个点位（客户端已把它画成击沉残骸）；升级点是按官服清掉旧点位。
	cell.ItemType = proto.Uint32(chapterAttachBoss)
	cell.ItemId = proto.Uint32(bossID)
	cell.ItemFlag = proto.Uint32(chapterCellActive)
	return cell
}
