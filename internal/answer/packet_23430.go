package answer

import (
	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// goldResourceID is the owned_resources id for gold (see orm.Commander.HasEnoughGold).
const goldResourceID = uint32(1)

// HandleLegacy23430 answers CS_23430 (gold-supply store state) with a constructed
// SC_23431.
//
// History: this used to replay a captured official SC_23431 byte-for-byte, because
// nothing in the 23000-23999 range had a protobuf definition. That had two problems:
// the reply carried whatever that particular account had at capture time, so it was
// wrong for every other player; and the deployment could not come up correctly without
// a packet trace, which is not something a deliverable may depend on.
//
// SC_23431.proto now exists, so the reply is built rather than replayed: `gold` comes
// from the commander's own resources, the rest are explicit zeros.
//
// The captured message (that account, at capture time) looked like:
//
//	gold=7045298 buy_num=15 max_profit=1544354 acc_profit=133835
//	item_list=[78 ids in 7..120] acc_buy_price>0 pre_buy_state=1 pre_timestamp>0
//	match_time=0 is_forbidden=0 game_num=53 inactive_*=0 back_forbidden=0
//	acc_item_price>0 get_relief_num=1
//
// ponytail: item_list is left empty. Those are the supply store's goods, i.e.
// configuration, and the 23xxx range has no config table we have located yet.
// Ceiling: the store page lists no goods until it is filled in. Upgrade path: find the
// table behind the client's supply-store UI and populate item_list from it.
//
// Every field is `required` in proto2, so all of them are set explicitly - omitting
// one makes proto.Marshal fail and the client never gets its answer.
func HandleLegacy23430(buffer *[]byte, client *connection.Client) (int, int, error) {
	var request protobuf.CS_23430
	if err := proto.Unmarshal(*buffer, &request); err != nil {
		return 0, 23431, err
	}

	var gold uint32
	if client.Commander != nil {
		gold = client.Commander.GetResourceCount(goldResourceID)
	}

	response := protobuf.SC_23431{
		Gold:          proto.Uint32(gold),
		BuyNum:        proto.Uint32(0),
		MaxProfit:     proto.Uint32(0),
		AccProfit:     proto.Int32(0),
		ItemList:      []uint32{},
		AccBuyPrice:   proto.Uint32(0),
		PreBuyState:   proto.Uint32(0),
		PreTimestamp:  proto.Uint32(0),
		MatchTime:     proto.Uint32(0),
		IsForbidden:   proto.Uint32(0),
		GameNum:       proto.Uint32(0),
		InactiveNum:   proto.Uint32(0),
		InactiveState: proto.Uint32(0),
		BackForbidden: proto.Uint32(0),
		AccItemPrice:  proto.Uint32(0),
		GetReliefNum:  proto.Uint32(0),
	}
	return client.SendMessage(23431, &response)
}
