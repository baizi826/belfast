package world

import (
	"testing"

	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// WORLDINFO 有 8 个 required 字段（map_id / submarine_state / action_power /
// action_power_extra / last_recover_timestamp / action_power_fetch_count /
// last_change_group_timestamp / enter_map_id）。漏一个 proto.Marshal 就报
// "required field not set"，表现为客户端整条连接被 reset —— SC_13001 当初就是这么炸的。
func TestBuildWorldInfoSetsAllRequiredFields(t *testing.T) {
	runtime := &orm.WorldRuntime{
		MapID:                    1034,
		Round:                    42,
		TaskFinishCount:          7,
		ActionPower:              40,
		ActionPowerExtra:         5,
		LastRecoverTimestamp:     1790613742,
		ActionPowerFetchCount:    2,
		LastChangeGroupTimestamp: 1790784000,
		EnterMapID:               34,
	}
	info := buildWorldInfo(runtime)
	if _, err := proto.Marshal(info); err != nil {
		t.Fatalf("marshal failed — a required WORLDINFO field is unset: %v", err)
	}
	if info.GetMapId() != 1034 || info.GetRound() != 42 || info.GetTaskFinishCount() != 7 {
		t.Fatalf("runtime values did not map through: %v", info)
	}
	if info.GetActionPower() != 40 || info.GetActionPowerExtra() != 5 {
		t.Fatalf("action power not mapped: %d / %d", info.GetActionPower(), info.GetActionPowerExtra())
	}
	if info.GetLastRecoverTimestamp() != 1790613742 || info.GetActionPowerFetchCount() != 2 {
		t.Fatalf("power recovery state not mapped: %d / %d",
			info.GetLastRecoverTimestamp(), info.GetActionPowerFetchCount())
	}
	if info.GetLastChangeGroupTimestamp() != 1790784000 {
		t.Fatalf("last_change_group_timestamp not mapped: %d", info.GetLastChangeGroupTimestamp())
	}
	if info.GetEnterMapId() != 34 {
		t.Fatalf("enter_map_id must come from EnterMapID, got %d", info.GetEnterMapId())
	}
}

// 负向控制：空 WORLDINFO 必须 marshal 失败，否则上面的测试证明不了任何东西。
func TestEmptyWorldInfoFailsToMarshal(t *testing.T) {
	if _, err := proto.Marshal(&protobuf.WORLDINFO{}); err == nil {
		t.Fatalf("expected marshal to reject an empty WORLDINFO (required fields unset)")
	}
}
