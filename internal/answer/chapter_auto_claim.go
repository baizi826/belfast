package answer

import (
	answerchapter "github.com/ggmolly/belfast/internal/answer/chapter"
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// HandleChapterAutoClaim answers CS_13014 —— 领取已完成的周回。
//
// 放在 answer 包而不是 chapter 包，是因为奖励要复用 applyCommanderExpGain
// （battle_session.go，含 user_level_config 的升级循环）。
//
// ponytail: drop_list / ticket_list / oil 仍为空 —— 掉落结算卡在"官方掉落权重"
// 这个盲区上（battle_session.go:653 同样是均匀随机的 TODO，没有官服样本可校准）。
// 天花板：玩家拿得到经验，拿不到掉落物。
// 升级路径：掉落权重解决后，在这里按 job.ConfigID 累计 drop_list。
func HandleChapterAutoClaim(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13014
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13015, err
	}
	jobs, err := answerchapter.TakeFinishedChapterAutoCommissions(client.Commander.CommanderID, payload.GetNum())
	if err != nil {
		return 0, 13015, err
	}
	if len(jobs) == 0 {
		// 没有到点的周回：客户端只读 result。
		response := protobuf.SC_13015{
			Result:                proto.Uint32(1),
			DropList:              []*protobuf.DROPINFO{},
			ChapterAutoTicketList: []*protobuf.CHAPTER_AUTO_TICKET{},
			Oil:                   proto.Uint32(0),
			Seconds:               proto.Uint32(0),
			WorldAp:               proto.Uint32(0),
			ClassExp:              proto.Uint32(0),
		}
		return client.SendMessage(13015, &response)
	}
	classExp := uint32(0)
	seconds := uint32(0)
	for _, job := range jobs {
		exp, err := answerchapter.ChapterAutoBaseExp(job.Type, job.ConfigID)
		if err != nil {
			return 0, 13015, err
		}
		classExp += exp
		seconds += job.CostTime
	}
	if err := applyCommanderExpGain(client, classExp); err != nil {
		return 0, 13015, err
	}
	response := protobuf.SC_13015{
		Result:                proto.Uint32(0),
		DropList:              []*protobuf.DROPINFO{},
		ChapterAutoTicketList: []*protobuf.CHAPTER_AUTO_TICKET{},
		Oil:                   proto.Uint32(0),
		Seconds:               proto.Uint32(seconds),
		WorldAp:               proto.Uint32(0),
		ClassExp:              proto.Uint32(classExp),
	}
	return client.SendMessage(13015, &response)
}
