---
name: belfast-save-import
description: 'Load a real official AzurLane account into the belfast private server: capture the login burst on the device, extract the payloads, write them into the player tables, then verify the client can resolve every ship. USE FOR: 导入官服账号 / 导入存档 / 账号数据为空 / 登录 burst 一大堆空壳 / the dock holds a handful of ships while the capture is a max-level account; deciding whether a capture may be used at all; adding a table to the importer; the account works but the client cannot show a ship the server claims it owns. DO NOT USE FOR: upgrading the data mirror (use belfast-upgrade); replaying a captured reply as a server response (that is a deliverable bug - see belfast-upgrade); exporting or backing up a save; client APK work; offline/DNAT hijack setup.'
---

# Belfast 官服账号导入

把一个**真实官服账号**灌进私服。

## 为什么存在

上游没有存档导入。没有它，私服上每个新账号都是空的（15 条船 / 6 个道具），而官服抓包是满级号
（**实测 1211 艘船 / 533 道具 / 388 任务**）⇒ `CS_11001` 那 52 条应答一大半是空壳
⇒ 界面轮着卡。

**补 handler 收敛不了**：空壳不是"包没实现"，是"账号没数据"。
`SC_11300`(公告) / `SC_20001`(任务) / `SC_19001`(后宅) 这些 handler 早就实现了，
官服那一份几万字节、我们发出去只有几十字节甚至 0 —— 差别就是库里没有这个账号的东西。
正确顺序是**先有账号数据**，再谈界面。

## 铁律：抓包可以当数据源，不可以当应答

| 用途 | 判定 |
|---|---|
| 抓包 → 提取账号数据（船/装备/道具/任务）→ 写进玩家表 | ✅ **正当用途**。导入完就与抓包解耦 |
| 抓包 → 原样回放某个应答的字节 | ❌ 技术债，见 `belfast-upgrade` 的"回放已消除" |
| 服务端启动时依赖某个抓包 `.bin` 文件 | ❌ 交付品不能依赖它 |

一句话判据：**导入完成后删掉抓包，私服照样能起、账号照样在。**

## 管线（4 步，全在 PC 侧跑，只有第 1 步在设备侧）

```
capture ──→ reply_map ──→ state/*.bin ──→ importsave ──→ check-save-import
 设备侧         PC 侧          PC 侧          PC 侧            PC 侧
```

### 1. capture：抓官服登录 burst（设备侧）

```sh
sh /data/local/tmp/capture-bili-save.sh start
#   → 在游戏里把所有界面点一遍（船坞、图鉴、任务、科研、后宅…）
sh /data/local/tmp/capture-bili-save.sh stop
```

- 脚本本体 `tools/capture-bili-save.sh`（`#!/system/bin/sh`，推送到设备跑）：
  `tcpdump -i any -s 0 -w <file> 'tcp and not port 443'`，**排除 443** 去掉 HTTPS/CDN 噪声。
- 默认文件名带时间戳，**不会覆盖** `/sdcard/bili_save.pcap`（那是官服存档抓包原件）。
- ⚠️ **抓之前 `am force-stop` 另一个客户端**：两个客户端同时在跑时 `tcpdump -i any` 会把两账号流量
  混进同一份 pcap，差点得出"官服启动即连 8102"的错误结论（8102 是九游的）。
- 存档分区 `save/bili/`（B站官服）与 `save/uc/`（九游）**不可混放**，各族的 `reply_map` 只在本族内部 diff。
- 官服游戏网关是 **`:8018`，单条长连接 272 包** ⇒ 请求/应答配对无歧义，这就是后面能自动配对的底气。

### 2. pcap → reply_map → 纯 payload

```powershell
$py = 'E:\Agent工作区\apkwork\py311\python.exe'
$st = 'E:\Agent工作区\碧蓝航线离线版本\save\bili'
# a) 抽帧并按序配对（7 字节帧头 + zlib 在这一步剥掉）
& $py .\tools\bhx-pkt.py   "$st\bili_save.pcap" --port 8018 --map "$st\reply_map.save.json"
# b) 解码成纯 protobuf payload
& $py .\tools\bhx-mapdump.py --map "$st\reply_map.save.json" --out "$st\state"
```

产出 `req<请求cmd>_rep<应答cmd>_<序号>.bin` + 同名 `.txt`（可读解码）。

实测（这份存档抓包）：**307 个 payload / 130 种应答命令**（其中只有 5 种被导入器认），文件名如
`req11001_rep12001_11.bin`、`req11001_rep12010_12.bin`…`_23.bin`、`req11001_rep15001_37.bin`。

- ⚠️ 必须走 `reply_map`（按序配对表）。直接从 pcap 猜 payload 边界会把多包粘一起。
- 帧格式（`bhx-pkt.py` 头注释）：`len(2) | flag(1) | cmd(2) | idx(2)` + payload，big-endian，`flag>0` ⇒ zlib。

