package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// chdump: decode a captured chapter packet so the map state can be compared
// against what this server builds. Usage: chdump <cmd> <file.bin> (13102/13104)
func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: chdump <cmd> <file.bin>")
		os.Exit(1)
	}
	cmd, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic(err)
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	switch cmd {
	case 13102:
		var m protobuf.SC_13102
		if err := proto.Unmarshal(data, &m); err != nil {
			panic(err)
		}
		cur := m.GetCurrentChapter()
		if cur == nil {
			fmt.Printf("result=%d current=nil\n", m.GetResult())
			return
		}
		fmt.Printf("result=%d chapter=%d cells=%d main=%d sub=%d support=%d escort=%d\n",
			m.GetResult(), cur.GetId(), len(cur.GetCellList()), len(cur.GetMainGroupList()),
			len(cur.GetSubmarineGroupList()), len(cur.GetSupportGroupList()), len(cur.GetEscortList()))
		types := map[uint32]int{}
		for _, c := range cur.GetCellList() {
			types[c.GetItemType()]++
			if c.GetItemType() != 0 {
				fmt.Printf("  cell (%d,%d) type=%d id=%d flag=%d data=%d\n",
					c.GetPos().GetRow(), c.GetPos().GetColumn(), c.GetItemType(),
					c.GetItemId(), c.GetItemFlag(), c.GetItemData())
			}
		}
		fmt.Println("  type counts:", types)
	default:
		fmt.Println("unsupported cmd")
	}
}
