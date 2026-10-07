package chapter

import (
	"testing"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// Official captures send start_time = time - 43200 (exactly 12h).
// Two samples: seq115 CS_13101 -> SC_13102 { time: 1790746481, startTime: 1790703281 }.
func TestChapterStartTimeIsTimeMinus43200(t *testing.T) {
	template := &chapterTemplate{
		Time:  100,
		Grids: [][]any{},
	}
	payload := &protobuf.CS_13101{Id: proto.Uint32(204)}

	info, _, err := buildCurrentChapterInfo(template, payload, 0)
	if err != nil {
		t.Fatalf("buildCurrentChapterInfo failed: %v", err)
	}

	delta := int64(info.GetTime()) - int64(info.GetStartTime())
	if delta != chapterStartTimeOffset {
		t.Fatalf("start_time must trail time by %d (official), got delta=%d (time=%d startTime=%d)",
			chapterStartTimeOffset, delta, info.GetTime(), info.GetStartTime())
	}
	if info.GetStartTime() == 0 {
		t.Fatalf("start_time must be set (required field), got 0")
	}
}
