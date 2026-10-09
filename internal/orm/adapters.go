package orm

import (
	"sort"
	"time"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

type BuildInfoPayload struct {
	Build      *Build
	PoolID     uint32
	BuildTime  uint32
	FinishTime time.Time
}

func ToProtoBuildInfo(payload BuildInfoPayload) *protobuf.BUILDINFO {
	finishTime := payload.FinishTime
	poolID := payload.PoolID
	if payload.Build != nil {
		finishTime = payload.Build.FinishesAt
		if payload.Build.PoolID != 0 {
			poolID = payload.Build.PoolID
		}
	}
	return &protobuf.BUILDINFO{
		Time:       proto.Uint32(payload.BuildTime),
		FinishTime: proto.Uint32(uint32(finishTime.Unix())),
		BuildId:    proto.Uint32(poolID),
	}
}

func ToProtoOwnedShip(ship OwnedShip, randomFlags []uint32, shadowSkins []OwnedShipShadowSkin) *protobuf.SHIPINFO {
	return toProtoOwnedShipWithRepairs(ship, randomFlags, shadowSkins, loadShipMetaRepairList(ship))
}

func toProtoOwnedShipWithRepairs(ship OwnedShip, randomFlags []uint32, shadowSkins []OwnedShipShadowSkin, repairs []uint32) *protobuf.SHIPINFO {
	equipInfo := buildEquipInfoList(ship)
	transformInfo := buildTransformInfoList(ship.Transforms)
	strengthInfo := buildStrengthInfoList(ship.Strengths)
	skinShadowList := buildSkinShadowList(shadowSkins)
	// 9.7 客户端把 SHIPINFO.state 拍平成 uint32（CN/EN/JP/KR/TW 五区描述符均 type=13，无 message_type），
	// 客户端里已不存在 SHIPSTATE 消息。原先嵌套的 StateInfo1..4 只作为 dorm 记账留在 DB，不上线。
	return &protobuf.SHIPINFO{
		Id:            proto.Uint32(ship.ID),
		TemplateId:    proto.Uint32(ship.ShipID),
		Level:         proto.Uint32(ship.Level),
		Exp:           proto.Uint32(ship.Exp),
		EquipInfoList: equipInfo,
		Energy:               proto.Uint32(ship.Energy),
		State:                proto.Uint32(ship.State),
		IsLocked:            proto.Uint32(boolToUint32(ship.IsLocked)),
		TransformList:       transformInfo,			SkillIdList:         buildSkillInfoList(ship.Skills),		Intimacy:            proto.Uint32(ship.Intimacy),
		Proficiency:         proto.Uint32(boolToUint32(ship.Proficiency)),
		StrengthList:        strengthInfo,
		CreateTime:          proto.Uint32(uint32(ship.CreateTime.Unix())),
		SkinId:              proto.Uint32(ship.SkinID),
		Propose:             proto.Uint32(boolToUint32(ship.Propose)),
		Name:                proto.String(ship.CustomName),
		ChangeNameTimestamp: proto.Uint32(uint32(ship.ChangeNameTimestamp.Unix())),
		MaxLevel:            proto.Uint32(ship.MaxLevel),
		CommonFlag:          proto.Uint32(boolToUint32(ship.CommonFlag)),
		ActivityNpc:         proto.Uint32(ship.ActivityNPC),
		MetaRepairList:      repairs,
		Spweapon:            nil,
		SkinShadowList:      skinShadowList,
		CharRandomFlag:      randomFlags,
	}
}

func loadShipMetaRepairList(ship OwnedShip) []uint32 {
	if ship.OwnerID == 0 || ship.ID == 0 {
		return nil
	}
	repairs, err := ListOwnedShipMetaRepairIDs(ship.OwnerID, ship.ID)
	if err != nil || len(repairs) == 0 {
		return nil
	}
	return repairs
}

func buildSkinShadowList(entries []OwnedShipShadowSkin) []*protobuf.KVDATA {
	if len(entries) == 0 {
		return nil
	}
	sorted := make([]OwnedShipShadowSkin, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ShadowID < sorted[j].ShadowID
	})
	result := make([]*protobuf.KVDATA, len(sorted))
	for i, entry := range sorted {
		result[i] = &protobuf.KVDATA{Key: proto.Uint32(entry.ShadowID), Value: proto.Uint32(entry.SkinID)}
	}
	return result
}

// buildSkillInfoList renders SHIPINFO.skill_id_list (field 10).
//
// The client draws a ship's skills straight from this list (SHIPSKILL: skill_id / skill_lv /
// skill_exp) and falls back to the ship's configured slots when it is empty - which is what made
// every skill on every ship show as a locked "可习得" placeholder with no way to upgrade it.
func buildSkillInfoList(skills []CommanderShipSkill) []*protobuf.SHIPSKILL {
	if len(skills) == 0 {
		return nil
	}
	sorted := make([]CommanderShipSkill, len(skills))
	copy(sorted, skills)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SkillPos < sorted[j].SkillPos })
	result := make([]*protobuf.SHIPSKILL, len(sorted))
	for i, entry := range sorted {
		result[i] = &protobuf.SHIPSKILL{
			SkillId:  proto.Uint32(entry.SkillID),
			SkillLv:  proto.Uint32(entry.Level),
			SkillExp: proto.Uint32(entry.Exp),
		}
	}
	return result
}

