package technology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
)

const (
	technologyOK      = uint32(0)
	technologyInvalid = uint32(1)
	technologyPersist = uint32(2)
)

func StartTechnologyResearch(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63001
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63002, err
	}

	response := &protobuf.SC_63002{Result: proto.Uint32(technologyInvalid), Time: proto.Uint32(0)}
	techID := payload.GetTechId()
	refreshID := payload.GetRefreshId()
	if techID == 0 || refreshID == 0 {
		return client.SendMessage(63002, response)
	}
	if err := client.Commander.Load(); err != nil {
		return 0, 63002, err
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())

		pool, ok := findTechnologyPool(state, refreshID)
		if !ok {
			return nil
		}
		project, ok := findTechnologyProject(pool, techID)
		if !ok {
			return nil
		}
		if hasActiveTechnology(state, uint32(time.Now().Unix())) {
			return nil
		}

		template, err := orm.GetTechnologyTemplate(techID)
		if err != nil {
			return nil
		}
		// Gated projects must never be startable: there is no campaign task to satisfy the
		// condition, and once finish_time passes the client's finishCondition dereferences a
		// nil TaskProxy VO and the main menu fails to load. See BuildTechnologyRefreshPools.
		if template.Condition != 0 {
			return nil
		}
		if !canConsumeTechnologyCost(client.Commander, template.Consume) {
			return nil
		}
		if err := consumeTechnologyCostTx(context.Background(), tx, client.Commander, template.Consume); err != nil {
			return err
		}

		finish := uint32(time.Now().Unix()) + template.Time
		project.FinishTime = finish
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		response.Time = proto.Uint32(finish)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "StartResearch", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63002, response)
}

func FinishTechnologyResearch(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63003
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63004, err
	}

	response := &protobuf.SC_63004{Result: proto.Uint32(technologyInvalid)}
	techID := payload.GetTechId()
	refreshID := payload.GetRefreshId()
	if techID == 0 || refreshID == 0 {
		return client.SendMessage(63004, response)
	}
	if err := client.Commander.Load(); err != nil {
		return 0, 63004, err
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		now := uint32(time.Now().Unix())
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())

		pool, ok := findTechnologyPool(state, refreshID)
		if !ok {
			return nil
		}
		project, ok := findTechnologyProject(pool, techID)
		if !ok || project.FinishTime == 0 || project.FinishTime > now {
			return nil
		}

		template, err := orm.GetTechnologyTemplate(techID)
		if err != nil {
			return nil
		}
		rewards := buildDropInfoList(template.DropClient)
		if err := grantTechnologyRewardsTx(context.Background(), tx, client.Commander, rewards); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}

		project.FinishTime = 0
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}

		response.Result = proto.Uint32(technologyOK)
		response.CommonList = rewards
		response.RefreshList = buildTechnologyRefreshList(state)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "FinishResearch", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63004, response)
}

func StopTechnologyResearch(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63005
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63006, err
	}
	response := &protobuf.SC_63006{Result: proto.Uint32(technologyInvalid)}
	techID := payload.GetTechId()
	refreshID := payload.GetRefreshId()
	if techID == 0 || refreshID == 0 {
		return client.SendMessage(63006, response)
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())

		pool, ok := findTechnologyPool(state, refreshID)
		if !ok {
			return nil
		}
		project, ok := findTechnologyProject(pool, techID)
		if !ok {
			return nil
		}
		now := uint32(time.Now().Unix())
		if project.FinishTime == 0 || project.FinishTime <= now {
			return nil
		}

		project.FinishTime = 0
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "StopResearch", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63006, response)
}

func RefreshTechnologyProjects(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63007
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63008, err
	}
	response := &protobuf.SC_63008{Result: proto.Uint32(technologyInvalid)}
	if payload.GetType() != 1 {
		return client.SendMessage(63008, response)
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		now := time.Now().UTC()
		normalizeTechnologyRefreshFlag(state, now)
		if state.RefreshFlag != 0 {
			return nil
		}
		if hasActiveTechnology(state, uint32(now.Unix())) {
			return nil
		}

		seed := uint32(now.Unix()) + client.Commander.CommanderID
		pools, err := orm.BuildTechnologyRefreshPools(seed)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		carryPoolTargets(state, pools)
		state.RefreshPools = pools
		state.RefreshFlag = 1
		state.RefreshDay = orm.CurrentTechnologyDay(now)
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		response.RefreshList = buildTechnologyRefreshList(state)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "RefreshResearch", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63008, response)
}

