package protobuf

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var (
	cmdIndexOnce sync.Once
	cmdIndex     map[string]protoreflect.MessageType
)

// shortNameIndex 建立「短名 → 消息类型」索引。
// 生成代码的 proto 包名是 protobuf，全名是 protobuf.SC_13102，
// 所以直接 FindMessageByName("SC_13102") 找不到 —— 必须按短名查。
func shortNameIndex() map[string]protoreflect.MessageType {
	cmdIndexOnce.Do(func() {
		cmdIndex = map[string]protoreflect.MessageType{}
		protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
			cmdIndex[string(mt.Descriptor().Name())] = mt
			return true
		})
	})
	return cmdIndex
}

// MessageForCmd 按包号取消息类型，先试 SC_<id> 再试 CS_<id>（两者都存在时优先 SC_）。
func MessageForCmd(id int) (proto.Message, error) {
	suffix := "_" + strconv.Itoa(id)
	index := shortNameIndex()
	for _, prefix := range []string{"SC", "CS"} {
		if mt, ok := index[prefix+suffix]; ok {
			return mt.New().Interface(), nil
		}
	}
	return nil, fmt.Errorf("proto 里没有 SC_%d / CS_%d 的消息类型", id, id)
}

// MessageForName 按 "SC_13102" 这样的名字取消息类型。
func MessageForName(name string) (proto.Message, error) {
	if mt, ok := shortNameIndex()[name]; ok {
		return mt.New().Interface(), nil
	}
	return nil, fmt.Errorf("proto 里没有消息类型 %q", name)
}

// CmdList 返回已注册的包号（形如 13102），用于枚举/自检。
func CmdList(prefix string) []int {
	out := []int{}
	for name := range shortNameIndex() {
		if !strings.HasPrefix(name, prefix+"_") {
			continue
		}
		id, err := strconv.Atoi(name[len(prefix)+1:])
		if err == nil {
			out = append(out, id)
		}
	}
	return out
}
