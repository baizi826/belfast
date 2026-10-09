---
name: belfast-upgrade
description: 'Upgrade the AzurLane private server (belfast) to match the live official client, and keep it independent of captures: refresh the Lua data mirror, rebuild the belfast-data layout, reseed Postgres, rebuild protobuf when the client protocol changed, then verify data/protocol/behaviour parity. USE FOR: after an official game update (weekly Thursday maintenance); before any private-server validation; when data may be stale relative to the live client; when asked to 升级私服 / 同步官服版本 / 版本一致; when packet-level differences might be caused by our build being older than the client; when check-upgrade-coverage reports tables stuck on 9.6; when auditing whether the server depends on captured packet replays instead of building its own responses. DO NOT USE FOR: implementing a single missing packet handler; player save import/export; client APK work; offline/DNAT hijack setup.'
---

# Belfast 私服升级

把私服刷到与**当前官方客户端**同一版本。没有这一步，任何"私服 vs 官服"的对比结论都不可信。

## 核心原则

**"版本一致"不是一个数字，是三层各自可观测的事实：**

| 层 | 观测点 | 判据 |
|---|---|---|
| 数据 | `activity_template` 的 `mark` 最大值 | 必须 == 上游 |
| 协议 | `proto-drift.py` 的"仅客户端有" | 必须 == 0 |
| 行为 | `net-gaps.py` 的未实现 CS 数 | 逐步收敛，不阻塞升级 |

`mark` 是活动的发布日期（`YYYYMMDD`），所以"最新 mark"就是一个直接的"数据有多新"探针。

## 第一步永远是体检

```powershell
# 不传参 = 全量报告；退出码 1 表示有层落后
& 'E:\Agent工作区\apkwork\py311\python.exe' .\scripts\version-check.py
```

输出形如：

```
upstream (lua)             mark=20261008  <-- newest upstream
mirror (al-lua)            mark=20260924  <-- STALE
built (belfast-data-97)    mark=20260924  <-- STALE
database (config_entries)  mark=20260924  <-- STALE
```

**`mirror` 落后** ⇒ 镜像没跟上，跑数据升级。
**只有 `built`/`database` 落后** ⇒ 镜像已新，加 `-SkipMirror`。

已知陷阱：`check-upgrade-coverage.py` **答不了这个问题**。它是相对冻结的 9.6 基座算的，会在数据落后两周时照样报"586 张表已升到 9.7"。

## 数据升级（最常见）

一条命令，实际是 **7 步 + 2 个子步**：

```
1/7   镜像官方 Lua                 CN/sharecfg (627) + CN/sharecfgdata (35)
2/7   镜像 CN/gamecfg/{skill,dorm}
2b/7  镜像客户端其余树             CN/net（协议）+ CN/model + CN/mgr + CN/support + CN/fixed
                                  + EN/JP/KR/TW 的 net
2c/7  协议漂移检查                 ↓ 见下一节，结论行可能误报
3/7   重建 belfast 布局            Lua -> JSON，带形状守卫
4/7   生成 GameCfg/{skill,dorm}.json
5/7   覆盖率复查
6-7/7 重启 + 重导入 + 等 8018 + 打印行数
```

```powershell
powershell -ExecutionPolicy Bypass -File 'E:\Agent工作区\碧蓝航线离线版本\tools\upgrade-server.ps1'
```

分步开关：

```powershell
# 只刷数据不重启（镜像已新、或只想产出数据目录时用）
powershell -ExecutionPolicy Bypass -File '...\upgrade-server.ps1' -SkipReseed
# 只重启重导入（数据已重建时用）
powershell -ExecutionPolicy Bypass -File '...\upgrade-server.ps1' -SkipMirror
```

**绝对不要**把这个脚本 pipe 进 `Select-Object` / `Out-Null` —— reseed 的子进程会一直持有句柄，管道永远等不到 EOF 而挂死。**重定向到日志文件再 tail**：

```powershell
powershell -ExecutionPolicy Bypass -File '...\upgrade-server.ps1' -SkipReseed *> 'C:\Users\niu\blhx-capture\upgrade.log'
```

`-SkipReseed` 跑完只刷前三层，DB 仍落后 —— 这是正常的，判据会明确显示出来。

### 步骤 3 的正常噪声（不用管）

