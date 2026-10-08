package answer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
)

const (
	worldResultSuccess                 = uint32(0)
	worldResultFailed                  = uint32(1)
	worldResultUnsupported             = uint32(2)
	worldResultTaskRefused             = uint32(6)
	worldResultActionPowerInsufficient = uint32(130)

	worldChapterRandomCategory   = "ShareCfg/world_chapter_random.json"
	worldChapterRandomCategoryLC = "sharecfgdata/world_chapter_random.json"
	worldChapterTemplateCategory = "ShareCfg/world_chapter_template.json"
	worldTaskDataCategory        = "ShareCfg/world_task_data.json"
	worldTaskDataCategoryLC      = "sharecfgdata/world_task_data.json"
	worldGamesetCategory         = "ShareCfg/gameset.json"
	worldGamesetCategoryLC       = "sharecfgdata/gameset.json"
)

var (
	errWorldTaskRefused = errors.New("world task refused")
	errWorldTaskInvalid = errors.New("world task invalid")
)

type worldChapterRandomConfig struct {
	ID         uint32 `json:"id"`
	TemplateID uint32 `json:"template_id"`
	Map        uint32 `json:"map"`
	MapID      uint32 `json:"map_id"`
	ChapterID  uint32 `json:"chapter_id"`
}

type worldTaskConfig struct {
	ID                 uint32          `json:"id"`
	NeedLevel          uint32          `json:"need_level"`
	NeedTaskComplete   uint32          `json:"need_task_complete"`
	CompleteCondition  uint32          `json:"complete_condition"`
	CompleteParameter  json.RawMessage `json:"complete_parameter"`
	CompleteStage      uint32          `json:"complete_stage"`
	CompleteTargetNum  uint32          `json:"complete_parameter_num"`
	ItemRetrieve       uint32          `json:"item_retrieve"`
	Drop               json.RawMessage `json:"drop"`
	Show               json.RawMessage `json:"show"`
	Exp                uint32          `json:"exp"`
	Intimacy           uint32          `json:"intimacy"`
	EventMapID         uint32          `json:"event_map_id"`
	AutoComplete       uint32          `json:"auto_complete"`
	TaskOperationStory uint32          `json:"task_op"`
}

func WorldActivate(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33101
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33102, err
	}

	response := protobuf.SC_33102{
		Result:    proto.Uint32(worldResultFailed),
		CountInfo: emptyWorldCountInfo(),
	}
	if payload.Id == nil || payload.EnterMapId == nil || payload.Camp == nil {
		return client.SendMessage(33102, &response)
	}
	if payload.GetId() == 0 || payload.GetEnterMapId() == 0 {
		return client.SendMessage(33102, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33102, err
	}
	now := uint32(time.Now().Unix())
	runtime.Camp = payload.GetCamp()
	runtime.MapID = payload.GetId()
	runtime.EnterMapID = payload.GetEnterMapId()
	runtime.LastChangeGroupTimestamp = now
	runtime.LastRecoverTimestamp = now
	runtime.FleetShipIDs = flattenEliteFleetShipIDs(payload.GetEliteFleetList())
	runtime.CommanderIDs = flattenEliteFleetCommanderIDs(payload.GetEliteFleetList())

	templateID, err := resolveWorldMapTemplateID(payload.GetId())
	if err != nil {
		return 0, 33102, err
	}
	runtime.SetMapTemplate(payload.GetId(), templateID)

	if err := orm.SaveWorldRuntime(runtime); err != nil {
		return 0, 33102, err
	}

	response.Result = proto.Uint32(worldResultSuccess)
	response.World = buildWorldInfo(runtime, payload.GetEliteFleetList())
	response.CountInfo = buildWorldCountInfo(runtime)
	response.ChapterAward = []*protobuf.CHAPTERAWARDINFO{}
	response.PortList = []uint32{}
	response.NewFlagPortList = []uint32{}
	return client.SendMessage(33102, &response)
}

