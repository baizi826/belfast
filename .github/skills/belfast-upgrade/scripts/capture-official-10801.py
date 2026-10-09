#!/usr/bin/env python3
"""Capture the authoritative SC_10801 from the REAL official gateway.

Why this is legitimate and not "capture replay":
  * CS_10800/SC_10801 travel over port 80 as PLAIN protobuf -- no TLS, no CA, no MITM.
  * The shipped server still CONSTRUCTS its own SC_10801 packet. What we take from this
    capture is only the resource-index LIST (per-category count + H), which is data --
    the same status as versions.json and the mirrored Lua tables.
  * Without it the list cannot be derived: H is NOT any digest of the index file
    (verified exhaustively), and the CDN exposes no directory listing.

Procedure: hijack OFF -> tcpdump on port 80 -> launch client -> parse.

    python capture-official-10801.py [--serial 127.0.0.1:16448] [--wait 60]
"""
from __future__ import annotations

import argparse
import re
import subprocess
import time
from pathlib import Path

PKG = "com.bilibili.azurlane"
MOD = "/data/adb/modules/belfast-hijack"
PCAP = "/data/local/tmp/official80.pcap"
LOCAL = Path(r"C:\Users\niu\blhx-capture\official80.pcap")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--serial", default="127.0.0.1:16448")
    ap.add_argument("--wait", type=int, default=60)
    ap.add_argument("--iface", default="wlan0")
    ap.add_argument("--keep-pcap", action="store_true")
    args = ap.parse_args()
    adb = ["adb", "-s", args.serial]

    def sh(cmd: str, su: bool = False) -> str:
        pre = ["shell", "su", "-c"] if su else ["shell"]
        r = subprocess.run(adb + pre + [cmd], capture_output=True, text=True,
                           encoding="utf-8", errors="replace")
        return ((r.stdout or "") + (r.stderr or "")).rstrip()

    print("=== 1. 确保劫持是 off（走真官方）===")
    sh(f"sh {MOD}/bin/ctl.sh set off", su=True)
    n = sh("iptables -t nat -S OUTPUT | grep -c -- '--uid-owner'", su=True)
    print(f"  uid 规则: {n}")

    print("\n=== 2. 启动 tcpdump（只抓 80）===")
    sh(f"pkill tcpdump", su=True)
    time.sleep(1)
    # -s 0: full packets, we need the whole 637-byte SC_10801
    r = subprocess.Popen(adb + ["shell", "su", "-c",
                                f"tcpdump -i {args.iface} -s 0 -w {PCAP} 'tcp port 80'"],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(3)
    print("  " + (sh("pgrep -l tcpdump", su=True) or "!! tcpdump 未启动"))

    print("\n=== 3. 启动客户端 ===")
    sh(f"am force-stop {PKG}")
    time.sleep(2)
    sh(f"monkey -p {PKG} -c android.intent.category.LAUNCHER 1")

    print(f"\n=== 4. 等 {args.wait}s ===")
    for i in range(args.wait // 15):
        time.sleep(15)
        print(f"  [{(i+1)*15:>3}s] pid={sh(f'pidof {PKG}') or '-'}")

    print("\n=== 5. 停止 tcpdump ===")
    sh("pkill -INT tcpdump", su=True)
    time.sleep(2)
    print("  " + sh(f"ls -la {PCAP}", su=True))

    print("\n=== 6. 拉回本地 ===")
    r2 = subprocess.run(adb + ["pull", PCAP, str(LOCAL)], capture_output=True, text=True,
                        encoding="utf-8", errors="replace")
    print("  " + ((r2.stdout or "") + (r2.stderr or "")).strip().splitlines()[-1])
    if LOCAL.exists():
        print(f"  {LOCAL}  {LOCAL.stat().st_size} B")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