- `encoded in 9.7, no 9.6 fallback, left out` —— `barrage_template_N` / `bullet_template_N` / `weapon_property_N` 这类分片表，9.7 改了编码方式，我们不消费
- `9.7 形状与 9.6 不兼容（拆分/索引化），保留 9.6` —— 形状守卫按设计生效（`gametip` 长期如此）
- `已桥接 N 个改名表` / `沿用 9.7 的 xxx` —— 改名桥接与去重
- **`FAILED=0` 才是要看的那一行**

跑完重新体检，`mirror` / `built` 应追平 `upstream`，`database` 需 reseed 后才追平。

## 协议漂移（2c 那一步）

```powershell
& 'E:\Agent工作区\apkwork\py311\python.exe' 'E:\Agent工作区\碧蓝航线离线版本\tools\proto-drift.py'
```

**⚠️ 这一步的结论行会误报，必须自己看两个数：**

```
客户端消息数: 1852   服务端 .pb.go 消息数: 1918

仅客户端有: 0          ← 判据 A：必须 == 0
仅服务端有: 42         ← 不算漂移
字段不一致的消息: 0    ← 判据 B：必须 == 0

结论: 有漂移 ⇒ 必须重新生成协议并重编服务端   ← 按退出码算的，误报
```

那 42 个"仅服务端有"固定是两类：
- 上游遗留死消息 —— 客户端 9.7 已删（`SHIPSTATE` / `SHIP_IN_DROM` / `CS_70001…` / `SC_70000…`）
- 生成器给分区加的 `_KR` 变体 —— 工具只归一化 `_P\d+`，`_KR`/`_KR_TW` 漏了

**判据只看 `仅客户端有 == 0` 且 `字段不一致 == 0`。** 2026-10-09 实测就是这个状态（`0 / 0`），**协议没变，不需要重建**。

真有漂移（判据 A 或 B 非 0）才跑：

```powershell
powershell -ExecutionPolicy Bypass -File 'E:\Agent工作区\碧蓝航线离线版本\tools\rebuild-proto.ps1'
```

## 验收

数据升级后：

1. `scripts/version-check.py` → `mirror` / `built` 追平 `upstream`；`database` 在 reseed 后才追平
2. `check-upgrade-coverage.py` → 确认"仍是 9.6"的只有 `GameCfg/{buff,card,dungeon,story}.json` + `gametip.json`（都不从 lua 正常转换，属预期）
3. `db-counts.sh` → `config_entries` 行数不低于升级前
4. 起服后客户端能进主界面
5. **抽验目标活动确实进来了**（端到端）：

```powershell
& 'E:\Agent工作区\apkwork\py311\python.exe' 'E:\Agent工作区\apkwork\tools\peek-cfg.py' `
    'E:\Agent工作区\apkwork\belfast-data-97\CN\ShareCfg\activity_template.json' 1000061
```

协议升级后额外：

6. `proto-drift.py` → **判据 A/B 均为 0**（不是退出码）
7. `proto-audit.ps1` → 确认改动的 `.pb.go` 没有变成"没人引用"（曾有一轮 170 个改动 = A24/B25/C96/D0，一个都不能删）

## 客户端资源清单（SC_10801）—— 每次官方更新都要刷

**这是最容易被漏掉的一步**：数据（Lua 表）和第 80 端口的版本握手是**两套版本**，
升级了数据不等于客户端能过版本检查。

### 症状

客户端停在 **「更新提示 INFORM / Hash文件校验失败，是否重试？」**，底部「正在检查更新...」。
服务端日志**一行都不涨**（客户端根本没走到连网关）。

### 契约（实测，2026-10-09）

SC_10801 的 `f4` 是一串 CDN **索引 key**，客户端拿它**拼 URL**去下载索引文件：

```
$azhash$<major>$<minor>$<build>$<H>     ← 主包，带客户端 build 号
$<cat>hash$<count>$<H>                  ← 其余 9 类
        ↓
