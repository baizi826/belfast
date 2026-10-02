package answer

import (
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"

	"google.golang.org/protobuf/proto"
)

func GetMetaProgress(buffer *[]byte, client *connection.Client) (int, int, error) {
	response := protobuf.SC_63315{
		Type: proto.Uint32(1),
	}
	// arg1 = META 船模板 id 全量表（官方同一时刻发 13 个 97xxxxx）。
	// 上游留空 ⇒ 客户端登录阶段整块跳过 META，连 CS_63317 都不发（实测）⇒ META/科研界面永远转圈。
	// ponytail: 发全量而非“已拥有”，因为客户端用它初始化界面；将来要做“只显示拥有的”再按 owned_ships 过滤。
	if ids, err := orm.ListMetaShipIds(); err == nil {
		response.Arg1 = ids
	}
	return client.SendMessage(63315, &response)
}
