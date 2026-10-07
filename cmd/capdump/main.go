// cmd/capdump —— 把官服抓包目录整体解码成 JSON，并按「回复包号」汇总字段路径。
//
// 目的：审计服务端各模块时，用官服真实回包里的字段清单当基准，
// 逐条对照我们的 handler 有没有发同样的字段（少发的字段 = 客户端 UI 会错）。
//
// 用法：
//
//	go run ./cmd/capdump -dir E:\Agent工作区\apkwork\captures -out E:\Agent工作区\apkwork\chdata\cap
//
// 产物：
//
//	<out>/<name>.json    —— 单个抓包的解码结果（req + rep）
//	<out>/_index.txt     —— 每个抓包一行：name req rep 字段数
//	<out>/_cmds.txt      —— 按 cmd 汇总的字段路径清单（审计清单）
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/ggmolly/belfast/internal/protobuf"
)

var nameRe = regexp.MustCompile(`^req(\d+)_rep(\d+)_(\d+)\.bin$`)

// 官服会话记录 reply_map.save.json 的结构。
type rawReply struct {
	Cmd     int    `json:"cmd"`
	Idx     int    `json:"idx"`
	Flag    int    `json:"flag"`
	Payload string `json:"payload"`
}

type rawEntry struct {
	ReqCmd     int        `json:"req_cmd"`
	ReqIdx     int        `json:"req_idx"`
	ReqPayload string     `json:"req_payload"`
	Replies    []rawReply `json:"replies"`
}

type rawMap struct {
	Port int        `json:"port"`
	Seq  []rawEntry `json:"seq"`
}

// decodePayload 把 base64 的 payload 按指定包号解成 JSON，并返回字段路径集合。
func decodePayload(b64 string, id int, prefix string) (json.RawMessage, map[string]bool, string) {
	paths := map[string]bool{}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, paths, "base64 解不开: " + err.Error()
	}
	if len(raw) == 0 {
		return json.RawMessage(`{}`), paths, ""
	}
	msg, err := protobuf.MessageForName(prefix + "_" + strconv.Itoa(id))
	if err != nil {
		return nil, paths, err.Error()
	}
	if err := proto.Unmarshal(raw, msg); err != nil {
		return nil, paths, "解析失败: " + err.Error()
	}
	collectPaths(msg.ProtoReflect(), "", 0, paths)
	return json.RawMessage(protojson.MarshalOptions{EmitUnpopulated: false}.Format(msg)), paths, ""
}

// decodeReplyMap 解官服会话记录：**每条请求都带请求体**，能直接看出
// 「客户端发了什么参数 → 官服回了什么」——这是 .bin 抓包里没有的信息。
func decodeReplyMap(path, out string) {
	blob, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读 reply_map 失败:", err)
		os.Exit(1)
	}
	var rm rawMap
	if err := json.Unmarshal(blob, &rm); err != nil {
		fmt.Fprintln(os.Stderr, "解析 reply_map 失败:", err)
		os.Exit(1)
	}
	dir := filepath.Join(out, "pairs")
	_ = os.MkdirAll(dir, 0o755)

	var b strings.Builder
	fmt.Fprintf(&b, "# 官服会话：%d 个请求（端口 %d）\n\n", len(rm.Seq), rm.Port)
	distinct := map[int]bool{}

	for _, e := range rm.Seq {
		distinct[e.ReqCmd] = true
		reqJSON, reqPaths, reqErr := decodePayload(e.ReqPayload, e.ReqCmd, "CS")
		record := map[string]any{
			"req_cmd": e.ReqCmd,
			"req_idx": e.ReqIdx,
		}
		if reqErr != "" {
			record["req_error"] = reqErr
		} else {
			record["request"] = reqJSON
		}
		replies := []map[string]any{}
		for _, r := range e.Replies {
			item := map[string]any{"cmd": r.Cmd, "flag": r.Flag}
			rj, _, rErr := decodePayload(r.Payload, r.Cmd, "SC")
			if rErr != "" {
				item["error"] = rErr
			} else {
				item["json"] = rj
			}
			replies = append(replies, item)
		}
		record["replies"] = replies

		name := fmt.Sprintf("seq%03d_req%d.json", e.ReqIdx, e.ReqCmd)
		out2, _ := json.MarshalIndent(record, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, name), out2, 0o644)

		fmt.Fprintf(&b, "### seq %d  CS_%d\n", e.ReqIdx, e.ReqCmd)
		fmt.Fprintf(&b, "  请求字段: %s\n", sortedJoin(reqPaths))
		for _, r := range e.Replies {
			_, rPaths, rErr := decodePayload(r.Payload, r.Cmd, "SC")
			if rErr != "" {
				fmt.Fprintf(&b, "  ← SC_%d  (%s)\n", r.Cmd, rErr)
				continue
			}
			fmt.Fprintf(&b, "  ← SC_%d  flag=%d  字段: %s\n", r.Cmd, r.Flag, sortedJoin(rPaths))
		}
	}
	fmt.Fprintf(&b, "\n# distinct req_cmd = %d\n", len(distinct))
	_ = os.WriteFile(filepath.Join(out, "_pairs.txt"), []byte(b.String()), 0o644)
	fmt.Printf("reply_map: %d 个请求（%d 种命令），成对结果见 %s\n", len(rm.Seq), len(distinct), dir)
}