https://line{1,3,4}-patch-blhx.bilibiligame.net/android/hash/<key>
```

**`H` 不是任何摘要。** 对抓到的 6 份索引逐个验证过
（crc64ecma / crc32 / adler32 / md5 / sha1 / sha256 / sha512，含常见截断与字节序）**全部不匹配**；
官方响应的 `ETag` 才是 `md5(body)`。CDN 也**没有目录接口**（`/android/hash/` 返回 200 空体）。
→ **H 是官方生成的索引版本标签，只能记录，不能算。**
（这与 `versions.json` 里的《客户端 build 号》不是一回事。）

### 刷新步骤

`CS_10800` 走 **80 端口明文 protobuf**（**不是 HTTPS**）⇒ **不需要 CA、不需要 MITM**。

```powershell
# 1. 关劫持（让它连真官方）+ 抓 80
E:\Agent工作区\apkwork\py311\python.exe tools\capture-official-10801.py --wait 60
# 2. 提取清单，写进数据目录
E:\Agent工作区\apkwork\py311\python.exe tools\refresh-client-resources.py `
    C:\Users\niu\blhx-capture\official80.pcap --datadir E:\Agent工作区\apkwork\belfast-data-97
```

产出 `BELFAST_DATA_DIR/client_resources.json`：
```json
{"version":"9.7.395","entries":["$azhash$9$7$395$2a544b966565ede4","$cvhash$1480$dac1efd2859ad69c", ...]}
```

`misc.localHashes()`（`internal/misc/game_update_local.go`）优先读它，缺失才回退到按数据树自算。
**回退路径必失败** —— 自算的 H 在 CDN 上不存在。

### 交叉验证（必做）

客户端本地 `files/version-<cat>.txt` 存着同样的 count，**必须与抓到的对上**：

```sh
adb -s 127.0.0.1:16448 shell "cat /sdcard/Android/data/com.bilibili.azurlane/files/version-cv.txt"
# 期望 0.0.1480，与 $cvhash$1480$... 一致
```

对不上说明抓包过期或不完整（漏包会被截断成只有部分分类）。

### 为什么这不算"抓包回放"

交付的服务端**仍然自己构造** SC_10801（`internal/answer/update_packet.go`），
运行时不读任何抓包文件。抓到的只是**资源清单这份数据** —— 与 `versions.json`、
镜像的 Lua 表同级。抓包在这里的用途是**取证**，不是运行时依赖。

## 实测基线（2026-10-09）

`upgrade-server.ps1`（**含 reseed**）完整跑通，`FAILED=0`，数据 **20260924 → 20261008**：

```
                    升级前      升级后
upstream (lua)      20261008    20261008
mirror  (al-lua)    20260924    20261008
built   (belfast-data-97)
                    20260924    20261008
database            20260924    20261008   ← 四层一致，version-check exit=0
```

- 官方 CN 版本号：**9.7.395**（上一轮记录的 9.7.394）
- 镜像实下：sharecfg 48 个 / sharecfgdata 10 个 / gamecfg skill 3 个 / model 6 个 / fixed 1 个
- 协议漂移：`仅客户端有 0` / `字段不一致 0` ⇒ **协议未变，不需重建**
- 步骤 3 统计：`converted=40 skipped=568 fell_back_to_9.6=6 encoded_left_out=13 failed=0`
- 覆盖率：585 真 9.7 / 5 仍 9.6 / 44 新增
- reseed 后：`config_entries` **789,957 行 / 674 类**，`items` 3216，`ships` 4123，`skills` 6930
- 端到端抽验：`activity_template` id=1000061（type=174 拉斐尔）、`item_virtual_data_statistics` id=220004（单次建造券）**均已入库**
- **客户端资源清单**：`client_resources.json` 已从 80 端口明文抓包刷新，10 条全部与客户端
  `version-<cat>.txt` 对上；客户端进启动页，VER 显示 **9.7.395**

**`Updating Configs` 会长时间不打印**（实测 8+ 分钟）—— 这是正常的，629 张表要导。
判活方式：看 `belfast` 进程 CPU 是否在涨（`Get-Process belfast`），别据日志沉默判定卡死。

**踩过的坑**：`cfg-query.py` 的游戏内 id 在 **`key`** 列，`id` 只是自增 bigint。
用它查 `key` 才是对的，否则会报 `NOT FOUND` 而误判成升级失败。

## 交付标准：抓包是取证，不是运行时依赖

**抓包的本职是拿数据，不是拿响应。** 两个用途必须分开，别让"能跑通"把界线糊掉：

| 用途 | 性质 |
|---|---|
| 抓包 → 提取**账号数据**（船 / 装备 / 关卡进度 / 道具）→ 导入私服 | ✅ 正当。一次性数据迁移，包是取证素材，用完即弃 |
| 抓包 → 把**响应字节**存成文件、启动时回放 | ❌ 交付物不是"某次官服会话的字节" |
| 服务端**自己构造响应** | ✅ 这才是要交付的东西 |

