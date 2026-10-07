package chapter

import (
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// 地图机制（按官服行为）：
//   - 神秘箱 attach=2（客户端 'MM'）：舰队走到格子上即拾取，箱子消失并补一次弹药
//     —— ALAS `clear_chosen_mystery` + `pick_up_ammo`（recover 最多 3）就是这个行为。
//   - 弹药箱 attach=3（'MA'）由 op 7（chapterOpSupply）补给，已在 chapter_actions.go 实现。
//
// ponytail: 神秘箱只补弹、不发放箱内物品（缺 chapter_box_template 奖励表）；
// 升级点 = 接上 box 奖励表后走 item 发放 + SC_13104.DropList。

// takeChapterBoxCell 拾取舰队脚下的神秘箱：箱子消失、补弹，返回要推给客户端的空格。
func takeChapterBoxCell(current *protobuf.CURRENTCHAPTERINFO, pos chapterPos, group *protobuf.GROUPINCHAPTER_P13, template *chapterTemplate) *protobuf.CHAPTERCELLINFO_P13 {
	if current == nil || group == nil || template == nil {
		return nil
	}
	idx, cell := findChapterCellAt(current, pos)
	if cell == nil || cell.GetItemType() != chapterAttachBox {
		return nil
	}
	if maxAmmo := template.AmmoTotal; maxAmmo > 0 && group.GetBullet() < maxAmmo {
		recover := maxAmmo - group.GetBullet()
		if recover > 3 {
			recover = 3
		}
		group.Bullet = proto.Uint32(group.GetBullet() + recover)
	}
	if idx >= 0 && idx < len(current.CellList) {
		current.CellList = append(current.CellList[:idx], current.CellList[idx+1:]...)
	}
	empty := &protobuf.CHAPTERCELLINFO_P13{
		Pos:      buildPos(pos),
		ItemType: proto.Uint32(0),
		ItemFlag: proto.Uint32(chapterCellActive),
	}
	upsertChapterCell(current, empty)
	return empty
}
