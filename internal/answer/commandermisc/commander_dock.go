package commandermisc

import (
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
)

func CommanderDock(buffer *[]byte, client *connection.Client) (int, int, error) {
	response := protobuf.SC_12010{}
	// Send ships 100:
	var shipList []*protobuf.SHIPINFO
	if len(client.Commander.Ships) > 100 {
		shipSlice := client.Commander.Ships[100:]
		shipIDs := make([]uint32, len(shipSlice))
		for i, ship := range shipSlice {
			shipIDs[i] = ship.ID
		}
		flags, err := orm.ListRandomFlagShipPhantoms(client.Commander.CommanderID, shipIDs)
		if err != nil {
			return 0, 12010, err
		}
		shadows, err := orm.ListOwnedShipShadowSkins(client.Commander.CommanderID, shipIDs)
		if err != nil {
			return 0, 12010, err
		}
		shipList = orm.ToProtoOwnedShipList(shipSlice, flags, shadows)
	}

	// ⚠ 必须分包：12010 是"第 100 条之后的所有船"，官服账号有 1200+ 条 ⇒ 一个包
	// 90+ KB，而包头长度字段只有 **16 位**（GeneratePacketHeader 写 len>>8,len）
	// ⇒ 92959 被截成 27455 ⇒ 客户端按错长度切包 ⇒ 整条流错位 ⇒ 报
	// `inflating: unknown compression method` ⇒ 卡死登录（实测 2026-10-03）。
	// 官服本来就是拆成 12 包发的（每包 1030–14700 B），客户端按包追加。
	// 每包 100 条 ≈ 7.7 KB，安全且与官服量级一致。
	const shipsPerPacket = 100
	if len(shipList) == 0 {
		return client.SendMessage(12010, &response)
	}
	for start := 0; start < len(shipList); start += shipsPerPacket {
		end := min(start+shipsPerPacket, len(shipList))
		chunk := &protobuf.SC_12010{ShipList: shipList[start:end]}
		if _, _, err := client.SendMessage(12010, chunk); err != nil {
			return 0, 12010, err
		}
	}
	return 0, 12010, nil
}