判据：**交付物必须能脱离官服抓包独立运行。** 一个私服如果启动依赖"某次抓到的官服字节"，官方一改协议/哈希它当场失效，且无法自行恢复。

> 表里第一行（抓包 → 账号数据 → 导入私服）的完整流程：抓 → 解成 payload → 写进玩家表 → 验收，
> 见 skill **`belfast-save-import`**。本节只负责划界限。

### 历史债已清（2026-10-09）

`run-belfast-win.ps1` 曾设 `BELFAST_SC10801_FILE` / `BELFAST_SC23431_FILE` 回放两个抓包。
**两处现已改为服务端造包，回放文件与相关环境变量均已删除。** 若将来又看到它们出现，说明有人退回了旧做法。

| 曾回放 | 现在 | 实现位置 |
|---|---|---|
| `SC_10801`（版本清单） | **服务端构造**；资源清单读 `client_resources.json`（数据），不出网 | `internal/misc/game_update_local.go` |
| `SC_23431`（军需补给状态） | **从玩家数据构造**，16 个 required 字段全设 | `internal/answer/packet_23430.go` |

**`SC_10801` 的关键约束（协议契约，写错客户端就要求下载）：**

```
$azhash$<major>$<minor>$<build>$<hash>    仅有它带版本号
$<cat>hash$<count>$<hash>                 cv / l2d / pic / bgm / painting / manga
                                          / cipher / dorm / map，顺序是契约
count-2                                   尾部两个各出现一次
dTag-1
```

**`count` 和 `hash` 必须逐字来自官方**，不能在服务端算 —— 它们是 CDN 上索引文件的地址，
见上一节《客户端资源清单》。`build` 号也要与官方当前资源版本一致（≈ `versions.json` 的 CN 值，
但**以抓到的为准**）。

⚠️ **trailer 由 `answer.updateVersions` 追加，`misc.localHashes` 只产 `$...hash$...` 条目。**
两边都加会重复 —— 2026-10-09 实测踩过（`localHashes` 加了，`updateVersions` 又加一次）。

⚠️ **`localHashes` 的 per-category 值只对本地数据树做指纹**（`count` = 数据目录文件数）。
官服那 10 个值描述的是**客户端资源**（立绘/语音/动画），服务端没有这些文件，原理上算不出真值。
这套输出是"格式合法、逻辑自洽、可复现"，**不等于客户端一定接受** —— 见下方"未验证"。

### 验证造包（不需要客户端、不需要劫持）

```powershell
# 起服后
& 'E:\Agent工作区\apkwork\py311\python.exe' 'E:\Agent工作区\apkwork\tools\probe-8018.py' 10800
& 'E:\Agent工作区\apkwork\py311\python.exe' 'E:\Agent工作区\apkwork\tools\probe-8018.py' 23430
```

它会发真实 `CS_10800` / `CS_23430` 并把回包按 protobuf 解出来。期望看到：

```
f4: str '$azhash$9$7$<当前版本>$<16 hex>'
f4: str '$cvhash$<n>$<16 hex>'  …（9 类）
f4: str 'count-2'
f4: str 'dTag-1'
```

单元测试另有两层：

```powershell
# 合成小目录：形状 / 确定性 / 跟随数据变化 / 无数据目录
go test ./internal/misc/ -run 'LocalHashes|SplitVersion|TreeFingerprint'
# trailer 契约
go test ./internal/answer/ -run '^TestUpdateVersions'
# 真实数据目录（627 文件）上的形状，需显式指认目录
$env:BELFAST_REAL_DATA_DIR='E:\Agent工作区\apkwork\belfast-data-97'
go test ./internal/misc/ -run TestLocalHashesAgainstRealTree -v
```

### 已验证（2026-10-09）

**客户端接受这套指纹。** 完整成功链（logcat）：

```
BilibiliCallBackListerner:LoginSuccess(String)
connect to gateway - line1-login-bili-blhx.bilibiligame.net:80
user logined............ 1
BilibiliSdkMgr:OnGatewayLogined()
```

画面停在启动页：**VER 9.7.395 / 服务器【Belfast】点击更换 / PRESS TO START**。
日志里的网关域名正是我们 SC_10801 的 `f1`，证明客户端用的是我们造的包。

