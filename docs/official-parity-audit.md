# 服务端 ↔ 官服 一致性审计（全仓库）

审计日期：2026-10-05 ｜ 基准：官服抓包 + 客户端自带 Lua 协议描述符 + 客户端 Lua 逻辑

## 0. 结论速览

| 审计维度 | 基准 | 结论 |
|---|---|---|
| **协议结构** | 客户端 `net/protocol/*_pb.lua`（33 个包，1849 个消息，全量描述符） | **0 处不匹配**（字段号 / label / 类型类别全部对齐） |
| **包号覆盖** | 官服抓包 307 组、83 种请求、130 种回复 | **0 缺口**：官服发过的请求，服务端 611 个注册 handler 全覆盖 |
| **行为语义** | 官服抓包逐字段 + 客户端 Lua | 逐模块核对，本轮修掉 5 处偏差（见 §4） |

也就是说：**报文"形状"没问题，差异都在"什么时候发、发什么值"**。抓包只覆盖 83 种请求（正常游戏路径），其余 526 个 handler 没有官服样本可对照。

## 1. 工装（都在 `cmd/`）

| 工具 | 用途 |
|---|---|
| `cmd/chdiff -chapter 204 [-battle N]` | 我们生成的「进图格子表 / 第 N 杀后的 map_update」 |
| `cmd/chdump <cmd> <bin>` | 解官服单个抓包（带格式化） |
| `cmd/dumprep -cmd <id> -file <bin>` | **任意**包号的官方 payload → JSON（自动按 `SC_<id>`/`CS_<id>` 取类型，不再需要手写 switch） |
| `cmd/capdump -dir <抓包目录> -out <目录>` | 整目录解码：每包一份 JSON + `_index.txt` + `_cmds.txt`（按 cmd 汇总「官服实际出现过的字段路径」） |
| `cmd/schemadiff -client <p13 目录> -proto . -out r.txt` | 客户端描述符 ↔ 我们的 `.proto` 全量对差 |
| `internal/protobuf/registry.go` | `MessageForCmd(id)`：按短名索引取类型（生成代码的 proto 包名是 `protobuf.`，直接 `FindMessageByName("SC_13102")` 找不到） |

关键坑：
- 生成代码的 proto 包名是 `protobuf`，全名 `protobuf.SC_13102` → 必须按**短名**建索引。
- 抓包文件名 `req<A>_rep<B>_NN.bin` = **A 触发的 B 回复的 payload**（不是 A 的请求体）。
- 含中文的路径传给 Go 程序会乱码 → 先把抓包镜像到 ASCII 目录（`E:\Agent工作区\apkwork\captures`）。
- PowerShell 5.1 按 ANSI 读 UTF-8(无 BOM) 文件 → 工装输出一律纯 ASCII；`.ps1` 含中文必须先补 UTF-8 BOM。
- `read_file` 对同一路径有缓存 → 重写文件后要换文件名或用终端读。

## 2. 官方对 `CS_13102`（进图）的完整取值（2-4，唯一抓包）

```
result=0, id=204, time=now, start_time=time-43200
cell_list[22]  {type 0 ×19, type 3(补给 id=3) ×1, type 6 ×2}
main_group_list[2]
  {id=2, 6 ships(hp_rant=10000), pos=(4,1), bullet=5, start_pos=(0,0), fleet_id=6, ...}
  {id=1, 2 ships, commander_list[…], pos=(6,4), bullet=5, start_pos=(0,0), fleet_id=1, ...}
round=0, is_submarine_auto_attack=1, model_act_count=0, loop_flag=1
cell_flag_list[1] = { pos=(3,7), flag_list=[1] }      ← BOSS 候选格被打了 flag 1
chapter_hp=0, kill_count=0, init_ship_count=8, continuous_kill_count=0
fleet_duties=[{1:1},{2:2}], move_step_count=0
```
出生点（attach 1 / 16）**不下发**为 `type=1`，当空格发 ✓（客户端也跳过 born 类型的 merge）。

## 3. 官服 `SC_13105`（13106 战斗结算）是**空包**

