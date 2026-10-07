// cmd/dumprep —— 把抓包里的某个 rep<cmd> payload 解成 JSON。
// 用途：拿官服的真实响应当「正确值」的对照（例如 SC_11003 的 registerTime / guideIndex），
// 而不是靠猜。
//
// 用法：go run ./cmd/dumprep -file rep11003.bin -cmd 11003 [-out out.json]
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/ggmolly/belfast/internal/protobuf"
)

func main() {
	file := flag.String("file", "", "纯 protobuf payload（抓包 .bin）")
	cmd := flag.String("cmd", "", "包号，如 11003")
	out := flag.String("out", "", "结果写到这里（UTF-8）；留空则打印到 stdout")
	flag.Parse()

	id, err := strconv.Atoi(*cmd)
	if err != nil || *file == "" {
		fmt.Fprintln(os.Stderr, "需要 -file 和 -cmd <包号>")
		flag.PrintDefaults()
		os.Exit(2)
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读文件失败:", err)
		os.Exit(1)
	}

	var m proto.Message
	m, err = protobuf.MessageForCmd(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := proto.Unmarshal(data, m); err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		os.Exit(1)
	}
	b, err := protojson.MarshalOptions{Indent: "  ", EmitUnpopulated: true}.Marshal(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "转 JSON 失败:", err)
		os.Exit(1)
	}
	if *out != "" {
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写文件失败:", err)
			os.Exit(1)
		}
		fmt.Printf("写到 %s（%d 字节，payload %d 字节）\n", *out, len(b), len(data))
		return
	}
	fmt.Println(string(b))
}