func sortedJoin(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func collectPaths(msg protoreflect.Message, prefix string, depth int, out map[string]bool) {
	if depth > 4 {
		return
	}
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		path := string(fd.Name())
		if prefix != "" {
			path = prefix + "." + path
		}
		switch {
		case fd.IsMap():
			out[path+"{map}"] = true
		case fd.IsList():
			out[fmt.Sprintf("%s[%d]", path, v.List().Len())] = true
			if fd.Kind() == protoreflect.MessageKind && v.List().Len() > 0 {
				collectPaths(v.List().Get(0).Message(), path+"[]", depth+1, out)
			}
		case fd.Kind() == protoreflect.MessageKind:
			collectPaths(v.Message(), path, depth+1, out)
		default:
			out[path] = true
		}
		return true
	})
}

func main() {
	dir := flag.String("dir", "", "抓包目录（req<A>_rep<B>_NN.bin，只有回复体）")
	out := flag.String("out", "", "输出目录")
	replyMap := flag.String("replymap", "", "官服会话记录 reply_map.save.json（含请求体，比 .bin 多一层信息）")
	flag.Parse()
	if *out == "" || (*dir == "" && *replyMap == "") {
		fmt.Fprintln(os.Stderr, "需要 -out，以及 -dir 或 -replymap 之一")
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}
	if *replyMap != "" {
		decodeReplyMap(*replyMap, *out)
	}
	if *dir == "" {
		return
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读目录失败:", err)
		os.Exit(1)
	}

	index := []string{}
	// cmd -> 字段路径 -> 出现次数
	byCmd := map[int]map[string]int{}
	// cmd -> 抓包个数
	cmdCount := map[int]int{}
	sort.Strings(index)

	for _, e := range entries {
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		req, _ := strconv.Atoi(m[1])
		rep, _ := strconv.Atoi(m[2])
		raw, err := os.ReadFile(filepath.Join(*dir, e.Name()))
		if err != nil {
			continue
		}

		record := map[string]any{}
		paths := map[string]bool{}

		// .bin 存的就是这个回复包的 payload（req<A>_rep<B>_NN = A 触发的 B 回复）。
		rm, err := protobuf.MessageForCmd(rep)
		if err != nil {
			record["missing_type"] = "SC_" + strconv.Itoa(rep)
		} else if err := proto.Unmarshal(raw, rm); err != nil {
			record["decode_error"] = err.Error()
		} else {
			record["type"] = "SC_" + strconv.Itoa(rep)
			// 用 RawMessage 嵌进去，避免整块 JSON 变成一层转义字符串不好读。
			record["json"] = json.RawMessage(protojson.MarshalOptions{EmitUnpopulated: false}.Format(rm))
			collectPaths(rm.ProtoReflect(), "", 0, paths)
		}

		name := strings.TrimSuffix(e.Name(), ".bin")
		blob, _ := json.MarshalIndent(record, "", "  ")
		_ = os.WriteFile(filepath.Join(*out, name+".json"), blob, 0o644)

		if byCmd[rep] == nil {
			byCmd[rep] = map[string]int{}
		}
		cmdCount[rep]++
		for p := range paths {
			byCmd[rep][p]++
		}
		index = append(index, fmt.Sprintf("%s\treq=%d\trep=%d\tfields=%d", name, req, rep, len(paths)))
	}

	sort.Strings(index)
	_ = os.WriteFile(filepath.Join(*out, "_index.txt"), []byte(strings.Join(index, "\n")+"\n"), 0o644)

	cmds := make([]int, 0, len(byCmd))
	for c := range byCmd {
		cmds = append(cmds, c)
	}
	sort.Ints(cmds)
	var b strings.Builder
	for _, c := range cmds {
		fmt.Fprintf(&b, "##### SC_%d (captures=%d)\n", c, cmdCount[c])
		keys := make([]string, 0, len(byCmd[c]))
		for k := range byCmd[c] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n := byCmd[c][k]
			mark := ""
			if n < cmdCount[c] {
				mark = fmt.Sprintf("   (only in %d/%d captures)", n, cmdCount[c])
			}
			fmt.Fprintf(&b, "  %s%s\n", k, mark)
		}
		b.WriteString("\n")
	}
	_ = os.WriteFile(filepath.Join(*out, "_cmds.txt"), []byte(b.String()), 0o644)
	fmt.Printf("processed %d captures, %d reply cmd types, output=%s\n", len(index), len(cmds), *out)
}
