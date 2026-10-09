package answer

import (
	"encoding/json"
	"testing"
)

// The data pipeline cannot tell an empty Lua table from an empty map, so destory_item /
// trans_use_item / restore_item may arrive as `{}`. It used to arrive only as `[]` in the
// parser's mind, and unmarshalling `{}` into [][]uint32 failed - which made every dismantle
// of those items answer with a generic failure and log nothing at all.
func TestParseItemPairsShapeTolerance(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    map[uint32]uint32
		wantErr bool
	}{
		{"empty object (the real pipeline output, 161 equipment rows)", "{}", map[uint32]uint32{}, false},
		{"empty array", "[]", map[uint32]uint32{}, false},
		{"json null", "null", map[uint32]uint32{}, false},
		{"absent field", "", map[uint32]uint32{}, false},
		{"whitespace around an empty object", " {} ", map[uint32]uint32{}, false},
		{"pairs", `[[17001,1],[17002,2]]`, map[uint32]uint32{17001: 1, 17002: 2}, false},
		{"duplicate ids accumulate", `[[17001,1],[17001,2]]`, map[uint32]uint32{17001: 3}, false},
		{"zero id or count is skipped", `[[0,5],[17001,0]]`, map[uint32]uint32{}, false},
		{"single-element pair is rejected", `[[17001]]`, nil, true},
		{"a map payload is rejected", `{"17001":1}`, nil, true},
		{"a scalar is rejected", `7`, nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseItemPairs(json.RawMessage(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("for %q: got %v, want %v", tc.in, got, tc.want)
			}
			for id, count := range tc.want {
				if got[id] != count {
					t.Fatalf("for %q: item %d = %d, want %d", tc.in, id, got[id], count)
				}
			}
		})
	}
}
