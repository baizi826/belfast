package orm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ggmolly/belfast/internal/db"
)

func TestTechnologyResearchStateRoundTrip(t *testing.T) {
	initCommanderItemTestDB(t)
	clearTable(t, &TechnologyResearchState{})
	clearTable(t, &Commander{})

	if err := CreateCommanderRoot(9801, 9801, "tech-state", 0, 0); err != nil {
		t.Fatalf("seed commander: %v", err)
	}

	state := &TechnologyResearchState{
		CommanderID:    9801,
		RefreshFlag:    1,
		RefreshDay:     20260221,
		CatchupVersion: 2,
		CatchupTarget:  19901,
		RefreshPools: []TechnologyRefreshPoolState{
			{ID: 1, Target: 0, Technologies: []TechnologyProjectState{{TechID: 1, FinishTime: 12345}}},
		},
		Queue: []TechnologyQueueState{{TechID: 1, RefreshID: 1, FinishTime: 12345}},
	}
	if err := SaveTechnologyResearchState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := GetTechnologyResearchState(9801)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if loaded.RefreshFlag != 1 || loaded.CatchupVersion != 2 {
		t.Fatalf("unexpected scalar fields: %+v", loaded)
	}
	if len(loaded.RefreshPools) != 1 || len(loaded.Queue) != 1 {
		t.Fatalf("unexpected collection fields: %+v", loaded)
	}
}

func TestBuildTechnologyRefreshPools(t *testing.T) {
	initCommanderItemTestDB(t)
	pools, err := BuildTechnologyRefreshPools(0)
	if err != nil {
		t.Fatalf("build pools: %v", err)
	}
	if len(pools) == 0 {
		t.Fatalf("expected at least one pool")
	}
	for _, pool := range pools {
		if pool.ID == 0 {
			t.Fatalf("pool id must be non-zero")
		}
		if len(pool.Technologies) == 0 {
			t.Fatalf("pool %d has no technologies", pool.ID)
		}
	}
}

// A non-zero `condition` is a task_data_template id, i.e. the project is gated behind a
// campaign task. The client's Technology:finishCondition does
//
//	getConfig("condition") == 0 or getProxy(TaskProxy):getTaskVO(<condition>):isFinish()
//
// and TaskProxy:getTaskVO returns nil for a task the account never accepted, so `:isFinish()`
// indexes nil and the main-menu red-dot registration dies. Official only offers ungated
// projects in the refresh pools, so gated templates must never be emitted as candidates.
func TestBuildTechnologyRefreshPoolsSkipsGatedProjects(t *testing.T) {
	initCommanderItemTestDB(t)
	clearTable(t, &ConfigEntry{})

	entries := []ConfigEntry{
		{Category: technologyDataTemplateCategory, Key: "10", Data: json.RawMessage(`{"id":10,"type":1,"time":60,"condition":0}`)},
		{Category: technologyDataTemplateCategory, Key: "20", Data: json.RawMessage(`{"id":20,"type":1,"time":60,"condition":52006}`)},
		{Category: technologyDataTemplateCategory, Key: "30", Data: json.RawMessage(`{"id":30,"type":1,"time":60}`)},
	}
	for i := range entries {
		if _, err := db.DefaultStore.Pool.Exec(context.Background(),
			`INSERT INTO config_entries (category, key, data) VALUES ($1, $2, $3)`,
			entries[i].Category, entries[i].Key, entries[i].Data); err != nil {
			t.Fatalf("seed config entry: %v", err)
		}
	}

	pools, err := BuildTechnologyRefreshPools(0)
	if err != nil {
		t.Fatalf("build pools: %v", err)
	}

	var got []uint32
	for _, pool := range pools {
		for _, project := range pool.Technologies {
			got = append(got, project.TechID)
		}
	}
	for _, id := range got {
		if id == 20 {
			t.Fatalf("gated project 20 (condition != 0) must not be a candidate; got %v", got)
		}
	}
	found := false
	for _, id := range got {
		if id == 10 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ungated project 10 must be a candidate; got %v", got)
	}
}