### 3. importsave：直连 DB 导入（权威路径）

```powershell
$go = 'E:\Agent工作区\apkwork\go-dist\go\bin\go.exe'
$d  = 'E:\Agent工作区\碧蓝航线离线版本\save\bili\state'
& $go run ./cmd/importsave -dir $d -commander 2890086143 `
      -dsn 'postgres://belfast@127.0.0.1:5432/belfast?sslmode=disable' -dry-run
```

`-dry-run` 把整个事务回滚 —— **先看数字对不对，再去掉它真写**。

实测输出（2026-10-09，这份抓包 / 这个账号）：

```
读入 307 个 payload；分组：items=1 ships=13 equips=1 tasks=1 ；未映射的包 125 种（不导入）
  items  533 条道具（另有 2 条 misc 未导入）
  ships  1211 条船（未导入：装备位 6055 / 技能 2376 / 强化 1450 / 改造 339 / 皮肤影 0）
  equips 151 条装备（另有 263 条特殊兵器未导入）
  tasks  388 条任务
```

**只认 5 个应答命令 → 4 张表**（`cmd/importsave/main.go` 的 `groupOf`）：

| 应答 cmd | 表 | 内容 |
|---|---|---|
| `15001` | `commander_items` | 道具（`item_list` + `limit_list` 合并）|
| `12001` + `12010` | `owned_ships` | 船坞（**两包都要**，见下）|
| `14001` | `owned_equipments` | 装备（按 `equipment_id` 聚合成 count）|
| `20001` | `commander_tasks` | 任务 |

**必须"按表分组"而不是"按包"**：`12001` 是第一批 100 艘，其余 12 份在 `12010` 里。若按包各做一次
"先清后插"，后一批会把前一批刚插的行删掉。导入器因此把同一张表的所有 payload 合并后再写一次。

其它保证：**单事务**（半个存档 = 坏账号）；`-keep` = 不清空（默认先清后插）；`-schema` 默认 `belfast`。
⚠️ 导入器报的数和库里的行数**可以不相等**：实测 `items` 导入 533 条、库里 535 条 ——
多出来的 2 条（`20001` 魔方、`15003` 快建）是测试脚本自己塞的，不是导入漏了。

### 4. check-save-import：客户端能不能解析（必跑）

```powershell
& $py .\.github\skills\belfast-save-import\scripts\check-save-import.py 2890086143
```

两条不变量，exit 1 = 有船客户端解析不了：每条 `owned_ships.ship_id` 都要在 `ships` 里有行，
且那行 `group_type` 非空。**第 2 条要重启过服务端才成立**（见下节）。

## ⭐ 三套编号：不搞清这个，导入必错

| 编号 | 是什么 | 例子 |
|---|---|---|
| `owned_ships.id` | 官服发的**实例唯一 id** | `12136171` |
| `owned_ships.ship_id` | **模板 id**（= `ships.template_id`）| `102284` / `101994` / `520024` |
| `ships.group_type` | **客户端图鉴/收藏的键** | `10126` |

```
owned_ships.ship_id  ──→  ships.template_id  ──→  ships.group_type
     （实例）                  （配置表）              （客户端要的）
