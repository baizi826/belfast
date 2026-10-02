package orm

import (
	"context"

	"github.com/ggmolly/belfast/internal/db"
)

// ListMetaShipIds 返回 META 船（余烬）的模板 id 列表（97xxxxx 段），用于 SC_63315.arg1。
//
// 背景（2026-10-02 实机抓包对比）：上游 GetMetaProgress 只发 {type:1}，arg1 是空表
// ⇒ 客户端在登录阶段拿到空表后**整块跳过 META 初始化**，连 CS_63317(META 战术信息) 都不发
// ⇒ META/科研相关界面永远转圈。官服同一时刻发的是 13 个 97xxxxx 的模板 id。
//
// ponytail: 直接按 id 段筛 ship_data_template，够用且不新增配置依赖；
// 若将来 cfg 里出现 97 段以外的 META 船，改读 ShareCfg/ship_meta_* 那几张表。
const metaShipTemplateCategory = "ShareCfg/ship_data_template.json"

func ListMetaShipIds() ([]uint32, error) {
	rows, err := db.DefaultStore.Pool.Query(context.Background(), `
SELECT "key"::bigint FROM config_entries
WHERE category = $1
  AND "key" ~ '^[0-9]+$'
  AND "key"::bigint >= 9700000
ORDER BY "key"::bigint
`, metaShipTemplateCategory)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]uint32, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, uint32(id))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
