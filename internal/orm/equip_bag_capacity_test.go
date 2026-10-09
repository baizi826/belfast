package orm

import "testing"

// 257 件装备 / 容量 300 —— 这正是被硬编码 250 卡死的账号形态。
//
// 回归点：常量写着 250 时 «EquipmentBagCount() >= 250» 恒真，于是卸装备(12006)、
// 图纸合成装备(14006)、船上改造装备(14013) 三个互不相干的功能一律回 result=1，
// 客户端统一显示"无效操作"。强化(14002) 之所以一直好用，就是因为它不查容量。
func TestEquipBagHasRoomFollowsAccountCapacity(t *testing.T) {
	commander := Commander{EquipBagMax: 300}
	for i := uint32(0); i < 257; i++ {
		commander.OwnedEquipments = append(commander.OwnedEquipments, OwnedEquipment{
			CommanderID: 1,
			EquipmentID: 1000 + i,
			Count:       1,
		})
	}

	if !commander.EquipBagHasRoom(1) {
		t.Fatalf("257/300 应还有空间：卸下一件装备必须放行")
	}
	if !commander.EquipBagHasRoom(43) {
		t.Fatalf("257+43 = 300 恰好装满，应还有空间")
	}
	if commander.EquipBagHasRoom(44) {
		t.Fatalf("257+44 > 300 应判定为没空间")
	}

	// 未经 DB 加载的 Commander（EquipBagMax 为 0）应退回历史默认 250，
	// 而不是把容量当成 0 从而拒绝一切操作。
	bare := Commander{}
	if !bare.EquipBagHasRoom(250) {
		t.Fatalf("空包 + 默认容量 250 应还有空间")
	}
	if bare.EquipBagHasRoom(251) {
		t.Fatalf("空包 + 251 > 默认容量 250 应判定为没空间")
	}

	// n 直接来自网络，count+n 不能回绕成"有空间"。
	if commander.EquipBagHasRoom(^uint32(0)) {
		t.Fatalf("num 溢出不应被判定为有空间")
	}
}