func WorldMapOperation(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33103
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33104, err
	}

	response := protobuf.SC_33104{
		Result:            proto.Uint32(worldResultFailed),
		EventId:           proto.Uint32(0),
		MovePath:          []*protobuf.CHAPTERCELLPOS_P33{},
		DropList:          []*protobuf.DROPINFO{},
		ShipUpdate:        []*protobuf.SHIPINCHAPTER_P33{},
		AiActList:         []*protobuf.AI_ACT_P33{},
		LandList:          []*protobuf.LANDINFO{},
		GroupUpdate:       []*protobuf.GROUPINFOUPDATE{},
		PosList:           []*protobuf.WORLDPOSINFO{},
		TargetList:        []*protobuf.WORLDTARGET{},
		CmdCollectionList: []*protobuf.GROUPCMDCOLLECTION{},
	}
	if payload.Act == nil || payload.GroupId == nil {
		return client.SendMessage(33104, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33104, err
	}
	if changed, _, err := orm.SyncWorldRuntime(runtime, time.Now().UTC()); err != nil {
		return 0, 33104, err
	} else if changed {
		if err := orm.SaveWorldRuntime(runtime); err != nil {
			return 0, 33104, err
		}
	}

	result := worldResultFailed
	saveRuntime := false
	act := payload.GetAct()
	switch act {
	case 1:
		if len(payload.GetPosList()) == 0 {
			break
		}
		if runtime.ActionPower == 0 {
			result = worldResultActionPowerInsufficient
			break
		}
		runtime.ActionPower--
		response.MovePath = cloneChapterCellPositions(payload.GetPosList())
		response.ActionPower = proto.Uint32(runtime.ActionPower)
		response.ActionPowerExtra = proto.Uint32(runtime.ActionPowerExtra)
		result = worldResultSuccess
		saveRuntime = true
	case 8:
		runtime.Round++
		result = worldResultSuccess
		saveRuntime = true
	case 10, 14:
		if runtime.ActionPower == 0 {
			result = worldResultActionPowerInsufficient
			break
		}
		runtime.ActionPower--
		response.ActionPower = proto.Uint32(runtime.ActionPower)
		response.ActionPowerExtra = proto.Uint32(runtime.ActionPowerExtra)
		if payload.GetActArg_1() > 0 {
			templateID := runtime.MapTemplate(payload.GetActArg_1())
			if templateID == 0 {
				templateID, err = resolveWorldMapTemplateID(payload.GetActArg_1())
				if err != nil {
					return 0, 33104, err
				}
				runtime.SetMapTemplate(payload.GetActArg_1(), templateID)
			}
			response.EnterMapId = proto.Uint32(payload.GetActArg_1())
			response.Id = &protobuf.WORLDMAPID{RandomId: proto.Uint32(payload.GetActArg_1()), TemplateId: proto.Uint32(templateID)}
			runtime.MapID = payload.GetActArg_1()
			runtime.EnterMapID = payload.GetActArg_1()
		}
		result = worldResultSuccess
		saveRuntime = true
	case 13, 26:
		targetMapID := payload.GetActArg_1()
		if targetMapID == 0 {
			targetMapID = runtime.MapID
		}
		if targetMapID == 0 {
			break
		}
		templateID := runtime.MapTemplate(targetMapID)
		if templateID == 0 {
			templateID, err = resolveWorldMapTemplateID(targetMapID)
			if err != nil {
				return 0, 33104, err
			}
			runtime.SetMapTemplate(targetMapID, templateID)
		}
		runtime.MapID = targetMapID
		runtime.EnterMapID = targetMapID
		response.EnterMapId = proto.Uint32(targetMapID)
		response.Id = &protobuf.WORLDMAPID{RandomId: proto.Uint32(targetMapID), TemplateId: proto.Uint32(templateID)}
		response.ActionPower = proto.Uint32(runtime.ActionPower)
		response.ActionPowerExtra = proto.Uint32(runtime.ActionPowerExtra)
		result = worldResultSuccess
		saveRuntime = true
	default:
		result = worldResultUnsupported
	}

	response.Result = proto.Uint32(result)
	if saveRuntime {
		if err := orm.SaveWorldRuntime(runtime); err != nil {
			return 0, 33104, err
		}
	}
	return client.SendMessage(33104, &response)
}

