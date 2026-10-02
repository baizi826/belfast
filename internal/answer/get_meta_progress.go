package answer

import (
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/protobuf"

	"google.golang.org/protobuf/proto"
)

func GetMetaProgress(buffer *[]byte, client *connection.Client) (int, int, error) {
	response := protobuf.SC_63315{
		Type: proto.Uint32(1),
	}
	// arg1 = 玩家**已拥有**的 META 船模板 id。官方发的是 13 个 97xxxxx，那是该账号的持有列表，
	// 不是模板全集（97 段共 636 条；发全集实测 3182 B，反而更糟）。
	// 本账号 owned_ships 里没有 97 段船 ⇒ 空表就是正确答案。要显示 META 船时再按 owned_ships 过滤。
	return client.SendMessage(63315, &response)
}