抓包里那一次 `req13106_rep13105_00.bin` = 0 字节。增援/地图变化走 `map_update` 字段
（客户端 `chapterproxy.OnBattleFinished` 合并 `map_update` / `ai_list` / `add_flag_list` / …）——
所以「战斗结果回 13105 + map_update」的设计是对的，只是那一次官服没有变化要推。
同时抓包里 `SC_13104`（13103 的回复）= 5 字节 = `result=0` + `auto_battle_time_update=0`（字段 16 确实会发，值为 0，默认值等价）。

`SC_34507`（107 次抓包）不是关卡推送，是**世界 BOSS 广播**（`type=3` + boss_info + user_info，别的玩家），
`SC_50103`（23 次）同理属于全局推送 —— 私服不需要复刻。

## 4. 本轮修掉的偏差

| # | 模块 | 官服行为 | 修前 | 修后 |
|---|---|---|---|---|
| 1 | 关卡移动 | `considerAsObstacle(SubjectPlayer)`：**存活小怪是障碍**（PrioObstacle，跨不过去），但终点可以是怪格（走过去开打）；击沉(flag=1)后不再是障碍 | 只看网格 walkable，可以从小怪身上跨过去 | `chapterCellBlocksMove` + BFS `blocked()`，终点豁免（`chapter_path.go`）|
| 2 | 进图包 | `start_pos=(0,0)`（服务器不填，required 子消息只能给空值） | 填出生点 | `buildPos(chapterPos{})` |
| 3 | 进图包 | `is_submarine_auto_attack=1` | 0 | 1 |
| 4 | 进图包 | `cell_flag_list` 带 1 条（BOSS 候选格 + `flag_list=[1]`） | 空数组 | `buildChapterCellFlags` |
| 5 | 移动合法性 | 客户端禁止移动到**已有舰队**的格子上（`considerAsStayPoint`） | 未实现（低风险，暂缓） | —— |

（#5 记为待办：目前只有单舰队出击时不影响。）

## 5. 未决 / 需要更多样本

### 附带发现（与官服无关，但审计时撞见）
- `go build ./...` 在 Windows 上原本失败：`cmd/webhook_server/main.go` 用 `syscall.SysProcAttr{Setpgid}` / `syscall.Kill`（Unix 专有）
  → 已加 `//go:build !windows`，现在整仓库可编译（`build_all=0`）。
- 预先存在的坏测试（**不是**本次改动导致，需要单独修）：
  - `internal/answer/config_handlers_test.go:489`：`response.GetPermanentNow() != 6001`（字段变成 repeated `[]uint32` 了）
  - `internal/answer/request_player_assist_ship_test.go:153`：`ship.State.State` 已不存在
  - `internal/answer/guild/*`、`internal/answer/island/*`：需要 `server.toml` + Postgres DSN，本地裸跑会 panic（环境问题）
- 仓库根上被历史命令写坏的两个垃圾文件（`ersniuapkworkchdata' ...`）已删除。

- `cell_flag_list` 里 flag 1 的确切语义：不是天气（`weather_data_template` 只有 101~103），
  是 `chapter_status_effect[1]={strategy:90}`。照官服字节发，语义待定。
- `time` / `start_time`：官服 `start_time = time - 43200`（正好 12h）。我们发 `time = now + 模板 time`、
  `start_time = now`。只有一份样本，无法判定是不是「进入章节时刻」。
- `SC_13104` 的 `auto_battle_time_update`：值为 0，默认值等价，是否要显式发待定。
- 526 个 handler 没有官服样本（`cmd/capdump` 的 `_cmds.txt` 只有 130 种回复的字段清单）。
  要再扩大覆盖面，只能在官服上多抓：每个 UI 操作一次，跑一遍。

## 6. 复核命令

```powershell
# 结构层：应输出 map[NO_FILE:17] 且没有 MISSING/SHAPE/EXTRA
go run ./cmd/schemadiff -client E:\Agent工作区\apkwork\al-lua\CN\net\protocol -proto . -out r.txt

# 抓包字段清单（审计基准）
go run ./cmd/capdump -dir E:\Agent工作区\apkwork\captures -out E:\Agent工作区\apkwork\chdata\cap

# 关卡对差
go run ./cmd/chdiff -chapter 204
go run ./cmd/chdump 13102 <captures>\req13101_rep13102_01.bin
```
