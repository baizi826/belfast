# 工具清单（升级链路）

> 完整索引在 `<WS>\tools\INDEX.md`（`<WS>` = `E:\Agent工作区\碧蓝航线离线版本`）。
> 本文件只列升级与验收直接相关的，以及本轮新增的。

## 路径常量

| 名称 | 值 |
|---|---|
| `<WS>` | `E:\Agent工作区\碧蓝航线离线版本` |
| 工作区 | `E:\Agent工作区\apkwork` |
| Python 3.11 | `E:\Agent工作区\apkwork\py311\python.exe` |
| Lua 镜像 | `E:\Agent工作区\apkwork\al-lua` |
| gamecfg 镜像 | `E:\Agent工作区\apkwork\al-gamecfg` |
| 9.6 基座 | `E:\Agent工作区\apkwork\belfast-data` |
| 9.7 输出 | `E:\Agent工作区\apkwork\belfast-data-97` |
| 私服源码 | `d:\KS\文档\VScode\belfast` |
| WSL 软链 | `~/bhxtools -> /mnt/e/Agent工作区/碧蓝航线离线版本/tools` |

## 数据升级

| 工具 | 平台 | 用途 |
|---|---|---|
| **`upgrade-server.ps1`** | PC/PS | ⭐ 一条命令跑完 7 步：镜像 Lua → 镜像 gamecfg → 重建布局 → 生成 GameCfg → 覆盖率复查 → 重启+重导入 → 等 8018 报行数。`-SkipMirror` / `-SkipReseed` 可分步 |
| `mirror-al-lua.ps1` | PC/PS | 镜像 `CN/sharecfg` + `CN/sharecfgdata`，按字节大小断点续传。`-RateKB` 默认 0（不限速）：实测无限速 655 KB/s vs `75k` 只有 50 KB/s |
| `mirror-al-gamecfg.py` | PC/Python3 | 镜像上游任意子树（`--base-path`）。WSL 连不上 github，必须 Windows 侧下 |
| `gen-belfast-data.py` | PC/Python3 | Lua → belfast 布局：文本扫描 → 失败回退执行式转换器（`lua2json-exec.py`，需 `lupa`）→ 9.6 形状守卫 → 同名过期副本清理 → 改名桥接。版本号由 `--cn-version` 传入 |
| `gen-gamecfg.py` | PC/Python3 | `CN/gamecfg/{skill,dorm}` → `GameCfg/skill.json`（7429 行）、`dorm.json`（48 行）。gamecfg 是朴素 `return {…}`，必须取块返回值 |
| `check-upgrade-coverage.py` | PC/Python3 | 覆盖率复查（按表名合并 `ShareCfg/` + `sharecfgdata/`）。⚠️ **只相对 9.6 基座算**，答不了"是否落后官方" |
| `run-belfast-win.ps1` | PC/PS | 起 belfast + gateway。⚠️ **不要加 `-NoNewWindow`**：子进程挂控制台，终端一收尾就被杀（实测两次导入半途消失） |
| `db-counts.sh` | WSL | 导入进度：`config_entries`/`categories`/`skills`/`ships`/`items` + GameCfg 各表行数 |

## 客户端资源清单（SC_10801）

**数据升级和第 80 端口的版本握手是两套版本**，升级了数据不等于客户端能过版本检查。
漏掉这步的症状：客户端卡在「Hash文件校验失败」，服务端日志一行不涨。

| 工具 | 平台 | 用途 |
|---|---|---|
| **`capture-official-10801.py`** | PC/Python3 | ⭐ 关劫持 → `tcpdump -i wlan0 'tcp port 80'` → 拉起客户端 → 拉回 pcap。`CS_10800` 是**明文 protobuf**，**不需要 CA/MITM** |
| `parse-official-10801.py` | PC/Python3 | 从 pcap 直接扫 `$<cat>hash$<n>$<H>`（不重实现分帧，模式无歧义） |
| **`refresh-client-resources.py`** | PC/Python3 | ⭐ 从 pcap 生成 `<datadir>/client_resources.json`，即 `misc.localHashes()` 优先读的那份数据 |
| `dump-client-version.py` | PC/Python3 | 读客户端 `files/version-<cat>.txt`（**交叉验证 count 必做**）与 `hashes*.csv` 概况 |
| `probe-cdn.py` | PC/Python3 | 探测官方 CDN：确认 H 是否被当路径键（伪造→404）、某版本索引是否存在 |
| `verify-h-is-filehash.py` | PC/Python3 | 证明 H **不是**文件摘要（crc64/crc32/adler/md5/sha1/sha256/sha512 × 截断字节序，全不匹配） |
| `hunt-device-hashlist.py` | PC/Python3 | 在设备上找现成的清单（`hashes*.csv`、`version-*.txt`、shared_prefs） |
| `run-accept-test.py` | PC/Python3 | ⭐ 一键验收：切 pdnat → 拉起客户端 → 扫 logcat 判定。⚠️ 别把 `CheckUpdate`/`BundleWizard` 当拒绝信号，每次启动都跑 |
| `dump-logcat.py` | PC/Python3 | 全量 logcat 按 tag 归类 + 关注行去重 |