func WorldMapRequest(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33106
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33107, err
	}

	response := protobuf.SC_33107{Result: proto.Uint32(worldResultFailed)}
	if payload.Id == nil || payload.GetId() == 0 {
		return client.SendMessage(33107, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33107, err
	}
	changed, monthReset, err := orm.SyncWorldRuntime(runtime, time.Now().UTC())
	if err != nil {
		return 0, 33107, err
	}
	templateKnown := runtime.MapTemplate(payload.GetId()) != 0
	templateID := runtime.MapTemplate(payload.GetId())
	if templateID == 0 {
		templateID, err = resolveWorldMapTemplateID(payload.GetId())
		if err != nil {
			return 0, 33107, err
		}
		runtime.SetMapTemplate(payload.GetId(), templateID)
		changed = true
	}
	if templateID == 0 {
		return client.SendMessage(33107, &response)
	}

	response.Result = proto.Uint32(worldResultSuccess)
	if monthReset {
		response.IsReset = proto.Uint32(1)
	} else {
		response.IsReset = proto.Uint32(0)
	}
	template, err := loadWorldChapterTemplate(templateID)
	if err != nil {
		return 0, 33107, err
	}
	response.Map = &protobuf.MAPINFO{
		Id:        &protobuf.WORLDMAPID{RandomId: proto.Uint32(payload.GetId()), TemplateId: proto.Uint32(templateID)},
		CellList:  []*protobuf.CHAPTERCELLINFO_P33{},
		StateFlag: buildWorldStateFlag(runtime, payload.GetId()),
		LandList:  buildWorldLandList(template),
		PosList:   buildWorldPosList(template),
	}
	if changed || !templateKnown {
		if err := orm.SaveWorldRuntime(runtime); err != nil {
			return 0, 33107, err
		}
	}
	return client.SendMessage(33107, &response)
}

func WorldStaminaExchange(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33108
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33109, err
	}

	response := protobuf.SC_33109{Result: proto.Uint32(worldResultFailed)}
	if payload.Type == nil || payload.GetType() != 1 {
		response.Result = proto.Uint32(worldResultUnsupported)
		return client.SendMessage(33109, &response)
	}

	gains, costs, err := loadWorldSupplyTables()
	if err != nil {
		return 0, 33109, err
	}
	if len(gains) == 0 || len(costs) == 0 {
		response.Result = proto.Uint32(worldResultUnsupported)
		return client.SendMessage(33109, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33109, err
	}
	now := time.Now().UTC()
	if changed, _, err := orm.SyncWorldRuntime(runtime, now); err != nil {
		return 0, 33109, err
	} else if changed {
		if err := orm.SaveWorldRuntime(runtime); err != nil {
			return 0, 33109, err
		}
	}
	maxMovePower, _, err := orm.LoadWorldMovePowerSettings()
	if err != nil {
		return 0, 33109, err
	}

	if int(runtime.StaminaExchangeTimes) >= len(costs) {
		response.Result = proto.Uint32(worldResultUnsupported)
		return client.SendMessage(33109, &response)
	}

	idx := int(runtime.StaminaExchangeTimes)
	gain := gains[idx]
	oilCost := costs[idx]
	if !client.Commander.HasEnoughResource(2, oilCost) {
		response.Result = proto.Uint32(worldResultFailed)
		return client.SendMessage(33109, &response)
	}
	if err := client.Commander.ConsumeResource(2, oilCost); err != nil {
		response.Result = proto.Uint32(worldResultFailed)
		return client.SendMessage(33109, &response)
	}

	runtime.ActionPower += gain
	if runtime.ActionPower >= maxMovePower {
		runtime.LastRecoverTimestamp = uint32(now.Unix())
	}
	runtime.StaminaExchangeTimes++
	if err := orm.SaveWorldRuntime(runtime); err != nil {
		return 0, 33109, err
	}

	response.Result = proto.Uint32(worldResultSuccess)
	return client.SendMessage(33109, &response)
}

func WorldTypedDataOperation(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33110
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33111, err
	}

	response := protobuf.SC_33111{Result: proto.Uint32(worldResultFailed)}
	if payload.Type == nil || payload.Data == nil {
		return client.SendMessage(33111, &response)
	}
	if payload.GetType() == 1 {
		response.Result = proto.Uint32(worldResultSuccess)
	} else {
		response.Result = proto.Uint32(worldResultUnsupported)
	}
	return client.SendMessage(33111, &response)
}