func ChangeRefreshTechnologyTendency(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63009
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63010, err
	}

	response := &protobuf.SC_63010{Result: proto.Uint32(technologyInvalid)}
	refreshID := payload.GetId()
	target := payload.GetTarget()
	if refreshID == 0 {
		return client.SendMessage(63010, response)
	}
	maxTarget, err := orm.MaxTechnologyBlueprintVersion()
	if err != nil {
		return 0, 63010, err
	}
	if target > maxTarget {
		return client.SendMessage(63010, response)
	}

	err = db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())

		pool, ok := findTechnologyPool(state, refreshID)
		if !ok {
			return nil
		}
		pool.Target = target
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "SetTendency", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63010, response)
}

func SelectTechnologyCatchupTarget(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63011
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63012, err
	}

	response := &protobuf.SC_63012{Result: proto.Uint32(technologyInvalid)}
	version := payload.GetVersion()
	target := payload.GetTarget()
	if version == 0 || target == 0 {
		return client.SendMessage(63012, response)
	}
	ok, err := isValidCatchupTarget(version, target)
	if err != nil {
		return 0, 63012, err
	}
	if !ok {
		return client.SendMessage(63012, response)
	}

	err = db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())
		if state.CatchupVersion != 0 && state.CatchupTarget != 0 {
			return nil
		}
		state.CatchupVersion = version
		state.CatchupTarget = target
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "SelectCatchupTarget", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63012, response)
}

func JoinTechnologyQueue(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63013
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63014, err
	}

	response := &protobuf.SC_63014{Result: proto.Uint32(technologyInvalid)}
	techID := payload.GetTechId()
	refreshID := payload.GetRefreshId()
	if techID == 0 || refreshID == 0 {
		return client.SendMessage(63014, response)
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())
		if len(state.Queue) >= 5 {
			return nil
		}

		pool, ok := findTechnologyPool(state, refreshID)
		if !ok {
			return nil
		}
		project, ok := findTechnologyProject(pool, techID)
		if !ok {
			return nil
		}
		now := uint32(time.Now().Unix())
		if project.FinishTime == 0 || project.FinishTime <= now {
			return nil
		}

		state.Queue = append(state.Queue, orm.TechnologyQueueState{TechID: techID, RefreshID: refreshID, FinishTime: project.FinishTime})
		sort.Slice(state.Queue, func(i, j int) bool {
			return state.Queue[i].FinishTime < state.Queue[j].FinishTime
		})
		project.FinishTime = 0
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		response.RefreshList = buildTechnologyRefreshList(state)
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "JoinQueue", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63014, response)
}