## KSU 劫持模块

源码 `E:\Agent工作区\碧蓝航线离线版本\ksu-belfast-hijack\`，打包 `make-ksu-module.py`。
装机后可用「执行」按钮 / WebUI / adb 三处控制，配置存 `/data/adb/belfast-hijack.conf`（模块目录**外**）。

| 工具 | 平台 | 用途 |
|---|---|---|
| `test-ksu-module.py` | PC/Python3 | 推到 `/data/local/tmp/bh-mod` + `sh -n` 全脚本 + 规则增删 |
| `install-ksu-module.py` | PC/Python3 | push zip → `ksud module install` → 查 staging（`modules_update/`） |
| `verify-module-live.py` | PC/Python3 | 只看不改：合并状态 / 权限 / 配置 / 规则 / WebUI |
| `test-idempotent.py` | PC/Python3 | ⭐ 唯一可运行检查：3×apply + 3×cycle 必须恰好 5 条规则、无重复 |
| `mumu-relaunch.py` | PC/Python3 | ⭐ **重启设备的正确方式**。别用 `adb reboot`（MuMu 上 adbd 不再自启，永久 offline） |
| `diag-iptables.py` | PC/Python3 | 诊断 `-D OUTPUT <n>` 静默失败（链头行导致行号 +1） |

## 协议

| 工具 | 平台 | 用途 |
|---|---|---|
| **`proto-drift.py`** | PC/Python3 | ⭐ 查"客户端包定义 vs 服务端 `.pb.go`"漂移，不需要 protoc。**判据只看 `仅客户端有 == 0` 且 `字段不一致 == 0`**；退出码把"仅服务端有"也算漂移，那类多是上游死消息和 `_KR` 变体 |
| **`rebuild-proto.ps1`** | PC/PS | ⭐ 一条命令重建协议并重编：5 区 `net/` 进克隆 → `proto_from_lua.py`（**必须 `-X utf8`**，Lua 里有中文）→ WSL protoc → 停服 → 双目标重编 → 漂移验收 → 重新拉起 |
| `rebuild-proto-wsl.sh` | WSL | 上面那个的 WSL 半边。⚠️ protoc 必须解包到 `$HOME`（`/mnt/c` 没有可执行位）；⚠️ 版本必须与已提交 `.pb.go` 头部一致（`protoc-gen-go v1.36.11` / `protoc 33.1`），否则上千文件大 diff；⚠️ 必须写进**克隆**，`wsl-build-belfast.sh` 每次用克隆覆盖 `~/belfast` |
| **`proto-audit.ps1`** | PC/PS | ⭐ 把改动的 `.pb.go` 分四类：A 被手写代码引用 / B 只被其它生成物引用 / C 有 `.proto` 但代码没用（= 协议到了功能没做）/ D 纯孤儿。2026-10-02 实测 170 个改动 = A24/B25/C96/D0，**一个都不能删** |
| **`net-gaps.py`** | PC/Python3 | ⭐ 算"客户端会发、服务端不答"的 CS 命令差集。2026-10-02 基线 **653 vs 606 ⇒ 缺 53**，集中在 `23xxx` 整段 |
| `internal/tools/proto_from_lua.py` | 仓库内 | 从 `CN/net/protocol/*_pb.lua` 生成 `.proto`。字段名两边 UPPER_SNAKE 同名对应，但要按文件分别比（`p18`/`p61` 会同名不同义，故生成器加 `_P18`/`_P61` 后缀） |

## 行为对比（验证阶段用）

| 工具 | 平台 | 用途 |
|---|---|---|
| **`pkt-diff.py`** | PC/Python3 | ⭐⭐ 官服 pcap vs 我们 pcap，逐命令比：只在一边有（分 CS/SC，带长度）/ 长度不同 / 内容不同（打首个差异偏移 + 双方 dump）。⚠️ **大部分差集是"官服那次逛的界面更多"**，只能拿同一动作的对应片段比 |
| `bhx-pkt.py` | PC/Python3 | 碧蓝协议解码器：按 TCP 序号重组 → zlib 解压 → protobuf 打印。`--dump` / `--map` / `--raw PORT` |
| `cmd/pcap_decode`（仓库内 Go） | PC/Go | 用 gopacket + 真实 protobuf 解成 JSON。⚠️ 读不了 LINUX_SLL2 链路层，需先转 RAW IP |
| `bhx-mapdump.py` | PC/Python3 | 把 `reply_map` 应答解成可读文本 / bin。⚠️ map 里的 payload 已解压，别重复 zlib |

## 本轮新增

| 工具 | 位置 | 用途 |
|---|---|---|
| **`version-check.py`** | **`./scripts/`（本 skill 内，唯一权威）** | ⭐ 四层版本对比，升级前后的硬判据 |
| **`probe-8018.py`** | **`./scripts/`** | ⭐ 发真实 CS_10800 / CS_23430 到 8018 并按 protobuf 解回包；验证服务端**自造响应**，不需要客户端/劫持/抓包 |
| `pcap-to-rawip.py` | `E:\Agent工作区\apkwork\tools\` | LINUX_SLL2（276）→ RAW IP（101），喂给 gopacket 系工具 |
| `blhx-decode.py` | 同上 | 修正后的帧解码器。**关键**：`len` 是"后续字节数"不含自身 ⇒ 总帧长 `= 2 + len`、body `= len - 5` |
| `cfg-query.py` | 同上 | 查 WSL Postgres 里的 ShareCfg。**列名是 `category` / `key`**（`id` 只是自增主键）；表名形如 `ShareCfg/xxx.json`。子命令 `--tables` `--schema` `--head` `--like` `--find` `--sql` |
| `al-lua-peek.py` | 同上 | 直接拉 `AzurLaneLuaScripts` 任意表并按 id / 关键词定位，不用全量下载 |
| `al-core-groups.py` | 同上 | 按 `page_core` 分组列活动。"多选一复刻"的判据是 `page_core` 含 `CoreActivity` + `type=174` + `is_show=2` |
| `al-remaster-groups.py` | 同上 | 按 `title_res_tag` 的 `reN` 分组（**会数错期数**，仅作交叉参考） |

## 去回放（2026-10-09 完成）

| 曾回放 | 现状 | 实现位置 |
|---|---|---|
| `SC_10801` 版本清单 | 本地数据目录生成指纹，不出网 | `internal/misc/game_update_local.go`：`localHashes` / `treeFingerprint` / `splitVersion` / `shortHash` |
| `SC_23431` 军需补给 | 从玩家数据构造 16 个 required 字段 | `internal/answer/packet_23430.go` |
| trailer 拼接 | `localHashes` 只产 `$...hash$...`，`count-2`/`dTag-1` 由 `answer.updateVersions` 追加 | `internal/answer/update_packet.go` |

删除物：`sc10801.bin` / `sc23431.bin`、`BELFAST_SC10801_FILE` / `BELFAST_SC23431_FILE`、
`getGameHashes` 里的 `net.Dial` 官服分支、`packet_23430.go` 的 `os.ReadFile` 回放。

测试：`internal/misc/game_update_local_test.go`（形状/确定性/跟随数据/无目录）、
`internal/misc/game_update_realdata_test.go`（真实数据树，需 `BELFAST_REAL_DATA_DIR`）、
`internal/answer/update_packet_test.go`（trailer 契约与缓存）。

## 上游事实（2026-10-01 逐个查证，2026-10-09 复核）

- `ggmolly/belfast-data`：**手工**从 `AzurLaneData` 整理，停更于 2026-05-08；ggmolly 于 05-10 归档 belfast
- `AzurLaneTools/AzurLaneData`（JSON）：停更于 2026-05-08
- `AzurLaneTools/AzurLaneLuaScripts`：**"Will update automatically"，每日更新** ← 唯一活源
- belfast 本体无数据 CI（只有 `go.yml` / `release-cd.yml`），Makefile 无数据目标
- AzurLaneTools 的生成器 `azex` 只以私有镜像发布（`ghcr.io/azurlanetools/azex/azex` 匿名拉取 `UNAUTHORIZED`，PyPI 也没有）
- ⇒ **"Lua → belfast 布局"上游从未公开**，`gen-belfast-data.py` 是我们的替代实现

## 实测基线

| 日期 | `mark`（数据新鲜度） | 协议漂移 | 未实现 CS |
|---|---|---|---|
| 2026-10-02 | — | 0 / 0 | 53 |
| **2026-10-09** | 本地 **20260924** vs 官方 **20261008** | — | — |
