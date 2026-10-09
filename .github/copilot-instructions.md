# 本工作区规则

## 工具脚本一律用 Python

**不要写 PowerShell 脚本，新脚本一律 Python（3.11，见 `E:\Agent工作区\apkwork\py311\python.exe`）。**

原因（都是本仓库反复踩过的坑）：

- **引号被三层吃掉**：`powershell → adb → sh` 嵌套时 `$`、反引号、`&`、`;`、`>` 全被各层解释器吃一遍。内联写 `adb shell su -c "..."` 基本必错。
- **中文路径乱码**：PS 把参数传给 native 程序（adb / java / go）时编码丢失，`E:\Agent工作区\...` 变成 `E:\Agent宸ヤ綔鍖篭...`，报「路径不存在」。Python 直接传 `str`，没问题。
- **`$var:` 被当成盘符**：`"$n: int"` 报 *变量引用无效*，必须写 `${n}:`。
- **UTF-8 无 BOM 的 `.ps1` 按 GBK 读**：脚本里含中文就报莫名其妙的语法错。
- **`-Include` 必须配 `-Recurse`**、`select-string` 没有 `-Recurse`、管道给 wsl 会提前终止。

**Python 替代写法：**

```python
# 跑命令（含中文路径、复杂引号）
import subprocess
subprocess.run(["adb", "-s", "127.0.0.1:16448", "shell", "su", "-c", cmd], check=True)

# 多命令逻辑写进 .py，不要在内联字符串里堆
# 文件读写用 pathlib / open(..., encoding="utf-8")
```

**设备端**仍然用 `.sh`（adb 只能送 shell 脚本），但**推送与调用由 Python 驱动**。

## 设备

- MuMu 模拟器，`adb -s 127.0.0.1:16448`（端口 = `16384 + 32 × 实例号`）
- root 通过 KernelSU：`su -c <cmd>`
- 中文路径交给 adb 会乱码，**落盘一律用 ASCII 路径**（如 `C:\Users\niu\...`）

## 服务端

- Go 1.27.1：`E:\Agent工作区\apkwork\go-dist\go\bin\go.exe`
- 环境：`GOPROXY=https://goproxy.cn,direct`、`GOSUMDB=off`、`GOTOOLCHAIN=local`
- 构建与测试：`go build ./...`、`go test ./internal/answer/chapter/`

## 每轮测试后的改动必须给 diff

**规则**：一次测试（客户端连一次私服 / 跑一次导入 / 跑一次验收）之后，只要动了东西，
交付时必须同时给出**改动前后的 diff**。不许用「改好了 / 已修复 / 我改了 N 个文件」替代。

| 改动类型 | 怎么给 diff |
|---|---|
| 代码 | `git diff`（或 `git show --stat` + 关键 hunk） |
| 数据 / 库 | 改动**前后各导一份账号快照**，diff 两个 `.sql` |

数据 diff 的做法（归档是确定性的：连导两次只差 `-- generated` 一行，所以**纯文本 diff 就是数据 diff**）：

```powershell
$py = 'E:\Agent工作区\apkwork\py311\python.exe'
$acc = 'E:\Agent工作区\碧蓝航线离线版本\save\bili\accounts'
Newest = { Get-ChildItem $acc -Filter '*.sql' | Sort-Object LastWriteTime -Descending | Select-Object -First 1 }

# 1) 改库前
& $py 'E:\Agent工作区\碧蓝航线离线版本\tools\db-save-export.py'
Copy-Item (& $Newest).FullName state-before.sql -Force

# 2) ... 跑导入 / 改库 ...

# 3) 改库后
& $py 'E:\Agent工作区\碧蓝航线离线版本\tools\db-save-export.py'
Copy-Item (& $Newest).FullName state-after.sql -Force

# 4) 纯文本 diff
Compare-Object (Get-Content state-before.sql) (Get-Content state-after.sql)
```

逐列看某几列的值：`python tools\archive-peek.py <archive.sql> <table> [col ...]`

**为什么有这条**（2026-10-09 实测的教训，不是假想）：

给导入器加「写船装备槽」时，`owned_ships` 有 **28 列**而导入器只写 **18 列**；
`DELETE + INSERT` 把没写的那 10 列（含 `is_secretary`）**静默重置成默认值**
⇒ 秘书舰没了 ⇒ `PlayerInfo` 找不到秘书、**直接 return 且不发 `SC_11003`** ⇒ 客户端卡在加载页。

当时的三条"正常"信号全都在骗人：
- 服务端日志**零 ERROR**（除了那一行 WARN 级别的提示）；
- 导入器**报"成功"**，`exit=0`；
- **行数没变**：`owned_ships` 1211 → 1211。

⇒ **只看行数、只看日志，都看不见**。只有"改动前后的状态 diff"能看出"这一列被擦了"。
这也是为什么该规则要求 diff 到**列**，而不只是表行数。
