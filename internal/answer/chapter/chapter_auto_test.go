package chapter

import "testing"

// 复现客户端 chapterautoproxy.lua:350 的 GetFixTime：
//
//	floor(seconds * time_rate) + time_correction
//
// 服务端给的 seconds 必须和客户端显示的一致，否则玩家看到的完成时刻会跳。
func TestComputeChapterAutoCostMatchesClientFixTime(t *testing.T) {
	cases := []struct {
		name string
		base uint32
		rate float64
		corr uint32
		want uint32
	}{
		{"2-4 默认参数 (time_rate=1, correction=0)", 100, 1, 0, 100},
		{"半速 + 3 秒修正，floor 截断而非四舍五入", 100, 0.5, 3, 53},
		{"修正单独生效", 100, 1, 25, 125},
		{"time_rate 为 0 时回退为 1（配置缺字段）", 10, 0, 7, 17},
		{"time_rate 为负时同样回退为 1", 10, -2, 0, 10},
	}
	for _, tc := range cases {
		stats := &chapterAutoStatistics{TimeRate: tc.rate, TimeCorrection: tc.corr}
		if got := computeChapterAutoCost(tc.base, stats); got != tc.want {
			t.Fatalf("%s: base=%d rate=%v corr=%d → 得到 %d，期望 %d",
				tc.name, tc.base, tc.rate, tc.corr, got, tc.want)
		}
	}
}

// base 为 0（模板 time 缺失）或配置查不到时必须返回 0，
// 调用方据此回 result != 0，而不是凭空造一个耗时把玩家卡在队列里。
func TestComputeChapterAutoCostRefusesWithoutBaseOrConfig(t *testing.T) {
	if got := computeChapterAutoCost(100, nil); got != 0 {
		t.Fatalf("stats 为 nil 时应返回 0，得到 %d", got)
	}
	if got := computeChapterAutoCost(0, &chapterAutoStatistics{TimeRate: 1, TimeCorrection: 5}); got != 0 {
		t.Fatalf("base 为 0 时应返回 0（不能让 correction 单独生效），得到 %d", got)
	}
}