func WorldResetOrKill(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33112
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33113, err
	}

	response := protobuf.SC_33113{
		Result:        proto.Uint32(worldResultFailed),
		DropList:      []*protobuf.DROPINFO{},
		Time:          proto.Uint32(0),
		SairenChapter: []uint32{},
	}
	if payload.Type == nil {
		return client.SendMessage(33113, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33113, err
	}
	now := uint32(time.Now().Unix())

	saveRuntime := false
	switch payload.GetType() {
	case 0:
		response.Result = proto.Uint32(worldResultSuccess)
	case 1:
		response.Result = proto.Uint32(worldResultSuccess)
		if runtime.ResetAvailableAtTimestamp > now {
			response.Time = proto.Uint32(runtime.ResetAvailableAtTimestamp - now)
		} else {
			response.Time = proto.Uint32(0)
			runtime.ResetAvailableAtTimestamp = now + 24*60*60
			saveRuntime = true
		}
	case 2:
		response.Result = proto.Uint32(worldResultSuccess)
		if runtime.SairenChapter == nil {
			runtime.SairenChapter = []uint32{}
		}
		response.SairenChapter = append([]uint32{}, runtime.SairenChapter...)
		saveRuntime = true
	default:
		response.Result = proto.Uint32(worldResultUnsupported)
	}

	if saveRuntime {
		if err := orm.SaveWorldRuntime(runtime); err != nil {
			return 0, 33113, err
		}
	}
	return client.SendMessage(33113, &response)
}

func WorldTriggerTask(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33205
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33206, err
	}

	response := protobuf.SC_33206{Result: proto.Uint32(worldResultFailed)}
	if payload.TaskId == nil || payload.GetTaskId() == 0 {
		return client.SendMessage(33206, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33206, err
	}
	config, ok, err := loadWorldTaskConfig(payload.GetTaskId())
	if err != nil {
		return 0, 33206, err
	}
	if !ok {
		return client.SendMessage(33206, &response)
	}
	if config.NeedLevel > uint32(client.Commander.Level) || config.NeedTaskComplete > runtime.TaskFinishCount {
		response.Result = proto.Uint32(worldResultTaskRefused)
		return client.SendMessage(33206, &response)
	}

	now := uint32(time.Now().Unix())
	err = orm.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		ctx := context.Background()
		existing, err := orm.GetCommanderTaskTx(ctx, tx, client.Commander.CommanderID, payload.GetTaskId())
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
		if err == nil && existing != nil {
			return errWorldTaskRefused
		}
		result, err := tx.Exec(ctx, `
INSERT INTO commander_tasks (commander_id, task_id, progress, accept_time, submit_time)
VALUES ($1, $2, 0, $3, 0)
ON CONFLICT (commander_id, task_id) DO NOTHING
`, int64(client.Commander.CommanderID), int64(payload.GetTaskId()), int64(now))
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return errWorldTaskRefused
		}
		return err
	})
	if err != nil {
		if errors.Is(err, errWorldTaskRefused) {
			response.Result = proto.Uint32(worldResultTaskRefused)
			return client.SendMessage(33206, &response)
		}
		return 0, 33206, err
	}

	response.Result = proto.Uint32(worldResultSuccess)
	response.Task = &protobuf.TASK_INFO{
		Id:          proto.Uint32(payload.GetTaskId()),
		Progress:    proto.Uint32(0),
		AcceptTime:  proto.Uint32(now),
		SubmiteTime: proto.Uint32(0),
		EventMapId:  proto.Uint32(config.EventMapID),
	}
	return client.SendMessage(33206, &response)
}