func buildTransformInfoList(transforms []OwnedShipTransform) []*protobuf.TRANSFORM_INFO {
	if len(transforms) == 0 {
		return nil
	}
	sorted := make([]OwnedShipTransform, len(transforms))
	copy(sorted, transforms)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TransformID < sorted[j].TransformID
	})
	result := make([]*protobuf.TRANSFORM_INFO, len(sorted))
	for i, entry := range sorted {
		result[i] = &protobuf.TRANSFORM_INFO{
			Id:    proto.Uint32(entry.TransformID),
			Level: proto.Uint32(entry.Level),
		}
	}
	return result
}

func buildStrengthInfoList(strengths []OwnedShipStrength) []*protobuf.STRENGTH_INFO {
	if len(strengths) == 0 {
		return nil
	}
	sorted := make([]OwnedShipStrength, len(strengths))
	copy(sorted, strengths)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StrengthID < sorted[j].StrengthID
	})
	result := make([]*protobuf.STRENGTH_INFO, len(sorted))
	for i, entry := range sorted {
		result[i] = &protobuf.STRENGTH_INFO{
			Id:  proto.Uint32(entry.StrengthID),
			Exp: proto.Uint32(entry.Exp),
		}
	}
	return result
}

func buildEquipInfoList(ship OwnedShip) []*protobuf.EQUIPSKIN_INFO {
	slotCount := uint32(3)
	if config, err := GetShipEquipConfig(ship.ShipID); err == nil {
		slotCount = config.SlotCount()
	}
	result := make([]*protobuf.EQUIPSKIN_INFO, slotCount)
	index := make(map[uint32]OwnedShipEquipment, len(ship.Equipments))
	for _, entry := range ship.Equipments {
		index[entry.Pos] = entry
	}
	for pos := uint32(1); pos <= slotCount; pos++ {
		entry, ok := index[pos]
		equipID := uint32(0)
		skinID := uint32(0)
		if ok {
			equipID = entry.EquipID
			skinID = entry.SkinID
		}
		result[pos-1] = &protobuf.EQUIPSKIN_INFO{
			Id:     proto.Uint32(equipID),
			SkinId: proto.Uint32(skinID),
		}
	}
	return result
}

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func ToProtoOwnedShipList(ships []OwnedShip, randomFlags map[uint32][]uint32, shadowSkins map[uint32][]OwnedShipShadowSkin) []*protobuf.SHIPINFO {
	ownerShips := make(map[uint32][]uint32)
	for _, ship := range ships {
		if ship.OwnerID == 0 || ship.ID == 0 {
			continue
		}
		ownerShips[ship.OwnerID] = append(ownerShips[ship.OwnerID], ship.ID)
	}
	repairsByShip := make(map[uint32][]uint32)
	for ownerID, shipIDs := range ownerShips {
		repairs, err := ListOwnedShipMetaRepairIDsByShips(ownerID, shipIDs)
		if err != nil {
			continue
		}
		for shipID, repairIDs := range repairs {
			repairsByShip[shipID] = repairIDs
		}
	}

	result := make([]*protobuf.SHIPINFO, len(ships))
	for i, ship := range ships {
		result[i] = toProtoOwnedShipWithRepairs(ship, randomFlags[ship.ID], shadowSkins[ship.ID], repairsByShip[ship.ID])
	}
	return result
}

func ToProtoDropInfoList(attachments []MailAttachment) []*protobuf.DROPINFO {
	result := make([]*protobuf.DROPINFO, len(attachments))
	for i, attachment := range attachments {
		a := attachment
		result[i] = &protobuf.DROPINFO{
			Type:   &a.Type,
			Id:     &a.ItemID,
			Number: &a.Quantity,
		}
	}
	return result
}

func ToProtoCompensationDropInfoList(attachments []CompensationAttachment) []*protobuf.DROPINFO {
	result := make([]*protobuf.DROPINFO, len(attachments))
	for i, attachment := range attachments {
		a := attachment
		result[i] = &protobuf.DROPINFO{
			Type:   &a.Type,
			Id:     &a.ItemID,
			Number: &a.Quantity,
		}
	}
	return result
}

func ToProtoProposeResponse(success bool) *protobuf.SC_12033 {
	result := proto.Uint32(0)
	if !success {
		result = proto.Uint32(1)
	}
	return &protobuf.SC_12033{
		Result: result,
		Time:   proto.Uint32(uint32(time.Now().Unix())),
	}
}
