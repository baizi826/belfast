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