func WorldSubmitTask(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_33207
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 33208, err
	}

	response := protobuf.SC_33208{
		Result:   proto.Uint32(worldResultFailed),
		Drops:    []*protobuf.DROPINFO{},
		Exp:      proto.Uint32(0),
		Intimacy: proto.Uint32(0),
	}
	if payload.TaskId == nil || payload.GetTaskId() == 0 {
		return client.SendMessage(33208, &response)
	}

	runtime, err := orm.LoadOrCreateWorldRuntime(client.Commander.CommanderID)
	if err != nil {
		return 0, 33208, err
	}
	config, ok, err := loadWorldTaskConfig(payload.GetTaskId())
	if err != nil {
		return 0, 33208, err
	}
	if !ok {
		return client.SendMessage(33208, &response)
	}

	drops := make(map[string]*protobuf.DROPINFO)
	now := uint32(time.Now().Unix())
	err = orm.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		ctx := context.Background()
		task, err := orm.GetCommanderTaskTx(ctx, tx, client.Commander.CommanderID, payload.GetTaskId())
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return errWorldTaskInvalid
			}
			return err
		}
		if task.SubmitTime != 0 || task.Progress < config.CompleteTargetNum {
			return errWorldTaskInvalid
		}

		if config.CompleteCondition == 2 && config.ItemRetrieve == 1 {
			itemID, itemCount := parseWorldTaskItemRequirement(config.CompleteParameter)
			if itemID != 0 && itemCount != 0 {
				if !client.Commander.HasEnoughItem(itemID, itemCount) {
					return errWorldTaskInvalid
				}
				if err := client.Commander.ConsumeItemTx(ctx, tx, itemID, itemCount); err != nil {
					return err
				}
			}
		}

		submitted, err := orm.MarkCommanderTaskSubmittedTx(ctx, tx, client.Commander.CommanderID, payload.GetTaskId(), now)
		if err != nil {
			return err
		}
		if !submitted {
			return errWorldTaskInvalid
		}

		for _, drop := range buildWorldTaskDrops(config) {
			accumulateDrop(drops, drop.GetType(), drop.GetId(), drop.GetNumber())
		}
		if err := applyLoveLetterDropsTx(ctx, tx, client, drops); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errWorldTaskInvalid) {
			return client.SendMessage(33208, &response)
		}
		return 0, 33208, err
	}

	runtime.TaskFinishCount++
	if config.CompleteStage > runtime.Progress {
		runtime.Progress = config.CompleteStage
	}
	if err := orm.SaveWorldRuntime(runtime); err != nil {
		return 0, 33208, err
	}

	response.Result = proto.Uint32(worldResultSuccess)
	response.Drops = dropMapToSortedList(drops)
	response.Exp = proto.Uint32(config.Exp)
	response.Intimacy = proto.Uint32(config.Intimacy)
	return client.SendMessage(33208, &response)
}

func flattenEliteFleetShipIDs(fleets []*protobuf.ELITEFLEETINFO) []uint32 {
	shipIDs := make([]uint32, 0)
	for _, fleet := range fleets {
		if fleet == nil {
			continue
		}
		shipIDs = append(shipIDs, fleet.GetShipIdList()...)
	}
	return shipIDs
}

func flattenEliteFleetCommanderIDs(fleets []*protobuf.ELITEFLEETINFO) []uint32 {
	ids := make([]uint32, 0)
	for _, fleet := range fleets {
		if fleet == nil {
			continue
		}
		for _, commander := range fleet.GetCommanders() {
			if commander == nil || commander.Id == nil {
				continue
			}
			ids = append(ids, commander.GetId())
		}
	}
	return ids
}

func buildWorldInfo(runtime *orm.WorldRuntime, fleets []*protobuf.ELITEFLEETINFO) *protobuf.WORLDINFO {
	groups := make([]*protobuf.GROUPINCHAPTER_P33, 0, len(fleets))
	for idx, fleet := range fleets {
		if fleet == nil {
			continue
		}
		ships := make([]*protobuf.SHIPINCHAPTER_P33, 0, len(fleet.GetShipIdList()))
		for _, shipID := range fleet.GetShipIdList() {
			ships = append(ships, &protobuf.SHIPINCHAPTER_P33{Id: proto.Uint32(shipID), HpRant: proto.Uint32(10000), BuffList: []*protobuf.BUFF_INFO{}})
		}
		groupPos := &protobuf.CHAPTERCELLPOS_P33{Row: proto.Uint32(1), Column: proto.Uint32(uint32(idx + 1))}
		group := &protobuf.GROUPINCHAPTER_P33{
			Id:               proto.Uint32(uint32(idx + 1)),
			ShipList:         ships,
			Pos:              groupPos,
			LossFlag:         proto.Uint32(0),
			BoxStrategyList:  []*protobuf.STRATEGYINFO_P33{},
			ShipStrategyList: []*protobuf.STRATEGYINFO_P33{},
			StrategyIds:      []uint32{},
			Bullet:           proto.Uint32(5),
			StartPos:         groupPos,
			AttachList:       []uint32{},
			DamageLevel:      proto.Uint32(0),
			BuffList:         []*protobuf.BUFF_INFO{},
			CommanderList:    fleet.GetCommanders(),
			KillCount:        proto.Uint32(0),
			BulletMax:        proto.Uint32(5),
		}
		groups = append(groups, group)
	}

	taskList := []*protobuf.TASK_INFO{}
	return &protobuf.WORLDINFO{
		MapId:                    proto.Uint32(runtime.MapID),
		Time:                     proto.Uint32(uint32(time.Now().Unix())),
		GroupList:                groups,
		Round:                    proto.Uint32(runtime.Round),
		TaskFinishCount:          proto.Uint32(runtime.TaskFinishCount),
		TaskList:                 taskList,
		SubmarineState:           proto.Uint32(0),
		ItemList:                 []*protobuf.WORLD_ITEM_INFO{},
		GoodsList:                []*protobuf.GOODS_INFO_P33{},
		ActionPower:              proto.Uint32(runtime.ActionPower),
		ActionPowerExtra:         proto.Uint32(runtime.ActionPowerExtra),
		LastRecoverTimestamp:     proto.Uint32(runtime.LastRecoverTimestamp),
		ActionPowerFetchCount:    proto.Uint32(runtime.ActionPowerFetchCount),
		LastChangeGroupTimestamp: proto.Uint32(runtime.LastChangeGroupTimestamp),
		EnterMapId:               proto.Uint32(runtime.EnterMapID),
		CdList:                   []*protobuf.IDTIMEINFO{},
		BuffList:                 []*protobuf.BUFF_INFO{},
		ChapterList:              []*protobuf.WORLDMAPID{},
		SairenChapter:            append([]uint32{}, runtime.SairenChapter...),
		MonthBoss:                []*protobuf.KVDATA{},
	}
}

