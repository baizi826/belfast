package answer

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

// parseItemPairs decodes the `[[itemID, count], ...]` shape that equip_data_template uses for
// destory_item / trans_use_item / restore_item.
//
// An empty Lua table reaches the database as `{}` and not `[]` - 161 equipment rows carry
// destory_item = "{}" - while `{}` cannot unmarshal into [][]uint32. That one mismatch made every
// dismantle of those items answer with a generic failure, silently and with nothing in the log.
// The data pipeline cannot tell an empty array from an empty map, so the reader accepts both;
// anything else still has to be a list of pairs.
func parseItemPairs(raw json.RawMessage) (map[uint32]uint32, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("null")) {
		return map[uint32]uint32{}, nil
	}

	var pairs [][]uint32
	if err := json.Unmarshal(trimmed, &pairs); err != nil {
		return nil, err
	}

	out := make(map[uint32]uint32, len(pairs))
	for _, pair := range pairs {
		if len(pair) != 2 {
			return nil, errors.New("item pair must be [id, count]")
		}
		if pair[0] == 0 || pair[1] == 0 {
			continue
		}
		if uint64(out[pair[0]])+uint64(pair[1]) > math.MaxUint32 {
			return nil, errors.New("item pair overflow")
		}
		out[pair[0]] += pair[1]
	}
	return out, nil
}

// addTransUseItems accumulates one template row's cost list into dst.
func addTransUseItems(dst map[uint32]uint32, raw json.RawMessage) error {
	pairs, err := parseItemPairs(raw)
	if err != nil {
		return err
	}
	for itemID, count := range pairs {
		dst[itemID] += count
	}
	return nil
}