**关键前提是 `client_resources.json` 存在且为官方真实值**（见上一节）。
只有 `versions.json` 里那个 build 号是不够的 —— H 必须来自官方。

**判定时别把 `CheckUpdate` / `BundleWizard` 当拒绝信号** —— 每次启动都跑，
成功那次也有 100+ 条。真正的拒绝标志是 `Hash文件校验失败`。

一键验收：

```powershell
E:\Agent工作区\apkwork\py311\python.exe tools\run-accept-test.py --wait 90
```

## 陷阱

**改玩家状态数据前先跑形状检查。** 手写数据把客户端引到 `TechnologyDataTemplate.consume` 的 `{}` 底雷上，handler 抛错导致连接 reset，**连主界面都进不去**，只能回退：

```powershell
wsl -e bash -c 'bash ~/bhxtools/db-cfg-shape.sh'
```

形状族 bug 已出现 5 例（`chapterTemplate`/`SetKeyArgs`/`ShipBreakoutItems`/`TechnologyRows`/`BlueprintIDList`）。通用规则：**命名切片类型 + 非 `[` 开头当空**。

**CJK 路径三层吃引号。** `PowerShell → wsl → bash` 会把中文路径和引号一起吞掉。已建软链 `~/bhxtools`：

```powershell
# 对：CJK 只出现在脚本文件内部
wsl -e bash -c 'bash ~/bhxtools/xxx.sh'
# 错：CJK 直接上命令行
wsl -e bash -c 'bash /mnt/e/Agent工作区/.../xxx.sh'
```

**新脚本一律 Python**，不写 PowerShell（工作区规则）。Python 解释器固定 `E:\Agent工作区\apkwork\py311\python.exe`。Python 打印 CJK 前设 `$env:PYTHONIOENCODING='utf-8'`，否则 GBK 控制台 `UnicodeEncodeError`。

**PC 的 WLAN IP 是 DHCP 动态的**，每次测试前重新确认。旧 IP 的 DNAT 规则用 `-D` 删不掉（需完全匹配含 `--to-destination`），必须先 `offline-mode.sh off` 再按行号清掉所有 `owner UID match <uid>` 的 nat 规则。

**改 `bin/ctl.sh` 等模块内文件不能直接 adb push**：`/data/adb/modules/` 受 SELinux 保护，
`push`/`chmod`/`ls` 都会 `Permission denied`（但文件本身 755、可执行）。
走 `/data/local/tmp` 中转 + `su -c cp`。改 `module.prop` 会触发 KSU 的 update 流程需要重启，改 `bin/` 不用。

**别在 MuMu 上用 `adb reboot`** —— adbd 不会再起来，设备永久 `offline`，
而 `MuMuManager info` 仍报 `is_android_started: true`（连 MuMu 自带的 adb 也 offline）。
恢复：`MuMuManager.exe control -v 2 shutdown` → `control -v 2 launch`（约 27 秒起）。
脚本 `tools/mumu-relaunch.py`。

**Android 的 `/system/bin/sh` 是 mksh，不支持 `echo > /dev/tcp/host/port`**（bash 特性）——
用它做端口探测会永远报 `closed`。改用 `nc -w 2 <host> <port> </dev/null`
（toybox nc：通 = rc 0 且静默，不通 = rc 1 + `nc: Timeout`）。

**`iptables -S OUTPUT` 第一行是链头 `-P OUTPUT ACCEPT`**，所以 `grep -n` 的行号比
iptables 自己的规则序号**大 1**；按行号 `-D OUTPUT <n>` 必须先 `grep '^-A OUTPUT'` 过滤，
否则每次删除都静默失败。且 `iptables -L --line-numbers` 在该设备报 `Permission denied`，不能当 fallback。

**两个客户端族存档分开**：`save/uc/`（九游）与 `save/bili/`（B站）不可混放。两个都能做实验，哪个方便用哪个。

## 形状族之外：语义族（服务器发合法的 id，客户端照样崩）

形状族是"客户端解析不了"，语义族是"客户端能解析、但拿到 id 后去查自己那张表查不到"。
两者症状一模一样（`E/Unity ... LuaException`），但修法完全不同：形状族改**类型**，语义族改**过滤条件**。

**症状**：`LuaException: model/vo/Technology:0: attempt to index a nil value`，栈里带
`finishCondition` / `isCompleted` / `PlayerProxy:IsShowCommssionTip` / `NewMainSceneBaseTheme:OnLoaded`
—— 也就是**主界面红点注册阶段**挂掉，进不去主菜单。