func buildWorldCountInfo(runtime *orm.WorldRuntime) *protobuf.COUNTINFO {
	activateCount := uint32(0)
	if runtime.MapID > 0 {
		activateCount = 1
	}
	return &protobuf.COUNTINFO{
		StepCount:      proto.Uint32(0),
		TreasureCount:  proto.Uint32(0),
		TaskProgress:   proto.Uint32(runtime.Progress),
		ActivateCount:  proto.Uint32(activateCount),
		CollectionList: []uint32{},
	}
}

func emptyWorldCountInfo() *protobuf.COUNTINFO {
	return &protobuf.COUNTINFO{
		StepCount:      proto.Uint32(0),
		TreasureCount:  proto.Uint32(0),
		TaskProgress:   proto.Uint32(0),
		ActivateCount:  proto.Uint32(0),
		CollectionList: []uint32{},
	}
}

func cloneChapterCellPositions(source []*protobuf.CHAPTERCELLPOS_P33) []*protobuf.CHAPTERCELLPOS_P33 {
	cloned := make([]*protobuf.CHAPTERCELLPOS_P33, 0, len(source))
	for _, pos := range source {
		if pos == nil {
			continue
		}
		cloned = append(cloned, &protobuf.CHAPTERCELLPOS_P33{Row: proto.Uint32(pos.GetRow()), Column: proto.Uint32(pos.GetColumn())})
	}
	return cloned
}

func resolveWorldMapTemplateID(randomID uint32) (uint32, error) {
	if randomID == 0 {
		return 0, nil
	}
	entry, err := orm.GetConfigEntry(worldChapterRandomCategory, strconv.FormatUint(uint64(randomID), 10))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			entry, err = orm.GetConfigEntry(worldChapterRandomCategoryLC, strconv.FormatUint(uint64(randomID), 10))
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					return randomID, nil
				}
				return 0, err
			}
		} else {
			return 0, err
		}
	}

	var cfg worldChapterRandomConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return 0, err
	}
	if cfg.TemplateID != 0 {
		return cfg.TemplateID, nil
	}
	if cfg.Map != 0 {
		return cfg.Map, nil
	}
	if cfg.MapID != 0 {
		return cfg.MapID, nil
	}
	if cfg.ChapterID != 0 {
		return cfg.ChapterID, nil
	}
	if cfg.ID != 0 {
		return cfg.ID, nil
	}
	return randomID, nil
}

func loadWorldTaskConfig(taskID uint32) (*worldTaskConfig, bool, error) {
	entry, err := orm.GetConfigEntry(worldTaskDataCategory, strconv.FormatUint(uint64(taskID), 10))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			entry, err = orm.GetConfigEntry(worldTaskDataCategoryLC, strconv.FormatUint(uint64(taskID), 10))
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					return nil, false, nil
				}
				return nil, false, err
			}
		} else {
			return nil, false, err
		}
	}
	var cfg worldTaskConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return nil, false, err
	}
	return &cfg, true, nil
}

