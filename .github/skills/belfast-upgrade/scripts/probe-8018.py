#!/usr/bin/env python3
"""Send a real CS packet to a locally running belfast and decode the reply.

Verifies the server *builds* its answer, with no client, no hijack and no captured
replay file. That is the property that matters: a deliverable must not need a packet
trace to answer.

    python probe-8018.py 10800        CS_10800 (state=56, platform="1") -> SC_10801
    python probe-8018.py 23430        CS_23430 (arg=0)                -> SC_23431
    python probe-8018.py 10800 --raw  dump the reply without decoding

Wire format (7-byte header, verified against captures):
    u16 len   -- bytes that FOLLOW, so frame = 2 + len
    u8  flag  -- non-zero means the body is zlib-compressed
    u16 cmd
    u16 idx
    body
"""
from __future__ import annotations

import argparse
import socket
import struct
import sys
import zlib


def varint(value: int) -> bytes:
    out = bytearray()
    while True:
        b = value & 0x7F
        value >>= 7
        if value:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def encode_body(fields: list[tuple[int, str, object]]) -> bytes:
    """fields are (number, kind, value) with kind 'v' (varint) or 's' (string)."""
    out = bytearray()
    for number, kind, value in fields:
        if kind == "v":
            out += varint(number << 3)          # wire type 0
            out += varint(int(value))
        elif kind == "s":
            payload = value.encode() if isinstance(value, str) else value
            out += varint((number << 3) | 2)    # wire type 2
            out += varint(len(payload))
            out += payload
        else:
            raise ValueError(f"unknown field kind {kind!r}")
    return bytes(out)


def parse_body(body: bytes, indent: str = "    ") -> None:
    """Minimal protobuf walker - enough to read scalars, strings and nested messages."""
    i = 0
    while i < len(body):
        start = i
        key = shift = 0
        while i < len(body):
            b = body[i]
            i += 1
            key |= (b & 0x7F) << shift
            shift += 7
            if not b & 0x80:
                break
        number, wire = key >> 3, key & 7
        if wire == 0:
            value = shift = 0
            while i < len(body):
                b = body[i]
                i += 1
                value |= (b & 0x7F) << shift
                shift += 7
                if not b & 0x80:
                    break
            print(f"{indent}f{number}: varint {value}")
        elif wire == 2:
            length = shift = 0
            while i < len(body):
                b = body[i]
                i += 1
                length |= (b & 0x7F) << shift
                shift += 7
                if not b & 0x80:
                    break
            chunk = body[i:i + length]
            i += length
            if chunk and all(32 <= c < 127 for c in chunk):
                print(f"{indent}f{number}: str {chunk.decode()!r}")
            else:
                print(f"{indent}f{number}: bytes[{length}] {chunk[:48].hex()}")
        else:
            print(f"{indent}f{number}: wire {wire} (unhandled), from offset {start}")
            return


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", type=int)
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=8018)
    ap.add_argument("--idx", type=int, default=1)
    ap.add_argument("--timeout", type=float, default=8.0)
    ap.add_argument("--raw", action="store_true", help="skip body decoding")
    args = ap.parse_args()

    if args.cmd == 10800:
        payload = encode_body([(1, "v", 56), (2, "s", "1")])
    elif args.cmd == 23430:
        payload = encode_body([(1, "v", 0)])
    else:
        payload = b""

    frame = struct.pack(">HBHH", 5 + len(payload), 0, args.cmd, args.idx) + payload
    print(f"--> CS_{args.cmd}: {len(frame)} bytes  {frame.hex()}")

    with socket.create_connection((args.host, args.port), timeout=args.timeout) as sock:
        sock.sendall(frame)
        sock.settimeout(args.timeout)
        try:
            data = sock.recv(65535)
        except socket.timeout:
            print("!! no reply within timeout")
            return 1

    print(f"<-- {len(data)} bytes  {data.hex()}")
    if len(data) < 7:
        print("!! reply too short to hold a header")
        return 1

    length, flag, cmd, idx = struct.unpack_from(">HBHH", data, 0)
    replied = data[7:2 + length]
    print(f"    cmd={cmd} idx={idx} flag={flag} body={len(replied)} bytes")
    if flag:
        replied = zlib.decompress(replied)
        print(f"    (zlib -> {len(replied)} bytes)")

    if args.raw:
        return 0
    print("    --- decoded ---")
    parse_body(replied)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
