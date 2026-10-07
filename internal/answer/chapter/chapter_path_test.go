package chapter

import (
	"testing"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// 客户端 chapterleveldata.lua 的移动规则：
// 存活小怪是障碍（PrioObstacle），跨不过去；但可以把它当终点（走过去就是打它）；
// 被击沉(flag=disabled)之后就不再挡路。
func TestChapterAliveEnemyBlocksMove(t *testing.T) {
	grids := []chapterGrid{
		{Row: 1, Column: 1, Walkable: true},
		{Row: 1, Column: 2, Walkable: true},
		{Row: 1, Column: 3, Walkable: true},
	}
	withEnemy := func(flag uint32) *protobuf.CURRENTCHAPTERINFO {
		return &protobuf.CURRENTCHAPTERINFO{
			CellList: []*protobuf.CHAPTERCELLINFO_P13{{
				Pos:      buildPos(chapterPos{Row: 1, Column: 2}),
				ItemType: proto.Uint32(chapterAttachEnemy),
				ItemId:   proto.Uint32(204010),
				ItemFlag: proto.Uint32(flag),
			}},
		}
	}
	start := chapterPos{Row: 1, Column: 1}
	behind := chapterPos{Row: 1, Column: 3}
	onEnemy := chapterPos{Row: 1, Column: 2}

	if path := findMovePath(grids, withEnemy(chapterCellActive), start, behind); path != nil {
		t.Fatalf("存活小怪挡路时不该有路径，得到 %v", path)
	}
	if path := findMovePath(grids, withEnemy(chapterCellDisabled), start, behind); len(path) != 3 {
		t.Fatalf("小怪被击沉后应该能走过去（3 格），得到 %v", path)
	}
	// 终点是怪格：允许（走过去开打）
	if path := findMovePath(grids, withEnemy(chapterCellActive), start, onEnemy); len(path) != 2 {
		t.Fatalf("应该能走到怪格上，得到 %v", path)
	}
}