func parseWorldTaskItemRequirement(raw json.RawMessage) (uint32, uint32) {
	decoded, err := decodeJSONValue(raw)
	if err != nil {
		return 0, 0
	}
	list, ok := decoded.([]any)
	if !ok || len(list) == 0 {
		return 0, 0
	}
	if pair, ok := list[0].([]any); ok {
		if len(pair) >= 2 {
			return parseAnyUint(pair[0]), parseAnyUint(pair[1])
		}
	}
	if len(list) >= 2 {
		return parseAnyUint(list[0]), parseAnyUint(list[1])
	}
	return 0, 0
}

func buildWorldTaskDrops(cfg *worldTaskConfig) []*protobuf.DROPINFO {
	raw := cfg.Show
	if len(raw) == 0 || string(raw) == "null" {
		raw = cfg.Drop
	}
	decoded, err := decodeJSONValue(raw)
	if err != nil {
		return []*protobuf.DROPINFO{}
	}
	rows, ok := decoded.([]any)
	if !ok {
		return []*protobuf.DROPINFO{}
	}
	drops := make([]*protobuf.DROPINFO, 0, len(rows))
	for _, row := range rows {
		values, ok := row.([]any)
		if !ok || len(values) < 3 {
			continue
		}
		dropType := parseAnyUint(values[0])
		dropID := parseAnyUint(values[1])
		dropCount := parseAnyUint(values[2])
		if dropType == 0 || dropID == 0 || dropCount == 0 {
			continue
		}
		drops = append(drops, &protobuf.DROPINFO{Type: proto.Uint32(dropType), Id: proto.Uint32(dropID), Number: proto.Uint32(dropCount)})
	}
	return drops
}

func loadWorldSupplyTables() ([]uint32, []uint32, error) {
	gainRows, err := loadWorldGamesetTable("world_supply_value")
	if err != nil {
		return nil, nil, err
	}
	costRows, err := loadWorldGamesetTable("world_supply_price")
	if err != nil {
		return nil, nil, err
	}
	gains := make([]uint32, 0, len(gainRows))
	costs := make([]uint32, 0, len(costRows))
	for _, row := range gainRows {
		if len(row) > 0 {
			gains = append(gains, row[0])
		}
	}
	for _, row := range costRows {
		if len(row) > 2 {
			costs = append(costs, row[2])
		} else if len(row) > 0 {
			costs = append(costs, row[len(row)-1])
		}
	}
	if len(gains) > len(costs) {
		gains = gains[:len(costs)]
	}
	if len(costs) > len(gains) {
		costs = costs[:len(gains)]
	}
	return gains, costs, nil
}

func loadWorldGamesetTable(key string) ([][]uint32, error) {
	entry, err := orm.GetConfigEntry(worldGamesetCategory, key)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			entry, err = orm.GetConfigEntry(worldGamesetCategoryLC, key)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	decoded, err := decodeJSONValue(entry.Data)
	if err != nil {
		return nil, err
	}
	if object, ok := decoded.(map[string]any); ok {
		if description, ok := object["description"]; ok {
			decoded = description
		}
	}
	rowsAny, ok := decoded.([]any)
	if !ok {
		return nil, fmt.Errorf("gameset %s is not an array", key)
	}

	rows := make([][]uint32, 0, len(rowsAny))
	for _, rowAny := range rowsAny {
		valuesAny, ok := rowAny.([]any)
		if !ok {
			continue
		}
		values := make([]uint32, 0, len(valuesAny))
		for _, value := range valuesAny {
			values = append(values, parseAnyUint(value))
		}
		if len(values) > 0 {
			rows = append(rows, values)
		}
	}
	return rows, nil
}