**根因**：`TechnologyProxy:updateTechnologys` 这样建对象：

```lua
Technology.New({ id = slot11.id, time = slot11.time, pool_id = slot6.id })   -- 注意：没传 queue
```

没传 `queue` ⇒ `slot0.isQueue = nil`，于是 `finishCondition` 的短路失效：

```lua
slot0.finishCondition = function(slot0)
  if slot0.isQueue then return true end          -- nil，跳过
  return slot0:getConfig("condition") == 0
      or getProxy(TaskProxy):getTaskVO(<condition>):isFinish()   -- ← 崩在这
end
```

`technology_data_template` 里 `condition != 0` 的条目 = **有前置战役任务**（condition 就是
`task_data_template` 的 id）。私服没有任务系统 ⇒ `TaskProxy:getTaskVO(cond)` 返回 nil ⇒
`:isFinish()` 索引 nil。**773 条里 144 条带前置**，所以只要能抽到一条就崩。

**判别式**：`Technology:isCompleted = isFinish() and finishCondition()`，而
`isFinish() = isActivate() and time <= now`、`isActivate() = time > 0`。
所以**只有 `time`（= 下发时的 `finish_time`）已过期**才会触发。
`finish_time = 0` 的候选全被短路掉 —— 这解释了症状的间歇性：持久化的
`technology_research_states.refresh_pools` 里 5 个候选只有已启动过的那条有非零 `finish_time`。

**修法（两处，都在服务端）**：

```go
// internal/orm/technology_research_state.go — BuildTechnologyRefreshPools
// 只把无前置的放进刷新池（官服也这样）
if template.Condition != 0 { continue }

// internal/answer/technology/research_handlers.go — StartTechnologyResearch
// 再堵一次，防止手工构造请求启动前置未满足的项目
if template.Condition != 0 { return nil }
```

**必须顺带清持久化状态** —— `BuildTechnologyRefreshPools` 只在行**首次创建**时跑，
`RefreshTechnologyProjects` 见 `RefreshFlag != 0` 就直接返回，所以旧的坏候选会永久留在库里：

```powershell
E:\Agent工作区\apkwork\py311\python.exe tools\reset-tech-state.py   # DELETE FROM technology_research_states
```

**通用规则**：**服务器任何"从配置表里挑一批 id 下发给客户端"的地方，都要按配置的语义字段先过滤一遍。**
客户端的 `getConfig(x)` 拿不到东西时不会返回 nil 让你判空，它会直接去索引下一层。
SC_17001 的 `group_type` 是同一个家族的另一种形态（我们发 `ship_id/10`，客户端按
`ship_data_group.get_id_list_by_group_type` 的 **key** 集合索引，两者差 15 个 id）。

排查这类问题的工具（都在 `tools/`，纯 Python，走 WSL 里的 psql）：

| 脚本 | 作用 |
|---|---|
| `dump-tech-rows.py` | 逐行 dump `config_entries` 的 data JSON，统计字段名/缺失字段 |
| `check-tech-cond.py` | 核对已下发 id 的 `condition` 值 + 全表 `condition != 0` 清单 |
| `dump-tech-state.py` | 反序列化 `technology_research_states.refresh_pools`，看实际下发的 id |
| `reset-tech-state.py` | 清掉持久化的刷新池 |

**注意 psql 的 category 是带前缀的完整路径**（`ShareCfg/technology_data_template.json`），
不是表名。查错名字会得到 0 行，然后误判成"服务端走了 ErrNotFound 兜底"。


## 上游为什么没有可调用的工具

`ggmolly/belfast-data` 和 `AzurLaneTools/AzurLaneData` 都已停更（2026-05-08），belfast 本体没有数据 CI，AzurLaneTools 的生成器 `azex` 只以私有镜像发布。**唯一还活着的官方渠道是 `AzurLaneLuaScripts`（每日更新）**，而"Lua → belfast 布局"这一步上游从未公开 —— `gen-belfast-data.py` 就是我们的替代实现。

⇒ **上游更新后重跑升级脚本即可，不要手改表。**

## 资源

- `./scripts/version-check.py` — 四层版本对比，升级前/后的硬判据
- `./scripts/probe-8018.py` — 发真实 CS 请求并解回包，验证服务端自造响应
- `./references/tools.md` — 完整工具清单与典型用法
