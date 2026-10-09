package orm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ggmolly/belfast/internal/db"
)

const technologyDataTemplateCategory = "ShareCfg/technology_data_template.json"

type TechnologyResearchState struct {
	CommanderID    uint32
	RefreshFlag    uint32
	RefreshDay     uint32
	CatchupVersion uint32
	CatchupTarget  uint32
	RefreshPools   []TechnologyRefreshPoolState
	Queue          []TechnologyQueueState

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (TechnologyResearchState) TableName() string {
	return "technology_research_states"
}

type TechnologyRefreshPoolState struct {
	ID           uint32                   `json:"id"`
	Target       uint32                   `json:"target"`
	Technologies []TechnologyProjectState `json:"technologies"`
}

type TechnologyProjectState struct {
	TechID     uint32 `json:"tech_id"`
	FinishTime uint32 `json:"finish_time"`
}

type TechnologyQueueState struct {
	TechID     uint32 `json:"tech_id"`
	RefreshID  uint32 `json:"refresh_id"`
	FinishTime uint32 `json:"finish_time"`
}

// TechnologyRows 容忍 [][]uint32 与 {} 两种形态（与 ShipBreakoutItems / chapterTemplate 同因同治）。
// 9.7 的 ShareCfg/technology_data_template.json 里有整行是空表 ⇒ consume/drop_client 被
// tools/lua2json.py 写成 {}，直接解进 [][]uint32 会报
// "cannot unmarshal object into Go struct field TechnologyDataTemplate.consume of type [][]uint32"，
// handler 把 error 往上抛 ⇒ client.CloseWithError reset 整条连接
// （实测 SC_63010 一进科研项目就掉线，连主界面都进不去）。
type TechnologyRows [][]uint32

func (s *TechnologyRows) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed[0] != '[' {
		*s = nil // {} / null / 其它非数组形态一律当空
		return nil
	}
	var v [][]uint32
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return err
	}
	*s = v
	return nil
}

type TechnologyDataTemplate struct {
	ID               uint32         `json:"id"`
	Type             uint32         `json:"type"`
	Time             uint32         `json:"time"`
	Condition        uint32         `json:"condition"`
	BlueprintVersion uint32         `json:"blueprint_version"`
	Consume          TechnologyRows `json:"consume"`
	DropClient       TechnologyRows `json:"drop_client"`
}

