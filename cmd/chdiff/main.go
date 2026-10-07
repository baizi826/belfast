package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/ggmolly/belfast/internal/answer/chapter"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/protobuf"
)

// chdiff: 用与 cmd/chdump 完全相同的格式打印「我们服务端生成的关卡包」，
// 以便和官服抓包做逐字段对差（官服侧：chdump 13102 <req13101_rep13102_01.bin>）。
//
//	chdiff -chapter 204            # 进图整套格子表（对应官服 SC_13102 的 cell_list）
//	chdiff -chapter 204 -battle 3  # 第 3 次击杀后我们该推的 map_update（对应官服 SC_13105）
func main() {
	chapterID := flag.Int("chapter", 0, "chapter id, e.g. 204 = 2-4; 1104 = 11-4")
	battle := flag.Int("battle", 0, "simulate the spawn step after N kills (0 = entry, matches SC_13102)")
	dsn := flag.String("dsn", "postgres://belfast@127.0.0.1:5432/belfast?sslmode=disable", "postgres dsn")
	flag.Parse()
	if *chapterID == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	// 注意：真实数据在 `belfast` schema（search_path 的 "$user"），传 "public" 会查到空表。
	store, err := db.InitDefaultStore(ctx, *dsn, "")
	if err != nil {
		fmt.Println("init store failed:", err)
		os.Exit(1)
	}
	var entryCount int64
	if err := store.Pool.QueryRow(ctx, `select count(*) from config_entries`).Scan(&entryCount); err != nil {
		fmt.Println("count config_entries failed:", err)
	} else {
		fmt.Printf("db check: config_entries=%d\n", entryCount)
	}
	template, err := chapter.LoadChapterTemplate(uint32(*chapterID), 0)
	if err != nil {
		fmt.Printf("load template %d failed: %v\n", *chapterID, err)
		os.Exit(1)
	}
	if template == nil {
		fmt.Printf("chapter %d not found\n", *chapterID)
		os.Exit(1)
	}

	var cells []*protobuf.CHAPTERCELLINFO_P13
	if *battle > 0 {
		cells, err = chapter.SimulateSpawnStep(template, uint32(*battle))
		if err != nil {
			fmt.Printf("simulate failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("chapter=%d name=%q battle=%d boss_refresh=%d enemy_refresh=%v -> map_update %d cells\n",
			template.ID, "", *battle, template.BossRefresh, template.EnemyRefresh, len(cells))
	} else {
		cells, err = chapter.BuildEntryCells(template)
		if err != nil {
			fmt.Printf("build failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("chapter=%d battle=0 cells=%d\n", template.ID, len(cells))
	}

	sorted := append([]*protobuf.CHAPTERCELLINFO_P13(nil), cells...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].GetPos(), sorted[j].GetPos()
		if a.GetRow() != b.GetRow() {
			return a.GetRow() < b.GetRow()
		}
		return a.GetColumn() < b.GetColumn()
	})
	types := map[uint32]int{}
	for _, c := range sorted {
		types[c.GetItemType()]++
		if c.GetItemType() != 0 {
			fmt.Printf("  cell (%d,%d) type=%d id=%d flag=%d data=%d\n",
				c.GetPos().GetRow(), c.GetPos().GetColumn(), c.GetItemType(), c.GetItemId(), c.GetItemFlag(), c.GetItemData())
		}
	}
	fmt.Println("  type counts:", types)
}
