package chapter

import "testing"

// 官服对照数据（抓包 + chapter_template.json）：
// 2-4: enemy_refresh=[2,2,1,2]（和=怪池 7），num_2=12，官服进图恰好 2 只怪、无 BOSS。
// 1-1: enemy_refresh=[1]，怪池 1，num_2=3 → 同一格反复刷到 3 杀才出 BOSS。
func TestChapterEnemyWavesFollowOfficialPacing(t *testing.T) {
	waves := chapterEnemyWaves{waves: []uint32{2, 2, 1, 2}, target: 12}
	cases := []struct {
		kills uint32
		alive int
		boss  bool
	}{
		{0, 2, false},  // 进图只有第一波
		{1, 1, false},  // 击破其中 1 只
		{2, 2, false},  // 第一波清完 → 补第二波
		{4, 1, false},  // 第二波清完 → 第三波（1 只）
		{5, 2, false},  // 第三波清完 → 第四波（2 只）
		{7, 2, false},  // 一轮刷完，循环补位
		{11, 1, false}, // 最后一波只补到目标数
		{12, 0, true},  // 达到 num_2 → BOSS 出现，且不再刷怪
	}
	for _, c := range cases {
		spawned, alive, boss := waves.summary(c.kills, 7)
		if len(alive) != c.alive || boss != c.boss {
			t.Fatalf("kills=%d: alive=%d boss=%v spawned=%d (want %d/%v)", c.kills, len(alive), boss, spawned, c.alive, c.boss)
		}
	}

	single := chapterEnemyWaves{waves: []uint32{1}, target: 3}
	if _, alive, boss := single.summary(0, 1); len(alive) != 1 || boss {
		t.Fatalf("1-1 进图应有 1 只怪且无 BOSS")
	}
	if _, alive, boss := single.summary(2, 1); len(alive) != 1 || boss {
		t.Fatalf("1-1 第 3 杀前应仍有 1 只怪且无 BOSS")
	}
	if _, alive, boss := single.summary(3, 1); len(alive) != 0 || !boss {
		t.Fatalf("1-1 达到 3 杀应清空怪并出现 BOSS")
	}
}
