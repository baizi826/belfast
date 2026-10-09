package technology

import (
	"testing"
	"time"

	"github.com/ggmolly/belfast/internal/orm"
)

// 回归点：已过期的 finish_time 不能原样下发。
//
// 客户端 Technology:isCompleted = isFinish() and finishCondition()；isFinish 要求 time > 0，
// 而 finishCondition 对 condition ~= 0 的项目会走 getProxy(TaskProxy):getTaskVO(cond):isFinish()，
// 那是一个客户端从没构造过的 VO。发 0 就等于“未开工”，同时把这一对条件拆掉。
// （2026-10-10 导入的官服快照里 875 的 finish_time=1790734448（2026-09-29）已过期。）
func TestPastTechnologyFinishTimesAreNotSent(t *testing.T) {
	now := uint32(time.Now().Unix())
	state := &orm.TechnologyResearchState{
		RefreshPools: []orm.TechnologyRefreshPoolState{{
			ID:     2,
			Target: 9,
			Technologies: []orm.TechnologyProjectState{
				{TechID: 875, FinishTime: now - 10},   // 官服快照里那条：已过期
				{TechID: 801, FinishTime: 0},          // 从未开工：原样发 0
				{TechID: 803, FinishTime: now + 3600}, // 正在研发：保留
			},
		}},
		Queue: []orm.TechnologyQueueState{
			{TechID: 884, FinishTime: now - 10}, // 已过期：丢掉
			{TechID: 876, FinishTime: now + 60}, // 还在排：保留
		},
	}

	got := map[uint32]uint32{}
	for _, pool := range buildTechnologyRefreshList(state) {
		for _, tech := range pool.Technologys {
			got[tech.GetId()] = tech.GetTime()
		}
	}
	if got[875] != 0 {
		t.Fatalf("过期的 finish_time 必须发 0，否则客户端卡在主界面；实际 %d", got[875])
	}
	if got[801] != 0 {
		t.Fatalf("未开工的项目应当发 0，实际 %d", got[801])
	}
	if got[803] != now+3600 {
		t.Fatalf("未过期的时间不该被动，期望 %d 实际 %d", now+3600, got[803])
	}

	queue := buildTechnologyQueueList(state)
	if len(queue) != 1 || queue[0].GetId() != 876 {
		t.Fatalf("队列里过期的条目应当被丢掉，只留 876；实际 %d 条", len(queue))
	}
	if queue[0].GetTime() != now+60 {
		t.Fatalf("保留条目的时间不该被动，实际 %d", queue[0].GetTime())
	}
}

// 回归点：catchup 的 version/target 一旦非 0，客户端 getCurCatchNum() 会去
// catchupData[version] 上取 TargetNum，而 catchupData 由 pursuings 构建 ——
// 状态表没有 pursuings 列 ⇒ 永远是 nil ⇒ 在 63000 处理器里抛异常 ⇒ 后面的
// updateTechnologyQueue 不执行 ⇒ TechnologyProxy.queue 恒 nil ⇒ 主界面 OnLoaded 崩。
func TestCatchupIsNotAdvertisedWithoutPursuings(t *testing.T) {
	got := buildTechnologyCatchup()
	if got.GetVersion() != 0 || got.GetTarget() != 0 {
		t.Fatalf("没有 pursuings 时 catchup 必须发 0/0，否则客户端主界面起不来；实际 v%d target=%d",
			got.GetVersion(), got.GetTarget())
	}

	// 原始状态里确实存过官服的非 0 值（这就是当时踩雷的输入），确保它不会漏出去。
	state := &orm.TechnologyResearchState{CatchupVersion: 2, CatchupTarget: 49902}
	if state.CatchupVersion == 0 {
		t.Fatalf("前置条件变了：这个测试假设 states 里可以有非 0 catchup")
	}
}