```

- **绝不要用 `ship_id / 10` 近似 `group_type`**：只在"原型船"上巧合成立。改造船模板 id 以 4 结尾
  （`101994`），`/10 = 10199` 是客户端查不到的 id ⇒ `ShipGroup.__index` 抛异常 ⇒
  登录立绘检查挂掉（`LoginMediator:checkPaintingRes` → `CollectionProxy:getGroups`）⇒ **进不了主界面**。
  （官方应答 793/793 个 id 验证过都是 group_type。）
- **`ships.group_type` 不是导入器写的**：它由**服务端启动时** `misc.BackfillShipGroupTypes`
  从 `<BELFAST_DATA_DIR>/ship_group_types.json` 回填。
  ⇒ **导入完必须重启服务端**，否则新船没有 group_type。
- 映射文件由 `tools/gen-ship-group-types.py` 从 **Lua 镜像**生成（随数据升级刷新），**不是抓包产物**。
  实测：`ships` 4123 行里 3972 行有 group_type，151 个模板不在映射里（该账号没用到）。
- 同样的编号错还存在于**皮肤计数**：`skins.ship_group` 与 `group_type` 同一编号空间，
  拿 `ship_id / 10` 去 join 会静默丢掉所有改造船的皮肤。

## commander_id 是谁

belfast 用 **SDK uid** 当 commander_id：

- 库里的键 = `2890086143`（B站 SDK uid）
- 官服 `SC_11003` 里的 `uid` = `1211829360`（**不是**库里的键）

账号识别链是 `yostarus_maps`（SDK arg2 → commander_id）。导入必须挂到前者。
⚠️ 导入器**不校验 commander 是否存在** ⇒ 先确认账号能登录，否则数据挂在一个没人用的 id 上。

## 验收

1. `scripts/check-save-import.py` —— 两条不变量（推荐，exit code 可判）。
2. `tools/verify-import.sh` —— 各表行数 + 关键道具 + 科研蓝图与实例的一致性（应为 0 条缺实例）。
3. 客户端进游戏看**船坞 / 图鉴 / 科研**。

⚠️ 判据别搞错：**`Hash文件校验失败` 才是拒绝信号**。`CheckUpdate` / `BundleWizard`
每次都跑，成功那次也有 100+ 条 —— 别把它当失败。

现场基线（2026-10-09，账号 2890086143）：
`owned_ships 1211 / commander_items 535 / owned_equipments 151 / commander_tasks 388`，
不变量 "查不到的船 0，没有 group_type 的船 0"。

## 还没导入的（别以为导完了）

`cmd/importsave` 只覆盖 4 张表。抓包里这些仍然空着：

| 内容 | 来源 | 实测条数 |
|---|---|---|
| 装备槽 / 技能 / 强化 / 改造 / 皮肤影 | `SHIPINFO` 的嵌套字段 | 6055 / 2376 / 1450 / 339 / 0 |
| 特殊兵器 | `SC_14001.spweapon_list` | 263 |
| `item_misc_list` | `SC_15001` | 2 |
| 科研 / 喵窝 / 宿舍家具 / 活动收藏 / 关卡进度 | `63100` `63000` `19001` `17001` `13001` … | 另有 **125 种**应答命令未映射 |

**扩它 = 加 `groupOf` 一条 + 写一个按表的导入函数**（先清后插、合并在同一事务）。

⚠️ 不要"哪个屏卡就补哪个 handler"，那条路已经证过收敛不了。

## 另一条路：走 REST API（要装备槽就得用它）

`tools/import-dock.py` 走 belfast 自己的 REST（`http://127.0.0.1:2289`），**不碰数据库**：

```
POST  /api/v1/players/{id}/ships                          建船
PATCH /api/v1/players/{id}/ships/{owned_id}               补创建接口不收的字段
PATCH /api/v1/players/{id}/ships/{owned_id}/equipment     装备槽
```

- 该用它的时候：① 不能动 DB ② **要带装备槽**（直连 DB 的导入器不写槽位，这是唯一路径）。
- 1211 艘 ≈ 3600 个请求；支持 `--dry-run` / `--limit N` / `--emit-curl` / `--ids-only`。
- ⚠️ **必须先合并分批**：`--state` 默认就是 `req11001_rep12001_11.bin` + `req11001_rep12010_*.bin`。
  用 `--ids-only` 逐页比对船集，**漏一页 = 少 100 艘**。
- ⚠️ `2289` **不要**加进 DNAT / 防火墙 —— 导入脚本在 PC 本机跑。
- 它按字段号读 protobuf（`internal/protobuf/SHIPINFO.pb.go`），不做"这段 bytes 能不能当 message 解"的猜测 ——
  否则 `SHIPINFO.name`(17) 这种字符串会被误判成子消息。

## 已知坑

- **psql 内联 heredoc 必炸**（`PowerShell → wsl → bash` 三层，`$`/引号/反引号各被吃一遍）。
  DB 查询和改数据一律写 `.py`，用 `$SQL` 占位符 + `subprocess`（模式见 `tools/*.py` 与
  `scripts/check-save-import.py`）。
- **CJK 路径当实参**给 native 程序（Go/Java/psql）可能乱码 ⇒ 用 `$var = '...'` 变量插值或先 `Set-Location`。
- **`-dir` 指整个 `save/bili/state` 没问题**：导入器只认 `req*_rep*.bin`，`.txt` 和 `reply_map*.json` 自动忽略。
- **Postgres 在 WSL**，但 `-h 127.0.0.1` 从 **Windows 侧可达**（实测 `go run ./cmd/importsave` 与
  `go test` 都能直连）⇒ 不必进 WSL 跑导入。
- **重导入/reseed 不动玩家表**：`UpdateAllData` 只 **upsert 配置表**（`ships`/`items`/`skins`/…），
  `owned_ships` 这些玩家表原样保留。反过来，配置表里**新增**的船模板 `group_type` 是 NULL，靠启动回填补。
- 历史遗留的 `tools/import-official-save.sh` 是**旧的 WSL 路线**（同步源码 → `~/belfast` 里编译）。
  现在直接在 Windows 侧 `go run ./cmd/importsave` 即可，不要照抄那个脚本。
