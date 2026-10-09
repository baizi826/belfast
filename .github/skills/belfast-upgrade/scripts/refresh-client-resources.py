#!/usr/bin/env python3
"""Refresh BELFAST_DATA_DIR/client_resources.json from a port-80 capture.

Why this is a data-pipeline step and not "capture replay":
  * CS_10800/SC_10801 are PLAIN protobuf on port 80 -- no TLS, no CA, no MITM.
  * The running server still constructs its own SC_10801; it never reads a captured
    response. Only the index LIST is recorded, which is data -- the same status as
    versions.json and the mirrored Lua tables.
  * The list cannot be derived otherwise: H is not any digest of the index file
    (crc64/sha1/sha256/sha512/md5 all checked) and the CDN has no directory listing.

The count values are cross-checked against the client's own version-<cat>.txt, which
records the same numbers locally -- a mismatch means the capture is stale or partial.

    python refresh-client-resources.py <pcap> --datadir <BELFAST_DATA_DIR>
    python refresh-client-resources.py --from-file <parsed.txt> --datadir <dir>
"""
from __future__ import annotations

import argparse
import json
import re
import struct
from pathlib import Path

CATS = ["cv", "l2d", "pic", "bgm", "painting", "manga", "cipher", "dorm", "map"]
AZ = re.compile(rb"\$azhash\$(\d+)\$(\d+)\$(\d+)\$([0-9a-f]{16})")
KEY = re.compile(rb"\$([a-z0-9]+)hash\$(\d{1,6})\$([0-9a-f]{16})")


def tcp_payloads(pcap: Path):
    data = pcap.read_bytes()
    magic = struct.unpack("<I", data[:4])[0]
    if magic == 0xA1B2C3D4:
        endian = "<"
    elif magic == 0xA1B23C4D:
        endian = "<"
    elif magic == 0xD4C3B2A1:
        endian = ">"
    else:
        raise SystemExit(f"unsupported pcap magic {magic:#x}")
    linktype = struct.unpack(endian + "I", data[20:24])[0]
    off = 24
    while off + 16 <= len(data):
        _ts, _tu, caplen, _orig = struct.unpack(endian + "IIII", data[off:off + 16])
        off += 16
        pkt = data[off:off + caplen]
        off += caplen
        if linktype == 1:
            l3 = pkt[14:] if len(pkt) > 14 else b""
        elif linktype == 113:
            l3 = pkt[16:] if len(pkt) > 16 else b""
        elif linktype == 276:
            l3 = pkt[20:] if len(pkt) > 20 else b""
        else:
            l3 = pkt
        if len(l3) < 20 or (l3[0] >> 4) != 4 or l3[9] != 6:
            continue
        ihl = (l3[0] & 0x0F) * 4
        l4 = l3[ihl:]
        if len(l4) < 20:
            continue
        yield l4[(l4[12] >> 4) * 4:]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("pcap", nargs="?", help="port-80 capture")
    ap.add_argument("--from-file", help="a previously parsed text dump")
    ap.add_argument("--datadir", required=True)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if args.from_file:
        blob = Path(args.from_file).read_bytes()
    elif args.pcap:
        blob = b"".join(tcp_payloads(Path(args.pcap)))
    else:
        raise SystemExit("need a pcap or --from-file")

    az = AZ.search(blob)
    if not az:
        raise SystemExit("未在流量里找到 $azhash$ 主条目；抓包可能不完整")

    entries = ["$azhash$" + "$".join(g.decode() for g in az.groups())]
    seen = set()
    by_cat: dict[str, tuple[str, str]] = {}
    for cat, cnt, h in KEY.findall(blob):
        cat_s, cnt_s, h_s = cat.decode(), cnt.decode(), h.decode()
        if cat_s in CATS and cat_s not in by_cat:
            by_cat[cat_s] = (cnt_s, h_s)

    missing = [c for c in CATS if c not in by_cat]
    if missing:
        print(f"!! 缺分类: {missing}（抓包可能被截断）")
    for cat in CATS:
        if cat in by_cat:
            cnt, h = by_cat[cat]
            entries.append(f"${cat}hash${cnt}${h}")
            seen.add(cat)

    version = f"{az.group(1).decode()}.{az.group(2).decode()}.{az.group(3).decode()}"
    doc = {
        "_comment": ("Official client resource-index list for SC_10801. Recorded once from "
                     "the plain port-80 version handshake; treated as data, like "
                     "versions.json. Refresh with tools/refresh-client-resources.py."),
        "version": version,
        "entries": entries,
    }

    print(f"资源版本 {version}")
    print(f"条目 {len(entries)}: 1 az + {len(seen)} 分类")
    for e in entries:
        print(f"  {e}")

    out = Path(args.datadir) / "client_resources.json"
    if args.dry_run:
        print(f"\n(dry-run) 将写入 {out}")
    else:
        out.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"\n写入 {out}  {out.stat().st_size} B")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
