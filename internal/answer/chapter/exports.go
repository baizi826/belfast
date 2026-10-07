package chapter

import (
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

type ChapterTemplate = chapterTemplate
type ChapterPos = chapterPos

const (
	ChapterOpMove   = chapterOpMove
	ChapterOpAmbush = chapterOpAmbush

	ChapterAttachBoss         = chapterAttachBoss
	ChapterAttachElite        = chapterAttachElite
	ChapterAttachAmbush       = chapterAttachAmbush
	ChapterAttachEnemy        = chapterAttachEnemy
	ChapterAttachTorpedoEnemy = chapterAttachTorpedoEnemy
	ChapterAttachChampion     = chapterAttachChampion
	ChapterAttachBombEnemy    = chapterAttachBombEnemy

	ChapterCellActive   = chapterCellActive
	ChapterCellDisabled = chapterCellDisabled
	ChapterCellAmbush   = chapterCellAmbush
)

func LoadChapterTemplate(chapterID uint32, loopFlag uint32) (*ChapterTemplate, error) {
	return loadChapterTemplate(chapterID, loopFlag)
}

func FindChapterCellAt(current *protobuf.CURRENTCHAPTERINFO, pos ChapterPos) (int, *protobuf.CHAPTERCELLINFO_P13) {
	return findChapterCellAt(current, chapterPos(pos))
}

func ParseEliteFleetFromState(state []byte) ([]*protobuf.FLEET_INFO, error) {
	return parseEliteFleetFromState(state)
}

func SetEliteFleetInState(state []byte, fleets []*protobuf.FLEET_INFO) ([]byte, error) {
	return setEliteFleetInState(state, fleets)
}

// BuildEntryCells 生成「进图（battle 0）」时的整套格子表，对差工装（cmd/chdiff）用，
// 对应官服 SC_13102 的 current_chapter.cell_list。
func BuildEntryCells(template *ChapterTemplate) ([]*protobuf.CHAPTERCELLINFO_P13, error) {
	grids, err := parseChapterGrids(template.Grids)
	if err != nil {
		return nil, err
	}
	return buildChapterCellsWithBossPlan(grids, template, 0, chapterBossPlan{ChapterID: template.ID}), nil
}

// SimulateSpawnStep 从进图状态开始依次应用 1..battles 次刷怪步，返回**最后一步**新刷出的格子，
// 对应官服 SC_13105 的 map_update（官服只推新刷出来的怪，不推被击沉的格）。
func SimulateSpawnStep(template *ChapterTemplate, battles uint32) ([]*protobuf.CHAPTERCELLINFO_P13, error) {
	entry, err := BuildEntryCells(template)
	if err != nil {
		return nil, err
	}
	grids, err := parseChapterGrids(template.Grids)
	if err != nil {
		return nil, err
	}
	current := &protobuf.CURRENTCHAPTERINFO{Id: proto.Uint32(template.ID), CellList: entry}
	var changed []*protobuf.CHAPTERCELLINFO_P13
	for battle := uint32(1); battle <= battles; battle++ {
		step := chapterSpawnStepForBattle(template, battle)
		changed = applyChapterSpawnStep(current, grids, template, step, chapterBossPlan{ChapterID: template.ID})
	}
	return changed, nil
}