func FinishQueueTechnology(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_63015
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 63016, err
	}

	response := &protobuf.SC_63016{Result: proto.Uint32(technologyInvalid)}
	if payload.GetId() != 0 {
		return client.SendMessage(63016, response)
	}
	if err := client.Commander.Load(); err != nil {
		return 0, 63016, err
	}

	err := db.DefaultStore.WithPGXTx(context.Background(), func(tx pgx.Tx) error {
		state, err := orm.GetOrCreateTechnologyResearchStateForUpdateTx(context.Background(), tx, client.Commander.CommanderID)
		if err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		normalizeTechnologyRefreshFlag(state, time.Now().UTC())
		if len(state.Queue) == 0 {
			return nil
		}

		now := uint32(time.Now().Unix())
		claimed := make([]orm.TechnologyQueueState, 0)
		remaining := state.Queue
		for len(remaining) > 0 {
			if remaining[0].FinishTime > now {
				break
			}
			claimed = append(claimed, remaining[0])
			remaining = remaining[1:]
		}
		if len(claimed) == 0 {
			return nil
		}

		drops := make([]*protobuf.TECHNOLOGYDROP, 0, len(claimed))
		for _, entry := range claimed {
			template, err := orm.GetTechnologyTemplate(entry.TechID)
			if err != nil {
				response.Result = proto.Uint32(technologyPersist)
				return err
			}
			rewards := buildDropInfoList(template.DropClient)
			if err := grantTechnologyRewardsTx(context.Background(), tx, client.Commander, rewards); err != nil {
				response.Result = proto.Uint32(technologyPersist)
				return err
			}
			drops = append(drops, &protobuf.TECHNOLOGYDROP{CommonList: rewards})
		}

		state.Queue = remaining
		if err := orm.SaveTechnologyResearchStateTx(context.Background(), tx, state); err != nil {
			response.Result = proto.Uint32(technologyPersist)
			return err
		}
		response.Result = proto.Uint32(technologyOK)
		response.Drops = drops
		return nil
	})
	if err != nil {
		logger.LogEvent("Technology", "FinishQueue", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	return client.SendMessage(63016, response)
}

func buildTechnologyRefreshSyncResponse(commanderID uint32) (*protobuf.SC_63000, error) {
	state, err := orm.GetOrCreateTechnologyResearchState(commanderID)
	if err != nil {
		return nil, err
	}
	normalized := normalizeTechnologyRefreshFlag(state, time.Now().UTC())
	if normalized {
		if err := orm.SaveTechnologyResearchState(state); err != nil {
			return nil, err
		}
	}
	return &protobuf.SC_63000{
		RefreshList: buildTechnologyRefreshList(state),
		RefreshFlag: proto.Uint32(state.RefreshFlag),
		Catchup:     buildTechnologyCatchup(),
		Queue:       buildTechnologyQueueList(state),
	}, nil
}

// buildTechnologyCatchup always reports "no catch-up in progress".
//
// The state table has no column for `pursuings`, and the client's updateTecCatchup ends with
// getCurCatchNum():
//
//	if curCatchupTecID ~= 0 and curCatchupGroupID ~= 0 then
//		return catchupData[curCatchupTecID]:getTargetNum(curCatchupGroupID)
//	end
//
// catchupData is built from `pursuings`, so a non-zero version with an empty pursuings list is a
// nil:getTargetNum() and throws. The exception aborts the 63000 handler *before*
// updateTechnologyQueue runs, so TechnologyProxy.queue stays nil; the main menu's red-dot pass
// then dies in getPlanningTechnologys (table.mergeArray(queue, ...)) from NewMainMellowTheme:OnLoaded
// and the client sits on the login background forever with heartbeats still flowing and nothing
// else in logcat (2026-10-10: importing the official snapshot's catchup v2/target 49902 did exactly
// this). A catch-up version we cannot back with pursuings must therefore not be sent.
//
// When `pursuings` get a column, pass state.CatchupVersion/CatchupTarget again - together with them.
func buildTechnologyCatchup() *protobuf.TECHNOLOGYCATCHUP {
	return &protobuf.TECHNOLOGYCATCHUP{Version: proto.Uint32(0), Target: proto.Uint32(0)}
}

func buildTechnologyRefreshList(state *orm.TechnologyResearchState) []*protobuf.TECHNOLOGYREFRESH {
	result := make([]*protobuf.TECHNOLOGYREFRESH, 0, len(state.RefreshPools))
	for _, pool := range state.RefreshPools {
		entry := &protobuf.TECHNOLOGYREFRESH{
			Id:          proto.Uint32(pool.ID),
			Target:      proto.Uint32(pool.Target),
			Technologys: make([]*protobuf.TECHNOLOGYINFO, 0, len(pool.Technologies)),
		}
		for _, project := range pool.Technologies {
			entry.Technologys = append(entry.Technologys, &protobuf.TECHNOLOGYINFO{Id: proto.Uint32(project.TechID), Time: proto.Uint32(technologyFinishTime(project.FinishTime))})
		}
		result = append(result, entry)
	}
	return result
}

func buildTechnologyQueueList(state *orm.TechnologyResearchState) []*protobuf.TECHNOLOGYINFO {
	result := make([]*protobuf.TECHNOLOGYINFO, 0, len(state.Queue))
	for _, queued := range state.Queue {
		if queued.FinishTime != 0 && technologyFinishTime(queued.FinishTime) == 0 {
			continue // 过期 = 已完成，发出去只会触发客户端的 completion 分支
		}
		result = append(result, &protobuf.TECHNOLOGYINFO{Id: proto.Uint32(queued.TechID), Time: proto.Uint32(queued.FinishTime)})
	}
	return result
}

// technologyFinishTime returns the finish_time that may be sent to the client.
//
// A time that has already passed must not leave here as a non-zero value. To the client's
// Technology:isCompleted/finishCondition a passed time means "completed", and that path walks the
// main-menu red-dot registration (PlayerProxy:IsShowCommssionTip), which dies on a VO the client
// never built: the menu never draws and the player sits on the background forever while the
// connection stays alive and heartbeats keep flowing - with no Lua error in logcat to show for it.
//
// Stale times reach us from two directions: an imported official snapshot (this account's research
// had finished 2026-09-29 by the time we served it) and a project that completed while the player
// was offline. Returning 0 puts such a project back to "available and not started", which is what
// the client does after the player collects a finished one.
func technologyFinishTime(finish uint32) uint32 {
	if finish == 0 || finish > uint32(time.Now().Unix()) {
		return finish
	}
	return 0
}

func carryPoolTargets(existing *orm.TechnologyResearchState, next []orm.TechnologyRefreshPoolState) {
	targetByPool := make(map[uint32]uint32, len(existing.RefreshPools))
	for _, pool := range existing.RefreshPools {
		targetByPool[pool.ID] = pool.Target
	}
	for i := range next {
		if target, ok := targetByPool[next[i].ID]; ok {
			next[i].Target = target
		}
	}
}

func normalizeTechnologyRefreshFlag(state *orm.TechnologyResearchState, now time.Time) bool {
	day := orm.CurrentTechnologyDay(now)
	if state.RefreshDay == day {
		return false
	}
	state.RefreshDay = day
	state.RefreshFlag = 0
	return true
}

func hasActiveTechnology(state *orm.TechnologyResearchState, now uint32) bool {
	for _, pool := range state.RefreshPools {
		for _, project := range pool.Technologies {
			if project.FinishTime > now {
				return true
			}
		}
	}
	return false
}

func findTechnologyPool(state *orm.TechnologyResearchState, refreshID uint32) (*orm.TechnologyRefreshPoolState, bool) {
	for i := range state.RefreshPools {
		if state.RefreshPools[i].ID == refreshID {
			return &state.RefreshPools[i], true
		}
	}
	return nil, false
}

func findTechnologyProject(pool *orm.TechnologyRefreshPoolState, techID uint32) (*orm.TechnologyProjectState, bool) {
	for i := range pool.Technologies {
		if pool.Technologies[i].TechID == techID {
			return &pool.Technologies[i], true
		}
	}
	return nil, false
}

func canConsumeTechnologyCost(commander *orm.Commander, consume [][]uint32) bool {
	for _, row := range consume {
		if len(row) < 3 {
			continue
		}
		dropType := row[0]
		id := row[1]
		count := row[2]
		switch dropType {
		case 1:
			if !commander.HasEnoughResource(id, count) {
				return false
			}
		case 2:
			if !commander.HasEnoughItem(id, count) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func consumeTechnologyCostTx(ctx context.Context, tx pgx.Tx, commander *orm.Commander, consume [][]uint32) error {
	for _, row := range consume {
		if len(row) < 3 {
			continue
		}
		dropType := row[0]
		id := row[1]
		count := row[2]
		switch dropType {
		case 1:
			if err := commander.ConsumeResourceTx(ctx, tx, id, count); err != nil {
				return err
			}
		case 2:
			if err := commander.ConsumeItemTx(ctx, tx, id, count); err != nil {
				return err
			}
		default:
			return errors.New("unsupported technology consume type")
		}
	}
	return nil
}

func buildDropInfoList(rows [][]uint32) []*protobuf.DROPINFO {
	result := make([]*protobuf.DROPINFO, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		result = append(result, &protobuf.DROPINFO{
			Type:   proto.Uint32(row[0]),
			Id:     proto.Uint32(row[1]),
			Number: proto.Uint32(row[2]),
		})
	}
	return result
}

func grantTechnologyRewardsTx(ctx context.Context, tx pgx.Tx, commander *orm.Commander, rewards []*protobuf.DROPINFO) error {
	for _, reward := range rewards {
		switch reward.GetType() {
		case 1:
			if err := commander.AddResourceTx(ctx, tx, reward.GetId(), reward.GetNumber()); err != nil {
				return err
			}
		case 2:
			if err := commander.AddItemTx(ctx, tx, reward.GetId(), reward.GetNumber()); err != nil {
				return err
			}
		}
	}
	return nil
}

func isValidCatchupTarget(version uint32, target uint32) (bool, error) {
	entries, err := orm.ListConfigEntries("ShareCfg/technology_catchup_template.json")
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return version == 1 && target == 29901, nil
		}
		return false, err
	}
	if len(entries) == 0 {
		return version > 0 && target > 0, nil
	}
	type catchupTemplate struct {
		ID         uint32   `json:"id"`
		CharChoice []uint32 `json:"char_choice"`
	}
	for _, entry := range entries {
		var parsed catchupTemplate
		if err := json.Unmarshal(entry.Data, &parsed); err != nil {
			return false, err
		}
		if parsed.ID != version {
			continue
		}
		for _, choice := range parsed.CharChoice {
			if choice == target {
				return true, nil
			}
		}
		return false, nil
	}
	return false, nil
}

func TechnologyResearchDebugSummary(commanderID uint32) string {
	state, err := orm.GetTechnologyResearchState(commanderID)
	if err != nil {
		return fmt.Sprintf("state-error:%v", err)
	}
	return fmt.Sprintf("refresh_pools=%d queue=%d", len(state.RefreshPools), len(state.Queue))
}
