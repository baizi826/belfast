// cmd/schemadiff —— 用客户端自带的 protobuf 描述符（al-lua/CN/net/protocol/*_pb.lua）
// 对差服务端仓库里的全部 .proto，找出「结构不匹配」的地方。
//
// 为什么需要它：抓包只覆盖了 83 个请求，而客户端 LUA 描述符覆盖**全部**包号。
// 结构不一致 = 客户端收到后字段对不上（丢数据 / 解析失败 / UI 不刷新），
// 而这类 bug 靠肉眼读 handler 是看不出来的。
//
// 判定三类问题：
//   MISSING  客户端有这个字段号，我们没有  → 我们发不出 / 或解官方包时丢掉
//   EXTRA    我们有，客户端没有            → 可疑（多半是我们自己编的）
//   SHAPE    字段号一致但 label/类型类别不一致（required/repeated/varint/len…）
//
// 用法：
//
//	go run ./cmd/schemadiff -client E:\Agent工作区\apkwork\al-lua\CN\net\protocol -proto . -out report.txt
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type field struct {
	Name     string
	Label    string // optional | required | repeated
	Class    string // varint | 64bit | len | 32bit
	TypeName string
}

type message map[int]field // 字段号 -> 字段

var (
	luaAttrRe  = regexp.MustCompile(`\.(\w+_FIELD)\.(\w+)\s*=\s*(.+)$`)
	luaStrRe   = regexp.MustCompile(`^"(.*)"$`)
	protoMsgRe = regexp.MustCompile(`(?m)^\s*message\s+(\w+)\s*\{`)
	protoFldRe = regexp.MustCompile(`(?m)^\s*(optional|required|repeated)?\s*([\w.]+)\s+(\w+)\s*=\s*(\d+)\s*[;\[]`)
)

// 客户端 type 数字 → 线格式类别（protobuf FieldDescriptorProto.Type）
func classOfType(n int) string {
	switch n {
	case 1, 6, 16: // double, fixed64, sfixed64
		return "64bit"
	case 2, 7, 15: // float, fixed32, sfixed32
		return "32bit"
	case 3, 4, 5, 8, 13, 14, 17, 18: // int64, uint64, int32, bool, uint32, enum, sint32, sint64
		return "varint"
	case 9, 11, 12: // string, message, bytes
		return "len"
	}
	return "?"
}

func labelOf(n int) string {
	switch n {
	case 1:
		return "optional"
	case 2:
		return "required"
	case 3:
		return "repeated"
	}
	return "?"
}

// 我们 .proto 里的类型名 → 线格式类别
func classOfGoType(t string) string {
	switch t {
	case "string", "bytes":
		return "len"
	case "double", "fixed64", "sfixed64":
		return "64bit"
	case "float", "fixed32", "sfixed32":
		return "32bit"
	case "int32", "int64", "uint32", "uint64", "sint32", "sint64", "bool":
		return "varint"
	}
	if strings.HasPrefix(t, "enum.") {
		return "varint"
	}
	return "len" // 其它按 message 处理
}

// parseClient 解析客户端 *_pb.lua，得到 full_name → (msg, field) 以及每个字段的属性。
func parseClient(dir string) (map[string]message, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*_pb.lua"))
	if err != nil {
		return nil, err
	}
	// constKey -> 属性
	type attr struct {
		name, label, typ, fullName string
		number                    int
	}
	consts := map[string]*attr{}
	get := func(k string) *attr {
		if consts[k] == nil {
			consts[k] = &attr{}
		}
		return consts[k]
	}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			m := luaAttrRe.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			key, attribute, value := m[1], m[2], strings.TrimSpace(m[3])
			if s := luaStrRe.FindStringSubmatch(value); s != nil {
				value = s[1]
			}
			a := get(key)
			switch attribute {
			case "name":
				a.name = value
			case "number":
				if n, err := strconv.Atoi(value); err == nil {
					a.number = n
				}
			case "label":
				if n, err := strconv.Atoi(value); err == nil {
					a.label = labelOf(n)
				}
			case "type":
				if n, err := strconv.Atoi(value); err == nil {
					a.typ = classOfType(n)
				}
			case "message_type":
				a.typ = "len"
			case "full_name":
				a.fullName = value
			}
		}
		_ = fh.Close()
	}

	out := map[string]message{}
	for _, a := range consts {
		// full_name 形如 .p13.sc_13104.result / .p13.current_chapter.cell_list
		parts := strings.Split(strings.TrimPrefix(a.fullName, "."), ".")
		if len(parts) != 3 {
			continue
		}
		msgName := strings.ToUpper(parts[1])
		num := a.number
		if num <= 0 {
			continue
		}
		if out[msgName] == nil {
			out[msgName] = message{}
		}
		out[msgName][num] = field{Name: a.name, Label: a.label, Class: a.typ}
	}
	return out, nil
}