func parseAnyUint(value any) uint32 {
	switch typed := value.(type) {
	case float64:
		if typed < 0 {
			return 0
		}
		return uint32(typed)
	case float32:
		if typed < 0 {
			return 0
		}
		return uint32(typed)
	case int:
		if typed < 0 {
			return 0
		}
		return uint32(typed)
	case int64:
		if typed < 0 {
			return 0
		}
		return uint32(typed)
	case int32:
		if typed < 0 {
			return 0
		}
		return uint32(typed)
	case uint32:
		return typed
	case uint64:
		return uint32(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		if parsed < 0 {
			return 0
		}
		return uint32(parsed)
	default:
		return 0
	}
}

func decodeJSONValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

// worldMapFlag* 是 SC_33107 的 state_flag 取值。客户端把它当"标志位序号集合"用
// （worldproxy.lua:320 NetUpdateMap）：
//
//	slot4[flag] = true; UpdateClearFlag(slot4[1]); UpdateVisionFlag(slot4[2] or ...);
//	NetUpdateMapDiscoveredCells(slot5.id, slot4[3], cell_list)
//
// 即 1=已通关、2=有视野、3=本包带了可用的格子数据。
const worldMapFlagHasCells = 3

// worldMapFlagCleared 对应客户端的 UpdateClearFlag（已通关）。
const worldMapFlagCleared = 1

// worldChapterTemplateConfig 只取重建地图需要的两个数组：
//
//	grids   = [[x, y, walkable], ...]
//	terrain = [[x, y, type, distance], ...]
type worldChapterTemplateConfig struct {
	ID      uint32  `json:"id"`
	Grids   [][]any `json:"grids"`
	Terrain [][]any `json:"terrain"`
}

func loadWorldChapterTemplate(templateID uint32) (*worldChapterTemplateConfig, error) {
	if templateID == 0 {
		return nil, nil
	}
	entry, err := orm.GetConfigEntry(worldChapterTemplateCategory, strconv.FormatUint(uint64(templateID), 10))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var cfg worldChapterTemplateConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// buildWorldStateFlag 返回 SC_33107 的 state_flag。
// 总是置 flag 3（本包带着可用的格子数据）；海域已压制时额外置 flag 1。
// flag 2（视野）还没落库，不宣称。
func buildWorldStateFlag(runtime *orm.WorldRuntime, randomID uint32) []uint32 {
	if runtime != nil && runtime.IsChapterCleared(randomID) {
		return []uint32{worldMapFlagCleared, worldMapFlagHasCells}
	}
	return []uint32{worldMapFlagHasCells}
}

// buildWorldLandList 把 terrain 的 [x, y, type, distance] 逐条转成 LANDINFO。
//
// dir 恒为 (1, 1)：官方那份抓包里 45 条 land 的 dir 全是 (1,1)，配置里也没有方向
// 字段，所以它是常量而不是算出来的值。
func buildWorldLandList(template *worldChapterTemplateConfig) []*protobuf.LANDINFO {
	if template == nil {
		return []*protobuf.LANDINFO{}
	}
	out := make([]*protobuf.LANDINFO, 0, len(template.Terrain))
	for _, row := range template.Terrain {
		if len(row) < 4 {
			continue
		}
		out = append(out, &protobuf.LANDINFO{
			Pos:      buildWorldGridPos(row[0], row[1]),
			Type:     proto.Uint32(parseAnyUint(row[2])),
			Dir:      &protobuf.CHAPTERCELLPOS_P33{Row: proto.Uint32(1), Column: proto.Uint32(1)},
			Distance: proto.Uint32(parseAnyUint(row[3])),
		})
	}
	return out
}

// buildWorldPosList 把 grids 里 walkable 的格子转成 WORLDPOSINFO。
// 官方那次 118 条，正好等于模板 400000 里 walkable=true 的格子数。
func buildWorldPosList(template *worldChapterTemplateConfig) []*protobuf.WORLDPOSINFO {
	if template == nil {
		return []*protobuf.WORLDPOSINFO{}
	}
	out := make([]*protobuf.WORLDPOSINFO, 0, len(template.Grids))
	for _, row := range template.Grids {
		if len(row) < 3 {
			continue
		}
		walkable, ok := row[2].(bool)
		if !ok || !walkable {
			continue
		}
		out = append(out, &protobuf.WORLDPOSINFO{
			Pos:      buildWorldGridPos(row[0], row[1]),
			ItemList: []*protobuf.WORLDITEMINFO{},
		})
	}
	return out
}

// buildWorldGridPos 把 grids/terrain 的坐标对转成 CHAPTERCELLPOS_P33。
//
// 配置里是 (x, y)，协议里叫 row/column。对照官方抓包：terrain [0,1] 对应
// land pos {row:0, column:1}，terrain [0,2] 对应 {row:0, column:2} —— 即 row=x、column=y。
func buildWorldGridPos(x any, y any) *protobuf.CHAPTERCELLPOS_P33 {
	return &protobuf.CHAPTERCELLPOS_P33{
		Row:    proto.Uint32(parseAnyUint(x)),
		Column: proto.Uint32(parseAnyUint(y)),
	}
}
