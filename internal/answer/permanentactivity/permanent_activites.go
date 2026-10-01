package permanentactivity

import (
	"encoding/json"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
)

type permanentActivity struct {
	ID uint32 `json:"id"`
}

func PermanentActivites(buffer *[]byte, client *connection.Client) (int, int, error) {
	entries, err := orm.ListConfigEntries("ShareCfg/activity_task_permanent.json")
	if err != nil {
		return 0, 11210, err
	}
	state, err := orm.GetOrCreateActivityPermanentState(client.Commander.CommanderID)
	if err != nil {
		return 0, 11210, err
	}
	// 9.7 客户端把 permanent_now 声明成 repeated（label=3）：标量发出去客户端解不出来，
	// 原先的 proto.Uint32(...) 在重生成后的类型上已经编不过。
	response := protobuf.SC_11210{
		PermanentActivity: make([]uint32, 0, len(entries)),
		PermanentNow:      []uint32{state.CurrentActivityID},
	}
	for _, entry := range entries {
		var activity permanentActivity
		if err := json.Unmarshal(entry.Data, &activity); err != nil {
			return 0, 11210, err
		}
		response.PermanentActivity = append(response.PermanentActivity, activity.ID)
	}
	return client.SendMessage(11210, &response)
}