func main() {
	clientDir := flag.String("client", "", "客户端 *_pb.lua 目录")
	protoRoot := flag.String("proto", ".", "仓库根（递归找 .proto）")
	out := flag.String("out", "", "报告文件")
	onlyMissing := flag.Bool("missing-only", false, "只报 MISSING")
	flag.Parse()
	if *clientDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "需要 -client 和 -out")
		os.Exit(2)
	}

	client, err := parseClient(*clientDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析客户端描述符失败:", err)
		os.Exit(1)
	}

	ours := map[string]message{}
	_ = filepath.WalkDir(*protoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return nil
		}
		blob, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(blob)
		for _, loc := range protoMsgRe.FindAllStringSubmatchIndex(text, -1) {
			name := text[loc[2]:loc[3]]
			bodyStart := loc[1]
			depth, end := 1, len(text)
			for i := bodyStart; i < len(text); i++ {
				switch text[i] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						end = i
					}
				}
				if depth == 0 {
					break
				}
			}
			body := text[bodyStart:end]
			if ours[name] == nil {
				ours[name] = message{}
			}
			for _, fm := range protoFldRe.FindAllStringSubmatch(body, -1) {
				label := fm[1]
				if label == "" {
					label = "optional"
				}
				num, _ := strconv.Atoi(fm[4])
				ours[name][num] = field{Name: fm[3], Label: label, Class: classOfGoType(fm[2]), TypeName: fm[2]}
			}
		}
		return nil
	})

	names := make([]string, 0, len(client))
	for name := range client {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	counts := map[string]int{}
	for _, name := range names {
		cf := client[name]
		of, ok := ours[name]
		if !ok {
			counts["NO_FILE"]++
			fmt.Fprintf(&b, "[NO_FILE] %s（我们仓库里没有这个 .proto）\n", name)
			continue
		}
		var lines []string
		nums := make([]int, 0, len(cf))
		for n := range cf {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		for _, n := range nums {
			c := cf[n]
			o, exists := of[n]
			if !exists {
				counts["MISSING"]++
				lines = append(lines, fmt.Sprintf("  MISSING  #%d %s (%s,%s)", n, c.Name, c.Label, c.Class))
				continue
			}
			if o.Label != c.Label || o.Class != c.Class {
				counts["SHAPE"]++
				lines = append(lines, fmt.Sprintf("  SHAPE    #%d %s: 客户端(%s,%s) vs 我们(%s,%s,%s)",
					n, c.Name, c.Label, c.Class, o.Label, o.Class, o.TypeName))
			}
		}
		onums := make([]int, 0, len(of))
		for n := range of {
			onums = append(onums, n)
		}
		sort.Ints(onums)
		for _, n := range onums {
			if _, exists := cf[n]; !exists && !*onlyMissing {
				counts["EXTRA"]++
				lines = append(lines, fmt.Sprintf("  EXTRA    #%d %s (%s,%s,%s)", n, of[n].Name, of[n].Label, of[n].Class, of[n].TypeName))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "===== %s\n", name)
			for _, l := range lines {
				b.WriteString(l + "\n")
			}
		}
	}
	fmt.Fprintf(&b, "\n合计: %v   客户端消息数=%d 我们的消息数=%d\n", counts, len(client), len(ours))
	_ = os.WriteFile(*out, []byte(b.String()), 0o644)
	fmt.Printf("written %s (%v)\n", *out, counts)
}
