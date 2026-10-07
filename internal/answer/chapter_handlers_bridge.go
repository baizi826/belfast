package answer

import (
	answerchapter "github.com/ggmolly/belfast/internal/answer/chapter"
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/protobuf"
)

type chapterTemplate = answerchapter.ChapterTemplate
type chapterPos = answerchapter.ChapterPos

const (
	chapterOpMove   = answerchapter.ChapterOpMove
	chapterOpAmbush = answerchapter.ChapterOpAmbush

	chapterAttachBoss         = answerchapter.ChapterAttachBoss
	chapterAttachElite        = answerchapter.ChapterAttachElite
	chapterAttachAmbush       = answerchapter.ChapterAttachAmbush
	chapterAttachEnemy        = answerchapter.ChapterAttachEnemy
	chapterAttachTorpedoEnemy = answerchapter.ChapterAttachTorpedoEnemy
	chapterAttachChampion     = answerchapter.ChapterAttachChampion
	chapterAttachBombEnemy    = answerchapter.ChapterAttachBombEnemy

	chapterCellActive   = answerchapter.ChapterCellActive
	chapterCellDisabled = answerchapter.ChapterCellDisabled
	chapterCellAmbush   = answerchapter.ChapterCellAmbush
)

func loadChapterTemplate(chapterID uint32, loopFlag uint32) (*chapterTemplate, error) {
	return answerchapter.LoadChapterTemplate(chapterID, loopFlag)
}

func findChapterCellAt(current *protobuf.CURRENTCHAPTERINFO, pos chapterPos) (int, *protobuf.CHAPTERCELLINFO_P13) {
	return answerchapter.FindChapterCellAt(current, answerchapter.ChapterPos(pos))
}

func parseEliteFleetFromState(state []byte) ([]*protobuf.FLEET_INFO, error) {
	return answerchapter.ParseEliteFleetFromState(state)
}

func setEliteFleetInState(state []byte, fleets []*protobuf.FLEET_INFO) ([]byte, error) {
	return answerchapter.SetEliteFleetInState(state, fleets)
}

func HandleChapterAction(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.HandleChapterAction(buffer, client)
}

func ChapterBaseSync(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.ChapterBaseSync(buffer, client)
}

func ChapterBattleResultRequest(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.ChapterBattleResultRequest(buffer, client)
}

func GetChapterDropShipList(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.GetChapterDropShipList(buffer, client)
}

// 自动作战（周回）：13012/13014/13016/13018。
func HandleChapterAutoStart(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.HandleChapterAutoStart(buffer, client)
}

func HandleChapterAutoClaim(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.HandleChapterAutoClaim(buffer, client)
}

func HandleChapterAutoUseTicket(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.HandleChapterAutoUseTicket(buffer, client)
}

func HandleChapterAutoBatch(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.HandleChapterAutoBatch(buffer, client)
}

// BuildChapterAutoBattleList 让 13001 的推送复用同一份编码。
func BuildChapterAutoBattleList(commanderID uint32) ([]*protobuf.CHAPTER_AUTO_BATTLE, error) {
	return answerchapter.BuildChapterAutoBattleList(commanderID)
}

func RemoveEliteTargetShip(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.RemoveEliteTargetShip(buffer, client)
}

func ChapterTracking(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.ChapterTracking(buffer, client)
}

func ChapterTrackingKR(buffer *[]byte, client *connection.Client) (int, int, error) {
	return answerchapter.ChapterTrackingKR(buffer, client)
}