func GetTechnologyResearchState(commanderID uint32) (*TechnologyResearchState, error) {
	state := &TechnologyResearchState{}
	var refreshPoolsRaw []byte
	var queueRaw []byte
	err := db.DefaultStore.Pool.QueryRow(context.Background(), `
SELECT commander_id, refresh_flag, refresh_day, catchup_version, catchup_target, refresh_pools, queue, created_at, updated_at
FROM technology_research_states
WHERE commander_id = $1
`, int64(commanderID)).Scan(
		&state.CommanderID,
		&state.RefreshFlag,
		&state.RefreshDay,
		&state.CatchupVersion,
		&state.CatchupTarget,
		&refreshPoolsRaw,
		&queueRaw,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	err = db.MapNotFound(err)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(refreshPoolsRaw, &state.RefreshPools); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(queueRaw, &state.Queue); err != nil {
		return nil, err
	}
	if state.RefreshPools == nil {
		state.RefreshPools = []TechnologyRefreshPoolState{}
	}
	if state.Queue == nil {
		state.Queue = []TechnologyQueueState{}
	}
	return state, nil
}

func GetTechnologyResearchStateForUpdateTx(ctx context.Context, tx pgx.Tx, commanderID uint32) (*TechnologyResearchState, error) {
	state := &TechnologyResearchState{}
	var refreshPoolsRaw []byte
	var queueRaw []byte
	err := tx.QueryRow(ctx, `
SELECT commander_id, refresh_flag, refresh_day, catchup_version, catchup_target, refresh_pools, queue, created_at, updated_at
FROM technology_research_states
WHERE commander_id = $1
FOR UPDATE
`, int64(commanderID)).Scan(
		&state.CommanderID,
		&state.RefreshFlag,
		&state.RefreshDay,
		&state.CatchupVersion,
		&state.CatchupTarget,
		&refreshPoolsRaw,
		&queueRaw,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	err = db.MapNotFound(err)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(refreshPoolsRaw, &state.RefreshPools); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(queueRaw, &state.Queue); err != nil {
		return nil, err
	}
	if state.RefreshPools == nil {
		state.RefreshPools = []TechnologyRefreshPoolState{}
	}
	if state.Queue == nil {
		state.Queue = []TechnologyQueueState{}
	}
	return state, nil
}

func GetOrCreateTechnologyResearchState(commanderID uint32) (*TechnologyResearchState, error) {
	state, err := GetTechnologyResearchState(commanderID)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	pools, err := BuildTechnologyRefreshPools(0)
	if err != nil {
		return nil, err
	}
	state = &TechnologyResearchState{
		CommanderID:  commanderID,
		RefreshDay:   CurrentTechnologyDay(time.Now().UTC()),
		RefreshPools: pools,
		Queue:        []TechnologyQueueState{},
	}
	if err := SaveTechnologyResearchState(state); err != nil {
		return nil, err
	}
	return GetTechnologyResearchState(commanderID)
}

func GetOrCreateTechnologyResearchStateForUpdateTx(ctx context.Context, tx pgx.Tx, commanderID uint32) (*TechnologyResearchState, error) {
	state, err := GetTechnologyResearchStateForUpdateTx(ctx, tx, commanderID)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	pools, err := BuildTechnologyRefreshPools(0)
	if err != nil {
		return nil, err
	}
	state = &TechnologyResearchState{
		CommanderID:  commanderID,
		RefreshDay:   CurrentTechnologyDay(time.Now().UTC()),
		RefreshPools: pools,
		Queue:        []TechnologyQueueState{},
	}
	if err := SaveTechnologyResearchStateTx(ctx, tx, state); err != nil {
		return nil, err
	}
	return GetTechnologyResearchStateForUpdateTx(ctx, tx, commanderID)
}

func SaveTechnologyResearchState(state *TechnologyResearchState) error {
	ctx := context.Background()
	return db.DefaultStore.WithPGXTx(ctx, func(tx pgx.Tx) error {
		return SaveTechnologyResearchStateTx(ctx, tx, state)
	})
}

func SaveTechnologyResearchStateTx(ctx context.Context, tx pgx.Tx, state *TechnologyResearchState) error {
	if state == nil {
		return errors.New("technology research state is nil")
	}
	if state.RefreshPools == nil {
		state.RefreshPools = []TechnologyRefreshPoolState{}
	}
	if state.Queue == nil {
		state.Queue = []TechnologyQueueState{}
	}
	refreshPoolsRaw, err := json.Marshal(state.RefreshPools)
	if err != nil {
		return err
	}
	queueRaw, err := json.Marshal(state.Queue)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO technology_research_states (
  commander_id, refresh_flag, refresh_day, catchup_version, catchup_target, refresh_pools, queue, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
ON CONFLICT (commander_id)
DO UPDATE SET
  refresh_flag = EXCLUDED.refresh_flag,
  refresh_day = EXCLUDED.refresh_day,
  catchup_version = EXCLUDED.catchup_version,
  catchup_target = EXCLUDED.catchup_target,
  refresh_pools = EXCLUDED.refresh_pools,
  queue = EXCLUDED.queue,
  updated_at = NOW()
`, int64(state.CommanderID), int64(state.RefreshFlag), int64(state.RefreshDay), int64(state.CatchupVersion), int64(state.CatchupTarget), refreshPoolsRaw, queueRaw)
	return err
}

func CurrentTechnologyDay(now time.Time) uint32 {
	now = now.UTC()
	return uint32(now.Year()*10000 + int(now.Month())*100 + now.Day())
}

func BuildTechnologyRefreshPools(seed uint32) ([]TechnologyRefreshPoolState, error) {
	entries, err := ListConfigEntries(technologyDataTemplateCategory)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return []TechnologyRefreshPoolState{{ID: 1, Target: 0, Technologies: []TechnologyProjectState{{TechID: 1, FinishTime: 0}}}}, nil
		}
		return nil, err
	}
	templates := make([]TechnologyDataTemplate, 0, len(entries))
	for _, entry := range entries {
		var template TechnologyDataTemplate
		if err := json.Unmarshal(entry.Data, &template); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	byPool := map[uint32][]TechnologyProjectState{}
	for _, template := range templates {
		if template.ID == 0 || template.Type == 0 {
			continue
		}
		// A non-zero `condition` is a task_data_template id: the project is gated behind a
		// campaign task. The client's Technology:finishCondition is
		//     getConfig("condition") == 0 or getProxy(TaskProxy):getTaskVO(cond):isFinish()
		// and TechnologyProxy:updateTechnologys builds the VO without `queue`, so isQueue is
		// nil and the `or` right-hand side runs. TaskProxy:getTaskVO returns nil for a task
		// this account never accepted, so `:isFinish()` indexes nil and the main-menu red-dot
		// registration dies (PlayerProxy:IsShowCommssionTip -> Technology:isCompleted ->
		// finishCondition). Official only offers ungated projects in the refresh pools.
		if template.Condition != 0 {
			continue
		}
		byPool[template.Type] = append(byPool[template.Type], TechnologyProjectState{TechID: template.ID, FinishTime: 0})
	}
	poolIDs := make([]uint32, 0, len(byPool))
	for id := range byPool {
		poolIDs = append(poolIDs, id)
	}
	sort.Slice(poolIDs, func(i, j int) bool {
		return poolIDs[i] < poolIDs[j]
	})

	pools := make([]TechnologyRefreshPoolState, 0, len(poolIDs))
	for _, poolID := range poolIDs {
		techs := byPool[poolID]
		sort.Slice(techs, func(i, j int) bool {
			return techs[i].TechID < techs[j].TechID
		})
		const maxPoolCandidates = 5
		if len(techs) > maxPoolCandidates {
			offset := int(seed % uint32(len(techs)))
			rotated := append(techs[offset:], techs[:offset]...)
			techs = rotated[:maxPoolCandidates]
		}
		pools = append(pools, TechnologyRefreshPoolState{ID: poolID, Target: 0, Technologies: techs})
	}
	if len(pools) == 0 {
		return []TechnologyRefreshPoolState{{ID: 1, Target: 0, Technologies: []TechnologyProjectState{{TechID: 1, FinishTime: 0}}}}, nil
	}
	return pools, nil
}

func GetTechnologyTemplate(techID uint32) (*TechnologyDataTemplate, error) {
	entry, err := GetConfigEntry(technologyDataTemplateCategory, fmt.Sprintf("%d", techID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return &TechnologyDataTemplate{ID: techID, Type: 1, Time: 60, Condition: 0, Consume: [][]uint32{}, DropClient: [][]uint32{{2, 59001, 1}}}, nil
		}
		return nil, err
	}
	var template TechnologyDataTemplate
	if err := json.Unmarshal(entry.Data, &template); err != nil {
		return nil, err
	}
	return &template, nil
}

func MaxTechnologyBlueprintVersion() (uint32, error) {
	entries, err := ListConfigEntries(technologyDataTemplateCategory)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return 1, nil
		}
		return 0, err
	}
	maxVersion := uint32(0)
	for _, entry := range entries {
		var template TechnologyDataTemplate
		if err := json.Unmarshal(entry.Data, &template); err != nil {
			return 0, err
		}
		if template.BlueprintVersion > maxVersion {
			maxVersion = template.BlueprintVersion
		}
	}
	if maxVersion == 0 {
		return 1, nil
	}
	return maxVersion, nil
}
