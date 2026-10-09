#!/usr/bin/env python3
"""Extract the resource-index list from a plain port-80 capture.

SC_10801 is plain protobuf, so the `$...hash$...` entries are readable in the TCP
payload. Rather than reimplementing the framing, scan every payload for the literal
pattern -- it is unambiguous and cannot false-positive on other traffic.

    python parse-official-10801.py [--pcap <file>]
"""
from __future__ import annotations

import argparse
import re
import struct
from pathlib import Path

DEFAULT = Path(r"C:\Users\niu\blhx-capture\official80.pcap")
KEY = re.compile(rb"\$([a-z0-9]+)hash\$([0-9]{1,6})\$([0-9a-f]{16})")
AZ = re.compile(rb"\$azhash\$([0-9]+)\$([0-9]+)\$([0-9]+)\$([0-9a-f]{16})")
URL = re.compile(rb"https?://[\x21-\x7e]{10,120}")
APK = re.compile(rb"blhx_[0-9][\x21-\x7e]{5,80}\.apk")

CATS = ["cv", "l2d", "pic", "bgm", "painting", "manga", "cipher", "dorm", "map"]


def tcp_payloads(pcap: Path):
    data = pcap.read_bytes()
    magic = struct.unpack("<I", data[:4])[0]
    if magic == 0xA1B2C3D4:
        endian, nano = "<", False
    elif magic == 0xA1B23C4D:
        endian, nano = "<", True
    elif magic == 0xD4C3B2A1:
        endian, nano = ">", False
    else:
        raise SystemExit(f"unsupported pcap magic {magic:#x}")

    linktype = struct.unpack(endian + "I", data[20:24])[0]
    off = 24
    while off + 16 <= len(data):
        _ts, _tu, caplen, _orig = struct.unpack(endian + "IIII", data[off:off + 16])
        off += 16
        pkt = data[off:off + caplen]
        off += caplen
        # LINUX_SLL2=276, LINUX_SLL=113, EN10MB=1
        if linktype == 1:
            if len(pkt) < 14:
                continue
            ihl = (pkt[14] & 0x0F) * 4 if len(pkt) > 14 else 0
            l3 = pkt[14:]
        elif linktype == 113:
            l3 = pkt[16:] if len(pkt) > 16 else b""
        elif linktype == 276:
            l3 = pkt[20:] if len(pkt) > 20 else b""
        else:
            l3 = pkt
        if len(l3) < 20 or (l3[0] >> 4) != 4:
            continue
        ihl = (l3[0] & 0x0F) * 4
        if l3[9] != 6:      # TCP only
            continue
        l4 = l3[ihl:]
        if len(l4) < 20:
            continue
        doff = (l4[12] >> 4) * 4
        yield l4[doff:]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--pcap", default=str(DEFAULT))
    args = ap.parse_args()
    p = Path(args.pcap)
    if not p.exists():
        print(f"missing {p}")
        return 1

    blob = b"".join(tcp_payloads(p))
    print(f"pcap {p.stat().st_size} B -> tcp payload {len(blob)} B")

    az = AZ.search(blob)
    if az:
        mj, mn, bd, h = (x.decode() for x in az.groups())
        print(f"\n=== az 主条目 ===")
        print(f"  $azhash${mj}${mn}${bd}${h}")
        print(f"  -> 资源版本 {mj}.{mn}.{bd}")

    print("\n=== 分类条目 ===")
    found = {}
    for cat, cnt, h in KEY.findall(blob):
        found.setdefault(cat.decode(), set()).add((cnt.decode(), h.decode()))
    for cat in CATS:
        for cnt, h in sorted(found.get(cat, [])):
            print(f"  ${cat}hash${cnt}${h}")

    print("\n=== 其他上下文 ===")
    for u in sorted({x.decode() for x in URL.findall(blob)})[:12]:
        print(f"  URL: {u}")
    for a in sorted({x.decode() for x in APK.findall(blob)}):
        print(f"  APK: {a}")
    for trailer in (b"count-2", b"dTag-1"):
        print(f"  trailer {trailer.decode()}: {'YES' if trailer in blob else 'no'}")

    print("\n=== 与客户端本地对比 ===")
    print("  客户端 version.txt 报 9.7.395; 本地 count 见 version-<cat>.txt")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
